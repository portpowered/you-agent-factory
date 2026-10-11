package runtime

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func (f *factoryImpl) submitInvocationWorkRequest(
	ctx context.Context,
	scheduleCtx context.Context,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	// Admission uses the command context; committed schedules use the runtime
	// context so they outlive the command and stop with the selected runtime.
	prepared, err := f.cfg.prepareSchedules(scheduleCtx, automations.InvocationScheduleRequest{
		RuntimeID:  f.cfg.runtimeID,
		FactoryDir: f.cfg.requestFactoryDirectory, FactoryConfig: factoryConfigFromFactoryConfig(f.cfg),
		RuntimeConfig: f.scheduleRuntimeConfig(), WorkRequest: request,
		Submitter: f.submitScheduledWork, Observe: f.observeInvocationSchedule,
		FailController: f.failInvocationScheduleController,
	})
	if err != nil {
		return work.WorkRequestSubmitResult{}, &interfaces.RequestValidationError{Message: err.Error()}
	}
	request = annotateInvocationScheduleRequest(request, factoryConfigFromFactoryConfig(f.cfg))

	result, err := f.engine.SubmitWorkRequest(ctx, request)
	if err != nil {
		prepared.Abort()
		return work.WorkRequestSubmitResult{}, err
	}
	prepared.Commit(result)
	return result, nil
}

func (f *factoryImpl) submitScheduledWork(
	ctx context.Context,
	request work.WorkRequest,
) error {
	_, err := f.engine.SubmitWorkRequest(ctx, request)
	return err
}

func (f *factoryImpl) failInvocationScheduleController(
	ctx context.Context,
	workID string,
) error {
	_, err := f.MoveWork(
		ctx, workID, "failed", work.WorkStateChangeSourceCascadingFailure,
		"invocation-schedule-failure-"+workID,
	)
	return err
}

// RecoverInvocationSchedules reconstructs duration jobs from the durable tags
// on active controller Work. Canonical Work remains authoritative: recovery
// continues the largest recorded sequence and never repeats trigger-at-start.
func (f *factoryImpl) RecoverInvocationSchedules(ctx context.Context) error {
	f.scheduleCtxMu.Lock()
	f.scheduleCtx = ctx
	f.scheduleCtxMu.Unlock()

	snapshot, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		return err
	}
	return f.recoverInvocationScheduleSnapshot(ctx, snapshot)
}

func (f *factoryImpl) recoverInvocationScheduleSnapshot(ctx context.Context, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) error {
	f.recoveryMu.Lock()
	defer f.recoveryMu.Unlock()
	if f.recovered {
		return nil
	}
	if snapshot == nil {
		f.recovered = true
		return nil
	}

	stateTypes := configuredPlaceStateTypes(factoryConfigFromFactoryConfig(f.cfg))
	maxSequences := maximumScheduleSequences(snapshot.Marking.Tokens)

	recoveredControllers := make(map[string]struct{})
	for _, token := range snapshot.Marking.Tokens {
		if token == nil || token.Color.Tags[automations.InvocationScheduleEveryTag] == "" {
			continue
		}
		if stateType := stateTypes[token.PlaceID]; stateType == interfaces.StateTypeTerminal || stateType == interfaces.StateTypeFailed {
			continue
		}
		if _, duplicate := recoveredControllers[token.Color.WorkID]; duplicate {
			continue
		}
		recoveredControllers[token.Color.WorkID] = struct{}{}
		traceID := token.Color.CurrentChainingTraceID
		if traceID == "" {
			traceID = token.Color.TraceID
		}
		if err := f.recoverInvocationScheduleController(ctx, token, traceID, maxSequences[traceID]); err != nil {
			return err
		}

	}
	f.recovered = true
	return nil
}

func maximumScheduleSequences(tokens map[string]*factorytoken.Token) map[string]int64 {
	maxSequences := make(map[string]int64)
	for _, token := range tokens {
		if token == nil {
			continue
		}
		traceID := token.Color.CurrentChainingTraceID
		if traceID == "" {
			traceID = token.Color.TraceID
		}
		if sequence, ok := scheduleSequence(token.Color.Tags); ok && sequence > maxSequences[traceID] {
			maxSequences[traceID] = sequence
		}
	}

	return maxSequences
}

