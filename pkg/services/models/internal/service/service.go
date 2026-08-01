package service

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/legacyhost"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	"go.uber.org/zap"
)

// ErrInvalidDependencies classifies model-service construction failures.
var ErrInvalidDependencies = errors.New("model service dependencies are invalid")

// Service owns model catalog, readiness, and pull behavior.
type Service struct {
	runtimeConfigLookup models.RuntimeConfigLoader
	host                modelhost.Host
	assetPuller         localmodels.AssetPuller
	loggerValue         *zap.Logger
	clock               func() time.Time
	pullMetrics         modelseffects.PullMetricsRecorder
}

// NewService constructs a model-domain service after validating every required
// collaborator. It applies only model-service-local defaults and performs no
// process-mode selection or application lifecycle work.
func NewService(
	runtimeConfig models.RuntimeConfigLoader,
	host modelhost.Host,
	assetPuller localmodels.AssetPuller,
	logger *zap.Logger,
	clock func() time.Time,
	pullMetrics modelseffects.PullMetricsRecorder,
) (*Service, error) {
	if runtimeConfig == nil {
		return nil, missingDependencyError("runtime configuration lookup")
	}
	if isNilDependency(host) {
		return nil, missingDependencyError("model host")
	}
	if isNilDependency(assetPuller) {
		return nil, missingDependencyError("model asset puller")
	}
	if logger == nil {
		return nil, missingDependencyError("logger")
	}
	if clock == nil {
		return nil, missingDependencyError("clock")
	}
	return &Service{
		runtimeConfigLookup: runtimeConfig,
		host:                host,
		assetPuller:         assetPuller,
		loggerValue:         logger,
		clock:               clock,
		pullMetrics:         pullMetrics,
	}, nil
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

func (s *Service) runtimeConfig() *models.RuntimeConfig {
	if s == nil || s.runtimeConfigLookup == nil {
		return nil
	}
	return s.runtimeConfigLookup()
}

func (s *Service) modelHost() modelhost.Host {
	if s == nil {
		return nil
	}
	return s.host
}

func (s *Service) now() time.Time {
	return s.clock()
}
