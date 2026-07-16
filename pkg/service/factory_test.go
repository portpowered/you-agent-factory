// pkgmaintcheck:ignore-file-lines consolidated same-package service tests remain on root-only runtime seams until dedicated service test seams are extracted.
// backendsizecheck:ignore-file consolidated same-package service tests remain on root-only runtime seams until dedicated service test seams are extracted.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/pkg/config"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/replay"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	sessioninvocation "github.com/portpowered/infinite-you/pkg/factory/sessions/invocation"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	initcmd "github.com/portpowered/infinite-you/pkg/transports/cli/init"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workerdiagnostics "github.com/portpowered/infinite-you/pkg/workers/diagnostics"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

const servicePortableBundledScriptBody = "Write-Output 'portable script'\n"
const serviceStreamedRecordingTimeout = 5 * time.Second

func TestModelServiceCompatibilityBoundaryFailsExplicitlyWhenUnattached(t *testing.T) {
	t.Parallel()

	var svc *FactoryService
	ctx := context.Background()
	_, listErr := svc.ListModels(ctx)
	_, getErr := svc.GetModel(ctx, "OMNIVOICE_Q4_K_M")
	_, pullErr := svc.PullModel(ctx, "OMNIVOICE_Q4_K_M")
	_, invokeErr := svc.InvokeModel(ctx, "OMNIVOICE_Q4_K_M", factoryapi.ModelInvocationRequest{Operation: "TTS"})
	for operation, err := range map[string]error{
		"list": listErr, "get": getErr, "pull": pullErr, "invoke": invokeErr,
	} {
		if !errors.Is(err, errModelServiceUnavailable) {
			t.Fatalf("%s model error = %v, want unavailable boundary", operation, err)
		}
	}
	if svc.CurrentModelRuntimeConfig() != nil {
		t.Fatal("nil service returned a runtime config")
	}
	if _, err := svc.BuildModelInvocationExecutor(nil, nil, "worker"); err == nil {
		t.Fatal("nil service built a model invocation executor")
	}
	if _, err := ModelServiceDependencies(FactoryServiceShell{}); err == nil {
		t.Fatal("ModelServiceDependencies() accepted a nil service shell")
	}
	empty := &FactoryService{}
	deps, err := ModelServiceDependencies(FactoryServiceShell{Service: empty})
	if err != nil || deps.Clock != nil || deps.RuntimeConfig == nil || deps.ModelInvocationExecutor == nil {
		t.Fatalf("ModelServiceDependencies(empty) = (%+v, %v), want callable required adapters and package-owned optional clock default", deps, err)
	}
	if AttachModelServiceCollaborator(FactoryServiceShell{}, unavailableModelService{}) != nil {
		t.Fatal("AttachModelServiceCollaborator() created a service for a nil shell")
	}
}

func TestExplicitServiceCollaboratorConstructorsAssembleInertGraph(t *testing.T) {
	t.Parallel()

	cfg, err := ConfigWithWorkerApplication(&FactoryServiceConfig{})
	if err != nil {
		t.Fatalf("ConfigWithWorkerApplication() error = %v", err)
	}
	clock := factory.EnsureClock(nil)
	logger := zap.NewNop()
	sessions := NewFactorySessionsRegistry()
	collaborators, err := NewFactoryServiceCollaborators(cfg, clock, logger, sessions)
	if err != nil {
		t.Fatalf("NewFactoryServiceCollaborators() error = %v", err)
	}
	if collaborators.Sessions != sessions || collaborators.LocalModels.Manager == nil ||
		collaborators.RuntimeBuild == nil || collaborators.WorkersScheduler == nil {
		t.Fatalf("constructed collaborators = %+v, want complete inert graph", collaborators)
	}
	runtimeBuildWithoutSessions, err := newRuntimeBuildService(cfg, clock, logger, &collaborators.LocalModels, nil, nil)
	if err != nil {
		t.Fatalf("newRuntimeBuildService() error = %v", err)
	}
	hosted := NewHostedWorkersConfig(cfg, logger, clock)
	if runtimeBuildWithoutSessions == nil || collaborators.Sessions != sessions || hosted.Logger == nil {
		t.Fatalf("explicit collaborators were not retained: collaborators=%+v hosted=%+v", collaborators, hosted)
	}
}

func TestInvocationBootstrapModelFixtureValidatesAndSuppliesProcessBoundary(t *testing.T) {
	t.Parallel()

	assets := localmodels.NewAssetPuller(t.TempDir())
	runtime := localmodels.NewOmniVoiceRuntime(nil)
	if err := ApplyInvocationBootstrapLocalModelTestFixture(nil, "http://127.0.0.1:1", runtime, assets); err == nil {
		t.Fatal("ApplyInvocationBootstrapLocalModelTestFixture(nil) succeeded")
	}
	cfg := &FactoryServiceConfig{}
	if err := ApplyInvocationBootstrapLocalModelTestFixture(cfg, " http://127.0.0.1:43210 ", runtime, assets); err != nil {
		t.Fatalf("ApplyInvocationBootstrapLocalModelTestFixture() error = %v", err)
	}
	if cfg.LocalModelRuntimeOverride != runtime || cfg.ModelAssets != assets || cfg.ModelHostOverride == nil ||
		!cfg.SkipBuiltInRunnerPrerequisiteValidation {
		t.Fatalf("fixture config = %+v, want exact runtime/assets and supervised host", cfg)
	}

	launcher := &invocationBootstrapFakeProcessLauncher{healthEndpoint: "http://127.0.0.1:43210"}
	process, err := launcher.Start(context.Background(), modelhost.ProcessStartSpec{})
	if err != nil || process.HealthEndpoint() != "http://127.0.0.1:43210" {
		t.Fatalf("fake process = (%+v, %v), want configured health endpoint", process, err)
	}
	if err := process.Stop(context.Background()); err != nil {
		t.Fatalf("fake process Stop() error = %v", err)
	}
	if err := process.Wait(); err != nil {
		t.Fatalf("fake process Wait() error = %v", err)
	}
}

func serviceNamedFactoryPayload(t *testing.T, project string) []byte {
	t.Helper()
	return serviceNamedFactoryPayloadWithWorkType(t, project, "task")
}

func serviceNamedFactoryPayloadWithVersion(t *testing.T, project string, version factoryapi.HybridLogicalTimestamp) []byte {
	t.Helper()
	return withServicePayloadVersion(t, serviceNamedFactoryPayload(t, project), version)
}

func serviceNamedFactoryPayloadWithWorkType(t *testing.T, project, workType string) []byte {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"name": project,
		"id":   project,
		"workTypes": []map[string]any{{
			"name": workType,
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":          "worker-a",
			"type":          "MODEL_WORKER",
			"modelProvider": "CODEX",
			"model":         "gpt-5-codex",
			"body":          "You are worker " + project + ".",
		}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": workType, "state": "init"}},
			"outputs":   []map[string]string{{"workType": workType, "state": "complete"}},
			"onFailure": []map[string]string{{"workType": workType, "state": "failed"}},
			"type":      "MODEL_WORKSTATION",
			"body":      "Do the " + project + " work.",
		}},
	})
	if err != nil {
		t.Fatalf("marshal named factory payload: %v", err)
	}
	return payload
}

func serviceNamedFactoryPayloadWithBundledInput(t *testing.T, project string) []byte {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(serviceNamedFactoryPayload(t, project), &payload); err != nil {
		t.Fatalf("unmarshal service factory payload: %v", err)
	}
	payload["supportingFiles"] = map[string]any{
		"bundledFiles": []map[string]any{
			{
				"type":       "INPUT",
				"targetPath": "factory/inputs/task/default/stale.md",
				"content": map[string]any{
					"encoding": string(factoryapi.BundledFileContentEncodingUtf8),
					"inline":   "stale starter\n",
				},
			},
		},
	}
	updated, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal service factory payload with bundled input: %v", err)
	}
	return updated
}

func serviceNamedFactoryContract(t *testing.T, name string) factoryapi.Factory {
	t.Helper()
	return serviceNamedFactoryContractWithWorkType(t, name, "task")
}

func serviceNamedFactoryContractWithBundledFiles(t *testing.T, name string) factoryapi.Factory {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"name": name,
		"id":   name,
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":          "worker-a",
			"type":          "MODEL_WORKER",
			"modelProvider": "CODEX",
			"model":         "gpt-5-codex",
			"body":          "You are worker " + name + ".",
		}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			"type":      "MODEL_WORKSTATION",
			"body":      "Do the " + name + " work.",
		}},
		"supportingFiles": map[string]any{
			"bundledFiles": []map[string]any{
				{
					"type":       "ROOT_HELPER",
					"targetPath": "Makefile",
					"content": map[string]any{
						"encoding": string(factoryapi.BundledFileContentEncodingUtf8),
						"inline":   "test:\n\tgo test ./...\n",
					},
				},
				{
					"type":       "DOC",
					"targetPath": "factory/docs/README.md",
					"content": map[string]any{
						"encoding": string(factoryapi.BundledFileContentEncodingUtf8),
						"inline":   "# Portable factory\n",
					},
				},
				{
					"type":       "SCRIPT",
					"targetPath": "factory/scripts/execute-story.ps1",
					"content": map[string]any{
						"encoding": string(factoryapi.BundledFileContentEncodingUtf8),
						"inline":   servicePortableBundledScriptBody,
					},
				},
				{
					"type":       "INPUT",
					"targetPath": "factory/inputs/task/default/starter.md",
					"content": map[string]any{
						"encoding": string(factoryapi.BundledFileContentEncodingUtf8),
						"inline":   "starter work\n",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal bundled named factory payload: %v", err)
	}

	generated, err := config.GeneratedFactoryFromOpenAPIJSON(payload)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON(%s bundled files): %v", name, err)
	}

	generated.Name = factoryapi.FactoryName(name)
	return generated
}

func serviceNamedFactoryContractWithWorkType(t *testing.T, name, workType string) factoryapi.Factory {
	t.Helper()

	generated, err := config.GeneratedFactoryFromOpenAPIJSON([]byte(`{
		"name":"` + name + `",
		"id":"` + name + `",
		"workTypes":[{"name":"` + workType + `","states":[
			{"name":"init","type":"INITIAL"},
			{"name":"complete","type":"TERMINAL"},
			{"name":"failed","type":"FAILED"}
		]}],
		"workers":[{"name":"worker-a","type":"MODEL_WORKER","modelProvider":"CODEX","model":"gpt-5-codex","body":"You are worker ` + name + `."}],
		"workstations":[{"name":"process","worker":"worker-a","type":"MODEL_WORKSTATION","body":"Do the ` + name + ` work.","inputs":[{"workType":"` + workType + `","state":"init"}],"outputs":[{"workType":"` + workType + `","state":"complete"}],"onFailure": [{"workType":"` + workType + `","state":"failed"}]}]
		}`))
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON(%s): %v", name, err)
	}

	generated.Name = factoryapi.FactoryName(name)
	return generated
}

func withServicePayloadVersion(t *testing.T, payload []byte, version factoryapi.HybridLogicalTimestamp) []byte {
	t.Helper()

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal service factory payload: %v", err)
	}
	decoded["version"] = map[string]any{
		"logical":  version.Logical,
		"physical": version.Physical.UTC().Format(time.RFC3339Nano),
	}
	updated, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("marshal service factory payload with version: %v", err)
	}
	return updated
}

func submitWorkRequestsToService(ctx context.Context, svc *FactoryService, reqs []work.SubmitRequest) error {
	workRequest := requests.WorkRequestFromSubmitRequests(reqs)
	_, err := svc.SubmitWorkRequest(ctx, workRequest)
	return err
}

func writeWorkRequestFile(t *testing.T, path string, req work.SubmitRequest) {
	t.Helper()
	data, err := json.Marshal(requests.WorkRequestFromSubmitRequests([]work.SubmitRequest{req}))
	if err != nil {
		t.Fatalf("marshal work request file: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write work request file: %v", err)
	}
}

func stopServiceModeRun(t *testing.T, cancel context.CancelFunc, errCh <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("service-mode run error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode run to stop")
	}
}

type aggregateSnapshotFactory struct {
	mu                       sync.Mutex
	engineState              *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	engineStateErr           error
	engineStateSnapshotCalls int
	streamGenerationID       string
	factoryEvents            []factoryapi.FactoryEvent
	factoryEventsErr         error
	factoryEventsCalls       int
	pauseErr                 error
	submitFunc               func(context.Context, work.WorkRequest) error
	submitCalls              int
	submissions              []work.WorkRequest
	waitToComplete           chan struct{}
}

func (f *aggregateSnapshotFactory) Run(context.Context) error { return nil }
func (f *aggregateSnapshotFactory) SubmitWorkRequest(ctx context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	normalized, err := requests.NormalizeWorkRequest(request, work.WorkRequestNormalizeOptions{})
	if err != nil {
		return work.WorkRequestSubmitResult{}, err
	}
	result := work.WorkRequestSubmitResult{RequestID: request.RequestID, Accepted: true}
	if len(normalized) > 0 {
		result.TraceID = normalized[0].TraceID
	}
	f.mu.Lock()
	f.submitCalls++
	f.submissions = append(f.submissions, request)
	f.mu.Unlock()
	if f.submitFunc != nil {
		return result, f.submitFunc(ctx, request)
	}
	return result, nil
}
func (f *aggregateSnapshotFactory) submissionSnapshot() (int, []work.WorkRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.submitCalls, append([]work.WorkRequest(nil), f.submissions...)
}
func (f *aggregateSnapshotFactory) SubscribeFactoryEvents(context.Context, *interfaces.FactoryEventReconnectCursor, interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
	streamGenerationID := strings.TrimSpace(f.streamGenerationID)
	if streamGenerationID == "" && f.engineState != nil {
		streamGenerationID = strings.TrimSpace(f.engineState.StreamGenerationID)
	}
	return &interfaces.FactoryEventStream{
		StreamGenerationID: streamGenerationID,
		Events:             make(chan interfaces.FactoryEvent),
	}, nil
}
func (f *aggregateSnapshotFactory) Pause(context.Context) error  { return f.pauseErr }
func (f *aggregateSnapshotFactory) Resume(context.Context) error { return nil }
func (f *aggregateSnapshotFactory) MoveWork(context.Context, string, string, work.WorkStateChangeSource, string) (work.OperatorMoveResult, error) {
	return work.OperatorMoveResult{}, errors.New("MoveWork is not implemented in aggregateSnapshotFactory")
}
func (f *aggregateSnapshotFactory) GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	f.engineStateSnapshotCalls++
	if f.engineStateErr != nil {
		return nil, f.engineStateErr
	}
	return f.engineState, nil
}
func (f *aggregateSnapshotFactory) GetFactoryEvents(context.Context) ([]interfaces.FactoryEvent, error) {
	f.factoryEventsCalls++
	if f.factoryEventsErr != nil {
		return nil, f.factoryEventsErr
	}
	events := make([]interfaces.FactoryEvent, 0, len(f.factoryEvents))
	for _, event := range f.factoryEvents {
		canonical, err := interfaces.NewFactoryEvent(event)
		if err != nil {
			return nil, err
		}
		events = append(events, canonical)
	}
	return events, nil
}
func (f *aggregateSnapshotFactory) WaitToComplete() <-chan struct{} {
	if f.waitToComplete != nil {
		return f.waitToComplete
	}
	return make(chan struct{})
}

type runtimeMetricsObserverFactory struct {
	mu          sync.RWMutex
	engineState *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
}

func (f *runtimeMetricsObserverFactory) Run(context.Context) error { return nil }
func (f *runtimeMetricsObserverFactory) SubmitWorkRequest(context.Context, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	return work.WorkRequestSubmitResult{}, nil
}
func (f *runtimeMetricsObserverFactory) SubscribeFactoryEvents(context.Context, *interfaces.FactoryEventReconnectCursor, interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
	return &interfaces.FactoryEventStream{Events: make(chan interfaces.FactoryEvent)}, nil
}
func (f *runtimeMetricsObserverFactory) Pause(context.Context) error  { return nil }
func (f *runtimeMetricsObserverFactory) Resume(context.Context) error { return nil }
func (f *runtimeMetricsObserverFactory) MoveWork(context.Context, string, string, work.WorkStateChangeSource, string) (work.OperatorMoveResult, error) {
	return work.OperatorMoveResult{}, nil
}
func (f *runtimeMetricsObserverFactory) GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.engineState, nil
}
func (f *runtimeMetricsObserverFactory) GetFactoryEvents(context.Context) ([]interfaces.FactoryEvent, error) {
	return nil, nil
}
func (f *runtimeMetricsObserverFactory) WaitToComplete() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}
func (f *runtimeMetricsObserverFactory) setEngineState(state *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.engineState = state
}

func TestFactoryService_WaitToComplete_ReturnsClosedChannelWithoutRuntime(t *testing.T) {
	svc := &FactoryService{}

	select {
	case <-svc.WaitToComplete():
	default:
		t.Fatal("expected WaitToComplete without runtime to return a closed channel")
	}
}

func TestFactoryService_WaitToComplete_DelegatesToActiveRuntime(t *testing.T) {
	waitCh := make(chan struct{})
	svc := &FactoryService{}
	bindServiceStartupRuntime(svc, &factoryRuntimeBundle{
		Factory: &aggregateSnapshotFactory{
			waitToComplete: waitCh,
		},
	})

	if got := svc.WaitToComplete(); got != waitCh {
		t.Fatalf("WaitToComplete channel = %p, want %p", got, waitCh)
	}
	close(waitCh)
}

func TestFactoryService_ObserveRuntimeMetrics_EmitsFailedLifecycleMetric(t *testing.T) {
	metricsSink, err := platformmetrics.BuildRuntimeMetricsSink(
		"session-failed",
		"runtime-failed",
		"/factory",
		"/factory/current",
		t.TempDir(),
		platformmetrics.RuntimeMetricsConfig{},
	)
	if err != nil {
		t.Fatalf("BuildRuntimeMetricsSink: %v", err)
	}
	defer metricsSink.Close()

	factoryStub := &runtimeMetricsObserverFactory{}
	factoryStub.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
	})

	handle := &liveRuntimeHandle{
		Bundle: &factoryRuntimeBundle{
			Factory:     factoryStub,
			MetricsSink: metricsSink,
			Logger:      zap.NewNop(),
		},
		RunDone: make(chan struct{}),
	}

	observerCtx, cancelObserver := context.WithCancel(context.Background())
	defer cancelObserver()

	done := make(chan struct{})
	go func() {
		factoryservice.ObserveRuntimeMetrics(observerCtx, handle)
		close(done)
	}()

	waitForRuntimeMetricsRecord(t, metricsSink.Path(), time.Second, func(record map[string]any) bool {
		return runtimeMetricNameAndValue(record, runtimeMetricStateActive, 1)
	}, "active runtime state")

	factoryStub.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusFinished,
		FactoryState:  string(interfaces.FactoryStateFailed),
	})
	handle.SetRunResult(fmt.Errorf("run failed"))

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for runtime metrics observer to stop")
	}

	records := waitForRuntimeMetricsRecord(t, metricsSink.Path(), time.Second, func(record map[string]any) bool {
		return runtimeMetricNameAndValue(record, runtimeMetricLifecycleStopped, 1) &&
			metricRecordString(record, "outcome") == "failed"
	}, "failed stop")

	foundFailedState := false
	foundFailedStop := false
	for _, record := range records {
		if runtimeMetricNameAndValue(record, runtimeMetricStateFailed, 1) {
			foundFailedState = true
		}
		if runtimeMetricNameAndValue(record, runtimeMetricLifecycleStopped, 1) &&
			metricRecordString(record, "outcome") == "failed" &&
			strings.Contains(metricRecordString(record, "reason"), "run failed") {
			foundFailedStop = true
		}
	}
	if !foundFailedState {
		t.Fatalf("runtime metrics records missing failed state gauge: %#v", records)
	}
	if !foundFailedStop {
		t.Fatalf("runtime metrics records missing failed lifecycle stop: %#v", records)
	}
}

func startRuntimeMetricsShutdownTestHandle(
	t *testing.T,
	engineState *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) (*FactoryService, *liveRuntimeHandle, *runtimeMetricsObserverFactory, string) {
	t.Helper()

	metricsSink, err := platformmetrics.BuildRuntimeMetricsSink(
		"session-shutdown",
		"runtime-shutdown",
		"/factory",
		"/factory/current",
		t.TempDir(),
		platformmetrics.RuntimeMetricsConfig{},
	)
	if err != nil {
		t.Fatalf("BuildRuntimeMetricsSink: %v", err)
	}

	factoryStub := &runtimeMetricsObserverFactory{}
	factoryStub.setEngineState(engineState)

	runCtx, runCancel := context.WithCancel(context.Background())
	t.Cleanup(runCancel)

	handle := &liveRuntimeHandle{
		Bundle: &factoryRuntimeBundle{
			Factory:     factoryStub,
			MetricsSink: metricsSink,
			Logger:      zap.NewNop(),
		},
		RunDone:   make(chan struct{}),
		RunCancel: runCancel,
	}

	svc := &FactoryService{}
	if err := svc.startLiveRuntimeSidecars(runCtx, handle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars: %v", err)
	}

	return svc, handle, factoryStub, metricsSink.Path()
}

