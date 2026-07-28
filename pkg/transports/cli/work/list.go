// Package work implements composition-facing facades over the Work-owned CLI adapter.
package work

import (
	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
	workcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
)

type ListConfig = workcli.ListConfig

func NewList(
	transport clihttp.Protocol,
	prepare workdomain.ListRequestPreparation,
) func(ListConfig) error {
	return workcli.NewList(transport, prepare)
}

func List(prepare workdomain.ListRequestPreparation, cfg ListConfig) error {
	return workcli.List(prepare, cfg)
}
