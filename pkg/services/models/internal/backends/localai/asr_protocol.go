package localai

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"strings"

	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"google.golang.org/protobuf/proto"
)

const (
	localAITranscriptionMethod = "/backend.Backend/AudioTranscription"
	// The pinned whisper bridge assigns this field directly to whisper.cpp's
	// n_threads setting. Keep omitted optional parameters valid for the real
	// backend instead of sending protobuf's zero value.
	localAIASRDefaultThreads uint32 = 4
)

// TempFile is the small lifecycle handle needed to reserve one private ASR
// input path. The writer is deliberately supplied separately so the staging
// policy stays at the Models construction boundary.
type TempFile interface {
	io.Closer
	Name() string
}

// TempFileFactory reserves one temporary path for a staged ASR input.
type TempFileFactory func(string, string) (TempFile, error)

// InputFileWriter and InputFileRemover are injected filesystem effects used
// only for the short-lived audio copy required by LocalAI's path-based RPC.
type InputFileWriter func(string, []byte) error
type InputFileRemover func(string) error

// NewPinnedASRBackend constructs the production adapter for LocalAI's pinned
// path-based AudioTranscription RPC. It returns nil when the composition root
// has not supplied the complete transport and staging boundary.
func NewPinnedASRBackend(
	dialer platformgrpc.Dialer,
	tempDirectory func() string,
	createTempFile TempFileFactory,
	writeFile InputFileWriter,
	removeFile InputFileRemover,
) func(context.Context, models.ASRBackendRequest) (models.ASRBackendResponse, error) {
	if dialer == nil || tempDirectory == nil || createTempFile == nil || writeFile == nil || removeFile == nil {
		return nil
	}
	client := grpcProtocolClient{dialer: dialer}
	return func(ctx context.Context, request models.ASRBackendRequest) (models.ASRBackendResponse, error) {
		return client.transcribe(ctx, request, tempDirectory, createTempFile, writeFile, removeFile)
	}
}

func (client grpcProtocolClient) transcribe(
	ctx context.Context,
	request models.ASRBackendRequest,
	tempDirectory func() string,
	createTempFile TempFileFactory,
	writeFile InputFileWriter,
	removeFile InputFileRemover,
) (models.ASRBackendResponse, error) {
	path, cleanup, err := stageASRAudio(ctx, request, tempDirectory, createTempFile, writeFile, removeFile)
	if err != nil {
		return models.ASRBackendResponse{}, err
	}
	defer cleanup()

	protocolRequest, err := transcriptRequest(path, request)
	if err != nil {
		return models.ASRBackendResponse{}, err
	}
	response := &TranscriptResult{}
	if err := client.invokeProto(ctx, localAITranscriptionMethod, protocolRequest, response); err != nil {
		return models.ASRBackendResponse{}, err
	}
	return transcriptResponse(response), nil
}

func stageASRAudio(
	ctx context.Context,
	request models.ASRBackendRequest,
	tempDirectory func() string,
	createTempFile TempFileFactory,
	writeFile InputFileWriter,
	removeFile InputFileRemover,
) (string, func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", func() {}, err
	}
	if len(request.Audio) == 0 {
		return "", func() {}, asrProtocolFailure("ASR audio input is empty", nil)
	}
	temporary, err := createTempFile(tempDirectory(), ".you-model-asr-*"+audioFileSuffix(request.MediaType))
	if err != nil || temporary == nil || strings.TrimSpace(temporary.Name()) == "" {
		if temporary != nil {
			path := strings.TrimSpace(temporary.Name())
			_ = temporary.Close()
			if path != "" {
				_ = removeFile(path)
			}
		}
		return "", func() {}, asrProtocolFailure("ASR audio staging is unavailable", err)
	}
	path := temporary.Name()
	cleanup := func() { _ = removeFile(path) }
	if err := temporary.Close(); err != nil {
		cleanup()
		return "", func() {}, asrProtocolFailure("ASR audio staging could not be closed", err)
	}
	if err := writeFile(path, request.Audio); err != nil {
		cleanup()
		return "", func() {}, asrProtocolFailure("ASR audio staging could not be written", err)
	}
	if err := ctx.Err(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return path, cleanup, nil
}

