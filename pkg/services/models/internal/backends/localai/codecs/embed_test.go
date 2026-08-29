package codecs_test

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai/codecs"
)

func TestEmbedCodecMapsRequestToFixtureProtocol(t *testing.T) {
	codec := codecs.NewEmbedCodec()
	request := models.InvokeModelRequest{
		Operation: models.OperationEMBED,
		Inputs: []models.InferenceInput{
			{
				Name:        "text",
				Modality:    models.ModalityText,
				ContentType: "text/plain",
				MediaType:   "text/plain",
				Content:     "Find similar work",
			},
			{
				Name:        "parameters",
				Modality:    models.ModalityJSON,
				ContentType: "application/json",
				MediaType:   "application/json",
				Content:     `{"normalize":true,"dimensions":4}`,
			},
		},
	}

	got, err := codec.MarshalRequest(request)
	if err != nil {
		t.Fatalf("MarshalRequest() error = %v", err)
	}
	want, err := os.ReadFile("testdata/embed-request.json")
	if err != nil {
		t.Fatalf("read request fixture: %v", err)
	}
	if string(got) != strings.TrimSpace(string(want)) {
		t.Fatalf("request payload = %s, want %s", got, want)
	}
}

func TestEmbedCodecAcceptsGenericLogicalContentTypes(t *testing.T) {
	codec := codecs.NewEmbedCodec()
	request := models.InvokeModelRequest{
		Operation: models.OperationEMBED,
		Inputs: []models.InferenceInput{
			{
				Name: "text", Modality: models.ModalityText,
				ContentType: "TEXT", MediaType: "text/plain", Content: "logical type",
			},
			{
				Name: "parameters", Modality: models.ModalityJSON,
				ContentType: "JSON", MediaType: "application/json", Content: `{"normalize":true}`,
			},
		},
	}

	got, err := codec.EncodeRequest(request)
	if err != nil {
		t.Fatalf("EncodeRequest() error = %v", err)
	}
	if got.Prompt != "logical type" || got.Parameters["normalize"] != true {
		t.Fatalf("EncodeRequest() = %#v, want logical content types mapped", got)
	}
}

func TestEmbedCodecMapsFixtureResponseToOneCanonicalOutput(t *testing.T) {
	codec := codecs.NewEmbedCodec()
	payload, err := os.ReadFile("testdata/embed-response.json")
	if err != nil {
		t.Fatalf("read response fixture: %v", err)
	}

	output, err := codec.DecodeResponse(payload)
	if err != nil {
		t.Fatalf("DecodeResponse() error = %v", err)
	}
	if output.Name != "embedding" || output.Modality != models.ModalityJSON || output.ContentType != "application/json" || output.MediaType != "application/json" {
		t.Fatalf("output metadata = %#v", output)
	}
	var vector []float64
	if err := json.Unmarshal([]byte(output.Content), &vector); err != nil {
		t.Fatalf("output content is not JSON: %v", err)
	}
	if len(vector) != 4 || vector[0] != 0.1 || vector[3] != 0.4 {
		t.Fatalf("output vector = %#v", vector)
	}
}

func TestEmbedCodecRejectsInvalidInputsWithoutLeakingValues(t *testing.T) {
	tests := []struct {
		name  string
		input []models.InferenceInput
		class models.InvocationFailureClass
	}{
		{
			name:  "missing text",
			input: []models.InferenceInput{{Name: "parameters", Modality: models.ModalityJSON, ContentType: "application/json", MediaType: "application/json", Content: `{}`}},
			class: models.InvocationFailureClassInvalidSlot,
		},
		{
			name:  "unknown slot",
			input: []models.InferenceInput{{Name: "secret-slot", Modality: models.ModalityText, ContentType: "text/plain", MediaType: "text/plain", Content: "token-value"}},
			class: models.InvocationFailureClassInvalidSlot,
		},
		{
			name: "repeated text",
			input: []models.InferenceInput{
				textInput("first"),
				textInput("second"),
			},
			class: models.InvocationFailureClassSlotArity,
		},
		{
			name:  "malformed parameters",
			input: []models.InferenceInput{{Name: "parameters", Modality: models.ModalityJSON, ContentType: "application/json", MediaType: "application/json", Content: `{"normalize":`}, textInput("query")},
			class: models.InvocationFailureClassInvalidParameter,
		},
		{
			name:  "unsupported parameter",
			input: []models.InferenceInput{{Name: "parameters", Modality: models.ModalityJSON, ContentType: "application/json", MediaType: "application/json", Content: `{"temperature":0.2}`}, textInput("query")},
			class: models.InvocationFailureClassInvalidParameter,
		},
	}

	codec := codecs.NewEmbedCodec()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := codec.EncodeRequest(models.InvokeModelRequest{Operation: models.OperationEMBED, Inputs: test.input})
			if err == nil {
				t.Fatal("EncodeRequest() error = nil")
			}
			var failure *models.InvocationFailure
			if !errors.As(err, &failure) {
				t.Fatalf("error type = %T, want *models.InvocationFailure", err)
			}
			if failure.Class != test.class {
				t.Fatalf("failure class = %s, want %s", failure.Class, test.class)
			}
			if strings.Contains(err.Error(), "token-value") || strings.Contains(err.Error(), "second") || strings.Contains(err.Error(), "temperature") {
				t.Fatalf("failure leaked input value: %v", err)
			}
		})
	}
}

func TestEmbedCodecRejectsMalformedAndOversizedResponsesAtomically(t *testing.T) {
	codec := codecs.NewEmbedCodec()
	for _, payload := range [][]byte{
		[]byte(`{"embeddings":[]}`),
		[]byte(`{"embeddings":[1]} trailing`),
		[]byte(`{"embeddings":["not-a-number"]}`),
		[]byte(strings.Repeat("x", int(codecs.MaxEmbeddingResponseBytes)+1)),
	} {
		output, err := codec.DecodeResponse(payload)
		if err == nil {
			t.Fatalf("DecodeResponse(%q) error = nil", payload[:min(len(payload), 24)])
		}
		if output != (models.InferenceContent{}) {
			t.Fatalf("malformed response returned partial output: %#v", output)
		}
		var failure *models.InvocationFailure
		if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassMalformedResponse {
			t.Fatalf("error = %#v, want malformed InvocationFailure", err)
		}
	}

	output, err := codec.DecodeResponseValue(codecs.EmbeddingResponse{Embeddings: []float64{math.NaN()}})
	if err == nil || output != (models.InferenceContent{}) {
		t.Fatalf("non-finite response output = %#v, error = %v", output, err)
	}
}

func textInput(content string) models.InferenceInput {
	return models.InferenceInput{
		Name:        "text",
		Modality:    models.ModalityText,
		ContentType: "text/plain",
		MediaType:   "text/plain",
		Content:     content,
	}
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