func TestRuntimeStopOutcome_PrefersTerminalResultOverForcedCancel(t *testing.T) {
	finished := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusFinished,
		FactoryState:  string(interfaces.FactoryStateRunning),
	}
	active := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
	}

	outcome, reason := factoryservice.RuntimeStopOutcome(finished, nil, true)
	if outcome != "completed" || reason != "" {
		t.Fatalf("RuntimeStopOutcome(finished, nil, forcedCancel=true) = (%q, %q), want (completed, \"\")", outcome, reason)
	}

	outcome, reason = factoryservice.RuntimeStopOutcome(active, context.Canceled, false)
	if outcome != "canceled" || reason != "" {
		t.Fatalf("RuntimeStopOutcome(active, context.Canceled, false) = (%q, %q), want (canceled, \"\")", outcome, reason)
	}

	outcome, reason = factoryservice.RuntimeStopOutcome(active, nil, true)
	if outcome != "canceled" || reason != "" {
		t.Fatalf("RuntimeStopOutcome(active, nil, forcedCancel=true) = (%q, %q), want (canceled, \"\")", outcome, reason)
	}
}

func TestFactoryService_StopLiveRuntime_EmitsCompletedLifecycleMetricThroughShutdownPath(t *testing.T) {
	svc, handle, _, metricsPath := startRuntimeMetricsShutdownTestHandle(t, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusFinished,
		FactoryState:  string(interfaces.FactoryStateRunning),
	})

	handle.SetRunResult(nil)
	if err := svc.stopLiveRuntime(handle); err != nil {
		t.Fatalf("stopLiveRuntime: %v", err)
	}

	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricNameAndValue(record, runtimeMetricLifecycleStopped, 1) &&
			metricRecordString(record, "outcome") == "completed"
	}, "completed stop through shutdown path")
}

func TestFactoryService_StopLiveRuntime_EmitsCanceledLifecycleMetricThroughShutdownPath(t *testing.T) {
	svc, handle, _, metricsPath := startRuntimeMetricsShutdownTestHandle(t, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
	})

	go func() {
		time.Sleep(20 * time.Millisecond)
		handle.SetRunResult(context.Canceled)
	}()

	if err := svc.stopLiveRuntime(handle); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("stopLiveRuntime: %v", err)
	}

	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricNameAndValue(record, runtimeMetricLifecycleStopped, 1) &&
			metricRecordString(record, "outcome") == "canceled"
	}, "canceled stop through shutdown path")
}

func TestFactoryService_StopLiveRuntime_EmitsCompletedWhenNaturalCompletionRacesCancellation(t *testing.T) {
	svc, handle, factoryStub, metricsPath := startRuntimeMetricsShutdownTestHandle(t, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
	})

	releaseCompletion := make(chan struct{})
	go func() {
		<-releaseCompletion
		factoryStub.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			RuntimeStatus: interfaces.RuntimeStatusFinished,
			FactoryState:  string(interfaces.FactoryStateRunning),
		})
		handle.SetRunResult(nil)
	}()

	stopDone := make(chan error, 1)
	go func() {
		stopDone <- svc.stopLiveRuntime(handle)
	}()

	close(releaseCompletion)
	if err := <-stopDone; err != nil {
		t.Fatalf("stopLiveRuntime: %v", err)
	}

	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricNameAndValue(record, runtimeMetricLifecycleStopped, 1) &&
			metricRecordString(record, "outcome") == "completed"
	}, "completed when natural completion races cancellation")
}

func TestFactoryService_StopLiveRuntime_EmitsFailedLifecycleMetricThroughShutdownPath(t *testing.T) {
	svc, handle, _, metricsPath := startRuntimeMetricsShutdownTestHandle(t, &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusFinished,
		FactoryState:  string(interfaces.FactoryStateFailed),
	})

	handle.SetRunResult(fmt.Errorf("execution failed"))
	if err := svc.stopLiveRuntime(handle); err == nil {
		t.Fatal("stopLiveRuntime error = nil, want execution failure")
	}

	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricNameAndValue(record, runtimeMetricLifecycleStopped, 1) &&
			metricRecordString(record, "outcome") == "failed" &&
			strings.Contains(metricRecordString(record, "reason"), "execution failed")
	}, "failed stop through shutdown path")
}

func TestFactoryService_Pause_RequiresActiveRuntimeAndWrapsPauseErrors(t *testing.T) {
	svc := &FactoryService{}
	if err := svc.Pause(context.Background()); err == nil || !strings.Contains(err.Error(), "runtime is not available") {
		t.Fatalf("Pause without runtime error = %v, want runtime unavailable", err)
	}

	bindServiceStartupRuntime(svc, &factoryRuntimeBundle{Factory: &aggregateSnapshotFactory{pauseErr: fmt.Errorf("pause failed")}})
	if err := svc.Pause(context.Background()); err == nil || !strings.Contains(err.Error(), "pause factory: pause failed") {
		t.Fatalf("Pause wrapped error = %v, want wrapped pause failure", err)
	}

	svc = &FactoryService{}
	bindServiceStartupRuntime(svc, &factoryRuntimeBundle{Factory: &aggregateSnapshotFactory{}})
	if err := svc.Pause(context.Background()); err != nil {
		t.Fatalf("Pause success error = %v", err)
	}
}

func TestFactoryService_CurrentRuntimeBundleAndDirComparisonHelpers(t *testing.T) {
	if bundle := (*FactoryService)(nil).currentRuntimeBundle(); bundle != nil {
		t.Fatalf("nil service currentRuntimeBundle = %#v, want nil", bundle)
	}

	svc := &FactoryService{}
	if bundle := svc.currentRuntimeBundle(); bundle != nil {
		t.Fatalf("empty service currentRuntimeBundle = %#v, want nil", bundle)
	}

	svc.policy = serviceCoordinatorPolicyFromConfig(&FactoryServiceConfig{Dir: "C:/factory"})
	mockFactory := &aggregateSnapshotFactory{}
	runtimeCfg := &config.LoadedFactoryConfig{}
	bindServiceStartupRuntime(svc, &factoryRuntimeBundle{
		Dir:        "C:/factory",
		Factory:    mockFactory,
		RuntimeCfg: runtimeCfg,
	})
	bundle := svc.currentRuntimeBundle()
	if bundle == nil {
		t.Fatal("expected populated currentRuntimeBundle")
	}
	if bundle.Dir != svc.coordinatorPolicy().dir || bundle.Factory != mockFactory || bundle.RuntimeCfg != runtimeCfg {
		t.Fatalf("currentRuntimeBundle = %#v, want startup bundle fields", bundle)
	}

	if factorysessions.SameFactoryDir("", svc.coordinatorPolicy().dir) {
		t.Fatal("SameFactoryDir should reject blank paths")
	}
	if !factorysessions.SameFactoryDir("C:/factory/./named", "C:/factory/named") {
		t.Fatal("SameFactoryDir should normalize equivalent paths")
	}
}

func TestLiveRuntimeHandle_CompletionHelpers(t *testing.T) {
	if !(*liveRuntimeHandle)(nil).Completed() {
		t.Fatal("nil liveRuntimeHandle should report completed")
	}
	if err := (*liveRuntimeHandle)(nil).Wait(); err != nil {
		t.Fatalf("nil liveRuntimeHandle wait error = %v, want nil", err)
	}

	handle := &liveRuntimeHandle{
		RunDone: make(chan struct{}),
	}
	if handle.Completed() {
		t.Fatal("open runDone should report incomplete")
	}
	handle.SetRunResult(fmt.Errorf("run failed"))
	if !handle.Completed() {
		t.Fatal("closed runDone should report completed")
	}
	if err := handle.Wait(); err == nil || err.Error() != "run failed" {
		t.Fatalf("wait error = %v, want run failed", err)
	}
}

type runningSessionServiceOptions struct {
	defaultFactory string
	namedFactories []string
	rootConfig     map[string]any
	runtimeLogDir  string
	recordPath     string
	extraOptions   []factory.FactoryOption
	logger         *zap.Logger
}

type runningSessionService struct {
	rootDir       string
	runtimeLogDir string
	metricsDir    string
	svc           *FactoryService
	runErrCh      chan error
	cancelRun     context.CancelFunc
	factoryDirs   map[string]string
	stopped       bool
}

type sessionWorkExpectation struct {
	session  *liveFactorySession
	workID   string
	traceID  string
	excluded []string
}

func startRunningSessionService(t *testing.T, options runningSessionServiceOptions) *runningSessionService {
	t.Helper()

	rootDir := t.TempDir()
	runtimeLogDir := options.runtimeLogDir
	if runtimeLogDir == "" {
		runtimeLogDir = filepath.Join(rootDir, "runtime-logs")
	}
	runtimeMetricsDir := filepath.Join(rootDir, "runtime-metrics")
	if options.rootConfig != nil {
		writeFactoryJSON(t, rootDir, options.rootConfig)
	}

	factoryDirs := map[string]string{}
	for _, name := range options.namedFactories {
		factoryDirs[name] = writeNamedFactoryFixture(t, rootDir, name)
	}

	if options.defaultFactory != "" {
		if err := config.WriteCurrentFactoryPointer(rootDir, options.defaultFactory); err != nil {
			t.Fatalf("WriteCurrentFactoryPointer(%s): %v", options.defaultFactory, err)
		}
	}

	logger := options.logger
	if logger == nil {
		logger = zap.NewNop()
	}
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:                 rootDir,
		RuntimeMode:         interfaces.RuntimeModeService,
		MockWorkersConfig:   config.NewEmptyMockWorkersConfig(),
		Logger:              logger,
		RuntimeLogDir:       runtimeLogDir,
		RuntimeMetricsDir:   runtimeMetricsDir,
		RecordPath:          options.recordPath,
		ExtraOptions:        options.extraOptions,
		SystemConfigHomeDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- svc.Run(runCtx)
	}()

	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "default runtime")

	harness := &runningSessionService{
		rootDir:       rootDir,
		runtimeLogDir: runtimeLogDir,
		metricsDir:    runtimeMetricsDir,
		svc:           svc,
		runErrCh:      runErrCh,
		cancelRun:     cancelRun,
		factoryDirs:   factoryDirs,
	}
	t.Cleanup(func() {
		harness.stop(t)
		removeRunningSessionServiceRoot(t, rootDir)
	})
	return harness
}

func (h *runningSessionService) stop(t *testing.T) {
	t.Helper()

	if h.stopped {
		return
	}
	h.stopped = true

	if h.svc != nil && h.svc.sessions != nil {
		for _, sessionID := range append([]string(nil), h.svc.sessions.IDs()...) {
			if sessionID == defaultFactorySessionID {
				continue
			}
			if err := h.svc.CloseFactorySession(context.Background(), sessionID); err != nil {
				t.Fatalf("CloseFactorySession(%s): %v", sessionID, err)
			}
		}
	}
	closeSessionServiceRuntimeLogs(t, h.svc)

	h.cancelRun()
	select {
	case err := <-h.runErrCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service shutdown")
	}
	closeSessionServiceRuntimeLogs(t, h.svc)
}

