package invocationreturnpolicy

import (
	"fmt"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work/internal/lineagegraph"
)

const (
	ReturnPolicySubmittedWorkTerminal = "SUBMITTED_WORK_TERMINAL"
	ReturnPolicyExplicit              = "EXPLICIT"

	invocationReturnPolicySubmittedWorkTerminal = ReturnPolicySubmittedWorkTerminal
	invocationReturnPolicyExplicit              = ReturnPolicyExplicit
)

// PrimaryResultErrorCode is the stable machine-readable failure code for
// invocation primary-result selection.
type PrimaryResultErrorCode string

const (
	PrimaryResultErrorCodeUnresolved  PrimaryResultErrorCode = "INVOCATION_PRIMARY_RESULT_UNRESOLVED"
	PrimaryResultErrorCodeFailed      PrimaryResultErrorCode = "INVOCATION_RUNTIME_FAILURE"
	PrimaryResultErrorCodeBlocked     PrimaryResultErrorCode = "INVOCATION_BLOCKED"
	PrimaryResultErrorCodeNeedsHuman  PrimaryResultErrorCode = "INVOCATION_NEEDS_HUMAN"
	PrimaryResultErrorCodePaused      PrimaryResultErrorCode = "INVOCATION_PAUSED"
	PrimaryResultErrorCodeInterrupted PrimaryResultErrorCode = "INVOCATION_INTERRUPTED"
)

// PrimaryResultSelectionInput carries the selected-tick world state for one
// invocation request together with the authored factory return policy.
type PrimaryResultSelectionInput struct {
	RequestID        string
	InvocationReturn *InvocationReturnConfig
	WorldState       InvocationWorldStateProvider
}

// PrimaryResultSelection carries the resolved primary work content returned for
// one invocation.
type PrimaryResultSelection struct {
	RequestID     string
	Policy        string
	WorkID        string
	WorkTypeName  string
	WorkName      string
	TerminalState string
	PrimaryResult []ContentPart
}

// InvocationFailureContext carries sanitized session and work identifiers that
// help operators recover from non-success invocation outcomes.
type InvocationFailureContext struct {
	SessionID string
	WorkID    string
	WorkName  string
	WorkState string
}

// PrimaryResultError describes a stable primary-result selection failure.
type PrimaryResultError struct {
	Code      PrimaryResultErrorCode
	Message   string
	RequestID string
	Policy    string
	Context   InvocationFailureContext
}

