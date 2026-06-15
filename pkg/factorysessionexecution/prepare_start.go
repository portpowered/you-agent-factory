package factorysessionexecution

import (
	"encoding/json"
	"fmt"
	"strings"

	workflowpolicy "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	workflowvalidation "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/validation"
)

// StartPrepareContext supplies filesystem and deployment inputs for durable start
// preparation before runtime execution begins.
type StartPrepareContext struct {
	StartSourceContext
	DeploymentCap int
}

// PreparedStart is the normalized, validated durable start tuple shared by
// runtime-backed async and sync session starts.
type PreparedStart struct {
	Request         StartRequest
	ResolvedSource  ResolvedSource
	Policy          PolicyProjection
	EffectivePolicy workflowpolicy.EffectivePolicy
	SourceRef       string
	SourceContent   string
	TupleHash       string
}

// PrepareStart normalizes one durable start request, resolves workflow source,
// validates source/args/policy/wait inputs, and projects effective policy before
// runtime execution begins.
func PrepareStart(req StartRequest, ctx StartPrepareContext) (PreparedStart, error) {
	normalized, err := NormalizeStartRequest(req)
	if err != nil {
		return PreparedStart{}, err
	}
	if err := validateStartArgs(normalized.Args); err != nil {
		return PreparedStart{}, err
	}
	if err := validateStartWait(normalized.Wait); err != nil {
		return PreparedStart{}, err
	}

	resolved, resolution, err := resolveStartSourceWithResolution(normalized, ctx.StartSourceContext)
	if err != nil {
		return PreparedStart{}, err
	}
	if err := validateResolvedSourceContent(resolution); err != nil {
		return PreparedStart{}, err
	}

	policyResolution := workflowpolicy.Resolve(workflowpolicy.Request{
		Requested:     normalized.RequestedPolicy,
		DeploymentCap: ctx.DeploymentCap,
	})
	if err := validationErrorFromPolicyIssues(policyResolution.Issues); err != nil {
		return PreparedStart{}, err
	}
	effectiveMap, err := effectivePolicyMap(policyResolution.Policy)
	if err != nil {
		return PreparedStart{}, fmt.Errorf("marshal effective policy: %w", err)
	}

	tupleHash, err := IdempotencyTupleHash(normalized)
	if err != nil {
		return PreparedStart{}, err
	}

	executableSource := strings.TrimSpace(resolution.Content)
	loaded, loadIssues := workflowvalidation.Load(workflowvalidation.LoadRequest{
		SourceRef: resolution.SourceRef,
		Content:   executableSource,
	})
	if len(loadIssues) > 0 {
		return PreparedStart{}, validationErrorFromSourceIssues(loadIssues)
	}

	return PreparedStart{
		Request:        normalized,
		ResolvedSource: resolved,
		Policy: PolicyProjection{
			Requested:     cloneArgs(normalized.RequestedPolicy),
			Effective:     effectiveMap,
			EffectiveHash: policyResolution.Hash,
		},
		EffectivePolicy: policyResolution.Policy,
		SourceRef:       loaded.SourceRef,
		SourceContent:   loaded.ExecutableSource,
		TupleHash:       tupleHash,
	}, nil
}

func resolveStartSourceWithResolution(req StartRequest, ctx StartSourceContext) (ResolvedSource, workflowsource.Resolution, error) {
	projectRoot := strings.TrimSpace(ctx.ProjectRoot)
	if projectRoot == "" {
		return ResolvedSource{}, workflowsource.Resolution{}, NewValidationError("projectRoot", "projectRoot is required")
	}

	sourceCtx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		return ResolvedSource{}, workflowsource.Resolution{}, NewValidationError("projectRoot", err.Error())
	}

	resolution := workflowsource.Resolve(startSourceRequest(req.Source), sourceCtx)
	if !resolution.Found {
		message := "workflow source could not be resolved"
		if len(resolution.Diagnostics) > 0 && strings.TrimSpace(resolution.Diagnostics[0].Message) != "" {
			message = resolution.Diagnostics[0].Message
		}
		return ResolvedSource{}, workflowsource.Resolution{}, NewValidationError("source", message)
	}

	resolved := ResolvedSource{
		Kind:       resolution.ResolvedKind,
		SourceRef:  resolution.SourceRef,
		SourceHash: resolution.SourceHash,
		Dialect:    resolution.Dialect,
		Metadata: map[string]string{
			"project": sourceCtx.ProjectRoot,
		},
	}
	if stage := resolutionOrderForLookupStage(resolution.LookupStage); stage != "" {
		resolved.ResolutionOrder = []string{stage}
	}
	return resolved, resolution, nil
}

func validateResolvedSourceContent(resolution workflowsource.Resolution) error {
	content := strings.TrimSpace(resolution.Content)
	if content == "" {
		return NewValidationError("source", "workflow source content is empty")
	}

	loaded, loadIssues := workflowvalidation.Load(workflowvalidation.LoadRequest{
		SourceRef: resolution.SourceRef,
		Content:   content,
	})
	if len(loadIssues) > 0 {
		return validationErrorFromSourceIssues(loadIssues)
	}

	validationResult := workflowvalidation.Validate(workflowvalidation.Request{
		Source:     wrapWorkflowSourceForValidation(loaded.ExecutableSource),
		SourceRef:  resolution.SourceRef,
		ConfigPath: "orchestrator.javascript",
		Metadata:   map[string]string{"project": resolution.SourceRef},
	})
	if validationResult.HasIssues() {
		return validationErrorFromSourceIssues(validationResult.Issues)
	}
	return nil
}

func wrapWorkflowSourceForValidation(source string) string {
	return "(function(){\n" + source + "\n})()"
}

func validateStartArgs(args map[string]any) error {
	if len(args) == 0 {
		return nil
	}
	if _, err := canonicalizeMap(args); err != nil {
		return NewValidationError("args", "workflow args must be JSON-compatible")
	}
	if _, err := json.Marshal(args); err != nil {
		return NewValidationError("args", "workflow args must be JSON-compatible")
	}
	return nil
}

func validateStartWait(wait *WaitOptions) error {
	if wait == nil || wait.TimeoutMillis == nil {
		return nil
	}
	if *wait.TimeoutMillis < 1 {
		return NewValidationError("wait.timeoutMillis", "timeoutMillis must be greater than zero when provided")
	}
	return nil
}

func validationErrorFromSourceIssues(issues []workflowvalidation.Issue) error {
	if len(issues) == 0 {
		return NewValidationError("source", "workflow source validation failed")
	}
	issue := issues[0]
	message := strings.TrimSpace(issue.Message)
	if message == "" {
		message = "workflow source validation failed"
	}
	message += issue.LocationSuffix()
	return NewValidationError("source", message)
}

func validationErrorFromPolicyIssues(issues []workflowpolicy.Issue) error {
	if len(issues) == 0 {
		return nil
	}
	issue := issues[0]
	message := strings.TrimSpace(issue.Message)
	if message == "" {
		message = "requested policy is invalid"
	}
	return NewValidationError("requestedPolicy", message)
}

func effectivePolicyMap(policy workflowpolicy.EffectivePolicy) (map[string]any, error) {
	encoded, err := json.Marshal(policy)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(encoded, &out); err != nil {
		return nil, err
	}
	return out, nil
}
