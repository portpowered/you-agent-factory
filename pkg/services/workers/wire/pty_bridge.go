package wire

import (
	"context"
	"errors"

	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	workersinternal "github.com/portpowered/infinite-you/pkg/services/workers/internal"
)

// adaptPTYAllocator keeps the Workers PTY request/result types below the
// Workers internal boundary. Process composition supplies the Providers-owned
// allocator, while direct invocation tests may supply the owner-private seam.
func adaptPTYAllocator(candidate any) workersinternal.PTYAllocator {
	if candidate == nil {
		return nil
	}
	if allocator, ok := candidate.(workersinternal.PTYAllocator); ok {
		return allocator
	}
	if allocator, ok := candidate.(providerswire.PTYAllocator); ok {
		return providerPTYAllocatorAdapter{allocator: allocator}
	}
	return nil
}

type providerPTYAllocatorAdapter struct {
	allocator providerswire.PTYAllocator
}

func (adapter providerPTYAllocatorAdapter) Allocate(
	ctx context.Context,
	launch workersinternal.PTYProcessLaunch,
	config workersinternal.PTYSessionConfig,
) (workersinternal.PTYSession, error) {
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

func (adapter providerPTYSessionAdapter) Run(ctx context.Context) (workersinternal.PTYSessionResult, error) {
	result, err := adapter.session.Run(ctx)
	return workersinternal.PTYSessionResult{
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
