package inference

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestGenericInvocationInputsMapsSupportedWorkPartsInOrder(t *testing.T) {
	artifactID := "models-inference:artifact:input"
	inputs, parameters, err := genericInvocationInputs([]workers.ResolvedModelOperationBinding{
		{
			Slot:    "text",
			Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}},
		},
		{
			Slot: "json",
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeJSON, JSON: []byte(`{"answer":42}`), ContentType: "application/json",
			}},
		},
		{
			Slot: "image",
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeImage, File: "file://image.png", ContentType: "image/png",
			}},
		},
		{
			Slot: "audio",
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeAudio, Text: "audio-reference", ContentType: "audio/wav", ArtifactID: artifactID,
			}},
		},
		{
			Slot: "binary",
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeBinary, URL: "data:application/octet-stream;base64,Yg==", ContentType: "application/octet-stream",
			}},
		},
		{
			Slot: "empty",
			Content: []work.WorkContentPart{
				{Type: work.WorkContentPartTypeText, Text: "  "},
				{Type: work.WorkContentPartTypeJSON},
				{Type: work.WorkContentPartTypeImage},
			},
		},
		{
			Slot:    "PARAMETERS",
			Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeJSON, JSON: []byte(`{"speed":1,"pitch":0.2}`)}},
		},
	})
	if err != nil {
		t.Fatalf("genericInvocationInputs() error = %v", err)
	}
	if len(inputs) != 5 {
		t.Fatalf("inputs = %#v, want five non-empty inputs", inputs)
	}
	assertGenericInputModalities(t, inputs)
	assertGenericInputMetadata(t, inputs, artifactID)
	assertGenericInputParameters(t, parameters)
}

func assertGenericInputModalities(t *testing.T, inputs []models.InferenceInput) {
	t.Helper()
	wantModalities := []models.Modality{
		models.ModalityText,
		models.ModalityJSON,
		models.ModalityImage,
		models.ModalityAudio,
		models.ModalityBinary,
	}
	for index, want := range wantModalities {
		if inputs[index].Modality != want {
			t.Fatalf("inputs[%d].Modality = %q, want %q", index, inputs[index].Modality, want)
		}
	}
}

func assertGenericInputMetadata(t *testing.T, inputs []models.InferenceInput, artifactID string) {
	t.Helper()
	if inputs[0].ContentType != "text/plain" || inputs[0].MediaType != "text/plain" {
		t.Fatalf("text metadata = %#v, want text/plain for both fields", inputs[0])
	}
	if inputs[1].Content != `{"answer":42}` || inputs[1].ContentType != "application/json" || inputs[1].MediaType != "application/json" {
		t.Fatalf("JSON input = %#v", inputs[1])
	}
	if inputs[2].Content != "file://image.png" || inputs[2].MediaType != "image/png" {
		t.Fatalf("image input = %#v", inputs[2])
	}
	if inputs[3].Artifact == nil || inputs[3].Artifact.String() != artifactID || inputs[3].Content != "audio-reference" {
		t.Fatalf("audio input = %#v", inputs[3])
	}
}

func assertGenericInputParameters(t *testing.T, parameters []models.OperationParameter) {
	t.Helper()
	if len(parameters) != 2 || parameters[0].Name != "speed" || parameters[1].Name != "pitch" {
		t.Fatalf("parameters = %#v, want ordered speed/pitch", parameters)
	}
}

func TestGenericInvocationRequestUsesWorkstationFallbackHolder(t *testing.T) {
	scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:generic-inputs")
	if err != nil {
		t.Fatalf("parse scope: %v", err)
	}
	request := validRequest()
	request.Dispatch.DispatchID = ""
	request.Dispatch.WorkstationName = "fallback-workstation"
	request.ModelOperation = "  " + models.OperationTTS + "  "
	request.ModelBindings = nil
	result, err := genericInvocationRequest(request, scope, models.LocalWorker{Model: "  tts  "})
	if err != nil {
		t.Fatalf("genericInvocationRequest() error = %v", err)
	}
	if result.Holder != "fallback-workstation" || result.Model.NameOrURI != "tts" || result.Operation != models.OperationTTS {
		t.Fatalf("generic invocation request = %#v", result)
	}
	if result.Scope != scope || result.OutputMode != models.OutputModeAuto {
		t.Fatalf("generic invocation request scope/output mode = %#v", result)
	}
}

