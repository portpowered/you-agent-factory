package replay

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// SideEffects replays recorded provider and script-command behavior through
// the normal worker side-effect interfaces.
type SideEffects struct {
	mu      sync.Mutex
	records []sideEffectRecord
}

type sideEffectRecord struct {
	dispatch      replayDispatch
	completion    *replayCompletion
	hasCompletion bool
	usedBy        string
}

// NewSideEffects builds replay-aware side-effect substitutes from an artifact.
func NewSideEffects(artifact *interfaces.ReplayArtifact) (*SideEffects, error) {
	eventLog, err := reduceReplayEvents(artifact)
	if err != nil {
		return nil, err
	}

	dispatches := make(map[string]replayDispatch, len(eventLog.Dispatches))
	for _, dispatch := range eventLog.Dispatches {
		dispatches[dispatch.dispatchID] = dispatch
	}

	completions := make(map[string]replayCompletion, len(eventLog.Completions))
	for _, completion := range eventLog.Completions {
		if _, ok := dispatches[completion.dispatchID]; !ok {
			return nil, newDivergenceError(
				DivergenceCategoryUnknownCompletion,
				completion.observedTick,
				completion.dispatchID,
				"recorded dispatch for completion "+completion.completionID,
				"completion references unknown dispatch "+completion.dispatchID,
				withExpectedEventID(completion.eventID),
			)
		}
		completions[completion.dispatchID] = completion
	}

	records := make([]sideEffectRecord, 0, len(eventLog.Dispatches))
	for _, dispatch := range eventLog.Dispatches {
		record := sideEffectRecord{dispatch: dispatch}
		if completion, ok := completions[dispatch.dispatchID]; ok {
			completionCopy := completion
			record.completion = &completionCopy
			record.hasCompletion = true
		}
		records = append(records, sideEffectRecord{
			dispatch:      record.dispatch,
			completion:    record.completion,
			hasCompletion: record.hasCompletion,
		})
	}

	return &SideEffects{records: records}, nil
}

// Infer implements workers.Provider by returning the recorded provider response
// for the matching dispatch.
func (s *SideEffects) Infer(ctx context.Context, req interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	record, err := s.claim("provider", func(candidate sideEffectRecord) bool {
		return providerRequestMatches(candidate, req)
	})
	if err != nil {
		return interfaces.InferenceResponse{}, err
	}
	if !record.hasCompletion {
		return interfaces.InferenceResponse{}, missingCompletionError(record.dispatch)
	}

	result := record.completion.result
	if result.Outcome == interfaces.OutcomeFailed && result.ProviderFailure != nil {
		return interfaces.InferenceResponse{}, workers.NewProviderError(
			result.ProviderFailure.Type,
			result.Error,
			errors.New(result.Error),
		)
	}

	return interfaces.InferenceResponse{
		Content:         result.Output,
		ProviderSession: cloneProviderSession(result.ProviderSession),
		Diagnostics:     cloneWorkDiagnostics(record.completion.diagnostics),
	}, nil
}

// Run implements workers.CommandRunner by returning the recorded script command
// outcome for the matching dispatch.
func (s *SideEffects) Run(ctx context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	record, err := s.claim("command", func(candidate sideEffectRecord) bool {
		return commandRequestMatches(candidate, req)
	})
	if err != nil {
		return workers.CommandResult{}, err
	}
	if !record.hasCompletion {
		return workers.CommandResult{}, missingCompletionError(record.dispatch)
	}
	if record.completion.diagnostics == nil || record.completion.diagnostics.Command == nil {
		result := workers.CommandResult{
			Stdout: []byte(record.completion.result.Output),
			Stderr: []byte(record.completion.result.Error),
		}
		if record.completion.result.Outcome == interfaces.OutcomeFailed {
			result.ExitCode = 1
		}
		return result, nil
	}

	command := record.completion.diagnostics.Command
	result := workers.CommandResult{
		Stdout:   []byte(command.Stdout),
		Stderr:   []byte(command.Stderr),
		ExitCode: command.ExitCode,
	}
	if command.TimedOut {
		return result, context.DeadlineExceeded
	}
	return result, nil
}

func missingCompletionError(dispatch replayDispatch) error {
	return fmt.Errorf("recorded dispatch %q for transition %q has no completion", dispatch.dispatchID, dispatch.dispatch.TransitionID)
}

