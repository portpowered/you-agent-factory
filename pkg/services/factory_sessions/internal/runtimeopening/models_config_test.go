package runtimeopening

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type runtimeConfigLookupStub struct {
	factoryDir string
	baseDir    string
	cfg        *factorydefinitions.FactoryConfig
}

func (s runtimeConfigLookupStub) FactoryConfig() *factorydefinitions.FactoryConfig { return s.cfg }
func (s runtimeConfigLookupStub) FactoryDir() string                               { return s.factoryDir }
func (s runtimeConfigLookupStub) RuntimeBaseDir() string                           { return s.baseDir }
func (s runtimeConfigLookupStub) Worker(string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	return nil, false
}
func (s runtimeConfigLookupStub) Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}

func TestProjectModelsRuntimeConfigReturnsNilWithoutFactoryConfig(t *testing.T) {
	t.Parallel()

	if got := ProjectModelsRuntimeConfig(nil); got != nil {
		t.Fatalf("ProjectModelsRuntimeConfig(nil) = %#v, want nil", got)
	}
	if got := ProjectModelsRuntimeConfig(runtimeConfigLookupStub{}); got != nil {
		t.Fatalf("ProjectModelsRuntimeConfig(empty lookup) = %#v, want nil", got)
	}
}

func TestProjectModelsRuntimeConfigProjectsWorkersResourcesAndOperations(t *testing.T) {
	t.Parallel()

	source := runtimeConfigLookupStub{
		factoryDir: "/factory",
		baseDir:    "/runtime",
		cfg: &factorydefinitions.FactoryConfig{
			Workers: []factorydefinitions.FactoryWorkerConfig{
				{
					Name:          "worker-a",
					Type:          "model",
					Model:         "fixture-model",
					ModelProvider: "mock",
					ModelLocality: "local",
					Command:       "echo",
					Args:          []string{"hello"},
					Operations: []factorydefinitions.ModelOperation{
						{
							Name: "invoke",
							Inputs: []factorydefinitions.ModelOperationSlot{
								{Name: "text", ContentTypes: []string{"text/plain"}, Required: true},
							},
							Outputs: []factorydefinitions.ModelOperationSlot{
								{Name: "result", ContentTypes: []string{"text/plain"}},
							},
						},
					},
					Resources: []factorydefinitions.ResourceConfig{
						{ID: "gpu", Name: "GPU", Type: "device", Capacity: 1},
					},
				},
			},
			Resources: []factorydefinitions.ResourceConfig{
				{ID: "shared", Name: "Shared", Type: "pool", Capacity: 2, Provider: "mock"},
			},
		},
	}

	projected := ProjectModelsRuntimeConfig(source)
	if projected == nil {
		t.Fatal("ProjectModelsRuntimeConfig() = nil, want projected runtime config")
	}
	if projected.FactoryDirectory != "/factory" || projected.BaseDirectory != "/runtime" {
		t.Fatalf(
			"projected directories = (%q, %q), want (/factory, /runtime)",
			projected.FactoryDirectory,
			projected.BaseDirectory,
		)
	}
	if len(projected.Workers) != 1 || projected.Workers[0].Name != "worker-a" {
		t.Fatalf("projected workers = %#v, want one worker-a entry", projected.Workers)
	}
	if len(projected.Workers[0].Operations) != 1 || projected.Workers[0].Operations[0].Name != "invoke" {
		t.Fatalf("projected operations = %#v, want invoke operation", projected.Workers[0].Operations)
	}
	if len(projected.Workers[0].Resources) != 1 || projected.Workers[0].Resources[0].ID != "gpu" {
		t.Fatalf("projected worker resources = %#v, want gpu resource", projected.Workers[0].Resources)
	}
	if len(projected.Resources) != 1 || projected.Resources[0].ID != "shared" {
		t.Fatalf("projected resources = %#v, want shared resource", projected.Resources)
	}
}

func TestDirectRuntimeModelWorkerSelectsConfiguredOperation(t *testing.T) {
	t.Parallel()

	config := &factorydefinitions.FactoryConfig{Workers: []factorydefinitions.FactoryWorkerConfig{
		{
			Name: "model-worker", Type: factorydefinitions.WorkerTypeModel, Model: "fixture-model",
			Operations: []factorydefinitions.ModelOperation{{Name: "invoke"}},
		},
		{Name: "agent-worker", Type: factorydefinitions.WorkerTypeAgent, Model: "fixture-model"},
	}}
	worker, operation, err := directRuntimeModelWorker(config, " FIXTURE-MODEL ", " invoke ")
	if err != nil {
		t.Fatalf("directRuntimeModelWorker() error = %v, want nil", err)
	}
	if worker == nil || worker.Name != "model-worker" || operation.Name != "invoke" {
		t.Fatalf("selected worker/operation = (%#v, %#v), want model-worker/invoke", worker, operation)
	}

	tests := []struct {
		name      string
		config    *factorydefinitions.FactoryConfig
		model     string
		operation string
		want      error
		contains  string
	}{
		{name: "missing config", model: "fixture-model", operation: "invoke", contains: "factory config is not available"},
		{name: "empty model", config: config, operation: "invoke", want: models.ErrNotFound},
		{name: "empty operation", config: config, model: "fixture-model", contains: "operation is required"},
		{name: "unknown model", config: config, model: "other-model", operation: "invoke", want: models.ErrNotFound},
		{name: "unsupported operation", config: config, model: "fixture-model", operation: "other", want: models.ErrUnsupportedOperation},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := directRuntimeModelWorker(test.config, test.model, test.operation)
			if err == nil {
				t.Fatal("directRuntimeModelWorker() error = nil, want error")
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want errors.Is(%v)", err, test.want)
			}
			if test.contains != "" && !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("error = %q, want substring %q", err, test.contains)
			}
		})
	}
}

