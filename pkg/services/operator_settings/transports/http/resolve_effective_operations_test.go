package http

import (
	"context"
	"errors"
	"reflect"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func TestAdapter_ResolveEffectiveInvokesFakeRootAndEncodesSuccess(t *testing.T) {
	t.Parallel()

	configPath := "/home/operator/.you-agent-factory/config.json"
	scopeID := "local-00000000-0000-4000-8000-000000000010"
	var invoked bool
	fake := &rootFake{
		resolveEffective: func(
			request operatorsettings.ResolveEffectiveRequest,
		) (operatorsettings.ResolveEffectiveResult, error) {
			invoked = true
			if request.DocumentBaseline.WorkerModelProvider != "codex" ||
				request.DocumentBaseline.WorkerModel != "gpt-5" {
				t.Fatalf("ResolveEffectiveRequest.DocumentBaseline = %#v, want codex/gpt-5", request.DocumentBaseline)
			}
			if request.BackendScopeID != scopeID {
				t.Fatalf("ResolveEffectiveRequest.BackendScopeID = %q, want %q", request.BackendScopeID, scopeID)
			}
			if request.InvocationOverrides.WorkerModel != "gpt-5.2" {
				t.Fatalf(
					"ResolveEffectiveRequest.InvocationOverrides = %#v, want model override gpt-5.2",
					request.InvocationOverrides,
				)
			}
			if request.ConfigPath != configPath {
				t.Fatalf("ResolveEffectiveRequest.ConfigPath = %q, want %q", request.ConfigPath, configPath)
			}
			return operatorsettings.ResolveEffectiveResult{
				Selection: operatorsettings.EffectiveSelection{
					BackendScopeID:            scopeID,
					WorkerModelProvider:       "codex",
					WorkerModel:               "gpt-5.2",
					WorkerModelProviderSource: operatorsettings.EffectiveLayerSourceFile,
					WorkerModelSource:         operatorsettings.EffectiveLayerSourceFlag,
					ConfigPath:                configPath,
				},
			}, nil
		},
	}
	adapter := NewAdapter(fake)

	response, err := adapter.ResolveEffective(context.Background(), ResolveEffectiveInput{
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "gpt-5",
		},
		BackendScopeID: scopeID,
		InvocationOverrides: EffectiveOverrideFactsInput{
			WorkerModel: "gpt-5.2",
		},
		ConfigPath: configPath,
	})
	if !invoked {
		t.Fatal("ResolveEffective did not invoke the injected Settings root")
	}
	if err != nil {
		t.Fatalf("ResolveEffective error = %v", err)
	}
	if response.Selection.WorkerModelProvider != "codex" ||
		response.Selection.WorkerModel != "gpt-5.2" ||
		response.Selection.WorkerModelProviderSource != "file" ||
		response.Selection.WorkerModelSource != "flag" {
		t.Fatalf("response.Selection = %#v, want resolved codex/gpt-5.2 with file/flag sources", response.Selection)
	}
}

func TestAdapter_ResolveEffectiveRejectsBaselineMismatchBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	expected := operatorsettings.DocumentDefaults{
		WorkerModelProvider: "openai",
		WorkerModel:         "gpt-4",
	}
	fake := &rootFake{
		resolveEffective: func(
			operatorsettings.ResolveEffectiveRequest,
		) (operatorsettings.ResolveEffectiveResult, error) {
			t.Fatal("fake root must not be invoked for baseline mismatch")
			return operatorsettings.ResolveEffectiveResult{}, nil
		},
	}
	adapter := NewAdapter(fake)

	_, err := adapter.ResolveEffective(context.Background(), ResolveEffectiveInput{
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "gpt-5",
		},
		ExpectedDocumentBaseline: &expected,
	})
	if err == nil || !errors.Is(err, operatorsettings.ErrResolutionConflict) {
		t.Fatalf("ResolveEffective error = %v, want baseline conflict", err)
	}
}

func TestAdapter_ResolveEffectivePropagatesTypedRootFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
	}{
		{name: "invalid_input", err: operatorsettings.ErrResolutionInvalidInput},
		{name: "unsupported_override", err: operatorsettings.ErrResolutionUnsupportedOverride},
		{name: "conflict", err: operatorsettings.ErrResolutionConflict},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fake := &rootFake{
				resolveEffective: func(
					operatorsettings.ResolveEffectiveRequest,
				) (operatorsettings.ResolveEffectiveResult, error) {
					return operatorsettings.ResolveEffectiveResult{}, test.err
				},
			}
			adapter := NewAdapter(fake)

			_, err := adapter.ResolveEffective(context.Background(), ResolveEffectiveInput{
				DocumentBaseline: operatorsettings.DocumentDefaults{
					WorkerModelProvider: "codex",
					WorkerModel:         "gpt-5",
				},
			})
			if err == nil || !errors.Is(err, test.err) {
				t.Fatalf("ResolveEffective error = %v, want %v", err, test.err)
			}
		})
	}
}

