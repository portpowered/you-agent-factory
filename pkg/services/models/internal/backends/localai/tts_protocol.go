package localai

import (
	"context"
	"errors"
	"os"
	"strings"

	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai/codecs"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	localAITTSMethod        = "/backend.Backend/TTS"
	ttsOutputFilePattern    = ".you-model-tts-*.wav"
	ttsAudioMediaType       = "audio/wav"
	ttsProtocolErrorMessage = "TTS backend protocol request failed"
)

// TTSOutputInspector and TTSOutputReader are the private filesystem effects
// used to consume the backend's path-based unary result. They are injected so
// tests never touch a real profile or cache.
type TTSOutputInspector func(string) (os.FileInfo, error)
type TTSOutputReader func(string) ([]byte, error)

// NewPinnedTTSBackend constructs the private adapter for LocalAI's unary,
// path-based TTS RPC. The returned backend owns one temporary destination per
// invocation and returns a detached provider-neutral response. A nil result
// means the complete private dependency boundary was not supplied.
func NewPinnedTTSBackend(
	dialer platformgrpc.Dialer,
	tempDirectory func() string,
	createTempFile TempFileFactory,
	inspectFile TTSOutputInspector,
	readFile TTSOutputReader,
	removeFile InputFileRemover,
) func(context.Context, codecs.TTSRequest) (codecs.TTSResponse, error) {
	if dialer == nil || tempDirectory == nil || createTempFile == nil || inspectFile == nil || readFile == nil || removeFile == nil {
		return nil
	}
	client := grpcProtocolClient{dialer: dialer}
	return func(ctx context.Context, request codecs.TTSRequest) (codecs.TTSResponse, error) {
		return client.synthesize(ctx, request, tempDirectory, createTempFile, inspectFile, readFile, removeFile)
	}
}

func (client grpcProtocolClient) synthesize(
	ctx context.Context,
	request codecs.TTSRequest,
	tempDirectory func() string,
	createTempFile TempFileFactory,
	inspectFile TTSOutputInspector,
	readFile TTSOutputReader,
	removeFile InputFileRemover,
) (codecs.TTSResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return codecs.TTSResponse{}, err
	}
	if strings.TrimSpace(request.Text) == "" {
		return codecs.TTSResponse{}, ttsProtocolFailure("TTS request is invalid", models.ErrInferenceFailed)
	}
	path, err := reserveTTSOutput(ctx, tempDirectory, createTempFile, removeFile)
	if err != nil {
		return codecs.TTSResponse{}, err
	}
	defer func() { _ = removeFile(path) }()

	protocolRequest, err := ttsProtocolRequest(path, request)
	if err != nil {
		return codecs.TTSResponse{}, err
	}
	endpoint, _ := ctx.Value(invocationEndpointContextKey{}).(string)
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return codecs.TTSResponse{}, ttsReadinessFailure("TTS backend endpoint is unavailable")
	}
	if client.dialer == nil {
		return codecs.TTSResponse{}, ttsReadinessFailure("TTS backend dialer is unavailable")
	}
	connection, err := client.dialer.Dial(ctx, endpoint)
	if err != nil {
		if connection != nil {
			_ = connection.Close()
		}
		return codecs.TTSResponse{}, ttsTransportFailure(ctx, "TTS backend connection failed", err)
	}
	if connection == nil {
		return codecs.TTSResponse{}, ttsReadinessFailure("TTS backend connection is unavailable")
	}
	defer func() { _ = connection.Close() }()

	payload, err := proto.Marshal(protocolRequest)
	if err != nil {
		return codecs.TTSResponse{}, ttsProtocolFailure(ttsProtocolErrorMessage, models.ErrInferenceFailed)
	}
	responsePayload, err := connection.Invoke(ctx, localAITTSMethod, payload)
	if err != nil {
		return codecs.TTSResponse{}, ttsTransportFailure(ctx, "TTS backend request failed", err)
	}
	result := &Result{}
	if err := proto.Unmarshal(responsePayload, result); err != nil {
		return codecs.TTSResponse{}, ttsMalformedResultFailure()
	}
	if !result.GetSuccess() {
		return codecs.TTSResponse{}, ttsProtocolFailure("TTS backend did not produce audio", models.ErrInferenceFailed)
	}
	if err := ctx.Err(); err != nil {
		return codecs.TTSResponse{}, err
	}
	audio, err := readTTSOutput(path, inspectFile, readFile)
	if err != nil {
		return codecs.TTSResponse{}, err
	}
	if err := ctx.Err(); err != nil {
		return codecs.TTSResponse{}, err
	}
	response := codecs.TTSResponse{Audio: audio, MediaType: ttsAudioMediaType}
	if _, err := codecs.NewTTSCodec().DecodeResponse(response); err != nil {
		return codecs.TTSResponse{}, err
	}
	return response, nil
}

