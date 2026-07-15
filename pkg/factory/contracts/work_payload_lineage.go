package factorycontracts

import (
	"sort"

	"github.com/portpowered/infinite-you/pkg/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	workerrunner "github.com/portpowered/infinite-you/pkg/workers/runner"
)

type WorkPayloadLineageProjection = work.WorkPayloadLineageProjection
type WorkPayloadSnapshot = work.WorkPayloadSnapshot
type WorkPayloadSnapshotKind = work.WorkPayloadSnapshotKind
type WorkPayloadContinuity = work.WorkPayloadContinuity
type WorkPayloadResolutionStatus = work.WorkPayloadResolutionStatus
type WorkPayloadRef = work.WorkPayloadRef
type WorkPayloadResolution = work.WorkPayloadResolution

const (
	WorkPayloadSnapshotKindWorkRequest     = work.WorkPayloadSnapshotKindWorkRequest
	WorkPayloadSnapshotKindDispatchOutput  = work.WorkPayloadSnapshotKindDispatchOutput
	WorkPayloadContinuityInitial           = work.WorkPayloadContinuityInitial
	WorkPayloadContinuitySameWorkID        = work.WorkPayloadContinuitySameWorkID
	WorkPayloadContinuityNewDownstreamWork = work.WorkPayloadContinuityNewDownstreamWork
	WorkPayloadResolutionResolved          = work.WorkPayloadResolutionResolved
	WorkPayloadResolutionUnavailable       = work.WorkPayloadResolutionUnavailable
)

type RunnerOptionalCapability = workerexecution.RunnerOptionalCapability
type RunnerOptionalCapabilityStatus = workerexecution.RunnerOptionalCapabilityStatus
type RunnerOptionalCapabilitySupport = workerexecution.RunnerOptionalCapabilitySupport
type RunnerCapabilities = workerexecution.RunnerCapabilities
type RunnerMetadata = workerexecution.RunnerMetadata
type RunnerSelectionSource = workerexecution.RunnerSelectionSource
type ResolvedRunnerSelection = workerexecution.ResolvedRunnerSelection

const (
	RunnerOptionalCapabilityImageInput        = workerexecution.RunnerOptionalCapabilityImageInput
	RunnerOptionalCapabilitySessionResume     = workerexecution.RunnerOptionalCapabilitySessionResume
	RunnerOptionalCapabilityStructuredOutput  = workerexecution.RunnerOptionalCapabilityStructuredOutput
	RunnerOptionalCapabilityWorkingDirectory  = workerexecution.RunnerOptionalCapabilityWorkingDirectory
	RunnerOptionalCapabilityWorktree          = workerexecution.RunnerOptionalCapabilityWorktree
	RunnerOptionalCapabilityStatusSupported   = workerexecution.RunnerOptionalCapabilityStatusSupported
	RunnerOptionalCapabilityStatusUnsupported = workerexecution.RunnerOptionalCapabilityStatusUnsupported
	RunnerIDCodex                             = workerexecution.RunnerIDCodex
	RunnerIDGemini                            = workerexecution.RunnerIDGemini
	RunnerIDKiro                              = workerexecution.RunnerIDKiro
	RunnerIDCursorCLI                         = workerexecution.RunnerIDCursorCLI
	RunnerIDOpenCode                          = workerexecution.RunnerIDOpenCode
	RunnerIDPi                                = workerexecution.RunnerIDPi
	RunnerIDAgy                               = workerexecution.RunnerIDAgy
	RunnerSelectionSourceWorkstation          = workerexecution.RunnerSelectionSourceWorkstation
	RunnerSelectionSourceFactory              = workerexecution.RunnerSelectionSourceFactory
	RunnerSelectionSourceLegacyProvider       = workerexecution.RunnerSelectionSourceLegacyProvider
	RunnerSelectionSourceDefault              = workerexecution.RunnerSelectionSourceDefault
)

func CanonicalChainingTraceIDs(traceIDs []string) []string {
	return work.CanonicalChainingTraceIDs(traceIDs)
}

func PreviousChainingTraceIDsFromTokens(tokens []Token) []string {
	traceIDs := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == DataTypeResource {
			continue
		}
		traceIDs = append(traceIDs, firstNonEmptyString(token.Color.CurrentChainingTraceID, token.Color.TraceID))
	}
	return work.CanonicalChainingTraceIDs(traceIDs)
}

func PreviousChainingTraceIDsFromTokenColors(colors []TokenColor) []string {
	traceIDs := make([]string, 0, len(colors))
	for _, color := range colors {
		if color.DataType == DataTypeResource {
			continue
		}
		traceIDs = append(traceIDs, firstNonEmptyString(color.CurrentChainingTraceID, color.TraceID))
	}
	return work.CanonicalChainingTraceIDs(traceIDs)
}

