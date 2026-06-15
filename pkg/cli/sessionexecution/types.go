package sessionexecution

import (
	"io"
)

// ExecutionMode selects synchronous or asynchronous durable session start.
type ExecutionMode string

const (
	ExecutionModeSync  ExecutionMode = "sync"
	ExecutionModeAsync ExecutionMode = "async"
)

// StartConfig holds CLI inputs for one durable Factory Session execution start.
type StartConfig struct {
	Mode              ExecutionMode
	RequestID         string
	FactoryID         string
	WorkflowName      string
	WorkflowFile      string
	ArgsJSON          string
	PolicyJSON        string
	PolicyHash        string
	WaitTimeoutMillis *int64
	CancelOnTimeout   bool
	PositionalArgs    []string
	Stdin             io.Reader
	StdinIsTTY        func() bool
}
