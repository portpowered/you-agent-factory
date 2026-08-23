package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/jsonvalue"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/legacysnapshot"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	sessionprojection "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionprojection"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workers "github.com/portpowered/infinite-you/pkg/services/workers"
)

// workRuntimeAdapter is the Factory Sessions-owned adapter from one live
// engine into Work's consumer-owned runtime port. Engine identities end here.
type workRuntimeAdapter struct {
	sessionID string
	clock     factoryruntime.Clock
	runtime   factoryruntime.Service
	// ingress is the Work-submission boundary declared when Factory Sessions
	// bound the runtime. It retires with factoryruntime.APIFactory.
	ingress factoryruntime.APIFactory
	// admissions is the session-scoped canonical Work admission projection.
	// It is initialized by Assembly from the runtime's ledger and advances from
	// appended events instead of replaying the full event history per read.
	admissions  *workAdmissionProjection
	readMetrics factorysessions.InvocationMetricsRecorder
}

const workReadMetricName = "work.read.snapshot"

type workReadObservation struct {
	snapshotDuration        time.Duration
	snapshotRows            int
	mappingDuration         time.Duration
	workRowsProjected       int
	admissionDuration       time.Duration
	admissionRows           int
	eventSubscriptions      int
	eventRecordsVisited     int
	projectionEventsVisited int
	projectionLockWait      time.Duration
	projectionCatchup       time.Duration
}

type workAdmissionReadStats struct {
	eventSubscriptions      int
	eventRecordsVisited     int
	projectionEventsVisited int
	projectionLockWait      time.Duration
	projectionCatchup       time.Duration
}

func (a workRuntimeAdapter) SubmitWorkRequest(ctx context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	if a.ingress == nil {
		return work.WorkRequestSubmitResult{}, fmt.Errorf("Factory Runtime work submission is required")
	}
	return a.ingress.SubmitWorkRequest(ctx, request)
}

func (a workRuntimeAdapter) MoveWork(ctx context.Context, workID, state string, source work.WorkStateChangeSource, requestID string) (work.OperatorMoveResult, error) {
	if a.runtime == nil {
		return work.OperatorMoveResult{}, fmt.Errorf("Factory Runtime work move is required")
	}
	result, err := a.runtime.ControlMoveWork(ctx, factoryruntime.MoveWorkRequest{
		WorkID: workID, StateName: state, Source: factoryruntime.WorkMoveSource(source), RequestID: requestID,
	})
	if err != nil {
		return work.OperatorMoveResult{}, translateMoveWorkFailure(err)
	}
	return work.OperatorMoveResult{
		WorkID: result.WorkID, WorkTypeID: result.WorkTypeID,
		FromState: result.FromState, ToState: result.ToState,
	}, nil
}

// translateMoveWorkFailure detaches engine-owned operator-move failures into
// the Work-owned sentinels Work's own surfaces branch on. Engine error identity
// ends at this adapter, exactly like engine result identity does. Failures the
// engine does not classify pass through unchanged.
func translateMoveWorkFailure(err error) error {
	switch {
	case errors.Is(err, factoryruntime.ErrMoveWorkRequestConflict):
		return work.ErrMoveWorkRequestAlreadyApplied
	case errors.Is(err, factoryruntime.ErrMoveWorkNotFound):
		return work.ErrMoveWorkNotFound
	case errors.Is(err, factoryruntime.ErrMoveWorkInvalidState):
		return work.ErrMoveWorkInvalidState
	case errors.Is(err, factoryruntime.ErrMoveWorkInFlightDispatch):
		return work.ErrMoveWorkInFlightDispatch
	case errors.Is(err, factoryruntime.ErrMoveWorkEngineTerminated):
		return work.ErrMoveWorkEngineTerminated
	default:
		return err
	}
}

