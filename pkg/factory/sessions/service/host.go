package service

import (
	"github.com/portpowered/infinite-you/pkg/factory/sessions/controlplane"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/dataplane"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/stream"
)

// Host exposes composition-root seams required by the session gateway.
type Host interface {
	controlplane.OpenControlHost
	controlplane.LiveReadHost
	controlplane.SyncPreflightHost
	controlplane.ResultReadHost
	controlplane.DurableLifecycleHost
	dataplane.LiveOpenHost
	dataplane.LiveLifecycleHost
	stream.Host
}
