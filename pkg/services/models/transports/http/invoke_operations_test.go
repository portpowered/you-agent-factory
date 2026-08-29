package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

type invokeInvokerFake struct {
	invoke func(context.Context, string, models.Request) (models.Result, error)
}

func (fake invokeInvokerFake) InvokeModel(
	ctx context.Context,
	name string,
	request models.Request,
) (models.Result, error) {
	return fake.invoke(ctx, name, request)
}

func TestAdapter_InvokeModelDecodesBodyAndInvokesInvokerWithMappedRequest(t *testing.T) {
	t.Parallel()

	var invokedName string
	var invokedRequest models.Request
	invoker := invokeInvokerFake{
		invoke: func(_ context.Context, name string, request models.Request) (models.Result, error) {
			invokedName = name
			invokedRequest = request
			return models.Result{
				ModelName: name,
				Worker:    "voice-local",
				Operation: request.Operation,
			}, nil
		},
	}
	handler := NewHandlerFromRoot(RootBinding{
		Models:  &rootFake{},
		Invoker: invoker,
	}, zap.NewNop())
	recorder := httptest.NewRecorder()
	body := `{"operation":"TTS","content":[{"type":"TEXT","text":"hello"}]}`

	handler.InvokeModel(
		recorder,
		httptest.NewRequest(http.MethodPost, "/models/voice/invocations", strings.NewReader(body)),
		"voice",
	)

	if invokedName != "voice" {
		t.Fatalf("InvokeModel invoked with name = %q, want voice", invokedName)
	}
	if invokedRequest.Operation != "TTS" || len(invokedRequest.Content) != 1 {
		t.Fatalf("InvokeModel request = %#v, want decoded TTS invocation", invokedRequest)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body = %s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ModelInvocationResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ModelName != "voice" || response.Worker != "voice-local" || response.Operation != "TTS" {
		t.Fatalf("response = %#v, want encoded invoke success", response)
	}
}

func TestAdapter_InvokeModelRejectsInvalidPayloadBeforeInvoker(t *testing.T) {
	t.Parallel()

	invoker := invokeInvokerFake{
		invoke: func(context.Context, string, models.Request) (models.Result, error) {
			t.Fatal("InvokeModel must not be called for invalid request")
			return models.Result{}, nil
		},
	}
	handler := NewHandlerFromRoot(RootBinding{
		Models:  &rootFake{},
		Invoker: invoker,
	}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.InvokeModel(
		recorder,
		httptest.NewRequest(http.MethodPost, "/models/voice/invocations", strings.NewReader(`{"operation":"TTS","content":{}}`)),
		"voice",
	)

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "content must be an array") {
		t.Fatalf("response = %d %s, want content validation error", recorder.Code, recorder.Body.String())
	}
}

func TestAdapter_InvokeModelRejectsMissingOperationBeforeInvoker(t *testing.T) {
	t.Parallel()

	invoker := invokeInvokerFake{
		invoke: func(context.Context, string, models.Request) (models.Result, error) {
			t.Fatal("InvokeModel must not be called for missing operation")
			return models.Result{}, nil
		},
	}
	handler := NewHandlerFromRoot(RootBinding{
		Models:  &rootFake{},
		Invoker: invoker,
	}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.InvokeModel(
		recorder,
		httptest.NewRequest(http.MethodPost, "/models/voice/invocations", strings.NewReader(`{"content":[{"type":"TEXT","text":"hello"}]}`)),
		"voice",
	)

	assertCatalogHTTPError(t, recorder, http.StatusBadRequest, "BAD_REQUEST", "operation is required")
}

func TestAdapter_InvokeModelServesStreamFileResponse(t *testing.T) {
	t.Parallel()

	streamFile := filepath.Join(t.TempDir(), "speech.wav")
	if err := os.WriteFile(streamFile, []byte("RIFF"), 0o600); err != nil {
		t.Fatalf("write stream file: %v", err)
	}
	invoker := invokeInvokerFake{
		invoke: func(_ context.Context, name string, request models.Request) (models.Result, error) {
			return models.Result{
				ModelName:         name,
				Operation:         request.Operation,
				StreamFile:        streamFile,
				StreamContentType: "audio/wav",
			}, nil
		},
	}
	handler := NewHandlerFromRoot(RootBinding{
		Models:  &rootFake{},
		Invoker: invoker,
	}, zap.NewNop())
	recorder := httptest.NewRecorder()
	body := `{"operation":"TTS","content":[{"type":"TEXT","text":"hello"}]}`

	handler.InvokeModel(
		recorder,
		httptest.NewRequest(http.MethodPost, "/models/voice/invocations", strings.NewReader(body)),
		"voice",
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "audio/wav" {
		t.Fatalf("content-type = %q, want audio/wav", got)
	}
	if body := recorder.Body.String(); body != "RIFF" {
		t.Fatalf("body = %q, want streamed file contents", body)
	}
}

var _ workers.ModelInvoker = invokeInvokerFake{}

func TestGenericInvocationMappingPreservesRepeatedOMNIInputsAndJSONOrder(t *testing.T) {
	t.Parallel()

	generated := genericOMNIRequestForTest()

	mapped, err := GenericInvocationRequestFromGenerated(generated)
	if err != nil {
		t.Fatalf("GenericInvocationRequestFromGenerated() error = %v", err)
	}
	assertGenericInvocationMapping(t, mapped)
	assertGenericInvocationRequestJSON(t, generated)
	assertGenericInvocationMappingDecodesBinaryInput(t)
}

func TestGenericInvocationMappingPreservesEmbedInputsAndOmittedOperation(t *testing.T) {
	t.Parallel()

	textValue := "Find similar work"
	parametersValue := `{"normalize":true,"dimensions":4}`
	inputs := []factoryapi.ModelInvocationInput{
		{Name: "text", Modality: factoryapi.ModelInvocationContentTypeText, Content: &textValue},
		{Name: "parameters", Modality: factoryapi.ModelInvocationContentTypeJSON, Content: &parametersValue},
	}
	outputMode := factoryapi.ModelInvocationOutputModeJSON
	mapped, err := GenericInvocationRequestFromGenerated(factoryapi.GenericModelInvocationRequest{
		Scope:      "scope-http-embed",
		Holder:     "http-embed",
		Model:      factoryapi.ModelReference{NameOrUri: "embed"},
		Inputs:     &inputs,
		OutputMode: &outputMode,
	})
	if err != nil {
		t.Fatalf("GenericInvocationRequestFromGenerated() error = %v", err)
	}
	if mapped.Operation != "" || mapped.Model.NameOrURI != "embed" || mapped.OutputMode != models.OutputModeJSON {
		t.Fatalf("mapped EMBED identity = %#v, want inferred operation and JSON output mode", mapped)
	}
	if len(mapped.Inputs) != 2 || mapped.Inputs[0].Name != "text" || mapped.Inputs[0].Content != textValue ||
		mapped.Inputs[1].Name != "parameters" || mapped.Inputs[1].Modality != models.ModalityJSON ||
		mapped.Inputs[1].Content != parametersValue {
		t.Fatalf("mapped EMBED inputs = %#v, want ordered text then parameters", mapped.Inputs)
	}
}

func TestHandler_InvokeGenericModelEmbedReturnsCanonicalNamedJSONOutput(t *testing.T) {
	t.Parallel()

	var captured models.InvokeModelRequest
	root := &rootFake{
		invokeGeneric: func(_ context.Context, request models.InvokeModelRequest) (models.InvokeModelResult, error) {
			captured = request
			return models.InvokeModelResult{Outputs: []models.InferenceOutput{
				{Name: "embedding", Modality: models.ModalityJSON, ContentType: "application/json", MediaType: "application/json", Content: `[0.1,0.2,0.3,0.4]`},
			}}, nil
		},
	}
	binding := testRootBinding(root)
	handler := NewHandlerFromRoot(binding, zap.NewNop())
	recorder := httptest.NewRecorder()
	body := `{"scope":"caller-supplied","holder":"operator","model":{"nameOrUri":"embed"},"inputs":[{"name":"text","modality":"TEXT","content":"Find similar work"},{"name":"parameters","modality":"JSON","content":"{\"normalize\":true}"}],"outputMode":"JSON"}`

	handler.InvokeGenericModel(
		recorder,
		httptest.NewRequest(http.MethodPost, "/models/invocations", strings.NewReader(body)),
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s, want 200", recorder.Code, recorder.Body.String())
	}
	assertCapturedEmbedRequest(t, captured, binding.Scope)
	assertCanonicalEmbedHTTPResponse(t, recorder)
}

func assertCapturedEmbedRequest(t *testing.T, request models.InvokeModelRequest, scope models.RuntimeScopeRef) {
	t.Helper()
	if request.Scope != scope || request.Model.NameOrURI != "embed" || request.Operation != "" ||
		request.OutputMode != models.OutputModeJSON || len(request.Inputs) != 2 {
		t.Fatalf("root EMBED request = %#v, want live scope, inferred operation, and ordered named inputs", request)
	}
	if request.Inputs[0].Name != "text" || request.Inputs[1].Name != "parameters" {
		t.Fatalf("root EMBED inputs = %#v, want text then parameters", request.Inputs)
	}
}

func assertCanonicalEmbedHTTPResponse(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	var response factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode EMBED response: %v", err)
	}
	if len(response.Outputs) != 1 {
		t.Fatalf("EMBED outputs = %#v, want exactly one output", response.Outputs)
	}
	output := response.Outputs[0]
	if output.Name != "embedding" || output.Modality != factoryapi.ModelInvocationContentTypeJSON ||
		output.ContentType == nil || *output.ContentType != "application/json" || output.MediaType == nil ||
		*output.MediaType != "application/json" || output.Content == nil || *output.Content != `[0.1,0.2,0.3,0.4]` {
		t.Fatalf("EMBED output = %#v, want canonical named JSON vector", output)
	}
	var vector []float64
	if err := json.Unmarshal([]byte(*output.Content), &vector); err != nil || len(vector) != 4 {
		t.Fatalf("EMBED output content = %q, want valid four-dimensional JSON array: %v", *output.Content, err)
	}
}

func TestHandler_InvokeGenericModelMapsEmbedContractFailureWithoutPartialOutput(t *testing.T) {
	t.Parallel()

	root := &rootFake{
		invokeGeneric: func(context.Context, models.InvokeModelRequest) (models.InvokeModelResult, error) {
			return models.InvokeModelResult{}, &models.InvocationFailure{
				Class:     models.InvocationFailureClassInvalidSlot,
				Message:   `unknown input slot "unknown"; valid: parameters, text`,
				Model:     models.ModelReference{NameOrURI: "embed"},
				Operation: models.OperationEMBED,
				Slot:      "unknown",
			}
		},
	}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
	recorder := httptest.NewRecorder()
	body := `{"scope":"factory-session:http-test","holder":"operator","model":{"nameOrUri":"embed"},"operation":"EMBED","inputs":[{"name":"unknown","modality":"TEXT","content":"Find similar work"}]}`

	handler.InvokeGenericModel(
		recorder,
		httptest.NewRequest(http.MethodPost, "/models/invocations", strings.NewReader(body)),
	)

	assertCatalogHTTPError(t, recorder, http.StatusBadRequest, "BAD_REQUEST", `unknown input slot "unknown"; valid: parameters, text`)
	if strings.Contains(recorder.Body.String(), "https://") || strings.Contains(recorder.Body.String(), "cache=") {
		t.Fatalf("EMBED failure leaked implementation detail: %s", recorder.Body.String())
	}
}

func TestGeneratedModelInvocationOperationRoundTripPreservesVideoSlotMetadata(t *testing.T) {
	t.Parallel()

	video := factoryapi.ModelInvocationContentTypeVideo
	required := false
	repeatable := true
	mediaTypes := []string{"video/*"}
	inputs := []factoryapi.ModelInvocationSlot{{
		Name:         "video",
		ContentTypes: []factoryapi.ModelInvocationContentType{video},
		Modality:     &video,
		Required:     &required,
		Repeatable:   &repeatable,
		MediaTypes:   &mediaTypes,
	}}
	want := factoryapi.ModelInvocationOperation{Name: models.OperationOMNI, Inputs: &inputs}

	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal generic operation: %v", err)
	}
	var got factoryapi.ModelInvocationOperation
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("unmarshal generic operation: %v", err)
	}
	assertVideoSlotRoundTrip(t, got)
}

