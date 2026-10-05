package internal_test

import (
	"context"
	"errors"
	runtimeopening "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime"
	orchestrationwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/wire"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	factorydefinitionfixtures "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryinternal "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	runtimebuild "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/build"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/definitionmapping"
	factoryruntimeorchestrationowner "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestrationowner"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"go.uber.org/zap"
)

func TestBuild_ConstructsRecordingsRootLedgerAndHostingCapabilities(t *testing.T) {
	sessions := &stubWorkerSessionsService{}
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}

	ledger := &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "runtime-recordings-root"}
	var capturedSource recordings.InitialStructureSource
	recorder := &runtimeRecordingsRecorderStub{}
	runtimeScopes := &testRuntimeScopeServiceStub{
		ledger:         ledger,
		recorder:       recorder,
		capturedSource: &capturedSource,
	}

	bundle, err := testRuntimeFactory().Build(
		context.Background(), dir, dir, "~default", "",
		"", interfaces.RuntimeModeBatch, false, nil, false, nil, nil,
		"", factory.RuntimeLogStorageConfig{},
		factoryinternal.RuntimeFileLoggingPolicyDisabled,
		factoryinternal.RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{},
		loaded, "runtime-recordings-root", "", clockwork.NewFakeClock(),
		"/recordings/session.json", nil, nil, false, nil, nil, nil, nil,
		runtimeScopes,
		testRuntimeWorkers{},
		sessions, sessions,
		nil,
	)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if capturedSource == nil {
		t.Fatal("ledger factory InitialStructureSource = nil")
	}
	if bundle.RecordingLedger() != ledger {
		t.Fatalf("RecordingLedger = %T, want injected root ledger", bundle.RecordingLedger())
	}
	if bundle.Recording != recorder {
		t.Fatalf("Recording = %T, want injected RuntimeRecorder", bundle.Recording)
	}
	if bundle.StreamGeneration() != "runtime-recordings-root" {
		t.Fatalf("stream generation = %q, want runtime-recordings-root", bundle.StreamGeneration())
	}
}

func TestBuild_ConstructsRunnableBundleWithoutRootService(t *testing.T) {
	sessions := &stubWorkerSessionsService{}
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	bundle, err := testRuntimeFactory().Build(
		context.Background(), dir, dir, "~default", "",
		"", interfaces.RuntimeModeBatch, false, nil, false, nil, nil,
		"", factory.RuntimeLogStorageConfig{},
		factoryinternal.RuntimeFileLoggingPolicyDisabled,
		factoryinternal.RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{},
		loaded, "runtime-test", "", clockwork.NewFakeClock(), "", nil, nil, false, nil, nil, nil, nil,
		testRuntimeScopeService(newTestRuntimeLedger),
		testRuntimeWorkers{},
		sessions, sessions,
		nil,
	)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if bundle == nil {
		t.Fatal("bundle = nil")
	}
	if bundle.Factory == nil {
		t.Fatal("bundle.Factory = nil, want runnable factory runtime")
	}
	if bundle.EventHistory == nil {
		t.Fatal("bundle.EventHistory = nil")
	}
	if bundle.Net == nil {
		t.Fatal("bundle.Net = nil")
	}
}

func TestBuild_SeparatesCompatibilitySelectorFromCanonicalRuntimeIdentity(t *testing.T) {
	sessions := &stubWorkerSessionsService{}
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}

	const (
		compatibilitySessionID = "~default"
		canonicalSessionID     = "550e8400-e29b-41d4-a716-446655440000"
	)
	var capturedRequest recordings.RuntimeScopeRequest
	runtimeScopes := &testRuntimeScopeServiceStub{
		ledger:          &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "identity-handoff"},
		capturedRequest: &capturedRequest,
	}

	bundle, err := testRuntimeFactory().Build(
		context.Background(), dir, dir, compatibilitySessionID, canonicalSessionID,
		"", interfaces.RuntimeModeBatch, false, nil, false, nil, nil,
		"", factory.RuntimeLogStorageConfig{},
		factoryinternal.RuntimeFileLoggingPolicyDisabled,
		factoryinternal.RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{},
		loaded, "runtime-identity-handoff", "", clockwork.NewFakeClock(), "", nil, nil, false, nil, nil, nil, nil,
		runtimeScopes,
		testRuntimeWorkers{},
		sessions, sessions,
		nil,
	)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if bundle.FactorySessionID != compatibilitySessionID {
		t.Fatalf("bundle FactorySessionID = %q, want compatibility selector %q", bundle.FactorySessionID, compatibilitySessionID)
	}
	workflowContext, ok := bundle.Factory.(factory.WorkflowContextProvider)
	if !ok {
		t.Fatalf("Factory = %T, want WorkflowContextProvider", bundle.Factory)
	}
	contextValue := workflowContext.WorkflowContext()
	if contextValue == nil {
		t.Fatal("workflow context = nil")
	}
	if got := contextValue.SessionID; got != canonicalSessionID {
		t.Fatalf("workflow context session ID = %q, want canonical identity %q", got, canonicalSessionID)
	}
	if capturedRequest.FactorySessionID != compatibilitySessionID || capturedRequest.CanonicalSessionID != canonicalSessionID {
		t.Fatalf("Recordings scope identities = factory=%q canonical=%q, want factory=%q canonical=%q",
			capturedRequest.FactorySessionID, capturedRequest.CanonicalSessionID,
			compatibilitySessionID, canonicalSessionID,
		)
	}
}

