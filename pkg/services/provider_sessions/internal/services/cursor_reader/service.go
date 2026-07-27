// Package cursor_reader defines the parent-private historical Cursor Provider
// Session reader selected by the Provider Sessions root.
package cursor_reader

import (
	"context"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
)

// Service reads one canonical Cursor Provider Session reference and returns
// only accepted normalized Provider Sessions detail.
type Service interface {
	Read(context.Context, providers.SessionRef) (providersessions.Detail, error)
}
