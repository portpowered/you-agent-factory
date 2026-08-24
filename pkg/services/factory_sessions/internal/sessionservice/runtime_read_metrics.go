package service

import (
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

type runtimeReadMetricsAdapter struct {
	recorder roles.InvocationMetricsRecorder
}

func (adapter runtimeReadMetricsAdapter) RecordRuntimeReadMetric(metric recordings.RuntimeReadMetric) {
	if adapter.recorder == nil {
		return
	}
	labels := make(map[string]string, len(metric.Labels))
	for key, value := range metric.Labels {
		labels[key] = value
	}
	adapter.recorder.RecordInvocationMetric(factorysessions.InvocationMetric{
		Name: metric.Name, Labels: labels,
	})
}

func (fs *SessionRuntime) bindRuntimeReadMetrics(bundle factoryRuntimeBundle) {
	if fs == nil || bundle == nil {
		return
	}
	ledger := runtimeLedgerForReadMetrics(bundle)
	binder, ok := ledger.(recordings.RuntimeReadMetricsBinder)
	if !ok || binder == nil {
		return
	}
	binder.SetRuntimeReadMetricsRecorder(runtimeReadMetricsAdapter{
		recorder: fs.invocationMetricsRecorder,
	})
}

// runtimeLedgerForReadMetrics keeps optional telemetry compatible with legacy
// RuntimeRecord test doubles that promote RecordingLedger from a nil embed.
func runtimeLedgerForReadMetrics(bundle factoryRuntimeBundle) (ledger recordings.Ledger) {
	defer func() {
		if recover() != nil {
			ledger = nil
		}
	}()
	return bundle.RecordingLedger()
}
