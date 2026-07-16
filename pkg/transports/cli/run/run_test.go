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

	initcmd "github.com/portpowered/infinite-you/pkg/transports/cli/init"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	invocations "github.com/portpowered/infinite-you/pkg/work/invocation"
)

type stubFactoryService struct {
	run                   func(context.Context) error
	snapshot              func(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error)
	runtimeLogDiagnostics service.RuntimeLogDiagnostics
}

func (s stubFactoryService) Run(ctx context.Context) error {
	return s.run(ctx)
}

func (s stubFactoryService) RuntimeLogDiagnostics() service.RuntimeLogDiagnostics {
	return s.runtimeLogDiagnostics
}

func (s stubFactoryService) GetEngineStateSnapshot(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	if s.snapshot == nil {
		return nil, errors.New("snapshot unavailable")
	}
	return s.snapshot(ctx)
}

type capturedOOTBSmokeRun struct {
	cfg *service.FactoryServiceConfig
	svc *service.FactoryService
}

func preserveRunGlobals(t *testing.T) {
	t.Helper()

	originalBuilder := buildFactoryService
	originalInvocationBootstrap := buildInvocationBootstrap
	originalBootstrap := bootstrapFactory
	originalOpener := dashboardOpener
	originalInteractive := interactiveOutput
	originalStartAPIServer := startAPIServer
	originalServeFactoryAPIServer := serveFactoryAPIServer
	if buildInvocationBootstrap == nil {
		buildInvocationBootstrap = func(ctx context.Context, cfg *service.FactoryServiceConfig) (InvocationRunner, error) {
			svc, err := service.BuildFactoryService(ctx, service.NormalizeInvocationBootstrapConfig(cfg))
			if err != nil {
				return nil, err
			}
			return service.NewInvocationBootstrap(svc)
		}
	}
	t.Cleanup(func() {
		buildFactoryService = originalBuilder
		buildInvocationBootstrap = originalInvocationBootstrap
		bootstrapFactory = originalBootstrap
		dashboardOpener = originalOpener
		interactiveOutput = originalInteractive
		startAPIServer = originalStartAPIServer
		serveFactoryAPIServer = originalServeFactoryAPIServer
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
		tokens   map[string]*interfaces.Token
		wantWIP  int
		wantDone int
		wantFail int
	}{
		{name: "empty marking", tokens: map[string]*interfaces.Token{}},
		{
			name: "mixed states",
			tokens: map[string]*interfaces.Token{
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
			tokens: map[string]*interfaces.Token{
				"t1": {ID: "t1", PlaceID: "page:completed"},
				"t2": {ID: "t2", PlaceID: "page:completed"},
			},
			wantDone: 2,
		},
		{
			name: "all failed",
			tokens: map[string]*interfaces.Token{
				"t1": {ID: "t1", PlaceID: "task:failed"},
				"t2": {ID: "t2", PlaceID: "task:failed"},
				"t3": {ID: "t3", PlaceID: "task:failed"},
			},
			wantFail: 3,
		},
		{
			name: "work type prefix stays local to suffix classification",
			tokens: map[string]*interfaces.Token{
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
			snap := &petri.MarkingSnapshot{Tokens: tt.tokens}
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

	got, err := LoadWorkFile(path)
	if err != nil {
		t.Fatalf("LoadWorkFile(%q): %v", path, err)
	}
	if got.RequestID != "docs-example-story-001" {
		t.Fatalf("request ID = %q, want docs-example-story-001", got.RequestID)
	}
	if got.Type != interfaces.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("type = %q, want %q", got.Type, interfaces.WorkRequestTypeFactoryRequestBatch)
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
	if _, err := factoryconfig.PersistNamedFactory(rootDir, "alpha", payload); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	if err := factoryconfig.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer: %v", err)
	}

	if err := bootstrapFactory(rootDir); err != nil {
		t.Fatalf("bootstrapFactory: %v", err)
	}

	inputDir := filepath.Join(rootDir, "alpha", interfaces.InputsDir, initcmd.DefaultFactoryInputType, interfaces.DefaultChannelName)
	if _, err := os.Stat(inputDir); err != nil {
		t.Fatalf("expected bootstrap to prepare current named-factory input dir %s: %v", inputDir, err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, interfaces.FactoryConfigFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected bootstrap to avoid creating legacy root factory.json, got err=%v", err)
	}
}

func TestRun_DefaultModeUsesBatchRuntimeAndExitsWhenRunReturns(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	var capturedMode interfaces.RuntimeMode
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
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

func TestRun_RecordOrReplayPathPassedToServiceConfig(t *testing.T) {
	originalDefaultRecordPath := defaultLiveRunRecordPath
	defer func() {
		defaultLiveRunRecordPath = originalDefaultRecordPath
	}()

	tests := []struct {
		name               string
		cfg                RunConfig
		defaultRecordPath  string
		wantRecordPath     string
		wantReplayPath     string
		wantGeneratorCalls int
	}{
		{
			name:               "default live mode",
			cfg:                RunConfig{},
			defaultRecordPath:  "auto-generated-recording.json",
			wantRecordPath:     "auto-generated-recording.json",
			wantGeneratorCalls: 1,
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
			name: "default recording disabled for one run",
			cfg: RunConfig{
				DisableDefaultRecording: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalBuilder := buildFactoryService
			defer func() {
				buildFactoryService = originalBuilder
			}()

			generatorCalls := 0
			defaultLiveRunRecordPath = func() (string, error) {
				generatorCalls++
				return tt.defaultRecordPath, nil
			}

			var capturedRecordPath string
			var capturedReplayPath string
			buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
				capturedRecordPath = cfg.RecordPath
				capturedReplayPath = cfg.ReplayPath
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
				if generatorCalls != 0 {
					t.Fatalf("default record path generator calls = %d, want 0", generatorCalls)
				}
				return
			}
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if capturedRecordPath != tt.wantRecordPath {
				t.Fatalf("record path = %q, want %q", capturedRecordPath, tt.wantRecordPath)
			}
			if capturedReplayPath != tt.wantReplayPath {
				t.Fatalf("replay path = %q, want %q", capturedReplayPath, tt.wantReplayPath)
			}
			if generatorCalls != tt.wantGeneratorCalls {
				t.Fatalf("default record path generator calls = %d, want %d", generatorCalls, tt.wantGeneratorCalls)
			}
		})
	}
}

func TestRun_DefaultRecordPathResolutionErrorSkipsServiceStart(t *testing.T) {
	originalBuilder := buildFactoryService
	originalDefaultRecordPath := defaultLiveRunRecordPath
	defer func() {
		buildFactoryService = originalBuilder
		defaultLiveRunRecordPath = originalDefaultRecordPath
	}()

	builderCalled := false
	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		builderCalled = true
		return stubFactoryService{run: func(context.Context) error { return nil }}, nil
	}
	defaultLiveRunRecordPath = func() (string, error) {
		return "", errors.New("home lookup failed")
	}

	err := Run(context.Background(), RunConfig{})
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

func TestGenerateDefaultLiveRunRecordPath_UsesRecordingsHierarchyAndSessionTemplate(t *testing.T) {
	originalTime := defaultLiveRunRecordTime
	originalUUID := defaultLiveRunRecordUUID
	defer func() {
		defaultLiveRunRecordTime = originalTime
		defaultLiveRunRecordUUID = originalUUID
	}()

	homeDir := t.TempDir()
	setUserHomeForTest(t, homeDir)
	defaultLiveRunRecordTime = func() time.Time {
		return time.Date(2026, time.May, 23, 18, 45, 12, 0, time.FixedZone("ICT", 7*60*60))
	}
	defaultLiveRunRecordUUID = func() string {
		return "uuid-1"
	}

	path, err := generateDefaultLiveRunRecordPath()
	if err != nil {
		t.Fatalf("generateDefaultLiveRunRecordPath: %v", err)
	}

	recordingsDir := defaultpaths.RecordingsDatedDir(
		defaultpaths.RecordingsRoot(homeDir),
		time.Date(2026, time.May, 23, 18, 45, 12, 0, time.FixedZone("ICT", 7*60*60)),
	)
	want := filepath.Join(
		recordingsDir,
		"factory-session-"+defaultRecordPathSessionToken+"-184512-uuid-1.json",
	)
	if path != want {
		t.Fatalf("generated path = %q, want %q", path, want)
	}
	if got := resolveDefaultSessionRecordPath(path); got != filepath.Join(
		recordingsDir,
		"factory-session-"+defaultFactorySessionID+"-184512-uuid-1.json",
	) {
		t.Fatalf("resolved default-session path = %q", got)
	}
}

func TestBuildApplicationSequentialHomesControlDefaultRecordingPath(t *testing.T) {
	ambientHome := t.TempDir()
	setUserHomeForTest(t, ambientHome)

	for _, homeDir := range []string{t.TempDir(), t.TempDir()} {
		var gotRecordPath string
		var gotSystemHome string
		var gotLogDir string
		var gotMetricsDir string
		application, err := BuildApplication(context.Background(), RunConfig{
			Dir: t.TempDir(), HomeDir: homeDir, Port: 0, SuppressDashboardRendering: true,
		}, func(_ context.Context, cfg *service.FactoryServiceConfig) (RuntimeRunner, error) {
			gotRecordPath = cfg.RecordPath
			gotSystemHome = cfg.SystemConfigHomeDir
			gotLogDir = cfg.RuntimeLogDir
			gotMetricsDir = cfg.RuntimeMetricsDir
			return stubFactoryService{run: func(context.Context) error { return nil }}, nil
		}, nil)
		if err != nil {
			t.Fatalf("BuildApplication(home %q) error = %v", homeDir, err)
		}
		wantRoot := defaultpaths.RecordingsRoot(homeDir)
		if !strings.HasPrefix(filepath.Clean(gotRecordPath), filepath.Clean(wantRoot)+string(os.PathSeparator)) {
			t.Fatalf("record path = %q, want below supplied home root %q", gotRecordPath, wantRoot)
		}
		if strings.HasPrefix(filepath.Clean(gotRecordPath), filepath.Clean(defaultpaths.RecordingsRoot(ambientHome))) {
			t.Fatalf("record path = %q, unexpectedly used ambient home %q", gotRecordPath, ambientHome)
		}
		if gotSystemHome != homeDir || gotLogDir != defaultpaths.RuntimeLogsRoot(homeDir) || gotMetricsDir != defaultpaths.RuntimeMetricsRoot(homeDir) {
			t.Fatalf("service home paths = home %q logs %q metrics %q; want roots below %q", gotSystemHome, gotLogDir, gotMetricsDir, homeDir)
		}
		if err := application.Run(context.Background()); err != nil {
			t.Fatalf("Application.Run(home %q) error = %v", homeDir, err)
		}
	}
}

func TestGenerateDefaultLiveRunRecordPath_UsesUniqueSuffixes(t *testing.T) {
	originalTime := defaultLiveRunRecordTime
	originalUUID := defaultLiveRunRecordUUID
	defer func() {
		defaultLiveRunRecordTime = originalTime
		defaultLiveRunRecordUUID = originalUUID
	}()

	homeDir := t.TempDir()
	setUserHomeForTest(t, homeDir)
	defaultLiveRunRecordTime = func() time.Time {
		return time.Date(2026, time.May, 23, 18, 45, 12, 0, time.FixedZone("ICT", 7*60*60))
	}
	nextUUID := []string{"uuid-1", "uuid-2"}
	defaultLiveRunRecordUUID = func() string {
		id := nextUUID[0]
		nextUUID = nextUUID[1:]
		return id
	}

	first, err := generateDefaultLiveRunRecordPath()
	if err != nil {
		t.Fatalf("generateDefaultLiveRunRecordPath(first): %v", err)
	}
	second, err := generateDefaultLiveRunRecordPath()
	if err != nil {
		t.Fatalf("generateDefaultLiveRunRecordPath(second): %v", err)
	}
	if first == second {
		t.Fatalf("generated paths matched: %q", first)
	}
}

func TestRun_WithBootstrapCallsBootstrapFactory(t *testing.T) {
	originalBuilder := buildFactoryService
	originalBootstrap := bootstrapFactory
	defer func() {
		buildFactoryService = originalBuilder
		bootstrapFactory = originalBootstrap
	}()

	dir := t.TempDir()
	var gotBootstrapDir string
	bootstrapFactory = func(inDir string) error {
		gotBootstrapDir = inDir
		return nil
	}

	var capturedMode interfaces.RuntimeMode
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		capturedMode = cfg.RuntimeMode
		return stubFactoryService{run: func(context.Context) error { return nil }}, nil
	}

	if err := Run(context.Background(), RunConfig{Bootstrap: true, Dir: dir}); err != nil {
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
		StartupOutput: &out,
	}
	if !emitStartupMessages(cfg, service.RuntimeLogDiagnostics{}, func(io.Writer) bool { return true }) {
		t.Fatal("startup messages did not request dashboard open")
	}
	dashboardReady := make(chan struct{})
	wait := openDashboardWhenServerReady(
		context.Background(), cfg, dashboardReady,
		func(_ context.Context, url string) error {
			openedURL = url
			close(opened)
			return nil
		},
	)
	select {
	case <-opened:
		t.Fatal("dashboard opener ran before API server readiness")
	default:
	}
	close(dashboardReady)
	select {
	case <-opened:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for dashboard opener")
	}
	wait()

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
		string(invocations.InputErrorCodeSourceConflict),
		string(invocations.InputSourcePositionalText),
		string(invocations.InputSourceStdinText),
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	}
}

func stringPtr(value string) *string {
	return &value
}

func assertInvocationRequestMatchesSharedResolver(
	t *testing.T,
	request *factoryapi.InvocationRequest,
	source invocations.InputSourceLabel,
	text string,
) {
	t.Helper()

	if request == nil {
		t.Fatal("invocation request = nil")
	}
	if request.SourceKind == nil || *request.SourceKind != factoryapi.InvocationInputSourceKindText {
		t.Fatalf("sourceKind = %v, want text", request.SourceKind)
	}

	sources := invocations.TextInputSources{}
	switch source {
	case invocations.InputSourcePositionalText:
		sources.PositionalText = &text
	case invocations.InputSourceStdinText:
		sources.StdinText = &text
	default:
		t.Fatalf("unsupported source label %q", source)
	}

	resolved, err := invocations.ResolveTextInput(sources)
	if err != nil {
		t.Fatalf("ResolveTextInput: %v", err)
	}
	want := invocationRequestFromResolvedInput(resolved)
	if got := extractInvocationText(t, request); got != extractInvocationText(t, want) {
		t.Fatalf("invocation text = %q, want %q", got, extractInvocationText(t, want))
	}
	if request.SourceKind == nil || want.SourceKind == nil || *request.SourceKind != *want.SourceKind {
		t.Fatalf("sourceKind = %v, want %v", request.SourceKind, want.SourceKind)
	}
}

func assertStableInvocationSourceConflictMessage(t *testing.T, got string, wantMessage string) {
	t.Helper()

	for _, fragment := range []string{
		string(invocations.InputSourcePositionalText),
		string(invocations.InputSourceStdinText),
		wantMessage,
	} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("error = %q, want fragment %q", got, fragment)
		}
	}
}

func invocationRequestFromLogicalAPIText(text string) (*factoryapi.InvocationRequest, error) {
	resolved, err := invocations.ResolveAPITextInputContent([]interfaces.WorkContentPart{{
		Type: interfaces.WorkContentPartTypeText,
		Text: text,
	}})
	if err != nil {
		return nil, err
	}
	return invocationRequestFromResolvedInput(resolved), nil
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
	if response.Status != result.Status {
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
	want []interfaces.WorkContentPart,
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

func TestSetBuildFactoryService_RegistersBuilder(t *testing.T) {
	original := buildFactoryService
	t.Cleanup(func() {
		buildFactoryService = original
	})

	builderErr := errors.New("registered builder")
	SetBuildFactoryService(func(
		_ context.Context,
		_ *service.FactoryServiceConfig,
	) (factoryServiceRunner, error) {
		return nil, builderErr
	})

	_, err := buildFactoryService(context.Background(), &service.FactoryServiceConfig{})
	if !errors.Is(err, builderErr) {
		t.Fatalf("buildFactoryService err = %v, want %v", err, builderErr)
	}
}

func TestSetBuildFactoryService_NilRestoresDefault(t *testing.T) {
	original := buildFactoryService
	t.Cleanup(func() {
		buildFactoryService = original
	})

	customErr := errors.New("custom builder")
	SetBuildFactoryService(func(
		_ context.Context,
		_ *service.FactoryServiceConfig,
	) (factoryServiceRunner, error) {
		return nil, customErr
	})
	SetBuildFactoryService(nil)

	secondErr := errors.New("second builder")
	SetBuildFactoryService(func(
		_ context.Context,
		_ *service.FactoryServiceConfig,
	) (factoryServiceRunner, error) {
		return nil, secondErr
	})

	_, err := buildFactoryService(context.Background(), &service.FactoryServiceConfig{})
	if !errors.Is(err, secondErr) {
		t.Fatalf("buildFactoryService err = %v, want %v", err, secondErr)
	}
}

func TestFactoryServiceBuilderFromService_AdaptsConcreteBuilder(t *testing.T) {
	original := buildFactoryService
	t.Cleanup(func() {
		buildFactoryService = original
	})

	builderErr := errors.New("adapted builder")
	SetBuildFactoryService(FactoryServiceBuilderFromService(func(
		_ context.Context,
		_ *service.FactoryServiceConfig,
	) (*service.FactoryService, error) {
		return nil, builderErr
	}))

	_, err := buildFactoryService(context.Background(), &service.FactoryServiceConfig{})
	if !errors.Is(err, builderErr) {
		t.Fatalf("buildFactoryService err = %v, want %v", err, builderErr)
	}
}

func TestBuildFactoryService_DefaultRequiresInjectedBuilder(t *testing.T) {
	original := buildFactoryService
	t.Cleanup(func() {
		buildFactoryService = original
	})
	buildFactoryService = defaultBuildFactoryService

	_, err := buildFactoryService(context.Background(), nil)
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
