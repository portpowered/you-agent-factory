package service

import (
	"context"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
)

// BuiltInRegistrations returns the immutable set of native adapters currently
// owned by Providers Execution. Identity, aliases, availability, and maximum
// capabilities are deliberately absent: the execution registry binds those
// facts from the canonical Providers catalog.
func BuiltInRegistrations() []execution.Registration {
	return []execution.Registration{
		unavailableRegistration(providers.IDCodex, "Codex"),
		unavailableRegistration(providers.IDClaude, "Claude"),
	}
}

func unavailableRegistration(
	id providers.ID,
	displayName string,
) execution.Registration {
	return execution.Registration{
		Provider: id,
		Attempt: func(
			context.Context,
			providers.ExecuteRequest,
		) (providers.ExecuteResult, error) {
			return providers.ExecuteResult{}, providers.ExecuteFailure{
				Kind: providers.ExecuteFailureKindDependency,
				Message: displayName +
					" native execution is unavailable",
			}
		},
	}
}
