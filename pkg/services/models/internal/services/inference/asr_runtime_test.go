package inference_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai/codecs"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	asrruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/runtime"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

func TestASRInvocationRuntimeMapsRequestAndReturnsNamedOutputs(t *testing.T) {
	t.Parallel()

	backend := &recordingASRBackend{response: codecs.ASRResponse{
		Text:     "hello world",
		Segments: []codecs.ASRSegment{{ID: 0, Start: 0, End: 1500, Text: "hello world"}},
	}}
	runtime, err := asrruntime.New(backend.transcribe)
	if err != nil {
		t.Fatalf("asrruntime.New() error = %v", err)
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
			runtime, err := asrruntime.New(backend.transcribe)
			if err != nil {
				t.Fatalf("asrruntime.New() error = %v", err)
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

func TestASRInvocationRuntimeFailureEvidenceCollapsesBackendCauses(t *testing.T) {
	t.Parallel()

	firstBackendCause := errors.New("rpc status=Unavailable path=C:\\private\\whisper-a token=secret-a")
	secondBackendCause := errors.New("rpc status=Internal path=C:\\private\\whisper-b token=secret-b")
	backendFailures := []struct {
		failure *models.InvocationFailure
		cause   error
	}{
		{
			failure: &models.InvocationFailure{
				Class: models.InvocationFailureClassBackendProtocol, Operation: models.OperationASR,
				Message: "ASR backend request failed", Cause: firstBackendCause,
			},
			cause: firstBackendCause,
		},
		{
			failure: &models.InvocationFailure{
				Class: models.InvocationFailureClassBackendProtocol, Operation: models.OperationASR,
				Message: "ASR backend request failed", Cause: secondBackendCause,
			},
			cause: secondBackendCause,
		},
	}
	var firstDigest string
	for index, backendFailure := range backendFailures {
		backend := &recordingASRBackend{err: backendFailure.failure}
		if !errors.Is(backendFailure.failure, backendFailure.cause) {
			t.Fatalf("test backend failure = %v, want its private transport cause", backendFailure.failure)
		}
		runtime, err := asrruntime.New(backend.transcribe)
		if err != nil {
			t.Fatalf("asrruntime.New() error = %v", err)
		}
		result, invocationErr := runtime.Invoke(
			t.Context(),
			inference.InvocationRuntimeRequest{Request: asrRuntimeRequest()},
		)
		if result.Content != nil || invocationErr == nil {
			t.Fatalf("failed invocation = result %#v, error %v; want no output and typed failure", result, invocationErr)
		}
		var failure *models.InvocationFailure
		if !errors.As(invocationErr, &failure) || failure.Class != models.InvocationFailureClassBackendProtocol {
			t.Fatalf("invocation error = %v, failure = %#v, want backend protocol failure", invocationErr, failure)
		}
		if invocationErr.Error() != "ASR backend invocation failed" || !errors.Is(invocationErr, models.ErrInferenceFailed) {
			t.Fatalf("invocation error identity = %q, want the stable ASR failure and ErrInferenceFailed", invocationErr.Error())
		}
		if errors.Is(invocationErr, backendFailure.cause) {
			t.Fatalf("invocation error unexpectedly retains backend cause %q", backendFailure.cause)
		}

		diagnostic := modelseffects.ProjectRuntimeFailure(
			modelseffects.WrapRuntimeFailure(modelseffects.RuntimeStageInvoke, invocationErr), 0,
		)
		if diagnostic.CauseSHA256 != modelseffects.RuntimeCauseSHA256(models.ErrInferenceFailed) {
			t.Fatalf("cause digest = %q, want the generic inference failure digest", diagnostic.CauseSHA256)
		}
		encodedDiagnostic, err := json.Marshal(diagnostic)
		if err != nil {
			t.Fatalf("marshal runtime diagnostic: %v", err)
		}
		for _, privateDetail := range []string{"whisper-a", "whisper-b", "secret-a", "secret-b", "Unavailable", "Internal"} {
			if strings.Contains(string(encodedDiagnostic), privateDetail) {
				t.Fatalf("runtime diagnostic leaked %q: %s", privateDetail, encodedDiagnostic)
			}
		}
		if index == 0 {
			firstDigest = diagnostic.CauseSHA256
		} else if diagnostic.CauseSHA256 != firstDigest {
			t.Fatalf("different backend failures produced digests %q and %q", firstDigest, diagnostic.CauseSHA256)
		}
	}
}

func TestASRInvocationRuntimeHonorsCancellationBeforeAndDuringBackendCall(t *testing.T) {
	t.Parallel()

	backend := &recordingASRBackend{waitForCancellation: true}
	runtime, err := asrruntime.New(backend.transcribe)
	if err != nil {
		t.Fatalf("asrruntime.New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err := runtime.Invoke(ctx, inference.InvocationRuntimeRequest{Request: asrRuntimeRequest()})
	if !errors.Is(err, context.Canceled) || result.Content != nil || backend.calls != 0 {
		t.Fatalf("pre-cancelled invocation = result %#v, error %v, backend calls %d; want cancellation and no call", result, err, backend.calls)
	}

	backend = &recordingASRBackend{waitForCancellation: true}
	runtime, err = asrruntime.New(backend.transcribe)
	if err != nil {
		t.Fatalf("asrruntime.New() error = %v", err)
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

	if _, err := asrruntime.New(nil); !errors.Is(err, models.ErrInvalidInferenceDependencies) {
		t.Fatalf("asrruntime.New(nil) error = %v, want ErrInvalidInferenceDependencies", err)
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

func (backend *recordingASRBackend) transcribe(ctx context.Context, request codecs.ASRRequest) (codecs.ASRResponse, []models.InferenceArtifact, error) {
	backend.calls++
	backend.request = request
	if backend.started != nil {
		close(backend.started)
	}
	if backend.waitForCancellation {
		<-ctx.Done()
		return codecs.ASRResponse{}, nil, ctx.Err()
	}
	return backend.response, nil, backend.err
}