func TestRuntimeModelBindingsResolveInputConfigDefaultAndOmittedSources(t *testing.T) {
	t.Parallel()

	worker := &factorydefinitions.FactoryWorkerConfig{Operations: []factorydefinitions.ModelOperation{{
		Name: "invoke",
		Inputs: []factorydefinitions.ModelOperationSlot{
			{Name: "input"},
			{Name: "config"},
			{Name: "default"},
			{Name: "omitted"},
		},
	}}}
	configPart := work.WorkContentPart{Type: work.WorkContentPartTypeJSON, JSON: json.RawMessage(`{"value":1}`)}
	defaultPart := work.WorkContentPart{Type: work.WorkContentPartTypeText, Text: "fallback"}
	request := models.Request{
		Operation: "invoke",
		Content:   []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Label: "prompt", Text: "hello"}},
		Bindings: []models.ModelOperationBinding{
			{Slot: "input", Selector: &models.ModelOperationBindingSelector{Label: "prompt"}},
			{Slot: "config", Config: []work.WorkContentPart{configPart}},
			{Slot: "default", DefaultContent: []work.WorkContentPart{defaultPart}},
		},
	}
	resolved, err := resolveRuntimeModelBindings(worker, request)
	if err != nil {
		t.Fatalf("resolveRuntimeModelBindings() error = %v, want nil", err)
	}
	if len(resolved) != 4 {
		t.Fatalf("resolved bindings = %#v, want four slots", resolved)
	}
	wantSources := []string{"INPUT", "CONFIG", "DEFAULT", "OMITTED"}
	for index, want := range wantSources {
		if resolved[index].Slot != worker.Operations[0].Inputs[index].Name || resolved[index].Source != want {
			t.Fatalf("resolved[%d] = %#v, want slot/source %q/%q", index, resolved[index], worker.Operations[0].Inputs[index].Name, want)
		}
	}
	if resolved[1].Content[0].JSON[0] != '{' || resolved[2].Content[0].Text != "fallback" {
		t.Fatalf("resolved content = %#v, want detached config/default content", resolved)
	}

	required := &factorydefinitions.FactoryWorkerConfig{Operations: []factorydefinitions.ModelOperation{{
		Name: "invoke", Inputs: []factorydefinitions.ModelOperationSlot{{Name: "required", Required: true}},
	}}}
	if _, err := resolveRuntimeModelBindings(required, models.Request{Operation: "invoke"}); err == nil || !strings.Contains(err.Error(), "required slot") {
		t.Fatalf("required missing binding error = %v, want required-slot error", err)
	}
}

func TestRuntimeModelSelectorAndContentTypes(t *testing.T) {
	t.Parallel()

	part := work.WorkContentPart{
		Type: work.WorkContentPartTypeImage, Slot: "image", Label: "cover", Role: "input",
	}
	tests := []struct {
		name     string
		selector *models.ModelOperationBindingSelector
		want     bool
	}{
		{name: "nil", want: false},
		{name: "empty", selector: &models.ModelOperationBindingSelector{}, want: true},
		{name: "all fields", selector: &models.ModelOperationBindingSelector{Slot: "image", Label: "cover", Type: "IMAGE", Role: "input"}, want: true},
		{name: "slot mismatch", selector: &models.ModelOperationBindingSelector{Slot: "other"}, want: false},
		{name: "label mismatch", selector: &models.ModelOperationBindingSelector{Label: "other"}, want: false},
		{name: "type mismatch", selector: &models.ModelOperationBindingSelector{Type: "TEXT"}, want: false},
		{name: "role mismatch", selector: &models.ModelOperationBindingSelector{Role: "output"}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runtimeModelSelectorMatches(part, test.selector); got != test.want {
				t.Fatalf("runtimeModelSelectorMatches() = %v, want %v", got, test.want)
			}
		})
	}
	if !runtimeModelSelectorEmpty(nil) || !runtimeModelSelectorEmpty(&models.ModelOperationBindingSelector{}) {
		t.Fatal("empty selector was not recognized")
	}
	if runtimeModelSelectorEmpty(&models.ModelOperationBindingSelector{Slot: "slot"}) {
		t.Fatal("non-empty selector was recognized as empty")
	}

	contentTypes := map[work.WorkContentPartType]string{
		work.WorkContentPartTypeText:   factorydefinitions.ModelOperationContentTypeText,
		work.WorkContentPartTypeImage:  factorydefinitions.ModelOperationContentTypeImage,
		work.WorkContentPartTypeAudio:  factorydefinitions.ModelOperationContentTypeAudio,
		work.WorkContentPartTypeJSON:   factorydefinitions.ModelOperationContentTypeJSON,
		work.WorkContentPartTypeBinary: factorydefinitions.ModelOperationContentTypeBinary,
		"custom":                       "custom",
	}
	for kind, want := range contentTypes {
		if got := runtimeModelContentType(work.WorkContentPart{Type: kind}); got != want {
			t.Errorf("runtimeModelContentType(%q) = %q, want %q", kind, got, want)
		}
	}
}

func TestRuntimeModelContentParsesStructuredAndTextResponses(t *testing.T) {
	t.Parallel()

	textOperation := factorydefinitions.ModelOperation{Name: "text", Outputs: []factorydefinitions.ModelOperationSlot{{
		Name: "result", ContentTypes: []string{factorydefinitions.ModelOperationContentTypeText},
	}}}
	structuredOperation := factorydefinitions.ModelOperation{Name: "structured", Outputs: []factorydefinitions.ModelOperationSlot{{
		Name: "result", ContentTypes: []string{factorydefinitions.ModelOperationContentTypeJSON},
	}}}
	tests := []struct {
		name      string
		raw       string
		operation factorydefinitions.ModelOperation
		want      []work.WorkContentPart
		wantErr   bool
	}{
		{name: "empty", operation: textOperation},
		{name: "array", raw: `[{"type":"TEXT","text":"hello"}]`, operation: textOperation, want: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}}},
		{name: "envelope", raw: `{"content":[{"type":"text","text":"hello"}]}`, operation: textOperation, want: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}}},
		{name: "text fallback", raw: "plain response", operation: textOperation, want: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "plain response"}}},
		{name: "invalid structured", raw: "plain response", operation: structuredOperation, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := runtimeModelContent(test.raw, test.operation)
			if (err != nil) != test.wantErr {
				t.Fatalf("runtimeModelContent() error = %v, wantErr %v", err, test.wantErr)
			}
			if test.wantErr {
				return
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("runtimeModelContent() = %#v, want %#v", got, test.want)
			}
		})
	}
	textCases := []struct {
		name string
		op   factorydefinitions.ModelOperation
		want bool
	}{
		{name: "no outputs", want: true},
		{name: "text outputs", op: textOperation, want: true},
		{name: "non text outputs", op: structuredOperation, want: false},
		{name: "untyped output", op: factorydefinitions.ModelOperation{Outputs: []factorydefinitions.ModelOperationSlot{{Name: "result"}}}, want: false},
	}
	for _, test := range textCases {
		if got := runtimeModelOnlyTextOutputs(test.op); got != test.want {
			t.Errorf("runtimeModelOnlyTextOutputs(%s) = %v, want %v", test.name, got, test.want)
		}
	}
}

