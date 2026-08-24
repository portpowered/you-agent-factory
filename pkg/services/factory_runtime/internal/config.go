package internal

import factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"

// RuntimeFileLoggingPolicy controls whether bundle construction creates a runtime file sink.
type RuntimeFileLoggingPolicy = factory.RuntimeFileLoggingPolicy

const (
	RuntimeFileLoggingPolicyEnabled  = factory.RuntimeFileLoggingPolicyEnabled
	RuntimeFileLoggingPolicyDisabled = factory.RuntimeFileLoggingPolicyDisabled
)

// RuntimeMetricsPolicy controls whether bundle construction creates a runtime metrics sink.
type RuntimeMetricsPolicy = factory.RuntimeMetricsPolicy

const (
	RuntimeMetricsPolicyEnabled  = factory.RuntimeMetricsPolicyEnabled
	RuntimeMetricsPolicyDisabled = factory.RuntimeMetricsPolicyDisabled
)
