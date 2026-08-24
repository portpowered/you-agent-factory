package inference_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai/codecs"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

func TestASRInvocationRuntimeMapsRequestAndReturnsNamedOutputs(t *testing.T) {
	t.Parallel()

	backend := &recordingASRBackend{response: codecs.ASRResponse{
		Text:     "hello world",
		Segments: []codecs.ASRSegment{{ID: 0, Start: 0, End: 1500, Text: "hello world"}},
	}}
	runtime, err := inference.NewASRInvocationRuntime(backend)
	if err != nil {
		t.Fatalf("NewASRInvocationRuntime() error = %v", err)
	}
	request := asrRuntimeRequest()
	result, err := runtime.Invoke(t.Context(), inference.InvocationRuntimeRequest{Request: request})
	if err != nil {
		t.Fatalf("ASRInvocationRuntime.Invoke() error = %v", err)
	}
	if len(result.Content) != 2 || result.Content[0].Name != "transcript" || result.Content[1].Name != "segments" {
		t.Fatalf("ASR runtime outputs = %#v, want transcript then segments", result.Content)
	}
	if result.Content[0].Content != "hello world" || result.Content[0].MediaType != "text/plain" {
		t.Fatalf("transcript output = %#v", result.Content[0])
	}
	if result.Content[1].Content != `[{"id":0,"start":0,"end":1500,"text":"hello world"}]` || result.Content[1].MediaType != "application/json" {
		t.Fatalf("segments output = %#v", result.Content[1])
	}
	if string(backend.request.Audio) != string([]byte{0, 1, 255, 127}) || backend.request.MediaType != "audio/wav" || backend.request.Prompt != "meeting" {
		t.Fatalf("backend request = %#v, want exact audio/media/prompt", backend.request)
	}
}

func TestASRInvocationRuntimeClassifiesMalformedAndBackendFailuresAtomically(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		backend   recordingASRBackend
		wantClass models.InvocationFailureClass
		wantLeak  string
	}{
		{
			name: "malformed response",
			backend: recordingASRBackend{response: codecs.ASRResponse{
				Text: "hello", Segments: nil,
			}},
			wantClass: models.InvocationFailureClassMalformedResponse,
		},
		{
			name:      "backend failure",
			backend:   recordingASRBackend{err: errors.New("private protocol endpoint")},
			wantClass: models.InvocationFailureClassBackendProtocol,
			wantLeak:  "private protocol endpoint",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := test.backend
			runtime, err := inference.NewASRInvocationRuntime(&backend)
			if err != nil {
				t.Fatalf("NewASRInvocationRuntime() error = %v", err)
			}
			result, err := runtime.Invoke(t.Context(), inference.InvocationRuntimeRequest{Request: asrRuntimeRequest()})
			if result.Content != nil {
				t.Fatalf("failed ASR invocation returned partial content: %#v", result.Content)
			}
			var failure *models.InvocationFailure
			if !errors.As(err, &failure) || failure.Class != test.wantClass {
				t.Fatalf("error = %v, failure = %#v, want class %q", err, failure, test.wantClass)
			}
			if test.wantLeak != "" && strings.Contains(err.Error(), test.wantLeak) {
				t.Fatalf("backend detail leaked through typed error: %v", err)
			}
		})
	}
}

func TestASRInvocationRuntimeHonorsCancellationBeforeAndDuringBackendCall(t *testing.T) {
	t.Parallel()

	backend := &recordingASRBackend{waitForCancellation: true}
	runtime, err := inference.NewASRInvocationRuntime(backend)
	if err != nil {
		t.Fatalf("NewASRInvocationRuntime() error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err := runtime.Invoke(ctx, inference.InvocationRuntimeRequest{Request: asrRuntimeRequest()})
	if !errors.Is(err, context.Canceled) || result.Content != nil || backend.calls != 0 {
		t.Fatalf("pre-cancelled invocation = result %#v, error %v, backend calls %d; want cancellation and no call", result, err, backend.calls)
	}

	backend = &recordingASRBackend{waitForCancellation: true}
	runtime, err = inference.NewASRInvocationRuntime(backend)
	if err != nil {
		t.Fatalf("NewASRInvocationRuntime() error = %v", err)
	}
	ctx, cancel = context.WithCancel(t.Context())
	backend.started = make(chan struct{})
	resultCh := make(chan struct {
		result inference.InvocationRuntimeResult
		err    error
	}, 1)
	go func() {
		result, err := runtime.Invoke(ctx, inference.InvocationRuntimeRequest{Request: asrRuntimeRequest()})
		resultCh <- struct {
			result inference.InvocationRuntimeResult
			err    error
		}{result: result, err: err}
	}()
	<-backend.started
	cancel()
	invocation := <-resultCh
	if !errors.Is(invocation.err, context.Canceled) || invocation.result.Content != nil {
		t.Fatalf("in-flight cancellation = result %#v, error %v; want cancellation and no content", invocation.result, invocation.err)
	}
}

func TestASRInvocationRuntimeRejectsNilBackend(t *testing.T) {
	t.Parallel()

	if _, err := inference.NewASRInvocationRuntime(nil); !errors.Is(err, models.ErrInvalidInferenceDependencies) {
		t.Fatalf("NewASRInvocationRuntime(nil) error = %v, want ErrInvalidInferenceDependencies", err)
	}
}

func asrRuntimeRequest() models.InvokeModelRequest {
	return models.InvokeModelRequest{
		Operation: models.OperationASR,
		Inputs: []models.InferenceInput{{
			Name: "audio", Modality: models.ModalityAudio,
			ContentType: "audio/wav", MediaType: "audio/wav",
			Content: string([]byte{0, 1, 255, 127}),
		}, {
			Name: "prompt", Modality: models.ModalityText,
			ContentType: "text/plain", MediaType: "text/plain", Content: "meeting",
		}},
	}
}

type recordingASRBackend struct {
	request             codecs.ASRRequest
	response            codecs.ASRResponse
	err                 error
	waitForCancellation bool
	started             chan struct{}
	calls               int
}

func (backend *recordingASRBackend) Transcribe(ctx context.Context, request codecs.ASRRequest) (codecs.ASRResponse, error) {
	backend.calls++
	backend.request = request
	if backend.started != nil {
		close(backend.started)
	}
	if backend.waitForCancellation {
		<-ctx.Done()
		return codecs.ASRResponse{}, ctx.Err()
	}
	return backend.response, backend.err
}