func transcriptRequest(path string, request models.ASRBackendRequest) (*TranscriptRequest, error) {
	result := &TranscriptRequest{
		Dst: path, Prompt: request.Prompt, Threads: localAIASRDefaultThreads,
	}
	for name, value := range request.Parameters {
		if err := applyTranscriptParameter(result, name, value); err != nil {
			return nil, err
		}
	}
	return result, nil
}

type transcriptParameterHandler func(*TranscriptRequest, any) error

var transcriptParameterHandlers = map[string]transcriptParameterHandler{
	"language":                applyTranscriptLanguage,
	"threads":                 applyTranscriptThreads,
	"translate":               applyTranscriptTranslate,
	"diarize":                 applyTranscriptDiarize,
	"temperature":             applyTranscriptTemperature,
	"timestamp_granularities": applyTranscriptTimestampGranularities,
}

func applyTranscriptParameter(result *TranscriptRequest, name string, value any) error {
	name = strings.TrimSpace(name)
	handler, ok := transcriptParameterHandlers[name]
	if !ok {
		return invalidASRParameter(name)
	}
	return handler(result, value)
}

func applyTranscriptLanguage(result *TranscriptRequest, value any) error {
	language, ok := value.(string)
	if !ok || strings.TrimSpace(language) == "" {
		return invalidASRParameter("language")
	}
	result.Language = language
	return nil
}

func applyTranscriptThreads(result *TranscriptRequest, value any) error {
	threads, ok := asrUint32(value)
	if !ok {
		return invalidASRParameter("threads")
	}
	result.Threads = threads
	return nil
}

func applyTranscriptTranslate(result *TranscriptRequest, value any) error {
	return applyTranscriptBoolean(&result.Translate, "translate", value)
}

func applyTranscriptDiarize(result *TranscriptRequest, value any) error {
	return applyTranscriptBoolean(&result.Diarize, "diarize", value)
}

func applyTranscriptBoolean(target *bool, name string, value any) error {
	boolean, ok := value.(bool)
	if !ok {
		return invalidASRParameter(name)
	}
	*target = boolean
	return nil
}

func applyTranscriptTemperature(result *TranscriptRequest, value any) error {
	temperature, ok := asrFloat32(value)
	if !ok {
		return invalidASRParameter("temperature")
	}
	result.Temperature = temperature
	return nil
}

func applyTranscriptTimestampGranularities(result *TranscriptRequest, value any) error {
	granularities, ok := asrStringSlice(value)
	if !ok || len(granularities) == 0 {
		return invalidASRParameter("timestamp_granularities")
	}
	result.TimestampGranularities = granularities
	return nil
}

func asrUint32(value any) (uint32, bool) {
	switch number := value.(type) {
	case json.Number:
		parsed, err := number.Int64()
		if err != nil {
			return 0, false
		}
		return asrPositiveUint32Int64(parsed)
	case int:
		return asrPositiveUint32Int64(int64(number))
	case int64:
		return asrPositiveUint32Int64(number)
	case uint:
		return asrPositiveUint32Uint64(uint64(number))
	case uint32:
		return asrPositiveUint32Uint64(uint64(number))
	case uint64:
		return asrPositiveUint32Uint64(number)
	case float64:
		return asrPositiveUint32Float64(number)
	default:
		return 0, false
	}
}

func asrPositiveUint32Int64(number int64) (uint32, bool) {
	if number <= 0 {
		return 0, false
	}
	return asrPositiveUint32Uint64(uint64(number))
}

func asrPositiveUint32Uint64(number uint64) (uint32, bool) {
	if number == 0 || number > math.MaxUint32 {
		return 0, false
	}
	return uint32(number), true
}

func asrPositiveUint32Float64(number float64) (uint32, bool) {
	if math.IsNaN(number) || math.IsInf(number, 0) || math.Trunc(number) != number || number <= 0 || number > math.MaxUint32 {
		return 0, false
	}
	return uint32(number), true
}

