//go:build !functionallong

package replay_contracts

import (
	"os"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/interfaces"
)

func assertReplayArtifactDoesNotContainRawValue(t *testing.T, artifactPath, rawValue string) {
	t.Helper()

	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("read replay artifact %s: %v", artifactPath, err)
	}
	if strings.Contains(string(data), rawValue) {
		t.Fatalf("replay artifact %s leaked raw environment value %q", artifactPath, rawValue)
	}
}

func replayEventCount(artifact *interfaces.ReplayArtifact, eventType factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range artifact.Events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func factoryWorksValue(value *[]factoryapi.Work) []factoryapi.Work {
	if value == nil {
		return nil
	}
	return *value
}

func factoryRelationsValue(value *[]factoryapi.Relation) []factoryapi.Relation {
	if value == nil {
		return nil
	}
	return *value
}

func stringPointerValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func stringSlicePointerValue(value *[]string) []string {
	if value == nil {
		return nil
	}
	return *value
}

func lastFactoryEventTick(events []factoryapi.FactoryEvent) int {
	tick := 0
	for _, event := range events {
		if event.Context.Tick > tick {
			tick = event.Context.Tick
		}
	}
	return tick
}