func (a workRuntimeAdapter) ReadWorkSnapshot(ctx context.Context) (work.ReadSnapshot, error) {
	if a.clock == nil {
		return work.ReadSnapshot{}, fmt.Errorf("Factory Session Work read clock is required")
	}
	snapshotStarted := a.clock.Now()
	legacyObservation, err := runtimebinding.LegacyObservationForService(a.runtime)
	if err != nil {
		return work.ReadSnapshot{}, err
	}
	snapshot, err := workRuntimeSnapshot(ctx, legacyObservation)
	if err != nil {
		return work.ReadSnapshot{}, err
	}
	if snapshot == nil {
		return work.ReadSnapshot{}, nil
	}
	snapshotDuration := a.clock.Now().Sub(snapshotStarted)
	mappingStarted := a.clock.Now()
	materialized := factoryruntime.CollectPublicWorkTokens(snapshot.Marking.Tokens, snapshot.Dispatches)
	names := runtimeWorkNames(materialized.Tokens)
	sessionSummary := sessionprojection.ProjectFactorySessionStopSummary(a.sessionID, snapshot, nil)
	result := work.ReadSnapshot{Items: make([]work.ReadModel, 0, len(materialized.Tokens))}
	for _, token := range materialized.Tokens {
		_, inFlight := materialized.InFlightOnlyByID[token.ID]
		item := runtimeWorkItem(token, snapshot.Topology, inFlight, names, runtimeReadFacts{
			dispatches: snapshot.Dispatches, dispatchHistory: snapshot.DispatchHistory, results: snapshot.Results,
		})
		item.HumanApproval = runtimeHumanApprovalForWork(a.sessionID, token.Color.WorkID, snapshot.Dispatches, snapshot.Topology)
		item.StopSummary = runtimeWorkStopSummary(sessionprojection.ProjectWorkStopSummary(a.sessionID, snapshot, token, sessionSummary))
		result.Items = append(result.Items, item)
	}
	mappingDuration := a.clock.Now().Sub(mappingStarted)
	admissionStarted := a.clock.Now()
	admissionStats := workAdmissionReadStats{}
	if a.admissions != nil {
		result.Admissions, admissionStats.projectionEventsVisited, admissionStats.projectionLockWait, admissionStats.projectionCatchup = a.admissions.SnapshotWithStats()
	} else {
		admissions, stats, err := a.readWorkAdmissionsWithStats(ctx)
		if err != nil {
			return work.ReadSnapshot{}, err
		}
		result.Admissions = admissions
		admissionStats = stats
	}
	a.recordWorkRead(workReadObservation{
		snapshotDuration:        snapshotDuration,
		snapshotRows:            len(materialized.Tokens),
		mappingDuration:         mappingDuration,
		workRowsProjected:       len(result.Items),
		admissionDuration:       a.clock.Now().Sub(admissionStarted),
		admissionRows:           len(result.Admissions),
		eventSubscriptions:      admissionStats.eventSubscriptions,
		eventRecordsVisited:     admissionStats.eventRecordsVisited,
		projectionEventsVisited: admissionStats.projectionEventsVisited,
		projectionLockWait:      admissionStats.projectionLockWait,
		projectionCatchup:       admissionStats.projectionCatchup,
	})
	return result, nil
}

func (a workRuntimeAdapter) recordWorkRead(observation workReadObservation) {
	if a.readMetrics == nil {
		return
	}
	a.readMetrics.RecordInvocationMetric(factorysessions.InvocationMetric{
		Name: workReadMetricName,
		Labels: map[string]string{
			"snapshot_reads":            "1",
			"snapshot_rows":             strconv.Itoa(observation.snapshotRows),
			"snapshot_duration_ns":      strconv.FormatInt(observation.snapshotDuration.Nanoseconds(), 10),
			"mapping_duration_ns":       strconv.FormatInt(observation.mappingDuration.Nanoseconds(), 10),
			"work_rows_projected":       strconv.Itoa(observation.workRowsProjected),
			"admission_duration_ns":     strconv.FormatInt(observation.admissionDuration.Nanoseconds(), 10),
			"admission_rows":            strconv.Itoa(observation.admissionRows),
			"event_subscriptions":       strconv.Itoa(observation.eventSubscriptions),
			"event_records_visited":     strconv.Itoa(observation.eventRecordsVisited),
			"projection_events_visited": strconv.Itoa(observation.projectionEventsVisited),
			"projection_lock_wait_ns":   strconv.FormatInt(observation.projectionLockWait.Nanoseconds(), 10),
			"projection_catchup_ns":     strconv.FormatInt(observation.projectionCatchup.Nanoseconds(), 10),
		},
	})
}

func workRuntimeSnapshot(
	ctx context.Context,
	observation legacysnapshot.Provider,
) (*legacysnapshot.Snapshot, error) {
	if fast, ok := observation.(legacysnapshot.WorkProvider); ok {
		return fast.GetWorkStateSnapshot(ctx)
	}
	return observation.GetEngineStateSnapshot(ctx)
}

