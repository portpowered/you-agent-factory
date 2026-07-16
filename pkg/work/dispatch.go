package work

// WorkDispatch is the canonical dispatch-owned runtime payload.
type WorkDispatch struct {
	DispatchID               string              `json:"dispatch_id"`
	TransitionID             string              `json:"transition_id"`
	WorkerType               string              `json:"worker_type,omitempty"`
	WorkstationName          string              `json:"workstation_name,omitempty"`
	ProjectID                string              `json:"project_id,omitempty"`
	CurrentChainingTraceID   string              `json:"current_chaining_trace_id,omitempty"`
	PreviousChainingTraceIDs []string            `json:"previous_chaining_trace_ids,omitempty"`
	Execution                ExecutionMetadata   `json:"execution,omitempty"`
	InputTokens              []any               `json:"input_tokens"`
	InputBindings            map[string][]string `json:"input_bindings,omitempty"`
}

type ExecutionMetadata struct {
	DispatchCreatedTick int      `json:"dispatch_created_tick,omitempty"`
	CurrentTick         int      `json:"current_tick,omitempty"`
	RequestID           string   `json:"request_id,omitempty"`
	TraceID             string   `json:"trace_id,omitempty"`
	WorkIDs             []string `json:"work_ids,omitempty"`
	ReplayKey           string   `json:"replay_key,omitempty"`
}

func CloneExecutionMetadata(metadata ExecutionMetadata) ExecutionMetadata {
	clone := metadata
	clone.WorkIDs = cloneStringSlice(metadata.WorkIDs)
	return clone
}

func CloneWorkDispatch(dispatch WorkDispatch) WorkDispatch {
	clone := dispatch
	clone.PreviousChainingTraceIDs = cloneStringSlice(dispatch.PreviousChainingTraceIDs)
	clone.Execution = CloneExecutionMetadata(dispatch.Execution)
	clone.InputTokens = cloneAnySlice(dispatch.InputTokens)
	clone.InputBindings = cloneStringSliceMap(dispatch.InputBindings)
	return clone
}

func cloneStringSliceMap(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string][]string, len(values))
	for key, items := range values {
		clone[key] = cloneStringSlice(items)
	}
	return clone
}

func cloneAnySlice(values []any) []any {
	if len(values) == 0 {
		return nil
	}
	clone := make([]any, len(values))
	copy(clone, values)
	return clone
}

func CloneWorkContentParts(parts []WorkContentPart) []WorkContentPart {
	if len(parts) == 0 {
		return nil
	}
	cloned := make([]WorkContentPart, len(parts))
	for i, part := range parts {
		cloned[i] = part
		cloned[i].JSON = append([]byte(nil), part.JSON...)
		cloned[i].Metadata = cloneAnyMap(part.Metadata)
	}
	return cloned
}

// CloneInvocationArguments returns a detached copy of runtime-only invocation
// argument metadata.
func CloneInvocationArguments(args *InvocationArguments) *InvocationArguments {
	if args == nil || len(args.Arguments) == 0 {
		return nil
	}
	clone := &InvocationArguments{Arguments: make(map[string]InvocationArgument, len(args.Arguments))}
	for name, argument := range args.Arguments {
		next := InvocationArgument{
			Values: cloneStringSlice(argument.Values), ValueMode: argument.ValueMode, Sensitive: argument.Sensitive,
		}
		if len(argument.Sources) > 0 {
			next.Sources = append([]InvocationArgumentSource(nil), argument.Sources...)
		}
		clone.Arguments[name] = next
	}
	return clone
}

func cloneAnyMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	clone := make(map[string]any, len(values))
	for key, value := range values {
		clone[key] = value
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
	return append([]string(nil), values...)
}
