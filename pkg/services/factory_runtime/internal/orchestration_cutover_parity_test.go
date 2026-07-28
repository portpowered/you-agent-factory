package internal_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/definitionmapping"
	factoryruntimejavascript "github.com/portpowered/infinite-you/pkg/services/factory_runtime/javascript"
	orchestration "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration"
	factoryruntimeorchestrationowner "github.com/portpowered/infinite-you/pkg/services/factory_runtime/orchestrationowner"
	factoryinternal "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

func TestOrchestrationCompilePetriNetMatchesDefinitionMappingCutover(t *testing.T) {
	t.Parallel()

	cfg := cutoverPetriFactoryConfig()
	compiler := factoryruntimeorchestrationowner.NewCompilation(testRuntimeID, nil, nil)
	orchestratedNet, err := compiler.CompilePetriNet(context.Background(), factory.OrchestrationCompileRequest{
		Config: cfg,
	})
	if err != nil {
		t.Fatalf("CompilePetriNet() error = %v", err)
	}

	mapper, err := definitionmapping.New(testRuntimeID)
	if err != nil {
		t.Fatalf("definitionmapping.New() error = %v", err)
	}
	mappedNet, err := mapper.Map(context.Background(), cfg)
	if err != nil {
		t.Fatalf("mapper.Map() error = %v", err)
	}

	if got, want := netStructureSummary(orchestratedNet), netStructureSummary(mappedNet); !reflect.DeepEqual(got, want) {
		t.Fatalf("orchestrated net summary = %#v, want definition-mapping parity %#v", got, want)
	}
}

func TestOrchestrationCompilePetriNetRejectsUnsupportedKindWithRuntimeDiagnostics(t *testing.T) {
	t.Parallel()

	compiler := factoryruntimeorchestrationowner.NewCompilation(testRuntimeID, nil, nil)
	_, err := compiler.CompilePetriNet(context.Background(), factory.OrchestrationCompileRequest{
		Config: &factorydefinitions.FactoryConfig{
			Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{
				Kind: "GRAPH",
			},
		},
	})
	var compileErr *orchestration.CompileError
	if !errors.As(err, &compileErr) {
		t.Fatalf("CompilePetriNet() error = %T(%v), want *orchestration.CompileError", err, err)
	}
	if !errors.Is(err, orchestration.ErrUnsupportedKind) {
		t.Fatalf("CompilePetriNet() error = %v, want ErrUnsupportedKind", err)
	}
	if len(compileErr.Diagnostics) == 0 || compileErr.Diagnostics[0].Code != "ORCHESTRATION_UNSUPPORTED_KIND" {
		t.Fatalf("diagnostics = %#v, want ORCHESTRATION_UNSUPPORTED_KIND", compileErr.Diagnostics)
	}
}