func assertVideoSlotRoundTrip(t *testing.T, operation factoryapi.ModelInvocationOperation) {
	t.Helper()
	if operation.Inputs == nil || len(*operation.Inputs) != 1 {
		t.Fatalf("generic operation inputs = %#v, want one video slot", operation.Inputs)
	}
	slot := (*operation.Inputs)[0]
	if slot.Name != "video" || len(slot.ContentTypes) != 1 || slot.ContentTypes[0] != factoryapi.ModelInvocationContentTypeVideo {
		t.Fatalf("generic video slot identity = %#v", slot)
	}
	if slot.Modality == nil || *slot.Modality != factoryapi.ModelInvocationContentTypeVideo {
		t.Fatalf("generic video slot modality = %#v", slot.Modality)
	}
	if slot.Required == nil || *slot.Required {
		t.Fatalf("generic video slot required = %#v, want false", slot.Required)
	}
	if slot.Repeatable == nil || !*slot.Repeatable {
		t.Fatalf("generic video slot repeatable = %#v, want true", slot.Repeatable)
	}
	if slot.MediaTypes == nil || len(*slot.MediaTypes) != 1 || (*slot.MediaTypes)[0] != "video/*" {
		t.Fatalf("generic video slot media types = %#v", slot.MediaTypes)
	}
}