func PreviousChainingTraceIDsFromWorkItems(items []FactoryWorkItem) []string {
	return work.PreviousChainingTraceIDsFromWorkItems(items)
}

func CurrentChainingTraceIDFromWorkItems(items []FactoryWorkItem) string {
	return work.CurrentChainingTraceIDFromWorkItems(items)
}

func CurrentChainingTraceIDFromTokens(tokens []Token) string {
	for _, token := range tokens {
		if token.Color.DataType == DataTypeResource || token.Color.WorkTypeID == SystemTimeWorkTypeID {
			continue
		}
		return firstNonEmptyString(token.Color.CurrentChainingTraceID, token.Color.TraceID)
	}
	for _, token := range tokens {
		if token.Color.DataType != DataTypeResource {
			return firstNonEmptyString(token.Color.CurrentChainingTraceID, token.Color.TraceID)
		}
	}
	return ""
}

func ChainingTraceDepthForTokenColor(color TokenColor) int {
	if color.ChainingTraceDepth > 0 {
		return color.ChainingTraceDepth
	}
	if firstNonEmptyString(color.CurrentChainingTraceID, color.TraceID) != "" {
		return 1
	}
	return 0
}

func ChainingTraceDepthForWorkItem(item FactoryWorkItem) int {
	return work.ChainingTraceDepthForWorkItem(item)
}

func ChainingTraceDepthFromTokenColors(colors []TokenColor) int {
	depth := 0
	for _, color := range colors {
		if color.DataType == DataTypeResource {
			continue
		}
		if candidate := ChainingTraceDepthForTokenColor(color); candidate > depth {
			depth = candidate
		}
	}
	if depth > 0 {
		return depth + 1
	}
	if CurrentChainingTraceIDFromTokenColors(colors) != "" {
		return 1
	}
	return 0
}

func CurrentChainingTraceIDFromTokenColors(colors []TokenColor) string {
	for _, color := range colors {
		if color.DataType == DataTypeResource || color.WorkTypeID == SystemTimeWorkTypeID {
			continue
		}
		return firstNonEmptyString(color.CurrentChainingTraceID, color.TraceID)
	}
	for _, color := range colors {
		if color.DataType != DataTypeResource {
			return firstNonEmptyString(color.CurrentChainingTraceID, color.TraceID)
		}
	}
	return ""
}

func sortedStringKeys(values map[string]WorkPayloadRef) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func appendUniqueString(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

type RunnerExecutionRequest = workerexecution.RunnerExecutionRequest
type RunnerExecutionResult = workerexecution.RunnerExecutionResult
type RunnerToolExecutionMode = workerexecution.RunnerToolExecutionMode
type RunnerBaselineCapability = workerexecution.RunnerBaselineCapability

const (
	RunnerToolExecutionModeRequired          = workerexecution.RunnerToolExecutionModeRequired
	RunnerToolExecutionModeDisabled          = workerexecution.RunnerToolExecutionModeDisabled
	RunnerBaselineCapabilityPromptSubmission = workerexecution.RunnerBaselineCapabilityPromptSubmission
	RunnerBaselineCapabilityToolExecution    = workerexecution.RunnerBaselineCapabilityToolExecution
)

func V1RunnerBaselineCapabilities() []RunnerBaselineCapability {
	return workerrunner.V1BaselineCapabilities()
}

func NewRunnerCapabilities(optional ...RunnerOptionalCapabilitySupport) RunnerCapabilities {
	return workerrunner.NewCapabilities(optional...)
}

func BuiltInRunnerMetadata(id string) (RunnerMetadata, bool) {
	return workerrunner.BuiltInRunnerMetadata(id)
}

func IsBuiltInRunnerID(id string) bool {
	return workerrunner.IsBuiltInRunnerID(id)
}

func ResolveOpenCodeAgent(workstationAgent, workerAgent string) string {
	return workerrunner.ResolveOpenCodeAgent(workstationAgent, workerAgent)
}

func ValidateOpenCodeAgentForRunnerSelection(workstationAgent, workerAgent string, selection ResolvedRunnerSelection) error {
	return workerrunner.ValidateOpenCodeAgentForRunnerSelection(workstationAgent, workerAgent, selection)
}

func ResolveRunnerSelection(workstationRunner, factoryRunner, workerModelProvider string) ResolvedRunnerSelection {
	return workerrunner.ResolveRunnerSelection(workstationRunner, factoryRunner, workerModelProvider)
}

func NormalizeRunnerID(id string) string {
	return workerrunner.NormalizeRunnerID(id)
}
