package runtimehost

import (
	"errors"
	"fmt"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
)

func asRuntimeBundle(bundle any) *factoryRuntimeBundle {
	if bundle == nil {
		return nil
	}
	return bundle.(*factoryRuntimeBundle)
}

func closeRuntimeBundleSinks(logSink *logging.RuntimeLogSink, metricsSink *logging.RuntimeMetricsSink) error {
	var errs []error
	if logSink != nil {
		if err := logSink.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close runtime log sink: %w", err))
		}
	}
	if metricsSink != nil {
		if err := metricsSink.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close runtime metrics sink: %w", err))
		}
	}
	return errors.Join(errs...)
}

func runtimeLogStartTimeString(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func modelEventDiagnostics(success *interfaces.WorkDiagnostics, err error) *factoryapi.SafeWorkDiagnostics {
	if success != nil {
		return interfaces.GeneratedSafeWorkDiagnosticsFromWorkDiagnostics(success)
	}
	var providerErr *workerprovider.ProviderError
	if errors.As(err, &providerErr) {
		return interfaces.GeneratedSafeWorkDiagnosticsFromWorkDiagnostics(providerErr.Diagnostics)
	}
	return nil
}

// ModelEventDiagnosticsForTest exposes model-event diagnostic projection for tests.
func ModelEventDiagnosticsForTest(success *interfaces.WorkDiagnostics, err error) *factoryapi.SafeWorkDiagnostics {
	return modelEventDiagnostics(success, err)
}

func modelEventErrorClass(err error) string {
	var readinessErr *apisurface.ManagedRuntimeInvocationError
	if errors.As(err, &readinessErr) && readinessErr.ReadinessState != "" {
		return "MANAGED_RUNTIME_" + string(readinessErr.ReadinessState)
	}
	var providerErr *workerprovider.ProviderError
	if errors.As(err, &providerErr) && providerErr.Type != "" {
		return string(providerErr.Type)
	}
	if err == nil {
		return ""
	}
	return "MODEL_EXECUTION_FAILED"
}

// ModelEventErrorClassForTest exposes model-event error classification for tests.
func ModelEventErrorClassForTest(err error) string {
	return modelEventErrorClass(err)
}

// NewRecordingModelRunnerForTest wraps a model runner with event recording for tests.
func NewRecordingModelRunnerForTest(
	inner workers.Runner,
	factoryCfg *interfaces.FactoryConfig,
	workerDef *interfaces.WorkerConfig,
	recorder modelEventRecorder,
	now func() time.Time,
) workers.Runner {
	return newRecordingModelRunner(inner, factoryCfg, workerDef, recorder, now)
}

// ModelEventContextForTest builds factory event context metadata for tests.
func ModelEventContextForTest(request interfaces.RunnerExecutionRequest, eventTime time.Time) factoryapi.FactoryEventContext {
	return modelEventContext(request, eventTime)
}
