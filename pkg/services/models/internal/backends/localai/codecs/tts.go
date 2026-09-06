// Package codecs contains the private wire mappings used by the LocalAI
// model backend. The codec boundary keeps provider protocol values out of
// the public Models contracts.
package codecs

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

const (
	// MaxTTSAudioBytes bounds the detached audio retained from one TTS result.
	// The pinned VibeVoice backend produces short PCM WAV files; keeping the
	// bound here prevents a malformed backend response from becoming an
	// unbounded allocation at the protocol boundary.
	MaxTTSAudioBytes int64 = 16 << 20
	// MaxTTSResponseBytes is the descriptive response-bound alias used by
	// callers that name the operation rather than its output media.
	MaxTTSResponseBytes = MaxTTSAudioBytes
)

var supportedTTSParameters = []string{"instructions", "language"}

// TTSRequest is the provider-neutral request passed to the private LocalAI
// protocol adapter. It deliberately contains no destination path, endpoint,
// credential, or backend-native handle.
type TTSRequest struct {
	Text       string         `json:"text"`
	Model      string         `json:"model,omitempty"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

// TTSResponse is the provider-neutral result returned by the private LocalAI
// protocol adapter. Audio is detached from the temporary backend destination.
type TTSResponse struct {
	Audio     []byte
	MediaType string
}

// TTSCodec maps the generic Models TTS contract to one pinned, provider-
// neutral request and one validated audio output. Its zero value is ready for
// use.
type TTSCodec struct{}

// TextToSpeechCodec is a descriptive alias for internal callers.
type TextToSpeechCodec = TTSCodec

// NewTTSCodec constructs the TTS codec.
func NewTTSCodec() TTSCodec { return TTSCodec{} }

// EncodeRequest validates and maps one generic Models TTS request. The voice
// slot is intentionally rejected: the current provider-neutral seam has no
// safe way to turn arbitrary audio content into the backend's path-like voice
// field without introducing a new staging contract.
func (TTSCodec) EncodeRequest(request models.InvokeModelRequest) (TTSRequest, error) {
	if operation := strings.TrimSpace(request.Operation); operation != "" && !strings.EqualFold(operation, models.OperationTTS) {
		return TTSRequest{}, ttsInvalidOperationFailure()
	}

	inputs := request.Inputs
	if len(inputs) == 0 && ttsHasInput(request.Input) {
		inputs = []models.InferenceInput{request.Input}
	}
	builder := newTTSRequestBuilder()
	for _, input := range inputs {
		if err := builder.addInput(input); err != nil {
			return TTSRequest{}, err
		}
	}
	if !builder.textSeen {
		return TTSRequest{}, ttsMissingTextFailure()
	}
	for _, parameter := range request.Parameters {
		if err := addTTSParameter(builder.parameters, parameter.Name, parameter.Value); err != nil {
			return TTSRequest{}, err
		}
	}
	if len(builder.parameters) == 0 {
		builder.parameters = nil
	}

	modelName := strings.TrimSpace(request.Model.NameOrURI)
	if modelName == "" {
		modelName = strings.TrimSpace(request.ModelName)
	}
	return TTSRequest{
		Text:       builder.text,
		Model:      modelName,
		Parameters: cloneTTSObject(builder.parameters),
	}, nil
}

// MarshalRequest maps and serializes a generic request for component tests
// and deterministic protocol fixtures. The LocalAI adapter uses protobuf,
// not this JSON representation, on the production wire.
func (codec TTSCodec) MarshalRequest(request models.InvokeModelRequest) ([]byte, error) {
	mapped, err := codec.EncodeRequest(request)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(mapped)
	if err != nil {
		return nil, ttsInvalidParametersFailure()
	}
	return payload, nil
}

// DecodeResponse validates one detached backend result and returns exactly
// one canonical audio output. No partial content is returned on failure.
func (TTSCodec) DecodeResponse(response TTSResponse) (models.InferenceContent, error) {
	mediaType := canonicalWAVMediaType(response.MediaType)
	if mediaType == "" || int64(len(response.Audio)) > MaxTTSAudioBytes || !validPCMWAV(response.Audio) {
		return models.InferenceContent{}, ttsMalformedResponseFailure()
	}
	audio := append([]byte(nil), response.Audio...)
	return models.InferenceContent{
		Name:        "audio",
		Modality:    models.ModalityAudio,
		ContentType: mediaType,
		MediaType:   mediaType,
		Content:     string(audio),
	}, nil
}

type ttsRequestBuilder struct {
	text       string
	textSeen   bool
	parameters map[string]any
}

func newTTSRequestBuilder() ttsRequestBuilder {
	return ttsRequestBuilder{parameters: make(map[string]any)}
}

func (builder *ttsRequestBuilder) addInput(input models.InferenceInput) error {
	switch strings.TrimSpace(input.Name) {
	case "text":
		if builder.textSeen {
			return ttsRepeatedSlotFailure("text")
		}
		if err := ttsValidateTextInput(input); err != nil {
			return err
		}
		builder.text = input.Content
		builder.textSeen = true
	case "parameters":
		if err := ttsValidateParametersInput(input); err != nil {
			return err
		}
		parsed, err := parseTTSParameterObject(input.Content)
		if err != nil {
			return ttsInvalidParametersFailure()
		}
		for name, value := range parsed {
			if err := addTTSParameter(builder.parameters, name, value); err != nil {
				return err
			}
		}
	case "voice":
		return ttsUnsupportedVoiceFailure()
	default:
		return ttsUnknownSlotFailure(input.Name)
	}
	return nil
}

func ttsHasInput(input models.InferenceInput) bool {
	return input.Name != "" || input.Modality != "" || input.ContentType != "" ||
		input.MediaType != "" || input.Content != "" || input.Artifact != nil
}

func ttsValidateTextInput(input models.InferenceInput) error {
	if input.Modality != models.ModalityText || input.Artifact != nil ||
		(input.ContentType != "" && !strings.EqualFold(strings.TrimSpace(input.ContentType), "text/plain")) ||
		(input.MediaType != "" && !strings.EqualFold(strings.TrimSpace(input.MediaType), "text/plain")) ||
		strings.TrimSpace(input.Content) == "" {
		return ttsInvalidTextFailure()
	}
	return nil
}

func ttsValidateParametersInput(input models.InferenceInput) error {
	if input.Modality != models.ModalityJSON || input.Artifact != nil ||
		(input.ContentType != "" && !strings.EqualFold(strings.TrimSpace(input.ContentType), "application/json")) ||
		(input.MediaType != "" && !strings.EqualFold(strings.TrimSpace(input.MediaType), "application/json")) {
		return ttsInvalidParametersFailure()
	}
	return nil
}

func parseTTSParameterObject(content string) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, io.ErrUnexpectedEOF
		}
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, io.ErrUnexpectedEOF
	}
	return object, nil
}

func addTTSParameter(parameters map[string]any, name string, value any) error {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ttsInvalidParameterFailure(name)
	}
	if _, exists := parameters[name]; exists {
		return ttsRepeatedParameterFailure(name)
	}
	if !ttsParameterValueSupported(name, value) {
		if !containsTTSParameter(name) {
			return ttsUnsupportedParameterFailure(name)
		}
		return ttsInvalidParameterFailure(name)
	}
	parameters[name] = cloneTTSJSONValue(value)
	return nil
}

func ttsParameterValueSupported(name string, value any) bool {
	if name != "language" && name != "instructions" {
		return false
	}
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) != ""
}

func containsTTSParameter(name string) bool {
	for _, supported := range supportedTTSParameters {
		if name == supported {
			return true
		}
	}
	return false
}

func cloneTTSObject(object map[string]any) map[string]any {
	if object == nil {
		return nil
	}
	cloned := make(map[string]any, len(object))
	for name, value := range object {
		cloned[name] = cloneTTSJSONValue(value)
	}
	return cloned
}

func cloneTTSJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneTTSObject(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneTTSJSONValue(item)
		}
		return cloned
	default:
		return value
	}
}

func canonicalWAVMediaType(mediaType string) string {
	mediaType = strings.ToLower(strings.TrimSpace(strings.SplitN(mediaType, ";", 2)[0]))
	switch mediaType {
	case "audio/wav", "audio/wave", "audio/x-wav":
		return "audio/wav"
	default:
		return ""
	}
}

func validPCMWAV(audio []byte) bool {
	if len(audio) < 12 || string(audio[0:4]) != "RIFF" || string(audio[8:12]) != "WAVE" {
		return false
	}
	if uint64(binary.LittleEndian.Uint32(audio[4:8]))+8 != uint64(len(audio)) {
		return false
	}
	var formatFound, dataFound bool
	var blockAlign uint16
	position := 12
	for position+8 <= len(audio) {
		chunkSize := uint64(binary.LittleEndian.Uint32(audio[position+4 : position+8]))
		chunkStart := position + 8
		chunkEnd := uint64(chunkStart) + chunkSize
		if chunkEnd > uint64(len(audio)) {
			return false
		}
		switch string(audio[position : position+4]) {
		case "fmt ":
			if formatFound || chunkSize < 16 {
				return false
			}
			format := binary.LittleEndian.Uint16(audio[chunkStart : chunkStart+2])
			channels := binary.LittleEndian.Uint16(audio[chunkStart+2 : chunkStart+4])
			sampleRate := binary.LittleEndian.Uint32(audio[chunkStart+4 : chunkStart+8])
			byteRate := binary.LittleEndian.Uint32(audio[chunkStart+8 : chunkStart+12])
			blockAlign = binary.LittleEndian.Uint16(audio[chunkStart+12 : chunkStart+14])
			bits := binary.LittleEndian.Uint16(audio[chunkStart+14 : chunkStart+16])
			if format != 1 || channels == 0 || sampleRate == 0 || blockAlign == 0 || bits == 0 ||
				uint64(sampleRate)*uint64(blockAlign) != uint64(byteRate) {
				return false
			}
			formatFound = true
		case "data":
			if dataFound || chunkSize == 0 || blockAlign == 0 || chunkSize%uint64(blockAlign) != 0 {
				return false
			}
			dataFound = true
		}
		position = int(chunkEnd)
		if chunkSize%2 != 0 {
			position++
		}
		if position > len(audio) {
			return false
		}
	}
	return formatFound && dataFound && position == len(audio)
}

func ttsInvalidOperationFailure() error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassInvalidOperation, Operation: models.OperationTTS,
		Message: "TTS invocation requires the TTS operation",
	}
}

func ttsMissingTextFailure() error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassInvalidSlot, Operation: models.OperationTTS,
		Slot: "text", ValidNames: []string{"parameters", "text", "voice"},
		Message: "TTS invocation requires one text input",
	}
}

func ttsUnknownSlotFailure(slot string) error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassInvalidSlot, Operation: models.OperationTTS,
		Slot: strings.TrimSpace(slot), ValidNames: []string{"parameters", "text", "voice"},
		Message: "TTS invocation contains an unknown input slot",
	}
}

func ttsRepeatedSlotFailure(slot string) error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassSlotArity, Operation: models.OperationTTS,
		Slot: strings.TrimSpace(slot), Message: "TTS invocation input slot may appear only once",
	}
}

func ttsInvalidTextFailure() error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassMediaCapability, Operation: models.OperationTTS,
		Slot: "text", Message: "TTS text input must be non-empty text/plain content",
	}
}

func ttsUnsupportedVoiceFailure() error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassMediaCapability, Operation: models.OperationTTS,
		Slot: "voice", Message: "TTS voice input is not supported by the pinned runtime",
	}
}

func ttsInvalidParametersFailure() error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassInvalidParameter, Operation: models.OperationTTS,
		Parameter: "parameters", ValidNames: append([]string(nil), supportedTTSParameters...),
		Message: "TTS parameters must be a JSON object",
	}
}

func ttsInvalidParameterFailure(name string) error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassInvalidParameter, Operation: models.OperationTTS,
		Parameter: strings.TrimSpace(name), ValidNames: append([]string(nil), supportedTTSParameters...),
		Message: "TTS parameter has an invalid value",
	}
}

func ttsUnsupportedParameterFailure(name string) error {
	validNames := append([]string(nil), supportedTTSParameters...)
	sort.Strings(validNames)
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassInvalidParameter, Operation: models.OperationTTS,
		Parameter: strings.TrimSpace(name), ValidNames: validNames,
		Message: "TTS parameter is not supported by the pinned runtime",
	}
}

func ttsRepeatedParameterFailure(name string) error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassInvalidParameter, Operation: models.OperationTTS,
		Parameter: strings.TrimSpace(name), Message: "TTS parameter may be provided only once",
	}
}

func ttsMalformedResponseFailure() error {
	return &models.InvocationFailure{
		Class: models.InvocationFailureClassMalformedResponse, Operation: models.OperationTTS,
		Slot: "audio", Message: "TTS backend response is malformed", Cause: models.ErrInferenceFailed,
	}
}
