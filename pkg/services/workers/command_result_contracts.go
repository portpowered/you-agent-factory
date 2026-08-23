package workers

import workerniagnostics "github.com/portpowered/infinite-you/pkg/services/workers/internal/diagnostics"

// DispatchCancellationReason identifies why a dispatch stopped before it
// produced a business Work result. The reason is deliberately separate from
// WorkOutcome so a superseded execution cannot be mistaken for a failure.
type DispatchCancellationReason string

const (
	DispatchCancellationReasonCanceled   DispatchCancellationReason = "CANCELED"
	DispatchCancellationReasonSuperseded DispatchCancellationReason = "SUPERSEDED"
)

// DispatchCancellation is the explicit lifecycle fact carried across
// execution, Worker Session, and Factory Runtime result boundaries.
type DispatchCancellation struct {
	Reason DispatchCancellationReason `json:"reason"`
}

func (value *DispatchCancellation) Clone() *DispatchCancellation {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
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
