package localai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	localaiproto "github.com/portpowered/infinite-you/tests/functional/internal/support/localai/protocol"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	// PinnedHostProtocolVersion mirrors the public Models construction seam's
	// pinned LocalAI protocol value. The generated protocol remains private to
	// this functional fixture package.
	PinnedHostProtocolVersion = "localai-backend-v1"
	fixtureDialTimeout        = 5 * time.Second
)

// InvocationBackend adapts the fixture's pinned gRPC protocol to the narrow
// Models edge used by the HTTP conformance test. It intentionally returns
// detached Models values and performs no production registration or routing.
func (fixture *Fixture) InvocationBackend(
	ctx context.Context,
	request models.InvokeModelRequest,
) ([]models.InferenceContent, []models.InferenceArtifact, error) {
	if fixture == nil {
		return nil, nil, errors.New("LocalAI fixture is nil")
	}
	connection, client, err := fixture.dial(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = connection.Close() }()

	inputs := request.Inputs
	if len(inputs) == 0 {
		inputs = []models.InferenceInput{request.Input}
	}
	switch strings.ToUpper(strings.TrimSpace(request.Operation)) {
	case models.OperationOMNI:
		return fixture.invokeOMNI(ctx, client, request, inputs)
	case models.OperationEMBED:
		return fixture.invokeEMBED(ctx, client, inputs)
	case models.OperationTTS:
		return fixture.invokeTTS(ctx, client, request, inputs)
	case models.OperationASR:
		return fixture.invokeASR(ctx, client, inputs)
	default:
		return nil, nil, fmt.Errorf("unsupported LocalAI fixture operation %q", request.Operation)
	}
}

func (fixture *Fixture) invokeOMNI(
	ctx context.Context,
	client localaiproto.BackendClient,
	request models.InvokeModelRequest,
	inputs []models.InferenceInput,
) ([]models.InferenceContent, []models.InferenceArtifact, error) {
	options := &localaiproto.PredictOptions{}
	for _, input := range inputs {
		switch input.Modality {
		case models.ModalityText:
			if input.Name == "prompt" || options.Prompt == "" {
				options.Prompt = input.Content
			}
		case models.ModalityImage:
			options.Images = append(options.Images, input.Content)
		case models.ModalityAudio:
			options.Audios = append(options.Audios, input.Content)
		case models.ModalityVideo:
			options.Videos = append(options.Videos, input.Content)
		}
	}
	response, err := client.Predict(ctx, options)
	if err != nil {
		return nil, nil, fmt.Errorf("invoke LocalAI OMNI: %w", err)
	}
	if len(response.GetMessage()) == 0 {
		return nil, nil, malformedResponse(models.OperationOMNI, "text")
	}
	return []models.InferenceContent{{
		Name:        "text",
		Modality:    models.ModalityText,
		ContentType: "text/plain",
		MediaType:   "text/plain",
		Content:     string(response.GetMessage()),
	}}, nil, nil
}

func (fixture *Fixture) invokeEMBED(
	ctx context.Context,
	client localaiproto.BackendClient,
	inputs []models.InferenceInput,
) ([]models.InferenceContent, []models.InferenceArtifact, error) {
	response, err := client.Embedding(ctx, &localaiproto.PredictOptions{Prompt: firstInputContent(inputs)})
	if err != nil {
		return nil, nil, fmt.Errorf("invoke LocalAI EMBED: %w", err)
	}
	if len(response.GetEmbeddings()) == 0 {
		return nil, nil, malformedResponse(models.OperationEMBED, "embedding")
	}
	content, err := json.Marshal(response.GetEmbeddings())
	if err != nil {
		return nil, nil, fmt.Errorf("encode LocalAI embedding: %w", err)
	}
	return []models.InferenceContent{{
		Name:        "embedding",
		Modality:    models.ModalityJSON,
		ContentType: "application/json",
		MediaType:   "application/json",
		Content:     string(content),
	}}, nil, nil
}

