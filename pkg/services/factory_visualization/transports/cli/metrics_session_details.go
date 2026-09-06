package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type metricsSessionWorkerDocument struct {
	Worker            string                      `json:"worker"`
	WorkerIdentity    string                      `json:"worker_identity"`
	WorkerSessionID   *string                     `json:"worker_session_id"`
	WorkerSessionIDs  []string                    `json:"worker_session_ids"`
	Sessions          int                         `json:"sessions"`
	DispatchIDs       []string                    `json:"dispatch_ids"`
	WorkIDs           []string                    `json:"work_ids"`
	WorkIdentity      string                      `json:"work_identity"`
	Attempts          int                         `json:"attempts"`
	Provider          *string                     `json:"provider"`
	Model             *string                     `json:"model"`
	ProviderIdentity  string                      `json:"provider_identity"`
	ModelIdentity     string                      `json:"model_identity"`
	AttemptOutcomes   metricsSessionOutcomeCounts `json:"attempt_outcomes"`
	QueueDuration     metricsSessionDuration      `json:"queue_duration"`
	ExecutionDuration metricsSessionDuration      `json:"execution_duration"`
	Cost              *metricsSessionCostDocument `json:"cost,omitempty"`
}

type metricsSessionDispatchDocument struct {
	DispatchID              *string                     `json:"dispatch_id"`
	DispatchIdentity        string                      `json:"dispatch_identity"`
	WorkIDs                 []string                    `json:"work_ids"`
	Worker                  string                      `json:"worker"`
	WorkerIdentity          string                      `json:"worker_identity"`
	WorkerSessionID         *string                     `json:"worker_session_id"`
	Provider                *string                     `json:"provider"`
	Model                   *string                     `json:"model"`
	ProviderIdentity        string                      `json:"provider_identity"`
	ModelIdentity           string                      `json:"model_identity"`
	Workstation             *string                     `json:"workstation"`
	Attempt                 int                         `json:"attempt"`
	AttemptIdentity         string                      `json:"attempt_identity"`
	RetryOfDispatchID       *string                     `json:"retry_of_dispatch_id"`
	Status                  string                      `json:"status"`
	Outcome                 *string                     `json:"outcome"`
	QueueDurationMillis     *int64                      `json:"queue_duration_ms"`
	ExecutionDurationMillis *int64                      `json:"execution_duration_ms"`
	Cost                    *metricsSessionCostDocument `json:"cost,omitempty"`
}

type metricsSessionCostDocument struct {
	KnownCost             *string                          `json:"known_cost"`
	Currency              string                           `json:"currency"`
	Status                string                           `json:"status"`
	Coverage              generatedclient.CostsCoverage    `json:"coverage"`
	TokenTotals           generatedclient.CostsTokenTotals `json:"token_totals"`
	UnpricedDispatchCount int                              `json:"unpriced_dispatch_count"`
	PriceSources          []string                         `json:"price_sources"`
}

type metricsSessionUsageFacts struct {
	Provider              string
	Model                 string
	ProviderConflict      bool
	ModelConflict         bool
	WorkerSessionID       string
	WorkerSessionConflict bool
	WorkIDs               map[string]struct{}
}

type metricsSessionAttemptFacts struct {
	DispatchID              *string
	WorkIDs                 []string
	WorkerSessionID         *string
	Worker                  *string
	Provider                *string
	Model                   *string
	Workstation             *string
	Attempt                 int
	RetryOfDispatchID       *string
	Status                  string
	Outcome                 *string
	QueueDurationMillis     *int64
	ExecutionDurationMillis *int64
}

type metricsSessionDetailAccumulator struct {
	workerSessionIDs   map[string]struct{}
	worker             string
	workerConflict     bool
	provider           string
	model              string
	providerConflict   bool
	modelConflict      bool
	dispatchIDs        map[string]struct{}
	workIDs            map[string]struct{}
	queueDurations     []int64
	executionDurations []int64
	outcomes           metricsSessionOutcomeCounts
	attempts           int
}

type metricsSessionCostIndex struct {
	byDispatch      map[string][]generatedclient.CostsLineItem
	byWorkerSession map[string][]generatedclient.CostsLineItem
	unknownDispatch []generatedclient.CostsLineItem
	unknownWorker   []generatedclient.CostsLineItem
}