func TestRuntimeModelStreamAndOutputHelpers(t *testing.T) {
	t.Parallel()
	testRuntimeModelOutputHelpers(t)
	testRuntimeModelStreamHelpers(t)
	testRuntimeModelBindingHelpers(t)
}

func testRuntimeModelOutputHelpers(t *testing.T) {
	t.Helper()
	text := work.WorkContentPart{Type: work.WorkContentPartTypeText, Text: "answer"}
	if got := runtimeModelOutput(workers.ExecuteResult{Output: workers.ProposedOutput{Primary: []work.WorkContentPart{text}}}); got != "answer" {
		t.Fatalf("runtimeModelOutput(text) = %q, want answer", got)
	}
	if got := runtimeModelOutput(workers.ExecuteResult{}); got != "" {
		t.Fatalf("runtimeModelOutput(empty) = %q, want empty", got)
	}
	structured := work.WorkContentPart{Type: work.WorkContentPartTypeJSON, JSON: json.RawMessage(`{"ok":true}`)}
	if got := runtimeModelOutput(workers.ExecuteResult{Output: workers.ProposedOutput{Primary: []work.WorkContentPart{structured}}}); !strings.Contains(got, `"ok"`) {
		t.Fatalf("runtimeModelOutput(structured) = %q, want JSON", got)
	}
	if got := runtimeModelOutput(workers.ExecuteResult{Output: workers.ProposedOutput{Primary: []work.WorkContentPart{{Type: work.WorkContentPartTypeJSON, Metadata: map[string]any{"unsupported": func() {}}}}}}); got != "" {
		t.Fatalf("runtimeModelOutput(unmarshalable) = %q, want empty fallback", got)
	}
}

func testRuntimeModelStreamHelpers(t *testing.T) {
	t.Helper()
	audio := []work.WorkContentPart{{Type: work.WorkContentPartTypeAudio, File: "answer.wav"}}
	file, contentType, err := runtimeModelStream(audio, &models.Options{ResponseMode: models.ResponseModeAudioStream})
	if err != nil || file != "answer.wav" || contentType != "application/octet-stream" {
		t.Fatalf("runtimeModelStream(audio) = (%q, %q, %v), want audio defaults", file, contentType, err)
	}
	if _, _, err := runtimeModelStream(nil, &models.Options{ResponseMode: models.ResponseModeAudioStream}); !errors.Is(err, models.ErrUnsupportedResponseMode) {
		t.Fatalf("runtimeModelStream(no audio) error = %v, want ErrUnsupportedResponseMode", err)
	}
	if file, contentType, err := runtimeModelStream(audio, nil); err != nil || file != "" || contentType != "" {
		t.Fatalf("runtimeModelStream(no options) = (%q, %q, %v), want empty success", file, contentType, err)
	}
	if file, contentType, err := runtimeModelStream([]work.WorkContentPart{{Type: work.WorkContentPartTypeAudio, File: "answer.wav", ContentType: "audio/wav"}}, &models.Options{ResponseMode: models.ResponseModeAudioStream}); err != nil || file != "answer.wav" || contentType != "audio/wav" {
		t.Fatalf("runtimeModelStream(explicit type) = (%q, %q, %v), want audio/wav", file, contentType, err)
	}
}

func testRuntimeModelBindingHelpers(t *testing.T) {
	t.Helper()
	text := work.WorkContentPart{Type: work.WorkContentPartTypeText, Text: "answer"}
	bindings := []models.ResolvedModelOperationBinding{{Slot: "input", Source: "INPUT", Content: []work.WorkContentPart{text}}}
	message := runtimeModelUserMessage(" invoke ", []work.WorkContentPart{text}, bindings)
	var decoded struct {
		Operation string                 `json:"operation"`
		Input     []work.WorkContentPart `json:"input"`
	}
	if err := json.Unmarshal([]byte(message), &decoded); err != nil || decoded.Operation != "invoke" || decoded.Input[0].Text != "answer" {
		t.Fatalf("runtimeModelUserMessage() = %q, decoded = %#v, error = %v", message, decoded, err)
	}
	workerBindings := runtimeWorkerBindings(bindings)
	if len(workerBindings) != 1 || workerBindings[0].Slot != "input" || workerBindings[0].Source != workers.ModelOperationBindingSourceInput {
		t.Fatalf("runtimeWorkerBindings() = %#v, want mapped input binding", workerBindings)
	}
	if runtimeWorkerBindings(nil) != nil || firstRuntimeModelValue("  ", "fallback") != "fallback" || firstRuntimeModelValue("value", "fallback") != "value" {
		t.Fatal("empty binding/value helper behavior was incorrect")
	}
	if got := runtimeModelUserMessage("invoke", []work.WorkContentPart{{Metadata: map[string]any{"unsupported": func() {}}}}, nil); got != "invoke" {
		t.Fatalf("runtimeModelUserMessage(unmarshalable) = %q, want operation fallback", got)
	}
}

