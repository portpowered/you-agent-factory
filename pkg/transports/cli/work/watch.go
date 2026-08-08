package work

import (
	"io"

	workcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli"
)

type WatchConfig = workcli.WatchConfig
type WatchTransition = workcli.WatchTransition

const WatchSchemaVersion = workcli.WatchSchemaVersion

func ValidateWatchConfig(cfg WatchConfig) error {
	return workcli.ValidateWatchConfig(cfg)
}

func RenderWatchTransition(output io.Writer, transition WatchTransition) error {
	return workcli.RenderWatchTransition(output, transition)
}