func TestGenericInvocationInputsRejectsInvalidBindings(t *testing.T) {
	cases := []struct {
		name     string
		binding  workers.ResolvedModelOperationBinding
		wantText string
	}{
		{
			name: "unsupported content type",
			binding: workers.ResolvedModelOperationBinding{
				Slot: "input", Content: []work.WorkContentPart{{Type: work.WorkContentPartType("unsupported"), Text: "value"}},
			},
			wantText: "inference model binding[0] content[0] is invalid",
		},
		{
			name: "invalid parameters content",
			binding: workers.ResolvedModelOperationBinding{
				Slot: "parameters", Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeAudio, URL: "file://not-json"}},
			},
			wantText: "parameters",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := genericInvocationInputs([]workers.ResolvedModelOperationBinding{test.binding})
			var providerErr *workers.ProviderError
			if err == nil || !errors.As(err, &providerErr) || !strings.Contains(providerErr.Message, test.wantText) {
				t.Fatalf("genericInvocationInputs() error = %v provider=%#v, want text %q", err, providerErr, test.wantText)
			}
		})
	}
}

func TestInferenceInputFromWorkPartSupportsMetadataAndArtifactErrors(t *testing.T) {
	cases := []struct {
		name        string
		part        work.WorkContentPart
		wantMod     models.Modality
		wantType    string
		wantMedia   string
		wantContent string
	}{
		{name: "text fallback", part: work.WorkContentPart{Type: work.WorkContentPartTypeText, Text: "text"}, wantMod: models.ModalityText, wantType: "text/plain", wantMedia: "text/plain", wantContent: "text"},
		{name: "text custom", part: work.WorkContentPart{Type: work.WorkContentPartTypeText, Text: "text", ContentType: "plain"}, wantMod: models.ModalityText, wantType: "plain", wantContent: "text"},
		{name: "JSON fallback", part: work.WorkContentPart{Type: work.WorkContentPartTypeJSON, JSON: []byte(`{"ok":true}`)}, wantMod: models.ModalityJSON, wantType: "application/json", wantMedia: "application/json", wantContent: `{"ok":true}`},
		{name: "image text fallback", part: work.WorkContentPart{Type: work.WorkContentPartTypeImage, Text: "image-ref", ContentType: "image/png"}, wantMod: models.ModalityImage, wantType: "image/png", wantMedia: "image/png", wantContent: "image-ref"},
		{name: "audio URL", part: work.WorkContentPart{Type: work.WorkContentPartTypeAudio, URL: "file://audio.wav", ContentType: "audio/wav"}, wantMod: models.ModalityAudio, wantType: "audio/wav", wantMedia: "audio/wav", wantContent: "file://audio.wav"},
		{name: "binary file", part: work.WorkContentPart{Type: work.WorkContentPartTypeBinary, File: "file://binary.bin", ContentType: "application/octet-stream"}, wantMod: models.ModalityBinary, wantType: "application/octet-stream", wantMedia: "application/octet-stream", wantContent: "file://binary.bin"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, err := inferenceInputFromWorkPart("slot", test.part)
			if err != nil {
				t.Fatalf("inferenceInputFromWorkPart() error = %v", err)
			}
			if got.Name != "slot" || got.Modality != test.wantMod || got.ContentType != test.wantType || got.MediaType != test.wantMedia || got.Content != test.wantContent {
				t.Fatalf("inference input = %#v", got)
			}
		})
	}
	if _, err := inferenceInputFromWorkPart("slot", work.WorkContentPart{Type: work.WorkContentPartType("unknown")}); err == nil {
		t.Fatal("unsupported Work content type returned nil error")
	}
	if _, err := inferenceInputFromWorkPart("slot", work.WorkContentPart{Type: work.WorkContentPartTypeText, Text: "value", ArtifactID: ""}); err != nil {
		t.Fatalf("text without artifact returned error: %v", err)
	}
	if _, err := inferenceInputFromWorkPart("slot", work.WorkContentPart{Type: work.WorkContentPartTypeText, Text: "value", ArtifactID: "  "}); err != nil {
		t.Fatalf("blank artifact without reference returned error: %v", err)
	}
	if _, err := inferenceInputFromWorkPart("slot", work.WorkContentPart{Type: work.WorkContentPartTypeText, Text: "value", ArtifactID: ""}); err != nil {
		t.Fatalf("empty artifact returned error: %v", err)
	}
	if _, err := inferenceInputFromWorkPart("slot", work.WorkContentPart{Type: work.WorkContentPartTypeText, Text: "value", ArtifactID: "models-inference:artifact:ok"}); err != nil {
		t.Fatalf("valid artifact returned error: %v", err)
	}
}

