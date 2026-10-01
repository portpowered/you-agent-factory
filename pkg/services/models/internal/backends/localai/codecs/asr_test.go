package codecs_test

import (
	"encoding/binary"
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

func TestASRCodecRejectsMalformedAndInvalidTimestampResponsesAtomically(t *testing.T) {
	codec := codecs.NewASRCodec()
	tests := []struct {
		name        string
		payload     []byte
		slot        string
		wantMessage string
	}{
		{name: "missing segments", payload: []byte(`{"text":"hello","segments":[]}`), slot: "segments", wantMessage: "ASR backend response is missing segments"},
		{name: "invalid segment bounds", payload: []byte(`{"text":"hello","segments":[{"id":0,"start":2,"end":1,"text":"bad"}]}`), slot: "segments", wantMessage: "ASR backend response contains an invalid segment"},
		{name: "trailing JSON", payload: []byte(`{"text":"hello","segments":[{"id":0,"start":0,"end":1,"text":"ok"}]} trailing`), slot: "", wantMessage: "ASR backend response is not valid JSON"},
		{name: "missing transcript", payload: []byte(`{"text":"  ","segments":[{"id":0,"start":0,"end":1,"text":"ok"}]}`), slot: "transcript", wantMessage: "ASR backend response is missing the transcript"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outputs, err := codec.DecodeResponse(test.payload)
			if err == nil {
				t.Fatalf("DecodeResponse() error = nil")
			}
			if len(outputs) != 0 {
				t.Fatalf("malformed response returned partial outputs: %#v", outputs)
			}
			var failure *models.InvocationFailure
			if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassMalformedResponse {
				t.Fatalf("error = %#v, want malformed InvocationFailure", err)
			}
			if failure.Operation != models.OperationASR || failure.Slot != test.slot || failure.Message != test.wantMessage {
				t.Fatalf("failure = %#v, want operation %q slot %q message %q", failure, models.OperationASR, test.slot, test.wantMessage)
			}
			if !errors.Is(err, models.ErrInferenceFailed) {
				t.Fatalf("error = %v, want ErrInferenceFailed cause", err)
			}
		})
	}
}

func TestASRCodecRejectsNonMonotonicAndOutOfDurationSegments(t *testing.T) {
	t.Parallel()

	codec := codecs.NewASRCodec()
	tests := []struct {
		name        string
		response    codecs.ASRResponse
		withAudio   bool
		wantMessage string
		wantPrefix  bool
	}{
		{
			name: "nonmonotonic timestamps",
			response: codecs.ASRResponse{
				Text: "hello",
				Segments: []codecs.ASRSegment{
					{ID: 0, Start: 0, End: 10, Text: "hello"},
					{ID: 1, Start: 5, End: 9, Text: "again"},
				},
			},
			wantMessage: "ASR backend response segments are out of order",
		},
		{
			name: "segment exceeds PCM duration",
			response: codecs.ASRResponse{
				Text:     "hello",
				Segments: []codecs.ASRSegment{{ID: 0, Start: 0, End: 1, Text: "hello"}},
			},
			withAudio:   true,
			wantMessage: "ASR backend response segment exceeds the audio duration",
			wantPrefix:  true,
		},
		{
			name: "invalid segment text",
			response: codecs.ASRResponse{
				Text:     "hello",
				Segments: []codecs.ASRSegment{{ID: 0, Start: 0, End: 1, Text: "  "}},
			},
			wantMessage: "ASR backend response contains an invalid segment",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				outputs []models.InferenceContent
				err     error
			)
			if test.withAudio {
				outputs, err = codec.DecodeResponseValueWithinAudio(test.response, durationTestWAV())
			} else {
				outputs, err = codec.DecodeResponseValue(test.response)
			}
			if err == nil || len(outputs) != 0 {
				t.Fatalf("response outputs = %#v, error = %v; want atomic malformed failure", outputs, err)
			}
			var failure *models.InvocationFailure
			if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassMalformedResponse {
				t.Fatalf("error = %v, failure = %#v, want malformed response", err, failure)
			}
			if failure.Operation != models.OperationASR || failure.Slot != "segments" {
				t.Fatalf("failure = %#v, want ASR segments failure", failure)
			}
			if test.wantPrefix {
				if !strings.HasPrefix(failure.Message, test.wantMessage) {
					t.Fatalf("failure message = %q, want prefix %q", failure.Message, test.wantMessage)
				}
			} else if failure.Message != test.wantMessage {
				t.Fatalf("failure = %#v, want ASR segments message %q", failure, test.wantMessage)
			}
		})
	}
}

