package definitions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// TestDefinitionsExecutionCatalogResolvesDetachedPolicyAtPublicRoot exercises
// the public Definitions composition boundary. ResolveExecutionCatalog is a
// Definitions operation without a Process.Execute transport representation, so
// this test uses the public service root directly and asserts its observable
// policy and diagnostic contract.
func TestDefinitionsExecutionCatalogResolvesDetachedPolicyAtPublicRoot(t *testing.T) {
	t.Parallel()

	service := newFunctionalDefinitionsService(t)
	request := functionalExecutionCatalogRequest()
	result, err := service.ResolveExecutionCatalog(t.Context(), request)
	if err != nil {
		t.Fatalf("ResolveExecutionCatalog() error = %v; result=%#v", err, result)
	}
	assertFunctionalExecutionCatalog(t, result)

	cloned := result.Clone()
	cloned.Workers["worker"].Args[0] = "mutated"
	cloned.Workstations["run"].Environment["GREETING"] = "mutated"
	cloned.Workstations["run"].OperationBindings[0].Config[0].Text = "mutated"
	if result.Workers["worker"].Args[0] == "mutated" ||
		result.Workstations["run"].Environment["GREETING"] == "mutated" ||
		result.Workstations["run"].OperationBindings[0].Config[0].Text == "mutated" {
		t.Fatal("ResolveExecutionCatalog result was not detached from its clone")
	}

	t.Run("nil definition returns typed diagnostics", func(t *testing.T) {
		result, err := service.ResolveExecutionCatalog(t.Context(), factorydefinitions.ResolveExecutionCatalogRequest{})
		var catalogErr *factorydefinitions.ExecutionCatalogError
		if !errors.As(err, &catalogErr) {
			t.Fatalf("error = %T %v, want *ExecutionCatalogError", err, err)
		}
		if len(result.Diagnostics) != 1 ||
			result.Diagnostics[0].Code != factorydefinitions.ExecutionCatalogDiagnosticInvalidDefinition {
			t.Fatalf("diagnostics = %#v, want invalid-definition diagnostic", result.Diagnostics)
		}
	})

	t.Run("references are reported together", func(t *testing.T) {
		request := functionalExecutionCatalogRequest()
		request.EffectiveDefinition.Runner = "missing-runner"
		request.EffectiveDefinition.Workers[0].Provider = "missing-provider"
		request.EffectiveDefinition.Workers[0].Model = "missing-model"
		request.EffectiveDefinition.Workstations[0].WorkerTypeName = "missing-worker"
		request.EffectiveDefinition.Workstations[0].Runner = "missing-runner"

		result, err := service.ResolveExecutionCatalog(t.Context(), request)
		assertExecutionCatalogDiagnostics(t, err, result, map[factorydefinitions.ExecutionCatalogDiagnosticCode]bool{
			factorydefinitions.ExecutionCatalogDiagnosticUnknownRunner:   false,
			factorydefinitions.ExecutionCatalogDiagnosticUnknownProvider: false,
			factorydefinitions.ExecutionCatalogDiagnosticUnknownModel:    false,
			factorydefinitions.ExecutionCatalogDiagnosticUnknownWorker:   false,
		})
	})

	t.Run("mixed interpolation reports a safe diagnostic", func(t *testing.T) {
		request := functionalExecutionCatalogRequest()
		delete(request.Invocation.Arguments.Arguments, "prompt")

		result, err := service.ResolveExecutionCatalog(t.Context(), request)
		assertExecutionCatalogDiagnostics(t, err, result, map[factorydefinitions.ExecutionCatalogDiagnosticCode]bool{
			factorydefinitions.ExecutionCatalogDiagnosticInvalidInterpolation: false,
		})
	})

	t.Run("invalid worker timeout reports a safe diagnostic", func(t *testing.T) {
		request := functionalExecutionCatalogRequest()
		request.EffectiveDefinition.Workers[0].Timeout = "not-a-duration"

		result, err := service.ResolveExecutionCatalog(t.Context(), request)
		assertExecutionCatalogDiagnostics(t, err, result, map[factorydefinitions.ExecutionCatalogDiagnosticCode]bool{
			factorydefinitions.ExecutionCatalogDiagnosticInvalidTimeout: false,
		})
	})
}