func TestRuntimeModelInvokerUsesSharedWorkersForManagedModel(t *testing.T) {
	t.Parallel()

	scope := mustRuntimeModelScope(t, "factory-session:models:local")
	worker := factorydefinitions.FactoryWorkerConfig{
		Name: "local-worker", Type: factorydefinitions.WorkerTypeModel, Model: "local-model", ModelLocality: factorydefinitions.ModelLocalityLocal,
		Resources:  []factorydefinitions.ResourceConfig{{ID: "gpu", Name: "GPU", Type: "device", Capacity: 1}},
		Operations: []factorydefinitions.ModelOperation{{Name: "invoke", Inputs: []factorydefinitions.ModelOperationSlot{{Name: "prompt"}}, Outputs: []factorydefinitions.ModelOperationSlot{{Name: "result", ContentTypes: []string{factorydefinitions.ModelOperationContentTypeText}}}}},
	}
	config := &factorydefinitions.FactoryConfig{Workers: []factorydefinitions.FactoryWorkerConfig{worker}, Resources: []factorydefinitions.ResourceConfig{{ID: "shared", Capacity: 2}}}
	modelsService := &runtimeInvokerModelsStub{
		readiness: models.GetModelReadinessResult{ModelName: "local-model", Readiness: models.Runtime{ReadinessState: models.ReadinessStateReady}},
	}
	workersService := &runtimeInvokerWorkersStub{result: workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeAccepted,
		Output:  workers.ProposedOutput{Primary: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "local answer"}}},
	}}
	sessions := &runtimeInvokerSessionsStub{projection: factorysessions.SessionProjection{Context: factorysessions.ProjectionContext{FactoryCfg: config}}}
	invoker := NewRuntimeModelInvoker(RuntimeModelInvokerConfig{
		Models: modelsService, Scope: scope, Sessions: sessions,
		Workers:   workersService,
		RuntimeID: "runtime-local", GenerationID: "generation-local",
		FactoryDirectory: "/factory", WorkingDirectory: "/factory/work",
	})
	result, err := invoker.InvokeModel(context.Background(), "local-model", models.Request{
		Operation: "invoke",
		Content:   []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Label: "prompt", Text: "hello"}},
		Bindings:  []models.ModelOperationBinding{{Slot: "prompt", Selector: &models.ModelOperationBindingSelector{Label: "prompt"}}},
	})
	assertManagedModelInvocation(t, result, err, scope, modelsService, workersService)
}

func TestRuntimeModelInvokerUsesEffectiveBuiltinWithoutDeclaredWorker(t *testing.T) {
	t.Parallel()

	scope := mustRuntimeModelScope(t, "factory-session:models:effective-builtin")
	modelsService := newEffectiveBuiltinModelsStub(t)
	workersService := effectiveBuiltinWorkersStub()
	invoker := NewRuntimeModelInvoker(RuntimeModelInvokerConfig{
		Models: modelsService, Scope: scope, Sessions: sessionsWithConfig(&factorydefinitions.FactoryConfig{}),
		Workers: workersService,
	})
	result, err := invoker.InvokeModel(context.Background(), models.BuiltInModelNameTTS, effectiveBuiltinRequest())

	assertEffectiveBuiltinInvocation(t, scope, modelsService, workersService, result, err)
}

func newEffectiveBuiltinModelsStub(t *testing.T) *runtimeInvokerModelsStub {
	t.Helper()
	definition, ok := (models.BuiltInCatalog{}).ModelDefinitionFor(models.BuiltInModelNameTTS)
	if !ok {
		t.Fatal("built-in TTS definition is unavailable")
	}
	return &runtimeInvokerModelsStub{
		resolution: models.ResolveModelReferenceResult{
			Resolved: models.ResolvedModelReference{Definition: definition},
		},
		readiness: models.GetModelReadinessResult{
			ModelName: models.BuiltInModelNameTTS,
			Readiness: models.Runtime{ReadinessState: models.ReadinessStateReady},
		},
	}
}

func effectiveBuiltinWorkersStub() *runtimeInvokerWorkersStub {
	return &runtimeInvokerWorkersStub{result: workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeAccepted,
		Output: workers.ProposedOutput{Primary: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeAudio, File: "/tmp/tts.wav", ContentType: "audio/wav",
		}}},
	}}
}

func effectiveBuiltinRequest() models.Request {
	return models.Request{
		Operation: models.OperationTTS,
		Content: []work.WorkContentPart{{
			Slot: "text", Type: work.WorkContentPartTypeText, Text: "hello",
		}},
	}
}

func assertEffectiveBuiltinInvocation(
	t *testing.T,
	scope models.RuntimeScopeRef,
	modelsService *runtimeInvokerModelsStub,
	workersService *runtimeInvokerWorkersStub,
	result models.Result,
	err error,
) {
	t.Helper()
	assertEffectiveBuiltinResult(t, result, err)
	assertEffectiveBuiltinResolution(t, scope, modelsService)
	assertEffectiveBuiltinExecution(t, workersService)
}

func assertEffectiveBuiltinResult(t *testing.T, result models.Result, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("InvokeModel(effective built-in) error = %v, want nil", err)
	}
	if result.ModelName != models.BuiltInModelNameTTS || result.Worker != models.BuiltInModelNameTTS ||
		result.Operation != models.OperationTTS || result.ProviderLocality != models.RuntimeModelLocalityLocal {
		t.Fatalf("effective built-in result = %#v, want tts identity/locality", result)
	}
	if len(result.Content) != 1 || result.Content[0].Type != work.WorkContentPartTypeAudio {
		t.Fatalf("effective built-in content = %#v, want one audio part", result.Content)
	}
}

func assertEffectiveBuiltinResolution(t *testing.T, scope models.RuntimeScopeRef, modelsService *runtimeInvokerModelsStub) {
	t.Helper()
	if len(modelsService.resolutionRequests) != 1 {
		t.Fatalf("resolution requests = %#v, want one effective-definition lookup", modelsService.resolutionRequests)
	}
	resolution := modelsService.resolutionRequests[0]
	if resolution.Scope != scope || resolution.Reference.NameOrURI != models.BuiltInModelNameTTS {
		t.Fatalf("resolution request = %#v, want opened scope and tts name", resolution)
	}
	if len(modelsService.readinessRequests) != 1 || modelsService.readinessRequests[0].Name != models.BuiltInModelNameTTS ||
		modelsService.readinessRequests[0].Operation != models.OperationTTS {
		t.Fatalf("readiness requests = %#v, want effective tts operation", modelsService.readinessRequests)
	}
}

