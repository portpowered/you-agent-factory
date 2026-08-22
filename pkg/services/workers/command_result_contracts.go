package workers

import workerniagnostics "github.com/portpowered/infinite-you/pkg/services/workers/internal/diagnostics"

// CommandResult is the observable worker subprocess result.
type CommandResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

const (
	RedactedCommandEnvValue              = workerniagnostics.RedactedCommandEnvValue
	MetadataOnlyCommandEnvValue          = workerniagnostics.MetadataOnlyCommandEnvValue
	CommandEnvClassificationSafe         = workerniagnostics.CommandEnvClassificationSafe
	CommandEnvClassificationRedacted     = workerniagnostics.CommandEnvClassificationRedacted
	CommandEnvClassificationMetadataOnly = workerniagnostics.CommandEnvClassificationMetadataOnly
)

type CommandEnvDiagnosticProjection = workerniagnostics.CommandEnvDiagnosticProjection

var ProjectCommandEnvForDiagnostics = workerniagnostics.ProjectCommandEnvForDiagnostics
