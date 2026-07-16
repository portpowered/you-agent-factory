package workflowpolicy

import (
	"strings"

	workerrunner "github.com/portpowered/infinite-you/pkg/workers/runner"
)

// BuildPreview projects preview/session-start policy metadata from one request.
func BuildPreview(input PreviewInput) Preview {
	resolution := Resolve(input.Request)
	preview := Preview{
		EffectivePolicy:    resolution.Policy,
		PolicyHash:         resolution.Hash,
		MaxChildCount:      resolution.Policy.MaxAgents,
		MaxConcurrency:     resolution.Policy.Concurrency,
		ValidationIssues:   resolution.Issues,
		DeniedCapabilities: DeniedCapabilitiesForReadOnly(resolution.Policy),
		BudgetDecisions: BudgetDecisions{
			MaxChildCount:  resolution.Policy.MaxAgents,
			MaxConcurrency: resolution.Policy.Concurrency,
		},
		TimeoutDecisions: TimeoutDecisions{
			RequestedMillis: input.TimeoutMillis,
			EffectiveMillis: effectiveTimeoutMillis(input.TimeoutMillis, resolution.Policy.MaxRunDurationMs),
		},
	}
	if runner := strings.TrimSpace(input.RequestedRunner); runner != "" {
		preview.RunnerDecision = resolveRunnerDecision(runner, resolution.Policy)
	}
	if model := strings.TrimSpace(input.RequestedModel); model != "" {
		preview.ModelDecision = resolveModelDecision(model, resolution.Policy)
	}
	if profile := strings.TrimSpace(input.RequestedProfile); profile != "" {
		preview.ProfileDecision = resolveProfileDecision(profile, resolution.Policy)
	}
	return preview
}

func effectiveTimeoutMillis(requested, policyMax *int64) *int64 {
	if requested != nil && *requested > 0 {
		return requested
	}
	if policyMax != nil && *policyMax > 0 {
		return policyMax
	}
	return nil
}

func resolveRunnerDecision(requested string, policy EffectivePolicy) *RunnerDecision {
	normalized := workerrunner.NormalizeRunnerID(requested)
	decision := &RunnerDecision{
		Requested: requested,
		Resolved:  normalized,
		Allowed:   workerrunner.IsBuiltInRunnerID(normalized),
	}
	if !decision.Allowed {
		decision.Diagnostic = &Diagnostic{
			Code:    CodeUnsupportedRunner,
			Message: "requested runner is not supported by the effective policy",
		}
		return decision
	}
	if len(policy.AllowedRunners) == 0 {
		return decision
	}
	for _, allowed := range policy.AllowedRunners {
		if workerrunner.NormalizeRunnerID(allowed) == normalized {
			return decision
		}
	}
	decision.Allowed = false
	decision.Diagnostic = &Diagnostic{
		Code:    CodeUnsupportedRunner,
		Message: "requested runner is not listed in policy.allowedRunners",
	}
	return decision
}

func resolveModelDecision(requested string, policy EffectivePolicy) *ModelDecision {
	decision := &ModelDecision{
		Requested: requested,
		Resolved:  strings.TrimSpace(requested),
		Allowed:   strings.TrimSpace(requested) != "",
	}
	if !decision.Allowed {
		decision.Diagnostic = &Diagnostic{
			Code:    CodeUnsupportedModel,
			Message: "requested model must be a non-empty string",
		}
		return decision
	}
	if len(policy.AllowedModels) == 0 {
		return decision
	}
	for _, allowed := range policy.AllowedModels {
		if strings.TrimSpace(allowed) == decision.Resolved {
			return decision
		}
	}
	decision.Allowed = false
	decision.Diagnostic = &Diagnostic{
		Code:    CodeUnsupportedModel,
		Message: "requested model is not listed in policy.allowedModels",
	}
	return decision
}

func resolveProfileDecision(requested string, policy EffectivePolicy) *ProfileDecision {
	profile := strings.ToLower(strings.TrimSpace(requested))
	decision := &ProfileDecision{
		Requested: requested,
		Resolved:  profile,
		Allowed:   true,
	}
	if _, ok := knownRouteProfiles[profile]; !ok {
		decision.Allowed = false
		decision.Diagnostic = &Diagnostic{
			Code:    CodeUnsupportedRouteProfile,
			Message: "requested route profile is not supported by the effective policy",
		}
		return decision
	}
	if len(policy.AllowedRouteProfiles) == 0 {
		return decision
	}
	for _, allowed := range policy.AllowedRouteProfiles {
		if strings.ToLower(strings.TrimSpace(allowed)) == profile {
			return decision
		}
	}
	decision.Allowed = false
	decision.Diagnostic = &Diagnostic{
		Code:    CodeUnsupportedRouteProfile,
		Message: "requested route profile is not listed in policy.allowedRouteProfiles",
	}
	return decision
}
