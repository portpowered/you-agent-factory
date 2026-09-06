package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/services/events"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func (r *registry) ListWorkerSessionObservations(
	ctx context.Context,
	req workersessions.ListWorkerSessionObservationsRequest,
) (workersessions.ListWorkerSessionObservationsResult, error) {
	listStartedAt := r.clock.Now()
	query, err := r.parseObservationListQuery(ctx, req)
	if err != nil {
		return workersessions.ListWorkerSessionObservationsResult{}, err
	}
	idCollectionStartedAt := r.clock.Now()
	ids, err := r.observationListIDsWithDurable(ctx, query.cursor, query.scope, req.States)
	if err != nil {
		return workersessions.ListWorkerSessionObservationsResult{}, err
	}
	idCollectionDuration := r.clock.Now().Sub(idCollectionStartedAt)
	pageIDs := observationListPage(ids, query.limit)
	projectionStartedAt := r.clock.Now()
	observations, err := r.projectObservationList(ctx, pageIDs)
	nextToken := observationListNextToken(ids, pageIDs)
	result := workersessions.ListWorkerSessionObservationsResult{
		Observations: observations,
		MaxResults:   query.limit,
		NextToken:    nextToken,
	}
	if err != nil {
		if errors.Is(err, workersessions.ErrObservationProjectionUnavailable) {
			r.logger.Info("worker session top-level observation list degraded", "scope", string(query.scope), "state_count", len(req.States), "result_count", len(observations), "optional_projection", "unavailable")
		}
		return result, err
	}
	r.logger.Debug(
		"worker session top-level observation list phases",
		"scope", string(query.scope),
		"candidate_count", len(ids),
		"result_count", len(observations),
		"id_collection_duration_ms", idCollectionDuration.Milliseconds(),
		"projection_duration_ms", r.clock.Now().Sub(projectionStartedAt).Milliseconds(),
		"total_duration_ms", r.clock.Now().Sub(listStartedAt).Milliseconds(),
	)
	r.logger.Info("worker session top-level observation list", "scope", string(query.scope), "state_count", len(req.States), "result_count", len(observations), "has_next", nextToken != "")
	return workersessions.ListWorkerSessionObservationsResult{
		Observations: observations,
		MaxResults:   query.limit,
		NextToken:    nextToken,
	}, nil
}

type observationListQuery struct {
	limit  int
	cursor string
	scope  workersessions.ObservationScope
}

func (r *registry) parseObservationListQuery(ctx context.Context, req workersessions.ListWorkerSessionObservationsRequest) (observationListQuery, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session top-level observation list rejected", "outcome", "invalid")
		return observationListQuery{}, err
	}
	if err := observationContextError(ctx); err != nil {
		return observationListQuery{}, err
	}
	limit := req.MaxResults
	if limit == 0 {
		limit = workersessions.DefaultWorkerSessionObservationListMaxResults
	}
	cursor, err := decodeObservationListCursor(req.NextToken)
	if err != nil {
		return observationListQuery{}, err
	}
	return observationListQuery{limit: limit, cursor: cursor, scope: req.Scope.Normalized()}, nil
}

func decodeObservationListCursor(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", workersessions.ErrInvalidObservationPagination
	}
	return string(decoded), nil
}

