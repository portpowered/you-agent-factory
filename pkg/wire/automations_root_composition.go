package wire

import (
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

func provideAutomationsCommandRunner(edges serviceedges.Edges) (platformprocess.CommandRunner, error) {
	if edges.ScriptCommandRunner != nil {
		return edges.ScriptCommandRunner, nil
	}
	return providePlatformProcessCommandRunner(edges)
}
