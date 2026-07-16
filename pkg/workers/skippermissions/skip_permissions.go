// Package skippermissions owns worker capability and invocation-override policy
// for provider-backed agent execution.
package skippermissions

import (
	"fmt"
	"strings"

	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

// EffectiveSkipPermissions resolves the invocation-time skip-permissions policy for
// a provider-backed worker. Persisted skipPermissions: true always wins. When the
// invocation override is present and true, agent workers also skip permissions
// for that invocation only.
func EffectiveSkipPermissions(
	persisted bool,
	workerType string,
	invocationOverride *bool,
) bool {
	if persisted {
		return true
	}
	if invocationOverride != nil && *invocationOverride && workertaxonomy.IsAgentWorkerType(workerType) {
		return true
	}
	return false
}

// AgentWorkerSupportsSkipPermissions reports whether an agent worker can honor
// skip-permissions through a supported CLI provider adapter.
func AgentWorkerSupportsSkipPermissions(worker *workerconfig.Config) bool {
	if worker == nil || !workertaxonomy.IsAgentWorkerType(worker.Type) {
		return true
	}
	if workertaxonomy.UsesModelhostLease(worker.Type, worker.ModelLocality) {
		return false
	}
	provider := strings.TrimSpace(worker.ModelProvider)
	if provider == "" {
		return true
	}
	for _, supported := range modelprovider.Supported() {
		if provider == string(supported) {
			return true
		}
	}
	return false
}

// ValidateInvocationSkipPermissionsForWorker fails closed when an invocation
// requests --skip-permissions but the agent worker cannot honor it.
func ValidateInvocationSkipPermissionsForWorker(
	worker *workerconfig.Config,
	invocationOverride *bool,
) error {
	if invocationOverride == nil || !*invocationOverride {
		return nil
	}
	if worker == nil || !workertaxonomy.IsAgentWorkerType(worker.Type) {
		return nil
	}
	if AgentWorkerSupportsSkipPermissions(worker) {
		return nil
	}
	return fmt.Errorf(
		"--skip-permissions requires a supported agent CLI provider; %s",
		agentWorkerSkipPermissionsUnsupportedDetail(worker),
	)
}

// ValidateInvocationSkipPermissionsWorkers validates every configured agent worker
// before dispatch when an invocation-scoped skip-permissions override is active.
func ValidateInvocationSkipPermissionsWorkers(
	factoryCfg *interfaces.FactoryConfig,
	runtimeCfg interfaces.RuntimeConfigLookup,
	invocationOverride *bool,
) error {
	if invocationOverride == nil || !*invocationOverride {
		return nil
	}
	if factoryCfg == nil || runtimeCfg == nil {
		return nil
	}
	for _, workerCfg := range factoryCfg.Workers {
		def, ok := runtimeCfg.Worker(workerCfg.Name)
		if !ok || def == nil || def.Type == "" {
			continue
		}
		if err := ValidateInvocationSkipPermissionsForWorker(def, invocationOverride); err != nil {
			return fmt.Errorf("worker %q: %w", workerCfg.Name, err)
		}
	}
	return nil
}

func agentWorkerSkipPermissionsUnsupportedDetail(worker *workerconfig.Config) string {
	if worker == nil {
		return "agent worker cannot honor unsafe permission bypass"
	}
	if workertaxonomy.UsesModelhostLease(worker.Type, worker.ModelLocality) {
		return "local managed model workers cannot honor CLI skip-permissions"
	}
	if provider := strings.TrimSpace(worker.ModelProvider); provider != "" {
		return fmt.Sprintf("model provider %q does not support skip-permissions", provider)
	}
	return "agent worker has no supported CLI model provider"
}
