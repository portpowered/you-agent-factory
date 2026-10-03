package wire

import (
	"encoding/json"
	"fmt"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"strings"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
)

func TestProvideRecordingsRootConstructsThroughRecordingsWire(t *testing.T) {
	t.Parallel()

	root, err := testRecordingsRoot(
		serviceedges.Edges{},
		recordings.LiveRecordingTargetPlannerFunc(
			func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
				return recordings.LiveRecordingTarget{}, nil
			},
		),
		platformreplay.Local{},
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("provideRecordingsRoot() error = %v", err)
	}
	if root == nil {
		t.Fatal("provideRecordingsRoot() returned nil root")
	}
	var published recordings.Service = root
	if _, err := published.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: "missing-wire-factory-root",
	}); err == nil {
		t.Fatal("LoadReplayRecording() error = nil, want missing recording failure")
	}
}

func TestWireUsesPrecomposedRecordingsRuntimeAndMCPRoles(t *testing.T) {
	t.Parallel()

	root, err := testRecordingsRoot(
		serviceedges.Edges{},
		recordings.LiveRecordingTargetPlannerFunc(
			func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
				return recordings.LiveRecordingTarget{}, nil
			},
		),
		platformreplay.Local{}, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("provideRecordingsRoot() error = %v", err)
	}
	opening, err := provideRecordingsRuntimeScopeService(root)
	if err != nil || opening == nil {
		t.Fatalf("provideRecordingsRuntimeScopeService(root) = %v, %v; want runtime opening", opening, err)
	}
	if _, err := provideRecordingsRuntimeScopeService(nil); err == nil {
		t.Fatal("provideRecordingsRuntimeScopeService(nil) error = nil, want capability validation")
	}

	buildServer := provideMCPServerBuilder(platformfilesystem.Local{}, nil, nil, platformfilesystem.Local{}, func() (string, error) { return t.TempDir(), nil })
	if buildServer == nil {
		t.Fatal("provideMCPServerBuilder() returned nil")
	}
	if server, err := buildServer("", nil, nil, nil, nil); err != nil || server == nil {
		t.Fatalf("buildServer(nil roles) = %v, %v; want inert protocol server", server, err)
	}
	if server, err := buildServer("", root, nil, nil, nil); err != nil || server == nil {
		t.Fatalf("buildServer(recordings root) = %v, %v; want owner-backed protocol server", server, err)
	}

	if _, err := provideHTTPRuntimeBindingWithMetrics(nil, nil, nil, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("provideHTTPRuntimeBindingWithMetrics(nil roles) error = nil, want required-owner validation")
	}
}

func TestHTTPRuntimeBindingRejectsMissingRoot(t *testing.T) {
	t.Parallel()

	_, err := newHTTPRuntimeHandlerWithMetrics(nil, "session-1", nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "Factory Sessions root is required") {
		t.Fatalf("newHTTPRuntimeHandlerWithMetrics() error = %v, want missing root", err)
	}
}

func TestHTTPRuntimeBindingRejectsUnknownSession(t *testing.T) {
	t.Parallel()

	_, err := newHTTPRuntimeHandlerWithMetrics(&factorysessionwire.Root{}, "session-1", nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "process root is required") {
		t.Fatalf("newHTTPRuntimeHandlerWithMetrics() error = %v, want unavailable session", err)
	}
}

func TestDirectJavaScriptHTTPCompositionRejectsMissingRoles(t *testing.T) {
	t.Parallel()

	if _, err := provideDirectJavaScriptHostAdapter(nil, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("provideDirectJavaScriptHostAdapter(nil roles) error = nil, want required-role validation")
	}
	if _, err := newDurableExecutionHTTPHandler(nil, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("newDurableExecutionHTTPHandler(nil roles) error = nil, want required-role validation")
	}
}

func wireCompositionRunRequestEvent(
	id string,
	sequence recordings.CanonicalEventSequence,
	scope recordings.CanonicalEventScope,
	recordedAt time.Time,
	generationID string,
) (recordings.CanonicalEvent, error) {
	snapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"id": "wire-composition-factory",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "ready", "type": "PROCESSING"},
				},
			},
		},
	})
	if err != nil {
		return recordings.CanonicalEvent{}, fmt.Errorf("factory snapshot: %w", err)
	}
	payload, err := json.Marshal(factorydefinitions.RunRequestEventPayload{
		Factory:    snapshot,
		RecordedAt: recordedAt,
	})
	if err != nil {
		return recordings.CanonicalEvent{}, fmt.Errorf("run request payload: %w", err)
	}
	return recordings.CanonicalEvent{
		ID:          recordings.CanonicalEventID(id),
		Kind:        recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeRunRequest),
		Sequence:    sequence,
		Scope:       scope,
		FactoryTick: 0,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: generationID,
			Sequence:           sequence,
		},
		RecordedAt: recordedAt,
		Payload:    string(payload),
	}, nil
}

func testRecordingsRoot(edges serviceedges.Edges, targets recordings.LiveRecordingTargetPlanner, storage platformreplay.Storage, captureSnapshot factorydefinitions.LoadedFactorySnapshotCapturer, replayInputs recordings.ReplayInputLoader, logger logging.Logger) (recordings.Service, error) {
	clock, err := provideRecordingClock(provideFactoryRuntimeClock(edges))
	if err != nil {
		return nil, err
	}
	router := recordingswire.NewRuntimeLedgerRouter(clock)
	ledger := recordingswire.RuntimeLedger(router)
	projection := recordingswire.NewProjectionService()
	publication, err := provideRecordingPublication(edges)
	if err != nil {
		return nil, err
	}
	readFile := provideRecordingReadFile(edges)
	lifecycle := recordingswire.NewRecordingLifecycleOwner(targets, provideRecordingSnapshotWriter(edges, storage, readFile), recordingswire.NewRecordingFlushTickerFactory(), clock)
	if logger == nil {
		logger = logging.NoopLogger{}
	}
	decodeSnapshot := factorydefinitionswire.FactorySnapshotJSONDecoder()
	return provideRecordingsRoot(edges, ledger, projection, lifecycle,
		recordingswire.NewArtifactsExportOwner(lifecycle, publication),
		recordingswire.NewReplayOwner(lifecycle, projection, readFile, decodeSnapshot),
		recordingswire.NewCanonicalLedgerOwner(ledger),
		recordingswire.NewHistoricalQueryOwner(readFile, projection), clock, logger,
		router, captureSnapshot, decodeSnapshot, provideReplayRuntimeConfigDecoder(), replayInputs), nil
}

func TestRecordingClockPreservesSelectedSourceAndRejectsTypedNil(t *testing.T) {
	t.Parallel()
	selected := platformclock.NewDeterministic(time.Unix(123, 456), time.Second)
	clock, err := provideRecordingClock(provideFactoryRuntimeClock(serviceedges.Edges{Clock: selected}))
	if err != nil || clock != selected {
		t.Fatalf("recording clock = %v, %v; want selected process source", clock, err)
	}
	if clock, err := provideRecordingClock(provideFactoryRuntimeClock(serviceedges.Edges{})); err != nil || clock == nil {
		t.Fatalf("default recording clock = %v, %v; want explicit process source", clock, err)
	}
	var absent *nilRecordingClock
	for _, source := range []recordings.RecordingClock{nil, absent} {
		if clock, err := provideRecordingClock(source); err == nil || clock != nil {
			t.Fatalf("absent clock = %v, %v; want construction failure", clock, err)
		}
	}
}

type nilRecordingClock struct{}

func (*nilRecordingClock) Now() time.Time { panic("absent recording clock activated") }