func TestOperationParametersDecodeValidation(t *testing.T) {
	cases := []struct {
		name string
		part work.WorkContentPart
		want bool
	}{
		{name: "JSON object", part: work.WorkContentPart{Type: work.WorkContentPartTypeJSON, JSON: []byte(`{"count":2}`)}, want: true},
		{name: "text object", part: work.WorkContentPart{Type: work.WorkContentPartTypeText, Text: `{"voice":"calm"}`}, want: true},
		{name: "empty", part: work.WorkContentPart{Type: work.WorkContentPartTypeJSON}, want: false},
		{name: "invalid JSON", part: work.WorkContentPart{Type: work.WorkContentPartTypeText, Text: "not-json"}, want: false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			parameters, err := operationParametersFromWorkPart(test.part)
			if test.want && (err != nil || len(parameters) != 1) {
				t.Fatalf("parameters=%#v error=%v, want one parameter", parameters, err)
			}
			if !test.want && err == nil {
				t.Fatal("invalid parameters returned nil error")
			}
		})
	}
	for _, part := range []work.WorkContentPart{
		{Type: work.WorkContentPartTypeImage, URL: "file://image.png"},
		{Type: work.WorkContentPartTypeAudio, URL: "file://audio.wav"},
	} {
		if _, err := parameterJSONFromWorkPart(part); err == nil {
			t.Fatalf("parameterJSONFromWorkPart(%q) returned nil error", part.Type)
		}
	}

	validAndInvalidJSON := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "empty stream", raw: "", want: false},
		{name: "array", raw: "[]", want: false},
		{name: "blank key", raw: `{"":1}`, want: false},
		{name: "value decode error", raw: `{"x":}`, want: false},
		{name: "missing closing object", raw: `{"x":1`, want: false},
		{name: "wrong closing delimiter", raw: `{"x":1]`, want: false},
		{name: "trailing value", raw: `{"x":1} true`, want: false},
		{name: "trailing malformed", raw: `{"x":1} trailing`, want: false},
		{name: "valid", raw: `{"x":1,"nested":{"ok":true}}`, want: true},
	}
	for _, test := range validAndInvalidJSON {
		t.Run("decode/"+test.name, func(t *testing.T) {
			parameters, err := decodeOperationParameters([]byte(test.raw))
			if test.want && (err != nil || len(parameters) != 2) {
				t.Fatalf("decodeOperationParameters() = %#v, %v; want two parameters", parameters, err)
			}
			if !test.want && err == nil {
				t.Fatalf("decodeOperationParameters(%q) returned nil error", test.raw)
			}
		})
	}
}

func TestProposedOutputMapsAllModalitiesAndArtifacts(t *testing.T) {
	artifactRef, err := (models.InferenceArtifactRef{}).Parse("models-inference:artifact:audio")
	if err != nil {
		t.Fatalf("parse artifact: %v", err)
	}
	proposal, err := proposedOutputFromModelResult(models.InvokeModelResult{Outputs: []models.InferenceOutput{
		{Name: "text", Modality: models.ModalityText, Content: "hello"},
		{Name: "json", Modality: models.ModalityJSON, Content: `{"ok":true}`},
		{Name: "audio", Modality: models.ModalityAudio, MediaType: "audio/wav", Content: "audio-bytes", Artifact: &models.InferenceArtifact{
			Artifact: artifactRef, Name: "speech.wav", Properties: map[string]string{"label": "speech.wav"},
		}},
		{Name: "image", Modality: models.ModalityImage, ContentType: "image/png", Content: "file://image.png"},
		{Name: "binary", Modality: models.ModalityBinary, ContentType: "application/octet-stream", Content: "data:application/octet-stream;base64,Yg=="},
	}})
	if err != nil {
		t.Fatalf("proposedOutputFromModelResult() error = %v", err)
	}
	if len(proposal.Primary) != 5 || len(proposal.ArtifactRefs) != 1 || proposal.ArtifactRefs[0].ArtifactID != artifactRef.String() {
		t.Fatalf("proposal = %#v, want five parts and one artifact ref", proposal)
	}
	assertProposedTextOutput(t, proposal.Primary[0])
	assertProposedJSONOutput(t, proposal.Primary[1])
	assertProposedAudioOutput(t, proposal.Primary[2], artifactRef)
	assertProposedMediaOutputs(t, proposal.Primary[3:])
}