func assertFunctionalExecutionCatalog(
	t *testing.T,
	result factorydefinitions.ResolveExecutionCatalogResult,
) {
	t.Helper()
	worker := result.Workers["worker"]
	workstation := result.Workstations["run"]
	logical := result.Workstations["logical"]
	checks := []struct {
		name string
		ok   bool
	}{
		{"definition version", result.DefinitionVersion == "7@2026-08-12T10:11:12Z"},
		{"worker model", worker.Model == "model-a"},
		{"worker provider", worker.ModelProvider == "provider-a"},
		{"worker args", worker.Args[0] == "--prompt=hello" && worker.Args[1] == "from-file"},
		{"worker timeout", worker.Timeout == 2*time.Second},
		{"worker stop token", worker.StopToken == "<STOP>"},
		{"worker policy", worker.AgentToolPolicy == "READ_ONLY"},
		{"worker operation", worker.Operations[0].Inputs[0].ContentTypes[0] == "TEXT"},
		{"worker resource", worker.Resources[0].Provider == "provider-a"},
		{"workstation runner", workstation.Runner == "codex" && workstation.RunnerSelectionSource == "workstation"},
		{"workstation prompt", workstation.PromptTemplate == "run hello"},
		{"workstation environment", workstation.Environment["GREETING"] == "hello hello"},
		{"workstation timeout", workstation.Timeout == 2*time.Second},
		{"workstation propagation", workstation.WorkPropagation == factorydefinitions.WorkPropagationModePreserveInput},
		{"workstation envelope", workstation.DecisionEnvelope && workstation.GoalRoutingDecisionEnvelope},
		{"workstation limit argument", workstation.Limits.MaxGeneratedWorkItemsArgument == "max-items"},
		{"binding selector", workstation.OperationBindings[0].Selector.Label == "accepted"},
		{"binding config", workstation.OperationBindings[0].Config[0].Text == "config hello"},
		{"binding JSON", workstation.OperationBindings[0].DefaultContent[0].JSON[0] == '{'},
		{"input guard", workstation.Inputs[0].Guard.MatchInput == "accepted"},
		{"classification route", workstation.ClassificationRoutes[0].Label == "accepted"},
		{"expected artifact", workstation.ExpectedArtifacts[0].Pattern == "artifacts/hello.txt"},
		{"workstation resource", workstation.Resources[0].Model == "model-a"},
		{"workstation guard", workstation.Guards[0].MaxVisitsArgument == "max-visits"},
		{"logical runner", logical.Runner == "codex" && logical.RunnerSelectionSource == "factory"},
		{"diagnostics", len(result.Diagnostics) == 0},
	}
	for _, check := range checks {
		if !check.ok {
			t.Errorf("resolved catalog %s check failed: %#v", check.name, result)
		}
	}
}

func assertExecutionCatalogDiagnostics(
	t *testing.T,
	err error,
	result factorydefinitions.ResolveExecutionCatalogResult,
	want map[factorydefinitions.ExecutionCatalogDiagnosticCode]bool,
) {
	t.Helper()
	if err == nil {
		t.Fatal("ResolveExecutionCatalog() error = nil, want typed diagnostics")
	}
	var catalogErr *factorydefinitions.ExecutionCatalogError
	if !errors.As(err, &catalogErr) {
		t.Fatalf("error = %T %v, want *ExecutionCatalogError", err, err)
	}
	if len(result.Diagnostics) == 0 || len(result.Diagnostics) != len(catalogErr.Diagnostics) {
		t.Fatalf("result diagnostics = %#v, error diagnostics = %#v", result.Diagnostics, catalogErr.Diagnostics)
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Message == "" || strings.Contains(diagnostic.Message, "missing-provider") {
			t.Fatalf("diagnostic contains unsafe or empty message: %#v", diagnostic)
		}
		if _, ok := want[diagnostic.Code]; ok {
			want[diagnostic.Code] = true
		}
	}
	for code, found := range want {
		if !found {
			t.Fatalf("diagnostic code %q missing from %#v", code, result.Diagnostics)
		}
	}
}

