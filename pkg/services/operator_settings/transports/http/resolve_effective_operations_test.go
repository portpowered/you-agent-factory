package http

import (
	"errors"
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
	adapter := NewAdapterFromRoot(RootBinding{Settings: fake})

	response, err := adapter.ResolveEffective(ResolveEffectiveInput{
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
	adapter := NewAdapterFromRoot(RootBinding{Settings: fake})

	_, err := adapter.ResolveEffective(ResolveEffectiveInput{
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
			adapter := NewAdapterFromRoot(RootBinding{Settings: fake})

			_, err := adapter.ResolveEffective(ResolveEffectiveInput{
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
	adapter := NewAdapterFromRoot(RootBinding{Settings: fake})

	_, err := adapter.ResolveEffective(ResolveEffectiveInput{
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "gpt-5",
		},
	})
	if err != nil {
		t.Fatalf("ResolveEffective error = %v", err)
	}
}
