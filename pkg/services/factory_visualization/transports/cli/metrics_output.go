package cli

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func renderHumanMetrics(
	groupBy string,
	sessionID string,
	result factoryvisualization.RuntimeMetricsQueryResult,
) string {
	var output strings.Builder
	if strings.TrimSpace(sessionID) == "" {
		fmt.Fprintln(&output, "Scope: all Factory Sessions")
	} else {
		fmt.Fprintf(&output, "Scope: Factory Session %s\n", sessionID)
	}
	fmt.Fprintf(&output, "Group by: %s\n", groupBy)
	fmt.Fprintf(&output, "Cost: %s\n", normalizeMetricsCostAvailability(result.Cost.Availability))
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "Totals:")
	renderMetricsAggregate(&output, "  ", result.Totals)
	fmt.Fprintln(&output)

	breakdowns := metricsBreakdowns(groupBy, result)
	fmt.Fprintf(&output, "Breakdown by %s: %d rows\n", groupBy, len(breakdowns))
	for _, breakdown := range breakdowns {
		fmt.Fprintf(&output, "  %s:\n", metricsBreakdownDisplayKey(breakdown.Key))
		renderMetricsAggregate(&output, "    ", breakdown.Aggregate)
	}
	return output.String()
}

func metricsBreakdownDisplayKey(key string) string {
	if key == factoryvisualization.RuntimeMetricsUnavailableProviderKey {
		return factoryvisualization.RuntimeMetricsUnavailableProviderLabel
	}
	return key
}

type metricsJSONDocument struct {
	Scope   metricsJSONScope     `json:"scope"`
	GroupBy string               `json:"group_by"`
	Units   metricsJSONUnits     `json:"units"`
	Cost    metricsJSONCost      `json:"cost"`
	Totals  metricsJSONAggregate `json:"totals"`
	Groups  []metricsJSONGroup   `json:"groups"`
}

type metricsJSONScope struct {
	Kind             string  `json:"kind"`
	FactorySessionID *string `json:"factory_session_id"`
}

type metricsJSONUnits struct {
	Tokens  string `json:"tokens"`
	Counts  string `json:"counts"`
	Latency string `json:"latency"`
}

type metricsJSONCost struct {
	Availability string `json:"availability"`
}

type metricsJSONGroup struct {
	Key       string               `json:"key"`
	Aggregate metricsJSONAggregate `json:"aggregate"`
}

type metricsJSONAggregate struct {
	InputTokens         float64            `json:"input_tokens"`
	OutputTokens        float64            `json:"output_tokens"`
	CompletedDispatches float64            `json:"completed_dispatches"`
	FailuresByReason    map[string]float64 `json:"failures_by_reason"`
	DispatchLatency     metricsJSONLatency `json:"dispatch_latency"`
	ProviderLatency     metricsJSONLatency `json:"provider_latency"`
}

type metricsJSONLatency struct {
	Unit    string   `json:"unit"`
	Samples int      `json:"samples"`
	P50     *float64 `json:"p50"`
	P95     *float64 `json:"p95"`
}

func renderMetricsOutput(
	groupBy string,
	sessionID string,
	jsonOutput bool,
	result factoryvisualization.RuntimeMetricsQueryResult,
) (string, error) {
	if !jsonOutput {
		return renderHumanMetrics(groupBy, sessionID, result), nil
	}
	document := metricsJSONDocument{
		Scope:   metricsJSONScopeForSession(sessionID),
		GroupBy: groupBy,
		Units: metricsJSONUnits{
			Tokens:  "tokens",
			Counts:  "count",
			Latency: "milliseconds",
		},
		Cost:   metricsJSONCost{Availability: normalizeMetricsCostAvailability(result.Cost.Availability)},
		Totals: toJSONAggregate(result.Totals),
		Groups: toJSONGroups(metricsBreakdowns(groupBy, result)),
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("encode metrics JSON: %w", err)
	}
	return string(append(encoded, '\n')), nil
}

func normalizeMetricsCostAvailability(availability factoryvisualization.RuntimeMetricsCostAvailability) string {
	normalized := strings.ToLower(strings.TrimSpace(string(availability)))
	if normalized == "" {
		return "unavailable"
	}
	return normalized
}