func reserveTTSOutput(
	ctx context.Context,
	tempDirectory func() string,
	createTempFile TempFileFactory,
	removeFile InputFileRemover,
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	temporary, err := createTempFile(tempDirectory(), ttsOutputFilePattern)
	path := ""
	if temporary != nil {
		path = temporary.Name()
	}
	if err != nil || temporary == nil || strings.TrimSpace(path) == "" {
		if temporary != nil {
			_ = temporary.Close()
		}
		if path != "" {
			_ = removeFile(path)
		}
		return "", ttsProtocolFailure("TTS output staging is unavailable", models.ErrInferenceFailed)
	}
	if err := temporary.Close(); err != nil {
		_ = removeFile(path)
		return "", ttsProtocolFailure("TTS output staging could not be closed", models.ErrInferenceFailed)
	}
	if err := ctx.Err(); err != nil {
		_ = removeFile(path)
		return "", err
	}
	return path, nil
}

func ttsProtocolRequest(path string, request codecs.TTSRequest) (*TTSRequest, error) {
	result := &TTSRequest{
		Text:  request.Text,
		Model: request.Model,
		Dst:   path,
	}
	for name, value := range request.Parameters {
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, ttsInvalidParameterFailure(name)
		}
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "language":
			result.Language = stringPointer(text)
		case "instructions":
			result.Instructions = stringPointer(text)
		default:
			return nil, ttsInvalidParameterFailure(name)
		}
	}
	return result, nil
}

func stringPointer(value string) *string { return &value }

func readTTSOutput(
	path string,
	inspectFile TTSOutputInspector,
	readFile TTSOutputReader,
) ([]byte, error) {
	info, err := inspectFile(path)
	if err != nil || info == nil || info.Size() <= 0 || info.Size() > codecs.MaxTTSAudioBytes {
		return nil, ttsMalformedAudioFailure()
	}
	audio, err := readFile(path)
	if err != nil || int64(len(audio)) != info.Size() || int64(len(audio)) > codecs.MaxTTSAudioBytes {
		return nil, ttsMalformedAudioFailure()
	}
	return append([]byte(nil), audio...), nil
}

func ttsTransportFailure(ctx context.Context, message string, err error) error {
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	if errors.Is(err, models.ErrUnavailable) || status.Code(err) == codes.Unavailable {
		return ttsReadinessFailure("TTS backend is unavailable")
	}
	if status.Code(err) == codes.FailedPrecondition {
		return &models.InvocationFailure{
			Class: models.InvocationFailureClassBackendProtocol, Operation: models.OperationTTS,
			Message: "TTS backend protocol is incompatible", Cause: models.ErrInferenceFailed,
		}
	}
	return ttsProtocolFailure(message, models.ErrInferenceFailed)
}

func ttsReadinessFailure(message string) error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassBackendReadiness, Operation: models.OperationTTS,
		Message: message, Cause: models.ErrUnavailable,
	}
}

func ttsProtocolFailure(message string, cause error) error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassBackendProtocol, Operation: models.OperationTTS,
		Message: message, Cause: cause,
	}
}

func ttsMalformedResultFailure() error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassMalformedResponse, Operation: models.OperationTTS,
		Slot: "audio", Message: "TTS backend result is malformed", Cause: models.ErrInferenceFailed,
	}
}

func ttsMalformedAudioFailure() error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassMalformedResponse, Operation: models.OperationTTS,
		Slot: "audio", Message: "TTS backend audio is malformed", Cause: models.ErrInferenceFailed,
	}
}

func ttsInvalidParameterFailure(name string) error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassInvalidParameter, Operation: models.OperationTTS,
		Parameter: strings.TrimSpace(name), Message: "TTS parameter has an invalid value",
	}
}
