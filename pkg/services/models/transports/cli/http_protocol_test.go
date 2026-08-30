package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type testHTTPClock struct{}

func (testHTTPClock) Now() time.Time { return time.Unix(1, 0) }
func testHTTPProtocol(t *testing.T) clihttp.Protocol {
	t.Helper()
	protocol, err := clihttp.NewProtocol(&http.Client{}, testHTTPClock{})
	if err != nil {
		t.Fatalf("build test HTTP protocol: %v", err)
	}
	return protocol
}

type pullProgressWriter struct {
	mu       sync.Mutex
	output   bytes.Buffer
	progress chan struct{}
	once     sync.Once
}

func TestLegacyModelsRemoveProjectsHTTPResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete || request.URL.Path != "/models/voice" {
			t.Fatalf("remove request = %s %s, want DELETE /models/voice", request.Method, request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"modelName":"voice","revision":"rev-1","cachePath":"/models/voice/rev-1","outcome":"REMOVED","bytesRemoved":42}`)
	}))
	t.Cleanup(server.Close)
	service := New(testHTTPProtocol(t), testModelInvocationBuilder)

	var human bytes.Buffer
	if err := service.Remove(RemoveConfig{Context: context.Background(), ModelName: "voice", Server: server.URL, Output: &human}); err != nil {
		t.Fatalf("legacy remove human: %v", err)
	}
	if !strings.Contains(human.String(), "voice\tREMOVED\trev-1") || !strings.Contains(human.String(), "42 bytes") {
		t.Fatalf("legacy remove human = %q, want rendered removal facts", human.String())
	}

	var structured bytes.Buffer
	if err := service.Remove(RemoveConfig{Context: context.Background(), ModelName: "voice", Server: server.URL, JSON: true, Output: &structured}); err != nil {
		t.Fatalf("legacy remove JSON: %v", err)
	}
	var response factoryapi.ModelRemoveResponse
	if err := json.Unmarshal(structured.Bytes(), &response); err != nil {
		t.Fatalf("legacy remove JSON decode: %v", err)
	}
	if response.ModelName != "voice" || response.BytesRemoved != 42 {
		t.Fatalf("legacy remove response = %#v, want voice/42", response)
	}

	legacy := &httpService{http: testHTTPProtocol(t)}
	var direct bytes.Buffer
	if err := legacy.Remove(RemoveConfig{
		Context: context.Background(), ModelName: "voice", Server: server.URL, Output: &direct,
	}); err != nil {
		t.Fatalf("direct legacy remove: %v", err)
	}
	if !strings.Contains(direct.String(), "voice\tREMOVED\trev-1") {
		t.Fatalf("direct legacy remove = %q, want rendered removal facts", direct.String())
	}
}

func TestLegacyHTTPAdapterValidatesOperationInputs(t *testing.T) {
	service := &httpService{}
	output := &bytes.Buffer{}
	assertError := func(name string, call func() error, want string) {
		t.Helper()
		err := call()
		if err == nil || err.Error() != want {
			t.Fatalf("%s error = %v, want %q", name, err, want)
		}
	}

	assertError("List context", func() error {
		return service.List(ListConfig{Output: output})
	}, "context is required")
	assertError("List output", func() error {
		return service.List(ListConfig{Context: context.Background()})
	}, "output writer is required")
	assertError("Inspect context", func() error {
		return service.Inspect(InspectConfig{Output: output})
	}, "context is required")
	assertError("Inspect output", func() error {
		return service.Inspect(InspectConfig{Context: context.Background()})
	}, "output writer is required")
	assertError("Invoke context", func() error {
		return service.Invoke(InvokeConfig{Output: output})
	}, "context is required")
	assertError("Invoke output", func() error {
		return service.Invoke(InvokeConfig{Context: context.Background()})
	}, "output writer is required")
	assertError("Invoke model", func() error {
		return service.Invoke(InvokeConfig{Context: context.Background(), Output: output})
	}, "model name is required")
	assertError("Invoke operation", func() error {
		return service.Invoke(InvokeConfig{Context: context.Background(), ModelName: "voice", Output: output})
	}, "--operation is required")
	assertError("Invoke text", func() error {
		return service.Invoke(InvokeConfig{Context: context.Background(), ModelName: "voice", Operation: "TTS", Output: output})
	}, "--text is required")
	assertError("Pull context", func() error {
		return service.Pull(PullConfig{Output: output})
	}, "context is required")
	assertError("Pull output", func() error {
		return service.Pull(PullConfig{Context: context.Background()})
	}, "output writer is required")
	assertError("Pull model", func() error {
		return service.Pull(PullConfig{Context: context.Background(), Output: output})
	}, "model name is required")
	assertError("Remove context", func() error {
		return service.Remove(RemoveConfig{Output: output})
	}, "context is required")
	assertError("Remove output", func() error {
		return service.Remove(RemoveConfig{Context: context.Background()})
	}, "output writer is required")
	assertError("Remove model", func() error {
		return service.Remove(RemoveConfig{Context: context.Background(), Output: output})
	}, "model name is required")
}

func (writer *pullProgressWriter) Write(payload []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if bytes.Contains(payload, []byte("models pull progress")) {
		writer.once.Do(func() { close(writer.progress) })
	}
	return writer.output.Write(payload)
}

func (writer *pullProgressWriter) String() string {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.output.String()
}

func TestModelsPullEmitsElapsedProgressOnDiagnosticsWhileWaiting(t *testing.T) {
	t.Parallel()

	pullStarted := make(chan struct{})
	releasePull := make(chan struct{})
	protocol, err := clihttp.NewProtocol(modelsPullDoer(func(*http.Request) (*http.Response, error) {
		close(pullStarted)
		<-releasePull
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				`{"modelName":"voice","providerLocality":"LOCAL","outcome":"PULLED","managedRuntimePull":{"identity":"voice","pullOutcome":"INSTALLED_SUCCESSFULLY","readinessState":"READY"}}`,
			)),
		}, nil
	}), testHTTPClock{})
	if err != nil {
		t.Fatalf("pull protocol: %v", err)
	}
	progress := &pullProgressWriter{progress: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		_, pullErr := pullModel(pullOptions{
			Context: context.Background(), ModelName: "voice", Server: "http://factory.test",
			Verbose: true, Diagnostics: progress, HTTP: protocol,
			ProgressInterval: 5 * time.Millisecond,
			Now:              testHTTPClock{}.Now,
		})
		done <- pullErr
	}()
	select {
	case <-pullStarted:
	case <-time.After(time.Second):
		t.Fatal("pull protocol was not invoked")
	}
	select {
	case <-progress.progress:
	case <-time.After(time.Second):
		t.Fatal("pull emitted no elapsed progress while waiting")
	}
	close(releasePull)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("pull error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pull did not stop progress after terminal response")
	}
	diagnostics := progress.String()
	if !strings.Contains(diagnostics, `models pull progress modelName="voice"`) ||
		!strings.Contains(diagnostics, "elapsed=") {
		t.Fatalf("progress diagnostics = %q, want model and elapsed time", diagnostics)
	}
}

func TestModelsVerboseLogsFailureStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"model not found","family":"NOT_FOUND","code":"NOT_FOUND"}`)
	}))
	defer server.Close()

	var diagnostics bytes.Buffer
	_, err := queryModel(queryOptions{
		Context:     context.Background(),
		HTTP:        testHTTPProtocol(t),
		Server:      strings.TrimSuffix(server.URL, "/"),
		ModelName:   "missing",
		Verbose:     true,
		Diagnostics: &diagnostics,
	})
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("queryModel error = %v, want ErrModelNotFound", err)
	}
	assertDiagnosticsContains(t, diagnostics.String(), []string{
		"models inspect response",
		"endpointPath=/models/missing",
		"status=404",
	})
}