func removeRunningSessionServiceRoot(t *testing.T, rootDir string) {
	t.Helper()
	if strings.TrimSpace(rootDir) == "" {
		return
	}

	var err error
	for attempt := 0; attempt < 20; attempt++ {
		err = os.RemoveAll(rootDir)
		if err == nil || os.IsNotExist(err) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("RemoveAll(%s): %v", rootDir, err)
}

func closeSessionServiceRuntimeLogs(t *testing.T, svc *FactoryService) {
	t.Helper()
	closed := make(map[*logging.RuntimeLogSink]struct{})
	closedMetrics := make(map[*platformmetrics.RuntimeMetricsSink]struct{})
	closeBundle := func(bundle *factoryRuntimeBundle) {
		if bundle == nil {
			return
		}
		if bundle.LogSink != nil {
			if _, seen := closed[bundle.LogSink]; !seen {
				closed[bundle.LogSink] = struct{}{}
				if err := bundle.LogSink.Close(); err != nil {
					t.Fatalf("logSink.Close: %v", err)
				}
			}
		}
		if bundle.MetricsSink != nil {
			if _, seen := closedMetrics[bundle.MetricsSink]; !seen {
				closedMetrics[bundle.MetricsSink] = struct{}{}
				if err := bundle.MetricsSink.Close(); err != nil {
					t.Fatalf("metricsSink.Close: %v", err)
				}
			}
		}
	}
	closeBundle(svc.currentRuntimeBundle())
	if svc.sessions != nil {
		for _, sessionID := range svc.sessions.IDs() {
			session := svc.sessionByID(sessionID)
			handle := liveSessionHandle(session)
			if handle != nil {
				closeBundle(handle.Bundle)
			}
		}
	}
}

func (h *runningSessionService) openFactorySession(t *testing.T, factoryName string) string {
	t.Helper()

	dir, ok := h.factoryDirs[factoryName]
	if !ok {
		t.Fatalf("factory fixture %q is not registered", factoryName)
	}
	sessionID, err := h.svc.openFactorySession(context.Background(), dir)
	if err != nil {
		t.Fatalf("openFactorySession(%s): %v", factoryName, err)
	}
	return sessionID
}

func (h *runningSessionService) requireSession(t *testing.T, sessionID string) *liveFactorySession {
	t.Helper()

	session, err := h.svc.requireSession(sessionID)
	if err != nil {
		t.Fatalf("expected session %q to be registered; got ids %v: %v", sessionID, h.svc.sessions.IDs(), err)
	}
	return session
}

func (h *runningSessionService) waitIdle(t *testing.T, sessionID, label string) {
	t.Helper()
	waitForSessionRuntimeStatus(t, h.svc, sessionID, interfaces.RuntimeStatusIdle, time.Second, label)
}

func assertSessionWorkIsolation(t *testing.T, expectations []sessionWorkExpectation) {
	t.Helper()

	for _, expectation := range expectations {
		submitSessionWork(t, expectation.session, expectation.workID, expectation.traceID)
	}
	for _, expectation := range expectations {
		waitForSessionEventsToContain(t, expectation.session, expectation.workID, time.Second)
		for _, excluded := range expectation.excluded {
			assertSessionEventsDoNotContain(t, expectation.session, excluded)
		}
	}
}

func assertSessionArtifactIsolation(t *testing.T, session *liveFactorySession, wantWork string, forbiddenWork map[string]string) {
	t.Helper()

	if session == nil || liveSessionHandle(session) == nil || liveSessionHandle(session).Bundle == nil {
		t.Fatal("expected live session runtime")
	}

	runtimeBundle := liveSessionHandle(session).Bundle
	artifact, err := replay.Load(runtimeBundle.RecordPath)
	if err != nil {
		t.Fatalf("Load(%s): %v", runtimeBundle.RecordPath, err)
	}
	payload, err := json.Marshal(artifact.Events)
	if err != nil {
		t.Fatalf("Marshal(%s events): %v", runtimeBundle.RecordPath, err)
	}
	if !strings.Contains(string(payload), wantWork) {
		t.Fatalf("artifact %s did not contain session work %q: %s", runtimeBundle.RecordPath, wantWork, string(payload))
	}
	for otherSessionID, otherWork := range forbiddenWork {
		if otherSessionID == session.ID {
			continue
		}
		if strings.Contains(string(payload), otherWork) {
			t.Fatalf("artifact %s leaked work %q from session %s: %s", runtimeBundle.RecordPath, otherWork, otherSessionID, string(payload))
		}
	}
}

func assertSessionRuntimeLogRecord(t *testing.T, session *liveFactorySession) {
	t.Helper()

	if session == nil || liveSessionHandle(session) == nil || liveSessionHandle(session).Bundle == nil {
		t.Fatal("expected live session runtime")
	}

	runtimeBundle := liveSessionHandle(session).Bundle
	logPath := runtimeBundle.LogSink.Path()
	if logPath == "" {
		t.Fatalf("session %s runtime log path is empty", session.ID)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read runtime log %s: %v", logPath, err)
	}
	records := parseRuntimeLogRecords(t, string(data))
	foundSessionRecord := false
	for _, record := range records {
		if record["session_id"] != session.ID {
			t.Fatalf("runtime log %s contained record for session %#v, want only %q in %#v", logPath, record["session_id"], session.ID, record)
		}
		foundSessionRecord = true
		if record["folder_path"] != runtimeBundle.FolderPath {
			t.Fatalf("session %s folder_path = %#v, want %q in %#v", session.ID, record["folder_path"], runtimeBundle.FolderPath, record)
		}
		if record["factory_dir"] != runtimeBundle.Dir {
			t.Fatalf("session %s factory_dir = %#v, want %q in %#v", session.ID, record["factory_dir"], runtimeBundle.Dir, record)
		}
		if record["runtime_instance_id"] == "" {
			t.Fatalf("session %s runtime_instance_id missing in %#v", session.ID, record)
		}
	}
	if !foundSessionRecord {
		t.Fatalf("runtime log %s did not contain any records for session %s:\n%s", logPath, session.ID, string(data))
	}
}

func assertSessionRuntimeLogPathsAreDistinct(t *testing.T, runtimeLogRoot string, sessions ...*liveFactorySession) {
	t.Helper()

	seenPaths := make(map[string]string, len(sessions))
	for _, session := range sessions {
		if session == nil || liveSessionHandle(session) == nil || liveSessionHandle(session).Bundle == nil || liveSessionHandle(session).Bundle.LogSink == nil {
			t.Fatal("expected live session runtime log sink")
		}
		path := liveSessionHandle(session).Bundle.LogSink.Path()
		if path == "" {
			t.Fatalf("session %s runtime log path is empty", session.ID)
		}
		if liveSessionHandle(session).Bundle.LogSink.RootDir() != runtimeLogRoot {
			t.Fatalf("session %s runtime log root = %q, want %q", session.ID, liveSessionHandle(session).Bundle.LogSink.RootDir(), runtimeLogRoot)
		}
		if otherSessionID, ok := seenPaths[path]; ok {
			t.Fatalf("sessions %s and %s shared runtime log path %q", otherSessionID, session.ID, path)
		}
		seenPaths[path] = session.ID
	}
}

func assertSessionRuntimeMetricsPathsAreDistinct(t *testing.T, metricsRoot string, sessions ...*liveFactorySession) {
	t.Helper()

	seenPaths := make(map[string]string, len(sessions))
	for _, session := range sessions {
		if session == nil || liveSessionHandle(session) == nil || liveSessionHandle(session).Bundle == nil || liveSessionHandle(session).Bundle.MetricsSink == nil {
			t.Fatal("expected live session runtime metrics sink")
		}
		path := liveSessionHandle(session).Bundle.MetricsSink.Path()
		if path == "" {
			t.Fatalf("session %s runtime metrics path is empty", session.ID)
		}
		if liveSessionHandle(session).Bundle.MetricsSink.RootDir() != metricsRoot {
			t.Fatalf("session %s runtime metrics root = %q, want %q", session.ID, liveSessionHandle(session).Bundle.MetricsSink.RootDir(), metricsRoot)
		}
		if otherSessionID, ok := seenPaths[path]; ok {
			t.Fatalf("sessions %s and %s shared runtime metrics path %q", otherSessionID, session.ID, path)
		}
		sessionComponent := strings.Trim(strings.TrimSpace(session.ID), "_.-~")
		if sessionComponent == "" {
			sessionComponent = "unknown"
		}
		baseName := filepath.Base(path)
		if session.IsDefault {
			if !strings.Contains(baseName, "-default-") && !strings.Contains(baseName, "-"+sessionComponent+"-") {
				t.Fatalf("session %s runtime metrics path %q does not include default session marker", session.ID, path)
			}
		} else if !strings.Contains(baseName, "-"+sessionComponent+"-") {
			t.Fatalf("session %s runtime metrics path %q does not include session ID", session.ID, path)
		}
		seenPaths[path] = session.ID
	}
}

func assertFactoryWorkType(t *testing.T, workTypes *[]factoryapi.WorkType, want string, label string) {
	t.Helper()

	if workTypes == nil || len(*workTypes) != 1 || (*workTypes)[0].Name != want {
		t.Fatalf("%s = %#v, want %q", label, workTypes, want)
	}
}

func assertSessionTargetMetadata(
	t *testing.T,
	target FactorySessionTarget,
	kind FactorySessionTargetKind,
	name string,
	label string,
	factoryDir string,
	project string,
) {
	t.Helper()

	if target.Ref.Kind != kind || target.Ref.Name != name || target.Label != label || target.FactoryDir != factoryDir || target.Project != project {
		t.Fatalf("session target = %#v, want kind=%q name=%q label=%q dir=%q project=%q", target, kind, name, label, factoryDir, project)
	}
}

func waitForSessionRuntimeStatus(
	t *testing.T,
	svc *FactoryService,
	sessionID string,
	want interfaces.RuntimeStatus,
	wait time.Duration,
	label string,
) {
	t.Helper()

	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		session := svc.sessionByID(sessionID)
		if session != nil && liveSessionHandle(session) != nil && liveSessionHandle(session).Bundle != nil {
			snap, err := liveSessionHandle(session).Bundle.Factory.GetEngineStateSnapshot(context.Background())
			if err == nil && snap.RuntimeStatus == want {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s to reach runtime status %s", label, want)
}

func waitForSessionFactoryState(
	t *testing.T,
	svc *FactoryService,
	sessionID string,
	want interfaces.FactoryState,
	wait time.Duration,
	label string,
) {
	t.Helper()

	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		session := svc.sessionByID(sessionID)
		if session != nil && liveSessionHandle(session) != nil && liveSessionHandle(session).Bundle != nil {
			snap, err := liveSessionHandle(session).Bundle.Factory.GetEngineStateSnapshot(context.Background())
			if err == nil && snap.FactoryState == string(want) {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s to reach factory state %s", label, want)
}

func decodeServiceRuntimeMetricsRecords(t *testing.T, data []byte) ([]map[string]any, bool) {
	t.Helper()
	// Exclusive-create sink reservation leaves an empty file until the first write.

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return nil, false
	}

	records := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode runtime metrics line %q: %v", line, err)
		}
		records = append(records, record)
	}
	if len(records) == 0 {
		return nil, false
	}
	return records, true
}

func readServiceRuntimeMetricsRecords(t *testing.T, path string) []map[string]any {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read runtime metrics %q: %v", path, err)
	}
	records, ok := decodeServiceRuntimeMetricsRecords(t, data)
	if !ok {
		t.Fatalf("runtime metrics %q contained no records", path)
	}
	return records
}

func waitForRuntimeMetricsRecord(
	t *testing.T,
	path string,
	wait time.Duration,
	predicate func(map[string]any) bool,
	label string,
) []map[string]any {
	t.Helper()

	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			t.Fatalf("read runtime metrics %q: %v", path, err)
		}
		records, ok := decodeServiceRuntimeMetricsRecords(t, data)
		if ok {
			for _, record := range records {
				if predicate(record) {
					return records
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	records := readServiceRuntimeMetricsRecords(t, path)
	t.Fatalf("timed out waiting for runtime metrics record %s in %q: %#v", label, path, records)
	return nil
}

func runtimeMetricNameAndValue(record map[string]any, name string, value float64) bool {
	if strings.TrimSpace(metricRecordString(record, "metric_name")) != name {
		return false
	}
	got, ok := record["value"].(float64)
	return ok && got == value
}

func metricRecordString(record map[string]any, key string) string {
	if record == nil {
		return ""
	}
	value, _ := record[key].(string)
	return value
}

func submitSessionWork(t *testing.T, session *liveFactorySession, workID, traceID string) {
	t.Helper()
	submitSessionWorkWithType(t, session, "task", workID, traceID)
}

func submitSessionWorkWithType(t *testing.T, session *liveFactorySession, workType, workID, traceID string) {
	t.Helper()

	if session == nil || liveSessionHandle(session) == nil || liveSessionHandle(session).Bundle == nil {
		t.Fatal("live session runtime is required")
	}
	request := requests.WorkRequestFromSubmitRequests([]work.SubmitRequest{{
		WorkID:     workID,
		Name:       workID,
		WorkTypeID: workType,
		TraceID:    traceID,
		Payload:    []byte(`{"title":"` + workID + `"}`),
	}})
	if _, err := liveSessionHandle(session).Bundle.Factory.SubmitWorkRequest(context.Background(), request); err != nil {
		t.Fatalf("SubmitWorkRequest(%s): %v", workID, err)
	}
}

func submitCompatWork(t *testing.T, svc *FactoryService, workID, traceID string) {
	t.Helper()

	request := requests.WorkRequestFromSubmitRequests([]work.SubmitRequest{{
		WorkID:     workID,
		Name:       workID,
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte(`{"title":"` + workID + `"}`),
	}})
	if _, err := svc.SubmitWorkRequest(context.Background(), request); err != nil {
		t.Fatalf("SubmitWorkRequest(%s): %v", workID, err)
	}
}

func selectCompatibilitySessionForTest(t *testing.T, svc *FactoryService, sessionID string) {
	t.Helper()

	if svc == nil || svc.sessions == nil {
		t.Fatal("service session manager is required")
	}
	if !svc.sessions.Select(sessionID) {
		t.Fatalf("session %q is not registered", sessionID)
	}
}

func assertSessionRemainsLive(t *testing.T, svc *FactoryService, sessionID string, wait time.Duration, label string) {
	t.Helper()

	session := svc.sessionByID(sessionID)
	if session == nil || liveSessionHandle(session) == nil {
		t.Fatalf("%s is not registered", label)
	}
	select {
	case <-liveSessionHandle(session).RunDone:
		t.Fatalf("%s stopped unexpectedly", label)
	case <-time.After(wait):
	}
}

func waitForSessionEventsToContain(t *testing.T, session *liveFactorySession, want string, wait time.Duration) {
	t.Helper()

	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if sessionEventsContain(t, session, want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for session events to contain %q", want)
}

func assertSessionEventsDoNotContain(t *testing.T, session *liveFactorySession, want string) {
	t.Helper()
	if sessionEventsContain(t, session, want) {
		t.Fatalf("session events unexpectedly contained %q", want)
	}
}

func sessionEventsContain(t *testing.T, session *liveFactorySession, want string) bool {
	t.Helper()

	if session == nil || liveSessionHandle(session) == nil || liveSessionHandle(session).Bundle == nil {
		t.Fatal("live session runtime is required")
	}
	events, err := liveSessionHandle(session).Bundle.Factory.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	payload, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("Marshal(events): %v", err)
	}
	return strings.Contains(string(payload), want)
}

func TestBuildFactoryService_LoadsFromFactoryJSON(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")

	// Create the inputs/ directory that the file watcher expects.
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	ctx := context.Background()
	svc, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	bundle := svc.currentRuntimeBundle()
	if bundle == nil {
		t.Fatal("expected startup runtime bundle")
	}
	if bundle.Net == nil {
		t.Fatal("expected non-nil net")
	}
	if _, ok := bundle.Net.WorkTypes["task"]; !ok {
		t.Error("expected 'task' work type in net topology")
	}
	if bundle.Factory == nil {
		t.Fatal("expected non-nil factory")
	}

}

func TestBuildFactoryService_ResolvesCurrentFactoryFromNamedLayoutPointer(t *testing.T) {
	rootDir := t.TempDir()

	alphaPayload := serviceNamedFactoryPayload(t, "alpha")
	if _, err := config.PersistNamedFactory(rootDir, "alpha", alphaPayload); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer: %v", err)
	}

	ctx := context.Background()
	svc, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	wantDir := filepath.Join(rootDir, "alpha")
	if svc.coordinatorPolicy().dir != wantDir {
		t.Fatalf("service dir = %q, want %q", svc.coordinatorPolicy().dir, wantDir)
	}
	runtimeCfg := svc.currentRuntimeConfig()
	if runtimeCfg == nil {
		t.Fatal("expected runtime config")
	}
	if runtimeCfg.FactoryDir() != wantDir {
		t.Fatalf("runtime config dir = %q, want %q", runtimeCfg.FactoryDir(), wantDir)
	}
	if runtimeCfg.FactoryConfig().Project != "alpha" {
		t.Fatalf("project = %q, want alpha", runtimeCfg.FactoryConfig().Project)
	}
}

func TestFactoryService_ActivateNamedFactory_SwapsPersistedFactoryAndUpdatesCurrentPointer(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if _, err := config.PersistNamedFactory(rootDir, "beta", serviceNamedFactoryPayload(t, "beta")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	if err := svc.ActivateNamedFactory(context.Background(), "beta"); err != nil {
		t.Fatalf("ActivateNamedFactory(beta): %v", err)
	}

	wantDir := filepath.Join(rootDir, "beta")
	if svc.coordinatorPolicy().dir != filepath.Join(rootDir, "alpha") {
		t.Fatalf("service dir = %q, want unchanged startup dir %q", svc.coordinatorPolicy().dir, filepath.Join(rootDir, "alpha"))
	}
	runtimeCfg := svc.currentRuntimeConfig()
	if runtimeCfg == nil {
		t.Fatal("expected runtime config after activation")
	}
	if got := runtimeCfg.FactoryConfig().Project; got != "beta" {
		t.Fatalf("active project = %q, want beta", got)
	}
	if got, err := config.ReadCurrentFactoryPointer(rootDir); err != nil {
		t.Fatalf("ReadCurrentFactoryPointer: %v", err)
	} else if got != "beta" {
		t.Fatalf("current factory pointer = %q, want beta", got)
	}
	if got, err := config.ResolveCurrentFactoryDir(rootDir); err != nil {
		t.Fatalf("ResolveCurrentFactoryDir: %v", err)
	} else if got != wantDir {
		t.Fatalf("resolved current dir = %q, want %q", got, wantDir)
	}
}

func TestFactoryService_ActivateNamedFactory_PreservesPortableLayoutWhenSwitchingFactories(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayloadWithPortableLayout(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if _, err := config.PersistNamedFactory(rootDir, "beta", serviceNamedFactoryPayloadWithPortableLayout(t, "beta")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	if err := svc.ActivateNamedFactory(context.Background(), "beta"); err != nil {
		t.Fatalf("ActivateNamedFactory(beta): %v", err)
	}

	current, err := svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory after activation: %v", err)
	}
	assertServicePortableLayoutResponse(t, current.Layout, "workstation:process", "task")
}

func serviceNamedFactoryPayloadWithPortableLayout(t *testing.T, project string) []byte {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(serviceNamedFactoryPayload(t, project), &payload); err != nil {
		t.Fatalf("unmarshal service factory payload: %v", err)
	}
	payload["layout"] = map[string]any{
		"schemaVersion": 1,
		"nodes": []map[string]any{{
			"id":       "workstation:process",
			"position": map[string]any{"x": 128, "y": 256},
			"size":     map[string]any{"width": 320, "height": 180},
			"locked":   true,
		}},
		"edges": []map[string]any{{
			"id":        "workstation-output:workstation:process->work-state:task:complete",
			"waypoints": []map[string]any{{"x": 180, "y": 220}},
		}},
		"groups": []map[string]any{{
			"id":      "group-1",
			"label":   "Main lane",
			"nodeIds": []string{"workstation:process"},
			"bounds":  map[string]any{"x": 100, "y": 200, "width": 400, "height": 240},
		}},
		"viewport":    map[string]any{"x": 40, "y": 60, "zoom": 0.9},
		"preferences": map[string]any{"direction": "RIGHT"},
	}
	updated, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal service factory payload with layout: %v", err)
	}
	return updated
}

func assertServicePortableLayoutResponse(t *testing.T, layout *factoryapi.FactoryLayout, wantNodeID, wantWorkType string) {
	t.Helper()

	if layout == nil {
		t.Fatal("expected current factory layout after activation")
	}
	if layout.SchemaVersion != 1 {
		t.Fatalf("layout schemaVersion = %d, want 1", layout.SchemaVersion)
	}
	if layout.Nodes == nil || len(*layout.Nodes) != 1 || (*layout.Nodes)[0].Id != wantNodeID {
		t.Fatalf("layout nodes = %#v, want %s", layout.Nodes, wantNodeID)
	}
	wantEdgeID := "workstation-output:workstation:process->work-state:" + wantWorkType + ":complete"
	if layout.Edges == nil || len(*layout.Edges) != 1 || (*layout.Edges)[0].Id != wantEdgeID {
		t.Fatalf("layout edges = %#v, want %s", layout.Edges, wantEdgeID)
	}
}

func TestFactoryService_ActivateNamedFactory_CanActivateSecondPersistedFactory(t *testing.T) {
	rootDir := t.TempDir()

	for _, name := range []string{"alpha", "beta", "gamma"} {
		if _, err := config.PersistNamedFactory(rootDir, name, serviceNamedFactoryPayload(t, name)); err != nil {
			t.Fatalf("PersistNamedFactory(%s): %v", name, err)
		}
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	if err := svc.ActivateNamedFactory(context.Background(), "beta"); err != nil {
		t.Fatalf("ActivateNamedFactory(beta): %v", err)
	}
	if err := svc.ActivateNamedFactory(context.Background(), "gamma"); err != nil {
		t.Fatalf("ActivateNamedFactory(gamma): %v", err)
	}

	runtimeCfg := svc.currentRuntimeConfig()
	if runtimeCfg == nil {
		t.Fatal("expected runtime config after second activation")
	}
	if got := runtimeCfg.FactoryConfig().Project; got != "gamma" {
		t.Fatalf("active project after second activation = %q, want gamma", got)
	}
	if got, err := config.ReadCurrentFactoryPointer(rootDir); err != nil {
		t.Fatalf("ReadCurrentFactoryPointer: %v", err)
	} else if got != "gamma" {
		t.Fatalf("current factory pointer = %q, want gamma", got)
	}
}

func TestFactoryService_ActivateNamedFactory_RejectsNonIdleRuntime(t *testing.T) {
	svc := &FactoryService{
		logger: zap.NewNop(),
	}
	bindServiceStartupRuntime(svc, &factoryRuntimeBundle{
		Factory: &aggregateSnapshotFactory{
			engineState: &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
				RuntimeStatus: interfaces.RuntimeStatusActive,
			},
		},
	})

	err := svc.ActivateNamedFactory(context.Background(), "beta")
	if err == nil {
		t.Fatal("expected non-idle activation to fail")
	}
	if !errors.Is(err, ErrFactoryActivationRequiresIdle) {
		t.Fatalf("expected ErrFactoryActivationRequiresIdle, got %v", err)
	}
}

func TestFactoryService_RequireIdleRuntime_TargetsActiveRunSession(t *testing.T) {
	idleFactory := &aggregateSnapshotFactory{
		engineState: &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			RuntimeStatus: interfaces.RuntimeStatusIdle,
		},
	}
	activeFactory := &aggregateSnapshotFactory{
		engineState: &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			RuntimeStatus: interfaces.RuntimeStatusActive,
		},
	}

	svc := &FactoryService{
		sessions: factorysessions.NewRegistry(),
		logger:   zap.NewNop(),
	}
	defaultHandle := &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{Factory: idleFactory}}
	betaHandle := &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{Factory: activeFactory}}
	svc.registerLiveSession(defaultFactorySessionID, defaultHandle, FactorySessionTarget{
		Ref: FactorySessionTargetRef{Kind: FactorySessionTargetKindDefault},
	}, false)
	svc.registerLiveSession("session-beta", betaHandle, FactorySessionTarget{
		Ref: FactorySessionTargetRef{Kind: FactorySessionTargetKindNamed, Name: "beta"},
	}, false)
	svc.setRunState(context.Background(), "session-beta", betaHandle)

	err := svc.requireIdleRuntime(context.Background())
	if err == nil {
		t.Fatal("requireIdleRuntime = nil, want active run session idle failure")
	}
	if !errors.Is(err, ErrFactoryActivationRequiresIdle) {
		t.Fatalf("requireIdleRuntime error = %v, want ErrFactoryActivationRequiresIdle", err)
	}
}

func TestFactoryService_ActivateNamedFactory_RollsBackCurrentPointerWhenReplacementBuildFails(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if _, err := config.PersistNamedFactory(rootDir, "beta", serviceNamedFactoryPayload(t, "beta")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	betaFactoryPath := filepath.Join(rootDir, "beta", interfaces.FactoryConfigFile)
	if err := os.WriteFile(betaFactoryPath, []byte(`{"id":"beta","workTypes":[`), 0o644); err != nil {
		t.Fatalf("corrupt beta factory.json: %v", err)
	}

	if err := svc.ActivateNamedFactory(context.Background(), "beta"); err == nil {
		t.Fatal("expected replacement build failure")
	}

	wantCurrentDir := filepath.Join(rootDir, "alpha")
	if svc.coordinatorPolicy().dir != wantCurrentDir {
		t.Fatalf("service dir after failed activation = %q, want %q", svc.coordinatorPolicy().dir, wantCurrentDir)
	}
	runtimeCfg := svc.currentRuntimeConfig()
	if runtimeCfg == nil {
		t.Fatal("expected runtime config after failed activation")
	}
	if got := runtimeCfg.FactoryConfig().Project; got != "alpha" {
		t.Fatalf("active project after failed activation = %q, want alpha", got)
	}
	if got, err := config.ReadCurrentFactoryPointer(rootDir); err != nil {
		t.Fatalf("ReadCurrentFactoryPointer: %v", err)
	} else if got != "alpha" {
		t.Fatalf("current factory pointer after failed activation = %q, want alpha", got)
	}
	if got, err := config.ResolveCurrentFactoryDir(rootDir); err != nil {
		t.Fatalf("ResolveCurrentFactoryDir: %v", err)
	} else if got != wantCurrentDir {
		t.Fatalf("resolved current dir after failed activation = %q, want %q", got, wantCurrentDir)
	}
}

func TestFactoryService_CreateNamedFactory_ActivatesPersistedFactoryFromDefaultRuntime(t *testing.T) {
	rootDir := t.TempDir()
	factoryPath := filepath.Join(rootDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, serviceNamedFactoryPayload(t, "root-runtime"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", factoryPath, err)
	}

	harness := startRunningSessionServiceOnDir(t, rootDir)
	defer harness.stop(t)

	created, err := harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		serviceNamedFactoryContract(t, "beta"),
	)
	if err != nil {
		t.Fatalf("SaveFactoryForSession(upsert beta): %v", err)
	}
	if created.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("created factory name = %q, want beta", created.Name)
	}
	assertCurrentFactoryPointer(t, rootDir, "beta", "after create from default runtime")
	assertServiceCurrentFactory(t, harness.svc, "beta", "after create from default runtime")
	runtimeCfg := harness.svc.currentRuntimeConfig()
	if runtimeCfg == nil || runtimeCfg.FactoryDir() != filepath.Join(rootDir, "beta") {
		t.Fatalf("service runtime dir after create = %q, want %q", runtimeCfg.FactoryDir(), filepath.Join(rootDir, "beta"))
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this named-factory portability test keeps bundled-file materialization assertions together on the service seam.
func TestFactoryService_CreateNamedFactory_MaterializesSupportedPortableBundledFiles(t *testing.T) {
	rootDir := t.TempDir()
	factoryPath := filepath.Join(rootDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, serviceNamedFactoryPayload(t, "root-runtime"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", factoryPath, err)
	}

	harness := startRunningSessionServiceOnDir(t, rootDir)
	defer harness.stop(t)

	created, err := harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		serviceNamedFactoryContractWithBundledFiles(t, "beta"),
	)
	if err != nil {
		t.Fatalf("SaveFactoryForSession(upsert beta): %v", err)
	}
	if created.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("created factory name = %q, want beta", created.Name)
	}
	if created.SupportingFiles == nil || created.SupportingFiles.BundledFiles == nil {
		t.Fatalf("created factory supportingFiles = %#v, want bundled files", created.SupportingFiles)
	}
	if len(*created.SupportingFiles.BundledFiles) != 4 {
		t.Fatalf("created factory bundled files = %#v, want 4 entries", created.SupportingFiles.BundledFiles)
	}
	bundledFiles := *created.SupportingFiles.BundledFiles
	assertServiceBundledFactoryEntry(t, bundledFiles[0], factoryapi.BundledFileTypeROOTHELPER, "Makefile", "test:\n\tgo test ./...\n")
	assertServiceBundledFactoryEntryWithoutInline(t, bundledFiles[1], factoryapi.BundledFileTypeDOC, "factory/docs/README.md")
	assertServiceBundledFactoryEntry(t, bundledFiles[2], factoryapi.BundledFileTypeINPUT, "factory/inputs/task/default/starter.md", "starter work\n")
	assertServiceBundledFactoryEntryWithoutInline(t, bundledFiles[3], factoryapi.BundledFileTypeSCRIPT, "factory/scripts/execute-story.ps1")

	importedDir := filepath.Join(rootDir, "beta")
	assertPortableServiceBundledFile(t, filepath.Join(importedDir, "Makefile"), "test:\n\tgo test ./...\n")
	assertPortableServiceBundledFile(t, filepath.Join(importedDir, "docs", "README.md"), "# Portable factory\n")
	assertPortableServiceBundledFile(t, filepath.Join(importedDir, "inputs", "task", "default", "starter.md"), "starter work\n")
	assertPortableServiceBundledFile(t, filepath.Join(importedDir, "scripts", "execute-story.ps1"), servicePortableBundledScriptBody)
	assertPortableServiceBundledFileMode(t, filepath.Join(importedDir, "scripts", "execute-story.ps1"), 0o755)

	factoryJSON, err := os.ReadFile(filepath.Join(importedDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(factoryJSON, &payload); err != nil {
		t.Fatalf("Unmarshal(factory.json): %v", err)
	}
	supportingFiles, ok := payload["supportingFiles"].(map[string]any)
	if !ok {
		t.Fatalf("expected supportingFiles object, got %#v", payload["supportingFiles"])
	}
	persistedBundledFiles, ok := supportingFiles["bundledFiles"].([]any)
	if !ok || len(persistedBundledFiles) != 4 {
		t.Fatalf("expected four bundled files, got %#v", supportingFiles["bundledFiles"])
	}
	for _, entry := range persistedBundledFiles {
		bundledFile, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("expected bundled file object, got %#v", entry)
		}
		content, ok := bundledFile["content"].(map[string]any)
		if !ok {
			t.Fatalf("expected bundled file content object, got %#v", bundledFile["content"])
		}
		targetPath, _ := bundledFile["targetPath"].(string)
		switch targetPath {
		case "Makefile":
			if got := content["inline"]; got != "test:\n\tgo test ./...\n" {
				t.Fatalf("expected persisted root helper inline content to stay inlined, got %#v", content)
			}
			if got := content["encoding"]; got != "utf-8" {
				t.Fatalf("expected persisted root helper encoding to stay canonical, got %#v", content)
			}
		case "factory/docs/README.md", "factory/inputs/task/default/starter.md", "factory/scripts/execute-story.ps1":
			if _, ok := content["inline"]; ok {
				t.Fatalf("expected persisted bundled file inline content to be omitted, got %#v", content)
			}
			if got := content["encoding"]; got != "utf-8" {
				t.Fatalf("expected persisted bundled file encoding to stay canonical, got %#v", content)
			}
		default:
			t.Fatalf("unexpected persisted bundled file targetPath = %#v", targetPath)
		}
	}
}

func TestFactoryService_BuildFactoryService_LogsPortableBundledFileReplacements(t *testing.T) {
	projectDir := t.TempDir()
	sourceDir := filepath.Join(projectDir, "factory")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(factory): %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "Makefile"), []byte("test:\n\tgo test ./...\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(Makefile): %v", err)
	}
	writeFactoryJSON(t, sourceDir, map[string]any{
		"name": "portable-runtime",
		"supportingFiles": map[string]any{
			"bundledFiles": []map[string]any{
				{
					"type":       "SCRIPT",
					"targetPath": "factory/scripts/execute-story.ps1",
					"content": map[string]any{
						"encoding": "utf-8",
						"inline":   servicePortableBundledScriptBody,
					},
				},
			},
		},
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":    "worker-a",
			"type":    "SCRIPT_WORKER",
			"command": "powershell",
			"args":    []string{"-File", "scripts/execute-story.ps1"},
		}},
		"workstations": []map[string]any{{
			"name":    "process",
			"worker":  "worker-a",
			"inputs":  []map[string]string{{"workType": "task", "state": "init"}},
			"outputs": []map[string]string{{"workType": "task", "state": "complete"}},
		}},
	})
	writeWorkstationAgentsMD(t, sourceDir, "process")
	if err := os.MkdirAll(filepath.Join(sourceDir, "scripts"), 0o755); err != nil {
		t.Fatalf("MkdirAll(factory/scripts): %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "scripts", "execute-story.ps1"), []byte("Write-Output 'stale script'\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(portable script): %v", err)
	}

	logCore, observedLogs := observer.New(zap.WarnLevel)
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               sourceDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.New(logCore),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if bundle := svc.currentRuntimeBundle(); bundle != nil {
		defer func() {
			if err := closeRuntimeBundleSinks(bundle.LogSink, bundle.MetricsSink); err != nil {
				t.Fatalf("Close(runtime artifact sinks): %v", err)
			}
		}()
	}

	if svc.currentRuntimeConfig() == nil {
		t.Fatal("expected runtime config after portable load")
	}
	assertPortableBundledReplacementLogged(t, observedLogs, sourceDir)
}