func assertEffectiveBuiltinExecution(t *testing.T, workersService *runtimeInvokerWorkersStub) {
	t.Helper()
	if len(workersService.requests) != 1 {
		t.Fatalf("Workers Execute requests = %d, want one", len(workersService.requests))
	}
	execute := workersService.requests[0]
	if execute.Target.WorkerName != models.BuiltInModelNameTTS || execute.Target.WorkerType != factorydefinitions.WorkerTypeInference ||
		execute.Target.Model.Name != models.BuiltInModelNameTTS || execute.Target.Model.Locality != models.RuntimeModelLocalityLocal {
		t.Fatalf("effective built-in target = %#v, want synthetic inference target", execute.Target)
	}
	if execute.Input.ModelRuntime == nil || execute.Input.ModelRuntime.Worker.Model != models.BuiltInModelNameTTS {
		t.Fatalf("effective built-in runtime input = %#v, want managed model projection", execute.Input.ModelRuntime)
	}
	if len(execute.Input.ModelBindings) != 3 || execute.Input.ModelBindings[0].Slot != "text" ||
		execute.Input.ModelBindings[0].Source != workers.ModelOperationBindingSourceInput ||
		execute.Input.ModelBindings[1].Source != workers.ModelOperationBindingSourceOmitted {
		t.Fatalf("effective built-in bindings = %#v, want projected TTS slots", execute.Input.ModelBindings)
	}
}

func TestRuntimeModelInvokerPreservesEffectiveBuiltinUnsupportedOperation(t *testing.T) {
	t.Parallel()

	scope := mustRuntimeModelScope(t, "factory-session:models:effective-unsupported")
	definition, ok := (models.BuiltInCatalog{}).ModelDefinitionFor(models.BuiltInModelNameTTS)
	if !ok {
		t.Fatal("built-in TTS definition is unavailable")
	}
	modelsService := &runtimeInvokerModelsStub{resolution: models.ResolveModelReferenceResult{
		Resolved: models.ResolvedModelReference{Definition: definition},
	}}
	invoker := NewRuntimeModelInvoker(RuntimeModelInvokerConfig{
		Models: modelsService, Scope: scope, Sessions: sessionsWithConfig(&factorydefinitions.FactoryConfig{}),
		Workers: &runtimeInvokerWorkersStub{},
	})
	_, err := invoker.InvokeModel(context.Background(), models.BuiltInModelNameTTS, models.Request{Operation: models.OperationASR})
	if err == nil || !errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("unsupported effective operation error = %v, want ErrUnsupportedOperation", err)
	}
	if len(modelsService.resolutionRequests) != 1 || len(modelsService.readinessRequests) != 0 {
		t.Fatalf("effective unsupported lookup state = resolutions=%d readiness=%d, want resolution only", len(modelsService.resolutionRequests), len(modelsService.readinessRequests))
	}
}

func assertManagedModelInvocation(
	t *testing.T,
	result models.Result,
	err error,
	scope models.RuntimeScopeRef,
	modelsService *runtimeInvokerModelsStub,
	workersService *runtimeInvokerWorkersStub,
) {
	t.Helper()
	if err != nil {
		t.Fatalf("InvokeModel(local) error = %v, want nil", err)
	}
	if result.ModelName != "local-model" || result.Worker != "local-worker" || result.Content[0].Text != "local answer" {
		t.Fatalf("InvokeModel(local) result = %#v, want local projection", result)
	}
	if len(modelsService.readinessRequests) != 1 || modelsService.readinessRequests[0].Scope != scope {
		t.Fatalf("readiness requests = %#v, want opened runtime scope", modelsService.readinessRequests)
	}
	if len(modelsService.resolutionRequests) != 0 {
		t.Fatalf("effective-definition resolution requests = %#v, want declared worker precedence", modelsService.resolutionRequests)
	}
	if len(workersService.requests) != 1 {
		t.Fatalf("Workers Execute requests = %d, want one", len(workersService.requests))
	}
	execute := workersService.requests[0]
	assertManagedModelTarget(t, execute)
	assertManagedModelInput(t, execute)
	runtime := execute.Input.ModelRuntime
	if runtime.Scope != scope || runtime.Worker.Name != "local-worker" ||
		len(runtime.Resources) != 1 || runtime.Resources[0].ID != "shared" {
		t.Fatalf("managed Models request projection = %#v, want opened scope and shared resource", runtime)
	}
}

func assertManagedModelTarget(t *testing.T, execute workers.ExecuteRequest) {
	t.Helper()
	if execute.Target.WorkerName != "local-worker" ||
		execute.Target.RunnerID != workers.RunnerIDCodex ||
		execute.Target.Provider.ID != workers.RunnerIDCodex ||
		execute.Target.Model.Name != "local-model" ||
		execute.Target.Model.Locality != factorydefinitions.ModelLocalityLocal {
		t.Fatalf("managed execution target = %#v, want private inference target", execute.Target)
	}
}

func assertManagedModelInput(t *testing.T, execute workers.ExecuteRequest) {
	t.Helper()
	if execute.Input.ModelOperation != "invoke" || len(execute.Input.ModelBindings) != 1 ||
		execute.Input.ModelBindings[0].Source != workers.ModelOperationBindingSourceInput ||
		execute.Input.ModelRuntime == nil || execute.Input.ModelRuntime.Scope.IsZero() ||
		execute.Input.WorkflowContext == nil {
		t.Fatalf("managed execution input = %#v, want detached model metadata", execute.Input)
	}
}

