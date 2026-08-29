package localai

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"strings"
	"testing"

	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	"github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
		!equalStrings(connection.request.Images, []string{
			base64.StdEncoding.EncodeToString([]byte("image-a.png")),
			base64.StdEncoding.EncodeToString([]byte("image-b.png")),
		}) ||
		!equalStrings(connection.request.Audios, []string{"audio-a.wav"}) ||
		!equalStrings(connection.request.Videos, []string{
			base64.StdEncoding.EncodeToString([]byte("video-a.mp4")),
		}) ||
		connection.request.Metadata["temperature"] != "0.2" {
		t.Fatal("pinned request fields do not preserve prompt/media order/metadata")
	}
}

func TestPinnedGRPCProtocolClientPreservesBinaryMediaThroughBase64Fields(t *testing.T) {
	t.Parallel()

	connection := &recordingGRPCConnection{}
	client := NewPinnedGRPCProtocolClient(recordingGRPCDialer{connection: connection})
	image := string([]byte{0x00, 0xff, 0x10, 0x80, 0x7f})
	if _, err := client.Predict(
		WithInvocationEndpoint(context.Background(), "127.0.0.1:50051"),
		PredictRequest{Inputs: []ProtocolInput{{Modality: models.ModalityImage, Content: image}}},
	); err != nil {
		t.Fatalf("Predict() error = %v", err)
	}
	if len(connection.request.Images) != 1 {
		t.Fatalf("encoded images = %#v, want one image", connection.request.Images)
	}
	decoded, err := base64.StdEncoding.DecodeString(connection.request.Images[0])
	if err != nil {
		t.Fatalf("DecodeString(image) error = %v", err)
	}
	if string(decoded) != image {
		t.Fatalf("decoded image = %v, want original bytes %v", decoded, []byte(image))
	}
}

func TestPinnedGRPCProtocolClientUsesChatDeltaTextWhenLegacyMessageIsEmpty(t *testing.T) {
	t.Parallel()

	connection := &recordingGRPCConnection{}
	connection.response, _ = proto.Marshal(&Reply{ChatDeltas: []*ChatDelta{
		{Content: "generated "}, {Content: "from chat deltas"},
	}})
	client := NewPinnedGRPCProtocolClient(recordingGRPCDialer{connection: connection})
	response, err := client.Predict(
		WithInvocationEndpoint(context.Background(), "127.0.0.1:50051"),
		PredictRequest{Prompt: "describe"},
	)
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}
	if response.Text != "generated from chat deltas" {
		t.Fatalf("Predict() text = %q, want concatenated chat-delta content", response.Text)
	}
}

func TestPinnedEmbeddingBackendMapsTextParametersAndResponse(t *testing.T) {
	t.Parallel()

	connection := &recordingGRPCConnection{}
	connection.response, _ = proto.Marshal(&EmbeddingResult{
		Embeddings: []float32{0.1, -0.2, 0.3},
	})
	backend := NewPinnedEmbeddingBackend(recordingGRPCDialer{connection: connection})
	response, err := backend(
		WithInvocationEndpoint(context.Background(), "grpc://127.0.0.1:50051"),
		models.EmbeddingBackendRequest{
			Text:       "Find similar work",
			Parameters: map[string]any{"normalize": true, "dimensions": 3},
		},
	)
	if err != nil {
		t.Fatalf("Embedding() error = %v", err)
	}
	if len(response.Embeddings) != 3 || response.Embeddings[0] != float64(float32(0.1)) ||
		response.Embeddings[1] != float64(float32(-0.2)) {
		t.Fatalf("Embedding() response = %#v, want three mapped values", response.Embeddings)
	}
	if connection.method != localAIEmbeddingMethod || connection.closed != 1 {
		t.Fatalf("embedding transport facts = method %q, closed %d, want Embedding and one close", connection.method, connection.closed)
	}
	if connection.embeddingRequest.GetPrompt() != "Find similar work" ||
		connection.embeddingRequest.GetMetadata()["normalize"] != "true" ||
		connection.embeddingRequest.GetMetadata()["dimensions"] != "3" {
		t.Fatalf("embedding request prompt=%q metadata=%v, want prompt and JSON metadata", connection.embeddingRequest.GetPrompt(), connection.embeddingRequest.GetMetadata())
	}
}

func TestPinnedEmbeddingBackendFailsClosedWithoutEndpoint(t *testing.T) {
	t.Parallel()

	backend := NewPinnedEmbeddingBackend(recordingGRPCDialer{connection: &recordingGRPCConnection{}})
	_, err := backend(context.Background(), models.EmbeddingBackendRequest{Text: "secret input"})
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassBackendProtocol {
		t.Fatalf("Embedding() error = %v, failure = %#v, want typed backend-protocol failure", err, failure)
	}
	if strings.Contains(err.Error(), "secret input") {
		t.Fatalf("Embedding() error leaked input: %v", err)
	}
}

