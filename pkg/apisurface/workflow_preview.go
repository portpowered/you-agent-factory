package apisurface

import (
	"encoding/json"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/workflowpolicy"
	"github.com/portpowered/infinite-you/pkg/workflowpreview"
	"github.com/portpowered/infinite-you/pkg/workflowsource"
)

// BuildWorkflowPreview is the shared API, CLI, MCP, and website entry point for
// workflow validation, source resolution, and policy preview projection.
func BuildWorkflowPreview(input workflowpreview.Request) workflowpreview.Preview {
	return workflowpreview.BuildPreview(input)
}

// BuildWorkflowSessionStartPreview is the shared entry point for session-start preview projection.
func BuildWorkflowSessionStartPreview(input workflowpreview.Request) workflowpreview.Preview {
	return workflowpreview.BuildSessionStartPreview(input)
}

// WorkflowPreviewRequestFromAPI maps one public request into the shared preview contract.
func WorkflowPreviewRequestFromAPI(req factoryapi.WorkflowPreviewRequest) (workflowpreview.Request, error) {
	sourceKind, err := workflowSourceKindFromAPI(string(req.SourceKind))
	if err != nil {
		return workflowpreview.Request{}, err
	}

	projectRoot := strings.TrimSpace(derefString(req.ProjectRoot))
	if projectRoot == "" {
		return workflowpreview.Request{}, &RequestValidationError{Message: "projectRoot is required"}
	}
	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		return workflowpreview.Request{}, &RequestValidationError{Message: err.Error()}
	}

	var argsSchema []byte
	if req.ArgsSchema != nil {
		encoded, marshalErr := json.Marshal(req.ArgsSchema)
		if marshalErr != nil {
			return workflowpreview.Request{}, &RequestValidationError{Message: "argsSchema must be a JSON object"}
		}
		argsSchema = encoded
	}

	var factoryDefault json.RawMessage
	if req.DefaultPolicy != nil {
		encoded, marshalErr := json.Marshal(req.DefaultPolicy)
		if marshalErr != nil {
			return workflowpreview.Request{}, &RequestValidationError{Message: "defaultPolicy must be a JSON object"}
		}
		factoryDefault = encoded
	}

	var requestedPolicy map[string]any
	if req.RequestedPolicy != nil {
		requestedPolicy = *req.RequestedPolicy
	}

	var metadata map[string]string
	if req.Metadata != nil {
		metadata = *req.Metadata
	}

	return workflowpreview.Request{
		Source: workflowsource.Request{
			Kind:               sourceKind,
			Value:              derefString(req.SourceValue),
			InlineSource:       derefString(req.InlineSource),
			ArtifactRoot:       derefString(req.ArtifactRoot),
			AllowFactoryLookup: req.AllowFactoryLookup != nil && *req.AllowFactoryLookup,
		},
		Context:              ctx,
		Metadata:             metadata,
		ArgsSchema:           argsSchema,
		FactoryDefaultPolicy: factoryDefault,
		RequestedPolicy:      requestedPolicy,
		RequestedRunner:      derefString(req.RequestedRunner),
		RequestedModel:       derefString(req.RequestedModel),
		RequestedProfile:     derefString(req.RequestedProfile),
		TimeoutMillis:        req.TimeoutMillis,
	}, nil
}

// WorkflowPreviewResultFromPreview maps the shared preview contract to the public API shape.
func WorkflowPreviewResultFromPreview(preview workflowpreview.Preview) factoryapi.WorkflowPreviewResult {
	return factoryapi.WorkflowPreviewResult{
		Valid:                  preview.Valid,
		SourceResolution:       workflowSourceResolutionFromPreview(preview.SourceResolution),
		SourceValidationIssues: workflowDiagnosticsFromSourceIssues(preview.SourceValidationIssues),
		PolicyPreview:          workflowPolicyPreviewFromPreview(preview.PolicyPreview),
		ResultConstraints:      workflowResultConstraintsFromPreview(preview.ResultConstraints),
	}
}

