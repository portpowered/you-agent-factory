package internal

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type invocationScheduleFactory struct {
	factory.Factory
	schedules     automations.InvocationScheduleService
	factoryDir    string
	factoryConfig *interfaces.FactoryConfig
	runtimeConfig interfaces.RuntimeConfigLookup
	ctxMu         sync.RWMutex
	ctx           context.Context
	recoveryMu    sync.Mutex
	recovered     bool
}

func attachInvocationScheduleFactory(ctx context.Context, automation automations.Service, instance factory.HostedInstance) {
	schedules, ok := automation.(automations.InvocationScheduleService)
	bundle, bundleOK := instance.(*factoryhost.Bundle)
	if !ok || !bundleOK || bundle == nil || bundle.Factory == nil || bundle.RuntimeCfg == nil {
		return
	}
	bundle.Factory = &invocationScheduleFactory{
		Factory: bundle.Factory, schedules: schedules,
		factoryDir: bundle.RuntimeCfg.FactoryDir(), factoryConfig: bundle.RuntimeCfg.FactoryConfig(),
		runtimeConfig: bundle.RuntimeCfg, ctx: ctx,
	}
}

func (wrapped *invocationScheduleFactory) setContext(ctx context.Context) {
	wrapped.ctxMu.Lock()
	wrapped.ctx = ctx
	wrapped.ctxMu.Unlock()
}

func (wrapped *invocationScheduleFactory) scheduleContext() context.Context {
	wrapped.ctxMu.RLock()
	defer wrapped.ctxMu.RUnlock()
	if wrapped.ctx == nil {
		return context.Background()
	}
	return wrapped.ctx
}

func (wrapped *invocationScheduleFactory) runtimeService() factory.Service {
	service, _ := wrapped.Factory.(factory.Service)
	return service
}

func (wrapped *invocationScheduleFactory) ControlPause(ctx context.Context, request factory.PauseRequest) (factory.PauseResult, error) {
	return wrapped.runtimeService().ControlPause(ctx, request)
}

func (wrapped *invocationScheduleFactory) ControlResume(ctx context.Context, request factory.ResumeRequest) (factory.ResumeResult, error) {
	return wrapped.runtimeService().ControlResume(ctx, request)
}

func (wrapped *invocationScheduleFactory) ControlTerminate(ctx context.Context, request factory.TerminateRequest) (factory.TerminateResult, error) {
	return wrapped.runtimeService().ControlTerminate(ctx, request)
}

func (wrapped *invocationScheduleFactory) ControlWaitToComplete(request factory.WaitToCompleteRequest) factory.WaitToCompleteResult {
	return wrapped.runtimeService().ControlWaitToComplete(request)
}

func (wrapped *invocationScheduleFactory) ControlMoveWork(ctx context.Context, request factory.MoveWorkRequest) (factory.MoveWorkResult, error) {
	return wrapped.runtimeService().ControlMoveWork(ctx, request)
}

func (wrapped *invocationScheduleFactory) Observe(ctx context.Context, request factory.ObserveRequest) (factory.ObserveResult, error) {
	return wrapped.runtimeService().Observe(ctx, request)
}

func (wrapped *invocationScheduleFactory) PlanDispatch(ctx context.Context, request factory.PlanDispatchRequest) (factory.PlanDispatchResult, error) {
	return wrapped.runtimeService().PlanDispatch(ctx, request)
}

func (wrapped *invocationScheduleFactory) AcceptDispatchResult(ctx context.Context, request factory.AcceptDispatchResultRequest) (factory.AcceptDispatchResultResult, error) {
	return wrapped.runtimeService().AcceptDispatchResult(ctx, request)
}

func (wrapped *invocationScheduleFactory) CaptureCheckpoint(ctx context.Context, request factory.CaptureCheckpointRequest) (factory.CaptureCheckpointResult, error) {
	return wrapped.runtimeService().CaptureCheckpoint(ctx, request)
}

func (wrapped *invocationScheduleFactory) LoadCheckpoint(ctx context.Context, request factory.LoadCheckpointRequest) (factory.LoadCheckpointResult, error) {
	return wrapped.runtimeService().LoadCheckpoint(ctx, request)
}

func (wrapped *invocationScheduleFactory) RestoreCheckpoint(ctx context.Context, request factory.RestoreCheckpointRequest) (factory.RestoreCheckpointResult, error) {
	return wrapped.runtimeService().RestoreCheckpoint(ctx, request)
}

