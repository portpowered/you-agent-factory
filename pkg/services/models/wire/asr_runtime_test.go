package wire

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"testing"
	"time"

	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	"github.com/portpowered/infinite-you/pkg/services/models"
	localai "github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	runtimehostwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/wire"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	"google.golang.org/protobuf/proto"
)

func TestInferenceRuntimeRoutesDefaultASRThroughPinnedProtocol(t *testing.T) {
	t.Parallel()

	responsePayload, err := proto.Marshal(&localai.TranscriptResult{
		Text:     "routed transcript",
		Segments: []*localai.TranscriptSegment{{Id: 0, Start: 0, End: 100_000_000, Text: "routed transcript"}},
	})
	if err != nil {
		t.Fatalf("marshal ASR response: %v", err)
	}
	endpoint := "grpc://127.0.0.1:45906"
	dialer := &asrRuntimeDialer{response: responsePayload}
	tempDir := t.TempDir()
	runtime, err := inferenceRuntime(invocationRuntimeOptions{
		Dialer:           dialer,
		ASRTempDirectory: func() string { return tempDir },
		ASRCreateTemp: func(directory, pattern string) (localai.TempFile, error) {
			return os.CreateTemp(directory, pattern)
		},
		ASRWriteFile: func(path string, content []byte) error {
			return os.WriteFile(path, content, 0o600)
		},
		ASRRemoveFile: os.Remove,
	})
	if err != nil {
		t.Fatalf("inferenceRuntime: %v", err)
	}
	operation, ok := (models.GenericOperationCatalog{}).GenericOperationContract(models.OperationASR)
	if !ok {
		t.Fatal("GenericOperationContract(ASR) = false")
	}
	result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request: models.InvokeModelRequest{
			Scope: mustRoutingScope(t), Model: models.ModelReference{NameOrURI: "asr"},
			Operation: models.OperationASR,
			Inputs: []models.InferenceInput{{
				Name: "audio", Modality: models.ModalityAudio,
				ContentType: "audio/wav", MediaType: "audio/wav", Content: "audio",
			}},
		},
		Operation: operation,
		HostSlot:  inference.HostHandleSlot{Endpoint: endpoint},
	})
	if err != nil || len(result.Content) != 2 || result.Content[0].Content != "routed transcript" || dialer.endpoint != endpoint {
		t.Fatalf("default ASR route = result:%#v error:%v endpoint:%q, want pinned response and selected endpoint", result, err, dialer.endpoint)
	}
	if result.Content[1].Name != "segments" || dialer.connection.closed != 1 {
		t.Fatalf("default ASR outputs/connection = %#v/%d, want segments and one close", result.Content, dialer.connection.closed)
	}
}

