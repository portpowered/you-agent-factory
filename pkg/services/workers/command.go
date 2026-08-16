package workers

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/work"
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
	// ExecutionLogger is the request-scoped command log sink installed by
	// Workers Execute and carried with the subprocess request itself. A
	// process-scoped runner reads it per call and never retains it, so
	// concurrent Factory Sessions cannot share or overwrite each other's
	// runtime log. It is intentionally excluded from serialized payloads.
	ExecutionLogger logging.Logger `json:"-"`
}
