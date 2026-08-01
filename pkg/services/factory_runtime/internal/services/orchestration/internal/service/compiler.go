// Package service implements inert orchestration kind selection and definition
// compilation for the parent-private Factory Runtime orchestration owner.
package service

import (
	"context"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/definitionmapping"
	orchestration "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration"
)

const (
	diagnosticCodeUnsupportedKind       = "ORCHESTRATION_UNSUPPORTED_KIND"
	diagnosticCodeDefinitionUnavailable = "ORCHESTRATION_DEFINITION_UNAVAILABLE"
	diagnosticCodeInvalidDefinition     = "ORCHESTRATION_INVALID_DEFINITION"
	diagnosticCodeJavaScriptMissingSource = "ORCHESTRATION_JAVASCRIPT_MISSING_SOURCE"
)

// Compiler selects orchestration kind, compiles activated definitions, and
// drives private JavaScript execute/resume without exposing VM internals.
type Compiler struct {
	newID     factoryruntime.IDGenerator
	workflows factoryruntime.JavaScriptWorkflowDefinitions
	runtime   factoryruntime.JavaScriptWorkflowRuntime
}

var _ orchestration.Service = (*Compiler)(nil)
var _ factoryruntime.OrchestrationJavaScriptExecution = (*Compiler)(nil)

// New constructs the parent-private orchestration owner.
func New(
	newID factoryruntime.IDGenerator,
	workflows factoryruntime.JavaScriptWorkflowDefinitions,
	runtime factoryruntime.JavaScriptWorkflowRuntime,
) *Compiler {
	return &Compiler{newID: newID, workflows: workflows, runtime: runtime}
}

// RunJavaScript executes one JavaScript orchestration variant through the
// private runtime while preserving Runtime-facing outcome vocabulary.
func (c *Compiler) RunJavaScript(
	ctx context.Context,
	request factoryruntime.JavaScriptRuntimeRequest,
	hooks factoryruntime.JavaScriptRuntimeHooks,
) (factoryruntime.JavaScriptRuntimeOutcome, error) {
	if c == nil || c.runtime == nil {
		return factoryruntime.JavaScriptRuntimeOutcome{}, compileError(
			orchestration.ErrInvalidDefinition,
			orchestration.KindJavaScript,
			orchestration.Diagnostic{
				Code:    diagnosticCodeInvalidDefinition,
				Message: "JavaScript orchestration runtime is required",
				Path:    "orchestration.javascript.run",
			},
		)
	}
	return c.runtime.Run(ctx, request, hooks)
}

// ResumeJavaScript rebuilds private resume state for a JavaScript orchestration
// variant without requiring peers to import VM internals.
func (c *Compiler) ResumeJavaScript(
	summary factoryruntime.JavaScriptCompletedCheckpointSummary,
	records []factoryruntime.JavaScriptRuntimeRecord,
) factoryruntime.JavaScriptResumeContext {
	if c == nil || c.runtime == nil {
		return factoryruntime.JavaScriptResumeContext{}
	}
	return c.runtime.ResumeContext(summary, records)
}