func workflowSourceKindFromAPI(kind string) (workflowsource.Kind, error) {
	switch workflowsource.Kind(strings.TrimSpace(kind)) {
	case workflowsource.KindFactoryID,
		workflowsource.KindFactoryInline,
		workflowsource.KindWorkflowFile,
		workflowsource.KindWorkflowName,
		workflowsource.KindInlineWorkflow:
		return workflowsource.Kind(strings.TrimSpace(kind)), nil
	default:
		return "", &RequestValidationError{Message: "sourceKind must be one of FACTORY_ID, FACTORY_INLINE, WORKFLOW_FILE, WORKFLOW_NAME, or INLINE_WORKFLOW"}
	}
}

func workflowSourceResolutionFromPreview(resolution workflowsource.Resolution) factoryapi.WorkflowSourceResolution {
	out := factoryapi.WorkflowSourceResolution{
		RequestKind: string(resolution.RequestKind),
		Found:       resolution.Found,
	}
	if diagnostics := workflowDiagnosticsFromSource(resolution.Diagnostics); len(diagnostics) > 0 {
		out.Diagnostics = &diagnostics
	}
	artifactRoot := workflowArtifactRootFromPreview(resolution.ArtifactRoot)
	out.ArtifactRoot = &artifactRoot
	if value := strings.TrimSpace(resolution.RequestValue); value != "" {
		out.RequestValue = &value
	}
	if value := string(resolution.ResolvedKind); value != "" {
		out.ResolvedKind = &value
	}
	if value := string(resolution.LookupStage); value != "" {
		out.LookupStage = &value
	}
	if value := strings.TrimSpace(resolution.SourceRef); value != "" {
		out.SourceRef = &value
	}
	if value := strings.TrimSpace(resolution.SourceHash); value != "" {
		out.SourceHash = &value
	}
	if value := strings.TrimSpace(resolution.OrchestratorKind); value != "" {
		out.OrchestratorKind = &value
	}
	if value := strings.TrimSpace(resolution.Dialect); value != "" {
		out.Dialect = &value
	}
	return out
}

func workflowArtifactRootFromPreview(decision workflowsource.ArtifactRootDecision) factoryapi.WorkflowArtifactRootDecision {
	out := factoryapi.WorkflowArtifactRootDecision{
		Requested: decision.Requested,
		Allowed:   decision.Allowed,
	}
	if value := strings.TrimSpace(decision.Effective); value != "" {
		out.Effective = &value
	}
	if decision.Diagnostic != nil {
		diagnostic := workflowDiagnosticFromSource(*decision.Diagnostic)
		out.Diagnostic = &diagnostic
	}
	return out
}

func workflowPolicyPreviewFromPreview(preview workflowpolicy.Preview) factoryapi.WorkflowPolicyPreview {
	effectivePolicy := map[string]interface{}{}
	if encoded, err := json.Marshal(preview.EffectivePolicy); err == nil {
		_ = json.Unmarshal(encoded, &effectivePolicy)
	}
	out := factoryapi.WorkflowPolicyPreview{
		EffectivePolicy:    effectivePolicy,
		PolicyHash:         preview.PolicyHash,
		MaxChildCount:      preview.MaxChildCount,
		MaxConcurrency:     preview.MaxConcurrency,
		DeniedCapabilities: workflowDiagnosticsFromPolicy(preview.DeniedCapabilities),
		ValidationIssues:   workflowDiagnosticsFromPolicyIssues(preview.ValidationIssues),
	}
	if preview.RunnerDecision != nil {
		decision := workflowDecisionMap(preview.RunnerDecision)
		out.RunnerDecision = &decision
	}
	if preview.ModelDecision != nil {
		decision := workflowDecisionMap(preview.ModelDecision)
		out.ModelDecision = &decision
	}
	if preview.ProfileDecision != nil {
		decision := workflowDecisionMap(preview.ProfileDecision)
		out.ProfileDecision = &decision
	}
	timeoutDecisions := map[string]interface{}{}
	if preview.TimeoutDecisions.RequestedMillis != nil {
		timeoutDecisions["requestedMillis"] = *preview.TimeoutDecisions.RequestedMillis
	}
	if preview.TimeoutDecisions.EffectiveMillis != nil {
		timeoutDecisions["effectiveMillis"] = *preview.TimeoutDecisions.EffectiveMillis
	}
	out.TimeoutDecisions = &timeoutDecisions
	budgetDecisions := map[string]interface{}{
		"maxChildCount":  preview.BudgetDecisions.MaxChildCount,
		"maxConcurrency": preview.BudgetDecisions.MaxConcurrency,
	}
	out.BudgetDecisions = &budgetDecisions
	return out
}