func addMetricsSessionDetails(
	document *metricsSessionDocument,
	usageRows []generatedclient.MetricsUsageRow,
	costReport *generatedclient.CostsReport,
	byWorker bool,
	byDispatch bool,
) {
	usage := metricsSessionUsageFactsByDispatch(usageRows)
	facts := make([]metricsSessionAttemptFacts, 0, len(document.Attempts))
	for _, attempt := range document.Attempts {
		facts = append(facts, metricsSessionAttemptFactsFor(attempt, usage))
	}
	costIndex := newMetricsSessionCostIndex(costReport)
	if byWorker {
		document.ByWorker = buildMetricsSessionWorkerDetails(facts, costIndex, costReport)
	}
	if byDispatch {
		document.ByDispatch = buildMetricsSessionDispatchDetails(facts, costIndex, costReport)
	}
}

func metricsSessionUsageFactsByDispatch(rows []generatedclient.MetricsUsageRow) map[string]metricsSessionUsageFacts {
	result := make(map[string]metricsSessionUsageFacts)
	for _, row := range rows {
		dispatchID := metricStringFromAPI(row.DispatchId)
		if dispatchID == "" {
			continue
		}
		facts := result[dispatchID]
		if facts.WorkIDs == nil {
			facts.WorkIDs = make(map[string]struct{})
		}
		mergeMetricsSessionIdentity(&facts.Provider, &facts.ProviderConflict, metricStringFromAPI(row.Provider))
		mergeMetricsSessionIdentity(&facts.Model, &facts.ModelConflict, metricStringFromAPI(row.Model))
		workerSessionID := metricStringFromAPI(row.WorkerSessionId)
		if workerSessionID != "" {
			if facts.WorkerSessionConflict {
				// Keep the conflict marker while retaining other usage facts.
			} else if facts.WorkerSessionID == "" {
				facts.WorkerSessionID = workerSessionID
			} else if facts.WorkerSessionID != workerSessionID {
				facts.WorkerSessionID = ""
				facts.WorkerSessionConflict = true
			}
		}
		if workID := metricStringFromAPI(row.WorkId); workID != "" {
			facts.WorkIDs[workID] = struct{}{}
		}
		result[dispatchID] = facts
	}
	return result
}

func mergeMetricsSessionIdentity(value *string, conflict *bool, candidate string) {
	if candidate == "" || *conflict {
		return
	}
	if *value == "" {
		*value = candidate
		return
	}
	if *value != candidate {
		*conflict = true
		*value = ""
	}
}

func metricsSessionAttemptFactsFor(
	attempt metricsSessionAttemptDocument,
	usage map[string]metricsSessionUsageFacts,
) metricsSessionAttemptFacts {
	facts := metricsSessionAttemptFacts{
		DispatchID:              attempt.DispatchID,
		WorkIDs:                 append([]string(nil), attempt.WorkIDs...),
		WorkerSessionID:         attempt.WorkerSessionID,
		Worker:                  attempt.Worker,
		Provider:                attempt.Provider,
		Model:                   attempt.Model,
		Workstation:             attempt.Workstation,
		Attempt:                 attempt.Attempt,
		RetryOfDispatchID:       attempt.RetryOfDispatchID,
		Status:                  attempt.Status,
		Outcome:                 attempt.Outcome,
		QueueDurationMillis:     attempt.QueueDurationMillis,
		ExecutionDurationMillis: attempt.ExecutionDurationMillis,
	}
	if attempt.DispatchID == nil {
		return facts
	}
	usageFacts, ok := usage[*attempt.DispatchID]
	if !ok {
		return facts
	}
	if usageFacts.ProviderConflict {
		facts.Provider = nil
	} else if facts.Provider == nil {
		facts.Provider = optionalMetricsSessionString(usageFacts.Provider)
	} else if usageFacts.Provider != "" && *facts.Provider != usageFacts.Provider {
		facts.Provider = nil
	}
	if usageFacts.ModelConflict {
		facts.Model = nil
	} else if facts.Model == nil {
		facts.Model = optionalMetricsSessionString(usageFacts.Model)
	} else if usageFacts.Model != "" && *facts.Model != usageFacts.Model {
		facts.Model = nil
	}
	if usageFacts.WorkerSessionConflict {
		facts.WorkerSessionID = nil
	} else if facts.WorkerSessionID == nil {
		facts.WorkerSessionID = optionalMetricsSessionString(usageFacts.WorkerSessionID)
	} else if usageFacts.WorkerSessionID != "" && *facts.WorkerSessionID != usageFacts.WorkerSessionID {
		facts.WorkerSessionID = nil
	}
	facts.WorkIDs = mergeMetricsSessionWorkIDs(facts.WorkIDs, usageFacts.WorkIDs)
	return facts
}