// Compile selects the authored orchestration kind and produces a runnable binding.
func (c *Compiler) Compile(
	ctx context.Context,
	req orchestration.CompileRequest,
) (orchestration.CompileResult, error) {
	if ctx == nil {
		return orchestration.CompileResult{}, compileError(
			orchestration.ErrInvalidDefinition,
			orchestration.Kind(""),
			orchestration.Diagnostic{
				Code:    diagnosticCodeInvalidDefinition,
				Message: "context is required",
				Path:    "orchestration.compile",
			},
		)
	}
	if err := ctx.Err(); err != nil {
		return orchestration.CompileResult{}, err
	}
	if req.Config == nil {
		return orchestration.CompileResult{}, compileError(
			orchestration.ErrDefinitionUnavailable,
			orchestration.Kind(""),
			orchestration.Diagnostic{
				Code:    diagnosticCodeDefinitionUnavailable,
				Message: "activated Factory definition is required",
				Path:    "factory",
			},
		)
	}

	kind, err := resolveKind(req.Config)
	if err != nil {
		return orchestration.CompileResult{}, err
	}

	switch kind {
	case orchestration.KindPetri:
		binding, compileErr := c.compilePetri(ctx, req.Config)
		if compileErr != nil {
			return orchestration.CompileResult{}, compileErr
		}
		return orchestration.CompileResult{Kind: kind, Binding: binding}, nil
	case orchestration.KindJavaScript:
		binding, compileErr := c.compileJavaScript(req)
		if compileErr != nil {
			return orchestration.CompileResult{}, compileErr
		}
		return orchestration.CompileResult{Kind: kind, Binding: binding}, nil
	default:
		return orchestration.CompileResult{}, compileError(
			orchestration.ErrUnsupportedKind,
			kind,
			orchestration.Diagnostic{
				Code: diagnosticCodeUnsupportedKind,
				Message: fmt.Sprintf(
					"unsupported orchestrator.kind %q (supported: %q, %q)",
					factorydefinitions.EffectiveOrchestratorKind(req.Config),
					orchestration.KindPetri,
					orchestration.KindJavaScript,
				),
				Path: "factory.orchestrator.kind",
			},
		)
	}
}

func resolveKind(cfg *factorydefinitions.FactoryConfig) (orchestration.Kind, error) {
	raw := factorydefinitions.EffectiveOrchestratorKind(cfg)
	canonical := factorydefinitions.StrictPublicFactoryOrchestratorKind(raw)
	switch orchestration.Kind(canonical) {
	case orchestration.KindPetri:
		return orchestration.KindPetri, nil
	case orchestration.KindJavaScript:
		return orchestration.KindJavaScript, nil
	default:
		return orchestration.Kind(""), compileError(
			orchestration.ErrUnsupportedKind,
			orchestration.Kind(""),
			orchestration.Diagnostic{
				Code: diagnosticCodeUnsupportedKind,
				Message: fmt.Sprintf(
					"unsupported orchestrator.kind %q (supported: %q, %q)",
					raw,
					orchestration.KindPetri,
					orchestration.KindJavaScript,
				),
				Path: "factory.orchestrator.kind",
			},
		)
	}
}

func (c *Compiler) compilePetri(
	ctx context.Context,
	cfg *factorydefinitions.FactoryConfig,
) (orchestration.Binding, error) {
	if c == nil || c.newID == nil {
		return nil, compileError(
			orchestration.ErrInvalidDefinition,
			orchestration.KindPetri,
			orchestration.Diagnostic{
				Code:    diagnosticCodeInvalidDefinition,
				Message: "orchestration compiler ID generator is required",
				Path:    "orchestration.petri",
			},
		)
	}
	mapper, err := definitionmapping.New(c.newID)
	if err != nil {
		return nil, compileError(
			orchestration.ErrInvalidDefinition,
			orchestration.KindPetri,
			orchestration.Diagnostic{
				Code:    diagnosticCodeInvalidDefinition,
				Message: err.Error(),
				Path:    "orchestration.petri",
			},
		)
	}
	net, err := mapper.Map(ctx, cfg)
	if err != nil {
		return nil, compileError(
			orchestration.ErrInvalidDefinition,
			orchestration.KindPetri,
			orchestration.Diagnostic{
				Code:    diagnosticCodeInvalidDefinition,
				Message: err.Error(),
				Path:    "factory",
			},
		)
	}
	return orchestration.NewPetriBinding(net), nil
}

