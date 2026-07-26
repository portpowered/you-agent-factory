// Package process defines the neutral handoff between process-owned lifecycle
// selection and customer-facing command adapters.
package process

import (
	"context"
	"io"

	"github.com/portpowered/infinite-you/pkg/initializer"
)

type workingDirectoryContextKey struct{}
type stdinTTYContextKey struct{}
type stdoutTTYContextKey struct{}

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

type RunIntent struct {
	DefaultInvocation     bool
	Continuous            bool
	APIEnabled            bool
	DashboardEnabled      bool
	WorkerSidecarsEnabled bool
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
	Arguments   []string
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	Context     context.Context
	HomeDir     func() (string, error)
	LookupEnv   func(string) (string, bool)
	Initializer Initializer
}

type CommandFactory interface {
	ExecuteCommand(CommandInvocation) error
}

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