func asrFloat32(value any) (float32, bool) {
	var number float64
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		number = parsed
	case float64:
		number = typed
	case float32:
		number = float64(typed)
	case int:
		number = float64(typed)
	case int64:
		number = float64(typed)
	default:
		return 0, false
	}
	if math.IsNaN(number) || math.IsInf(number, 0) || math.Abs(number) > math.MaxFloat32 {
		return 0, false
	}
	return float32(number), true
}

func asrStringSlice(value any) ([]string, bool) {
	switch values := value.(type) {
	case []string:
		result := append([]string(nil), values...)
		for _, item := range result {
			if item != "segment" && item != "word" {
				return nil, false
			}
		}
		return result, true
	case []any:
		result := make([]string, len(values))
		for index, value := range values {
			item, ok := value.(string)
			if !ok || (item != "segment" && item != "word") {
				return nil, false
			}
			result[index] = item
		}
		return result, true
	default:
		return nil, false
	}
}

func transcriptResponse(response *TranscriptResult) models.ASRBackendResponse {
	if response == nil {
		return models.ASRBackendResponse{}
	}
	result := models.ASRBackendResponse{Text: response.GetText()}
	result.Segments = make([]models.ASRBackendSegment, 0, len(response.GetSegments()))
	for _, segment := range response.GetSegments() {
		if segment == nil {
			continue
		}
		result.Segments = append(result.Segments, models.ASRBackendSegment{
			ID: segment.GetId(), Start: segment.GetStart(), End: segment.GetEnd(), Text: segment.GetText(),
		})
	}
	return result
}

func (client grpcProtocolClient) invokeProto(
	ctx context.Context,
	method string,
	request proto.Message,
	response proto.Message,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	endpoint, _ := ctx.Value(invocationEndpointContextKey{}).(string)
	if strings.TrimSpace(endpoint) == "" {
		return asrProtocolFailure("ASR backend endpoint is unavailable", nil)
	}
	if client.dialer == nil {
		return asrProtocolFailure("ASR backend dialer is unavailable", models.ErrUnavailable)
	}
	payload, err := proto.Marshal(request)
	if err != nil {
		return asrProtocolFailure("ASR backend request could not be serialized", err)
	}
	connection, err := client.dialer.Dial(ctx, endpoint)
	if err != nil {
		return protocolContextOrFailure(ctx, "ASR backend connection failed", err)
	}
	if connection == nil {
		return asrProtocolFailure("ASR backend connection is unavailable", models.ErrUnavailable)
	}
	defer func() { _ = connection.Close() }()
	responsePayload, err := connection.Invoke(ctx, method, payload)
	if err != nil {
		return protocolContextOrFailure(ctx, "ASR backend request failed", err)
	}
	if err := proto.Unmarshal(responsePayload, response); err != nil {
		return asrProtocolFailure("ASR backend response was malformed", err)
	}
	return nil
}

func protocolContextOrFailure(ctx context.Context, message string, err error) error {
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	return asrProtocolFailure(message, err)
}

func asrProtocolFailure(message string, cause error) error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassBackendProtocol, Operation: models.OperationASR,
		Message: message, Cause: cause,
	}
}

func invalidASRParameter(name string) error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassInvalidParameter, Operation: models.OperationASR,
		Parameter: strings.TrimSpace(name), Message: "ASR parameter is invalid",
	}
}

func audioFileSuffix(mediaType string) string {
	mediaType = strings.ToLower(strings.TrimSpace(strings.SplitN(mediaType, ";", 2)[0]))
	switch mediaType {
	case "audio/wav", "audio/wave", "audio/x-wav":
		return ".wav"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/mp4", "audio/x-m4a":
		return ".m4a"
	case "audio/ogg":
		return ".ogg"
	case "audio/flac":
		return ".flac"
	case "audio/webm":
		return ".webm"
	default:
		return ".audio"
	}
}
