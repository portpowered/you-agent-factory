package service

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type historyIdentity struct {
	worker, factory, attempt string
}

func identityOfHistory(o workersessions.Observation) historyIdentity {
	return historyIdentity{o.WorkerSessionID, o.FactorySessionID, o.AttemptID}
}

func (r *registry) historyObservations(ctx context.Context, req workersessions.ListWorkerSessionObservationsRequest) ([]workersessions.Observation, error) {
	if req.History == workersessions.ObservationHistoryActive {
		return r.activeHistoryObservations(ctx, req)
	}
	// Ownership must be sampled without state filters: an active session with
	// a nonmatching state must still be excluded from the archived selection.
	ownerRequest := req
	ownerRequest.States = nil
	active, err := r.activeHistoryObservations(ctx, ownerRequest)
	if err != nil {
		return nil, err
	}
	owners := make(map[historyIdentity]struct{}, len(active))
	result := make([]workersessions.Observation, 0, len(active))
	for _, observation := range active {
		owners[identityOfHistory(observation)] = struct{}{}
		if req.History == workersessions.ObservationHistoryAll && observationStateMatches(observation.State, req.States) {
			result = append(result, observation)
		}
	}
	if r.logs == nil {
		return nil, workersessions.ErrObservationProjectionUnavailable
	}
	archived, err := r.logs.archivedHistory(ctx, req, owners)
	if err != nil {
		return nil, err
	}
	for i := range archived {
		archived[i] = r.withContinuationCapability(ctx, archived[i])
	}
	result = append(result, archived...)
	sort.Slice(result, func(i, j int) bool {
		left, right := identityOfHistory(result[i]), identityOfHistory(result[j])
		if left.worker != right.worker {
			return left.worker < right.worker
		}
		if left.factory != right.factory {
			return left.factory < right.factory
		}
		return left.attempt < right.attempt
	})
	return result, nil
}

func (s *LogReader) archivedHistory(ctx context.Context, req workersessions.ListWorkerSessionObservationsRequest, owners map[historyIdentity]struct{}) ([]workersessions.Observation, error) {
	result := make([]workersessions.Observation, 0)
	seen := make(map[historyIdentity]struct{})
	request := recordings.WorkerCapturedCatalogRequest{Limit: 1000}
	for {
		if err := observationContextError(ctx); err != nil {
			return nil, err
		}
		page, err := s.reader.ListWorkerSessionCaptures(ctx, request)
		if err != nil {
			return nil, workersessions.ErrObservationProjectionUnavailable
		}
		for _, item := range page.Items {
			if !catalogHistoryMatches(item.Catalog, req) {
				continue
			}
			observation, err := capturedHistoryIdentity(item, owners)
			if err != nil {
				return nil, err
			}
			if observation == nil || !observationStateMatches(observation.State, req.States) {
				continue
			}
			s.applyArchivedCatalogTerminalCause(ctx, item, observation)
			key := identityOfHistory(*observation)
			if _, duplicate := seen[key]; duplicate {
				return nil, workersessions.ErrObservationProjectionUnavailable
			}
			seen[key] = struct{}{}
			result = append(result, observation.Clone())
		}
		if page.NextToken == "" {
			return result, nil
		}
		request.NextToken = page.NextToken
	}
}

func catalogHistoryMatches(entry recordings.WorkerSessionCatalogEntry, req workersessions.ListWorkerSessionObservationsRequest) bool {
	// Captures do not retain RuntimeID; never infer it from a current host.
	return req.RuntimeID == "" && observationScopeMatches(entry.Origin == "direct", req.Scope) &&
		(req.FactorySessionID == "" || entry.FactorySessionID == strings.TrimSpace(req.FactorySessionID))
}

