package recordingfixtures

import (
	"context"
	"sync"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// ScriptedRuntimeLedger is an inert Recordings root-contract fake. Tests may
// preload canonical event drafts or inspect Calls; the fake deliberately does
// not reproduce Recordings event construction, normalization, or reduction.
type ScriptedRuntimeLedger struct {
	mu sync.Mutex

	Events             []factorydefinitions.FactoryEvent
	Calls              []string
	WorkRequests       []work.WorkRequestRecord
	WorkStateChanges   []work.WorkStateChangeRecord
	WorkstationResults []workers.WorkResult
	InferenceEvents    []workers.InferenceEvent
	LifecycleControls  []recordings.SessionLifecycleControlInput
	GenerationID       string
	SubscribeResult    factorydefinitions.FactoryEventStream
	SubscribeError     error
	eventRecorders     []func(factorydefinitions.FactoryEvent)
	eventTypeRecorders []func(factorydefinitions.FactoryEventType)
}

var _ recordings.RuntimeEventLedger = (*ScriptedRuntimeLedger)(nil)

func (l *ScriptedRuntimeLedger) CanonicalEvents() []factorydefinitions.FactoryEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]factorydefinitions.FactoryEvent(nil), l.Events...)
}

func (l *ScriptedRuntimeLedger) CallCount(name string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	count := 0
	for _, call := range l.Calls {
		if call == name {
			count++
		}
	}
	return count
}

func (l *ScriptedRuntimeLedger) CallsSnapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.Calls...)
}