func (s *SideEffects) claim(kind string, matches func(sideEffectRecord) bool) (sideEffectRecord, error) {
	if s == nil {
		return sideEffectRecord{}, fmt.Errorf("replay side effects are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.records {
		if s.records[i].usedBy != "" {
			continue
		}
		if !matches(s.records[i]) {
			continue
		}
		s.records[i].usedBy = kind
		return s.records[i], nil
	}
	return sideEffectRecord{}, newDivergenceError(
		DivergenceCategorySideEffectMismatch,
		0,
		"",
		"recorded "+kind+" request",
		"replay "+kind+" request did not match a recorded completion",
	)
}

func providerRequestMatches(record sideEffectRecord, req interfaces.ProviderInferenceRequest) bool {
	dispatch := record.dispatch.dispatch
	if !executionMetadataMatches(dispatch.Execution, req.Dispatch.Execution) {
		return false
	}
	if req.WorkerType != "" && req.WorkerType != dispatch.WorkerType {
		return false
	}
	if req.WorkstationType != "" && req.WorkstationType != dispatch.WorkstationName {
		return false
	}
	if record.completion != nil && record.completion.diagnostics != nil && record.completion.diagnostics.Provider != nil {
		provider := record.completion.diagnostics.Provider
		if provider.Provider != "" && req.ModelProvider != "" && provider.Provider != req.ModelProvider {
			return false
		}
		if provider.Model != "" && req.Model != "" && provider.Model != req.Model {
			return false
		}
	}
	return true
}

func commandRequestMatches(record sideEffectRecord, req workers.CommandRequest) bool {
	dispatch := record.dispatch.dispatch
	if !executionMetadataMatches(dispatch.Execution, req.Execution) {
		return false
	}
	if !record.hasCompletion {
		return true
	}
	if record.completion.diagnostics == nil || record.completion.diagnostics.Command == nil {
		return true
	}
	command := record.completion.diagnostics.Command
	if command.Command != "" && command.Command != req.Command {
		return false
	}
	if len(command.Args) > 0 && !reflect.DeepEqual(command.Args, req.Args) {
		return false
	}
	if command.WorkingDir != "" && command.WorkingDir != req.WorkDir {
		return false
	}
	return true
}

func executionMetadataMatches(recorded, observed interfaces.ExecutionMetadata) bool {
	if recorded.ReplayKey != "" && observed.ReplayKey != recorded.ReplayKey {
		return false
	}
	if recorded.TraceID != "" && observed.TraceID != recorded.TraceID {
		return false
	}
	if len(recorded.WorkIDs) > 0 && !reflect.DeepEqual(recorded.WorkIDs, observed.WorkIDs) {
		return false
	}
	return true
}

func cloneProviderSession(in *interfaces.ProviderSessionMetadata) *interfaces.ProviderSessionMetadata {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func cloneWorkDiagnostics(in *interfaces.WorkDiagnostics) *interfaces.WorkDiagnostics {
	if in == nil {
		return nil
	}
	out := &interfaces.WorkDiagnostics{
		RenderedPrompt: cloneRenderedPromptDiagnostic(in.RenderedPrompt),
		Provider:       cloneProviderDiagnostic(in.Provider),
		Command:        cloneCommandDiagnostic(in.Command),
		Metadata:       cloneSideEffectStringMap(in.Metadata),
	}
	if in.Panic != nil {
		out.Panic = &interfaces.PanicDiagnostic{
			Message: in.Panic.Message,
			Stack:   in.Panic.Stack,
		}
	}
	return out
}

func cloneRenderedPromptDiagnostic(in *interfaces.RenderedPromptDiagnostic) *interfaces.RenderedPromptDiagnostic {
	if in == nil {
		return nil
	}
	return &interfaces.RenderedPromptDiagnostic{
		SystemPromptHash: in.SystemPromptHash,
		UserMessageHash:  in.UserMessageHash,
		Variables:        cloneSideEffectStringMap(in.Variables),
	}
}

func cloneProviderDiagnostic(in *interfaces.ProviderDiagnostic) *interfaces.ProviderDiagnostic {
	if in == nil {
		return nil
	}
	return &interfaces.ProviderDiagnostic{
		Provider:         in.Provider,
		Model:            in.Model,
		RequestMetadata:  cloneSideEffectStringMap(in.RequestMetadata),
		ResponseMetadata: cloneSideEffectStringMap(in.ResponseMetadata),
	}
}

func cloneCommandDiagnostic(in *interfaces.CommandDiagnostic) *interfaces.CommandDiagnostic {
	if in == nil {
		return nil
	}
	return &interfaces.CommandDiagnostic{
		Command:    in.Command,
		Args:       append([]string(nil), in.Args...),
		Stdin:      in.Stdin,
		Env:        cloneSideEffectStringMap(in.Env),
		Stdout:     in.Stdout,
		Stderr:     in.Stderr,
		ExitCode:   in.ExitCode,
		TimedOut:   in.TimedOut,
		Duration:   in.Duration,
		WorkingDir: in.WorkingDir,
	}
}

func cloneSideEffectStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

var _ workers.Provider = (*SideEffects)(nil)
var _ workers.CommandRunner = (*SideEffects)(nil)
