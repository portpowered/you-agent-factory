package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type watchAcceptedEvent struct {
	eventType factoryapi.FactoryEventType
	sequence  int
	signature []byte
}

type watchWorkObservation struct {
	workTypeName string
	state        string
	terminal     bool
}

// watchReducer projects the canonical Factory stream into Work transitions.
// It deliberately owns only the short-lived state required by one watch
// invocation; the Factory Event ledger remains the source of truth.
type watchReducer struct {
	sessionID  string
	hasLast    bool
	last       int
	lastID     string
	accepted   map[string]watchAcceptedEvent
	stateTypes map[string]map[string]factoryapi.WorkStateType
	cohort     map[string]watchWorkObservation
}

func newWatchReducer(sessionID string) *watchReducer {
	return &watchReducer{
		sessionID:  sessionID,
		accepted:   make(map[string]watchAcceptedEvent),
		stateTypes: make(map[string]map[string]factoryapi.WorkStateType),
		cohort:     make(map[string]watchWorkObservation),
	}
}

// Accept accepts one canonical event and returns a transition only for a
// WORK_STATE_CHANGE event. The final bool reports whether finite mode may
// stop after this event has been applied.
func (r *watchReducer) Accept(event factoryapi.FactoryEvent) (WatchTransition, bool, bool, error) {
	if r == nil {
		return WatchTransition{}, false, false, fmt.Errorf("work watch reducer is required")
	}
	if strings.TrimSpace(event.Id) == "" {
		return WatchTransition{}, false, false, fmt.Errorf("work watch event id is required")
	}
	if event.Context.Sequence < 0 {
		return WatchTransition{}, false, false, fmt.Errorf("work watch event %q has negative context.sequence %d", event.Id, event.Context.Sequence)
	}
	replay, err := r.acceptOrdering(event)
	if err != nil {
		return WatchTransition{}, false, false, err
	}
	if replay {
		return WatchTransition{}, false, r.Completed(), nil
	}
	transition, produced, err := r.applyEvent(event)
	if err != nil {
		return WatchTransition{}, false, false, err
	}
	return transition, produced, r.Completed(), nil
}

// acceptOrdering validates and records event's canonical ordering position.
// It reports true when event is an exact replay or a refreshed non-projecting
// enrichment at a strictly later canonical position.
func (r *watchReducer) acceptOrdering(event factoryapi.FactoryEvent) (bool, error) {
	signature, err := json.Marshal(event)
	if err != nil {
		return false, fmt.Errorf("fingerprint work watch event %q: %w", event.Id, err)
	}
	if prior, ok := r.accepted[event.Id]; ok {
		if prior.sequence == event.Context.Sequence && bytes.Equal(prior.signature, signature) {
			return true, nil
		}
		if prior.eventType == event.Type && isNonProjectingWatchEvent(event.Type) {
			// Provider retries can refresh a non-projecting canonical payload while
			// retaining the event identity. Accept later enrichment without emitting
			// a transition. A non-increasing position is a conflicting reuse, not a
			// provider retry.
			if !r.hasLast || event.Context.Sequence <= r.last {
				return false, fmt.Errorf(
					"work watch enrichment %q has non-increasing canonical sequence %d after %d",
					event.Id, event.Context.Sequence, r.last,
				)
			}
			r.accepted[event.Id] = watchAcceptedEvent{
				eventType: event.Type,
				sequence:  event.Context.Sequence,
				signature: append([]byte(nil), signature...),
			}
			r.last = event.Context.Sequence
			r.lastID = event.Id
			r.hasLast = true
			return true, nil
		}
		if prior.sequence != event.Context.Sequence || !bytes.Equal(prior.signature, signature) {
			return false, fmt.Errorf("work watch event id %q was reused with conflicting canonical data", event.Id)
		}
		return true, nil
	}
	if r.hasLast && event.Context.Sequence <= r.last {
		return false, fmt.Errorf(
			"work watch canonical sequence regressed or conflicted: event %q has %d after %d",
			event.Id, event.Context.Sequence, r.last,
		)
	}
	r.accepted[event.Id] = watchAcceptedEvent{
		eventType: event.Type,
		sequence:  event.Context.Sequence,
		signature: append([]byte(nil), signature...),
	}
	r.last = event.Context.Sequence
	r.lastID = event.Id
	r.hasLast = true
	return false, nil
}

// isNonProjectingWatchEvent identifies event families this reducer consumes
// only for canonical ordering. Events that update Factory or Work projection
// state remain strict because refreshed data could change the projection.
func isNonProjectingWatchEvent(eventType factoryapi.FactoryEventType) bool {
	switch eventType {
	case factoryapi.FactoryEventTypeInitialStructureRequest,
		factoryapi.FactoryEventTypeRunRequest,
		factoryapi.FactoryEventTypeFactoryChange,
		factoryapi.FactoryEventTypeWorkRequest,
		factoryapi.FactoryEventTypeWorkStateChange:
		return false
	default:
		return true
	}
}

