package replay

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/agent-factory/internal/testpath"
	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
)

func TestArtifactFromEventStream_ParsesCanonicalEventStreamAndSkipsTruncatedTail(t *testing.T) {
	artifact := testReplayArtifact(t,
		replayWorkRequestEvent(t, "request-1", 1, "api", []factoryapi.Work{{
			Name:         "task-1",
			TraceId:      stringPtrIfNotEmpty("trace-1"),
			WorkId:       stringPtrIfNotEmpty("work-1"),
			WorkTypeName: stringPtrIfNotEmpty("task"),
		}}, nil),
	)

	data, err := json.Marshal(artifact.Events[0])
	if err != nil {
		t.Fatalf("Marshal event: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	stream := "data: " + strings.Join(lines, "\n") + "\n\n" +
		`data: {"id":"truncated"` + "\n"

	result, err := ArtifactFromEventStream(strings.NewReader(stream))
	if err != nil {
		t.Fatalf("ArtifactFromEventStream: %v", err)
	}

	if result.ParsedEvents != 1 {
		t.Fatalf("ParsedEvents = %d, want 1", result.ParsedEvents)
	}
	if result.SkippedTrailingBlocks != 1 {
		t.Fatalf("SkippedTrailingBlocks = %d, want 1", result.SkippedTrailingBlocks)
	}
	if got := result.Artifact.Factory.Workers; got == nil || len(*got) != 1 {
		t.Fatalf("artifact factory workers = %#v, want hydrated factory config", got)
	}
}

func TestArtifactFromEventStreamFile_ConvertsAgentFailsLog(t *testing.T) {
	path := testpath.MustRepoPathFromCaller(t, 0, "factory", "logs", "agent-fails.json")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("root event-stream fixture not present in this checkout: %v", err)
	}

	result, err := ArtifactFromEventStreamFile(path)
	if err != nil {
		t.Fatalf("ArtifactFromEventStreamFile: %v", err)
	}

	if result.ParsedEvents < 1000 {
		t.Fatalf("ParsedEvents = %d, want large recovered event stream", result.ParsedEvents)
	}
	if result.Artifact.RecordedAt.IsZero() {
		t.Fatal("artifact recordedAt is zero, want hydrated run-start timestamp")
	}
	if got := result.Artifact.Factory.WorkTypes; got == nil || len(*got) == 0 {
		t.Fatalf("artifact factory work types = %#v, want hydrated factory config", got)
	}
	if guards := generatedWorkstationGuardsByName(t, result.Artifact.Factory, "executor-loop-breaker"); len(guards) != 1 {
		t.Fatalf("executor-loop-breaker guards = %#v, want hydrated visit-count guard", guards)
	}
}

func generatedWorkstationGuardsByName(t *testing.T, factory factoryapi.Factory, name string) []factoryapi.WorkstationGuard {
	t.Helper()
	if factory.Workstations == nil {
		t.Fatal("generated factory workstations = nil")
	}
	for _, workstation := range *factory.Workstations {
		if workstation.Name != name {
			continue
		}
		if workstation.Guards == nil {
			return nil
		}
		return append([]factoryapi.WorkstationGuard(nil), (*workstation.Guards)...)
	}
	t.Fatalf("generated factory missing workstation %q", name)
	return nil
}