func TestBuild_UsesCompatibilityIdentityWhenCanonicalIdentityIsEmpty(t *testing.T) {
	sessions := &stubWorkerSessionsService{}
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	bundle, err := testRuntimeFactory().Build(
		context.Background(), dir, dir, "~default", "",
		"", interfaces.RuntimeModeBatch, false, nil, false, nil, nil,
		"", factory.RuntimeLogStorageConfig{},
		factoryinternal.RuntimeFileLoggingPolicyDisabled,
		factoryinternal.RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{},
		loaded, "runtime-identity-fallback", "", clockwork.NewFakeClock(), "", nil, nil, false, nil, nil, nil, nil,
		testRuntimeScopeService(newTestRuntimeLedger),
		testRuntimeWorkers{},
		sessions, sessions,
		nil,
	)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	workflowContext, ok := bundle.Factory.(factory.WorkflowContextProvider)
	if !ok {
		t.Fatalf("Factory = %T, want WorkflowContextProvider", bundle.Factory)
	}
	if got := workflowContext.WorkflowContext().SessionID; got != "~default" {
		t.Fatalf("workflow context session ID = %q, want effective compatibility identity %q", got, "~default")
	}
}

func TestBuild_FinalizesRecordingBeforeClosingRuntimeSinksOnPartialFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logDir := t.TempDir()
	metricsDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}

	events := make([]string, 0, 3)
	recorder := &runtimeRecordingsRecorderStub{onFinalize: func() {
		events = append(events, "recording.finalize")
	}}
	runtimeScopes := &testRuntimeScopeServiceStub{
		ledger:   &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "partial-runtime"},
		recorder: recorder,
	}
	_, err = testRuntimeFactoryWithSinkCallbacks(logDir, metricsDir,
		func() { events = append(events, "log.close") },
		func() { events = append(events, "metrics.close") },
	).Build(
		context.Background(), dir, dir, "~default", "",
		"", interfaces.RuntimeModeBatch, false, nil, false, nil, nil,
		logDir, factory.RuntimeLogStorageConfig{},
		"", "", metricsDir, factory.RuntimeMetricsStorageConfig{},
		loaded, "partial-runtime", "", clockwork.NewFakeClock(), "recording.json", nil, nil, false, nil, nil, nil, nil,
		runtimeScopes,
		testRuntimeWorkers{}, &stubWorkerSessionsService{}, nil, nil,
	)
	if err == nil {
		t.Fatal("Build succeeded, want partial-opening failure")
	}
	if got, want := events, []string{"recording.finalize", "log.close", "metrics.close"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("partial-opening cleanup order = %#v, want %#v", got, want)
	}
}

func TestBuild_PreservesOpeningAndCleanupFailures(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"log", "metrics", "recording", "engine"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
			loaded, err := loadedFactoryFixture(dir)
			if err != nil {
				t.Fatal(err)
			}
			openingErr := errors.New("opening failed")
			logErr := errors.New("log remains open")
			metricsErr := errors.New("metrics remains open")
			recordingErr := errors.New("recording remains open")
			var events []string
			logOwner := testRuntimeLogOwner{root: dir, closeErr: logErr, onClose: func() { events = append(events, "log.close") }}
			metricsOwner := testRuntimeMetricsOwner{root: dir, closeErr: metricsErr, onClose: func() { events = append(events, "metrics.close") }}
			scopes := &testRuntimeScopeServiceStub{
				ledger: &recordingfixtures.ScriptedRuntimeLedger{GenerationID: stage},
				recorder: &runtimeRecordingsRecorderStub{finalizeErr: recordingErr, onFinalize: func() {
					events = append(events, "recording.finalize")
				}},
			}
			wantEvents := []string{"recording.finalize", "log.close", "metrics.close"}
			wantErrors := []error{logErr, metricsErr, recordingErr}
			switch stage {
			case "log":
				logOwner.openErr = openingErr
				wantEvents = []string{"log.close"}
				wantErrors = []error{openingErr, logErr}
			case "metrics":
				metricsOwner.openErr = openingErr
				wantEvents = []string{"log.close", "metrics.close"}
				wantErrors = []error{openingErr, logErr, metricsErr}
			case "recording":
				scopes.openErr = openingErr
				wantErrors = append(wantErrors, openingErr)
			}
			bundle, err := testRuntimeFactoryWithOwners(logOwner, metricsOwner).Build(
				context.Background(), dir, dir, "~default", "", "", interfaces.RuntimeModeBatch,
				false, nil, false, nil, nil, dir, factory.RuntimeLogStorageConfig{}, "", "", dir,
				factory.RuntimeMetricsStorageConfig{}, loaded, stage, "", clockwork.NewFakeClock(),
				"recording.json", nil, nil, false, nil, nil, nil, nil, scopes,
				testRuntimeWorkers{}, &stubWorkerSessionsService{}, nil, nil,
			)
			if bundle == nil || bundle.RuntimeService() != nil || err == nil {
				t.Fatalf("Build = (%v, %v), want partial ownership without a live service", bundle, err)
			}
			for _, cause := range wantErrors {
				if !errors.Is(err, cause) {
					t.Errorf("Build error %v lost cause %v", err, cause)
				}
			}
			if !reflect.DeepEqual(events, wantEvents) {
				t.Fatalf("cleanup order = %v, want %v", events, wantEvents)
			}
		})
	}
}