func genericOMNIRequestForTest() factoryapi.GenericModelInvocationRequest {
	outputMode := factoryapi.ModelInvocationOutputModeJSON
	offline := true
	operation := factoryapi.ModelOperationName(models.OperationOMNI)
	inputs := []factoryapi.ModelInvocationInput{
		{Name: "prompt", Modality: factoryapi.ModelInvocationContentTypeText, Content: stringPointer("compare")},
		{Name: "image", Modality: factoryapi.ModelInvocationContentTypeImage, MediaType: stringPointer("image/png"), Content: stringPointer("first")},
		{Name: "image", Modality: factoryapi.ModelInvocationContentTypeImage, MediaType: stringPointer("image/jpeg"), Content: stringPointer("second")},
	}
	parameters := []factoryapi.ModelInvocationParameter{{Name: "temperature", Value: map[string]any{"value": 0.2}}}
	return factoryapi.GenericModelInvocationRequest{
		Scope:      "scope-http-001",
		Holder:     "http",
		Model:      factoryapi.ModelReference{NameOrUri: "llm"},
		Operation:  &operation,
		Inputs:     &inputs,
		Parameters: &parameters,
		OutputMode: &outputMode,
		Offline:    &offline,
	}
}

func assertGenericInvocationMapping(t *testing.T, mapped models.GenericInvocationRequest) {
	t.Helper()
	if err := mapped.Validate(); err != nil {
		t.Fatalf("mapped request Validate() error = %v", err)
	}
	if mapped.Model.NameOrURI != "llm" {
		t.Fatalf("mapped model = %#v", mapped.Model)
	}
	if len(mapped.Inputs) != 3 || mapped.Inputs[1].Name != "image" || mapped.Inputs[2].Content != "second" {
		t.Fatalf("mapped inputs = %#v", mapped.Inputs)
	}
	if mapped.OutputMode != models.OutputModeJSON {
		t.Fatalf("mapped output mode = %q", mapped.OutputMode)
	}
	if !mapped.Offline || mapped.Parameters[0].Name != "temperature" {
		t.Fatalf("mapped controls = %#v", mapped)
	}
}