func metricsJSONScopeForSession(sessionID string) metricsJSONScope {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return metricsJSONScope{Kind: "all_factory_sessions"}
	}
	return metricsJSONScope{Kind: "factory_session", FactorySessionID: &sessionID}
}

func toJSONGroups(breakdowns []factoryvisualization.RuntimeMetricsBreakdown) []metricsJSONGroup {
	groups := make([]metricsJSONGroup, 0, len(breakdowns))
	for _, breakdown := range breakdowns {
		groups = append(groups, metricsJSONGroup{
			Key:       breakdown.Key,
			Aggregate: toJSONAggregate(breakdown.Aggregate),
		})
	}
	return groups
}

func toJSONAggregate(aggregate factoryvisualization.RuntimeMetricsAggregate) metricsJSONAggregate {
	return metricsJSONAggregate{
		InputTokens:         aggregate.InputTokens,
		OutputTokens:        aggregate.OutputTokens,
		CompletedDispatches: aggregate.CompletedDispatches,
		FailuresByReason:    cloneMetricFailures(aggregate.FailuresByReason),
		DispatchLatency:     toJSONLatency(aggregate.DispatchDuration),
		ProviderLatency:     toJSONLatency(aggregate.ProviderDuration),
	}
}

func cloneMetricFailures(failures map[string]float64) map[string]float64 {
	if len(failures) == 0 {
		return map[string]float64{}
	}
	clone := make(map[string]float64, len(failures))
	for key, value := range failures {
		clone[key] = value
	}
	return clone
}

func toJSONLatency(duration *factoryvisualization.RuntimeMetricsDuration) metricsJSONLatency {
	if duration == nil {
		return metricsJSONLatency{Unit: "milliseconds"}
	}
	unit := duration.Unit
	if strings.TrimSpace(unit) == "" {
		unit = "milliseconds"
	}
	if duration.Samples == 0 {
		return metricsJSONLatency{Unit: unit}
	}
	return metricsJSONLatency{
		Unit:    unit,
		Samples: duration.Samples,
		P50:     duration.P50,
		P95:     duration.P95,
	}
}

func metricsBreakdowns(
	groupBy string,
	result factoryvisualization.RuntimeMetricsQueryResult,
) []factoryvisualization.RuntimeMetricsBreakdown {
	var source []factoryvisualization.RuntimeMetricsBreakdown
	switch groupBy {
	case metricsGroupByWorker:
		source = result.WorkerTypes
	case metricsGroupByProvider:
		source = result.Providers
	default:
		source = result.Workstations
	}
	breakdowns := append([]factoryvisualization.RuntimeMetricsBreakdown(nil), source...)
	sort.SliceStable(breakdowns, func(i, j int) bool {
		return breakdowns[i].Key < breakdowns[j].Key
	})
	return breakdowns
}

func renderMetricsAggregate(
	output *strings.Builder,
	indent string,
	aggregate factoryvisualization.RuntimeMetricsAggregate,
) {
	fmt.Fprintf(output, "%sInput tokens: %s\n", indent, formatMetricValue(aggregate.InputTokens))
	fmt.Fprintf(output, "%sOutput tokens: %s\n", indent, formatMetricValue(aggregate.OutputTokens))
	fmt.Fprintf(output, "%sCompleted dispatches: %s\n", indent, formatMetricValue(aggregate.CompletedDispatches))
	renderFailureReasons(output, indent, aggregate.FailuresByReason)
	renderLatency(output, indent, "Dispatch latency (milliseconds)", aggregate.DispatchDuration)
	renderLatency(output, indent, "Provider latency (milliseconds)", aggregate.ProviderDuration)
}

