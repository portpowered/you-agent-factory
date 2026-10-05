package service

import (
	"context"
	"encoding/json"
	"slices"
	"sort"
	"strings"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func historyFilter(req workersessions.ListWorkerSessionObservationsRequest) string {
	states := slices.Clone(req.States)
	slices.Sort(states)
	states = slices.Compact(states)
	if len(states) == 0 {
		states = nil
	}
	data, _ := json.Marshal(struct {
		History          workersessions.ObservationHistory
		Scope            workersessions.ObservationScope
		States           []workersessions.State
		FactorySessionID string
		RuntimeID        string
	}{req.History, req.Scope.Normalized(), states, strings.TrimSpace(req.FactorySessionID), req.RuntimeID})
	return string(data)
}

func (r *registry) listHistory(ctx context.Context, req workersessions.ListWorkerSessionObservationsRequest) (workersessions.ListWorkerSessionObservationsResult, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session history list rejected", "outcome", "invalid")
		return workersessions.ListWorkerSessionObservationsResult{}, err
	}
	if err := observationContextError(ctx); err != nil {
		r.logger.Info("worker session history list rejected", "outcome", "canceled")
		return workersessions.ListWorkerSessionObservationsResult{}, err
	}
	limit := req.MaxResults
	if limit == 0 {
		limit = workersessions.DefaultWorkerSessionObservationListMaxResults
	}
	filter := historyFilter(req)
	var result workersessions.ListWorkerSessionObservationsResult
	var err error
	if strings.TrimSpace(req.NextToken) != "" {
		result, err = r.historySnapshots.next(strings.TrimSpace(req.NextToken), filter, limit, r.clock.Now())
	} else {
		observations, projectionErr := r.historyObservations(ctx, req)
		if projectionErr != nil {
			r.logger.Info("worker session history list", "outcome", "projection_unavailable")
			return workersessions.ListWorkerSessionObservationsResult{}, projectionErr
		}
		if err := observationContextError(ctx); err != nil {
			return workersessions.ListWorkerSessionObservationsResult{}, err
		}
		result, err = r.historySnapshots.first(observations, filter, limit, r.clock.Now())
	}
	if canceled := observationContextError(ctx); canceled != nil {
		return workersessions.ListWorkerSessionObservationsResult{}, canceled
	}
	r.logger.Info("worker session history list", "scope", string(req.Scope.Normalized()), "result_count", len(result.Observations), "has_next", result.NextToken != "", "failed", err != nil)
	return result, err
}

type activeHistoryCandidate struct {
	id          string
	session     workersessions.Session
	metadata    *observation
	supervision *supervision
	runtime     *runtimeAttempt
}

func (r *registry) activeHistoryCandidates(req workersessions.ListWorkerSessionObservationsRequest) []activeHistoryCandidate {
	r.mu.RLock()
	defer r.mu.RUnlock()
	candidates := make([]activeHistoryCandidate, 0, len(r.observations))
	for id, metadata := range r.observations {
		session, exists := r.sessions[id]
		if !exists || metadata == nil || session.State.Terminal() || !observationStateMatches(session.State, req.States) {
			continue
		}
		if !observationScopeMatches(metadata.direct, req.Scope) || !observationFactoryScopeMatches(metadata, req.FactorySessionID) || (req.RuntimeID != "" && metadata.runtimeID != req.RuntimeID) {
			continue
		}
		candidates = append(candidates, activeHistoryCandidate{id, cloneSession(session), cloneObservation(metadata), r.supervisions[id], r.runtimeAttemptControls[id]})
	}
	return candidates
}

func (c activeHistoryCandidate) owned() bool {
	if c.runtime != nil {
		select {
		case <-c.runtime.completed:
			return false
		default:
			return true
		}
	}
	return c.supervision != nil && c.supervision.isAccepted()
}

func (r *registry) activeHistoryObservations(ctx context.Context, req workersessions.ListWorkerSessionObservationsRequest) ([]workersessions.Observation, error) {
	candidates := r.activeHistoryCandidates(req)
	observations := make([]workersessions.Observation, 0, len(candidates))
	for _, candidate := range candidates {
		if err := observationContextError(ctx); err != nil {
			return nil, err
		}
		if !candidate.owned() {
			continue
		}
		projected := baseObservation(candidate.id, candidate.session, candidate.metadata)
		applyObservationTiming(&projected, candidate.session, candidate.metadata, r.clock)
		if err := projected.Validate(); err != nil {
			return nil, err
		}
		observations = append(observations, projected)
	}
	sort.Slice(observations, func(i, j int) bool {
		left, right := observations[i], observations[j]
		if left.WorkerSessionID != right.WorkerSessionID {
			return left.WorkerSessionID < right.WorkerSessionID
		}
		if left.FactorySessionID != right.FactorySessionID {
			return left.FactorySessionID < right.FactorySessionID
		}
		return left.AttemptID < right.AttemptID
	})
	return slices.CompactFunc(observations, func(left, right workersessions.Observation) bool {
		return left.WorkerSessionID == right.WorkerSessionID && left.FactorySessionID == right.FactorySessionID && left.AttemptID == right.AttemptID
	}), nil
}