func TestOrchestrationCompileSelectsJavaScriptKindWithoutPetriNet(t *testing.T) {
	t.Parallel()

	workflows := cutoverJavaScriptWorkflows()
	compiler := factoryruntimeorchestrationowner.NewCompilation(testRuntimeID, workflows, workflows)
	result, err := compiler.Compile(context.Background(), factory.OrchestrationCompileRequest{
		Config: cutoverJavaScriptFactoryConfig(`workflow.final("ok");`),
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if result.Kind != factory.OrchestrationKindJavaScript {
		t.Fatalf("Kind = %q, want JAVASCRIPT", result.Kind)
	}
	_, err = compiler.CompilePetriNet(context.Background(), factory.OrchestrationCompileRequest{
		Config: cutoverJavaScriptFactoryConfig(`workflow.final("ok");`),
	})
	if err == nil {
		t.Fatal("CompilePetriNet() error = nil, want non-PETRI rejection")
	}
}

func TestBuildThroughOrchestrationPreservesRunnablePetriTopology(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatalf("loadedFactoryFixture: %v", err)
	}
	bundle, err := testRuntimeFactory().Build(
		context.Background(), dir, dir, "~default",
		"", factorydefinitions.RuntimeModeBatch, false, nil, nil, nil, false, nil, nil,
		"", factory.RuntimeLogStorageConfig{},
		factoryinternal.RuntimeFileLoggingPolicyDisabled,
		factoryinternal.RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{},
		loaded, zap.NewNop(), "runtime-cutover", "", clockwork.NewFakeClock(), "", nil, nil, nil, nil, nil,
		nil,
		newTestRuntimeLedger,
		func(recordings.WorkerEventRecorder, *zap.Logger) (map[string]workers.WorkerExecutor, error) {
			return nil, nil
		},
		testRuntimeWorkers{},
		nil,
	)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if bundle == nil || bundle.Net == nil || bundle.Factory == nil {
		t.Fatal("bundle missing runnable Petri runtime")
	}
	wantWorkTypes := []string{"task"}
	if got := netStructureSummary(bundle.Net).WorkTypeNames; !reflect.DeepEqual(got, wantWorkTypes) {
		t.Fatalf("built work types = %#v, want %#v", got, wantWorkTypes)
	}
	if got := netStructureSummary(bundle.Net); got.PlaceCount == 0 || got.TransitionCount == 0 {
		t.Fatalf("built topology = %#v, want non-empty Petri runtime", got)
	}
}

func TestBuildThroughOrchestrationOpensInlineJavaScriptFactory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, map[string]any{
		"name": "inline-javascript-build",
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"inlineSource": map[string]any{
					"encoding": "utf-8",
					"inline":   `workflow.final("ok");`,
				},
				"argsSchema": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
				},
			},
		},
	})

	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatalf("loadedFactoryFixture: %v", err)
	}
	workflows := cutoverJavaScriptWorkflows()
	bundle, err := factoryinternal.NewRuntimeFactory(
		nil, nil, nil, nil, testRuntimeLoggerFactory, nil, nil,
		testRuntimeID, testRuntimeID, localRuntimeFiles{}, localRuntimeFiles{}, filepath.WalkDir,
		factoryruntimeorchestrationowner.NewCompilation(testRuntimeID, workflows, workflows),
	).Build(
		context.Background(), dir, dir, "~default",
		"", factorydefinitions.RuntimeModeBatch, false, nil, nil, nil, false, nil, nil,
		"", factory.RuntimeLogStorageConfig{},
		factoryinternal.RuntimeFileLoggingPolicyDisabled,
		factoryinternal.RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{},
		loaded, zap.NewNop(), "runtime-cutover-js", "", clockwork.NewFakeClock(), "", nil, nil, nil, nil, nil,
		nil,
		newTestRuntimeLedger,
		func(recordings.WorkerEventRecorder, *zap.Logger) (map[string]workers.WorkerExecutor, error) {
			return nil, nil
		},
		testRuntimeWorkers{},
		nil,
	)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if bundle == nil || bundle.Factory == nil {
		t.Fatal("bundle missing runnable JavaScript runtime")
	}
}

type netStructure struct {
	WorkTypeNames   []string
	PlaceCount      int
	TransitionCount int
	ResourceCount   int
}

func netStructureSummary(net *state.Net) netStructure {
	if net == nil {
		return netStructure{}
	}
	workTypeNames := make([]string, 0, len(net.WorkTypes))
	for name := range net.WorkTypes {
		workTypeNames = append(workTypeNames, name)
	}
	sort.Strings(workTypeNames)
	return netStructure{
		WorkTypeNames:   workTypeNames,
		PlaceCount:      len(net.Places),
		TransitionCount: len(net.Transitions),
		ResourceCount:   len(net.Resources),
	}
}

func cutoverPetriFactoryConfig() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "story",
			States: []factorydefinitions.StateConfig{
				{Name: "init", Type: factorydefinitions.StateTypeInitial},
				{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
				{Name: "failed", Type: factorydefinitions.StateTypeFailed},
			},
		}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name: "reviewer",
			Inputs: []factorydefinitions.IOConfig{{
				StateName: "init", WorkTypeName: "story",
			}},
			Outputs: []factorydefinitions.IOConfig{{
				StateName: "complete", WorkTypeName: "story",
			}},
		}},
	}
}

func cutoverJavaScriptFactoryConfig(inline string) *factorydefinitions.FactoryConfig {
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

type cutoverWorkflowSourceFiles struct{}

func (cutoverWorkflowSourceFiles) ReadDir(path string) ([]fs.DirEntry, error) { return os.ReadDir(path) }
func (cutoverWorkflowSourceFiles) ReadFile(path string) ([]byte, error)       { return os.ReadFile(path) }
func (cutoverWorkflowSourceFiles) Stat(path string) (fs.FileInfo, error)      { return os.Stat(path) }

func cutoverJavaScriptWorkflows() factory.JavaScriptWorkflows {
	return factoryruntimejavascript.New(cutoverWorkflowSourceFiles{}, os.UserHomeDir, filepath.EvalSymlinks)
}