func (l *ScriptedRuntimeLedger) Subscribe(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (factorydefinitions.FactoryEventStream, error) {
	l.recordCall("Subscribe")
	if l.SubscribeError != nil {
		return factorydefinitions.FactoryEventStream{}, l.SubscribeError
	}
	result := l.SubscribeResult
	if result.History == nil {
		result.History = l.CanonicalEvents()
	}
	if result.StreamGenerationID == "" {
		result.StreamGenerationID = l.StreamGenerationID()
	}
	return result, nil
}

func (l *ScriptedRuntimeLedger) StreamGenerationID() string {
	if l.GenerationID == "" {
		return "scripted-runtime-ledger"
	}
	return l.GenerationID
}

func (l *ScriptedRuntimeLedger) AddEventRecorder(
	recorder func(factorydefinitions.FactoryEvent),
) {
	if recorder == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.eventRecorders = append(l.eventRecorders, recorder)
}

func (l *ScriptedRuntimeLedger) AddEventTypeRecorder(
	recorder func(factorydefinitions.FactoryEventType),
) {
	if recorder == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.eventTypeRecorders = append(l.eventTypeRecorders, recorder)
}

func (l *ScriptedRuntimeLedger) AppendRecordedEvent(
	event factorydefinitions.FactoryEvent,
) {
	l.mu.Lock()
	l.Events = append(l.Events, event)
	recorders := append([]func(factorydefinitions.FactoryEvent){}, l.eventRecorders...)
	typeRecorders := append([]func(factorydefinitions.FactoryEventType){}, l.eventTypeRecorders...)
	l.mu.Unlock()
	for _, recorder := range recorders {
		recorder(event)
	}
	for _, recorder := range typeRecorders {
		recorder(event.Type)
	}
}

func (l *ScriptedRuntimeLedger) RecordRunRequest() {
	l.recordCall("RecordRunRequest")
}

func (l *ScriptedRuntimeLedger) RecordInitialStructure() {
	l.recordCall("RecordInitialStructure")
}

func (l *ScriptedRuntimeLedger) RecordWorkRequest(
	_ int,
	record work.WorkRequestRecord,
	_ time.Time,
) {
	l.mu.Lock()
	l.WorkRequests = append(l.WorkRequests, record)
	l.mu.Unlock()
	l.recordCall("RecordWorkRequest")
}

func (l *ScriptedRuntimeLedger) RecordWorkInput(
	int,
	work.SubmitRequest,
	workers.Token,
	time.Time,
) {
	l.recordCall("RecordWorkInput")
}

func (l *ScriptedRuntimeLedger) RecordWorkstationRequest(
	int,
	factorydefinitions.FactoryDispatchRecord,
	time.Time,
) {
	l.recordCall("RecordWorkstationRequest")
}

func (l *ScriptedRuntimeLedger) RecordWorkstationResponse(
	_ int,
	result workers.WorkResult,
	_ factorydefinitions.CompletedDispatch,
) {
	l.mu.Lock()
	l.WorkstationResults = append(l.WorkstationResults, result)
	l.mu.Unlock()
	l.recordCall("RecordWorkstationResponse")
}

func (l *ScriptedRuntimeLedger) RecordRunResponse(
	int,
	factorydefinitions.FactoryState,
	string,
	time.Time,
) {
	l.recordCall("RecordRunResponse")
}

func (l *ScriptedRuntimeLedger) RecordWorkStateChange(
	_ int,
	record work.WorkStateChangeRecord,
	_ time.Time,
) {
	l.mu.Lock()
	l.WorkStateChanges = append(l.WorkStateChanges, record)
	l.mu.Unlock()
	l.recordCall("RecordWorkStateChange")
}

func (l *ScriptedRuntimeLedger) RecordFactoryStateChange(
	int,
	factorydefinitions.FactoryState,
	factorydefinitions.FactoryState,
	string,
	time.Time,
) {
	l.recordCall("RecordFactoryStateChange")
}

func (l *ScriptedRuntimeLedger) RecordFactoryChange(
	int,
	factorydefinitions.FactoryChangeEventPayload,
	time.Time,
) {
	l.recordCall("RecordFactoryChange")
}

func (l *ScriptedRuntimeLedger) RecordSessionLifecycleFromFactoryConfig(
	string,
	*factorydefinitions.FactoryConfig,
	int,
	time.Time,
) {
	l.recordCall("RecordSessionLifecycleFromFactoryConfig")
}

func (l *ScriptedRuntimeLedger) RecordSessionLifecycleCompletion(
	string,
	*factorydefinitions.FactoryConfig,
	int,
	factorydefinitions.FactoryState,
	string,
	time.Time,
) {
	l.recordCall("RecordSessionLifecycleCompletion")
}

func (l *ScriptedRuntimeLedger) RecordSessionPaused(
	input recordings.SessionLifecycleControlInput,
	_ time.Time,
) {
	l.mu.Lock()
	l.LifecycleControls = append(l.LifecycleControls, input)
	l.mu.Unlock()
	l.recordCall("RecordSessionPaused")
}

func (l *ScriptedRuntimeLedger) RecordSessionResumed(
	input recordings.SessionLifecycleControlInput,
	_ time.Time,
) {
	l.mu.Lock()
	l.LifecycleControls = append(l.LifecycleControls, input)
	l.mu.Unlock()
	l.recordCall("RecordSessionResumed")
}

func (l *ScriptedRuntimeLedger) RecordSessionLifecycleControl(
	input recordings.SessionLifecycleControlInput,
	_ time.Time,
) {
	l.mu.Lock()
	l.LifecycleControls = append(l.LifecycleControls, input)
	l.mu.Unlock()
	l.recordCall("RecordSessionLifecycleControl")
}

func (l *ScriptedRuntimeLedger) SetFactoryRunnerOverride(string) {
	l.recordCall("SetFactoryRunnerOverride")
}

func (l *ScriptedRuntimeLedger) SetInitialStructureFactory(
	*factorydefinitions.FactorySnapshot,
) {
	l.recordCall("SetInitialStructureFactory")
}

func (l *ScriptedRuntimeLedger) RecordInferenceEvent(event workers.InferenceEvent) {
	l.mu.Lock()
	l.InferenceEvents = append(l.InferenceEvents, event)
	l.mu.Unlock()
	l.recordCall("RecordInferenceEvent")
}

func (l *ScriptedRuntimeLedger) RecordModelEvent(workers.ModelEvent) {
	l.recordCall("RecordModelEvent")
}

func (l *ScriptedRuntimeLedger) RecordScriptEvent(workers.ScriptEvent) {
	l.recordCall("RecordScriptEvent")
}

func (l *ScriptedRuntimeLedger) RecordAgentRunEvent(workers.AgentRunResponseEvent) {
	l.recordCall("RecordAgentRunEvent")
}

func (l *ScriptedRuntimeLedger) recordCall(name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Calls = append(l.Calls, name)
}
