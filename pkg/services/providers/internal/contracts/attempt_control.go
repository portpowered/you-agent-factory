package contracts

import "context"

// AttemptControl is a process-local capability for one exact Provider
// execution, exposed to peers through the Providers root contract alias.
// It is operation data, not a separately constructed product service.
type AttemptControl interface {
	// ForceKill confirms the owned process tree and Provider execution joined.
	// Unsupported or expired handles return false without effects.
	ForceKill(context.Context) (bool, error)
}
