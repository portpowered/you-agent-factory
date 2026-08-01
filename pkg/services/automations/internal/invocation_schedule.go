package internal

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-co-op/gocron/v2"
	"go.uber.org/zap"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

const (
	invocationIntervalMinimum = time.Second
	invocationIntervalMaximum = 7 * 24 * time.Hour

	timeActualAtTag       = "agent_factory.time.actual_at"
	timeSequenceTag       = "agent_factory.time.sequence"
	timeTriggerOutcomeTag = "agent_factory.time.trigger_outcome"
	timeIntervalTag       = "agent_factory.time.interval"

	triggerOutcomeScheduled = "SCHEDULED"
	triggerOutcomeSkipped   = "SKIPPED_OVERLAP"
)

type preparedInvocationSchedule struct {
	owner             *Service
	ctx               context.Context
	scheduler         gocron.Scheduler
	request           automations.InvocationScheduleRequest
	workstation       interfaces.FactoryWorkstationConfig
	controller        work.Work
	executionWorkType string
	executionState    string
	skippedState      string
	every             time.Duration
	triggerAtStart    bool
	maxFailures       int

	mu                sync.Mutex
	controllerWorkID  string
	controllerTraceID string
	nextNominal       time.Time
	sequence          int64
	disabled          bool
}

// PrepareInvocationSchedules validates and constructs inert duration jobs for
// authored CRON intervals that reference normalized invocation arguments.
func (s *Service) PrepareInvocationSchedules(
	ctx context.Context,
	request automations.InvocationScheduleRequest,
) (automations.PreparedInvocationSchedules, error) {
	if request.FactoryConfig == nil || request.RuntimeConfig == nil || request.Submitter == nil {
		return automations.PreparedInvocationSchedules{}, nil
	}

	prepared := make([]*preparedInvocationSchedule, 0)
	for _, workstation := range request.FactoryConfig.Workstations {
		if workstation.Kind != interfaces.WorkstationKindCron || workstation.Cron == nil {
			continue
		}
		parameter, dynamic := invocationParameterReference(workstation.Cron.Every)
		if !dynamic {
			continue
		}
		for _, controller := range request.WorkRequest.Works {
			if !cronControllerInput(workstation, controller.WorkTypeID) {
				continue
			}
			value, ok := invocationArgumentValue(controller.InvocationArguments, parameter)
			if !ok {
				continue
			}
			every, err := parseInvocationInterval(value)
			if err != nil {
				abortPreparedInvocationSchedules(prepared)
				return automations.PreparedInvocationSchedules{}, fmt.Errorf("workstation %q: %w", workstation.Name, err)
			}
			executionWorkType, executionState, err := scheduledExecutionOutput(workstation, controller.WorkTypeID)
			if err != nil {
				abortPreparedInvocationSchedules(prepared)
				return automations.PreparedInvocationSchedules{}, err
			}
			skippedState, err := terminalStateNamed(request.FactoryConfig, executionWorkType, "skipped")
			if err != nil {
				abortPreparedInvocationSchedules(prepared)
				return automations.PreparedInvocationSchedules{}, err
			}
			triggerAtStart, err := invocationBool(controller.InvocationArguments, "triggerAtStart", workstation.Cron.TriggerAtStart)
			if err != nil {
				abortPreparedInvocationSchedules(prepared)
				return automations.PreparedInvocationSchedules{}, err
			}
			maxFailures, err := invocationNonNegativeInt(controller.InvocationArguments, "maxConsecutiveFailures")
			if err != nil {
				abortPreparedInvocationSchedules(prepared)
				return automations.PreparedInvocationSchedules{}, err
			}

			scheduler, err := gocron.NewScheduler(
				gocron.WithClock(s.supervisorClock()),
				gocron.WithLocation(time.UTC),
			)
			if err != nil {
				abortPreparedInvocationSchedules(prepared)
				return automations.PreparedInvocationSchedules{}, fmt.Errorf("construct invocation interval scheduler: %w", err)
			}
			entry := &preparedInvocationSchedule{
				owner: s, ctx: ctx, scheduler: scheduler, request: request,
				workstation: workstation, controller: cloneScheduledWork(controller),
				executionWorkType: executionWorkType, executionState: executionState,
				skippedState: skippedState, every: every, triggerAtStart: triggerAtStart,
				maxFailures: maxFailures, sequence: request.ResumeSequence,
			}
			if request.SuppressTriggerAtStart {
				entry.triggerAtStart = false
			}
			_, err = scheduler.NewJob(
				gocron.DurationJob(every),
				gocron.NewTask(entry.fire),
				gocron.WithSingletonMode(gocron.LimitModeReschedule),
			)
			if err != nil {
				_ = scheduler.Shutdown()
				abortPreparedInvocationSchedules(prepared)
				return automations.PreparedInvocationSchedules{}, fmt.Errorf("register invocation interval %q: %w", value, err)
			}
			prepared = append(prepared, entry)
		}
	}

	return automations.PreparedInvocationSchedules{
		CommitFunc: func(result work.WorkRequestSubmitResult) {
			for _, entry := range prepared {
				entry.commit(result)
			}
		},
		AbortFunc: func() { abortPreparedInvocationSchedules(prepared) },
	}, nil
}