func TestBuild_FailedOpeningRetainsRetryableCleanupWithoutRepeatingReleasedResources(t *testing.T) {
	t.Parallel()
	for _, failed := range []string{"none", "recording", "log", "metrics"} {
		t.Run(failed, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
			loaded, err := loadedFactoryFixture(dir)
			if err != nil {
				t.Fatal(err)
			}
			openingErr, releaseErr := errors.New("recording opening failed"), errors.New("release failed")
			calls := map[string]int{}
			logSink := &testRuntimeLogSink{logger: zap.NewNop(), onClose: func() { calls["log"]++ }}
			metricsSink := &testRuntimeMetricsSink{onClose: func() { calls["metrics"]++ }}
			recorder := &runtimeRecordingsRecorderStub{onFinalize: func() { calls["recording"]++ }}
			switch failed {
			case "recording":
				recorder.finalizeErr = releaseErr
			case "log":
				logSink.closeErr = releaseErr
			case "metrics":
				metricsSink.closeErr = releaseErr
			}
			owner := testRuntimeFactoryWithOwners(
				testRuntimeLogOwnerFunc(func(factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error) { return logSink, nil }),
				testRuntimeMetricsOwnerFunc(func(factory.RuntimeMetricsScopeRequest) (factory.RuntimeMetricsSink, error) { return metricsSink, nil }),
			)
			bundle, err := owner.Build(
				context.Background(), dir, dir, "candidate", "", "", interfaces.RuntimeModeBatch,
				false, nil, false, nil, nil, dir, factory.RuntimeLogStorageConfig{}, "", "", dir,
				factory.RuntimeMetricsStorageConfig{}, loaded, "candidate-runtime", "", clockwork.NewFakeClock(),
				"recording.json", nil, nil, false, nil, nil, nil, nil,
				&testRuntimeScopeServiceStub{recorder: recorder, openErr: openingErr},
				testRuntimeWorkers{}, &stubWorkerSessionsService{}, nil, nil,
			)
			if !errors.Is(err, openingErr) || errors.Is(err, releaseErr) != (failed != "none") {
				t.Fatalf("Build error = %v, lost opening or release cause", err)
			}
			if failed == "none" {
				if bundle != nil {
					t.Fatal("successful unwind retained resources")
				}
				return
			}
			if bundle == nil || bundle.RuntimeService() != nil || bundle.RuntimeInstanceID != "candidate-runtime" {
				t.Fatalf("partial ownership = %#v, want addressed cleanup without a service", bundle)
			}
			if err := errors.Join(bundle.FinalizeRecording(time.Time{}), bundle.CloseArtifacts()); !errors.Is(err, releaseErr) {
				t.Fatalf("failed cleanup retry = %v, want retained release failure", err)
			}
			recorder.finalizeErr, logSink.closeErr, metricsSink.closeErr = nil, nil, nil
			if err := errors.Join(bundle.FinalizeRecording(time.Time{}), bundle.CloseArtifacts(), bundle.CloseArtifacts()); err != nil {
				t.Fatalf("successful cleanup retry: %v", err)
			}
			assertOpeningResourceReleaseCounts(t, calls, failed)
		})
	}
}

func assertOpeningResourceReleaseCounts(t *testing.T, calls map[string]int, failed string) {
	t.Helper()
	for _, resource := range []string{"recording", "log", "metrics"} {
		want := 1
		if resource == failed {
			want = 3
		}
		if calls[resource] != want {
			t.Errorf("%s releases = %d, want %d", resource, calls[resource], want)
		}
	}
}

