// Package work implements composition-facing facades over the Work-owned CLI adapter.
package work

import (
	workcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
)

type MoveConfig = workcli.MoveConfig
type MoveSuccessResult = workcli.MoveSuccessResult

func NewMove(transport clihttp.Protocol) func(MoveConfig) error {
	return workcli.BindMove(transport)
}

func Move(cfg MoveConfig) error {
	return workcli.Move(cfg)
}
