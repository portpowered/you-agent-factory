package service

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	"go.uber.org/zap"
)

// ErrInvalidDependencies classifies model-service construction failures.
var ErrInvalidDependencies = errors.New("model service dependencies are invalid")

// Dependencies carries runtime inputs for model-domain operations.
// RuntimeConfig, ModelHost, ModelAssetPuller, and ModelInvocationExecutor are
// required. Logger, Clock, ModelPullMetrics, and FactoryRunnerID are optional;
// NewService uses a no-op logger and metrics recorder plus the system clock
// when those collaborators are omitted.
type Dependencies struct {
	RuntimeConfig           func() *factoryconfig.LoadedFactoryConfig
	ModelHost               modelhost.Host
	ModelAssetPuller        localmodels.AssetPuller
	Logger                  *zap.Logger
	Clock                   func() time.Time
	ModelPullMetrics        PullMetricsRecorder
	ModelInvocationExecutor ModelInvocationExecutor
	FactoryRunnerID         string
}

// Service owns direct model catalog, pull, and invocation behavior.
type Service struct {
	deps Dependencies
}

// NewService constructs a model-domain service after validating every required
// collaborator. It applies only model-service-local defaults and performs no
// process-mode selection or application lifecycle work.
func NewService(deps Dependencies) (*Service, error) {
	if deps.RuntimeConfig == nil {
		return nil, missingDependencyError("runtime configuration lookup")
	}
	if deps.RuntimeConfig() == nil {
		return nil, missingDependencyError("runtime configuration")
	}
	if isNilDependency(deps.ModelHost) {
		return nil, missingDependencyError("model host")
	}
	if isNilDependency(deps.ModelAssetPuller) {
		return nil, missingDependencyError("model asset puller")
	}
	if deps.ModelInvocationExecutor == nil {
		return nil, missingDependencyError("model invocation executor")
	}
	if deps.Logger == nil {
		deps.Logger = zap.NewNop()
	}
	if deps.Clock == nil {
		deps.Clock = time.Now
	}
	if deps.ModelPullMetrics == nil {
		deps.ModelPullMetrics = discardPullMetrics{}
	}
	return &Service{deps: deps}, nil
}

func missingDependencyError(name string) error {
	return fmt.Errorf("%w: %s is required", ErrInvalidDependencies, name)
}

func isNilDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

type discardPullMetrics struct{}

func (discardPullMetrics) RecordModelPullMetric(PullMetric) {}

func (s *Service) runtimeConfig() *factoryconfig.LoadedFactoryConfig {
	if s == nil || s.deps.RuntimeConfig == nil {
		return nil
	}
	return s.deps.RuntimeConfig()
}

func (s *Service) modelHost() modelhost.Host {
	if s == nil {
		return nil
	}
	return s.deps.ModelHost
}

func (s *Service) now() time.Time {
	if s == nil || s.deps.Clock == nil {
		return time.Now()
	}
	return s.deps.Clock()
}
