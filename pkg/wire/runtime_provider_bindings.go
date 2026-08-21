package wire

import (
	"context"
	"errors"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// newConfiguredProvidersService always installs the shell-free Antigravity
// print-mode command effect. An injected serviceedges.Edges.AgyPTYHost exists
// only to satisfy the legacy PTY allocator construction port and must never
// suppress the canonical command adapter; the command effect unconditionally
// takes priority over the legacy PTY effect in executionwire's built-in
// dependency selection.
func newConfiguredProvidersService(
	options []providerswire.Option,
	agyRunner platformprocess.CommandRunner,
) (providers.Service, error) {
	options = append(options, providerswire.WithAgyCommandRunner(agyRunner))
	return providerswire.NewService(options...)
}

// providerPTYAllocator projects the Providers-owned PTY effect into the
// legacy Workers consumer boundary. It is a composition-only bridge; neither
// provider adapters nor the Providers root depend on Workers PTY contracts.
func providerPTYAllocator(allocator providerswire.PTYAllocator) workers.PTYAllocator {
	if allocator == nil {
		return nil
	}
	return providerPTYAllocatorAdapter{allocator: allocator}
}

type providerPTYAllocatorAdapter struct {
	allocator providerswire.PTYAllocator
}

func (adapter providerPTYAllocatorAdapter) Allocate(
	ctx context.Context,
	launch workers.PTYProcessLaunch,
	config workers.PTYSessionConfig,
) (workers.PTYSession, error) {
	session, err := adapter.allocator.Allocate(ctx, providerswire.PTYProcessLaunch{
		Executable: launch.Executable,
		Argv:       append([]string(nil), launch.Argv...),
		WorkDir:    launch.WorkDir,
		Env:        append([]string(nil), launch.Env...),
	}, providerswire.PTYSessionConfig{
		MaxCaptureBytes: config.MaxCaptureBytes,
		IdleTimeout:     config.IdleTimeout,
		HardTimeout:     config.HardTimeout,
	})
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, errors.New("provider PTY allocator returned a nil session")
	}
	return providerPTYSessionAdapter{session: session}, nil
}

type providerPTYSessionAdapter struct {
	session providerswire.PTYSession
}

func (adapter providerPTYSessionAdapter) Run(ctx context.Context) (workers.PTYSessionResult, error) {
	result, err := adapter.session.Run(ctx)
	return workers.PTYSessionResult{
		ExitCode:    result.ExitCode,
		RawBytes:    append([]byte(nil), result.RawBytes...),
		CleanedText: result.CleanedText,
		TimedOut:    result.TimedOut,
		CapacityHit: result.CapacityHit,
	}, err
}

func (adapter providerPTYSessionAdapter) Close() error {
	return adapter.session.Close()
}
