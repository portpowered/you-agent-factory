package runtimehost

import (
	"errors"
	"fmt"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
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
