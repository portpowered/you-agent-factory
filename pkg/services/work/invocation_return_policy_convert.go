package work

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/work/internal/invocationreturnpolicy"
)

func mapInvocationReturnPolicyError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, invocationreturnpolicy.ErrUnsupportedReturnPolicy) {
		return ErrUnsupportedReturnPolicy
	}
	if errors.Is(err, invocationreturnpolicy.ErrInvalidInvocationInput) {
		var inputErr *invocationreturnpolicy.InputError
		if errors.As(err, &inputErr) {
			return fmt.Errorf("%w: %w", ErrInvalidInvocationInput, inputErrorFromInternal(inputErr))
		}
		var argumentErr *invocationreturnpolicy.ArgumentError
		if errors.As(err, &argumentErr) {
			return fmt.Errorf("%w: %w", ErrInvalidInvocationInput, argumentErrorFromInternal(argumentErr))
		}
		var contentErr *invocationreturnpolicy.TextContentValidationError
		if errors.As(err, &contentErr) {
			return fmt.Errorf("%w: %w", ErrInvalidInvocationInput, &TextContentValidationError{Message: contentErr.Message})
		}
		return fmt.Errorf("%w: %w", ErrInvalidInvocationInput, err)
	}
	var argumentErr *invocationreturnpolicy.ArgumentError
	if errors.As(err, &argumentErr) {
		return argumentErrorFromInternal(argumentErr)
	}
	var inputErr *invocationreturnpolicy.InputError
	if errors.As(err, &inputErr) {
		return inputErrorFromInternal(inputErr)
	}
	var contentErr *invocationreturnpolicy.TextContentValidationError
	if errors.As(err, &contentErr) {
		return &TextContentValidationError{Message: contentErr.Message}
	}
	var primaryErr *invocationreturnpolicy.PrimaryResultError
	if errors.As(err, &primaryErr) {
		return primaryResultErrorFromInternal(primaryErr)
	}
	return err
}

func invocationSignatureToInternal(signature *InvocationSignatureConfig) *invocationreturnpolicy.InvocationSignatureConfig {
	if signature == nil {
		return nil
	}
	inner := &invocationreturnpolicy.InvocationSignatureConfig{
		UnknownNamedArgumentPolicy: signature.UnknownNamedArgumentPolicy,
	}
	for _, parameter := range signature.Parameters {
		inner.Parameters = append(inner.Parameters, invocationreturnpolicy.InvocationParameterConfig{
			Name:          parameter.Name,
			ExternalName:  parameter.ExternalName,
			Aliases:       cloneStringSlice(parameter.Aliases),
			TypeHint:      parameter.TypeHint,
			ValueMode:     parameter.ValueMode,
			Required:      parameter.Required,
			Sensitive:     parameter.Sensitive,
			Choices:       cloneStringSlice(parameter.Choices),
			DefaultValue:  parameter.DefaultValue,
			DefaultValues: cloneStringSlice(parameter.DefaultValues),
			Bindings:      invocationBindingsToInternal(parameter.Bindings),
		})
	}
	return inner
}

func invocationBindingsToInternal(bindings []InvocationParameterBindingConfig) []invocationreturnpolicy.InvocationParameterBindingConfig {
	if len(bindings) == 0 {
		return nil
	}
	converted := make([]invocationreturnpolicy.InvocationParameterBindingConfig, len(bindings))
	for i, binding := range bindings {
		converted[i] = invocationreturnpolicy.InvocationParameterBindingConfig{
			Kind:     binding.Kind,
			Position: binding.Position,
		}
	}
	return converted
}

func contentPartsToInternal(parts []WorkContentPart) []invocationreturnpolicy.ContentPart {
	if len(parts) == 0 {
		return nil
	}
	converted := make([]invocationreturnpolicy.ContentPart, len(parts))
	for i, part := range parts {
		converted[i] = invocationreturnpolicy.ContentPart{
			Type:        invocationreturnpolicy.ContentPartType(part.Type),
			Text:        part.Text,
			URL:         part.URL,
			File:        part.File,
			JSON:        append(json.RawMessage(nil), part.JSON...),
			Slot:        part.Slot,
			Label:       part.Label,
			Role:        part.Role,
			ContentType: part.ContentType,
			ArtifactID:  part.ArtifactID,
			Metadata:    cloneAnyMap(part.Metadata),
		}
	}
	return converted
}