func firstNonEmptyScheduleTag(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func annotateInvocationScheduleRequest(request work.WorkRequest, config *interfaces.FactoryConfig) work.WorkRequest {
	request.Works = append([]work.Work(nil), request.Works...)
	if config == nil {
		return request
	}
	for index := range request.Works {
		item := request.Works[index]
		for _, workstation := range config.Workstations {
			if workstation.Kind != interfaces.WorkstationKindCron || workstation.Cron == nil ||
				!workstationConsumes(workstation, item.WorkTypeID) {
				continue
			}
			parameter := exactInvocationReference(workstation.Cron.Every)
			value := invocationValue(item.InvocationArguments, parameter)
			if parameter == "" || value == "" {
				continue
			}
			if item.Tags == nil {
				item.Tags = make(map[string]string)
			} else {
				item.Tags = cloneScheduleTags(item.Tags)
			}
			if every, err := time.ParseDuration(value); err == nil {
				item.Tags[automations.InvocationScheduleEveryTag] = every.String()
			}
			item.Tags[automations.InvocationScheduleWorkstationTag] = workstation.Name
			item.Tags[automations.InvocationScheduleTriggerAtStartTag] = firstScheduleValue(item.InvocationArguments, "triggerAtStart", "false")
			item.Tags[automations.InvocationScheduleMaxConsecutiveFailuresTag] = firstScheduleValue(item.InvocationArguments, "maxConsecutiveFailures", "0")
		}
		request.Works[index] = item
	}
	return request
}

func workstationConsumes(workstation interfaces.FactoryWorkstationConfig, workTypeName string) bool {
	for _, input := range workstation.Inputs {
		if input.WorkTypeName == workTypeName {
			return true
		}
	}
	return false
}

func exactInvocationReference(value string) string {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "${") || !strings.HasSuffix(trimmed, "}") {
		return ""
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "${"), "}"))
}

func invocationValue(arguments *work.InvocationArguments, name string) string {
	if arguments == nil || name == "" {
		return ""
	}
	argument, ok := arguments.Arguments[name]
	if !ok || len(argument.Values) != 1 {
		return ""
	}
	return strings.TrimSpace(argument.Values[0])
}

func firstScheduleValue(arguments *work.InvocationArguments, name, fallback string) string {
	if value := invocationValue(arguments, name); value != "" {
		return value
	}
	return fallback
}

func cloneScheduleTags(tags map[string]string) map[string]string {
	cloned := make(map[string]string, len(tags)+4)
	for key, value := range tags {
		cloned[key] = value
	}
	return cloned
}

func (f *factoryImpl) observeInvocationSchedule(
	ctx context.Context,
	request automations.InvocationScheduleObservationRequest,
) (automations.InvocationScheduleObservation, error) {
	snapshot, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		return automations.InvocationScheduleObservation{}, err
	}
	if snapshot == nil {
		return automations.InvocationScheduleObservation{}, nil
	}
	stateTypes := configuredPlaceStateTypes(factoryConfigFromFactoryConfig(f.cfg))
	observation := automations.InvocationScheduleObservation{}
	failuresBySequence := make(map[int64]interfaces.StateType)

	for _, token := range snapshot.Marking.Tokens {
		if token == nil || token.Color.CurrentChainingTraceID != request.ControllerTraceID {
			continue
		}
		stateType := stateTypes[token.PlaceID]
		if token.Color.WorkID == request.ControllerWorkID && token.Color.WorkTypeID != request.ExecutionWorkType {
			observation.ControllerActive = stateType != interfaces.StateTypeTerminal && stateType != interfaces.StateTypeFailed
		}
		if token.Color.WorkTypeID != request.ExecutionWorkType {
			continue
		}
		if stateType != interfaces.StateTypeTerminal && stateType != interfaces.StateTypeFailed {
			observation.ExecutionActive = true
		}
		if sequence, ok := scheduleSequence(token.Color.Tags); ok &&
			(stateType == interfaces.StateTypeTerminal || stateType == interfaces.StateTypeFailed) {
			failuresBySequence[sequence] = stateType
		}
	}
	observeScheduledDispatches(snapshot.Dispatches, request, &observation)
	observation.ConsecutiveFailures = consecutiveScheduleFailures(failuresBySequence)
	return observation, nil
}