func functionalExecutionCatalogRequest() factorydefinitions.ResolveExecutionCatalogRequest {
	return factorydefinitions.ResolveExecutionCatalogRequest{
		EffectiveDefinition: functionalExecutionCatalogDefinition(),
		Invocation:          functionalExecutionCatalogInvocation(),
		References:          functionalExecutionCatalogReferences(),
	}
}

func functionalExecutionCatalogDefinition() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{
		Name:   "functional-detached-factory",
		Runner: "codex",
		Version: &factorydefinitions.FactoryVersion{
			Logical:  7,
			Physical: time.Date(2026, 8, 12, 10, 11, 12, 0, time.UTC),
		},
		Guards: []factorydefinitions.FactoryGuardConfig{{
			Type:          factorydefinitions.GuardTypeInferenceThrottle,
			ModelProvider: "provider-a",
			Model:         "model-a",
		}},
		Workers: []factorydefinitions.FactoryWorkerConfig{functionalExecutionCatalogWorker()},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			functionalExecutionCatalogRunWorkstation(),
			functionalExecutionCatalogLogicalWorkstation(),
		},
	}
}

func functionalExecutionCatalogWorker() factorydefinitions.FactoryWorkerConfig {
	return factorydefinitions.FactoryWorkerConfig{
		ID:                          "worker-id",
		Name:                        "worker",
		Type:                        factorydefinitions.WorkerTypeInference,
		Provider:                    "${provider}",
		Model:                       "${model}",
		ModelProvider:               "${provider}",
		ReasoningEffort:             "${effort}",
		ModelLocality:               "${locality}",
		ExecutorProvider:            "${executor}",
		Command:                     "${command}",
		Args:                        []string{"--prompt=${prompt}", "${file}"},
		Timeout:                     "${timeout}",
		StopToken:                   "${stop}",
		Body:                        "worker body ${prompt}",
		PromptSourcePath:            "workers/worker/AGENTS.md",
		SessionID:                   "session-id",
		RuntimeDefaultModelProvider: "fallback-provider",
		RuntimeDefaultModel:         "fallback-model",
		SkipPermissions:             true,
		AgentTools:                  &factorydefinitions.AgentToolsConfig{Policy: "READ_ONLY"},
		Operations:                  []factorydefinitions.ModelOperation{functionalExecutionCatalogWorkerOperation()},
		Resources: []factorydefinitions.ResourceConfig{{
			ID: "worker-resource", Name: "worker-model", Type: factorydefinitions.ResourceTypeModel,
			Capacity: 2, Model: "model-a", Backend: "local", LoadPolicy: "shared", Provider: "provider-a",
		}},
	}
}

func functionalExecutionCatalogWorkerOperation() factorydefinitions.ModelOperation {
	return factorydefinitions.ModelOperation{
		Name: "answer",
		Inputs: []factorydefinitions.ModelOperationSlot{{
			Name: "prompt", ContentTypes: []string{"TEXT"}, Required: true,
		}},
		Outputs: []factorydefinitions.ModelOperationSlot{{
			Name: "result", ContentTypes: []string{"JSON"},
		}},
	}
}

