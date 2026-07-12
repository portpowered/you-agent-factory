package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/cmd/factory/compose"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

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

func TestRun_StartupOutputReportsRuntimeLogPathAndUTCStartTime(t *testing.T) {
	originalBuilder := buildFactoryService
	originalOpener := dashboardOpener
	originalInteractive := interactiveOutput
	defer func() {
		buildFactoryService = originalBuilder
		dashboardOpener = originalOpener
		interactiveOutput = originalInteractive
	}()

	startedAt := time.Date(2026, 5, 29, 4, 45, 3, 0, time.UTC)
	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubFactoryService{
			runtimeLogDiagnostics: service.RuntimeLogDiagnostics{
				Path:                "/tmp/runtime-logs/2026-05/2026-05-29/044503-runtime.log",
				RootDir:             "/tmp/runtime-logs",
				StartTimeUTC:        startedAt,
				MetricsPath:         "/tmp/runtime-metrics/2026/05/29/044503-session-runtime.log",
				MetricsRootDir:      "/tmp/runtime-metrics",
				MetricsStartTimeUTC: startedAt,
			},
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
	if !strings.Contains(output, "Runtime log: /tmp/runtime-logs/2026-05/2026-05-29/044503-runtime.log") {
		t.Fatalf("startup output = %q, want runtime log path", output)
	}
	if !strings.Contains(output, "Runtime log start (UTC): 2026-05-29 04:45:03 UTC") {
		t.Fatalf("startup output = %q, want UTC runtime log start", output)
	}
	if !strings.Contains(output, "Runtime metrics: /tmp/runtime-metrics/2026/05/29/044503-session-runtime.log") {
		t.Fatalf("startup output = %q, want runtime metrics path", output)
	}
	if !strings.Contains(output, "Runtime metrics start (UTC): 2026-05-29 04:45:03 UTC") {
		t.Fatalf("startup output = %q, want UTC runtime metrics start", output)
	}
	if strings.Contains(output, "0001-01-01") {
		t.Fatalf("startup output = %q, must not expose Go zero-time output", output)
	}
}

func TestRun_StartupOutputUsesFallbackForMissingRuntimeLogStartTime(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubFactoryService{
			runtimeLogDiagnostics: service.RuntimeLogDiagnostics{
				Path: "/tmp/runtime.log",
			},
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	var out bytes.Buffer
	err := Run(context.Background(), RunConfig{
		Dir:           "factory",
		Port:          0,
		StartupOutput: &out,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Runtime log start (UTC): n/a") {
		t.Fatalf("startup output = %q, want n/a for missing runtime log start", output)
	}
	if strings.Contains(output, "0001-01-01") {
		t.Fatalf("startup output = %q, must not expose Go zero-time output", output)
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
	wantURL := DashboardURL("localhost", capturedPort)
	if !strings.Contains(output, "Dashboard URL: "+wantURL) {
		t.Fatalf("startup output = %q, want resolved dashboard URL %q", output, wantURL)
	}
	if strings.Contains(output, DashboardURL("localhost", busyPort)) {
		t.Fatalf("startup output = %q, should not report busy dashboard URL %q", output, DashboardURL("localhost", busyPort))
	}
}

func TestRun_VerboseStartupDiagnosticsReportResolvedRuntimeMetadata(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	dir, _ := writeDashboardRunFixture(t)
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

	var diagnostics bytes.Buffer
	err := Run(context.Background(), RunConfig{
		Dir:      dir,
		Workflow: "workflow-1",
		OperatorDefaults: operatorconfig.ResolvedDefaults{
			WorkerModelProvider:       "CODEX",
			WorkerModel:               "gpt-5-codex",
			WorkerModelProviderSource: operatorconfig.SourceFlag,
			WorkerModelSource:         operatorconfig.SourceFlag,
			ConfigPath:                "/tmp/config.json",
		},
		Port:                       busyPort,
		AutoPort:                   true,
		DisableDefaultRecording:    true,
		RuntimeLogDir:              "logs/runtime",
		RuntimeMetricsDir:          "logs/metrics",
		RuntimeMetricsConfig:       logging.RuntimeMetricsConfig{MaxSize: 19, MaxBackups: 8, MaxAge: 17, Compress: true},
		MockWorkersEnabled:         true,
		SuppressDashboardRendering: true,
		Verbose:                    true,
		Diagnostics:                &diagnostics,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedPort == busyPort {
		t.Fatalf("service port = busy port %d, want auto-resolved fallback", busyPort)
	}

	got := diagnostics.String()
	for _, want := range []string{
		"run startup",
		"factoryDir=" + strconv.Quote(dir),
		"configuredDir=" + strconv.Quote(dir),
		"runtimeMode=BATCH",
		`workflow="workflow-1"`,
		"operatorDefaults precedence=file < env < flag",
		"provider=CODEX",
		"providerSource=flag",
		"model=gpt-5-codex",
		"modelSource=flag",
		"mockWorkers=true",
		"recording=disabled",
		`runtimeLogDir="logs/runtime"`,
		"runtimeLogRoll=size_mb=0 backups=0 age_days=0 compress=false",
		`runtimeMetricsDir="logs/metrics"`,
		"runtimeMetricsRoll=size_mb=19 backups=8 age_days=17 compress=true",
		"dashboardPort=",
		"requestedDashboardPort=",
		"autoPort=fallback",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, got)
		}
	}
}

func TestRun_StartupDiagnosticsStaySilentWhenVerboseDisabled(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubFactoryService{run: func(context.Context) error { return nil }}, nil
	}

	var diagnostics bytes.Buffer
	err := Run(context.Background(), RunConfig{
		DisableDefaultRecording: true,
		Diagnostics:             &diagnostics,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if diagnostics.Len() != 0 {
		t.Fatalf("diagnostics = %q, want empty when verbose is disabled", diagnostics.String())
	}
}

func TestRun_VerboseNamedFactoryDiagnosticsReportPrecedenceWithoutPayloadContent(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubFactoryService{run: func(context.Context) error { return nil }}, nil
	}

	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	resolution := &factoryconfig.NamedFactoryResolution{
		Name:               "alpha",
		FactoryDir:         "/tmp/project/factory/alpha",
		Source:             factoryconfig.NamedFactoryResolutionSourceProjectLocal,
		ProjectRoot:        "/tmp/project/factory",
		GlobalRoot:         "/tmp/home/.you-agent-factory/factories",
		PrecedenceDecision: factoryconfig.NamedFactoryPrecedenceDecisionProjectOverGlobal,
	}

	var diagnostics bytes.Buffer
	err := Run(context.Background(), RunConfig{
		Dir:                     resolution.FactoryDir,
		DisableDefaultRecording: true,
		Verbose:                 true,
		Diagnostics:             &diagnostics,
		Logger:                  logger,
		NamedFactoryResolution:  resolution,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	gotDiagnostics := diagnostics.String()
	for _, want := range []string{
		"run named-factory resolution",
		`name="alpha"`,
		"source=project-local",
		`resolvedFactoryDir="/tmp/project/factory/alpha"`,
		"precedence=project-local-over-global",
	} {
		if !strings.Contains(gotDiagnostics, want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, gotDiagnostics)
		}
	}
	if strings.Contains(gotDiagnostics, "secret") {
		t.Fatalf("diagnostics leaked payload content: %s", gotDiagnostics)
	}

	resolvedLogs := observed.FilterMessage("named factory resolved").All()
	if len(resolvedLogs) != 1 {
		t.Fatalf("named factory resolved logs = %d, want 1", len(resolvedLogs))
	}
	resolvedContext := resolvedLogs[0].ContextMap()
	if got := resolvedContext["named_factory_precedence_decision"]; got != "project-local-over-global" {
		t.Fatalf("resolved log precedence = %#v, want project-local-over-global", got)
	}
	if got := resolvedContext["named_factory_resolution_source"]; got != "project-local" {
		t.Fatalf("resolved log source = %#v, want project-local", got)
	}
	if observed.FilterMessage("named factory precedence selected").Len() != 1 {
		t.Fatalf("named factory precedence logs = %d, want 1", observed.FilterMessage("named factory precedence selected").Len())
	}
}

func TestRun_LogsBuiltInNamedFactoryMaterialization(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubFactoryService{run: func(context.Context) error { return nil }}, nil
	}

	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	resolution := &factoryconfig.NamedFactoryResolution{
		Name:               "@you/tts",
		FactoryDir:         "/tmp/home/.you-agent-factory/factories/@you/tts",
		Source:             factoryconfig.NamedFactoryResolutionSourceBuiltin,
		ProjectRoot:        "/tmp/project/factory",
		GlobalRoot:         "/tmp/home/.you-agent-factory/factories",
		PrecedenceDecision: factoryconfig.NamedFactoryPrecedenceDecisionNone,
	}

	err := Run(context.Background(), RunConfig{
		Dir:                     resolution.FactoryDir,
		DisableDefaultRecording: true,
		Logger:                  logger,
		NamedFactoryResolution:  resolution,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	materializedLogs := observed.FilterMessage("named factory built-in materialized").All()
	if len(materializedLogs) != 1 {
		t.Fatalf("built-in materialized logs = %d, want 1", len(materializedLogs))
	}
	context := materializedLogs[0].ContextMap()
	if got := context["named_factory_name"]; got != "@you/tts" {
		t.Fatalf("built-in log name = %#v, want @you/tts", got)
	}
	if got := context["named_factory_target_dir"]; got != "/tmp/home/.you-agent-factory/factories/@you/tts" {
		t.Fatalf("built-in log target dir = %#v", got)
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

func TestRun_ShutdownOutputReportsResolvedAutoGeneratedRecordingPath(t *testing.T) {
	originalBuilder := buildFactoryService
	originalDefaultRecordPath := defaultLiveRunRecordPath
	defer func() {
		buildFactoryService = originalBuilder
		defaultLiveRunRecordPath = originalDefaultRecordPath
	}()

	defaultLiveRunRecordPath = func() (string, error) {
		return "/tmp/.you-agent-factory/recordings/2026-05/2026-05-23/factory-session-" + defaultRecordPathSessionToken + "-184512-uuid-1.json", nil
	}
	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubFactoryService{run: func(context.Context) error { return nil }}, nil
	}

	var out bytes.Buffer
	err := Run(context.Background(), RunConfig{
		StartupOutput: &out,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(out.String(), "Recording saved: /tmp/.you-agent-factory/recordings/2026-05/2026-05-23/factory-session-~default-184512-uuid-1.json") {
		t.Fatalf("shutdown output = %q, want resolved recording path", out.String())
	}
}

func TestRun_WireBuiltFactoryServiceServesStatus(t *testing.T) {
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	writeRunWireTestWorkerAgentsMD(t, dir, "worker-a")
	writeRunWireTestWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("Close listener: %v", err)
	}

	originalStartAPIServer := startAPIServer
	originalServeFactoryAPIServer := serveFactoryAPIServer
	defer func() {
		startAPIServer = originalStartAPIServer
		serveFactoryAPIServer = originalServeFactoryAPIServer
	}()
	serveFactoryAPIServer = compose.ServeAPIServer
	startAPIServer = func(
		ctx context.Context,
		runtime apisurface.APISurface,
		bindPort int,
		logger *zap.Logger,
		markReady func(),
	) error {
		apiListener, listenErr := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", bindPort))
		if listenErr != nil {
			return listenErr
		}
		return serveAPIServer(ctx, runtime, bindPort, logger, markReady, apiListener)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- Run(ctx, RunConfig{
			Dir:                        dir,
			Continuously:               true,
			MockWorkersEnabled:         true,
			Port:                       port,
			SuppressDashboardRendering: true,
			Logger:                     zap.NewNop(),
		})
	}()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	var status factoryapi.StatusResponse
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/status")
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			t.Fatalf("read /status body: %v", readErr)
		}
		if resp.StatusCode != http.StatusOK {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if err := json.Unmarshal(body, &status); err != nil {
			t.Fatalf("decode /status: %v", err)
		}
		break
	}
	if status.FactoryState == "" {
		t.Fatalf("GET /status factory_state empty after polling %s/status", baseURL)
	}

	cancel()
	if err := <-runErrCh; err != nil && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}
}

func TestRun_WireBuiltFactoryServiceListsModels(t *testing.T) {
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	writeRunWireTestWorkerAgentsMD(t, dir, "worker-a")
	writeRunWireTestWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("Close listener: %v", err)
	}

	originalStartAPIServer := startAPIServer
	originalServeFactoryAPIServer := serveFactoryAPIServer
	defer func() {
		startAPIServer = originalStartAPIServer
		serveFactoryAPIServer = originalServeFactoryAPIServer
	}()
	serveFactoryAPIServer = compose.ServeAPIServer
	startAPIServer = func(
		ctx context.Context,
		runtime apisurface.APISurface,
		bindPort int,
		logger *zap.Logger,
		markReady func(),
	) error {
		apiListener, listenErr := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", bindPort))
		if listenErr != nil {
			return listenErr
		}
		return serveAPIServer(ctx, runtime, bindPort, logger, markReady, apiListener)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- Run(ctx, RunConfig{
			Dir:                        dir,
			Continuously:               true,
			MockWorkersEnabled:         true,
			Port:                       port,
			SuppressDashboardRendering: true,
			Logger:                     zap.NewNop(),
		})
	}()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	var models factoryapi.ListModelsResponse
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/models")
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			t.Fatalf("read /models body: %v", readErr)
		}
		if resp.StatusCode != http.StatusOK {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if err := json.Unmarshal(body, &models); err != nil {
			t.Fatalf("decode /models: %v", err)
		}
		break
	}
	if len(models.Results) == 0 {
		t.Fatalf("models list empty after startup, want configured model summaries")
	}

	cancel()
	select {
	case err := <-runErrCh:
		if err != nil && err != context.Canceled {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for Run to exit after cancel")
	}
}

func writeRunWireTestWorkerAgentsMD(t *testing.T, factoryDir, workerName string) {
	t.Helper()
	workerDir := filepath.Join(factoryDir, "workers", workerName)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("create worker dir: %v", err)
	}
	content := "---\ntype: MODEL_WORKER\nmodel: claude-3-5-haiku-20241022\n---\nYou are a helpful assistant.\n"
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write worker AGENTS.md: %v", err)
	}
}

func writeRunWireTestWorkstationAgentsMD(t *testing.T, factoryDir, workstationName string) {
	t.Helper()
	workstationDir := filepath.Join(factoryDir, "workstations", workstationName)
	if err := os.MkdirAll(workstationDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	content := "---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n"
	if err := os.WriteFile(filepath.Join(workstationDir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}

func TestHumanProgressRenderableType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		eventType responsestream.EventType
		want      bool
	}{
		{eventType: responsestream.EventTypeProgress, want: true},
		{eventType: responsestream.EventTypeStarted, want: true},
		{eventType: responsestream.EventTypeTextDelta, want: false},
		{eventType: responsestream.EventTypeFinalText, want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(string(tc.eventType), func(t *testing.T) {
			t.Parallel()
			if got := humanProgressRenderableType(tc.eventType); got != tc.want {
				t.Fatalf("humanProgressRenderableType(%q) = %t, want %t", tc.eventType, got, tc.want)
			}
		})
	}
}

func TestBoundedHumanProgressPayload(t *testing.T) {
	t.Parallel()

	payload := strings.Repeat("a", maxHumanProgressLineBytes+10)
	got := boundedHumanProgressPayload(payload)
	if len([]byte(got)) > maxHumanProgressLineBytes+3 {
		t.Fatalf("bounded payload too long: %d bytes", len([]byte(got)))
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("bounded payload = %q, want ellipsis suffix", got)
	}
}

func TestFormatCompactionNotice(t *testing.T) {
	t.Parallel()

	got := formatCompactionNotice(responsestream.CompactionSummary{
		Reason:                responsestream.CompactionReasonCoalesced,
		DroppedSequenceCount:  2,
		FirstRetainedSequence: 5,
	})
	if got != "stream coalesced (2 earlier events omitted)" {
		t.Fatalf("notice = %q", got)
	}
}
