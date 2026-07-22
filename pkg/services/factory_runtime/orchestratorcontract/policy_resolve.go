package orchestratorcontract

import (
	"encoding/json"
	"fmt"
)

// Resolve merges factory and request policy layers into one effective policy.
func Resolve(request Request) Resolution {
	policy := DefaultEffectivePolicy()
	var issues []Issue

	merged, documentIssue, err := mergedPolicyMap(request)
	if documentIssue != nil {
		issues = append(issues, *documentIssue)
		return Resolution{Policy: policy, Hash: Hash(policy), Issues: issues}
	}
	if err != nil {
		issues = append(issues, Issue{
			Code:    CodeInvalidPolicyDocument,
			Message: err.Error(),
			Path:    "policy",
		})
		return Resolution{Policy: policy, Hash: Hash(policy), Issues: issues}
	}

	policy, err = policyFromMap(merged)
	if err != nil {
		issues = append(issues, Issue{
			Code:    CodeInvalidPolicyDocument,
			Message: fmt.Sprintf("invalid effective policy: %v", err),
			Path:    "policy",
		})
		return Resolution{Policy: policy, Hash: Hash(policy), Issues: issues}
	}

	cap := deploymentCap(request)
	issues = append(issues, validatePolicyMap(merged, cap)...)
	issues = append(issues, Validate(policy, cap)...)
	return Resolution{
		Policy: policy,
		Hash:   Hash(policy),
		Issues: issues,
	}
}

func mergedPolicyMap(request Request) (map[string]any, *Issue, error) {
	merged := defaultPolicyMap()

	factoryDocument, err := parsePolicyDocument(request.FactoryDefault)
	if err != nil {
		return nil, &Issue{
			Code:    CodeInvalidPolicyDocument,
			Message: err.Error(),
			Path:    "orchestrator.javascript.defaultPolicy",
		}, nil
	}
	overlayMap(merged, factoryDocument)

	if len(request.Requested) > 0 {
		overlayMap(merged, request.Requested)
	}
	return merged, nil, nil
}

func defaultPolicyMap() map[string]any {
	policy := DefaultEffectivePolicy()
	encoded, err := json.Marshal(policy)
	if err != nil {
		return map[string]any{}
	}
	var merged map[string]any
	if err := json.Unmarshal(encoded, &merged); err != nil {
		return map[string]any{}
	}
	return merged
}

func overlayMap(base map[string]any, overrides map[string]any) {
	for key, value := range overrides {
		base[key] = value
	}
}

// ResolveFromFactoryDefault resolves policy from a factory defaultPolicy blob.
func ResolveFromFactoryDefault(raw json.RawMessage) Resolution {
	return Resolve(Request{FactoryDefault: raw})
}
