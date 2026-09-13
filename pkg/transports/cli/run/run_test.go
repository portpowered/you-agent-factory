package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/transports/cli/batchload"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"

	"github.com/portpowered/infinite-you/internal/testutil"
	factorydefinitionfixtures "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/platform/metrics"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingscli "github.com/portpowered/infinite-you/pkg/services/recordings/transports/cli"
)

func TestEmitHistoricalReplayInspectionIncludesLegacyWorkerHistoryOutcome(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	err := emitHistoricalReplayInspection(&output, factorysessions.HistoricalReplayInspection{
		Session: factorysessions.SessionReadResult{
			SessionID: "legacy-session", Status: factorysessions.LifecycleStatusFailed,
			ResolvedSource: factorysessions.ResolvedSource{SourceRef: "workflow/legacy.js"},
		},
		WorkerHistory: recordings.PortableRecordingWorkerHistory{
			Availability: recordings.PortableRecordingWorkerHistoryUnavailable,
			Reason:       recordings.PortableRecordingWorkerHistoryReasonLegacySchema,
		},
	})
	if err != nil {
		t.Fatalf("emitHistoricalReplayInspection() error = %v", err)
	}
	want := "Worker history: UNAVAILABLE (reason=SCHEMA_DID_NOT_RECORD_CANONICAL_WORKER_HISTORY)"
	if !strings.Contains(output.String(), want) {
		t.Fatalf("historical replay output = %q, want %q", output.String(), want)
	}
}

func TestOperationRunDisclosesReplayHomeBeforeInspection(t *testing.T) {
	var output bytes.Buffer
	operation := &Operation{
		cfg: RunConfig{
			HomeDir:       "operator-home",
			Output:        &output,
			StartupOutput: &output,
		},
		runner: stubFactoryService{run: func(context.Context) error { return nil }},
		historicalReplay: &factorysessions.HistoricalReplayInspection{
			Session: factorysessions.SessionReadResult{SessionID: "replay-session"},
		},
	}

	if err := operation.Run(context.Background()); err != nil {
		t.Fatalf("Operation.Run() error = %v, want successful replay", err)
	}
	homeIndex := strings.Index(output.String(), "Home directory: operator-home\n")
	inspectionIndex := strings.Index(output.String(), "Replayed Factory Session: replay-session\n")
	if homeIndex < 0 || inspectionIndex < 0 || homeIndex > inspectionIndex {
		t.Fatalf("replay output ordering is wrong:\n%s", output.String())
	}
}

type stubFactoryService struct {
	runtimehost.Service
	run                   func(context.Context) error
	snapshot              func(context.Context) (*interfaces.EngineStateSnapshot[runtimehost.PetriMarkingSnapshot, *runtimehost.Net], error)
	cleanInvocation       runtimehost.CleanInvocationSnapshot
	runtimeLogDiagnostics runtimehost.RuntimeLogDiagnostics
}

func (s stubFactoryService) Run(ctx context.Context) error {
	return s.run(ctx)
}

func (stubFactoryService) ControlWaitToComplete(runtimehost.WaitToCompleteRequest) runtimehost.WaitToCompleteResult {
	done := make(chan struct{})
	close(done)
	return runtimehost.WaitToCompleteResult{Done: done}
}

func (s stubFactoryService) RuntimeLogDiagnostics() runtimehost.RuntimeLogDiagnostics {
	return s.runtimeLogDiagnostics
}

func (s stubFactoryService) GetEngineStateSnapshot(ctx context.Context) (*interfaces.EngineStateSnapshot[runtimehost.PetriMarkingSnapshot, *runtimehost.Net], error) {
	if s.snapshot == nil {
		return nil, errors.New("snapshot unavailable")
	}
	return s.snapshot(ctx)
}

func (s stubFactoryService) CleanInvocationSnapshot(ctx context.Context) (runtimehost.CleanInvocationSnapshot, error) {
	return s.cleanInvocation, nil
}

func (s stubFactoryService) RuntimeObservation(ctx context.Context) (factoryvisualization.RuntimeObservation, error) {
	snapshot, err := s.GetEngineStateSnapshot(ctx)
	if err != nil {
		return factoryvisualization.RuntimeObservation{}, err
	}
	if snapshot == nil {
		return factoryvisualization.RuntimeObservation{}, nil
	}
	return factoryvisualization.RuntimeObservation{
		TickCount:     snapshot.TickCount,
		FactoryState:  snapshot.FactoryState,
		RuntimeStatus: snapshot.RuntimeStatus,
		Uptime:        snapshot.Uptime,
	}, nil
}

