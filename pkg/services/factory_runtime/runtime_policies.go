package factory

import factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

// RuntimeOpeningRequest contains Factory Runtime lifecycle and observability
// values. It intentionally excludes Factory Definition, Worker, Recording,
// Models, and Factory Session policy.
type RuntimeOpeningRequest struct {
	Mode              factorydefinitions.RuntimeMode
	Verbose           bool
	RuntimeInstanceID string
	LogDirectory      string
	FileLoggingPolicy RuntimeFileLoggingPolicy
	LogConfig         RuntimeLogStorageConfig
	MetricsDirectory  string
	MetricsPolicy     RuntimeMetricsPolicy
	MetricsConfig     RuntimeMetricsStorageConfig
}

// RuntimeFileLoggingPolicy controls whether runtime construction creates a
// file-backed log sink.
type RuntimeFileLoggingPolicy string

const (
	RuntimeFileLoggingPolicyEnabled  RuntimeFileLoggingPolicy = "enabled"
	RuntimeFileLoggingPolicyDisabled RuntimeFileLoggingPolicy = "disabled"
)

// RuntimeMetricsPolicy controls whether runtime construction creates a
// file-backed metrics sink.
type RuntimeMetricsPolicy string

const (
	RuntimeMetricsPolicyEnabled  RuntimeMetricsPolicy = "enabled"
	RuntimeMetricsPolicyDisabled RuntimeMetricsPolicy = "disabled"
)
