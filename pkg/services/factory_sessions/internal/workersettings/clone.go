package workersettings

import factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"

// Clone detaches settings carried across a Factory Session execution boundary.
func Clone(settings *factoryruntime.JavaScriptWorkerSettings) *factoryruntime.JavaScriptWorkerSettings {
	if settings == nil {
		return nil
	}
	cloned := *settings
	if settings.Presets != nil {
		cloned.Presets = make(map[string]factoryruntime.JavaScriptWorkerPreset, len(settings.Presets))
		for id, preset := range settings.Presets {
			cloned.Presets[id] = preset
		}
	}
	return &cloned
}
