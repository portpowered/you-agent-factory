package service

import (
	"github.com/portpowered/infinite-you/pkg/factorysessions/controlplane"
	"github.com/portpowered/infinite-you/pkg/factorysessions/dataplane"
)

// Host exposes composition-root seams required by the session gateway.
type Host interface {
	controlplane.OpenControlHost
	controlplane.LiveReadHost
	dataplane.LiveOpenHost
	dataplane.LiveLifecycleHost
}
