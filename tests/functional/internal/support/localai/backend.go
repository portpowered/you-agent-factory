package localai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/models"
	localaiproto "github.com/portpowered/infinite-you/tests/functional/internal/support/localai/protocol"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const (
	fixtureDialTimeout = 5 * time.Second
)

// InvocationBackend adapts the fixture's pinned gRPC protocol to the narrow
// Models edge used by the HTTP conformance test. It intentionally returns
// detached Models values and performs no production registration or routing.
func (fixture *Fixture) InvocationBackend(
	ctx context.Context,
	request models.InvokeModelRequest,
) ([]models.InferenceContent, []models.InferenceArtifact, error) {
	if fixture == nil {
		return nil, nil, backendFailure(
			request,
			models.InvocationFailureClassBackendReadiness,
			"LocalAI backend is unavailable; start the managed backend and verify its health",
			errors.New("LocalAI fixture is nil"),
		)
	}
	connection, client, err := fixture.dial(ctx)
	if err != nil {
		return nil, nil, backendRPCFailure(request, err)
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
		return fixture.invokeEMBED(ctx, client, request, inputs)
	case models.OperationTTS:
		return fixture.invokeTTS(ctx, client, request, inputs)
	case models.OperationASR:
		return fixture.invokeASR(ctx, client, request, inputs)
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
		return nil, nil, backendRPCFailure(request, err)
	}
	if response == nil || len(response.GetMessage()) == 0 {
		return nil, nil, malformedResponse(request, "text")
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
	request models.InvokeModelRequest,
	inputs []models.InferenceInput,
) ([]models.InferenceContent, []models.InferenceArtifact, error) {
	response, err := client.Embedding(ctx, &localaiproto.PredictOptions{Prompt: firstInputContent(inputs)})
	if err != nil {
		return nil, nil, backendRPCFailure(request, err)
	}
	if response == nil || len(response.GetEmbeddings()) == 0 {
		return nil, nil, malformedResponse(request, "embedding")
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
		return nil, nil, backendRPCFailure(request, err)
	}
	response, err := stream.Recv()
	if err != nil {
		return nil, nil, backendRPCFailure(request, err)
	}
	if response == nil || len(response.GetAudio()) == 0 {
		return nil, nil, malformedResponse(request, "audio")
	}
	return []models.InferenceContent{{
		Name:        "audio",
		Modality:    models.ModalityAudio,
		ContentType: "audio/wav",
		MediaType:   "audio/wav",
		// The generic Models boundary carries arbitrary bytes in a Go string;
		// only the text-only ASR fixture field needs a transport encoding.
		Content: string(response.GetAudio()),
	}}, nil, nil
}

// ASRBackend adapts the fixture protocol to the typed Models ASR effect. It
// returns decoded transcript facts; the Models codec owns validation and
// named-output materialization.
func (fixture *Fixture) ASRBackend(
	ctx context.Context,
	request models.ASRBackendRequest,
) (models.ASRBackendResponse, error) {
	if fixture == nil {
		return models.ASRBackendResponse{}, errors.New("LocalAI fixture is nil")
	}
	connection, client, err := fixture.dial(ctx)
	if err != nil {
		return models.ASRBackendResponse{}, err
	}
	defer func() { _ = connection.Close() }()

	// The pinned fixture protocol carries ASR input in a protobuf string field,
	// so encode the Models-owned audio bytes before crossing that wire.
	response, err := client.AudioTranscription(ctx, asrProtocolRequest(request))
	if err != nil {
		return models.ASRBackendResponse{}, fmt.Errorf("invoke LocalAI ASR: %w", err)
	}
	if response == nil || strings.TrimSpace(response.GetText()) == "" {
		return models.ASRBackendResponse{}, errors.New("LocalAI ASR response is missing transcript")
	}
	segments := make([]models.ASRBackendSegment, 0, len(response.GetSegments()))
	for _, segment := range response.GetSegments() {
		if segment == nil {
			continue
		}
		segments = append(segments, models.ASRBackendSegment{
			ID: segment.GetId(), Start: segment.GetStart(), End: segment.GetEnd(), Text: segment.GetText(),
		})
	}
	return models.ASRBackendResponse{Text: response.GetText(), Segments: segments}, nil
}

func (fixture *Fixture) invokeASR(
	ctx context.Context,
	client localaiproto.BackendClient,
	request models.InvokeModelRequest,
	inputs []models.InferenceInput,
) ([]models.InferenceContent, []models.InferenceArtifact, error) {
	response, err := client.AudioTranscription(ctx, &localaiproto.TranscriptRequest{
		Prompt: firstInputContent(inputs),
	})
	if err != nil {
		return nil, nil, backendRPCFailure(request, err)
	}
	if response == nil || strings.TrimSpace(response.GetText()) == "" {
		return nil, nil, malformedResponse(request, "transcript")
	}
	if len(response.GetSegments()) == 0 {
		return nil, nil, malformedResponse(request, "segments")
	}
	segments := make([]models.ASRBackendSegment, 0, len(response.GetSegments()))
	for _, segment := range response.GetSegments() {
		if segment == nil {
			continue
		}
		segments = append(segments, models.ASRBackendSegment{
			ID: segment.GetId(), Start: segment.GetStart(), End: segment.GetEnd(), Text: segment.GetText(),
		})
	}
	if len(segments) == 0 {
		return nil, nil, malformedResponse(request, "segments")
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

func asrProtocolRequest(request models.ASRBackendRequest) *localaiproto.TranscriptRequest {
	return &localaiproto.TranscriptRequest{
		Prompt: base64.StdEncoding.EncodeToString(request.Audio),
	}
}

func firstInputContent(inputs []models.InferenceInput) string {
	if len(inputs) == 0 {
		return ""
	}
	return inputs[0].Content
}

func malformedResponse(request models.InvokeModelRequest, slot string) error {
	model := invocationModelName(request)
	operation := invocationOperationName(request)
	return &models.InvocationFailure{
		Class:     models.InvocationFailureClassMalformedResponse,
		Model:     invocationModelReference(request),
		Operation: operation,
		Slot:      slot,
		Message: fmt.Sprintf(
			"LocalAI backend returned malformed response for model %q operation %q: output slot %q is missing; verify the pinned LocalAI response contract",
			model, operation, slot,
		),
	}
}

func backendRPCFailure(request models.InvokeModelRequest, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	switch status.Code(err) {
	case codes.Unavailable:
		return backendFailure(
			request,
			models.InvocationFailureClassBackendReadiness,
			"LocalAI backend is unavailable; start the managed backend and verify its health",
			err,
		)
	case codes.FailedPrecondition:
		return backendFailure(
			request,
			models.InvocationFailureClassBackendProtocol,
			"LocalAI backend protocol is incompatible; use the pinned LocalAI backend protocol",
			err,
		)
	default:
		return backendFailure(
			request,
			models.InvocationFailureClassBackendProtocol,
			"LocalAI backend returned an unusable response; verify the pinned LocalAI backend contract",
			err,
		)
	}
}

func backendFailure(
	request models.InvokeModelRequest,
	class models.InvocationFailureClass,
	action string,
	cause error,
) error {
	model := invocationModelName(request)
	operation := invocationOperationName(request)
	return &models.InvocationFailure{
		Class:     class,
		Model:     invocationModelReference(request),
		Operation: operation,
		Message:   fmt.Sprintf("%s for model %q operation %q", action, model, operation),
		Cause:     cause,
	}
}

func invocationModelReference(request models.InvokeModelRequest) models.ModelReference {
	if !request.Model.IsZero() {
		return request.Model
	}
	return models.ModelReference{NameOrURI: request.ModelName}
}

func invocationModelName(request models.InvokeModelRequest) string {
	model := strings.TrimSpace(invocationModelReference(request).NameOrURI)
	if model == "" {
		return "unknown"
	}
	return model
}

func invocationOperationName(request models.InvokeModelRequest) string {
	operation := strings.ToUpper(strings.TrimSpace(request.Operation))
	if operation == "" {
		return "unknown"
	}
	return operation
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
