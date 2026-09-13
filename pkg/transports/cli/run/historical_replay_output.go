package run

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func normalizedHistoricalReplayFactoryProjection(
	projection factorysessions.HistoricalReplayFactoryProjection,
) factorysessions.HistoricalReplayFactoryProjection {
	if projection.Availability != "" {
		return projection
	}
	projection.Availability = factorysessions.HistoricalReplayFactoryProjectionUnavailable
	projection.Reason = factorysessions.HistoricalReplayFactoryProjectionReasonNotRecorded
	projection.State = nil
	return projection
}

func quoteHistoricalReplayValue(value string) string {
	return strconv.Quote(value)
}

func historicalReplayLifecycle(
	inspection factorysessions.HistoricalReplayInspection,
) (control string, terminal bool, final string) {
	projection := normalizedHistoricalReplayFactoryProjection(inspection.FactoryProjection)
	if projection.State != nil && projection.State.SessionBracket != nil {
		bracket := projection.State.SessionBracket
		return bracket.LifecycleControlStatus, bracket.Terminal, bracket.FinalStatus
	}
	status := inspection.Session.Status
	switch status {
	case factorysessions.LifecycleStatusSucceeded,
		factorysessions.LifecycleStatusFailed,
		factorysessions.LifecycleStatusCanceled,
		factorysessions.LifecycleStatusTimedOut,
		factorysessions.LifecycleStatusInterrupted,
		factorysessions.LifecycleStatusTerminated:
		return "", true, string(status)
	default:
		return "", false, ""
	}
}

func emitHistoricalReplayFactoryFacts(
	output io.Writer,
	inspection factorysessions.HistoricalReplayInspection,
) error {
	projection := normalizedHistoricalReplayFactoryProjection(inspection.FactoryProjection)
	if projection.State == nil {
		return nil
	}
	state := projection.State
	knownIDs, hasCanonicalWorkEvidence := historicalReplayKnownWorkIDs(*state)
	for _, item := range historicalReplayWorkItems(*state, knownIDs) {
		if _, err := fmt.Fprintf(
			output,
			"Work: id=%s type=%s state=%s trace=%s parent=%s\n",
			quoteHistoricalReplayValue(item.ID),
			quoteHistoricalReplayValue(item.WorkTypeID),
			quoteHistoricalReplayValue(item.State),
			quoteHistoricalReplayValue(item.TraceID),
			quoteHistoricalReplayValue(item.ParentID),
		); err != nil {
			return fmt.Errorf("write historical replay inspection: %w", err)
		}
		previous := stableHistoricalReplayTraceIDs(item.PreviousChainingTraceIDs)
		if _, err := fmt.Fprintf(
			output,
			"Lineage: work=%s current=%s previous=%s\n",
			quoteHistoricalReplayValue(item.ID),
			quoteHistoricalReplayValue(item.CurrentChainingTraceID),
			quoteHistoricalReplayValue(strings.Join(previous, ",")),
		); err != nil {
			return fmt.Errorf("write historical replay inspection: %w", err)
		}
	}
	for _, relation := range historicalReplayRelations(*state, knownIDs, hasCanonicalWorkEvidence) {
		if _, err := fmt.Fprintf(
			output,
			"Relation: source=%s type=%s target=%s required=%s request=%s trace=%s\n",
			quoteHistoricalReplayValue(relation.SourceWorkID),
			quoteHistoricalReplayValue(relation.Type),
			quoteHistoricalReplayValue(relation.TargetWorkID),
			quoteHistoricalReplayValue(relation.RequiredState),
			quoteHistoricalReplayValue(relation.RequestID),
			quoteHistoricalReplayValue(relation.TraceID),
		); err != nil {
			return fmt.Errorf("write historical replay inspection: %w", err)
		}
	}
	for _, failure := range historicalReplayFailures(*state, knownIDs, hasCanonicalWorkEvidence) {
		reason, message := "", ""
		if failure.FailureDetail != nil {
			reason = string(failure.FailureDetail.Reason)
			message = failure.FailureDetail.Message
		}
		if reason == "" && failure.ArtifactVerification != nil {
			reason = string(failure.ArtifactVerification.Code)
		}
		if message == "" && failure.ArtifactVerification != nil {
			message = "expected artifact verification failed"
		}
		if _, err := fmt.Fprintf(
			output,
			"Failure: work=%s dispatch=%s reason=%s message=%s\n",
			quoteHistoricalReplayValue(failure.WorkItem.ID),
			quoteHistoricalReplayValue(failure.DispatchID),
			quoteHistoricalReplayValue(reason),
			quoteHistoricalReplayValue(message),
		); err != nil {
			return fmt.Errorf("write historical replay inspection: %w", err)
		}
	}
	return nil
}

