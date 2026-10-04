package wire

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"encoding/binary"
	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
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

func TestMediaAdaptersUseSelectedRunnerAndPreserveItsFailure(t *testing.T) {
	t.Parallel()
	for _, video := range []bool{false, true} {
		for _, fail := range []bool{false, true} {
			name := "audio"
			if video {
				name = "video"
			}
			if fail {
				name += "/failure"
			} else {
				name += "/success"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				input, wantAudio := []byte("selected media input"), ttsRouteWAV()
				wantErr := errors.New("selected media runner failed")
				ctx := t.Context()
				calls := []string{}
				staged := map[string][]byte{}
				options := mediaAdapterOptions(t, staged, input, wantAudio)
				options.VideoAudioRunner = mediaAdapterRunner(func(gotCtx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
					if gotCtx != ctx {
						t.Fatal("media runner lost selected context")
					}
					assertSelectedMediaCommand(t, request, staged, input)
					calls = append(calls, request.Command)
					if fail {
						return platformprocess.CommandResult{}, wantErr
					}
					return platformprocess.CommandResult{Stdout: []byte(`{"streams":[{"codec_type":"video"},{"codec_type":"audio"}]}`)}, nil
				})
				adapter := audioNormalizer(options)
				wantCalls := []string{"ffmpeg"}
				if video {
					adapter = videoAudioExtractor(options)
					wantCalls = []string{"ffprobe", "ffmpeg"}
				}
				got, err := adapter(ctx, input)
				if fail {
					wantCalls = wantCalls[:1]
					if !errors.Is(err, wantErr) || got != nil {
						t.Fatalf("media failure = (%q, %v), want original cause without audio", got, err)
					}
				} else if err != nil || !bytes.Equal(got, wantAudio) {
					t.Fatalf("media result = (%q, %v), want selected WAV", got, err)
				}
				if !reflect.DeepEqual(calls, wantCalls) || len(staged) != 0 {
					t.Fatalf("commands/remaining staging = %v/%v, want %v and cleanup", calls, staged, wantCalls)
				}
			})
		}
	}
}

func assertSelectedMediaCommand(t *testing.T, request platformprocess.CommandRequest, staged map[string][]byte, input []byte) {
	t.Helper()
	path := ""
	for index, arg := range request.Args {
		if arg == "-i" && index+1 < len(request.Args) {
			path = request.Args[index+1]
		}
	}
	if !bytes.Equal(staged[path], input) || len(request.Stdin) != 0 {
		t.Fatalf("%s command lost seekable media input: %#v", request.Command, request)
	}
	if request.Command == "ffmpeg" {
		for _, arg := range []string{"-ac", "1", "-ar", "16000", "pcm_s16le"} {
			found := false
			for _, got := range request.Args {
				if got == arg {
					found = true
				}
			}
			if !found {
				t.Fatalf("decoder args %v omit %q", request.Args, arg)
			}
		}
	}
}

func mediaAdapterOptions(t *testing.T, staged map[string][]byte, input, audio []byte) invocationRuntimeOptions {
	t.Helper()
	directory := filepath.Join("controlled", "media")
	nextPath := 0
	return invocationRuntimeOptions{
		ASRTempDirectory: func() string { return directory },
		ASRCreateTemp: func(dir, pattern string) (localai.TempFile, error) {
			if dir != directory {
				t.Fatalf("staging directory = %q, want %q", dir, directory)
			}
			nextPath++
			path := filepath.Join(dir, pattern)
			staged[path] = nil
			return mediaAdapterTempFile(path), nil
		},
		ASRWriteFile: func(path string, body []byte) error {
			if !bytes.Equal(body, input) {
				t.Fatal("staging lost selected input")
			}
			staged[path] = append([]byte(nil), body...)
			return nil
		},
		ASRReadFile: func(path string) ([]byte, error) {
			if _, ok := staged[path]; !ok || nextPath != 2 {
				t.Fatal("read without reserved decoder output")
			}
			return append([]byte(nil), audio...), nil
		},
		ASRRemoveFile: func(path string) error {
			if _, ok := staged[path]; !ok {
				t.Fatal("duplicate staging cleanup")
			}
			delete(staged, path)
			return nil
		},
	}
}

type mediaAdapterRunner func(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error)

func (runner mediaAdapterRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return runner(ctx, request)
}

type mediaAdapterTempFile string

func (file mediaAdapterTempFile) Name() string { return string(file) }
func (mediaAdapterTempFile) Close() error      { return nil }

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
