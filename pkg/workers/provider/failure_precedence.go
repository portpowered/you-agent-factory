package provider

import "github.com/portpowered/infinite-you/pkg/interfaces"

// FailureSignalTier orders competing invocation failure signals. Lower tiers
// outrank higher tiers when multiple candidates are present for one outcome.
type FailureSignalTier int

const (
	FailureSignalTierCancelTimeout FailureSignalTier = iota
	FailureSignalTierStructured
	FailureSignalTierStderr
	FailureSignalTierExit
)

// ProviderFailureResolution is the winning provider failure outcome plus a
// bounded internal cause excerpt from the selected signal.
type ProviderFailureResolution struct {
	Result        ProviderFailureResult
	InternalCause string
}

// CompetingFailureSignal is one candidate outcome before shared precedence
// selection. Recognized marks whether the tier produced a known failure class
// rather than an unknown fallback excerpt.
type CompetingFailureSignal struct {
	Tier          FailureSignalTier
	Recognized    bool
	Result        ProviderFailureResult
	InternalCause string
}

// IsRecognizedFailureType reports whether a failure type is a known class.
func IsRecognizedFailureType(failureType interfaces.WorkFailureType) bool {
	return failureType != "" && failureType != interfaces.WorkFailureTypeUnknown
}

// SelectFailureByPrecedence collapses competing signals to one authoritative
// outcome using the shared precedence model:
//
//  1. cancellation and timeout
//  2. recognized structured provider errors
//  3. recognized stderr classification
//  4. unrecognized structured provider errors
//  5. unrecognized stderr classification
//  6. generic process-exit fallback
func SelectFailureByPrecedence(signals []CompetingFailureSignal) (ProviderFailureResolution, bool) {
	if len(signals) == 0 {
		return ProviderFailureResolution{}, false
	}
	if selected, ok := selectFailureForTier(signals, FailureSignalTierCancelTimeout, false); ok {
		return selected, true
	}
	if selected, ok := selectFailureForTier(signals, FailureSignalTierStructured, true); ok {
		return selected, true
	}
	if selected, ok := selectFailureForTier(signals, FailureSignalTierStderr, true); ok {
		return selected, true
	}
	if selected, ok := selectFailureForTier(signals, FailureSignalTierStructured, false); ok {
		return selected, true
	}
	if selected, ok := selectFailureForTier(signals, FailureSignalTierStderr, false); ok {
		return selected, true
	}
	return selectFailureForTier(signals, FailureSignalTierExit, false)
}

func selectFailureForTier(signals []CompetingFailureSignal, tier FailureSignalTier, recognizedOnly bool) (ProviderFailureResolution, bool) {
	var selected ProviderFailureResolution
	var found bool
	for _, signal := range signals {
		if signal.Tier != tier {
			continue
		}
		if recognizedOnly && !signal.Recognized {
			continue
		}
		selected = ProviderFailureResolution{
			Result:        signal.Result,
			InternalCause: signal.InternalCause,
		}
		found = true
	}
	return selected, found
}

// CodexProcessExitFailureLayers exposes the structured ERROR-record,
// stderr-classified, and generic exit outcomes from bounded subprocess output
// without applying structured-stream precedence.
func CodexProcessExitFailureLayers(result CommandResult) (
	structured ProviderFailureResult,
	hasStructured bool,
	stderr ProviderFailureResult,
	hasStderr bool,
	exit ProviderFailureResult,
) {
	streams := []string{
		tailForCodexErrorScan(result.Stderr),
		tailForCodexErrorScan(result.Stdout),
	}
	if failure, ok := lastCodexStructuredFailure(streams); ok {
		structured = failure
		hasStructured = true
	}
	if failure, ok := lastCodexTextFailure(streams, result.ExitCode); ok {
		stderr = failure
		hasStderr = true
	}
	exit = codexProcessExitFallback(result.ExitCode)
	return structured, hasStructured, stderr, hasStderr, exit
}

// CodexProcessExitFailureLayersWithCause returns process-exit failure layers
// plus bounded internal-cause excerpts for structured and stderr signals.
func CodexProcessExitFailureLayersWithCause(result CommandResult) (
	structured ProviderFailureResult,
	structuredCause string,
	hasStructured bool,
	stderr ProviderFailureResult,
	stderrCause string,
	hasStderr bool,
	exit ProviderFailureResult,
) {
	structured, hasStructured, stderr, hasStderr, exit = CodexProcessExitFailureLayers(result)
	if hasStructured {
		structuredCause = codexProcessExitStructuredInternalCause(result)
	}
	if hasStderr {
		stderrCause = codexProcessExitStderrInternalCause(result)
	}
	return structured, structuredCause, hasStructured, stderr, stderrCause, hasStderr, exit
}
