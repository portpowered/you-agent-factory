// Package codecs contains the private wire mappings used by the LocalAI
// model backend. The codec deliberately exposes only provider-neutral Models
// contracts at its public boundary.
package codecs

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

const (
	// MaxEmbeddingResponseBytes bounds the amount of backend data that the
	// codec will retain while decoding one embedding response.
	MaxEmbeddingResponseBytes int64 = 16 << 20
	maxEmbeddingDimensions          = 1 << 20
)

var supportedEmbeddingParameters = []string{
	"dimensions",
	"encoding_format",
	"normalize",
}

// EmbeddingRequest is the small fixture/backend representation used by the
// EMBED adapter. It contains no model paths, credentials, or backend handles.
type EmbeddingRequest struct {
	Prompt     string         `json:"prompt"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

// EmbeddingResponse is the backend representation of one embedding result.
type EmbeddingResponse struct {
	Embeddings []float64 `json:"embeddings"`
}

// EmbedCodec maps the generic Models EMBED contract to and from the narrow
// LocalAI fixture protocol. Its zero value is ready for use.
type EmbedCodec struct{}

// EmbeddingCodec is a descriptive alias for callers that prefer the full
// operation name.
type EmbeddingCodec = EmbedCodec

// EMBEDCodec preserves the operation's contract spelling for internal users.
type EMBEDCodec = EmbedCodec

// EMBEDRequest is an operation-spelling alias for the fixture request.
type EMBEDRequest = EmbeddingRequest

// EMBEDResponse is an operation-spelling alias for the fixture response.
type EMBEDResponse = EmbeddingResponse

// NewEmbedCodec constructs the EMBED codec.
func NewEmbedCodec() EmbedCodec {
	return EmbedCodec{}
}

// EncodeRequest validates and maps a generic Models invocation to the
// provider-neutral fixture request. The returned map is detached from the
// caller's input.
func (EmbedCodec) EncodeRequest(request models.InvokeModelRequest) (EmbeddingRequest, error) {
	if operation := strings.TrimSpace(request.Operation); operation != "" && !strings.EqualFold(operation, models.OperationEMBED) {
		return EmbeddingRequest{}, invalidOperationFailure(operation)
	}

	inputs := request.Inputs
	if len(inputs) == 0 && hasInput(request.Input) {
		inputs = []models.InferenceInput{request.Input}
	}

	var (
		text       string
		textSeen   bool
		paramsSeen bool
		parameters = make(map[string]any)
	)
	for _, input := range inputs {
		switch input.Name {
		case "text":
			if textSeen {
				return EmbeddingRequest{}, repeatedSlotFailure("text")
			}
			if err := validateTextInput(input); err != nil {
				return EmbeddingRequest{}, err
			}
			text = input.Content
			textSeen = true
		case "parameters":
			if paramsSeen {
				return EmbeddingRequest{}, repeatedSlotFailure("parameters")
			}
			if err := validateParametersInput(input); err != nil {
				return EmbeddingRequest{}, err
			}
			parsed, err := parseParameterObject(input.Content)
			if err != nil {
				return EmbeddingRequest{}, invalidParametersFailure()
			}
			for name, value := range parsed {
				if err := addParameter(parameters, name, value); err != nil {
					return EmbeddingRequest{}, err
				}
			}
			paramsSeen = true
		default:
			return EmbeddingRequest{}, unknownSlotFailure(input.Name)
		}
	}

	if !textSeen {
		return EmbeddingRequest{}, missingTextFailure()
	}
	for _, parameter := range request.Parameters {
		if err := addParameter(parameters, parameter.Name, parameter.Value); err != nil {
			return EmbeddingRequest{}, err
		}
	}

	if len(parameters) == 0 {
		parameters = nil
	}
	return EmbeddingRequest{
		Prompt:     text,
		Parameters: cloneObject(parameters),
	}, nil
}

// MarshalRequest maps and serializes a request using deterministic JSON
// object ordering.
func (codec EmbedCodec) MarshalRequest(request models.InvokeModelRequest) ([]byte, error) {
	mapped, err := codec.EncodeRequest(request)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(mapped)
	if err != nil {
		return nil, invalidParametersFailure()
	}
	return payload, nil
}

// DecodeResponse validates one backend response and returns exactly one
// detached, canonical Models output.
func (codec EmbedCodec) DecodeResponse(payload []byte) (models.InferenceContent, error) {
	if int64(len(payload)) > MaxEmbeddingResponseBytes {
		return models.InferenceContent{}, malformedResponseFailure()
	}

	var response EmbeddingResponse
	if err := decodeSingleJSON(payload, &response); err != nil {
		return models.InferenceContent{}, malformedResponseFailure()
	}
	return codec.DecodeResponseValue(response)
}

// DecodeResponseValue validates a decoded fixture response. It is useful for
// backend adapters that already decoded their protocol response.
func (codec EmbedCodec) DecodeResponseValue(response EmbeddingResponse) (models.InferenceContent, error) {
	if len(response.Embeddings) == 0 || len(response.Embeddings) > maxEmbeddingDimensions {
		return models.InferenceContent{}, malformedResponseFailure()
	}
	for _, value := range response.Embeddings {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return models.InferenceContent{}, malformedResponseFailure()
		}
	}

	content, err := json.Marshal(response.Embeddings)
	if err != nil || int64(len(content)) > MaxEmbeddingResponseBytes {
		return models.InferenceContent{}, malformedResponseFailure()
	}
	return models.InferenceContent{
		Name:        "embedding",
		Modality:    models.ModalityJSON,
		ContentType: "application/json",
		MediaType:   "application/json",
		Content:     string(content),
	}, nil
}

func hasInput(input models.InferenceInput) bool {
	return input.Name != "" || input.Modality != "" || input.ContentType != "" || input.MediaType != "" || input.Content != "" || input.Artifact != nil
}

func validateTextInput(input models.InferenceInput) error {
	if input.Modality != models.ModalityText ||
		(input.ContentType != "" && input.ContentType != "text/plain") ||
		(input.MediaType != "" && input.MediaType != "text/plain") ||
		input.Artifact != nil || strings.TrimSpace(input.Content) == "" {
		return invalidTextInputFailure()
	}
	return nil
}

func validateParametersInput(input models.InferenceInput) error {
	if input.Modality != models.ModalityJSON ||
		(input.ContentType != "" && input.ContentType != "application/json") ||
		(input.MediaType != "" && input.MediaType != "application/json") ||
		input.Artifact != nil {
		return invalidParametersFailure()
	}
	return nil
}

func parseParameterObject(content string) (map[string]any, error) {
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

func decodeSingleJSON(payload []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return io.ErrUnexpectedEOF
		}
		return err
	}
	return nil
}

func addParameter(parameters map[string]any, name string, value any) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return invalidParameterFailure(name)
	}
	if _, exists := parameters[name]; exists {
		return repeatedParameterFailure(name)
	}
	if err := validateParameter(name, value); err != nil {
		return err
	}
	parameters[name] = cloneJSONValue(value)
	return nil
}

func validateParameter(name string, value any) error {
	switch name {
	case "normalize":
		if _, ok := value.(bool); !ok {
			return invalidParameterFailure(name)
		}
	case "dimensions":
		if !positiveInteger(value) {
			return invalidParameterFailure(name)
		}
	case "encoding_format":
		format, ok := value.(string)
		if !ok || format != "float" {
			return invalidParameterFailure(name)
		}
	default:
		return unsupportedParameterFailure(name)
	}
	return nil
}

func positiveInteger(value any) bool {
	switch number := value.(type) {
	case json.Number:
		integer, err := number.Int64()
		return err == nil && integer > 0 && integer <= maxEmbeddingDimensions
	case float64:
		return !math.IsNaN(number) && !math.IsInf(number, 0) && math.Trunc(number) == number && number > 0 && number <= maxEmbeddingDimensions
	case float32:
		converted := float64(number)
		return !math.IsNaN(converted) && !math.IsInf(converted, 0) && math.Trunc(converted) == converted && converted > 0 && converted <= maxEmbeddingDimensions
	case int:
		return number > 0 && number <= maxEmbeddingDimensions
	case int8:
		return number > 0
	case int16:
		return number > 0
	case int32:
		return number > 0
	case int64:
		return number > 0 && number <= maxEmbeddingDimensions
	case uint:
		return number > 0 && number <= maxEmbeddingDimensions
	case uint8:
		return number > 0
	case uint16:
		return number > 0
	case uint32:
		return number > 0 && number <= maxEmbeddingDimensions
	case uint64:
		return number > 0 && number <= maxEmbeddingDimensions
	default:
		return false
	}
}

func cloneObject(object map[string]any) map[string]any {
	if object == nil {
		return nil
	}
	cloned := make(map[string]any, len(object))
	for name, value := range object {
		cloned[name] = cloneJSONValue(value)
	}
	return cloned
}

func cloneJSONValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		return cloneObject(value)
	case []any:
		cloned := make([]any, len(value))
		for index, item := range value {
			cloned[index] = cloneJSONValue(item)
		}
		return cloned
	default:
		return value
	}
}

func invalidOperationFailure(operation string) error {
	return &models.InvocationFailure{
		Class:     models.InvocationFailureClassInvalidOperation,
		Operation: models.OperationEMBED,
		Message:   "EMBED invocation requires the EMBED operation",
	}
}

func missingTextFailure() error {
	return &models.InvocationFailure{
		Class:      models.InvocationFailureClassInvalidSlot,
		Operation:  models.OperationEMBED,
		Slot:       "text",
		ValidNames: []string{"parameters", "text"},
		Message:    "EMBED invocation requires one text input",
	}
}

func unknownSlotFailure(slot string) error {
	return &models.InvocationFailure{
		Class:      models.InvocationFailureClassInvalidSlot,
		Operation:  models.OperationEMBED,
		Slot:       slot,
		ValidNames: []string{"parameters", "text"},
		Message:    "EMBED invocation contains an unknown input slot",
	}
}

func repeatedSlotFailure(slot string) error {
	return &models.InvocationFailure{
		Class:     models.InvocationFailureClassSlotArity,
		Operation: models.OperationEMBED,
		Slot:      slot,
		Message:   "EMBED invocation input slot may appear only once",
	}
}

func invalidTextInputFailure() error {
	return &models.InvocationFailure{
		Class:     models.InvocationFailureClassMediaCapability,
		Operation: models.OperationEMBED,
		Slot:      "text",
		Message:   "EMBED text input must be non-empty text/plain content",
	}
}

func invalidParametersFailure() error {
	return &models.InvocationFailure{
		Class:     models.InvocationFailureClassInvalidParameter,
		Operation: models.OperationEMBED,
		Parameter: "parameters",
		Message:   "EMBED parameters must be a JSON object",
	}
}

func invalidParameterFailure(name string) error {
	return &models.InvocationFailure{
		Class:      models.InvocationFailureClassInvalidParameter,
		Operation:  models.OperationEMBED,
		Parameter:  name,
		ValidNames: append([]string(nil), supportedEmbeddingParameters...),
		Message:    "EMBED parameter has an invalid value",
	}
}

func unsupportedParameterFailure(name string) error {
	validNames := append([]string(nil), supportedEmbeddingParameters...)
	sort.Strings(validNames)
	return &models.InvocationFailure{
		Class:      models.InvocationFailureClassInvalidParameter,
		Operation:  models.OperationEMBED,
		Parameter:  name,
		ValidNames: validNames,
		Message:    "EMBED parameter is not supported",
	}
}

func repeatedParameterFailure(name string) error {
	return &models.InvocationFailure{
		Class:     models.InvocationFailureClassInvalidParameter,
		Operation: models.OperationEMBED,
		Parameter: name,
		Message:   "EMBED parameter may be provided only once",
	}
}

func malformedResponseFailure() error {
	return &models.InvocationFailure{
		Class:     models.InvocationFailureClassMalformedResponse,
		Operation: models.OperationEMBED,
		Message:   "EMBED backend response is malformed",
		Cause:     models.ErrInferenceFailed,
	}
}
