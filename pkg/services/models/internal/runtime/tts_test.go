package runtime_test

import (
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"time"

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

func TestTTSInvocationRuntimePreservesPrivateBackendCauseWithoutOutput(t *testing.T) {
	t.Parallel()
	private := errors.New("private backend path and token")
	runtime, err := ttsruntime.NewTTS(func(context.Context, codecs.TTSRequest) (codecs.TTSResponse, error) {
		return codecs.TTSResponse{Audio: ttsRuntimeWAV()}, private
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Invoke(t.Context(), inference.InvocationRuntimeRequest{Request: models.InvokeModelRequest{
		Operation: models.OperationTTS,
		Inputs:    []models.InferenceInput{{Name: "text", Modality: models.ModalityText, Content: "hello"}},
	}})
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassBackendProtocol ||
		!errors.Is(err, private) || !errors.Is(err, models.ErrInferenceFailed) ||
		result.Content != nil || result.Artifacts != nil || strings.Contains(err.Error(), "private") {
		t.Fatalf("backend failure = result:%#v error:%v, want safe message, preserved cause, and no output", result, err)
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

// Each collaborator is a completed fake role: this isolates the selector.
type fixedInvocationRole string

func (role fixedInvocationRole) Invoke(_ context.Context, request inference.InvocationRuntimeRequest) (inference.InvocationRuntimeResult, error) {
	return inference.InvocationRuntimeResult{Content: []models.InferenceContent{{Name: "selected", Content: string(role)}}}, nil
}

func TestInvocationSelectsCompletedRoles(t *testing.T) {
	t.Parallel()
	runtime, err := ttsruntime.NewInvocationRuntime(fixedInvocationRole("generic"), fixedInvocationRole("omni"),
		fixedInvocationRole("asr"), fixedInvocationRole("embedding"), fixedInvocationRole("tts"))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ operation, want string }{
		{"custom", "generic"}, {" omni ", "omni"}, {"asr", "asr"}, {"EMBED", "embedding"}, {"TTS", "tts"},
	} {
		result, err := runtime.Invoke(t.Context(), inference.InvocationRuntimeRequest{Request: models.InvokeModelRequest{Operation: test.operation}})
		if err != nil || len(result.Content) != 1 || result.Content[0].Content != test.want {
			t.Fatalf("operation %q = %#v, %v; want %s", test.operation, result, err, test.want)
		}
	}
}

func TestInvocationRejectsIncompleteRoles(t *testing.T) {
	t.Parallel()
	role := fixedInvocationRole("complete")
	for missing := 0; missing < 5; missing++ {
		roles := []ttsruntime.InvocationRuntime{role, role, role, role, role}
		roles[missing] = nil
		if _, err := ttsruntime.NewInvocationRuntime(roles[0], roles[1], roles[2], roles[3], roles[4]); !errors.Is(err, models.ErrInvalidInferenceDependencies) {
			t.Fatalf("missing role %d: %v", missing, err)
		}
	}
	var missingRole *fixedInvocationRole
	if _, err := ttsruntime.NewInvocationRuntime(missingRole, role, role, role, role); !errors.Is(err, models.ErrInvalidInferenceDependencies) {
		t.Fatalf("typed nil role: %v", err)
	}
}

func TestInvocationUnavailablePreservesContextFailureAndTypedError(t *testing.T) {
	t.Parallel()
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	expired, release := context.WithDeadline(t.Context(), time.Time{})
	t.Cleanup(release)
	for _, test := range []struct {
		name string
		ctx  context.Context
		want error
	}{
		{name: "nil", want: models.ErrUnavailable},
		{name: "active", ctx: t.Context(), want: models.ErrUnavailable},
		{name: "canceled", ctx: canceled, want: context.Canceled},
		{name: "expired", ctx: expired, want: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := (ttsruntime.UnavailableInvocationRuntime{}).Invoke(test.ctx, inference.InvocationRuntimeRequest{
				Operation: models.Operation{Name: " ASR "},
				Request:   models.InvokeModelRequest{Operation: models.OperationTTS},
			})
			if !errors.Is(err, test.want) || result.Content != nil || result.Artifacts != nil {
				t.Fatalf("unavailable invocation = %#v, %v; want %v and no output", result, err, test.want)
			}
			var failure *models.InvocationFailure
			if errors.Is(test.want, models.ErrUnavailable) {
				if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassConfiguration || failure.Operation != models.OperationASR {
					t.Fatalf("unavailable error = %v; want typed configuration failure for ASR", err)
				}
			} else if errors.As(err, &failure) || errors.Is(err, models.ErrUnavailable) {
				t.Fatalf("context error = %v; want cancellation or deadline without unavailable wrapping", err)
			}
		})
	}
}

func TestInvocationGenericPreservesDeniedCauseAndDetachedArtifact(t *testing.T) {
	t.Parallel()
	denied := &models.InvocationFailure{Class: models.InvocationFailureClassBackendProtocol, Message: "controlled policy denial", Cause: models.ErrUnavailable}
	backend := ttsruntime.NewGenericInvocationRuntime(func(context.Context, models.InvokeModelRequest) ([]models.InferenceContent, []models.InferenceArtifact, error) {
		return nil, nil, denied
	})
	result, err := backend.Invoke(t.Context(), inference.InvocationRuntimeRequest{})
	if !errors.Is(err, denied) || result.Content != nil || result.Artifacts != nil {
		t.Fatalf("denied generic = %#v, %v", result, err)
	}
	artifact, err := (models.InferenceArtifactRef{}).Parse("artifact://selected")
	if err != nil {
		t.Fatal(err)
	}
	properties := map[string]string{"source": "selected"}
	backend = ttsruntime.NewGenericInvocationRuntime(func(context.Context, models.InvokeModelRequest) ([]models.InferenceContent, []models.InferenceArtifact, error) {
		return []models.InferenceContent{{Content: "selected output"}}, []models.InferenceArtifact{{Artifact: artifact, Name: "selected", MediaType: "text/plain", Properties: properties}}, nil
	})
	result, err = backend.Invoke(t.Context(), inference.InvocationRuntimeRequest{})
	if err != nil || len(result.Content) != 1 || result.Content[0].Content != "selected output" || len(result.Artifacts) != 1 || result.Artifacts[0].RefValue != artifact.String() {
		t.Fatalf("generic output = %#v, %v", result, err)
	}
	properties["source"] = "changed"
	if result.Artifacts[0].Properties["source"] != "selected" {
		t.Fatalf("artifact properties changed after backend mutation: %#v", result.Artifacts[0].Properties)
	}
}
