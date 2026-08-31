// Package process defines the neutral handoff between process-owned lifecycle
// selection and customer-facing command adapters.
package process

import (
	"context"
	"encoding/json"
	"io"

	"github.com/portpowered/infinite-you/pkg/initializer"
)

type workingDirectoryContextKey struct{}
type stdinTTYContextKey struct{}
type stdoutTTYContextKey struct{}
type stderrTTYContextKey struct{}

func WithWorkingDirectory(ctx context.Context, dir string) context.Context {
	return context.WithValue(ctx, workingDirectoryContextKey{}, dir)
}

func WorkingDirectory(ctx context.Context) string {
	value, _ := ctx.Value(workingDirectoryContextKey{}).(string)
	return value
}

func WithStdinTTY(ctx context.Context, isTTY bool) context.Context {
	return context.WithValue(ctx, stdinTTYContextKey{}, isTTY)
}

func StdinIsTTY(ctx context.Context) bool {
	value, _ := ctx.Value(stdinTTYContextKey{}).(bool)
	return value
}

func WithStdoutTTY(ctx context.Context, isTTY bool) context.Context {
	return context.WithValue(ctx, stdoutTTYContextKey{}, isTTY)
}

func StdoutIsTTY(ctx context.Context) bool {
	value, _ := ctx.Value(stdoutTTYContextKey{}).(bool)
	return value
}

func WithStderrTTY(ctx context.Context, isTTY bool) context.Context {
	return context.WithValue(ctx, stderrTTYContextKey{}, isTTY)
}

func StderrIsTTY(ctx context.Context) bool {
	value, _ := ctx.Value(stderrTTYContextKey{}).(bool)
	return value
}

type RunIntent struct {
	DefaultInvocation     bool
	Continuous            bool
	APIEnabled            bool
	DashboardEnabled      bool
	WorkerSidecarsEnabled bool
	// Cancellation is the invocation-local authority created by the
	// application process. It is carried explicitly so hosted controls can
	// request the same cancellation observed by the command lifecycle.
	Cancellation initializer.InvocationCancellation
}

type MCPIntent struct {
	FixtureCatalogPath string
	RuntimeBacked      bool
	ProjectRoot        string
	HomeDir            string
	Stdin              io.Reader
	Stdout             io.Writer
}

// RunSelection is one invocation-local CLI run choice. Initializer forwards
// the typed customer intent unchanged; the already-injected selection and
// opening edge returns an inert application with its complete lifecycle plan.
type RunSelection interface {
	Open(context.Context, RunIntent) (initializer.RunApplication, error)
}

type RunHandler func(context.Context, RunIntent, RunSelection) error
type StdioHandler func(context.Context, MCPIntent) error

// StdioApplicationOpener opens one lifecycle-ready stdio application. Product
// selection and service opening stay behind the injected operation; the
// Initializer only activates the returned lifecycle.
type StdioApplicationOpener interface {
	OpenStdio(context.Context, MCPIntent) (initializer.RunApplication, error)
}

type Initializer interface {
	ProcessContext(context.Context) (context.Context, func())
	InitializeSystem(context.Context, string) error
	Run(context.Context, RunIntent, RunSelection) error
	Stdio(context.Context, MCPIntent) error
}

type CommandInvocation struct {
	Arguments    []string
	Stdin        io.Reader
	Stdout       io.Writer
	Stderr       io.Writer
	Context      context.Context
	Cancellation initializer.InvocationCancellation
	HomeDir      func() (string, error)
	LookupEnv    func(string) (string, bool)
	Initializer  Initializer
}

type CommandFactory interface {
	ExecuteCommand(CommandInvocation) error
}

// WorkerRecordingReader is the neutral process capability for loading one
// detached Worker recording snapshot. The application process carries the
// JSON value across the initializer boundary; the Recordings-owned root
// adapter decodes it back into the domain snapshot without exposing a product
// service dependency to pkg/initializer/application.
type WorkerRecordingReader interface {
	LoadWorkerRecording(context.Context, string) (json.RawMessage, error)
}

// DetachedOperationsCapability is the neutral process handoff for the
// Factory Sessions detached operation view. The initializer retains the
// selected capability without importing the Sessions service; pkg/root
// reifies the opaque value at the caller-facing boundary.
type DetachedOperationsCapability interface {
	DetachedOperations() any
}

// RuntimeMetricsQueryCapability is the neutral process handoff for the
// read-only Factory Runtime metrics query. The initializer retains the
// selected capability without importing the Factory Visualization service;
// pkg/root reifies the opaque value at the caller-facing boundary.
type RuntimeMetricsQueryCapability interface {
	RuntimeMetricsQuery() any
}

// ExecutionRuntimeOpeningCapability is the neutral process handoff for the
// canonical Factory Sessions durable-execution opening. The initializer keeps
// the capability opaque; pkg/root reifies its public service-owned contract.
type ExecutionRuntimeOpeningCapability interface {
	ExecutionRuntimeOpening() any
}

// RuntimeCostsQueryCapability is the neutral process handoff for the
// stateless Costs valuation operation. The initializer retains the selected
// capability without importing either Operator Settings or Factory
// Visualization; pkg/root reifies the opaque value at the caller boundary.
type RuntimeCostsQueryCapability interface {
	RuntimeCostsQuery() any
}

// ACPServer is the neutral application-process capability for serving the ACP
// protocol. The transport package supplies the concrete implementation at the
// composition root; the initializer only retains the protocol operation.
type ACPServer interface {
	Serve(context.Context, io.Reader, io.Writer) error
}

// MCPServer is the neutral process capability for serving an already-bound
// Factory Session MCP server over caller-owned stdio streams. The process
// contract carries the protocol role without importing the MCP transport
// implementation into the initializer.
type MCPServer interface {
	ServeStdio(context.Context, io.Reader, io.Writer) error
}

// MCPServerFactory constructs one MCP server around an already-opened,
// caller-selected durable execution. The execution is opaque here so the
// initializer does not depend on Factory Sessions or transport packages.
type MCPServerFactory func(any) (MCPServer, error)

type Functions struct {
	ProcessContextFunc   func(context.Context) (context.Context, func())
	InitializeSystemFunc func(context.Context, string) error
	RunFunc              RunHandler
	StdioFunc            StdioHandler
}

func (f Functions) InitializeSystem(ctx context.Context, homeDir string) error {
	if f.InitializeSystemFunc == nil {
		return nil
	}
	return f.InitializeSystemFunc(ctx, homeDir)
}

func (f Functions) ProcessContext(ctx context.Context) (context.Context, func()) {
	if f.ProcessContextFunc == nil {
		return ctx, func() {}
	}
	return f.ProcessContextFunc(ctx)
}

func (f Functions) Run(ctx context.Context, intent RunIntent, selection RunSelection) error {
	if f.RunFunc == nil {
		return nil
	}
	return f.RunFunc(ctx, intent, selection)
}

func (f Functions) Stdio(ctx context.Context, intent MCPIntent) error {
	if f.StdioFunc == nil {
		return nil
	}
	return f.StdioFunc(ctx, intent)
}
