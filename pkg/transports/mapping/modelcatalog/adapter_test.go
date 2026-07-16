package modelcatalog

import (
	"context"
	"errors"
	"testing"

	modelinference "github.com/portpowered/infinite-you/pkg/models/inference"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
)

func TestInvocationRequestFromGeneratedMapsDomainInput(t *testing.T) {
	t.Parallel()

	var contentPart factoryapi.WorkContentPart
	if err := contentPart.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeTextUpper,
		Text: "hello",
		Slot: stringPointer("text"),
	}); err != nil {
		t.Fatalf("build generated content: %v", err)
	}
	mode := factoryapi.AUDIOSTREAM
	bindings := []factoryapi.WorkstationOperationBinding{{
		Slot: "text",
		Selector: &factoryapi.WorkstationOperationBindingSelector{
			Slot: stringPointer("text"),
		},
	}}

	got := invocationRequestFromGenerated(factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Content:   &factoryapi.WorkContent{contentPart},
		Bindings:  &bindings,
		Options:   &factoryapi.ModelInvocationOptions{ResponseMode: &mode},
	})

	if got.Operation != "TTS" || got.Options == nil || got.Options.ResponseMode != modelinference.ResponseModeAudioStream {
		t.Fatalf("request identity/options = %#v, want TTS audio stream", got)
	}
	if len(got.Content) != 1 || got.Content[0].Type != work.WorkContentPartTypeText || got.Content[0].Text != "hello" {
		t.Fatalf("content = %#v, want canonical text", got.Content)
	}
	if len(got.Bindings) != 1 || got.Bindings[0].Slot != "text" ||
		got.Bindings[0].Selector == nil || got.Bindings[0].Selector.Slot != "text" {
		t.Fatalf("bindings = %#v, want canonical selector", got.Bindings)
	}
}

func TestInvocationErrorFromDomainPreservesTargetAndClassification(t *testing.T) {
	t.Parallel()

	cause := workerprovider.NewProviderError(
		workerexecution.WorkFailureTypeTimeout,
		"provider timed out",
		context.DeadlineExceeded,
	)
	err := invocationErrorFromDomain(&modelinference.TargetError{
		ModelName:  "model-a",
		WorkerName: "worker-a",
		Operation:  "TTS",
		Cause:      cause,
	}, "request-model", "request-operation")

	failure, ok := apisurface.AsInferenceFailure(err)
	if !ok || failure.Class != apisurface.InferenceFailureClassTimeout {
		t.Fatalf("error = %v, want timeout inference failure", err)
	}
	if failure.ModelName != "model-a" || failure.WorkerName != "worker-a" || failure.Operation != "TTS" {
		t.Fatalf("failure target = %#v, want domain target", failure)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline cause", err)
	}
}

func stringPointer(value string) *string { return &value }
