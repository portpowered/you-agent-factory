package run

import (
	"io"
	"strings"

	factoryruntimecli "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/cli"
)

const (
	InvocationErrorCodeFailed       = factoryruntimecli.InvocationErrorCodeFailed
	InvocationErrorCodeCancelled    = factoryruntimecli.InvocationErrorCodeCancelled
	InvocationErrorCodeTimeout      = factoryruntimecli.InvocationErrorCodeTimeout
	CurrentFactoryNotFoundCode      = factoryruntimecli.CurrentFactoryNotFoundCode
	CurrentFactoryInvalidCode       = factoryruntimecli.CurrentFactoryInvalidCode
	InvocationOutputConflictCode    = factoryruntimecli.InvocationOutputConflictCode
	InvocationOutputUnsupportedCode = factoryruntimecli.InvocationOutputUnsupportedCode
	ServerBindFailedCode            = factoryruntimecli.ServerBindFailedCode
	InvocationOutputPrimaryResult   = factoryruntimecli.InvocationOutputPrimaryResult
	InvocationOutputResponseStream  = factoryruntimecli.InvocationOutputResponseStream
)

type InvocationError = factoryruntimecli.InvocationError

// WriteInvocationError renders the stable clean-invocation failure contract to
// stderr. It returns true when err matched an invocation contract error.
func WriteInvocationError(w io.Writer, err error, quiet bool) bool {
	return factoryruntimecli.WriteInvocationError(w, err, quiet)
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
	return factoryruntimecli.ValidateInvocationOutputMode(factoryruntimecli.ValidateInvocationOutputModeRequest{
		InvocationOutputMode: cfg.InvocationOutputMode,
		Continuously:         cfg.Continuously,
		InvocationMode:       invocationMode,
	})
}

func isResponseStreamOutputMode(mode string) bool {
	return strings.TrimSpace(mode) == InvocationOutputResponseStream
}
