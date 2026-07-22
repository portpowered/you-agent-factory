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
	support.SkipLongFunctional(t, "slow replay checked-in sample artifact sweep")

	artifactPath := testutil.MustRepoPath(t, "factory/logs/agent-fails.replay.json")
	assertReplayArtifactReplaysOverSSEWithRuntimeMirroring(t, t.TempDir(), "", artifactPath)
}

func TestReplayEventStreamArtifactSmoke_ReplaysCheckedInSampleArtifactWithCopiedRootFactoryDefinition(t *testing.T) {
	support.SkipLongFunctional(t, "slow replay checked-in sample artifact sweep")

	copiedFactoryDir := testutil.CopyFixtureDir(t, testutil.MustRepoPath(t, "tests/adhoc/factory"))
	artifactPath := testutil.MustRepoPath(t, "factory/logs/agent-fails.replay.json")

	assertReplayArtifactReplaysOverSSEWithRuntimeMirroring(t, copiedFactoryDir, copiedFactoryDir, artifactPath)
}

func assertReplayArtifactReplaysOverSSEWithRuntimeMirroring(
	t *testing.T,
	factoryDir string,
	executionBaseDir string,
	artifactPath string,
) {
	t.Helper()

	server := startReplayFunctionalServer(
		t,
		factoryDir,
		false,
		executionBaseDir,
		"--replay",
		artifactPath,
	)

	stream := openFactoryEventHTTPStream(t, support.DefaultSessionEventsURL(server.URL()))
	runStarted, first := requireFunctionalEventStreamPrelude(t, stream)
	streamedEvents := collectUnifiedSmokeEventsUntilRunResponse(
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

	assertReplayEventTimelineWellFormed(t, streamedEvents)
	assertReplayPublicHistoryHasRuntimeActivity(t, streamedEvents)
}

func assertReplayEventTimelineWellFormed(
	t *testing.T,
	streamedEvents []factoryapi.FactoryEvent,
) {
	t.Helper()

	runResponseIndex := lastIndexOfFunctionalEventType(streamedEvents, factoryapi.FactoryEventTypeRunResponse)
	if runResponseIndex < 0 {
		t.Fatalf("public event history missing RUN_RESPONSE: %#v", unifiedSmokeEventSummaries(streamedEvents))
	}
	for index, event := range streamedEvents[:runResponseIndex+1] {
		if event.Id == "" {
			t.Fatalf("public event[%d] has empty id", index)
		}
		if index > 0 && event.Context.Sequence <= streamedEvents[index-1].Context.Sequence {
			t.Fatalf("public event sequence did not increase at index %d: %d <= %d", index, event.Context.Sequence, streamedEvents[index-1].Context.Sequence)
		}
	}
}

func assertReplayPublicHistoryHasRuntimeActivity(
	t *testing.T,
	streamedEvents []factoryapi.FactoryEvent,
) {
	t.Helper()

	if got := countReplayEvents(streamedEvents, factoryapi.FactoryEventTypeDispatchRequest); got == 0 {
		t.Fatal("public replay event history contains no dispatched Work")
	}
	for _, event := range streamedEvents {
		if event.Type != factoryapi.FactoryEventTypeRunResponse {
			continue
		}
		payload, err := event.Payload.AsRunResponseEventPayload()
		if err != nil {
			t.Fatalf("decode public RUN_RESPONSE %q: %v", event.Id, err)
		}
		if payload.State == nil || *payload.State == "" {
			t.Fatalf("public RUN_RESPONSE state = %#v, want a terminal Factory state", payload.State)
		}
		return
	}
	t.Fatal("public replay event history contains no RUN_RESPONSE")
}