func contentPartsFromInternal(parts []invocationreturnpolicy.ContentPart) []WorkContentPart {
	if len(parts) == 0 {
		return nil
	}
	converted := make([]WorkContentPart, len(parts))
	for i, part := range parts {
		converted[i] = WorkContentPart{
			Type:        WorkContentPartType(part.Type),
			Text:        part.Text,
			URL:         part.URL,
			File:        part.File,
			JSON:        append(json.RawMessage(nil), part.JSON...),
			Slot:        part.Slot,
			Label:       part.Label,
			Role:        part.Role,
			ContentType: part.ContentType,
			ArtifactID:  part.ArtifactID,
			Metadata:    cloneAnyMap(part.Metadata),
		}
	}
	return converted
}

func namedArgumentInputsToInternal(inputs []NamedArgumentInput) []invocationreturnpolicy.NamedArgumentInput {
	if len(inputs) == 0 {
		return nil
	}
	converted := make([]invocationreturnpolicy.NamedArgumentInput, len(inputs))
	for i, input := range inputs {
		converted[i] = invocationreturnpolicy.NamedArgumentInput{
			Key:    input.Key,
			Values: cloneStringSlice(input.Values),
		}
	}
	return converted
}

func normalizeArgumentsInputToInternal(input NormalizeArgumentsInput) invocationreturnpolicy.NormalizeArgumentsInput {
	return invocationreturnpolicy.NormalizeArgumentsInput{
		Signature:            invocationSignatureToInternal(input.Signature),
		PositionalArgs:       cloneStringSlice(input.PositionalArgs),
		NamedArgs:            namedArgumentInputsToInternal(input.NamedArgs),
		DirectArgs:           namedArgumentInputsToInternal(input.DirectArgs),
		StdinText:            input.StdinText,
		CompatibilityText:    input.CompatibilityText,
		CompatibilityContent: contentPartsToInternal(input.CompatibilityContent),
	}
}

func normalizedArgumentsFromInternal(input invocationreturnpolicy.NormalizedArguments) NormalizedArguments {
	outer := NormalizedArguments{
		Arguments:        make(map[string]NormalizedArgument, len(input.Arguments)),
		UnknownNamedArgs: make(map[string][]string, len(input.UnknownNamedArgs)),
	}
	for name, argument := range input.Arguments {
		outer.Arguments[name] = NormalizedArgument{
			Values:    cloneStringSlice(argument.Values),
			Sensitive: argument.Sensitive,
			Sources:   argumentSourcesFromInternal(argument.Sources),
		}
	}
	for name, values := range input.UnknownNamedArgs {
		outer.UnknownNamedArgs[name] = cloneStringSlice(values)
	}
	if input.CompatibilityInput != nil {
		resolved := resolvedInputFromInternal(*input.CompatibilityInput)
		outer.CompatibilityInput = &resolved
	}
	return outer
}

func argumentSourcesFromInternal(sources []invocationreturnpolicy.ArgumentSource) []ArgumentSource {
	if len(sources) == 0 {
		return nil
	}
	converted := make([]ArgumentSource, len(sources))
	for i, source := range sources {
		converted[i] = ArgumentSource{
			Kind:   ArgumentSourceKind(source.Kind),
			Name:   source.Name,
			Redact: source.Redact,
		}
	}
	return converted
}

func resolvedInputFromInternal(input invocationreturnpolicy.ResolvedInput) ResolvedInput {
	return ResolvedInput{
		Source:  InputSourceLabel(input.Source),
		Text:    input.Text,
		Content: contentPartsFromInternal(input.Content),
	}
}

func argumentErrorFromInternal(err *invocationreturnpolicy.ArgumentError) *ArgumentError {
	if err == nil {
		return nil
	}
	return &ArgumentError{
		Code:       ArgumentErrorCode(err.Code),
		Message:    err.Message,
		Parameter:  err.Parameter,
		Argument:   err.Argument,
		SourceKind: ArgumentSourceKind(err.SourceKind),
	}
}

func argumentErrorToInternal(err *ArgumentError) *invocationreturnpolicy.ArgumentError {
	if err == nil {
		return nil
	}
	return &invocationreturnpolicy.ArgumentError{
		Code:       invocationreturnpolicy.ArgumentErrorCode(err.Code),
		Message:    err.Message,
		Parameter:  err.Parameter,
		Argument:   err.Argument,
		SourceKind: invocationreturnpolicy.ArgumentSourceKind(err.SourceKind),
	}
}

func inputErrorFromInternal(err *invocationreturnpolicy.InputError) *InputError {
	if err == nil {
		return nil
	}
	conflicts := make([]InputSourceLabel, len(err.ConflictingSources))
	for i, source := range err.ConflictingSources {
		conflicts[i] = InputSourceLabel(source)
	}
	return &InputError{
		Code:               InputErrorCode(err.Code),
		Message:            err.Message,
		Source:             InputSourceLabel(err.Source),
		ConflictingSources: conflicts,
	}
}