func TestRuntimeModelInvokerUsesSharedWorkersForRemoteModel(t *testing.T) {
	t.Parallel()

	scope := mustRuntimeModelScope(t, "factory-session:models:remote")
	config := &factorydefinitions.FactoryConfig{Workers: []factorydefinitions.FactoryWorkerConfig{{
		Name: "remote-worker", Type: factorydefinitions.WorkerTypeModel, Model: "remote-model", ModelProvider: "mock", ModelLocality: factorydefinitions.ModelLocalityCloud, Body: "system",
		Operations: []factorydefinitions.ModelOperation{{Name: "invoke", Outputs: []factorydefinitions.ModelOperationSlot{{Name: "result", ContentTypes: []string{factorydefinitions.ModelOperationContentTypeText}}}}},
	}}}
	workersService := &runtimeInvokerWorkersStub{result: workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeAccepted,
		Output:  workers.ProposedOutput{Primary: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "remote answer"}}},
	}}
	invoker := NewRuntimeModelInvoker(RuntimeModelInvokerConfig{
		Models: &runtimeInvokerModelsStub{readiness: models.GetModelReadinessResult{Readiness: models.Runtime{ReadinessState: models.ReadinessStateReady}}},
		Scope:  scope, Sessions: &runtimeInvokerSessionsStub{projection: factorysessions.SessionProjection{Context: factorysessions.ProjectionContext{FactoryCfg: config}}},
		Workers: workersService, RuntimeID: "runtime-remote", GenerationID: "generation-remote", FactoryDirectory: "/factory", WorkingDirectory: "/factory/work",
	})
	result, err := invoker.InvokeModel(context.Background(), "remote-model", models.Request{Operation: "invoke", Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}}})
	if err != nil {
		t.Fatalf("InvokeModel(remote) error = %v, want nil", err)
	}
	if result.Content[0].Text != "remote answer" || result.ProviderLocality != "CLOUD" {
		t.Fatalf("InvokeModel(remote) result = %#v, want remote answer/CLOUD", result)
	}
	if len(workersService.requests) != 1 {
		t.Fatalf("Workers Execute requests = %d, want one", len(workersService.requests))
	}
	execute := workersService.requests[0]
	if execute.Correlation.RuntimeID != "runtime-remote" || execute.Correlation.GenerationID != "generation-remote" || execute.Correlation.DispatchID != directModelInvocationDispatchID {
		t.Fatalf("execution correlation = %#v, want runtime/generation/dispatch", execute.Correlation)
	}
	if execute.Target.WorkerName != "remote-worker" || execute.Target.Provider.ID != "mock" || execute.Target.RunnerID != workers.RunnerIDCodex || execute.Target.Prompt.UserMessage == "" {
		t.Fatalf("execution target = %#v, want shared Workers target", execute.Target)
	}
	if len(execute.Input.Work) != 0 || execute.Input.ModelOperation != "invoke" || execute.Input.WorkflowContext == nil {
		t.Fatalf("execution input = %#v, want direct model metadata without Work capability", execute.Input)
	}
}

func TestRuntimeModelInvokerRejectsUnhandledAndFailedExecution(t *testing.T) {
	t.Parallel()

	scope := mustRuntimeModelScope(t, "factory-session:models:errors")
	config := &factorydefinitions.FactoryConfig{Workers: []factorydefinitions.FactoryWorkerConfig{{
		Name: "local-worker", Type: factorydefinitions.WorkerTypeModel, Model: "local-model", ModelLocality: factorydefinitions.ModelLocalityLocal,
		Operations: []factorydefinitions.ModelOperation{{Name: "invoke"}},
	}}}
	baseModels := &runtimeInvokerModelsStub{readiness: models.GetModelReadinessResult{Readiness: models.Runtime{ReadinessState: models.ReadinessStateReady}}}
	localWorkers := &runtimeInvokerWorkersStub{result: workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeFailed,
		Failure: &workers.ExecutionFailure{Message: "local inference failed"},
	}}
	localInvoker := NewRuntimeModelInvoker(RuntimeModelInvokerConfig{
		Models: baseModels, Scope: scope,
		Sessions: &runtimeInvokerSessionsStub{projection: factorysessions.SessionProjection{Context: factorysessions.ProjectionContext{FactoryCfg: config}}},
		Workers:  localWorkers,
	})
	if _, err := localInvoker.InvokeModel(context.Background(), "local-model", models.Request{Operation: "invoke"}); err == nil || !strings.Contains(err.Error(), "local inference failed") {
		t.Fatalf("failed local execution error = %v, want Workers failure", err)
	}

	remoteConfig := &factorydefinitions.FactoryConfig{Workers: []factorydefinitions.FactoryWorkerConfig{{
		Name: "remote-worker", Type: factorydefinitions.WorkerTypeModel, Model: "remote-model", ModelLocality: factorydefinitions.ModelLocalityCloud, ModelProvider: "mock",
		Operations: []factorydefinitions.ModelOperation{{Name: "invoke"}},
	}}}
	failedWorkers := &runtimeInvokerWorkersStub{result: workers.ExecuteResult{Outcome: workers.ExecutionOutcomeFailed, Failure: &workers.ExecutionFailure{Message: "provider failed"}}}
	remoteInvoker := NewRuntimeModelInvoker(RuntimeModelInvokerConfig{
		Models: baseModels, Scope: scope, Sessions: &runtimeInvokerSessionsStub{projection: factorysessions.SessionProjection{Context: factorysessions.ProjectionContext{FactoryCfg: remoteConfig}}}, Workers: failedWorkers,
	})
	if _, err := remoteInvoker.InvokeModel(context.Background(), "remote-model", models.Request{Operation: "invoke"}); err == nil || !strings.Contains(err.Error(), "provider failed") {
		t.Fatalf("failed remote invocation error = %v, want provider failure", err)
	}
}

