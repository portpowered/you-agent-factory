package localai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	"github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	localAIHealthMethod    = "/backend.Backend/Health"
	localAILoadModelMethod = "/backend.Backend/LoadModel"
	localAIPredictMethod   = "/backend.Backend/Predict"
	localAIEmbeddingMethod = "/backend.Backend/Embedding"
	localAIModelBatchSize  = 512
)

type invocationEndpointContextKey struct{}

// WithInvocationEndpoint attaches the private, already-selected host address
// to one invocation. The endpoint never enters a public Models request or
// response and is consumed only by the production LocalAI transport.
func WithInvocationEndpoint(ctx context.Context, endpoint string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, invocationEndpointContextKey{}, strings.TrimSpace(endpoint))
}

// NewPinnedGRPCProtocolClient constructs the production adapter for the
// pinned LocalAI Predict RPC. A nil dialer intentionally returns nil so wire
// composition can preserve the fail-closed behavior used by pure fixtures.
func NewPinnedGRPCProtocolClient(dialer platformgrpc.Dialer) ProtocolClient {
	if dialer == nil {
		return nil
	}
	return grpcProtocolClient{dialer: dialer}
}

// NewPinnedEmbeddingBackend constructs the production adapter for LocalAI's
// pinned Embedding RPC. It returns nil when no transport was supplied so the
// Models wire can keep an unbound composition fail-closed.
func NewPinnedEmbeddingBackend(
	dialer platformgrpc.Dialer,
) func(context.Context, models.EmbeddingBackendRequest) (models.EmbeddingBackendResponse, error) {
	if dialer == nil {
		return nil
	}
	client := grpcProtocolClient{dialer: dialer}
	return client.Embedding
}

// NewPinnedGRPCHostProtocolNegotiator creates the matching readiness probe
// for the same LocalAI transport. It checks Health and, when a model path is
// available, loads that model before declaring the host ready. The requested
// backend identity remains a Models-owned fact because the wire reply does not
// carry a backend selector.
func NewPinnedGRPCHostProtocolNegotiator(
	dialer platformgrpc.Dialer,
) modelseffects.HostProtocolNegotiator {
	if dialer == nil {
		return nil
	}
	return grpcHostProtocolNegotiator{dialer: dialer}
}

type grpcHostProtocolNegotiator struct {
	dialer platformgrpc.Dialer
}

func (negotiator grpcHostProtocolNegotiator) Negotiate(
	ctx context.Context,
	endpoint string,
	request modelseffects.HostProtocolNegotiationRequest,
) (modelseffects.HostProtocolNegotiationResult, error) {
	if request.ProtocolVersion != modelseffects.PinnedHostProtocolVersion {
		return modelseffects.HostProtocolNegotiationResult{}, models.ErrHostProtocolIncompatible
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if negotiator.dialer == nil {
		return modelseffects.HostProtocolNegotiationResult{}, models.ErrHostProtocolIncompatible
	}
	connection, err := negotiator.dialer.Dial(ctx, endpoint)
	if err != nil {
		return modelseffects.HostProtocolNegotiationResult{}, fmt.Errorf(
			"%w: LocalAI health connection failed: %v", models.ErrHostProtocolIncompatible, err,
		)
	}
	if connection == nil {
		return modelseffects.HostProtocolNegotiationResult{}, fmt.Errorf(
			"%w: LocalAI health connection is unavailable", models.ErrHostProtocolIncompatible,
		)
	}
	defer func() { _ = connection.Close() }()
	health, err := proto.Marshal(&HealthMessage{})
	if err != nil {
		return modelseffects.HostProtocolNegotiationResult{}, fmt.Errorf(
			"%w: LocalAI health request could not be serialized: %v", models.ErrHostProtocolIncompatible, err,
		)
	}
	if _, err := connection.Invoke(ctx, localAIHealthMethod, health); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return modelseffects.HostProtocolNegotiationResult{}, contextErr
		}
		return modelseffects.HostProtocolNegotiationResult{}, fmt.Errorf(
			"%w: LocalAI health request failed: %v", models.ErrHostProtocolIncompatible, err,
		)
	}
	if strings.TrimSpace(request.ModelPath) != "" {
		if err := loadModel(ctx, connection, request); err != nil {
			return modelseffects.HostProtocolNegotiationResult{}, err
		}
	}
	return modelseffects.HostProtocolNegotiationResult{
		ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		Backend:         request.Backend,
		Ready:           true,
	}, nil
}