// workAdmissionProjection is the session-scoped admission read view used by
// Work list reads. The canonical ledger replays its existing prefix once when
// it is bound, then applies only newly appended Work Request events. Readers
// receive a detached copy and never hold the projection lock while selecting
// or mapping Work rows.
type workAdmissionProjection struct {
	sessionID string
	clock     factoryruntime.Clock

	mu                sync.RWMutex
	admissions        []work.WorkAdmission
	seenEvents        map[string]struct{}
	binding           *workAdmissionProjectionBinding
	generationRuntime *factorysessions.LiveRuntime
	generationLedger  recordings.Ledger
	generationSet     bool
	closed            bool
}

type workAdmissionProjectionBinding struct {
	ledger        recordings.Ledger
	ready         chan struct{}
	readyOnce     sync.Once
	callbackMu    sync.RWMutex
	projection    *workAdmissionProjection
	eventsVisited int
	catchup       time.Duration
}

func newWorkAdmissionProjection(sessionID string, clock factoryruntime.Clock) *workAdmissionProjection {
	return &workAdmissionProjection{
		sessionID:  strings.TrimSpace(sessionID),
		clock:      clock,
		seenEvents: make(map[string]struct{}),
	}
}

func newWorkAdmissionProjectionForGeneration(
	sessionID string,
	runtime *factorysessions.LiveRuntime,
	ledger recordings.Ledger,
	clock factoryruntime.Clock,
) *workAdmissionProjection {
	projection := newWorkAdmissionProjection(sessionID, clock)
	projection.generationRuntime = runtime
	projection.generationLedger = ledger
	projection.generationSet = true
	return projection
}

func (p *workAdmissionProjection) matchesGeneration(
	runtime *factorysessions.LiveRuntime,
	ledger recordings.Ledger,
) bool {
	if p == nil {
		return false
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return !p.closed && p.generationSet &&
		p.generationRuntime == runtime && sameLedger(p.generationLedger, ledger)
}

// Bind attaches the projection to one canonical ledger. AddEventRecorder
// replays the ledger prefix synchronously, which makes initialization and
// catch-up visible before the first Work read. Rebinding replaces the view with
// the replacement ledger's prefix; callbacks retained by the old ledger are
// ignored so a stopped runtime cannot repopulate the new session view.
func (p *workAdmissionProjection) Bind(ledger recordings.Ledger) {
	if p == nil || ledger == nil {
		return
	}
	for {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return
		}
		if !p.generationSet {
			p.generationLedger = ledger
			p.generationSet = true
		}
		if p.binding != nil && sameLedger(p.binding.ledger, ledger) {
			ready := p.binding.ready
			p.mu.Unlock()
			<-ready
			return
		}
		binding := &workAdmissionProjectionBinding{
			ledger: ledger, ready: make(chan struct{}), projection: p,
		}
		p.binding = binding
		p.admissions = nil
		p.seenEvents = make(map[string]struct{})
		p.mu.Unlock()

		startedAt := p.clock.Now()
		ledger.AddEventRecorder(binding.applyEvent)
		binding.callbackMu.Lock()
		binding.catchup = p.clock.Now().Sub(startedAt)
		binding.callbackMu.Unlock()
		binding.readyOnce.Do(func() { close(binding.ready) })
		return
	}
}

// applyEvent is registered on the ledger as a binding-owned method value. The
// callback therefore retains only the binding after Release severs its
// projection pointer, instead of retaining the projection and its ledger.
func (binding *workAdmissionProjectionBinding) applyEvent(event factorydefinitions.FactoryEvent) {
	if binding == nil {
		return
	}
	binding.callbackMu.RLock()
	projection := binding.projection
	binding.callbackMu.RUnlock()
	if projection != nil {
		projection.applyEvent(binding, event)
	}
}

// Snapshot returns the current admission facts without exposing mutable
// projection storage to Work's query path.
func (p *workAdmissionProjection) Snapshot() []work.WorkAdmission {
	admissions, _, _, _ := p.SnapshotWithStats()
	return admissions
}

