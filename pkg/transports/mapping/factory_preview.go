package apisurface

import (
	"encoding/json"
	"strings"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// FactoryPreviewInputFromAPI decodes one canonical Factory preview request into
// transport-independent edge fields. Factory Runtime applies source defaults.
func FactoryPreviewInputFromAPI(
	req factoryapi.FactoryPreviewRequest,
) (factory.WorkflowPreviewInput, error) {
	return factoryPreviewInputFromAPI(req.SourceKind, req.ProjectRoot, req.SourceValue, req.InlineSource, req.ArtifactRoot, req.AllowFactoryLookup, req.Metadata, req.ArgsSchema, req.DefaultPolicy, req.RequestedPolicy, req.RequestedRunner, req.RequestedModel, req.RequestedProfile, req.TimeoutMillis)
}

func factoryPreviewInputFromAPI(
	sourceKind factoryapi.FactoryPreviewRequestSourceKind,
	projectRoot *string,
	sourceValue *string,
	inlineSource *string,
	artifactRoot *string,
	allowFactoryLookup *bool,
	metadata *map[string]string,
	argsSchema *map[string]interface{},
	defaultPolicy *map[string]interface{},
	requestedPolicy *map[string]interface{},
	requestedRunner *string,
	requestedModel *string,
	requestedProfile *string,
	timeoutMillis *int64,
) (factory.WorkflowPreviewInput, error) {
	kind, err := workflowSourceKindFromAPI(string(sourceKind))
	if err != nil {
		return factory.WorkflowPreviewInput{}, err
	}

	root := strings.TrimSpace(derefString(projectRoot))
	if root == "" {
		return factory.WorkflowPreviewInput{}, &RequestValidationError{Message: "projectRoot is required"}
	}

	var encodedArgsSchema []byte
	if argsSchema != nil {
		encoded, marshalErr := json.Marshal(argsSchema)
		if marshalErr != nil {
			return factory.WorkflowPreviewInput{}, &RequestValidationError{Message: "argsSchema must be a JSON object"}
		}
		encodedArgsSchema = encoded
	}

	var factoryDefault json.RawMessage
	if defaultPolicy != nil {
		encoded, marshalErr := json.Marshal(defaultPolicy)
		if marshalErr != nil {
			return factory.WorkflowPreviewInput{}, &RequestValidationError{Message: "defaultPolicy must be a JSON object"}
		}
		factoryDefault = encoded
	}

	var requestedPolicyMap map[string]any
	if requestedPolicy != nil {
		requestedPolicyMap = *requestedPolicy
	}

	var metadataMap map[string]string
	if metadata != nil {
		metadataMap = *metadata
	}

	return factory.WorkflowPreviewInput{
		ProjectRoot: root,
		Source: factory.WorkflowSourceRequest{
			Kind:               kind,
			Value:              derefString(sourceValue),
			InlineSource:       derefString(inlineSource),
			ArtifactRoot:       derefString(artifactRoot),
			AllowFactoryLookup: allowFactoryLookup != nil && *allowFactoryLookup,
		},
		Metadata:             metadataMap,
		ArgsSchema:           encodedArgsSchema,
		FactoryDefaultPolicy: factoryDefault,
		RequestedPolicy:      requestedPolicyMap,
		RequestedRunner:      derefString(requestedRunner),
		RequestedModel:       derefString(requestedModel),
		RequestedProfile:     derefString(requestedProfile),
		TimeoutMillis:        timeoutMillis,
	}, nil
}

// FactoryPreviewResultFromPreview maps the shared preview contract to the canonical Factory preview API shape.
func FactoryPreviewResultFromPreview(preview factory.WorkflowPreview) factoryapi.FactoryPreviewResult {
	return factoryapi.FactoryPreviewResult{
		Valid:                  preview.Valid,
		SourceResolution:       workflowSourceResolutionFromPreview(preview.SourceResolution),
		SourceValidationIssues: workflowDiagnosticsFromSourceIssues(preview.SourceValidationIssues),
		PolicyPreview:          workflowPolicyPreviewFromPreview(preview.PolicyPreview),
		ResultConstraints:      workflowResultConstraintsFromPreview(preview.ResultConstraints),
	}
}

func workflowSourceKindFromAPI(kind string) (factory.WorkflowSourceKind, error) {
	switch factory.WorkflowSourceKind(strings.TrimSpace(kind)) {
	case factory.WorkflowSourceKindFactoryID,
		factory.WorkflowSourceKindFactoryInline,
		factory.WorkflowSourceKindWorkflowFile,
		factory.WorkflowSourceKindWorkflowName,
		factory.WorkflowSourceKindInlineWorkflow:
		return factory.WorkflowSourceKind(strings.TrimSpace(kind)), nil
	default:
		return "", &RequestValidationError{Message: "sourceKind must be one of FACTORY_ID, FACTORY_INLINE, WORKFLOW_FILE, WORKFLOW_NAME, or INLINE_WORKFLOW"}
	}
}

func workflowSourceResolutionFromPreview(resolution factory.WorkflowSourceResolution) factoryapi.WorkflowSourceResolution {
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

func workflowArtifactRootFromPreview(decision factory.WorkflowSourceArtifactRootDecision) factoryapi.WorkflowArtifactRootDecision {
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

func workflowPolicyPreviewFromPreview(preview factory.JavaScriptPolicyPreview) factoryapi.WorkflowPolicyPreview {
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

func workflowResultConstraintsFromPreview(constraints factory.WorkflowResultConstraints) factoryapi.WorkflowResultConstraints {
	return factoryapi.WorkflowResultConstraints{
		RequiresStructuredCloneableJson: constraints.RequiresStructuredCloneableJSON,
		ArtifactUriScheme:               constraints.ArtifactURIScheme,
		MaxEmbeddedBytes:                constraints.MaxEmbeddedBytes,
		RejectedValueKinds:              append([]string(nil), constraints.RejectedValueKinds...),
	}
}

func workflowDiagnosticsFromSourceIssues(issues []factory.WorkflowPreviewSourceValidationIssue) []factoryapi.WorkflowDiagnostic {
	out := make([]factoryapi.WorkflowDiagnostic, 0, len(issues))
	for _, issue := range issues {
		out = append(out, workflowDiagnosticFromSourceIssue(issue))
	}
	return out
}

func workflowDiagnosticFromSourceIssue(issue factory.WorkflowPreviewSourceValidationIssue) factoryapi.WorkflowDiagnostic {
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

func workflowDiagnosticsFromSource(diagnostics []factory.WorkflowSourceDiagnostic) []factoryapi.WorkflowDiagnostic {
	out := make([]factoryapi.WorkflowDiagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		out = append(out, workflowDiagnosticFromSource(diagnostic))
	}
	return out
}

func workflowDiagnosticFromSource(diagnostic factory.WorkflowSourceDiagnostic) factoryapi.WorkflowDiagnostic {
	return factoryapi.WorkflowDiagnostic{
		Code:    diagnostic.Code,
		Message: diagnostic.Message,
	}
}

func workflowDiagnosticsFromPolicy(diagnostics []factory.JavaScriptPolicyDiagnostic) []factoryapi.WorkflowDiagnostic {
	out := make([]factoryapi.WorkflowDiagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		out = append(out, factoryapi.WorkflowDiagnostic{
			Code:    diagnostic.Code,
			Message: diagnostic.Message,
		})
	}
	return out
}

func workflowDiagnosticsFromPolicyIssues(issues []factory.JavaScriptPolicyIssue) []factoryapi.WorkflowDiagnostic {
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

// FactoryWorkflowValidationResult is the shared workflow source validation contract
// for CLI loopback and future HTTP parity surfaces.
type FactoryWorkflowValidationResult struct {
	Valid               bool                                `json:"valid"`
	SourceResolution    factoryapi.WorkflowSourceResolution `json:"sourceResolution"`
	BlockingDiagnostics []factoryapi.WorkflowDiagnostic     `json:"blockingDiagnostics"`
}

// FactoryWorkflowValidationResultFromPreview maps preview output to the validation contract.
func FactoryWorkflowValidationResultFromPreview(preview factory.WorkflowPreview) FactoryWorkflowValidationResult {
	previewResult := FactoryPreviewResultFromPreview(preview)
	return FactoryWorkflowValidationResult{
		Valid:               preview.Valid,
		SourceResolution:    previewResult.SourceResolution,
		BlockingDiagnostics: blockingDiagnosticsFromPreviewResult(previewResult),
	}
}

func blockingDiagnosticsFromPreviewResult(result factoryapi.FactoryPreviewResult) []factoryapi.WorkflowDiagnostic {
	out := make([]factoryapi.WorkflowDiagnostic, 0)
	if result.SourceResolution.Diagnostics != nil {
		if !result.SourceResolution.Found {
			out = append(out, *result.SourceResolution.Diagnostics...)
		} else {
			for _, diagnostic := range *result.SourceResolution.Diagnostics {
				if diagnostic.Code == factory.WorkflowSourceCodeConflict {
					out = append(out, diagnostic)
				}
			}
		}
	}
	if result.SourceResolution.ArtifactRoot != nil &&
		result.SourceResolution.ArtifactRoot.Diagnostic != nil &&
		!result.SourceResolution.ArtifactRoot.Allowed &&
		strings.TrimSpace(result.SourceResolution.ArtifactRoot.Requested) != "" {
		out = append(out, *result.SourceResolution.ArtifactRoot.Diagnostic)
	}
	out = append(out, result.SourceValidationIssues...)
	out = append(out, result.PolicyPreview.ValidationIssues...)
	return out
}