func assertDiagnosticsContains(t *testing.T, got string, wants []string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, got)
		}
	}
}

func TestPullMappingProjectsDiagnosticFields(t *testing.T) {
	t.Parallel()

	result := modelinference.PullResult{
		ModelName: "voice", Outcome: "PULLED", CachePath: "cache", Revision: "rev",
		PullDiagnostics: modelinference.PullDiagnostics{
			ModelName: "voice", ResolvedRepository: "owner/repo", Revision: "rev",
			File: "weights.gguf", Operation: "download", RequestURL: "https://assets.example.test/weights.gguf",
			UpstreamStatusCode: http.StatusBadGateway,
		},
		SourceKind: "UPSTREAM_REPOSITORY", SourceID: "owner/repo", ResolverNotes: "resolved",
	}
	diagnostics := managedRuntimePullDiagnostics(result)
	if diagnostics == nil || diagnostics.ModelName == nil || *diagnostics.ModelName != "voice" ||
		diagnostics.RequestUrl == nil || diagnostics.UpstreamStatusCode == nil ||
		*diagnostics.UpstreamStatusCode != http.StatusBadGateway {
		t.Fatalf("managed pull diagnostics = %#v, want safe diagnostic fields", diagnostics)
	}
	source := managedRuntimePullSourceDiagnostics(result)
	if source == nil || source.SourceKind == nil || source.SourceId == nil || source.ResolverNotes == nil {
		t.Fatalf("source diagnostics = %#v, want source facts", source)
	}
	if managedRuntimePullDiagnostics(modelinference.PullResult{}) != nil ||
		managedRuntimePullSourceDiagnostics(modelinference.PullResult{}) != nil {
		t.Fatal("empty pull diagnostics unexpectedly projected")
	}
}

