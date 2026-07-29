package agy

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/commanddispatch"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/commandenv"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// ExecutableDependencies are policy-free host effects used to resolve the
// executable selected by Providers execution policy.
type ExecutableDependencies struct {
	Locator   platformprocess.ExecutableLocator
	Inspector platformfilesystem.PathInspector
}

// PTYEffectOptions configures the native Agy PTY execution effect.
type PTYEffectOptions struct {
	FactoryRoot   string
	Allocator     agypty.PTYAllocator
	Executable    string
	SessionConfig agypty.SessionConfig
	ExecutableDependencies
}

// NewPTYEffect binds one PTY allocator to the Agy adapter.
func NewPTYEffect(options PTYEffectOptions) Effect {
	if options.Allocator == nil {
		return nil
	}
	executable := strings.TrimSpace(options.Executable)
	if executable == "" {
		executable = string(providers.IDAgy)
	}
	sessionConfig := options.SessionConfig
	if sessionConfig == (agypty.SessionConfig{}) {
		sessionConfig = agypty.DefaultSessionConfig()
	}
	factoryRoot := strings.TrimSpace(options.FactoryRoot)
	return EffectFunc(func(
		ctx context.Context,
		request providers.ExecuteRequest,
		observe func([]byte) error,
	) (EffectResult, error) {
		started := time.Now()
		sessionRef := sessionRefFromRequest(request.ResumeSession)
		launch, err := buildPTYLaunch(request, ptyLaunchConfig{
			factoryRoot:   factoryRoot,
			executable:    executable,
			locator:       options.Locator,
			inspector:     options.Inspector,
			sessionConfig: sessionConfig,
		})
		if err != nil {
			return EffectResult{SessionRef: sessionRef}, orchestrationFailure(err)
		}
		result, runErr := runPTY(ctx, options.Allocator, launch, sessionConfig, observe)
		cleaned := cleanedPTYText(result)
		effectResult := EffectResult{
			DurationMillis: time.Since(started).Milliseconds(),
			SessionRef:     sessionRef,
			CapturedStdout: []byte(cleaned),
		}
		if runErr != nil {
			return effectResult, nativePTYError(ctx, result, runErr)
		}
		return effectResult, nil
	})
}

type ptyLaunchConfig struct {
	factoryRoot   string
	executable    string
	locator       platformprocess.ExecutableLocator
	inspector     platformfilesystem.PathInspector
	sessionConfig agypty.SessionConfig
}

type ptyLaunch struct {
	launch agypty.ProcessLaunch
}

func buildPTYLaunch(
	request providers.ExecuteRequest,
	config ptyLaunchConfig,
) (ptyLaunch, error) {
	executable, err := resolveExecutable(
		config.executable,
		config.locator,
		config.inspector,
	)
	if err != nil {
		return ptyLaunch{}, err
	}
	workDir, err := resolveWorkDir(config.factoryRoot, request.WorkingDirectory)
	if err != nil {
		return ptyLaunch{}, err
	}
	argv, err := agypty.BuildArgv(agypty.ArgvSpec{
		Executable: executable,
		Subcommand: []string{"chat"},
		Flags:      buildFlags(request),
		Prompt:     request.UserMessage,
	})
	if err != nil {
		return ptyLaunch{}, err
	}
	if err := agypty.ValidateArgv(argv); err != nil {
		return ptyLaunch{}, err
	}
	command := commanddispatch.WorkersCommand(request, workers.CommandRequest{
		Command: argv[0],
		Args:    argv[1:],
		Env: commandenv.Build(
			request.ProcessEnvironment,
			request.EnvVars,
		),
		WorkDir: workDir,
	})
	if len(request.InputTokens) > 0 {
		command.InputTokens = append([]any(nil), request.InputTokens...)
	}
	return ptyLaunch{
		launch: agypty.ProcessLaunch{
			Executable: command.Command,
			Argv:       append([]string{command.Command}, command.Args...),
			WorkDir:    command.WorkDir,
			Env:        command.Env,
		},
	}, nil
}

func buildFlags(request providers.ExecuteRequest) []string {
	flags := []string{"--headless"}
	if model := strings.TrimSpace(request.Model); model != "" {
		flags = append(flags, "--model", model)
	}
	if request.ResumeSession != nil {
		if sessionID := strings.TrimSpace(request.ResumeSession.ID); sessionID != "" {
			flags = append(flags, "--session", sessionID)
		}
	}
	return flags
}

func resolveExecutable(
	executable string,
	locator platformprocess.ExecutableLocator,
	inspector platformfilesystem.PathInspector,
) (string, error) {
	if locator == nil {
		return "", fmt.Errorf("agy: executable locator is required")
	}
	if inspector == nil {
		return "", fmt.Errorf("agy: executable path inspector is required")
	}
	executable = strings.TrimSpace(executable)
	if executable == "" {
		executable = string(providers.IDAgy)
	}
	executable = filepath.Clean(executable)
	if executable == "" || executable == "." {
		return "", fmt.Errorf("agy: executable is required")
	}
	if !filepath.IsAbs(executable) && !strings.ContainsAny(executable, `/\`) {
		if lookedUp, err := locator.LookPath(executable); err == nil {
			executable = lookedUp
		} else {
			return executable, nil
		}
	}
	info, err := inspector.Stat(executable)
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

func resolveWorkDir(factoryRoot, rawPath string) (string, error) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		rawPath = "."
	}
	factoryRoot = strings.TrimSpace(factoryRoot)
	if factoryRoot == "" {
		normalized := filepath.Clean(filepath.FromSlash(rawPath))
		if filepath.IsAbs(normalized) {
			return normalized, nil
		}
		return "", fmt.Errorf("agy: factory root is required")
	}
	return agypty.ResolveWorkspaceDir(factoryRoot, rawPath)
}

func runPTY(
	ctx context.Context,
	allocator agypty.PTYAllocator,
	launch ptyLaunch,
	sessionConfig agypty.SessionConfig,
	observe func([]byte) error,
) (agypty.SessionResult, error) {
	if err := ctx.Err(); err != nil {
		return agypty.SessionResult{}, err
	}
	session, err := allocator.Allocate(ctx, launch.launch, sessionConfig)
	if err != nil {
		return agypty.SessionResult{}, err
	}
	defer session.Close()

	result, err := session.Run(ctx)
	if err != nil {
		return result, err
	}
	cleaned := cleanedPTYText(result)
	commandResult := agypty.SessionResult{
		ExitCode:    result.ExitCode,
		CleanedText: cleaned,
		RawBytes:    result.RawBytes,
		TimedOut:    result.TimedOut,
	}
	if result.TimedOut {
		commandResult.ExitCode = 124
		return commandResult, fmt.Errorf("%w", agypty.ErrSessionTimedOut)
	}
	if len(cleaned) > 0 {
		if observeErr := observe([]byte(cleaned)); observeErr != nil {
			return commandResult, observeErr
		}
	}
	if commandResult.ExitCode != 0 {
		return commandResult, fmt.Errorf("%w: exit code %d", agypty.ErrNonzeroExit, commandResult.ExitCode)
	}
	return commandResult, nil
}

func cleanedPTYText(result agypty.SessionResult) string {
	if len(result.RawBytes) > 0 {
		return agypty.CleanTerminal(result.RawBytes)
	}
	return result.CleanedText
}
