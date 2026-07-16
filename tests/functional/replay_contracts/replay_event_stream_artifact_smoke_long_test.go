//go:build functionallong

package replay_contracts

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestReplayEventStreamArtifactSmoke_ReplaysCheckedInSampleArtifact(t *testing.T) {
	support.SkipLongFunctional(t, "slow replay artifact event-stream smoke")

	artifactPath := support.RecordReplayFixture(t)
	assertReplayArtifactReplaysOverSSE(t, t.TempDir(), "", artifactPath)
}

func TestReplayEventStreamArtifactSmoke_ReplaysWithCopiedRootFactoryDefinition(t *testing.T) {
	support.SkipLongFunctional(t, "slow replay artifact root-factory portability sweep")

	copiedFactoryDir := testutil.CopyFixtureDir(t, testutil.MustRepoPath(t, "tests/adhoc/factory"))
	artifactPath := support.RecordReplayFixture(t)
	assertReplayArtifactReplaysOverSSE(t, copiedFactoryDir, copiedFactoryDir, artifactPath)
}

func assertReplayArtifactReplaysOverSSE(
	t *testing.T,
	factoryDir string,
	executionBaseDir string,
	artifactPath string,
) {
	t.Helper()

	server := startReplayFunctionalServer(t, factoryDir, artifactPath, executionBaseDir)
	stream := openFactoryEventHTTPStream(t, support.DefaultSessionEventsURL(server.URL()))
	runStarted, first := requireFunctionalEventStreamPrelude(t, stream)
	events := collectUnifiedSmokeEventsUntilRunResponse(
		t,
		stream,
		[]factoryapi.FactoryEvent{runStarted, first},
		30*time.Second,
	)
	stream.close()

	select {
	case <-server.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for replay server run to finish")
	}

	assertReplayEventStreamTerminalOutcome(t, events)
}

func assertReplayEventStreamTerminalOutcome(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	if len(events) < 3 {
		t.Fatalf("replay event stream has %d events, want prelude and terminal response", len(events))
	}
	for i := 1; i < len(events); i++ {
		if events[i].Context.Sequence <= events[i-1].Context.Sequence {
			t.Fatalf("event sequences are not strictly increasing at %d: %d then %d", i, events[i-1].Context.Sequence, events[i].Context.Sequence)
		}
	}
	terminal := events[len(events)-1]
	if terminal.Type != factoryapi.FactoryEventTypeRunResponse {
		t.Fatalf("terminal event = %s, want RUN_RESPONSE", terminal.Type)
	}
	payload, err := terminal.Payload.AsRunResponseEventPayload()
	if err != nil {
		t.Fatalf("decode terminal run response: %v", err)
	}
	if payload.State == nil || *payload.State != factoryapi.FactoryStateCompleted {
		t.Fatalf("terminal run state = %#v, want %q", payload.State, factoryapi.FactoryStateCompleted)
	}
}
