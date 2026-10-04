package service

import (
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	agyadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy"
	claudeadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/claude"
	codexadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/codex"
)

// BuiltInRegistrations returns the immutable set of native adapters currently
// owned by Providers Execution. Identity, aliases, availability, and maximum
// capabilities are deliberately absent: the execution registry binds those
// facts from the canonical Providers catalog.
func BuiltInRegistrations(
	antigravity agyadapter.Effect,
	codex codexadapter.Effect,
	claude claudeadapter.Effect,
) []execution.Registration {
	return []execution.Registration{
		agyadapter.NewRegistration(antigravity),
		codexadapter.NewRegistration(codex),
		claudeadapter.NewRegistration(claude),
	}
}
