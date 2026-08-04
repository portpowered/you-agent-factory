package runtimeopening

import (
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// bareRuntimeRecorder satisfies recordings.RuntimeRecorder through a nil
// embed only, so it deliberately does not expose
// recordings.RuntimeRecordingBinder.
type bareRuntimeRecorder struct {
	recordings.RuntimeRecorder
}

// fakeLifecycleRecordingsService satisfies recordings.RecordingLifecycle
// through a nil embed; bindRuntimeRecordingLifecycle only needs to observe
// its identity, never call through it.
type fakeLifecycleRecordingsService struct {
	recordings.RecordingLifecycle
}

// fakeBindingRuntimeRecorder tracks BindRecordingLifecycle calls and returns a
// configurable error.
type fakeBindingRuntimeRecorder struct {
	recordings.RuntimeRecorder
	calls        int
	gotLifecycle recordings.RecordingLifecycle
	gotScope     recordings.CanonicalEventScope
	bindErr      error
}

func (recorder *fakeBindingRuntimeRecorder) BindRecordingLifecycle(
	lifecycle recordings.RecordingLifecycle,
	scope recordings.CanonicalEventScope,
) error {
	recorder.calls++
	recorder.gotLifecycle = lifecycle
	recorder.gotScope = scope
	return recorder.bindErr
}

var _ recordings.RuntimeRecordingBinder = (*fakeBindingRuntimeRecorder)(nil)

func TestBindRuntimeRecordingLifecycleNilRecorderIsNoOp(t *testing.T) {
	t.Parallel()

	err := bindRuntimeRecordingLifecycle(
		nil,
		&fakeLifecycleRecordingsService{},
		recordings.CanonicalEventScope{FactorySessionID: "~default"},
	)
	if err != nil {
		t.Fatalf("bindRuntimeRecordingLifecycle() error = %v, want nil for a nil runtime recording", err)
	}
}

func TestBindRuntimeRecordingLifecycleRejectsRecorderWithoutBinder(t *testing.T) {
	t.Parallel()

	err := bindRuntimeRecordingLifecycle(
		&bareRuntimeRecorder{},
		&fakeLifecycleRecordingsService{},
		recordings.CanonicalEventScope{FactorySessionID: "~default"},
	)
	if err == nil || !strings.Contains(err.Error(), "does not support Recordings binding") {
		t.Fatalf("bindRuntimeRecordingLifecycle() error = %v, want binder-support error", err)
	}
}

func TestBindRuntimeRecordingLifecycleSuppliesTheNarrowCapabilityExplicitly(t *testing.T) {
	t.Parallel()

	lifecycleService := &fakeLifecycleRecordingsService{}
	recorder := &fakeBindingRuntimeRecorder{}
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-under-test"}

	if err := bindRuntimeRecordingLifecycle(recorder, lifecycleService, scope); err != nil {
		t.Fatalf("bindRuntimeRecordingLifecycle() error = %v, want nil", err)
	}
	if recorder.calls != 1 {
		t.Fatalf("BindRecordingLifecycle calls = %d, want exactly 1", recorder.calls)
	}
	if recorder.gotLifecycle != recordings.RecordingLifecycle(lifecycleService) {
		t.Fatalf("bound lifecycle = %#v, want the narrowed Service identity", recorder.gotLifecycle)
	}
	if recorder.gotScope != scope {
		t.Fatalf("bound scope = %#v, want %#v", recorder.gotScope, scope)
	}
}

func TestBindRuntimeRecordingLifecyclePropagatesBindFailure(t *testing.T) {
	t.Parallel()

	bindErr := errors.New("bind rejected")
	recorder := &fakeBindingRuntimeRecorder{bindErr: bindErr}

	err := bindRuntimeRecordingLifecycle(
		recorder,
		&fakeLifecycleRecordingsService{},
		recordings.CanonicalEventScope{FactorySessionID: "~default"},
	)
	if !errors.Is(err, bindErr) {
		t.Fatalf("bindRuntimeRecordingLifecycle() error = %v, want to wrap %v", err, bindErr)
	}
	if !strings.Contains(err.Error(), "bind runtime recording") {
		t.Fatalf("bindRuntimeRecordingLifecycle() error = %v, want bind-runtime-recording context", err)
	}
}
