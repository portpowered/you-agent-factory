package invocations

import "github.com/portpowered/infinite-you/pkg/interfaces"

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
