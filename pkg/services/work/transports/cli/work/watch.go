package work

import (
	"io"

	workcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
)

type ReconnectWait = workcli.ReconnectWait

type WatchConfig = workcli.WatchConfig
type WatchTransition = workcli.WatchTransition

const WatchSchemaVersion = workcli.WatchSchemaVersion

func NewWatch(transport clihttp.Protocol, wait workcli.ReconnectWait) func(WatchConfig) error {
	return workcli.NewWatch(transport, wait)
}

func ValidateWatchConfig(cfg WatchConfig) error {
	return workcli.ValidateWatchConfig(cfg)
}

func RenderWatchTransition(output io.Writer, transition WatchTransition) error {
	return workcli.RenderWatchTransition(output, transition)
}
