package factorydefinitions

import (
	"context"
	"errors"
	"fmt"
	"strings"

	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"
	catalogresource "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/resource"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/authoredmodel/workers"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// Root-owned aliases expose the Factory Definition vocabulary without making
// peer services import implementation subpackages.
type FactoryConfig = contracts.FactoryConfig
type FactoryWebhookConfig = contracts.FactoryWebhookConfig
type FactoryWebhookFilterConfig = contracts.FactoryWebhookFilterConfig
type FactoryWebhookDeliveryPolicyConfig = contracts.FactoryWebhookDeliveryPolicyConfig
type FactoryWebhookEffectiveDeliveryPolicy = contracts.FactoryWebhookEffectiveDeliveryPolicy
type NameValueConfig = contracts.NameValueConfig
type NameValueValidationError = contracts.NameValueValidationError
type SaveMode = contracts.SaveMode
type NamedFactoryPersistenceMode = contracts.NamedFactoryPersistenceMode
type NamedFactoryPersistenceRequest = contracts.NamedFactoryPersistenceRequest
type NamedFactoryPersistenceResult = contracts.NamedFactoryPersistenceResult
type NamedFactoryPersistenceOperation = contracts.NamedFactoryPersistenceOperation
type EditableFactory = contracts.EditableFactory
type ValidationProfile = contracts.ValidationProfile
type WorkflowSourceReader = contracts.WorkflowSourceReader
type ValidationSeverity = contracts.ValidationSeverity
type ValidationSubjectType = contracts.ValidationSubjectType
type ValidationSubjectLocation = contracts.ValidationSubjectLocation
type ValidationResult = contracts.ValidationResult
type TopologyFinding = contracts.TopologyFinding
type TopologyValidationResult = contracts.TopologyValidationResult
type BlockingFactoryLoadError = contracts.BlockingFactoryLoadError
type ValidationTopologyError = contracts.ValidationTopologyError
type FactoryOrchestratorJavaScriptConfig = contracts.FactoryOrchestratorJavaScriptConfig
type FactoryGuardConfig = contracts.FactoryGuardConfig
type FactoryWorkstationConfig = contracts.FactoryWorkstationConfig
type FactoryWorkerConfig = workerconfig.Config
type HostedLinearWorkerConfig = workerconfig.HostedLinearWorkerConfig
type HostedLinearWorkerClaimConfig = workerconfig.HostedLinearWorkerClaimConfig
type HostedLinearWorkerMappingConfig = workerconfig.HostedLinearWorkerMappingConfig
type HostedWorkerAuthConfig = workerconfig.HostedWorkerAuthConfig
type AgentToolsConfig = workerconfig.AgentToolsConfig
type ModelOperation = workerconfig.ModelOperation
type ModelOperationSlot = workerconfig.ModelOperationSlot
type ResourceConfig = catalogresource.Config
type InvocationSignatureConfig = contracts.InvocationSignatureConfig
type InvocationExampleConfig = contracts.InvocationExampleConfig
type InvocationExampleArguments = contracts.InvocationExampleArguments
type InvocationOutputContractConfig = contracts.InvocationOutputContractConfig
type InvocationParameterConfig = contracts.InvocationParameterConfig
type ModelOperationBinding = contracts.ModelOperationBinding
type ModelOperationBindingSelector = contracts.ModelOperationBindingSelector
type ModelProvider = contracts.ModelProvider
type RuntimeConfigLookup = contracts.RuntimeConfigLookup
type RuntimeDefinitionLookup = contracts.RuntimeDefinitionLookup
type RuntimeFactoryConfigLookup = contracts.RuntimeFactoryConfigLookup
type PromptSource = contracts.PromptSource
type RuntimePromptSourceLookup = contracts.RuntimePromptSourceLookup
type WorkstationKind = contracts.WorkstationKind
type WorkstationLimits = contracts.WorkstationLimits
type Workstation = contracts.Workstation
type ExpectedArtifactConfig = contracts.ExpectedArtifactConfig
type GuardMatchConfig = contracts.GuardMatchConfig
type LogicalRoundTripConfig = contracts.LogicalRoundTripConfig
type GuardType = contracts.GuardType
type InputGuardConfig = contracts.InputGuardConfig
type InputTypeConfig = contracts.InputTypeConfig
type IOConfig = contracts.IOConfig
type StateConfig = contracts.StateConfig
type StateType = contracts.StateType
type WorkTypeConfig = contracts.WorkTypeConfig
type WorkerWorkstationBehaviorClass = contracts.WorkerWorkstationBehaviorClass
type PendingFactoryGraphTopology = contracts.PendingFactoryGraphTopology
type ValidationTarget = contracts.ValidationTarget
type ValidationSubject = contracts.ValidationSubject

const (
	NameValueTypeLocalizableAsset      = contracts.NameValueTypeLocalizableAsset
	SaveModeReplaceCurrent             = contracts.SaveModeReplaceCurrent
	SaveModeUpsertNamedAndActivate     = contracts.SaveModeUpsertNamedAndActivate
	NamedFactoryPersistenceModeCreate  = contracts.NamedFactoryPersistenceModeCreate
	NamedFactoryPersistenceModeReplace = contracts.NamedFactoryPersistenceModeReplace
	ValidationProfileTopology          = contracts.ValidationProfileTopology
	ValidationProfilePrePersist        = contracts.ValidationProfilePrePersist
	DefaultTopologyValidationMessage   = contracts.DefaultTopologyValidationMessage

	FactoryWebhookEventTypeWorkStateChange     = contracts.FactoryWebhookEventTypeWorkStateChange
	FactoryWebhookEventTypeDispatchResponse    = contracts.FactoryWebhookEventTypeDispatchResponse
	FactoryWebhookEventTypeDispatchReconciled  = contracts.FactoryWebhookEventTypeDispatchReconciled
	FactoryWebhookEventTypeDispatchInterrupted = contracts.FactoryWebhookEventTypeDispatchInterrupted
	FactoryWebhookDispatchStatusFailed         = contracts.FactoryWebhookDispatchStatusFailed
	FactoryWebhookDispatchStatusInterrupted    = contracts.FactoryWebhookDispatchStatusInterrupted
	DefaultFactoryWebhookRequestTimeout        = contracts.DefaultFactoryWebhookRequestTimeout
	DefaultFactoryWebhookMaxAttempts           = contracts.DefaultFactoryWebhookMaxAttempts
	DefaultFactoryWebhookInitialBackoff        = contracts.DefaultFactoryWebhookInitialBackoff
	DefaultFactoryWebhookBackoffMultiplier     = contracts.DefaultFactoryWebhookBackoffMultiplier
	DefaultFactoryWebhookMaxBackoff            = contracts.DefaultFactoryWebhookMaxBackoff

	InvocationParameterTypeHintString        = contracts.InvocationParameterTypeHintString
	InvocationParameterTypeHintPath          = contracts.InvocationParameterTypeHintPath
	InvocationParameterTypeHintFilePath      = contracts.InvocationParameterTypeHintFilePath
	InvocationParameterTypeHintDirectoryPath = contracts.InvocationParameterTypeHintDirectoryPath
	InvocationParameterTypeHintNumberString  = contracts.InvocationParameterTypeHintNumberString
	InvocationParameterTypeHintBooleanString = contracts.InvocationParameterTypeHintBooleanString
	InvocationParameterTypeHintJSON          = contracts.InvocationParameterTypeHintJSON

	InvocationParameterValueModeExact        = contracts.InvocationParameterValueModeExact
	InvocationParameterValueModeRepeated     = contracts.InvocationParameterValueModeRepeated
	InvocationParameterValueModeVariadic     = contracts.InvocationParameterValueModeVariadic
	InvocationParameterValueModeFileContents = contracts.InvocationParameterValueModeFileContents

	InvocationParameterBindingKindNamed     = contracts.InvocationParameterBindingKindNamed
	InvocationParameterBindingKindStdin     = contracts.InvocationParameterBindingKindStdin
	InvocationParameterBindingKindNamedRest = contracts.InvocationParameterBindingKindNamedRest

	InvocationUnknownNamedArgumentPolicyReject  = contracts.InvocationUnknownNamedArgumentPolicyReject
	InvocationUnknownNamedArgumentPolicyAllow   = contracts.InvocationUnknownNamedArgumentPolicyAllow
	InvocationUnknownNamedArgumentPolicyCollect = contracts.InvocationUnknownNamedArgumentPolicyCollect

	InvocationOutputContractModeInline = contracts.InvocationOutputContractModeInline
	InvocationOutputContractModeFile   = contracts.InvocationOutputContractModeFile
	InvocationOutputContractModeJSON   = contracts.InvocationOutputContractModeJSON

	InvocationReturnPolicySubmittedWorkTerminal = contracts.InvocationReturnPolicySubmittedWorkTerminal
	InvocationReturnPolicyExplicit              = contracts.InvocationReturnPolicyExplicit

	FactoryAgentsFileName                 = contracts.FactoryAgentsFileName
	FactoryConfigIgnoredFieldWarningCode  = contracts.FactoryConfigIgnoredFieldWarningCode
	WorkersDir                            = contracts.WorkersDir
	WorkstationsDir                       = contracts.WorkstationsDir
	BundledFileTypeRootHelper             = contracts.BundledFileTypeRootHelper
	SystemTimeDashboardExpiryTransitionID = contracts.SystemTimeDashboardExpiryTransitionID
)

var (
	ErrCurrentFactoryNotFound = contracts.ErrCurrentFactoryNotFound
	ErrFactoryVersionStale    = contracts.ErrFactoryVersionStale
)

var (
	ValidateNameValue                            = contracts.ValidateNameValue
	ResolveNameValue                             = contracts.ResolveNameValue
	ResolveFactoryWebhookDeliveryPolicy          = contracts.ResolveFactoryWebhookDeliveryPolicy
	CanonicalFactoryGraphEntityID                = contracts.CanonicalFactoryGraphEntityID
	CanonicalFactoryGraphWorkTypeID              = contracts.CanonicalFactoryGraphWorkTypeID
	CanonicalFactoryGraphWorkStateID             = contracts.CanonicalFactoryGraphWorkStateID
	CanonicalFactoryGraphResourceID              = contracts.CanonicalFactoryGraphResourceID
	CanonicalFactoryGraphWorkerID                = contracts.CanonicalFactoryGraphWorkerID
	CanonicalFactoryGraphWorkstationID           = contracts.CanonicalFactoryGraphWorkstationID
	IsBundledFileGraphNodeID                     = contracts.IsBundledFileGraphNodeID
	IsPetriOrchestratorFactory                   = contracts.IsPetriOrchestratorFactory
	PublicWorkerModelProviderFromInternal        = contracts.PublicWorkerModelProviderFromInternal
	PermissivePublicFactoryRunnerID              = contracts.PermissivePublicFactoryRunnerID
	PermissivePublicFactoryRunnerSelectionSource = contracts.PermissivePublicFactoryRunnerSelectionSource
	SupportedModelProviders                      = contracts.SupportedModelProviders
)

type PortableBundledFileReplacement = contracts.PortableBundledFileReplacement
type MutableLoadedFactorySource = contracts.MutableLoadedFactorySource
type DefinitionSession = contracts.DefinitionSession

const (
	ResourceStateAvailable                               = contracts.ResourceStateAvailable
	ResourceTypeModel                                    = catalogresource.TypeModel
	ResourceTypeProviderQuota                            = catalogresource.TypeProviderQuota
	ResourceTypeInvocationSlot                           = catalogresource.TypeInvocationSlot
	WorkerTypeAgent                                      = contracts.WorkerTypeAgent
	WorkerTypeInference                                  = contracts.WorkerTypeInference
	WorkerTypeModel                                      = contracts.WorkerTypeModel
	WorkerTypeScript                                     = contracts.WorkerTypeScript
	WorkerTypeHosted                                     = contracts.WorkerTypeHosted
	WorkerTypePoller                                     = contracts.WorkerTypePoller
	ModelLocalityLocal                                   = workerconfig.ModelLocalityLocal
	ModelLocalityCloud                                   = workerconfig.ModelLocalityCloud
	AgentToolPolicyDisabled                              = workerconfig.AgentToolPolicyDisabled
	AgentToolPolicyEnabled                               = workerconfig.AgentToolPolicyEnabled
	AgentToolPolicyReadOnly                              = workerconfig.AgentToolPolicyReadOnly
	ModelOperationContentTypeAudio                       = workerconfig.ModelOperationContentTypeAudio
	ModelOperationContentTypeBinary                      = workerconfig.ModelOperationContentTypeBinary
	ModelOperationContentTypeImage                       = workerconfig.ModelOperationContentTypeImage
	ModelOperationContentTypeJSON                        = workerconfig.ModelOperationContentTypeJSON
	ModelOperationContentTypeText                        = workerconfig.ModelOperationContentTypeText
	WorkerModelProviderDefault                           = contracts.WorkerModelProviderDefault
	WorkstationTypeLogical                               = contracts.WorkstationTypeLogical
	WorkstationTypePoller                                = contracts.WorkstationTypePoller
	WorkstationTypeInference                             = contracts.WorkstationTypeInference
	WorkstationTypeModel                                 = contracts.WorkstationTypeModel
	WorkerWorkstationBehaviorInference                   = contracts.WorkerWorkstationBehaviorInference
	WorkerWorkstationBehaviorAgent                       = contracts.WorkerWorkstationBehaviorAgent
	WorkerWorkstationBehaviorScript                      = contracts.WorkerWorkstationBehaviorScript
	WorkerWorkstationBehaviorPoller                      = contracts.WorkerWorkstationBehaviorPoller
	ValidationCodeFactorySessionField                    = contracts.ValidationCodeFactorySessionField
	ValidationCodeFactorySessionTarget                   = contracts.ValidationCodeFactorySessionTarget
	ValidationCodeFactoryPayloadInvalid                  = contracts.ValidationCodeFactoryPayloadInvalid
	ValidationCodeFactoryNameInvalid                     = contracts.ValidationCodeFactoryNameInvalid
	ValidationCodeFactoryVersionStale                    = contracts.ValidationCodeFactoryVersionStale
	ValidationCodeFactoryRuntimeNotIdle                  = contracts.ValidationCodeFactoryRuntimeNotIdle
	ValidationCodeLayoutUnknownNodeReference             = contracts.ValidationCodeLayoutUnknownNodeReference
	ValidationCodeLayoutInvalidGeometry                  = contracts.ValidationCodeLayoutInvalidGeometry
	ValidationCodeLayoutInvalidValue                     = contracts.ValidationCodeLayoutInvalidValue
	ValidationCodeLayoutImageBudgetExceeded              = contracts.ValidationCodeLayoutImageBudgetExceeded
	ValidationCodeWorkerWorkstationBehaviorCompatibility = contracts.ValidationCodeWorkerWorkstationBehaviorCompatibility
	ValidationSeverityError                              = contracts.ValidationSeverityError
	ValidationSeverityWarning                            = contracts.ValidationSeverityWarning
	ValidationSeverityHint                               = contracts.ValidationSeverityHint
	ValidationSubjectTypeFactory                         = contracts.ValidationSubjectTypeFactory
	ValidationSubjectTypeWorkstation                     = contracts.ValidationSubjectTypeWorkstation
	ValidationSubjectTypeWorkType                        = contracts.ValidationSubjectTypeWorkType
	ValidationSubjectTypeWorkState                       = contracts.ValidationSubjectTypeWorkState
	ValidationSubjectTypeWorker                          = contracts.ValidationSubjectTypeWorker
	ValidationSubjectTypeResource                        = contracts.ValidationSubjectTypeResource
	ValidationSubjectTypeRoute                           = contracts.ValidationSubjectTypeRoute
	ValidationSubjectLocationOnRejection                 = contracts.ValidationSubjectLocationOnRejection
	ValidationSubjectLocationOnFailure                   = contracts.ValidationSubjectLocationOnFailure
	ValidationSubjectLocationOutputs                     = contracts.ValidationSubjectLocationOutputs
	ValidationSubjectLocationInputs                      = contracts.ValidationSubjectLocationInputs
	ValidationSubjectLocationStates                      = contracts.ValidationSubjectLocationStates
	ValidationSubjectLocationTerminal                    = contracts.ValidationSubjectLocationTerminal
	ValidationSubjectLocationReference                   = contracts.ValidationSubjectLocationReference
	ValidationSubjectLocationDefinition                  = contracts.ValidationSubjectLocationDefinition
)

var CanonicalEventTime = contracts.CanonicalEventTime
var CanonicalizeReasoningEffort = contracts.CanonicalizeReasoningEffort
var CanonicalizeOperatorWorkerModelProviderInput = contracts.CanonicalizeOperatorWorkerModelProviderInput
var IsSymbolicWorkerModelProviderDefault = contracts.IsSymbolicWorkerModelProviderDefault
var UsesModelhostLease = contracts.UsesModelhostLease
var IsAgentRunWorkstationType = contracts.IsAgentRunWorkstationType
var IsHumanApprovalWorkstationType = contracts.IsHumanApprovalWorkstationType
var IsAgentWorkerType = contracts.IsAgentWorkerType
var IsInferenceRunWorkstationType = contracts.IsInferenceRunWorkstationType
var IsInferenceWorkerType = contracts.IsInferenceWorkerType
var IsScriptWorkerType = contracts.IsScriptWorkerType
var ExemptFromWorkerWorkstationCompatibility = contracts.ExemptFromWorkerWorkstationCompatibility
var EffectiveWorkstationTypeForCompatibility = contracts.EffectiveWorkstationTypeForCompatibility
var WorkerBehaviorClass = contracts.WorkerBehaviorClass
var ExpectedWorkerBehaviorClassForWorkstation = contracts.ExpectedWorkerBehaviorClassForWorkstation
var WorkerMatchesWorkstationBehavior = contracts.WorkerMatchesWorkstationBehavior
var PublicWorkerTypeForFactoryUsage = contracts.PublicWorkerTypeForFactoryUsage
var IsScheduledLegacyModelPair = contracts.IsScheduledLegacyModelPair
var EffectiveWorkstationBehaviorClass = contracts.EffectiveWorkstationBehaviorClass
var IsLegacyGrandfatheredWorkerWorkstationPair = contracts.IsLegacyGrandfatheredWorkerWorkstationPair
var RequiresWorkerWorkstationBehaviorCompatibility = contracts.RequiresWorkerWorkstationBehaviorCompatibility
var CompatibleWorkerWorkstationBehavior = contracts.CompatibleWorkerWorkstationBehavior
var RuntimeBehaviorClassLabel = contracts.RuntimeBehaviorClassLabel
var WorkerWorkstationBehaviorMismatchMessage = contracts.WorkerWorkstationBehaviorMismatchMessage
var AgentToolsAllowExecution = workerconfig.AgentToolsAllowExecution
var EffectiveAgentToolPolicy = workerconfig.EffectiveAgentToolPolicy
var IsKnownAgentToolPolicy = workerconfig.IsKnownAgentToolPolicy
var NormalizeAgentToolPolicy = workerconfig.NormalizeAgentToolPolicy
var CloneWorkerConfig = workerconfig.Clone
var EffectiveExpectedArtifacts = contracts.EffectiveExpectedArtifacts
var NormalizeExpectedArtifactConfigs = contracts.NormalizeExpectedArtifactConfigs
var ValidateExpectedArtifactConfig = contracts.ValidateExpectedArtifactConfig

// Foreign-vocabulary deletion-only aliases below are retained until
// CLN-DEF-CONTRACTS story 007 deletes the contracts mega-barrel. Event envelope
// vocabulary was rehomed to pkg/services/recordings in story 003; world-state,
// dispatch, and replay vocabulary were rehomed in story 004; worker and provider
// execution vocabulary was rehomed to pkg/services/workers and
// pkg/services/providers in story 005.

type (
	ArcMode                                          = contracts.ArcMode
	BundledFileConfig                                = contracts.BundledFileConfig
	CronConfig                                       = contracts.CronConfig
	EnabledTransition                                = contracts.EnabledTransition
	EngineStateSnapshot[TMarking any, TTopology any] = contracts.EngineStateSnapshot[TMarking, TTopology]
	FactoryArtifact                                  = contracts.FactoryArtifact
	FactoryArtifactCaptureMetadata                   = contracts.FactoryArtifactCaptureMetadata
	FactoryArtifactRedactionCounts                   = contracts.FactoryArtifactRedactionCounts
	FactoryArtifactRef                               = contracts.FactoryArtifactRef
	FactoryCompletionRecord                          = contracts.FactoryCompletionRecord
	FactoryConstraint                                = contracts.FactoryConstraint
	FactoryInvocationResult                          = contracts.FactoryInvocationResult
	FactoryLayoutBoundsConfig                        = contracts.FactoryLayoutBoundsConfig
	FactoryLayoutAnnotationConfig                    = contracts.FactoryLayoutAnnotationConfig
	FactoryLayoutConfig                              = contracts.FactoryLayoutConfig
	FactoryLayoutEdgeConfig                          = contracts.FactoryLayoutEdgeConfig
	FactoryLayoutEmptyStateConfig                    = contracts.FactoryLayoutEmptyStateConfig
	FactoryLayoutGroupConfig                         = contracts.FactoryLayoutGroupConfig
	FactoryLayoutImageConfig                         = contracts.FactoryLayoutImageConfig
	FactoryLayoutImageSourceConfig                   = contracts.FactoryLayoutImageSourceConfig
	FactoryLayoutNodeConfig                          = contracts.FactoryLayoutNodeConfig
	FactoryLayoutNoteConfig                          = contracts.FactoryLayoutNoteConfig
	FactoryLayoutPointConfig                         = contracts.FactoryLayoutPointConfig
	FactoryLayoutPreferencesConfig                   = contracts.FactoryLayoutPreferencesConfig
	FactoryLayoutSizeConfig                          = contracts.FactoryLayoutSizeConfig
	FactoryLayoutViewportConfig                      = contracts.FactoryLayoutViewportConfig
	FactoryOrchestratorJavaScriptAgent               = contracts.FactoryOrchestratorJavaScriptAgent
	FactoryResource                                  = contracts.FactoryResource
	FactoryResourceUnit                              = contracts.FactoryResourceUnit
	FactorySessionArtifactState                      = contracts.FactorySessionArtifactState
	FactorySessionJavaScriptCheckpointEventRef       = contracts.FactorySessionJavaScriptCheckpointEventRef
	FactorySessionJavaScriptCheckpointRef            = contracts.FactorySessionJavaScriptCheckpointRef
	FactorySessionJavaScriptRuntimeState             = contracts.FactorySessionJavaScriptRuntimeState
	FactorySessionJavaScriptScriptStatus             = contracts.FactorySessionJavaScriptScriptStatus
	FactorySessionLifecycleControlKind               = contracts.FactorySessionLifecycleControlKind
	FactorySessionLifecycleControlOutcome            = contracts.FactorySessionLifecycleControlOutcome
	FactorySessionLifecycleStatus                    = contracts.FactorySessionLifecycleStatus
	FactorySessionResultStatus                       = contracts.FactorySessionResultStatus
	FactorySnapshot                                  = contracts.FactorySnapshot
	FactoryVersion                                   = contracts.FactoryVersion
	FiringDecision                                   = contracts.FiringDecision
	GuardConfig                                      = contracts.GuardConfig
	InvocationParameterBindingConfig                 = contracts.InvocationParameterBindingConfig
	InvocationReturnConfig                           = contracts.InvocationReturnConfig
	JavaScriptCheckpointArtifactRef                  = contracts.JavaScriptCheckpointArtifactRef
	JavaScriptCheckpointRecord                       = contracts.JavaScriptCheckpointRecord
	MarkingMutation                                  = contracts.MarkingMutation
	OrchestratorPhaseStatus                          = contracts.OrchestratorPhaseStatus
	PortableResourceManifestConfig                   = contracts.PortableResourceManifestConfig
	RequestValidationError                           = contracts.RequestValidationError
	RequiredToolConfig                               = contracts.RequiredToolConfig
	RuntimeMode                                      = contracts.RuntimeMode
	RuntimeStatus                                    = contracts.RuntimeStatus
	TerminationClassification                        = contracts.TerminationClassification
	TerminationResult                                = contracts.TerminationResult
	RuntimeWorkstationLookup                         = contracts.RuntimeWorkstationLookup
	SubmissionHookContext[TSnapshot any]             = contracts.SubmissionHookContext[TSnapshot]
	SubmissionHookResult                             = contracts.SubmissionHookResult
	TickResult                                       = contracts.TickResult
	TokenMutationRecord                              = contracts.TokenMutationRecord
	WorkPropagationMode                              = contracts.WorkPropagationMode
	WorkRequestPayload                               = contracts.WorkRequestPayload
	WorkstationLoader                                = contracts.WorkstationLoader
	BundledFileContentConfig                         = contracts.BundledFileContentConfig
	ClassificationRouteConfig                        = contracts.ClassificationRouteConfig
	FactoryOrchestratorConfig                        = contracts.FactoryOrchestratorConfig
	FactoryOrchestratorJavaScriptInlineSource        = contracts.FactoryOrchestratorJavaScriptInlineSource
	FactoryOrchestratorPetriConfig                   = contracts.FactoryOrchestratorPetriConfig
	FactoryStateChangePayload                        = contracts.FactoryStateChangePayload
	FactoryTraceData                                 = contracts.FactoryTraceData
	InputKind                                        = contracts.InputKind
	InvocationTerminalStatus                         = contracts.InvocationTerminalStatus
	RelationshipChangePayload                        = contracts.RelationshipChangePayload
	WorkInputPayload                                 = contracts.WorkInputPayload
	WorkPropagationConfig                            = contracts.WorkPropagationConfig
)

const (
	ArcModeObserve                                = contracts.ArcModeObserve
	ArcModeConsume                                = contracts.ArcModeConsume
	ArtifactsDirectory                            = contracts.ArtifactsDirectory
	DefaultChannelName                            = contracts.DefaultChannelName
	FactoryConfigFile                             = contracts.FactoryConfigFile
	FactoryDir                                    = contracts.FactoryDir
	DefaultCurrentFactoryName                     = contracts.DefaultCurrentFactoryName
	FactorySessionJavaScriptScriptStatusFailed    = contracts.FactorySessionJavaScriptScriptStatusFailed
	FactorySessionJavaScriptScriptStatusFinished  = contracts.FactorySessionJavaScriptScriptStatusFinished
	FactorySessionJavaScriptScriptStatusIdle      = contracts.FactorySessionJavaScriptScriptStatusIdle
	FactorySessionJavaScriptScriptStatusPaused    = contracts.FactorySessionJavaScriptScriptStatusPaused
	FactorySessionJavaScriptScriptStatusRunning   = contracts.FactorySessionJavaScriptScriptStatusRunning
	FactorySessionLifecycleControlOutcomeAccepted = contracts.FactorySessionLifecycleControlOutcomeAccepted
	FactorySessionLifecycleControlPause           = contracts.FactorySessionLifecycleControlPause
	FactorySessionLifecycleControlResume          = contracts.FactorySessionLifecycleControlResume
	FactorySessionLifecycleStatusFailed           = contracts.FactorySessionLifecycleStatusFailed
	FactorySessionLifecycleStatusPaused           = contracts.FactorySessionLifecycleStatusPaused
	FactorySessionLifecycleStatusRunning          = contracts.FactorySessionLifecycleStatusRunning
	FactorySessionLifecycleStatusSucceeded        = contracts.FactorySessionLifecycleStatusSucceeded
	FactorySessionResultStatusFailedWithPartial   = contracts.FactorySessionResultStatusFailedWithPartial
	FactorySessionResultStatusFinal               = contracts.FactorySessionResultStatusFinal
	FactorySessionResultStatusPartial             = contracts.FactorySessionResultStatusPartial
	HostedWorkerProviderLinear                    = contracts.HostedWorkerProviderLinear
	InputsDir                                     = contracts.InputsDir
	InvocationErrorCodeCanceled                   = contracts.InvocationErrorCodeCanceled
	InvocationErrorCodeRuntimeFailure             = contracts.InvocationErrorCodeRuntimeFailure
	InvocationErrorCodeTimedOut                   = contracts.InvocationErrorCodeTimedOut
	InvocationParameterBindingKindPositional      = contracts.InvocationParameterBindingKindPositional
	InvocationTerminalStatusCanceled              = contracts.InvocationTerminalStatusCanceled
	InvocationTerminalStatusCompleted             = contracts.InvocationTerminalStatusCompleted
	InvocationTerminalStatusFailed                = contracts.InvocationTerminalStatusFailed
	InvocationTerminalStatusTimedOut              = contracts.InvocationTerminalStatusTimedOut
	JavaScriptCheckpointArtifactKind              = contracts.JavaScriptCheckpointArtifactKind
	JavaScriptCheckpointArtifactVisibility        = contracts.JavaScriptCheckpointArtifactVisibility
	MutationConsume                               = contracts.MutationConsume
	MutationCreate                                = contracts.MutationCreate
	MutationMove                                  = contracts.MutationMove
	OrchestratorKindJavaScript                    = contracts.OrchestratorKindJavaScript
	OrchestratorKindPetri                         = contracts.OrchestratorKindPetri
	OrchestratorPhaseStatusActive                 = contracts.OrchestratorPhaseStatusActive
	OrchestratorPhaseStatusCompleted              = contracts.OrchestratorPhaseStatusCompleted
	OrchestratorPhaseStatusSkipped                = contracts.OrchestratorPhaseStatusSkipped
	RejectionFeedback                             = contracts.RejectionFeedback
	RuntimeModeBatch                              = contracts.RuntimeModeBatch
	RuntimeModeService                            = contracts.RuntimeModeService
	RuntimeStatusActive                           = contracts.RuntimeStatusActive
	RuntimeStatusFinished                         = contracts.RuntimeStatusFinished
	RuntimeStatusIdle                             = contracts.RuntimeStatusIdle
	TerminationClassificationComplete             = contracts.TerminationClassificationComplete
	TerminationClassificationIncomplete           = contracts.TerminationClassificationIncomplete
	SystemTimeExpiryTransitionID                  = contracts.SystemTimeExpiryTransitionID
	SystemTimePendingState                        = contracts.SystemTimePendingState
	SystemTimeWorkTypeID                          = contracts.SystemTimeWorkTypeID
	TimeWorkSourceCron                            = contracts.TimeWorkSourceCron
	TimeWorkTagKeyCronWorkstation                 = contracts.TimeWorkTagKeyCronWorkstation
	TimeWorkTagKeyDueAt                           = contracts.TimeWorkTagKeyDueAt
	TimeWorkTagKeyExpiresAt                       = contracts.TimeWorkTagKeyExpiresAt
	TimeWorkTagKeyJitter                          = contracts.TimeWorkTagKeyJitter
	TimeWorkTagKeyNominalAt                       = contracts.TimeWorkTagKeyNominalAt
	TimeWorkTagKeySource                          = contracts.TimeWorkTagKeySource
	WorkPropagationModeOutputAsPayload            = contracts.WorkPropagationModeOutputAsPayload
	WorkPropagationModePreserveInput              = contracts.WorkPropagationModePreserveInput
	WorkstationKindCron                           = contracts.WorkstationKindCron
	GuardTypeAllChildrenComplete                  = contracts.GuardTypeAllChildrenComplete
	GuardTypeInferenceThrottle                    = contracts.GuardTypeInferenceThrottle
	GuardTypeMatchesFields                        = contracts.GuardTypeMatchesFields
	GuardTypeSameName                             = contracts.GuardTypeSameName
	GuardTypeSameTraceID                          = contracts.GuardTypeSameTraceID
	GuardTypeVisitCount                           = contracts.GuardTypeVisitCount
	InputKindDefault                              = contracts.InputKindDefault
	SystemTimePendingPlaceID                      = contracts.SystemTimePendingPlaceID
	WorkflowsDir                                  = contracts.WorkflowsDir
	WorkID                                        = contracts.WorkID
	WorkTypeID                                    = contracts.WorkTypeID
	TraceID                                       = contracts.TraceID
	ParentID                                      = contracts.ParentID
	WorkstationKindPoller                         = contracts.WorkstationKindPoller
	WorkstationKindRepeater                       = contracts.WorkstationKindRepeater
	WorkstationKindStandard                       = contracts.WorkstationKindStandard
	WorkstationTypeClassify                       = contracts.WorkstationTypeClassify
	WorkstationTypeHumanApproval                  = contracts.WorkstationTypeHumanApproval
	WorkstationOutcomeFormatDecisionEnvelope      = contracts.WorkstationOutcomeFormatDecisionEnvelope
	BundledFileEncodingUTF8                       = contracts.BundledFileEncodingUTF8
	BundledFileTypeDoc                            = contracts.BundledFileTypeDoc
	BundledFileTypeInput                          = contracts.BundledFileTypeInput
	BundledFileTypeScript                         = contracts.BundledFileTypeScript
	CurrentFactoryPointerFile                     = contracts.CurrentFactoryPointerFile
	GuardTypeAnyChildFailed                       = contracts.GuardTypeAnyChildFailed
	OrchestratorInlineEncoding                    = contracts.OrchestratorInlineEncoding
	SupportedFactoryLayoutSchemaVersion           = contracts.SupportedFactoryLayoutSchemaVersion
	SystemTimeDashboardPendingPlaceID             = contracts.SystemTimeDashboardPendingPlaceID
	SystemTimeDashboardWorkTypeID                 = contracts.SystemTimeDashboardWorkTypeID
	WorkstationTypeAgent                          = contracts.WorkstationTypeAgent
	WorkstationTypeInvoke                         = contracts.WorkstationTypeInvoke
	WorkstationTypeScript                         = contracts.WorkstationTypeScript
	WorkTypeHandlingBehaviorDefault               = contracts.WorkTypeHandlingBehaviorDefault
)

var (
	ErrInvalidNamedFactory                = contracts.ErrInvalidNamedFactory
	ErrNamedFactoryAlreadyExists          = contracts.ErrNamedFactoryAlreadyExists
	ErrInvalidNamedFactoryName            = contracts.ErrInvalidNamedFactoryName
	ErrFactoryLayoutNotFound              = contracts.ErrFactoryLayoutNotFound
	ErrNamedFactoryNotFound               = contracts.ErrNamedFactoryNotFound
	ErrNamedFactoryIsCurrent              = contracts.ErrNamedFactoryIsCurrent
	ErrFactoryInstallationContention      = contracts.ErrFactoryInstallationContention
	NewBlockingFactoryLoadError           = contracts.NewBlockingFactoryLoadError
	AsBlockingFactoryLoadError            = contracts.AsBlockingFactoryLoadError
	NewValidationTopologyError            = contracts.NewValidationTopologyError
	FormFactoryPayloadValidationTarget    = contracts.FormFactoryPayloadValidationTarget
	InvalidFactoryNameValidationTarget    = contracts.InvalidFactoryNameValidationTarget
	StaleFactoryVersionValidationTarget   = contracts.StaleFactoryVersionValidationTarget
	FactoryRuntimeNotIdleValidationTarget = contracts.FactoryRuntimeNotIdleValidationTarget
	FactorySessionFieldValidationTarget   = contracts.FactorySessionFieldValidationTarget
	FactorySessionTargetValidationTarget  = contracts.FactorySessionTargetValidationTarget
	ResolveValidationProfile              = contracts.ResolveValidationProfile
	IsLayoutValidationCode                = contracts.IsLayoutValidationCode

	CanonicalBundledFileID                                 = contracts.CanonicalBundledFileID
	CanonicalPublicWorkstationKind                         = contracts.CanonicalPublicWorkstationKind
	CloneFactoryConfig                                     = contracts.CloneFactoryConfig
	CloneGuardMatchConfig                                  = contracts.CloneGuardMatchConfig
	CloneIOConfigs                                         = contracts.CloneIOConfigs
	CloneModelOperationBindings                            = contracts.CloneModelOperationBindings
	CloneModelOperations                                   = contracts.CloneModelOperations
	CloneWorkstationConfig                                 = contracts.CloneWorkstationConfig
	CloneWorkstationInputs                                 = contracts.CloneWorkstationInputs
	EffectiveOrchestratorKind                              = contracts.EffectiveOrchestratorKind
	ErrFactoryActivationRequiresIdle                       = contracts.ErrFactoryActivationRequiresIdle
	FirstRuntimeDefinitionLookup                           = contracts.FirstRuntimeDefinitionLookup
	IsJavaScriptOrchestratorFactory                        = contracts.IsJavaScriptOrchestratorFactory
	IsPollerWorkerType                                     = contracts.IsPollerWorkerType
	IsSystemTimeToken                                      = contracts.IsSystemTimeToken
	NewFactorySnapshot                                     = contracts.NewFactorySnapshot
	PermissivePublicFactoryWorkerModelProvider             = contracts.PermissivePublicFactoryWorkerModelProvider
	PermissivePublicFactoryWorkerProvider                  = contracts.PermissivePublicFactoryWorkerProvider
	PublicWorkerModelProviderFromInternalRuntime           = contracts.PublicWorkerModelProviderFromInternalRuntime
	PublicWorkerProviderFromInternalRuntime                = contracts.PublicWorkerProviderFromInternalRuntime
	PublicWorkerTypeFromInternalRuntime                    = contracts.PublicWorkerTypeFromInternalRuntime
	PublicWorkstationTypeFromInternalRuntime               = contracts.PublicWorkstationTypeFromInternalRuntime
	StrictPublicFactoryOrchestratorKind                    = contracts.StrictPublicFactoryOrchestratorKind
	StrictPublicFactoryWorkstationType                     = contracts.StrictPublicFactoryWorkstationType
	StrictPublicFactoryWorkerModelProvider                 = contracts.StrictPublicFactoryWorkerModelProvider
	InternalModelProviderFromPublicWorkerModelProvider     = contracts.InternalModelProviderFromPublicWorkerModelProvider
	AcceptedPublicWorkerModelProviderSummary               = contracts.AcceptedPublicWorkerModelProviderSummary
	BuildPendingFactoryGraphTopology                       = contracts.BuildPendingFactoryGraphTopology
	InternalRuntimeWorkerTypeFromPublic                    = contracts.InternalRuntimeWorkerTypeFromPublic
	InternalRuntimeWorkstationTypeFromPublic               = contracts.InternalRuntimeWorkstationTypeFromPublic
	IsPollerWorkerPublicType                               = contracts.IsPollerWorkerPublicType
	PermissivePublicFactoryHostedWorkerProvider            = contracts.PermissivePublicFactoryHostedWorkerProvider
	PermissivePublicFactoryResourceType                    = contracts.PermissivePublicFactoryResourceType
	PermissivePublicFactoryWorkerModelLocality             = contracts.PermissivePublicFactoryWorkerModelLocality
	PermissivePublicFactoryWorkerModelOperationContentType = contracts.PermissivePublicFactoryWorkerModelOperationContentType
	PermissivePublicFactoryWorkstationOutcomeFormat        = contracts.PermissivePublicFactoryWorkstationOutcomeFormat
	PermissivePublicFactoryWorkstationType                 = contracts.PermissivePublicFactoryWorkstationType
	StrictPublicFactoryHostedWorkerProvider                = contracts.StrictPublicFactoryHostedWorkerProvider
	StrictPublicFactoryResourceType                        = contracts.StrictPublicFactoryResourceType
	StrictPublicFactoryRunnerID                            = contracts.StrictPublicFactoryRunnerID
	StrictPublicFactoryWorkerModelLocality                 = contracts.StrictPublicFactoryWorkerModelLocality
	StrictPublicFactoryWorkerModelOperationContentType     = contracts.StrictPublicFactoryWorkerModelOperationContentType
	StrictPublicFactoryWorkerProvider                      = contracts.StrictPublicFactoryWorkerProvider
	StrictPublicFactoryWorkerType                          = contracts.StrictPublicFactoryWorkerType
	StrictPublicFactoryWorkstationOutcomeFormat            = contracts.StrictPublicFactoryWorkstationOutcomeFormat
	StrictPublicWorkstationKind                            = contracts.StrictPublicWorkstationKind
	StrictPublicWorkTypeHandlingBehavior                   = contracts.StrictPublicWorkTypeHandlingBehavior
)

// ResolveRuntimeSnapshotRequest selects exactly one authored Factory source.
// FactoryDir and SourcePath are interchangeable path forms; Canonical is the
// in-memory canonical form. Requests contain only identity, execution-base,
// and invocation-context values, never runtime collaborators.
type ResolveRuntimeSnapshotRequest struct {
	FactoryDir       string
	SourcePath       string
	Canonical        []byte
	ExecutionBaseDir string
	Invocation       RuntimeSnapshotInvocationContext
}

// RuntimeSnapshotInvocationContext carries value-only context that belongs to
// the invocation which requested resolution. Definitions does not retain a
// session, runtime, provider, model, filesystem, or executor handle here.
type RuntimeSnapshotInvocationContext struct {
	FactorySessionID string
	WorkflowID       string
	Arguments        *work.InvocationArguments
}

// ResolveRuntimeSnapshotResult carries one detached Runtime input. Every
// nested value is copied from the loaded source before it crosses the
// Definitions boundary.
type ResolveRuntimeSnapshotResult struct {
	Snapshot RuntimeSnapshot
}

// RuntimeSnapshot is the immutable-by-convention value projection consumed by
// Runtime activation. It deliberately contains authored/effective facts only:
// no service interfaces, callbacks, providers, model handles, executors,
// filesystems, or mutable loaded-source references are retained.
type RuntimeSnapshot struct {
	FactoryDir        string
	RuntimeBaseDir    string
	Invocation        RuntimeSnapshotInvocationContext
	DefinitionVersion *FactoryVersion
	EffectiveFactory  FactoryConfig
	Workers           []FactoryWorkerConfig
	Workstations      []FactoryWorkstationConfig
	AutomationSources []RuntimeAutomationSource
	PromptSources     []RuntimePromptSource
	BundledFiles      []PortableBundledFileReplacement
}

// Clone returns a detached copy suitable for crossing another service
// boundary. Runtime activation stores the copy it was given so a caller
// cannot change an already accepted activation by retaining one of the nested
// maps, slices, or pointers in its request.
func (snapshot RuntimeSnapshot) Clone() (RuntimeSnapshot, error) {
	config, err := CloneFactoryConfig(&snapshot.EffectiveFactory)
	if err != nil {
		return RuntimeSnapshot{}, fmt.Errorf("clone runtime snapshot Factory: %w", err)
	}
	if config == nil {
		return RuntimeSnapshot{}, fmt.Errorf("clone runtime snapshot Factory: configuration is required")
	}
	for index := range config.Workers {
		if index < len(snapshot.EffectiveFactory.Workers) {
			config.Workers[index] = CloneWorkerConfig(snapshot.EffectiveFactory.Workers[index])
		}
	}
	for index := range config.Workstations {
		if index < len(snapshot.EffectiveFactory.Workstations) {
			config.Workstations[index] = CloneWorkstationConfig(snapshot.EffectiveFactory.Workstations[index])
		}
	}

	cloned := RuntimeSnapshot{
		FactoryDir:     snapshot.FactoryDir,
		RuntimeBaseDir: snapshot.RuntimeBaseDir,
		Invocation: RuntimeSnapshotInvocationContext{
			FactorySessionID: snapshot.Invocation.FactorySessionID,
			WorkflowID:       snapshot.Invocation.WorkflowID,
			Arguments:        work.CloneInvocationArguments(snapshot.Invocation.Arguments),
		},
		EffectiveFactory:  *config,
		Workers:           make([]FactoryWorkerConfig, len(snapshot.Workers)),
		Workstations:      make([]FactoryWorkstationConfig, len(snapshot.Workstations)),
		AutomationSources: make([]RuntimeAutomationSource, len(snapshot.AutomationSources)),
		PromptSources:     append([]RuntimePromptSource(nil), snapshot.PromptSources...),
		BundledFiles:      append([]PortableBundledFileReplacement(nil), snapshot.BundledFiles...),
	}
	if snapshot.DefinitionVersion != nil {
		version := *snapshot.DefinitionVersion
		cloned.DefinitionVersion = &version
	}
	for index, worker := range snapshot.Workers {
		cloned.Workers[index] = CloneWorkerConfig(worker)
	}
	for index, workstation := range snapshot.Workstations {
		cloned.Workstations[index] = CloneWorkstationConfig(workstation)
	}
	for index, source := range snapshot.AutomationSources {
		cloned.AutomationSources[index] = source
		cloned.AutomationSources[index].Workstation = CloneWorkstationConfig(source.Workstation)
		if source.Worker != nil {
			worker := CloneWorkerConfig(*source.Worker)
			cloned.AutomationSources[index].Worker = &worker
		}
	}
	return cloned, nil
}

// RuntimeAutomationSource is the value-only automation definition associated
// with one effective workstation. Runtime and Automations use the embedded
// workstation/worker policy to create their own isolated live source state.
type RuntimeAutomationSource struct {
	ID              string
	Kind            RuntimeAutomationSourceKind
	WorkstationName string
	WorkerName      string
	Workstation     FactoryWorkstationConfig
	Worker          *FactoryWorkerConfig
	Schedule        string
	Every           string
	TriggerAtStart  bool
}

// RuntimeAutomationSourceKind classifies the authored trigger shape without
// exposing Automations implementation types.
type RuntimeAutomationSourceKind string

const (
	RuntimeAutomationSourceKindWorkstation RuntimeAutomationSourceKind = "WORKSTATION"
	RuntimeAutomationSourceKindCron        RuntimeAutomationSourceKind = "CRON"
	RuntimeAutomationSourceKindScript      RuntimeAutomationSourceKind = "SCRIPT"
	RuntimeAutomationSourceKindPoller      RuntimeAutomationSourceKind = "POLLER"
	RuntimeAutomationSourceKindHosted      RuntimeAutomationSourceKind = "HOSTED"
)

// RuntimePromptSource preserves fixed authored prompt identity separately from
// the effective Factory configuration, which intentionally omits source paths.
type RuntimePromptSource struct {
	Role       string
	Name       string
	Path       string
	IsTemplate bool
}

// RuntimeSnapshotDiagnosticCode classifies a typed resolution failure.
type RuntimeSnapshotDiagnosticCode string

const (
	RuntimeSnapshotDiagnosticInvalidRequest    RuntimeSnapshotDiagnosticCode = "invalid-request"
	RuntimeSnapshotDiagnosticInvalidDefinition RuntimeSnapshotDiagnosticCode = "invalid-definition"
	RuntimeSnapshotDiagnosticUnavailable       RuntimeSnapshotDiagnosticCode = "resolver-unavailable"
	RuntimeSnapshotDiagnosticCanceled          RuntimeSnapshotDiagnosticCode = "canceled"
)

// RuntimeSnapshotDiagnostic is a sensitive-safe failure fact that can be
// surfaced by transports without exposing loader implementation details.
type RuntimeSnapshotDiagnostic struct {
	Code    RuntimeSnapshotDiagnosticCode
	Field   string
	Message string
}

var (
	// ErrRuntimeSnapshotResolutionFailed is the stable umbrella for snapshot
	// resolution failures.
	ErrRuntimeSnapshotResolutionFailed = errors.New("runtime snapshot resolution failed")
	// ErrInvalidRuntimeSnapshotRequest reports missing or conflicting source
	// identity fields before any loader is invoked.
	ErrInvalidRuntimeSnapshotRequest = errors.New("invalid runtime snapshot request")
	// ErrInvalidRuntimeSnapshotDefinition reports a source that could not be
	// loaded, validated, or detached into an effective Factory.
	ErrInvalidRuntimeSnapshotDefinition = errors.New("invalid runtime snapshot definition")
	// ErrRuntimeSnapshotResolverUnavailable reports missing construction ports.
	ErrRuntimeSnapshotResolverUnavailable = errors.New("runtime snapshot resolver unavailable")
)

// RuntimeSnapshotResolutionError carries a stable diagnostic plus the typed
// Definitions error returned by source validation/loading when one exists.
type RuntimeSnapshotResolutionError struct {
	Diagnostic RuntimeSnapshotDiagnostic
	Cause      error
}

func (e *RuntimeSnapshotResolutionError) Error() string {
	if e == nil {
		return ErrRuntimeSnapshotResolutionFailed.Error()
	}
	message := strings.TrimSpace(e.Diagnostic.Message)
	if message == "" {
		message = string(e.Diagnostic.Code)
	}
	if e.Cause != nil {
		return fmt.Sprintf("%v: %s: %v", ErrRuntimeSnapshotResolutionFailed, message, e.Cause)
	}
	return fmt.Sprintf("%v: %s", ErrRuntimeSnapshotResolutionFailed, message)
}

func (e *RuntimeSnapshotResolutionError) Unwrap() error {
	if e == nil {
		return ErrRuntimeSnapshotResolutionFailed
	}
	return e.Cause
}

func (e *RuntimeSnapshotResolutionError) Is(target error) bool {
	if target == ErrRuntimeSnapshotResolutionFailed {
		return true
	}
	if e == nil {
		return false
	}
	switch e.Diagnostic.Code {
	case RuntimeSnapshotDiagnosticInvalidRequest:
		return target == ErrInvalidRuntimeSnapshotRequest
	case RuntimeSnapshotDiagnosticInvalidDefinition:
		return target == ErrInvalidRuntimeSnapshotDefinition
	case RuntimeSnapshotDiagnosticUnavailable:
		return target == ErrRuntimeSnapshotResolverUnavailable
	default:
		return false
	}
}

// RuntimeSnapshotOperation is the owner-composed operation used to attach
// snapshot resolution to the singular Definitions root.
type RuntimeSnapshotOperation func(
	context.Context,
	ResolveRuntimeSnapshotRequest,
) (ResolveRuntimeSnapshotResult, error)