func assertGenericInvocationRequestJSON(t *testing.T, generated factoryapi.GenericModelInvocationRequest) {
	t.Helper()
	encoded, err := json.Marshal(generated)
	if err != nil {
		t.Fatalf("marshal generated request: %v", err)
	}
	var decoded factoryapi.GenericModelInvocationRequest
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal generated request: %v", err)
	}
	if decoded.Inputs == nil || len(*decoded.Inputs) != 3 || (*decoded.Inputs)[1].Name != "image" || (*decoded.Inputs)[2].MediaType == nil || *(*decoded.Inputs)[2].MediaType != "image/jpeg" {
		t.Fatalf("serialized inputs = %#v", decoded.Inputs)
	}
}

func TestGenericInvocationResponseMappingPreservesASROutputsAndFailureIdentity(t *testing.T) {
	t.Parallel()

	artifact, err := (models.InferenceArtifactRef{}).Parse("artifact:segments")
	if err != nil {
		t.Fatalf("parse artifact: %v", err)
	}
	result := models.GenericInvocationResult{Outputs: []models.InferenceOutput{
		{Name: "transcript", Modality: models.ModalityText, ContentType: "text/plain", MediaType: "text/plain", Content: "hello"},
		{Name: "segments", Modality: models.ModalityJSON, ContentType: "application/json", MediaType: "application/json", Artifact: &models.InferenceArtifact{
			Artifact: artifact, Name: "segments.json", MediaType: "application/json", SizeBytes: 12,
		}},
	}}
	projected := GenericInvocationResponseToGenerated(result)
	assertASRResponseMapping(t, projected)
	assertASRResponseJSON(t, projected)

	failure := &models.InvocationFailure{
		Class:     models.InvocationFailureClassBackendProtocol,
		Message:   "backend protocol is incompatible",
		Model:     models.ModelReference{NameOrURI: "asr"},
		Operation: models.OperationASR,
		Slot:      "segments",
	}
	projectedFailure := GenericInvocationFailureToGenerated(failure)
	assertASRFailureMapping(t, projectedFailure, failure)
}