func mergeMetricsSessionWorkIDs(values []string, additional map[string]struct{}) []string {
	seen := make(map[string]struct{}, len(values)+len(additional))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			seen[value] = struct{}{}
		}
	}
	for value := range additional {
		if value = strings.TrimSpace(value); value != "" {
			seen[value] = struct{}{}
		}
	}
	merged := make([]string, 0, len(seen))
	for value := range seen {
		merged = append(merged, value)
	}
	sort.Strings(merged)
	return merged
}

func newMetricsSessionCostIndex(report *generatedclient.CostsReport) *metricsSessionCostIndex {
	if report == nil {
		return nil
	}
	index := &metricsSessionCostIndex{
		byDispatch:      make(map[string][]generatedclient.CostsLineItem),
		byWorkerSession: make(map[string][]generatedclient.CostsLineItem),
	}
	for _, item := range report.LineItems {
		dispatchID := metricStringFromAPI(item.DispatchId)
		workerSessionID := metricStringFromAPI(item.WorkerSessionId)
		if dispatchID == "" {
			index.unknownDispatch = append(index.unknownDispatch, item)
		} else {
			index.byDispatch[dispatchID] = append(index.byDispatch[dispatchID], item)
		}
		if workerSessionID == "" {
			index.unknownWorker = append(index.unknownWorker, item)
		} else {
			index.byWorkerSession[workerSessionID] = append(index.byWorkerSession[workerSessionID], item)
		}
	}
	return index
}

func buildMetricsSessionWorkerDetails(
	facts []metricsSessionAttemptFacts,
	costIndex *metricsSessionCostIndex,
	costReport *generatedclient.CostsReport,
) []metricsSessionWorkerDocument {
	workerGroupsBySession := metricsSessionWorkerGroupsBySession(facts)
	groups := make(map[string]*metricsSessionDetailAccumulator)
	for _, fact := range facts {
		key := metricsSessionWorkerGroupKey(fact, workerGroupsBySession)
		group := groups[key]
		if group == nil {
			group = newMetricsSessionDetailAccumulator()
			groups[key] = group
		}
		group.add(fact)
	}
	costItemsByWorker := metricsSessionCostItemsByWorkerGroup(costIndex, workerGroupsBySession)
	for key := range costItemsByWorker {
		if _, exists := groups[key]; !exists {
			groups[key] = newMetricsSessionDetailAccumulator()
		}
		for _, item := range costItemsByWorker[key] {
			groups[key].addWorkerSessionID(metricStringFromAPI(item.WorkerSessionId))
		}
	}
	keys := sortedMetricsSessionDetailKeys(groups)
	result := make([]metricsSessionWorkerDocument, 0, len(keys))
	for _, key := range keys {
		group := groups[key]
		items := costItemsByWorker[key]
		provider := metricsSessionIdentityWithItems(group.providerValue(), items, true)
		model := metricsSessionIdentityWithItems(group.modelValue(), items, false)
		worker := group.workerValue()
		if key != "unavailable" {
			worker = optionalMetricsSessionString(key)
		}
		workIDs := sortedMetricsSessionSet(group.workIDs)
		workerSessionIDs := sortedMetricsSessionSet(group.workerSessionIDs)
		row := metricsSessionWorkerDocument{
			Worker:            metricsSessionDisplayIdentity(worker),
			WorkerIdentity:    metricsSessionIdentityLabel(worker != nil),
			WorkerSessionID:   group.workerSessionID(),
			WorkerSessionIDs:  workerSessionIDs,
			Sessions:          len(workerSessionIDs),
			DispatchIDs:       sortedMetricsSessionSet(group.dispatchIDs),
			WorkIDs:           workIDs,
			WorkIdentity:      metricsSessionIdentityLabel(len(workIDs) > 0),
			Attempts:          group.attempts,
			Provider:          provider,
			Model:             model,
			ProviderIdentity:  metricsSessionIdentityLabel(provider != nil),
			ModelIdentity:     metricsSessionIdentityLabel(model != nil),
			AttemptOutcomes:   group.outcomes,
			QueueDuration:     metricsSessionDurationFor(group.queueDurations, group.attempts),
			ExecutionDuration: metricsSessionDurationFor(group.executionDurations, group.attempts),
		}
		if costReport != nil {
			row.Cost = metricsSessionCostDocumentFor(items, string(costReport.Currency))
		}
		result = append(result, row)
	}
	return result
}

