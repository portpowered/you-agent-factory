package run

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/service"
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
				Path:         "/tmp/runtime-logs/2026-05/2026-05-29/044503-runtime.log",
				RootDir:      "/tmp/runtime-logs",
				StartTimeUTC: startedAt,
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
		Dir:                        dir,
		Workflow:                   "workflow-1",
		RunnerID:                   "codex",
		Port:                       busyPort,
		AutoPort:                   true,
		DisableDefaultRecording:    true,
		RuntimeLogDir:              "logs/runtime",
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
		`factoryDir="` + dir + `"`,
		`configuredDir="` + dir + `"`,
		"runtimeMode=BATCH",
		`workflow="workflow-1"`,
		"runnerOverride=true",
		"mockWorkers=true",
		"recording=disabled",
		`runtimeLogDir="logs/runtime"`,
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
		FactoryDir:         "/tmp/home/.you-agent-factory/factories/@you%2Ftts",
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
	if got := context["named_factory_target_dir"]; got != "/tmp/home/.you-agent-factory/factories/@you%2Ftts" {
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