func TestASRCodecDurationRejectionReportsTimingCoordinates(t *testing.T) {
	t.Parallel()

	codec := codecs.NewASRCodec()
	response := codecs.ASRResponse{
		Text:     "hello",
		Segments: []codecs.ASRSegment{{ID: 0, Start: 0, End: 1, Text: "hello"}},
	}
	_, err := codec.DecodeResponseValueWithinAudio(response, durationTestWAV())
	if err == nil {
		t.Fatal("DecodeResponseValueWithinAudio() error = nil, want duration rejection")
	}
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassMalformedResponse {
		t.Fatalf("error = %v, failure = %#v, want malformed response", err, failure)
	}
	const prefix = "ASR backend response segment exceeds the audio duration"
	if !strings.HasPrefix(failure.Message, prefix) {
		t.Fatalf("failure message = %q, want prefix %q", failure.Message, prefix)
	}
	for _, coordinate := range []string{"segment_index=0", "segment_end_ms=1", "audio_duration_ms=0.042", "overrun_ms=0.958"} {
		if !strings.Contains(failure.Message, coordinate) {
			t.Fatalf("failure message = %q, want coordinate %q", failure.Message, coordinate)
		}
	}
	if strings.Contains(failure.Message, "hello") {
		t.Fatalf("failure message = %q, must not leak transcript text", failure.Message)
	}
}

func durationTestWAV() []byte {
	audio := make([]byte, 46)
	copy(audio[0:4], "RIFF")
	binary.LittleEndian.PutUint32(audio[4:8], uint32(len(audio)-8))
	copy(audio[8:12], "WAVE")
	copy(audio[12:16], "fmt ")
	binary.LittleEndian.PutUint32(audio[16:20], 16)
	binary.LittleEndian.PutUint16(audio[20:22], 1)
	binary.LittleEndian.PutUint16(audio[22:24], 1)
	binary.LittleEndian.PutUint32(audio[24:28], 24000)
	binary.LittleEndian.PutUint32(audio[28:32], 48000)
	binary.LittleEndian.PutUint16(audio[32:34], 2)
	binary.LittleEndian.PutUint16(audio[34:36], 16)
	copy(audio[36:40], "data")
	binary.LittleEndian.PutUint32(audio[40:44], 2)
	audio[44], audio[45] = 0x01, 0x02
	return audio
}

func TestASRCodecAcceptsLargeTranscriptAndSegmentOutputs(t *testing.T) {
	text := strings.Repeat("spoken words ", (16<<20)/len("spoken words ")+1)
	response := codecs.ASRResponse{Text: text, Segments: []codecs.ASRSegment{{ID: 0, Start: 0, End: 1, Text: text}}}
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	outputs, err := codecs.NewASRCodec().DecodeResponse(payload)
	if err != nil || len(outputs) != 2 {
		t.Fatalf("large ASR response output count=%d error=%v", len(outputs), err)
	}
	if outputs[0].Name != "transcript" || outputs[0].Content != text {
		t.Fatal("large transcript was lost or truncated")
	}
	var segments []codecs.ASRSegment
	if err := json.Unmarshal([]byte(outputs[1].Content), &segments); err != nil || len(segments) != 1 || segments[0] != response.Segments[0] {
		t.Fatalf("large segment output was lost or truncated: %v", err)
	}
}
func TestASRCodecAcceptsMoreThanMillionSegments(t *testing.T) {
	const count = 1<<20 + 1
	response := codecs.ASRResponse{Text: "long recording", Segments: make([]codecs.ASRSegment, count)}
	for index := range response.Segments {
		response.Segments[index] = codecs.ASRSegment{ID: int32(index), Start: int64(index), End: int64(index + 1), Text: "word"}
	}
	outputs, err := codecs.NewASRCodec().DecodeResponseValue(response)
	if err != nil || len(outputs) != 2 {
		t.Fatalf("long ASR segmentation output count=%d error=%v", len(outputs), err)
	}
	if strings.Count(outputs[1].Content, `"id":`) != count {
		t.Fatal("long segment output lost timestamped entries")
	}
	last, err := json.Marshal(response.Segments[count-1])
	if err != nil || !strings.HasSuffix(outputs[1].Content, string(last)+"]") {
		t.Fatal("long segment output lost its final timestamped entry")
	}
}