func TestBuild_AssemblyOpeningFailureRetainsCleanupAtRootAndRetriesSameIdentity(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatal(err)
	}
	clock := clockwork.NewFakeClock()
	openingErr, cleanupErr := errors.New("opening failed"), errors.New("finalization failed")
	finalizations, attempts := 0, 0
	recorder := &runtimeRecordingsRecorderStub{finalizeErr: cleanupErr, onFinalize: func() { finalizations++ }}
	scopes := &testRuntimeScopeServiceStub{recorder: recorder, openErr: openingErr,
		ledger: &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "candidate-runtime"}}
	sessions := &stubWorkerSessionsService{}
	loader := func(string, interfaces.WorkstationLoader) (interfaces.MutableLoadedFactorySource, error) {
		return loaded, nil
	}
	assembly, err := factoryinternal.NewAssembly(testRuntimeFactory(), sessions, sessions, testRuntimeWorkers{},
		platformclock.Real{}, testCleanupAssemblyHost{}, runtimebuild.New(nil, loader, testRuntimeID, zap.NewNop()), nil)
	if err != nil {
		t.Fatal(err)
	}
	root, err := factoryinternal.NewRoot(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := factory.RuntimeActivationRequest{RuntimeID: "candidate-runtime", FactorySessionID: "candidate",
		Snapshot: interfaces.RuntimeSnapshot{FactoryDir: dir, RuntimeBaseDir: dir,
			DefinitionVersion: &interfaces.FactoryVersion{Logical: 1}, EffectiveFactory: *loaded.FactoryConfig()}}
	start := func(ctx context.Context, request factory.RuntimeActivationRequest) (*factory.RuntimeActivation, error) {
		attempts++
		return assembleCleanupTestRuntime(ctx, assembly, dir, loaded, scopes, clock, request)
	}
	result, err := root.Activate(t.Context(), request, start)
	assertOpeningFailureWithoutPublication(t, result, err, openingErr, cleanupErr)
	if _, err := root.Activate(t.Context(), request, start); !errors.Is(err, factory.ErrRuntimeActivationConflict) || attempts != 1 {
		t.Fatalf("pending cleanup activation = %v, attempts %d, want conflict without another opening", err, attempts)
	}
	if _, err := root.Deactivate(t.Context(), factory.RuntimeDeactivationRequest{RuntimeID: request.RuntimeID}); !errors.Is(err, cleanupErr) {
		t.Fatalf("failed explicit cleanup = %v, want retained cause", err)
	}
	if finalizations != 3 {
		t.Fatalf("failed finalizations = %d, want build unwind, Root unwind, and explicit retry", finalizations)
	}
	recorder.finalizeErr, scopes.openErr = nil, nil
	if _, err := root.Deactivate(t.Context(), factory.RuntimeDeactivationRequest{RuntimeID: request.RuntimeID}); err != nil {
		t.Fatalf("successful explicit cleanup: %v", err)
	}
	result, err = root.Activate(t.Context(), request, start)
	if err != nil || result.Binding.IsZero() || result.State != factory.RuntimeLifecycleStateActive || attempts != 2 {
		t.Fatalf("same-identity retry = %#v, %v, attempts %d", result, err, attempts)
	}
	if _, err := root.Deactivate(t.Context(), factory.RuntimeDeactivationRequest{Binding: result.Binding}); err != nil {
		t.Fatalf("close retried generation: %v", err)
	}
}

func assertOpeningFailureWithoutPublication(t *testing.T, result factory.RuntimeActivationResult, err, openingErr, cleanupErr error) {
	t.Helper()
	if !errors.Is(err, openingErr) || !errors.Is(err, cleanupErr) || !result.Binding.IsZero() {
		t.Fatalf("failed activation = %#v, %v, want both causes and no publication", result, err)
	}
}

// This component cell runs the real Runtime assembly/build/Root chain with
// controlled Recordings effects. Sessions publication and public commands are
// proved separately by the functional lane.
func assembleCleanupTestRuntime(
	ctx context.Context, assembly *factoryinternal.Assembly, dir string,
	loaded interfaces.MutableLoadedFactorySource, scopes recordings.RuntimeScopeService,
	clock factory.Clock, request factory.RuntimeActivationRequest,
) (*factory.RuntimeActivation, error) {
	_, record, _, _, _, err := assembly.Assemble(
		ctx, "", "", false, "recording.json", "", request.FactorySessionID, request.FactorySessionID,
		nil, nil, nil, nil, nil, nil, interfaces.RuntimeModeBatch, nil, false, nil, nil,
		"", factory.RuntimeLogStorageConfig{}, factory.RuntimeFileLoggingPolicyDisabled,
		factory.RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{}, 0,
		"", "", false, false, nil, clock, zap.NewNop(), nil, nil, nil, nil, nil, scopes, nil,
		dir, dir, dir, loaded, request.RuntimeID, nil, nil, nil, nil, nil, false,
	)
	if record == nil {
		return nil, err
	}
	return &factory.RuntimeActivation{Service: record.RuntimeService(), Close: func(context.Context) error {
		finalizer := record.(interface{ FinalizeRecording(time.Time) error })
		return errors.Join(finalizer.FinalizeRecording(clock.Now()), record.CloseArtifacts())
	}}, err
}

type testCleanupAssemblyHost struct{ instancehost.Service }

func (host testCleanupAssemblyHost) Scope(factory.Clock) instancehost.Service { return host }

