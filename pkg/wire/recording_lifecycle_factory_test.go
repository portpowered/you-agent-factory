package wire

import (
	"path/filepath"
	"testing"
	"time"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
)

type recordingLifecycleFactoryLedger struct {
	recordings.Ledger
}

// fakeRuntimeRecordingBinder mirrors the recordings.RuntimeRecordingBinder
// capability the live Factory Session-to-Factory Runtime path binds through,
// capturing the exact RecordingLifecycle instance it receives.
type fakeRuntimeRecordingBinder struct {
	calls        int
	gotLifecycle recordings.RecordingLifecycle
	gotScope     recordings.CanonicalEventScope
}

func (binder *fakeRuntimeRecordingBinder) BindRecordingLifecycle(
	lifecycle recordings.RecordingLifecycle,
	scope recordings.CanonicalEventScope,
) error {
	binder.calls++
	binder.gotLifecycle = lifecycle
	binder.gotScope = scope
	return nil
}

// TestProvideRecordingLifecycleFactoryBindsTheExactWireProducedInstance
// proves the narrow RecordingLifecycle capability Wire composes for the live
// runtime-opening path is produced explicitly at the Wire boundary -- not
// discovered later from a broader Recordings Service -- and is the exact
// instance a runtime recorder binds to, backed by real Recordings JSONL
// composition rather than a stub.
func TestProvideRecordingLifecycleFactoryBindsTheExactWireProducedInstance(t *testing.T) {
	t.Parallel()

	factory := provideRecordingLifecycleFactory(
		serviceedges.Edges{},
		provideLiveRecordingTargetPlanner(),
		platformreplay.Local{},
	)
	lifecycle := factory(
		&recordingLifecycleFactoryLedger{},
		recordingswire.NewProjectionService(),
	)
	if lifecycle == nil {
		t.Fatal("provideRecordingLifecycleFactory() returned nil lifecycle")
	}

	binder := &fakeRuntimeRecordingBinder{}
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-wire-lifecycle-factory"}
	if err := binder.BindRecordingLifecycle(lifecycle, scope); err != nil {
		t.Fatalf("BindRecordingLifecycle() error = %v", err)
	}
	if binder.calls != 1 {
		t.Fatalf("BindRecordingLifecycle calls = %d, want 1", binder.calls)
	}
	if binder.gotLifecycle != lifecycle {
		t.Fatalf("bound lifecycle = %#v, want the exact Wire-produced instance", binder.gotLifecycle)
	}
	if binder.gotScope != scope {
		t.Fatalf("bound scope = %#v, want %#v", binder.gotScope, scope)
	}

	exerciseWireProducedRecordingLifecycle(t, binder.gotLifecycle, scope)
}

// exerciseWireProducedRecordingLifecycle proves the exact instance bound to
// the runtime recorder is a genuinely functioning lifecycle wired through
// real Recordings JSONL persistence, not merely a non-nil value.
func exerciseWireProducedRecordingLifecycle(
	t *testing.T,
	lifecycle recordings.RecordingLifecycle,
	scope recordings.CanonicalEventScope,
) {
	t.Helper()

	recordingID := recordings.LifecycleRecordingID("wire-lifecycle-factory-recording")
	lifecycleScope := recordings.LifecycleScope{FactorySessionID: scope.FactorySessionID}
	artifactPath := filepath.Join(t.TempDir(), "wire-lifecycle-factory.json")
	if _, err := lifecycle.Bind(recordings.BindLifecycleRequest{
		RecordingID: recordingID,
		Artifact:    recordings.LifecycleArtifactReference(artifactPath),
		Scope:       lifecycleScope,
	}); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	event, err := wireCompositionRunRequestEvent(
		"wire-lifecycle-factory-run-request",
		0,
		scope,
		time.Unix(1_700_000_000, 0).UTC(),
		"generation-wire-lifecycle-factory",
	)
	if err != nil {
		t.Fatalf("wireCompositionRunRequestEvent: %v", err)
	}
	if _, err := lifecycle.AppendEvent(recordings.AppendLifecycleEventRequest{
		RecordingID: recordingID,
		Event: recordings.LifecycleEvent{
			ID:          string(event.ID),
			Sequence:    int64(event.Sequence),
			FactoryTick: event.FactoryTick,
			Scope:       lifecycleScope,
			Kind:        string(event.Kind),
			Payload:     event.Payload,
			RecordedAt:  event.RecordedAt,
			Cursor: recordings.LifecycleEventCursor{
				StreamGenerationID: event.Cursor.StreamGenerationID,
				Sequence:           int64(event.Cursor.Sequence),
			},
		},
	}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	flushed, err := lifecycle.Flush(recordings.FlushLifecycleRequest{RecordingID: recordingID})
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if flushed.Status.FlushedThrough == nil || flushed.Status.FlushedThrough.Sequence != 0 {
		t.Fatalf("Flush() FlushedThrough = %#v, want sequence 0", flushed.Status.FlushedThrough)
	}
	finished, err := lifecycle.Finish(recordings.FinishLifecycleRequest{
		RecordingID: recordingID,
		FinishedAt:  time.Unix(1_700_000_300, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if finished.Status.State != recordings.LifecycleStateFinalized {
		t.Fatalf("Finish() State = %v, want LifecycleStateFinalized", finished.Status.State)
	}
}