func TestPinnedEmbeddingBackendClassifiesTransportFailures(t *testing.T) {
	for _, test := range []struct {
		name    string
		cause   error
		class   models.InvocationFailureClass
		message string
	}{
		{
			name:    "backend unavailable",
			cause:   status.Error(codes.Unavailable, "backend stopped"),
			class:   models.InvocationFailureClassBackendReadiness,
			message: "LocalAI backend is unavailable",
		},
		{
			name:    "protocol mismatch",
			cause:   status.Error(codes.FailedPrecondition, "wrong protocol"),
			class:   models.InvocationFailureClassBackendProtocol,
			message: "LocalAI backend protocol is incompatible",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			connection := &recordingGRPCConnection{invokeErr: test.cause}
			backend := NewPinnedEmbeddingBackend(recordingGRPCDialer{connection: connection})
			_, err := backend(
				WithInvocationEndpoint(context.Background(), "127.0.0.1:50051"),
				models.EmbeddingBackendRequest{Text: "query"},
			)
			var failure *models.InvocationFailure
			if !errors.As(err, &failure) || failure.Class != test.class || !strings.Contains(failure.Message, test.message) {
				t.Fatalf("Embedding() error = %v, failure = %#v, want %s containing %q", err, failure, test.class, test.message)
			}
			if connection.closed != 1 {
				t.Fatalf("connection closes = %d, want one close after transport failure", connection.closed)
			}
		})
	}
}

func TestPinnedEmbeddingBackendReturnsContextCancellationBeforeDial(t *testing.T) {
	dialer := &countingEmbeddingDialer{}
	backend := NewPinnedEmbeddingBackend(dialer)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := backend(WithInvocationEndpoint(ctx, "127.0.0.1:50051"), models.EmbeddingBackendRequest{Text: "query"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Embedding() error = %v, want context cancellation", err)
	}
	if dialer.calls != 0 {
		t.Fatalf("dial calls = %d, want no dial after cancellation", dialer.calls)
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

func TestPinnedGRPCHostProtocolNegotiatorLoadsDeclaredModelAfterHealth(t *testing.T) {
	t.Parallel()

	connection := &recordingGRPCConnection{}
	connection.response, _ = proto.Marshal(&Result{Success: true, Message: "loaded"})
	negotiator := NewPinnedGRPCHostProtocolNegotiator(recordingGRPCDialer{connection: connection})
	result, err := negotiator.Negotiate(context.Background(), "grpc://127.0.0.1:50051", modelseffects.HostProtocolNegotiationRequest{
		ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		Backend:         "localai-llamacpp",
		ModelName:       "llm",
		ModelPath:       `C:\models\llm\model.gguf`,
	})
	if err != nil {
		t.Fatalf("Negotiate() error = %v", err)
	}
	if !result.Ready || connection.method != localAILoadModelMethod || connection.closed != 1 {
		t.Fatalf("load transport facts = method %q, ready %t, closed %d, want LoadModel/ready/one close", connection.method, result.Ready, connection.closed)
	}
	if connection.loadRequest.GetModel() != "llm" ||
		connection.loadRequest.GetModelFile() != `C:\models\llm\model.gguf` ||
		connection.loadRequest.GetNBatch() != localAIModelBatchSize {
		t.Fatalf(
			"load request model=%q modelFile=%q nBatch=%d, want model name, nonzero batch size, and exact model path",
			connection.loadRequest.GetModel(), connection.loadRequest.GetModelFile(), connection.loadRequest.GetNBatch(),
		)
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
	method           string
	request          PredictOptions
	embeddingRequest PredictOptions
	loadRequest      ModelOptions
	response         []byte
	invokeErr        error
	closed           int
}

func (connection *recordingGRPCConnection) Invoke(
	_ context.Context,
	method string,
	payload []byte,
) ([]byte, error) {
	connection.method = method
	if connection.invokeErr != nil {
		return nil, connection.invokeErr
	}
	if method == localAIPredictMethod {
		if err := proto.Unmarshal(payload, &connection.request); err != nil {
			return nil, err
		}
	}
	if method == localAIEmbeddingMethod {
		if err := proto.Unmarshal(payload, &connection.embeddingRequest); err != nil {
			return nil, err
		}
	}
	if method == localAILoadModelMethod {
		if err := proto.Unmarshal(payload, &connection.loadRequest); err != nil {
			return nil, err
		}
	}
	return connection.response, nil
}

type countingEmbeddingDialer struct {
	calls int
}

func (dialer *countingEmbeddingDialer) Dial(context.Context, string) (platformgrpc.Connection, error) {
	dialer.calls++
	return &recordingGRPCConnection{}, nil
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
