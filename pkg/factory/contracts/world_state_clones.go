package factorycontracts

import (
	"github.com/portpowered/infinite-you/pkg/work"
	workerdiagnostics "github.com/portpowered/infinite-you/pkg/workers/diagnostics"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

// CloneFactoryWorldDispatchCompletion returns a detached copy of one canonical
// selected-tick dispatch completion record.
func CloneFactoryWorldDispatchCompletion(completion FactoryWorldDispatchCompletion) FactoryWorldDispatchCompletion {
	clone := completion
	clone.Result.FailureMetadata = workerexecution.CloneWorkFailureMetadata(completion.Result.FailureMetadata)
	clone.WorkItemIDs = cloneStringSlice(completion.WorkItemIDs)
	clone.ConsumedInputs = cloneWorkstationInputs(completion.ConsumedInputs)
	clone.InputWorkItems = cloneFactoryWorkItems(completion.InputWorkItems)
	clone.OutputWorkItems = cloneFactoryWorkItems(completion.OutputWorkItems)
	clone.PreviousChainingTraceIDs = cloneStringSlice(completion.PreviousChainingTraceIDs)
	clone.TraceIDs = cloneStringSlice(completion.TraceIDs)
	clone.ProviderSession = workerexecution.CloneProviderSessionMetadata(completion.ProviderSession)
	clone.Diagnostics = workerdiagnostics.CloneSafeWorkDiagnostics(completion.Diagnostics)
	clone.TerminalWork = cloneFactoryTerminalWork(completion.TerminalWork)
	return clone
}

// CloneFactoryWorldProviderSessionRecord returns a detached copy of one
// canonical selected-tick provider-session record.
func CloneFactoryWorldProviderSessionRecord(record FactoryWorldProviderSessionRecord) FactoryWorldProviderSessionRecord {
	clone := record
	clone.ProviderSession = *workerexecution.CloneProviderSessionMetadata(&record.ProviderSession)
	clone.Diagnostics = workerdiagnostics.CloneSafeWorkDiagnostics(record.Diagnostics)
	clone.WorkItemIDs = cloneStringSlice(record.WorkItemIDs)
	clone.WorkItems = cloneFactoryWorldWorkItemRefs(record.WorkItems)
	clone.ConsumedInputs = cloneWorkstationInputs(record.ConsumedInputs)
	clone.PreviousChainingTraceIDs = cloneStringSlice(record.PreviousChainingTraceIDs)
	clone.TraceIDs = cloneStringSlice(record.TraceIDs)
	return clone
}

// CloneFactoryWorldInferenceAttemptsByDispatchID returns a detached copy of
// selected-tick inference attempts keyed by dispatch and request ID.
func CloneFactoryWorldInferenceAttemptsByDispatchID(
	attemptsByDispatchID map[string]map[string]FactoryWorldInferenceAttempt,
) map[string]map[string]FactoryWorldInferenceAttempt {
	if len(attemptsByDispatchID) == 0 {
		return nil
	}
	clone := make(map[string]map[string]FactoryWorldInferenceAttempt, len(attemptsByDispatchID))
	for dispatchID, attempts := range attemptsByDispatchID {
		if len(attempts) == 0 {
			continue
		}
		clone[dispatchID] = make(map[string]FactoryWorldInferenceAttempt, len(attempts))
		for requestID, attempt := range attempts {
			clone[dispatchID][requestID] = cloneFactoryWorldInferenceAttempt(attempt)
		}
	}
	if len(clone) == 0 {
		return nil
	}
	return clone
}

// CloneWorkstationInputs returns a detached copy of canonical workstation
// inputs for selected-tick runtime projections.
func CloneWorkstationInputs(inputs []WorkstationInput) []WorkstationInput {
	return cloneWorkstationInputs(inputs)
}

func cloneFactoryWorldInferenceAttempt(attempt FactoryWorldInferenceAttempt) FactoryWorldInferenceAttempt {
	clone := attempt
	clone.ExitCode = cloneIntPtr(attempt.ExitCode)
	clone.ProviderSession = workerexecution.CloneProviderSessionMetadata(attempt.ProviderSession)
	clone.Diagnostics = workerdiagnostics.CloneSafeWorkDiagnostics(attempt.Diagnostics)
	return clone
}

func cloneFactoryTerminalWork(terminalWork *FactoryTerminalWork) *FactoryTerminalWork {
	if terminalWork == nil {
		return nil
	}
	clone := *terminalWork
	clone.WorkItem.PreviousChainingTraceIDs = cloneStringSlice(terminalWork.WorkItem.PreviousChainingTraceIDs)
	clone.WorkItem.Tags = cloneStringMap(terminalWork.WorkItem.Tags)
	return &clone
}

func cloneFactoryWorkItems(items []work.FactoryWorkItem) []work.FactoryWorkItem {
	if len(items) == 0 {
		return nil
	}
	clone := make([]work.FactoryWorkItem, len(items))
	for i, item := range items {
		clone[i] = item
		clone[i].PreviousChainingTraceIDs = cloneStringSlice(item.PreviousChainingTraceIDs)
		clone[i].Tags = cloneStringMap(item.Tags)
	}
	return clone
}

func cloneWorkstationInputs(inputs []WorkstationInput) []WorkstationInput {
	if len(inputs) == 0 {
		return nil
	}
	clone := make([]WorkstationInput, len(inputs))
	for i, input := range inputs {
		clone[i] = input
		if input.WorkItem != nil {
			item := *input.WorkItem
			item.PreviousChainingTraceIDs = cloneStringSlice(item.PreviousChainingTraceIDs)
			item.Tags = cloneStringMap(item.Tags)
			clone[i].WorkItem = &item
		}
		if input.Resource != nil {
			resource := *input.Resource
			clone[i].Resource = &resource
		}
	}
	return clone
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	clone := make([]string, len(values))
	copy(clone, values)
	return clone
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneFactoryWorldWorkItemRef(ref FactoryWorldWorkItemRef) FactoryWorldWorkItemRef {
	clone := ref
	clone.PreviousChainingTraceIDs = cloneStringSlice(ref.PreviousChainingTraceIDs)
	clone.LineageParentWorkIDs = cloneStringSlice(ref.LineageParentWorkIDs)
	clone.Content = work.CloneWorkContentParts(ref.Content)
	return clone
}

func cloneFactoryWorldWorkItemRefs(refs []FactoryWorldWorkItemRef) []FactoryWorldWorkItemRef {
	if len(refs) == 0 {
		return nil
	}
	clones := make([]FactoryWorldWorkItemRef, len(refs))
	for i := range refs {
		clones[i] = cloneFactoryWorldWorkItemRef(refs[i])
	}
	return clones
}
