// Package skippermissions owns worker capability and invocation-override policy
// for provider-backed agent execution.
package skippermissions

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
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
	if invocationOverride != nil && *invocationOverride && interfaces.IsAgentWorkerType(workerType) {
		return true
	}
	return false
}
