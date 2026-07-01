package service

import (
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/controlplane"
	"github.com/portpowered/infinite-you/pkg/factorysessions/dataplane"
)

// Host exposes composition-root seams required by the session gateway.
type Host interface {
	controlplane.OpenControlHost
	dataplane.LiveOpenHost
	RequireSession(sessionID string) (*factorysessions.LiveSession, error)
}
