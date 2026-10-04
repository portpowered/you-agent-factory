// Package service preserves the existing public Work/private policy compatibility
// boundary. These mappings retain detached values, ordered content and typed
// errors while using completed policies. Representation consolidation is outside
// this construction change; it can remove these adapters with the old private models.
package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/requestadmission"
)

type ContentPreparationAdapter struct {
	Inner requestadmission.ContentPreparation
}

func (adapter ContentPreparationAdapter) PrepareWorkContent(
	ctx context.Context,
	content []work.WorkContentPart,
) ([]work.WorkContentPart, error) {
	prepared, err := adapter.Inner.PrepareWorkContent(
		ctx,
		workContentPartsToAdmission(content),
	)
	if err != nil {
		return nil, mapRequestPreparationError(err)
	}
	return workContentPartsFromAdmission(prepared), nil
}

type RequestPreparationServiceAdapter struct {
	Inner requestadmission.RequestPreparationService
}

func (a RequestPreparationServiceAdapter) PrepareWorkRequest(
	ctx context.Context,
	input work.WorkRequestPreparation,
) (work.WorkRequest, error) {
	prepared, err := a.Inner.PrepareWorkRequest(ctx, requestadmission.WorkRequestPreparation{
		Request:           workRequestToAdmission(input.Request),
		CanonicalJSON:     input.CanonicalJSON,
		DefaultWorkTypeID: input.DefaultWorkTypeID,
	})
	if err != nil {
		return work.WorkRequest{}, mapRequestPreparationError(err)
	}
	return workRequestFromAdmission(prepared), nil
}

type RequestPreparationContentBridge struct {
	Content work.ContentPreparation
}