func assertPortableBundledReplacementLogged(t *testing.T, observedLogs *observer.ObservedLogs, sourceDir string) {
	t.Helper()

	warnings := observedLogs.FilterMessage("runtime config load replaced portable bundled files").All()
	if len(warnings) != 1 {
		t.Fatalf("replacement warning count = %d, want 1", len(warnings))
	}
	fields := warnings[0].ContextMap()
	targetPaths, ok := fields["target_paths"].([]any)
	if !ok {
		t.Fatalf("replacement warning target_paths = %#v, want []any", fields["target_paths"])
	}
	if len(targetPaths) != 1 || targetPaths[0] != "factory/scripts/execute-story.ps1" {
		t.Fatalf("replacement warning target_paths = %#v, want [factory/scripts/execute-story.ps1]", targetPaths)
	}
	data, err := os.ReadFile(filepath.Join(sourceDir, "scripts", "execute-story.ps1"))
	if err != nil {
		t.Fatalf("ReadFile(portable script): %v", err)
	}
	if got := string(data); got != servicePortableBundledScriptBody {
		t.Fatalf("materialized script after replacement = %q, want %q", got, servicePortableBundledScriptBody)
	}
}

func TestFactoryService_CreateNamedFactory_RejectsReservedCurrentFactoryName(t *testing.T) {
	rootDir := t.TempDir()
	factoryPath := filepath.Join(rootDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, serviceNamedFactoryPayload(t, "root-runtime"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", factoryPath, err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	_, err = svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		serviceNamedFactoryContract(t, string(apisurface.DefaultCurrentFactoryName)),
	)
	if !errors.Is(err, apisurface.ErrInvalidNamedFactoryName) {
		t.Fatalf("SaveFactoryForSession(upsert %q) error = %v, want %v", apisurface.DefaultCurrentFactoryName, err, apisurface.ErrInvalidNamedFactoryName)
	}
	assertCurrentFactoryPointerMissing(t, rootDir, "after reserved-name rejection")
}

func TestFactoryService_CreateNamedFactory_RejectsMissingFailureRouteTargets(t *testing.T) {
	rootDir := t.TempDir()
	factoryPath := filepath.Join(rootDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, serviceNamedFactoryPayload(t, "root-runtime"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", factoryPath, err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	invalid := serviceNamedFactoryContractWithWorkType(t, "beta", "task")
	if invalid.WorkTypes == nil || invalid.Workstations == nil {
		t.Fatal("expected fixture work types and workstations")
	}
	(*invalid.WorkTypes)[0].States = []factoryapi.WorkState{
		{Name: "in-review", Type: factoryapi.WorkStateTypePROCESSING},
		{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
	}
	repeater := factoryapi.WorkstationKindRepeater
	(*invalid.Workstations)[0].Name = "bob"
	(*invalid.Workstations)[0].Behavior = &repeater
	(*invalid.Workstations)[0].Inputs = []factoryapi.WorkstationIO{}
	(*invalid.Workstations)[0].Outputs = &[]factoryapi.WorkstationIO{{WorkType: "task", State: "in-review"}}
	(*invalid.Workstations)[0].OnFailure = nil
	(*invalid.Workstations)[0].OnRejection = &[]factoryapi.WorkstationIO{{WorkType: "task", State: "complete"}}

	_, err = svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		invalid,
	)
	var topologyErr *apisurface.TopologyValidationError
	if !errors.As(err, &topologyErr) {
		t.Fatalf("SaveFactoryForSession(upsert) error = %v, want topology validation error", err)
	}
	assertHasValidationTarget(
		t,
		topologyErr.Targets,
		factoryvalidation.CodeWorkstationMissingFailureRoute,
		factoryapi.FactoryValidationSubjectTypeWorkstation,
		"bob",
		factoryapi.FactoryValidationSubjectLocationOnFailure,
		"bob ON_FAILURE target",
	)
}

func TestFactoryService_SaveFactoryForSession_UpsertReplacesExistingNamedFactory(t *testing.T) {
	rootDir := t.TempDir()
	factoryPath := filepath.Join(rootDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, serviceNamedFactoryPayload(t, "root-runtime"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", factoryPath, err)
	}

	harness := startRunningSessionServiceOnDir(t, rootDir)
	defer harness.stop(t)

	created, err := harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		serviceNamedFactoryContract(t, "beta"),
	)
	if err != nil {
		t.Fatalf("SaveFactoryForSession(upsert beta): %v", err)
	}
	replacement := serviceNamedFactoryContractWithWorkType(t, "beta", "story")
	if created.Version == nil {
		t.Fatal("expected created factory version metadata")
	}
	replacement.Version = &factoryapi.HybridLogicalTimestamp{
		Logical:  created.Version.Logical + 1,
		Physical: created.Version.Physical.Add(time.Second),
	}
	replaced, err := harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		replacement,
	)
	if err != nil {
		t.Fatalf("SaveFactoryForSession(upsert replace beta): %v", err)
	}
	assertFactoryWorkType(t, replaced.WorkTypes, "story", "replaced beta work types")
	assertCurrentFactoryPointer(t, rootDir, "beta", "after upsert replace")
}

func TestFactoryService_ActivateNamedFactory_FromDefaultRuntimeLeavesRootReadableWhenReplacementBuildFails(t *testing.T) {
	rootDir := t.TempDir()
	factoryPath := filepath.Join(rootDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, serviceNamedFactoryPayload(t, "root-runtime"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", factoryPath, err)
	}
	if _, err := config.PersistNamedFactory(rootDir, "beta", serviceNamedFactoryPayload(t, "beta")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}
	corruptNamedFactoryConfig(t, rootDir, "beta")

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	if err := svc.ActivateNamedFactory(context.Background(), "beta"); err == nil {
		t.Fatal("expected replacement build failure")
	}

	assertCurrentFactoryPointerMissing(t, rootDir, "after failed activation from default runtime")
	current, err := svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory after failed activation from default runtime: %v", err)
	}
	if current.Name != apisurface.DefaultCurrentFactoryName {
		t.Fatalf("current factory name after failed activation = %q, want %q", current.Name, apisurface.DefaultCurrentFactoryName)
	}
	if current.Id == nil || *current.Id != "root-runtime" {
		t.Fatalf("current factory id after failed activation = %#v, want root-runtime", current.Id)
	}
	runtimeCfg := svc.currentRuntimeConfig()
	if runtimeCfg == nil || runtimeCfg.FactoryDir() != rootDir {
		t.Fatalf("service runtime dir after failed activation = %q, want %q", runtimeCfg.FactoryDir(), rootDir)
	}
}

func TestFactoryService_GetCurrentFactory_ReadsDurablePointerAndCanonicalPayload(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if _, err := config.PersistNamedFactory(rootDir, "beta", serviceNamedFactoryPayload(t, "beta")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "beta"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(beta): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	current, err := svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory: %v", err)
	}
	if current.Name != factoryapi.FactoryName("alpha") {
		t.Fatalf("current factory name = %q, want alpha", current.Name)
	}
	if current.Id == nil || *current.Id != "alpha" {
		t.Fatalf("current factory id = %#v, want alpha", current.Id)
	}
	runtimeCfg := svc.currentRuntimeConfig()
	if runtimeCfg == nil || runtimeCfg.FactoryConfig().Project != "beta" {
		t.Fatalf("service runtime project = %q, want unchanged beta runtime", runtimeCfg.FactoryConfig().Project)
	}
}

func TestFactoryService_GetCurrentFactory_IncludesVersionMetadata(t *testing.T) {
	rootDir := t.TempDir()
	versionTime := time.Date(2026, 5, 23, 12, 30, 0, 0, time.UTC)

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayloadWithVersion(t, "alpha", factoryapi.HybridLogicalTimestamp{
		Logical:  23,
		Physical: versionTime,
	})); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	current, err := svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory: %v", err)
	}
	if current.Name != factoryapi.FactoryName("alpha") {
		t.Fatalf("current factory name = %q, want alpha", current.Name)
	}
	if current.Version == nil || current.Version.Logical != 23 || !current.Version.Physical.Equal(versionTime) {
		t.Fatalf("current factory version = %#v, want logical=23 physical=%s", current.Version, versionTime)
	}
}

func TestFactoryService_SaveCurrentFactory_ReplacesCurrentDefinition(t *testing.T) {
	rootDir := t.TempDir()
	initialVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  41,
		Physical: time.Date(2026, 5, 23, 13, 0, 0, 0, time.UTC),
	}

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayloadWithVersion(t, "alpha", initialVersion)); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	replacement := serviceNamedFactoryContractWithWorkType(t, "alpha", "story")
	replacement.Version = &factoryapi.HybridLogicalTimestamp{
		Logical:  initialVersion.Logical + 1,
		Physical: initialVersion.Physical.Add(time.Second),
	}
	harness := startRunningSessionServiceOnDir(t, rootDir)
	defer harness.stop(t)

	saved, err := harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeReplaceCurrent,
		replacement,
	)
	if err != nil {
		t.Fatalf("SaveFactoryForSession(replace current): %v", err)
	}
	assertFactoryWorkType(t, saved.WorkTypes, "story", "saved work types")
	assertFactoryVersionAdvanced(t, saved.Version, initialVersion)

	current, err := harness.svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory after save: %v", err)
	}
	assertFactoryWorkType(t, current.WorkTypes, "story", "current work types after save")
	assertMatchingFactoryVersion(t, current.Version, saved.Version, "current version after save")
	loaded, err := config.LoadRuntimeConfig(filepath.Join(rootDir, "alpha"), nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(alpha) after save: %v", err)
	}
	assertPersistedFactoryVersionMatchesAPI(t, loaded.FactoryConfig().Version, saved.Version, "persisted version after save")
	restarted, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService(restarted): %v", err)
	}
	restartedCurrent, err := restarted.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory(restarted): %v", err)
	}
	assertMatchingFactoryVersion(t, restartedCurrent.Version, saved.Version, "restarted version")
	assertCurrentFactoryPointer(t, rootDir, "alpha", "after current factory save")
}

func TestFactoryService_SaveCurrentFactory_RejectsStaleBaseVersion(t *testing.T) {
	rootDir := t.TempDir()
	initialVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  7,
		Physical: time.Date(2026, 5, 23, 14, 0, 0, 0, time.UTC),
	}
	newerVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  8,
		Physical: initialVersion.Physical.Add(time.Second),
	}

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayloadWithVersion(t, "alpha", initialVersion)); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	harness := startRunningSessionServiceOnDir(t, rootDir)
	defer harness.stop(t)

	current, err := harness.svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory: %v", err)
	}

	if current.Version == nil {
		t.Fatal("expected current factory version metadata")
	}
	if _, err := config.ReplaceNamedFactory(rootDir, "alpha", serviceNamedFactoryPayloadWithVersion(t, "alpha", newerVersion)); err != nil {
		t.Fatalf("ReplaceNamedFactory(alpha newer version): %v", err)
	}

	replacement := serviceNamedFactoryContractWithWorkType(t, "alpha", "story")
	replacement.Version = current.Version
	_, err = harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeReplaceCurrent,
		replacement,
	)
	if !errors.Is(err, apisurface.ErrFactoryVersionStale) {
		t.Fatalf("SaveFactoryForSession error = %v, want stale version", err)
	}

	currentAfterStaleSave, err := harness.svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory after stale save: %v", err)
	}
	if currentAfterStaleSave.WorkTypes == nil || (*currentAfterStaleSave.WorkTypes)[0].Name != "task" {
		t.Fatalf("current work types after stale save = %#v, want unchanged task", currentAfterStaleSave.WorkTypes)
	}
	if currentAfterStaleSave.Version == nil || currentAfterStaleSave.Version.Logical != newerVersion.Logical || !currentAfterStaleSave.Version.Physical.Equal(newerVersion.Physical) {
		t.Fatalf("current version after stale save = %#v, want %#v", currentAfterStaleSave.Version, newerVersion)
	}
}

func TestFactoryService_SaveCurrentFactory_RejectsDuplicateAndDanglingTopology(t *testing.T) {
	rootDir := t.TempDir()
	initialVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  11,
		Physical: time.Date(2026, 5, 23, 15, 0, 0, 0, time.UTC),
	}

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayloadWithVersion(t, "alpha", initialVersion)); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	harness := startRunningSessionServiceOnDir(t, rootDir)
	defer harness.stop(t)

	currentBeforeInvalidSave, err := harness.svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory before rejected save: %v", err)
	}
	if currentBeforeInvalidSave.Version == nil {
		t.Fatal("expected current factory version metadata")
	}

	replacement := serviceNamedFactoryContractWithWorkType(t, "alpha", "story")
	replacement.Version = &factoryapi.HybridLogicalTimestamp{
		Logical:  currentBeforeInvalidSave.Version.Logical + 1,
		Physical: currentBeforeInvalidSave.Version.Physical.Add(time.Second),
	}
	if replacement.Workers == nil || replacement.Workstations == nil {
		t.Fatal("expected fixture workers and workstations")
	}
	*replacement.Workers = append(*replacement.Workers, (*replacement.Workers)[0])
	(*replacement.Workstations)[0].Worker = "missing-worker"
	(*replacement.Workstations)[0].Outputs = &[]factoryapi.WorkstationIO{{WorkType: "story", State: "missing-state"}}

	_, err = harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeReplaceCurrent,
		replacement,
	)
	var topologyErr *apisurface.TopologyValidationError
	if !errors.As(err, &topologyErr) {
		t.Fatalf("SaveFactoryForSession error = %v, want topology validation error", err)
	}
	assertCanonicalTopologyTargets(t, topologyErr.Targets)

	current, err := harness.svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory after rejected save: %v", err)
	}
	if current.WorkTypes == nil || (*current.WorkTypes)[0].Name != "task" {
		t.Fatalf("current work types after rejected topology = %#v, want unchanged task", current.WorkTypes)
	}
}

func TestFactoryService_SaveCurrentFactory_RejectsMissingOutcomeRoutes(t *testing.T) {
	rootDir := t.TempDir()
	initialVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  11,
		Physical: time.Date(2026, 5, 23, 15, 0, 0, 0, time.UTC),
	}

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayloadWithVersion(t, "alpha", initialVersion)); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	harness := startRunningSessionServiceOnDir(t, rootDir)
	defer harness.stop(t)

	current, err := harness.svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory before rejected save: %v", err)
	}
	if current.Version == nil {
		t.Fatal("expected current factory version metadata")
	}

	replacement := serviceNamedFactoryContractWithWorkType(t, "alpha", "task")
	replacement.Version = &factoryapi.HybridLogicalTimestamp{
		Logical:  current.Version.Logical + 1,
		Physical: current.Version.Physical.Add(time.Second),
	}
	if replacement.WorkTypes == nil || replacement.Workstations == nil {
		t.Fatal("expected fixture work types and workstations")
	}
	(*replacement.WorkTypes)[0].States = []factoryapi.WorkState{
		{Name: "in-review", Type: factoryapi.WorkStateTypePROCESSING},
		{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
	}
	repeater := factoryapi.WorkstationKindRepeater
	(*replacement.Workstations)[0].Behavior = &repeater
	(*replacement.Workstations)[0].Inputs = []factoryapi.WorkstationIO{}
	(*replacement.Workstations)[0].Outputs = &[]factoryapi.WorkstationIO{{WorkType: "task", State: "in-review"}}
	(*replacement.Workstations)[0].OnFailure = nil
	(*replacement.Workstations)[0].OnRejection = nil

	_, err = harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeReplaceCurrent,
		replacement,
	)
	var topologyErr *apisurface.TopologyValidationError
	if !errors.As(err, &topologyErr) {
		t.Fatalf("SaveFactoryForSession error = %v, want topology validation error", err)
	}
	assertHasValidationTargetCode(t, topologyErr.Targets, factoryvalidation.CodeWorkstationMissingFailureRoute, "missing failure route target")
	assertHasValidationTargetCode(t, topologyErr.Targets, factoryvalidation.CodeWorkstationMissingRejectionRoute, "missing rejection route target")
}

func TestFactoryService_SaveFactoryForSession_RejectsDuplicateDefaultHandlingWorkTypes(t *testing.T) {
	rootDir := t.TempDir()
	initialVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  11,
		Physical: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
	}

	payload, err := json.Marshal(map[string]any{
		"name": "alpha",
		"id":   "alpha",
		"version": map[string]any{
			"logical":  initialVersion.Logical,
			"physical": initialVersion.Physical.UTC().Format(time.RFC3339Nano),
		},
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{"name": "worker-a", "type": "MODEL_WORKER", "modelProvider": "CODEX", "model": "gpt-5-codex", "body": "worker"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"type":      "MODEL_WORKSTATION",
			"body":      "process",
			"inputs":    []map[string]string{{"workType": "story", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "story", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "story", "state": "failed"}},
		}},
	})
	if err != nil {
		t.Fatalf("marshal alpha payload: %v", err)
	}
	if _, err := config.PersistNamedFactory(rootDir, "alpha", payload); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	harness := startRunningSessionServiceOnDir(t, rootDir)
	defer harness.stop(t)

	current, err := harness.svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory before rejected save: %v", err)
	}
	if current.Version == nil {
		t.Fatal("expected current factory version metadata")
	}

	replacement, err := factoryfixtures.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}
	if replacement.WorkTypes == nil {
		t.Fatal("expected alpha fixture work types")
	}
	defaultBehavior := factoryapi.WorkTypeHandlingBehaviorDefault
	(*replacement.WorkTypes)[0].HandlingBehavior = &[]factoryapi.WorkTypeHandlingBehavior{defaultBehavior}
	second := (*replacement.WorkTypes)[0]
	second.Name = "task"
	second.HandlingBehavior = &[]factoryapi.WorkTypeHandlingBehavior{defaultBehavior}
	*replacement.WorkTypes = append(*replacement.WorkTypes, second)
	replacement.Version = &factoryapi.HybridLogicalTimestamp{
		Logical:  current.Version.Logical + 1,
		Physical: current.Version.Physical.Add(time.Second),
	}

	_, err = harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeReplaceCurrent,
		replacement,
	)
	var topologyErr *apisurface.TopologyValidationError
	if !errors.As(err, &topologyErr) {
		t.Fatalf("SaveFactoryForSession error = %v, want topology validation error", err)
	}
	assertHasValidationTargetCode(
		t,
		topologyErr.Targets,
		factoryvalidation.CodeWorkTypeHandlingBehaviorUniqueDefault,
		"duplicate default handling target",
	)

	currentAfterRejectedSave, err := harness.svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory after rejected save: %v", err)
	}
	if currentAfterRejectedSave.WorkTypes == nil || len(*currentAfterRejectedSave.WorkTypes) != 1 {
		t.Fatalf("current work types after rejected save = %#v, want unchanged single story work type", currentAfterRejectedSave.WorkTypes)
	}
}

