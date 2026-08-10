package run

import (
	"errors"
	"fmt"
	"io"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimecli "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
)

const (
	InvocationErrorCodeFailed       = factoryruntimecli.InvocationErrorCodeFailed
	InvocationErrorCodeCancelled    = factoryruntimecli.InvocationErrorCodeCancelled
	InvocationErrorCodeTimeout      = factoryruntimecli.InvocationErrorCodeTimeout
	CurrentFactoryNotFoundCode      = factoryruntimecli.CurrentFactoryNotFoundCode
	CurrentFactoryInvalidCode       = factoryruntimecli.CurrentFactoryInvalidCode
	InvocationOutputConflictCode    = factoryruntimecli.InvocationOutputConflictCode
	InvocationOutputUnsupportedCode = factoryruntimecli.InvocationOutputUnsupportedCode
	RemoteLocalHostingConflictCode  = factoryruntimecli.RemoteLocalHostingConflictCode
	ServerBindFailedCode            = factoryruntimecli.ServerBindFailedCode
	InvocationOutputPrimaryResult   = factoryruntimecli.InvocationOutputPrimaryResult
	InvocationOutputResponseStream  = factoryruntimecli.InvocationOutputResponseStream
)

type InvocationError = factoryruntimecli.InvocationError

// WriteInvocationError renders the stable clean-invocation failure contract to
// stderr. It returns true when err matched an invocation contract error.
func WriteInvocationError(w io.Writer, err error, quiet bool) bool {
	handled := factoryruntimecli.WriteInvocationError(w, err, quiet)
	if handled {
		clidiag.MarkDiagnosticRendered(w)
	}
	return handled
}

// WriteIncompleteDrainError renders the finite-run failure contract for a
// drained runtime that still owns non-terminal customer Work. This is kept at
// the human CLI boundary so the runtime error remains useful to other callers.
func WriteIncompleteDrainError(w io.Writer, err error) bool {
	var incompleteDrainErr *factoryruntime.IncompleteDrainError
	if !errors.As(err, &incompleteDrainErr) {
		return false
	}
	if w != nil {
		_, _ = fmt.Fprintf(w, "Error: %s\n", incompleteDrainErr.Error())
	}
	return true
}

// MapCurrentFactoryFailure classifies failures from the exact Current Factory
// selection before they cross the public run-command error boundary.
func MapCurrentFactoryFailure(err error) error {
	return factoryruntimecli.MapCurrentFactoryFailure(err)
}

// MapServerFailure classifies terminal listener binding failures at the CLI
// boundary while preserving all other errors for their owning mapper.
func MapServerFailure(err error) error {
	return factoryruntimecli.MapServerFailure(err)
}

// MapInvocationFailure preserves authored invocation errors and classifies
// pre-terminal failures that occurred before an InvocationResponse existed.
func MapInvocationFailure(err error) error {
	return factoryruntimecli.MapInvocationFailure(err)
}

func NormalizeInvocationOutputMode(raw string) (string, error) {
	return factoryruntimecli.NormalizeInvocationOutputMode(raw)
}

// ValidateInvocationOutputSelection rejects competing public stdout selectors.
// JSON plus response-stream is one accepted JSON-stream selection; quiet cannot
// be combined with either global JSON or an explicit --output selection.
func ValidateInvocationOutputSelection(quiet, jsonOutput, explicitOutput bool) error {
	return factoryruntimecli.ValidateInvocationOutputSelection(quiet, jsonOutput, explicitOutput)
}

func validateInvocationOutputMode(cfg RunConfig, invocationMode bool) error {
	return runtimeCLIService(cfg).ValidateInvocationOutputMode(factoryruntimecli.ValidateInvocationOutputModeRequest{
		InvocationOutputMode: cfg.InvocationOutputMode,
		Continuously:         cfg.Continuously,
		InvocationMode:       invocationMode,
	})
}

func isResponseStreamOutputMode(mode string) bool {
	return strings.TrimSpace(mode) == InvocationOutputResponseStream
}