func TestBuild_ProductionObservabilityPoliciesEnableRuntimeSinksByDefault(t *testing.T) {
	sessions := &stubWorkerSessionsService{}
	dir := t.TempDir()
	logDir := t.TempDir()
	metricsDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	bundle, err := testRuntimeFactoryWithSinks(logDir, metricsDir).Build(
		context.Background(), dir, dir, "~default", "",
		"", interfaces.RuntimeModeBatch, false, nil, false, nil, nil,
		logDir, factory.RuntimeLogStorageConfig{},
		"", "", metricsDir, factory.RuntimeMetricsStorageConfig{},
		loaded, "runtime-observability", "", clockwork.NewFakeClock(), "", nil, nil, false, nil, nil, nil, nil,
		testRuntimeScopeService(newTestRuntimeLedger),
		testRuntimeWorkers{},
		sessions, sessions,
		nil,
	)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if bundle == nil {
		t.Fatal("bundle = nil")
	}
	if bundle.LogSink == nil {
		t.Fatal("LogSink = nil, want runtime log sink when production policy is unset")
	}
	if bundle.MetricsSink == nil {
		t.Fatal("MetricsSink = nil, want runtime metrics sink when production policy is unset")
	}
	if bundle.LogSink.Artifact().RootDir != logDir {
		t.Fatalf("LogSink root = %q, want %q", bundle.LogSink.Artifact().RootDir, logDir)
	}
	if bundle.MetricsSink.Artifact().RootDir != metricsDir {
		t.Fatalf("MetricsSink artifact root = %q, want %q", bundle.MetricsSink.Artifact().RootDir, metricsDir)
	}
	if filepath.Base(bundle.LogSink.Artifact().Path) == "" {
		t.Fatal("LogSink.Path() = empty")
	}
	if filepath.Base(bundle.MetricsSink.Path()) == "" {
		t.Fatal("MetricsSink.Path() = empty")
	}

	disabledBundle, err := testRuntimeFactory().Build(
		context.Background(), dir, dir, "~default", "",
		"", interfaces.RuntimeModeBatch, false, nil, false, nil, nil,
		logDir, factory.RuntimeLogStorageConfig{},
		factoryinternal.RuntimeFileLoggingPolicyDisabled,
		factoryinternal.RuntimeMetricsPolicyDisabled,
		metricsDir, factory.RuntimeMetricsStorageConfig{},
		loaded, "runtime-disabled", "", clockwork.NewFakeClock(), "", nil, nil, false, nil, nil, nil, nil,
		testRuntimeScopeService(newTestRuntimeLedger),
		testRuntimeWorkers{},
		sessions, sessions,
		nil,
	)
	if err != nil {
		t.Fatalf("Build disabled policy: %v", err)
	}
	if disabledBundle == nil {
		t.Fatal("disabled policy bundle = nil")
	}
	if disabledBundle.LogSink != nil {
		t.Fatal("LogSink = non-nil, want nil when runtime file logging is explicitly disabled")
	}
	if disabledBundle.MetricsSink != nil {
		t.Fatal("MetricsSink = non-nil, want nil when runtime metrics policy is explicitly disabled")
	}
}

type testRuntimeWorkers struct{ workers.Service }

// stubWorkerSessionsService is a minimal workersessions.Service double for
// build-composition tests: Start hands the request straight to the resolved
// Workers execution boundary, mirroring the real cutover seam's shape
// without pulling in the peer worker_sessions implementation package.
type stubWorkerSessionsService struct {
	factory.WorkerAttemptOpener
	execution workers.Service
}

// This execution-only double has no durable captured activity.
func (*stubWorkerSessionsService) GetCapturedObservation(context.Context, workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error) {
	return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
}

func (*stubWorkerSessionsService) ReadLogs(context.Context, workersessions.ReadLogsRequest) (workersessions.LogPage, error) {
	return workersessions.LogPage{}, workersessions.ErrSessionNotFound
}

func (*stubWorkerSessionsService) ReadLogsArtifact(context.Context, string, string) (io.ReadCloser, error) {
	return nil, workersessions.ErrSessionNotFound
}

func (s *stubWorkerSessionsService) Reserve(context.Context, workersessions.ReserveRequest) (workersessions.Session, error) {
	return workersessions.Session{}, nil
}

func (s *stubWorkerSessionsService) Get(context.Context, workersessions.GetRequest) (workersessions.Session, error) {
	return workersessions.Session{}, nil
}

func (s *stubWorkerSessionsService) List(context.Context, workersessions.ListRequest) (workersessions.ListResult, error) {
	return workersessions.ListResult{}, nil
}

func (s *stubWorkerSessionsService) ListObservations(context.Context, workersessions.ListObservationsRequest) (workersessions.ListObservationsResult, error) {
	return workersessions.ListObservationsResult{}, nil
}

func (s *stubWorkerSessionsService) GetObservation(context.Context, workersessions.GetObservationRequest) (workersessions.Observation, error) {
	return workersessions.Observation{}, nil
}

