// Package work implements composition-facing facades over the Work-owned CLI adapter.
package work

import (
	workcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
)

type ShowConfig = workcli.ShowConfig

func NewShow(transport clihttp.Protocol) func(ShowConfig) error {
	return workcli.NewShow(transport)
}

func Show(cfg ShowConfig) error {
	return workcli.Show(cfg)
}