func TestPullMappingProjectsOutcomeDefaults(t *testing.T) {
	t.Parallel()

	if got := managedRuntimePullOutcome(modelinference.PullResult{Outcome: "PULLED"}); got != factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY {
		t.Fatalf("blank managed outcome = %q, want INSTALLED_SUCCESSFULLY", got)
	}
	if got := managedRuntimePullOutcome(modelinference.PullResult{}); got != factoryapi.ManagedRuntimePullOutcomeUNSUPPORTEDRUNTIME {
		t.Fatalf("empty managed outcome = %q, want UNSUPPORTED_RUNTIME", got)
	}
	if got := managedRuntimePullReadiness(modelinference.PullResult{}); got != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("blank readiness = %q, want READY", got)
	}
	if got := managedRuntimeLifecycleFromPull(factoryapi.ModelPullResponse{
		ManagedRuntimePull: factoryapi.ManagedRuntimePullResult{PullOutcome: factoryapi.ManagedRuntimePullOutcomeSTILLLOADING},
	}); got != string(factoryapi.ManagedRuntimeLifecycleStateINSTALLING) {
		t.Fatalf("loading lifecycle = %q, want INSTALLING", got)
	}
	if got := managedRuntimeLifecycleFromPull(factoryapi.ModelPullResponse{}); got != "UNKNOWN" {
		t.Fatalf("unknown lifecycle = %q, want UNKNOWN", got)
	}
}

func TestPullMappingClassifiesResponseFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		statusCode int
		body       string
		wantError  bool
	}{
		{name: "non classified status", statusCode: http.StatusOK, body: `{}`, wantError: false},
		{name: "invalid body", statusCode: http.StatusUnprocessableEntity, body: `{`, wantError: false},
		{name: "successful outcome", statusCode: http.StatusGatewayTimeout, body: `{"managedRuntimePull":{"pullOutcome":"ALREADY_READY"}}`, wantError: false},
		{name: "gateway failure", statusCode: http.StatusGatewayTimeout, body: `{"managedRuntimePull":{"pullOutcome":"TIMED_OUT","readinessState":"FAILED"}}`, wantError: true},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := managedRuntimePullResponseError(testCase.statusCode, []byte(testCase.body))
			if (err != nil) != testCase.wantError {
				t.Fatalf("managedRuntimePullResponseError() = %v, want error=%v", err, testCase.wantError)
			}
		})
	}
}

