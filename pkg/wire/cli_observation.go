package wire

import (
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
)

func provideCLIObserver(edges serviceedges.Edges) platformprocess.CLIObserver {
	return edges.CLIObserver
}

func provideCLICommandFactory(operations cli.CommandOperations) cli.CommandFactory {
	return cli.NewCommandFactory(operations)
}