func (s *stubWorkerSessionsService) GetObservationByWorkerSessionID(context.Context, workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error) {
	return workersessions.Observation{}, nil
}

func (s *stubWorkerSessionsService) ListWorkerSessionObservations(context.Context, workersessions.ListWorkerSessionObservationsRequest) (workersessions.ListWorkerSessionObservationsResult, error) {
	return workersessions.ListWorkerSessionObservationsResult{}, nil
}

func (s *stubWorkerSessionsService) StreamObservations(context.Context, workersessions.StreamObservationsRequest) (workersessions.ObservationSubscription, error) {
	return workersessions.ObservationSubscription{}, nil
}

func (s *stubWorkerSessionsService) StreamObservationsByWorkerSessionID(context.Context, workersessions.StreamObservationsByWorkerSessionIDRequest) (workersessions.ObservationSubscription, error) {
	return workersessions.ObservationSubscription{}, nil
}

func (s *stubWorkerSessionsService) ReadTranscript(context.Context, workersessions.ReadTranscriptRequest) (workersessions.ReadTranscriptResult, error) {
	return workersessions.ReadTranscriptResult{}, nil
}

func (s *stubWorkerSessionsService) ReadTranscriptByWorkerSessionID(context.Context, workersessions.ReadTranscriptByWorkerSessionIDRequest) (workersessions.ReadTranscriptResult, error) {
	return workersessions.ReadTranscriptResult{}, nil
}

func (s *stubWorkerSessionsService) InvokeSession(ctx context.Context, req workersessions.InvokeSessionRequest) (workersessions.InvokeSessionResult, error) {
	return workersessions.InvokeSessionResult{
		Session: workersessions.Session{ID: req.ID, State: workersessions.StateCompleted},
	}, nil
}

func (s *stubWorkerSessionsService) Start(ctx context.Context, req workersessions.StartRequest) (workersessions.StartResult, error) {
	result, err := s.InvokeSession(ctx, workersessions.InvokeSessionRequest{
		ID:        req.ID,
		Execution: req.Execution,
		Retry:     req.Retry,
	})
	return workersessions.StartResult{Session: result.Session}, err
}

func (s *stubWorkerSessionsService) Continue(context.Context, workersessions.ContinueRequest) (workersessions.ContinueResult, error) {
	return workersessions.ContinueResult{}, nil
}

func (s *stubWorkerSessionsService) Interrupt(context.Context, workersessions.InterruptRequest) (workersessions.InterruptResult, error) {
	return workersessions.InterruptResult{}, nil
}

func (s *stubWorkerSessionsService) PublishRecord(context.Context, workersessions.PublishRecordRequest) (workersessions.PublishRecordResult, error) {
	return workersessions.PublishRecordResult{}, nil
}

func (s *stubWorkerSessionsService) AssociateProviderSession(context.Context, workersessions.ProviderSessionAssociationRequest) (workersessions.ProviderSessionAssociationResult, error) {
	return workersessions.ProviderSessionAssociationResult{}, nil
}

func (s *stubWorkerSessionsService) ObserveProviderSession(context.Context, workersessions.ProviderSessionObservationRequest) (workersessions.ProviderSessionAssociationResult, error) {
	return workersessions.ProviderSessionAssociationResult{}, nil
}

func (s *stubWorkerSessionsService) EnsureProviderBinding(context.Context, workersessions.ProviderBindingRequest) (workersessions.ProviderBindingResult, error) {
	return workersessions.ProviderBindingResult{}, nil
}

func (s *stubWorkerSessionsService) WorkerSessionIDForDispatch(_ context.Context, dispatchID string) (string, error) {
	return dispatchID, nil
}

func (s *stubWorkerSessionsService) Pause(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return workersessions.ControlResult{}, nil
}

func (s *stubWorkerSessionsService) Resume(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return workersessions.ControlResult{}, nil
}

func (s *stubWorkerSessionsService) Cancel(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return workersessions.ControlResult{}, nil
}

func (s *stubWorkerSessionsService) Terminate(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return workersessions.ControlResult{}, nil
}

func loadedFactoryFixture(dir string) (interfaces.MutableLoadedFactorySource, error) {
	payload, err := os.ReadFile(filepath.Join(dir, interfaces.FactoryConfigFile))
	if err != nil {
		return nil, err
	}
	config, err := factorymapping.NewFactoryConfigMapper().Expand(payload)
	if err != nil {
		return nil, err
	}
	return factorydefinitionfixtures.NewLoadedSource(dir, config, nil, nil)
}

func testOrchestrationCompilation() factory.OrchestrationCompilation {
	return factoryruntimeorchestrationowner.NewCompilation(orchestrationwire.New(testDefinitionMapper(), nil, nil))
}

func testDefinitionMapper() *definitionmapping.Mapper {
	mapper, err := definitionmapping.New(testRuntimeID)
	if err != nil {
		panic(err) // The fixed test ID generator is required by this fixture.
	}
	return mapper
}

