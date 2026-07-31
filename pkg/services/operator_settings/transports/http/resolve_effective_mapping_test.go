package http

import (
	"errors"
	"reflect"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func TestResolveEffectiveRequestFromHTTP_MapsRootRequest(t *testing.T) {
	t.Parallel()

	expectedBaseline := operatorsettings.DocumentDefaults{
		WorkerModelProvider: "codex",
		WorkerModel:         "gpt-5",
	}
	request, err := ResolveEffectiveRequestFromHTTP(ResolveEffectiveInput{
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "gpt-5",
		},
		BackendScopeID: "local-00000000-0000-4000-8000-000000000010",
		WorkerPresets: []operatorsettings.DocumentWorkerPreset{
			{ID: "default", ModelProvider: "codex", Model: "gpt-5"},
		},
		ExpectedDocumentBaseline: &expectedBaseline,
		EnvironmentOverrides: EffectiveOverrideFactsInput{
			WorkerModelProvider: "openai",
		},
		InvocationOverrides: EffectiveOverrideFactsInput{
			WorkerModel: "gpt-5.2",
		},
		ConfigPath: "/home/operator/.you-agent-factory/config.json",
	})
	if err != nil {
		t.Fatalf("ResolveEffectiveRequestFromHTTP: %v", err)
	}
	if request.DocumentBaseline.WorkerModelProvider != "codex" ||
		request.DocumentBaseline.WorkerModel != "gpt-5" {
		t.Fatalf("request.DocumentBaseline = %#v, want codex/gpt-5 baseline", request.DocumentBaseline)
	}
	if request.BackendScopeID != "local-00000000-0000-4000-8000-000000000010" {
		t.Fatalf("request.BackendScopeID = %q, want backend scope", request.BackendScopeID)
	}
	if len(request.WorkerPresets) != 1 || request.WorkerPresets[0].ID != "default" {
		t.Fatalf("request.WorkerPresets = %#v, want default preset", request.WorkerPresets)
	}
	if request.ExpectedDocumentBaseline == nil ||
		request.ExpectedDocumentBaseline.WorkerModelProvider != "codex" {
		t.Fatalf("request.ExpectedDocumentBaseline = %#v, want expected baseline", request.ExpectedDocumentBaseline)
	}
	if request.EnvironmentOverrides.WorkerModelProvider != "openai" {
		t.Fatalf("request.EnvironmentOverrides = %#v, want env provider override", request.EnvironmentOverrides)
	}
	if request.InvocationOverrides.WorkerModel != "gpt-5.2" {
		t.Fatalf("request.InvocationOverrides = %#v, want invocation model override", request.InvocationOverrides)
	}
	if request.ConfigPath != "/home/operator/.you-agent-factory/config.json" {
		t.Fatalf("request.ConfigPath = %q, want config path", request.ConfigPath)
	}
}

func TestResolveEffectiveRequestFromHTTP_RejectsBaselineMismatchBeforeRoot(t *testing.T) {
	t.Parallel()

	expected := operatorsettings.DocumentDefaults{
		WorkerModelProvider: "openai",
		WorkerModel:         "gpt-4",
	}
	_, err := ResolveEffectiveRequestFromHTTP(ResolveEffectiveInput{
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "gpt-5",
		},
		ExpectedDocumentBaseline: &expected,
	})
	if err == nil || !errors.Is(err, operatorsettings.ErrResolutionConflict) {
		t.Fatalf("ResolveEffectiveRequestFromHTTP = %v, want baseline conflict", err)
	}
}

func TestResolveEffectiveResponseToHTTP_MapsPrecedenceSourcesAndSelectedValues(t *testing.T) {
	t.Parallel()

	result := operatorsettings.ResolveEffectiveResult{
		Selection: operatorsettings.EffectiveSelection{
			BackendScopeID: "local-00000000-0000-4000-8000-000000000010",
			WorkerPresets: []operatorsettings.DocumentWorkerPreset{
				{ID: "default", ModelProvider: "codex", Model: "gpt-5"},
			},
			WorkerModelProvider:       "codex",
			WorkerModel:               "gpt-5.2",
			WorkerModelProviderSource: operatorsettings.EffectiveLayerSourceEnv,
			WorkerModelSource:         operatorsettings.EffectiveLayerSourceFlag,
			ConfigPath:                "/tmp/config.json",
		},
	}
	response := ResolveEffectiveResponseToHTTP(result)

	want := EffectiveSelectionResponse{
		BackendScopeID:            "local-00000000-0000-4000-8000-000000000010",
		WorkerModelProvider:       "codex",
		WorkerModel:               "gpt-5.2",
		WorkerModelProviderSource: "env",
		WorkerModelSource:         "flag",
		ConfigPath:                "/tmp/config.json",
	}
	if response.Selection.BackendScopeID != want.BackendScopeID ||
		response.Selection.WorkerModelProvider != want.WorkerModelProvider ||
		response.Selection.WorkerModel != want.WorkerModel ||
		response.Selection.WorkerModelProviderSource != want.WorkerModelProviderSource ||
		response.Selection.WorkerModelSource != want.WorkerModelSource ||
		response.Selection.ConfigPath != want.ConfigPath {
		t.Fatalf("response.Selection = %#v, want %#v", response.Selection, want)
	}
	if len(response.Selection.WorkerPresets) != 1 {
		t.Fatalf("response.Selection.WorkerPresets = %#v, want one preset", response.Selection.WorkerPresets)
	}
	if response.Selection.WorkerPresets[0].Id != "default" {
		t.Fatalf("response.Selection.WorkerPresets[0].Id = %q, want default", response.Selection.WorkerPresets[0].Id)
	}
	if response.Selection.WorkerPresets[0].Model == nil ||
		*response.Selection.WorkerPresets[0].Model != "gpt-5" {
		t.Fatalf("response.Selection.WorkerPresets[0].Model = %#v, want gpt-5", response.Selection.WorkerPresets[0].Model)
	}
}

func TestResolveEffectiveResponseToHTTP_IsStableRoundTripForSelectionFields(t *testing.T) {
	t.Parallel()

	selection := operatorsettings.EffectiveSelection{
		BackendScopeID:            "scope-a",
		WorkerModelProvider:       "codex",
		WorkerModel:               "gpt-5",
		WorkerModelProviderSource: operatorsettings.EffectiveLayerSourceFile,
		WorkerModelSource:         operatorsettings.EffectiveLayerSourceFile,
		ConfigPath:                "/tmp/config.json",
	}
	first := effectiveSelectionToHTTP(selection)
	second := effectiveSelectionToHTTP(selection)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("effectiveSelectionToHTTP() = %#v then %#v, want stable mapping", first, second)
	}
}
