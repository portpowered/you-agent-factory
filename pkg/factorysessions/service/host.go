package service

import (
	"github.com/portpowered/infinite-you/pkg/factorysessions/controlplane"
	"github.com/portpowered/infinite-you/pkg/factorysessions/dataplane"
	"github.com/portpowered/infinite-you/pkg/factorysessions/stream"
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
