package application

import (
	"context"
	"fmt"
	"sync"

	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
)

// ProviderRegistry is the narrow immutable provider identity projection
// retained by the application process. The provider domain implements it
// without reversing the initializer dependency direction.
type ProviderRegistry interface {
	CanonicalIdentity(string) (string, error)
}

// ProcessLifecycle is the exact application-owned cleanup role for inert
// services that retain resources lazily while commands execute.
type ProcessLifecycle interface {
	Close(context.Context) error
}

// Process is the inert, behavior-bearing process entrypoint assembled by
// Wire. It retains only its command/lifecycle roles, immutable provider
// authority, and the production ACP server, not runtime configuration,
// service graphs, or edge bundles; the lazy Initializer owns runtime
// construction after CLI parsing.
type Process struct {
	commandFactory processcontract.CommandFactory
	initializer    processcontract.Initializer
	providers      ProviderRegistry
	lifecycle      ProcessLifecycle
	acpServer      processcontract.ACPServer
	workerReader   processcontract.WorkerRecordingReader
	detachedOps    processcontract.DetachedOperationsCapability
	runtimeMetrics processcontract.RuntimeMetricsQueryCapability
	executionOpen  processcontract.ExecutionRuntimeOpeningCapability
	runtimeCosts   processcontract.RuntimeCostsQueryCapability
	mcpServer      processcontract.MCPServerFactory
}

// invocationCancellation is owned by one Process.Execute call. It is kept
// behind the neutral initializer contract so a hosted control can request
// cancellation without receiving a context callback or a package-global
// function.
type invocationCancellation struct {
	once   sync.Once
	cancel context.CancelFunc
}

func (c *invocationCancellation) Cancel() {
	if c == nil || c.cancel == nil {
		return
	}
	c.once.Do(c.cancel)
}

func NewProcess(
	commandFactory processcontract.CommandFactory,
	initializer processcontract.Initializer,
	providers ProviderRegistry,
	lifecycle ProcessLifecycle,
	acpServer processcontract.ACPServer,
	workerReader processcontract.WorkerRecordingReader,
	detachedOps processcontract.DetachedOperationsCapability,
	runtimeMetrics processcontract.RuntimeMetricsQueryCapability,
	executionOpen processcontract.ExecutionRuntimeOpeningCapability,
) (*Process, error) {
	return newProcess(commandFactory, initializer, providers, lifecycle, acpServer, workerReader, detachedOps, runtimeMetrics, executionOpen, nil, nil)
}

// NewProcessWithRuntimeCostsAndExecution constructs the canonical process
// with both the Costs query and durable Factory Session opening capabilities.
func NewProcessWithRuntimeCostsAndExecution(
	commandFactory processcontract.CommandFactory,
	initializer processcontract.Initializer,
	providers ProviderRegistry,
	lifecycle ProcessLifecycle,
	acpServer processcontract.ACPServer,
	workerReader processcontract.WorkerRecordingReader,
	detachedOps processcontract.DetachedOperationsCapability,
	runtimeMetrics processcontract.RuntimeMetricsQueryCapability,
	executionOpen processcontract.ExecutionRuntimeOpeningCapability,
	runtimeCosts processcontract.RuntimeCostsQueryCapability,
) (*Process, error) {
	return newProcess(commandFactory, initializer, providers, lifecycle, acpServer, workerReader, detachedOps, runtimeMetrics, executionOpen, runtimeCosts, nil)
}

// NewProcessWithRuntimeCostsAndExecutionAndMCP constructs the canonical
// process with durable Factory Session opening and the root-owned MCP server
// factory capability.
func NewProcessWithRuntimeCostsAndExecutionAndMCP(
	commandFactory processcontract.CommandFactory,
	initializer processcontract.Initializer,
	providers ProviderRegistry,
	lifecycle ProcessLifecycle,
	acpServer processcontract.ACPServer,
	workerReader processcontract.WorkerRecordingReader,
	detachedOps processcontract.DetachedOperationsCapability,
	runtimeMetrics processcontract.RuntimeMetricsQueryCapability,
	executionOpen processcontract.ExecutionRuntimeOpeningCapability,
	runtimeCosts processcontract.RuntimeCostsQueryCapability,
	mcpServer processcontract.MCPServerFactory,
) (*Process, error) {
	return newProcess(commandFactory, initializer, providers, lifecycle, acpServer, workerReader, detachedOps, runtimeMetrics, executionOpen, runtimeCosts, mcpServer)
}