func (r *registry) observationListIDs(cursor string, scope workersessions.ObservationScope, states []workersessions.State) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.observations))
	for id, metadata := range r.observations {
		if metadata == nil || id <= cursor || !observationScopeMatches(metadata.direct, scope) {
			continue
		}
		session, exists := r.sessions[id]
		if !exists || !observationStateMatches(session.State, states) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (r *registry) observationListIDsWithDurable(
	ctx context.Context,
	cursor string,
	scope workersessions.ObservationScope,
	states []workersessions.State,
) ([]string, error) {
	ids := make(map[string]struct{})
	for _, id := range r.observationListIDs(cursor, scope, states) {
		ids[id] = struct{}{}
	}
	durable, err := r.durableWorkerProjections(ctx, recordings.WorkerRecordingListRequest{})
	if err != nil {
		return nil, err
	}
	for _, projection := range durable {
		if projection.WorkerSessionID <= cursor || !observationScopeMatches(projection.FactorySessionID == "", scope) {
			continue
		}
		state, stateErr := durableWorkerState(projection)
		if stateErr != nil || !observationStateMatches(state, states) {
			continue
		}
		ids[projection.WorkerSessionID] = struct{}{}
	}
	result := make([]string, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	sort.Strings(result)
	return result, nil
}

func observationListPage(ids []string, limit int) []string {
	if len(ids) <= limit {
		return ids
	}
	return ids[:limit]
}

func (r *registry) projectObservationList(ctx context.Context, ids []string) ([]workersessions.Observation, error) {
	observations := make([]workersessions.Observation, 0, len(ids))
	var optionalProjectionErr error
	for _, id := range ids {
		projected, err := r.projectObservation(ctx, id)
		if errors.Is(err, workersessions.ErrObservationProjectionUnavailable) {
			observations = append(observations, projected)
			optionalProjectionErr = workersessions.ErrObservationProjectionUnavailable
			continue
		}
		if err != nil {
			return nil, err
		}
		observations = append(observations, projected)
	}
	return observations, optionalProjectionErr
}

func observationListNextToken(allIDs, pageIDs []string) string {
	if len(allIDs) <= len(pageIDs) || len(pageIDs) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(pageIDs[len(pageIDs)-1]))
}

func observationScopeMatches(direct bool, scope workersessions.ObservationScope) bool {
	switch scope.Normalized() {
	case workersessions.ObservationScopeDirect:
		return direct
	case workersessions.ObservationScopeFactory:
		return !direct
	case workersessions.ObservationScopeAll:
		return true
	default:
		return false
	}
}

func observationStateMatches(state workersessions.State, states []workersessions.State) bool {
	if len(states) == 0 {
		return true
	}
	return slices.Contains(states, state)
}

// ReadTranscriptByWorkerSessionID resolves the registry-owned association
// before projecting a direct or Factory Worker Session transcript. The caller
// cannot substitute a provider tuple or cause a provider-session discovery.
func (r *registry) ReadTranscriptByWorkerSessionID(
	ctx context.Context,
	req workersessions.ReadTranscriptByWorkerSessionIDRequest,
) (workersessions.ReadTranscriptResult, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session identity transcript read rejected", "outcome", "invalid")
		return workersessions.ReadTranscriptResult{}, err
	}
	if err := observationContextError(ctx); err != nil {
		return workersessions.ReadTranscriptResult{}, err
	}
	// A retained Worker recording is the provider-neutral identity boundary.
	// Prefer it even when this process still has a provider association so a
	// restarted or provider-degraded read cannot change the Worker-ID contract.
	if projection, found, err := r.durableWorkerProjection(ctx, req.WorkerSessionID); err != nil {
		return workersessions.ReadTranscriptResult{}, err
	} else if found {
		result, err := durableTranscript(projection)
		if err != nil {
			return workersessions.ReadTranscriptResult{}, err
		}
		return r.attachLiveProviderTranscript(req.WorkerSessionID, result), nil
	}
	session, metadata, ok := r.loadObservationState(req.WorkerSessionID)
	if ok && session.ProviderSessionAssociation != nil {
		providerSession := session.ProviderSessionAssociation.Reference
		return r.projectTranscript(ctx, session, metadata, providerSession)
	}
	if !ok {
		r.logger.Info("worker session identity transcript read", "workerSessionID", req.WorkerSessionID, "outcome", "not_found")
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationSessionNotFound
	}
	providerSession := providers.SessionRef{}
	if session.ProviderSessionAssociation != nil {
		providerSession = session.ProviderSessionAssociation.Reference
	}
	return r.projectTranscript(ctx, session, metadata, providerSession)
}

