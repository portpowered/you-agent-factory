package service

import "testing"

// These tiny records preserve attribution branches formerly exercised only by
// the large streaming/GC fixture. Scale is not needed to prove provider policy.
func TestMetricProviderAttributionIgnoresUnrelatedMetrics(t *testing.T) {
	builder := newMetricProviderAttributionBuilder()
	builder.add(RuntimeMetricRecord{"metric_name": "worker.completed", "provider": "provider-a", "dispatch_id": "dispatch-a"})
	if len(builder.result().byDispatch) != 0 {
		t.Fatal("unrelated metric contributed provider attribution")
	}
	var absent *metricProviderAttributionBuilder
	absent.add(RuntimeMetricRecord{})
}

func TestMetricProviderAttributionUsesProviderWithoutDispatch(t *testing.T) {
	attribution := newMetricProviderAttributionBuilder().result()
	if got := attribution.providerFor(RuntimeMetricRecord{"provider": "provider-a"}); got != "provider-a" {
		t.Fatalf("provider = %q, want provider-a", got)
	}
	if !isProviderAttributionMetric("provider.completed") {
		t.Fatal("provider completion must contribute attribution")
	}
}
