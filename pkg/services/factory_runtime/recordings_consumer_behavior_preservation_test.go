package factory_test

import (
	"context"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/replayhooks"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

var (
	_ factoryruntime.HostedInstance = (*factoryhost.Bundle)(nil)
	_ factoryruntime.HostedLedger   = (*recordingfixtures.ScriptedRuntimeLedger)(nil)
)

// TestRuntimeRecordingsConsumerBehaviorPreserved proves CUT-RUN-REC story 004:
// hosting, replay, and execution paths that depend on Recordings keep working
// through root contracts and Runtime root aliases without deep Recordings imports.
func TestRuntimeRecordingsConsumerBehaviorPreserved(t *testing.T) {
	t.Parallel()

	testHostedRuntimeExposesRecordingsRootCapabilities(t)
	testHostingStopFinalizesRecordingsRootRecorder(t)
	testReplayExecutionHandoffConsumesRecordingsRootVocabulary(t)
	testExecutionCompletionPlannerAcceptsRecordingsRootAlias(t)
}

func testHostedRuntimeExposesRecordingsRootCapabilities(t *testing.T) {
	t.Helper()

	ledger := &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "preserve-hosted-ledger"}
	recorder := &runtimeRecordingsRecorderStub{}
	bundle := factoryhost.NewBundle(
		"/runtime",
		"/factory",
		"runtime-preserve",
		"backend-preserve",
		time.Date(2026, 7, 28, 15, 0, 0, 0, time.UTC),
		ledger,
		nil,
		nil,
		zap.NewNop(),
		nil,
		nil,
		recorder,
		"/recordings/preserve.json",
		nil,
	)

	var hosted factoryruntime.HostedInstance = bundle
	if hosted.RecordingLedger() != ledger {
		t.Fatalf("RecordingLedger = %T, want root ledger fake", hosted.RecordingLedger())
	}
	if hosted.StreamGeneration() != "preserve-hosted-ledger" {
		t.Fatalf("stream generation = %q, want preserve-hosted-ledger", hosted.StreamGeneration())
	}

	var rootLedger recordings.Ledger = hosted.RecordingLedger()
	if rootLedger.StreamGenerationID() != "preserve-hosted-ledger" {
		t.Fatalf("ledger stream generation = %q, want preserve-hosted-ledger", rootLedger.StreamGenerationID())
	}
}

func testHostingStopFinalizesRecordingsRootRecorder(t *testing.T) {
	t.Helper()

	finishedAt := time.Date(2026, 7, 28, 15, 5, 0, 0, time.UTC)
	recording := &preservedTerminalRecording{}
	handle := &factoryhost.Handle{
		Bundle:  &factoryhost.Bundle{Recording: recording},
		RunDone: make(chan struct{}),
	}
	handle.SetRunResult(nil)

	if err := factoryhost.Stop(handle, clockwork.NewFakeClockAt(finishedAt)); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if recording.finalizeCalls != 1 || !recording.finishedAt.Equal(finishedAt) {
		t.Fatalf(
			"recording finalization = (%d, %s), want one call at %s",
			recording.finalizeCalls,
			recording.finishedAt,
			finishedAt,
		)
	}
}

func testReplayExecutionHandoffConsumesRecordingsRootVocabulary(t *testing.T) {
	t.Helper()

	submissionHook := &preservedReplayHook{name: factoryruntime.ReplaySubmissionHookName}
	workStateHook := &preservedReplayHook{name: factoryruntime.ReplayWorkStateChangeHookName}
	var capturedArtifact *recordings.ReplayArtifact

	var replayFactory recordings.ReplayExecutionFactory = func(
		artifact *recordings.ReplayArtifact,
	) (
		workers.Runner,
		workers.CommandRunner,
		[]recordings.ReplayHook,
		recordings.CompletionDeliveryPlanner,
		error,
	) {
		capturedArtifact = artifact
		return nil, nil, []recordings.ReplayHook{submissionHook, workStateHook}, &runtimeReplayPlannerStub{}, nil
	}

	rootFactory := factoryruntime.ReplayExecutionFactory(replayFactory)
	_, _, hooks, _, err := rootFactory(&recordings.ReplayArtifact{SchemaVersion: "1"})
	if err != nil {
		t.Fatalf("ReplayExecutionFactory: %v", err)
	}
	if capturedArtifact == nil || capturedArtifact.SchemaVersion != "1" {
		t.Fatalf("captured artifact = %#v, want schema version 1", capturedArtifact)
	}

	adapted := replayhooks.Adapt(hooks)
	if len(adapted) != 2 {
		t.Fatalf("adapted hooks = %d, want 2", len(adapted))
	}
	if adapted[0].Name() != factoryruntime.ReplaySubmissionHookName {
		t.Fatalf("submission hook name = %q, want %q", adapted[0].Name(), factoryruntime.ReplaySubmissionHookName)
	}
	if adapted[1].Name() != factoryruntime.ReplayWorkStateChangeHookName {
		t.Fatalf(
			"work-state hook name = %q, want %q",
			adapted[1].Name(),
			factoryruntime.ReplayWorkStateChangeHookName,
		)
	}

	var runtimeHook factoryruntime.ReplayHook = submissionHook
	if runtimeHook.Name() != factoryruntime.ReplaySubmissionHookName {
		t.Fatalf("root ReplayHook alias name = %q, want %q", runtimeHook.Name(), factoryruntime.ReplaySubmissionHookName)
	}
}

func testExecutionCompletionPlannerAcceptsRecordingsRootAlias(t *testing.T) {
	t.Helper()

	planner := &runtimeReplayPlannerStub{}
	var recordingsPlanner recordings.CompletionDeliveryPlanner = planner
	var runtimePlanner factoryruntime.CompletionDeliveryPlanner = recordingsPlanner

	deliveryTick, keepAlive, err := runtimePlanner.DeliveryTickForDispatch(work.WorkDispatch{DispatchID: "dispatch-preserve"})
	if err != nil {
		t.Fatalf("DeliveryTickForDispatch: %v", err)
	}
	if deliveryTick != 7 || !keepAlive {
		t.Fatalf("delivery tick = (%d, %v), want (7, true)", deliveryTick, keepAlive)
	}
}

type preservedTerminalRecording struct {
	finalizeCalls int
	finishedAt    time.Time
}

func (*preservedTerminalRecording) BindRecordingService(
	recordings.Service,
	recordings.CanonicalEventScope,
) error {
	return nil
}
func (*preservedTerminalRecording) Start(context.Context)               {}
func (*preservedTerminalRecording) Stop()                               {}
func (*preservedTerminalRecording) RecordEvent(interfaces.FactoryEvent) {}
func (*preservedTerminalRecording) RecordError(error)                   {}
func (*preservedTerminalRecording) Finish(time.Time)                    {}
func (*preservedTerminalRecording) Flush() error                        { return nil }
func (*preservedTerminalRecording) Err() error                          { return nil }
func (r *preservedTerminalRecording) Finalize(finishedAt time.Time) error {
	r.finalizeCalls++
	r.finishedAt = finishedAt
	return nil
}

var _ recordings.RuntimeRecorder = (*preservedTerminalRecording)(nil)

type preservedReplayHook struct {
	name string
}

func (hook *preservedReplayHook) Name() string { return hook.name }
func (hook *preservedReplayHook) Priority() int {
	return 0
}
func (hook *preservedReplayHook) OnTick(
	context.Context,
	recordings.ReplaySnapshot,
) (recordings.ReplayHookResult, error) {
	return recordings.ReplayHookResult{}, nil
}

var _ recordings.ReplayHook = (*preservedReplayHook)(nil)