func (r *registry) projectTranscript(
	ctx context.Context,
	session workersessions.Session,
	metadata *observation,
	providerSession providers.SessionRef,
) (workersessions.ReadTranscriptResult, error) {
	if !session.State.Terminal() {
		r.logger.Info("worker session transcript read", "workerSessionID", session.ID, "outcome", "active")
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptActive
	}
	if session.ProviderSessionAssociation == nil {
		r.logger.Info("worker session transcript read", "workerSessionID", session.ID, "outcome", "unavailable")
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptUnavailable
	}
	if r.providerSessions == nil {
		r.logger.Info("worker session transcript read", "workerSessionID", session.ID, "outcome", "projection_unavailable")
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptProjectionUnavailable
	}

	projected, projectErr := r.providerSessions.Project(providersessions.ProjectRequest{
		Session: providerSession.Clone(),
		Context: ctx,
	})
	if projectErr != nil {
		if errors.Is(projectErr, context.Canceled) || errors.Is(projectErr, providersessions.ErrOperationCanceled) {
			return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationCanceled
		}
		if transcriptSourceUnavailable(projectErr) {
			return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptUnavailable
		}
		return workersessions.ReadTranscriptResult{}, fmt.Errorf("%w: %v", workersessions.ErrObservationTranscriptProjectionUnavailable, projectErr)
	}

	result := workersessions.ReadTranscriptResult{
		WorkerSessionID: session.ID,
		ProviderSession: providerSession.Clone(),
		WorkIDs:         append([]string(nil), metadata.workIDs...),
		AttemptID:       metadata.attemptID,
		State:           session.State,
		Entries:         transcriptEntries(projected.Detail.Transcript),
	}
	if session.ProviderSessionAssociation != nil {
		result.TurnID = session.ProviderSessionAssociation.TurnID
		result.AttemptID = session.ProviderSessionAssociation.AttemptID
	}
	if err := result.Validate(); err != nil {
		return workersessions.ReadTranscriptResult{}, fmt.Errorf("validate Worker Session transcript: %w", err)
	}
	r.logger.Info("worker session transcript read", "workerSessionID", result.WorkerSessionID, "outcome", "success", "result_count", len(result.Entries))
	return result, nil
}

// ReadTranscript returns the final normalized transcript for one Worker
// Session. The canonical Worker Session identity is preferred; the exact
// Provider Session reference remains a compatibility lookup. The lifecycle
// check happens before the provider projection so an active session has a
// stable active outcome even when its provider source is not yet readable.
func (r *registry) ReadTranscript(ctx context.Context, req workersessions.ReadTranscriptRequest) (workersessions.ReadTranscriptResult, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session transcript read rejected", "outcome", "invalid")
		return workersessions.ReadTranscriptResult{}, err
	}
	req.WorkerSessionID = strings.TrimSpace(req.WorkerSessionID)
	if err := observationContextError(ctx); err != nil {
		return workersessions.ReadTranscriptResult{}, err
	}
	if req.WorkerSessionID != "" {
		// Worker-ID transcript reads are backed by the durable source-native
		// history whenever it is present. Provider-reference reads retain the
		// existing Provider Sessions projection below.
		if projection, found, durableErr := r.durableWorkerProjection(ctx, req.WorkerSessionID); durableErr != nil {
			return workersessions.ReadTranscriptResult{}, durableErr
		} else if found {
			result, err := durableTranscript(projection)
			if err != nil {
				return workersessions.ReadTranscriptResult{}, err
			}
			return r.attachLiveProviderTranscript(req.WorkerSessionID, result), nil
		}
	}

	session, metadata, err := r.transcriptSession(req)
	if err != nil {
		r.logger.Info("worker session transcript read", "outcome", "not_found")
		return workersessions.ReadTranscriptResult{}, err
	}
	providerSession := req.ProviderSession
	if req.WorkerSessionID != "" && session.ProviderSessionAssociation != nil {
		providerSession = session.ProviderSessionAssociation.Reference
	}
	return r.projectTranscript(ctx, session, metadata, providerSession)
}

func (r *registry) transcriptSession(req workersessions.ReadTranscriptRequest) (workersessions.Session, *observation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, 1)
	for id, session := range r.sessions {
		matches := id == req.WorkerSessionID
		if req.WorkerSessionID == "" {
			matches = session.ProviderSessionAssociation != nil && session.ProviderSessionAssociation.Reference == req.ProviderSession
		}
		if matches {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return workersessions.Session{}, nil, workersessions.ErrObservationSessionNotFound
	}
	sortStrings(ids)
	session := cloneSession(r.sessions[ids[0]])
	metadata := cloneObservation(r.observations[ids[0]])
	if metadata == nil {
		return workersessions.Session{}, nil, workersessions.ErrObservationSessionNotFound
	}
	return session, metadata, nil
}