func assertASRResponseMapping(t *testing.T, projected factoryapi.GenericModelInvocationResponse) {
	t.Helper()
	if len(projected.Outputs) != 2 || projected.Outputs[0].Name != "transcript" || projected.Outputs[1].Name != "segments" {
		t.Fatalf("projected outputs = %#v", projected.Outputs)
	}
	if projected.Outputs[1].Artifact == nil || projected.Outputs[1].Artifact.ArtifactRef != "artifact:segments" {
		t.Fatalf("projected artifact = %#v", projected.Outputs[1].Artifact)
	}
}

func assertASRResponseJSON(t *testing.T, projected factoryapi.GenericModelInvocationResponse) {
	t.Helper()
	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatalf("marshal generated response: %v", err)
	}
	var decoded factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal generated response: %v", err)
	}
	if len(decoded.Outputs) != 2 || decoded.Outputs[0].Name != "transcript" {
		t.Fatalf("serialized outputs = %#v", decoded.Outputs)
	}
	if decoded.Outputs[1].Artifact == nil || decoded.Outputs[1].Artifact.ArtifactRef != "artifact:segments" {
		t.Fatalf("serialized artifact = %#v", decoded.Outputs[1].Artifact)
	}
}

func assertASRFailureMapping(t *testing.T, projected *factoryapi.ModelInvocationFailure, failure *models.InvocationFailure) {
	t.Helper()
	if projected == nil || projected.Class != factoryapi.ModelInvocationFailureClassBackendProtocol {
		t.Fatalf("projected failure = %#v", projected)
	}
	if projected.Model == nil || projected.Model.NameOrUri != "asr" {
		t.Fatalf("projected failure model = %#v", projected.Model)
	}
	if projected.Slot == nil || *projected.Slot != "segments" {
		t.Fatalf("projected failure slot = %#v", projected.Slot)
	}
	if projected.Message != failure.Message {
		t.Fatalf("projected failure message = %q, want %q", projected.Message, failure.Message)
	}
}