func functionalExecutionCatalogRunWorkstation() factorydefinitions.FactoryWorkstationConfig {
	return factorydefinitions.FactoryWorkstationConfig{
		ID: "workstation-id", Name: "run", Type: factorydefinitions.WorkstationTypeModel,
		WorkerTypeName: "worker", Runner: "${runner}", PromptFile: "${promptFile}",
		OutputSchema: "${schema}", OutputContract: "${contract}",
		PromptTemplate: "run ${prompt}", Body: "workstation body ${prompt}",
		WorkingDirectory: "${directory}", Worktree: "${worktree}", Timeout: "${timeout}",
		OutcomeFormat:   factorydefinitions.DecisionEnvelopeOutcomeFormat,
		WorkPropagation: &factorydefinitions.WorkPropagationConfig{Mode: factorydefinitions.WorkPropagationModePreserveInput},
		Limits: factorydefinitions.WorkstationLimits{
			MaxRetries: 2, MaxExecutionTime: "${timeout}", MaxGeneratedWorkItems: 4,
			MaxGeneratedWorkItemsArgument: "${maxItems}", MaxGeneratedWorkItemsArgumentOffset: 3,
		},
		Env:              map[string]string{"GREETING": "hello ${prompt}"},
		StopWords:        []string{"${stopWord}"},
		RuntimeStopWords: []string{"${runtimeStopWord}"},
		Cron: &factorydefinitions.CronConfig{
			Schedule: "${schedule}", Every: "${every}", Jitter: "${jitter}", ExpiryWindow: "${expiry}",
		},
		OperationBindings: []factorydefinitions.ModelOperationBinding{functionalExecutionCatalogOperationBinding()},
		Inputs:            []factorydefinitions.IOConfig{functionalExecutionCatalogInput()},
		Outputs:           []factorydefinitions.IOConfig{{WorkTypeName: "${workType}", StateName: "done"}},
		OnContinue:        []factorydefinitions.IOConfig{{WorkTypeName: "${workType}", StateName: "continue"}},
		OnRejection:       []factorydefinitions.IOConfig{{WorkTypeName: "${workType}", StateName: "rejected"}},
		OnFailure:         []factorydefinitions.IOConfig{{WorkTypeName: "${workType}", StateName: "failed"}},
		ClassificationRoutes: []factorydefinitions.ClassificationRouteConfig{{
			Label: "${label}", Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "${workType}", StateName: "classified"}},
		}},
		ExpectedArtifacts: []factorydefinitions.ExpectedArtifactConfig{{
			Name: "${artifact}", Pattern: "artifacts/${prompt}.txt", NonEmpty: true,
		}},
		Resources: []factorydefinitions.ResourceConfig{{
			ID: "workstation-resource", Name: "workstation-model", Type: factorydefinitions.ResourceTypeModel,
			Capacity: 1, Model: "model-a", Provider: "provider-a",
		}},
		Guards: []factorydefinitions.GuardConfig{{
			Type: factorydefinitions.GuardTypeVisitCount, Workstation: "${guardWorkstation}",
			MaxVisits: 2, MaxVisitsArgument: "${maxVisits}",
			MatchConfig: &factorydefinitions.GuardMatchConfig{InputKey: "${inputKey}"},
		}},
	}
}

func functionalExecutionCatalogOperationBinding() factorydefinitions.ModelOperationBinding {
	return factorydefinitions.ModelOperationBinding{
		Slot: "${slot}",
		Selector: &factorydefinitions.ModelOperationBindingSelector{
			Slot: "${slot}", Label: "${label}", Type: "${contentType}", Role: "${role}",
		},
		Config: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText, Text: "config ${prompt}", URL: "${url}",
			File: "${contentFile}", Slot: "${slot}", Label: "${label}", Role: "${role}",
			ContentType: "${contentType}", JSON: json.RawMessage(`{"answer":"${prompt}"}`),
		}},
		DefaultContent: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeJSON, Text: "default ${prompt}", JSON: json.RawMessage(`{"answer":"${prompt}"}`),
		}},
	}
}