func loadModel(
	ctx context.Context,
	connection platformgrpc.Connection,
	request modelseffects.HostProtocolNegotiationRequest,
) error {
	modelFile := strings.TrimSpace(request.ModelPath)
	options, err := vibeVoiceLoadOptions(request)
	if err != nil {
		return fmt.Errorf(
			"%w: LocalAI VibeVoice model layout is invalid",
			models.ErrHostProtocolIncompatible,
		)
	}
	payload, err := proto.Marshal(&ModelOptions{
		Model:      request.ModelName,
		NBatch:     localAIModelBatchSize,
		Embeddings: strings.EqualFold(request.ModelName, models.BuiltInModelNameEmbed),
		ModelFile:  modelFile,
		ModelPath:  filepath.Dir(modelFile),
		Options:    options,
	})
	if err != nil {
		return fmt.Errorf(
			"%w: LocalAI model load request could not be serialized: %v",
			models.ErrHostProtocolIncompatible, err,
		)
	}
	responsePayload, err := connection.Invoke(ctx, localAILoadModelMethod, payload)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		return fmt.Errorf(
			"%w: LocalAI model load request failed: %v",
			models.ErrHostProtocolIncompatible, err,
		)
	}
	response := &Result{}
	if err := proto.Unmarshal(responsePayload, response); err != nil {
		return fmt.Errorf(
			"%w: LocalAI model load response was malformed: %v",
			models.ErrHostProtocolIncompatible, err,
		)
	}
	if !response.Success {
		message := strings.TrimSpace(response.Message)
		if message == "" {
			message = "LocalAI model load response was unsuccessful"
		}
		return fmt.Errorf("%w: %s", models.ErrHostProtocolIncompatible, message)
	}
	return nil
}

type grpcProtocolClient struct {
	dialer platformgrpc.Dialer
}

func (client grpcProtocolClient) Predict(
	ctx context.Context,
	request PredictRequest,
) (PredictResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return PredictResponse{}, err
	}
	endpoint, _ := ctx.Value(invocationEndpointContextKey{}).(string)
	if strings.TrimSpace(endpoint) == "" {
		return PredictResponse{}, protocolFailure("LocalAI Predict endpoint is unavailable", nil)
	}
	if client.dialer == nil {
		return PredictResponse{}, protocolFailure("LocalAI Predict dialer is unavailable", models.ErrUnavailable)
	}
	options, err := predictOptions(request)
	if err != nil {
		return PredictResponse{}, protocolFailure("LocalAI Predict request could not be encoded", err)
	}
	payload, err := proto.Marshal(options)
	if err != nil {
		return PredictResponse{}, protocolFailure("LocalAI Predict request could not be serialized", err)
	}
	connection, err := client.dialer.Dial(ctx, endpoint)
	if err != nil {
		return PredictResponse{}, protocolFailure("LocalAI Predict connection failed", err)
	}
	if connection == nil {
		return PredictResponse{}, protocolFailure("LocalAI Predict connection is unavailable", models.ErrUnavailable)
	}
	defer func() { _ = connection.Close() }()
	responsePayload, err := connection.Invoke(ctx, localAIPredictMethod, payload)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return PredictResponse{}, contextErr
		}
		return PredictResponse{}, protocolFailure("LocalAI Predict request failed", err)
	}
	response := &Reply{}
	if err := proto.Unmarshal(responsePayload, response); err != nil {
		return PredictResponse{}, protocolFailure("LocalAI Predict response was malformed", err)
	}
	text := string(response.Message)
	if text == "" {
		var builder strings.Builder
		for _, delta := range response.GetChatDeltas() {
			if delta != nil {
				builder.WriteString(delta.GetContent())
			}
		}
		text = builder.String()
	}
	return PredictResponse{
		Text:  text,
		Usage: usageJSON(response),
	}, nil
}

func (client grpcProtocolClient) Embedding(
	ctx context.Context,
	request models.EmbeddingBackendRequest,
) (models.EmbeddingBackendResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return models.EmbeddingBackendResponse{}, err
	}
	endpoint, _ := ctx.Value(invocationEndpointContextKey{}).(string)
	if strings.TrimSpace(endpoint) == "" {
		return models.EmbeddingBackendResponse{}, embeddingProtocolFailure(
			"LocalAI Embedding endpoint is unavailable", nil,
		)
	}
	if client.dialer == nil {
		return models.EmbeddingBackendResponse{}, embeddingProtocolFailure(
			"LocalAI Embedding dialer is unavailable", models.ErrUnavailable,
		)
	}
	options, err := embeddingOptions(request)
	if err != nil {
		return models.EmbeddingBackendResponse{}, embeddingProtocolFailure(
			"LocalAI Embedding request could not be encoded", err,
		)
	}
	payload, err := proto.Marshal(options)
	if err != nil {
		return models.EmbeddingBackendResponse{}, embeddingProtocolFailure(
			"LocalAI Embedding request could not be serialized", err,
		)
	}
	connection, err := client.dialer.Dial(ctx, endpoint)
	if err != nil {
		return models.EmbeddingBackendResponse{}, embeddingProtocolFailure(
			"LocalAI Embedding connection failed", err,
		)
	}
	if connection == nil {
		return models.EmbeddingBackendResponse{}, embeddingProtocolFailure(
			"LocalAI Embedding connection is unavailable", models.ErrUnavailable,
		)
	}
	defer func() { _ = connection.Close() }()
	responsePayload, err := connection.Invoke(ctx, localAIEmbeddingMethod, payload)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return models.EmbeddingBackendResponse{}, contextErr
		}
		return models.EmbeddingBackendResponse{}, embeddingRPCFailure(err)
	}
	response := &EmbeddingResult{}
	if err := proto.Unmarshal(responsePayload, response); err != nil {
		return models.EmbeddingBackendResponse{}, embeddingProtocolFailure(
			"LocalAI Embedding response was malformed", err,
		)
	}
	embeddings := response.GetEmbeddings()
	values := make([]float64, len(embeddings))
	for index, value := range embeddings {
		values[index] = float64(value)
	}
	return models.EmbeddingBackendResponse{Embeddings: values}, nil
}

