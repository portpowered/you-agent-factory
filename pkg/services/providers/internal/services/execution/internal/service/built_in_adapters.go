package service

import (
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	claudeadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/claude"
	codexadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/codex"
	cursoradapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/cursor"
	opencodeadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/opencode"
)

// BuiltInDependencies carries exact provider-native invocation effects.
type BuiltInDependencies struct {
	Codex    codexadapter.Effect
	Claude   claudeadapter.Effect
	Cursor   cursoradapter.Effect
	OpenCode opencodeadapter.Effect
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
		claudeadapter.NewRegistration(effects.Claude),
		cursoradapter.NewRegistration(effects.Cursor),
		opencodeadapter.NewRegistration(effects.OpenCode),
	}
}