func functionalExecutionCatalogInput() factorydefinitions.IOConfig {
	return factorydefinitions.IOConfig{
		WorkTypeName: "${workType}", StateName: "${state}",
		Guard: &factorydefinitions.InputGuardConfig{
			Type: factorydefinitions.GuardTypeMatchesFields, MatchInput: "${label}",
			ParentInput: "${parent}", SpawnedBy: "${spawned}",
		},
	}
}

func functionalExecutionCatalogLogicalWorkstation() factorydefinitions.FactoryWorkstationConfig {
	return factorydefinitions.FactoryWorkstationConfig{
		ID: "logical-id", Name: "logical", Type: factorydefinitions.WorkstationTypeLogical,
		Inputs:  []factorydefinitions.IOConfig{{WorkTypeName: "${workType}", StateName: "ready"}},
		Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "${workType}", StateName: "done"}},
	}
}

func functionalExecutionCatalogInvocation() factorydefinitions.InvocationDefinitionContext {
	return factorydefinitions.InvocationDefinitionContext{
		Arguments: functionalExecutionCatalogArguments(),
		ReadFile: func(name string) ([]byte, error) {
			if name != "payload.txt" {
				return nil, fmt.Errorf("unexpected file %q", name)
			}
			return []byte("from-file"), nil
		},
	}
}

func functionalExecutionCatalogArguments() *work.InvocationArguments {
	return &work.InvocationArguments{Arguments: map[string]work.InvocationArgument{
		"provider":         {Values: []string{"provider-a"}},
		"model":            {Values: []string{"model-a"}},
		"effort":           {Values: []string{"high"}},
		"locality":         {Values: []string{"LOCAL"}},
		"executor":         {Values: []string{"SCRIPT_WRAP"}},
		"command":          {Values: []string{"you-worker"}},
		"prompt":           {Values: []string{"hello"}},
		"file":             {Values: []string{"payload.txt"}, ValueMode: work.InvocationParameterValueModeFileContents},
		"timeout":          {Values: []string{"2s"}},
		"stop":             {Values: []string{"<STOP>"}},
		"runner":           {Values: []string{"codex"}},
		"promptFile":       {Values: []string{"prompt.md"}},
		"schema":           {Values: []string{"schema.json"}},
		"contract":         {Values: []string{"contract.json"}},
		"directory":        {Values: []string{"workspace"}},
		"worktree":         {Values: []string{"tree"}},
		"maxItems":         {Values: []string{"max-items"}},
		"stopWord":         {Values: []string{"STOP"}},
		"runtimeStopWord":  {Values: []string{"RUNTIME_STOP"}},
		"schedule":         {Values: []string{"0 * * * *"}},
		"every":            {Values: []string{"1h"}},
		"jitter":           {Values: []string{"5m"}},
		"expiry":           {Values: []string{"10m"}},
		"slot":             {Values: []string{"prompt"}},
		"label":            {Values: []string{"accepted"}},
		"contentType":      {Values: []string{"TEXT"}},
		"role":             {Values: []string{"user"}},
		"url":              {Values: []string{"https://example.test/input"}},
		"contentFile":      {Values: []string{"input.txt"}},
		"json":             {Values: []string{`{"answer":"ok"}`}},
		"workType":         {Values: []string{"task"}},
		"state":            {Values: []string{"ready"}},
		"parent":           {Values: []string{"parent-input"}},
		"spawned":          {Values: []string{"source-work"}},
		"artifact":         {Values: []string{"answer"}},
		"guardWorkstation": {Values: []string{"run"}},
		"maxVisits":        {Values: []string{"max-visits"}},
		"inputKey":         {Values: []string{"input-key"}},
	}}
}

func functionalExecutionCatalogReferences() factorydefinitions.ExecutionCatalogReferenceCatalog {
	return factorydefinitions.ExecutionCatalogReferenceCatalog{
		Runners:   map[string]struct{}{"codex": {}},
		Providers: map[string]struct{}{"provider-a": {}},
		Models:    map[string]struct{}{"model-a": {}},
	}
}