func invocationReturnToInternal(cfg *InvocationReturnConfig) *invocationreturnpolicy.InvocationReturnConfig {
	if cfg == nil {
		return nil
	}
	return &invocationreturnpolicy.InvocationReturnConfig{
		Policy:        cfg.Policy,
		WorkTypeName:  cfg.WorkTypeName,
		TerminalState: cfg.TerminalState,
		WorkName:      cfg.WorkName,
	}
}

type invocationWorldStateProviderAdapter struct {
	provider InvocationWorldStateProvider
}

func (a invocationWorldStateProviderAdapter) InvocationWorldState() invocationreturnpolicy.InvocationWorldState {
	if a.provider == nil {
		return invocationreturnpolicy.InvocationWorldState{}
	}
	return invocationWorldStateToInternal(a.provider.InvocationWorldState())
}

func invocationWorldStateToInternal(state InvocationWorldState) invocationreturnpolicy.InvocationWorldState {
	inner := invocationreturnpolicy.InvocationWorldState{
		PayloadLineage:           toLineageProjection(state.PayloadLineage),
		FactoryState:             state.FactoryState,
		WorkRequestsByID:         make(map[string]invocationreturnpolicy.InvocationWorkRequest, len(state.WorkRequestsByID)),
		WorkItemsByID:            make(map[string]invocationreturnpolicy.WorkItem, len(state.WorkItemsByID)),
		FailedWorkItemsByID:      make(map[string]invocationreturnpolicy.WorkItem, len(state.FailedWorkItemsByID)),
		TerminalWorkByID:         make(map[string]invocationreturnpolicy.InvocationTerminalWork, len(state.TerminalWorkByID)),
		WorkStateChangesByWorkID: make(map[string][]invocationreturnpolicy.InvocationWorkStateChange, len(state.WorkStateChangesByWorkID)),
	}
	for id, request := range state.WorkRequestsByID {
		inner.WorkRequestsByID[id] = invocationreturnpolicy.InvocationWorkRequest{
			TraceID:   request.TraceID,
			WorkItems: workItemsToInternal(request.WorkItems),
		}
	}
	for id, item := range state.WorkItemsByID {
		inner.WorkItemsByID[id] = workItemToInternal(item)
	}
	for id, item := range state.FailedWorkItemsByID {
		inner.FailedWorkItemsByID[id] = workItemToInternal(item)
	}
	for id, terminal := range state.TerminalWorkByID {
		inner.TerminalWorkByID[id] = invocationreturnpolicy.InvocationTerminalWork{
			Status:   terminal.Status,
			WorkItem: workItemToInternal(terminal.WorkItem),
		}
	}
	for id, changes := range state.WorkStateChangesByWorkID {
		converted := make([]invocationreturnpolicy.InvocationWorkStateChange, len(changes))
		for i, change := range changes {
			converted[i] = invocationreturnpolicy.InvocationWorkStateChange{
				WorkID:       change.WorkID,
				WorkTypeName: change.WorkTypeName,
				ToState:      change.ToState,
				ToPlaceID:    change.ToPlaceID,
				RequestID:    change.RequestID,
			}
		}
		inner.WorkStateChangesByWorkID[id] = converted
	}
	if state.JavaScriptRuntime != nil {
		inner.JavaScriptRuntime = &invocationreturnpolicy.InvocationJavaScriptRuntime{
			Dispatches: invocationDispatchesToInternal(state.JavaScriptRuntime.Dispatches),
		}
	}
	if state.SessionBracket != nil {
		inner.SessionBracket = &invocationreturnpolicy.InvocationSessionBracket{
			SessionID:              state.SessionBracket.SessionID,
			LifecycleControlStatus: state.SessionBracket.LifecycleControlStatus,
			FinalStatus:            state.SessionBracket.FinalStatus,
			FailureReason:          state.SessionBracket.FailureReason,
		}
	}
	return inner
}

func invocationDispatchesToInternal(dispatches []InvocationDispatchState) []invocationreturnpolicy.InvocationDispatchState {
	if len(dispatches) == 0 {
		return nil
	}
	converted := make([]invocationreturnpolicy.InvocationDispatchState, len(dispatches))
	for i, dispatch := range dispatches {
		converted[i] = invocationreturnpolicy.InvocationDispatchState{
			ID:             dispatch.ID,
			Status:         dispatch.Status,
			RelatedWorkIDs: cloneStringSlice(dispatch.RelatedWorkIDs),
		}
	}
	return converted
}

func workItemsToInternal(items []FactoryWorkItem) []invocationreturnpolicy.WorkItem {
	if len(items) == 0 {
		return nil
	}
	converted := make([]invocationreturnpolicy.WorkItem, len(items))
	for i, item := range items {
		converted[i] = workItemToInternal(item)
	}
	return converted
}

