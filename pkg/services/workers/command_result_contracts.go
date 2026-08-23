package workers

import workerniagnostics "github.com/portpowered/infinite-you/pkg/services/workers/internal/diagnostics"

const (
	RedactedCommandEnvValue              = workerniagnostics.RedactedCommandEnvValue
	MetadataOnlyCommandEnvValue          = workerniagnostics.MetadataOnlyCommandEnvValue
	CommandEnvClassificationSafe         = workerniagnostics.CommandEnvClassificationSafe
	CommandEnvClassificationRedacted     = workerniagnostics.CommandEnvClassificationRedacted
	CommandEnvClassificationMetadataOnly = workerniagnostics.CommandEnvClassificationMetadataOnly
)

type CommandEnvDiagnosticProjection = workerniagnostics.CommandEnvDiagnosticProjection

var ProjectCommandEnvForDiagnostics = workerniagnostics.ProjectCommandEnvForDiagnostics
