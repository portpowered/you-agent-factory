package runtime_test

import (
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai/codecs"
	ttsruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/runtime"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

func TestTTSInvocationRuntimeMapsOneDetachedAudioOutput(t *testing.T) {
	t.Parallel()

	var got codecs.TTSRequest
	runtime, err := ttsruntime.NewTTS(func(_ context.Context, request codecs.TTSRequest) (codecs.TTSResponse, error) {
		got = request
		return codecs.TTSResponse{Audio: ttsRuntimeWAV(), MediaType: "audio/wave; codec=pcm"}, nil
	})
	if err != nil {
		t.Fatalf("ttsruntime.NewTTS() error = %v", err)
	}
	result, err := runtime.Invoke(t.Context(), inference.InvocationRuntimeRequest{Request: models.InvokeModelRequest{
		Operation:  models.OperationTTS,
		Model:      models.ModelReference{NameOrURI: "vibevoice"},
		Inputs:     []models.InferenceInput{{Name: "text", Modality: models.ModalityText, Content: "hello"}},
		Parameters: []models.OperationParameter{{Name: "language", Value: "en"}},
	}})
	if err != nil {
		t.Fatalf("TTS invocation error = %v", err)
	}
	assertDetachedTTSResult(t, result)
	assertDetachedTTSRequest(t, got)
}

func assertDetachedTTSResult(t *testing.T, result inference.InvocationRuntimeResult) {
	t.Helper()
	if len(result.Content) != 1 || len(result.Artifacts) != 1 {
		t.Fatalf("TTS invocation result = %#v, want one content and one artifact", result)
	}
	content := result.Content[0]
	if content.Name != "audio" || content.Modality != models.ModalityAudio || content.MediaType != "audio/wav" || content.Content != string(ttsRuntimeWAV()) {
		t.Fatalf("TTS content = %#v, want one canonical detached WAV output", content)
	}
	artifact := result.Artifacts[0]
	if artifact.Name != "audio" || artifact.MediaType != "audio/wav" || artifact.SizeBytes != int64(len(ttsRuntimeWAV())) || artifact.Properties["label"] != "speech.wav" || !strings.HasPrefix(artifact.Properties["digest"], "sha256:") {
		t.Fatalf("TTS artifact = %#v, want semantic audio metadata", artifact)
	}
}

func assertDetachedTTSRequest(t *testing.T, request codecs.TTSRequest) {
	t.Helper()
	if request.Text != "hello" || request.Model != "vibevoice" || request.Parameters["language"] != "en" {
		t.Fatalf("TTS backend request = %#v, want normalized text/model/parameters", request)
	}
}

func TestTTSInvocationRuntimePreservesTypedFailureAndCancellation(t *testing.T) {
	t.Parallel()

	backendFailure := &models.InvocationFailure{
		Class:     models.InvocationFailureClassBackendReadiness,
		Operation: models.OperationTTS,
		Message:   "backend unavailable",
		Cause:     models.ErrUnavailable,
	}
	backendCalls := 0
	runtime, err := ttsruntime.NewTTS(func(context.Context, codecs.TTSRequest) (codecs.TTSResponse, error) {
		backendCalls++
		return codecs.TTSResponse{}, backendFailure
	})
	if err != nil {
		t.Fatalf("ttsruntime.NewTTS() error = %v", err)
	}
	result, err := runtime.Invoke(t.Context(), inference.InvocationRuntimeRequest{Request: models.InvokeModelRequest{
		Operation: models.OperationTTS,
		Inputs:    []models.InferenceInput{{Name: "text", Modality: models.ModalityText, Content: "hello"}},
	}})
	if !errors.Is(err, models.ErrUnavailable) || !errors.Is(err, backendFailure) || result.Content != nil || backendCalls != 1 {
		t.Fatalf("typed backend failure = result:%#v error:%v calls:%d, want preserved readiness failure and no output", result, err, backendCalls)
	}
	if strings.Contains(err.Error(), "private") {
		t.Fatalf("typed failure leaked private detail: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err = runtime.Invoke(ctx, inference.InvocationRuntimeRequest{Request: models.InvokeModelRequest{
		Operation: models.OperationTTS,
		Inputs:    []models.InferenceInput{{Name: "text", Modality: models.ModalityText, Content: "cancelled"}},
	}})
	if !errors.Is(err, context.Canceled) || result.Content != nil || backendCalls != 1 {
		t.Fatalf("pre-cancelled TTS = result:%#v error:%v calls:%d, want cancellation before backend", result, err, backendCalls)
	}
}

func TestTTSInvocationRuntimeRejectsNilBackend(t *testing.T) {
	t.Parallel()

	if _, err := ttsruntime.NewTTS(nil); !errors.Is(err, models.ErrInvalidInferenceDependencies) {
		t.Fatalf("ttsruntime.NewTTS(nil) error = %v, want ErrInvalidInferenceDependencies", err)
	}
}

func ttsRuntimeWAV() []byte {
	audio := make([]byte, 46)
	copy(audio[0:4], "RIFF")
	binary.LittleEndian.PutUint32(audio[4:8], uint32(len(audio)-8))
	copy(audio[8:12], "WAVE")
	copy(audio[12:16], "fmt ")
	binary.LittleEndian.PutUint32(audio[16:20], 16)
	binary.LittleEndian.PutUint16(audio[20:22], 1)
	binary.LittleEndian.PutUint16(audio[22:24], 1)
	binary.LittleEndian.PutUint32(audio[24:28], 24000)
	binary.LittleEndian.PutUint32(audio[28:32], 48000)
	binary.LittleEndian.PutUint16(audio[32:34], 2)
	binary.LittleEndian.PutUint16(audio[34:36], 16)
	copy(audio[36:40], "data")
	binary.LittleEndian.PutUint32(audio[40:44], 2)
	audio[44] = 0x01
	audio[45] = 0x02
	return audio
}