type metricsSessionWorkerGroup struct {
	key      string
	conflict bool
}

func metricsSessionWorkerGroupsBySession(facts []metricsSessionAttemptFacts) map[string]metricsSessionWorkerGroup {
	groups := make(map[string]metricsSessionWorkerGroup)
	for _, fact := range facts {
		sessionID := metricStringFromAPI(fact.WorkerSessionID)
		worker := metricStringFromAPI(fact.Worker)
		if sessionID == "" || worker == "" {
			continue
		}
		group, exists := groups[sessionID]
		if !exists {
			groups[sessionID] = metricsSessionWorkerGroup{key: worker}
			continue
		}
		if group.conflict || group.key == worker {
			continue
		}
		group.key = ""
		group.conflict = true
		groups[sessionID] = group
	}
	return groups
}

func metricsSessionWorkerGroupKey(
	fact metricsSessionAttemptFacts,
	groupsBySession map[string]metricsSessionWorkerGroup,
) string {
	sessionID := metricStringFromAPI(fact.WorkerSessionID)
	if group, ok := groupsBySession[sessionID]; ok && group.conflict {
		return "unavailable"
	}
	worker := metricStringFromAPI(fact.Worker)
	if worker == "" {
		if group, ok := groupsBySession[sessionID]; ok {
			worker = group.key
		}
	}
	if worker == "" {
		return "unavailable"
	}
	return worker
}

func metricsSessionCostItemsByWorkerGroup(
	costIndex *metricsSessionCostIndex,
	groupsBySession map[string]metricsSessionWorkerGroup,
) map[string][]generatedclient.CostsLineItem {
	itemsByGroup := make(map[string][]generatedclient.CostsLineItem)
	if costIndex == nil {
		return itemsByGroup
	}
	for workerSessionID, items := range costIndex.byWorkerSession {
		key := "unavailable"
		if group, ok := groupsBySession[workerSessionID]; ok && !group.conflict && group.key != "" {
			key = group.key
		}
		itemsByGroup[key] = append(itemsByGroup[key], items...)
	}
	if len(costIndex.unknownWorker) > 0 {
		itemsByGroup["unavailable"] = append(itemsByGroup["unavailable"], costIndex.unknownWorker...)
	}
	return itemsByGroup
}

type metricsSessionAttemptState struct {
	key                 string
	dispatchID          string
	retryOfDispatchID   string
	workIDs             map[string]struct{}
	workerSessionIDs    map[string]struct{}
	worker              string
	workerConflict      bool
	provider            string
	model               string
	providerConflict    bool
	modelConflict       bool
	workstation         string
	workstationConflict bool
	queuedAt            *time.Time
	startedAt           *time.Time
	terminalAt          *time.Time
	executionDuration   *int64
	outcome             string
	status              string
	terminal            bool
	firstEventIndex     int
	attempt             int
}

func (state *metricsSessionAttemptState) identityValue(value string, conflict bool) *string {
	if conflict {
		return nil
	}
	return optionalMetricsSessionString(value)
}

func (state *metricsSessionAttemptState) observeWorker(value *string) {
	value = normalizedMetricsSessionPointer(value)
	if value == nil || state.workerConflict {
		return
	}
	if state.worker == "" {
		state.worker = *value
		return
	}
	if state.worker != *value {
		state.worker = ""
		state.workerConflict = true
	}
}