func TestGenericInvocationRequestMappingRejectsInvalidArtifactAsTypedFailure(t *testing.T) {
	t.Parallel()

	inputs := []factoryapi.ModelInvocationInput{{
		Name:        "image",
		Modality:    factoryapi.ModelInvocationContentTypeImage,
		ArtifactRef: stringPointer(" "),
	}}
	operation := factoryapi.ModelOperationName(models.OperationOMNI)
	_, err := GenericInvocationRequestFromGenerated(factoryapi.GenericModelInvocationRequest{
		Scope:     "scope-http-002",
		Holder:    "http",
		Model:     factoryapi.ModelReference{NameOrUri: "llm"},
		Operation: &operation,
		Inputs:    &inputs,
	})
	var failure *models.InvocationFailure
	if err == nil || !asInvocationFailure(err, &failure) || failure.Class != models.InvocationFailureClassArtifact {
		t.Fatalf("error = %v, failure = %#v, want typed artifact failure", err, failure)
	}
	assertGenericInvocationMappingRejectsMultipleInputCarriers(t)
}

func assertGenericInvocationMappingDecodesBinaryInput(t *testing.T) {
	t.Helper()
	want := []byte{0x00, 0xff, 0x89, 0x50, 0x4e, 0x47}
	inputs := []factoryapi.ModelInvocationInput{{
		Name:          "image",
		Modality:      factoryapi.ModelInvocationContentTypeImage,
		MediaType:     stringPointer("image/png"),
		ContentBase64: &want,
	}}
	operation := factoryapi.ModelOperationName(models.OperationOMNI)
	mapped, err := GenericInvocationRequestFromGenerated(factoryapi.GenericModelInvocationRequest{
		Scope:     "scope-http-binary",
		Holder:    "http",
		Model:     factoryapi.ModelReference{NameOrUri: "llm"},
		Operation: &operation,
		Inputs:    &inputs,
	})
	if err != nil {
		t.Fatalf("GenericInvocationRequestFromGenerated() error = %v", err)
	}
	if len(mapped.Inputs) != 1 || !bytes.Equal([]byte(mapped.Inputs[0].Content), want) {
		t.Fatalf("mapped binary input = %#v, want exact bytes", mapped.Inputs)
	}
	if mapped.Inputs[0].ContentType != "" || mapped.Inputs[0].MediaType != "image/png" {
		t.Fatalf("mapped binary metadata = %#v, want media type only", mapped.Inputs[0])
	}

	encoded, err := json.Marshal(factoryapi.GenericModelInvocationRequest{
		Scope: "scope-http-binary", Holder: "http", Model: factoryapi.ModelReference{NameOrUri: "llm"}, Inputs: &inputs,
	})
	if err != nil {
		t.Fatalf("marshal binary request = %v", err)
	}
	var decoded factoryapi.GenericModelInvocationRequest
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal binary request = %v", err)
	}
	if decoded.Inputs == nil || len(*decoded.Inputs) != 1 || !bytes.Equal(*(*decoded.Inputs)[0].ContentBase64, want) {
		t.Fatalf("round-tripped binary input = %#v, want exact bytes", decoded.Inputs)
	}
}