func assertProposedTextOutput(t *testing.T, part work.WorkContentPart) {
	t.Helper()
	if part.Type != work.WorkContentPartTypeText || part.Text != "hello" || part.ContentType != "text/plain" {
		t.Fatalf("text proposal = %#v", part)
	}
}

func assertProposedJSONOutput(t *testing.T, part work.WorkContentPart) {
	t.Helper()
	if part.Type != work.WorkContentPartTypeJSON || string(part.JSON) != `{"ok":true}` || part.ContentType != "application/json" {
		t.Fatalf("JSON proposal = %#v", part)
	}
}

func assertProposedAudioOutput(t *testing.T, part work.WorkContentPart, artifactRef models.InferenceArtifactRef) {
	t.Helper()
	if part.Type != work.WorkContentPartTypeAudio || !strings.HasPrefix(part.URL, "data:audio/wav;base64,") || part.ArtifactID != artifactRef.String() || part.Metadata["label"] != "speech.wav" {
		t.Fatalf("audio proposal = %#v", part)
	}
}

func assertProposedMediaOutputs(t *testing.T, parts []work.WorkContentPart) {
	t.Helper()
	if parts[0].Type != work.WorkContentPartTypeImage || parts[0].URL != "file://image.png" || parts[1].Type != work.WorkContentPartTypeBinary || parts[1].URL == "" {
		t.Fatalf("media proposals = %#v", parts)
	}
}

func TestProposedOutputUsesContentFallbackAndNamesByModality(t *testing.T) {
	proposal, err := proposedOutputFromModelResult(models.InvokeModelResult{Content: []models.InferenceContent{
		{Modality: models.ModalityAudio, ContentType: "audio/wav", Content: "audio"},
		{Modality: models.ModalityText, ContentType: "text/plain", Content: "text"},
		{Modality: models.ModalityJSON, ContentType: "application/json", Content: `{"ok":true}`},
	}})
	if err != nil {
		t.Fatalf("content fallback error = %v", err)
	}
	wantNames := []string{"audio", "text", "json"}
	for index, want := range wantNames {
		if proposal.Primary[index].Slot != want {
			t.Fatalf("proposal[%d].Slot = %q, want %q", index, proposal.Primary[index].Slot, want)
		}
	}
	if _, err := proposedOutputFromModelResult(models.InvokeModelResult{}); err == nil {
		t.Fatal("empty model result returned nil error")
	}
}

func TestWorkContentPartFromModelOutputRejectsMalformedOutputs(t *testing.T) {
	cases := []models.InferenceOutput{
		{Name: "json", Modality: models.ModalityJSON, Content: "not-json"},
		{Name: "audio", Modality: models.ModalityAudio},
		{Name: "image", Modality: models.ModalityImage},
		{Name: "binary", Modality: models.ModalityBinary},
		{Name: "unknown", Modality: models.Modality("video"), Content: "value"},
	}
	for _, output := range cases {
		if _, err := workContentPartFromModelOutput(output, 0); err == nil {
			t.Fatalf("workContentPartFromModelOutput(%q) returned nil error", output.Name)
		}
	}
	if _, err := workContentPartFromModelOutput(models.InferenceOutput{ContentType: "application/octet-stream", Content: "value"}, 0); err == nil {
		t.Fatal("output with unsupported media-derived modality returned nil error")
	}
}

func TestGenericInvocationInputMetadataBranches(t *testing.T) {
	t.Parallel()
	if got, want := inputContentMetadata("", "fallback"); got != want || got != "fallback" {
		t.Fatalf("inputContentMetadata empty = %q/%q, want fallback", got, want)
	}
	if got, media := inputContentMetadata(" image/png ", "fallback"); got != "image/png" || media != "image/png" {
		t.Fatalf("inputContentMetadata media = %q/%q", got, media)
	}
	if got, media := inputContentMetadata("token", "fallback"); got != "token" || media != "" {
		t.Fatalf("inputContentMetadata token = %q/%q", got, media)
	}
}