func TestRuntimeModelInvokerPreservesWorkersCancellation(t *testing.T) {
	t.Parallel()

	scope := mustRuntimeModelScope(t, "factory-session:models:canceled")
	config := &factorydefinitions.FactoryConfig{Workers: []factorydefinitions.FactoryWorkerConfig{{
		Name: "local-worker", Type: factorydefinitions.WorkerTypeModel,
		Model: "local-model", ModelLocality: factorydefinitions.ModelLocalityLocal,
		Operations: []factorydefinitions.ModelOperation{{Name: "invoke"}},
	}}}
	invoker := NewRuntimeModelInvoker(RuntimeModelInvokerConfig{
		Models: &runtimeInvokerModelsStub{
			readiness: models.GetModelReadinessResult{Readiness: models.Runtime{ReadinessState: models.ReadinessStateReady}},
		},
		Scope:    scope,
		Sessions: sessionsWithConfig(config),
		Workers:  &runtimeInvokerWorkersStub{err: context.Canceled},
	})

	if _, err := invoker.InvokeModel(context.Background(), "local-model", models.Request{Operation: "invoke"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Workers execution error = %v, want context.Canceled", err)
	}
}

func TestRuntimeModelInvokerRejectsUnavailableOpenedCapabilities(t *testing.T) {
	t.Parallel()

	scope := mustRuntimeModelScope(t, "factory-session:models:invalid")
	localConfig := &factorydefinitions.FactoryConfig{Workers: []factorydefinitions.FactoryWorkerConfig{{
		Name: "local-worker", Type: factorydefinitions.WorkerTypeModel, Model: "local-model", ModelLocality: factorydefinitions.ModelLocalityLocal,
		Operations: []factorydefinitions.ModelOperation{{Name: "invoke"}},
	}}}
	sessions := &runtimeInvokerSessionsStub{projection: factorysessions.SessionProjection{Context: factorysessions.ProjectionContext{FactoryCfg: localConfig}}}
	readyModels := &runtimeInvokerModelsStub{readiness: models.GetModelReadinessResult{Readiness: models.Runtime{ReadinessState: models.ReadinessStateReady}}}
	request := models.Request{Operation: "invoke"}
	assertRuntimeModelInvokerBaseUnavailable(t, readyModels, sessions, request)
	assertRuntimeModelInvokerLookupFailures(t, scope, localConfig, sessions, readyModels, request)
	assertRuntimeModelInvokerLocalFailures(t, scope, sessions, readyModels, request)
	assertRuntimeModelInvokerRemoteFailures(t, scope, readyModels, request)
}

func assertRuntimeModelInvokerBaseUnavailable(
	t *testing.T,
	readyModels *runtimeInvokerModelsStub,
	sessions *runtimeInvokerSessionsStub,
	request models.Request,
) {
	t.Helper()
	var nilInvoker *runtimeModelInvoker
	if _, err := nilInvoker.InvokeModel(context.Background(), "local-model", request); err == nil || !strings.Contains(err.Error(), "Models service is not available") {
		t.Fatalf("nil invoker error = %v, want Models unavailable", err)
	}
	if _, err := (&runtimeModelInvoker{}).InvokeModel(context.Background(), "local-model", request); err == nil || !strings.Contains(err.Error(), "Models service is not available") {
		t.Fatalf("nil Models error = %v, want Models unavailable", err)
	}
	if _, err := NewRuntimeModelInvoker(RuntimeModelInvokerConfig{Models: readyModels}).InvokeModel(context.Background(), "local-model", request); err == nil || !strings.Contains(err.Error(), "Factory Session service is not available") {
		t.Fatalf("nil Sessions error = %v, want Factory Session unavailable", err)
	}
	if _, err := NewRuntimeModelInvoker(RuntimeModelInvokerConfig{Models: readyModels, Sessions: sessions}).InvokeModel(context.Background(), "local-model", request); !errors.Is(err, models.ErrRuntimeScopeInvalid) {
		t.Fatalf("zero scope error = %v, want ErrRuntimeScopeInvalid", err)
	}
}

func assertRuntimeModelInvokerLookupFailures(
	t *testing.T,
	scope models.RuntimeScopeRef,
	localConfig *factorydefinitions.FactoryConfig,
	sessions *runtimeInvokerSessionsStub,
	readyModels *runtimeInvokerModelsStub,
	request models.Request,
) {
	t.Helper()
	sessions.err = errors.New("session lookup failed")
	if _, err := NewRuntimeModelInvoker(RuntimeModelInvokerConfig{Models: readyModels, Scope: scope, Sessions: sessions}).InvokeModel(context.Background(), "local-model", request); err == nil || !strings.Contains(err.Error(), "session lookup failed") {
		t.Fatalf("session lookup error = %v, want lookup failure", err)
	}
	sessions.err = nil

	configWithoutModel := &factorydefinitions.FactoryConfig{}
	missingWorkerSessions := &runtimeInvokerSessionsStub{projection: factorysessions.SessionProjection{Context: factorysessions.ProjectionContext{FactoryCfg: configWithoutModel}}}
	missingWorkerErr := func() error {
		_, err := NewRuntimeModelInvoker(RuntimeModelInvokerConfig{Models: readyModels, Scope: scope, Sessions: missingWorkerSessions}).InvokeModel(context.Background(), "missing-model", request)
		return err
	}()
	var missingModelFailure *models.InvocationFailure
	if missingWorkerErr == nil || !errors.As(missingWorkerErr, &missingModelFailure) || missingModelFailure.Class != models.InvocationFailureClassInvalidModelReference {
		t.Fatalf("missing worker error = %v, want invalid-model-reference classification", missingWorkerErr)
	}

	readinessErrorModels := &runtimeInvokerModelsStub{readinessErr: errors.New("readiness lookup failed")}
	if _, err := NewRuntimeModelInvoker(RuntimeModelInvokerConfig{Models: readinessErrorModels, Scope: scope, Sessions: sessionsWithConfig(localConfig)}).InvokeModel(context.Background(), "local-model", request); err == nil || !strings.Contains(err.Error(), "readiness lookup failed") {
		t.Fatalf("readiness lookup error = %v, want lookup failure", err)
	}
	blockedModels := &runtimeInvokerModelsStub{readiness: models.GetModelReadinessResult{Readiness: models.Runtime{ReadinessState: models.ReadinessStateMissing}}}
	if _, err := NewRuntimeModelInvoker(RuntimeModelInvokerConfig{Models: blockedModels, Scope: scope, Sessions: sessionsWithConfig(localConfig)}).InvokeModel(context.Background(), "local-model", request); !errors.Is(err, models.ErrMissing) {
		t.Fatalf("blocked readiness error = %v, want ErrMissing", err)
	}

	requiredConfig := &factorydefinitions.FactoryConfig{Workers: []factorydefinitions.FactoryWorkerConfig{{
		Name: "local-worker", Type: factorydefinitions.WorkerTypeModel, Model: "local-model", ModelLocality: factorydefinitions.ModelLocalityLocal,
		Operations: []factorydefinitions.ModelOperation{{Name: "invoke", Inputs: []factorydefinitions.ModelOperationSlot{{Name: "required", Required: true}}}},
	}}}
	if _, err := NewRuntimeModelInvoker(RuntimeModelInvokerConfig{Models: readyModels, Scope: scope, Sessions: sessionsWithConfig(requiredConfig)}).InvokeModel(context.Background(), "local-model", request); err == nil || !strings.Contains(err.Error(), "required slot") {
		t.Fatalf("required binding error = %v, want required-slot failure", err)
	}
}

func assertRuntimeModelInvokerLocalFailures(
	t *testing.T,
	scope models.RuntimeScopeRef,
	sessions *runtimeInvokerSessionsStub,
	readyModels *runtimeInvokerModelsStub,
	request models.Request,
) {
	t.Helper()
	localErrorWorkers := &runtimeInvokerWorkersStub{err: errors.New("local Workers execution failed")}
	if _, err := NewRuntimeModelInvoker(RuntimeModelInvokerConfig{
		Models: readyModels, Scope: scope, Sessions: sessions, Workers: localErrorWorkers,
	}).InvokeModel(context.Background(), "local-model", request); err == nil || !strings.Contains(err.Error(), "local Workers execution failed") {
		t.Fatalf("local Workers execution error = %v, want local failure", err)
	}
}

func assertRuntimeModelInvokerRemoteFailures(
	t *testing.T,
	scope models.RuntimeScopeRef,
	readyModels *runtimeInvokerModelsStub,
	request models.Request,
) {
	t.Helper()
	remoteConfig := &factorydefinitions.FactoryConfig{Workers: []factorydefinitions.FactoryWorkerConfig{{
		Name: "remote-worker", Type: factorydefinitions.WorkerTypeModel, Model: "remote-model", ModelLocality: factorydefinitions.ModelLocalityCloud,
		Operations: []factorydefinitions.ModelOperation{{Name: "invoke", Outputs: []factorydefinitions.ModelOperationSlot{{Name: "result", ContentTypes: []string{factorydefinitions.ModelOperationContentTypeJSON}}}}},
	}}}
	remoteSessions := sessionsWithConfig(remoteConfig)
	if _, err := NewRuntimeModelInvoker(RuntimeModelInvokerConfig{Models: readyModels, Scope: scope, Sessions: remoteSessions}).InvokeModel(context.Background(), "remote-model", request); err == nil || !strings.Contains(err.Error(), "Workers service is not available") {
		t.Fatalf("nil Workers error = %v, want Workers unavailable", err)
	}
	workersError := &runtimeInvokerWorkersStub{err: errors.New("Workers execution failed")}
	if _, err := NewRuntimeModelInvoker(RuntimeModelInvokerConfig{Models: readyModels, Scope: scope, Sessions: remoteSessions, Workers: workersError}).InvokeModel(context.Background(), "remote-model", request); err == nil || !strings.Contains(err.Error(), "Workers execution failed") {
		t.Fatalf("Workers execution error = %v, want execution failure", err)
	}
	invalidOutput := &runtimeInvokerWorkersStub{result: workers.ExecuteResult{Outcome: workers.ExecutionOutcomeAccepted, Output: workers.ProposedOutput{Primary: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "not JSON"}}}}}
	if _, err := NewRuntimeModelInvoker(RuntimeModelInvokerConfig{Models: readyModels, Scope: scope, Sessions: remoteSessions, Workers: invalidOutput}).InvokeModel(context.Background(), "remote-model", request); err == nil || !strings.Contains(err.Error(), "not valid WorkContent JSON") {
		t.Fatalf("invalid structured output error = %v, want content validation failure", err)
	}
}