func TestPullMappingFailureUnwrapsDiagnostics(t *testing.T) {
	t.Parallel()

	var nilFailure *managedRuntimePullFailure
	if nilFailure.Error() != "" || nilFailure.Unwrap() != nil {
		t.Fatalf("nil pull failure methods = (%q, %v), want empty/nil", nilFailure.Error(), nilFailure.Unwrap())
	}
	failure := &managedRuntimePullFailure{
		Outcome:     factoryapi.ManagedRuntimePullOutcomeSOURCEFETCHFAILED,
		Readiness:   factoryapi.ManagedRuntimeReadinessStateFAILED,
		Diagnostics: errors.New("diagnostic"),
	}
	if failure.Error() == "" || !errors.Is(failure, failure.Diagnostics) || failure.CLIErrorCode() != managedRuntimePullFailureCode {
		t.Fatalf("pull failure = %v, want coded diagnostic failure", failure)
	}
}

func TestCLIOutputShapeValidationBranches(t *testing.T) {
	t.Parallel()

	detail := cliOutputValidationDetail()
	cases := []struct {
		name string
		cfg  InvokeConfig
		op   string
		want string
	}{
		{name: "mapping with output path", cfg: InvokeConfig{OutputMappings: []string{"text=a"}, OutputPath: "speech.wav"}, op: "OMNI", want: "cannot be combined"},
		{name: "mapping missing output", cfg: InvokeConfig{OutputMappings: []string{"text=a"}, JSON: true}, op: "OMNI", want: "cover every output"},
		{name: "mapping unknown slot", cfg: InvokeConfig{OutputMappings: []string{"other=a", "usage=b"}, JSON: true}, op: "OMNI", want: "unknown slot"},
		{name: "multiple outputs without mode", cfg: InvokeConfig{}, op: "OMNI", want: "multiple model outputs"},
		{name: "unknown operation without mode", cfg: InvokeConfig{}, op: "UNKNOWN", want: "--output is required"},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := validateCLIOutputShape(testCase.cfg, detail, testCase.op)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("validateCLIOutputShape() = %v, want %q", err, testCase.want)
			}
		})
	}
	valid := []struct {
		name   string
		config InvokeConfig
		detail modelinference.Detail
		op     string
	}{
		{name: "json", config: InvokeConfig{JSON: true}, detail: detail, op: "OMNI"},
		{name: "audio", config: InvokeConfig{OutputPath: "speech.wav"}, detail: cliOutputAudioDetail(), op: "AUDIO"},
		{name: "inline", config: InvokeConfig{}, detail: cliOutputInlineDetail(), op: "TEXT"},
	}
	for _, testCase := range valid {
		if err := validateCLIOutputShape(testCase.config, testCase.detail, testCase.op); err != nil {
			t.Fatalf("%s output shape = %v, want success", testCase.name, err)
		}
	}
}

func TestCLIOutputMappingValidationBranches(t *testing.T) {
	t.Parallel()

	detail := cliOutputValidationDetail()
	cases := []struct {
		name   string
		values []string
		want   string
	}{
		{name: "missing equals", values: []string{"text"}, want: "expected slot=path"},
		{name: "empty slot", values: []string{"=text.out"}, want: "slot and path are required"},
		{name: "dash path", values: []string{"text=-"}, want: "path '-'"},
		{name: "duplicate slot", values: []string{"text=a", "text=b"}, want: "duplicate"},
		{name: "duplicate path", values: []string{"text=a", "usage=a"}, want: "same path"},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseGenericCLIOutputMappings(testCase.values); err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("parseGenericCLIOutputMappings() = %v, want %q", err, testCase.want)
			}
		})
	}
	if err := validateGenericCLIOutputMappings([]string{"text=a", "usage=b"}, detail.Operations[0], true); err != nil {
		t.Fatalf("valid output mappings = %v, want success", err)
	}
	if err := validateGenericCLIOutputMappings([]string{"text=a"}, detail.Operations[0], false); err == nil || !strings.Contains(err.Error(), "unknown operation") {
		t.Fatalf("unknown operation mappings = %v, want unknown-operation error", err)
	}
}