func cronControllerInput(workstation interfaces.FactoryWorkstationConfig, workTypeName string) bool {
	for _, input := range workstation.Inputs {
		if input.WorkTypeName == workTypeName && workTypeName != interfaces.SystemTimeWorkTypeID {
			return true
		}
	}
	return false
}

func (entry *preparedInvocationSchedule) commit(result work.WorkRequestSubmitResult) {
	if entry == nil || entry.scheduler == nil || !result.Accepted {
		return
	}
	entry.mu.Lock()
	entry.controllerWorkID = result.WorkID
	entry.controllerTraceID = result.TraceID
	entry.nextNominal = entry.owner.supervisorClock().Now().UTC().Add(entry.every)
	entry.mu.Unlock()
	entry.scheduler.Start()
	go func() {
		<-entry.ctx.Done()
		_ = entry.scheduler.Shutdown()
	}()
	if entry.triggerAtStart {
		entry.fireAt(entry.owner.supervisorClock().Now().UTC())
	}
}

func (entry *preparedInvocationSchedule) fire() {
	entry.mu.Lock()
	nominal := entry.nextNominal
	entry.nextNominal = entry.nextNominal.Add(entry.every)
	entry.mu.Unlock()
	entry.fireAt(nominal)
}

func (entry *preparedInvocationSchedule) fireAt(nominal time.Time) {
	entry.mu.Lock()
	if entry.disabled {
		entry.mu.Unlock()
		return
	}
	controllerWorkID := entry.controllerWorkID
	controllerTraceID := entry.controllerTraceID
	entry.sequence++
	sequence := entry.sequence
	entry.mu.Unlock()

	observation := automations.InvocationScheduleObservation{ControllerActive: true}
	if entry.request.Observe != nil {
		var err error
		observation, err = entry.request.Observe(entry.ctx, automations.InvocationScheduleObservationRequest{
			ControllerWorkID: controllerWorkID, ControllerTraceID: controllerTraceID,
			ExecutionWorkType: entry.executionWorkType,
		})
		if err != nil {
			entry.owner.logger().Error("invocation interval observation failed", zap.Error(err))
			return
		}
	}
	failureCeilingReached := entry.maxFailures > 0 && observation.ConsecutiveFailures >= entry.maxFailures
	if !observation.ControllerActive || failureCeilingReached {
		entry.mu.Lock()
		entry.disabled = true
		entry.mu.Unlock()
		if failureCeilingReached && entry.request.FailController != nil {
			if err := entry.request.FailController(entry.ctx, controllerWorkID); err != nil && entry.ctx.Err() == nil {
				entry.owner.logger().Error("invocation interval failure ceiling transition failed",
					zap.String("workstation", entry.workstation.Name), zap.Error(err))
			}
		}
		return
	}

	actual := entry.owner.supervisorClock().Now().UTC()
	state := entry.executionState
	outcome := triggerOutcomeScheduled
	if observation.ExecutionActive {
		state = entry.skippedState
		outcome = triggerOutcomeSkipped
	}
	workID := fmt.Sprintf("%s/scheduled/%06d", controllerWorkID, sequence)
	tags := cloneStringMap(entry.controller.Tags)
	if tags == nil {
		tags = make(map[string]string)
	}
	tags[interfaces.TimeWorkTagKeyCronWorkstation] = entry.workstation.Name
	tags[interfaces.TimeWorkTagKeyNominalAt] = nominal.UTC().Format(time.RFC3339Nano)
	tags[timeActualAtTag] = actual.Format(time.RFC3339Nano)
	tags[timeSequenceTag] = strconv.FormatInt(sequence, 10)
	tags[timeTriggerOutcomeTag] = outcome
	tags[timeIntervalTag] = entry.every.String()

	request := work.WorkRequest{
		RequestID: "request-" + strings.NewReplacer("/", "-", " ", "-").Replace(workID),
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name: "scheduled:" + entry.workstation.Name, WorkID: workID,
			WorkTypeID: entry.executionWorkType, State: state,
			CurrentChainingTraceID: controllerTraceID, TraceID: controllerTraceID,
			Content: cloneWorkContent(entry.controller.Content), Tags: tags,
			InvocationArguments: work.CloneInvocationArguments(entry.controller.InvocationArguments),
		}},
	}
	if err := entry.request.Submitter(entry.ctx, request); err != nil && entry.ctx.Err() == nil {
		entry.owner.logger().Error("invocation interval trigger failed",
			zap.String("workstation", entry.workstation.Name),
			zap.Int64("sequence", sequence), zap.Error(err))
	}
}

