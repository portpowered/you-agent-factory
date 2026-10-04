// Package cursorscopes owns scoped script-poller recovery facts.
package cursorscopes

import (
	"context"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
)

type CursorScope struct {
	RuntimeID string
	BaseDir   string
}

type CommitCursorRequest struct {
	AutomationID   string
	InstanceID     string
	ExpectedCursor automations.Cursor
	Cursor         automations.Cursor
	Checkpoint     string
}

type ResumeCursor struct {
	Cursor     automations.Cursor
	Checkpoint string
}

type CursorScopes interface {
	GetCursor(context.Context, CursorScope, automations.GetCursorRequest) (automations.GetCursorResult, error)
	CommitCursor(context.Context, CursorScope, CommitCursorRequest) error
	// ReleaseScope discards runtime resources after the caller has stopped and
	// joined all scope users. Durable recovery files remain available on restart.
	ReleaseScope(CursorScope)
}