func historicalReplayKnownWorkIDs(
	state recordings.FactoryWorldState,
) (map[string]struct{}, bool) {
	known := make(map[string]struct{})
	hasEvidence := false
	for _, request := range state.WorkRequestsByID {
		for _, item := range request.WorkItems {
			if id := strings.TrimSpace(item.ID); id != "" {
				known[id] = struct{}{}
				hasEvidence = true
			}
		}
	}
	for _, snapshot := range state.PayloadLineage.SnapshotsByID {
		if snapshot.SourceKind != work.WorkPayloadSnapshotKindWorkRequest &&
			snapshot.SourceKind != work.WorkPayloadSnapshotKindDispatchOutput {
			continue
		}
		id := strings.TrimSpace(snapshot.WorkID)
		if id == "" {
			id = strings.TrimSpace(snapshot.WorkItem.ID)
		}
		if id != "" {
			known[id] = struct{}{}
			hasEvidence = true
		}
	}
	if hasEvidence {
		return known, true
	}
	for id := range state.WorkItemsByID {
		if id = strings.TrimSpace(id); id != "" {
			known[id] = struct{}{}
		}
	}
	for id := range state.ActiveWorkItemsByID {
		if id = strings.TrimSpace(id); id != "" {
			known[id] = struct{}{}
		}
	}
	for id := range state.FailedWorkItemsByID {
		if id = strings.TrimSpace(id); id != "" {
			known[id] = struct{}{}
		}
	}
	for id, terminal := range state.TerminalWorkByID {
		if id = strings.TrimSpace(id); id != "" {
			known[id] = struct{}{}
		} else if itemID := strings.TrimSpace(terminal.WorkItem.ID); itemID != "" {
			known[itemID] = struct{}{}
		}
	}
	return known, false
}

func historicalReplayWorkItems(
	state recordings.FactoryWorldState,
	known map[string]struct{},
) []work.FactoryWorkItem {
	ids := make([]string, 0, len(known))
	for id := range known {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	items := make([]work.FactoryWorkItem, 0, len(ids))
	for _, id := range ids {
		item, ok := state.WorkItemsByID[id]
		if !ok {
			item, ok = state.ActiveWorkItemsByID[id]
		}
		if !ok {
			item, ok = state.FailedWorkItemsByID[id]
		}
		if !ok {
			if terminal, found := state.TerminalWorkByID[id]; found {
				item, ok = terminal.WorkItem, true
			}
		}
		if !ok {
			if snapshotID := state.PayloadLineage.LatestSnapshotIDByWorkID[id]; snapshotID != "" {
				if snapshot, found := state.PayloadLineage.SnapshotsByID[snapshotID]; found {
					item, ok = snapshot.WorkItem, true
				}
			}
		}
		if !ok {
			for _, request := range state.WorkRequestsByID {
				for _, candidate := range request.WorkItems {
					if candidate.ID == id {
						item, ok = candidate, true
						break
					}
				}
				if ok {
					break
				}
			}
		}
		if !ok {
			continue
		}
		if item.ID == "" {
			item.ID = id
		}
		items = append(items, item)
	}
	return items
}

func historicalReplayRelations(
	state recordings.FactoryWorldState,
	known map[string]struct{},
	hasEvidence bool,
) []work.FactoryRelation {
	relations := make([]work.FactoryRelation, 0)
	keys := make([]string, 0, len(state.RelationsByWorkID))
	for key := range state.RelationsByWorkID {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		for _, relation := range state.RelationsByWorkID[key] {
			if relation.SourceWorkID == "" {
				relation.SourceWorkID = key
			}
			if hasEvidence {
				if _, ok := known[relation.SourceWorkID]; !ok {
					continue
				}
				if relation.TargetWorkID != "" {
					if _, ok := known[relation.TargetWorkID]; !ok {
						continue
					}
				}
			}
			relations = append(relations, relation)
		}
	}
	sort.SliceStable(relations, func(left, right int) bool {
		l, r := relations[left], relations[right]
		return historicalReplayRelationKey(l) < historicalReplayRelationKey(r)
	})
	return relations
}

func historicalReplayRelationKey(relation work.FactoryRelation) string {
	return strings.Join([]string{
		relation.SourceWorkID, relation.Type, relation.TargetWorkID,
		relation.RequiredState, relation.RequestID, relation.TraceID,
	}, "\x00")
}

func historicalReplayFailures(
	state recordings.FactoryWorldState,
	known map[string]struct{},
	hasEvidence bool,
) []recordings.FactoryWorldFailureDetail {
	keys := make([]string, 0, len(state.FailureDetailsByWorkID))
	for key := range state.FailureDetailsByWorkID {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	failures := make([]recordings.FactoryWorldFailureDetail, 0, len(keys))
	for _, key := range keys {
		failure := state.FailureDetailsByWorkID[key]
		id := strings.TrimSpace(key)
		if id == "" {
			id = strings.TrimSpace(failure.WorkItem.ID)
		}
		if failure.WorkItem.ID == "" {
			failure.WorkItem.ID = id
		}
		if hasEvidence {
			if _, ok := known[id]; !ok {
				continue
			}
		}
		failures = append(failures, failure)
	}
	sort.SliceStable(failures, func(left, right int) bool {
		l, r := failures[left], failures[right]
		return strings.Join([]string{l.WorkItem.ID, l.DispatchID, l.TransitionID}, "\x00") <
			strings.Join([]string{r.WorkItem.ID, r.DispatchID, r.TransitionID}, "\x00")
	})
	return failures
}

func stableHistoricalReplayTraceIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
