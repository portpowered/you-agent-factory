package recordingmcp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	mcprecording "github.com/portpowered/infinite-you/pkg/services/recordings/transports/mcp"
)

func TestBind_ReadPortableArtifactSuccessReturnsOutcomeFromInjectedRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeRecordingsRoot{
		invoked: &invoked,
		readPortableArtifact: func(
			ctx context.Context,
			request recordings.ReadPortableArtifactRequest,
		) (recordings.ReadPortableArtifactResult, error) {
			if request.RecordingID != testReplayRecordingID {
				t.Fatalf("recordingId = %q, want %q", request.RecordingID, testReplayRecordingID)
			}
			if request.Reference != testArtifactReference {
				t.Fatalf("reference = %q, want %q", request.Reference, testArtifactReference)
			}
			if ctx == nil {
				t.Fatal("context is required")
			}
			return recordings.ReadPortableArtifactResult{
				Artifact: recordings.PortableArtifact{
					SchemaVersion: recordings.PortableArtifactSchemaV1,
					Summary: recordings.PortableArtifactSummary{
						RecordingID: testReplayRecordingID,
						Reference:   testArtifactReference,
						State:       recordings.RecordingFinalized,
						EventCount:  2,
						Available:   true,
					},
					Integrity: recordings.PortableArtifactIntegrity{
						Algorithm: recordings.PortableArtifactIntegritySHA256,
						Digest:    "abc123",
					},
				},
			}, nil
		},
	}
	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: fake})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolReadPortableArtifact,
		testReadPortableArtifactInputJSON(),
	)
	if err != nil {
		t.Fatalf("CallTool(read_portable_artifact) transport error = %v, want typed tool response", err)
	}
	if !invoked {
		t.Fatal("fake recordings root was not invoked")
	}
	var response mcprecording.ToolResponse[recordings.ReadPortableArtifactResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("tool response = %s, want success envelope", raw)
	}
	if response.Result.Artifact.SchemaVersion != recordings.PortableArtifactSchemaV1 {
		t.Fatalf("schemaVersion = %q, want %q", response.Result.Artifact.SchemaVersion, recordings.PortableArtifactSchemaV1)
	}
	if response.Result.Artifact.Summary.RecordingID != testReplayRecordingID {
		t.Fatalf("summary.recordingId = %q, want %q", response.Result.Artifact.Summary.RecordingID, testReplayRecordingID)
	}
	if response.Result.Artifact.Summary.EventCount != 2 {
		t.Fatalf("summary.eventCount = %d, want 2", response.Result.Artifact.Summary.EventCount)
	}
}

func TestBind_ReadPortableArtifactUnavailableReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeRecordingsRoot{
		readPortableArtifact: func(_ context.Context, _ recordings.ReadPortableArtifactRequest) (recordings.ReadPortableArtifactResult, error) {
			return recordings.ReadPortableArtifactResult{}, recordings.ErrPortableArtifactUnavailable
		},
	}
	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: fake})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolReadPortableArtifact,
		testReadPortableArtifactInputJSON(),
	)
	if err != nil {
		t.Fatalf("CallTool(read_portable_artifact) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"recording.artifact.unavailable",
		false,
		testReplayRecordingID,
	)
	if envelope.Message != "portable recording artifact unavailable" {
		t.Fatalf("error.message = %q, want %q; envelope = %#v", envelope.Message, "portable recording artifact unavailable", envelope)
	}
}

func TestBind_ReadPortableArtifactInvalidReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeRecordingsRoot{
		readPortableArtifact: func(_ context.Context, _ recordings.ReadPortableArtifactRequest) (recordings.ReadPortableArtifactResult, error) {
			return recordings.ReadPortableArtifactResult{}, recordings.ErrInvalidPortableArtifact
		},
	}
	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: fake})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolReadPortableArtifact,
		testReadPortableArtifactInputJSON(),
	)
	if err != nil {
		t.Fatalf("CallTool(read_portable_artifact) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"recording.artifact.invalid",
		false,
		testReplayRecordingID,
	)
	if envelope.Message != "invalid portable recording artifact" {
		t.Fatalf("error.message = %q, want %q; envelope = %#v", envelope.Message, "invalid portable recording artifact", envelope)
	}
}

func TestBind_ReadPortableArtifactForeignReturnsTypedErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeRecordingsRoot{
		readPortableArtifact: func(_ context.Context, _ recordings.ReadPortableArtifactRequest) (recordings.ReadPortableArtifactResult, error) {
			return recordings.ReadPortableArtifactResult{}, recordings.ErrForeignPortableArtifact
		},
	}
	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: fake})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolReadPortableArtifact,
		testReadPortableArtifactInputJSON(),
	)
	if err != nil {
		t.Fatalf("CallTool(read_portable_artifact) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(
		t,
		raw,
		"recording.artifact.foreign",
		false,
		testReplayRecordingID,
	)
	if envelope.Message != "foreign portable recording artifact handle" {
		t.Fatalf("error.message = %q, want %q; envelope = %#v", envelope.Message, "foreign portable recording artifact handle", envelope)
	}
}

func TestBind_ReadPortableArtifactFailuresDistinctFromReplayFailures(t *testing.T) {
	t.Parallel()

	invalidArtifactRaw := mustCallReadPortableArtifact(t, fakeRecordingsRoot{
		readPortableArtifact: func(_ context.Context, _ recordings.ReadPortableArtifactRequest) (recordings.ReadPortableArtifactResult, error) {
			return recordings.ReadPortableArtifactResult{}, recordings.ErrInvalidPortableArtifact
		},
	})
	replayNotFinalizedRaw := mustCallLoadReplay(t, fakeRecordingsRoot{
		loadReplay: func(_ recordings.LoadReplayRecordingRequest) (recordings.LoadReplayRecordingResult, error) {
			return recordings.LoadReplayRecordingResult{}, recordings.ErrReplayRecordingNotFinalized
		},
	})

	artifactEnvelope := assertTypedToolErrorEnvelope(t, invalidArtifactRaw, "recording.artifact.invalid", false, testReplayRecordingID)
	replayEnvelope := assertTypedToolErrorEnvelope(t, replayNotFinalizedRaw, "recording.replay.not_finalized", false, testReplayRecordingID)
	if artifactEnvelope.Code == replayEnvelope.Code {
		t.Fatalf("artifact and replay error codes should differ: %#v vs %#v", artifactEnvelope, replayEnvelope)
	}
}

func TestBind_ReadPortableArtifactInvalidJSONReturnsBadRequestWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := mcprecording.Bind(mcprecording.RootDependencies{
		Recordings: fakeRecordingsRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolReadPortableArtifact,
		json.RawMessage(`{"recordingId":`),
	)
	if err != nil {
		t.Fatalf("CallTool(read_portable_artifact) transport error = %v, want typed tool response", err)
	}
	assertReadPortableArtifactBadRequestToolResponse(t, raw)
	if invoked {
		t.Fatal("fake recordings root was invoked for invalid JSON decode")
	}
}

func assertReadPortableArtifactBadRequestToolResponse(t *testing.T, raw json.RawMessage) {
	t.Helper()
	envelope := assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false, "")
	if !strings.Contains(envelope.Message, "decode read portable artifact input") {
		t.Fatalf("error.message = %q, want decode read portable artifact input context", envelope.Message)
	}
}
