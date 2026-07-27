package service

import (
	"context"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	codexadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/codex"
)

// BuiltInDependencies carries exact provider-native invocation effects.
type BuiltInDependencies struct {
	Codex codexadapter.Effect
}

// BuiltInRegistrations returns the immutable set of native adapters currently
// owned by Providers Execution. Identity, aliases, availability, and maximum
// capabilities are deliberately absent: the execution registry binds those
// facts from the canonical Providers catalog.
func BuiltInRegistrations(
	dependencies ...BuiltInDependencies,
) []execution.Registration {
	var effects BuiltInDependencies
	if len(dependencies) > 0 {
		effects = dependencies[0]
	}
	return []execution.Registration{
		codexadapter.NewRegistration(effects.Codex),
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
