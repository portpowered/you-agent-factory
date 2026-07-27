package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimejavascript "github.com/portpowered/infinite-you/pkg/services/factory_runtime/javascript"
	orchestration "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/internal/service"
)

const validInlineWorkflowSource = `
workflow.final("ok");
`

func TestCompileSelectsPetriKindForLegacyDefinition(t *testing.T) {
	t.Parallel()

	compiler := internalservice.New(testIDGenerator(), testJavaScriptWorkflows(), testJavaScriptWorkflows())
	result, err := compiler.Compile(context.Background(), orchestration.CompileRequest{
		Config: minimalPetriFactoryConfig(),
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if result.Kind != orchestration.KindPetri {
		t.Fatalf("Kind = %q, want PETRI", result.Kind)
	}
	if result.Binding == nil || result.Binding.OrchestrationKind() != orchestration.KindPetri {
		t.Fatalf("Binding = %#v, want non-nil PETRI binding", result.Binding)
	}
}

func TestCompileSelectsJavaScriptKindForInlineWorkflow(t *testing.T) {
	t.Parallel()

	compiler := internalservice.New(testIDGenerator(), testJavaScriptWorkflows(), testJavaScriptWorkflows())
	result, err := compiler.Compile(context.Background(), orchestration.CompileRequest{
		Config: minimalJavaScriptFactoryConfig(validInlineWorkflowSource),
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if result.Kind != orchestration.KindJavaScript {
		t.Fatalf("Kind = %q, want JAVASCRIPT", result.Kind)
	}
	if result.Binding == nil || result.Binding.OrchestrationKind() != orchestration.KindJavaScript {
		t.Fatalf("Binding = %#v, want non-nil JAVASCRIPT binding", result.Binding)
	}
}

func TestCompileSelectsJavaScriptKindForSourceRefWorkflow(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	workflowSource := `agent.run({ prompt: "review", label: "child" });
return { ok: true };`
	if err := os.WriteFile(filepath.Join(dir, "workflow.js"), []byte(workflowSource), 0o600); err != nil {
		t.Fatalf("write workflow source: %v", err)
	}

	compiler := internalservice.New(testIDGenerator(), testJavaScriptWorkflows(), testJavaScriptWorkflows())
	reader := factoryruntime.NewWorkflowSourceReader(dir, localWorkflowSourceFiles{})
	result, err := compiler.Compile(context.Background(), orchestration.CompileRequest{
		Config: &factorydefinitions.FactoryConfig{
			Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{
				Kind: factorydefinitions.OrchestratorKindJavaScript,
				JavaScript: &factorydefinitions.FactoryOrchestratorJavaScriptConfig{
					SourceRef: "workflow.js",
					ArgsSchema: json.RawMessage(`{
						"type":"object",
						"required":["prompt"],
						"properties":{"prompt":{"type":"string"}},
						"additionalProperties":false
					}`),
				},
			},
		},
		SourceReader: reader,
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if result.Kind != orchestration.KindJavaScript {
		t.Fatalf("Kind = %q, want JAVASCRIPT", result.Kind)
	}
	if result.Binding == nil || result.Binding.OrchestrationKind() != orchestration.KindJavaScript {
		t.Fatalf("Binding = %#v, want non-nil JAVASCRIPT binding", result.Binding)
	}
}

func TestCompileRejectsUnsupportedOrchestrationKind(t *testing.T) {
	t.Parallel()

	compiler := internalservice.New(testIDGenerator(), testJavaScriptWorkflows(), testJavaScriptWorkflows())
	_, err := compiler.Compile(context.Background(), orchestration.CompileRequest{
		Config: &factorydefinitions.FactoryConfig{
			Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{
				Kind: "GRAPH",
			},
		},
	})
	assertCompileError(t, err, orchestration.ErrUnsupportedKind, "ORCHESTRATION_UNSUPPORTED_KIND")
}

func TestCompileRejectsMissingActivatedDefinition(t *testing.T) {
	t.Parallel()

	compiler := internalservice.New(testIDGenerator(), testJavaScriptWorkflows(), testJavaScriptWorkflows())
	_, err := compiler.Compile(context.Background(), orchestration.CompileRequest{})
	assertCompileError(t, err, orchestration.ErrDefinitionUnavailable, "ORCHESTRATION_DEFINITION_UNAVAILABLE")
}

func TestCompileRejectsPetriCompileWithoutIDGenerator(t *testing.T) {
	t.Parallel()

	compiler := internalservice.New(nil, testJavaScriptWorkflows(), testJavaScriptWorkflows())
	_, err := compiler.Compile(context.Background(), orchestration.CompileRequest{
		Config: minimalPetriFactoryConfig(),
	})
	assertCompileError(t, err, orchestration.ErrInvalidDefinition, "ORCHESTRATION_INVALID_DEFINITION")
}

func TestCompileRejectsInvalidJavaScriptInlineSource(t *testing.T) {
	t.Parallel()

	compiler := internalservice.New(testIDGenerator(), testJavaScriptWorkflows(), testJavaScriptWorkflows())
	_, err := compiler.Compile(context.Background(), orchestration.CompileRequest{
		Config: minimalJavaScriptFactoryConfig(`workflow.final("ok");\nphase("setup";\n`),
	})
	var compileErr *orchestration.CompileError
	if !errors.As(err, &compileErr) {
		t.Fatalf("Compile() error = %T(%v), want *orchestration.CompileError", err, err)
	}
	if !errors.Is(err, orchestration.ErrInvalidDefinition) {
		t.Fatalf("Compile() error = %v, want ErrInvalidDefinition", err)
	}
	if compileErr.Orchestrator != orchestration.KindJavaScript {
		t.Fatalf("Orchestrator = %q, want JAVASCRIPT", compileErr.Orchestrator)
	}
	if len(compileErr.Diagnostics) == 0 || compileErr.Diagnostics[0].Code == "" {
		t.Fatalf("Diagnostics = %#v, want typed JavaScript validation code", compileErr.Diagnostics)
	}
}

func TestCompileRejectsJavaScriptMissingSource(t *testing.T) {
	t.Parallel()

	compiler := internalservice.New(testIDGenerator(), testJavaScriptWorkflows(), testJavaScriptWorkflows())
	_, err := compiler.Compile(context.Background(), orchestration.CompileRequest{
		Config: &factorydefinitions.FactoryConfig{
			Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{
				Kind:       factorydefinitions.OrchestratorKindJavaScript,
				JavaScript: &factorydefinitions.FactoryOrchestratorJavaScriptConfig{},
			},
		},
	})
	assertCompileError(t, err, orchestration.ErrInvalidDefinition, "ORCHESTRATION_JAVASCRIPT_MISSING_SOURCE")
}

func TestCompileIsInertAndDoesNotRequireRuntimeSideEffects(t *testing.T) {
	t.Parallel()

	compiler := internalservice.New(testIDGenerator(), testJavaScriptWorkflows(), testJavaScriptWorkflows())
	result, err := compiler.Compile(context.Background(), orchestration.CompileRequest{
		Config: minimalPetriFactoryConfig(),
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if result.Binding == nil {
		t.Fatal("Binding = nil, want compiled binding without runtime startup")
	}
}

func assertCompileError(t *testing.T, err error, want error, wantCode string) {
	t.Helper()
	var compileErr *orchestration.CompileError
	if !errors.As(err, &compileErr) {
		t.Fatalf("Compile() error = %T(%v), want *orchestration.CompileError", err, err)
	}
	if !errors.Is(err, want) {
		t.Fatalf("Compile() error = %v, want %v", err, want)
	}
	if len(compileErr.Diagnostics) == 0 {
		t.Fatalf("Diagnostics = nil, want at least one diagnostic")
	}
	if compileErr.Diagnostics[0].Code != wantCode {
		t.Fatalf("Diagnostics[0].Code = %q, want %q", compileErr.Diagnostics[0].Code, wantCode)
	}
}

func minimalPetriFactoryConfig() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "task",
			States: []factorydefinitions.StateConfig{
				{Name: "init", Type: factorydefinitions.StateTypeInitial},
				{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
				{Name: "failed", Type: factorydefinitions.StateTypeFailed},
			},
		}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name: "processor",
			Inputs: []factorydefinitions.IOConfig{{
				StateName: "init", WorkTypeName: "task",
			}},
			Outputs: []factorydefinitions.IOConfig{{
				StateName: "complete", WorkTypeName: "task",
			}},
		}},
	}
}

func minimalJavaScriptFactoryConfig(inline string) *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{
		Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{
			Kind: factorydefinitions.OrchestratorKindJavaScript,
			JavaScript: &factorydefinitions.FactoryOrchestratorJavaScriptConfig{
				InlineSource: &factorydefinitions.FactoryOrchestratorJavaScriptInlineSource{
					Encoding: "utf-8",
					Inline:   strings.TrimSpace(inline),
				},
				ArgsSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
			},
		},
	}
}

func testIDGenerator() factoryruntime.IDGenerator {
	next := 0
	return func() string {
		next++
		return fmt.Sprintf("orchestration-test-id-%d", next)
	}
}

type localWorkflowSourceFiles struct{}

func (localWorkflowSourceFiles) ReadDir(path string) ([]fs.DirEntry, error) { return os.ReadDir(path) }
func (localWorkflowSourceFiles) ReadFile(path string) ([]byte, error)       { return os.ReadFile(path) }
func (localWorkflowSourceFiles) Stat(path string) (fs.FileInfo, error)      { return os.Stat(path) }

func testJavaScriptWorkflows() factoryruntime.JavaScriptWorkflows {
	return factoryruntimejavascript.New(localWorkflowSourceFiles{}, os.UserHomeDir, filepath.EvalSymlinks)
}
