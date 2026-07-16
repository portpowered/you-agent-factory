package runtimehost

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	workerdiagnostics "github.com/portpowered/infinite-you/pkg/workers/diagnostics"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
)

func asRuntimeBundle(bundle any) *factoryRuntimeBundle {
	if bundle == nil {
		return nil
	}
	return bundle.(*factoryRuntimeBundle)
}

func closeRuntimeBundleSinks(logSink *logging.RuntimeLogSink, metricsSink *platformmetrics.RuntimeMetricsSink) error {
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

func modelEventDiagnostics(success *workerexecution.WorkDiagnostics, err error) json.RawMessage {
	var safe *workerdiagnostics.SafeWorkDiagnostics
	if success != nil {
		safe = workerdiagnostics.SafeWorkDiagnosticsFromWorkDiagnostics(success)
	} else {
		var providerErr *workerprovider.ProviderError
		if errors.As(err, &providerErr) {
			safe = workerdiagnostics.SafeWorkDiagnosticsFromWorkDiagnostics(providerErr.Diagnostics)
		}
	}
	payload, encodeErr := workerdiagnostics.SafeWorkDiagnosticsEventPayload(safe)
	if encodeErr != nil || string(payload) == "null" {
		return nil
	}
	return payload
}
