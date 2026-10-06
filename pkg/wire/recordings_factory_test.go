package wire

import (
	"context"
	"strings"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestProvideRecordingsRootConstructsThroughRecordingsWire(t *testing.T) {
	t.Parallel()
	var observed recordings.Service
	var snapshotReader recordingswire.WorkSnapshotReader
	root := testRecordingsRoot(serviceedges.Edges{
		RecordingsRootObserver: func(service recordings.Service) { observed = service },
		RecordingsWorkSnapshotReaderObserver: func(reader interface {
			ReadWorkSnapshot(context.Context, string) (work.ReadSnapshot, error)
		}) {
			snapshotReader = reader
		},
	}, inertArtifactsOwner{}, inertReplayOwner{})
	if root == nil || observed != root || snapshotReader == nil {
		t.Fatal("provideRecordingsRoot did not publish the inert completed root and work reader")
	}
}

func TestWireUsesPrecomposedRecordingsRuntimeAndMCPRoles(t *testing.T) {
	t.Parallel()

	root := testRecordingsRoot(serviceedges.Edges{}, inertArtifactsOwner{}, inertReplayOwner{})
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
	if server, err := buildServer("", "http://selected-host:7437", nil, nil, nil, nil); err != nil || server == nil {
		t.Fatalf("buildServer(nil roles) = %v, %v; want inert protocol server", server, err)
	}
	if server, err := buildServer("", "http://selected-host:7437", root, nil, nil, nil); err != nil || server == nil {
		t.Fatalf("buildServer(recordings root) = %v, %v; want owner-backed protocol server", server, err)
	}

	if _, err := provideHTTPRuntimeBindingWithMetrics(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("provideHTTPRuntimeBindingWithMetrics(nil roles) error = nil, want required-owner validation")
	}
}

func TestHTTPRuntimeBindingRejectsMissingRoot(t *testing.T) {
	t.Parallel()

	_, err := newHTTPRuntimeHandlerWithMetrics(nil, "session-1", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "Factory Sessions root is required") {
		t.Fatalf("newHTTPRuntimeHandlerWithMetrics() error = %v, want missing root", err)
	}
}

func TestHTTPRuntimeBindingRejectsUnknownSession(t *testing.T) {
	t.Parallel()

	_, err := newHTTPRuntimeHandlerWithMetrics(&factorysessionwire.Root{}, "session-1", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "process root is required") {
		t.Fatalf("newHTTPRuntimeHandlerWithMetrics() error = %v, want unavailable session", err)
	}
}

func TestDirectJavaScriptHTTPCompositionRejectsMissingRoles(t *testing.T) {
	t.Parallel()

	if _, err := provideDirectJavaScriptHostAdapter(nil, nil, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("provideDirectJavaScriptHostAdapter(nil roles) error = nil, want required-role validation")
	}
	if _, err := newDurableExecutionHTTPHandler(nil, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("newDurableExecutionHTTPHandler(nil roles) error = nil, want required-role validation")
	}
}

// Unconfigured operations panic. These collaborators are inert completed
// capabilities, not another graph assembled by the provider tests.
type inertLedger struct{ recordings.Ledger }
type inertProjection struct{ recordings.ProjectionService }
type inertLifecycleOwner struct {
	recordingswire.RecordingLifecycleOwner
}
type inertCanonicalOwner struct {
	recordingswire.CanonicalLedgerOwner
}
type inertHistoricalOwner struct {
	recordingswire.HistoricalQueryOwner
}
type inertArtifactsOwner struct {
	recordingswire.ArtifactsExportOwner
}
type inertReplayOwner struct{ recordingswire.ReplayOwner }

func testRecordingsRoot(edges serviceedges.Edges, artifacts recordingswire.ArtifactsExportOwner, replay recordingswire.ReplayOwner) recordings.Service {
	return provideRecordingsRoot(edges, inertLedger{}, inertProjection{}, inertLifecycleOwner{},
		artifacts, replay, inertCanonicalOwner{}, inertHistoricalOwner{}, platformclock.Real{}, logging.NoopLogger{},
		nil, nil, nil, nil, nil)
}

func TestRecordingClockPreservesSelectedSourceAndRejectsTypedNil(t *testing.T) {
	t.Parallel()
	selected := platformclock.NewDeterministic(time.Unix(123, 456), time.Second)
	clock, err := provideRecordingClock(selected)
	if err != nil || clock != selected {
		t.Fatalf("recording clock = %v, %v; want selected process source", clock, err)
	}
	if clock, err := provideRecordingClock(selectedTestTimeEdges(serviceedges.Edges{}).Clock); err != nil || clock == nil {
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
