package agy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/workers/agypty"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/workers/provider/commandenv"
)

// Adapter implements the provider-neutral final-only contract for configured Agy
// workers using the native Go PTY boundary.
type Adapter struct {
	FactoryRoot string
	Executable  string
	Allocator   agypty.PTYAllocator
	SessionCfg  agypty.SessionConfig
}

// Option configures one Agy adapter.
type Option func(*Adapter)

// WithAllocator injects a PTY allocator for hermetic tests.
func WithAllocator(allocator agypty.PTYAllocator) Option {
	return func(a *Adapter) {
		a.Allocator = allocator
	}
}

// WithExecutable overrides the resolved Agy binary path.
func WithExecutable(executable string) Option {
	return func(a *Adapter) {
		a.Executable = executable
	}
}

// WithSessionConfig overrides default PTY session limits.
func WithSessionConfig(cfg agypty.SessionConfig) Option {
	return func(a *Adapter) {
		a.SessionCfg = cfg
	}
}

// NewAdapter constructs an Agy adapter bound to one factory root.
func NewAdapter(factoryRoot string, opts ...Option) *Adapter {
	adapterValue := &Adapter{
		FactoryRoot: strings.TrimSpace(factoryRoot),
		Executable:  string(modelprovider.Agy),
		SessionCfg:  agypty.DefaultSessionConfig(),
	}
	for _, opt := range opts {
		opt(adapterValue)
	}
	return adapterValue
}

// NewAdapterWithAllocator constructs an Agy adapter with its required native
// PTY edge made explicit. Provider CommandRunner injection is deliberately not
// part of this contract.
func NewAdapterWithAllocator(factoryRoot string, allocator agypty.PTYAllocator, opts ...Option) (*Adapter, error) {
	if allocator == nil {
		return nil, fmt.Errorf("construct Agy provider: PTY allocator is required")
	}
	opts = append(opts, WithAllocator(allocator))
	return NewAdapter(factoryRoot, opts...), nil
}

func (a *Adapter) Identity() adapter.Identity {
	return adapter.Identity(modelprovider.Agy)
}

func (a *Adapter) BuildCommand(_ context.Context, input adapter.CommandContext) (adapter.CommandBuildResult, error) {
	req := input.Request
	executable, err := a.resolveExecutable()
	if err != nil {
		return adapter.CommandBuildResult{}, err
	}
	workDir, err := a.resolveWorkDir(req.WorkingDirectory)
	if err != nil {
		return adapter.CommandBuildResult{}, err
	}
	argv, err := agypty.BuildArgv(agypty.ArgvSpec{
		Executable: executable,
		Subcommand: []string{"chat"},
		Flags:      a.buildFlags(req),
		Prompt:     req.UserMessage,
	})
	if err != nil {
		return adapter.CommandBuildResult{}, err
	}
	if err := agypty.ValidateArgv(argv); err != nil {
		return adapter.CommandBuildResult{}, err
	}

	command := workerprocess.SubprocessRequestBase(req.Dispatch)
	command.Command = argv[0]
	command.Args = argv[1:]
	command.Env = commandenv.Build(req.EnvVars)
	command.WorkDir = workDir
	command.InputTokens = append([]any(nil), req.InputTokens...)
	if req.WorkerType != "" {
		command.WorkerType = req.WorkerType
	}
	if req.WorkstationType != "" {
		command.WorkstationName = req.WorkstationType
	}
	if req.ProjectID != "" {
		command.ProjectID = req.ProjectID
	}
	return adapter.CommandBuildResult{Request: command}, nil
}

func (a *Adapter) buildFlags(req workerexecution.ProviderInferenceRequest) []string {
	flags := []string{"--headless"}
	if model := strings.TrimSpace(req.Model); model != "" {
		flags = append(flags, "--model", model)
	}
	if sessionID := strings.TrimSpace(req.SessionID); sessionID != "" {
		flags = append(flags, "--session", sessionID)
	}
	return flags
}

func (a *Adapter) resolveExecutable() (string, error) {
	executable := strings.TrimSpace(a.Executable)
	if executable == "" {
		executable = string(modelprovider.Agy)
	}
	executable = filepath.Clean(executable)
	if executable == "" || executable == "." {
		return "", fmt.Errorf("agy: executable is required")
	}
	if !filepath.IsAbs(executable) && !strings.ContainsAny(executable, `/\`) {
		if lookedUp, err := exec.LookPath(executable); err == nil {
			executable = lookedUp
		} else {
			return executable, nil
		}
	}
	info, err := os.Stat(executable)
	if err == nil {
		if info.IsDir() {
			return "", fmt.Errorf("agy: executable must not be a directory")
		}
		return executable, nil
	}
	if os.IsNotExist(err) && !filepath.IsAbs(executable) {
		return executable, nil
	}
	if os.IsNotExist(err) {
		return "", fmt.Errorf("%w: %s", ErrMissingExecutable, executable)
	}
	return "", fmt.Errorf("agy: resolve executable: %w", err)
}

func (a *Adapter) resolveWorkDir(rawPath string) (string, error) {
	factoryRoot := strings.TrimSpace(a.FactoryRoot)
	if factoryRoot == "" {
		return "", fmt.Errorf("agy: factory root is required")
	}
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		rawPath = "."
	}
	return agypty.ResolveWorkspaceDir(factoryRoot, rawPath)
}

func (a *Adapter) NewDecoder(_ context.Context, _ adapter.DecoderContext) (adapter.Decoder, error) {
	return finalOnlyDecoder{}, nil
}

func (a *Adapter) ParseFinal(ctx context.Context, input adapter.FinalParseContext) (adapter.FinalParseResult, error) {
	return parseFinalOnly(ctx, input)
}

func (a *Adapter) Capabilities(context.Context, adapter.CapabilityContext) (adapter.CapabilityResult, error) {
	return adapter.CapabilityResult{Capabilities: adapter.Capabilities{
		MessageSnapshots: true,
		FinalOnly:        true,
	}}, nil
}

func (a *Adapter) ClassifyFailure(ctx context.Context, input adapter.FailureContext) adapter.FailureResult {
	_ = a
	return classifyFailure(ctx, input)
}

// PTYAllocator returns the configured allocator or the platform default.
func (a *Adapter) PTYAllocator() (agypty.PTYAllocator, error) {
	if a.Allocator != nil {
		return a.Allocator, nil
	}
	return agypty.NewDefaultPlatformAllocatorFactory().NewAllocator()
}

// PTYRunner returns a runner that shares this adapter's PTY configuration.
func (a *Adapter) PTYRunner() (PTYRunner, error) {
	allocator, err := a.PTYAllocator()
	if err != nil {
		return PTYRunner{}, err
	}
	return NewPTYRunner(allocator, a.SessionCfg)
}

var _ adapter.Adapter = (*Adapter)(nil)
