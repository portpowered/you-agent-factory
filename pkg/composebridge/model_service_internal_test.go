package composebridge

import (
	"strings"
	"testing"

	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
)

func TestComposeModelServiceRejectsMissingCoreBeforeConstruction(t *testing.T) {
	t.Parallel()

	models, err := composeModelService(nil, &runtimehost.Config{})
	if models != nil || err == nil || !strings.Contains(err.Error(), "runtime core and clock are required") {
		t.Fatalf("composeModelService() = (%v, %v), want missing-core construction error", models, err)
	}
}

func TestModelPullMetricsAdapterMapsMetricToRuntimeBoundary(t *testing.T) {
	t.Parallel()

	recorder := &recordingRuntimePullMetrics{}
	modelPullMetricsAdapter{inner: recorder}.RecordModelPullMetric(modelsservice.PullMetric{
		Name:   "managed_runtime.pull.success",
		Labels: map[string]string{"model": "OMNIVOICE_Q4_K_M"},
	})
	if recorder.metric.Name != "managed_runtime.pull.success" || recorder.metric.Labels["model"] != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("mapped metric = %+v, want exact name and labels", recorder.metric)
	}
}

type recordingRuntimePullMetrics struct {
	metric runtimehost.InvocationMetric
}

func (r *recordingRuntimePullMetrics) RecordModelPullMetric(metric runtimehost.InvocationMetric) {
	r.metric = metric
}
