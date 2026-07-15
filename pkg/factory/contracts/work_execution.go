package factorycontracts

import (
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

type WorkerState = workerexecution.WorkerState
type InferenceResponse = workerexecution.InferenceResponse
type WorkResult = workerexecution.WorkResult
type ProviderSessionMetadata = workerexecution.ProviderSessionMetadata
type WorkOutcome = workerexecution.WorkOutcome
type WorkMetrics = workerexecution.WorkMetrics
type WorkDiagnostics = workerexecution.WorkDiagnostics
type RenderedPromptDiagnostic = workerexecution.RenderedPromptDiagnostic
type ProviderDiagnostic = workerexecution.ProviderDiagnostic
type InvocationDiagnostic = workerexecution.InvocationDiagnostic
type InvocationParameterDiagnostic = workerexecution.InvocationParameterDiagnostic
type CommandDiagnostic = workerexecution.CommandDiagnostic
type PanicDiagnostic = workerexecution.PanicDiagnostic
type WorkFailureFamily = workerexecution.WorkFailureFamily
type WorkFailureType = workerexecution.WorkFailureType
type FailureDetail = workerexecution.FailureDetail
type WorkFailureDecision = workerexecution.WorkFailureDecision
type WorkFailureMetadata = workerexecution.WorkFailureMetadata

const (
	OutcomeAccepted = workerexecution.OutcomeAccepted
	OutcomeContinue = workerexecution.OutcomeContinue
	OutcomeRejected = workerexecution.OutcomeRejected
	OutcomeFailed   = workerexecution.OutcomeFailed

	ProviderResponseMetadataDurationMS    = workerexecution.ProviderResponseMetadataDurationMS
	ProviderResponseMetadataDurationAPIMS = workerexecution.ProviderResponseMetadataDurationAPIMS
	ProviderResponseMetadataInputTokens   = workerexecution.ProviderResponseMetadataInputTokens
	ProviderResponseMetadataOutputTokens  = workerexecution.ProviderResponseMetadataOutputTokens

	WorkFailureFamilyTerminal  = workerexecution.WorkFailureFamilyTerminal
	WorkFailureFamilyRetryable = workerexecution.WorkFailureFamilyRetryable
	WorkFailureFamilyThrottle  = workerexecution.WorkFailureFamilyThrottle

	WorkFailureTypeAuthFailure         = workerexecution.WorkFailureTypeAuthFailure
	WorkFailureTypePermanentBadRequest = workerexecution.WorkFailureTypePermanentBadRequest
	WorkFailureTypeThrottled           = workerexecution.WorkFailureTypeThrottled
	WorkFailureTypeInternalServerError = workerexecution.WorkFailureTypeInternalServerError
	WorkFailureTypeTimeout             = workerexecution.WorkFailureTypeTimeout
	WorkFailureTypeUnknown             = workerexecution.WorkFailureTypeUnknown
	WorkFailureTypeMisconfigured       = workerexecution.WorkFailureTypeMisconfigured
	WorkFailureTypeCommandLineTooLong  = workerexecution.WorkFailureTypeCommandLineTooLong
	WorkFailureTypeMissingExecutable   = workerexecution.WorkFailureTypeMissingExecutable
)

func CanonicalProviderSessionProvider(provider string) string {
	return workerexecution.CanonicalProviderSessionProvider(provider)
}

// ReplayArtifact remains a Factory replay compatibility contract until the
// Factory ownership migration moves the remaining event and projection types.
type ReplayArtifact struct {
	SchemaVersion string                    `json:"schemaVersion"`
	RecordedAt    time.Time                 `json:"recordedAt"`
	Events        []factoryapi.FactoryEvent `json:"events"`
	Factory       factoryapi.Factory        `json:"-"`
	Diagnostics   ReplayDiagnostics         `json:"-"`
	WallClock     *ReplayWallClockMetadata  `json:"-"`
}

type ReplayDiagnostics struct {
	Notes   []string                       `json:"notes,omitempty"`
	Workers map[string]SafeWorkDiagnostics `json:"workers,omitempty"`
}

type ReplayWallClockMetadata struct {
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
}