func assertGenericInvocationMappingRejectsMultipleInputCarriers(t *testing.T) {
	t.Helper()
	content := "inline"
	binary := []byte{0x01}
	inputs := []factoryapi.ModelInvocationInput{{
		Name: "image", Modality: factoryapi.ModelInvocationContentTypeImage,
		Content: &content, ContentBase64: &binary,
	}}
	_, err := GenericInvocationRequestFromGenerated(factoryapi.GenericModelInvocationRequest{
		Scope: "scope-http-carrier", Holder: "http", Model: factoryapi.ModelReference{NameOrUri: "llm"}, Inputs: &inputs,
	})
	var failure *models.InvocationFailure
	if err == nil || !asInvocationFailure(err, &failure) || failure.Class != models.InvocationFailureClassInvalidParameter ||
		!strings.Contains(failure.Message, "only one of content, contentBase64, or artifactRef") {
		t.Fatalf("multiple input carriers error = %v, failure = %#v, want typed validation failure", err, failure)
	}
}

func TestHandler_InvokeGenericModelUsesModelsRootAndPreservesNamedOutputs(t *testing.T) {
	t.Parallel()

	var captured models.InvokeModelRequest
	root := &rootFake{
		invokeGeneric: func(_ context.Context, request models.InvokeModelRequest) (models.InvokeModelResult, error) {
			captured = request
			return models.InvokeModelResult{Outputs: []models.InferenceOutput{
				{Name: "transcript", Modality: models.ModalityText, ContentType: "TEXT", Content: "hello"},
				{Name: "segments", Modality: models.ModalityJSON, ContentType: "JSON", Content: `[{"start":0}]`},
			}}, nil
		},
	}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
	recorder := httptest.NewRecorder()
	body := `{"scope":"factory-session:http-test","holder":"operator","model":{"nameOrUri":"asr"},"operation":"ASR","inputs":[{"name":"audio","modality":"AUDIO","content":"bytes"}]}`

	handler.InvokeGenericModel(
		recorder,
		httptest.NewRequest(http.MethodPost, "/models/invocations", strings.NewReader(body)),
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s, want 200", recorder.Code, recorder.Body.String())
	}
	if captured.Model.NameOrURI != "asr" || captured.Operation != "ASR" || captured.Holder != "operator" || len(captured.Inputs) != 1 {
		t.Fatalf("root request = %#v, want mapped generic request", captured)
	}
	var response factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode generic response: %v", err)
	}
	if len(response.Outputs) != 2 || response.Outputs[0].Name != "transcript" || response.Outputs[1].Name != "segments" {
		t.Fatalf("outputs = %#v, want ordered named outputs", response.Outputs)
	}
}

func TestHandler_InvokeGenericModelRejectsInvalidRequestBeforeRoot(t *testing.T) {
	t.Parallel()

	root := &rootFake{
		invokeGeneric: func(context.Context, models.InvokeModelRequest) (models.InvokeModelResult, error) {
			t.Fatal("generic root must not run for an invalid request")
			return models.InvokeModelResult{}, nil
		},
	}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.InvokeGenericModel(
		recorder,
		httptest.NewRequest(http.MethodPost, "/models/invocations", strings.NewReader(`{"scope":"factory-session:http-test","holder":"operator","model":{"nameOrUri":"asr"},"operation":"ASR","unexpected":true}`)),
	)

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "invalid request payload") {
		t.Fatalf("response = %d %s, want strict generic payload error", recorder.Code, recorder.Body.String())
	}
}

func TestHandler_InvokeGenericModelMapsTypedRootFailure(t *testing.T) {
	t.Parallel()

	root := &rootFake{
		invokeGeneric: func(context.Context, models.InvokeModelRequest) (models.InvokeModelResult, error) {
			return models.InvokeModelResult{}, &models.InvocationFailure{
				Class:   models.InvocationFailureClassBackendReadiness,
				Message: "model backend is not ready",
			}
		},
	}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
	recorder := httptest.NewRecorder()
	handler.InvokeGenericModel(
		recorder,
		httptest.NewRequest(http.MethodPost, "/models/invocations", strings.NewReader(`{"scope":"factory-session:http-test","holder":"operator","model":{"nameOrUri":"asr"},"operation":"ASR"}`)),
	)

	assertCatalogHTTPError(t, recorder, http.StatusServiceUnavailable, "MODEL_BACKEND_NOT_READY", "model backend is not ready")
}