func (p *workAdmissionProjection) SnapshotWithStats() ([]work.WorkAdmission, int, time.Duration, time.Duration) {
	if p == nil {
		return nil, 0, 0, 0
	}
	var lockWait time.Duration
	for {
		lockStarted := p.clock.Now()
		p.mu.RLock()
		lockWait += p.clock.Now().Sub(lockStarted)
		binding := p.binding
		if binding == nil {
			admissions := append([]work.WorkAdmission(nil), p.admissions...)
			p.mu.RUnlock()
			return admissions, 0, lockWait, 0
		}
		ready := binding.ready
		p.mu.RUnlock()
		<-ready

		lockStarted = p.clock.Now()
		p.mu.RLock()
		lockWait += p.clock.Now().Sub(lockStarted)
		if p.binding != binding {
			p.mu.RUnlock()
			continue
		}
		admissions := append([]work.WorkAdmission(nil), p.admissions...)
		eventsVisited := binding.eventsVisited
		binding.callbackMu.RLock()
		catchup := binding.catchup
		binding.callbackMu.RUnlock()
		p.mu.RUnlock()
		return admissions, eventsVisited, lockWait, catchup
	}
}

// Release retires the projection and prevents callbacks retained by a
// recording ledger from keeping session state alive or mutating it after the
// session has been removed. Existing detached readers retain their immutable
// admission slice until they finish.
func (p *workAdmissionProjection) Release() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.closed = true
	binding := p.binding
	p.binding = nil
	p.generationRuntime = nil
	p.generationLedger = nil
	p.seenEvents = nil
	p.mu.Unlock()
	if binding != nil {
		binding.callbackMu.Lock()
		binding.projection = nil
		binding.ledger = nil
		binding.callbackMu.Unlock()
		binding.readyOnce.Do(func() { close(binding.ready) })
	}
}

func (p *workAdmissionProjection) applyEvent(
	binding *workAdmissionProjectionBinding,
	event factorydefinitions.FactoryEvent,
) {
	if p == nil {
		return
	}
	p.mu.Lock()
	if p.binding != binding || p.closed {
		p.mu.Unlock()
		return
	}
	binding.eventsVisited++
	p.mu.Unlock()
	if event.Type != factorydefinitions.FactoryEventTypeWorkRequest {
		return
	}
	admissions := workAdmissionsFromFactoryEvents(p.sessionID, []factorydefinitions.FactoryEvent{event})
	if len(admissions) == 0 {
		return
	}
	eventKey := workAdmissionEventKey(event)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.binding != binding {
		return
	}
	if eventKey != "" {
		if _, seen := p.seenEvents[eventKey]; seen {
			return
		}
		p.seenEvents[eventKey] = struct{}{}
	}
	for _, admission := range admissions {
		admission.Order = len(p.admissions)
		p.admissions = append(p.admissions, admission)
	}
}

func workAdmissionEventKey(event factorydefinitions.FactoryEvent) string {
	if event.Id != "" {
		return "id:" + event.Id
	}
	if event.Context.Sequence == 0 && len(event.Payload) == 0 {
		return ""
	}
	return string(event.Type) + ":" + strconv.Itoa(event.Context.Sequence) + ":" + string(event.Payload)
}

func sameLedger(left, right recordings.Ledger) bool {
	if left == nil || right == nil {
		return left == right
	}
	leftValue, rightValue := reflect.ValueOf(left), reflect.ValueOf(right)
	if leftValue.Type() != rightValue.Type() || !leftValue.Type().Comparable() {
		return false
	}
	return leftValue.Interface() == rightValue.Interface()
}

func (a workRuntimeAdapter) readWorkAdmissionsWithStats(ctx context.Context) ([]work.WorkAdmission, workAdmissionReadStats, error) {
	stats := workAdmissionReadStats{eventSubscriptions: 1}
	if a.ingress == nil {
		return nil, stats, errors.New("Factory Runtime Work admission history is required")
	}
	historyContext := ctx
	if historyContext == nil {
		historyContext = context.Background()
	}
	historyContext, cancelHistory := context.WithCancel(historyContext)
	defer cancelHistory()
	stream, err := a.ingress.SubscribeFactoryEvents(
		historyContext,
		nil,
		factorydefinitions.FactoryEventReconnectScope{SessionID: a.sessionID},
	)
	if err != nil {
		return nil, stats, fmt.Errorf("subscribe Work admission history: %w", err)
	}
	if stream == nil {
		return nil, stats, errors.New("subscribe Work admission history: stream is unavailable")
	}
	stats.eventRecordsVisited = len(stream.History)
	return workAdmissionsFromFactoryEvents(a.sessionID, stream.History), stats, nil
}