func configuredPlaceStateTypes(config *interfaces.FactoryConfig) map[string]interfaces.StateType {
	result := make(map[string]interfaces.StateType)
	if config == nil {
		return result
	}
	for _, workType := range config.WorkTypes {
		workTypeIDs := []string{workType.ID, workType.Name}
		for _, state := range workType.States {
			stateIDs := []string{state.ID, state.Name}
			for _, workTypeID := range workTypeIDs {
				for _, stateID := range stateIDs {
					if workTypeID != "" && stateID != "" {
						result[workTypeID+":"+stateID] = state.Type
					}
				}
			}
		}
	}
	return result
}

func scheduleSequence(tags map[string]string) (int64, bool) {
	value := strings.TrimSpace(tags["agent_factory.time.sequence"])
	if value == "" {
		return 0, false
	}
	sequence, err := strconv.ParseInt(value, 10, 64)
	return sequence, err == nil
}

func consecutiveScheduleFailures(outcomes map[int64]interfaces.StateType) int {
	sequences := make([]int64, 0, len(outcomes))
	for sequence := range outcomes {
		sequences = append(sequences, sequence)
	}
	sort.Slice(sequences, func(i, j int) bool { return sequences[i] > sequences[j] })
	failures := 0
	for _, sequence := range sequences {
		switch outcomes[sequence] {
		case interfaces.StateTypeFailed:
			failures++
		case interfaces.StateTypeTerminal:
			return failures
		}
	}
	return failures
}

func (f *factoryImpl) scheduleContext() context.Context {
	f.scheduleCtxMu.RLock()
	defer f.scheduleCtxMu.RUnlock()
	return f.scheduleCtx
}

func (f *factoryImpl) scheduleRuntimeConfig() interfaces.RuntimeConfigLookup {
	return f.cfg.scheduleConfig
}

func (f *factoryImpl) recoverInvocationScheduleController(ctx context.Context, token *factorytoken.Token, traceID string, sequence int64) error {
	arguments := &work.InvocationArguments{Arguments: map[string]work.InvocationArgument{
		"every": {Values: []string{token.Color.Tags[automations.InvocationScheduleEveryTag]}},
		"triggerAtStart": {Values: []string{firstNonEmptyScheduleTag(
			token.Color.Tags[automations.InvocationScheduleTriggerAtStartTag], "false",
		)}},
		"maxConsecutiveFailures": {Values: []string{firstNonEmptyScheduleTag(
			token.Color.Tags[automations.InvocationScheduleMaxConsecutiveFailuresTag], "0",
		)}},
	}}
	request := work.WorkRequest{
		RequestID: token.Color.RequestID, CurrentChainingTraceID: traceID,
		Type: work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name: token.Color.Name, WorkID: token.Color.WorkID, RequestID: token.Color.RequestID,
			WorkTypeID: token.Color.WorkTypeID, CurrentChainingTraceID: traceID, TraceID: traceID,
			Content: token.Color.Content, Payload: token.Color.Payload,
			Tags: cloneScheduleTags(token.Color.Tags), InvocationArguments: arguments,
		}},
	}
	prepared, prepareErr := f.cfg.prepareSchedules(ctx, automations.InvocationScheduleRequest{
		RuntimeID:  f.cfg.runtimeID,
		FactoryDir: f.cfg.requestFactoryDirectory, FactoryConfig: factoryConfigFromFactoryConfig(f.cfg),
		RuntimeConfig: f.scheduleRuntimeConfig(), WorkRequest: request,
		ResumeSequence: sequence, SuppressTriggerAtStart: true,
		Submitter: f.submitScheduledWork, Observe: f.observeInvocationSchedule,
		FailController: f.failInvocationScheduleController,
	})
	if prepareErr != nil {
		return prepareErr
	}
	prepared.Commit(work.WorkRequestSubmitResult{
		Accepted: true, RequestID: token.Color.RequestID, WorkID: token.Color.WorkID,
		TraceID: traceID, Name: token.Color.Name, WorkTypeName: token.Color.WorkTypeID,
	})
	return nil
}

func observeScheduledDispatches(dispatches map[string]*interfaces.DispatchEntry, request automations.InvocationScheduleObservationRequest, observation *automations.InvocationScheduleObservation) {
	for _, dispatch := range dispatches {
		if dispatch == nil {
			continue
		}
		for _, token := range dispatch.ConsumedTokens {
			if token.Color.CurrentChainingTraceID != request.ControllerTraceID {
				continue
			}
			if token.Color.WorkID == request.ControllerWorkID {
				observation.ControllerActive = true
			}
			if token.Color.WorkTypeID == request.ExecutionWorkType {
				observation.ExecutionActive = true
			}
		}
	}
}