func testRuntimeFactory() *factoryinternal.RuntimeFactory {
	return factoryinternal.NewRuntimeFactory(
		zap.NewNop(), testRuntimeLoggerFactory, nil, nil,
		testRuntimeID, testRuntimeID, localRuntimeFiles{}, localRuntimeFiles{}, filepath.WalkDir,
		testOrchestrationCompilation(),
		platformclock.Real{}, testDefinitionMapper(),
		runtimeopening.NewEngineOpening(nil, nil, nil, nil, outputAsPayloadPolicy(), nil, testRuntimeID, testRuntimeID, localRuntimeFiles{}, nil),
	)
}

func testRuntimeFactoryWithSinks(logDir, metricsDir string) *factoryinternal.RuntimeFactory {
	return testRuntimeFactoryWithSinkCallbacks(logDir, metricsDir, nil, nil)
}

func testRuntimeFactoryWithSinkCallbacks(
	logDir string,
	metricsDir string,
	onLogClose func(),
	onMetricsClose func(),
) *factoryinternal.RuntimeFactory {
	return testRuntimeFactoryWithOwners(
		testRuntimeLogOwner{root: logDir, onClose: onLogClose},
		testRuntimeMetricsOwner{root: metricsDir, onClose: onMetricsClose},
	)
}

func testRuntimeFactoryWithOwners(logOwner factory.RuntimeLogOwner, metricsOwner factory.RuntimeMetricsOwner) *factoryinternal.RuntimeFactory {
	return factoryinternal.NewRuntimeFactory(
		zap.NewNop(), testRuntimeLoggerFactory,
		logOwner, metricsOwner,
		testRuntimeID, testRuntimeID, localRuntimeFiles{}, localRuntimeFiles{}, filepath.WalkDir,
		testOrchestrationCompilation(),
		platformclock.Real{}, testDefinitionMapper(),
		runtimeopening.NewEngineOpening(nil, nil, nil, nil, outputAsPayloadPolicy(), nil, testRuntimeID, testRuntimeID, localRuntimeFiles{}, nil),
	)
}

func outputAsPayloadPolicy() interfaces.WorkPropagationPolicyService {
	return interfaces.WorkPropagationPolicyFunc(func(
		*interfaces.FactoryWorkstationConfig,
	) interfaces.WorkPropagationMode {
		return interfaces.WorkPropagationModeOutputAsPayload
	})
}

func newTestRuntimeLedger(
	recordings.InitialStructureSource,
	func() time.Time,
	interfaces.RuntimeDefinitionLookup,
) recordings.RuntimeEventLedger {
	return &recordingfixtures.ScriptedRuntimeLedger{}
}

func testRuntimeScopeService(
	ledgerFactory func(
		recordings.InitialStructureSource,
		func() time.Time,
		interfaces.RuntimeDefinitionLookup,
	) recordings.RuntimeEventLedger,
) recordings.RuntimeScopeService {
	return &testRuntimeScopeServiceStub{ledgerFactory: ledgerFactory}
}

type testRuntimeScopeServiceStub struct {
	openErr         error
	ledger          recordings.RuntimeEventLedger
	ledgerFactory   func(recordings.InitialStructureSource, func() time.Time, interfaces.RuntimeDefinitionLookup) recordings.RuntimeEventLedger
	recorder        recordings.RuntimeRecorder
	capturedSource  *recordings.InitialStructureSource
	capturedRequest *recordings.RuntimeScopeRequest
}

func (runtimeScopes *testRuntimeScopeServiceStub) OpenRuntime(
	_ context.Context,
	request recordings.RuntimeScopeRequest,
) (recordings.RuntimeScopeResult, error) {
	if runtimeScopes.capturedSource != nil {
		*runtimeScopes.capturedSource = request.Topology
	}
	if runtimeScopes.capturedRequest != nil {
		*runtimeScopes.capturedRequest = request
	}
	ledger := runtimeScopes.ledger
	if runtimeScopes.ledgerFactory != nil {
		ledger = runtimeScopes.ledgerFactory(request.Topology, request.Now, request.Definitions)
	}
	return recordings.RuntimeScopeResult{Ledger: ledger, Recorder: runtimeScopes.recorder}, runtimeScopes.openErr
}

func (*testRuntimeScopeServiceStub) Projection() recordings.ProjectionService { return nil }

func (*testRuntimeScopeServiceStub) ReconstructCanonicalFactoryWorldState(
	[]interfaces.FactoryEvent,
	int,
) (recordings.FactoryWorldState, error) {
	return recordings.FactoryWorldState{}, nil
}

func (*testRuntimeScopeServiceStub) ReplayClock(*recordings.ReplayArtifact) recordings.Clock {
	return nil
}

func (*testRuntimeScopeServiceStub) ReplayExecution(
	*recordings.ReplayArtifact,
) (providers.Service, platformprocess.CommandRunner, []recordings.ReplayHook, recordings.CompletionDeliveryPlanner, error) {
	return nil, nil, nil, nil, nil
}

