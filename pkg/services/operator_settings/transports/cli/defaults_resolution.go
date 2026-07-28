package cli

import (
	"fmt"
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// ResolveOperatorDefaultsConfig carries observed environment and flag layers for
// one defaults-resolution invocation.
type ResolveOperatorDefaultsConfig struct {
	HomeDir     string
	Environment operatorsettings.Defaults
	Flags       operatorsettings.FlagOverrides
}

// ResolveOperatorDefaults delegates defaults-resolution intent to the
// Settings-owned CLI adapter Service.
func ResolveOperatorDefaults(
	cfg ResolveOperatorDefaultsConfig,
	root operatorsettings.Service,
) (operatorsettings.ResolvedDefaults, error) {
	adapter := New(root)
	if adapter == nil {
		return operatorsettings.ResolvedDefaults{}, fmt.Errorf("operator settings service is required")
	}
	return adapter.ResolveOperatorDefaults(cfg)
}

func (service *service) ResolveOperatorDefaults(
	cfg ResolveOperatorDefaultsConfig,
) (operatorsettings.ResolvedDefaults, error) {
	homeDir := strings.TrimSpace(cfg.HomeDir)
	if homeDir == "" {
		return operatorsettings.ResolvedDefaults{}, fmt.Errorf("home directory is required")
	}
	path := operatorsettings.DefaultConfigPath(homeDir)
	loaded, err := service.root.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if err != nil {
		return operatorsettings.ResolvedDefaults{}, err
	}
	resolved, err := service.root.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: loaded.Document.Defaults,
		BackendScopeID:   loaded.Document.BackendScopeID,
		WorkerPresets:    loaded.Document.WorkerPresets,
		EnvironmentOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: strings.TrimSpace(cfg.Environment.WorkerModelProvider),
			WorkerModel:         strings.TrimSpace(cfg.Environment.WorkerModel),
		},
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: strings.TrimSpace(cfg.Flags.WorkerModelProvider),
			WorkerModel:         strings.TrimSpace(cfg.Flags.WorkerModel),
		},
		ConfigPath: path,
	})
	if err != nil {
		return operatorsettings.ResolvedDefaults{}, err
	}
	selection := resolved.Selection
	return operatorsettings.ResolvedDefaults{
		WorkerModelProvider:       selection.WorkerModelProvider,
		WorkerModel:               selection.WorkerModel,
		WorkerModelProviderSource: effectiveLayerSourceToSource(selection.WorkerModelProviderSource),
		WorkerModelSource:         effectiveLayerSourceToSource(selection.WorkerModelSource),
		ConfigPath:                selection.ConfigPath,
	}, nil
}

func effectiveLayerSourceToSource(
	source operatorsettings.EffectiveLayerSource,
) operatorsettings.Source {
	switch source {
	case operatorsettings.EffectiveLayerSourceEnv:
		return operatorsettings.SourceEnv
	case operatorsettings.EffectiveLayerSourceFlag:
		return operatorsettings.SourceFlag
	default:
		return operatorsettings.SourceFile
	}
}