func newFunctionalDefinitionsService(t *testing.T) factorydefinitions.Service {
	t.Helper()
	loader := functionalDefinitionsLoader(t)
	service, err := factorydefinitionswire.NewService(
		functionalSessionHost{},
		functionalActivationGateway{},
		functionalValidator{},
		functionalPersistence{},
		loader,
		func(string, *factorydefinitions.FactoryConfig, bool, bool) error { return nil },
		func(string, *factorydefinitions.FactoryConfig) error { return nil },
		functionalNamedPaths{},
		functionalNamedFactoryCatalogFileSystem{},
		factorydefinitionswire.StaticClock(time.Date(2026, 8, 12, 10, 11, 12, 0, time.UTC)),
		functionalVersionFileSystem{},
		func(context.Context, factorydefinitions.ListEffectiveFactoriesRequest) (factorydefinitions.ListEffectiveFactoriesResult, error) {
			return factorydefinitions.ListEffectiveFactoriesResult{}, nil
		},
		factorydefinitions.PackagedFactoryCatalogOperations{
			List: func(context.Context, factorydefinitions.ListBuiltInPackagedFactoriesRequest) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
				return factorydefinitions.ListBuiltInPackagedFactoriesResult{}, nil
			},
			Resolve: func(context.Context, factorydefinitions.ResolveBuiltInPackagedFactoryRequest) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
				return factorydefinitions.ResolveBuiltInPackagedFactoryResult{}, nil
			},
		},
		factorydefinitions.PackagedFactoryInstallationOperations{
			Install: func(context.Context, factorydefinitions.PackagedFactoryInstallParams) (factorydefinitions.PackagedFactoryInstallResult, error) {
				return factorydefinitions.PackagedFactoryInstallResult{}, nil
			},
		},
		functionalRequiredToolChecker{},
		functionalOrchestratorValidator{},
		platformfilesystem.Local{},
		functionalDirectoryReplacementStore{},
	)
	if err != nil {
		t.Fatalf("construct public Definitions service: %v", err)
	}
	if service == nil {
		t.Fatal("public Definitions service is nil")
	}
	return service
}

func functionalDefinitionsLoader(t *testing.T) *factorydefinitionswire.Loader {
	t.Helper()
	fileSystem := platformfilesystem.Local{}
	return factorydefinitionswire.NewLoader(
		func(string, *factorydefinitions.FactoryConfig, bool, bool) error { return nil },
		func(string, *factorydefinitions.FactoryConfig) error { return nil },
		func(string, *factorydefinitions.FactoryConfig) ([]factorydefinitions.PortableBundledFileReplacement, error) {
			return nil, nil
		},
		fileSystem,
		functionalNamedPaths{},
		fileSystem,
		func(string, factorydefinitions.BundledFileConfig) (string, bool) { return "", false },
		fileSystem,
		functionalRequiredToolChecker{},
	)
}

// These inert adapters keep this test focused on the public Definitions root.
// ResolveExecutionCatalog does not invoke any of the lifecycle, persistence,
// filesystem, or validation peers supplied during composition.
type functionalSessionHost struct{ factorydefinitions.SessionHost }
type functionalActivationGateway struct {
	factorydefinitions.DefinitionActivationGateway
}
type functionalValidator struct{ factorydefinitions.Validator }
type functionalPersistence struct{ factorydefinitions.Persistence }
type functionalNamedPaths struct {
	factorydefinitions.NamedPathResolver
}
type functionalNamedFactoryCatalogFileSystem struct {
	factorydefinitions.NamedFactoryCatalogFileSystem
}
type functionalVersionFileSystem struct {
	factorydefinitions.VersionFileSystem
}
type functionalRequiredToolChecker struct {
	factorydefinitions.RequiredToolChecker
}
type functionalOrchestratorValidator struct {
	factorydefinitions.OrchestratorDefinitionValidator
}
type functionalDirectoryReplacementStore struct {
	factorydefinitions.DirectoryReplacementStore
}
