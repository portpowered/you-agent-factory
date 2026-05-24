package interfaces

// FactorySubmissionRecord stores the engine tick at which a submit request
// became visible to the runtime.
type FactorySubmissionRecord struct {
	SubmissionID string
	ObservedTick int
	Request      SubmitRequest
	Source       string
}

// FactoryDispatchRecord stores a raw WorkDispatch plus token mutations held
// while the worker is in flight.
type FactoryDispatchRecord struct {
	DispatchID     string
	CreatedTick    int
	Dispatch       WorkDispatch
	HeldMutations  []MarkingMutation
	ConsumedTokens []string
}

// FactoryCompletionRecord stores a worker result at the logical tick where the
// engine observed it.
type FactoryCompletionRecord struct {
	CompletionID string
	DispatchID   string
	ObservedTick int
	Result       WorkResult
}

// SubmissionHookContext is the input passed to engine-owned submission hooks
// once per logical tick.
type SubmissionHookContext[TSnapshot any] struct {
	Snapshot          TSnapshot
	ContinuationState map[string]string
}

// SubmissionHookResult contains all due hook output observed by the engine at
// one logical tick.
type SubmissionHookResult struct {
	GeneratedBatches  []GeneratedSubmissionBatch
	Results           []WorkResult
	ContinuationState map[string]string
	KeepAlive         bool
}