func projectObservationEvent(record events.Record, workerSessionIDArgs ...string) workersessions.ObservationEvent {
	workerSessionID := observationWorkerSessionIDFromTopic(record.ID.Topic)
	if len(workerSessionIDArgs) > 0 && strings.TrimSpace(workerSessionIDArgs[0]) != "" {
		workerSessionID = strings.TrimSpace(workerSessionIDArgs[0])
	}
	return workersessions.ObservationEvent{
		Position: uint64(record.ID.Position),
		Cursor: workersessions.ObservationCursor{
			WorkerSessionID: workerSessionID,
			Position:        uint64(record.ID.Position),
		},
		SourceType:     string(record.SourceType),
		SourceID:       string(record.SourceID),
		SourceSequence: uint64(record.SourceSequence),
		SourceEventID:  string(record.SourceEventID),
		SchemaID:       string(record.SchemaID),
		Payload:        append([]byte(nil), record.Payload...),
	}
}

func observationWorkerSessionIDFromTopic(topic events.Topic) string {
	value := strings.TrimSpace(string(topic))
	value = strings.TrimPrefix(value, "worker-session/")
	value = strings.TrimSuffix(value, "/events")
	return value
}

func (r *registry) ListObservations(ctx context.Context, req workersessions.ListObservationsRequest) (workersessions.ListObservationsResult, error) {
	listStartedAt := r.clock.Now()
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session observation list rejected", "outcome", "invalid")
		return workersessions.ListObservationsResult{}, err
	}
	if err := observationContextError(ctx); err != nil {
		return workersessions.ListObservationsResult{}, err
	}

	r.mu.RLock()
	ids := make([]observationOrder, 0)
	for id, current := range r.observations {
		if containsString(current.workIDs, req.WorkID) {
			ids = append(ids, observationOrder{
				id:        id,
				startedAt: current.startedAt,
				attemptID: current.attemptID,
			})
		}
	}
	r.mu.RUnlock()
	durable, durableErr := r.durableWorkerProjections(ctx, recordings.WorkerRecordingListRequest{WorkID: req.WorkID})
	if durableErr != nil {
		return workersessions.ListObservationsResult{}, durableErr
	}
	seen := make(map[string]struct{}, len(ids)+len(durable))
	for _, item := range ids {
		seen[item.id] = struct{}{}
	}
	for _, projection := range durable {
		if _, exists := seen[projection.WorkerSessionID]; exists {
			continue
		}
		ids = append(ids, observationOrder{
			id:        projection.WorkerSessionID,
			startedAt: durableObservationStartedAt(projection),
			attemptID: projection.AttemptID,
		})
		seen[projection.WorkerSessionID] = struct{}{}
	}
	idCollectionDuration := r.clock.Now().Sub(listStartedAt)
	if len(ids) == 0 {
		r.logger.Info("worker session observation list", "workID", req.WorkID, "outcome", "not_found")
		return workersessions.ListObservationsResult{}, workersessions.ErrObservationWorkNotFound
	}
	sortObservationOrder(ids)

	projectionStartedAt := r.clock.Now()
	observations := make([]workersessions.Observation, 0, len(ids))
	for _, item := range ids {
		projected, err := r.projectObservation(ctx, item.id)
		if err != nil {
			return workersessions.ListObservationsResult{}, err
		}
		observations = append(observations, projected)
	}
	sortObservationAttempts(observations)
	r.logger.Debug(
		"worker session observation list phases",
		"candidate_count", len(ids),
		"result_count", len(observations),
		"id_collection_duration_ms", idCollectionDuration.Milliseconds(),
		"projection_duration_ms", r.clock.Now().Sub(projectionStartedAt).Milliseconds(),
		"total_duration_ms", r.clock.Now().Sub(listStartedAt).Milliseconds(),
	)
	r.logger.Info("worker session observation list", "workID", req.WorkID, "outcome", "success", "result_count", len(observations))
	return workersessions.ListObservationsResult{Observations: observations}, nil
}

func (r *registry) GetObservation(ctx context.Context, req workersessions.GetObservationRequest) (workersessions.Observation, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session observation get rejected", "outcome", "invalid")
		return workersessions.Observation{}, err
	}
	if err := observationContextError(ctx); err != nil {
		return workersessions.Observation{}, err
	}

	r.mu.RLock()
	ids := make([]string, 0, 1)
	for id, session := range r.sessions {
		if session.ProviderSessionAssociation != nil &&
			session.ProviderSessionAssociation.Reference == req.ProviderSession {
			ids = append(ids, id)
		}
	}
	r.mu.RUnlock()
	if len(ids) == 0 {
		r.logger.Info("worker session observation get", "outcome", "not_found")
		return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
	}
	// An exact Provider Session identity must be unique. If corrupted or
	// legacy state ever contains two matches, deterministic identity order
	// still makes the result stable without exposing both as one observation.
	sortStrings(ids)
	projected, err := r.projectObservation(ctx, ids[0])
	if err != nil {
		return workersessions.Observation{}, err
	}
	r.logger.Info("worker session observation get", "workerSessionID", projected.WorkerSessionID, "outcome", "success")
	return projected, nil
}

