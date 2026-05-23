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
	"github.com/portpowered/infinite-you/pkg/service"
	"go.uber.org/zap"
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