func TestFactoryService_SaveFactoryForSession_AllowsSingleDefaultHandlingWorkType(t *testing.T) {
	rootDir := t.TempDir()
	initialVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  11,
		Physical: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
	}

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayloadWithVersion(t, "alpha", initialVersion)); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	harness := startRunningSessionServiceOnDir(t, rootDir)
	defer harness.stop(t)

	current, err := harness.svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory before save: %v", err)
	}
	if current.Version == nil {
		t.Fatal("expected current factory version metadata")
	}

	replacement := serviceNamedFactoryContractWithWorkType(t, "alpha", "story")
	defaultBehavior := factoryapi.WorkTypeHandlingBehaviorDefault
	if replacement.WorkTypes == nil {
		t.Fatal("expected replacement work types")
	}
	(*replacement.WorkTypes)[0].HandlingBehavior = &[]factoryapi.WorkTypeHandlingBehavior{defaultBehavior}
	replacement.Version = &factoryapi.HybridLogicalTimestamp{
		Logical:  current.Version.Logical + 1,
		Physical: current.Version.Physical.Add(time.Second),
	}

	saved, err := harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeReplaceCurrent,
		replacement,
	)
	if err != nil {
		t.Fatalf("SaveFactoryForSession error = %v, want success", err)
	}
	if saved.WorkTypes == nil || (*saved.WorkTypes)[0].HandlingBehavior == nil || len(*(*saved.WorkTypes)[0].HandlingBehavior) != 1 {
		t.Fatalf("saved work types = %#v, want one DEFAULT handlingBehavior", saved.WorkTypes)
	}
	if (*(*saved.WorkTypes)[0].HandlingBehavior)[0] != defaultBehavior {
		t.Fatalf("saved handlingBehavior = %#v, want DEFAULT", (*saved.WorkTypes)[0].HandlingBehavior)
	}
}

func assertCanonicalTopologyTargets(t *testing.T, targets []factoryapi.FactoryValidationTarget) {
	t.Helper()

	if len(targets) < 3 {
		t.Fatalf("topology targets = %#v, want duplicate worker, missing worker, and dangling output targets", targets)
	}

	assertHasValidationTarget(
		t,
		targets,
		factoryvalidation.CodeDuplicateIdentifier,
		factoryapi.FactoryValidationSubjectTypeWorker,
		"worker-a",
		factoryapi.FactoryValidationSubjectLocationDefinition,
		"duplicate worker target",
	)
	assertHasValidationTarget(
		t,
		targets,
		factoryvalidation.CodeDanglingWorkerReference,
		factoryapi.FactoryValidationSubjectTypeWorkstation,
		"process",
		factoryapi.FactoryValidationSubjectLocationReference,
		"missing workstation worker target",
	)
	assertHasValidationTarget(
		t,
		targets,
		factoryvalidation.CodeDanglingPlaceReference,
		factoryapi.FactoryValidationSubjectTypeRoute,
		"process->story:missing-state",
		factoryapi.FactoryValidationSubjectLocationOutputs,
		"dangling output target",
	)
}

func assertHasValidationTargetCode(t *testing.T, targets []factoryapi.FactoryValidationTarget, code, want string) {
	t.Helper()
	for _, target := range targets {
		if target.Code == code {
			return
		}
	}
	t.Fatalf("topology targets = %#v, want %s", targets, want)
}

func assertHasValidationTarget(
	t *testing.T,
	targets []factoryapi.FactoryValidationTarget,
	code string,
	subjectType factoryapi.FactoryValidationSubjectType,
	subjectID string,
	location factoryapi.FactoryValidationSubjectLocation,
	want string,
) {
	t.Helper()
	for _, target := range targets {
		if target.Code != code {
			continue
		}
		if target.Subject.Type == subjectType && target.Subject.Id == subjectID && target.Subject.Location == location {
			return
		}
	}
	t.Fatalf("topology targets = %#v, want %s", targets, want)
}

func TestFactoryService_GetCurrentFactory_CollectsSupportedPortableBundledFilesFromDisk(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	alphaDir := filepath.Join(rootDir, "alpha")
	writePortableServiceBundledFile(t, filepath.Join(alphaDir, "scripts", "execute-story.ps1"), servicePortableBundledScriptBody)
	writePortableServiceBundledFile(t, filepath.Join(alphaDir, "docs", "README.md"), "# Portable factory\n")
	writePortableServiceBundledFile(t, filepath.Join(rootDir, "Makefile"), "test:\n\tgo test ./...\n")
	writePortableServiceBundledFile(t, filepath.Join(rootDir, "README.md"), "outside allowlist\n")

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	current, err := svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory: %v", err)
	}
	if current.SupportingFiles == nil {
		t.Fatal("expected current factory to include supportingFiles")
	}
	if current.SupportingFiles.BundledFiles == nil || len(*current.SupportingFiles.BundledFiles) != 2 {
		t.Fatalf("expected 2 bundled files, got %#v", current.SupportingFiles.BundledFiles)
	}
	bundledFiles := *current.SupportingFiles.BundledFiles
	assertServiceBundledFactoryEntry(t, bundledFiles[0], factoryapi.BundledFileTypeDOC, "factory/docs/README.md", "# Portable factory\n")
	assertServiceBundledFactoryEntry(t, bundledFiles[1], factoryapi.BundledFileTypeSCRIPT, "factory/scripts/execute-story.ps1", servicePortableBundledScriptBody)
}

func TestFactoryService_GetCurrentFactory_CollectsNestedFactoryDocsFromDisk(t *testing.T) {
	const nestedDocPath = "factory/docs/standards/review.md"
	const nestedDocBody = "# Review standards\n"

	rootDir := t.TempDir()

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	alphaDir := filepath.Join(rootDir, "alpha")
	writePortableServiceBundledFile(t, filepath.Join(alphaDir, "docs", "README.md"), "# Portable factory\n")
	writePortableServiceBundledFile(t, filepath.Join(alphaDir, "docs", "standards", "review.md"), nestedDocBody)
	writePortableServiceBundledFile(t, filepath.Join(alphaDir, "scripts", "execute-story.ps1"), servicePortableBundledScriptBody)

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	current, err := svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory: %v", err)
	}

	got := serviceBundledFilesByTarget(t, current)
	if entry, ok := got[nestedDocPath]; !ok {
		t.Fatalf("current factory bundled files = %#v, want nested doc %q", got, nestedDocPath)
	} else {
		assertServiceBundledFactoryEntry(t, entry, factoryapi.BundledFileTypeDOC, nestedDocPath, nestedDocBody)
	}
	assertServiceBundledFactoryEntry(t, got["factory/docs/README.md"], factoryapi.BundledFileTypeDOC, "factory/docs/README.md", "# Portable factory\n")
}

func TestFactoryService_GetCurrentFactory_ManifestAuthoritativeDocsExcludeUnlistedTopLevelOrphans(t *testing.T) {
	const nestedDocPath = "factory/docs/standards/review.md"
	const nestedDocBody = "# Review standards\n"
	const orphanDocPath = "factory/docs/orphan.md"

	rootDir := t.TempDir()
	payload, err := json.Marshal(map[string]any{
		"name": "alpha",
		"id":   "alpha",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":          "worker-a",
			"type":          "MODEL_WORKER",
			"modelProvider": "CODEX",
			"model":         "gpt-5-codex",
			"body":          "You are worker alpha.",
		}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			"type":      "MODEL_WORKSTATION",
			"body":      "Do the alpha work.",
		}},
		"supportingFiles": map[string]any{
			"bundledFiles": []map[string]any{{
				"type":       "DOC",
				"targetPath": "factory/docs/README.md",
				"content": map[string]any{
					"encoding": string(factoryapi.BundledFileContentEncodingUtf8),
					"inline":   "# Portable factory\n",
				},
			}},
		},
	})
	if err != nil {
		t.Fatalf("marshal named factory payload: %v", err)
	}

	if _, err := config.PersistNamedFactory(rootDir, "alpha", payload); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	alphaDir := filepath.Join(rootDir, "alpha")
	writePortableServiceBundledFile(t, filepath.Join(alphaDir, "docs", "README.md"), "# Portable factory\n")
	writePortableServiceBundledFile(t, filepath.Join(alphaDir, "docs", "orphan.md"), "orphan content\n")
	writePortableServiceBundledFile(t, filepath.Join(alphaDir, "docs", "standards", "review.md"), nestedDocBody)

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	current, err := svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory: %v", err)
	}

	got := serviceBundledFilesByTarget(t, current)
	if _, ok := got[orphanDocPath]; ok {
		t.Fatalf("current factory bundled files = %#v, want unlisted top-level orphan doc excluded", got)
	}
	assertServiceBundledFactoryEntry(t, got["factory/docs/README.md"], factoryapi.BundledFileTypeDOC, "factory/docs/README.md", "# Portable factory\n")
	assertServiceBundledFactoryEntry(t, got[nestedDocPath], factoryapi.BundledFileTypeDOC, nestedDocPath, nestedDocBody)
}

func TestFactoryService_GetCurrentFactory_InlinesPortableFilesAndStarterInputs(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayloadWithBundledInput(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	alphaDir := filepath.Join(rootDir, "alpha")
	if err := os.Remove(filepath.Join(alphaDir, "inputs", "task", "default", "stale.md")); err != nil {
		t.Fatalf("Remove(stale starter): %v", err)
	}
	writePortableServiceBundledFile(t, filepath.Join(alphaDir, "scripts", "execute-story.ps1"), servicePortableBundledScriptBody)
	writePortableServiceBundledFile(t, filepath.Join(alphaDir, "scripts", ".gitkeep"), "ignored\n")
	writePortableServiceBundledFile(t, filepath.Join(alphaDir, "scripts", "draft.tmp"), "ignored\n")
	writePortableServiceBundledFile(t, filepath.Join(alphaDir, "docs", "README.md"), "# Portable factory\n")
	writePortableServiceBundledFile(t, filepath.Join(rootDir, "Makefile"), "test:\n\tgo test ./...\n")
	writePortableServiceBundledFile(t, filepath.Join(alphaDir, "inputs", "task", "default", "starter.md"), "fresh starter\n")
	writePortableServiceBundledFile(t, filepath.Join(alphaDir, "inputs", "task", "default", ".gitkeep"), "ignored\n")
	writePortableServiceBundledFile(t, filepath.Join(alphaDir, "inputs", "task", "default", "draft.swp"), "ignored\n")
	writePortableServiceBundledFile(t, filepath.Join(alphaDir, "inputs", "unknown", "default", "starter.md"), "ignored\n")
	writePortableServiceBundledFile(t, filepath.Join(alphaDir, "inputs", "task", "default", "nested", "starter.md"), "ignored\n")

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	current, err := svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory: %v", err)
	}

	got := serviceBundledFilesByTarget(t, current)
	want := map[string]struct {
		fileType factoryapi.BundledFileType
		inline   string
	}{
		"factory/docs/README.md":                 {fileType: factoryapi.BundledFileTypeDOC, inline: "# Portable factory\n"},
		"factory/inputs/task/default/starter.md": {fileType: factoryapi.BundledFileTypeINPUT, inline: "fresh starter\n"},
		"factory/scripts/execute-story.ps1":      {fileType: factoryapi.BundledFileTypeSCRIPT, inline: servicePortableBundledScriptBody},
	}
	if len(got) != len(want) {
		t.Fatalf("bundled files = %#v, want targets %#v", got, want)
	}
	for targetPath, wantEntry := range want {
		bundledFile, ok := got[targetPath]
		if !ok {
			t.Fatalf("missing bundled file %q in %#v", targetPath, got)
		}
		assertServiceBundledFactoryEntry(t, bundledFile, wantEntry.fileType, targetPath, wantEntry.inline)
	}
	if stale := got["factory/inputs/task/default/stale.md"]; stale.TargetPath != "" {
		t.Fatalf("stale input bundled file survived readback: %#v", stale)
	}
}

func TestFactoryService_SaveDefaultCurrentFactory_PersistsSplitLayout(t *testing.T) {
	rootDir := t.TempDir()
	factoryPath := filepath.Join(rootDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, serviceNamedFactoryPayload(t, "root-runtime"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", factoryPath, err)
	}
	staleWorkerDir := filepath.Join(rootDir, interfaces.WorkersDir, "stale-worker")
	if err := os.MkdirAll(staleWorkerDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(stale-worker): %v", err)
	}
	if err := os.WriteFile(filepath.Join(staleWorkerDir, interfaces.FactoryAgentsFileName), []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile(stale AGENTS.md): %v", err)
	}

	harness := startRunningSessionServiceOnDir(t, rootDir)
	defer harness.stop(t)

	current, err := harness.svc.GetCurrentFactoryForSession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetCurrentFactoryForSession: %v", err)
	}
	if current.Version == nil {
		t.Fatal("expected default current factory version metadata")
	}

	replacement := serviceNamedFactoryContractWithWorkType(t, "root-runtime", "story")
	replacement.Name = apisurface.DefaultCurrentFactoryName
	replacement.Version = &factoryapi.HybridLogicalTimestamp{
		Logical:  current.Version.Logical + 1,
		Physical: current.Version.Physical.Add(time.Second),
	}
	if _, err := harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeReplaceCurrent,
		replacement,
	); err != nil {
		t.Fatalf("SaveFactoryForSession(replace default): %v", err)
	}

	assertServiceSplitLayoutAtRoot(t, rootDir, "root-runtime", "You are worker root-runtime.")
	if _, err := os.Stat(staleWorkerDir); !os.IsNotExist(err) {
		t.Fatalf("stale worker dir after save: stat err=%v, want removed", err)
	}
}

func assertServiceSplitLayoutAtRoot(t *testing.T, rootDir, project, wantWorkerBody string) {
	t.Helper()

	factoryJSON, err := os.ReadFile(filepath.Join(rootDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	if strings.Contains(string(factoryJSON), wantWorkerBody) {
		t.Fatalf("factory.json should omit inlined worker body %q, got %s", wantWorkerBody, factoryJSON)
	}

	agentsPath := filepath.Join(rootDir, interfaces.WorkersDir, "worker-a", interfaces.FactoryAgentsFileName)
	agents, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(worker AGENTS.md): %v", err)
	}
	if !strings.Contains(string(agents), wantWorkerBody) {
		t.Fatalf("worker AGENTS.md = %q, want body %q", agents, wantWorkerBody)
	}

	workstationPath := filepath.Join(rootDir, interfaces.WorkstationsDir, "process", interfaces.FactoryAgentsFileName)
	if _, err := os.Stat(workstationPath); err != nil {
		t.Fatalf("expected workstation AGENTS.md at %s: %v", workstationPath, err)
	}

	loaded, err := config.LoadRuntimeConfig(rootDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	if loaded.FactoryConfig().Project != project {
		t.Fatalf("project = %q, want %q", loaded.FactoryConfig().Project, project)
	}
}

func TestFactoryService_GetCurrentFactory_FallsBackToRootRuntimeWhenPointerMissing(t *testing.T) {
	rootDir := t.TempDir()
	factoryPath := filepath.Join(rootDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, serviceNamedFactoryPayload(t, "root-runtime"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", factoryPath, err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	current, err := svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory: %v", err)
	}
	if current.Name != apisurface.DefaultCurrentFactoryName {
		t.Fatalf("current factory name = %q, want %q", current.Name, apisurface.DefaultCurrentFactoryName)
	}
	if current.Id == nil || *current.Id != "root-runtime" {
		t.Fatalf("current factory id = %#v, want root-runtime", current.Id)
	}
	runtimeCfg := svc.currentRuntimeConfig()
	if runtimeCfg == nil || runtimeCfg.FactoryDir() != rootDir {
		t.Fatalf("service runtime dir = %q, want %q", runtimeCfg.FactoryDir(), rootDir)
	}
}

func TestFactoryService_GetCurrentFactory_ReturnsNotFoundWhenPointerMissingWithoutRuntimeFallback(t *testing.T) {
	rootDir := t.TempDir()
	svc := &FactoryService{
		policy: serviceCoordinatorPolicyFromConfig(&FactoryServiceConfig{Dir: rootDir}),
	}

	_, err := svc.GetCurrentFactory(context.Background())
	if !errors.Is(err, ErrCurrentFactoryNotFound) {
		t.Fatalf("GetCurrentFactory missing pointer error = %v, want %v", err, ErrCurrentFactoryNotFound)
	}
}

func TestFactoryService_GetCurrentFactory_WrapsMissingPersistedFactoryDir(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(rootDir, interfaces.CurrentFactoryPointerFile),
		[]byte("missing\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(current-factory.txt): %v", err)
	}

	svc := &FactoryService{
		policy:         serviceCoordinatorPolicyFromConfig(&FactoryServiceConfig{Dir: rootDir}),
		factoryRootDir: rootDir,
	}

	_, err := svc.GetCurrentFactory(context.Background())
	if err == nil {
		t.Fatal("expected missing persisted factory dir error")
	}
	if !strings.Contains(err.Error(), `resolve current factory "missing"`) {
		t.Fatalf("GetCurrentFactory resolve error = %v, want wrapped missing-factory context", err)
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_ValidateOnlyReturnsInitsNewFactoryForEmptyReadableFolder(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	before := harness.svc.sessions.Count()
	emptyDir := filepath.Join(harness.rootDir, "empty")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatalf("Mkdir(empty): %v", err)
	}

	result, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), emptyDir, nil, true, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(validate only, empty): %v", err)
	}
	if result == nil {
		t.Fatal("validate-only empty-folder result = nil, want initsNewFactory metadata")
	}
	if !result.InitsNewFactory {
		t.Fatalf("validate-only initsNewFactory = false, want true")
	}
	if result.FolderPath != emptyDir {
		t.Fatalf("validate-only folderPath = %q, want absolute %q", result.FolderPath, emptyDir)
	}
	if result.SessionID != "" {
		t.Fatalf("validate-only session id = %q, want none", result.SessionID)
	}
	if len(result.Targets) != 0 {
		t.Fatalf("validate-only targets = %#v, want none", result.Targets)
	}
	if got := harness.svc.sessions.Count(); got != before {
		t.Fatalf("validate-only empty-folder mutated live sessions to %d, want %d", got, before)
	}
}

func TestFactoryService_OpenFactorySession_ValidateOnlyMapsInitsNewFactoryToAPIResponse(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	emptyDir := filepath.Join(harness.rootDir, "empty")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatalf("Mkdir(empty): %v", err)
	}

	validateOnly := true
	response, err := harness.svc.OpenFactorySession(context.Background(), factoryapi.OpenFactorySessionRequest{
		FolderPath:   emptyDir,
		ValidateOnly: &validateOnly,
	})
	if err != nil {
		t.Fatalf("OpenFactorySession(validate only, empty): %v", err)
	}
	if response.InitsNewFactory == nil || !*response.InitsNewFactory {
		t.Fatalf("response.initsNewFactory = %#v, want true", response.InitsNewFactory)
	}
	if response.FolderPath == nil || *response.FolderPath != emptyDir {
		t.Fatalf("response.folderPath = %#v, want %q", response.FolderPath, emptyDir)
	}
	if response.Session != nil {
		t.Fatalf("response.session = %#v, want none", response.Session)
	}
	if response.Targets != nil {
		t.Fatalf("response.targets = %#v, want none", response.Targets)
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_AlignsValidateAndOpenDiscoveryAcrossRunnableEmptyAndBrokenFolders(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha"},
	})
	defer harness.stop(t)

	emptyDir := filepath.Join(harness.rootDir, "empty")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatalf("Mkdir(empty): %v", err)
	}

	brokenDir := filepath.Join(harness.rootDir, "broken")
	if err := os.MkdirAll(brokenDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", brokenDir, err)
	}
	if err := os.WriteFile(filepath.Join(brokenDir, interfaces.FactoryConfigFile), []byte(`{"name":`), 0o644); err != nil {
		t.Fatalf("WriteFile(broken factory.json): %v", err)
	}

	before := harness.svc.sessions.Count()
	assertValidateRunnableDiscovery(t, harness, before)
	assertOpenRunnableDiscovery(t, harness, before+1)
	assertValidateEmptyDiscovery(t, harness, emptyDir, before+1)
	assertOpenEmptyDiscoveryFailure(t, harness, emptyDir, before+1)
	assertValidateBrokenDiscoveryFailure(t, harness, brokenDir, before+1)
	assertOpenBrokenDiscoveryFailure(t, harness, brokenDir, before+1)
}

func assertValidateRunnableDiscovery(t *testing.T, harness *runningSessionService, wantSessionCount int) {
	t.Helper()

	validateRunnable, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), harness.rootDir, nil, true, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(validate runnable): %v", err)
	}
	if validateRunnable == nil || validateRunnable.InitsNewFactory || len(validateRunnable.Targets) == 0 || validateRunnable.SessionID != "" {
		t.Fatalf("validate runnable result = %#v, want discovered targets without session or init-new-factory", validateRunnable)
	}
	assertLiveSessionCount(t, harness, "validate runnable", wantSessionCount)
}

func assertOpenRunnableDiscovery(t *testing.T, harness *runningSessionService, wantSessionCount int) {
	t.Helper()

	openRunnable, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), harness.rootDir, nil, false, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(open runnable): %v", err)
	}
	if openRunnable == nil || openRunnable.SessionID == "" {
		t.Fatalf("open runnable result = %#v, want session id", openRunnable)
	}
	assertLiveSessionCount(t, harness, "open runnable", wantSessionCount)
}

func assertValidateEmptyDiscovery(t *testing.T, harness *runningSessionService, emptyDir string, wantSessionCount int) {
	t.Helper()

	validateEmpty, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), emptyDir, nil, true, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(validate empty): %v", err)
	}
	if validateEmpty == nil || !validateEmpty.InitsNewFactory || validateEmpty.FolderPath != emptyDir {
		t.Fatalf("validate empty result = %#v, want init-new-factory metadata", validateEmpty)
	}
	assertLiveSessionCount(t, harness, "validate empty", wantSessionCount)
}

