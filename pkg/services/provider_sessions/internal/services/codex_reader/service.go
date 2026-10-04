// Package codexreader defines the parent-private Codex historical-session
// reader owned by Provider Sessions.
package codexreader

import (
	"context"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// Service resolves and projects Codex-native history into Provider
// Sessions-owned values. Only the Provider Sessions implementation consumes
// this parent-private contract.
type Service interface {
	Details(context.Context, providers.SessionRef) (providersessions.Detail, error)
}
