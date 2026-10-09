package internal_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryinternal "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	runtimebuild "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/build"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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

	bundle, err := testRuntimeFactory(runtimeScopes).Build(
		context.Background(), zap.NewNop(), dir, dir, "~default", "",
		"", interfaces.RuntimeModeBatch, false, nil, false,
		"", factory.RuntimeLogStorageConfig{},
		factoryinternal.RuntimeFileLoggingPolicyDisabled,
		factoryinternal.RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{},
		loaded, "runtime-recordings-root", "", clockwork.NewFakeClock(),
		"/recordings/session.json", nil, nil, false, nil, nil, nil,
		0, nil,
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
	bundle, err := testRuntimeFactory(testRuntimeScopeService(newTestRuntimeLedger)).Build(
		context.Background(), zap.NewNop(), dir, dir, "~default", "",
		"", interfaces.RuntimeModeBatch, false, nil, false,
		"", factory.RuntimeLogStorageConfig{},
		factoryinternal.RuntimeFileLoggingPolicyDisabled,
		factoryinternal.RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{},
		loaded, "runtime-test", "", clockwork.NewFakeClock(), "", nil, nil, false, nil, nil, nil,
		0, nil,
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

	bundle, err := testRuntimeFactory(runtimeScopes).Build(
		context.Background(), zap.NewNop(), dir, dir, compatibilitySessionID, canonicalSessionID,
		"", interfaces.RuntimeModeBatch, false, nil, false,
		"", factory.RuntimeLogStorageConfig{},
		factoryinternal.RuntimeFileLoggingPolicyDisabled,
		factoryinternal.RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{},
		loaded, "runtime-identity-handoff", "", clockwork.NewFakeClock(), "", nil, nil, false, nil, nil, nil,
		0, nil,
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
	bundle, err := testRuntimeFactory(testRuntimeScopeService(newTestRuntimeLedger)).Build(
		context.Background(), zap.NewNop(), dir, dir, "~default", "",
		"", interfaces.RuntimeModeBatch, false, nil, false,
		"", factory.RuntimeLogStorageConfig{},
		factoryinternal.RuntimeFileLoggingPolicyDisabled,
		factoryinternal.RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{},
		loaded, "runtime-identity-fallback", "", clockwork.NewFakeClock(), "", nil, nil, false, nil, nil, nil,
		0, nil,
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
		func() { events = append(events, "metrics.close") }, runtimeScopes).Build(
		context.Background(), zap.NewNop(), dir, dir, "~default", "",
		"", interfaces.RuntimeModeBatch, false, nil, false,
		logDir, factory.RuntimeLogStorageConfig{},
		"", "", metricsDir, factory.RuntimeMetricsStorageConfig{},
		loaded, "partial-runtime", "", clockwork.NewFakeClock(), "recording.json", nil, nil, false, nil, nil, nil,
		0, nil,
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
			bundle, err := testRuntimeFactoryWithOwners(logOwner, metricsOwner, scopes).Build(
				context.Background(), zap.NewNop(), dir, dir, "~default", "", "", interfaces.RuntimeModeBatch,
				false, nil, false, dir, factory.RuntimeLogStorageConfig{}, "", "", dir,
				factory.RuntimeMetricsStorageConfig{}, loaded, stage, "", clockwork.NewFakeClock(),
				"recording.json", nil, nil, false, nil, nil, nil,
				0, nil,
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
				testRuntimeLogOwnerFunc(func(*zap.Logger, factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error) { return logSink, nil }),
				testRuntimeMetricsOwnerFunc(func(factory.RuntimeMetricsScopeRequest) (factory.RuntimeMetricsSink, error) { return metricsSink, nil }),
				&testRuntimeScopeServiceStub{recorder: recorder, openErr: openingErr},
			)
			bundle, err := owner.Build(
				context.Background(), zap.NewNop(), dir, dir, "candidate", "", "", interfaces.RuntimeModeBatch,
				false, nil, false, dir, factory.RuntimeLogStorageConfig{}, "", "", dir,
				factory.RuntimeMetricsStorageConfig{}, loaded, "candidate-runtime", "", clockwork.NewFakeClock(),
				"recording.json", nil, nil, false, nil, nil, nil,
				0, nil,
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

func TestBuild_InitialOpeningFailureRetainsCleanupAtRootAndRetriesSameIdentity(t *testing.T) {
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
	opening, err := factoryinternal.NewBundleOpening(testRuntimeFactory(scopes).Build, platformclock.Real{}, testRuntimeWorkers{}, sessions, sessions, nil, scopes, nil)
	if err != nil {
		t.Fatal(err)
	}
	assembly, err := factoryinternal.NewAssembly(opening.Open, factoryinternal.NewSidecarOpening(nil, platformclock.Real{}), testCleanupAssemblyHost{}, runtimebuild.New(nil, loader, testRuntimeID, zap.NewNop(), nil, nil, nil, nil), scopes, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	root, err := factoryinternal.NewRoot(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := factory.RuntimeActivationRequest{RuntimeID: "candidate-runtime", FactorySessionID: "candidate",
		Snapshot: interfaces.RuntimeSnapshot{FactoryDir: dir, RuntimeBaseDir: dir,
			DefinitionVersion: &interfaces.FactoryVersion{Logical: 1}, EffectiveFactory: *loaded.FactoryConfig()},
		Runtime: factory.RuntimeSelection{Mode: interfaces.RuntimeModeBatch,
			FileLoggingPolicy: factory.RuntimeFileLoggingPolicyDisabled, MetricsPolicy: factory.RuntimeMetricsPolicyDisabled},
		Inputs: factory.RuntimeActivationInputs{Definition: factory.RuntimeActivationDefinitionInputs{Directory: dir, ExecutionBaseDir: dir},
			Recordings: factory.RuntimeActivationRecordingInputs{RecordPath: "recording.json"}},
	}
	initial := factoryinternal.NewInitialActivation(assembly, clock, zap.NewNop(),
		func(*interfaces.RuntimeSnapshot, string) (interfaces.MutableLoadedFactorySource, error) {
			return loaded, nil
		})
	start := func(ctx context.Context, request factory.RuntimeActivationRequest) (*factory.RuntimeActivation, error) {
		attempts++
		return openTestInitialActivation(ctx, initial, request)
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

func openTestInitialActivation(ctx context.Context, initial *factoryinternal.InitialActivation,
	request factory.RuntimeActivationRequest,
) (*factory.RuntimeActivation, error) {
	opening, err := initial.Open(ctx, request, nil)
	if opening == nil {
		return nil, err
	}
	return opening.Activation, err
}

func assertOpeningFailureWithoutPublication(t *testing.T, result factory.RuntimeActivationResult, err, openingErr, cleanupErr error) {
	t.Helper()
	if !errors.Is(err, openingErr) || !errors.Is(err, cleanupErr) || !result.Binding.IsZero() {
		t.Fatalf("failed activation = %#v, %v, want both causes and no publication", result, err)
	}
}

func assembleTestRuntimeRecord(
	ctx context.Context, assembly *factoryinternal.Assembly, dir string,
	loaded interfaces.MutableLoadedFactorySource,
	clock factory.Clock, request factory.RuntimeActivationRequest,
) (factory.RuntimeReplacementBuilder, factory.RuntimeRecord, factory.RuntimeInitialCompletion, error) {
	opening, err := assembleTestInitialOpening(ctx, assembly, dir, loaded, clock, request)
	if opening == nil {
		return nil, nil, factory.RuntimeInitialCompletion{}, err
	}
	return opening.ReplacementBuilder, opening.Record, opening.Completion, err
}

func assembleTestInitialOpening(
	ctx context.Context, assembly *factoryinternal.Assembly, dir string,
	loaded interfaces.MutableLoadedFactorySource,
	clock factory.Clock, request factory.RuntimeActivationRequest,
	observations ...factory.SessionObservations,
) (*factory.RuntimeInitialOpening, error) {
	var observe factory.SessionObservations
	if len(observations) > 0 {
		observe = observations[0]
	}
	request.Inputs.Definition.Directory = dir
	request.Inputs.Definition.ExecutionBaseDir = dir
	request.Inputs.Recordings.RecordPath = "recording.json"
	request.Runtime.Mode = interfaces.RuntimeModeBatch
	request.Runtime.FileLoggingPolicy = factory.RuntimeFileLoggingPolicyDisabled
	request.Runtime.MetricsPolicy = factory.RuntimeMetricsPolicyDisabled
	return assembly.AssembleInitial(ctx, request, loaded, clock, zap.NewNop(), observe)
}

func TestInitialActivationReplacementRetainsSelectionsAndCanRetry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatal(err)
	}
	clock := clockwork.NewFakeClock()
	loadErr := errors.New("replacement load failed")
	failLoad := true
	loader := func(path string, _ interfaces.WorkstationLoader) (interfaces.MutableLoadedFactorySource, error) {
		if failLoad {
			return nil, loadErr
		}
		return loadedFactoryFixture(path)
	}
	sessions := &stubWorkerSessionsService{}
	var recorded recordings.RuntimeScopeRequest
	scopes := &testRuntimeScopeServiceStub{capturedRequest: &recorded,
		ledgerFactory: func(recordings.InitialStructureSource, func() time.Time, interfaces.RuntimeDefinitionLookup) recordings.RuntimeEventLedger {
			return &recordingfixtures.ScriptedRuntimeLedger{GenerationID: recorded.RecordingID}
		}}
	var snapshots []interfaces.LoadedFactorySource
	snapshot := func(source interfaces.LoadedFactorySource) (*interfaces.FactorySnapshot, error) {
		snapshots = append(snapshots, source)
		return nil, nil
	}
	engines := &controlledEngineOpening{}
	runtimeOwner := testRuntimeFactoryWithEffectsAndOpening(testRuntimeLogOwner{}, testRuntimeMetricsOwner{}, nil, nil, nil, scopes, engines)
	opening, err := factoryinternal.NewBundleOpening(runtimeOwner.Build, platformclock.Real{}, testRuntimeWorkers{}, sessions, sessions, nil, scopes, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	assembly, err := factoryinternal.NewAssembly(opening.Open, factoryinternal.NewSidecarOpening(nil, platformclock.Real{}), testCleanupAssemblyHost{}, runtimebuild.New(nil, loader, testRuntimeID, zap.NewNop(), nil, nil, nil, nil), scopes, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	builder, initial, spec, err := assembleTestRuntimeRecord(t.Context(), assembly, dir, loaded, clock,
		factory.RuntimeActivationRequest{FactorySessionID: "candidate", RuntimeID: "initial-runtime"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = initial.CloseArtifacts() })
	if initial.StreamGeneration() != "initial-runtime" || recorded.RecordingID != "initial-runtime" {
		t.Fatalf("initial identity = %q/%q", initial.StreamGeneration(), recorded.RecordingID)
	}
	if replacement, err := builder.BuildReplacement(t.Context(), dir, dir, "candidate", dir); !errors.Is(err, loadErr) || replacement != nil {
		t.Fatalf("failed replacement = %#v, %v, want load failure without resource", replacement, err)
	}
	failLoad = false
	clock.Advance(time.Minute)
	replacement, err := builder.BuildReplacement(t.Context(), dir, dir, "candidate", dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = replacement.CloseArtifacts() })
	assertReplacementSelections(t, initial, replacement, recorded, clock)
	assertReplacementBuildValues(t, initial, replacement, recorded, spec, engines.calls[0], dir)
	if len(snapshots) != 2 || snapshots[0] != initial.LoadedRuntimeConfig() || snapshots[1] != replacement.LoadedRuntimeConfig() {
		t.Fatalf("selected snapshot sources = %#v, want independent initial and replacement candidates", snapshots)
	}
}

func assertReplacementBuildValues(t *testing.T, initial, replacement factory.RuntimeRecord, recorded recordings.RuntimeScopeRequest, spec factory.RuntimeInitialCompletion, engine controlledEngineCall, dir string) {
	t.Helper()
	if recorded.FactorySessionID != "candidate" || recorded.RecordPath != "recording.candidate.json" {
		t.Fatalf("replacement recording selections = %#v", recorded)
	}
	if replacement.LoadedRuntimeConfig().FactoryDir() != dir || replacement.LoadedRuntimeConfig().RuntimeBaseDir() != dir {
		t.Fatal("replacement changed the selected factory or execution directory")
	}
	if initial.StreamGeneration() != "initial-runtime" || spec.SessionID != "candidate" || engine.runtimeID != "initial-runtime" || engine.mutations != nil {
		t.Fatal("replacement mutated the initial recording or completion identity")
	}
}

func assertReplacementSelections(t *testing.T, initial, replacement factory.RuntimeRecord, recorded recordings.RuntimeScopeRequest, clock factory.Clock) {
	t.Helper()
	if !replacement.StartTime().Equal(clock.Now()) || recorded.RecordingID != testRuntimeID() {
		t.Fatalf("replacement time/identity = %v/%q", replacement.StartTime(), recorded.RecordingID)
	}
	if replacement.LoadedRuntimeConfig() == initial.LoadedRuntimeConfig() || replacement.RecordingLedger() == initial.RecordingLedger() {
		t.Fatal("replacement shares candidate or history with the initial generation")
	}
	if initial.StartTime().Equal(replacement.StartTime()) || initial.StreamGeneration() != "initial-runtime" {
		t.Fatal("replacement changed the initial generation's clock or identity")
	}
}

type testCleanupAssemblyHost struct{ instancehost.Service }

func (host testCleanupAssemblyHost) Scope(factory.Clock) instancehost.Service { return host }

func TestBuild_FileLoggingRetainsSelectedLogger(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatal(err)
	}
	core, logs := observer.New(zap.InfoLevel)
	selected := zap.New(core).With(zap.String("invocation_id", "selected-invocation"))
	var received *zap.Logger
	owner := testRuntimeLogOwnerFunc(func(logger *zap.Logger, _ factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error) {
		received = logger
		return &testRuntimeLogSink{logger: logger}, nil
	})
	sessions := &stubWorkerSessionsService{}
	bundle, err := testRuntimeFactoryWithOwners(owner, nil, testRuntimeScopeService(newTestRuntimeLedger)).Build(
		t.Context(), selected, dir, dir, "selected-session", "", "", interfaces.RuntimeModeBatch,
		false, nil, false, dir, factory.RuntimeLogStorageConfig{},
		factory.RuntimeFileLoggingPolicyEnabled, factory.RuntimeMetricsPolicyDisabled,
		"", factory.RuntimeMetricsStorageConfig{}, loaded, "selected-runtime", "", clockwork.NewFakeClock(),
		"", nil, nil, false, nil, nil, nil,
		0, nil,
		testRuntimeWorkers{}, sessions, sessions, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bundle.CloseArtifacts() })
	if received != selected {
		t.Fatal("runtime log owner did not receive the selected scoped logger")
	}
	bundle.Logger.Info("file-logging-selection")
	entries := logs.FilterMessage("file-logging-selection").All()
	if len(entries) != 1 {
		t.Fatalf("selected backend records = %d, want one", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["invocation_id"] != "selected-invocation" || fields["session_id"] != "selected-session" {
		t.Fatalf("file-backed runtime log attribution = %#v", fields)
	}
}

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
	bundle, err := testRuntimeFactoryWithSinks(logDir, metricsDir, testRuntimeScopeService(newTestRuntimeLedger)).Build(
		context.Background(), zap.NewNop(), dir, dir, "~default", "",
		"", interfaces.RuntimeModeBatch, false, nil, false,
		logDir, factory.RuntimeLogStorageConfig{},
		"", "", metricsDir, factory.RuntimeMetricsStorageConfig{},
		loaded, "runtime-observability", "", clockwork.NewFakeClock(), "", nil, nil, false, nil, nil, nil,
		0, nil,
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

	disabledBundle, err := testRuntimeFactory(testRuntimeScopeService(newTestRuntimeLedger)).Build(
		context.Background(), zap.NewNop(), dir, dir, "~default", "",
		"", interfaces.RuntimeModeBatch, false, nil, false,
		logDir, factory.RuntimeLogStorageConfig{},
		factoryinternal.RuntimeFileLoggingPolicyDisabled,
		factoryinternal.RuntimeMetricsPolicyDisabled,
		metricsDir, factory.RuntimeMetricsStorageConfig{},
		loaded, "runtime-disabled", "", clockwork.NewFakeClock(), "", nil, nil, false, nil, nil, nil,
		0, nil,
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