func TestCLIOutputPresentationBranches(t *testing.T) {
	t.Parallel()

	required := true
	optional := false
	inputs := []modelinference.OperationSlot{
		{Name: "audio", Modality: modelinference.ModalityAudio},
		{Name: "optional", Modality: modelinference.ModalityText, Required: &optional},
		{Name: "text", Modality: modelinference.ModalityText, Required: &required, MediaTypes: []string{"text/custom"}},
	}
	assertJoinedCLITextInputBranches(t, inputs, optional)
	assertCLIOutputPresentationRequest(t, inputs)
}

func assertJoinedCLITextInputBranches(t *testing.T, inputs []modelinference.OperationSlot, optional bool) {
	t.Helper()
	if input := joinedCLITextInput(inputs); input == nil || input.Name != "text" {
		t.Fatalf("joinedCLITextInput() = %#v, want required text input", input)
	}
	if input := joinedCLITextInput([]modelinference.OperationSlot{{Name: "optional", Modality: modelinference.ModalityText, Required: &optional}}); input == nil || input.Name != "optional" {
		t.Fatalf("joinedCLITextInput() = %#v, want optional text input", input)
	}
	if input := joinedCLITextInput([]modelinference.OperationSlot{{Name: "audio", Modality: modelinference.ModalityAudio}}); input != nil {
		t.Fatalf("joinedCLITextInput() = %#v, want nil", input)
	}
}

func assertCLIOutputPresentationRequest(t *testing.T, inputs []modelinference.OperationSlot) {
	t.Helper()
	scope, err := (modelinference.RuntimeScopeRef{}).Parse("output-coverage:scope")
	if err != nil {
		t.Fatalf("parse scope: %v", err)
	}
	catalog := modelinference.Detail{Summary: modelinference.Summary{ProviderLocality: modelinference.LocalityCloud}, Capabilities: []modelinference.Capability{{
		Worker: "text-worker", ProviderLocality: modelinference.LocalityCloud,
		Operations: []modelinference.Operation{{Name: "OMNI", Inputs: inputs}},
	}}}
	request := joinedCLIInvocationRequestFromInputs(scope, "model", "OMNI", "hello", nil, nil, catalog)
	if request.Inputs[0].Name != "text" || request.Inputs[0].ContentType != "text/custom" || request.Inputs[0].Content != "hello" {
		t.Fatalf("joined request = %#v, want selected text slot", request)
	}
	if worker, locality := catalogPresentationForOperation(catalog, "OMNI"); worker != "text-worker" || locality != string(modelinference.LocalityCloud) {
		t.Fatalf("catalog presentation = (%q, %q), want text-worker/REMOTE", worker, locality)
	}
	if worker, locality := catalogPresentationForOperation(catalog, "MISSING"); worker != "" || locality != string(modelinference.LocalityCloud) {
		t.Fatalf("catalog fallback presentation = (%q, %q), want empty/REMOTE", worker, locality)
	}
	bindings := resolvedPresentationBindings(catalog, "OMNI", "hello")
	if len(bindings) != 1 || bindings[0].Slot != "audio" || bindings[0].Source != "INPUT" || bindings[0].Content[0].Text != "hello" {
		t.Fatalf("presentation bindings = %#v, want first named input binding", bindings)
	}
	if bindings := resolvedPresentationBindings(catalog, "MISSING", "hello"); len(bindings) != 0 {
		t.Fatalf("missing presentation bindings = %#v, want empty", bindings)
	}
}

func TestCLIOutputContentProjectionBranches(t *testing.T) {
	t.Parallel()

	assertCLIContentParts(t)
	assertCLIInferenceArtifactProjection(t)
}

