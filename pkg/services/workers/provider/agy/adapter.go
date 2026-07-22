package agy

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/commandenv"
)

// Adapter implements the provider-neutral final-only contract for configured Agy
// workers using the native Go PTY boundary.
type Adapter struct {
	FactoryRoot string
	Executable  string
	Allocator   agypty.PTYAllocator
	SessionCfg  agypty.SessionConfig
	Locator     platformprocess.ExecutableLocator
	Inspector   platformfilesystem.PathInspector
}

// ExecutableDependencies are the policy-free host effects used to resolve
// the executable selected by Workers policy.
type ExecutableDependencies struct {
	Locator   platformprocess.ExecutableLocator
	Inspector platformfilesystem.PathInspector
}

// NewAdapter constructs an Agy adapter bound to one factory root.
func NewAdapter(factoryRoot string) *Adapter {
	return NewAdapterWithDependencies(factoryRoot, nil, "", agypty.SessionConfig{})
}

// NewAdapterWithDependencies constructs an Agy adapter from direct
// collaborators. Configuration zero values are normalized, while external
// executable effects remain required and fail closed when command building.
func NewAdapterWithDependencies(
	factoryRoot string,
	allocator agypty.PTYAllocator,
	executable string,
	sessionConfig agypty.SessionConfig,
	executableDependencies ...ExecutableDependencies,
) *Adapter {
	if strings.TrimSpace(executable) == "" {
		executable = string(modelprovider.ProviderAgy)
	}
	if sessionConfig == (agypty.SessionConfig{}) {
		sessionConfig = agypty.DefaultSessionConfig()
	}
	adapter := &Adapter{
		FactoryRoot: strings.TrimSpace(factoryRoot),
		Executable:  executable,
		Allocator:   allocator,
		SessionCfg:  sessionConfig,
	}
	if len(executableDependencies) > 0 {
		adapter.Locator = executableDependencies[0].Locator
		adapter.Inspector = executableDependencies[0].Inspector
	}
	return adapter
}

// NewAdapterWithAllocator constructs an Agy adapter with its required native
// PTY edge made explicit. Provider CommandRunner injection is deliberately not
// part of this contract.
func NewAdapterWithAllocator(
	factoryRoot string,
	allocator agypty.PTYAllocator,
	executableDependencies ...ExecutableDependencies,
) (*Adapter, error) {
	if allocator == nil {
		return nil, fmt.Errorf("construct Agy provider: PTY allocator is required")
	}
	return NewAdapterWithDependencies(factoryRoot, allocator, "", agypty.SessionConfig{}, executableDependencies...), nil
}

// NewAdapterWithExecutable constructs an adapter with an explicit executable.
// Composition must still supply the executable effects used by BuildCommand.
func NewAdapterWithExecutable(
	factoryRoot string,
	executable string,
	executableDependencies ...ExecutableDependencies,
) *Adapter {
	return NewAdapterWithDependencies(factoryRoot, nil, executable, agypty.SessionConfig{}, executableDependencies...)
}

func (a *Adapter) Identity() adapter.Identity {
	return adapter.Identity(modelprovider.ProviderAgy)
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
	command.Env = commandenv.Build(req.ProcessEnvironment, req.EnvVars)
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
	if a == nil || a.Locator == nil {
		return "", fmt.Errorf("agy: executable locator is required")
	}
	if a.Inspector == nil {
		return "", fmt.Errorf("agy: executable path inspector is required")
	}
	executable := strings.TrimSpace(a.Executable)
	if executable == "" {
		executable = string(modelprovider.ProviderAgy)
	}
	executable = filepath.Clean(executable)
	if executable == "" || executable == "." {
		return "", fmt.Errorf("agy: executable is required")
	}
	if !filepath.IsAbs(executable) && !strings.ContainsAny(executable, `/\`) {
		if lookedUp, err := a.Locator.LookPath(executable); err == nil {
			executable = lookedUp
		} else {
			return executable, nil
		}
	}
	info, err := a.Inspector.Stat(executable)
	if err == nil {
		if info.IsDir() {
			return "", fmt.Errorf("agy: executable must not be a directory")
		}
		return executable, nil
	}
	if errors.Is(err, fs.ErrNotExist) && !filepath.IsAbs(executable) {
		return executable, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
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
	return nil, errors.New("agy: PTY allocator is required")
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