func capturedHistoryIdentity(item recordings.WorkerCapturedCatalogItem, owners map[historyIdentity]struct{}) (*workersessions.Observation, error) {
	opening, err := historyOpening(item)
	if err != nil {
		return nil, err
	}
	key := historyIdentity{opening.WorkerSessionID, opening.FactorySessionID, opening.AttemptID}
	if _, owned := owners[key]; owned {
		return nil, nil
	}
	terminal := item.Terminal
	if terminal == nil {
		if !item.OwnerLost {
			return nil, workersessions.ErrObservationProjectionUnavailable
		}
		terminal = &recordings.WorkerRecordingTerminal{Status: string(workersessions.StateFailed)}
	}
	observation, err := capturedTerminalIdentity(recordings.WorkerCapturedActivityPage{
		Opening: item.Opening, Terminal: terminal, Health: item.Health, HealthReason: item.HealthReason,
	}, opening.WorkerSessionID)
	if err != nil {
		return nil, err
	}
	if item.Terminal == nil {
		observation.ConfirmationState = workersessions.ConfirmationStateUnconfirmed
		observation.RecordingHealth = recordings.WorkerRecordingStatusIncomplete
		observation.Failure = &workersessions.FailureCause{Kind: workersessions.FailureCauseProcessGone, Detail: "the recorded worker host is no longer the current owner"}
	} else if item.Terminal.Position < 1 || uint64(item.Terminal.Position) > item.Catalog.CommittedPosition {
		observation.ConfirmationState = workersessions.ConfirmationStateUnconfirmed
	}
	for _, record := range item.MetadataRecords {
		var draft workers.Draft
		if json.Unmarshal(record.Payload, &draft) == nil {
			applyCapturedSessionFacts(&observation, draft)
			applyCapturedUsageFacts(&observation, draft)
		}
	}
	if err := applyCapturedTerminalFacts(&observation, item); err != nil {
		return nil, err
	}
	applyCapturedTiming(&observation, item.Terminal, item.Catalog.CommittedPosition, item.CapturedAt)
	if item.SuccessorWorkerSessionID != "" {
		if observation.SuccessorWorkerSessionID != "" && observation.SuccessorWorkerSessionID != item.SuccessorWorkerSessionID {
			return nil, workersessions.ErrObservationProjectionUnavailable
		}
		observation.SuccessorWorkerSessionID = item.SuccessorWorkerSessionID
	}
	return &observation, observation.Validate()
}

func applyCapturedSessionFacts(observation *workersessions.Observation, draft workers.Draft) {
	if draft.Kind != workers.KindSession || draft.Phase != workers.PhaseUpdated {
		return
	}
	var payload workers.SessionPayload
	if json.Unmarshal(draft.Payload, &payload) != nil || payload.WorkerSessionID != observation.WorkerSessionID ||
		(draft.DispatchID != "" && draft.DispatchID != observation.AttemptID) ||
		(payload.FactorySessionID != "" && payload.FactorySessionID != observation.FactorySessionID) ||
		(payload.AttemptID != "" && payload.AttemptID != observation.AttemptID) {
		return
	}
	if draft.Provenance.Provider != "" {
		observation.Provider = draft.Provenance.Provider
	}
	if payload.Model != "" {
		observation.Model = &payload.Model
	}
	if payload.ReasoningEffort != "" {
		observation.ReasoningEffort = &payload.ReasoningEffort
	}
	applyCapturedLineage(observation, payload)
}

func applyCapturedLineage(observation *workersessions.Observation, payload workers.SessionPayload) {
	if payload.Lineage != nil {
		if payload.Lineage.PredecessorWorkerSessionID != "" {
			observation.PredecessorWorkerSessionID = payload.Lineage.PredecessorWorkerSessionID
		}
		if payload.Lineage.SuccessorWorkerSessionID != "" {
			observation.SuccessorWorkerSessionID = payload.Lineage.SuccessorWorkerSessionID
		}
	}
}

func historyOpening(item recordings.WorkerCapturedCatalogItem) (workers.SessionPayload, error) {
	var draft workers.Draft
	var opening workers.SessionPayload
	if json.Unmarshal(item.Opening.Payload, &draft) != nil || draft.Kind != workers.KindSession || draft.Phase != workers.PhaseStarted ||
		json.Unmarshal(draft.Payload, &opening) != nil || opening.WorkerSessionID != item.Catalog.WorkerSessionID || opening.FactorySessionID != item.Catalog.FactorySessionID {
		return workers.SessionPayload{}, workersessions.ErrObservationProjectionUnavailable
	}
	return opening, nil
}
