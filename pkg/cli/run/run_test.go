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

	"github.com/portpowered/infinite-you/pkg/apisurface"
	initcmd "github.com/portpowered/infinite-you/pkg/cli/init"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"go.uber.org/zap"
)

type stubFactoryService struct {
	run func(context.Context) error
}

func (s stubFactoryService) Run(ctx context.Context) error {
	return s.run(ctx)
}

type capturedOOTBSmokeRun struct {
	cfg *service.FactoryServiceConfig
	svc *service.FactoryService
}

func preserveRunGlobals(t *testing.T) {
	t.Helper()

	originalBuilder := buildFactoryService
	originalBootstrap := bootstrapFactory
	originalOpener := dashboardOpener
	originalInteractive := interactiveOutput
	originalStartAPIServer := startAPIServer
	t.Cleanup(func() {
		buildFactoryService = originalBuilder
		bootstrapFactory = originalBootstrap
		dashboardOpener = originalOpener
		interactiveOutput = originalInteractive
		startAPIServer = originalStartAPIServer
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

func TestLoadWorkFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "work.json")

	req := interfaces.WorkRequest{
		RequestID: "request-1",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "source-file",
			WorkTypeID: "task",
			TraceID:    "trace-1",
			Payload:    map[string]any{"file": "test.go"},
			Tags:       map[string]string{"priority": "high"},
		}},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := LoadWorkFile(path)
	if err != nil {
		t.Fatalf("LoadWorkFile: %v", err)
	}
	if got.Type != interfaces.WorkRequestTypeFactoryRequestBatch {
		t.Errorf("Type = %q, want %q", got.Type, interfaces.WorkRequestTypeFactoryRequestBatch)
	}
	if len(got.Works) != 1 || got.Works[0].WorkTypeID != "task" {
		t.Fatalf("Works = %#v, want one task work item", got.Works)
	}
	if got.Works[0].TraceID != "trace-1" {
		t.Errorf("TraceID = %q, want trace-1", got.Works[0].TraceID)
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

func TestLoadWorkFile_NotFound(t *testing.T) {
	_, err := LoadWorkFile("/nonexistent/path.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadWorkFile_RejectsRetiredTargetStateAlias(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "work.json")
	writeFile(t, path, `{
  "request_id": "request-cli-target-state",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "draft", "work_type_name": "task", "target_state": "waiting"}
  ]
}`)

	_, err := LoadWorkFile(path)
	if err == nil {
		t.Fatal("expected retired target_state alias to fail")
	}
	if !strings.Contains(err.Error(), "target_state") || !strings.Contains(err.Error(), "state") {
		t.Fatalf("error = %q, want target_state rejection with state guidance", err.Error())
	}
}

func TestLoadWorkFile_RejectsConflictingTraceAliases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "work.json")
	writeFile(t, path, `{
  "requestId": "request-cli-trace-conflict",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "draft",
      "workTypeName": "task",
      "currentChainingTraceId": "chain-a",
      "traceId": "trace-b"
    }
  ]
}`)

	_, err := LoadWorkFile(path)
	if err == nil {
		t.Fatal("expected conflicting trace aliases to fail")
	}
	if !strings.Contains(err.Error(), "currentChainingTraceId and traceId must match") {
		t.Fatalf("error = %q, want conflicting trace alias rejection", err.Error())
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

	want := filepath.Join(
		homeDir,
		defaultRecordingsDir,
		"2026-05",
		"2026-05-23",
		"factory-session-"+defaultRecordPathSessionToken+"-184512-uuid-1.json",
	)
	if path != want {
		t.Fatalf("generated path = %q, want %q", path, want)
	}
	if got := resolveDefaultSessionRecordPath(path); got != filepath.Join(
		homeDir,
		defaultRecordingsDir,
		"2026-05",
		"2026-05-23",
		"factory-session-"+defaultFactorySessionID+"-184512-uuid-1.json",
	) {
		t.Fatalf("resolved default-session path = %q", got)
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
	preserveRunGlobals(t)

	bootstrapFactory = func(string) error {
		return nil
	}
	useAPIServerBackedServiceBuilder()

	var openedURL string
	opened := make(chan struct{})
	installReadyDashboardOpenAssertions(t, &openedURL, opened)

	var out bytes.Buffer
	err := Run(context.Background(), RunConfig{
		Dir:           "factory",
		Port:          7437,
		Bootstrap:     true,
		Continuously:  true,
		OpenDashboard: true,
		StartupOutput: &out,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
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

func useAPIServerBackedServiceBuilder() {
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubFactoryService{
			run: func(ctx context.Context) error {
				return cfg.APIServerStarter(ctx, nil, cfg.Port, zap.NewNop())
			},
		}, nil
	}
}

func installReadyDashboardOpenAssertions(t *testing.T, openedURL *string, opened chan struct{}) {
	t.Helper()

	dashboardOpener = func(_ context.Context, url string) error {
		*openedURL = url
		close(opened)
		return nil
	}
	interactiveOutput = func(io.Writer) bool {
		return true
	}
	startAPIServer = func(
		ctx context.Context,
		_ apisurface.APISurface,
		_ int,
		_ *zap.Logger,
		markReady func(),
	) error {
		if *openedURL != "" {
			t.Fatalf("dashboard opener ran before API server readiness: %q", *openedURL)
		}
		markReady()
		select {
		case <-opened:
		case <-ctx.Done():
			t.Fatal("context canceled before dashboard opened")
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for dashboard opener")
		}
		return nil
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
