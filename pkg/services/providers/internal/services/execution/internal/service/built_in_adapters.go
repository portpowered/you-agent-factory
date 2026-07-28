package service

import (
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	agyadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy"
	claudeadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/claude"
	codexadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/codex"
	cursoradapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/cursor"
	geminiadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/gemini"
	kiroadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/kiro"
	opencodeadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/opencode"
	piadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/pi"
)

// BuiltInDependencies carries exact provider-native invocation effects.
type BuiltInDependencies struct {
	Agy      agyadapter.Effect
	Codex    codexadapter.Effect
	Claude   claudeadapter.Effect
	Cursor   cursoradapter.Effect
	Gemini   geminiadapter.Effect
	Kiro     kiroadapter.Effect
	OpenCode opencodeadapter.Effect
	Pi       piadapter.Effect
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
		agyadapter.NewRegistration(effects.Agy),
		codexadapter.NewRegistration(effects.Codex),
		claudeadapter.NewRegistration(effects.Claude),
		cursoradapter.NewRegistration(effects.Cursor),
		geminiadapter.NewRegistration(effects.Gemini),
		kiroadapter.NewRegistration(effects.Kiro),
		opencodeadapter.NewRegistration(effects.OpenCode),
		piadapter.NewRegistration(effects.Pi),
	}
}