func (wrapped *invocationScheduleFactory) SubmitWorkRequest(
	ctx context.Context,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	prepared, err := wrapped.schedules.PrepareInvocationSchedules(wrapped.scheduleContext(), automations.InvocationScheduleRequest{
		FactoryDir: wrapped.factoryDir, FactoryConfig: wrapped.factoryConfig,
		RuntimeConfig: wrapped.runtimeConfig, WorkRequest: request,
		Submitter:      wrapped.submitScheduledWork,
		Observe:        wrapped.observeInvocationSchedule,
		FailController: wrapped.failInvocationScheduleController,
	})
	if err != nil {
		return work.WorkRequestSubmitResult{}, &interfaces.RequestValidationError{Message: err.Error()}
	}
	request = annotateInvocationScheduleRequest(request, wrapped.factoryConfig)

	result, err := wrapped.Factory.SubmitWorkRequest(ctx, request)
	if err != nil {
		prepared.Abort()
		return work.WorkRequestSubmitResult{}, err
	}
	prepared.Commit(result)
	return result, nil
}

func (wrapped *invocationScheduleFactory) submitScheduledWork(
	ctx context.Context,
	request work.WorkRequest,
) error {
	_, err := wrapped.Factory.SubmitWorkRequest(ctx, request)
	return err
}

func (wrapped *invocationScheduleFactory) failInvocationScheduleController(
	ctx context.Context,
	workID string,
) error {
	_, err := wrapped.Factory.MoveWork(
		ctx, workID, "failed", work.WorkStateChangeSourceCascadingFailure,
		"invocation-schedule-failure-"+workID,
	)
	return err
}

// recoverInvocationSchedules reconstructs duration jobs from the durable tags
// on active controller Work. Canonical Work remains authoritative: recovery
// continues the largest recorded sequence and never repeats trigger-at-start.
func (wrapped *invocationScheduleFactory) recoverInvocationSchedules(ctx context.Context) error {
	wrapped.recoveryMu.Lock()
	defer wrapped.recoveryMu.Unlock()
	if wrapped.recovered {
		return nil
	}

	snapshot, err := wrapped.Factory.GetEngineStateSnapshot(ctx)
	if err != nil {
		return err
	}
	if snapshot == nil {
		wrapped.recovered = true
		return nil
	}

	stateTypes := configuredPlaceStateTypes(wrapped.factoryConfig)
	maxSequences := make(map[string]int64)
	for _, token := range snapshot.Marking.Tokens {
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
		prepared, prepareErr := wrapped.schedules.PrepareInvocationSchedules(ctx, automations.InvocationScheduleRequest{
			FactoryDir: wrapped.factoryDir, FactoryConfig: wrapped.factoryConfig,
			RuntimeConfig: wrapped.runtimeConfig, WorkRequest: request,
			ResumeSequence: maxSequences[traceID], SuppressTriggerAtStart: true,
			Submitter: wrapped.submitScheduledWork, Observe: wrapped.observeInvocationSchedule,
			FailController: wrapped.failInvocationScheduleController,
		})
		if prepareErr != nil {
			return prepareErr
		}
		prepared.Commit(work.WorkRequestSubmitResult{
			Accepted: true, RequestID: token.Color.RequestID, WorkID: token.Color.WorkID,
			TraceID: traceID, Name: token.Color.Name, WorkTypeName: token.Color.WorkTypeID,
		})
	}
	wrapped.recovered = true
	return nil
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

func (wrapped *invocationScheduleFactory) observeInvocationSchedule(
	ctx context.Context,
	request automations.InvocationScheduleObservationRequest,
) (automations.InvocationScheduleObservation, error) {
	snapshot, err := wrapped.Factory.GetEngineStateSnapshot(ctx)
	if err != nil {
		return automations.InvocationScheduleObservation{}, err
	}
	if snapshot == nil {
		return automations.InvocationScheduleObservation{}, nil
	}
	stateTypes := configuredPlaceStateTypes(wrapped.factoryConfig)
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
	for _, dispatch := range snapshot.Dispatches {
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

var _ factory.Factory = (*invocationScheduleFactory)(nil)
var _ factory.Service = (*invocationScheduleFactory)(nil)