func (state *metricsSessionAttemptState) observeWorkstation(value *string) {
	value = normalizedMetricsSessionPointer(value)
	if value == nil || state.workstationConflict {
		return
	}
	if state.workstation == "" {
		state.workstation = *value
		return
	}
	if state.workstation != *value {
		state.workstation = ""
		state.workstationConflict = true
	}
}

type metricsSessionReducer struct {
	sessionID          string
	workIDs            map[string]struct{}
	workerSessions     map[string]struct{}
	workstationNames   map[string]string
	workstationWorkers map[string]string
	attempts           map[string]*metricsSessionAttemptState
	startedAt          *time.Time
	completedAt        *time.Time
	asOf               *time.Time
	status             string
	eventIndex         int
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
		sessionID:          strings.TrimSpace(sessionID),
		workIDs:            make(map[string]struct{}),
		workerSessions:     make(map[string]struct{}),
		workstationNames:   make(map[string]string),
		workstationWorkers: make(map[string]string),
		attempts:           make(map[string]*metricsSessionAttemptState),
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
	if err := reducer.consumeSessionEvent(event); err != nil {
		return err
	}
	return reducer.consumeDispatchEvent(event)
}

func (reducer *metricsSessionReducer) consumeSessionEvent(event factoryapi.FactoryEvent) error {
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
	case factoryapi.FactoryEventTypeInitialStructureRequest:
		return reducer.consumeInitialStructure(event)
	}
	return nil
}

