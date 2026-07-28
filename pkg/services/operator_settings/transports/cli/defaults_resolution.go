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
	return operatorsettings.Resolve(operatorsettings.ResolveInput{
		File: operatorsettings.Defaults{
			WorkerModelProvider: loaded.Document.Defaults.WorkerModelProvider,
			WorkerModel:         loaded.Document.Defaults.WorkerModel,
		},
		Env: operatorsettings.Defaults{
			WorkerModelProvider: strings.TrimSpace(cfg.Environment.WorkerModelProvider),
			WorkerModel:         strings.TrimSpace(cfg.Environment.WorkerModel),
		},
		Flag: operatorsettings.Defaults{
			WorkerModelProvider: strings.TrimSpace(cfg.Flags.WorkerModelProvider),
			WorkerModel:         strings.TrimSpace(cfg.Flags.WorkerModel),
		},
	}, path)
}