func assertOpenEmptyDiscoveryFailure(t *testing.T, harness *runningSessionService, emptyDir string, wantSessionCount int) {
	t.Helper()

	if _, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), emptyDir, nil, false, false); err == nil {
		t.Fatal("OpenFactorySessionFromFolder(open empty) error = nil, want not-runnable failure")
	} else {
		assertFactorySessionValidationTarget(t, err, "not_runnable", "folderPath")
	}
	assertLiveSessionCount(t, harness, "open empty", wantSessionCount)
}

func assertValidateBrokenDiscoveryFailure(t *testing.T, harness *runningSessionService, brokenDir string, wantSessionCount int) {
	t.Helper()

	validateBroken, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), brokenDir, nil, true, false)
	if err == nil {
		t.Fatal("OpenFactorySessionFromFolder(validate broken) error = nil, want config-load failure")
	}
	assertFactorySessionConfigLoadFailure(t, err, "default")
	if validateBroken != nil {
		t.Fatalf("validate broken result = %#v, want no init-new-factory fallback", validateBroken)
	}
	assertLiveSessionCount(t, harness, "validate broken", wantSessionCount)
}

func assertOpenBrokenDiscoveryFailure(t *testing.T, harness *runningSessionService, brokenDir string, wantSessionCount int) {
	t.Helper()

	if _, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), brokenDir, nil, false, false); err == nil {
		t.Fatal("OpenFactorySessionFromFolder(open broken) error = nil, want config-load failure")
	} else {
		assertFactorySessionConfigLoadFailure(t, err, "default")
	}
	assertLiveSessionCount(t, harness, "open broken", wantSessionCount)
}

func assertLiveSessionCount(t *testing.T, harness *runningSessionService, label string, want int) {
	t.Helper()

	if got := harness.svc.sessions.Count(); got != want {
		t.Fatalf("%s mutated live sessions to %d, want %d", label, got, want)
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_ValidateOnlyRunnableFolderOmitsInitsNewFactory(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha"},
	})
	defer harness.stop(t)

	result, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), harness.rootDir, nil, true, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(validate only, runnable): %v", err)
	}
	if result == nil {
		t.Fatal("validate-only runnable result = nil, want targets")
	}
	if result.InitsNewFactory {
		t.Fatal("validate-only initsNewFactory = true, want false for runnable folder")
	}
	if len(result.Targets) == 0 {
		t.Fatalf("validate-only targets = %#v, want runnable targets", result.Targets)
	}
}

func setupInitNewFactoryProjectDir(t *testing.T, rootDir, name string) (projectDir, sentinelPath string, sentinelContents []byte, existingDir string) {
	t.Helper()

	projectDir = filepath.Join(rootDir, name)
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatalf("Mkdir(%s): %v", name, err)
	}
	sentinelPath = filepath.Join(projectDir, "README.md")
	sentinelContents = []byte("existing project notes\n")
	if err := os.WriteFile(sentinelPath, sentinelContents, 0o644); err != nil {
		t.Fatalf("WriteFile(sentinel): %v", err)
	}
	existingDir = filepath.Join(projectDir, "src")
	if err := os.Mkdir(existingDir, 0o755); err != nil {
		t.Fatalf("Mkdir(existing src): %v", err)
	}
	return projectDir, sentinelPath, sentinelContents, existingDir
}

func assertNestedInitScaffoldLayout(t *testing.T, nestedFactoryDir string) {
	t.Helper()

	factoryConfigPath := filepath.Join(nestedFactoryDir, interfaces.FactoryConfigFile)
	written, err := os.ReadFile(factoryConfigPath)
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	if normalizeInitFactoryJSON(t, string(written)) != normalizeInitFactoryJSON(t, initcmd.DefaultFactoryJSON()) {
		t.Fatalf("written factory.json does not match embedded default scaffold")
	}
	processorWorkerPath := filepath.Join(nestedFactoryDir, interfaces.WorkersDir, "processor", interfaces.FactoryAgentsFileName)
	if _, err := os.Stat(processorWorkerPath); err != nil {
		t.Fatalf("Stat(processor AGENTS.md): %v", err)
	}
	defaultInputDir := filepath.Join(nestedFactoryDir, interfaces.InputsDir, "task", interfaces.DefaultChannelName)
	if _, err := os.Stat(defaultInputDir); err != nil {
		t.Fatalf("Stat(default input dir): %v", err)
	}
}

func assertNoRootLevelScaffoldPaths(t *testing.T, projectDir string) {
	t.Helper()

	for _, rootOnlyPath := range []string{
		filepath.Join(projectDir, interfaces.FactoryConfigFile),
		filepath.Join(projectDir, interfaces.WorkersDir),
		filepath.Join(projectDir, interfaces.WorkstationsDir),
		filepath.Join(projectDir, interfaces.InputsDir),
	} {
		if _, err := os.Stat(rootOnlyPath); !os.IsNotExist(err) {
			t.Fatalf("root scaffold path %q should not exist after init-new-factory, stat err=%v", rootOnlyPath, err)
		}
	}
}

func assertProjectRootContentsPreserved(t *testing.T, sentinelPath string, sentinelContents []byte, existingDir string) {
	t.Helper()

	preserved, err := os.ReadFile(sentinelPath)
	if err != nil {
		t.Fatalf("ReadFile(sentinel): %v", err)
	}
	if string(preserved) != string(sentinelContents) {
		t.Fatalf("sentinel contents = %q, want %q", preserved, sentinelContents)
	}
	if _, err := os.Stat(existingDir); err != nil {
		t.Fatalf("existing root directory removed: %v", err)
	}
}

func assertValidateAfterNestedInit(
	t *testing.T,
	validateResult *FactorySessionOpenResult,
	projectDir, nestedFactoryDir string,
) {
	t.Helper()

	if validateResult == nil || validateResult.InitsNewFactory {
		t.Fatalf("validate-after-init result = %#v, want runnable targets without initsNewFactory", validateResult)
	}
	if len(validateResult.Targets) != 1 {
		t.Fatalf("validate-after-init targets = %#v, want one nested factory target", validateResult.Targets)
	}
	assertSessionTargetMetadata(
		t,
		validateResult.Targets[0],
		FactorySessionTargetKindNamed,
		interfaces.FactoryDir,
		interfaces.FactoryDir,
		nestedFactoryDir,
		interfaces.FactoryDir,
	)
	if validateResult.Targets[0].FolderPath != projectDir {
		t.Fatalf("validate-after-init folder path = %q, want %q", validateResult.Targets[0].FolderPath, projectDir)
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_InitNewFactoryCreatesScaffoldAndOpensSession(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	before := harness.svc.sessions.Count()
	projectDir, sentinelPath, sentinelContents, existingDir := setupInitNewFactoryProjectDir(t, harness.rootDir, "new-factory")

	result, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), projectDir, nil, false, true)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(init new factory): %v", err)
	}
	if result == nil || result.SessionID == "" {
		t.Fatalf("init-new-factory result = %#v, want session id", result)
	}

	nestedFactoryDir := filepath.Join(projectDir, interfaces.FactoryDir)
	assertNestedInitScaffoldLayout(t, nestedFactoryDir)
	assertNoRootLevelScaffoldPaths(t, projectDir)
	assertProjectRootContentsPreserved(t, sentinelPath, sentinelContents, existingDir)

	session := harness.requireSession(t, result.SessionID)
	assertNestedInitSessionMetadata(t, session, projectDir, nestedFactoryDir)
	if got := harness.svc.sessions.Count(); got != before+1 {
		t.Fatalf("live session count = %d, want %d", got, before+1)
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_InitNewFactoryThenReopenThroughSelectedFolder(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	projectDir := filepath.Join(harness.rootDir, "reopen-project")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatalf("Mkdir(reopen-project): %v", err)
	}
	nestedFactoryDir := filepath.Join(projectDir, interfaces.FactoryDir)

	initResult, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), projectDir, nil, false, true)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(init new factory): %v", err)
	}
	if initResult == nil || initResult.SessionID == "" {
		t.Fatalf("init-new-factory result = %#v, want session id", initResult)
	}

	initSession := harness.requireSession(t, initResult.SessionID)
	assertNestedInitSessionMetadata(t, initSession, projectDir, nestedFactoryDir)

	listAfterInit, err := harness.svc.ListFactorySessions(context.Background())
	if err != nil {
		t.Fatalf("ListFactorySessions after init: %v", err)
	}
	assertListContainsNestedInitSession(t, listAfterInit.Sessions, initResult.SessionID, projectDir, nestedFactoryDir)

	validateResult, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), projectDir, nil, true, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(validate after init): %v", err)
	}
	assertValidateAfterNestedInit(t, validateResult, projectDir, nestedFactoryDir)

	if err := harness.svc.CloseFactorySession(context.Background(), initResult.SessionID); err != nil {
		t.Fatalf("CloseFactorySession(init session): %v", err)
	}

	reopenAPI, err := harness.svc.OpenFactorySession(context.Background(), factoryapi.OpenFactorySessionRequest{
		FolderPath: projectDir,
	})
	if err != nil {
		t.Fatalf("OpenFactorySession(reopen via API): %v", err)
	}
	if reopenAPI.Session == nil || reopenAPI.Session.Id == "" {
		t.Fatalf("reopen API response.session = %#v, want live session summary", reopenAPI.Session)
	}
	if reopenAPI.Session.Id == initResult.SessionID {
		t.Fatalf("reopened session id = %q, want a new session identity", reopenAPI.Session.Id)
	}
	assertNestedInitAPISessionMetadata(t, *reopenAPI.Session, reopenAPI.Session.Id, projectDir, nestedFactoryDir)

	reopenSession := harness.requireSession(t, reopenAPI.Session.Id)
	assertNestedInitSessionMetadata(t, reopenSession, projectDir, nestedFactoryDir)

	listAfterReopen, err := harness.svc.ListFactorySessions(context.Background())
	if err != nil {
		t.Fatalf("ListFactorySessions after reopen: %v", err)
	}
	assertListContainsNestedInitSession(t, listAfterReopen.Sessions, reopenAPI.Session.Id, projectDir, nestedFactoryDir)
}

func assertNestedInitSessionMetadata(t *testing.T, session *factorysessions.LiveSession, projectDir, nestedFactoryDir string) {
	t.Helper()

	if session.FolderPath != projectDir {
		t.Fatalf("session folder path = %q, want %q", session.FolderPath, projectDir)
	}
	if session.FactoryDir != nestedFactoryDir {
		t.Fatalf("session factory dir = %q, want %q", session.FactoryDir, nestedFactoryDir)
	}
	if liveSessionHandle(session).Bundle.Dir != nestedFactoryDir {
		t.Fatalf("session runtime dir = %q, want %q", liveSessionHandle(session).Bundle.Dir, nestedFactoryDir)
	}
}

func assertNestedInitAPISessionMetadata(
	t *testing.T,
	summary factoryapi.FactorySessionSummary,
	wantSessionID string,
	projectDir string,
	nestedFactoryDir string,
) {
	t.Helper()

	if summary.Id != wantSessionID {
		t.Fatalf("session summary id = %q, want %q", summary.Id, wantSessionID)
	}
	if summary.FolderPath != projectDir {
		t.Fatalf("session summary folderPath = %q, want %q", summary.FolderPath, projectDir)
	}
	if summary.FactoryDir != nestedFactoryDir {
		t.Fatalf("session summary factoryDir = %q, want %q", summary.FactoryDir, nestedFactoryDir)
	}
}

func assertListContainsNestedInitSession(
	t *testing.T,
	summaries []factoryapi.FactorySessionSummary,
	wantSessionID string,
	projectDir string,
	nestedFactoryDir string,
) {
	t.Helper()

	for _, summary := range summaries {
		if summary.Id != wantSessionID {
			continue
		}
		assertNestedInitAPISessionMetadata(t, summary, wantSessionID, projectDir, nestedFactoryDir)
		return
	}
	t.Fatalf("session list = %#v, want session %q", summaries, wantSessionID)
}

func TestFactoryService_OpenFactorySessionFromFolder_InitNewFactoryRejectsConflictingNestedFactoryDir(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	before := harness.svc.sessions.Count()
	projectDir := filepath.Join(harness.rootDir, "conflicting-nested-factory")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatalf("Mkdir(projectDir): %v", err)
	}
	rootSentinelPath := filepath.Join(projectDir, "README.md")
	rootSentinelContents := []byte("existing project notes\n")
	if err := os.WriteFile(rootSentinelPath, rootSentinelContents, 0o644); err != nil {
		t.Fatalf("WriteFile(root sentinel): %v", err)
	}

	nestedFactoryDir := filepath.Join(projectDir, interfaces.FactoryDir)
	if err := os.Mkdir(nestedFactoryDir, 0o755); err != nil {
		t.Fatalf("Mkdir(nested factory): %v", err)
	}
	nestedSentinelPath := filepath.Join(nestedFactoryDir, "notes.txt")
	nestedSentinelContents := []byte("pre-existing nested notes\n")
	if err := os.WriteFile(nestedSentinelPath, nestedSentinelContents, 0o644); err != nil {
		t.Fatalf("WriteFile(nested sentinel): %v", err)
	}
	beforeSnapshot := snapshotDirectoryTree(t, projectDir)

	_, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), projectDir, nil, false, true)
	if err == nil || !strings.Contains(err.Error(), "conflicting content") {
		t.Fatalf("OpenFactorySessionFromFolder(conflicting nested factory) error = %v, want conflict failure", err)
	}
	assertFactorySessionValidationTarget(t, err, factorysessions.ValidationReasonConflict, "folderPath")
	if got := harness.svc.sessions.Count(); got != before {
		t.Fatalf("init-new-factory conflict mutated live sessions to %d, want %d", got, before)
	}

	afterSnapshot := snapshotDirectoryTree(t, projectDir)
	if afterSnapshot != beforeSnapshot {
		t.Fatalf("directory tree changed after rejected init-new-Factory:\nbefore=%s\nafter=%s", beforeSnapshot, afterSnapshot)
	}
	preservedRoot, err := os.ReadFile(rootSentinelPath)
	if err != nil {
		t.Fatalf("ReadFile(root sentinel): %v", err)
	}
	if string(preservedRoot) != string(rootSentinelContents) {
		t.Fatalf("root sentinel contents = %q, want %q", preservedRoot, rootSentinelContents)
	}
	preservedNested, err := os.ReadFile(nestedSentinelPath)
	if err != nil {
		t.Fatalf("ReadFile(nested sentinel): %v", err)
	}
	if string(preservedNested) != string(nestedSentinelContents) {
		t.Fatalf("nested sentinel contents = %q, want %q", preservedNested, nestedSentinelContents)
	}
}

func snapshotDirectoryTree(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		line := rel + "\t" + info.Mode().String()
		if !entry.IsDir() {
			contents, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			line += "\t" + string(contents)
		}
		lines = append(lines, line)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshotDirectoryTree(%s): %v", root, err)
	}
	return strings.Join(lines, "\n")
}

func TestFactoryService_OpenFactorySessionFromFolder_InitNewFactoryRejectsRunnableFolder(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha"},
	})
	defer harness.stop(t)

	before := harness.svc.sessions.Count()
	if _, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), harness.rootDir, nil, false, true); err == nil || !strings.Contains(err.Error(), "already exposes runnable factory targets") {
		t.Fatalf("OpenFactorySessionFromFolder(init on runnable folder) error = %v, want already-runnable failure", err)
	} else {
		assertFactorySessionValidationTarget(t, err, factorysessions.ValidationReasonNotRunnable, "folderPath")
	}
	if got := harness.svc.sessions.Count(); got != before {
		t.Fatalf("init-new-factory rejection mutated live sessions to %d, want %d", got, before)
	}
}

func TestFactoryService_OpenFactorySession_InitNewFactoryMapsToAPIResponseAndListsSession(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	emptyDir := filepath.Join(harness.rootDir, "api-init")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatalf("Mkdir(api-init): %v", err)
	}

	initNewFactory := true
	response, err := harness.svc.OpenFactorySession(context.Background(), factoryapi.OpenFactorySessionRequest{
		FolderPath:     emptyDir,
		InitNewFactory: &initNewFactory,
	})
	if err != nil {
		t.Fatalf("OpenFactorySession(init new factory): %v", err)
	}
	if response.Session == nil || response.Session.Id == "" {
		t.Fatalf("response.session = %#v, want live session summary", response.Session)
	}
	if response.Session.FolderPath != emptyDir {
		t.Fatalf("response.session.folderPath = %q, want %q", response.Session.FolderPath, emptyDir)
	}
	wantFactoryDir := filepath.Join(emptyDir, interfaces.FactoryDir)
	if response.Session.FactoryDir != wantFactoryDir {
		t.Fatalf("response.session.factoryDir = %q, want %q", response.Session.FactoryDir, wantFactoryDir)
	}

	listResponse, err := harness.svc.ListFactorySessions(context.Background())
	if err != nil {
		t.Fatalf("ListFactorySessions: %v", err)
	}
	assertListContainsNestedInitSession(
		t,
		listResponse.Sessions,
		response.Session.Id,
		emptyDir,
		wantFactoryDir,
	)
}

func TestFactoryService_OpenFactorySession_InitNewFactoryRejectsValidateOnlyCombination(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	emptyDir := filepath.Join(harness.rootDir, "conflict")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatalf("Mkdir(conflict): %v", err)
	}

	validateOnly := true
	initNewFactory := true
	_, err := harness.svc.OpenFactorySession(context.Background(), factoryapi.OpenFactorySessionRequest{
		FolderPath:     emptyDir,
		ValidateOnly:   &validateOnly,
		InitNewFactory: &initNewFactory,
	})
	if err == nil || !strings.Contains(err.Error(), "initNewFactory cannot be combined with validateOnly") {
		t.Fatalf("OpenFactorySession(conflicting flags) error = %v, want mutual-exclusion failure", err)
	} else {
		assertFactorySessionValidationTarget(t, err, factorysessions.ValidationReasonRequired, "initNewFactory")
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_LogsBrokenDiscoveryTargetForValidateAndOpen(t *testing.T) {
	rootDir := t.TempDir()
	writeFactoryJSON(t, rootDir, minimalFactoryConfig())

	logCore, observedLogs := observer.New(zap.ErrorLevel)
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.New(logCore),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	brokenDir := filepath.Join(rootDir, "broken")
	if err := os.MkdirAll(brokenDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", brokenDir, err)
	}
	if err := os.WriteFile(filepath.Join(brokenDir, interfaces.FactoryConfigFile), []byte(`{"name":`), 0o644); err != nil {
		t.Fatalf("WriteFile(broken factory.json): %v", err)
	}
	resolvedBrokenDir, err := filepath.Abs(brokenDir)
	if err != nil {
		t.Fatalf("Abs(%s): %v", brokenDir, err)
	}
	resolvedBrokenDir = filepath.Clean(resolvedBrokenDir)

	validateResult, err := svc.OpenFactorySessionFromFolder(context.Background(), brokenDir, nil, true, false)
	if err == nil {
		t.Fatal("OpenFactorySessionFromFolder(validate broken) error = nil, want config-load failure")
	}
	assertFactorySessionConfigLoadFailure(t, err, "default")
	if validateResult != nil {
		t.Fatalf("validate result = %#v, want no init-new-factory fallback on broken config", validateResult)
	}

	_, err = svc.OpenFactorySessionFromFolder(context.Background(), brokenDir, nil, false, false)
	if err == nil {
		t.Fatal("OpenFactorySessionFromFolder(open broken) error = nil, want config-load failure")
	}
	assertFactorySessionConfigLoadFailure(t, err, "default")

	matchingLogs := 0
	for _, entry := range observedLogs.FilterMessage("factory session discovery target runtime config load failed").All() {
		if !observedLogFieldEquals(entry, "target_factory_dir", resolvedBrokenDir) {
			continue
		}
		matchingLogs++
		assertObservedLogFieldPresent(t, entry, "submitted_folder_path")
		assertLogField(t, entry, "target_kind", string(factorysessions.TargetKindDefault))
		assertLogField(t, entry, "target_display_name", "default")
		assertObservedLogFieldContains(t, entry, "failure_summary", "unexpected end of JSON input")
	}
	if matchingLogs != 2 {
		t.Fatalf("matching discovery failure logs = %d, want 2", matchingLogs)
	}
}

func TestFactoryService_ProbeFactorySessionTarget_DoesNotLogSuccessfulProbe(t *testing.T) {
	rootDir := t.TempDir()
	writeFactoryJSON(t, rootDir, minimalFactoryConfig())

	logCore, observedLogs := observer.New(zap.ErrorLevel)
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.New(logCore),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	target, ok, failure := svc.probeFactorySessionTarget(rootDir, rootDir, factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault})
	if !ok {
		t.Fatal("probeFactorySessionTarget(valid root) = false, want runnable target")
	}
	if failure != nil {
		t.Fatalf("probeFactorySessionTarget(valid root) failure = %#v, want nil", failure)
	}
	if target.FactoryDir == "" {
		t.Fatalf("probeFactorySessionTarget(valid root) = %#v, want populated target", target)
	}
	if got := len(observedLogs.FilterMessage("factory session discovery target runtime config load failed").All()); got != 0 {
		t.Fatalf("discovery failure logs = %d, want 0", got)
	}
}

