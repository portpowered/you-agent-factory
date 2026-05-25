package interfaces

// CloneToken returns a detached copy of the canonical runtime token shape.
func CloneToken(token Token) Token {
	return Token{
		ID:        token.ID,
		PlaceID:   token.PlaceID,
		Color:     CloneTokenColor(token.Color),
		CreatedAt: token.CreatedAt,
		EnteredAt: token.EnteredAt,
		History:   CloneTokenHistory(token.History),
	}
}

// CloneTokens returns detached copies of canonical runtime tokens.
func CloneTokens(tokens []Token) []Token {
	if tokens == nil {
		return nil
	}
	clones := make([]Token, len(tokens))
	for i := range tokens {
		clones[i] = CloneToken(tokens[i])
	}
	return clones
}

// CloneTokenColor returns a detached copy of the canonical runtime token color.
func CloneTokenColor(color TokenColor) TokenColor {
	return TokenColor{
		Name:                     color.Name,
		RequestID:                color.RequestID,
		WorkID:                   color.WorkID,
		WorkTypeID:               color.WorkTypeID,
		DataType:                 color.DataType,
		ChainingTraceDepth:       color.ChainingTraceDepth,
		CurrentChainingTraceID:   color.CurrentChainingTraceID,
		PreviousChainingTraceIDs: cloneStringSlice(color.PreviousChainingTraceIDs),
		TraceID:                  color.TraceID,
		ParentID:                 color.ParentID,
		Tags:                     cloneStringMap(color.Tags),
		Relations:                cloneRelations(color.Relations),
		Content:                  CloneWorkContentParts(color.Content),
		Payload:                  cloneBytes(color.Payload),
	}
}

// CloneWorkContentParts returns a detached copy of canonical work content parts.
func CloneWorkContentParts(parts []WorkContentPart) []WorkContentPart {
	if len(parts) == 0 {
		return nil
	}
	clone := make([]WorkContentPart, len(parts))
	copy(clone, parts)
	return clone
}

func cloneWorkContentParts(parts []WorkContentPart) []WorkContentPart {
	return CloneWorkContentParts(parts)
}

// CloneTokenHistory returns a detached copy of canonical runtime token history.
func CloneTokenHistory(history TokenHistory) TokenHistory {
	return TokenHistory{
		TotalVisits:         cloneStringIntMap(history.TotalVisits),
		ConsecutiveFailures: cloneStringIntMap(history.ConsecutiveFailures),
		PlaceVisits:         cloneStringIntMap(history.PlaceVisits),
		TotalDuration:       history.TotalDuration,
		LastError:           history.LastError,
		FailureLog:          cloneFailureRecords(history.FailureLog),
	}
}

// CloneProviderSessionMetadata returns a detached copy of canonical provider
// session metadata.
func CloneProviderSessionMetadata(session *ProviderSessionMetadata) *ProviderSessionMetadata {
	if session == nil {
		return nil
	}
	clone := *session
	return &clone
}

// CloneWorkFailureMetadata returns a detached copy of canonical work failure
// metadata.
func CloneWorkFailureMetadata(failure *WorkFailureMetadata) *WorkFailureMetadata {
	if failure == nil {
		return nil
	}
	clone := *failure
	return &clone
}

// CloneProviderFailureMetadata retains the legacy helper name while runtime
// paths migrate to generalized failure metadata.
func CloneProviderFailureMetadata(failure *ProviderFailureMetadata) *ProviderFailureMetadata {
	return CloneWorkFailureMetadata(failure)
}

// CloneSafeWorkDiagnostics returns a detached copy of the canonical safe
// diagnostics boundary.
func CloneSafeWorkDiagnostics(diagnostics *SafeWorkDiagnostics) *SafeWorkDiagnostics {
	if diagnostics == nil {
		return nil
	}
	return &SafeWorkDiagnostics{
		RenderedPrompt: cloneSafeRenderedPromptDiagnostic(diagnostics.RenderedPrompt),
		Provider:       cloneSafeProviderDiagnostic(diagnostics.Provider),
	}
}