func sessionsWithConfig(config *factorydefinitions.FactoryConfig) *runtimeInvokerSessionsStub {
	return &runtimeInvokerSessionsStub{projection: factorysessions.SessionProjection{Context: factorysessions.ProjectionContext{FactoryCfg: config}}}
}

type runtimeInvokerModelsStub struct {
	models.Service
	readiness          models.GetModelReadinessResult
	readinessErr       error
	readinessRequests  []models.GetModelReadinessRequest
	resolution         models.ResolveModelReferenceResult
	resolutionErr      error
	resolutionRequests []models.ResolveModelReferenceRequest
}

func (stub *runtimeInvokerModelsStub) GetModelReadiness(_ context.Context, request models.GetModelReadinessRequest) (models.GetModelReadinessResult, error) {
	stub.readinessRequests = append(stub.readinessRequests, request)
	return stub.readiness, stub.readinessErr
}

func (stub *runtimeInvokerModelsStub) ResolveModelReference(_ context.Context, request models.ResolveModelReferenceRequest) (models.ResolveModelReferenceResult, error) {
	stub.resolutionRequests = append(stub.resolutionRequests, request)
	return stub.resolution, stub.resolutionErr
}

type runtimeInvokerSessionsStub struct {
	factorysessions.Service
	projection factorysessions.SessionProjection
	err        error
}

func (stub *runtimeInvokerSessionsStub) GetFactorySession(context.Context, string) (factorysessions.SessionProjection, error) {
	return stub.projection, stub.err
}

type runtimeInvokerWorkersStub struct {
	workers.Service
	requests []workers.ExecuteRequest
	result   workers.ExecuteResult
	err      error
}

func (stub *runtimeInvokerWorkersStub) Execute(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
	stub.requests = append(stub.requests, request)
	return stub.result, stub.err
}

func mustRuntimeModelScope(t *testing.T, value string) models.RuntimeScopeRef {
	t.Helper()
	scope, err := (models.RuntimeScopeRef{}).Parse(value)
	if err != nil {
		t.Fatalf("parse runtime Models scope %q: %v", value, err)
	}
	return scope
}