func embeddingOptions(request models.EmbeddingBackendRequest) (*PredictOptions, error) {
	parameters := make([]models.OperationParameter, 0, len(request.Parameters))
	for name, value := range request.Parameters {
		parameters = append(parameters, models.OperationParameter{Name: name, Value: value})
	}
	options, err := predictOptions(PredictRequest{Prompt: request.Text, Parameters: parameters})
	if err != nil {
		return nil, err
	}
	// The pinned LocalAI Embedding RPC reads the dedicated field 36 rather
	// than Prompt. Keep Prompt populated for the shared request shape, but make
	// the operation-specific wire binding explicit.
	options.Embeddings = request.Text
	return options, nil
}

func predictOptions(request PredictRequest) (*PredictOptions, error) {
	options := &PredictOptions{Prompt: request.Prompt}
	for _, input := range request.Inputs {
		value := predictInputValue(input)
		switch input.Modality {
		case models.ModalityImage:
			options.Images = append(options.Images, value)
		case models.ModalityAudio:
			options.Audios = append(options.Audios, value)
		case models.ModalityVideo:
			options.Videos = append(options.Videos, value)
		}
	}
	if len(request.Parameters) == 0 {
		return options, nil
	}
	options.Metadata = make(map[string]string, len(request.Parameters))
	for _, parameter := range request.Parameters {
		name := strings.TrimSpace(parameter.Name)
		if name == "" {
			continue
		}
		value, err := json.Marshal(parameter.Value)
		if err != nil {
			return nil, fmt.Errorf("parameter %q: %w", name, err)
		}
		options.Metadata[name] = string(value)
	}
	return options, nil
}

// predictInputValue maps the public byte-preserving content carrier to the
// pinned LocalAI protobuf convention. LocalAI's media fields are protobuf
// strings containing base64 data; references remain references because the
// backend may resolve them from its own storage.
func predictInputValue(input ProtocolInput) string {
	if input.Content == "" {
		return input.Reference
	}
	switch input.Modality {
	case models.ModalityImage, models.ModalityAudio, models.ModalityVideo:
		return base64.StdEncoding.EncodeToString([]byte(input.Content))
	default:
		return input.Content
	}
}

func usageJSON(response *Reply) string {
	if response == nil || (response.Tokens == 0 && response.PromptTokens == 0) {
		return ""
	}
	value, err := json.Marshal(struct {
		Tokens       int32 `json:"tokens"`
		PromptTokens int32 `json:"promptTokens"`
	}{Tokens: response.Tokens, PromptTokens: response.PromptTokens})
	if err != nil {
		return ""
	}
	return string(value)
}

func protocolFailure(message string, cause error) error {
	return &models.InvocationFailure{
		Class:     models.InvocationFailureClassBackendProtocol,
		Operation: models.OperationOMNI,
		Message:   message,
		Cause:     cause,
	}
}

func embeddingProtocolFailure(message string, cause error) error {
	return &models.InvocationFailure{
		Class:     models.InvocationFailureClassBackendProtocol,
		Operation: models.OperationEMBED,
		Message:   message,
		Cause:     cause,
	}
}

func embeddingRPCFailure(err error) error {
	message := "LocalAI backend returned an unusable response; verify the pinned LocalAI backend contract"
	class := models.InvocationFailureClassBackendProtocol
	switch status.Code(err) {
	case codes.Unavailable:
		class = models.InvocationFailureClassBackendReadiness
		message = "LocalAI backend is unavailable; start the managed backend and verify its health"
	case codes.FailedPrecondition:
		message = "LocalAI backend protocol is incompatible; use the pinned LocalAI backend protocol"
	}
	return &models.InvocationFailure{
		Class:     class,
		Operation: models.OperationEMBED,
		Message:   message,
		Cause:     err,
	}
}
