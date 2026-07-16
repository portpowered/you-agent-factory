package sessionexecution

import (
	"context"
	"io"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
)

// ExecutionMode selects synchronous or asynchronous durable session start.
type ExecutionMode string

const (
	ExecutionModeSync  ExecutionMode = "sync"
	ExecutionModeAsync ExecutionMode = "async"
)

// ExecutionBackendConfig selects which shared durable execution service backs
// workflow CLI start and inspection commands.
type ExecutionBackendConfig struct {
	Provider    string
	ProjectRoot string
}

// ServiceRequest contains the transport-normalized inputs needed by the wiring
// layer to construct one durable Factory Session execution collaborator.
type ServiceRequest struct {
	ExecutionBackendConfig
	FixtureCatalogPath string
	ChildExecutorMode  string
}

// ServiceOwner couples a durable execution collaborator to the resources that
// keep it alive. The command that receives an owner must close it when that
// command finishes.
type ServiceOwner interface {
	factorysessionexecution.Service
	Close() error
}

// ServiceBuilder constructs an owned durable execution collaborator outside
// the CLI transport boundary.
type ServiceBuilder func(context.Context, ServiceRequest) (ServiceOwner, error)

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
	ChildExecutorMode string
	PositionalArgs    []string
	Stdin             io.Reader
	StdinIsTTY        func() bool
}
