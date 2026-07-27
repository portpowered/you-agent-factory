package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
)

type service struct {
	scopes          runtimescopes.Service
	assets          scopedassets.Service
	processLauncher models.HostProcessLauncher
	hostHTTP        models.HostHTTPDoer
	hostClock       models.HostClock
	hostLogger      models.HostDiagnosticLogger
	hostMetrics     models.HostMetricsRecorder
}

var _ runtimehost.Service = (*service)(nil)

// New constructs an inert Runtime Host that validates and retains injected
// supervision effects without launching subprocesses or starting lifecycle.
func New(
	scopes runtimescopes.Service,
	assets scopedassets.Service,
	processLauncher models.HostProcessLauncher,
	hostHTTP models.HostHTTPDoer,
	hostClock models.HostClock,
	hostLogger models.HostDiagnosticLogger,
	hostMetrics models.HostMetricsRecorder,
) runtimehost.Service {
	return &service{
		scopes:          scopes,
		assets:          assets,
		processLauncher: processLauncher,
		hostHTTP:        hostHTTP,
		hostClock:       hostClock,
		hostLogger:      hostLogger,
		hostMetrics:     hostMetrics,
	}
}

func (s *service) InspectModelHost(
	ctx context.Context,
	request models.InspectModelHostRequest,
) (models.InspectModelHostResult, error) {
	if err := request.Validate(); err != nil {
		return models.InspectModelHostResult{}, err
	}
	if err := hostContextError(ctx); err != nil {
		return models.InspectModelHostResult{}, err
	}
	if s == nil || s.scopes == nil || s.assets == nil {
		return models.InspectModelHostResult{}, models.ErrUnavailable
	}
	if _, err := s.scopes.Resolve(runtimescopes.Reference(request.Scope.String())); err != nil {
		return models.InspectModelHostResult{}, scopeError(err)
	}
	inspection, err := s.assets.InspectRuntimeCache(ctx, models.InspectModelAssetsRequest{
		Scope: request.Scope,
		Name:  request.Name,
	})
	if err != nil {
		return models.InspectModelHostResult{}, err
	}
	return models.InspectModelHostResult{
		Host: hostSnapshotFromAssets(request.Scope, request.Name, inspection),
	}, nil
}

func (s *service) EnsureModelHost(
	context.Context,
	models.EnsureModelHostRequest,
) (models.EnsureModelHostResult, error) {
	return models.EnsureModelHostResult{}, models.ErrUnsupportedOperation
}

func (s *service) StopModelHost(
	context.Context,
	models.StopModelHostRequest,
) (models.StopModelHostResult, error) {
	return models.StopModelHostResult{}, models.ErrUnsupportedOperation
}

func hostSnapshotFromAssets(
	scope models.RuntimeScopeRef,
	modelName string,
	inspection scopedassets.RuntimeCacheInspection,
) models.ModelHostSnapshot {
	snapshot := models.ModelHostSnapshot{
		Scope:       scope,
		ModelName:   modelName,
		Diagnostics: map[string]string{},
	}
	if !inspection.Supported {
		snapshot.ReadinessState = models.ReadinessStateUnsupported
		snapshot.LifecycleState = models.LifecycleStateNotApplicable
		return snapshot
	}
	if inspection.Installed {
		snapshot.ReadinessState = models.ReadinessStateReady
		snapshot.LifecycleState = models.LifecycleStateInstalled
		if inspection.CachePath != "" {
			snapshot.Diagnostics["cachePath"] = inspection.CachePath
		}
		if inspection.Revision != "" {
			snapshot.Diagnostics["revision"] = inspection.Revision
		}
		if inspection.InstalledFileCount > 0 {
			snapshot.Diagnostics["installedFileCount"] = fmt.Sprintf("%d", inspection.InstalledFileCount)
		}
		return snapshot
	}
	snapshot.ReadinessState = models.ReadinessStateMissing
	snapshot.LifecycleState = models.LifecycleStateNotInstalled
	if len(inspection.MissingAssets) > 0 {
		snapshot.Diagnostics["missingAssets"] = strings.Join(inspection.MissingAssets, ",")
	}
	return snapshot
}

func scopeError(err error) error {
	switch {
	case errors.Is(err, runtimescopes.ErrScopeForeign):
		return fmt.Errorf("%w: %v", models.ErrRuntimeScopeForeign, err)
	case errors.Is(err, runtimescopes.ErrScopeClosed):
		return fmt.Errorf("%w: %v", models.ErrRuntimeScopeClosed, err)
	case errors.Is(err, runtimescopes.ErrScopeUnknown):
		return fmt.Errorf("%w: %v", models.ErrRuntimeScopeStale, err)
	default:
		return models.ErrUnavailable
	}
}

func hostContextError(ctx context.Context) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf(
		"%w: %w",
		models.ErrHostCancelled,
		errors.Join(ctx.Err(), context.Cause(ctx)),
	)
}
