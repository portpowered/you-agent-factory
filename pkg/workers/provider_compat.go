package workers

import (
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
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
type InferenceProgressPublisher = workerprovider.InferenceProgressPublisher
type RecordingProvider = workerprovider.RecordingProvider
type RecordingProviderOption = workerprovider.RecordingProviderOption
type MockWorkerCommandRunner = mockworker.MockWorkerCommandRunner

const (
	providerSessionKindSessionID       = "session_id"
	providerSessionKindConversationID  = "conversation_id"
	providerSessionKindResponseID      = "response_id"
	codexWindowsProcessFailureExitCode = 4294967295
)

func NewProviderError(errorType workerexecution.WorkFailureType, message string, cause error) *ProviderError {
	return workerprovider.NewProviderError(errorType, message, cause)
}

func NewProviderErrorWithSession(errorType workerexecution.WorkFailureType, message string, cause error, session *workerexecution.ProviderSessionMetadata) *ProviderError {
	return workerprovider.NewProviderErrorWithSession(errorType, message, cause, session)
}
