package codecs

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

func TestTTSCodecEncodesTextModelAndConfirmedParameters(t *testing.T) {
	t.Parallel()

	codec := NewTTSCodec()
	request, err := codec.EncodeRequest(models.InvokeModelRequest{
		Model:     models.ModelReference{NameOrURI: "tts"},
		Operation: models.OperationTTS,
		Inputs: []models.InferenceInput{
			{Name: "text", Modality: models.ModalityText, ContentType: "text/plain", Content: "hello"},
			{Name: "voice", Modality: models.ModalityAudio, ContentType: "audio/wav", Content: string(testWAV())},
			{Name: "parameters", Modality: models.ModalityJSON, ContentType: "application/json", Content: `{"language":"en"}`},
		},
		Parameters: []models.OperationParameter{{Name: "instructions", Value: "speak clearly"}},
	})
	if err != nil {
		t.Fatalf("EncodeRequest() error = %v", err)
	}
	if request.Text != "hello" || request.Voice != string(testWAV()) || request.Model != "tts" || request.Parameters["language"] != "en" || request.Parameters["instructions"] != "speak clearly" {
		t.Fatalf("encoded TTS request = %#v, want detached text/model/parameters", request)
	}

	request.Parameters["language"] = "mutated"
	if request.Parameters["language"] != "mutated" {
		t.Fatal("encoded request should be mutable by its owner")
	}
}

func TestTTSCodecAcceptsRefTextParameter(t *testing.T) {
	t.Parallel()

	codec := NewTTSCodec()
	request, err := codec.EncodeRequest(models.InvokeModelRequest{
		Operation: models.OperationTTS,
		Inputs: []models.InferenceInput{
			{Name: "text", Modality: models.ModalityText, ContentType: "text/plain", Content: "hello"},
		},
		Parameters: []models.OperationParameter{{Name: "ref_text", Value: "reference transcript"}},
	})
	if err != nil {
		t.Fatalf("EncodeRequest() error = %v", err)
	}
	if request.Parameters["ref_text"] != "reference transcript" {
		t.Fatalf("encoded TTS request parameters = %#v, want ref_text", request.Parameters)
	}
}

func TestTTSCodecValidatesExplicitBudgetAndLeavesDefaultUnset(t *testing.T) {
	t.Parallel()
	text := models.InferenceInput{Name: "text", Modality: models.ModalityText, Content: "hello"}
	for _, value := range []any{int(1), int32(1), uint64(1), float64(1), json.Number("1.0"), json.Number("1e0"), json.Number("1000e-3"), json.Number("2147483647.0000000"), json.Number("2.147483647e9"), int64(math.MaxInt32)} {
		request, err := (TTSCodec{}).EncodeRequest(models.InvokeModelRequest{
			Operation: models.OperationTTS, Inputs: []models.InferenceInput{text},
			Parameters: []models.OperationParameter{{Name: " MAX_NEW_TOKENS ", Value: value}},
		})
		if err != nil || len(request.Parameters) != 1 {
			t.Fatalf("explicit budget %v = %#v/%v", value, request, err)
		}
		if _, ok := TTSMaxNewTokens(request.Parameters["max_new_tokens"]); !ok {
			t.Fatalf("encoded budget %v is invalid", value)
		}
	}
	for _, value := range []any{0, -1, 1.5, "1", true, nil, []int{1}, math.NaN(), math.Inf(1), int64(math.MaxInt32) + 1, uint64(math.MaxUint64), json.Number("2147483648"), json.Number("1e100"), json.Number("1.0000000000000001"), json.Number("2147483647.0000001"), json.Number("1e999999999"), json.Number("1e-999999999")} {
		_, err := (TTSCodec{}).EncodeRequest(models.InvokeModelRequest{
			Operation: models.OperationTTS, Inputs: []models.InferenceInput{text},
			Parameters: []models.OperationParameter{{Name: "max_new_tokens", Value: value}},
		})
		var failure *models.InvocationFailure
		if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassInvalidParameter || failure.Parameter != "max_new_tokens" {
			t.Fatalf("invalid budget %v = %v, want coded invalid parameter", value, err)
		}
	}
	request, err := (TTSCodec{}).EncodeRequest(models.InvokeModelRequest{Operation: models.OperationTTS, Inputs: []models.InferenceInput{text}})
	if err != nil || request.Parameters != nil {
		t.Fatalf("omitted budget = %#v/%v, want no imposed parameters", request, err)
	}
}

