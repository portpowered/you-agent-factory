package orchestratorcontract

import (
	"encoding/json"
	"fmt"
	"strings"
)

func parsePolicyDocument(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("parse policy document: %w", err)
	}
	return document, nil
}

func policyFromMap(document map[string]any) (EffectivePolicy, error) {
	encoded, err := json.Marshal(document)
	if err != nil {
		return EffectivePolicy{}, err
	}
	var policy EffectivePolicy
	if err := json.Unmarshal(encoded, &policy); err != nil {
		return EffectivePolicy{}, err
	}
	return normalizePolicy(policy), nil
}

func normalizePolicy(policy EffectivePolicy) EffectivePolicy {
	policy.Mode = strings.TrimSpace(policy.Mode)
	if policy.Mode == "" {
		policy.Mode = ModeReadOnly
	}
	if policy.MaxAgents <= 0 {
		policy.MaxAgents = DefaultMaxAgents
	}
	if policy.Concurrency <= 0 {
		policy.Concurrency = defaultConcurrencyForMaxAgents(policy.MaxAgents)
	}
	if policy.MaxDepth <= 0 {
		policy.MaxDepth = DefaultMaxDepth
	}
	if policy.MaxRetries < 0 {
		policy.MaxRetries = DefaultMaxRetries
	}
	if policy.OutputAuditMode == "" {
		policy.OutputAuditMode = OutputAuditModeAuto
	}
	if policy.WritableRoots == nil {
		policy.WritableRoots = []string{}
	}
	return policy
}
