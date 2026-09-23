package wire

import (
	"context"
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
	host, err := runtimehostwire.NewService(
		scopes, assets, launcher, httpDoer, clock, nil, nil,
		runtimehost.Options{
			Platform:           models.AssetHostPlatform{OperatingSystem: "windows", Architecture: "amd64"},
			ProtocolNegotiator: protocol, CompatibilityChecker: compatibility,
			RuntimeEvidence: recorder, IdleUnloadAfter: time.Hour,
		},
	)
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