func (*testRuntimeScopeServiceStub) LoadReplayInput(recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
	return recordings.LoadReplayInputResult{}, nil
}

func (*testRuntimeScopeServiceStub) LoadResumeInput(recordings.LoadResumeInputRequest) (recordings.LoadResumeInputResult, error) {
	return recordings.LoadResumeInputResult{}, nil
}

var _ recordings.RuntimeScopeService = (*testRuntimeScopeServiceStub)(nil)

func testRuntimeLoggerFactory(*zap.Logger, bool) factory.Logger { return factory.NoopLogger{} }

type testRuntimeLogSink struct {
	logger   *zap.Logger
	artifact factory.RuntimeLogArtifact
	onClose  func()
	closeErr error
}

func (sink *testRuntimeLogSink) Logger() *zap.Logger                  { return sink.logger }
func (sink *testRuntimeLogSink) Artifact() factory.RuntimeLogArtifact { return sink.artifact }
func (sink *testRuntimeLogSink) Close() error {
	if sink != nil && sink.onClose != nil {
		sink.onClose()
	}
	return sink.closeErr
}

type testRuntimeLogOwner struct {
	root     string
	onClose  func()
	openErr  error
	closeErr error
}

type testRuntimeLogOwnerFunc func(factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error)

func (owner testRuntimeLogOwnerFunc) Open(request factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error) {
	return owner(request)
}

func (owner testRuntimeLogOwner) Open(request factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error) {
	return &testRuntimeLogSink{logger: zap.NewNop(), onClose: owner.onClose, closeErr: owner.closeErr, artifact: factory.RuntimeLogArtifact{
		Path: filepath.Join(owner.root, request.RuntimeInstanceID+".runtime.log"), RootDir: owner.root,
		StartTimeUTC: time.Now().UTC(), Config: request.Config,
	}}, owner.openErr
}

type testRuntimeMetricsSink struct {
	artifact factory.RuntimeMetricsArtifact
	onClose  func()
	closeErr error
}

func (s *testRuntimeMetricsSink) Counter(context.Context, string, float64, factory.Fields) error {
	return nil
}
func (s *testRuntimeMetricsSink) Gauge(context.Context, string, float64, factory.Fields) error {
	return nil
}
func (s *testRuntimeMetricsSink) Sample(context.Context, string, float64, string, factory.Fields) error {
	return nil
}
func (s *testRuntimeMetricsSink) Close() error {
	if s != nil && s.onClose != nil {
		s.onClose()
	}
	return s.closeErr
}
func (s *testRuntimeMetricsSink) Path() string { return s.artifact.Path }
func (s *testRuntimeMetricsSink) Artifact() factory.RuntimeMetricsArtifact {
	return s.artifact
}

type testRuntimeMetricsOwner struct {
	root     string
	onClose  func()
	openErr  error
	closeErr error
}

type testRuntimeMetricsOwnerFunc func(factory.RuntimeMetricsScopeRequest) (factory.RuntimeMetricsSink, error)

func (owner testRuntimeMetricsOwnerFunc) Open(request factory.RuntimeMetricsScopeRequest) (factory.RuntimeMetricsSink, error) {
	return owner(request)
}

func (owner testRuntimeMetricsOwner) Open(request factory.RuntimeMetricsScopeRequest) (factory.RuntimeMetricsSink, error) {
	return &testRuntimeMetricsSink{onClose: owner.onClose, closeErr: owner.closeErr, artifact: factory.RuntimeMetricsArtifact{
		Path: filepath.Join(owner.root, request.Scope.RuntimeInstanceID+".runtime-metrics.log"), RootDir: owner.root,
		StartTimeUTC: time.Now().UTC(),
	}}, owner.openErr
}

type runtimeRecordingsRecorderStub struct {
	onFinalize  func()
	finalizeErr error
}

func (*runtimeRecordingsRecorderStub) BindRecordingLifecycle(
	recordings.RecordingLifecycle,
	recordings.CanonicalEventScope,
) error {
	return nil
}

func (*runtimeRecordingsRecorderStub) Start(context.Context)               {}
func (*runtimeRecordingsRecorderStub) Stop()                               {}
func (*runtimeRecordingsRecorderStub) RecordEvent(interfaces.FactoryEvent) {}
func (*runtimeRecordingsRecorderStub) RecordError(error)                   {}
func (*runtimeRecordingsRecorderStub) Finish(time.Time)                    {}
func (*runtimeRecordingsRecorderStub) Flush() error                        { return nil }
func (*runtimeRecordingsRecorderStub) Err() error                          { return nil }
func (recorder *runtimeRecordingsRecorderStub) Finalize(time.Time) error {
	if recorder != nil && recorder.onFinalize != nil {
		recorder.onFinalize()
	}
	return recorder.finalizeErr
}

var _ recordings.RuntimeRecorder = (*runtimeRecordingsRecorderStub)(nil)