func (c *Compiler) compileJavaScript(
	req orchestration.CompileRequest,
) (orchestration.Binding, error) {
	jsCfg, err := requireJavaScriptCompileConfig(c, req.Config)
	if err != nil {
		return nil, err
	}

	diagnostics := javascriptConfigDiagnostics(c.workflows, jsCfg)
	if len(diagnostics) > 0 {
		return nil, compileError(orchestration.ErrInvalidDefinition, orchestration.KindJavaScript, diagnostics...)
	}

	inline := strings.TrimSpace(inlineSource(jsCfg))
	if inline != "" {
		return orchestration.NewJavaScriptBinding("inline", "", true), nil
	}

	sourceRef := strings.TrimSpace(jsCfg.SourceRef)
	if sourceRef == "" {
		return nil, compileError(
			orchestration.ErrInvalidDefinition,
			orchestration.KindJavaScript,
			orchestration.Diagnostic{
				Code:    diagnosticCodeJavaScriptMissingSource,
				Message: "orchestrator.javascript requires inlineSource or sourceRef",
				Path:    "factory.orchestrator.javascript",
			},
		)
	}
	return c.compileJavaScriptSourceRef(req, jsCfg, sourceRef)
}

func requireJavaScriptCompileConfig(
	c *Compiler,
	cfg *factorydefinitions.FactoryConfig,
) (*factorydefinitions.FactoryOrchestratorJavaScriptConfig, error) {
	if cfg == nil || cfg.Orchestrator == nil || cfg.Orchestrator.JavaScript == nil {
		return nil, compileError(
			orchestration.ErrInvalidDefinition,
			orchestration.KindJavaScript,
			orchestration.Diagnostic{
				Code:    diagnosticCodeJavaScriptMissingSource,
				Message: "orchestrator.javascript configuration is required",
				Path:    "factory.orchestrator.javascript",
			},
		)
	}
	if c == nil || c.workflows == nil {
		return nil, compileError(
			orchestration.ErrInvalidDefinition,
			orchestration.KindJavaScript,
			orchestration.Diagnostic{
				Code:    diagnosticCodeInvalidDefinition,
				Message: "JavaScript workflow validation service is unavailable",
				Path:    "factory.orchestrator.javascript",
			},
		)
	}
	return cfg.Orchestrator.JavaScript, nil
}

func (c *Compiler) compileJavaScriptSourceRef(
	req orchestration.CompileRequest,
	jsCfg *factorydefinitions.FactoryOrchestratorJavaScriptConfig,
	sourceRef string,
) (orchestration.Binding, error) {
	if req.SourceReader == nil {
		return nil, compileError(
			orchestration.ErrInvalidDefinition,
			orchestration.KindJavaScript,
			orchestration.Diagnostic{
				Code:    factoryruntime.WorkflowValidationCodeSourceUnreadable,
				Message: "workflow source reader is required to compile sourceRef workflows",
				Path:    "factory.orchestrator.javascript.sourceRef",
			},
		)
	}

	content, err := req.SourceReader.ReadWorkflowSource(sourceRef)
	if err != nil {
		return nil, compileError(
			orchestration.ErrInvalidDefinition,
			orchestration.KindJavaScript,
			orchestration.Diagnostic{
				Code:    factoryruntime.WorkflowValidationCodeSourceUnreadable,
				Message: fmt.Sprintf("unable to read workflow source %q: %v", sourceRef, err),
				Path:    "factory.orchestrator.javascript.sourceRef",
			},
		)
	}
	loadReq := workflowValidationLoadRequest(req.SourceReader, sourceRef, content)
	loaded, loadIssues := c.workflows.LoadSource(loadReq)
	if len(loadIssues) > 0 {
		return nil, compileError(
			orchestration.ErrInvalidDefinition,
			orchestration.KindJavaScript,
			workflowIssuesToDiagnostics(loadIssues)...,
		)
	}
	if expectedHash := strings.TrimSpace(jsCfg.SourceHash); expectedHash != "" && expectedHash != loaded.SourceHash {
		return nil, compileError(
			orchestration.ErrInvalidDefinition,
			orchestration.KindJavaScript,
			orchestration.Diagnostic{
				Code: factoryruntime.WorkflowValidationCodeSourceHashMismatch,
				Message: fmt.Sprintf(
					"orchestrator.javascript.sourceHash %q does not match loaded workflow source hash %q",
					expectedHash,
					loaded.SourceHash,
				),
				Path: "factory.orchestrator.javascript.sourceHash",
			},
		)
	}
	fileResult := c.workflows.ValidateLoaded(loaded, factoryruntime.WorkflowValidationRequest{
		ConfigPath: "orchestrator.javascript.sourceRef",
		Metadata:   jsCfg.Metadata,
		ArgsSchema: jsCfg.ArgsSchema,
	})
	if len(fileResult.Issues) > 0 {
		return nil, compileError(
			orchestration.ErrInvalidDefinition,
			orchestration.KindJavaScript,
			workflowIssuesToDiagnostics(fileResult.Issues)...,
		)
	}
	return orchestration.NewJavaScriptBinding(sourceRef, loaded.SourceHash, false), nil
}

