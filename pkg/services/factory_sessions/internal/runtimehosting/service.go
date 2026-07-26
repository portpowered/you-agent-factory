// Package runtimehosting owns the process-hosting phase for a started Factory
// Session runtime.
package runtimehosting

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"go.uber.org/zap"
)

const startupReadabilityDelay = 250 * time.Millisecond

// Service hosts the API edge and completes the startup protocol for one
// already-started Factory Session runtime.
type Service struct {
	start platformhttpserver.Starter
}

// New constructs an inert runtime-host operation from the exact external HTTP
// effects selected by Wire.
func New(start platformhttpserver.Starter) *Service {
	return &Service{start: start}
}

// Run hosts the transport until cancellation or runtime completion.
func (service *Service) Run(
	ctx context.Context,
	handler http.Handler,
	runtime roles.LifecycleRuntime,
	logger *zap.Logger,
	request factorysessions.RuntimeHostRequest,
	observer factorysessions.RuntimeHostObserver,
) error {
	if runtime == nil {
		return errors.New("host Factory Session runtime: lifecycle runtime is required")
	}
	if hosted := runtime.CurrentRuntimeBundle(); hosted != nil {
		logger = hosted.RuntimeLogger()
	}

	transportCtx, cancel := context.WithCancel(ctx)
	var transport sync.WaitGroup
	defer func() {
		cancel()
		transport.Wait()
	}()

	bound := make(chan platformhttpserver.Binding, 1)
	apiExit := service.startAPI(transportCtx, &transport, handler, request, logger, bound)
	serviceMode := factoryruntime.RuntimeModeOrDefault(request.RuntimeMode) == interfaces.RuntimeModeService
	binding, err := service.waitForStartupReadability(
		ctx, serviceMode, request.WorkFile, apiExit, request.Port, bound,
	)
	if err != nil {
		return runtime.FailStartup(err)
	}
	if binding.Port > 0 {
		request.Port = binding.Port
	}
	if err := runtime.CompleteStartup(ctx); err != nil {
		return err
	}
	if binding.Port > 0 && observer != nil {
		observer(factorysessions.RuntimeHostBinding{Port: binding.Port})
	}
	logStartup(logger, runtime.CurrentRuntimeBundle(), request)
	if err := runtime.WaitForRuntime(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("factory run: %w", err)
	}
	return nil
}

func (service *Service) startAPI(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	handler http.Handler,
	request factorysessions.RuntimeHostRequest,
	logger *zap.Logger,
	bound chan<- platformhttpserver.Binding,
) <-chan error {
	if service == nil || service.start == nil || request.Port <= 0 {
		return nil
	}
	exit := make(chan error, 1)
	sidecars.Add(1)
	go func() {
		defer sidecars.Done()
		err := service.start(ctx, platformhttpserver.StartRequest{
			Handler: handler, Port: request.Port, AutoPort: request.AutoPort, Logger: logger,
			OnBound: func(binding platformhttpserver.Binding) {
				bound <- binding
			},
		})
		exit <- err
		close(exit)
		if err != nil && logger != nil {
			logger.Error("API server error", zap.Error(err))
		}
	}()
	return exit
}

func (service *Service) waitForStartupReadability(
	ctx context.Context,
	serviceMode bool,
	workFile string,
	apiExit <-chan error,
	port int,
	bound <-chan platformhttpserver.Binding,
) (platformhttpserver.Binding, error) {
	if port <= 0 {
		return platformhttpserver.Binding{}, nil
	}
	if service == nil || service.start == nil {
		return platformhttpserver.Binding{}, errors.New("host Factory Session runtime: HTTP starter is required")
	}
	var binding platformhttpserver.Binding
	select {
	case binding = <-bound:
	case err := <-apiExit:
		return platformhttpserver.Binding{}, factoryruntime.StartupReadinessError(err)
	case <-ctx.Done():
		return platformhttpserver.Binding{}, ctx.Err()
	}
	if !serviceMode || workFile == "" {
		return binding, nil
	}

	timer := time.NewTimer(startupReadabilityDelay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return binding, nil
	case err := <-apiExit:
		return platformhttpserver.Binding{}, factoryruntime.StartupReadinessError(err)
	case <-ctx.Done():
		return platformhttpserver.Binding{}, ctx.Err()
	}
}

func logStartup(
	logger *zap.Logger,
	runtime factoryruntime.HostedInstance,
	request factorysessions.RuntimeHostRequest,
) {
	if logger == nil || runtime == nil {
		return
	}
	diagnostics := runtime.RuntimeDiagnostics()
	if diagnostics.Path == "" {
		return
	}
	logger.Info("factory started",
		zap.String("dir", request.Directory),
		zap.String("runtime_log_path", diagnostics.Path),
		zap.String("runtime_log_root", diagnostics.RootDir),
		zap.String("runtime_log_start_time_utc", diagnostics.StartTimeUTC.UTC().Format(time.RFC3339Nano)),
		zap.String("runtime_log_appender", logging.RuntimeLogAppenderZapRollingFile),
		zap.Int("runtime_log_max_size_mb", diagnostics.MaxSizeMB),
		zap.Int("runtime_log_max_backups", diagnostics.MaxBackups),
		zap.Int("runtime_log_max_age_days", diagnostics.MaxAgeDays),
		zap.Bool("runtime_log_compress", diagnostics.Compress),
		zap.String("runtime_env_log_channel", logging.RuntimeEnvLogChannelRecord),
		zap.String("runtime_success_command_output", logging.RuntimeSuccessCommandOutputPolicy),
		zap.String("runtime_failure_command_output", logging.RuntimeFailureCommandOutputPolicy),
		zap.String("runtime_verbose_command_output", logging.RuntimeVerboseCommandOutputPolicy),
		zap.String("record_command_diagnostics", logging.RuntimeRecordCommandDiagnosticsMode),
		zap.String("runtime_mode", string(factoryruntime.RuntimeModeOrDefault(request.RuntimeMode))),
		zap.Bool("mock-workers", request.MockWorkers),
		zap.Int("port", request.Port),
	)
}

var _ roles.RuntimeHostOperation = (*Service)(nil)
