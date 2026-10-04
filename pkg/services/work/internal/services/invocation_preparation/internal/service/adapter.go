// Package service preserves the existing public Work/private policy compatibility
// boundary. These mappings retain detached values, ordered content and typed
// errors while using completed policies. Representation consolidation is outside
// this construction change; it can remove these adapters with the old private models.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/invocationreturnpolicy"
)

type InvocationInputPreparationAdapter struct {
	Inner invocationreturnpolicy.InvocationInputPreparation
}

func (adapter InvocationInputPreparationAdapter) PrepareInvocationInput(
	ctx context.Context,
	request work.InvocationInputPreparationRequest,
) (work.PreparedInvocationInput, error) {
	prepared, err := adapter.Inner.PrepareInvocationInput(
		ctx,
		invocationInputPreparationRequestToInternal(request),
	)
	if err != nil {
		return work.PreparedInvocationInput{}, mapInvocationReturnPolicyError(err)
	}
	return preparedInvocationInputFromInternal(prepared), nil
}

func argumentErrorFromInternal(err *invocationreturnpolicy.ArgumentError) *work.ArgumentError {
	if err == nil {
		return nil
	}
	return &work.ArgumentError{
		Code:       work.ArgumentErrorCode(err.Code),
		Message:    err.Message,
		Parameter:  err.Parameter,
		Argument:   err.Argument,
		SourceKind: work.ArgumentSourceKind(err.SourceKind),
	}
}

func argumentSourcesFromInternal(sources []invocationreturnpolicy.ArgumentSource) []work.ArgumentSource {
	if len(sources) == 0 {
		return nil
	}
	converted := make([]work.ArgumentSource, len(sources))
	for i, source := range sources {
		converted[i] = work.ArgumentSource{
			Kind:   work.ArgumentSourceKind(source.Kind),
			Name:   source.Name,
			Redact: source.Redact,
		}
	}
	return converted
}

func cloneAnyMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	clone := make(map[string]any, len(values))
	for key, value := range values {
		clone[key] = cloneAnyValue(value)
	}
	return clone
}

func cloneAnySlice(values []any) []any {
	if len(values) == 0 {
		return nil
	}
	clone := make([]any, len(values))
	for index, value := range values {
		clone[index] = cloneAnyValue(value)
	}
	return clone
}

