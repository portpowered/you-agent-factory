package workers

import workstationenv "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/envdiagnostics"

// CommandResult is the observable worker subprocess result.
type CommandResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

const (
	RedactedCommandEnvValue              = workstationenv.RedactedCommandEnvValue
	MetadataOnlyCommandEnvValue          = workstationenv.MetadataOnlyCommandEnvValue
	CommandEnvClassificationSafe         = workstationenv.CommandEnvClassificationSafe
	CommandEnvClassificationRedacted     = workstationenv.CommandEnvClassificationRedacted
	CommandEnvClassificationMetadataOnly = workstationenv.CommandEnvClassificationMetadataOnly
)

type CommandEnvDiagnosticProjection = workstationenv.CommandEnvDiagnosticProjection

var ProjectCommandEnvForDiagnostics = workstationenv.ProjectCommandEnvForDiagnostics