func workflowResultConstraintsFromPreview(constraints workflowpreview.ResultConstraints) factoryapi.WorkflowResultConstraints {
	return factoryapi.WorkflowResultConstraints{
		RequiresStructuredCloneableJson: constraints.RequiresStructuredCloneableJSON,
		ArtifactUriScheme:               constraints.ArtifactURIScheme,
		MaxEmbeddedBytes:                constraints.MaxEmbeddedBytes,
		RejectedValueKinds:              append([]string(nil), constraints.RejectedValueKinds...),
	}
}

func workflowDiagnosticsFromSourceIssues(issues []workflowpreview.SourceValidationIssue) []factoryapi.WorkflowDiagnostic {
	out := make([]factoryapi.WorkflowDiagnostic, 0, len(issues))
	for _, issue := range issues {
		out = append(out, workflowDiagnosticFromSourceIssue(issue))
	}
	return out
}

func workflowDiagnosticFromSourceIssue(issue workflowpreview.SourceValidationIssue) factoryapi.WorkflowDiagnostic {
	out := factoryapi.WorkflowDiagnostic{
		Code:    issue.Code,
		Message: issue.Message,
	}
	if value := strings.TrimSpace(issue.Path); value != "" {
		out.Path = &value
	}
	if issue.Line > 0 {
		line := issue.Line
		out.Line = &line
	}
	if issue.Column > 0 {
		column := issue.Column
		out.Column = &column
	}
	return out
}

func workflowDiagnosticsFromSource(diagnostics []workflowsource.Diagnostic) []factoryapi.WorkflowDiagnostic {
	out := make([]factoryapi.WorkflowDiagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		out = append(out, workflowDiagnosticFromSource(diagnostic))
	}
	return out
}

func workflowDiagnosticFromSource(diagnostic workflowsource.Diagnostic) factoryapi.WorkflowDiagnostic {
	return factoryapi.WorkflowDiagnostic{
		Code:    diagnostic.Code,
		Message: diagnostic.Message,
	}
}

func workflowDiagnosticsFromPolicy(diagnostics []workflowpolicy.Diagnostic) []factoryapi.WorkflowDiagnostic {
	out := make([]factoryapi.WorkflowDiagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		out = append(out, factoryapi.WorkflowDiagnostic{
			Code:    diagnostic.Code,
			Message: diagnostic.Message,
		})
	}
	return out
}

func workflowDiagnosticsFromPolicyIssues(issues []workflowpolicy.Issue) []factoryapi.WorkflowDiagnostic {
	out := make([]factoryapi.WorkflowDiagnostic, 0, len(issues))
	for _, issue := range issues {
		diagnostic := factoryapi.WorkflowDiagnostic{
			Code:    issue.Code,
			Message: issue.Message,
		}
		if value := strings.TrimSpace(issue.Path); value != "" {
			diagnostic.Path = &value
		}
		out = append(out, diagnostic)
	}
	return out
}

func workflowDecisionMap(decision any) map[string]interface{} {
	encoded, err := json.Marshal(decision)
	if err != nil {
		return map[string]interface{}{}
	}
	var out map[string]interface{}
	if err := json.Unmarshal(encoded, &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