func (reducer *metricsSessionReducer) consumeDispatchEvent(event factoryapi.FactoryEvent) error {
	switch event.Type {
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
	case factoryapi.FactoryEventTypeModelRequest:
		return reducer.consumeModelRequest(event)
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

func buildMetricsSessionDispatchDetails(
	facts []metricsSessionAttemptFacts,
	costIndex *metricsSessionCostIndex,
	costReport *generatedclient.CostsReport,
) []metricsSessionDispatchDocument {
	result := make([]metricsSessionDispatchDocument, 0, len(facts))
	seen := make(map[string]struct{}, len(facts))
	for _, fact := range facts {
		key := metricStringFromAPI(fact.DispatchID)
		if key == "" {
			key = "unavailable"
		}
		seen[key] = struct{}{}
		result = append(result, metricsSessionDispatchDocumentFor(
			fact, metricsSessionCostItemsForDispatch(costIndex, key), costReport))
	}
	if costIndex != nil {
		result = appendMetricsSessionCostOnlyDispatches(result, seen, costIndex, costReport)
	}
	return result
}

func metricsSessionDispatchDocumentFor(
	fact metricsSessionAttemptFacts,
	items []generatedclient.CostsLineItem,
	costReport *generatedclient.CostsReport,
) metricsSessionDispatchDocument {
	provider := metricsSessionIdentityWithItems(fact.Provider, items, true)
	model := metricsSessionIdentityWithItems(fact.Model, items, false)
	workIDs := mergeMetricsSessionWorkIDs(fact.WorkIDs, metricsSessionItemsWorkIDsSet(items))
	row := metricsSessionDispatchDocument{
		DispatchID:              fact.DispatchID,
		DispatchIdentity:        metricsSessionIdentityLabel(fact.DispatchID != nil),
		WorkIDs:                 workIDs,
		Worker:                  metricsSessionDisplayIdentity(fact.Worker),
		WorkerIdentity:          metricsSessionIdentityLabel(fact.Worker != nil),
		WorkerSessionID:         fact.WorkerSessionID,
		Provider:                provider,
		Model:                   model,
		ProviderIdentity:        metricsSessionIdentityLabel(provider != nil),
		ModelIdentity:           metricsSessionIdentityLabel(model != nil),
		Workstation:             fact.Workstation,
		Attempt:                 fact.Attempt,
		AttemptIdentity:         metricsSessionIdentityLabel(fact.Attempt > 0),
		RetryOfDispatchID:       fact.RetryOfDispatchID,
		Status:                  fact.Status,
		Outcome:                 fact.Outcome,
		QueueDurationMillis:     fact.QueueDurationMillis,
		ExecutionDurationMillis: fact.ExecutionDurationMillis,
	}
	if costReport != nil {
		row.Cost = metricsSessionCostDocumentFor(items, string(costReport.Currency))
	}
	return row
}

func appendMetricsSessionCostOnlyDispatches(
	result []metricsSessionDispatchDocument,
	seen map[string]struct{},
	index *metricsSessionCostIndex,
	report *generatedclient.CostsReport,
) []metricsSessionDispatchDocument {
	keys := make([]string, 0, len(index.byDispatch))
	for key := range index.byDispatch {
		if _, exists := seen[key]; !exists {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		id := key
		result = append(result, metricsSessionDispatchDocumentFor(
			metricsSessionAttemptFacts{DispatchID: &id, Status: "UNKNOWN"},
			index.byDispatch[key], report))
	}
	if _, exists := seen["unavailable"]; !exists && len(index.unknownDispatch) > 0 {
		result = append(result, metricsSessionDispatchDocumentFor(
			metricsSessionAttemptFacts{Status: "UNKNOWN"}, index.unknownDispatch, report))
	}
	return result
}

func newMetricsSessionDetailAccumulator() *metricsSessionDetailAccumulator {
	return &metricsSessionDetailAccumulator{
		workerSessionIDs: make(map[string]struct{}),
		dispatchIDs:      make(map[string]struct{}),
		workIDs:          make(map[string]struct{}),
	}
}

func (group *metricsSessionDetailAccumulator) add(fact metricsSessionAttemptFacts) {
	group.attempts++
	if fact.DispatchID != nil {
		group.dispatchIDs[*fact.DispatchID] = struct{}{}
	}
	group.addWorkerSessionID(metricStringFromAPI(fact.WorkerSessionID))
	for _, workID := range fact.WorkIDs {
		if workID != "" {
			group.workIDs[workID] = struct{}{}
		}
	}
	mergeMetricsSessionIdentity(&group.provider, &group.providerConflict, metricStringFromAPI(fact.Provider))
	mergeMetricsSessionIdentity(&group.model, &group.modelConflict, metricStringFromAPI(fact.Model))
	mergeMetricsSessionIdentity(&group.worker, &group.workerConflict, metricStringFromAPI(fact.Worker))
	if fact.QueueDurationMillis != nil {
		group.queueDurations = append(group.queueDurations, *fact.QueueDurationMillis)
	}
	if fact.ExecutionDurationMillis != nil {
		group.executionDurations = append(group.executionDurations, *fact.ExecutionDurationMillis)
	}
	if fact.Outcome == nil {
		group.outcomes.Unknown++
	} else {
		incrementMetricsSessionOutcome(&group.outcomes, *fact.Outcome)
	}
}

func (group *metricsSessionDetailAccumulator) addWorkerSessionID(workerSessionID string) {
	if workerSessionID != "" {
		group.workerSessionIDs[workerSessionID] = struct{}{}
	}
}

func (group *metricsSessionDetailAccumulator) workerSessionID() *string {
	if len(group.workerSessionIDs) != 1 {
		return nil
	}
	for value := range group.workerSessionIDs {
		return optionalMetricsSessionString(value)
	}
	return nil
}

func (group *metricsSessionDetailAccumulator) workerValue() *string {
	if group.workerConflict {
		return nil
	}
	return optionalMetricsSessionString(group.worker)
}

func (group *metricsSessionDetailAccumulator) providerValue() *string {
	if group.providerConflict {
		return nil
	}
	return optionalMetricsSessionString(group.provider)
}

func (group *metricsSessionDetailAccumulator) modelValue() *string {
	if group.modelConflict {
		return nil
	}
	return optionalMetricsSessionString(group.model)
}

func metricsSessionCostDocumentFor(items []generatedclient.CostsLineItem, currency string) *metricsSessionCostDocument {
	document := &metricsSessionCostDocument{
		Currency:     currency,
		Status:       "NO_USAGE",
		PriceSources: []string{},
	}
	if len(items) == 0 {
		return document
	}
	var priced, unpriced int
	pairs := make(map[string]struct{})
	dispatches := make(map[string]struct{})
	priceSources := make(map[string]struct{})
	for index, item := range items {
		provider := metricStringFromAPI(item.Provider)
		model := metricStringFromAPI(item.Model)
		pairs[provider+"/"+model] = struct{}{}
		status := strings.ToUpper(strings.TrimSpace(string(item.Status)))
		if status == "PRICED" {
			priced++
			if item.PriceSource != nil && strings.TrimSpace(string(*item.PriceSource)) != "" {
				priceSources[string(*item.PriceSource)] = struct{}{}
			}
		} else {
			unpriced++
			dispatchID := metricStringFromAPI(item.DispatchId)
			if dispatchID == "" {
				dispatchID = fmt.Sprintf("unknown-%d", index)
			}
			dispatches[dispatchID] = struct{}{}
		}
	}
	document.Coverage = generatedclient.CostsCoverage{
		EncounteredProviderModels: len(pairs),
		EncounteredRows:           len(items),
		PricedProviderModels:      metricsSessionPricedPairCount(items),
		PricedRows:                priced,
		UnpricedProviderModels:    metricsSessionUnpricedPairCount(items),
		UnpricedRows:              unpriced,
	}
	document.KnownCost = metricsSessionExactCost(items)
	document.TokenTotals = metricsSessionCostTokenTotals(items)
	document.UnpricedDispatchCount = len(dispatches)
	document.PriceSources = sortedMetricsSessionSet(priceSources)
	switch {
	case priced == len(items):
		document.Status = "PRICED"
	case priced > 0:
		document.Status = "PARTIAL"
	default:
		document.Status = "UNPRICED"
	}
	return document
}

func metricsSessionPricedPairCount(items []generatedclient.CostsLineItem) int {
	byPair := make(map[string]bool)
	for _, item := range items {
		key := metricStringFromAPI(item.Provider) + "/" + metricStringFromAPI(item.Model)
		if _, exists := byPair[key]; !exists {
			byPair[key] = true
		}
		if strings.ToUpper(strings.TrimSpace(string(item.Status))) != "PRICED" {
			byPair[key] = false
		}
	}
	count := 0
	for _, priced := range byPair {
		if priced {
			count++
		}
	}
	return count
}

func metricsSessionUnpricedPairCount(items []generatedclient.CostsLineItem) int {
	byPair := make(map[string]bool)
	for _, item := range items {
		key := metricStringFromAPI(item.Provider) + "/" + metricStringFromAPI(item.Model)
		if _, exists := byPair[key]; !exists {
			byPair[key] = false
		}
		if strings.ToUpper(strings.TrimSpace(string(item.Status))) != "PRICED" {
			byPair[key] = true
		}
	}
	count := 0
	for _, unpriced := range byPair {
		if unpriced {
			count++
		}
	}
	return count
}

func metricsSessionCostTokenTotals(items []generatedclient.CostsLineItem) generatedclient.CostsTokenTotals {
	return generatedclient.CostsTokenTotals{
		TotalTokens: metricsSessionSumLineItems(items, func(item generatedclient.CostsLineItem) *int64 {
			return sumMetricTokenClasses(item.InputTokens, item.OutputTokens)
		}),
		InputTokens:           metricsSessionSumLineItems(items, func(item generatedclient.CostsLineItem) *int64 { return item.InputTokens }),
		CachedInputTokens:     metricsSessionSumLineItems(items, func(item generatedclient.CostsLineItem) *int64 { return item.CachedInputTokens }),
		OutputTokens:          metricsSessionSumLineItems(items, func(item generatedclient.CostsLineItem) *int64 { return item.OutputTokens }),
		ReasoningOutputTokens: metricsSessionSumLineItems(items, func(item generatedclient.CostsLineItem) *int64 { return item.ReasoningOutputTokens }),
	}
}

func metricsSessionSumLineItems(items []generatedclient.CostsLineItem, value func(generatedclient.CostsLineItem) *int64) *int64 {
	if len(items) == 0 {
		return nil
	}
	var total int64
	for _, item := range items {
		measurement := value(item)
		if measurement == nil {
			return nil
		}
		total += *measurement
	}
	return &total
}

func sumMetricTokenClasses(input, output *int64) *int64 {
	if input == nil || output == nil {
		return nil
	}
	total := *input + *output
	return &total
}