func (r *registry) GetObservationByWorkerSessionID(ctx context.Context, req workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session observation get by Worker Session rejected", "outcome", "invalid")
		return workersessions.Observation{}, err
	}
	req.WorkerSessionID = strings.TrimSpace(req.WorkerSessionID)
	if err := observationContextError(ctx); err != nil {
		return workersessions.Observation{}, err
	}
	// Worker-ID lookup is the provider-neutral history boundary. It must not
	// require transcript enrichment from Provider Sessions: a worker can emit
	// canonical lifecycle/output records without a readable provider transcript.
	projected, err := r.projectWorkerSessionIdentity(ctx, req.WorkerSessionID)
	if err != nil {
		r.logger.Info("worker session observation get by Worker Session", "workerSessionID", req.WorkerSessionID, "outcome", "not_found")
		return workersessions.Observation{}, err
	}
	r.logger.Info("worker session observation get by Worker Session", "workerSessionID", projected.WorkerSessionID, "outcome", "success")
	return projected, nil
}

func (r *registry) projectObservation(ctx context.Context, id string) (workersessions.Observation, error) {
	if err := observationContextError(ctx); err != nil {
		return workersessions.Observation{}, err
	}
	projected, err := r.projectWorkerSessionIdentity(ctx, id)
	if err != nil {
		return workersessions.Observation{}, err
	}

	if !projected.ProviderSessionAvailable {
		return projected, nil
	}
	return r.enrichWithProviderSessionsProjection(ctx, projected)
}

// projectWorkerSessionIdentity projects the registry-owned Worker Session
// identity and lifecycle timing without consulting Provider Sessions. The
// canonical Worker-ID history stream uses this boundary so missing provider
// transcript storage cannot hide retained Worker Session records.
func (r *registry) projectWorkerSessionIdentity(ctx context.Context, id string) (workersessions.Observation, error) {
	if err := observationContextError(ctx); err != nil {
		return workersessions.Observation{}, err
	}
	// Durable Worker history is provider-neutral and authoritative for the
	// canonical Worker-ID read. This also prevents a provider association in
	// an in-memory registry from leaking into a restarted-history response.
	if projection, found, err := r.durableWorkerProjection(ctx, id); err != nil {
		return workersessions.Observation{}, err
	} else if found {
		projected, err := durableObservation(projection)
		if err != nil {
			return workersessions.Observation{}, err
		}
		return r.attachLiveProviderAssociation(id, projected), nil
	}

	session, metadata, ok := r.loadObservationState(id)
	if !ok {
		projection, found, err := r.durableWorkerProjection(ctx, id)
		if err != nil {
			return workersessions.Observation{}, err
		}
		if !found {
			return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
		}
		projected, err := durableObservation(projection)
		if err != nil {
			return workersessions.Observation{}, err
		}
		return r.attachLiveProviderAssociation(id, projected), nil
	}

	projected := baseObservation(id, session, metadata)
	applyObservationTiming(&projected, session, metadata, r.clock)
	projected.Failure = observedTerminalCause(session)
	return projected, nil
}

// attachLiveProviderAssociation preserves a provider reference only when the
// current process still owns the Worker Session association. The durable
// recording remains the source of lifecycle and output facts; after restart,
// loadObservationState returns false and the same Worker-ID read stays
// provider-neutral.
func (r *registry) attachLiveProviderAssociation(
	id string,
	projected workersessions.Observation,
) workersessions.Observation {
	session, _, ok := r.loadObservationState(id)
	if !ok || session.ProviderSessionAssociation == nil {
		return projected
	}
	association := session.ProviderSessionAssociation
	projected.ProviderSession = association.Reference.Clone()
	projected.ProviderSessionAvailable = true
	if strings.TrimSpace(association.TurnID) != "" {
		projected.TurnID = association.TurnID
	}
	if strings.TrimSpace(association.AttemptID) != "" {
		projected.AttemptID = association.AttemptID
	}
	return projected
}