func TestGeneratedClientGenericContractsPreserveOrderedValuesAndModelConfig(t *testing.T) {
	t.Parallel()
	assertGeneratedClientRequestRoundTrip(t)
	assertGeneratedClientModelConfigRoundTrip(t)
}

func assertGeneratedClientRequestRoundTrip(t *testing.T) {
	t.Helper()
	outputMode := generatedclient.ModelInvocationOutputModeJSON
	offline := true
	operation := generatedclient.ModelOperationName(models.OperationOMNI)
	inputs := []generatedclient.ModelInvocationInput{
		{Name: "prompt", Modality: generatedclient.ModelInvocationContentTypeText, Content: stringPointer("compare")},
		{Name: "image", Modality: generatedclient.ModelInvocationContentTypeImage, MediaType: stringPointer("image/png"), Content: stringPointer("first")},
		{Name: "image", Modality: generatedclient.ModelInvocationContentTypeImage, MediaType: stringPointer("image/jpeg"), Content: stringPointer("second")},
	}
	request := generatedclient.GenericModelInvocationRequest{
		Scope:      "scope-client-001",
		Holder:     "generated-client",
		Model:      generatedclient.ModelReference{NameOrUri: "llm"},
		Operation:  &operation,
		Inputs:     &inputs,
		OutputMode: &outputMode,
		Offline:    &offline,
	}

	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal generated client request: %v", err)
	}
	var decoded generatedclient.GenericModelInvocationRequest
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal generated client request: %v", err)
	}
	if decoded.Inputs == nil || len(*decoded.Inputs) != 3 || (*decoded.Inputs)[1].Name != "image" || (*decoded.Inputs)[2].MediaType == nil || *(*decoded.Inputs)[2].MediaType != "image/jpeg" {
		t.Fatalf("generated client inputs = %#v", decoded.Inputs)
	}
	if decoded.OutputMode == nil || *decoded.OutputMode != generatedclient.ModelInvocationOutputModeJSON || decoded.Offline == nil || !*decoded.Offline {
		t.Fatalf("generated client controls = %#v", decoded)
	}
}

func assertGeneratedClientModelConfigRoundTrip(t *testing.T) {
	t.Helper()
	operations := []generatedclient.GlobalConfigModelOperation{"OMNI"}
	backend := "localai-llamacpp"
	models := generatedclient.GlobalConfigModels{
		"llm":    {Backend: &backend, Operations: &operations},
		"custom": {Source: stringPointer("hf://example/custom.gguf"), Backend: &backend, Operations: &operations},
	}
	config := generatedclient.GlobalConfig{Models: &models}
	configJSON, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal generated client model config: %v", err)
	}
	var decodedConfig generatedclient.GlobalConfig
	if err := json.Unmarshal(configJSON, &decodedConfig); err != nil {
		t.Fatalf("unmarshal generated client model config: %v", err)
	}
	if decodedConfig.Models == nil || (*decodedConfig.Models)["llm"].Backend == nil || *(*decodedConfig.Models)["llm"].Backend != backend || len(*(*decodedConfig.Models)["custom"].Operations) != 1 {
		t.Fatalf("generated client model config = %#v", decodedConfig.Models)
	}
}

func asInvocationFailure(err error, target **models.InvocationFailure) bool {
	if typed, ok := err.(*models.InvocationFailure); ok {
		*target = typed
		return true
	}
	return false
}

func stringPointer(value string) *string {
	return &value
}
