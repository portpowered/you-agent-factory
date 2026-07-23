package factorysessions

import cursors "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/cursors"

// Reconnect cursor recovery contracts are exposed from the Factory Sessions
// root. Cursor tracking and persistence remain owner-internal.
type (
	CursorIdentityScope   = cursors.IdentityScope
	CursorPreflightReason = cursors.PreflightReason
	CursorPreflightResult = cursors.PreflightResult
	CursorStore           = cursors.Store
)

const (
	CursorPreflightStale           = cursors.PreflightCursorStale
	CursorPreflightSessionNotFound = cursors.PreflightSessionNotFound
	CursorPreflightSessionRemapped = cursors.PreflightSessionRemapped
)