func (e *PrimaryResultError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func invocationWorldState(input PrimaryResultSelectionInput) InvocationWorldState {
	if input.WorldState == nil {
		return InvocationWorldState{}
	}
	return input.WorldState.InvocationWorldState()
}

// ResolvePrimaryResult applies the canonical invocation primary-result rules
// against one selected-tick factory world state. The singular Work root Service
// publishes this capability as Service.ResolvePrimaryResult.
func ResolvePrimaryResult(input PrimaryResultSelectionInput) (PrimaryResultSelection, error) {
	requestID := strings.TrimSpace(input.RequestID)
	if requestID == "" {
		return PrimaryResultSelection{}, fmt.Errorf("invocation request ID is required")
	}

	request, ok := invocationWorldState(input).WorkRequestsByID[requestID]
	if !ok || len(request.WorkItems) == 0 {
		return PrimaryResultSelection{}, unresolvedPrimaryResultError(
			requestID,
			resolvedInvocationReturnPolicy(input.InvocationReturn),
			"invocation request has no submitted work in the selected world state",
		)
	}

	switch policy := resolvedInvocationReturnPolicy(input.InvocationReturn); policy {
	case invocationReturnPolicySubmittedWorkTerminal:
		return resolveSubmittedWorkTerminalPrimaryResult(requestID, invocationWorldState(input), request.WorkItems)
	case invocationReturnPolicyExplicit:
		return resolveExplicitPrimaryResult(requestID, invocationWorldState(input), request.WorkItems, input.InvocationReturn)
	default:
		return PrimaryResultSelection{}, fmt.Errorf("%w: %q", ErrUnsupportedReturnPolicy, policy)
	}
}

func resolveSubmittedWorkTerminalPrimaryResult(
	requestID string,
	state InvocationWorldState,
	submitted []WorkItem,
) (PrimaryResultSelection, error) {
	for _, item := range submitted {
		logicalWorkID := logicalWorkIDForSubmittedItem(state.PayloadLineage, item.ID)
		for _, terminalWorkID := range sortedTerminalWorkIDs(state.TerminalWorkByID) {
			terminal := state.TerminalWorkByID[terminalWorkID]
			if isFailedTerminalWork(terminal) {
				continue
			}
			if logicalWorkID == "" {
				if terminal.WorkItem.ID != item.ID {
					continue
				}
			} else if logicalWorkIDForSelectedItem(state.PayloadLineage, terminal.WorkItem.ID) != logicalWorkID {
				continue
			}
			return selectedPrimaryResult(
				requestID,
				invocationReturnPolicySubmittedWorkTerminal,
				terminal.WorkItem,
			), nil
		}
	}

	return PrimaryResultSelection{}, unresolvedPrimaryResultError(
		requestID,
		invocationReturnPolicySubmittedWorkTerminal,
		"submitted work did not resolve to terminal output",
	)
}

func resolveExplicitPrimaryResult(
	requestID string,
	state InvocationWorldState,
	submitted []WorkItem,
	cfg *InvocationReturnConfig,
) (PrimaryResultSelection, error) {
	scope := invocationScopeWorkIDs(state.PayloadLineage, submitted)
	matches := collectExplicitPrimaryResultMatches(state, scope, cfg)
	if len(matches) == 0 {
		matches = collectExplicitPrimaryResultMatchesForInvocationTrace(state, requestID, submitted, cfg)
	}
	if len(matches) == 0 {
		matches = collectUniqueExplicitTerminalMatches(state, cfg)
	}

	switch len(matches) {
	case 1:
		return selectedPrimaryResult(requestID, invocationReturnPolicyExplicit, matches[0]), nil
	case 0:
		return PrimaryResultSelection{}, unresolvedPrimaryResultError(
			requestID,
			invocationReturnPolicyExplicit,
			"explicit invocation return policy did not resolve terminal output in scope",
		)
	default:
		return PrimaryResultSelection{}, unresolvedPrimaryResultError(
			requestID,
			invocationReturnPolicyExplicit,
			fmt.Sprintf("explicit invocation return policy matched %d terminal outputs in scope", len(matches)),
		)
	}
}

func collectExplicitPrimaryResultMatches(
	state InvocationWorldState,
	scope map[string]struct{},
	cfg *InvocationReturnConfig,
) []WorkItem {
	matches := make([]WorkItem, 0, len(state.TerminalWorkByID))
	for _, terminalWorkID := range sortedTerminalWorkIDs(state.TerminalWorkByID) {
		terminal := state.TerminalWorkByID[terminalWorkID]
		if isFailedTerminalWork(terminal) {
			continue
		}
		if _, ok := scope[terminal.WorkItem.ID]; !ok {
			continue
		}
		if !explicitPrimaryResultMatches(terminal.WorkItem, cfg) {
			continue
		}
		matches = append(matches, terminal.WorkItem)
	}
	return matches
}

func collectExplicitPrimaryResultMatchesForInvocationTrace(
	state InvocationWorldState,
	requestID string,
	submitted []WorkItem,
	cfg *InvocationReturnConfig,
) []WorkItem {
	traceIDs := invocationTraceIDs(state, requestID, submitted)
	if len(traceIDs) == 0 {
		return nil
	}

	matches := make([]WorkItem, 0, 1)
	for _, terminalWorkID := range sortedTerminalWorkIDs(state.TerminalWorkByID) {
		terminal := state.TerminalWorkByID[terminalWorkID]
		if isFailedTerminalWork(terminal) {
			continue
		}
		if _, ok := traceIDs[strings.TrimSpace(terminal.WorkItem.TraceID)]; !ok {
			continue
		}
		if !explicitPrimaryResultMatches(terminal.WorkItem, cfg) {
			continue
		}
		matches = append(matches, terminal.WorkItem)
	}
	return matches
}

func invocationTraceIDs(
	state InvocationWorldState,
	requestID string,
	submitted []WorkItem,
) map[string]struct{} {
	traceIDs := make(map[string]struct{})
	if request, ok := state.WorkRequestsByID[requestID]; ok {
		if traceID := strings.TrimSpace(request.TraceID); traceID != "" {
			traceIDs[traceID] = struct{}{}
		}
	}
	for _, item := range submitted {
		if traceID := strings.TrimSpace(item.TraceID); traceID != "" {
			traceIDs[traceID] = struct{}{}
		}
	}
	return traceIDs
}

func collectUniqueExplicitTerminalMatches(
	state InvocationWorldState,
	cfg *InvocationReturnConfig,
) []WorkItem {
	matches := make([]WorkItem, 0, 1)
	for _, terminalWorkID := range sortedTerminalWorkIDs(state.TerminalWorkByID) {
		terminal := state.TerminalWorkByID[terminalWorkID]
		if isFailedTerminalWork(terminal) {
			continue
		}
		if !explicitPrimaryResultMatches(terminal.WorkItem, cfg) {
			continue
		}
		matches = append(matches, terminal.WorkItem)
	}
	return matches
}

func isFailedTerminalWork(terminal InvocationTerminalWork) bool {
	return strings.TrimSpace(terminal.Status) == "FAILED"
}

func selectedPrimaryResult(requestID, policy string, item WorkItem) PrimaryResultSelection {
	return PrimaryResultSelection{
		RequestID:     requestID,
		Policy:        policy,
		WorkID:        item.ID,
		WorkTypeName:  item.WorkTypeID,
		WorkName:      item.DisplayName,
		TerminalState: terminalStateName(item),
		PrimaryResult: cloneContentParts(item.Content),
	}
}

func unresolvedPrimaryResultError(requestID, policy, reason string) error {
	return &PrimaryResultError{
		Code:      PrimaryResultErrorCodeUnresolved,
		RequestID: requestID,
		Policy:    policy,
		Message:   fmt.Sprintf("invocation primary result unresolved: %s", reason),
	}
}

// ClassifyInvocationControlState inspects reconstructed session and dispatch
// lifecycle facts that explain why an invocation stopped without a primary
// result.
func ClassifyInvocationControlState(
	sessionID string,
	snapshotFactoryState string,
	input PrimaryResultSelectionInput,
) (*PrimaryResultError, bool) {
	if paused := classifyPausedInvocation(sessionID, snapshotFactoryState, input); paused != nil {
		return paused, true
	}
	if interrupted := classifyInterruptedInvocation(sessionID, input); interrupted != nil {
		return interrupted, true
	}
	return nil, false
}

// ClassifyMissingPrimaryResult inspects the selected-tick world state for
// authored non-success work states that explain why no primary result exists.
func ClassifyMissingPrimaryResult(input PrimaryResultSelectionInput) (*PrimaryResultError, bool) {
	requestID := strings.TrimSpace(input.RequestID)
	if requestID == "" {
		return nil, false
	}
	request, ok := invocationWorldState(input).WorkRequestsByID[requestID]
	if !ok || len(request.WorkItems) == 0 {
		return nil, false
	}

	scope := invocationScopeWorkIDs(invocationWorldState(input).PayloadLineage, request.WorkItems)
	for _, stateName := range []string{"blocked", "needs-human"} {
		item, found := scopedWorkItemInState(invocationWorldState(input).WorkItemsByID, scope, stateName)
		if !found {
			continue
		}
		return classifiedPrimaryResultError(requestID, resolvedInvocationReturnPolicy(input.InvocationReturn), item), true
	}
	return nil, false
}

// ClassifyMissingPrimaryResultWorkItem maps one current work item onto the
// stable blocked or needs-human invocation outcome when that state explains the
// missing primary result.
func ClassifyMissingPrimaryResultWorkItem(
	requestID string,
	invocationReturn *InvocationReturnConfig,
	item WorkItem,
	sessionID string,
) *PrimaryResultError {
	result := classifiedPrimaryResultError(requestID, resolvedInvocationReturnPolicy(invocationReturn), item)
	if result != nil {
		result.Context.SessionID = strings.TrimSpace(sessionID)
	}
	return result
}

// ClassifyFailedInvocation inspects scoped failed work or failed session
// lifecycle facts that explain why no primary result was produced.
func ClassifyFailedInvocation(
	sessionID string,
	input PrimaryResultSelectionInput,
) (*PrimaryResultError, bool) {
	requestID := strings.TrimSpace(input.RequestID)
	if requestID == "" {
		return nil, false
	}
	request, ok := invocationWorldState(input).WorkRequestsByID[requestID]
	if !ok || len(request.WorkItems) == 0 {
		return nil, false
	}

	scope := invocationScopeWorkIDs(invocationWorldState(input).PayloadLineage, request.WorkItems)
	if item, found := scopedFailedWorkItem(invocationWorldState(input), scope, request.WorkItems); found {
		return failedPrimaryResultError(
			requestID,
			resolvedInvocationReturnPolicy(input.InvocationReturn),
			item,
		), true
	}
	if item, found := requestMatchedFailedWorkItem(invocationWorldState(input), requestID); found {
		return failedPrimaryResultError(
			requestID,
			resolvedInvocationReturnPolicy(input.InvocationReturn),
			item,
		), true
	}

	if strings.TrimSpace(invocationWorldState(input).FactoryState) == "FAILED" {
		sessionLabel := invocationSessionLabel(sessionID, invocationWorldState(input))
		return &PrimaryResultError{
			Code:      PrimaryResultErrorCodeFailed,
			RequestID: requestID,
			Policy:    resolvedInvocationReturnPolicy(input.InvocationReturn),
			Message:   fmt.Sprintf("invocation failed: session %q reached a failed state before a primary result was available", sessionLabel),
			Context:   invocationFailureContextFromScopedWork(sessionID, input),
		}, true
	}
	if bracket := invocationWorldState(input).SessionBracket; bracket != nil && strings.TrimSpace(bracket.FinalStatus) == "FAILED" {
		sessionLabel := invocationSessionLabel(sessionID, invocationWorldState(input))
		return &PrimaryResultError{
			Code:      PrimaryResultErrorCodeFailed,
			RequestID: requestID,
			Policy:    resolvedInvocationReturnPolicy(input.InvocationReturn),
			Message:   fmt.Sprintf("invocation failed: session %q reached a failed state before a primary result was available", sessionLabel),
			Context:   invocationFailureContextFromScopedWork(sessionID, input),
		}, true
	}

	return nil, false
}

func classifiedPrimaryResultError(
	requestID string,
	policy string,
	item WorkItem,
) *PrimaryResultError {
	stateName := currentWorkStateName(item)
	stateLabel := workStateLabel(item)
	workLabel := workDisplayLabel(item)
	context := invocationFailureContextFromWorkItem("", item)
	switch stateName {
	case "blocked":
		return &PrimaryResultError{
			Code:      PrimaryResultErrorCodeBlocked,
			RequestID: requestID,
			Policy:    policy,
			Message:   fmt.Sprintf("invocation blocked: work %q is waiting in state %q", workLabel, stateLabel),
			Context:   context,
		}
	case "needs-human":
		return &PrimaryResultError{
			Code:      PrimaryResultErrorCodeNeedsHuman,
			RequestID: requestID,
			Policy:    policy,
			Message:   fmt.Sprintf("invocation needs human input: work %q is waiting in state %q", workLabel, stateLabel),
			Context:   context,
		}
	default:
		return nil
	}
}

func failedPrimaryResultError(
	requestID string,
	policy string,
	item WorkItem,
) *PrimaryResultError {
	return &PrimaryResultError{
		Code:      PrimaryResultErrorCodeFailed,
		RequestID: requestID,
		Policy:    policy,
		Message:   fmt.Sprintf("invocation failed: work %q reached failed state %q before a primary result was available", workDisplayLabel(item), workStateLabel(item)),
		Context:   invocationFailureContextFromWorkItem("", item),
	}
}

func classifyPausedInvocation(
	sessionID string,
	snapshotFactoryState string,
	input PrimaryResultSelectionInput,
) *PrimaryResultError {
	factoryState := strings.TrimSpace(snapshotFactoryState)
	if factoryState == "" {
		factoryState = strings.TrimSpace(invocationWorldState(input).FactoryState)
	}
	if factoryState != "PAUSED" {
		if bracket := invocationWorldState(input).SessionBracket; bracket == nil || strings.TrimSpace(bracket.LifecycleControlStatus) != "PAUSED" {
			return nil
		}
	}
	sessionLabel := invocationSessionLabel(sessionID, invocationWorldState(input))
	context := invocationFailureContextFromScopedWork(sessionID, input)
	return &PrimaryResultError{
		Code:      PrimaryResultErrorCodePaused,
		RequestID: strings.TrimSpace(input.RequestID),
		Policy:    resolvedInvocationReturnPolicy(input.InvocationReturn),
		Message:   fmt.Sprintf("invocation paused: session %q is paused; resume the session to continue waiting for primary result", sessionLabel),
		Context:   context,
	}
}

func classifyInterruptedInvocation(
	sessionID string,
	input PrimaryResultSelectionInput,
) *PrimaryResultError {
	requestID := strings.TrimSpace(input.RequestID)
	if requestID == "" {
		return nil
	}
	request, ok := invocationWorldState(input).WorkRequestsByID[requestID]
	if !ok || len(request.WorkItems) == 0 {
		return nil
	}

	scope := invocationScopeWorkIDs(invocationWorldState(input).PayloadLineage, request.WorkItems)
	sessionLabel := invocationSessionLabel(sessionID, invocationWorldState(input))
	if runtime := invocationWorldState(input).JavaScriptRuntime; runtime != nil {
		dispatches := append([]InvocationDispatchState(nil), runtime.Dispatches...)
		sort.Slice(dispatches, func(i, j int) bool {
			return dispatches[i].ID < dispatches[j].ID
		})
		for _, dispatch := range dispatches {
			if strings.TrimSpace(dispatch.Status) != "INTERRUPTED" {
				continue
			}
			if !dispatchMatchesInvocationScope(dispatch, scope) {
				continue
			}
			work, workLabel := interruptedDispatchWorkItem(dispatch, invocationWorldState(input).WorkItemsByID, scope)
			return interruptedPrimaryResultError(
				sessionLabel,
				dispatch.ID,
				work,
				workLabel,
				sessionID,
				requestID,
				resolvedInvocationReturnPolicy(input.InvocationReturn),
			)
		}
	}

	if bracket := invocationWorldState(input).SessionBracket; bracket != nil {
		if strings.TrimSpace(bracket.FinalStatus) == "INTERRUPTED" || strings.TrimSpace(bracket.FailureReason) == "DISPATCH_INTERRUPTED" {
			return &PrimaryResultError{
				Code:      PrimaryResultErrorCodeInterrupted,
				RequestID: requestID,
				Policy:    resolvedInvocationReturnPolicy(input.InvocationReturn),
				Message:   fmt.Sprintf("invocation interrupted: session %q was interrupted before a primary result was available", sessionLabel),
				Context:   invocationFailureContextFromScopedWork(sessionID, input),
			}
		}
	}

	return nil
}

func interruptedPrimaryResultError(
	sessionLabel string,
	dispatchID string,
	work WorkItem,
	workLabel string,
	sessionID string,
	requestID string,
	policy string,
) *PrimaryResultError {
	message := fmt.Sprintf(
		"invocation interrupted: session %q dispatch %q was interrupted before a primary result was available",
		sessionLabel,
		strings.TrimSpace(dispatchID),
	)
	if strings.TrimSpace(workLabel) != "" {
		message = fmt.Sprintf(
			"invocation interrupted: session %q dispatch %q for work %q was interrupted before a primary result was available",
			sessionLabel,
			strings.TrimSpace(dispatchID),
			strings.TrimSpace(workLabel),
		)
	}
	return &PrimaryResultError{
		Code:      PrimaryResultErrorCodeInterrupted,
		RequestID: requestID,
		Policy:    policy,
		Message:   message,
		Context:   invocationFailureContextFromWorkItem(sessionID, work),
	}
}

func invocationSessionLabel(sessionID string, state InvocationWorldState) string {
	if trimmed := strings.TrimSpace(sessionID); trimmed != "" {
		return trimmed
	}
	if state.SessionBracket != nil && strings.TrimSpace(state.SessionBracket.SessionID) != "" {
		return strings.TrimSpace(state.SessionBracket.SessionID)
	}
	return "unknown"
}

func dispatchMatchesInvocationScope(dispatch InvocationDispatchState, scope map[string]struct{}) bool {
	if len(scope) == 0 || len(dispatch.RelatedWorkIDs) == 0 {
		return len(scope) == 0
	}
	for _, workID := range dispatch.RelatedWorkIDs {
		if _, ok := scope[strings.TrimSpace(workID)]; ok {
			return true
		}
	}
	return false
}

func interruptedDispatchWorkItem(
	dispatch InvocationDispatchState,
	workItems map[string]WorkItem,
	scope map[string]struct{},
) (WorkItem, string) {
	for _, workID := range dispatch.RelatedWorkIDs {
		trimmed := strings.TrimSpace(workID)
		if _, ok := scope[trimmed]; !ok {
			continue
		}
		item, ok := workItems[trimmed]
		if !ok {
			return WorkItem{ID: trimmed}, trimmed
		}
		return item, workDisplayLabel(item)
	}
	return WorkItem{}, ""
}

func invocationFailureContextFromScopedWork(
	sessionID string,
	input PrimaryResultSelectionInput,
) InvocationFailureContext {
	context := InvocationFailureContext{SessionID: strings.TrimSpace(sessionID)}
	requestID := strings.TrimSpace(input.RequestID)
	if requestID == "" {
		return context
	}
	request, ok := invocationWorldState(input).WorkRequestsByID[requestID]
	if !ok || len(request.WorkItems) == 0 {
		return context
	}

	scope := invocationScopeWorkIDs(invocationWorldState(input).PayloadLineage, request.WorkItems)
	if item, found := scopedCurrentWorkItem(invocationWorldState(input).WorkItemsByID, scope); found {
		return invocationFailureContextFromWorkItem(sessionID, item)
	}
	return invocationFailureContextFromWorkItem(sessionID, request.WorkItems[0])
}

func invocationFailureContextFromWorkItem(sessionID string, item WorkItem) InvocationFailureContext {
	return InvocationFailureContext{
		SessionID: strings.TrimSpace(sessionID),
		WorkID:    strings.TrimSpace(item.ID),
		WorkName:  strings.TrimSpace(workDisplayLabel(item)),
		WorkState: strings.TrimSpace(workStateLabel(item)),
	}
}

func scopedCurrentWorkItem(
	workItems map[string]WorkItem,
	scope map[string]struct{},
) (WorkItem, bool) {
	if len(workItems) == 0 || len(scope) == 0 {
		return WorkItem{}, false
	}
	workIDs := make([]string, 0, len(workItems))
	for workID := range workItems {
		if _, ok := scope[workID]; ok {
			workIDs = append(workIDs, workID)
		}
	}
	sort.Strings(workIDs)
	for _, workID := range workIDs {
		item := workItems[workID]
		if strings.TrimSpace(workID) == "" {
			continue
		}
		return item, true
	}
	return WorkItem{}, false
}

func resolvedInvocationReturnPolicy(cfg *InvocationReturnConfig) string {
	if cfg == nil || strings.TrimSpace(cfg.Policy) == "" {
		return invocationReturnPolicySubmittedWorkTerminal
	}
	return strings.TrimSpace(cfg.Policy)
}

func logicalWorkIDForSubmittedItem(lineage lineagegraph.WorkPayloadLineageProjection, workID string) string {
	resolution := lineage.ResolveInitialSubmittedSnapshot(workID)
	if resolution.Status == lineagegraph.WorkPayloadResolutionResolved && resolution.Snapshot != nil {
		return resolution.Snapshot.LogicalWorkID
	}
	return workID
}

func logicalWorkIDForSelectedItem(lineage lineagegraph.WorkPayloadLineageProjection, workID string) string {
	resolution := lineage.ResolveSelectedWorkSnapshot(workID)
	if resolution.Status == lineagegraph.WorkPayloadResolutionResolved && resolution.Snapshot != nil && resolution.Snapshot.LogicalWorkID != "" {
		return resolution.Snapshot.LogicalWorkID
	}
	return workID
}

func explicitPrimaryResultMatches(item WorkItem, cfg *InvocationReturnConfig) bool {
	if cfg == nil {
		return false
	}
	if item.WorkTypeID != strings.TrimSpace(cfg.WorkTypeName) {
		return false
	}
	if terminalStateName(item) != strings.TrimSpace(cfg.TerminalState) {
		return false
	}
	if workName := strings.TrimSpace(cfg.WorkName); workName != "" && item.DisplayName != workName {
		return false
	}
	return true
}

func terminalStateName(item WorkItem) string {
	if state := strings.TrimSpace(item.State); state != "" {
		return state
	}
	_, suffix, ok := strings.Cut(strings.TrimSpace(item.PlaceID), ":")
	if ok {
		return suffix
	}
	return strings.TrimSpace(item.PlaceID)
}

func sortedTerminalWorkIDs(terminal map[string]InvocationTerminalWork) []string {
	if len(terminal) == 0 {
		return nil
	}
	ids := make([]string, 0, len(terminal))
	for workID := range terminal {
		ids = append(ids, workID)
	}
	sort.Strings(ids)
	return ids
}

func scopedWorkItemInState(
	workItems map[string]WorkItem,
	scope map[string]struct{},
	wantState string,
) (WorkItem, bool) {
	if len(workItems) == 0 || len(scope) == 0 {
		return WorkItem{}, false
	}
	ids := make([]string, 0, len(workItems))
	for workID := range workItems {
		if _, ok := scope[workID]; ok {
			ids = append(ids, workID)
		}
	}
	sort.Strings(ids)
	for _, workID := range ids {
		item := workItems[workID]
		if currentWorkStateName(item) == wantState {
			return item, true
		}
	}
	return WorkItem{}, false
}

func scopedFailedWorkItem(
	state InvocationWorldState,
	scope map[string]struct{},
	submitted []WorkItem,
) (WorkItem, bool) {
	if item, ok := scopedWorkItemInState(state.FailedWorkItemsByID, scope, "failed"); ok {
		return item, true
	}
	terminalIDs := sortedTerminalWorkIDs(state.TerminalWorkByID)
	for _, workID := range terminalIDs {
		if _, ok := scope[workID]; !ok {
			continue
		}
		terminal := state.TerminalWorkByID[workID]
		if strings.TrimSpace(terminal.Status) != "FAILED" {
			continue
		}
		return terminal.WorkItem, true
	}
	traceIDs := submittedTraceIDs(submitted)
	if len(traceIDs) == 0 {
		return WorkItem{}, false
	}
	return traceMatchedFailedWorkItem(state.FailedWorkItemsByID, traceIDs)
}

func submittedTraceIDs(submitted []WorkItem) map[string]struct{} {
	traceIDs := make(map[string]struct{}, len(submitted))
	for _, item := range submitted {
		traceID := strings.TrimSpace(item.TraceID)
		if traceID == "" {
			continue
		}
		traceIDs[traceID] = struct{}{}
	}
	return traceIDs
}

func traceMatchedFailedWorkItem(
	workItems map[string]WorkItem,
	traceIDs map[string]struct{},
) (WorkItem, bool) {
	if len(workItems) == 0 || len(traceIDs) == 0 {
		return WorkItem{}, false
	}
	ids := make([]string, 0, len(workItems))
	for workID := range workItems {
		ids = append(ids, workID)
	}
	sort.Strings(ids)
	for _, workID := range ids {
		item := workItems[workID]
		if _, ok := traceIDs[strings.TrimSpace(item.TraceID)]; ok {
			return item, true
		}
	}
	return WorkItem{}, false
}

func requestMatchedFailedWorkItem(
	state InvocationWorldState,
	requestID string,
) (WorkItem, bool) {
	trimmedRequestID := strings.TrimSpace(requestID)
	if trimmedRequestID == "" || len(state.WorkStateChangesByWorkID) == 0 {
		return WorkItem{}, false
	}
	workIDs := make([]string, 0, len(state.WorkStateChangesByWorkID))
	for workID := range state.WorkStateChangesByWorkID {
		workIDs = append(workIDs, workID)
	}
	sort.Strings(workIDs)
	for _, workID := range workIDs {
		records := state.WorkStateChangesByWorkID[workID]
		for _, record := range records {
			if strings.TrimSpace(record.RequestID) != trimmedRequestID {
				continue
			}
			if strings.TrimSpace(record.ToState) != "failed" && placeStateName(record.ToPlaceID) != "failed" {
				continue
			}
			if item, ok := state.FailedWorkItemsByID[workID]; ok {
				return item, true
			}
			if item, ok := state.WorkItemsByID[workID]; ok {
				return item, true
			}
			return WorkItem{
				ID:         workID,
				WorkTypeID: strings.TrimSpace(record.WorkTypeName),
				State:      "failed",
				PlaceID:    strings.TrimSpace(record.ToPlaceID),
			}, true
		}
	}
	return WorkItem{}, false
}

func placeStateName(placeID string) string {
	trimmed := strings.TrimSpace(placeID)
	if trimmed == "" {
		return ""
	}
	if _, suffix, ok := strings.Cut(trimmed, ":"); ok {
		return suffix
	}
	return trimmed
}

func workDisplayLabel(item WorkItem) string {
	if label := strings.TrimSpace(item.DisplayName); label != "" {
		return label
	}
	if label := strings.TrimSpace(item.ID); label != "" {
		return label
	}
	return "submitted work"
}

func workStateLabel(item WorkItem) string {
	if placeID := strings.TrimSpace(item.PlaceID); placeID != "" {
		return placeID
	}
	stateName := currentWorkStateName(item)
	if workType := strings.TrimSpace(item.WorkTypeID); workType != "" && stateName != "" {
		return workType + ":" + stateName
	}
	return stateName
}

func currentWorkStateName(item WorkItem) string {
	if placeID := strings.TrimSpace(item.PlaceID); placeID != "" {
		if _, suffix, ok := strings.Cut(placeID, ":"); ok {
			return suffix
		}
		return placeID
	}
	return terminalStateName(item)
}

func invocationScopeWorkIDs(
	lineage lineagegraph.WorkPayloadLineageProjection,
	submitted []WorkItem,
) map[string]struct{} {
	scopeWorkIDs := make(map[string]struct{}, len(submitted))
	scopeLogicalIDs := make(map[string]struct{}, len(submitted))
	for _, item := range submitted {
		if item.ID == "" {
			continue
		}
		scopeWorkIDs[item.ID] = struct{}{}
		scopeLogicalIDs[logicalWorkIDForSubmittedItem(lineage, item.ID)] = struct{}{}
	}

	for _, snapshot := range sortedLineageSnapshots(lineage) {
		if snapshot.WorkID == "" {
			continue
		}
		if _, ok := scopeWorkIDs[snapshot.WorkID]; ok {
			scopeLogicalIDs[snapshot.LogicalWorkID] = struct{}{}
			continue
		}
		if _, ok := scopeLogicalIDs[snapshot.LogicalWorkID]; ok || hasScopedParent(snapshot, scopeWorkIDs, scopeLogicalIDs) {
			scopeWorkIDs[snapshot.WorkID] = struct{}{}
			if snapshot.LogicalWorkID != "" {
				scopeLogicalIDs[snapshot.LogicalWorkID] = struct{}{}
			}
		}
	}

	return scopeWorkIDs
}

func sortedLineageSnapshots(lineage lineagegraph.WorkPayloadLineageProjection) []lineagegraph.WorkPayloadSnapshot {
	if len(lineage.SnapshotsByID) == 0 {
		return nil
	}
	snapshots := make([]lineagegraph.WorkPayloadSnapshot, 0, len(lineage.SnapshotsByID))
	for _, snapshot := range lineage.SnapshotsByID {
		snapshots = append(snapshots, snapshot)
	}
	sort.Slice(snapshots, func(i, j int) bool {
		if snapshots[i].ObservedTick == snapshots[j].ObservedTick {
			return snapshots[i].SnapshotID < snapshots[j].SnapshotID
		}
		return snapshots[i].ObservedTick < snapshots[j].ObservedTick
	})
	return snapshots
}

func hasScopedParent(
	snapshot lineagegraph.WorkPayloadSnapshot,
	scopeWorkIDs map[string]struct{},
	scopeLogicalIDs map[string]struct{},
) bool {
	for _, parentWorkID := range snapshot.ParentWorkIDs {
		if _, ok := scopeWorkIDs[parentWorkID]; ok {
			return true
		}
	}
	for _, parentLogicalWorkID := range snapshot.ParentLogicalWorkIDs {
		if _, ok := scopeLogicalIDs[parentLogicalWorkID]; ok {
			return true
		}
	}
	return false
}
