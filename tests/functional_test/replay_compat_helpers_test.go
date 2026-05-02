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

func stringPointerValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func eventString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func eventStringSlice(values *[]string) []string {
	if values == nil {
		return nil
	}
	return *values
}
