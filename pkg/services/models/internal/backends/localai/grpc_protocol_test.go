package localai

import (
	"context"
	"net"
	"testing"

	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	"github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

func TestPinnedGRPCProtocolClientMapsOrderedOmniValuesToPinnedFields(t *testing.T) {
	t.Parallel()

	connection := &recordingGRPCConnection{}
	connection.response, _ = proto.Marshal(&Reply{Message: []byte("generated"), Tokens: 3, PromptTokens: 2})
	client := NewPinnedGRPCProtocolClient(recordingGRPCDialer{connection: connection})
	request := PredictRequest{
		Prompt: "describe the inputs",
		Inputs: []ProtocolInput{
			{Slot: "prompt", Modality: models.ModalityText, Content: "describe the inputs"},
			{Slot: "image", Modality: models.ModalityImage, Content: "image-a.png"},
			{Slot: "audio", Modality: models.ModalityAudio, Reference: "audio-a.wav"},
			{Slot: "image", Modality: models.ModalityImage, Content: "image-b.png"},
			{Slot: "video", Modality: models.ModalityVideo, Content: "video-a.mp4"},
		},
		Parameters: []models.OperationParameter{{Name: "temperature", Value: 0.2}},
	}
	ctx := WithInvocationEndpoint(context.Background(), "grpc://127.0.0.1:50051")
	response, err := client.Predict(ctx, request)
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}
	if response.Text != "generated" || response.Usage == "" {
		t.Fatalf("Predict() response = %#v, want generated text and usage", response)
	}
	if connection.method != localAIPredictMethod || connection.closed != 1 {
		t.Fatalf("transport facts = method %q, closed %d, want Predict and one close", connection.method, connection.closed)
	}
	if connection.request.Prompt != request.Prompt ||
		!equalStrings(connection.request.Images, []string{"image-a.png", "image-b.png"}) ||
		!equalStrings(connection.request.Audios, []string{"audio-a.wav"}) ||
		!equalStrings(connection.request.Videos, []string{"video-a.mp4"}) ||
		connection.request.Metadata["temperature"] != "0.2" {
		t.Fatal("pinned request fields do not preserve prompt/media order/metadata")
	}
}

func TestPinnedGRPCHostProtocolNegotiatorUsesHealthRPC(t *testing.T) {
	t.Parallel()

	connection := &recordingGRPCConnection{}
	negotiator := NewPinnedGRPCHostProtocolNegotiator(recordingGRPCDialer{connection: connection})
	result, err := negotiator.Negotiate(context.Background(), "grpc://127.0.0.1:50051", modelseffects.HostProtocolNegotiationRequest{
		ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		Backend:         "localai-llamacpp",
	})
	if err != nil {
		t.Fatalf("Negotiate() error = %v", err)
	}
	if result.ProtocolVersion != modelseffects.PinnedHostProtocolVersion ||
		result.Backend != "localai-llamacpp" || !result.Ready {
		t.Fatalf("Negotiate() result = %#v, want ready pinned result", result)
	}
	if connection.method != localAIHealthMethod || connection.closed != 1 {
		t.Fatalf("health transport facts = method %q, closed %d, want Health and one close", connection.method, connection.closed)
	}
}

func TestPinnedGRPCProtocolClientUsesNetworkDialerAgainstLocalHost(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpcgo.NewServer()
	server.RegisterService(&localAIBackendServiceDesc, networkBackendImpl{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	client := NewPinnedGRPCProtocolClient(platformgrpc.NetworkDialer{})
	response, err := client.Predict(
		WithInvocationEndpoint(context.Background(), listener.Addr().String()),
		PredictRequest{Prompt: "network proof"},
	)
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}
	if response.Text != "network response" {
		t.Fatalf("Predict() text = %q, want network response", response.Text)
	}
}

type recordingGRPCDialer struct {
	connection *recordingGRPCConnection
}

func (dialer recordingGRPCDialer) Dial(context.Context, string) (platformgrpc.Connection, error) {
	return dialer.connection, nil
}

type recordingGRPCConnection struct {
	method   string
	request  PredictOptions
	response []byte
	closed   int
}

func (connection *recordingGRPCConnection) Invoke(
	_ context.Context,
	method string,
	payload []byte,
) ([]byte, error) {
	connection.method = method
	if method == localAIPredictMethod {
		if err := proto.Unmarshal(payload, &connection.request); err != nil {
			return nil, err
		}
	}
	return connection.response, nil
}

func (connection *recordingGRPCConnection) Close() error {
	connection.closed++
	return nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

type networkBackend interface {
	Health(context.Context, *HealthMessage) (*Reply, error)
	Predict(context.Context, *PredictOptions) (*Reply, error)
}

type networkBackendImpl struct{}

func (networkBackendImpl) Health(context.Context, *HealthMessage) (*Reply, error) {
	return &Reply{}, nil
}

func (networkBackendImpl) Predict(context.Context, *PredictOptions) (*Reply, error) {
	return &Reply{Message: []byte("network response")}, nil
}

var localAIBackendServiceDesc = grpcgo.ServiceDesc{
	ServiceName: "backend.Backend",
	HandlerType: (*networkBackend)(nil),
	Methods: []grpcgo.MethodDesc{
		{MethodName: "Health", Handler: localAIHealthHandler},
		{MethodName: "Predict", Handler: localAIPredictHandler},
	},
}

func localAIHealthHandler(
	srv any,
	ctx context.Context,
	decode func(any) error,
	interceptor grpcgo.UnaryServerInterceptor,
) (any, error) {
	request := new(HealthMessage)
	if err := decode(request); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(networkBackend).Health(ctx, request)
	}
	info := &grpcgo.UnaryServerInfo{Server: srv, FullMethod: localAIHealthMethod}
	handler := func(ctx context.Context, request any) (any, error) {
		return srv.(networkBackend).Health(ctx, request.(*HealthMessage))
	}
	return interceptor(ctx, request, info, handler)
}

func localAIPredictHandler(
	srv any,
	ctx context.Context,
	decode func(any) error,
	interceptor grpcgo.UnaryServerInterceptor,
) (any, error) {
	request := new(PredictOptions)
	if err := decode(request); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(networkBackend).Predict(ctx, request)
	}
	info := &grpcgo.UnaryServerInfo{Server: srv, FullMethod: localAIPredictMethod}
	handler := func(ctx context.Context, request any) (any, error) {
		return srv.(networkBackend).Predict(ctx, request.(*PredictOptions))
	}
	return interceptor(ctx, request, info, handler)
}