func invocationParameterReference(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 4 || !strings.HasPrefix(trimmed, "${") || !strings.HasSuffix(trimmed, "}") {
		return "", false
	}
	name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "${"), "}"))
	return name, name != ""
}

func invocationArgumentValue(args *work.InvocationArguments, name string) (string, bool) {
	if args == nil {
		return "", false
	}
	argument, ok := args.Arguments[name]
	if !ok || len(argument.Values) != 1 {
		return "", false
	}
	return strings.TrimSpace(argument.Values[0]), true
}

func parseInvocationInterval(value string) (time.Duration, error) {
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || parsed < invocationIntervalMinimum || parsed > invocationIntervalMaximum {
		return 0, fmt.Errorf("interval %q must be a duration from 1s through 168h", value)
	}
	return parsed, nil
}

func scheduledExecutionOutput(workstation interfaces.FactoryWorkstationConfig, controllerWorkType string) (string, string, error) {
	for _, output := range workstation.Outputs {
		if output.WorkTypeName != "" && output.WorkTypeName != controllerWorkType {
			return output.WorkTypeName, output.StateName, nil
		}
	}
	return "", "", fmt.Errorf("cron workstation %q requires a non-controller scheduled output", workstation.Name)
}

func terminalStateNamed(config *interfaces.FactoryConfig, workTypeName, stateName string) (string, error) {
	for _, workType := range config.WorkTypes {
		if workType.Name != workTypeName {
			continue
		}
		for _, state := range workType.States {
			if state.Name == stateName && state.Type == interfaces.StateTypeTerminal {
				return state.Name, nil
			}
		}
	}
	return "", fmt.Errorf("scheduled work type %q requires terminal state %q", workTypeName, stateName)
}

func invocationBool(args *work.InvocationArguments, name string, fallback bool) (bool, error) {
	value, ok := invocationArgumentValue(args, name)
	if !ok || value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("argument %q must be true or false", name)
	}
	return parsed, nil
}

func invocationNonNegativeInt(args *work.InvocationArguments, name string) (int, error) {
	value, ok := invocationArgumentValue(args, name)
	if !ok || value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("argument %q must be a non-negative integer", name)
	}
	return parsed, nil
}

func abortPreparedInvocationSchedules(entries []*preparedInvocationSchedule) {
	for _, entry := range entries {
		if entry != nil && entry.scheduler != nil {
			_ = entry.scheduler.Shutdown()
		}
	}
}

func cloneScheduledWork(value work.Work) work.Work {
	value.Content = cloneWorkContent(value.Content)
	value.Tags = cloneStringMap(value.Tags)
	value.InvocationArguments = work.CloneInvocationArguments(value.InvocationArguments)
	return value
}

func cloneWorkContent(value []work.WorkContentPart) []work.WorkContentPart {
	if value == nil {
		return nil
	}
	cloned := make([]work.WorkContentPart, len(value))
	copy(cloned, value)
	for index := range cloned {
		cloned[index].JSON = append([]byte(nil), value[index].JSON...)
	}
	return cloned
}

func cloneStringMap(value map[string]string) map[string]string {
	if value == nil {
		return nil
	}
	cloned := make(map[string]string, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}