func assertObservedLogFieldContains(t *testing.T, entry observer.LoggedEntry, key, want string) {
	t.Helper()
	for _, field := range entry.Context {
		if field.Key != key {
			continue
		}
		if strings.Contains(field.String, want) {
			return
		}
		t.Fatalf("log field %q = %q, want substring %q", key, field.String, want)
	}
	t.Fatalf("log field %q missing from %#v", key, entry.Context)
}

func observedLogFieldEquals(entry observer.LoggedEntry, key, want string) bool {
	for _, field := range entry.Context {
		if field.Key == key && field.String == want {
			return true
		}
	}
	return false
}

func assertObservedLogFieldPresent(t *testing.T, entry observer.LoggedEntry, key string) {
	t.Helper()
	for _, field := range entry.Context {
		if field.Key == key && strings.TrimSpace(field.String) != "" {
			return
		}
	}
	t.Fatalf("log field %q missing or empty in %#v", key, entry.Context)
}

func normalizeInitFactoryJSON(t *testing.T, raw string) string {
	t.Helper()
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("normalizeInitFactoryJSON unmarshal: %v", err)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("normalizeInitFactoryJSON marshal: %v", err)
	}
	return string(encoded)
}

// Session factory save tests (merged from factory_session_save*_test.go for pkg-file-count)
func TestFactoryService_SaveFactoryForSession_UpsertOnNonDefaultSessionDoesNotMutateDefault(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha", "beta"},
	})
	defer harness.stop(t)

	betaSessionID := harness.openFactorySession(t, "beta")
	harness.waitIdle(t, betaSessionID, "beta runtime")

	created, err := harness.svc.SaveFactoryForSession(
		context.Background(),
		betaSessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		serviceNamedFactoryContractWithWorkType(t, "gamma", "gamma-task"),
	)
	if err != nil {
		t.Fatalf("SaveFactoryForSession(upsert gamma on beta session): %v", err)
	}
	if created.Name != factoryapi.FactoryName("gamma") {
		t.Fatalf("created factory name = %q, want gamma", created.Name)
	}

	betaCurrent, err := harness.svc.GetCurrentFactoryForSession(context.Background(), betaSessionID)
	if err != nil {
		t.Fatalf("GetCurrentFactoryForSession(beta) after upsert: %v", err)
	}
	assertFactoryName(t, betaCurrent.Name, "gamma", "beta session current factory after upsert")
	assertFactoryWorkType(t, betaCurrent.WorkTypes, "gamma-task", "beta session work types after upsert")

	defaultCurrent, err := harness.svc.GetCurrentFactoryForSession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetCurrentFactoryForSession(default) after beta upsert: %v", err)
	}
	assertFactoryName(t, defaultCurrent.Name, "alpha", "default session current factory after beta upsert")
	assertFactoryWorkType(t, defaultCurrent.WorkTypes, "task", "default session work types after beta upsert")

	assertCurrentFactoryPointer(t, harness.rootDir, "alpha", "global default pointer after beta session upsert")
	betaSession := harness.requireSession(t, betaSessionID)
	betaPersistRoot := sessionFactoryPersistRoot(harness.svc.factoryRootDir, betaSession)
	assertCurrentFactoryPointer(t, betaPersistRoot, "gamma", "beta session pointer after upsert")
	if _, err := config.ResolveNamedFactoryDir(harness.rootDir, "gamma"); err == nil {
		t.Fatal("expected gamma factory to persist only under the beta session root, not the service root")
	}
}

func TestFactoryService_SaveFactoryForSession_UpsertReplaceRequiresFreshVersion(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha"},
	})
	defer harness.stop(t)

	created, err := harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		serviceNamedFactoryContract(t, "beta"),
	)
	if err != nil {
		t.Fatalf("SaveFactoryForSession(upsert beta): %v", err)
	}
	if created.Version == nil {
		t.Fatal("expected created factory version metadata")
	}

	stale := serviceNamedFactoryContractWithWorkType(t, "beta", "story")
	stale.Version = created.Version
	_, err = harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		stale,
	)
	if err == nil {
		t.Fatal("expected stale upsert replace to fail")
	}

	fresh := serviceNamedFactoryContractWithWorkType(t, "beta", "story")
	fresh.Version = &factoryapi.HybridLogicalTimestamp{
		Logical:  created.Version.Logical + 1,
		Physical: created.Version.Physical.Add(time.Second),
	}
	replaced, err := harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		fresh,
	)
	if err != nil {
		t.Fatalf("SaveFactoryForSession(upsert replace beta): %v", err)
	}
	assertFactoryWorkType(t, replaced.WorkTypes, "story", "replaced beta work types")
}

func TestFactoryService_SaveFactoryForSession_ReplaceCurrentRejectsMissingVersion(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha"},
	})
	defer harness.stop(t)

	current, err := harness.svc.GetCurrentFactoryForSession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetCurrentFactoryForSession: %v", err)
	}

	replacement := serviceNamedFactoryContractWithWorkType(t, "alpha", "story")
	replacement.Version = nil
	_, err = harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeReplaceCurrent,
		replacement,
	)
	if !errors.Is(err, apisurface.ErrFactoryVersionStale) {
		t.Fatalf("SaveFactoryForSession(replace missing version) error = %v, want stale version", err)
	}

	reloaded, err := harness.svc.GetCurrentFactoryForSession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetCurrentFactoryForSession after rejected save: %v", err)
	}
	assertFactoryWorkType(t, reloaded.WorkTypes, "task", "unchanged work types after missing-version reject")
	if current.Version != nil && reloaded.Version != nil {
		assertMatchingFactoryVersion(t, reloaded.Version, current.Version, "unchanged version after missing-version reject")
	}
}

func TestFactoryService_SaveFactoryForSession_UpsertCreateAllowsOmittedVersion(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha"},
	})
	defer harness.stop(t)

	// CI runners can be slower to reach idle than the default 1s harness wait.
	waitForSessionRuntimeStatus(
		t,
		harness.svc,
		defaultFactorySessionID,
		interfaces.RuntimeStatusIdle,
		5*time.Second,
		"upsert create readiness",
	)

	created, err := harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		serviceNamedFactoryContract(t, "beta"),
	)
	if err != nil {
		t.Fatalf("SaveFactoryForSession(upsert create beta): %v", err)
	}
	if created.Version == nil {
		t.Fatal("expected created factory version metadata when client omitted version")
	}
	if created.Version.Logical.Int64() < 1 {
		t.Fatalf("created version logical = %d, want >= 1", created.Version.Logical.Int64())
	}
	assertFactoryWorkType(t, created.WorkTypes, "task", "created beta work types")
}

func TestFactoryService_SaveFactoryForSession_UpsertReplaceUsesOnDiskVersion(t *testing.T) {
	rootDir := t.TempDir()
	initialVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  3,
		Physical: time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC),
	}
	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayloadWithVersion(t, "alpha", initialVersion)); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	harness := startRunningSessionServiceOnDir(t, rootDir)
	defer harness.stop(t)

	created, err := harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		serviceNamedFactoryContract(t, "beta"),
	)
	if err != nil {
		t.Fatalf("SaveFactoryForSession(upsert create beta): %v", err)
	}
	if created.Version == nil {
		t.Fatal("expected created beta version metadata")
	}

	onDiskVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  created.Version.Logical + 1,
		Physical: created.Version.Physical.Add(time.Second),
	}
	if _, err := config.ReplaceNamedFactory(rootDir, "beta", serviceNamedFactoryPayloadWithVersion(t, "beta", onDiskVersion)); err != nil {
		t.Fatalf("ReplaceNamedFactory(beta newer on-disk version): %v", err)
	}

	stale := serviceNamedFactoryContractWithWorkType(t, "beta", "story")
	stale.Version = created.Version
	_, err = harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		stale,
	)
	if !errors.Is(err, apisurface.ErrFactoryVersionStale) {
		t.Fatalf("SaveFactoryForSession(upsert replace stale) error = %v, want stale version", err)
	}

	fresh := serviceNamedFactoryContractWithWorkType(t, "beta", "story")
	fresh.Version = &factoryapi.HybridLogicalTimestamp{
		Logical:  onDiskVersion.Logical + 1,
		Physical: onDiskVersion.Physical.Add(time.Second),
	}
	replaced, err := harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		fresh,
	)
	if err != nil {
		t.Fatalf("SaveFactoryForSession(upsert replace fresh): %v", err)
	}
	assertFactoryWorkType(t, replaced.WorkTypes, "story", "replaced beta work types")
	assertFactoryVersionAdvanced(t, replaced.Version, onDiskVersion)
}

func TestFactoryService_SaveFactoryForSession_UpsertReplaceDoesNotReturnAlreadyExists(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha"},
	})
	defer harness.stop(t)

	created, err := harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		serviceNamedFactoryContract(t, "beta"),
	)
	if err != nil {
		t.Fatalf("SaveFactoryForSession(upsert create beta): %v", err)
	}
	if created.Version == nil {
		t.Fatal("expected created factory version metadata")
	}

	replacement := serviceNamedFactoryContractWithWorkType(t, "beta", "story")
	replacement.Version = &factoryapi.HybridLogicalTimestamp{
		Logical:  created.Version.Logical + 1,
		Physical: created.Version.Physical.Add(time.Second),
	}
	replaced, err := harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		replacement,
	)
	if errors.Is(err, config.ErrNamedFactoryAlreadyExists) {
		t.Fatalf("SaveFactoryForSession(upsert replace beta) error = %v, want replace not FACTORY_ALREADY_EXISTS", err)
	}
	if err != nil {
		t.Fatalf("SaveFactoryForSession(upsert replace beta): %v", err)
	}
	assertFactoryWorkType(t, replaced.WorkTypes, "story", "replaced beta work types")
	if replaced.Version == nil {
		t.Fatal("expected replaced factory version metadata")
	}
	assertFactoryVersionAdvanced(t, replaced.Version, *created.Version)
}
func TestRuntimeModelService_PullThenInvoke_UsesManagedRuntimeReadiness(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	runtime := &fakeLocalModelRuntime{
		response: workerexecution.InferenceResponse{
			Content: mustMarshalAudioContentResponse(t, audioPath),
		},
	}
	cache := localModelTestCacheLayout(t)
	puller := &managedPullMetricsAssetPuller{
		result: apisurface.ModelPullResult{
			ModelName:        "OMNIVOICE_Q4_K_M",
			ProviderLocality: workerconfig.ModelLocalityLocal,
			Outcome:          "PULLED",
			CachePath:        cache.CachePath,
			Revision:         cache.Revision,
		},
		inspection: localmodels.RuntimeCacheInspection{
			Supported:          true,
			Installed:          true,
			CachePath:          cache.CachePath,
			Revision:           cache.Revision,
			InstalledFileCount: len(cache.Files),
		},
		cache: cache,
	}

	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", localModelFactoryConfig(), localModelRuntimeWorkers(), nil)
	svc := &FactoryService{
		policy:      serviceCoordinatorPolicyFromConfig(&FactoryServiceConfig{}),
		modelAssets: puller,
		cfg:         serviceTestConfigWithWorkerApplication(t, &FactoryServiceConfig{}),
	}
	bindServiceStartupRuntime(svc, &factoryRuntimeBundle{
		RuntimeCfg:  runtimeCfg,
		ModelAssets: puller,
		LocalModels: newManagedLocalModelManager(puller, runtime),
	})
	attachModelServiceForTest(t, svc)

	pullResult, err := svc.PullModel(context.Background(), "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	if pullResult.ReadinessState != "READY" {
		t.Fatalf("pull readiness = %q, want READY", pullResult.ReadinessState)
	}

	mode := factoryapi.AUDIOSTREAM
	result, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Content: &factoryapi.WorkContent{
			mustGeneratedServiceTextPart(t, "hello after pull"),
		},
		Options: &factoryapi.ModelInvocationOptions{ResponseMode: &mode},
	})
	if err != nil {
		t.Fatalf("InvokeModel after pull: %v", err)
	}
	if result.ModelName != "OMNIVOICE_Q4_K_M" || result.StreamFile != audioPath {
		t.Fatalf("result = %#v, want OMNIVOICE audio stream at %s", result, audioPath)
	}
	if runtime.invocationCount() != 1 {
		t.Fatalf("invocation count = %d, want 1", runtime.invocationCount())
	}
}

func TestRuntimeModelService_PullModel_RecordsManagedRuntimeMetrics(t *testing.T) {
	recorder := &capturingModelPullMetricsRecorder{}
	logCore, observedLogs := observer.New(zap.InfoLevel)
	runtimeCfg, err := factoryconfig.NewLoadedFactoryConfig(t.TempDir(), &interfaces.FactoryConfig{
		Name: "factory",
		Workers: []workerconfig.Config{{
			Name:          "voice-local",
			Type:          interfaces.WorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelLocality: workerconfig.ModelLocalityLocal,
			Operations:    []workerconfig.ModelOperation{{Name: "TTS"}},
		}},
		Resources: []factoryresource.Config{{
			Name:     "omnivoice-cache",
			Type:     factoryresource.TypeModel,
			Capacity: 1,
			Model:    "OMNIVOICE_Q4_K_M",
		}},
	}, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	svc := newModelCatalogServiceForTest(runtimeCfg, &managedPullMetricsAssetPuller{
		result: apisurface.ModelPullResult{
			ModelName: "OMNIVOICE_Q4_K_M",
			Outcome:   "ALREADY_PRESENT",
			CachePath: "/tmp/cache",
			Revision:  "rev1",
		},
		inspection: localmodels.RuntimeCacheInspection{
			Supported: true,
			Installed: true,
			CachePath: "/tmp/cache",
			Revision:  "rev1",
		},
	})
	svc.logger = zap.New(logCore)
	svc.cfg = &FactoryServiceConfig{
		ModelPullMetricsRecorder: recorder,
	}
	attachModelServiceForTest(t, svc)

	if _, err := svc.PullModel(context.Background(), "OMNIVOICE_Q4_K_M"); err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	recorder.assertContainsMetric(t, modelPullMetricAttempts, map[string]string{"model_name": "OMNIVOICE_Q4_K_M"})
	recorder.assertContainsMetric(t, modelPullMetricSuccess, map[string]string{
		"model_name":      "OMNIVOICE_Q4_K_M",
		"pull_outcome":    "ALREADY_READY",
		"readiness_state": "READY",
	})
	if got := recorder.count(); got != 2 {
		t.Fatalf("model pull metric count = %d, want one attempt and one success", got)
	}
	if got := observedLogs.FilterMessage("managed runtime pull completed").Len(); got != 1 {
		t.Fatalf("managed runtime pull completion log count = %d, want 1", got)
	}
}

func TestRuntimeModelService_PullModel_RecordsSourceFailureMetric(t *testing.T) {
	recorder := &capturingModelPullMetricsRecorder{}
	logCore, observedLogs := observer.New(zap.WarnLevel)
	runtimeCfg, err := factoryconfig.NewLoadedFactoryConfig(t.TempDir(), &interfaces.FactoryConfig{
		Name: "factory",
		Workers: []workerconfig.Config{{
			Name:          "voice-local",
			Type:          interfaces.WorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelLocality: workerconfig.ModelLocalityLocal,
			Operations:    []workerconfig.ModelOperation{{Name: "TTS"}},
		}},
		Resources: []factoryresource.Config{{
			Name:     "omnivoice-cache",
			Type:     factoryresource.TypeModel,
			Capacity: 1,
			Model:    "OMNIVOICE_Q4_K_M",
		}},
	}, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	svc := newModelCatalogServiceForTest(runtimeCfg, &managedPullMetricsAssetPuller{
		err: apisurface.ErrManagedRuntimeSourceFetchFailed,
	})
	svc.logger = zap.New(logCore)
	svc.cfg = &FactoryServiceConfig{
		ModelPullMetricsRecorder: recorder,
	}
	attachModelServiceForTest(t, svc)

	_, err = svc.PullModel(context.Background(), "OMNIVOICE_Q4_K_M")
	pullErr, ok := apisurface.AsManagedRuntimePullError(err)
	if !ok {
		t.Fatalf("PullModel error = %v, want managed runtime pull failure", err)
	}
	if pullErr.Result.ManagedPullOutcome != "SOURCE_FETCH_FAILED" ||
		pullErr.Result.ReadinessState != "FAILED" {
		t.Fatalf("pull failure result = %#v, want SOURCE_FETCH_FAILED/FAILED", pullErr.Result)
	}
	if !errors.Is(err, apisurface.ErrManagedRuntimeSourceFetchFailed) {
		t.Fatalf("PullModel error = %v, want source fetch failure cause", err)
	}
	recorder.assertContainsMetric(t, modelPullMetricFailure, map[string]string{
		"model_name":   "OMNIVOICE_Q4_K_M",
		"pull_outcome": "SOURCE_FETCH_FAILED",
	})
	recorder.assertContainsMetric(t, modelPullMetricSourceFailure, map[string]string{
		"model_name": "OMNIVOICE_Q4_K_M",
	})
	if got := recorder.count(); got != 3 {
		t.Fatalf("model pull metric count = %d, want one attempt, failure, and source failure", got)
	}
	if got := observedLogs.FilterMessage("managed runtime pull failed").Len(); got != 1 {
		t.Fatalf("managed runtime pull failure log count = %d, want 1", got)
	}
}

type capturingModelPullMetricsRecorder struct {
	mu      sync.Mutex
	metrics []InvocationMetric
}

func (r *capturingModelPullMetricsRecorder) RecordModelPullMetric(metric InvocationMetric) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics = append(r.metrics, metric)
}

func (r *capturingModelPullMetricsRecorder) assertContainsMetric(t *testing.T, name string, labels map[string]string) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, metric := range r.metrics {
		if metric.Name != name {
			continue
		}
		match := true
		for key, value := range labels {
			if metric.Labels[key] != value {
				match = false
				break
			}
		}
		if match {
			return
		}
	}
	t.Fatalf("metrics %#v do not contain %q with labels %#v", r.metrics, name, labels)
}

func (r *capturingModelPullMetricsRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.metrics)
}

type managedPullMetricsAssetPuller struct {
	result     apisurface.ModelPullResult
	inspection localmodels.RuntimeCacheInspection
	cache      localmodels.CacheLayout
	err        error
}

func (p *managedPullMetricsAssetPuller) PullModel(context.Context, *factoryconfig.LoadedFactoryConfig, string) (apisurface.ModelPullResult, error) {
	return p.result, p.err
}

func (p *managedPullMetricsAssetPuller) EnsureModelAvailable(context.Context, *factoryconfig.LoadedFactoryConfig, *workerconfig.Config) error {
	return nil
}

func (p *managedPullMetricsAssetPuller) ResolveModelCache(context.Context, *factoryconfig.LoadedFactoryConfig, *workerconfig.Config) (localmodels.CacheLayout, error) {
	return p.cache, nil
}

func (p *managedPullMetricsAssetPuller) InspectRuntimeCache(context.Context, *factoryconfig.LoadedFactoryConfig, string) (localmodels.RuntimeCacheInspection, error) {
	return p.inspection, nil
}

type capturingInvocationMetricsRecorder struct {
	mu      sync.Mutex
	metrics []InvocationMetric
}

func (r *capturingInvocationMetricsRecorder) RecordInvocationMetric(metric InvocationMetric) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics = append(r.metrics, metric)
}

func TestResolveFactoryServiceRoot_AssignsLoggerAndRuntimeInstanceID(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	cfg := &FactoryServiceConfig{Dir: rootDir}

	resolved, err := ResolveFactoryServiceRoot(cfg)
	if err != nil {
		t.Fatalf("ResolveFactoryServiceRoot: %v", err)
	}
	if resolved.FactoryRootDir != rootDir {
		t.Fatalf("FactoryRootDir = %q, want %q", resolved.FactoryRootDir, rootDir)
	}
	if resolved.BaseLogger == nil || cfg.Logger == nil {
		t.Fatal("expected base logger to be assigned")
	}
	if resolved.BaseLogger != cfg.Logger {
		t.Fatal("resolved BaseLogger should match cfg.Logger")
	}
	if strings.TrimSpace(cfg.RuntimeInstanceID) == "" {
		t.Fatal("expected runtime instance id to be generated")
	}
}

func TestFactoryService_GetCurrentNamedFactory_FallsBackToLiveRuntimeWhenPointerMissing(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	writeFactoryJSON(t, rootDir, minimalFactoryConfig())

	runtimeCfg, err := config.LoadRuntimeConfig(rootDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	svc := &FactoryService{
		factoryRootDir: rootDir,
		cfg:            &FactoryServiceConfig{Dir: rootDir},
	}
	bindServiceStartupRuntime(svc, &factoryRuntimeBundle{
		Dir:        rootDir,
		FolderPath: rootDir,
		RuntimeCfg: runtimeCfg,
	})

	current, err := svc.GetCurrentNamedFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentNamedFactory: %v", err)
	}
	if current.Name != apisurface.DefaultCurrentFactoryName {
		t.Fatalf("current factory name = %q, want %q", current.Name, apisurface.DefaultCurrentFactoryName)
	}
}