// applyEvent projects one non-replay canonical event into reducer state and
// returns a transition only for a WORK_STATE_CHANGE event.
func (r *watchReducer) applyEvent(event factoryapi.FactoryEvent) (WatchTransition, bool, error) {
	switch event.Type {
	case factoryapi.FactoryEventTypeInitialStructureRequest:
		return WatchTransition{}, false, r.applyInitialStructureRequest(event)
	case factoryapi.FactoryEventTypeRunRequest:
		return WatchTransition{}, false, r.applyRunRequestEvent(event)
	case factoryapi.FactoryEventTypeFactoryChange:
		return WatchTransition{}, false, r.applyFactoryChangeEvent(event)
	case factoryapi.FactoryEventTypeWorkRequest:
		return WatchTransition{}, false, r.applyWorkRequestFromEvent(event)
	case factoryapi.FactoryEventTypeWorkStateChange:
		return r.applyWorkStateChangeFromEvent(event)
	}
	return WatchTransition{}, false, nil
}

func (r *watchReducer) applyInitialStructureRequest(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsInitialStructureRequestEventPayload()
	if err != nil {
		return fmt.Errorf("decode initial Factory metadata for event %q: %w", event.Id, err)
	}
	if err := r.replaceFactory(payload.Factory); err != nil {
		return fmt.Errorf("apply initial Factory metadata for event %q: %w", event.Id, err)
	}
	return nil
}

func (r *watchReducer) applyRunRequestEvent(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsRunRequestEventPayload()
	if err != nil {
		return fmt.Errorf("decode run Factory metadata for event %q: %w", event.Id, err)
	}
	if err := r.replaceFactory(payload.Factory); err != nil {
		return fmt.Errorf("apply run Factory metadata for event %q: %w", event.Id, err)
	}
	return nil
}

func (r *watchReducer) applyFactoryChangeEvent(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsFactoryChangeEventPayload()
	if err != nil {
		return fmt.Errorf("decode changed Factory metadata for event %q: %w", event.Id, err)
	}
	if err := r.replaceFactory(payload.Factory); err != nil {
		return fmt.Errorf("apply changed Factory metadata for event %q: %w", event.Id, err)
	}
	return nil
}

func (r *watchReducer) applyWorkRequestFromEvent(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsWorkRequestEventPayload()
	if err != nil {
		return fmt.Errorf("decode Work cohort event %q: %w", event.Id, err)
	}
	if err := r.applyWorkRequest(event, payload); err != nil {
		return fmt.Errorf("apply Work cohort event %q: %w", event.Id, err)
	}
	return nil
}

func (r *watchReducer) applyWorkStateChangeFromEvent(event factoryapi.FactoryEvent) (WatchTransition, bool, error) {
	payload, err := event.Payload.AsWorkStateChangeEventPayload()
	if err != nil {
		return WatchTransition{}, false, fmt.Errorf("decode Work transition event %q: %w", event.Id, err)
	}
	transition, err := r.applyWorkStateChange(event, payload)
	if err != nil {
		return WatchTransition{}, false, fmt.Errorf("apply Work transition event %q: %w", event.Id, err)
	}
	return transition, true, nil
}

// Cursor returns the last accepted canonical event identity and ordering
// position. The cursor is ephemeral and belongs only to this watch invocation;
// callers use it to resume the same stream after a transient disconnect.
func (r *watchReducer) Cursor() *watchEventCursor {
	if r == nil || !r.hasLast {
		return nil
	}
	return &watchEventCursor{EventID: r.lastID, Sequence: r.last}
}

// Completed reports whether at least one observed Work exists and every
// observed Work is in an authoritative terminal or failed state.
func (r *watchReducer) Completed() bool {
	if r == nil || len(r.cohort) == 0 {
		return false
	}
	for _, observation := range r.cohort {
		if !observation.terminal {
			return false
		}
	}
	return true
}

func (r *watchReducer) replaceFactory(factory factoryapi.Factory) error {
	r.stateTypes = make(map[string]map[string]factoryapi.WorkStateType)
	if factory.WorkTypes == nil {
		return nil
	}
	for _, workType := range *factory.WorkTypes {
		workTypeName := strings.TrimSpace(workType.Name)
		if workTypeName == "" {
			return fmt.Errorf("Factory work type name is required")
		}
		states := make(map[string]factoryapi.WorkStateType, len(workType.States))
		for _, state := range workType.States {
			stateName := strings.TrimSpace(state.Name)
			if stateName == "" {
				return fmt.Errorf("Factory work type %q has a state without a name", workTypeName)
			}
			states[stateName] = state.Type
		}
		r.stateTypes[workTypeName] = states
	}
	for workID, observation := range r.cohort {
		if observation.workTypeName == "" || observation.state == "" {
			continue
		}
		if stateType, ok := r.stateType(observation.workTypeName, observation.state); ok {
			observation.terminal = isTerminalWorkState(stateType)
			r.cohort[workID] = observation
		}
	}
	return nil
}