func TestTTSCodecRejectsInvalidInputsWithoutProviderValues(t *testing.T) {
	t.Parallel()

	validText := models.InferenceInput{Name: "text", Modality: models.ModalityText, Content: "hello"}
	cases := []struct {
		name   string
		inputs []models.InferenceInput
		params []models.OperationParameter
		want   models.InvocationFailureClass
		safe   string
	}{
		{name: "missing text", want: models.InvocationFailureClassInvalidSlot},
		{name: "wrong text media", inputs: []models.InferenceInput{{Name: "text", Modality: models.ModalityAudio, Content: "audio"}}, want: models.InvocationFailureClassMediaCapability},
		{name: "repeated text", inputs: []models.InferenceInput{validText, validText}, want: models.InvocationFailureClassSlotArity},
		{name: "unsupported parameter", inputs: []models.InferenceInput{validText}, params: []models.OperationParameter{{Name: "temperature", Value: 0.2}}, want: models.InvocationFailureClassInvalidParameter},
		{name: "voice requires audio content", inputs: []models.InferenceInput{validText, {Name: "voice", Modality: models.ModalityAudio, ContentType: "audio/wav"}}, want: models.InvocationFailureClassMediaCapability},
		{name: "voice rejects text media", inputs: []models.InferenceInput{validText, {Name: "voice", Modality: models.ModalityText, Content: "not-audio"}}, want: models.InvocationFailureClassMediaCapability},
		{name: "voice rejects malformed wav", inputs: []models.InferenceInput{validText, {Name: "voice", Modality: models.ModalityAudio, ContentType: "audio/wav", Content: "not-a-wav"}}, want: models.InvocationFailureClassMediaCapability},
		{name: "voice rejects non-wav media", inputs: []models.InferenceInput{validText, {Name: "voice", Modality: models.ModalityAudio, ContentType: "audio/mpeg", Content: string(testWAV())}}, want: models.InvocationFailureClassMediaCapability},
		{name: "repeated parameter", inputs: []models.InferenceInput{validText, {Name: "parameters", Modality: models.ModalityJSON, Content: `{"language":"en"}`}}, params: []models.OperationParameter{{Name: "language", Value: "fr"}}, want: models.InvocationFailureClassInvalidParameter},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := (TTSCodec{}).EncodeRequest(models.InvokeModelRequest{
				Operation: models.OperationTTS, Inputs: test.inputs, Parameters: test.params,
			})
			var failure *models.InvocationFailure
			if !errors.As(err, &failure) || failure.Class != test.want {
				t.Fatalf("EncodeRequest() error = %v, failure = %#v, want class %q", err, failure, test.want)
			}
			if test.safe != "" && strings.Contains(err.Error(), test.safe) {
				t.Fatalf("EncodeRequest() error = %q, must not expose voice path", err)
			}
		})
	}
}

func TestTTSCodecDecodesOneSemanticAudioOutput(t *testing.T) {
	t.Parallel()

	audio := testWAV()
	content, err := (TTSCodec{}).DecodeResponse(TTSResponse{Audio: audio, MediaType: "audio/wav; charset=binary"})
	if err != nil {
		t.Fatalf("DecodeResponse() error = %v", err)
	}
	if content.Name != "audio" || content.Modality != models.ModalityAudio || content.ContentType != "audio/wav" || content.MediaType != "audio/wav" || !bytes.Equal([]byte(content.Content), audio) {
		t.Fatalf("decoded TTS content = %#v, want one detached audio/wav output", content)
	}
	audio[44] = 0x7f
	if content.Content == string(audio) {
		t.Fatal("decoded content retained caller audio mutation")
	}
}

func TestTTSCodecRejectsMalformedOrOversizedAudioAtomically(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		response  TTSResponse
		wantClass models.InvocationFailureClass
	}{
		{name: "wrong media", response: TTSResponse{Audio: testWAV(), MediaType: "audio/mpeg"}},
		{name: "empty", response: TTSResponse{MediaType: "audio/wav"}},
		{name: "not wav", response: TTSResponse{Audio: []byte("not audio"), MediaType: "audio/wav"}},
		{name: "oversized", response: TTSResponse{Audio: make([]byte, MaxTTSAudioBytes+1), MediaType: "audio/wav"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			content, err := (TTSCodec{}).DecodeResponse(test.response)
			var failure *models.InvocationFailure
			if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassMalformedResponse {
				t.Fatalf("DecodeResponse() = content:%#v error:%v failure:%#v, want malformed typed failure", content, err, failure)
			}
			if content != (models.InferenceContent{}) {
				t.Fatalf("DecodeResponse() content = %#v, want no partial content", content)
			}
		})
	}
}

func testWAV() []byte {
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
	audio[44] = 0x01
	audio[45] = 0x02
	return audio
}