func cloneAnyValue(value any) any {
	switch typed := value.(type) {
	case []any:
		return cloneAnySlice(typed)
	case map[string]any:
		return cloneAnyMap(typed)
	case []string:
		return append([]string(nil), typed...)
	case []byte:
		return append([]byte(nil), typed...)
	case map[string]string:
		return cloneStringMap(typed)
	case map[string][]string:
		return cloneStringSliceMap(typed)
	default:
		return value
	}
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

func contentPartsFromInternal(parts []invocationreturnpolicy.ContentPart) []work.WorkContentPart {
	if len(parts) == 0 {
		return nil
	}
	converted := make([]work.WorkContentPart, len(parts))
	for i, part := range parts {
		converted[i] = work.WorkContentPart{
			Type:        work.WorkContentPartType(part.Type),
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

func contentPartsToInternal(parts []work.WorkContentPart) []invocationreturnpolicy.ContentPart {
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

func inputErrorFromInternal(err *invocationreturnpolicy.InputError) *work.InputError {
	if err == nil {
		return nil
	}
	conflicts := make([]work.InputSourceLabel, len(err.ConflictingSources))
	for i, source := range err.ConflictingSources {
		conflicts[i] = work.InputSourceLabel(source)
	}
	return &work.InputError{
		Code:               work.InputErrorCode(err.Code),
		Message:            err.Message,
		Source:             work.InputSourceLabel(err.Source),
		ConflictingSources: conflicts,
	}
}

func invocationBindingsToInternal(bindings []work.InvocationParameterBindingConfig) []invocationreturnpolicy.InvocationParameterBindingConfig {
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

func invocationInputPreparationRequestToInternal(request work.InvocationInputPreparationRequest) invocationreturnpolicy.InvocationInputPreparationRequest {
	return invocationreturnpolicy.InvocationInputPreparationRequest{
		Arguments:            cloneStringSlice(request.Arguments),
		Signature:            invocationSignatureToInternal(request.Signature),
		StdinText:            request.StdinText,
		FilePath:             request.FilePath,
		DirectArgs:           namedArgumentInputsToInternal(request.DirectArgs),
		CompatibilityContent: contentPartsToInternal(request.CompatibilityContent),
	}
}

func invocationSignatureToInternal(signature *work.InvocationSignatureConfig) *invocationreturnpolicy.InvocationSignatureConfig {
	if signature == nil {
		return nil
	}
	Inner := &invocationreturnpolicy.InvocationSignatureConfig{
		UnknownNamedArgumentPolicy: signature.UnknownNamedArgumentPolicy,
	}
	for _, parameter := range signature.Parameters {
		Inner.Parameters = append(Inner.Parameters, invocationreturnpolicy.InvocationParameterConfig{
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
	return Inner
}

func mapInvocationReturnPolicyError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, invocationreturnpolicy.ErrUnsupportedReturnPolicy) {
		return work.ErrUnsupportedReturnPolicy
	}
	if errors.Is(err, invocationreturnpolicy.ErrInvalidInvocationInput) {
		var inputErr *invocationreturnpolicy.InputError
		if errors.As(err, &inputErr) {
			return fmt.Errorf("%w: %w", work.ErrInvalidInvocationInput, inputErrorFromInternal(inputErr))
		}
		var argumentErr *invocationreturnpolicy.ArgumentError
		if errors.As(err, &argumentErr) {
			return fmt.Errorf("%w: %w", work.ErrInvalidInvocationInput, argumentErrorFromInternal(argumentErr))
		}
		var contentErr *invocationreturnpolicy.TextContentValidationError
		if errors.As(err, &contentErr) {
			return fmt.Errorf("%w: %w", work.ErrInvalidInvocationInput, &work.TextContentValidationError{Message: contentErr.Message})
		}
		return fmt.Errorf("%w: %w", work.ErrInvalidInvocationInput, err)
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
		return &work.TextContentValidationError{Message: contentErr.Message}
	}
	var primaryErr *invocationreturnpolicy.PrimaryResultError
	if errors.As(err, &primaryErr) {
		return primaryResultErrorFromInternal(primaryErr)
	}
	return err
}

func namedArgumentInputsToInternal(inputs []work.NamedArgumentInput) []invocationreturnpolicy.NamedArgumentInput {
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

func normalizedArgumentsFromInternal(input invocationreturnpolicy.NormalizedArguments) work.NormalizedArguments {
	outer := work.NormalizedArguments{
		Arguments:        make(map[string]work.NormalizedArgument, len(input.Arguments)),
		UnknownNamedArgs: make(map[string][]string, len(input.UnknownNamedArgs)),
	}
	for name, argument := range input.Arguments {
		outer.Arguments[name] = work.NormalizedArgument{
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

func preparedInvocationInputFromInternal(prepared invocationreturnpolicy.PreparedInvocationInput) work.PreparedInvocationInput {
	outer := work.PreparedInvocationInput{Source: work.InputSourceLabel(prepared.Source)}
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

func primaryResultErrorFromInternal(err *invocationreturnpolicy.PrimaryResultError) *work.PrimaryResultError {
	if err == nil {
		return nil
	}
	return &work.PrimaryResultError{
		Code:      work.PrimaryResultErrorCode(err.Code),
		Message:   err.Message,
		RequestID: err.RequestID,
		Policy:    err.Policy,
		Context: work.InvocationFailureContext{
			SessionID:       err.Context.SessionID,
			WorkID:          err.Context.WorkID,
			WorkName:        err.Context.WorkName,
			WorkState:       err.Context.WorkState,
			ApprovalID:      err.Context.ApprovalID,
			DispatchID:      err.Context.DispatchID,
			WorkstationID:   err.Context.WorkstationID,
			WorkstationName: err.Context.WorkstationName,
			Decisions:       append([]string(nil), err.Context.Decisions...),
		},
	}
}

func resolvedInputFromInternal(input invocationreturnpolicy.ResolvedInput) work.ResolvedInput {
	return work.ResolvedInput{
		Source:  work.InputSourceLabel(input.Source),
		Text:    input.Text,
		Content: contentPartsFromInternal(input.Content),
	}
}
