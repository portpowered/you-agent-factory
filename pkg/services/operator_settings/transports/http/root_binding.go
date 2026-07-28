package http

import operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"

// SettingsRoot is the accepted Operator Settings root contract used by the HTTP
// adapter. Adapter-owned operations invoke this surface rather than Settings
// internal packages.
type SettingsRoot = operatorsettings.Service

// RootBinding binds the HTTP adapter to one injected Settings root.
type RootBinding struct {
	Settings SettingsRoot
}

// NewAdapterFromRoot constructs an HTTP adapter that calls through the supplied
// Settings root. Tests inject a focused fake implementing Service operations
// without constructing document stores, filesystem codecs, resolution Wire
// graphs, or service-local composition.
func NewAdapterFromRoot(binding RootBinding) *Adapter {
	return NewAdapter(binding.Settings)
}