func TestAdapter_ResolveEffectiveDoesNotMutateOperatorDocumentState(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		loadDocument: func(
			operatorsettings.LoadDocumentRequest,
		) (operatorsettings.LoadDocumentResult, error) {
			t.Fatal("ResolveEffective must not invoke LoadDocument")
			return operatorsettings.LoadDocumentResult{}, nil
		},
		applyDocumentUpdate: func(
			operatorsettings.ApplyDocumentUpdateRequest,
		) (operatorsettings.ApplyDocumentUpdateResult, error) {
			t.Fatal("ResolveEffective must not invoke ApplyDocumentUpdate")
			return operatorsettings.ApplyDocumentUpdateResult{}, nil
		},
		resolveEffective: func(
			operatorsettings.ResolveEffectiveRequest,
		) (operatorsettings.ResolveEffectiveResult, error) {
			return operatorsettings.ResolveEffectiveResult{
				Selection: operatorsettings.EffectiveSelection{
					WorkerModelProvider: "codex",
					WorkerModel:         "gpt-5",
				},
			}, nil
		},
	}
	adapter := NewAdapter(fake)

	_, err := adapter.ResolveEffective(context.Background(), ResolveEffectiveInput{
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "gpt-5",
		},
	})
	if err != nil {
		t.Fatalf("ResolveEffective error = %v", err)
	}
}

func TestAdapter_ResolveEffectiveForwardsAllFacts(t *testing.T) {
	t.Parallel()
	baseline := operatorsettings.DocumentDefaults{WorkerModelProvider: "codex", WorkerModel: "gpt-5"}
	presets := []operatorsettings.DocumentWorkerPreset{
		{ID: "focused", ModelProvider: "codex", Model: "gpt-5", ReasoningEffort: "high"},
	}
	requests := make(chan operatorsettings.ResolveEffectiveRequest, 1)
	fake := &rootFake{
		loadDocument: func(operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
			t.Error("resolve invoked load")
			return operatorsettings.LoadDocumentResult{}, nil
		},
		applyDocumentUpdate: func(operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error) {
			t.Error("resolve invoked update")
			return operatorsettings.ApplyDocumentUpdateResult{}, nil
		},
		resolveEffective: func(request operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error) {
			requests <- request
			return operatorsettings.ResolveEffectiveResult{Selection: operatorsettings.EffectiveSelection{
				BackendScopeID: " scope ", ConfigPath: " /owner/config.json ", WorkerPresets: presets,
				WorkerModelProvider: " codex ", WorkerModel: " gpt-5 ",
				WorkerModelProviderSource: operatorsettings.EffectiveLayerSourceEnv,
				WorkerModelSource:         operatorsettings.EffectiveLayerSourceFlag,
			}}, nil
		},
	}
	response, err := NewAdapter(fake).ResolveEffective(context.Background(), ResolveEffectiveInput{
		DocumentBaseline: baseline, ExpectedDocumentBaseline: &baseline, WorkerPresets: presets,
		BackendScopeID: " scope ", ConfigPath: " /request/config.json ",
		EnvironmentOverrides: EffectiveOverrideFactsInput{
			WorkerModelProvider: " openai ", WorkerModel: " env-model ", WorkerPresetID: " env-preset ",
		},
		InvocationOverrides: EffectiveOverrideFactsInput{
			WorkerModelProvider: " codex ", WorkerModel: " flag-model ", WorkerPresetID: " focused ",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: baseline, ExpectedDocumentBaseline: &baseline, WorkerPresets: presets,
		BackendScopeID: "scope", ConfigPath: "/request/config.json",
		EnvironmentOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "openai", WorkerModel: "env-model", WorkerPresetID: "env-preset",
		},
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "codex", WorkerModel: "flag-model", WorkerPresetID: "focused",
		},
	}
	if got := <-requests; !reflect.DeepEqual(got, want) {
		t.Fatalf("owner request = %#v, want %#v", got, want)
	}
	assertOwnerSelection(t, response.Selection)
}

func assertOwnerSelection(t *testing.T, selection EffectiveSelectionResponse) {
	t.Helper()
	if selection.BackendScopeID != "scope" || selection.ConfigPath != "/owner/config.json" ||
		selection.WorkerModelProvider != "codex" || selection.WorkerModel != "gpt-5" ||
		selection.WorkerModelProviderSource != "env" || selection.WorkerModelSource != "flag" ||
		len(selection.WorkerPresets) != 1 {
		t.Fatalf("selection = %#v, want owner values and sources", selection)
	}
	preset := selection.WorkerPresets[0]
	if preset.Id != "focused" || preset.ModelProvider != "codex" || preset.Model == nil ||
		*preset.Model != "gpt-5" || preset.ReasoningEffort == nil || *preset.ReasoningEffort != "high" {
		t.Fatalf("preset = %#v, want all owner preset facts", preset)
	}
}