func workAdmissionsFromFactoryEvents(
	sessionID string,
	events []factorydefinitions.FactoryEvent,
) []work.WorkAdmission {
	admissions := make([]work.WorkAdmission, 0)
	for _, event := range events {
		if event.Type != factorydefinitions.FactoryEventTypeWorkRequest {
			continue
		}
		if sessionID != "" && event.Context.SessionID != nil && *event.Context.SessionID != sessionID {
			continue
		}
		var payload work.WorkRequestEventPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			continue
		}
		for index, item := range payload.Works {
			workID := item.WorkID
			if workID == "" && event.Context.WorkIDs != nil && index < len(*event.Context.WorkIDs) {
				workID = (*event.Context.WorkIDs)[index]
			}
			if workID == "" || item.Name == "" {
				continue
			}
			admissions = append(admissions, work.WorkAdmission{
				WorkID: workID,
				Name:   item.Name,
				Order:  len(admissions),
			})
		}
	}
	return admissions
}

func runtimeHumanApprovalForWork(
	sessionID string,
	workID string,
	dispatches map[string]*factoryruntime.DispatchEntry,
	topology *legacysnapshot.RuntimeTopology,
) *work.HumanApprovalReadModel {
	if workID == "" || topology == nil || len(dispatches) == 0 {
		return nil
	}
	dispatchIDs := make([]string, 0, len(dispatches))
	for dispatchID := range dispatches {
		dispatchIDs = append(dispatchIDs, dispatchID)
	}
	sort.Strings(dispatchIDs)
	for _, dispatchID := range dispatchIDs {
		entry := dispatches[dispatchID]
		if entry == nil {
			continue
		}
		transition := topology.Transitions[entry.TransitionID]
		if transition == nil || transition.Type != factoryruntime.PetriTransitionHumanApproval {
			continue
		}
		for _, token := range entry.ConsumedTokens {
			if token.Color.WorkID != workID {
				continue
			}
			return &work.HumanApprovalReadModel{
				ApprovalID: approvalIDForDispatch(entry.DispatchID), SessionID: sessionID,
				DispatchID: entry.DispatchID, WorkstationID: entry.TransitionID,
				WorkstationName: entry.WorkstationName, Decisions: []string{"APPROVE", "REJECT"},
				Status: "PENDING",
			}
		}
	}
	return nil
}

func approvalIDForDispatch(dispatchID string) string {
	return "approval-" + dispatchID
}

type runtimeReadFacts struct {
	dispatches      map[string]*factoryruntime.DispatchEntry
	dispatchHistory []factoryruntime.CompletedDispatch
	results         []workers.WorkResult
}

func runtimeWorkItem(
	token *workers.Token,
	net *legacysnapshot.RuntimeTopology,
	inFlight bool,
	names map[string]string,
	facts ...runtimeReadFacts,
) work.ReadModel {
	var readFacts runtimeReadFacts
	if len(facts) > 0 {
		readFacts = facts[0]
	}
	name := runtimeFirstNonEmpty(token.Color.Name, token.Color.WorkID, token.ID)
	state := runtimeWorkState(token, net, inFlight)
	item := work.ReadModel{CursorID: token.ID, Name: name, WorkID: token.Color.WorkID, RequestID: token.Color.RequestID, WorkTypeName: token.Color.WorkTypeID, State: state, FailureDetail: runtimeWorkFailureDetail(token.Color.WorkID, state, readFacts.dispatchHistory, readFacts.results), ChainingTraceDepth: token.Color.ChainingTraceDepth, CurrentChainingTraceID: runtimeFirstNonEmpty(token.Color.CurrentChainingTraceID, token.Color.TraceID), PreviousChainingTraceIDs: append([]string(nil), token.Color.PreviousChainingTraceIDs...), TraceID: token.Color.TraceID, Content: work.CloneWorkContentParts(token.Color.Content), StructuredResult: jsonvalue.Clone(token.Color.StructuredResult), StructuredResultPresent: jsonvalue.Present(token.Color.StructuredResult, token.Color.StructuredResultPresent), Tags: work.CloneTags(token.Color.Tags), ExpectedArtifacts: (factoryruntime.WorkArtifactProjection{}).Project(factoryruntime.WorkArtifactProjectionInput{Token: token, Topology: net, Dispatches: readFacts.dispatches, DispatchHistory: readFacts.dispatchHistory, Results: readFacts.results})}
	for _, relation := range token.Color.Relations {
		item.Relations = append(item.Relations, work.ReadRelation{Type: relation.Type, SourceWorkName: name, TargetWorkName: runtimeFirstNonEmpty(names[relation.TargetWorkID], relation.TargetWorkID), TargetWorkID: relation.TargetWorkID, RequiredState: relation.RequiredState})
	}
	return item
}

