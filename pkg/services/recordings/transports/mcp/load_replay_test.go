package recordingmcp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	mcprecording "github.com/portpowered/infinite-you/pkg/services/recordings/transports/mcp"
)

const testReplayRecordingID = "recording-mcp-replay-001"
const testArtifactReference = "artifact-ref-mcp-001"

func TestBind_LoadReplaySuccessReturnsFactsFromInjectedRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	wantEventID := recordings.CanonicalEventID("event-replay-001")
	fake := fakeRecordingsRoot{
		invoked: &invoked,
		loadReplay: func(request recordings.LoadReplayRecordingRequest) (recordings.LoadReplayRecordingResult, error) {
			if request.RecordingID != testReplayRecordingID {
				t.Fatalf("recordingId = %q, want %q", request.RecordingID, testReplayRecordingID)
			}
			return recordings.LoadReplayRecordingResult{
				Recording: recordings.ReplayRecordingFacts{
					RecordingID: testReplayRecordingID,
					Scope:       recordings.CanonicalEventScope{FactorySessionID: "session-replay-001"},
					Events: []recordings.CanonicalEvent{
						{ID: wantEventID, Kind: recordings.CanonicalEventKind("WORK_REQUEST")},
					},
				},
			}, nil
		},
	}
	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: fake})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolLoadReplay,
		json.RawMessage(`{"recordingId":"`+testReplayRecordingID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(load_replay) transport error = %v, want typed tool response", err)
	}
	if !invoked {
		t.Fatal("fake recordings root was not invoked")
	}
	var response mcprecording.ToolResponse[recordings.LoadReplayRecordingResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("tool response = %s, want success envelope", raw)
	}
	if response.Result.Recording.RecordingID != testReplayRecordingID {
		t.Fatalf("recordingId = %q, want %q", response.Result.Recording.RecordingID, testReplayRecordingID)
	}
	if len(response.Result.Recording.Events) != 1 || response.Result.Recording.Events[0].ID != wantEventID {
		t.Fatalf("events = %#v, want one event with id %q", response.Result.Recording.Events, wantEventID)
	}
}

func TestBind_LoadReplayNotFoundReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeRecordingsRoot{
		loadReplay: func(_ recordings.LoadReplayRecordingRequest) (recordings.LoadReplayRecordingResult, error) {
			return recordings.LoadReplayRecordingResult{}, recordings.ErrReplayRecordingNotFound
		},
	}
	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: fake})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolLoadReplay,
		json.RawMessage(`{"recordingId":"`+missingRecordingID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(load_replay) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"recording.replay.not_found",
		false,
		missingRecordingID,
	)
	if envelope.Message != "replay recording not found" {
		t.Fatalf("error.message = %q, want %q; envelope = %#v", envelope.Message, "replay recording not found", envelope)
	}
}

func TestBind_LoadReplayNotFinalizedReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeRecordingsRoot{
		loadReplay: func(_ recordings.LoadReplayRecordingRequest) (recordings.LoadReplayRecordingResult, error) {
			return recordings.LoadReplayRecordingResult{}, recordings.ErrReplayRecordingNotFinalized
		},
	}
	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: fake})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolLoadReplay,
		json.RawMessage(`{"recordingId":"`+testReplayRecordingID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(load_replay) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"recording.replay.not_finalized",
		false,
		testReplayRecordingID,
	)
	if envelope.Message != "replay recording is not finalized" {
		t.Fatalf("error.message = %q, want %q; envelope = %#v", envelope.Message, "replay recording is not finalized", envelope)
	}
}

func TestBind_LoadReplayFailuresDistinctFromPortableArtifactFailures(t *testing.T) {
	t.Parallel()

	replayNotFoundRaw := mustCallLoadReplay(t, fakeRecordingsRoot{
		loadReplay: func(_ recordings.LoadReplayRecordingRequest) (recordings.LoadReplayRecordingResult, error) {
			return recordings.LoadReplayRecordingResult{}, recordings.ErrReplayRecordingNotFound
		},
	})
	artifactUnavailableRaw := mustCallReadPortableArtifact(t, fakeRecordingsRoot{
		readPortableArtifact: func(_ context.Context, _ recordings.ReadPortableArtifactRequest) (recordings.ReadPortableArtifactResult, error) {
			return recordings.ReadPortableArtifactResult{}, recordings.ErrPortableArtifactUnavailable
		},
	})

	replayEnvelope := assertTypedToolErrorEnvelope(t, replayNotFoundRaw, "recording.replay.not_found", false, testReplayRecordingID)
	artifactEnvelope := assertTypedToolErrorEnvelope(
		t,
		artifactUnavailableRaw,
		"recording.artifact.unavailable",
		false,
		testReplayRecordingID,
	)
	if replayEnvelope.Code == artifactEnvelope.Code {
		t.Fatalf("replay and artifact error codes should differ: %#v vs %#v", replayEnvelope, artifactEnvelope)
	}
}

func TestBind_LoadReplayInvalidJSONReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := mcprecording.Bind(mcprecording.RootDependencies{
		Recordings: fakeRecordingsRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolLoadReplay,
		json.RawMessage(`{"recordingId":`),
	)
	if err != nil {
		t.Fatalf("CallTool(load_replay) transport error = %v, want typed tool response", err)
	}
	assertLoadReplayBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake recordings root was invoked for invalid JSON decode")
	}
}

func mustCallLoadReplay(t *testing.T, fake fakeRecordingsRoot) json.RawMessage {
	t.Helper()

	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: fake})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolLoadReplay,
		json.RawMessage(`{"recordingId":"`+testReplayRecordingID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(load_replay) transport error = %v", err)
	}
	return raw
}

func mustCallReadPortableArtifact(t *testing.T, fake fakeRecordingsRoot) json.RawMessage {
	t.Helper()

	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: fake})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolReadPortableArtifact,
		testReadPortableArtifactInputJSON(),
	)
	if err != nil {
		t.Fatalf("CallTool(read_portable_artifact) transport error = %v", err)
	}
	return raw
}

func testReadPortableArtifactInputJSON() json.RawMessage {
	return json.RawMessage(`{"recordingId":"` + testReplayRecordingID + `","reference":"` + testArtifactReference + `"}`)
}

func assertLoadReplayBadRequestToolResponse(t *testing.T, raw json.RawMessage) {
	t.Helper()
	envelope := assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false, "")
	if !strings.Contains(envelope.Message, "decode load replay input") {
		t.Fatalf("error.message = %q, want decode load replay input context", envelope.Message)
	}
}
