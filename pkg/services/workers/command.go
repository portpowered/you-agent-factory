package workers

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workerenvdiagnostics "github.com/portpowered/infinite-you/pkg/services/workers/envdiagnostics"
)

// CommandRunner is the Workers-owned subprocess execution port.
type CommandRunner interface {
	Run(context.Context, CommandRequest) (CommandResult, error)
}

// CommandRequest carries worker execution policy and its subprocess inputs.
type CommandRequest struct {
	Command                  string                 `json:"command"`
	Args                     []string               `json:"args,omitempty"`
	Stdin                    []byte                 `json:"stdin,omitempty"`
	Env                      []string               `json:"env,omitempty"`
	WorkDir                  string                 `json:"work_dir,omitempty"`
	DispatchID               string                 `json:"dispatch_id,omitempty"`
	TransitionID             string                 `json:"transition_id,omitempty"`
	WorkerType               string                 `json:"worker_type,omitempty"`
	WorkstationName          string                 `json:"workstation_name,omitempty"`
	ProjectID                string                 `json:"project_id,omitempty"`
	CurrentChainingTraceID   string                 `json:"current_chaining_trace_id,omitempty"`
	PreviousChainingTraceIDs []string               `json:"previous_chaining_trace_ids,omitempty"`
	Execution                work.ExecutionMetadata `json:"execution,omitempty"`
	InputTokens              []any                  `json:"input_tokens,omitempty"`
	InputBindings            map[string][]string    `json:"input_bindings,omitempty"`
}

// CommandResult is the observable worker subprocess result.
type CommandResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

const (
	RedactedCommandEnvValue         = workerenvdiagnostics.RedactedCommandEnvValue
	MetadataOnlyCommandEnvValue     = workerenvdiagnostics.MetadataOnlyCommandEnvValue
	CommandEnvClassificationSafe    = workerenvdiagnostics.CommandEnvClassificationSafe
	CommandEnvClassificationRedacted = workerenvdiagnostics.CommandEnvClassificationRedacted
	CommandEnvClassificationMetadataOnly = workerenvdiagnostics.CommandEnvClassificationMetadataOnly
)

type (
	CommandEnvClassification       = workerenvdiagnostics.CommandEnvClassification
	CommandEnvDiagnosticProjection = workerenvdiagnostics.CommandEnvDiagnosticProjection
)

var (
	ClassifyCommandEnvKey           = workerenvdiagnostics.ClassifyCommandEnvKey
	ProjectCommandEnvForDiagnostics = workerenvdiagnostics.ProjectCommandEnvForDiagnostics
)
