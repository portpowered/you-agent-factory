package models

// HostDiagnosticLogger receives structured model-host lifecycle diagnostics.
type HostDiagnosticLogger interface {
	Info(string, map[string]string)
	Warn(string, map[string]string)
}

// HostMetricsRecorder receives model-host counter emissions.
type HostMetricsRecorder interface {
	RecordMetric(string, map[string]string)
}
