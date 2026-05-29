package workers

import (
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers/mockworker"
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
type ModelProvider = interfaces.ModelProvider
type MockWorkerCommandRunner = mockworker.MockWorkerCommandRunner

const (
	ModelProviderClaude   = interfaces.ModelProviderClaude
	ModelProviderCodex    = interfaces.ModelProviderCodex
	ModelProviderGemini   = interfaces.ModelProviderGemini
	ModelProviderKiro     = interfaces.ModelProviderKiro
	ModelProviderCursor   = interfaces.ModelProviderCursor
	ModelProviderOpenCode = interfaces.ModelProviderOpenCode

	providerSessionKindSessionID       = "session_id"
	providerSessionKindConversationID  = "conversation_id"
	providerSessionKindResponseID      = "response_id"
	codexWindowsProcessFailureExitCode = 4294967295
)

func NewProviderError(errorType interfaces.ProviderErrorType, message string, cause error) *ProviderError {
	return workerprovider.NewProviderError(errorType, message, cause)
}

func NewProviderErrorWithSession(errorType interfaces.ProviderErrorType, message string, cause error, session *interfaces.ProviderSessionMetadata) *ProviderError {
	return workerprovider.NewProviderErrorWithSession(errorType, message, cause, session)
}

func ProviderFailureDecisionFromMetadata(metadata *interfaces.ProviderFailureMetadata) interfaces.ProviderFailureDecision {
	return workerprovider.ProviderFailureDecisionFromMetadata(metadata)
}