func TestFactoryService_CurrentFactoryDefinitionVersionAtRoot_UsesConfigVersionOrFileModTime(t *testing.T) {
	t.Parallel()

	t.Run("named factory version from config", func(t *testing.T) {
		rootDir := t.TempDir()
		versionTime := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
		if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayloadWithVersion(t, "alpha", factoryapi.HybridLogicalTimestamp{
			Logical:  23,
			Physical: versionTime,
		})); err != nil {
			t.Fatalf("PersistNamedFactory: %v", err)
		}

		got, err := (&FactoryService{}).currentFactoryDefinitionVersionAtRoot(rootDir, "alpha")
		if err != nil {
			t.Fatalf("currentFactoryDefinitionVersionAtRoot: %v", err)
		}
		if got.Logical != 23 || !got.Physical.Equal(versionTime) {
			t.Fatalf("version = %#v, want logical=23 physical=%s", got, versionTime)
		}
	})

	t.Run("default factory version from file mod time", func(t *testing.T) {
		rootDir := t.TempDir()
		writeFactoryJSON(t, rootDir, minimalFactoryConfig())
		modTime := time.Date(2026, time.February, 3, 4, 5, 6, 0, time.UTC)
		factoryPath := filepath.Join(rootDir, interfaces.FactoryConfigFile)
		if err := os.Chtimes(factoryPath, modTime, modTime); err != nil {
			t.Fatalf("Chtimes: %v", err)
		}

		got, err := (&FactoryService{}).currentFactoryDefinitionVersionAtRoot(rootDir, apisurface.DefaultCurrentFactoryName)
		if err != nil {
			t.Fatalf("currentFactoryDefinitionVersionAtRoot: %v", err)
		}
		if got.Logical.Int64() != modTime.UnixNano() || !got.Physical.Equal(modTime) {
			t.Fatalf("version = %#v, want logical=%d physical=%s", got, modTime.UnixNano(), modTime)
		}
	})
}

func TestFactoryService_ComposeCollaboratorSnapshot_ReflectsCoreAndFactorySave(t *testing.T) {
	t.Parallel()

	core := &FactoryCore{
		collaborators: FactoryServiceCollaborators{
			Sessions:         factorysessions.NewRegistry(),
			LocalModels:      localModelDomain{Manager: &managedLocalModelManager{}},
			RuntimeBuild:     &runtimebuild.Service{},
			WorkersScheduler: workersservice.New(workersservice.Config{}),
		},
		hostedWorkers: hostedworkers.Config{Logger: zap.NewNop()},
		startupBundle: &factoryRuntimeBundle{
			ModelResources: newLocalModelResourceLimiter(),
			LocalModels:    &managedLocalModelManager{},
		},
		modelAssets: staticModelAssetPuller{},
	}

	svc := NewFactoryServiceFromCore(core)
	shell := FactoryServiceShell{Service: svc}
	svc = AttachModelServiceCollaborator(shell, serviceCompatibilityModelAPI{})
	svc.factorySave = &recordingFactorySaveSaver{}

	snapshot := svc.ComposeCollaboratorSnapshot()
	if !snapshot.SessionsInitialized || !snapshot.RuntimeBuildInitialized || !snapshot.WorkersSchedulerInitialized || !snapshot.LocalModelsInitialized {
		t.Fatalf("snapshot missing core collaborators: %+v", snapshot)
	}
	if !snapshot.ModelAssetsInitialized || !snapshot.ModelServiceInitialized || !snapshot.FactorySaveInitialized || !snapshot.DefinitionsInitialized {
		t.Fatalf("snapshot missing service collaborators: %+v", snapshot)
	}
	if !snapshot.HostedWorkersLoggerReady || !snapshot.BundleModelResources || !snapshot.BundleLocalModels {
		t.Fatalf("snapshot missing runtime bundle collaborators: %+v", snapshot)
	}
}

func TestFactoryService_RuntimeLogDiagnostics_ReportsRuntimeArtifacts(t *testing.T) {
	t.Parallel()

	logDir := t.TempDir()
	metricsDir := t.TempDir()
	logSink, err := logging.BuildRuntimeLogger(zap.NewNop(), "runtime-1", logDir, logging.RuntimeLogConfig{})
	if err != nil {
		t.Fatalf("BuildRuntimeLogger: %v", err)
	}
	defer func() {
		if closeErr := logSink.Close(); closeErr != nil {
			t.Fatalf("close log sink: %v", closeErr)
		}
	}()
	metricsSink, err := platformmetrics.BuildRuntimeMetricsSink("session-1", "runtime-1", "/tmp/folder", "/tmp/factory", metricsDir, platformmetrics.RuntimeMetricsConfig{})
	if err != nil {
		t.Fatalf("BuildRuntimeMetricsSink: %v", err)
	}
	defer func() {
		if closeErr := metricsSink.Close(); closeErr != nil {
			t.Fatalf("close metrics sink: %v", closeErr)
		}
	}()

	svc := &FactoryService{
		startupBundle: &factoryRuntimeBundle{
			LogSink:     logSink,
			MetricsSink: metricsSink,
		},
	}

	diagnostics := svc.RuntimeLogDiagnostics()
	if diagnostics.Path != logSink.Path() || diagnostics.RootDir != logSink.RootDir() {
		t.Fatalf("log diagnostics = %#v, want path %q root %q", diagnostics, logSink.Path(), logSink.RootDir())
	}
	if diagnostics.MetricsPath != metricsSink.Path() || diagnostics.MetricsRootDir != metricsSink.RootDir() {
		t.Fatalf("metrics diagnostics = %#v, want path %q root %q", diagnostics, metricsSink.Path(), metricsSink.RootDir())
	}
	if !diagnostics.StartTimeUTC.Equal(logSink.StartTimeUTC()) || !diagnostics.MetricsStartTimeUTC.Equal(metricsSink.StartTimeUTC()) {
		t.Fatalf("diagnostic start times = %#v", diagnostics)
	}
}

func TestRuntimeSessionBaseLogger_PreservesBaseLoggerWhenFileLoggingDisabled(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.InfoLevel)
	logger := runtimebuild.NewSessionLogger(runtimeSessionBaseLogger(zap.New(core), nil), "session-1", "/tmp/folder", "/tmp/factory")

	logger.Info("session logger still active")

	entries := observed.FilterMessage("session logger still active").All()
	if len(entries) != 1 {
		t.Fatalf("session logger entry count = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if got := fields["session_id"]; got != "session-1" {
		t.Fatalf("session_id = %#v, want session-1", got)
	}
	if got := fields["folder_path"]; got != "/tmp/folder" {
		t.Fatalf("folder_path = %#v, want /tmp/folder", got)
	}
	if got := fields["factory_dir"]; got != "/tmp/factory" {
		t.Fatalf("factory_dir = %#v, want /tmp/factory", got)
	}
}

func TestModelEventHelpersAndModelHostAdapters(t *testing.T) {
	t.Parallel()

	t.Run("model event diagnostics", testModelEventDiagnosticsBranches)
	t.Run("model event error classes", testModelEventErrorClassBranches)
	t.Run("model host logger adapter", testModelHostLoggerAdapterBranches)
	t.Run("model host metrics and diagnostics", testModelHostMetricsAndDiagnosticsBranches)
}

func testModelEventDiagnosticsBranches(t *testing.T) {
	t.Helper()

	successDiagnostics := &workerexecution.WorkDiagnostics{
		Provider: &workerexecution.ProviderDiagnostic{
			ResponseMetadata: map[string]string{"request_id": "req-1"},
		},
	}
	assertModelEventDiagnosticsRequestID(t, modelEventDiagnostics(successDiagnostics, nil), "req-1")

	providerDiagnostics := &workerexecution.WorkDiagnostics{
		Provider: &workerexecution.ProviderDiagnostic{
			ResponseMetadata: map[string]string{"request_id": "req-2"},
		},
	}
	providerErr := workerprovider.NewProviderError(workerexecution.WorkFailureTypeTimeout, "timeout", errors.New("boom"))
	providerErr.Diagnostics = providerDiagnostics
	assertModelEventDiagnosticsRequestID(t, modelEventDiagnostics(nil, providerErr), "req-2")
}

func assertModelEventDiagnosticsRequestID(t *testing.T, payload json.RawMessage, want string) {
	t.Helper()
	diagnostics, err := workerdiagnostics.SafeWorkDiagnosticsFromEventPayload(payload)
	if err != nil {
		t.Fatalf("decode model event diagnostics: %v", err)
	}
	if diagnostics == nil || diagnostics.Provider == nil || diagnostics.Provider.ResponseMetadata["request_id"] != want {
		t.Fatalf("model event diagnostics = %#v, want request_id %q", diagnostics, want)
	}
}

func testModelEventErrorClassBranches(t *testing.T) {
	t.Helper()

	providerErr := workerprovider.NewProviderError(workerexecution.WorkFailureTypeTimeout, "timeout", errors.New("boom"))
	readinessErr := &apisurface.ManagedRuntimeInvocationError{ReadinessState: factoryapi.ManagedRuntimeReadinessStateLOADING}
	if got := modelEventErrorClass(readinessErr); got != "MANAGED_RUNTIME_LOADING" {
		t.Fatalf("managed runtime error class = %q, want MANAGED_RUNTIME_LOADING", got)
	}
	if got := modelEventErrorClass(providerErr); got != string(workerexecution.WorkFailureTypeTimeout) {
		t.Fatalf("provider error class = %q, want %q", got, workerexecution.WorkFailureTypeTimeout)
	}
	if got := modelEventErrorClass(errors.New("plain failure")); got != "MODEL_EXECUTION_FAILED" {
		t.Fatalf("plain error class = %q, want MODEL_EXECUTION_FAILED", got)
	}
	if got := modelEventErrorClass(nil); got != "" {
		t.Fatalf("nil error class = %q, want empty string", got)
	}
}

func testModelHostLoggerAdapterBranches(t *testing.T) {
	t.Helper()

	core, observed := observer.New(zap.InfoLevel)
	hostLogger := newZapModelHostLogger(zap.New(core))
	if hostLogger == nil {
		t.Fatal("expected model host logger")
	}
	hostLogger.Info("loaded", map[string]string{"identity": "model-a"})
	hostLogger.Warn("slow", map[string]string{"state": "warming"})
	entries := observed.All()
	if len(entries) != 2 {
		t.Fatalf("entry count = %d, want 2", len(entries))
	}
	if fields := entries[0].ContextMap(); fields["identity"] != "model-a" {
		t.Fatalf("info fields = %#v, want identity=model-a", fields)
	}
	if fields := entries[1].ContextMap(); fields["state"] != "warming" {
		t.Fatalf("warn fields = %#v, want state=warming", fields)
	}
	if got := modelHostZapFields(nil); got != nil {
		t.Fatalf("modelHostZapFields(nil) = %#v, want nil", got)
	}
}

func testModelHostMetricsAndDiagnosticsBranches(t *testing.T) {
	t.Helper()

	recorder := &capturingInvocationMetricsRecorder{}
	adapter := newModelHostMetricsRecorder(recorder)
	if adapter == nil {
		t.Fatal("expected metrics recorder adapter")
	}
	adapter.RecordMetric("runtime.loaded", map[string]string{"identity": "model-a"})
	adapter.RecordMetric("   ", map[string]string{"ignored": "true"})
	if len(recorder.metrics) != 1 {
		t.Fatalf("metric count = %d, want 1", len(recorder.metrics))
	}
	if recorder.metrics[0].Name != "runtime.loaded" || recorder.metrics[0].Labels["identity"] != "model-a" {
		t.Fatalf("recorded metric = %#v", recorder.metrics[0])
	}

	diagnostics := modelHostDiagnostics(&FactoryServiceConfig{InvocationMetricsRecorder: recorder}, zap.NewNop())
	if diagnostics.Logger == nil || diagnostics.Metrics == nil {
		t.Fatalf("modelHostDiagnostics = %#v, want logger and metrics", diagnostics)
	}
}

type serviceCompatibilitySessionGateway struct {
	sessionGateway
	getFactorySession func(context.Context, string) (factorysessions.ProjectionContext, error)
}

func (f serviceCompatibilitySessionGateway) GetFactorySession(ctx context.Context, sessionID string) (factorysessions.ProjectionContext, error) {
	return f.getFactorySession(ctx, sessionID)
}

type serviceCompatibilityModelAPI struct {
	apisurface.ModelAPI
	getModel func(context.Context, string) (factoryapi.ModelDetail, error)
}

func (f serviceCompatibilityModelAPI) GetModel(ctx context.Context, modelName string) (factoryapi.ModelDetail, error) {
	return f.getModel(ctx, modelName)
}

type serviceCompatibilityFactorySave struct {
	save func(context.Context, string, factoryapi.FactorySaveMode, factoryapi.Factory) (factoryapi.Factory, error)
}

func (f serviceCompatibilityFactorySave) Save(ctx context.Context, sessionID string, mode factoryapi.FactorySaveMode, request factoryapi.Factory) (factoryapi.Factory, error) {
	return f.save(ctx, sessionID, mode, request)
}

type serviceCompatibilityInvocationAPI struct {
	invoke func(context.Context, string, sessioninvocation.InvocationRequest) (apisurface.FactoryInvocationResult, error)
}

func (f serviceCompatibilityInvocationAPI) InvokeFactorySession(ctx context.Context, sessionID string, request sessioninvocation.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
	return f.invoke(ctx, sessionID, request)
}

type serviceCompatibilityDurableExecutionAPI struct {
	apisurface.DurableSessionAPI
	startAsync func(context.Context, factoryapi.FactorySessionExecutionRequest) (factoryapi.FactorySessionExecutionResponse, error)
	pause      func(context.Context, string, factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
}

func (f serviceCompatibilityDurableExecutionAPI) PauseDurableFactorySession(ctx context.Context, sessionID string, request factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	return f.pause(ctx, sessionID, request)
}

func (f serviceCompatibilityDurableExecutionAPI) StartDurableFactorySessionAsync(ctx context.Context, request factoryapi.FactorySessionExecutionRequest) (factoryapi.FactorySessionExecutionResponse, error) {
	return f.startAsync(ctx, request)
}

func TestFactoryServiceCompatibilityFacadeForwardsToCanonicalCollaborators(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), struct{}{}, "compatibility-context")
	sentinel := errors.New("typed collaborator outcome")
	requestFactory := factoryapi.Factory{Name: "submitted"}
	calls := map[string]int{}

	service := &FactoryService{}
	service.sessionGateway = serviceCompatibilitySessionGateway{getFactorySession: func(gotCtx context.Context, sessionID string) (factorysessions.ProjectionContext, error) {
		calls["session"]++
		if gotCtx != ctx || sessionID != "missing-session" {
			t.Fatalf("session args = (%v, %q)", gotCtx, sessionID)
		}
		return factorysessions.ProjectionContext{}, sentinel
	}}
	service.modelService = serviceCompatibilityModelAPI{getModel: func(gotCtx context.Context, modelName string) (factoryapi.ModelDetail, error) {
		calls["model"]++
		if gotCtx != ctx || modelName != "missing-model" {
			t.Fatalf("model args = (%v, %q)", gotCtx, modelName)
		}
		return factoryapi.ModelDetail{}, sentinel
	}}
	service.factorySave = serviceCompatibilityFactorySave{save: func(gotCtx context.Context, sessionID string, mode factoryapi.FactorySaveMode, request factoryapi.Factory) (factoryapi.Factory, error) {
		calls["factory-definition"]++
		if gotCtx != ctx || sessionID != "session-1" || mode != factoryapi.FactorySaveModeReplaceCurrent || request.Name != requestFactory.Name {
			t.Fatalf("factory-definition args = (%v, %q, %q, %#v)", gotCtx, sessionID, mode, request)
		}
		return factoryapi.Factory{}, sentinel
	}}
	service.sessionInvoker = serviceCompatibilityInvocationAPI{invoke: func(gotCtx context.Context, sessionID string, request sessioninvocation.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
		calls["invocation"]++
		if gotCtx != ctx || sessionID != "session-1" {
			t.Fatalf("invocation args = (%v, %q, %#v)", gotCtx, sessionID, request)
		}
		return apisurface.FactoryInvocationResult{}, sentinel
	}}
	service.durableExecutionAPI = serviceCompatibilityDurableExecutionAPI{startAsync: func(gotCtx context.Context, request factoryapi.FactorySessionExecutionRequest) (factoryapi.FactorySessionExecutionResponse, error) {
		calls["durable-execution"]++
		if gotCtx != ctx {
			t.Fatalf("durable context was not preserved")
		}
		return factoryapi.FactorySessionExecutionResponse{}, sentinel
	}}

	_, sessionErr := service.GetFactorySession(ctx, "missing-session")
	_, modelErr := service.GetModel(ctx, "missing-model")
	_, definitionErr := service.SaveFactoryForSession(ctx, "session-1", factoryapi.FactorySaveModeReplaceCurrent, requestFactory)
	_, invocationErr := service.InvokeFactorySession(ctx, "session-1", factoryapi.InvocationRequest{})
	_, durableErr := service.StartDurableFactorySessionAsync(ctx, factoryapi.FactorySessionExecutionRequest{})
	for role, err := range map[string]error{"session": sessionErr, "model": modelErr, "factory-definition": definitionErr, "invocation": invocationErr, "durable-execution": durableErr} {
		if !errors.Is(err, sentinel) || calls[role] != 1 {
			t.Errorf("%s result = (%v, %d calls), want unchanged error and one call", role, err, calls[role])
		}
	}
}

func TestFactoryServiceCompatibilityFacadePreservesTypedOutcomes(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), struct{ name string }{"typed"}, "outcomes")
	notFound := errors.New("missing service session")
	validation := &apisurface.RequestValidationError{Message: "invalid factory definition"}
	wantInvocation := apisurface.FactoryInvocationResult{
		RequestID: "request-typed", TraceID: "trace-typed",
		Status: "COMPLETED",
	}
	wantLifecycle := factoryapi.FactorySessionLifecycleControlResponse{
		SessionId: "durable-1", Operation: factoryapi.FactorySessionLifecycleControlKindPause,
		Status: factoryapi.FactorySessionDurableLifecycleStatusPaused,
	}
	calls := map[string]int{}

	svc := &FactoryService{
		sessionGateway: serviceCompatibilitySessionGateway{getFactorySession: func(gotCtx context.Context, sessionID string) (factorysessions.ProjectionContext, error) {
			calls["not-found"]++
			requireServiceCompatibility(t, gotCtx == ctx && sessionID == "missing", "session args = (%v, %q)", gotCtx, sessionID)
			return factorysessions.ProjectionContext{}, errors.Join(apisurface.ErrFactorySessionNotFound, notFound)
		}},
		factorySave: serviceCompatibilityFactorySave{save: func(gotCtx context.Context, sessionID string, mode factoryapi.FactorySaveMode, request factoryapi.Factory) (factoryapi.Factory, error) {
			calls["validation"]++
			requireServiceCompatibility(t, gotCtx == ctx && sessionID == "session-1" && mode == factoryapi.FactorySaveModeReplaceCurrent, "factory-definition args = (%v, %q, %q)", gotCtx, sessionID, mode)
			return factoryapi.Factory{}, validation
		}},
		sessionInvoker: serviceCompatibilityInvocationAPI{invoke: func(gotCtx context.Context, sessionID string, request sessioninvocation.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
			calls["invocation"]++
			requireServiceCompatibility(t, gotCtx == ctx && sessionID == "session-1", "invocation args = (%v, %q)", gotCtx, sessionID)
			return wantInvocation, nil
		}},
	}
	svc.durableExecutionAPI = serviceCompatibilityDurableExecutionAPI{pause: func(gotCtx context.Context, sessionID string, request factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error) {
		calls["lifecycle"]++
		requireServiceCompatibility(t, gotCtx == ctx && sessionID == "durable-1", "lifecycle args = (%v, %q)", gotCtx, sessionID)
		return wantLifecycle, nil
	}}

	_, sessionErr := svc.GetFactorySession(ctx, "missing")
	_, validationErr := svc.SaveFactoryForSession(ctx, "session-1", factoryapi.FactorySaveModeReplaceCurrent, factoryapi.Factory{})
	invocation, invocationErr := svc.InvokeFactorySession(ctx, "session-1", factoryapi.InvocationRequest{})
	lifecycle, lifecycleErr := svc.PauseDurableFactorySession(ctx, "durable-1", factoryapi.FactorySessionLifecycleControlRequest{})
	requireServiceCompatibility(t, errors.Is(sessionErr, apisurface.ErrFactorySessionNotFound) && errors.Is(sessionErr, notFound), "session error = %v, want typed not-found", sessionErr)
	var gotValidation *apisurface.RequestValidationError
	requireServiceCompatibility(t, errors.As(validationErr, &gotValidation) && gotValidation == validation, "validation error = %#v, want unchanged %#v", validationErr, validation)
	requireServiceCompatibility(t, invocationErr == nil && reflect.DeepEqual(invocation, wantInvocation), "invocation = (%#v, %v), want %#v", invocation, invocationErr, wantInvocation)
	requireServiceCompatibility(t, lifecycleErr == nil && reflect.DeepEqual(lifecycle, wantLifecycle), "lifecycle = (%#v, %v), want %#v", lifecycle, lifecycleErr, wantLifecycle)
	for outcome, count := range calls {
		requireServiceCompatibility(t, count == 1, "%s calls = %d, want 1", outcome, count)
	}
}

func requireServiceCompatibility(t *testing.T, condition bool, format string, args ...any) {
	t.Helper()
	if !condition {
		t.Fatalf(format, args...)
	}
}

var _ apisurface.APISurface = (*FactoryService)(nil)
var _ apisurface.SessionAPISurface = (*FactoryService)(nil)