func (r *registry) attachLiveProviderTranscript(
	id string,
	result workersessions.ReadTranscriptResult,
) workersessions.ReadTranscriptResult {
	session, _, ok := r.loadObservationState(id)
	if !ok || session.ProviderSessionAssociation == nil {
		return result
	}
	association := session.ProviderSessionAssociation
	result.ProviderSession = association.Reference.Clone()
	if strings.TrimSpace(association.TurnID) != "" {
		result.TurnID = association.TurnID
	}
	if strings.TrimSpace(association.AttemptID) != "" {
		result.AttemptID = association.AttemptID
	}
	return result
}

// controlTerminalCause maps the two absorbing control states to the operator
// control outcome that produced them, with the fixed safe detail naming it.
// These reasons are derived from state rather than classified from a Workers
// result, so they carry their own detail instead of a genericFailureDetail
// fallback.
var controlTerminalCause = map[workersessions.State]workersessions.FailureCause{
	workersessions.StateCanceled: {
		Kind:   workersessions.FailureCauseOperatorCanceled,
		Detail: "an operator cancel control ended the Worker Session",
	},
	workersessions.StateTerminated: {
		Kind:   workersessions.FailureCauseOperatorTerminated,
		Detail: "an operator terminate control ended the Worker Session",
	},
}

// observedTerminalCause names why a session ended. A committed FailureCause is
// authoritative. A session ended by an operator control never has one:
// commitControlTerminal deliberately keeps Result nil so a control invents no
// terminal result, and the boundary cancel path commits an empty one. Deriving
// the control outcome from the absorbing state preserves that invariant while
// still naming the reason, so the inspection surface no longer reports
// "unavailable" for a cancellation — which is indistinguishable from a failure
// whose cause was never recorded.
func observedTerminalCause(session workersessions.Session) *workersessions.FailureCause {
	if session.Result != nil && session.Result.Cause != nil {
		cause := *session.Result.Cause
		return &cause
	}
	cause, ok := controlTerminalCause[session.State]
	if !ok {
		return nil
	}
	cause.Detail = boundedFailureDetail(cause.Kind, cause.Detail)
	return &cause
}

// loadObservationState returns detached snapshots of the registered session
// and observation metadata for id. ok is false when either is missing.
func (r *registry) loadObservationState(id string) (workersessions.Session, *observation, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	session, exists := r.sessions[id]
	metadata := r.observations[id]
	if exists {
		session = cloneSession(session)
	}
	if metadata != nil {
		metadata = cloneObservation(metadata)
	}
	if !exists || metadata == nil {
		return workersessions.Session{}, nil, false
	}
	return session, metadata, true
}

// baseObservation projects the registry-owned identity, correlation, and
// lifecycle facts that never require the Provider Sessions root.
func baseObservation(id string, session workersessions.Session, metadata *observation) workersessions.Observation {
	projected := workersessions.Observation{
		WorkerSessionID:            id,
		PredecessorWorkerSessionID: session.PredecessorWorkerSessionID,
		SuccessorWorkerSessionID:   session.SuccessorWorkerSessionID,
		Model:                      cloneOptionalExecutionFact(session.Model),
		ReasoningEffort:            cloneOptionalExecutionFact(session.ReasoningEffort),
		TokenUsage:                 cloneObservationTokenUsage(metadata.tokenUsage),
		Direct:                     metadata.direct,
		FactorySessionID:           metadata.factorySessionID,
		WorkIDs:                    append([]string(nil), metadata.workIDs...),
		TurnID:                     metadata.turnID,
		AttemptID:                  metadata.attemptID,
		State:                      session.State,
		ConfirmationState:          workersessions.ConfirmationStateUnconfirmed,
		DurationBasis:              workersessions.DurationBasisUnavailable,
		Transcript:                 workersessions.TranscriptAvailabilityUnavailable,
	}
	if strings.TrimSpace(metadata.usageModel) != "" {
		model := strings.TrimSpace(metadata.usageModel)
		projected.Model = &model
	}
	if session.ProviderSessionAssociation != nil {
		projected.ProviderSession = session.ProviderSessionAssociation.Reference.Clone()
		projected.ProviderSessionAvailable = true
		projected.TurnID = session.ProviderSessionAssociation.TurnID
		projected.AttemptID = session.ProviderSessionAssociation.AttemptID
	}
	return projected
}

