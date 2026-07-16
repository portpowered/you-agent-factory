package adapter_test

import (
	"context"
	"encoding/json"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/work"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
)

type recordingAdapter struct {
	identity adapter.Identity
	decoder  *recordingDecoder
}

func (a *recordingAdapter) Identity() adapter.Identity { return a.identity }

func (a *recordingAdapter) BuildCommand(_ context.Context, input adapter.CommandContext) (adapter.CommandBuildResult, error) {
	return adapter.CommandBuildResult{Request: workerexecution.SubprocessExecutionRequest{
		Command:    "fake-provider",
		Args:       []string{"run", input.Request.Model},
		Stdin:      []byte(input.Request.UserMessage),
		DispatchID: input.Request.Dispatch.DispatchID,
	}}, nil
}

func (a *recordingAdapter) NewDecoder(_ context.Context, _ adapter.DecoderContext) (adapter.Decoder, error) {
	a.decoder = &recordingDecoder{}
	return a.decoder, nil
}

func (a *recordingAdapter) ParseFinal(_ context.Context, input adapter.FinalParseContext) (adapter.FinalParseResult, error) {
	return adapter.FinalParseResult{Response: workerexecution.InferenceResponse{Content: string(input.CommandResult.Stdout)}}, nil
}

func (a *recordingAdapter) Capabilities(context.Context, adapter.CapabilityContext) (adapter.CapabilityResult, error) {
	return adapter.CapabilityResult{Capabilities: adapter.Capabilities{
		NativeStreaming: true, MessageDeltas: true, MessageSnapshots: true,
		ReasoningSummaries: true, ToolLifecycle: true,
		ToolOutputDeltas: true, StableItemIDs: true,
	}}, nil
}

func (a *recordingAdapter) ClassifyFailure(_ context.Context, input adapter.FailureContext) adapter.FailureResult {
	if input.CommandError == nil && input.CommandResult.ExitCode == 0 {
		return adapter.FailureResult{}
	}
	return adapter.FailureResult{Failure: &adapter.FailureFacts{
		Family:  workerexecution.WorkFailureFamilyRetryable,
		Type:    workerexecution.WorkFailureTypeInternalServerError,
		Message: "provider failed",
		Retry:   adapter.RetryGuidance{Retryable: true},
	}}
}

type recordingDecoder struct {
	streams []adapter.OutputStream
	buffer  []byte
	flushed adapter.FlushReason
}

func (d *recordingDecoder) Observe(_ context.Context, observation adapter.Observation) (adapter.DecodeResult, error) {
	d.streams = append(d.streams, observation.Stream)
	d.buffer = append(d.buffer, observation.Chunk...)
	return adapter.DecodeResult{}, nil
}

func (d *recordingDecoder) Flush(_ context.Context, input adapter.FlushContext) (adapter.DecodeResult, error) {
	d.flushed = input.Reason
	return adapter.DecodeResult{Drafts: []responseevents.Draft{{
		Kind:  responseevents.KindMessage,
		Phase: responseevents.PhaseCompleted,
		Provenance: responseevents.Provenance{
			Provider: "fake", Delivery: responseevents.DeliveryNativeStream,
			Representation: responseevents.RepresentationSnapshot,
			Fidelity:       responseevents.FidelityNormalized,
		},
		Payload: json.RawMessage(`{"role":"assistant","contentBlocks":[{"kind":"TEXT","text":"done"}]}`),
	}}}, nil
}

func TestAdapterContract_BuildsCommandWithoutExecutingIt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fake := &recordingAdapter{identity: "fake"}
	command, err := fake.BuildCommand(ctx, adapter.CommandContext{Request: workerexecution.ProviderInferenceRequest{
		Dispatch: work.WorkDispatch{DispatchID: "dispatch-1"},
		Model:    "model-1", UserMessage: "private prompt",
	}})
	if err != nil {
		t.Fatalf("BuildCommand() error = %v", err)
	}
	if command.Request.Command != "fake-provider" || command.Request.DispatchID != "dispatch-1" {
		t.Fatalf("command request = %#v", command.Request)
	}
}

func TestDecoderContract_PreservesStateAcrossIndependentStreamsAndFlush(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fake := &recordingAdapter{identity: "fake"}
	decoder, err := fake.NewDecoder(ctx, adapter.DecoderContext{RunID: "run-1", DispatchID: "dispatch-1"})
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}
	for _, observation := range []adapter.Observation{
		{Stream: adapter.OutputStreamStdout, Chunk: []byte("out")},
		{Stream: adapter.OutputStreamStderr, Chunk: []byte("err")},
	} {
		if _, err := decoder.Observe(ctx, observation); err != nil {
			t.Fatalf("Observe(%q) error = %v", observation.Stream, err)
		}
	}
	decoded, err := decoder.Flush(ctx, adapter.FlushContext{Reason: adapter.FlushReasonCompleted})
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if len(fake.decoder.streams) != 2 || fake.decoder.streams[0] != adapter.OutputStreamStdout || fake.decoder.streams[1] != adapter.OutputStreamStderr {
		t.Fatalf("observed streams = %#v", fake.decoder.streams)
	}
	if fake.decoder.flushed != adapter.FlushReasonCompleted || len(decoded.Drafts) != 1 {
		t.Fatalf("flush result = %#v, reason = %q", decoded, fake.decoder.flushed)
	}
	if err := responseevents.ValidateDraft(decoded.Drafts[0]); err != nil {
		t.Fatalf("ValidateDraft() error = %v", err)
	}
}

func TestAdapterContract_ParsesFinalAndReportsCapabilities(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fake := &recordingAdapter{identity: "fake"}
	final, err := fake.ParseFinal(ctx, adapter.FinalParseContext{
		CommandResult: workerprocess.CommandResult{Stdout: []byte("done")},
	})
	if err != nil || final.Response.Content != "done" {
		t.Fatalf("ParseFinal() = %#v, %v", final, err)
	}
	capabilities, err := fake.Capabilities(ctx, adapter.CapabilityContext{})
	if err != nil || !capabilities.Capabilities.NativeStreaming || capabilities.Capabilities.FinalOnly {
		t.Fatalf("Capabilities() = %#v, %v", capabilities, err)
	}
}

func TestAdapterContract_ClassifiesFailureWithoutExecutingRetryPolicy(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fake := &recordingAdapter{identity: "fake"}
	failure := fake.ClassifyFailure(ctx, adapter.FailureContext{
		CommandResult: workerprocess.CommandResult{ExitCode: 1},
	})
	if failure.Failure == nil || !failure.Failure.Retry.Retryable {
		t.Fatalf("ClassifyFailure() = %#v", failure)
	}
}

var _ adapter.Adapter = (*recordingAdapter)(nil)
