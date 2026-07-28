package http

import (
	"github.com/portpowered/infinite-you/pkg/services/automations"
)

// AutomationsRoot is the accepted Automations root contract used by the HTTP
// adapter. Adapter-owned operations invoke this surface rather than Automations
// internal packages.
type AutomationsRoot = automations.Root

// RootBinding binds the HTTP adapter to one injected Automations root.
type RootBinding struct {
	Automations AutomationsRoot
}

// NewAdapterFromRoot constructs an HTTP adapter that calls through the supplied
// Automations root. Tests inject a focused fake implementing Root operations
// without constructing real reconciliation, cron, script-poller, filesystem-watcher,
// hosted-source, or service-local Wire graphs.
func NewAdapterFromRoot(binding RootBinding) *Adapter {
	return NewAdapter(binding.Automations)
}
