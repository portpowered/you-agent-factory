package functional_test

import (
	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/interfaces"
)

func replayEventCount(artifact *interfaces.ReplayArtifact, eventType factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range artifact.Events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}
