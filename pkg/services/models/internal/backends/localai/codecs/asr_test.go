package codecs_test

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai/codecs"
)

func TestASRCodecMapsAudioBytesAndMediaToFixtureRequest(t *testing.T) {
	codec := codecs.NewASRCodec()
	request := models.InvokeModelRequest{
		Operation: models.OperationASR,
		Inputs: []models.InferenceInput{
			{
				Name: "audio", Modality: models.ModalityAudio,
				ContentType: "audio/wav", MediaType: "audio/wav",
				Content: string([]byte{0, 1, 255, 127}),
			},
			{
				Name: "prompt", Modality: models.ModalityText,
				ContentType: "text/plain", MediaType: "text/plain", Content: "meeting",
			},
			{
				Name: "parameters", Modality: models.ModalityJSON,
				ContentType: "application/json", MediaType: "application/json",
				Content: `{"language":"en","temperature":0.2}`,
			},
		},
	}

	mapped, err := codec.EncodeRequest(request)
	if err != nil {
		t.Fatalf("EncodeRequest() error = %v", err)
	}
	if string(mapped.Audio) != string([]byte{0, 1, 255, 127}) || mapped.MediaType != "audio/wav" || mapped.Prompt != "meeting" {
		t.Fatalf("mapped request = %#v, want exact audio/media/prompt", mapped)
	}

	payload, err := codec.MarshalRequest(request)
	if err != nil {
		t.Fatalf("MarshalRequest() error = %v", err)
	}
	want, err := os.ReadFile("testdata/asr-request.json")
	if err != nil {
		t.Fatalf("read request fixture: %v", err)
	}
	if string(payload) != strings.TrimSpace(string(want)) {
		t.Fatalf("request payload = %s, want %s", payload, want)
	}
}

func TestASRCodecMapsFixtureResponseToCanonicalNamedOutputs(t *testing.T) {
	codec := codecs.NewASRCodec()
	payload, err := os.ReadFile("testdata/asr-response.json")
	if err != nil {
		t.Fatalf("read response fixture: %v", err)
	}

	outputs, err := codec.DecodeResponse(payload)
	if err != nil {
		t.Fatalf("DecodeResponse() error = %v", err)
	}
	if len(outputs) != 2 || outputs[0].Name != "transcript" || outputs[1].Name != "segments" {
		t.Fatalf("outputs = %#v, want transcript then segments", outputs)
	}
	if outputs[0].Content != "hello world" || outputs[0].MediaType != "text/plain" {
		t.Fatalf("transcript output = %#v", outputs[0])
	}
	var segments []codecs.ASRSegment
	if err := json.Unmarshal([]byte(outputs[1].Content), &segments); err != nil {
		t.Fatalf("segments output is not JSON: %v", err)
	}
	if len(segments) != 1 || segments[0].Start != 0 || segments[0].End != 1500 || segments[0].Text != "hello world" {
		t.Fatalf("segments = %#v, want exact timestamps", segments)
	}
}

func TestASRCodecRejectsInvalidInputsWithoutLeakingValues(t *testing.T) {
	validAudio := models.InferenceInput{
		Name: "audio", Modality: models.ModalityAudio,
		ContentType: "audio/wav", MediaType: "audio/wav", Content: "audio-bytes",
	}
	tests := []struct {
		name  string
		input []models.InferenceInput
		class models.InvocationFailureClass
	}{
		{
			name:  "missing audio",
			input: []models.InferenceInput{{Name: "prompt", Modality: models.ModalityText, ContentType: "text/plain", MediaType: "text/plain", Content: "hint"}},
			class: models.InvocationFailureClassInvalidSlot,
		},
		{
			name:  "wrong media",
			input: []models.InferenceInput{{Name: "audio", Modality: models.ModalityAudio, ContentType: "text/plain", MediaType: "text/plain", Content: "secret-audio"}},
			class: models.InvocationFailureClassMediaCapability,
		},
		{
			name:  "repeated audio",
			input: []models.InferenceInput{validAudio, validAudio},
			class: models.InvocationFailureClassSlotArity,
		},
		{
			name:  "malformed parameters",
			input: []models.InferenceInput{validAudio, {Name: "parameters", Modality: models.ModalityJSON, ContentType: "application/json", MediaType: "application/json", Content: `{"language":`}},
			class: models.InvocationFailureClassInvalidParameter,
		},
		{
			name:  "unsupported parameter",
			input: []models.InferenceInput{validAudio, {Name: "parameters", Modality: models.ModalityJSON, ContentType: "application/json", MediaType: "application/json", Content: `{"token":"secret"}`}},
			class: models.InvocationFailureClassInvalidParameter,
		},
	}

	codec := codecs.NewASRCodec()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := codec.EncodeRequest(models.InvokeModelRequest{Operation: models.OperationASR, Inputs: test.input})
			if err == nil {
				t.Fatal("EncodeRequest() error = nil")
			}
			var failure *models.InvocationFailure
			if !errors.As(err, &failure) || failure.Class != test.class {
				t.Fatalf("error = %v, failure = %#v, want class %q", err, failure, test.class)
			}
			if strings.Contains(err.Error(), "secret-audio") || strings.Contains(err.Error(), "secret") {
				t.Fatalf("failure leaked input value: %v", err)
			}
		})
	}
}

func TestASRCodecRejectsMalformedOversizedAndInvalidTimestampResponsesAtomically(t *testing.T) {
	codec := codecs.NewASRCodec()
	for _, payload := range [][]byte{
		[]byte(`{"text":"hello","segments":[]}`),
		[]byte(`{"text":"hello","segments":[{"id":0,"start":2,"end":1,"text":"bad"}]}`),
		[]byte(`{"text":"hello","segments":[{"id":0,"start":0,"end":1,"text":"ok"}]} trailing`),
		[]byte(strings.Repeat("x", int(codecs.MaxASRResponseBytes)+1)),
	} {
		outputs, err := codec.DecodeResponse(payload)
		if err == nil {
			t.Fatalf("DecodeResponse(%q) error = nil", payload[:minASRTest(len(payload), 24)])
		}
		if len(outputs) != 0 {
			t.Fatalf("malformed response returned partial outputs: %#v", outputs)
		}
		var failure *models.InvocationFailure
		if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassMalformedResponse {
			t.Fatalf("error = %#v, want malformed InvocationFailure", err)
		}
	}
}

func minASRTest(left, right int) int {
	if left < right {
		return left
	}
	return right
}