func (b RequestPreparationContentBridge) PrepareWorkContent(
	ctx context.Context,
	content []requestadmission.ContentPart,
) ([]requestadmission.ContentPart, error) {
	prepared, err := b.Content.PrepareWorkContent(ctx, workContentPartsFromAdmission(content))
	if err != nil {
		return nil, err
	}
	return workContentPartsToAdmission(prepared), nil
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

func invocationArgumentSourcesFromAdmission(sources []requestadmission.InvocationArgumentSource) []work.InvocationArgumentSource {
	converted := make([]work.InvocationArgumentSource, len(sources))
	for i, source := range sources {
		converted[i] = work.InvocationArgumentSource{
			Kind:   source.Kind,
			Name:   source.Name,
			Redact: source.Redact,
		}
	}
	return converted
}

func invocationArgumentSourcesToAdmission(sources []work.InvocationArgumentSource) []requestadmission.InvocationArgumentSource {
	converted := make([]requestadmission.InvocationArgumentSource, len(sources))
	for i, source := range sources {
		converted[i] = requestadmission.InvocationArgumentSource{
			Kind:   source.Kind,
			Name:   source.Name,
			Redact: source.Redact,
		}
	}
	return converted
}

func invocationArgumentsFromAdmission(args *requestadmission.InvocationArguments) *work.InvocationArguments {
	if args == nil || len(args.Arguments) == 0 {
		return nil
	}
	clone := &work.InvocationArguments{
		Arguments: make(map[string]work.InvocationArgument, len(args.Arguments)),
	}
	for name, argument := range args.Arguments {
		next := work.InvocationArgument{
			Values:    cloneStringSlice(argument.Values),
			ValueMode: argument.ValueMode,
			Sensitive: argument.Sensitive,
		}
		if len(argument.Sources) > 0 {
			next.Sources = append([]work.InvocationArgumentSource(nil), invocationArgumentSourcesFromAdmission(argument.Sources)...)
		}
		clone.Arguments[name] = next
	}
	return clone
}

func invocationArgumentsToAdmission(args *work.InvocationArguments) *requestadmission.InvocationArguments {
	if args == nil || len(args.Arguments) == 0 {
		return nil
	}
	clone := &requestadmission.InvocationArguments{
		Arguments: make(map[string]requestadmission.InvocationArgument, len(args.Arguments)),
	}
	for name, argument := range args.Arguments {
		next := requestadmission.InvocationArgument{
			Values:    cloneStringSlice(argument.Values),
			ValueMode: argument.ValueMode,
			Sensitive: argument.Sensitive,
		}
		if len(argument.Sources) > 0 {
			next.Sources = append([]requestadmission.InvocationArgumentSource(nil), invocationArgumentSourcesToAdmission(argument.Sources)...)
		}
		clone.Arguments[name] = next
	}
	return clone
}

func mapRequestPreparationError(err error) error {
	if err == nil {
		return nil
	}
	var Inner *requestadmission.RequestPreparationError
	if errors.As(err, &Inner) {
		return &work.RequestPreparationError{Message: Inner.Message, Cause: Inner.Cause}
	}
	return err
}

func relationsFromAdmission(relations []requestadmission.Relation) []work.Relation {
	if len(relations) == 0 {
		return nil
	}
	converted := make([]work.Relation, len(relations))
	for i, rel := range relations {
		converted[i] = work.Relation{
			Type:          work.RelationType(rel.Type),
			TargetWorkID:  rel.TargetWorkID,
			RequiredState: rel.RequiredState,
		}
	}
	return converted
}

func relationsToAdmission(relations []work.Relation) []requestadmission.Relation {
	if len(relations) == 0 {
		return nil
	}
	converted := make([]requestadmission.Relation, len(relations))
	for i, rel := range relations {
		converted[i] = requestadmission.Relation{
			Type:          requestadmission.RelationType(rel.Type),
			TargetWorkID:  rel.TargetWorkID,
			RequiredState: rel.RequiredState,
		}
	}
	return converted
}

func workContentPartsFromAdmission(parts []requestadmission.ContentPart) []work.WorkContentPart {
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

func workContentPartsToAdmission(parts []work.WorkContentPart) []requestadmission.ContentPart {
	if len(parts) == 0 {
		return nil
	}
	converted := make([]requestadmission.ContentPart, len(parts))
	for i, part := range parts {
		converted[i] = requestadmission.ContentPart{
			Type:        requestadmission.ContentPartType(part.Type),
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

func workFromAdmission(item requestadmission.Work) work.Work {
	return work.Work{
		Name:                     item.Name,
		WorkID:                   item.WorkID,
		RequestID:                item.RequestID,
		WorkTypeID:               item.WorkTypeID,
		State:                    item.State,
		ChainingTraceDepth:       item.ChainingTraceDepth,
		CurrentChainingTraceID:   item.CurrentChainingTraceID,
		PreviousChainingTraceIDs: cloneStringSlice(item.PreviousChainingTraceIDs),
		TraceID:                  item.TraceID,
		Content:                  workContentPartsFromAdmission(item.Content),
		Payload:                  item.Payload,
		Tags:                     cloneStringMap(item.Tags),
		ExecutionID:              item.ExecutionID,
		RuntimeRelations:         relationsFromAdmission(item.RuntimeRelations),
		InvocationArguments:      invocationArgumentsFromAdmission(item.InvocationArguments),
	}
}

func workRelationFromAdmission(rel requestadmission.WorkRelation) work.WorkRelation {
	return work.WorkRelation{
		Type:           work.WorkRelationType(rel.Type),
		SourceWorkName: rel.SourceWorkName,
		TargetWorkID:   rel.TargetWorkID,
		TargetWorkName: rel.TargetWorkName,
		RequiredState:  rel.RequiredState,
	}
}

func workRelationToAdmission(rel work.WorkRelation) requestadmission.WorkRelation {
	return requestadmission.WorkRelation{
		Type:           requestadmission.WorkRelationType(rel.Type),
		SourceWorkName: rel.SourceWorkName,
		TargetWorkID:   rel.TargetWorkID,
		TargetWorkName: rel.TargetWorkName,
		RequiredState:  rel.RequiredState,
	}
}

func workRequestFromAdmission(req requestadmission.Request) work.WorkRequest {
	outer := work.WorkRequest{
		RequestID:              req.RequestID,
		CurrentChainingTraceID: req.CurrentChainingTraceID,
		Type:                   work.WorkRequestType(req.Type),
	}
	for _, item := range req.Works {
		outer.Works = append(outer.Works, workFromAdmission(item))
	}
	for _, rel := range req.Relations {
		outer.Relations = append(outer.Relations, workRelationFromAdmission(rel))
	}
	return outer
}

func workRequestToAdmission(req work.WorkRequest) requestadmission.Request {
	Inner := requestadmission.Request{
		RequestID:              req.RequestID,
		CurrentChainingTraceID: req.CurrentChainingTraceID,
		Type:                   requestadmission.RequestType(req.Type),
	}
	for _, item := range req.Works {
		Inner.Works = append(Inner.Works, workToAdmission(item))
	}
	for _, rel := range req.Relations {
		Inner.Relations = append(Inner.Relations, workRelationToAdmission(rel))
	}
	return Inner
}

func workToAdmission(item work.Work) requestadmission.Work {
	return requestadmission.Work{
		Name:                     item.Name,
		WorkID:                   item.WorkID,
		RequestID:                item.RequestID,
		WorkTypeID:               item.WorkTypeID,
		State:                    item.State,
		ChainingTraceDepth:       item.ChainingTraceDepth,
		CurrentChainingTraceID:   item.CurrentChainingTraceID,
		PreviousChainingTraceIDs: cloneStringSlice(item.PreviousChainingTraceIDs),
		TraceID:                  item.TraceID,
		Content:                  workContentPartsToAdmission(item.Content),
		Payload:                  item.Payload,
		Tags:                     cloneStringMap(item.Tags),
		ExecutionID:              item.ExecutionID,
		RuntimeRelations:         relationsToAdmission(item.RuntimeRelations),
		InvocationArguments:      invocationArgumentsToAdmission(item.InvocationArguments),
	}
}