func workItemToInternal(item FactoryWorkItem) invocationreturnpolicy.WorkItem {
	return invocationreturnpolicy.WorkItem{
		ID:                       item.ID,
		WorkTypeID:               item.WorkTypeID,
		State:                    item.State,
		DisplayName:              item.DisplayName,
		ChainingTraceDepth:       item.ChainingTraceDepth,
		CurrentChainingTraceID:   item.CurrentChainingTraceID,
		PreviousChainingTraceIDs: cloneStringSlice(item.PreviousChainingTraceIDs),
		TraceID:                  item.TraceID,
		Content:                  contentPartsToInternal(item.Content),
		ParentID:                 item.ParentID,
		PlaceID:                  item.PlaceID,
		Tags:                     cloneStringMap(item.Tags),
	}
}

func primaryResultSelectionInputToInternal(input PrimaryResultSelectionInput) invocationreturnpolicy.PrimaryResultSelectionInput {
	return invocationreturnpolicy.PrimaryResultSelectionInput{
		RequestID:        input.RequestID,
		InvocationReturn: invocationReturnToInternal(input.InvocationReturn),
		WorldState:       invocationWorldStateProviderAdapter{provider: input.WorldState},
	}
}

func primaryResultSelectionFromInternal(selection invocationreturnpolicy.PrimaryResultSelection) PrimaryResultSelection {
	return PrimaryResultSelection{
		RequestID:     selection.RequestID,
		Policy:        selection.Policy,
		WorkID:        selection.WorkID,
		WorkTypeName:  selection.WorkTypeName,
		WorkName:      selection.WorkName,
		TerminalState: selection.TerminalState,
		PrimaryResult: contentPartsFromInternal(selection.PrimaryResult),
	}
}

func primaryResultErrorFromInternal(err *invocationreturnpolicy.PrimaryResultError) *PrimaryResultError {
	if err == nil {
		return nil
	}
	return &PrimaryResultError{
		Code:      PrimaryResultErrorCode(err.Code),
		Message:   err.Message,
		RequestID: err.RequestID,
		Policy:    err.Policy,
		Context: InvocationFailureContext{
			SessionID: err.Context.SessionID,
			WorkID:    err.Context.WorkID,
			WorkName:  err.Context.WorkName,
			WorkState: err.Context.WorkState,
		},
	}
}

func invocationInputPreparationRequestToInternal(request InvocationInputPreparationRequest) invocationreturnpolicy.InvocationInputPreparationRequest {
	return invocationreturnpolicy.InvocationInputPreparationRequest{
		Arguments:            cloneStringSlice(request.Arguments),
		Signature:            invocationSignatureToInternal(request.Signature),
		StdinText:            request.StdinText,
		DirectArgs:           namedArgumentInputsToInternal(request.DirectArgs),
		CompatibilityContent: contentPartsToInternal(request.CompatibilityContent),
	}
}

func preparedInvocationInputFromInternal(prepared invocationreturnpolicy.PreparedInvocationInput) PreparedInvocationInput {
	outer := PreparedInvocationInput{Source: InputSourceLabel(prepared.Source)}
	if prepared.ResolvedInput != nil {
		resolved := resolvedInputFromInternal(*prepared.ResolvedInput)
		outer.ResolvedInput = &resolved
	}
	if prepared.NormalizedArguments != nil {
		normalized := normalizedArgumentsFromInternal(*prepared.NormalizedArguments)
		outer.NormalizedArguments = &normalized
	}
	return outer
}

func runtimeInvocationArgumentsFromInternal(
	signature *InvocationSignatureConfig,
	normalized *invocationreturnpolicy.NormalizedArguments,
) *InvocationArguments {
	if signature == nil || normalized == nil {
		return nil
	}
	inner := invocationreturnpolicy.RuntimeInvocationArguments(
		invocationSignatureToInternal(signature),
		normalized,
	)
	if inner == nil {
		return nil
	}
	outer := &InvocationArguments{Arguments: make(map[string]InvocationArgument, len(inner.Arguments))}
	for name, argument := range inner.Arguments {
		sources := make([]InvocationArgumentSource, len(argument.Sources))
		for i, source := range argument.Sources {
			sources[i] = InvocationArgumentSource{
				Kind:   source.Kind,
				Name:   source.Name,
				Redact: source.Redact,
			}
		}
		outer.Arguments[name] = InvocationArgument{
			Values:    cloneStringSlice(argument.Values),
			ValueMode: argument.ValueMode,
			Sensitive: argument.Sensitive,
			Sources:   sources,
		}
	}
	return outer
}