func TestQwenASRNormalizesNoncanonicalAudioBeforeCodec(t *testing.T) {
	t.Parallel()
	wav24 := ttsRouteWAV()
	wav16 := append([]byte(nil), wav24...)
	binary.LittleEndian.PutUint32(wav16[24:28], 16000)
	binary.LittleEndian.PutUint32(wav16[28:32], 32000)
	for _, test := range []struct {
		name, media string
		audio       []byte
		normalized  int
	}{
		{"24k WAV", "audio/wav", wav24, 1},
		{"padded slot", "audio/wav", wav24, 1},
		{"compressed MP3", "audio/mpeg", []byte("controlled MP3"), 1},
		{"canonical WAV", "audio/wav", wav16, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			backendError := errors.New("native backend detailed failure")
			runtime, err := newASRInvocationRuntime(func(_ context.Context, request models.ASRBackendRequest) (models.ASRBackendResponse, error) {
				if !bytes.Equal(request.Audio, wav16) || request.MediaType != "audio/wav" {
					t.Fatalf("backend received unnormalized audio: %s", request.MediaType)
				}
				return models.ASRBackendResponse{}, backendError
			}, nil, func(_ context.Context, audio []byte) ([]byte, error) {
				calls++
				if !bytes.Equal(audio, test.audio) {
					t.Fatal("normalization lost original audio")
				}
				return wav16, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			slotName := "audio"
			if test.name == "padded slot" {
				slotName = " audio "
			}
			_, err = runtime.Invoke(localai.WithInvocationBackend(context.Background(), "localai-qwen3-asr-cpp"), inference.InvocationRuntimeRequest{
				Request: models.InvokeModelRequest{Operation: models.OperationASR, Inputs: []models.InferenceInput{{Name: slotName, Modality: models.ModalityAudio, MediaType: test.media, ContentType: test.media, Content: string(test.audio)}}},
			})
			if !errors.Is(err, backendError) || !errors.Is(err, models.ErrInferenceFailed) {
				t.Fatalf("backend cause lost: %v", err)
			}
			if calls != test.normalized {
				t.Fatalf("normalization calls=%d want=%d", calls, test.normalized)
			}
		})
	}
}

type asrRuntimeDialer struct {
	endpoint   string
	response   []byte
	connection *asrRuntimeConnection
}

func (dialer *asrRuntimeDialer) Dial(_ context.Context, endpoint string) (platformgrpc.Connection, error) {
	dialer.endpoint = endpoint
	dialer.connection = &asrRuntimeConnection{response: append([]byte(nil), dialer.response...)}
	return dialer.connection, nil
}

type asrRuntimeConnection struct {
	response []byte
	closed   int
}

func (connection *asrRuntimeConnection) Invoke(context.Context, string, []byte) ([]byte, error) {
	return append([]byte(nil), connection.response...), nil
}

func (connection *asrRuntimeConnection) Close() error {
	connection.closed++
	return nil
}

func newControlledASRHost(
	t *testing.T,
	scopes runtimescopes.Service,
	assets scopedassets.Service,
	launcher modelseffects.HostProcessLauncher,
	httpDoer modelseffects.HostHTTPDoer,
	clock modelseffects.HostClock,
	protocol modelseffects.HostProtocolNegotiator,
	compatibility modelseffects.HostCompatibilityChecker,
	recorder modelseffects.RuntimeEvidenceRecorder,
) runtimehost.Service {
	t.Helper()
	host, err := newRuntimeHostFixture(scopes, assets, launcher, httpDoer, clock, nil, nil, models.AssetHostPlatform{OperatingSystem: "windows", Architecture: "amd64"}, protocol, compatibility, nil, recorder, time.Hour, 0)
	if err != nil {
		t.Fatalf("construct Runtime Host: %v", err)
	}
	return host
}

func newTestASRInvocationRuntime(
	t *testing.T,
	dialer platformgrpc.Dialer,
	tempDirectory string,
) invocationRuntime {
	t.Helper()
	runtime, err := inferenceRuntime(invocationRuntimeOptions{
		Dialer:           dialer,
		ASRTempDirectory: func() string { return tempDirectory },
		ASRCreateTemp: func(directory, pattern string) (localai.TempFile, error) {
			return os.CreateTemp(directory, pattern)
		},
		ASRWriteFile: func(path string, content []byte) error {
			return os.WriteFile(path, content, 0o600)
		},
		ASRRemoveFile: os.Remove,
	})
	if err != nil {
		t.Fatalf("construct Models invocation runtime: %v", err)
	}
	return runtime
}

func newRuntimeHostFixture(scopes runtimescopes.Service, assets scopedassets.Service, launcher modelseffects.HostProcessLauncher, httpDoer modelseffects.HostHTTPDoer, clock modelseffects.HostClock, logger modelseffects.HostDiagnosticLogger, metrics modelseffects.HostMetricsRecorder, platform models.AssetHostPlatform, protocol modelseffects.HostProtocolNegotiator, compatibility modelseffects.HostCompatibilityChecker, resolve modelseffects.HostResolveSymlinks, evidence modelseffects.RuntimeEvidenceRecorder, idle time.Duration, maximum int) (runtimehost.Service, error) {
	state := runtimehostwire.NewSlotState()
	facts := runtimehostwire.NewSlotFacts(scopes, assets, state)
	coordinator := runtimehostwire.NewSlotCoordinator(state, scopes, clock, logger, metrics, idle)
	leases, err := runtimehostwire.NewLeases(clock, facts, coordinator)
	if err != nil {
		return nil, err
	}
	return runtimehostwire.NewService(scopes, assets, leases, state, launcher, httpDoer, clock, logger, metrics, platform, protocol, compatibility, resolve, evidence, idle, maximum)
}