func assertCLIContentParts(t *testing.T) {
	t.Helper()
	parts := inferenceContentToWorkParts([]modelinference.InferenceContent{
		{ContentType: "audio/wav", Content: "speech.wav"},
		{ContentType: "image/png", Content: "https://image"},
		{ContentType: "application/json", Content: `{"ok":true}`},
		{Content: "plain text"},
		{ContentType: "text/custom", Content: "custom"},
	})
	if len(parts) != 5 || parts[0].Type != work.WorkContentPartTypeAudio || parts[1].Type != work.WorkContentPartTypeImage ||
		parts[2].Type != work.WorkContentPartTypeJSON || parts[3].Type != work.WorkContentPartTypeText {
		t.Fatalf("content parts = %#v, want audio/image/json/text projections", parts)
	}
	var parsedJSON map[string]any
	if err := json.Unmarshal(parts[2].JSON, &parsedJSON); err != nil || parsedJSON["ok"] != true {
		t.Fatalf("JSON content part = %q, %v", parts[2].JSON, err)
	}
	if parts[0].File != "speech.wav" || parts[1].URL != "https://image" || parts[3].ContentType != "text/plain" {
		t.Fatalf("content part payloads = %#v, want projected payloads", parts)
	}
}

func assertCLIInferenceArtifactProjection(t *testing.T) {
	t.Helper()
	ref, err := (modelinference.InferenceArtifactRef{}).Parse("artifact:usage")
	if err != nil {
		t.Fatalf("parse artifact ref: %v", err)
	}
	result := modelinference.InvokeModelResult{
		ModelName: "model", Operation: "OMNI",
		Content:   []modelinference.InferenceContent{{ContentType: "text/plain", Content: "answer"}},
		Artifacts: []modelinference.InferenceArtifact{{Artifact: ref}},
		Outputs: []modelinference.InferenceOutput{{
			Name: "usage", Modality: modelinference.ModalityJSON, ContentType: "application/json", MediaType: "application/json", Content: `{"tokens":2}`,
			Artifact: &modelinference.InferenceArtifact{Artifact: ref, Name: "usage.json", MediaType: "application/json", SizeBytes: 7, Properties: map[string]string{"digest": "sha"}},
		}, {Name: "empty-artifact", Artifact: &modelinference.InferenceArtifact{}}},
	}
	response := genericInvocationResponseFromInferenceResult(result)
	if len(response.Outputs) != 2 || response.Outputs[0].Artifact == nil || response.Outputs[0].Artifact.SizeBytes == nil ||
		*response.Outputs[0].Artifact.SizeBytes != 7 || response.Outputs[1].Artifact != nil {
		t.Fatalf("generic response = %#v, want projected artifact metadata", response)
	}
	if genericCLIStringPointer("") != nil || genericCLIStringPointer(" value ") == nil {
		t.Fatal("generic string pointer normalization did not distinguish empty and non-empty values")
	}
}

func TestCLIOutputResponseClassificationBranches(t *testing.T) {
	t.Parallel()

	inlineDetail := cliOutputInlineDetail()
	jsonDetail := cliOutputValidationDetail()
	result := modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Content: "answer"}}}
	if genericCLIJSONResult(InvokeConfig{}, inlineDetail, "TEXT", result) {
		t.Fatal("non-JSON result classified as JSON")
	}
	if !genericCLIJSONResult(InvokeConfig{JSON: true}, jsonDetail, "OMNI", modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{}, {}}}) {
		t.Fatal("multiple JSON outputs not classified as JSON")
	}
	if !genericCLIInlineOutput(InvokeConfig{}, inlineDetail, "TEXT") || genericCLIInlineOutput(InvokeConfig{OutputPath: "speech.wav"}, inlineDetail, "TEXT") {
		t.Fatal("inline output classification did not honor modality or output path")
	}
	for _, modality := range []modelinference.Modality{modelinference.ModalityText, modelinference.ModalityJSON} {
		if !genericCLIInlineModality(modality) {
			t.Fatalf("modality %q = false, want inline", modality)
		}
	}
	if genericCLIInlineModality(modelinference.ModalityAudio) {
		t.Fatal("audio modality classified as inline")
	}
}