func newProcess(
	commandFactory processcontract.CommandFactory,
	initializer processcontract.Initializer,
	providers ProviderRegistry,
	lifecycle ProcessLifecycle,
	acpServer processcontract.ACPServer,
	workerReader processcontract.WorkerRecordingReader,
	detachedOps processcontract.DetachedOperationsCapability,
	runtimeMetrics processcontract.RuntimeMetricsQueryCapability,
	executionOpen processcontract.ExecutionRuntimeOpeningCapability,
	runtimeCosts processcontract.RuntimeCostsQueryCapability,
	mcpServer processcontract.MCPServerFactory,
) (*Process, error) {
	if providers == nil {
		return nil, fmt.Errorf("construct application process: provider registry is required")
	}
	if lifecycle == nil {
		return nil, fmt.Errorf("construct application process: lifecycle is required")
	}
	return &Process{
		commandFactory: commandFactory,
		initializer:    initializer,
		providers:      providers,
		lifecycle:      lifecycle,
		acpServer:      acpServer,
		workerReader:   workerReader,
		detachedOps:    detachedOps,
		runtimeMetrics: runtimeMetrics,
		executionOpen:  executionOpen,
		runtimeCosts:   runtimeCosts,
		mcpServer:      mcpServer,
	}, nil
}

// Close releases resources retained by services during prior Execute calls.
// It is safe to call more than once after active command invocations stop.
func (p *Process) Close(ctx context.Context) error {
	if p == nil || p.lifecycle == nil {
		return nil
	}
	return p.lifecycle.Close(ctx)
}

// ProviderRegistry returns the immutable provider authority composed for this
// process. It is exposed for embedding and customer-scale verification.
func (p *Process) ProviderRegistry() ProviderRegistry {
	if p == nil {
		return nil
	}
	return p.providers
}

// ACPServer returns the production ACP stdio server Wire composed from this
// same process's canonical Chat Sessions authority. It is exposed for
// embedding and customer-scale verification; construction alone performs no
// I/O, so returning it starts no connection.
func (p *Process) ACPServer() processcontract.ACPServer {
	if p == nil {
		return nil
	}
	return p.acpServer
}

// WorkerRecordingReader returns the neutral detached Worker snapshot reader
// composed for this process. The application process does not depend on the
// Recordings service; the root boundary owns decoding into its public value.
func (p *Process) WorkerRecordingReader() processcontract.WorkerRecordingReader {
	if p == nil {
		return nil
	}
	return p.workerReader
}

// DetachedOperations returns the opaque Factory Sessions capability composed
// for this process. The root package owns its typed public projection.
func (p *Process) DetachedOperations() processcontract.DetachedOperationsCapability {
	if p == nil {
		return nil
	}
	return p.detachedOps
}

// RuntimeMetricsQuery returns the opaque read-only Factory Runtime metrics
// query composed for this process. The root package owns its typed public
// projection.
func (p *Process) RuntimeMetricsQuery() processcontract.RuntimeMetricsQueryCapability {
	if p == nil {
		return nil
	}
	return p.runtimeMetrics
}

// ExecutionRuntimeOpening returns the canonical Factory Sessions durable
// execution opening capability composed for this process.
func (p *Process) ExecutionRuntimeOpening() processcontract.ExecutionRuntimeOpeningCapability {
	if p == nil {
		return nil
	}
	return p.executionOpen
}

// RuntimeCostsQuery returns the opaque stateless Costs capability composed for
// this process. pkg/root owns the typed public projection.
func (p *Process) RuntimeCostsQuery() processcontract.RuntimeCostsQueryCapability {
	if p == nil {
		return nil
	}
	return p.runtimeCosts
}

// MCPServerFactory returns the canonical MCP transport factory composed by
// Wire. The factory is inert until a caller supplies an opened execution.
func (p *Process) MCPServerFactory() processcontract.MCPServerFactory {
	if p == nil {
		return nil
	}
	return p.mcpServer
}

// Execute constructs and runs one fresh command tree using invocation-local
// process boundaries. The Process remains reusable for later invocations.
func (p *Process) Execute(input Input) error {
	normalized, err := normalize(input)
	if err != nil {
		return err
	}
	if p == nil || p.initializer == nil {
		return fmt.Errorf("execute application process: initializer is required")
	}
	processCtx, stop := p.initializer.ProcessContext(normalized.context)
	if processCtx == nil || stop == nil {
		return fmt.Errorf("execute application process: initializer returned an invalid process context")
	}
	defer stop()
	ctx, cancel := context.WithCancel(processCtx)
	defer cancel()
	cancellation := &invocationCancellation{cancel: cancel}
	if p.commandFactory == nil {
		return fmt.Errorf("execute application process: command factory is required")
	}
	ctx = processcontract.WithWorkingDirectory(ctx, normalized.workingDir)
	ctx = processcontract.WithStdinTTY(ctx, normalized.stdinIsTTY)
	ctx = processcontract.WithStdoutTTY(ctx, normalized.stdoutIsTTY)
	ctx = processcontract.WithStderrTTY(ctx, normalized.stderrIsTTY)
	return p.commandFactory.ExecuteCommand(processcontract.CommandInvocation{
		Arguments: normalized.argumentsCopy(), Stdin: normalized.stdin,
		Stdout: normalized.stdout, Stderr: normalized.stderr, Context: ctx,
		Cancellation: cancellation,
		HomeDir:      func() (string, error) { return homeDir(normalized) },
		LookupEnv:    normalized.lookupEnv, Initializer: p.initializer,
	})
}
