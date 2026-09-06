package wire

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"sync"
	"testing"

	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	"github.com/portpowered/infinite-you/pkg/services/models"
	localai "github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	"google.golang.org/protobuf/proto"
)

func TestInferenceRuntimeRoutesPrivateTTSBeforeGenericBackend(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	transport := &ttsRouteTransport{audio: ttsRouteWAV()}
	genericCalls := 0
	runtime, err := inferenceRuntime(invocationRuntimeOptions{
		Backend: func(context.Context, models.InvokeModelRequest) ([]models.InferenceContent, []models.InferenceArtifact, error) {
			genericCalls++
			return []models.InferenceContent{{Name: "generic", Content: "must not run"}}, nil, nil
		},
		Dialer:           transport,
		TTSTempDirectory: func() string { return tempDir },
		TTSCreateTemp: func(directory, pattern string) (localai.TempFile, error) {
			return os.CreateTemp(directory, pattern)
		},
		TTSInspectFile: os.Stat,
		TTSReadFile:    os.ReadFile,
		TTSRemoveFile:  os.Remove,
	})
	if err != nil {
		t.Fatalf("inferenceRuntime() error = %v", err)
	}
	operation, ok := (models.GenericOperationCatalog{}).GenericOperationContract(models.OperationTTS)
	if !ok {
		t.Fatal("GenericOperationContract(TTS) = false")
	}
	result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request: models.InvokeModelRequest{
			Model:     models.ModelReference{NameOrURI: "vibevoice"},
			Operation: models.OperationTTS,
			Inputs:    []models.InferenceInput{{Name: "text", Modality: models.ModalityText, Content: "hello"}},
		},
		Operation: operation,
		HostSlot:  inference.HostHandleSlot{Endpoint: "grpc://tts-fixture"},
	})
	if err != nil {
		t.Fatalf("private TTS invocation error = %v", err)
	}
	assertPrivateTTSRouteResult(t, genericCalls, result)
	assertPrivateTTSRouteTransport(t, transport)
	assertPrivateTTSRouteOutputRemoved(t, transport.request.GetDst())
}

func assertPrivateTTSRouteResult(t *testing.T, genericCalls int, result inference.InvocationRuntimeResult) {
	t.Helper()
	if genericCalls != 0 || len(result.Content) != 1 || result.Content[0].Name != "audio" || result.Content[0].MediaType != "audio/wav" || len(result.Artifacts) != 1 || result.Artifacts[0].Name != "audio" {
		t.Fatalf("TTS route = genericCalls:%d content:%#v artifacts:%#v, want private semantic audio route", genericCalls, result.Content, result.Artifacts)
	}
}

func assertPrivateTTSRouteTransport(t *testing.T, transport *ttsRouteTransport) {
	t.Helper()
	if transport.invokes != 1 || transport.closes != 1 || transport.method != "/backend.Backend/TTS" || transport.request.GetText() != "hello" || transport.request.GetModel() != "vibevoice" {
		t.Fatalf("TTS transport = %#v, want one pinned request and close", transport)
	}
	if transport.request.GetDst() == "" || transport.request.GetDst() == "grpc://tts-fixture" {
		t.Fatalf("TTS destination = %q, want private staged path", transport.request.GetDst())
	}
}

func assertPrivateTTSRouteOutputRemoved(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("TTS staged output stat error = %v, want removed output", err)
	}
}

