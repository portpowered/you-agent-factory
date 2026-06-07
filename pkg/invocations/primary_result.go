package invocations

import (
	"fmt"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	invocationReturnPolicySubmittedWorkTerminal = "SUBMITTED_WORK_TERMINAL"
	invocationReturnPolicyExplicit              = "EXPLICIT"
)

// PrimaryResultErrorCode is the stable machine-readable failure code for
// invocation primary-result selection.
type PrimaryResultErrorCode string

const (
	PrimaryResultErrorCodeUnresolved PrimaryResultErrorCode = "INVOCATION_PRIMARY_RESULT_UNRESOLVED"
)

// PrimaryResultSelectionInput carries the selected-tick world state for one
// invocation request together with the authored factory return policy.
type PrimaryResultSelectionInput struct {
	RequestID        string
	InvocationReturn *interfaces.InvocationReturnConfig
	WorldState       interfaces.FactoryWorldState
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
	PrimaryResult []interfaces.WorkContentPart
}

// PrimaryResultError describes a stable primary-result selection failure.
type PrimaryResultError struct {
	Code      PrimaryResultErrorCode
	Message   string
	RequestID string
	Policy    string
}

func (e *PrimaryResultError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// ResolvePrimaryResult applies the canonical invocation primary-result rules
// against one selected-tick factory world state.
func ResolvePrimaryResult(input PrimaryResultSelectionInput) (PrimaryResultSelection, error) {
	requestID := strings.TrimSpace(input.RequestID)
	if requestID == "" {
		return PrimaryResultSelection{}, fmt.Errorf("invocation request ID is required")
	}

	request, ok := input.WorldState.WorkRequestsByID[requestID]
	if !ok || len(request.WorkItems) == 0 {
		return PrimaryResultSelection{}, unresolvedPrimaryResultError(
			requestID,
			resolvedInvocationReturnPolicy(input.InvocationReturn),
			"invocation request has no submitted work in the selected world state",
		)
	}

	switch policy := resolvedInvocationReturnPolicy(input.InvocationReturn); policy {
	case invocationReturnPolicySubmittedWorkTerminal:
		return resolveSubmittedWorkTerminalPrimaryResult(requestID, input.WorldState, request.WorkItems)
	case invocationReturnPolicyExplicit:
		return resolveExplicitPrimaryResult(requestID, input.WorldState, request.WorkItems, input.InvocationReturn)
	default:
		return PrimaryResultSelection{}, fmt.Errorf("unsupported invocation return policy %q", policy)
	}
}

func resolveSubmittedWorkTerminalPrimaryResult(
	requestID string,
	state interfaces.FactoryWorldState,
	submitted []interfaces.FactoryWorkItem,
) (PrimaryResultSelection, error) {
	for _, item := range submitted {
		logicalWorkID := logicalWorkIDForSubmittedItem(state.PayloadLineage, item.ID)
		for _, terminalWorkID := range sortedTerminalWorkIDs(state.TerminalWorkByID) {
			terminal := state.TerminalWorkByID[terminalWorkID]
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
	state interfaces.FactoryWorldState,
	submitted []interfaces.FactoryWorkItem,
	cfg *interfaces.InvocationReturnConfig,
) (PrimaryResultSelection, error) {
	scope := invocationScopeWorkIDs(state.PayloadLineage, submitted)
	matches := make([]interfaces.FactoryWorkItem, 0, len(state.TerminalWorkByID))
	for _, terminalWorkID := range sortedTerminalWorkIDs(state.TerminalWorkByID) {
		terminal := state.TerminalWorkByID[terminalWorkID]
		if _, ok := scope[terminal.WorkItem.ID]; !ok {
			continue
		}
		if !explicitPrimaryResultMatches(terminal.WorkItem, cfg) {
			continue
		}
		matches = append(matches, terminal.WorkItem)
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

func selectedPrimaryResult(requestID, policy string, item interfaces.FactoryWorkItem) PrimaryResultSelection {
	return PrimaryResultSelection{
		RequestID:     requestID,
		Policy:        policy,
		WorkID:        item.ID,
		WorkTypeName:  item.WorkTypeID,
		WorkName:      item.DisplayName,
		TerminalState: terminalStateName(item),
		PrimaryResult: interfaces.CloneWorkContentParts(item.Content),
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

func resolvedInvocationReturnPolicy(cfg *interfaces.InvocationReturnConfig) string {
	if cfg == nil || strings.TrimSpace(cfg.Policy) == "" {
		return invocationReturnPolicySubmittedWorkTerminal
	}
	return strings.TrimSpace(cfg.Policy)
}

func logicalWorkIDForSubmittedItem(lineage interfaces.WorkPayloadLineageProjection, workID string) string {
	resolution := lineage.ResolveInitialSubmittedSnapshot(workID)
	if resolution.Status == interfaces.WorkPayloadResolutionResolved && resolution.Snapshot != nil {
		return resolution.Snapshot.LogicalWorkID
	}
	return workID
}

func logicalWorkIDForSelectedItem(lineage interfaces.WorkPayloadLineageProjection, workID string) string {
	resolution := lineage.ResolveSelectedWorkSnapshot(workID)
	if resolution.Status == interfaces.WorkPayloadResolutionResolved && resolution.Snapshot != nil && resolution.Snapshot.LogicalWorkID != "" {
		return resolution.Snapshot.LogicalWorkID
	}
	return workID
}

func explicitPrimaryResultMatches(item interfaces.FactoryWorkItem, cfg *interfaces.InvocationReturnConfig) bool {
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

func terminalStateName(item interfaces.FactoryWorkItem) string {
	if state := strings.TrimSpace(item.State); state != "" {
		return state
	}
	_, suffix, ok := strings.Cut(strings.TrimSpace(item.PlaceID), ":")
	if ok {
		return suffix
	}
	return strings.TrimSpace(item.PlaceID)
}

func sortedTerminalWorkIDs(terminal map[string]interfaces.FactoryTerminalWork) []string {
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

func invocationScopeWorkIDs(
	lineage interfaces.WorkPayloadLineageProjection,
	submitted []interfaces.FactoryWorkItem,
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

func sortedLineageSnapshots(lineage interfaces.WorkPayloadLineageProjection) []interfaces.WorkPayloadSnapshot {
	if len(lineage.SnapshotsByID) == 0 {
		return nil
	}
	snapshots := make([]interfaces.WorkPayloadSnapshot, 0, len(lineage.SnapshotsByID))
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
	snapshot interfaces.WorkPayloadSnapshot,
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
