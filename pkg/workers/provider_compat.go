package workers

import (
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
)

type Provider = workerprovider.Provider
type ProviderError = workerprovider.ProviderError
type ProviderErrorCorpus = workerprovider.ProviderErrorCorpus
type ProviderErrorCorpusEntry = workerprovider.ProviderErrorCorpusEntry
type ScriptWrapProvider = workerprovider.ScriptWrapProvider
type ScriptWrapProviderOption = workerprovider.ScriptWrapProviderOption
type InferenceEventRecorder = workerprovider.InferenceEventRecorder
type RecordingProvider = workerprovider.RecordingProvider
type RecordingProviderOption = workerprovider.RecordingProviderOption
type ModelProvider = workerprovider.ModelProvider

const (
	ModelProviderClaude   = workerprovider.ModelProviderClaude
	ModelProviderCodex    = workerprovider.ModelProviderCodex
	ModelProviderGemini   = workerprovider.ModelProviderGemini
	ModelProviderKiro     = workerprovider.ModelProviderKiro
	ModelProviderCursor   = workerprovider.ModelProviderCursor
	ModelProviderOpenCode = workerprovider.ModelProviderOpenCode

	providerSessionKindSessionID       = "session_id"
	providerSessionKindConversationID  = "conversation_id"
	providerSessionKindResponseID      = "response_id"
	codexWindowsProcessFailureExitCode = 4294967295
)

func WithSkipPermissions(skip bool) ScriptWrapProviderOption {
	return workerprovider.WithSkipPermissions(skip)
}

func WithProviderLogger(logger logging.Logger) ScriptWrapProviderOption {
	return workerprovider.WithProviderLogger(logger)
}

func WithProviderCommandRunner(runner CommandRunner) ScriptWrapProviderOption {
	return workerprovider.WithProviderCommandRunner(runner)
}

func NewScriptWrapProvider(opts ...ScriptWrapProviderOption) *ScriptWrapProvider {
	return workerprovider.NewScriptWrapProvider(opts...)
}

func ContainsStopToken(output, stopToken string) bool {
	return workerprovider.ContainsStopToken(output, stopToken)
}

func NewProviderError(errorType interfaces.ProviderErrorType, message string, cause error) *ProviderError {
	return workerprovider.NewProviderError(errorType, message, cause)
}

func NewProviderErrorWithSession(errorType interfaces.ProviderErrorType, message string, cause error, session *interfaces.ProviderSessionMetadata) *ProviderError {
	return workerprovider.NewProviderErrorWithSession(errorType, message, cause, session)
}

func ClassifyProviderFailure(err *ProviderError) interfaces.ProviderFailureDecision {
	return workerprovider.ClassifyProviderFailure(err)
}

func ProviderFailureDecisionFromMetadata(metadata *interfaces.ProviderFailureMetadata) interfaces.ProviderFailureDecision {
	return workerprovider.ProviderFailureDecisionFromMetadata(metadata)
}

func ProviderFailureMetadataFromError(err *ProviderError) *interfaces.ProviderFailureMetadata {
	return workerprovider.WorkFailureMetadataFromError(err)
}

func LoadProviderErrorCorpus() (ProviderErrorCorpus, error) {
	return workerprovider.LoadProviderErrorCorpus()
}

func WithRecordingProviderClock(now func() time.Time) RecordingProviderOption {
	return workerprovider.WithRecordingProviderClock(now)
}

func NewRecordingProvider(inner Provider, recorder InferenceEventRecorder, opts ...RecordingProviderOption) *RecordingProvider {
	return workerprovider.NewRecordingProvider(inner, recorder, opts...)
}