func TestInferenceRuntimeFailsClosedWhenPrivateTTSDependenciesAreMissing(t *testing.T) {
	t.Parallel()

	genericCalls := 0
	runtime, err := inferenceRuntime(invocationRuntimeOptions{
		Backend: func(context.Context, models.InvokeModelRequest) ([]models.InferenceContent, []models.InferenceArtifact, error) {
			genericCalls++
			return []models.InferenceContent{{Name: "generic"}}, nil, nil
		},
	})
	if err != nil {
		t.Fatalf("inferenceRuntime() error = %v", err)
	}
	result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{Request: models.InvokeModelRequest{
		Operation: models.OperationTTS,
		Inputs:    []models.InferenceInput{{Name: "text", Modality: models.ModalityText, Content: "must fail closed"}},
	}})
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassConfiguration || !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("missing private TTS = result:%#v error:%v failure:%#v, want typed configuration failure", result, err, failure)
	}
	if genericCalls != 0 || len(result.Content) != 0 || len(result.Artifacts) != 0 {
		t.Fatalf("missing private TTS = genericCalls:%d result:%#v, want no fallback or output", genericCalls, result)
	}
}

func TestInferenceRuntimePrivateTTSReleasesAndRecoversExactlyOnce(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	transport := &ttsRouteTransport{audio: ttsRouteWAV(), failFirst: true}
	var mu sync.Mutex
	removed := make([]string, 0, 2)
	runtime, err := inferenceRuntime(invocationRuntimeOptions{
		Dialer:           transport,
		TTSTempDirectory: func() string { return tempDir },
		TTSCreateTemp: func(directory, pattern string) (localai.TempFile, error) {
			return os.CreateTemp(directory, pattern)
		},
		TTSInspectFile: os.Stat,
		TTSReadFile:    os.ReadFile,
		TTSRemoveFile: func(path string) error {
			mu.Lock()
			removed = append(removed, path)
			mu.Unlock()
			return os.Remove(path)
		},
	})
	if err != nil {
		t.Fatalf("inferenceRuntime() error = %v", err)
	}
	request := inference.InvocationRuntimeRequest{Request: models.InvokeModelRequest{
		Operation: models.OperationTTS,
		Model:     models.ModelReference{NameOrURI: "vibevoice"},
		Inputs:    []models.InferenceInput{{Name: "text", Modality: models.ModalityText, Content: "recover"}},
	}, HostSlot: inference.HostHandleSlot{Endpoint: "tts-fixture"}}
	result, err := runtime.Invoke(context.Background(), request)
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassBackendReadiness || !errors.Is(err, models.ErrUnavailable) || result.Content != nil {
		t.Fatalf("first TTS = result:%#v error:%v failure:%#v, want typed readiness failure", result, err, failure)
	}
	result, err = runtime.Invoke(context.Background(), request)
	if err != nil || len(result.Content) != 1 || result.Content[0].Name != "audio" {
		t.Fatalf("recovered TTS = result:%#v error:%v, want one audio output", result, err)
	}
	mu.Lock()
	removedCount := len(removed)
	mu.Unlock()
	if transport.invokes != 2 || transport.closes != 2 || removedCount != 2 {
		t.Fatalf("TTS recovery lifecycle = invokes:%d closes:%d removes:%d, want exactly once per attempt", transport.invokes, transport.closes, removedCount)
	}
}

type ttsRouteTransport struct {
	mu        sync.Mutex
	audio     []byte
	failFirst bool
	invokes   int
	closes    int
	method    string
	request   localai.TTSRequest
}

func (transport *ttsRouteTransport) Dial(context.Context, string) (platformgrpc.Connection, error) {
	return transport, nil
}

func (transport *ttsRouteTransport) Invoke(_ context.Context, method string, payload []byte) ([]byte, error) {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	transport.invokes++
	transport.method = method
	if err := proto.Unmarshal(payload, &transport.request); err != nil {
		return nil, err
	}
	if transport.failFirst && transport.invokes == 1 {
		return nil, models.ErrUnavailable
	}
	if err := os.WriteFile(transport.request.GetDst(), transport.audio, 0o600); err != nil {
		return nil, err
	}
	response, err := proto.Marshal(&localai.Result{Success: true})
	return response, err
}

func (transport *ttsRouteTransport) Close() error {
	transport.mu.Lock()
	transport.closes++
	transport.mu.Unlock()
	return nil
}

func ttsRouteWAV() []byte {
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
