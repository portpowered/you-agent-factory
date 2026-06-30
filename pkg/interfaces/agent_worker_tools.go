package interfaces

import (
	"strings"
)

const (
	AgentWorkerToolPolicyDisabled = "DISABLED"
	AgentWorkerToolPolicyReadOnly = "READ_ONLY"
	AgentWorkerToolPolicyEnabled  = "ENABLED"
)

// AgentWorkerToolsConfig carries explicit tool policy for AGENT_WORKER definitions.
type AgentWorkerToolsConfig struct {
	Policy string `json:"policy" yaml:"policy"`
}

// EffectiveAgentWorkerToolPolicy returns the configured policy or DISABLED when unset.
func EffectiveAgentWorkerToolPolicy(cfg *AgentWorkerToolsConfig) string {
	if cfg == nil || strings.TrimSpace(cfg.Policy) == "" {
		return AgentWorkerToolPolicyDisabled
	}
	return strings.TrimSpace(cfg.Policy)
}

// NormalizeAgentWorkerToolPolicy maps public API values onto canonical runtime strings.
func NormalizeAgentWorkerToolPolicy(policy string) string {
	switch strings.TrimSpace(policy) {
	case "", AgentWorkerToolPolicyDisabled:
		return AgentWorkerToolPolicyDisabled
	case AgentWorkerToolPolicyReadOnly:
		return AgentWorkerToolPolicyReadOnly
	case AgentWorkerToolPolicyEnabled:
		return AgentWorkerToolPolicyEnabled
	default:
		return strings.TrimSpace(policy)
	}
}

// IsKnownAgentWorkerToolPolicy reports whether policy is one of the supported modes.
func IsKnownAgentWorkerToolPolicy(policy string) bool {
	switch NormalizeAgentWorkerToolPolicy(policy) {
	case AgentWorkerToolPolicyDisabled, AgentWorkerToolPolicyReadOnly, AgentWorkerToolPolicyEnabled:
		return true
	default:
		return false
	}
}

// AgentWorkerToolsAllowExecution reports whether the policy permits tool execution.
func AgentWorkerToolsAllowExecution(policy string) bool {
	switch NormalizeAgentWorkerToolPolicy(policy) {
	case AgentWorkerToolPolicyReadOnly, AgentWorkerToolPolicyEnabled:
		return true
	default:
		return false
	}
}

// CloneAgentWorkerToolsConfig returns a detached copy of agent tool config.
func CloneAgentWorkerToolsConfig(cfg *AgentWorkerToolsConfig) *AgentWorkerToolsConfig {
	if cfg == nil {
		return nil
	}
	return &AgentWorkerToolsConfig{Policy: cfg.Policy}
}