// CloneFactoryWorldDispatchCompletion returns a detached copy of one canonical
// selected-tick dispatch completion record.
func CloneFactoryWorldDispatchCompletion(completion FactoryWorldDispatchCompletion) FactoryWorldDispatchCompletion {
	clone := completion
	failureMetadata := CanonicalWorkFailureMetadata(completion.Result.FailureMetadata, completion.Result.ProviderFailure)
	clone.Result.FailureMetadata = CloneWorkFailureMetadata(failureMetadata)
	clone.Result.ProviderFailure = CloneProviderFailureMetadata(failureMetadata)
	clone.WorkItemIDs = cloneStringSlice(completion.WorkItemIDs)
	clone.ConsumedInputs = cloneWorkstationInputs(completion.ConsumedInputs)
	clone.InputWorkItems = cloneFactoryWorkItems(completion.InputWorkItems)
	clone.OutputWorkItems = cloneFactoryWorkItems(completion.OutputWorkItems)
	clone.PreviousChainingTraceIDs = cloneStringSlice(completion.PreviousChainingTraceIDs)
	clone.TraceIDs = cloneStringSlice(completion.TraceIDs)
	clone.ProviderSession = CloneProviderSessionMetadata(completion.ProviderSession)
	clone.Diagnostics = CloneSafeWorkDiagnostics(completion.Diagnostics)
	clone.TerminalWork = cloneFactoryTerminalWork(completion.TerminalWork)
	return clone
}

// CloneFactoryWorldProviderSessionRecord returns a detached copy of one
// canonical selected-tick provider-session record.
func CloneFactoryWorldProviderSessionRecord(record FactoryWorldProviderSessionRecord) FactoryWorldProviderSessionRecord {
	clone := record
	clone.ProviderSession = *CloneProviderSessionMetadata(&record.ProviderSession)
	clone.Diagnostics = CloneSafeWorkDiagnostics(record.Diagnostics)
	clone.WorkItemIDs = cloneStringSlice(record.WorkItemIDs)
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

func cloneSafeRenderedPromptDiagnostic(diagnostic *SafeRenderedPromptDiagnostic) *SafeRenderedPromptDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &SafeRenderedPromptDiagnostic{
		SystemPromptHash: diagnostic.SystemPromptHash,
		UserMessageHash:  diagnostic.UserMessageHash,
		Variables:        cloneStringMap(diagnostic.Variables),
	}
}

func cloneSafeProviderDiagnostic(diagnostic *SafeProviderDiagnostic) *SafeProviderDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &SafeProviderDiagnostic{
		Provider:         diagnostic.Provider,
		Model:            diagnostic.Model,
		RequestMetadata:  cloneStringMap(diagnostic.RequestMetadata),
		ResponseMetadata: cloneStringMap(diagnostic.ResponseMetadata),
	}
}

func cloneFactoryWorldInferenceAttempt(attempt FactoryWorldInferenceAttempt) FactoryWorldInferenceAttempt {
	clone := attempt
	clone.ExitCode = cloneIntPtr(attempt.ExitCode)
	clone.ProviderSession = CloneProviderSessionMetadata(attempt.ProviderSession)
	clone.Diagnostics = CloneSafeWorkDiagnostics(attempt.Diagnostics)
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

func cloneFactoryWorkItems(items []FactoryWorkItem) []FactoryWorkItem {
	if len(items) == 0 {
		return nil
	}
	clone := make([]FactoryWorkItem, len(items))
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

func cloneRelations(relations []Relation) []Relation {
	if relations == nil {
		return nil
	}
	clone := make([]Relation, len(relations))
	copy(clone, relations)
	return clone
}

func cloneBytes(values []byte) []byte {
	if values == nil {
		return nil
	}
	clone := make([]byte, len(values))
	copy(clone, values)
	return clone
}

func cloneFailureRecords(records []FailureRecord) []FailureRecord {
	if records == nil {
		return nil
	}
	clone := make([]FailureRecord, len(records))
	copy(clone, records)
	return clone
}

func cloneStringIntMap(values map[string]int) map[string]int {
	if values == nil {
		return nil
	}
	clone := make(map[string]int, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
