package workers

import (
	"context"
	"errors"
	"time"
)

const (
	DefaultPTYMaxCaptureBytes = 4 * 1024 * 1024
	MaxPTYMaxCaptureBytes     = 16 * 1024 * 1024
	DefaultPTYIdleTimeout     = 30 * time.Second
	DefaultPTYHardTimeout     = 10 * time.Minute
)

var (
	ErrPTYUnsupportedPlatform = errors.New("agypty: platform PTY allocation is not supported")
	ErrPTYAllocationFailed    = errors.New("agypty: PTY allocation failed")
	ErrPTYSessionTimedOut     = errors.New("agypty: session timed out")
	ErrPTYNonzeroExit         = errors.New("agypty: process exited with nonzero status")
	ErrPTYClockRequired       = errors.New("agypty: clock is required")
	ErrPTYHostRequired        = errors.New("agypty: native PTY host is required")
)

// PTYSessionConfig carries bounded capture and timeout policy for one session.
type PTYSessionConfig struct {
	MaxCaptureBytes int
	IdleTimeout     time.Duration
	HardTimeout     time.Duration
}

func DefaultPTYSessionConfig() PTYSessionConfig {
	return PTYSessionConfig{
		MaxCaptureBytes: DefaultPTYMaxCaptureBytes,
		IdleTimeout:     DefaultPTYIdleTimeout,
		HardTimeout:     DefaultPTYHardTimeout,
	}
}

// PTYProcessLaunch is the typed subprocess description for one PTY-backed run.
type PTYProcessLaunch struct {
	Executable string
	Argv       []string
	WorkDir    string
	Env        []string
}

// PTYSessionResult is the observable outcome after cleanup.
type PTYSessionResult struct {
	ExitCode    int
	RawBytes    []byte
	CleanedText string
	TimedOut    bool
	CapacityHit bool
}

type PTYKind int

const (
	PTYKindUnknown PTYKind = iota
	PTYKindPOSIX
	PTYKindConPTY
)

func (kind PTYKind) String() string {
	switch kind {
	case PTYKindPOSIX:
		return "posix"
	case PTYKindConPTY:
		return "conpty"
	default:
		return "unknown"
	}
}

// PTYAllocator opens a platform PTY for one supervised child process.
type PTYAllocator interface {
	Allocate(context.Context, PTYProcessLaunch, PTYSessionConfig) (PTYSession, error)
}

// PTYSession is the seam for bounded capture, timeout signaling, and cleanup.
type PTYSession interface {
	Run(context.Context) (PTYSessionResult, error)
	Close() error
}

// MockPTYAllocator is a hermetic root-contract implementation for peer tests.
type MockPTYAllocator struct {
	Sessions []*MockPTYSession
	Result   PTYSessionResult
	Err      error
}

func (allocator *MockPTYAllocator) Allocate(
	_ context.Context,
	launch PTYProcessLaunch,
	config PTYSessionConfig,
) (PTYSession, error) {
	if allocator.Err != nil {
		return nil, allocator.Err
	}
	session := &MockPTYSession{Launch: launch, Config: config, Result: allocator.Result}
	allocator.Sessions = append(allocator.Sessions, session)
	return session, nil
}

type MockPTYSession struct {
	Launch  PTYProcessLaunch
	Config  PTYSessionConfig
	Result  PTYSessionResult
	RunErr  error
	Closed  bool
	RunCall int
}

func (session *MockPTYSession) Run(context.Context) (PTYSessionResult, error) {
	session.RunCall++
	if session.RunErr != nil {
		return PTYSessionResult{}, session.RunErr
	}
	return session.Result, nil
}

func (session *MockPTYSession) Close() error {
	session.Closed = true
	return nil
}