func buildTransportTestRuntime(
	_ context.Context,
	_ *testRuntimeSelections,
	_ serviceedges.Edges,
) (RuntimeRunner, error) {
	snapshot := completedTransportTestSnapshot()
	return stubFactoryService{
		run: func(context.Context) error { return nil },
		cleanInvocation: runtimehost.CleanInvocationSnapshot{Work: []runtimehost.CleanInvocationWork{{
			WorkID: "dashboard-render-test-work", Name: "dashboard-render-test-work", WorkTypeID: "task",
			State: "done", StateCategory: string(runtimehost.StateCategoryTerminal),
			Output: "mock worker accepted", TraceID: "dashboard-render-test-trace",
		}}},
		snapshot: func(context.Context) (*interfaces.EngineStateSnapshot[runtimehost.PetriMarkingSnapshot, *runtimehost.Net], error) {
			return snapshot, nil
		},
	}, nil
}

func completedTransportTestSnapshot() *interfaces.EngineStateSnapshot[runtimehost.PetriMarkingSnapshot, *runtimehost.Net] {
	var topology runtimehost.Net
	if err := json.Unmarshal([]byte(`{
		"id":"transport-test",
		"places":{"task:done":{"id":"task:done","type_id":"task","state":"done"}},
		"transitions":{},"arcs":{},
		"work_types":{"task":{"id":"task","name":"task","states":[{"value":"done","category":"TERMINAL"}]}},
		"resources":{},"limits":{}
	}`), &topology); err != nil {
		panic(err)
	}
	token := &runtimehost.RuntimeToken{
		ID:      "dashboard-render-test-work",
		PlaceID: "task:done",
		Color: runtimehost.RuntimeTokenColor{
			WorkID:     "dashboard-render-test-work",
			WorkTypeID: "task",
			TraceID:    "dashboard-render-test-trace",
			Payload:    []byte("mock worker accepted"),
		},
	}
	return &interfaces.EngineStateSnapshot[runtimehost.PetriMarkingSnapshot, *runtimehost.Net]{
		Topology: &topology,
		Marking: runtimehost.PetriMarkingSnapshot{
			Tokens:      map[string]*runtimehost.RuntimeToken{token.ID: token},
			PlaceTokens: map[string][]string{token.PlaceID: {token.ID}},
		},
	}
}

func preserveRunGlobals(t *testing.T) {
	t.Helper()

	originalBuilder := openTestRuntimeRunner
	originalInvocationBootstrap := openTestInvocationRunner
	if openTestInvocationRunner == nil {
		openTestInvocationRunner = func(context.Context, *testRuntimeSelections, serviceedges.Edges) (InvocationRunner, error) {
			return &capturingBootstrapRunner{}, nil
		}
	}
	t.Cleanup(func() {
		openTestRuntimeRunner = originalBuilder
		openTestInvocationRunner = originalInvocationBootstrap
	})
}

func setUserHomeForTest(t *testing.T, homeDir string) {
	t.Helper()

	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv("HOMEDRIVE", filepath.VolumeName(homeDir))
	t.Setenv("HOMEPATH", string(os.PathSeparator))
}