func TestGenericInvocationInlineOutputURLBranches(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"data:text/plain;base64,eA==", "file://value", "http://value", "https://value"} {
		if got := inlineOutputURL(value, "text/plain"); got != value {
			t.Fatalf("inlineOutputURL(%q) = %q, want unchanged URL", value, got)
		}
	}
	if got := inlineOutputURL("", "audio/wav"); got != "" {
		t.Fatalf("inlineOutputURL empty = %q, want empty", got)
	}
	if got := inlineOutputURL("bytes", "invalid"); !strings.HasPrefix(got, "data:application/octet-stream;base64,") {
		t.Fatalf("inlineOutputURL invalid media = %q", got)
	}
}

func TestGenericInvocationOutputModalityBranches(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		values []string
		want   models.Modality
	}{
		{values: []string{"text/plain"}, want: models.ModalityText},
		{values: []string{"audio/wav"}, want: models.ModalityAudio},
		{values: []string{"image/png"}, want: models.ModalityImage},
		{values: []string{"application/json"}, want: models.ModalityJSON},
		{values: []string{"application/octet-stream"}, want: ""},
		{values: []string{"", "audio/wav"}, want: models.ModalityAudio},
	} {
		if got := modalityFromMediaType(test.values...); got != test.want {
			t.Fatalf("modalityFromMediaType(%v) = %q, want %q", test.values, got, test.want)
		}
	}
}

func TestGenericInvocationDefaultOutputNamesAndText(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		modality models.Modality
		index    int
		want     string
	}{
		{models.ModalityAudio, 0, "audio"},
		{models.ModalityText, 1, "text"},
		{models.ModalityJSON, 2, "json"},
		{models.ModalityImage, 3, "output-4"},
	} {
		if got := defaultOutputName(test.modality, test.index); got != test.want {
			t.Fatalf("defaultOutputName(%q,%d) = %q, want %q", test.modality, test.index, got, test.want)
		}
	}
	text := textContentFromProposedOutput(workers.ProposedOutput{Primary: []work.WorkContentPart{
		{Type: work.WorkContentPartTypeText, Text: " first "},
		{Type: work.WorkContentPartTypeAudio, URL: "audio"},
		{Type: work.WorkContentPartTypeText, Text: "second"},
		{Type: work.WorkContentPartTypeText, Text: "  "},
	}})
	if text != "first\nsecond" {
		t.Fatalf("textContentFromProposedOutput() = %q, want joined text", text)
	}
}

func TestRunnerUsesRequestScopedModelsOverrideAndRuntimeProjection(t *testing.T) {
	modelsEdge := &captureModelsService{result: models.InvokeModelResult{
		Status:  models.ModelInvocationStatusCompleted,
		Outputs: []models.InferenceOutput{{Name: "text", Modality: models.ModalityText, Content: "override output"}},
	}}
	runner := newTestRunner(t, &captureModelsService{result: models.InvokeModelResult{
		Status:  models.ModelInvocationStatusCompleted,
		Outputs: []models.InferenceOutput{{Name: "text", Modality: models.ModalityText, Content: "wrong edge"}},
	}}, nil)
	scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:request-scope")
	if err != nil {
		t.Fatalf("parse request scope: %v", err)
	}
	request := validRequest()
	request.ModelRuntime = &workers.ModelRuntimeInput{
		Scope:     scope,
		Worker:    models.LocalWorker{Name: "request-worker", Type: models.RuntimeWorkerTypeInference, Model: "request-model", ModelLocality: models.RuntimeModelLocalityLocal},
		Resources: []models.LocalResource{{ID: "request-resource", Name: "request", Capacity: 1}},
	}
	request.ModelInvocationOverride = modelsEdge
	result, err := runner.Execute(context.Background(), request)
	if err != nil {
		var providerErr *workers.ProviderError
		errors.As(err, &providerErr)
		t.Fatalf("Execute() error = %v provider=%#v", err, providerErr)
	}
	if result.Content != "override output" || modelsEdge.Calls() != 1 {
		t.Fatalf("result=%#v calls=%d, want request-scoped override", result, modelsEdge.Calls())
	}
	captured := modelsEdge.Request()
	if captured.Scope != scope || captured.Model.NameOrURI != "request-model" || captured.Holder != "dispatch-1" {
		t.Fatalf("request-scoped invocation = %#v", captured)
	}
	if _, err := runner.Execute(context.Background(), func() workers.RunnerExecutionRequest {
		invalid := validRequest()
		invalid.ModelInvocationOverride = "not-a-model-invoker"
		return invalid
	}()); err == nil {
		t.Fatal("invalid request-scoped Models override returned nil error")
	}
}

