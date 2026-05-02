package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	initcmd "github.com/portpowered/infinite-you/pkg/cli/init"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/service"
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

func TestCountTokenStates(t *testing.T) {
	tests := []struct {
		name     string
		tokens   map[string]*interfaces.Token
		wantWIP  int
		wantDone int
		wantFail int
	}{
		{
			name:   "empty marking",
			tokens: map[string]*interfaces.Token{},
		},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap := &petri.MarkingSnapshot{
				Tokens: tt.tokens,
			}
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
		{0, "0m"},
		{30 * time.Second, "0m"},
		{5 * time.Minute, "5m"},
		{90 * time.Minute, "1h 30m"},
		{2*time.Hour + 15*time.Minute, "2h 15m"},
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

func TestBootstrapFactory_UsesCurrentNamedFactoryPointerLayout(t *testing.T) {
	rootDir := t.TempDir()

	payload, err := json.Marshal(map[string]any{
		"name": "alpha-factory",
		"id": "alpha",
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
			"name":           "execute-alpha",
			"worker":         "executor",
			"inputs":         []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":        []map[string]string{{"workType": "task", "state": "complete"}},
			"type":           "MODEL_WORKSTATION",
			"promptTemplate": "Implement {{ .WorkID }}.",
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
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	if err := Run(context.Background(), RunConfig{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedMode != interfaces.RuntimeModeBatch {
		t.Fatalf("runtime mode = %q, want %q", capturedMode, interfaces.RuntimeModeBatch)
	}
}

func TestRun_RecordOrReplayPathPassedToServiceConfig(t *testing.T) {
	tests := []struct {
		name           string
		cfg            RunConfig
		wantRecordPath string
		wantReplayPath string
	}{
		{
			name:           "record mode",
			cfg:            RunConfig{RecordPath: "run.replay.json"},
			wantRecordPath: "run.replay.json",
		},
		{
			name:           "replay mode",
			cfg:            RunConfig{ReplayPath: "existing.replay.json"},
			wantReplayPath: "existing.replay.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalBuilder := buildFactoryService
			defer func() {
				buildFactoryService = originalBuilder
			}()

			var capturedRecordPath string
			var capturedReplayPath string
			buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
				capturedRecordPath = cfg.RecordPath
				capturedReplayPath = cfg.ReplayPath
				return stubFactoryService{
					run: func(context.Context) error {
						return nil
					},
				}, nil
			}

			if err := Run(context.Background(), tt.cfg); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if capturedRecordPath != tt.wantRecordPath {
				t.Fatalf("record path = %q, want %q", capturedRecordPath, tt.wantRecordPath)
			}
			if capturedReplayPath != tt.wantReplayPath {
				t.Fatalf("replay path = %q, want %q", capturedReplayPath, tt.wantReplayPath)
			}
		})
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
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{Bootstrap: true, Dir: dir})
	if err != nil {
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

func TestRun_StartupOutputFallsBackWhenDashboardOpenFails(t *testing.T) {
	originalBuilder := buildFactoryService
	originalOpener := dashboardOpener
	originalInteractive := interactiveOutput
	originalStartAPIServer := startAPIServer
	defer func() {
		buildFactoryService = originalBuilder
		dashboardOpener = originalOpener
		interactiveOutput = originalInteractive
		startAPIServer = originalStartAPIServer
	}()

	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubFactoryService{
			run: func(ctx context.Context) error {
				return cfg.APIServerStarter(ctx, nil, cfg.Port, zap.NewNop())
			},
		}, nil
	}
	openAttempted := make(chan struct{})
	dashboardOpener = func(_ context.Context, _ string) error {
		close(openAttempted)
		return errors.New("browser unavailable")
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
		markReady()
		select {
		case <-openAttempted:
		case <-ctx.Done():
			t.Fatal("context canceled before dashboard open fallback")
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for dashboard open fallback")
		}
		return nil
	}

	var out bytes.Buffer
	err := Run(context.Background(), RunConfig{
		Dir:           "factory",
		Port:          7437,
		OpenDashboard: true,
		StartupOutput: &out,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Dashboard auto-open unavailable: browser unavailable") {
		t.Fatalf("startup output = %q, want unavailable fallback", output)
	}
	if !strings.Contains(output, "Open the dashboard at http://localhost:7437/dashboard/ui") {
		t.Fatalf("startup output = %q, want manual dashboard URL", output)
	}
}

func TestRun_StartupOutputReportsDashboardWhenAutoOpenDisabled(t *testing.T) {
	originalBuilder := buildFactoryService
	originalOpener := dashboardOpener
	originalInteractive := interactiveOutput
	defer func() {
		buildFactoryService = originalBuilder
		dashboardOpener = originalOpener
		interactiveOutput = originalInteractive
	}()

	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}
	dashboardOpener = func(_ context.Context, _ string) error {
		t.Fatal("dashboard opener should not be called when auto-open is disabled")
		return nil
	}
	interactiveOutput = func(io.Writer) bool {
		return true
	}

	var out bytes.Buffer
	err := Run(context.Background(), RunConfig{
		Dir:           "factory",
		Port:          7437,
		StartupOutput: &out,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Dashboard URL: http://localhost:7437/dashboard/ui") {
		t.Fatalf("startup output = %q, want dashboard URL", output)
	}
	if !strings.Contains(output, "Dashboard auto-open disabled; open http://localhost:7437/dashboard/ui") {
		t.Fatalf("startup output = %q, want disabled fallback", output)
	}
}

func TestRun_AutoPortResolvesBusyPreferredPortBeforeServiceBuildAndStartupOutput(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	busyListener, busyPort := listenOnBusyTCPPort(t)
	defer busyListener.Close()

	var capturedPort int
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		capturedPort = cfg.Port
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	var out bytes.Buffer
	err := Run(context.Background(), RunConfig{
		Dir:           "factory",
		Port:          busyPort,
		AutoPort:      true,
		StartupOutput: &out,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedPort == busyPort {
		t.Fatalf("service port = busy port %d, want auto-resolved fallback", busyPort)
	}
	if capturedPort <= 0 {
		t.Fatalf("service port = %d, want positive resolved port", capturedPort)
	}

	output := out.String()
	wantURL := DashboardURL(capturedPort)
	if !strings.Contains(output, "Dashboard URL: "+wantURL) {
		t.Fatalf("startup output = %q, want resolved dashboard URL %q", output, wantURL)
	}
	if strings.Contains(output, DashboardURL(busyPort)) {
		t.Fatalf("startup output = %q, should not report busy dashboard URL %q", output, DashboardURL(busyPort))
	}
}

func TestRun_StartupOutputSkipsDashboardOpenWhenOutputIsNonInteractive(t *testing.T) {
	originalBuilder := buildFactoryService
	originalOpener := dashboardOpener
	originalInteractive := interactiveOutput
	defer func() {
		buildFactoryService = originalBuilder
		dashboardOpener = originalOpener
		interactiveOutput = originalInteractive
	}()

	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}
	dashboardOpener = func(_ context.Context, _ string) error {
		t.Fatal("dashboard opener should not be called for non-interactive output")
		return nil
	}
	interactiveOutput = func(io.Writer) bool {
		return false
	}

	var out bytes.Buffer
	err := Run(context.Background(), RunConfig{
		Dir:           "factory",
		Port:          7437,
		OpenDashboard: true,
		StartupOutput: &out,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Dashboard auto-open disabled; open http://localhost:7437/dashboard/ui") {
		t.Fatalf("startup output = %q, want non-interactive fallback", output)
	}
}

func TestRun_BootstrapErrorSkipsServiceStart(t *testing.T) {
	originalBuilder := buildFactoryService
	originalBootstrap := bootstrapFactory
	defer func() {
		buildFactoryService = originalBuilder
		bootstrapFactory = originalBootstrap
	}()

	bootstrapFactory = func(_ string) error {
		return errors.New("bootstrap failed")
	}

	builderCalled := false
	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		builderCalled = true
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{Bootstrap: true})
	if err == nil {
		t.Fatal("expected bootstrap failure")
	}
	if !strings.Contains(err.Error(), "bootstrap failed") {
		t.Fatalf("error = %q, want bootstrap failure", err.Error())
	}
	if builderCalled {
		t.Fatal("factory service builder should not run when bootstrap fails")
	}
}

func TestRun_VerbosePassedToServiceConfig(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	var capturedVerbose bool
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		capturedVerbose = cfg.Verbose
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	if err := Run(context.Background(), RunConfig{Verbose: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !capturedVerbose {
		t.Fatal("verbose = false, want true")
	}
}

func TestRun_DefaultsExecutionBaseDirToCurrentWorkingDirectory(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	workingDirectory := t.TempDir()
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatalf("Chdir(%q): %v", workingDirectory, err)
	}
	defer func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}()

	var capturedBaseDir string
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		capturedBaseDir = cfg.ExecutionBaseDir
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	if err := Run(context.Background(), RunConfig{Dir: "factory"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedBaseDir != workingDirectory {
		t.Fatalf("execution base dir = %q, want %q", capturedBaseDir, workingDirectory)
	}
}

func TestRun_ExplicitExecutionBaseDirOverridesCurrentWorkingDirectory(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	workingDirectory := t.TempDir()
	overrideDir := t.TempDir()
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatalf("Chdir(%q): %v", workingDirectory, err)
	}
	defer func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}()

	var capturedBaseDir string
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		capturedBaseDir = cfg.ExecutionBaseDir
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	if err := Run(context.Background(), RunConfig{Dir: "factory", ExecutionBaseDir: overrideDir}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedBaseDir != overrideDir {
		t.Fatalf("execution base dir = %q, want %q", capturedBaseDir, overrideDir)
	}
}

func TestRun_RuntimeLogConfigPassedToServiceConfig(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	var capturedConfig *service.FactoryServiceConfig
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		capturedConfig = cfg
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	runtimeLogConfig := logging.RuntimeLogConfig{
		MaxSize:    12,
		MaxBackups: 6,
		MaxAge:     21,
		Compress:   true,
	}
	err := Run(context.Background(), RunConfig{
		RuntimeLogDir:    "runtime-logs",
		RuntimeLogConfig: runtimeLogConfig,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedConfig == nil {
		t.Fatal("expected factory service to be built")
	}
	if capturedConfig.RuntimeLogDir != "runtime-logs" {
		t.Fatalf("runtime log dir = %q, want runtime-logs", capturedConfig.RuntimeLogDir)
	}
	if capturedConfig.RuntimeLogConfig != runtimeLogConfig {
		t.Fatalf("runtime log config = %#v, want %#v", capturedConfig.RuntimeLogConfig, runtimeLogConfig)
	}
}

func TestRun_WithMockWorkersWithoutPathPassesDefaultConfigToService(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	var capturedConfig *service.FactoryServiceConfig
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		capturedConfig = cfg
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	if err := Run(context.Background(), RunConfig{MockWorkersEnabled: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedConfig == nil {
		t.Fatal("expected factory service to be built")
	}
	if capturedConfig.MockWorkersConfig == nil {
		t.Fatal("expected mock workers config to be passed to service")
	}
	if len(capturedConfig.MockWorkersConfig.MockWorkers) != 0 {
		t.Fatalf("mock worker count = %d, want empty default accept config", len(capturedConfig.MockWorkersConfig.MockWorkers))
	}
}

func TestRun_WithMockWorkersConfigPathLoadsConfigBeforeServiceStart(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	dir := t.TempDir()
	mockWorkersPath := filepath.Join(dir, "mock-workers.json")
	writeFile(t, mockWorkersPath, `{
  "mockWorkers": [
    {
      "id": "reviewer-rejects",
      "workerName": "reviewer",
      "runType": "reject",
      "rejectConfig": {
        "stderr": "needs changes",
        "exitCode": 42
      }
    }
  ]
}
`)

	var capturedConfig *service.FactoryServiceConfig
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		capturedConfig = cfg
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		MockWorkersEnabled:    true,
		MockWorkersConfigPath: mockWorkersPath,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedConfig == nil || capturedConfig.MockWorkersConfig == nil {
		t.Fatal("expected loaded mock workers config to be passed to service")
	}
	got := capturedConfig.MockWorkersConfig.MockWorkers
	if len(got) != 1 {
		t.Fatalf("mock worker count = %d, want 1", len(got))
	}
	if got[0].ID != "reviewer-rejects" || got[0].WorkerName != "reviewer" {
		t.Fatalf("loaded mock worker = %#v, want reviewer target", got[0])
	}
	if got[0].RejectConfig == nil || got[0].RejectConfig.ExitCode == nil || *got[0].RejectConfig.ExitCode != 42 {
		t.Fatalf("reject config = %#v, want exit code 42", got[0].RejectConfig)
	}
}

func TestRun_WithMockWorkersInvalidPathFailsBeforeServiceStart(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	builderCalled := false
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		builderCalled = true
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		MockWorkersEnabled:    true,
		MockWorkersConfigPath: filepath.Join(t.TempDir(), "missing.json"),
	})
	if err == nil {
		t.Fatal("expected missing mock workers config path to fail")
	}
	if !strings.Contains(err.Error(), "read mock workers config") {
		t.Fatalf("error = %q, want read mock workers config context", err.Error())
	}
	if builderCalled {
		t.Fatal("factory service builder should not be called when mock config loading fails")
	}
}

func TestRun_WithMockWorkersInvalidJSONFailsBeforeServiceStart(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	dir := t.TempDir()
	mockWorkersPath := filepath.Join(dir, "mock-workers.json")
	writeFile(t, mockWorkersPath, `{"mockWorkers":[{"runType":"bogus"}]}`)

	builderCalled := false
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		builderCalled = true
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		MockWorkersEnabled:    true,
		MockWorkersConfigPath: mockWorkersPath,
	})
	if err == nil {
		t.Fatal("expected invalid mock workers config to fail")
	}
	if !strings.Contains(err.Error(), "runType must be one of") {
		t.Fatalf("error = %q, want runType validation context", err.Error())
	}
	if builderCalled {
		t.Fatal("factory service builder should not be called when mock config validation fails")
	}
}

func TestRun_DefaultDashboardRendering_PrintsSimpleDashboardOutput(t *testing.T) {
	dir, workFile := writeDashboardRunFixture(t)

	output, err := runWithCapturedStdout(t, RunConfig{
		Dir:                dir,
		Port:               0,
		WorkFile:           workFile,
		MockWorkersEnabled: true,
		Logger:             zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(output, "Factory:") {
		t.Fatalf("expected simple dashboard output, got %q", output)
	}
}

func TestRun_SuppressDashboardRendering_SkipsSimpleDashboardOutput(t *testing.T) {
	dir, workFile := writeDashboardRunFixture(t)

	output, err := runWithCapturedStdout(t, RunConfig{
		Dir:                        dir,
		Port:                       0,
		WorkFile:                   workFile,
		MockWorkersEnabled:         true,
		SuppressDashboardRendering: true,
		Logger:                     zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if output != "" {
		t.Fatalf("expected no simple dashboard output, got %q", output)
	}
}

func TestRun_ContinuouslyUsesServiceModeUntilCanceled(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	started := make(chan struct{})
	var capturedMode interfaces.RuntimeMode
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		capturedMode = cfg.RuntimeMode
		return stubFactoryService{
			run: func(ctx context.Context) error {
				close(started)
				<-ctx.Done()
				return nil
			},
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, RunConfig{Continuously: true})
	}()

	select {
	case <-started:
	case err := <-errCh:
		t.Fatalf("Run returned before cancellation: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for continuous run to start")
	}

	if capturedMode != interfaces.RuntimeModeService {
		t.Fatalf("runtime mode = %q, want %q", capturedMode, interfaces.RuntimeModeService)
	}

	select {
	case err := <-errCh:
		t.Fatalf("Run returned before cancellation: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for continuous run to stop after cancellation")
	}
}

func TestRun_OOTBIntegrationSmokeBootstrapsProcessesDefaultTaskAndReportsDashboard(t *testing.T) {
	preserveRunGlobals(t)

	dir := filepath.Join(t.TempDir(), "factory")
	taskPath := filepath.Join(dir, "inputs", "tasks", "default", "ootb-smoke.md")
	installOOTBSmokeBootstrap(taskPath)
	disableInteractiveDashboardForSmoke(t)

	capturedCh := make(chan capturedOOTBSmokeRun, 1)
	captureOOTBSmokeServiceBuilds(capturedCh)

	port := unusedTCPPort(t)
	var out bytes.Buffer
	cancel, errCh := startOOTBSmokeRun(t, dir, port, &out)
	captured := waitForOOTBSmokeServiceStartup(t, capturedCh, errCh)

	assertOOTBSmokeStartupConfig(t, captured.cfg, dir, taskPath)
	snapshot := waitForOOTBSmokeTaskCompletion(t, captured.svc, errCh)
	assertOOTBSmokeTaskResult(t, snapshot)
	assertContinuousRunStillActive(t, errCh)
	assertOOTBSmokeStartupOutput(t, out.String(), dir, port)
	stopOOTBSmokeRun(t, cancel, errCh)
}

func installOOTBSmokeBootstrap(taskPath string) {
	originalBootstrap := bootstrapFactory
	bootstrapFactory = func(inDir string) error {
		if err := originalBootstrap(inDir); err != nil {
			return err
		}
		return os.WriteFile(taskPath, []byte("# OOTB smoke\n\nConfirm the default task path is processed."), 0o644)
	}
}

func disableInteractiveDashboardForSmoke(t *testing.T) {
	t.Helper()

	dashboardOpener = func(_ context.Context, _ string) error {
		t.Fatal("dashboard opener should not run for non-interactive smoke output")
		return nil
	}
	interactiveOutput = func(io.Writer) bool {
		return false
	}
}

func captureOOTBSmokeServiceBuilds(capturedCh chan<- capturedOOTBSmokeRun) {
	buildFactoryService = func(ctx context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		cfgCopy := *cfg
		svc, err := service.BuildFactoryService(ctx, cfg)
		if err != nil {
			return nil, err
		}
		capturedCh <- capturedOOTBSmokeRun{cfg: &cfgCopy, svc: svc}
		return svc, nil
	}
}

func startOOTBSmokeRun(t *testing.T, dir string, port int, out *bytes.Buffer) (context.CancelFunc, chan error) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, RunConfig{
			Dir:                dir,
			Bootstrap:          true,
			Continuously:       true,
			MockWorkersEnabled: true,
			Port:               port,
			OpenDashboard:      true,
			StartupOutput:      out,
			Logger:             zap.NewNop(),
		})
	}()
	return cancel, errCh
}

func waitForOOTBSmokeServiceStartup(
	t *testing.T,
	capturedCh <-chan capturedOOTBSmokeRun,
	errCh <-chan error,
) capturedOOTBSmokeRun {
	t.Helper()

	select {
	case captured := <-capturedCh:
		return captured
	case err := <-errCh:
		t.Fatalf("Run returned before service startup: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for factory service startup")
	}
	return capturedOOTBSmokeRun{}
}

func assertOOTBSmokeStartupConfig(t *testing.T, cfg *service.FactoryServiceConfig, dir, taskPath string) {
	t.Helper()

	if cfg.RuntimeMode != interfaces.RuntimeModeService {
		t.Fatalf("runtime mode = %q, want %q", cfg.RuntimeMode, interfaces.RuntimeModeService)
	}
	if cfg.MockWorkersConfig == nil {
		t.Fatalf("mock-worker config was not passed through: %#v", cfg)
	}
	if _, err := os.Stat(filepath.Join(dir, "factory.json")); err != nil {
		t.Fatalf("expected bootstrap to create factory.json: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(taskPath)); err != nil {
		t.Fatalf("expected bootstrap to create inputs/tasks/default: %v", err)
	}
}

func waitForOOTBSmokeTaskCompletion(
	t *testing.T,
	svc *service.FactoryService,
	errCh <-chan error,
) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	for {
		snapshot, err := svc.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot: %v", err)
		}
		if len(snapshot.Marking.TokensInPlace("tasks:complete")) == 1 {
			return snapshot
		}
		select {
		case err := <-errCh:
			t.Fatalf("Run returned before completing default task: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for default task; places: %#v", snapshot.Marking.PlaceTokens)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func assertOOTBSmokeTaskResult(
	t *testing.T,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) {
	t.Helper()

	if got := len(snapshot.Marking.TokensInPlace("tasks:complete")); got != 1 {
		t.Fatalf("tasks:complete token count = %d, want 1; places: %#v", got, snapshot.Marking.PlaceTokens)
	}
	if got := len(snapshot.Marking.TokensInPlace("tasks:failed")); got != 0 {
		t.Fatalf("tasks:failed token count = %d, want 0; places: %#v", got, snapshot.Marking.PlaceTokens)
	}
}

func assertContinuousRunStillActive(t *testing.T, errCh <-chan error) {
	t.Helper()

	select {
	case err := <-errCh:
		t.Fatalf("continuous Run returned before cancellation: %v", err)
	default:
	}
}

func assertOOTBSmokeStartupOutput(t *testing.T, output, dir string, port int) {
	t.Helper()

	wantURL := fmt.Sprintf("http://localhost:%d/dashboard/ui", port)
	for _, want := range []string{
		"Factory initiated: " + dir,
		"Factory directory ready: " + dir,
		"Runtime mode: continuous",
		"Dashboard URL: " + wantURL,
		"Dashboard auto-open disabled; open " + wantURL,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("startup output = %q, want %q", output, want)
		}
	}
}

func stopOOTBSmokeRun(t *testing.T, cancel context.CancelFunc, errCh <-chan error) {
	t.Helper()

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for continuous run to stop after cancellation")
	}
}

func writeDashboardRunFixture(t *testing.T) (string, string) {
	t.Helper()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "factory.json"), `{
  "name": "dashboard-run-fixture",
  "workTypes": [
    {
      "name": "task",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "done", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "workers": [
    { "name": "script-worker" }
  ],
  "workstations": [
    {
      "name": "run-script",
      "worker": "script-worker",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "done" }],
      "onFailure": { "workType": "task", "state": "failed" }
    }
  ]
}
`)
	writeFile(t, filepath.Join(dir, "workers", "script-worker", "AGENTS.md"), `---
type: SCRIPT_WORKER
command: echo
args:
  - "dashboard-test"
---
`)
	writeFile(t, filepath.Join(dir, "workstations", "run-script", "AGENTS.md"), `---
type: MODEL_WORKSTATION
---
Run the script.
`)

	workFile := filepath.Join(t.TempDir(), "work.json")
	req := interfaces.WorkRequest{
		Type: interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "dashboard-render-test-work",
			WorkID:     "dashboard-render-test-work",
			WorkTypeID: "task",
			TraceID:    "dashboard-render-test-trace",
			Payload:    "exercise dashboard rendering",
		}},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal work file: %v", err)
	}
	writeFile(t, workFile, string(data))

	return dir, workFile
}

func unusedTCPPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on unused TCP port: %v", err)
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address has type %T, want *net.TCPAddr", listener.Addr())
	}
	return addr.Port
}

func listenOnBusyTCPPort(t *testing.T) (net.Listener, int) {
	t.Helper()

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen on busy TCP port fixture: %v", err)
	}

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		listener.Close()
		t.Fatalf("listener address has type %T, want *net.TCPAddr", listener.Addr())
	}
	return listener, addr.Port
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent dir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runWithCapturedStdout(t *testing.T, cfg RunConfig) (string, error) {
	t.Helper()

	oldStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}

	readCh := make(chan []byte, 1)
	readErrCh := make(chan error, 1)
	go func() {
		data, readErr := io.ReadAll(readPipe)
		readCh <- data
		readErrCh <- readErr
	}()

	os.Stdout = writePipe
	runErr := Run(context.Background(), cfg)
	os.Stdout = oldStdout

	if err := writePipe.Close(); err != nil {
		t.Fatalf("close captured stdout writer: %v", err)
	}
	output := <-readCh
	if err := <-readErrCh; err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	if err := readPipe.Close(); err != nil {
		t.Fatalf("close captured stdout reader: %v", err)
	}

	return string(output), runErr
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