func cliOutputValidationDetail() modelinference.Detail {
	required := true
	return modelinference.Detail{Summary: modelinference.Summary{Operations: []modelinference.Operation{{
		Name:    "OMNI",
		Inputs:  []modelinference.OperationSlot{{Name: "audio", Modality: modelinference.ModalityAudio}, {Name: "text", Modality: modelinference.ModalityText, Required: &required}},
		Outputs: []modelinference.OperationSlot{{Name: "text", Modality: modelinference.ModalityText}, {Name: "usage", Modality: modelinference.ModalityJSON}},
	}}}}
}

func cliOutputAudioDetail() modelinference.Detail {
	return modelinference.Detail{Summary: modelinference.Summary{Operations: []modelinference.Operation{{
		Name: "AUDIO", Outputs: []modelinference.OperationSlot{{Name: "audio", Modality: modelinference.ModalityAudio}},
	}}}}
}

func cliOutputInlineDetail() modelinference.Detail {
	return modelinference.Detail{Summary: modelinference.Summary{Operations: []modelinference.Operation{{
		Name: "TEXT", Outputs: []modelinference.OperationSlot{{Name: "text", Modality: modelinference.ModalityText}},
	}}}}
}

func TestModelsRemoteGenericInvokeUsesDedicatedLongRunningProtocol(t *testing.T) {
	t.Parallel()

	var standardCalls atomic.Int32
	standard, err := clihttp.NewProtocol(modelsPullDoer(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.Path != "/models/llm" {
			return nil, fmt.Errorf("standard protocol request = %s %s, want GET /models/llm", request.Method, request.URL.Path)
		}
		standardCalls.Add(1)
		body, marshalErr := json.Marshal(remoteOMNIModelDetail())
		if marshalErr != nil {
			return nil, marshalErr
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(body)),
		}, nil
	}), testHTTPClock{})
	if err != nil {
		t.Fatalf("standard protocol: %v", err)
	}

	var longRunningCalls atomic.Int32
	longRunning, err := clihttp.NewProtocol(modelsPullDoer(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.Path != "/models/invocations" {
			return nil, fmt.Errorf("long-running protocol request = %s %s, want POST /models/invocations", request.Method, request.URL.Path)
		}
		if _, ok := request.Context().Deadline(); ok {
			return nil, errors.New("remote inference inherited a fixed client deadline")
		}
		longRunningCalls.Add(1)
		text := "remote answer"
		body, marshalErr := json.Marshal(factoryapi.GenericModelInvocationResponse{
			Outputs: []factoryapi.ModelInvocationOutput{{
				Name:     "text",
				Modality: factoryapi.ModelInvocationContentTypeText,
				Content:  &text,
			}},
		})
		if marshalErr != nil {
			return nil, marshalErr
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(body)),
		}, nil
	}), testHTTPClock{})
	if err != nil {
		t.Fatalf("long-running protocol: %v", err)
	}

	service := &httpService{
		http:            standard,
		pullHTTP:        longRunning,
		inputFileReader: remoteStaticInputReader([]byte("PNG")),
	}
	var output bytes.Buffer
	if err := service.Invoke(remoteInvokeConfig(context.Background(), "http://factory.test", []string{
		"prompt=Describe this image", "image=@fixture.png",
	}, &output)); err != nil {
		t.Fatalf("remote generic invoke: %v", err)
	}
	if output.String() != "remote answer" {
		t.Fatalf("remote output = %q, want long-running response", output.String())
	}
	if standardCalls.Load() != 1 || longRunningCalls.Load() != 1 {
		t.Fatalf("protocol calls = standard:%d long-running:%d, want one catalog and one inference call", standardCalls.Load(), longRunningCalls.Load())
	}
}
