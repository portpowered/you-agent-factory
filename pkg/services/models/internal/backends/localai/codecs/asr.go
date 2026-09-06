// Package codecs contains the private wire mappings used by the LocalAI
// model backend. The codec boundary keeps provider protocol values out of
// the public Models contracts.
package codecs

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

const (
	// MaxASRResponseBytes bounds the backend response retained by one
	// transcription. It also bounds the canonical segments output.
	MaxASRResponseBytes int64 = 16 << 20
	maxASRSegments            = 1 << 20
)

var supportedASRParameters = []string{
	"diarize",
	"language",
	"temperature",
	"threads",
	"timestamp_granularities",
	"translate",
}

// ASRRequest is the provider-neutral representation passed to a LocalAI ASR
// protocol adapter. Audio is kept as bytes so the adapter cannot silently
// replace or re-encode the caller's payload.
type ASRRequest struct {
	Audio      []byte         `json:"audio"`
	MediaType  string         `json:"media_type"`
	Prompt     string         `json:"prompt,omitempty"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

// ASRSegment is the canonical timestamped segment representation emitted by
// the ASR adapter. Start and End are backend timestamp units and are preserved
// without rounding.
type ASRSegment struct {
	ID    int32  `json:"id"`
	Start int64  `json:"start"`
	End   int64  `json:"end"`
	Text  string `json:"text"`
}

// ASRResponse is the narrow decoded LocalAI transcription response.
type ASRResponse struct {
	Text     string       `json:"text"`
	Segments []ASRSegment `json:"segments"`
}

// ASRCodec maps the generic Models ASR contract to and from the private
// LocalAI representation. Its zero value is ready for use.
type ASRCodec struct{}

// AudioTranscriptionCodec is a descriptive alias for callers that prefer the
// backend operation's full name.
type AudioTranscriptionCodec = ASRCodec

// NewASRCodec constructs the ASR codec.
func NewASRCodec() ASRCodec { return ASRCodec{} }

type asrRequestBuilder struct {
	audio          []byte
	audioMedia     string
	audioSeen      bool
	prompt         string
	parametersSeen bool
	promptSeen     bool
	parameters     map[string]any
}

func newASRRequestBuilder() asrRequestBuilder {
	return asrRequestBuilder{parameters: make(map[string]any)}
}

func (builder *asrRequestBuilder) addInput(input models.InferenceInput) error {
	switch strings.TrimSpace(input.Name) {
	case "audio":
		if builder.audioSeen {
			return asrRepeatedSlotFailure("audio")
		}
		mediaType, err := asrValidateAudioInput(input)
		if err != nil {
			return err
		}
		builder.audio = []byte(input.Content)
		builder.audioMedia = mediaType
		builder.audioSeen = true
	case "prompt":
		if builder.promptSeen {
			return asrRepeatedSlotFailure("prompt")
		}
		if err := asrValidatePromptInput(input); err != nil {
			return err
		}
		builder.prompt = input.Content
		builder.promptSeen = true
	case "parameters":
		if builder.parametersSeen {
			return asrRepeatedSlotFailure("parameters")
		}
		if err := asrValidateParametersInput(input); err != nil {
			return err
		}
		parsed, err := asrParseParameterObject(input.Content)
		if err != nil {
			return asrInvalidParametersFailure()
		}
		for name, value := range parsed {
			if err := asrAddParameter(builder.parameters, name, value); err != nil {
				return err
			}
		}
		builder.parametersSeen = true
	default:
		return asrUnknownSlotFailure(input.Name)
	}
	return nil
}

// EncodeRequest validates and maps one generic Models request. The returned
// audio and parameters are detached from caller-owned values.
func (ASRCodec) EncodeRequest(request models.InvokeModelRequest) (ASRRequest, error) {
	if operation := strings.TrimSpace(request.Operation); operation != "" && !strings.EqualFold(operation, models.OperationASR) {
		return ASRRequest{}, asrInvalidOperationFailure(operation)
	}

	inputs := request.Inputs
	if len(inputs) == 0 && asrHasInput(request.Input) {
		inputs = []models.InferenceInput{request.Input}
	}

	builder := newASRRequestBuilder()
	for _, input := range inputs {
		if err := builder.addInput(input); err != nil {
			return ASRRequest{}, err
		}
	}

	if !builder.audioSeen {
		return ASRRequest{}, asrMissingAudioFailure()
	}
	for _, parameter := range request.Parameters {
		if err := asrAddParameter(builder.parameters, parameter.Name, parameter.Value); err != nil {
			return ASRRequest{}, err
		}
	}
	if len(builder.parameters) == 0 {
		builder.parameters = nil
	}
	return ASRRequest{
		Audio:      append([]byte(nil), builder.audio...),
		MediaType:  builder.audioMedia,
		Prompt:     builder.prompt,
		Parameters: asrCloneObject(builder.parameters),
	}, nil
}

// MarshalRequest serializes the mapped request. encoding/json represents the
// audio bytes as base64, which is lossless and deterministic for protocol
// adapters that use JSON envelopes.
func (codec ASRCodec) MarshalRequest(request models.InvokeModelRequest) ([]byte, error) {
	mapped, err := codec.EncodeRequest(request)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(mapped)
	if err != nil {
		return nil, asrInvalidParametersFailure()
	}
	return payload, nil
}

// DecodeResponse validates a JSON response and returns both required named
// outputs atomically. On failure it returns no partial output.
func (codec ASRCodec) DecodeResponse(payload []byte) ([]models.InferenceContent, error) {
	if int64(len(payload)) > MaxASRResponseBytes {
		return nil, asrMalformedResponseFailure("")
	}
	var response ASRResponse
	if err := asrDecodeSingleJSON(payload, &response); err != nil {
		return nil, asrMalformedResponseFailure("")
	}
	return codec.DecodeResponseValue(response)
}

// DecodeResponseValue validates an already decoded backend response and
// emits canonical transcript and JSON segments content in declared order.
func (ASRCodec) DecodeResponseValue(response ASRResponse) ([]models.InferenceContent, error) {
	return decodeASRResponse(response, nil)
}

// DecodeResponseValueWithinAudio applies the response invariants and, when
// the request is a PCM WAV, bounds every segment by the decoded input
// duration. LocalAI reports segment timestamps in milliseconds; keeping this
// check at the private codec boundary prevents an otherwise valid-looking
// transcript from publishing impossible media coordinates.
func (ASRCodec) DecodeResponseValueWithinAudio(response ASRResponse, audio []byte) ([]models.InferenceContent, error) {
	durationMilliseconds, hasDuration := pcmWAVDurationMilliseconds(audio)
	if !hasDuration {
		return decodeASRResponse(response, nil)
	}
	return decodeASRResponse(response, &durationMilliseconds)
}

func decodeASRResponse(response ASRResponse, durationMilliseconds *float64) ([]models.InferenceContent, error) {
	if strings.TrimSpace(response.Text) == "" {
		return nil, asrMalformedResponseFailure("transcript")
	}
	if len(response.Segments) == 0 || len(response.Segments) > maxASRSegments {
		return nil, asrMalformedResponseFailure("segments")
	}
	var previousStart, previousEnd int64
	for index, segment := range response.Segments {
		if segment.ID < 0 || segment.Start < 0 || segment.End <= segment.Start || strings.TrimSpace(segment.Text) == "" {
			return nil, asrMalformedResponseFailure("segments")
		}
		if index > 0 && (segment.Start < previousStart || segment.End < previousEnd) {
			return nil, asrMalformedResponseFailure("segments")
		}
		if durationMilliseconds != nil && float64(segment.End) > *durationMilliseconds {
			return nil, asrMalformedResponseFailure("segments")
		}
		previousStart, previousEnd = segment.Start, segment.End
	}
	segments, err := json.Marshal(response.Segments)
	if err != nil || int64(len(segments)) > MaxASRResponseBytes {
		return nil, asrMalformedResponseFailure("segments")
	}
	return []models.InferenceContent{
		{
			Name: "transcript", Modality: models.ModalityText,
			ContentType: "text/plain", MediaType: "text/plain", Content: response.Text,
		},
		{
			Name: "segments", Modality: models.ModalityJSON,
			ContentType: "application/json", MediaType: "application/json", Content: string(segments),
		},
	}, nil
}

func pcmWAVDurationMilliseconds(audio []byte) (float64, bool) {
	if len(audio) < 12 || string(audio[0:4]) != "RIFF" || string(audio[8:12]) != "WAVE" ||
		uint64(binary.LittleEndian.Uint32(audio[4:8]))+8 != uint64(len(audio)) {
		return 0, false
	}
	var sampleRate uint32
	var blockAlign uint16
	var dataBytes uint64
	formatFound, dataFound := false, false
	position := 12
	for position+8 <= len(audio) {
		chunkSize := uint64(binary.LittleEndian.Uint32(audio[position+4 : position+8]))
		chunkStart := position + 8
		chunkEnd := uint64(chunkStart) + chunkSize
		next := chunkEnd + chunkSize%2
		if chunkEnd > uint64(len(audio)) || next > uint64(len(audio)) {
			return 0, false
		}
		switch string(audio[position : position+4]) {
		case "fmt ":
			if formatFound || chunkSize < 16 {
				return 0, false
			}
			chunk := audio[chunkStart:int(chunkEnd)]
			format := binary.LittleEndian.Uint16(chunk[0:2])
			channels := binary.LittleEndian.Uint16(chunk[2:4])
			sampleRate = binary.LittleEndian.Uint32(chunk[4:8])
			blockAlign = binary.LittleEndian.Uint16(chunk[12:14])
			bits := binary.LittleEndian.Uint16(chunk[14:16])
			if format != 1 || channels == 0 || sampleRate == 0 || blockAlign == 0 || bits == 0 {
				return 0, false
			}
			formatFound = true
		case "data":
			if dataFound {
				return 0, false
			}
			dataBytes = chunkSize
			dataFound = true
		}
		position = int(next)
	}
	if !formatFound || !dataFound || dataBytes == 0 || dataBytes%uint64(blockAlign) != 0 {
		return 0, false
	}
	return float64(dataBytes/uint64(blockAlign)) * 1000 / float64(sampleRate), true
}

func asrHasInput(input models.InferenceInput) bool {
	return input.Name != "" || input.Modality != "" || input.ContentType != "" || input.MediaType != "" || input.Content != "" || input.Artifact != nil
}

func asrValidateAudioInput(input models.InferenceInput) (string, error) {
	if input.Modality != models.ModalityAudio || input.Artifact != nil || len(input.Content) == 0 {
		return "", asrInvalidAudioFailure()
	}
	mediaType := strings.TrimSpace(input.MediaType)
	if mediaType == "" {
		mediaType = strings.TrimSpace(input.ContentType)
	}
	if !asrMatchesMediaType(mediaType, "audio/*") {
		return "", asrInvalidAudioFailure()
	}
	return strings.ToLower(strings.SplitN(mediaType, ";", 2)[0]), nil
}

func asrValidatePromptInput(input models.InferenceInput) error {
	if input.Modality != models.ModalityText || input.Artifact != nil ||
		(input.ContentType != "" && !strings.EqualFold(strings.TrimSpace(input.ContentType), "text/plain")) ||
		(input.MediaType != "" && !strings.EqualFold(strings.TrimSpace(input.MediaType), "text/plain")) ||
		strings.TrimSpace(input.Content) == "" {
		return &models.InvocationFailure{
			Class: models.InvocationFailureClassMediaCapability, Operation: models.OperationASR,
			Slot: "prompt", Message: "ASR prompt input must be non-empty text/plain content",
		}
	}
	return nil
}

func asrValidateParametersInput(input models.InferenceInput) error {
	if input.Modality != models.ModalityJSON || input.Artifact != nil ||
		(input.ContentType != "" && !strings.EqualFold(strings.TrimSpace(input.ContentType), "application/json")) ||
		(input.MediaType != "" && !strings.EqualFold(strings.TrimSpace(input.MediaType), "application/json")) {
		return asrInvalidParametersFailure()
	}
	return nil
}

func asrParseParameterObject(content string) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("trailing JSON value")
		}
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("parameters are not an object")
	}
	return object, nil
}

func asrAddParameter(parameters map[string]any, name string, value any) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return asrInvalidParameterFailure(name)
	}
	if _, exists := parameters[name]; exists {
		return asrRepeatedParameterFailure(name)
	}
	if err := asrValidateParameter(name, value); err != nil {
		return err
	}
	parameters[name] = asrCloneJSONValue(value)
	return nil
}

var asrParameterValidators = map[string]func(any) bool{
	"language":                asrValidLanguage,
	"threads":                 asrPositiveInteger,
	"translate":               asrValidBoolean,
	"diarize":                 asrValidBoolean,
	"temperature":             asrFiniteNumber,
	"timestamp_granularities": asrValidTimestampGranularities,
}

func asrValidateParameter(name string, value any) error {
	validator, ok := asrParameterValidators[name]
	if !ok {
		return asrUnsupportedParameterFailure(name)
	}
	if !validator(value) {
		return asrInvalidParameterFailure(name)
	}
	return nil
}

func asrValidLanguage(value any) bool {
	language, ok := value.(string)
	return ok && strings.TrimSpace(language) != ""
}

func asrValidBoolean(value any) bool {
	_, ok := value.(bool)
	return ok
}

func asrValidTimestampGranularities(value any) bool {
	values, ok := value.([]any)
	if !ok || len(values) == 0 {
		return false
	}
	for _, item := range values {
		granularity, ok := item.(string)
		if !ok || (granularity != "segment" && granularity != "word") {
			return false
		}
	}
	return true
}

func asrPositiveInteger(value any) bool {
	switch number := value.(type) {
	case json.Number:
		integer, err := number.Int64()
		return err == nil && integer > 0
	case int:
		return number > 0
	case int64:
		return number > 0
	case float64:
		return !math.IsNaN(number) && !math.IsInf(number, 0) && math.Trunc(number) == number && number > 0
	default:
		return false
	}
}

func asrFiniteNumber(value any) bool {
	switch number := value.(type) {
	case json.Number:
		_, err := number.Float64()
		return err == nil
	case float64:
		return !math.IsNaN(number) && !math.IsInf(number, 0)
	case float32:
		return !math.IsNaN(float64(number)) && !math.IsInf(float64(number), 0)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	default:
		return false
	}
}

func asrMatchesMediaType(actual, declared string) bool {
	actual = strings.ToLower(strings.TrimSpace(strings.SplitN(actual, ";", 2)[0]))
	declared = strings.ToLower(strings.TrimSpace(strings.SplitN(declared, ";", 2)[0]))
	if actual == "" || !strings.HasPrefix(actual, "audio/") {
		return false
	}
	return declared == "audio/*" || declared == actual
}

func asrDecodeSingleJSON(payload []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return err
	}
	return nil
}

func asrCloneObject(object map[string]any) map[string]any {
	if object == nil {
		return nil
	}
	cloned := make(map[string]any, len(object))
	for name, value := range object {
		cloned[name] = asrCloneJSONValue(value)
	}
	return cloned
}

func asrCloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return asrCloneObject(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = asrCloneJSONValue(item)
		}
		return cloned
	default:
		return value
	}
}

func asrInvalidOperationFailure(operation string) error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassInvalidOperation, Operation: models.OperationASR,
		Message: "ASR invocation requires the ASR operation",
	}
}

func asrMissingAudioFailure() error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassInvalidSlot, Operation: models.OperationASR,
		Slot: "audio", ValidNames: []string{"audio", "parameters", "prompt"},
		Message: "ASR invocation requires one audio input",
	}
}

func asrUnknownSlotFailure(slot string) error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassInvalidSlot, Operation: models.OperationASR,
		Slot: slot, ValidNames: []string{"audio", "parameters", "prompt"},
		Message: "ASR invocation contains an unknown input slot",
	}
}

func asrRepeatedSlotFailure(slot string) error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassSlotArity, Operation: models.OperationASR,
		Slot: slot, Message: "ASR invocation input slot may appear only once",
	}
}

func asrInvalidAudioFailure() error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassMediaCapability, Operation: models.OperationASR,
		Slot: "audio", Message: "ASR audio input must be non-empty audio/* content",
	}
}

func asrInvalidParametersFailure() error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassInvalidParameter, Operation: models.OperationASR,
		Parameter: "parameters", Message: "ASR parameters must be a JSON object",
	}
}

func asrInvalidParameterFailure(name string) error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassInvalidParameter, Operation: models.OperationASR,
		Parameter: name, ValidNames: append([]string(nil), supportedASRParameters...),
		Message: "ASR parameter has an invalid value",
	}
}

func asrUnsupportedParameterFailure(name string) error {
	validNames := append([]string(nil), supportedASRParameters...)
	sort.Strings(validNames)
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassInvalidParameter, Operation: models.OperationASR,
		Parameter: name, ValidNames: validNames, Message: "ASR parameter is not supported",
	}
}

func asrRepeatedParameterFailure(name string) error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassInvalidParameter, Operation: models.OperationASR,
		Parameter: name, Message: "ASR parameter may be provided only once",
	}
}

func asrMalformedResponseFailure(slot string) error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassMalformedResponse, Operation: models.OperationASR,
		Slot: slot, Message: "ASR backend response is malformed", Cause: models.ErrInferenceFailed,
	}
}
