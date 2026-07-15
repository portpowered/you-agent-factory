// Package terminalpolicy resolves one quiet/normal/verbose terminal-output mode
// per CLI invocation and exposes coherent writer and logger-sink decisions.
package terminalpolicy

import (
	"errors"
	"io"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"go.uber.org/zap"
)

// Mode is the resolved terminal-output mode for one CLI invocation.
type Mode string

const (
	ModeQuiet   Mode = "quiet"
	ModeNormal  Mode = "normal"
	ModeVerbose Mode = "verbose"
)

// Options carries the CLI flags that resolve terminal-output mode.
type Options struct {
	Quiet   bool
	Verbose bool
	Debug   bool
}

// Policy is the resolved terminal-output contract for one CLI invocation.
type Policy struct {
	mode     Mode
	debug    bool
	resolved bool
}

// LoggerBuilder constructs the structured logger selected for verbose mode.
// The CLI composition root owns the concrete builder and injects it here.
type LoggerBuilder func(verbose, debug bool) (*zap.Logger, error)

// Resolve returns the single terminal-output mode for one CLI invocation.
// Quiet wins over verbose/debug. Debug implies verbose diagnostics channels.
func Resolve(opts Options) Policy {
	if opts.Quiet {
		return Policy{mode: ModeQuiet, resolved: true}
	}
	if opts.Verbose || opts.Debug {
		return Policy{mode: ModeVerbose, debug: opts.Debug, resolved: true}
	}
	return Policy{mode: ModeNormal, resolved: true}
}

// Resolved reports whether the policy was produced by Resolve.
func (p Policy) Resolved() bool {
	return p.resolved
}

// Mode returns the resolved terminal-output mode.
func (p Policy) Mode() Mode {
	if p.mode == "" {
		return ModeNormal
	}
	return p.mode
}

// DebugEnabled reports whether lower-level command diagnostics are enabled.
func (p Policy) DebugEnabled() bool {
	return p.debug
}

// VerboseEnabled reports whether verbose runtime and command diagnostics are enabled.
func (p Policy) VerboseEnabled() bool {
	return p.Mode() == ModeVerbose
}

// AllowsHumanTerminalOutput reports whether human-facing startup, progress, and
// operator status lines may write to stdout/stderr.
func (p Policy) AllowsHumanTerminalOutput() bool {
	return p.Mode() != ModeQuiet
}

// AllowsCommandDiagnostics reports whether concise command diagnostics may write
// to the terminal diagnostics channel.
func (p Policy) AllowsCommandDiagnostics() bool {
	return p.Mode() == ModeVerbose
}

// AllowsStructuredLogTerminal reports whether raw structured logger output may
// appear on the terminal. Normal mode keeps structured logs off the terminal.
func (p Policy) AllowsStructuredLogTerminal() bool {
	return p.debug
}

// DiagnosticsWriter returns the terminal diagnostics sink for this policy.
func (p Policy) DiagnosticsWriter(stderr io.Writer) io.Writer {
	if !p.AllowsCommandDiagnostics() {
		return nil
	}
	return stderr
}

// HumanTerminalWriter returns the terminal sink for human-facing startup and
// progress output. Quiet mode suppresses the terminal while leaving callers free
// to route intentional primary results through other writers.
func (p Policy) HumanTerminalWriter(stdout io.Writer) io.Writer {
	if !p.AllowsHumanTerminalOutput() {
		return nil
	}
	return stdout
}

// BuildLogger creates the zap logger for this policy using the logger builder
// supplied by the CLI composition root. Quiet mode discards terminal logger
// output while still allowing BuildRuntimeLogger to tee structured runtime
// records into rolling file sinks.
func (p Policy) BuildLogger(build LoggerBuilder) (*zap.Logger, error) {
	switch p.Mode() {
	case ModeQuiet:
		return zap.NewNop(), nil
	case ModeNormal:
		return logging.BuildTerminalMutedLogger()
	default:
		if build == nil {
			return nil, errors.New("verbose terminal policy requires a logger builder")
		}
		return build(p.VerboseEnabled(), p.DebugEnabled())
	}
}

// DiagnosticsEnabled chooses whether command diagnostics should emit, honoring
// a resolved policy when present and otherwise falling back to legacy verbose.
func DiagnosticsEnabled(policy Policy, legacyVerbose bool) bool {
	if policy.Resolved() {
		return policy.AllowsCommandDiagnostics()
	}
	return legacyVerbose
}
