// Package config owns worker configuration and capability contracts.
package workerconfig

import (
	"strings"
	"time"

	factoryresource "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/resource"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/authoredmodel/namevalue"
)

type Config struct {
	ID               string                    `json:"id,omitempty" yaml:"id,omitempty"`
	Name             string                    `json:"name" yaml:"name,omitempty"`
	Description      *namevalue.Config         `json:"description,omitempty" yaml:"description,omitempty"`
	Type             string                    `json:"type" yaml:"type"`
	Provider         string                    `json:"provider,omitempty" yaml:"provider,omitempty"`
	Model            string                    `json:"model,omitempty" yaml:"model,omitempty"`
	ModelProvider    string                    `json:"modelProvider,omitempty" yaml:"modelProvider,omitempty"`
	ModelLocality    string                    `json:"modelLocality,omitempty" yaml:"modelLocality,omitempty"`
	ExecutorProvider string                    `json:"executorProvider,omitempty" yaml:"executorProvider,omitempty"`
	Operations       []ModelOperation          `json:"operations,omitempty" yaml:"operations,omitempty"`
	Command          string                    `json:"command,omitempty" yaml:"command,omitempty"`
	Args             []string                  `json:"args,omitempty" yaml:"args,omitempty"`
	Resources        []factoryresource.Config  `json:"resources,omitempty" yaml:"resources,omitempty"`
	Timeout          string                    `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	StopToken        string                    `json:"stopToken,omitempty" yaml:"stopToken,omitempty"`
	SkipPermissions  bool                      `json:"skipPermissions,omitempty" yaml:"skipPermissions,omitempty"`
	Auth             *HostedWorkerAuthConfig   `json:"auth,omitempty" yaml:"auth,omitempty"`
	Linear           *HostedLinearWorkerConfig `json:"linear,omitempty" yaml:"linear,omitempty"`
	AgentTools       *AgentToolsConfig         `json:"agentTools,omitempty" yaml:"agentTools,omitempty"`
	Body             string                    `json:"body,omitempty" yaml:"-"`
	SessionID        string                    `json:"-" yaml:"-"`
	Concurrency      int                       `json:"-" yaml:"-"`
	// RuntimeDefaultModelProvider and RuntimeDefaultModel retain operator
	// fallbacks for invocation-interpolated selections. They are effective
	// runtime metadata and never part of an authored Factory definition.
	RuntimeDefaultModelProvider string `json:"-" yaml:"-"`
	RuntimeDefaultModel         string `json:"-" yaml:"-"`
}

func (w *Config) TimeoutDuration() time.Duration {
	if w == nil || w.Timeout == "" {
		return 0
	}
	d, _ := time.ParseDuration(w.Timeout)
	return d
}

type HostedWorkerAuthConfig struct {
	SecretRef string `json:"secretRef,omitempty" yaml:"secretRef,omitempty"`
}
type HostedLinearWorkerConfig struct {
	PollInterval string                          `json:"pollInterval,omitempty" yaml:"pollInterval,omitempty"`
	TeamIDs      []string                        `json:"teamIds,omitempty" yaml:"teamIds,omitempty"`
	StateIDs     []string                        `json:"stateIds,omitempty" yaml:"stateIds,omitempty"`
	Mapping      HostedLinearWorkerMappingConfig `json:"mapping,omitempty" yaml:"mapping,omitempty"`
	Claim        *HostedLinearWorkerClaimConfig  `json:"claim,omitempty" yaml:"claim,omitempty"`
}
type HostedLinearWorkerMappingConfig struct {
	WorkType string `json:"workType,omitempty" yaml:"workType,omitempty"`
	State    string `json:"state,omitempty" yaml:"state,omitempty"`
}
type HostedLinearWorkerClaimConfig struct {
	AssigneeField string `json:"assigneeField,omitempty" yaml:"assigneeField,omitempty"`
}

const (
	ModelLocalityLocal              = "LOCAL"
	ModelLocalityCloud              = "CLOUD"
	ModelOperationContentTypeText   = "TEXT"
	ModelOperationContentTypeImage  = "IMAGE"
	ModelOperationContentTypeAudio  = "AUDIO"
	ModelOperationContentTypeJSON   = "JSON"
	ModelOperationContentTypeBinary = "BINARY"
)

type ModelOperation struct {
	Name    string               `json:"name" yaml:"name"`
	Inputs  []ModelOperationSlot `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	Outputs []ModelOperationSlot `json:"outputs,omitempty" yaml:"outputs,omitempty"`
}
type ModelOperationSlot struct {
	Name         string   `json:"name" yaml:"name"`
	ContentTypes []string `json:"contentTypes,omitempty" yaml:"contentTypes,omitempty"`
	Required     bool     `json:"required,omitempty" yaml:"required,omitempty"`
}

const (
	AgentToolPolicyDisabled = "DISABLED"
	AgentToolPolicyReadOnly = "READ_ONLY"
	AgentToolPolicyEnabled  = "ENABLED"
)

type AgentToolsConfig struct {
	Policy string `json:"policy" yaml:"policy"`
}

func EffectiveAgentToolPolicy(cfg *AgentToolsConfig) string {
	if cfg == nil || strings.TrimSpace(cfg.Policy) == "" {
		return AgentToolPolicyDisabled
	}
	return strings.TrimSpace(cfg.Policy)
}
func NormalizeAgentToolPolicy(policy string) string {
	switch strings.TrimSpace(policy) {
	case "", AgentToolPolicyDisabled:
		return AgentToolPolicyDisabled
	case AgentToolPolicyReadOnly:
		return AgentToolPolicyReadOnly
	case AgentToolPolicyEnabled:
		return AgentToolPolicyEnabled
	default:
		return strings.TrimSpace(policy)
	}
}
func IsKnownAgentToolPolicy(policy string) bool {
	switch NormalizeAgentToolPolicy(policy) {
	case AgentToolPolicyDisabled, AgentToolPolicyReadOnly, AgentToolPolicyEnabled:
		return true
	default:
		return false
	}
}
func AgentToolsAllowExecution(policy string) bool {
	switch NormalizeAgentToolPolicy(policy) {
	case AgentToolPolicyReadOnly, AgentToolPolicyEnabled:
		return true
	default:
		return false
	}
}