func runtimeWorkFailureDetail(
	workID string,
	state *work.State,
	history []factoryruntime.CompletedDispatch,
	results []workers.WorkResult,
) *work.FailureDetail {
	if strings.TrimSpace(workID) == "" || state == nil || state.Type != work.StateTypeFailed {
		return nil
	}
	for index := len(history) - 1; index >= 0; index-- {
		dispatch := history[index]
		if dispatch.Outcome != workers.OutcomeFailed || !runtimeCompletedDispatchContainsWork(dispatch, workID) {
			continue
		}
		if dispatch.FailureDetail != nil {
			return &work.FailureDetail{Reason: string(dispatch.FailureDetail.Reason), Message: dispatch.FailureDetail.Message}
		}
		for resultIndex := len(results) - 1; resultIndex >= 0; resultIndex-- {
			result := results[resultIndex]
			if result.DispatchID != dispatch.DispatchID || result.FailureDetail == nil {
				continue
			}
			return &work.FailureDetail{Reason: string(result.FailureDetail.Reason), Message: result.FailureDetail.Message}
		}
		return nil
	}
	return nil
}

func runtimeCompletedDispatchContainsWork(dispatch factoryruntime.CompletedDispatch, workID string) bool {
	for _, token := range dispatch.ConsumedTokens {
		if token.Color.WorkID == workID {
			return true
		}
	}
	for _, mutation := range dispatch.OutputMutations {
		if mutation.TokenID == workID || (mutation.Token != nil && mutation.Token.Color.WorkID == workID) {
			return true
		}
	}
	return false
}

func runtimeWorkState(token *workers.Token, net *legacysnapshot.RuntimeTopology, inFlight bool) *work.State {
	if token == nil {
		return nil
	}
	workType, stateName := token.Color.WorkTypeID, token.State
	if stateName == "" {
		return nil
	}
	category := string(factoryruntime.CategoryForState(runtimeWorkTypes(net), workType, stateName))
	if inFlight {
		category = work.StateTypeProcessing
	}
	return &work.State{Name: stateName, Type: category}
}

func runtimeWorkTypes(net *legacysnapshot.RuntimeTopology) map[string]*legacysnapshot.RuntimeWorkType {
	if net == nil {
		return nil
	}
	return net.WorkTypes
}
func runtimeWorkNames(tokens []*workers.Token) map[string]string {
	result := make(map[string]string, len(tokens))
	for _, token := range tokens {
		if token != nil && token.Color.WorkID != "" {
			result[token.Color.WorkID] = runtimeFirstNonEmpty(token.Color.Name, token.Color.WorkID, token.ID)
		}
	}
	return result
}
func runtimeFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func runtimeWorkStopSummary(summary *factorysessions.StopSummary) *work.StopSummary {
	if summary == nil {
		return nil
	}
	result := &work.StopSummary{SessionID: summary.SessionID, StopKind: string(summary.StopKind), SessionLifecycleStatus: summary.SessionLifecycleStatus, WorkID: summary.WorkID, WorkName: summary.WorkName, WorkTypeName: summary.WorkTypeName, WorkState: summary.WorkState, LatestResultSummary: summary.LatestResultSummary, SuggestedRecoverySurface: summary.SuggestedRecoverySurface, SuggestedRecoveryAction: summary.SuggestedRecoveryAction}
	if summary.LatestDispatch != nil {
		result.LatestDispatch = &work.StopDispatchSummary{DispatchID: summary.LatestDispatch.DispatchID, Status: string(summary.LatestDispatch.Status), DispatchKind: string(summary.LatestDispatch.DispatchKind), WorkstationName: summary.LatestDispatch.WorkstationName}
		if summary.LatestDispatch.FailureDetail != nil {
			result.LatestDispatch.FailureDetail = &work.StopFailureDetail{Reason: string(summary.LatestDispatch.FailureDetail.Reason), Message: summary.LatestDispatch.FailureDetail.Message}
		}
	}
	return result
}
