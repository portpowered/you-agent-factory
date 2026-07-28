package run

import (
	factoryruntimecli "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/cli"
)

func runtimeCLIService(cfg RunConfig) factoryruntimecli.Service {
	if cfg.RuntimeCLI != nil {
		return cfg.RuntimeCLI
	}
	return factoryruntimecli.BindService(factoryruntimecli.Config{})
}
