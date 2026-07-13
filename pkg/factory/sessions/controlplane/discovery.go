package controlplane

import (
	"github.com/portpowered/infinite-you/pkg/factory/sessions"
)

// DiscoveryHost discovers runnable factory session targets under one folder.
type DiscoveryHost interface {
	DiscoverTargets(folderPath string) ([]factorysessions.Target, error)
}
