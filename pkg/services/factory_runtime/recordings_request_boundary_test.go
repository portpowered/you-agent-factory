package factory_test

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

// TestRuntimeHostingExposesRecordingsRootCapabilities proves that the Factory
// Runtime hosting boundary exposes Recordings capabilities through the
// published service root contract.
func TestRuntimeHostingExposesRecordingsRootCapabilities(t *testing.T) {
	t.Parallel()

	testRuntimeHostingExposesRecordingsRootCapabilities(t)
}

func testRuntimeHostingExposesRecordingsRootCapabilities(t *testing.T) {
	t.Helper()

	ledger := &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "hosted-recordings-root"}
	recorder := &runtimeRecordingsRecorderStub{}
	bundle := factoryhost.NewBundle(
		"/runtime",
		"/factory",
		"runtime-1",
		"session-1",
		"backend-1",
		time.Date(2026, 7, 28, 14, 0, 0, 0, time.UTC),
		ledger,
		nil,
		nil,
		zap.NewNop(),
		nil,
		nil,
		recorder,
		"/recordings/session.json",
		nil,
	)
	if bundle.RecordingLedger() != ledger {
		t.Fatalf("RecordingLedger = %T, want injected root ledger", bundle.RecordingLedger())
	}
	if bundle.Recording != recorder {
		t.Fatalf("Recording = %T, want injected RuntimeRecorder", bundle.Recording)
	}
	if bundle.StreamGeneration() != "hosted-recordings-root" {
		t.Fatalf("stream generation = %q, want hosted-recordings-root", bundle.StreamGeneration())
	}
}

// runtimeReplayPlannerStub is shared by the root-level Recordings contract
// tests. It deliberately contains no engine or Petri representation.
type runtimeReplayPlannerStub struct{}

func (stub *runtimeReplayPlannerStub) DeliveryTickForDispatch(
	work.WorkDispatch,
) (int, bool, error) {
	return 7, true, nil
}

type runtimeRecordingsRecorderStub struct {
	recordedEvents int
}

func (*runtimeRecordingsRecorderStub) BindRecordingLifecycle(
	recordings.RecordingLifecycle,
	recordings.CanonicalEventScope,
) error {
	return nil
}

func (*runtimeRecordingsRecorderStub) Start(context.Context) {}
func (*runtimeRecordingsRecorderStub) Stop()                 {}

func (stub *runtimeRecordingsRecorderStub) RecordEvent(interfaces.FactoryEvent) {
	stub.recordedEvents++
}

func (*runtimeRecordingsRecorderStub) RecordError(error) {}
func (*runtimeRecordingsRecorderStub) Finish(time.Time)  {}
func (*runtimeRecordingsRecorderStub) Flush() error      { return nil }
func (*runtimeRecordingsRecorderStub) Err() error        { return nil }
func (*runtimeRecordingsRecorderStub) Finalize(time.Time) error {
	return nil
}

var _ recordings.RuntimeRecorder = (*runtimeRecordingsRecorderStub)(nil)