func TestCountTokenStates(t *testing.T) {
	tests := []struct {
		name     string
		tokens   map[string]*runtimehost.RuntimeToken
		wantWIP  int
		wantDone int
		wantFail int
	}{
		{name: "empty marking", tokens: map[string]*runtimehost.RuntimeToken{}},
		{
			name: "mixed states",
			tokens: map[string]*runtimehost.RuntimeToken{
				"t1": {ID: "t1", PlaceID: "task:todo"},
				"t2": {ID: "t2", PlaceID: "task:in-progress"},
				"t3": {ID: "t3", PlaceID: "task:completed"},
				"t4": {ID: "t4", PlaceID: "task:completed"},
				"t5": {ID: "t5", PlaceID: "task:failed"},
			},
			wantWIP:  2,
			wantDone: 2,
			wantFail: 1,
		},
		{
			name: "all completed",
			tokens: map[string]*runtimehost.RuntimeToken{
				"t1": {ID: "t1", PlaceID: "page:completed"},
				"t2": {ID: "t2", PlaceID: "page:completed"},
			},
			wantDone: 2,
		},
		{
			name: "all failed",
			tokens: map[string]*runtimehost.RuntimeToken{
				"t1": {ID: "t1", PlaceID: "task:failed"},
				"t2": {ID: "t2", PlaceID: "task:failed"},
				"t3": {ID: "t3", PlaceID: "task:failed"},
			},
			wantFail: 3,
		},
		{
			name: "work type prefix stays local to suffix classification",
			tokens: map[string]*runtimehost.RuntimeToken{
				"t1": {ID: "t1", PlaceID: "story:phase:completed"},
				"t2": {ID: "t2", PlaceID: "story:phase:failed"},
				"t3": {ID: "t3", PlaceID: "story:phase:queued"},
			},
			wantWIP:  1,
			wantDone: 1,
			wantFail: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap := &runtimehost.PetriMarkingSnapshot{Tokens: tt.tokens}
			wip, done, failed := CountTokenStates(snap)
			if wip != tt.wantWIP {
				t.Errorf("wip = %d, want %d", wip, tt.wantWIP)
			}
			if done != tt.wantDone {
				t.Errorf("done = %d, want %d", done, tt.wantDone)
			}
			if failed != tt.wantFail {
				t.Errorf("failed = %d, want %d", failed, tt.wantFail)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{90 * time.Minute, "1h30m"},
		{2*time.Hour + 15*time.Minute, "2h15m"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := FormatDuration(tt.d)
			if got != tt.want {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestDocsExampleStartupWorkFile(t *testing.T) {
	path := testutil.MustRepoPath(t, "docs/examples/startup-work.json")

	got, err := batchload.LoadFromFile(func(gotPath string) (work.WorkRequest, error) {
		if gotPath != path {
			t.Fatalf("path = %q, want %q", gotPath, path)
		}
		return work.WorkRequest{
			RequestID: "docs-example-story-001", Type: work.WorkRequestTypeFactoryRequestBatch,
			Works: []work.Work{{WorkTypeID: "story", State: "init", Payload: map[string]any{"story": "fixture"}}},
		}, nil
	}, path)
	if err != nil {
		t.Fatalf("LoadFromFile(%q): %v", path, err)
	}
	if got.RequestID != "docs-example-story-001" {
		t.Fatalf("request ID = %q, want docs-example-story-001", got.RequestID)
	}
	if got.Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("type = %q, want %q", got.Type, work.WorkRequestTypeFactoryRequestBatch)
	}
	if len(got.Works) != 1 {
		t.Fatalf("work count = %d, want 1", len(got.Works))
	}

	work := got.Works[0]
	if work.WorkTypeID != "story" {
		t.Fatalf("work type = %q, want story", work.WorkTypeID)
	}
	if work.State != "init" {
		t.Fatalf("state = %q, want init", work.State)
	}
	if work.Payload == nil {
		t.Fatal("payload is empty")
	}
}

func TestBootstrapFactory_UsesCurrentNamedFactoryPointerLayout(t *testing.T) {
	rootDir := t.TempDir()

	payload, err := json.Marshal(map[string]any{
		"name": "alpha-factory",
		"id":   "alpha",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"workers": []map[string]any{{
			"name": "executor",
			"type": "MODEL_WORKER",
			"body": "You are the executor.",
		}},
		"workstations": []map[string]any{{
			"name":    "execute-alpha",
			"worker":  "executor",
			"inputs":  []map[string]string{{"workType": "task", "state": "init"}},
			"outputs": []map[string]string{{"workType": "task", "state": "complete"}},
			"type":    "MODEL_WORKSTATION",
			"body":    "Implement {{ .WorkID }}.",
		}},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if _, err := factorydefinitionfixtures.SeedNamedFactoryUnchecked(filepath.Join(rootDir, "alpha"), payload); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(rootDir, interfaces.CurrentFactoryPointerFile),
		[]byte("alpha\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer: %v", err)
	}

	if err := bootstrapFactoryWithInitializer(rootDir, nil, func(string) (string, error) {
		return filepath.Join(rootDir, "alpha"), nil
	}, platformfilesystem.Local{}); err != nil {
		t.Fatalf("bootstrapFactory: %v", err)
	}

	inputDir := filepath.Join(rootDir, "alpha", interfaces.InputsDir, interfaces.DefaultFactoryInputType, interfaces.DefaultChannelName)
	if _, err := os.Stat(inputDir); err != nil {
		t.Fatalf("expected bootstrap to prepare current named-factory input dir %s: %v", inputDir, err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, interfaces.FactoryConfigFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected bootstrap to avoid creating legacy root factory.json, got err=%v", err)
	}
}

func TestRun_DefaultModeUsesBatchRuntimeAndExitsWhenRunReturns(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	var capturedMode interfaces.RuntimeMode
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedMode = cfg.RuntimeMode
		return stubFactoryService{run: func(context.Context) error { return nil }}, nil
	}

	if err := Run(context.Background(), RunConfig{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedMode != interfaces.RuntimeModeBatch {
		t.Fatalf("runtime mode = %q, want %q", capturedMode, interfaces.RuntimeModeBatch)
	}
}

type recordOrReplayPathCase struct {
	name              string
	cfg               RunConfig
	wantDefaultRecord bool
	wantRecordPath    string
	wantReplayPath    string
	wantResumePath    string
}

func TestRun_RecordOrReplayPathPassedToServiceConfig(t *testing.T) {
	for _, tt := range recordOrReplayPathCases() {
		t.Run(tt.name, func(t *testing.T) {
			runRecordOrReplayPathCase(t, tt)
		})
	}
}

func recordOrReplayPathCases() []recordOrReplayPathCase {
	return []recordOrReplayPathCase{
		{
			name:              "default live mode",
			cfg:               RunConfig{},
			wantDefaultRecord: true,
		},
		{
			name:           "record mode",
			cfg:            RunConfig{RecordPath: "run.replay.json"},
			wantRecordPath: "run.replay.json",
		},
		{
			name: "record mode with one-shot opt-out rejects conflicting flags",
			cfg: RunConfig{
				RecordPath:              "run.replay.json",
				DisableDefaultRecording: true,
			},
		},
		{
			name:           "replay mode",
			cfg:            RunConfig{ReplayPath: "existing.replay.json"},
			wantReplayPath: "existing.replay.json",
		},
		{
			name:              "resume mode",
			cfg:               RunConfig{ResumePath: "existing.recording.json"},
			wantDefaultRecord: true,
			wantResumePath:    "existing.recording.json",
		},
		{
			name:           "resume mode with explicit successor",
			cfg:            RunConfig{ResumePath: "existing.recording.json", RecordPath: "successor.recording.json"},
			wantRecordPath: "successor.recording.json",
			wantResumePath: "existing.recording.json",
		},
		{
			name: "default recording disabled for one run",
			cfg: RunConfig{
				DisableDefaultRecording: true,
			},
		},
	}
}

func runRecordOrReplayPathCase(t *testing.T, tt recordOrReplayPathCase) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	tt.cfg.HomeDir = t.TempDir()
	plannedPath := filepath.Join(tt.cfg.HomeDir, "planned-recording.json")
	tt.cfg.RecordingTargetPlanner = recordings.LiveRecordingTargetPlannerFunc(func(request recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
		if request.HomeDir != tt.cfg.HomeDir || request.ReportedSessionID != defaultFactorySessionID {
			t.Fatalf("recording request = %#v", request)
		}
		if _, err := uuid.Parse(request.CanonicalSessionID); err != nil {
			t.Fatalf("canonical session ID = %q, want UUID: %v", request.CanonicalSessionID, err)
		}
		return recordings.LiveRecordingTarget{ServicePath: plannedPath, ReportedPath: plannedPath}, nil
	})

	var capturedRecordPath string
	var capturedReplayPath string
	var capturedResumePath string
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedRecordPath = cfg.RecordPath
		capturedReplayPath = cfg.ReplayPath
		capturedResumePath = cfg.ResumePath
		return stubFactoryService{run: func(context.Context) error { return nil }}, nil
	}

	err := Run(context.Background(), tt.cfg)
	if tt.cfg.DisableDefaultRecording && tt.cfg.RecordPath != "" {
		if err == nil {
			t.Fatal("expected conflicting --record and --no-record settings to fail")
		}
		if !strings.Contains(err.Error(), "--no-record cannot be used with --record") {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if tt.wantDefaultRecord {
		if capturedRecordPath != plannedPath {
			t.Fatalf("record path = %q, want injected planned path %q", capturedRecordPath, plannedPath)
		}
	} else if capturedRecordPath != tt.wantRecordPath {
		t.Fatalf("record path = %q, want %q", capturedRecordPath, tt.wantRecordPath)
	}
	if capturedReplayPath != tt.wantReplayPath {
		t.Fatalf("replay path = %q, want %q", capturedReplayPath, tt.wantReplayPath)
	}
	if capturedResumePath != tt.wantResumePath {
		t.Fatalf("resume path = %q, want %q", capturedResumePath, tt.wantResumePath)
	}
}

func TestRun_DefaultRecordPathResolutionErrorSkipsServiceStart(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	builderCalled := false
	openTestRuntimeRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		builderCalled = true
		return stubFactoryService{run: func(context.Context) error { return nil }}, nil
	}
	err := runWithTestRuntimeRunner(context.Background(), RunConfig{
		HomeDir: "home",
		RecordingTargetPlanner: recordings.LiveRecordingTargetPlannerFunc(func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
			return recordings.LiveRecordingTarget{}, errors.New("target planning failed")
		}),
	}, nil)
	if err == nil {
		t.Fatal("expected default record path resolution to fail")
	}
	if !strings.Contains(err.Error(), "resolve default replay record path") {
		t.Fatalf("unexpected error: %v", err)
	}
	if builderCalled {
		t.Fatal("factory service builder should not run when default record path resolution fails")
	}
}

func TestResolveRecordPathForRunDelegatesToInjectedRecordingsCLIAdapter(t *testing.T) {
	t.Parallel()

	called := false
	adapter := stubRecordingsCLIAdapter{resolve: func(request recordingscli.InvocationRequest) (recordingscli.ResolvedRecordPath, error) {
		called = true
		if request.RecordPath != "explicit.replay.json" {
			t.Fatalf("request = %#v", request)
		}
		return recordingscli.ResolvedRecordPath{ServicePath: "explicit.replay.json"}, nil
	}}

	resolved, err := resolveRecordPathForRun(RunConfig{
		RecordPath:    "explicit.replay.json",
		RecordingsCLI: adapter,
	})
	if err != nil {
		t.Fatalf("resolveRecordPathForRun() error = %v", err)
	}
	if !called {
		t.Fatal("expected injected Recordings CLI adapter to resolve the record path")
	}
	if resolved.servicePath != "explicit.replay.json" {
		t.Fatalf("service path = %q, want explicit.replay.json", resolved.servicePath)
	}
}

type stubRecordingsCLIAdapter struct {
	resolve func(recordingscli.InvocationRequest) (recordingscli.ResolvedRecordPath, error)
}

func (adapter stubRecordingsCLIAdapter) ResolveRecordPath(
	request recordingscli.InvocationRequest,
) (recordingscli.ResolvedRecordPath, error) {
	return adapter.resolve(request)
}

func (adapter stubRecordingsCLIAdapter) ResolveRecordPathWithContext(
	ctx context.Context,
	request recordingscli.InvocationRequest,
) (recordingscli.ResolvedRecordPath, error) {
	return recordingscli.New().ResolveRecordPathWithContext(ctx, request)
}

func (adapter stubRecordingsCLIAdapter) ReportRecordingPathOnShutdown(
	output io.Writer,
	resolved recordingscli.ResolvedRecordPath,
) {
	recordingscli.New().ReportRecordingPathOnShutdown(output, resolved)
}

func (adapter stubRecordingsCLIAdapter) RecordingDiagnosticsLabel(
	resolved recordingscli.ResolvedRecordPath,
	replayPath string,
) string {
	return recordingscli.New().RecordingDiagnosticsLabel(resolved, replayPath)
}

func TestResolveRecordPathForRunRequiresInjectedRecordingsCLIAdapter(t *testing.T) {
	t.Parallel()

	_, err := resolveRecordPathForRun(RunConfig{HomeDir: "home"})
	if err == nil || err.Error() != "Recordings CLI adapter is required" {
		t.Fatalf("resolveRecordPathForRun() error = %v, want required adapter", err)
	}
}

func TestResolveRecordPathForRunRequiresInjectedRecordingPlanner(t *testing.T) {
	t.Parallel()

	_, err := resolveRecordPathForRun(RunConfig{
		HomeDir:       "home",
		RecordingsCLI: recordingscli.New(),
	})
	if err == nil || err.Error() != "Recordings live recording target planner is required" {
		t.Fatalf("resolveRecordPathForRun() error = %v, want required planner", err)
	}
}

func TestOpenSequentialHomesControlDefaultRecordingPath(t *testing.T) {
	ambientHome := t.TempDir()
	setUserHomeForTest(t, ambientHome)

	for _, homeDir := range []string{t.TempDir(), t.TempDir()} {
		plannedPath := filepath.Join(homeDir, "recordings", "planned.json")
		var plannedRequest recordings.LiveRecordingTargetRequest
		var gotRecordPath string
		var gotSystemHome string
		var gotLogDir string
		var gotMetricsDir string
		factory := testRunnerOpeners{runtime: func(
			_ context.Context,
			cfg *testRuntimeSelections,
			_ serviceedges.Edges,
		) (RuntimeRunner, error) {
			gotRecordPath = cfg.RecordPath
			gotSystemHome = cfg.SystemConfigHomeDir
			gotLogDir = cfg.RuntimeLogDir
			gotMetricsDir = cfg.RuntimeMetricsDir
			return stubFactoryService{run: func(context.Context) error { return nil }}, nil
		}}
		operation, err := Open(context.Background(), ensureTestRecordingsCLI(RunConfig{
			Dir: t.TempDir(), HomeDir: homeDir, Port: 0, SuppressDashboardRendering: true,
			StdinIsTTY: func() bool { return true },
			RecordingTargetPlanner: recordings.LiveRecordingTargetPlannerFunc(func(request recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
				plannedRequest = request
				return recordings.LiveRecordingTarget{ServicePath: plannedPath, ReportedPath: plannedPath}, nil
			}),
		}), factory.BuildRunner, factory.Invocation(), testResponsePresentation(), nil, testMockWorkersConfigLoader, testRuntimeOpeningRequestFactory)
		if err != nil {
			t.Fatalf("Open(home %q) error = %v", homeDir, err)
		}
		if plannedRequest.HomeDir != homeDir || gotRecordPath != plannedPath {
			t.Fatalf("planner request/path = %#v / %q, want home %q / %q", plannedRequest, gotRecordPath, homeDir, plannedPath)
		}
		if gotSystemHome != homeDir || gotLogDir != logging.RuntimeLogsRoot(homeDir) || gotMetricsDir != metrics.RuntimeMetricsRoot(homeDir) {
			t.Fatalf("service home paths = home %q logs %q metrics %q; want roots below %q", gotSystemHome, gotLogDir, gotMetricsDir, homeDir)
		}
		if err := operation.Run(context.Background()); err != nil {
			t.Fatalf("Operation.Run(home %q) error = %v", homeDir, err)
		}
	}
}

func TestRun_WithBootstrapCallsBootstrapFactory(t *testing.T) {
	originalBuilder := openTestRuntimeRunner
	defer func() {
		openTestRuntimeRunner = originalBuilder
	}()

	dir := t.TempDir()
	var gotBootstrapDir string
	resolve := func(inDir string) (string, error) {
		gotBootstrapDir = inDir
		return dir, nil
	}

	var capturedMode interfaces.RuntimeMode
	openTestRuntimeRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (factoryServiceRunner, error) {
		capturedMode = cfg.RuntimeMode
		return stubFactoryService{run: func(context.Context) error { return nil }}, nil
	}

	if err := Run(context.Background(), RunConfig{
		Bootstrap: true, Dir: dir,
		ResolveCurrentFactoryDir: resolve,
		DirectoryCreator:         platformfilesystem.Local{},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gotBootstrapDir != dir {
		t.Fatalf("bootstrap dir = %q, want %q", gotBootstrapDir, dir)
	}
	if capturedMode != interfaces.RuntimeModeBatch {
		t.Fatalf("runtime mode = %q, want %q", capturedMode, interfaces.RuntimeModeBatch)
	}
}

func TestRun_StartupOutputReportsDashboardAndOpensBrowser(t *testing.T) {
	var openedURL string
	opened := make(chan struct{})
	var out bytes.Buffer
	cfg := RunConfig{
		Dir:           "factory",
		Port:          7437,
		Bootstrap:     true,
		Continuously:  true,
		OpenDashboard: true,
		OutputIsTTY:   true,
		StartupOutput: &out,
	}
	if !emitStartupMessages(cfg, runtimehost.RuntimeLogDiagnostics{}) {
		t.Fatal("startup messages did not request dashboard open")
	}
	openDashboardAtBoundEndpoint(
		context.Background(), cfg,
		func(_ context.Context, url string) error {
			openedURL = url
			close(opened)
			return nil
		},
	)
	select {
	case <-opened:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for dashboard opener")
	}
	wantURL := "http://localhost:7437/dashboard/ui"
	if openedURL != wantURL {
		t.Fatalf("opened URL = %q, want %q", openedURL, wantURL)
	}
	output := out.String()
	for _, want := range []string{
		"Factory initiated: factory",
		"Factory directory ready: factory",
		"Runtime mode: continuous",
		"Dashboard URL: " + wantURL,
		"Opening dashboard: " + wantURL,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("startup output = %q, want %q", output, want)
		}
	}
}

func TestIsTerminalState(t *testing.T) {
	for _, s := range []string{"completed"} {
		if !isTerminalState(s) {
			t.Errorf("expected %q to be terminal", s)
		}
	}
	if isTerminalState("in-progress") {
		t.Error("in-progress should not be terminal")
	}
}

func TestIsFailedState(t *testing.T) {
	for _, s := range []string{"failed"} {
		if !isFailedState(s) {
			t.Errorf("expected %q to be failed", s)
		}
	}
	if isFailedState("done") {
		t.Error("done should not be failed")
	}
}

func assertStableSourceConflictError(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("expected stable source conflict error")
	}
	for _, want := range []string{
		string(work.InputErrorCodeSourceConflict),
		string(work.InputSourcePositionalText),
		string(work.InputSourceStdinText),
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	}
}

func stringPtr(value string) *string {
	return &value
}

func preparedTextInvocationInput(source work.InputSourceLabel, text string) work.PreparedInvocationInput {
	resolved := &work.ResolvedInput{Source: source, Text: text, Content: []work.WorkContentPart{{
		Type: work.WorkContentPartTypeText, Text: text,
	}}}
	return work.PreparedInvocationInput{Source: source, ResolvedInput: resolved}
}

func preparedTextInvocationInputPtr(source work.InputSourceLabel, text string) *work.PreparedInvocationInput {
	prepared := preparedTextInvocationInput(source, text)
	return &prepared
}

func scriptedInvocationConflictError() error {
	return MapInvocationInputError(&work.InputError{
		Code: work.InputErrorCodeSourceConflict, Message: "invocation input sources conflict: positional_text, stdin_text",
		ConflictingSources: []work.InputSourceLabel{work.InputSourcePositionalText, work.InputSourceStdinText},
	})
}

func assertInvocationRequestMatchesSharedResolver(
	t *testing.T,
	request *factoryapi.InvocationRequest,
	source work.InputSourceLabel,
	text string,
) {
	t.Helper()

	if request == nil {
		t.Fatal("invocation request = nil")
	}
	if request.SourceKind == nil || *request.SourceKind != factoryapi.InvocationInputSourceKindText {
		t.Fatalf("sourceKind = %v, want text", request.SourceKind)
	}

	switch source {
	case work.InputSourcePositionalText, work.InputSourceStdinText:
	default:
		t.Fatalf("unsupported source label %q", source)
	}
	if got := extractInvocationText(t, request); got != text {
		t.Fatalf("invocation text = %q, want %q", got, text)
	}
}

func assertStableInvocationSourceConflictMessage(t *testing.T, got string, wantMessage string) {
	t.Helper()

	for _, fragment := range []string{
		string(work.InputSourcePositionalText),
		string(work.InputSourceStdinText),
		wantMessage,
	} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("error = %q, want fragment %q", got, fragment)
		}
	}
}

func invocationRequestFromLogicalAPIText(text string) (*factoryapi.InvocationRequest, error) {
	sourceKind := factoryapi.InvocationInputSourceKindText
	content := contentcontract.GeneratedPtrFromParts([]work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: text}})
	return &factoryapi.InvocationRequest{SourceKind: &sourceKind, Content: content}, nil
}

func assertEquivalentInvocationRequests(
	t *testing.T,
	cliRequest *factoryapi.InvocationRequest,
	apiRequest *factoryapi.InvocationRequest,
) {
	t.Helper()

	if cliRequest == nil || apiRequest == nil {
		t.Fatal("invocation request = nil")
	}
	if cliRequest.SourceKind == nil || apiRequest.SourceKind == nil || *cliRequest.SourceKind != *apiRequest.SourceKind {
		t.Fatalf("sourceKind = %v, want %v", cliRequest.SourceKind, apiRequest.SourceKind)
	}
	if got := extractInvocationText(t, cliRequest); got != extractInvocationText(t, apiRequest) {
		t.Fatalf("invocation text = %q, want %q", got, extractInvocationText(t, apiRequest))
	}
}

func assertInvocationResponseMatchesFactoryResult(
	t *testing.T,
	response factoryapi.InvocationResponse,
	result apisurface.FactoryInvocationResult,
) {
	t.Helper()

	if response.RequestId != result.RequestID {
		t.Fatalf("requestId = %q, want %q", response.RequestId, result.RequestID)
	}
	if response.TraceId != result.TraceID {
		t.Fatalf("traceId = %q, want %q", response.TraceId, result.TraceID)
	}
	if response.Status != factoryapi.InvocationTerminalStatus(result.Status) {
		t.Fatalf("status = %q, want %q", response.Status, result.Status)
	}
	if len(result.PrimaryResult) == 0 {
		if response.PrimaryResult != nil {
			t.Fatalf("primary result = %#v, want none", response.PrimaryResult)
		}
	} else {
		assertGeneratedWorkContentPartsFromResponse(t, response.PrimaryResult, result.PrimaryResult)
	}
	assertOptionalStringPointerEquals(t, "errorCode", response.ErrorCode, result.ErrorCode)
	assertOptionalStringPointerEquals(t, "message", response.Message, result.Message)
	assertOptionalStringPointerEquals(t, "sessionId", response.SessionId, result.SessionID)
	assertOptionalStringPointerEquals(t, "workId", response.WorkId, result.WorkID)
	assertOptionalStringPointerEquals(t, "workName", response.WorkName, result.WorkName)
	assertOptionalStringPointerEquals(t, "workState", response.WorkState, result.WorkState)
}

func assertOptionalStringPointerEquals[T ~string](t *testing.T, field string, got *T, want string) {
	t.Helper()

	if want == "" {
		if got != nil {
			t.Fatalf("%s = %#v, want nil", field, *got)
		}
		return
	}
	if got == nil {
		t.Fatalf("%s = nil, want %q", field, want)
	}
	if string(*got) != want {
		t.Fatalf("%s = %q, want %q", field, string(*got), want)
	}
}

func assertGeneratedWorkContentPartsFromResponse(
	t *testing.T,
	content *factoryapi.WorkContent,
	want []work.WorkContentPart,
) {
	t.Helper()

	if content == nil {
		t.Fatal("primary result content = nil")
	}
	if len(*content) != len(want) {
		t.Fatalf("primary result parts = %d, want %d", len(*content), len(want))
	}
	for i, part := range want {
		gotPart, err := (*content)[i].AsWorkTextContentPart()
		if err != nil {
			t.Fatalf("AsWorkTextContentPart[%d]: %v", i, err)
		}
		if gotPart.Text != part.Text {
			t.Fatalf("primary result[%d].text = %q, want %q", i, gotPart.Text, part.Text)
		}
	}
}

func withRunOutput(cfg RunConfig, output *bytes.Buffer) RunConfig {
	cfg.Output = output
	return cfg
}

func TestBuildFactoryService_DefaultRequiresInjectedBuilder(t *testing.T) {
	original := openTestRuntimeRunner
	t.Cleanup(func() {
		openTestRuntimeRunner = original
	})
	openTestRuntimeRunner = missingTestRuntimeRunner

	_, err := openTestRuntimeRunner(context.Background(), nil, serviceedges.Edges{})
	if err == nil || !strings.Contains(err.Error(), "dependency-injected builder is required") {
		t.Fatalf("default builder err = %v, want dependency-injected builder requirement", err)
	}
}

func TestRuntimeLogDiagnosticsForRunnerMapsRuntimeHostMetadata(t *testing.T) {
	t.Parallel()

	want := runtimehost.RuntimeLogDiagnostics{
		Path: "/logs/runtime.jsonl", RootDir: "/logs", StartTimeUTC: time.Unix(10, 0).UTC(),
		MetricsPath: "/metrics/runtime.jsonl", MetricsRootDir: "/metrics", MetricsStartTimeUTC: time.Unix(20, 0).UTC(),
	}
	got := runtimeLogDiagnosticsForRunner(runtimeHostDiagnosticsRunner{diagnostics: want})
	if got.Path != want.Path || got.RootDir != want.RootDir || !got.StartTimeUTC.Equal(want.StartTimeUTC) ||
		got.MetricsPath != want.MetricsPath || got.MetricsRootDir != want.MetricsRootDir || !got.MetricsStartTimeUTC.Equal(want.MetricsStartTimeUTC) {
		t.Fatalf("runtimeLogDiagnosticsForRunner() = %+v, want %+v", got, want)
	}
}

type runtimeHostDiagnosticsRunner struct {
	diagnostics runtimehost.RuntimeLogDiagnostics
}

func (runtimeHostDiagnosticsRunner) Run(context.Context) error { return nil }

func (r runtimeHostDiagnosticsRunner) RuntimeLogDiagnostics() runtimehost.RuntimeLogDiagnostics {
	return r.diagnostics
}
