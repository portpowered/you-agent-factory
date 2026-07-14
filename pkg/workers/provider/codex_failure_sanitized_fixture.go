package provider

import "github.com/portpowered/infinite-you/pkg/interfaces"

// CodexSanitizedFailureFixture is the safe public projection used by Codex
// alignment tests. It carries only customer-safe fields plus a bounded
// internal-cause excerpt suitable for maintainer diagnostics.
type CodexSanitizedFailureFixture struct {
	Type          interfaces.WorkFailureType   `json:"type"`
	Family        interfaces.WorkFailureFamily `json:"family"`
	Message       string                       `json:"message"`
	Retryable     bool                         `json:"retryable"`
	Terminal      bool                         `json:"terminal"`
	InternalCause string                       `json:"internal_cause,omitempty"`
}

// CodexSanitizedFailureFixtureFromResolution projects one winning Codex failure
// resolution onto the sanitized alignment fixture shape.
func CodexSanitizedFailureFixtureFromResolution(resolution ProviderFailureResolution) CodexSanitizedFailureFixture {
	providerErr := NewProviderErrorFromResult(resolution.Result, ProviderFailureInternalCauseError(resolution.InternalCause))
	decision := WorkFailureDecisionFromProviderError(providerErr)
	return CodexSanitizedFailureFixture{
		Type:          providerErr.Type,
		Family:        providerErr.Family,
		Message:       providerErr.Message,
		Retryable:     decision.Retryable,
		Terminal:      decision.Terminal,
		InternalCause: resolution.InternalCause,
	}
}

// CodexSanitizedFailureFixtureFromProviderError projects a normalized provider
// error onto the sanitized alignment fixture shape.
func CodexSanitizedFailureFixtureFromProviderError(providerErr *ProviderError) CodexSanitizedFailureFixture {
	if providerErr == nil {
		return CodexSanitizedFailureFixture{}
	}
	decision := WorkFailureDecisionFromProviderError(providerErr)
	internalCause := ""
	if providerErr.Cause != nil {
		internalCause = providerErr.Cause.Error()
	}
	return CodexSanitizedFailureFixture{
		Type:          providerErr.Type,
		Family:        providerErr.Family,
		Message:       providerErr.Message,
		Retryable:     decision.Retryable,
		Terminal:      decision.Terminal,
		InternalCause: internalCause,
	}
}