// applyObservationTiming fills projected's start/end/duration fields from
// metadata, using clock for an active (non-terminal) session's elapsed time.
func applyObservationTiming(projected *workersessions.Observation, session workersessions.Session, metadata *observation, clock platformclock.Source) {
	if metadata.startedAt.IsZero() {
		return
	}
	started := metadata.startedAt
	projected.StartedAt = &started
	switch {
	case metadata.endedAt != nil:
		ended := *metadata.endedAt
		projected.EndedAt = &ended
		projected.Duration = nonNegativeDuration(ended.Sub(started))
		projected.DurationBasis = workersessions.DurationBasisRecordedTimestamps
	case !session.Terminal():
		projected.Duration = nonNegativeDuration(clock.Now().Sub(started))
		projected.DurationBasis = workersessions.DurationBasisActiveClock
	}
}

func nonNegativeDuration(duration time.Duration) *time.Duration {
	if duration < 0 {
		duration = 0
	}
	return &duration
}

type usageProjectionPayload struct {
	InputTokens           *int64 `json:"inputTokens"`
	CachedInputTokens     *int64 `json:"cachedInputTokens"`
	OutputTokens          *int64 `json:"outputTokens"`
	ReasoningOutputTokens *int64 `json:"reasoningOutputTokens"`
	TotalTokens           *int64 `json:"totalTokens"`
	Model                 string `json:"model"`
}

func (r *registry) updateUsageProjection(sessionID string, draft workers.Draft) {
	usage, model, ok := usageProjectionFromDraft(draft)
	if !ok {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	metadata := r.observations[strings.TrimSpace(sessionID)]
	if metadata == nil {
		return
	}
	metadata.tokenUsage = usage
	if strings.TrimSpace(model) != "" {
		metadata.usageModel = strings.TrimSpace(model)
	}
}

func usageProjectionFromDraft(draft workers.Draft) (*workersessions.TokenUsage, string, bool) {
	if draft.Kind != workers.KindUsage || draft.Phase != workers.PhaseUpdated {
		return nil, "", false
	}
	var payload usageProjectionPayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		return nil, "", false
	}
	if payload.InputTokens == nil && payload.CachedInputTokens == nil &&
		payload.OutputTokens == nil && payload.ReasoningOutputTokens == nil &&
		payload.TotalTokens == nil {
		return nil, "", false
	}
	return &workersessions.TokenUsage{
		InputTokens:           int64PointerToInt(payload.InputTokens),
		CachedInputTokens:     int64PointerToInt(payload.CachedInputTokens),
		OutputTokens:          int64PointerToInt(payload.OutputTokens),
		ReasoningOutputTokens: int64PointerToInt(payload.ReasoningOutputTokens),
		TotalTokens:           int64PointerToInt(payload.TotalTokens),
	}, payload.Model, true
}

