package agypty

import (
	"context"
	"fmt"
	"strings"
)

// ptyAllocation is the platform-specific PTY resource bundle returned by Allocate.
type ptyAllocation interface {
	Close() error
}

// PTYKind identifies which platform PTY mechanism was allocated.
type PTYKind int

const (
	PTYKindUnknown PTYKind = iota
	PTYKindPOSIX
	PTYKindConPTY
)

func (k PTYKind) String() string {
	switch k {
	case PTYKindPOSIX:
		return "posix"
	case PTYKindConPTY:
		return "conpty"
	default:
		return "unknown"
	}
}

func normalizeSessionConfig(cfg SessionConfig) SessionConfig {
	if cfg.MaxCaptureBytes <= 0 {
		cfg.MaxCaptureBytes = DefaultMaxCaptureBytes
	}
	if cfg.MaxCaptureBytes > MaxMaxCaptureBytes {
		cfg.MaxCaptureBytes = MaxMaxCaptureBytes
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = DefaultIdleTimeout
	}
	if cfg.HardTimeout <= 0 {
		cfg.HardTimeout = DefaultHardTimeout
	}
	return cfg
}

func validateProcessLaunch(launch ProcessLaunch) error {
	if strings.TrimSpace(launch.Executable) == "" {
		return fmt.Errorf("agypty: executable is required")
	}
	if len(launch.Argv) == 0 {
		return fmt.Errorf("agypty: argv is required")
	}
	if err := ValidateArgv(launch.Argv); err != nil {
		return err
	}
	return nil
}

func checkAllocateContext(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func wrapPTYAllocationFailure(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", ErrPTYAllocationFailed, err)
}
