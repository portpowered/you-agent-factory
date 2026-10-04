package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/requestadmission"
)

type fakeContent func(context.Context, []requestadmission.ContentPart) ([]requestadmission.ContentPart, error)

func (f fakeContent) PrepareWorkContent(ctx context.Context, parts []requestadmission.ContentPart) ([]requestadmission.ContentPart, error) {
	return f(ctx, parts)
}

type fakeRequest func(context.Context, requestadmission.WorkRequestPreparation) (requestadmission.Request, error)

func (f fakeRequest) PrepareWorkRequest(ctx context.Context, input requestadmission.WorkRequestPreparation) (requestadmission.Request, error) {
	return f(ctx, input)
}
func TestContentMappingDetachesAndPreservesOrder(t *testing.T) {
	t.Parallel()
	input := []work.WorkContentPart{{Type: work.WorkContentPartTypeJSON, JSON: []byte(`{"input":true}`), Metadata: map[string]any{"tags": []string{"first"}}}, {Type: work.WorkContentPartTypeText, Text: "second"}}
	adapter := ContentPreparationAdapter{Inner: fakeContent(func(ctx context.Context, parts []requestadmission.ContentPart) ([]requestadmission.ContentPart, error) {
		if ctx != t.Context() || len(parts) != 2 || parts[1].Text != "second" {
			t.Fatalf("mapped input = %#v", parts)
		}
		parts[0].JSON[0] = '['
		parts[0].Metadata["tags"].([]string)[0] = "changed"
		return parts, nil
	})}
	got, err := adapter.PrepareWorkContent(t.Context(), input)
	if err != nil || string(input[0].JSON) != `{"input":true}` || input[0].Metadata["tags"].([]string)[0] != "first" || got[1].Text != "second" {
		t.Fatalf("content = %#v, input = %#v, error = %v", got, input, err)
	}
}
func TestRequestMappingPreservesIdentityLineageAndTypedErrors(t *testing.T) {
	t.Parallel()
	input := work.WorkRequestPreparation{DefaultWorkTypeID: "task", CanonicalJSON: []byte("original"), Request: work.WorkRequest{RequestID: "request", Relations: []work.WorkRelation{{Type: work.WorkRelationType("PARENT_CHILD"), SourceWorkName: "draft", TargetWorkName: "parent"}}, Works: []work.Work{{Name: "draft", WorkTypeID: "task", PreviousChainingTraceIDs: []string{"ancestor"}, Tags: map[string]string{"tag": "value"}, RuntimeRelations: []work.Relation{{TargetWorkID: "parent", Type: work.RelationType("PARENT_CHILD")}}, InvocationArguments: &work.InvocationArguments{Arguments: map[string]work.InvocationArgument{"input": {Values: []string{"value"}, Sources: []work.InvocationArgumentSource{{Kind: "NAMED", Name: "input", Redact: true}}}}}, Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeJSON, JSON: []byte(`{"value":true}`), Metadata: map[string]any{"nested": map[string]any{"rows": []any{"one"}}, "bytes": []byte("raw"), "tags": map[string]string{"key": "value"}, "sets": map[string][]string{"key": []string{"value"}}}}}}}}}
	adapter := RequestPreparationServiceAdapter{Inner: fakeRequest(func(ctx context.Context, mapped requestadmission.WorkRequestPreparation) (requestadmission.Request, error) {
		if ctx != t.Context() || mapped.DefaultWorkTypeID != "task" || string(mapped.CanonicalJSON) != "original" {
			t.Fatalf("mapped input = %#v", mapped)
		}
		return mapped.Request, nil
	})}
	got, err := adapter.PrepareWorkRequest(t.Context(), input)
	if err != nil || !reflect.DeepEqual(got, input.Request) {
		t.Fatalf("request = %#v, error = %v", got, err)
	}
	got.Works[0].PreviousChainingTraceIDs[0] = "changed"
	if input.Request.Works[0].PreviousChainingTraceIDs[0] != "ancestor" {
		t.Fatal("mapped lineage aliases caller")
	}
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded, errors.New("invalid content")} {
		adapter.Inner = fakeRequest(func(context.Context, requestadmission.WorkRequestPreparation) (requestadmission.Request, error) {
			return requestadmission.Request{}, &requestadmission.RequestPreparationError{Message: "safe validation failure", Cause: cause}
		})
		_, err := adapter.PrepareWorkRequest(t.Context(), input)
		var typed *work.RequestPreparationError
		if !errors.As(err, &typed) || typed.Message != "safe validation failure" || !errors.Is(err, cause) {
			t.Fatalf("mapped error = %v", err)
		}
	}
}