func int64PointerToInt(value *int64) *int {
	if value == nil {
		return nil
	}
	converted := int(*value)
	return &converted
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func observationTokenUsage(source *providersessions.TokenUsage) *workersessions.TokenUsage {
	if source == nil {
		return nil
	}
	return &workersessions.TokenUsage{
		CacheWriteTokens:      cloneInt(source.CacheWriteTokens),
		CachedInputTokens:     cloneInt(source.CachedInputTokens),
		InputTokens:           cloneInt(source.InputTokens),
		OutputTokens:          cloneInt(source.OutputTokens),
		ReasoningOutputTokens: cloneInt(source.ReasoningOutputTokens),
		TotalTokens:           cloneInt(source.TotalTokens),
	}
}

// observationTurnUsage derives per-turn input context from cumulative usage
// counters already retained by Provider Sessions. A decreasing counter makes
// the sequence unsupported because its baseline cannot be interpreted as a
// cumulative total, so the optional projection is omitted.
func observationTurnUsage(cumulativeInputTokens []int) *workersessions.TurnUsage {
	if len(cumulativeInputTokens) == 0 {
		return nil
	}

	previous := 0
	final := 0
	peak := 0
	for _, cumulative := range cumulativeInputTokens {
		if cumulative < previous {
			return nil
		}
		perTurn := cumulative - previous
		if perTurn > peak {
			peak = perTurn
		}
		final = perTurn
		previous = cumulative
	}

	return &workersessions.TurnUsage{
		TurnCount:          len(cumulativeInputTokens),
		FinalContextTokens: final,
		PeakContextTokens:  peak,
	}
}

func observationParseDiagnostics(source providersessions.ParseSummary) workersessions.ParseDiagnostics {
	result := workersessions.ParseDiagnostics{
		EventCount:         source.EventCount,
		MalformedLineCount: source.MalformedLineCount,
		UnknownEventCount:  source.UnknownEventCount,
		Errors:             make([]workersessions.ParseDiagnostic, 0, len(source.ParseErrors)),
	}
	for _, item := range source.ParseErrors {
		result.Errors = append(result.Errors, workersessions.ParseDiagnostic{
			Code:       "provider_session_parse_error",
			LineNumber: item.LineNumber,
			Message:    safeDiagnosticMessage(item.Message),
		})
	}
	return result
}

func safeDiagnosticMessage(message string) string {
	message = strings.Join(strings.Fields(message), " ")
	if message == "" || strings.ContainsAny(message, `/\`) {
		return "provider session parse error"
	}
	lower := strings.ToLower(message)
	for _, sensitive := range []string{"password", "authorization", "bearer ", "secret", "prompt"} {
		if strings.Contains(lower, sensitive) {
			return "provider session parse error"
		}
	}
	if len(message) > 256 {
		message = message[:256]
	}
	return message
}

// enrichWithProviderSessionsProjection adds transcript availability, token
// usage, and parse diagnostics from the Provider Sessions root. It is only
// called when projected already carries an available Provider Session
// reference.
func (r *registry) enrichWithProviderSessionsProjection(ctx context.Context, projected workersessions.Observation) (workersessions.Observation, error) {
	if r.providerSessions == nil {
		return projected, workersessions.ErrObservationProjectionUnavailable
	}
	result, err := r.providerSessions.Project(providersessions.ProjectRequest{
		Session: projected.ProviderSession.Clone(),
		Context: ctx,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, providersessions.ErrOperationCanceled) {
			return workersessions.Observation{}, workersessions.ErrObservationCanceled
		}
		return projected, workersessions.ErrObservationProjectionUnavailable
	}
	projected.Transcript = workersessions.TranscriptAvailabilityAvailable
	if usage := observationTokenUsage(result.Detail.Parse.TokenUsage); usage != nil {
		projected.TokenUsage = usage
	}
	projected.TurnUsage = observationTurnUsage(result.Detail.Parse.CumulativeInputTokens)
	projected.Parse = observationParseDiagnostics(result.Detail.Parse)
	return projected, nil
}

func observationContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return workersessions.ErrObservationCanceled
	}
	return nil
}

func cloneObservation(value *observation) *observation {
	if value == nil {
		return nil
	}
	clone := *value
	clone.workIDs = append([]string(nil), value.workIDs...)
	if value.endedAt != nil {
		ended := *value.endedAt
		clone.endedAt = &ended
	}
	clone.tokenUsage = cloneObservationTokenUsage(value.tokenUsage)
	return &clone
}

func cloneObservationTokenUsage(value *workersessions.TokenUsage) *workersessions.TokenUsage {
	if value == nil {
		return nil
	}
	clone := value.Clone()
	return &clone
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

type observationOrder struct {
	id        string
	startedAt time.Time
	attemptID string
}

func sortObservationOrder(values []observationOrder) {
	sort.SliceStable(values, func(i, j int) bool {
		left, right := values[i], values[j]
		switch {
		case !left.startedAt.Equal(right.startedAt):
			return left.startedAt.Before(right.startedAt)
		case left.attemptID != right.attemptID:
			return left.attemptID < right.attemptID
		default:
			return left.id < right.id
		}
	})
}

func sortObservationAttempts(observations []workersessions.Observation) {
	sort.SliceStable(observations, func(i, j int) bool {
		left, right := observations[i], observations[j]
		switch {
		case left.StartedAt != nil && right.StartedAt != nil && !left.StartedAt.Equal(*right.StartedAt):
			return left.StartedAt.Before(*right.StartedAt)
		case left.StartedAt != nil && right.StartedAt == nil:
			return true
		case left.StartedAt == nil && right.StartedAt != nil:
			return false
		case left.AttemptID != right.AttemptID:
			return left.AttemptID < right.AttemptID
		default:
			return left.WorkerSessionID < right.WorkerSessionID
		}
	})
}
