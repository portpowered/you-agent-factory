package inference_test

import (
	"bytes"
	"context"
	"encoding/binary"
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
	runtime, err := asrruntime.New(backend.transcribe, nil, nil)
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
			runtime, err := asrruntime.New(backend.transcribe, nil, nil)
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

func TestASRInvocationRuntimeFailureEvidenceDistinguishesBackendCauses(t *testing.T) {
	t.Parallel()

	firstDigest := runASRRuntimeFailureEvidenceCase(t, errors.New("transport unavailable private-a"))
	secondDigest := runASRRuntimeFailureEvidenceCase(t, errors.New("transport internal private-b"))
	if secondDigest == firstDigest {
		t.Fatalf("different backend failures collapsed to digests %q and %q", secondDigest, firstDigest)
	}
}

func runASRRuntimeFailureEvidenceCase(t *testing.T, cause error) string {
	t.Helper()
	failure := &models.InvocationFailure{
		Class: models.InvocationFailureClassBackendProtocol, Operation: models.OperationASR,
		Message: "ASR backend request failed", Cause: cause,
	}
	backend := &recordingASRBackend{err: failure}
	if !errors.Is(failure, cause) {
		t.Fatalf("test backend failure = %v, want its private transport cause", failure)
	}
	runtime, err := asrruntime.New(backend.transcribe, nil, nil)
	if err != nil {
		t.Fatalf("asrruntime.New() error = %v", err)
	}
	result, invocationErr := runtime.Invoke(t.Context(), inference.InvocationRuntimeRequest{Request: asrRuntimeRequest()})
	assertASRRuntimeFailure(t, result, invocationErr, cause)
	diagnostic := modelseffects.ProjectRuntimeFailure(
		modelseffects.WrapRuntimeFailure(modelseffects.RuntimeStageInvoke, invocationErr), 0,
	)
	assertASRFailureDiagnostic(t, diagnostic, cause)
	return diagnostic.CauseSHA256
}

func assertASRRuntimeFailure(
	t *testing.T,
	result inference.InvocationRuntimeResult,
	err error,
	cause error,
) {
	t.Helper()
	if result.Content != nil || err == nil {
		t.Fatalf("failed invocation = result %#v, error %v; want no output and typed failure", result, err)
	}
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassBackendProtocol {
		t.Fatalf("invocation error = %v, failure = %#v, want backend protocol failure", err, failure)
	}
	if err.Error() != "ASR backend request failed" || !errors.Is(err, models.ErrInferenceFailed) || !errors.Is(err, cause) {
		t.Fatalf("invocation error identity = %q, want safe ASR failure preserving its backend cause", err.Error())
	}
}

func assertASRFailureDiagnostic(t *testing.T, diagnostic modelseffects.RuntimeFailureDiagnostic, cause error) {
	t.Helper()
	if diagnostic.CauseSHA256 != modelseffects.RuntimeCauseSHA256(errors.Join(models.ErrInferenceFailed, cause)) {
		t.Fatalf("cause digest = %q, want the private cause digest", diagnostic.CauseSHA256)
	}
	encodedDiagnostic, err := json.Marshal(diagnostic)
	if err != nil {
		t.Fatalf("marshal runtime diagnostic: %v", err)
	}
	for _, privateDetail := range []string{"private-a", "private-b"} {
		if strings.Contains(string(encodedDiagnostic), privateDetail) {
			t.Fatalf("runtime diagnostic leaked a backend detail")
		}
	}
}
func TestASRInvocationRuntimeHonorsCancellationBeforeAndDuringBackendCall(t *testing.T) {
	t.Parallel()

	backend := &recordingASRBackend{waitForCancellation: true}
	runtime, err := asrruntime.New(backend.transcribe, nil, nil)
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
	runtime, err = asrruntime.New(backend.transcribe, nil, nil)
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

	if _, err := asrruntime.New(nil, nil, nil); !errors.Is(err, models.ErrInvalidInferenceDependencies) {
		t.Fatalf("asrruntime.New(nil, nil, nil) error = %v, want ErrInvalidInferenceDependencies", err)
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

func TestASRInvocationRuntimeExtractsVideoBeforeCodecAndBoundsSegmentsByWAV(t *testing.T) {
	t.Parallel()
	for _, overDuration := range []bool{false, true} {
		t.Run(map[bool]string{false: "video transcript", true: "segment exceeds decoded duration"}[overDuration], func(t *testing.T) {
			wav := asrVideoRuntimeWAV(16000)
			backend := &recordingASRBackend{response: codecs.ASRResponse{Text: "hello", Segments: []codecs.ASRSegment{{ID: 0, End: 1000, Text: "hello"}}}}
			if overDuration {
				backend.response.Segments[0].End = 2000
			}
			var extracted []byte
			runtime, err := asrruntime.New(backend.transcribe, func(ctx context.Context, video []byte) ([]byte, error) {
				extracted = append([]byte(nil), video...)
				return wav, nil
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			request := asrRuntimeRequest()
			request.Inputs[0].Content, request.Inputs[0].MediaType, request.Inputs[0].ContentType = "original MP4 bytes", "video/mp4", "video/mp4"
			result, err := runtime.Invoke(t.Context(), inference.InvocationRuntimeRequest{Request: request})
			assertASRVideoTranscriptionRequest(t, backend.request, request, extracted, wav)
			if overDuration {
				var failure *models.InvocationFailure
				if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassMalformedResponse || result.Content != nil {
					t.Fatalf("duration failure = %#v/%v", result, err)
				}
			} else if err != nil || len(result.Content) != 2 || result.Content[0].Content != "hello" {
				t.Fatalf("video transcript = %#v/%v", result, err)
			}
		})
	}
}

func TestASRInvocationRuntimeVideoFailuresStopBeforeTranscription(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name    string
		audio   []byte
		err     error
		message string
	}{
		{name: "silent video", message: "no audio track"},
		{name: "missing decoder", err: errors.New("video audio decoder failed: ffmpeg executable not found"), message: "ffmpeg executable not found"},
		{name: "canceled extraction", err: context.Canceled},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			backend := &recordingASRBackend{}
			runtime, err := asrruntime.New(backend.transcribe, func(context.Context, []byte) ([]byte, error) { return scenario.audio, scenario.err }, nil)
			if err != nil {
				t.Fatal(err)
			}
			request := asrRuntimeRequest()
			request.Inputs[0].MediaType = "video/mp4"
			result, err := runtime.Invoke(t.Context(), inference.InvocationRuntimeRequest{Request: request})
			if err == nil || result.Content != nil || backend.calls != 0 {
				t.Fatalf("failed video reached transcription: %#v/%v calls=%d", result, err, backend.calls)
			}
			if scenario.err == context.Canceled {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation lost: %v", err)
				}
				return
			}
			if !strings.Contains(err.Error(), scenario.message) {
				t.Fatalf("unreadable video error = %v, want %q", err, scenario.message)
			}
		})
	}
}

func asrVideoRuntimeWAV(samples int) []byte {
	wav := make([]byte, 44+samples*2)
	copy(wav, "RIFF")
	binary.LittleEndian.PutUint32(wav[4:8], uint32(len(wav)-8))
	copy(wav[8:12], "WAVE")
	copy(wav[12:16], "fmt ")
	binary.LittleEndian.PutUint32(wav[16:20], 16)
	binary.LittleEndian.PutUint16(wav[20:22], 1)
	binary.LittleEndian.PutUint16(wav[22:24], 1)
	binary.LittleEndian.PutUint32(wav[24:28], 16000)
	binary.LittleEndian.PutUint32(wav[28:32], 32000)
	binary.LittleEndian.PutUint16(wav[32:34], 2)
	binary.LittleEndian.PutUint16(wav[34:36], 16)
	copy(wav[36:40], "data")
	binary.LittleEndian.PutUint32(wav[40:44], uint32(samples*2))
	return wav
}

func assertASRVideoTranscriptionRequest(t *testing.T, backend codecs.ASRRequest, request models.InvokeModelRequest, extracted, wav []byte) {
	t.Helper()
	if string(extracted) != "original MP4 bytes" || !bytes.Equal(backend.Audio, wav) || backend.MediaType != "audio/wav" || backend.Prompt != "meeting" {
		t.Fatalf("video normalization lost audio or prompt: request=%#v extracted=%q", backend, extracted)
	}
	if request.Inputs[0].Content != "original MP4 bytes" || request.Inputs[0].MediaType != "video/mp4" {
		t.Fatal("video preprocessing mutated caller-owned inputs")
	}
}
