package localai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	"github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	"google.golang.org/protobuf/proto"
)

const (
	localAIHealthMethod  = "/backend.Backend/Health"
	localAIPredictMethod = "/backend.Backend/Predict"
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

// NewPinnedGRPCHostProtocolNegotiator creates the matching readiness probe
// for the same LocalAI transport. Health is the pinned protocol's only
// readiness operation; the requested backend identity remains a Models-owned
// fact because the wire reply does not carry a backend selector.
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
	connection, err := negotiator.dialer.Dial(ctx, endpoint)
	if err != nil {
		return modelseffects.HostProtocolNegotiationResult{}, fmt.Errorf(
			"%w: LocalAI health connection failed: %v", models.ErrHostProtocolIncompatible, err,
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
	return modelseffects.HostProtocolNegotiationResult{
		ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		Backend:         request.Backend,
		Ready:           true,
	}, nil
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
	return PredictResponse{
		Text:  string(response.Message),
		Usage: usageJSON(response),
	}, nil
}

func predictOptions(request PredictRequest) (*PredictOptions, error) {
	options := &PredictOptions{Prompt: request.Prompt}
	for _, input := range request.Inputs {
		value := input.Content
		if value == "" {
			value = input.Reference
		}
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