func TestRunnerRejectsFailedAndCancelledModelStatuses(t *testing.T) {
	for _, status := range []models.ModelInvocationStatus{models.ModelInvocationStatusFailed, models.ModelInvocationStatusCancelled} {
		t.Run(string(status), func(t *testing.T) {
			runner := newTestRunner(t, &captureModelsService{result: models.InvokeModelResult{Status: status}}, nil)
			if _, err := runner.Execute(context.Background(), validRequest()); err == nil {
				t.Fatalf("Execute() status=%q returned nil error", status)
			}
		})
	}
}

func TestInferenceFactorySnapshotsCopyResources(t *testing.T) {
	if got := WorkerFromFactory(nil); !reflect.DeepEqual(got, models.LocalWorker{}) {
		t.Fatalf("WorkerFromFactory(nil) = %#v, want zero worker", got)
	}
	worker := WorkerFromFactory(&interfaces.FactoryWorkerConfig{
		Name: "factory-worker", Type: interfaces.WorkerTypeInference, Model: "factory-model", ModelLocality: models.RuntimeModelLocalityLocal,
		Resources: []interfaces.ResourceConfig{{ID: "resource", Name: "model", Type: "MODEL", Capacity: 2, Model: "factory-model", Backend: "fixture", LoadPolicy: "shared", Provider: "local"}},
	})
	if worker.Name != "factory-worker" || worker.Model != "factory-model" || len(worker.Resources) != 1 || worker.Resources[0].ID != "resource" {
		t.Fatalf("WorkerFromFactory() = %#v", worker)
	}
	if got := ResourcesFromFactory(nil); got != nil {
		t.Fatalf("ResourcesFromFactory(nil) = %#v, want nil", got)
	}
	resources := ResourcesFromFactory([]interfaces.ResourceConfig{{ID: "r", Name: "resource", Capacity: 3, Backend: "fixture"}})
	if len(resources) != 1 || resources[0].ID != "r" || resources[0].Capacity != 3 || resources[0].Backend != "fixture" {
		t.Fatalf("ResourcesFromFactory() = %#v", resources)
	}
	if got := snapshotResources(nil); got != nil {
		t.Fatalf("snapshotResources(nil) = %#v, want nil", got)
	}
}

func TestWorkContentPartEmptyRecognizesArtifactBackedParts(t *testing.T) {
	if workContentPartEmpty(work.WorkContentPart{Type: work.WorkContentPartTypeText, ArtifactID: "models-inference:artifact:text"}) {
		t.Fatal("artifact-backed text was considered empty")
	}
	if workContentPartEmpty(work.WorkContentPart{Type: work.WorkContentPartTypeJSON, ArtifactID: "models-inference:artifact:json"}) {
		t.Fatal("artifact-backed JSON was considered empty")
	}
	if workContentPartEmpty(work.WorkContentPart{Type: work.WorkContentPartTypeAudio, File: "file://audio.wav"}) {
		t.Fatal("file-backed media was considered empty")
	}
}

func TestNormalizeInvocationErrorPreservesProviderAndCancellation(t *testing.T) {
	runner := newTestRunner(t, &captureModelsService{}, nil).(*runner)
	providerErr := workers.NewProviderError(workers.WorkFailureTypeThrottled, "throttled", nil)
	if got := runner.normalizeInvocationError(providerErr, validRequest()); !errors.Is(got, providerErr) {
		t.Fatalf("provider error normalized to %v, want original", got)
	}
	if got := runner.normalizeInvocationError(context.Canceled, validRequest()); !errors.Is(got, context.Canceled) {
		t.Fatalf("cancellation normalized to %v, want context.Canceled", got)
	}
}
