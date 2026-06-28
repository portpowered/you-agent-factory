package factorysessionexecution

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/runtimepersist"
	jsstore "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/store"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
)

// PersistedRuntimeSessionState is a JSON-serializable durable runtime session snapshot
// used to reload terminal or recoverable JavaScript runtime sessions across CLI invocations.
type PersistedRuntimeSessionState struct {
	Session                   SessionReadResult
	Result                    ResultReadResult
	Dispatches                []DispatchSummary
	DispatchJavaScript        map[string]DispatchJavaScriptProjection
	DispatchStatusTransitions map[string][]DispatchStatus
	Artifacts                 []ArtifactSummary
	Events                    []json.RawMessage
	RuntimeRecords            []workflowruntime.RuntimeRecord
	CheckpointSummary         *jsstore.CheckpointSummary
	StartRequest              *StartRequest
	ResolvedSource            ResolvedSource
	SourceContent             string
}

func persistedSnapshotFromRuntimeState(state runtimeSessionState) PersistedRuntimeSessionState {
	snapshot := PersistedRuntimeSessionState{
		Session:           cloneSessionRead(state.session),
		Result:            cloneResultRead(state.result),
		Dispatches:        cloneDispatchSummaries(state.dispatches),
		Artifacts:         cloneArtifactSummaries(state.artifacts),
		RuntimeRecords:    cloneRuntimeRecords(state.runtimeRecords),
		CheckpointSummary: cloneCheckpointSummary(state.checkpointSummary),
		StartRequest:      cloneStartRequestPtr(state.startRequest),
		ResolvedSource:    state.resolvedSource,
		SourceContent:     state.sourceContent,
	}
	if len(state.dispatchJavaScript) > 0 {
		snapshot.DispatchJavaScript = cloneDispatchJavaScriptProjections(state.dispatchJavaScript)
	}
	if len(state.dispatchStatusTransitions) > 0 {
		snapshot.DispatchStatusTransitions = cloneDispatchStatusTransitions(state.dispatchStatusTransitions)
	}
	if len(state.events) > 0 {
		snapshot.Events = make([]json.RawMessage, len(state.events))
		for i, event := range state.events {
			snapshot.Events[i] = append(json.RawMessage(nil), event...)
		}
	}
	return snapshot
}

func runtimeStateFromPersistedSnapshot(snapshot PersistedRuntimeSessionState) runtimeSessionState {
	state := runtimeSessionState{
		session:           cloneSessionRead(snapshot.Session),
		result:            cloneResultRead(snapshot.Result),
		dispatches:        cloneDispatchSummaries(snapshot.Dispatches),
		artifacts:         cloneArtifactSummaries(snapshot.Artifacts),
		runtimeRecords:    cloneRuntimeRecords(snapshot.RuntimeRecords),
		checkpointSummary: cloneCheckpointSummary(snapshot.CheckpointSummary),
		startRequest:      cloneStartRequestPtr(snapshot.StartRequest),
		resolvedSource:    snapshot.ResolvedSource,
		sourceContent:     snapshot.SourceContent,
	}
	if len(snapshot.DispatchJavaScript) > 0 {
		state.dispatchJavaScript = cloneDispatchJavaScriptProjections(snapshot.DispatchJavaScript)
	}
	if len(snapshot.DispatchStatusTransitions) > 0 {
		state.dispatchStatusTransitions = cloneDispatchStatusTransitions(snapshot.DispatchStatusTransitions)
	}
	if len(snapshot.Events) > 0 {
		state.events = make([]json.RawMessage, len(snapshot.Events))
		for i, event := range snapshot.Events {
			state.events[i] = append(json.RawMessage(nil), event...)
		}
	}
	return state
}

func (s *JavaScriptRuntimeService) persistTerminalSessionState(state runtimeSessionState) error {
	return s.persistSessionSnapshot(state)
}

func (s *JavaScriptRuntimeService) persistSessionSnapshot(state runtimeSessionState) error {
	if s.sessionPersistDir == "" {
		return nil
	}
	sessionID := strings.TrimSpace(state.session.SessionID)
	if sessionID == "" {
		return nil
	}
	if !shouldPersistSessionSnapshot(state) {
		return nil
	}
	snapshot := persistedSnapshotFromRuntimeState(state)
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal durable session snapshot: %w", err)
	}
	if err := runtimepersist.SaveBytes(s.sessionPersistDir, sessionID, encoded); err != nil {
		return fmt.Errorf("persist durable session snapshot: %w", err)
	}
	return nil
}

func shouldPersistSessionSnapshot(state runtimeSessionState) bool {
	if IsTerminalLifecycleStatus(state.session.Status) {
		return true
	}
	if state.session.Status == LifecycleStatusInterrupted && state.checkpointSummary != nil {
		return true
	}
	return false
}

func cloneStartRequest(req StartRequest) *StartRequest {
	cloned := req
	cloned.Source = cloneStartSource(req.Source)
	cloned.Args = cloneArgs(req.Args)
	cloned.RequestedPolicy = cloneArgs(req.RequestedPolicy)
	if req.Orchestrator != nil {
		orchestrator := *req.Orchestrator
		cloned.Orchestrator = &orchestrator
	}
	if req.Runtime != nil {
		runtime := *req.Runtime
		cloned.Runtime = &runtime
	}
	if req.Wait != nil {
		wait := *req.Wait
		cloned.Wait = &wait
	}
	return &cloned
}

func cloneStartRequestPtr(req *StartRequest) *StartRequest {
	if req == nil {
		return nil
	}
	return cloneStartRequest(*req)
}

func cloneStartSource(source Source) Source {
	cloned := source
	if source.InlineWorkflow != nil {
		inline := *source.InlineWorkflow
		inline.Metadata = cloneStringStringMap(source.InlineWorkflow.Metadata)
		cloned.InlineWorkflow = &inline
	}
	if len(source.FactoryInline) > 0 {
		cloned.FactoryInline = append(json.RawMessage(nil), source.FactoryInline...)
	}
	return cloned
}

func cloneStringStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