func renderFailureReasons(output *strings.Builder, indent string, failures map[string]float64) {
	if len(failures) == 0 {
		fmt.Fprintf(output, "%sFailures by reason: none\n", indent)
		return
	}
	keys := make([]string, 0, len(failures))
	for key := range failures {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fmt.Fprintf(output, "%sFailures by reason:\n", indent)
	for _, key := range keys {
		fmt.Fprintf(output, "%s  %s: %s\n", indent, key, formatMetricValue(failures[key]))
	}
}

func renderLatency(
	output *strings.Builder,
	indent string,
	label string,
	duration *factoryvisualization.RuntimeMetricsDuration,
) {
	if duration == nil || duration.Samples == 0 || duration.P50 == nil || duration.P95 == nil {
		fmt.Fprintf(output, "%s%s: no samples\n", indent, label)
		return
	}
	fmt.Fprintf(output, "%s%s: p50=%s, p95=%s, samples=%d\n",
		indent, label, formatMetricValue(*duration.P50), formatMetricValue(*duration.P95), duration.Samples)
}

func formatMetricValue(value float64) string {
	if value == 0 {
		return "0"
	}
	if math.Trunc(value) == value {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

type metricsSessionDocument struct {
	FactorySessionID          string                          `json:"factory_session_id"`
	AsOf                      *time.Time                      `json:"as_of"`
	Status                    string                          `json:"status"`
	Units                     metricsSessionUnits             `json:"units"`
	ElapsedWallTimeMillis     *int64                          `json:"elapsed_wall_time_ms"`
	DistinctWorkItems         int                             `json:"distinct_work_items"`
	DispatchAttempts          int                             `json:"dispatch_attempts"`
	WorkerSessions            int                             `json:"worker_sessions"`
	MaxConcurrentExecutions   int                             `json:"max_concurrent_executions"`
	SummedExecutionTimeMillis *int64                          `json:"summed_execution_time_ms"`
	SummedQueueTimeMillis     *int64                          `json:"summed_queue_time_ms"`
	Retries                   int                             `json:"retries"`
	AttemptOutcomes           metricsSessionOutcomeCounts     `json:"attempt_outcomes"`
	Incomplete                metricsSessionIncomplete        `json:"incomplete"`
	QueueDuration             metricsSessionDuration          `json:"queue_duration"`
	ExecutionDuration         metricsSessionDuration          `json:"execution_duration"`
	Attempts                  []metricsSessionAttemptDocument `json:"attempts"`
}

type metricsSessionUnits struct {
	Duration string `json:"duration"`
	Counts   string `json:"counts"`
}

type metricsSessionOutcomeCounts struct {
	Accepted    int `json:"accepted"`
	Continued   int `json:"continued"`
	Rejected    int `json:"rejected"`
	Failed      int `json:"failed"`
	Interrupted int `json:"interrupted"`
	Canceled    int `json:"canceled"`
	Unknown     int `json:"unknown"`
}

type metricsSessionIncomplete struct {
	Queued         int `json:"queued"`
	Running        int `json:"running"`
	MissingOutcome int `json:"missing_outcome"`
}

type metricsSessionDuration struct {
	Unit     string `json:"unit"`
	Samples  int    `json:"samples"`
	Excluded int    `json:"excluded"`
	P50      *int64 `json:"p50"`
	P95      *int64 `json:"p95"`
}

type metricsSessionAttemptDocument struct {
	DispatchID              *string  `json:"dispatch_id"`
	WorkID                  *string  `json:"work_id"`
	WorkIDs                 []string `json:"work_ids"`
	WorkerSessionID         *string  `json:"worker_session_id"`
	Attempt                 int      `json:"attempt"`
	RetryOfDispatchID       *string  `json:"retry_of_dispatch_id"`
	Status                  string   `json:"status"`
	Outcome                 *string  `json:"outcome"`
	QueueDurationMillis     *int64   `json:"queue_duration_ms"`
	ExecutionDurationMillis *int64   `json:"execution_duration_ms"`
}

type metricsSessionAttemptState struct {
	key               string
	dispatchID        string
	retryOfDispatchID string
	workIDs           map[string]struct{}
	workerSessionIDs  map[string]struct{}
	queuedAt          *time.Time
	startedAt         *time.Time
	terminalAt        *time.Time
	executionDuration *int64
	outcome           string
	status            string
	terminal          bool
	firstEventIndex   int
	attempt           int
}

type metricsSessionReducer struct {
	sessionID      string
	workIDs        map[string]struct{}
	workerSessions map[string]struct{}
	attempts       map[string]*metricsSessionAttemptState
	startedAt      *time.Time
	completedAt    *time.Time
	asOf           *time.Time
	status         string
	eventIndex     int
}

func reduceMetricsSession(sessionID string, events []factoryapi.FactoryEvent) (metricsSessionDocument, error) {
	reducer := newMetricsSessionReducer(sessionID)
	for _, event := range deduplicateMetricsSessionEvents(events) {
		if err := reducer.consume(event); err != nil {
			return metricsSessionDocument{}, err
		}
		reducer.eventIndex++
	}
	return reducer.document(), nil
}

func newMetricsSessionReducer(sessionID string) *metricsSessionReducer {
	return &metricsSessionReducer{
		sessionID:      strings.TrimSpace(sessionID),
		workIDs:        make(map[string]struct{}),
		workerSessions: make(map[string]struct{}),
		attempts:       make(map[string]*metricsSessionAttemptState),
	}
}

func deduplicateMetricsSessionEvents(events []factoryapi.FactoryEvent) []factoryapi.FactoryEvent {
	seen := make(map[string]struct{}, len(events))
	result := make([]factoryapi.FactoryEvent, 0, len(events))
	for _, event := range events {
		key := metricsSessionEventKey(event)
		if key != "" {
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
		}
		result = append(result, event)
	}
	return result
}

func metricsSessionEventKey(event factoryapi.FactoryEvent) string {
	if id := strings.TrimSpace(event.Id); id != "" {
		return "id:" + id
	}
	if event.Context.SessionSequence != nil {
		return fmt.Sprintf("session-sequence:%d", *event.Context.SessionSequence)
	}
	if event.Context.Sequence != 0 {
		return fmt.Sprintf("sequence:%d", event.Context.Sequence)
	}
	return ""
}

func (reducer *metricsSessionReducer) consume(event factoryapi.FactoryEvent) error {
	reducer.observeTime(event.Context.EventTime)
	reducer.addWorkIDs(event.Context.WorkIds)
	switch event.Type {
	case factoryapi.FactoryEventTypeSessionStarted:
		return reducer.consumeSessionStarted(event)
	case factoryapi.FactoryEventTypeSessionCompleted:
		return reducer.consumeSessionCompleted(event)
	case factoryapi.FactoryEventTypeSessionLifecycleControl:
		return reducer.consumeSessionLifecycleControl(event)
	case factoryapi.FactoryEventTypeSessionPaused:
		return reducer.consumeSessionPaused(event)
	case factoryapi.FactoryEventTypeSessionResumed:
		return reducer.consumeSessionResumed(event)
	case factoryapi.FactoryEventTypeWorkRequest:
		return reducer.consumeWorkRequest(event)
	case factoryapi.FactoryEventTypeWorkStateChange:
		return reducer.consumeWorkStateChange(event)
	case factoryapi.FactoryEventTypeDispatchQueued:
		return reducer.consumeDispatchQueued(event)
	case factoryapi.FactoryEventTypeDispatchRequest:
		return reducer.consumeDispatchRequest(event)
	case factoryapi.FactoryEventTypeDispatchResponse:
		return reducer.consumeDispatchResponse(event)
	case factoryapi.FactoryEventTypeDispatchReconciled:
		return reducer.consumeDispatchReconciled(event)
	case factoryapi.FactoryEventTypeDispatchInterrupted:
		return reducer.consumeDispatchInterrupted(event)
	case factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation:
		return reducer.consumeWorkerSessionAssociation(event)
	default:
		return nil
	}
}

func (reducer *metricsSessionReducer) consumeSessionStarted(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsSessionStartedEventPayload()
	if err != nil {
		return fmt.Errorf("decode session started payload: %w", err)
	}
	startedAt := payload.StartedAt
	if startedAt.IsZero() {
		startedAt = event.Context.EventTime
	}
	reducer.setEarliest(&reducer.startedAt, startedAt)
	reducer.status = "RUNNING"
	reducer.observeTime(startedAt)
	return nil
}

func (reducer *metricsSessionReducer) consumeSessionCompleted(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsSessionCompletedEventPayload()
	if err != nil {
		return fmt.Errorf("decode session completed payload: %w", err)
	}
	completedAt := payload.CompletedAt
	if completedAt.IsZero() {
		completedAt = event.Context.EventTime
	}
	reducer.setLatest(&reducer.completedAt, completedAt)
	reducer.observeTime(completedAt)
	if status := strings.TrimSpace(string(payload.FinalStatus)); status != "" {
		reducer.status = status
	}
	return nil
}

func (reducer *metricsSessionReducer) consumeSessionLifecycleControl(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsSessionLifecycleControlEventPayload()
	if err != nil {
		return fmt.Errorf("decode session lifecycle payload: %w", err)
	}
	if status := strings.TrimSpace(string(payload.NewStatus)); status != "" {
		reducer.status = status
	}
	reducer.observeTime(payload.OccurredAt)
	return nil
}

func (reducer *metricsSessionReducer) consumeSessionPaused(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsSessionPausedEventPayload()
	if err != nil {
		return fmt.Errorf("decode session paused payload: %w", err)
	}
	if status := strings.TrimSpace(string(payload.Status)); status != "" {
		reducer.status = status
	}
	reducer.observeTime(payload.PausedAt)
	return nil
}

func (reducer *metricsSessionReducer) consumeSessionResumed(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsSessionResumedEventPayload()
	if err != nil {
		return fmt.Errorf("decode session resumed payload: %w", err)
	}
	if status := strings.TrimSpace(string(payload.Status)); status != "" {
		reducer.status = status
	}
	reducer.observeTime(payload.ResumedAt)
	return nil
}

func (reducer *metricsSessionReducer) consumeWorkRequest(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsWorkRequestEventPayload()
	if err != nil {
		return fmt.Errorf("decode work request payload: %w", err)
	}
	if payload.Works == nil {
		return nil
	}
	for _, work := range *payload.Works {
		reducer.addWorkID(pointerString(work.WorkId))
	}
	return nil
}

func (reducer *metricsSessionReducer) consumeWorkStateChange(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsWorkStateChangeEventPayload()
	if err != nil {
		return fmt.Errorf("decode work state change payload: %w", err)
	}
	reducer.addWorkID(payload.WorkId)
	return nil
}

func (reducer *metricsSessionReducer) consumeDispatchQueued(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsDispatchQueuedEventPayload()
	if err != nil {
		return fmt.Errorf("decode dispatch queued payload: %w", err)
	}
	state := reducer.attemptFor(event)
	if !state.terminal {
		state.status = "QUEUED"
	}
	reducer.setEarliest(&state.queuedAt, event.Context.EventTime)
	if payload.InputWorkIds != nil {
		for _, workID := range *payload.InputWorkIds {
			state.addWorkID(workID)
			reducer.addWorkID(workID)
		}
	}
	state.retryOfDispatchID = pointerString(payload.RetryOfDispatchId)
	return nil
}

func (reducer *metricsSessionReducer) consumeDispatchRequest(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsDispatchRequestEventPayload()
	if err != nil {
		return fmt.Errorf("decode dispatch request payload: %w", err)
	}
	state := reducer.attemptFor(event)
	if !state.terminal {
		state.status = "RUNNING"
	}
	reducer.setEarliest(&state.startedAt, event.Context.EventTime)
	for _, input := range payload.Inputs {
		state.addWorkID(input.WorkId)
		reducer.addWorkID(input.WorkId)
	}
	return nil
}

func (reducer *metricsSessionReducer) consumeDispatchResponse(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		return fmt.Errorf("decode dispatch response payload: %w", err)
	}
	state := reducer.attemptFor(event)
	state.terminal = true
	state.status = "COMPLETED"
	state.outcome = normalizeMetricsSessionOutcome(string(payload.Outcome))
	if state.outcome != "" {
		state.status = state.outcome
	}
	reducer.setLatest(&state.terminalAt, event.Context.EventTime)
	if payload.DurationMillis != nil && *payload.DurationMillis >= 0 {
		value := *payload.DurationMillis
		state.executionDuration = &value
	}
	if payload.OutputWork != nil {
		for _, work := range *payload.OutputWork {
			reducer.addWorkID(pointerString(work.WorkId))
		}
	}
	return nil
}

func (reducer *metricsSessionReducer) consumeDispatchReconciled(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsDispatchReconciledEventPayload()
	if err != nil {
		return fmt.Errorf("decode dispatch reconciled payload: %w", err)
	}
	state := reducer.attemptFor(event)
	status := strings.ToUpper(strings.TrimSpace(string(payload.ReconciledStatus)))
	if dispatchStatusIsTerminal(status) {
		if !state.terminal || state.outcome == "" {
			state.terminal = true
			state.outcome = normalizeMetricsSessionOutcome(status)
			state.status = status
		}
		reducer.setLatest(&state.terminalAt, event.Context.EventTime)
		return nil
	}
	if !state.terminal && status != "" {
		state.status = status
	}
	return nil
}

func (reducer *metricsSessionReducer) consumeDispatchInterrupted(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsDispatchInterruptedEventPayload()
	if err != nil {
		return fmt.Errorf("decode dispatch interrupted payload: %w", err)
	}
	state := reducer.attemptFor(event)
	if !state.terminal || state.outcome == "" {
		state.terminal = true
		state.status = "INTERRUPTED"
		state.outcome = "INTERRUPTED"
	}
	when := payload.InterruptedAt
	if when.IsZero() {
		when = event.Context.EventTime
	}
	reducer.setLatest(&state.terminalAt, when)
	reducer.observeTime(when)
	return nil
}

func (reducer *metricsSessionReducer) consumeWorkerSessionAssociation(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsDispatchWorkerSessionAssociationEventPayload()
	if err != nil {
		return fmt.Errorf("decode worker session association payload: %w", err)
	}
	workerSessionID := strings.TrimSpace(payload.WorkerSessionId)
	if workerSessionID == "" {
		return nil
	}
	reducer.workerSessions[workerSessionID] = struct{}{}
	if event.Context.DispatchId != nil {
		state := reducer.attemptFor(event)
		state.workerSessionIDs[workerSessionID] = struct{}{}
	}
	return nil
}

func (reducer *metricsSessionReducer) attemptFor(event factoryapi.FactoryEvent) *metricsSessionAttemptState {
	key := metricsSessionAttemptKey(event)
	state, exists := reducer.attempts[key]
	if !exists {
		state = &metricsSessionAttemptState{
			key:              key,
			workIDs:          make(map[string]struct{}),
			workerSessionIDs: make(map[string]struct{}),
			firstEventIndex:  reducer.eventIndex,
		}
		reducer.attempts[key] = state
	}
	if event.Context.DispatchId != nil {
		state.dispatchID = strings.TrimSpace(*event.Context.DispatchId)
	}
	for _, workID := range pointerStrings(event.Context.WorkIds) {
		state.addWorkID(workID)
	}
	return state
}

func metricsSessionAttemptKey(event factoryapi.FactoryEvent) string {
	if event.Context.DispatchId != nil && strings.TrimSpace(*event.Context.DispatchId) != "" {
		return "dispatch:" + strings.TrimSpace(*event.Context.DispatchId)
	}
	if id := strings.TrimSpace(event.Id); id != "" {
		return "event:" + id
	}
	if event.Context.SessionSequence != nil {
		return fmt.Sprintf("session-sequence:%d", *event.Context.SessionSequence)
	}
	return fmt.Sprintf("sequence:%d:%s", event.Context.Sequence, event.Type)
}

func (state *metricsSessionAttemptState) addWorkID(workID string) {
	if workID = strings.TrimSpace(workID); workID != "" {
		state.workIDs[workID] = struct{}{}
	}
}

func (reducer *metricsSessionReducer) addWorkIDs(workIDs *[]string) {
	for _, workID := range pointerStrings(workIDs) {
		reducer.addWorkID(workID)
	}
}

func (reducer *metricsSessionReducer) addWorkID(workID string) {
	if workID = strings.TrimSpace(workID); workID != "" {
		reducer.workIDs[workID] = struct{}{}
	}
}

func (reducer *metricsSessionReducer) observeTime(value time.Time) {
	if value.IsZero() {
		return
	}
	normalized := value.UTC()
	if reducer.asOf == nil || normalized.After(*reducer.asOf) {
		reducer.asOf = &normalized
	}
}

func (reducer *metricsSessionReducer) setEarliest(target **time.Time, value time.Time) {
	if value.IsZero() {
		return
	}
	normalized := value.UTC()
	if *target == nil || normalized.Before(**target) {
		*target = &normalized
	}
	reducer.observeTime(normalized)
}

func (reducer *metricsSessionReducer) setLatest(target **time.Time, value time.Time) {
	if value.IsZero() {
		return
	}
	normalized := value.UTC()
	if *target == nil || normalized.After(**target) {
		*target = &normalized
	}
	reducer.observeTime(normalized)
}

func sumMetricsSessionDurations(values []int64) *int64 {
	if len(values) == 0 {
		return nil
	}
	var total int64
	for _, value := range values {
		total += value
	}
	return &total
}

func metricsSessionDurationFor(values []int64, total int) metricsSessionDuration {
	duration := metricsSessionDuration{Unit: "milliseconds", Excluded: total - len(values)}
	if len(values) == 0 {
		return duration
	}
	ordered := append([]int64(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	duration.Samples = len(ordered)
	duration.P50 = metricsSessionQuantile(ordered, 0.50)
	duration.P95 = metricsSessionQuantile(ordered, 0.95)
	return duration
}

func metricsSessionQuantile(values []int64, percentile float64) *int64 {
	if len(values) == 0 {
		return nil
	}
	index := int(math.Ceil(percentile*float64(len(values)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	value := values[index]
	return &value
}

type metricsSessionBoundary struct {
	when  time.Time
	delta int
}

func metricsSessionMaxConcurrency(states []*metricsSessionAttemptState, asOf *time.Time) int {
	boundaries := make([]metricsSessionBoundary, 0, len(states)*2)
	for _, state := range states {
		if state.startedAt == nil {
			continue
		}
		end := state.terminalAt
		if end == nil && !state.terminal {
			end = asOf
		}
		if end == nil && state.executionDuration != nil {
			derived := state.startedAt.Add(time.Duration(*state.executionDuration) * time.Millisecond)
			end = &derived
		}
		if end == nil || end.Before(*state.startedAt) {
			continue
		}
		boundaries = append(boundaries,
			metricsSessionBoundary{when: *state.startedAt, delta: 1},
			metricsSessionBoundary{when: *end, delta: -1},
		)
	}
	sort.SliceStable(boundaries, func(i, j int) bool {
		if boundaries[i].when.Equal(boundaries[j].when) {
			return boundaries[i].delta < boundaries[j].delta
		}
		return boundaries[i].when.Before(boundaries[j].when)
	})
	active, maximum := 0, 0
	for _, boundary := range boundaries {
		active += boundary.delta
		if active < 0 {
			active = 0
		}
		if active > maximum {
			maximum = active
		}
	}
	return maximum
}

func normalizeMetricsSessionOutcome(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	switch value {
	case "ACCEPTED", "CONTINUE", "REJECTED", "FAILED", "INTERRUPTED", "CANCELED":
		return value
	case "COMPLETED", "SUCCEEDED":
		return "ACCEPTED"
	default:
		return value
	}
}

func dispatchStatusIsTerminal(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "COMPLETED", "FAILED", "INTERRUPTED", "CANCELED", "TERMINATED":
		return true
	default:
		return false
	}
}

func incrementMetricsSessionOutcome(counts *metricsSessionOutcomeCounts, outcome string) {
	switch normalizeMetricsSessionOutcome(outcome) {
	case "ACCEPTED":
		counts.Accepted++
	case "CONTINUE":
		counts.Continued++
	case "REJECTED":
		counts.Rejected++
	case "FAILED":
		counts.Failed++
	case "INTERRUPTED":
		counts.Interrupted++
	case "CANCELED":
		counts.Canceled++
	default:
		counts.Unknown++
	}
}

func pointerStrings(values *[]string) []string {
	if values == nil {
		return nil
	}
	return *values
}

func pointerString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func renderMetricsSessionOutput(document metricsSessionDocument, jsonOutput bool) (string, error) {
	if jsonOutput {
		encoded, err := json.Marshal(document)
		if err != nil {
			return "", fmt.Errorf("encode metrics session JSON: %w", err)
		}
		return string(append(encoded, '\n')), nil
	}
	return renderHumanMetricsSession(document), nil
}

func renderHumanMetricsSession(document metricsSessionDocument) string {
	var output strings.Builder
	fmt.Fprintf(&output, "Factory Session %s metrics as of %s\n\n", document.FactorySessionID, formatMetricsSessionTime(document.AsOf))
	renderSessionSummaryValue(&output, "STATUS", document.Status)
	renderSessionSummaryDuration(&output, "ELAPSED WALL TIME", document.ElapsedWallTimeMillis)
	renderSessionSummaryValue(&output, "DISTINCT WORK ITEMS", strconv.Itoa(document.DistinctWorkItems))
	renderSessionSummaryValue(&output, "DISPATCH ATTEMPTS", strconv.Itoa(document.DispatchAttempts))
	renderSessionSummaryValue(&output, "WORKER SESSIONS", strconv.Itoa(document.WorkerSessions))
	renderSessionSummaryValue(&output, "MAX CONCURRENT EXECUTIONS", strconv.Itoa(document.MaxConcurrentExecutions))
	renderSessionSummaryDuration(&output, "SUMMED EXECUTION TIME", document.SummedExecutionTimeMillis)
	renderSessionSummaryDuration(&output, "SUMMED QUEUE TIME", document.SummedQueueTimeMillis)
	renderSessionSummaryValue(&output, "RETRIES", strconv.Itoa(document.Retries))
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "ATTEMPT OUTCOMES     COUNT")
	renderSessionOutcome(&output, "Accepted", document.AttemptOutcomes.Accepted)
	renderSessionOutcome(&output, "Continued", document.AttemptOutcomes.Continued)
	renderSessionOutcome(&output, "Rejected", document.AttemptOutcomes.Rejected)
	renderSessionOutcome(&output, "Failed", document.AttemptOutcomes.Failed)
	renderSessionOutcome(&output, "Interrupted", document.AttemptOutcomes.Interrupted)
	renderSessionOutcome(&output, "Canceled", document.AttemptOutcomes.Canceled)
	if document.AttemptOutcomes.Unknown > 0 {
		renderSessionOutcome(&output, "Unknown", document.AttemptOutcomes.Unknown)
	}
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "INCOMPLETE ATTEMPTS")
	renderSessionOutcome(&output, "Queued", document.Incomplete.Queued)
	renderSessionOutcome(&output, "Running", document.Incomplete.Running)
	renderSessionOutcome(&output, "Missing outcome", document.Incomplete.MissingOutcome)
	fmt.Fprintln(&output)
	renderHumanMetricsSessionDuration(&output, "QUEUE DURATION", document.QueueDuration)
	fmt.Fprintln(&output)
	renderHumanMetricsSessionDuration(&output, "EXECUTION DURATION", document.ExecutionDuration)
	if len(document.Attempts) > 0 {
		fmt.Fprintln(&output)
		fmt.Fprintln(&output, "ATTEMPT IDENTITIES")
		fmt.Fprintln(&output, "DISPATCH                 WORK                     ATTEMPT  OUTCOME")
		for _, attempt := range document.Attempts {
			dispatchID := metricsSessionDisplayPointer(attempt.DispatchID)
			workID := metricsSessionDisplayPointer(attempt.WorkID)
			outcome := metricsSessionDisplayPointer(attempt.Outcome)
			fmt.Fprintf(&output, "%-24s %-24s %7d  %s\n", dispatchID, workID, attempt.Attempt, outcome)
		}
	}
	return output.String()
}

func renderSessionSummaryValue(output *strings.Builder, label, value string) {
	fmt.Fprintf(output, "%-30s %s\n", label, value)
}

func renderSessionSummaryDuration(output *strings.Builder, label string, value *int64) {
	if value == nil {
		renderSessionSummaryValue(output, label, "unavailable")
		return
	}
	renderSessionSummaryValue(output, label, formatMetricsSessionDuration(*value))
}

func renderSessionOutcome(output *strings.Builder, label string, value int) {
	fmt.Fprintf(output, "%-22s %d\n", label, value)
}

func renderHumanMetricsSessionDuration(output *strings.Builder, label string, duration metricsSessionDuration) {
	fmt.Fprintf(output, "%s     SAMPLES  EXCLUDED  P50   P95\n", label)
	fmt.Fprintf(output, "%-16s %7d %8d  %-5s %-5s\n", duration.Unit, duration.Samples, duration.Excluded,
		formatMetricsSessionOptionalDuration(duration.P50), formatMetricsSessionOptionalDuration(duration.P95))
}

func formatMetricsSessionTime(value *time.Time) string {
	if value == nil {
		return "unavailable"
	}
	return value.UTC().Format(time.RFC3339)
}

func formatMetricsSessionOptionalDuration(value *int64) string {
	if value == nil {
		return "unavailable"
	}
	return formatMetricsSessionDuration(*value)
}

func formatMetricsSessionDuration(milliseconds int64) string {
	if milliseconds < 1000 {
		return fmt.Sprintf("%dms", milliseconds)
	}
	seconds := milliseconds / 1000
	minutes := seconds / 60
	seconds %= 60
	hours := minutes / 60
	minutes %= 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %02ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

func metricsSessionDisplayPointer(value *string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "unavailable"
	}
	return *value
}