func (r *watchReducer) applyWorkRequest(
	event factoryapi.FactoryEvent,
	payload factoryapi.WorkRequestEventPayload,
) error {
	if payload.Works != nil {
		for _, item := range *payload.Works {
			workID := stringValue(item.WorkId)
			if workID == "" {
				return fmt.Errorf("event %q contains Work without workId", event.Id)
			}
			workTypeName := stringValue(item.WorkTypeName)
			if workTypeName == "" {
				return fmt.Errorf("event %q Work %q has no workTypeName", event.Id, workID)
			}
			observation := watchWorkObservation{workTypeName: workTypeName}
			if item.State != nil {
				if strings.TrimSpace(item.State.Name) == "" {
					return fmt.Errorf("event %q Work %q has a state without a name", event.Id, workID)
				}
				observation.state = item.State.Name
				stateType := item.State.Type
				if authoritativeType, ok := r.stateType(workTypeName, item.State.Name); ok {
					stateType = authoritativeType
				} else if item.State.Type != "" {
					r.setStateType(workTypeName, item.State.Name, item.State.Type)
				}
				observation.terminal = isTerminalWorkState(stateType)
			}
			if err := r.recordWork(workID, observation); err != nil {
				return err
			}
		}
	}
	if payload.Works == nil && event.Context.WorkIds != nil {
		for _, workID := range *event.Context.WorkIds {
			if strings.TrimSpace(workID) == "" {
				return fmt.Errorf("event %q contains an empty context workId", event.Id)
			}
			if err := r.recordWork(workID, watchWorkObservation{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *watchReducer) applyWorkStateChange(
	event factoryapi.FactoryEvent,
	payload factoryapi.WorkStateChangeEventPayload,
) (WatchTransition, error) {
	for field, value := range map[string]string{
		"workId":       payload.WorkId,
		"workTypeName": payload.WorkTypeName,
		"fromState":    payload.FromState,
		"toState":      payload.ToState,
		"source":       string(payload.Source),
	} {
		if strings.TrimSpace(value) == "" {
			return WatchTransition{}, fmt.Errorf("event %q transition %s is required", event.Id, field)
		}
	}
	if event.Context.EventTime.IsZero() {
		return WatchTransition{}, fmt.Errorf("event %q transition eventTime is required", event.Id)
	}
	stateType, ok := r.stateType(payload.WorkTypeName, payload.ToState)
	if !ok {
		return WatchTransition{}, fmt.Errorf(
			"event %q transition target state %q for work type %q has no authoritative state metadata",
			event.Id, payload.ToState, payload.WorkTypeName,
		)
	}
	observation := watchWorkObservation{
		workTypeName: payload.WorkTypeName,
		state:        payload.ToState,
		terminal:     isTerminalWorkState(stateType),
	}
	if err := r.recordWork(payload.WorkId, observation); err != nil {
		return WatchTransition{}, err
	}
	transition := WatchTransition{
		SessionID:     r.sessionID,
		EventID:       event.Id,
		Sequence:      int64(event.Context.Sequence),
		EventTime:     event.Context.EventTime,
		WorkID:        payload.WorkId,
		WorkTypeName:  payload.WorkTypeName,
		FromState:     payload.FromState,
		ToState:       payload.ToState,
		Source:        string(payload.Source),
		Terminal:      observation.terminal,
		TriggerWorkID: stringValue(payload.TriggerWorkId),
		Reason:        stringValue(payload.Reason),
	}
	return transition, nil
}

func (r *watchReducer) recordWork(workID string, observation watchWorkObservation) error {
	if strings.TrimSpace(workID) == "" {
		return fmt.Errorf("workId is required")
	}
	prior, ok := r.cohort[workID]
	if ok &&
		prior.workTypeName != "" && observation.workTypeName != "" &&
		prior.workTypeName != observation.workTypeName {
		return fmt.Errorf("Work %q changed work type from %q to %q", workID, prior.workTypeName, observation.workTypeName)
	}
	if observation.workTypeName == "" {
		observation.workTypeName = prior.workTypeName
	}
	r.cohort[workID] = observation
	return nil
}

func (r *watchReducer) setStateType(workTypeName, stateName string, stateType factoryapi.WorkStateType) {
	states := r.stateTypes[workTypeName]
	if states == nil {
		states = make(map[string]factoryapi.WorkStateType)
		r.stateTypes[workTypeName] = states
	}
	states[stateName] = stateType
}

func (r *watchReducer) stateType(workTypeName, stateName string) (factoryapi.WorkStateType, bool) {
	states := r.stateTypes[workTypeName]
	if states == nil {
		return "", false
	}
	stateType, ok := states[stateName]
	return stateType, ok && stateType != ""
}

func isTerminalWorkState(stateType factoryapi.WorkStateType) bool {
	return stateType == factoryapi.WorkStateTypeTERMINAL || stateType == factoryapi.WorkStateTypeFAILED
}