type factoryRootWorkflowSourceReader interface {
	FactoryRoot() string
}

func workflowValidationLoadRequest(
	reader factoryruntime.WorkflowSourceReader,
	sourceRef string,
	content string,
) factoryruntime.WorkflowValidationLoadRequest {
	request := factoryruntime.WorkflowValidationLoadRequest{
		SourceRef: sourceRef,
		Content:   content,
	}
	if rootReader, ok := reader.(factoryRootWorkflowSourceReader); ok {
		request.FactoryRoot = rootReader.FactoryRoot()
		request.BundleReader = reader
	}
	return request
}

func javascriptConfigDiagnostics(
	workflows factoryruntime.JavaScriptWorkflowDefinitions,
	cfg *factorydefinitions.FactoryOrchestratorJavaScriptConfig,
) []orchestration.Diagnostic {
	configResult := workflows.Validate(factoryruntime.WorkflowValidationRequest{
		ConfigPath: "orchestrator.javascript",
		Metadata:   cfg.Metadata,
		ArgsSchema: cfg.ArgsSchema,
	})
	diagnostics := workflowIssuesToDiagnostics(configResult.Issues)

	inline := strings.TrimSpace(inlineSource(cfg))
	if inline == "" {
		return diagnostics
	}
	inlineResult := workflows.Validate(factoryruntime.WorkflowValidationRequest{
		Source:     inline,
		SourceRef:  "inline",
		ConfigPath: "orchestrator.javascript.inlineSource",
		Metadata:   cfg.Metadata,
		ArgsSchema: cfg.ArgsSchema,
	})
	return append(diagnostics, workflowIssuesToDiagnostics(inlineResult.Issues)...)
}

func inlineSource(cfg *factorydefinitions.FactoryOrchestratorJavaScriptConfig) string {
	if cfg == nil || cfg.InlineSource == nil {
		return ""
	}
	return cfg.InlineSource.Inline
}

func workflowIssuesToDiagnostics(
	issues []factoryruntime.WorkflowValidationIssue,
) []orchestration.Diagnostic {
	if len(issues) == 0 {
		return nil
	}
	diagnostics := make([]orchestration.Diagnostic, 0, len(issues))
	for _, issue := range issues {
		path := "factory.orchestrator.javascript"
		switch {
		case strings.HasPrefix(issue.Path, "orchestrator.javascript."):
			path = "factory." + strings.TrimPrefix(issue.Path, "orchestrator.")
		case issue.Path == "inline":
			path = "factory.orchestrator.javascript.inlineSource"
		case issue.Path != "":
			path = "factory.orchestrator.javascript.sourceRef"
		}
		diagnostics = append(diagnostics, orchestration.Diagnostic{
			Code:    issue.Code,
			Message: issue.Message + issue.LocationSuffix(),
			Path:    path,
		})
	}
	return diagnostics
}

func compileError(
	err error,
	kind orchestration.Kind,
	diagnostics ...orchestration.Diagnostic,
) *orchestration.CompileError {
	return &orchestration.CompileError{
		Err:         err,
		Orchestrator: kind,
		Diagnostics: diagnostics,
	}
}