func (fixture *Fixture) invokeTTS(
	ctx context.Context,
	client localaiproto.BackendClient,
	request models.InvokeModelRequest,
	inputs []models.InferenceInput,
) ([]models.InferenceContent, []models.InferenceArtifact, error) {
	stream, err := client.TTSStream(ctx, &localaiproto.TTSRequest{
		Model: request.Model.NameOrURI,
		Text:  firstInputContent(inputs),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("invoke LocalAI TTS: %w", err)
	}
	response, err := stream.Recv()
	if err != nil {
		return nil, nil, fmt.Errorf("receive LocalAI TTS: %w", err)
	}
	if len(response.GetAudio()) == 0 {
		return nil, nil, malformedResponse(models.OperationTTS, "audio")
	}
	return []models.InferenceContent{{
		Name:        "audio",
		Modality:    models.ModalityAudio,
		ContentType: "audio/wav",
		MediaType:   "audio/wav",
		Content:     base64.StdEncoding.EncodeToString(response.GetAudio()),
	}}, nil, nil
}

func (fixture *Fixture) invokeASR(
	ctx context.Context,
	client localaiproto.BackendClient,
	inputs []models.InferenceInput,
) ([]models.InferenceContent, []models.InferenceArtifact, error) {
	response, err := client.AudioTranscription(ctx, &localaiproto.TranscriptRequest{
		Prompt: firstInputContent(inputs),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("invoke LocalAI ASR: %w", err)
	}
	if strings.TrimSpace(response.GetText()) == "" || len(response.GetSegments()) == 0 {
		return nil, nil, malformedResponse(models.OperationASR, "transcript")
	}
	segments := make([]fixtureTranscriptSegmentValue, 0, len(response.GetSegments()))
	for _, segment := range response.GetSegments() {
		if segment == nil {
			continue
		}
		segments = append(segments, fixtureTranscriptSegmentValue{
			ID: segment.GetId(), Start: segment.GetStart(), End: segment.GetEnd(), Text: segment.GetText(),
		})
	}
	if len(segments) == 0 {
		return nil, nil, malformedResponse(models.OperationASR, "segments")
	}
	segmentContent, err := json.Marshal(segments)
	if err != nil {
		return nil, nil, fmt.Errorf("encode LocalAI ASR segments: %w", err)
	}
	return []models.InferenceContent{
		{
			Name: "transcript", Modality: models.ModalityText,
			ContentType: "text/plain", MediaType: "text/plain", Content: response.GetText(),
		},
		{
			Name: "segments", Modality: models.ModalityJSON,
			ContentType: "application/json", MediaType: "application/json", Content: string(segmentContent),
		},
	}, nil, nil
}

type fixtureTranscriptSegmentValue struct {
	ID    int32  `json:"id"`
	Start int64  `json:"start"`
	End   int64  `json:"end"`
	Text  string `json:"text"`
}

func firstInputContent(inputs []models.InferenceInput) string {
	if len(inputs) == 0 {
		return ""
	}
	return inputs[0].Content
}

func malformedResponse(operation, slot string) error {
	return &models.InvocationFailure{
		Class:     models.InvocationFailureClassMalformedResponse,
		Operation: operation,
		Slot:      slot,
		Message:   fmt.Sprintf("LocalAI fixture returned no %s output", slot),
	}
}

func (fixture *Fixture) dial(ctx context.Context) (*grpc.ClientConn, localaiproto.BackendClient, error) {
	dialContext, cancel := context.WithTimeout(ctx, fixtureDialTimeout)
	defer cancel()
	connection, err := grpc.DialContext(
		dialContext,
		fixture.Endpoint(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("dial LocalAI fixture: %w", err)
	}
	return connection, localaiproto.NewBackendClient(connection), nil
}

// GRPCDialer exposes the fixture through the Models managed-host edge. The
// method's anonymous return interface intentionally matches edges.Edges so the
// generated protocol client and connection remain inside this support package.
func (fixture *Fixture) GRPCDialer() interface {
	Dial(context.Context, string) (interface {
		Negotiate(context.Context, serviceedges.ModelHostProtocolNegotiationRequest) (serviceedges.ModelHostProtocolNegotiationResult, error)
		Close() error
	}, error)
} {
	return fixtureGRPCDialer{fixture: fixture}
}

type fixtureGRPCDialer struct {
	fixture *Fixture
}

func (dialer fixtureGRPCDialer) Dial(
	ctx context.Context,
	endpoint string,
) (interface {
	Negotiate(context.Context, serviceedges.ModelHostProtocolNegotiationRequest) (serviceedges.ModelHostProtocolNegotiationResult, error)
	Close() error
}, error) {
	if dialer.fixture == nil {
		return nil, errors.New("LocalAI fixture dialer is nil")
	}
	if strings.TrimSpace(endpoint) == "" {
		endpoint = dialer.fixture.Endpoint()
	}
	dialContext, cancel := context.WithTimeout(ctx, fixtureDialTimeout)
	defer cancel()
	connection, err := grpc.DialContext(
		dialContext,
		endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("dial LocalAI managed host: %w", err)
	}
	return &fixtureGRPCConnection{
		connection: connection,
		client:     localaiproto.NewBackendClient(connection),
	}, nil
}

type fixtureGRPCConnection struct {
	connection *grpc.ClientConn
	client     localaiproto.BackendClient
}

func (connection *fixtureGRPCConnection) Negotiate(
	ctx context.Context,
	request serviceedges.ModelHostProtocolNegotiationRequest,
) (serviceedges.ModelHostProtocolNegotiationResult, error) {
	if request.ProtocolVersion != PinnedHostProtocolVersion {
		return serviceedges.ModelHostProtocolNegotiationResult{}, models.ErrHostProtocolIncompatible
	}
	health, err := connection.client.Health(ctx, &localaiproto.HealthMessage{})
	if err != nil {
		return serviceedges.ModelHostProtocolNegotiationResult{}, err
	}
	if string(health.GetMessage()) != FixtureHealthMessage {
		return serviceedges.ModelHostProtocolNegotiationResult{}, models.ErrHostProtocolIncompatible
	}
	return serviceedges.ModelHostProtocolNegotiationResult{
		ProtocolVersion: request.ProtocolVersion,
		Backend:         request.Backend,
		Ready:           true,
	}, nil
}

func (connection *fixtureGRPCConnection) Close() error {
	if connection == nil || connection.connection == nil {
		return nil
	}
	return connection.connection.Close()
}
