package cli_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorsettingscli "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/cli"
)

func TestBindConfigureRequiresSettingsRoot(t *testing.T) {
	t.Parallel()

	if operation := operatorsettingscli.BindConfigure(nil); operation != nil {
		t.Fatalf("BindConfigure(nil) = %T, want nil", operation)
	}
}

func TestBindConfigureDelegatesThroughAdapterService(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	model := "free-form/model:v1"
	root := newCompositionSettingsRoot(map[string]operatorsettings.Document{
		configPath: {
			BackendScopeID: "scope-1",
			Defaults: operatorsettings.DocumentDefaults{
				WorkerModelProvider: "codex",
				WorkerModel:         "gpt-5",
			},
			Runtime: operatorsettings.EmptyDocument().Runtime,
		},
	})
	operation := operatorsettingscli.BindConfigure(root)
	if operation == nil {
		t.Fatal("BindConfigure(root) = nil, want composition operation")
	}

	var output bytes.Buffer
	err := operation(operatorsettingscli.ConfigureConfig{
		Context:  context.Background(),
		HomeDir:  homeDir,
		Provider: "CODEX",
		Model:    &model,
		Output:   &output,
	})
	if err != nil {
		t.Fatalf("operation(cfg) error = %v", err)
	}
	if !strings.Contains(output.String(), configPath) {
		t.Fatalf("output = %q, want configured defaults presentation", output.String())
	}
}

func TestBindConfigureMatchesFreeFunctionFacade(t *testing.T) {
	t.Parallel()

	configureErr := errors.New("configure failed")
	root := newCompositionSettingsRoot(nil)
	root.configureErr = configureErr
	operation := operatorsettingscli.BindConfigure(root)

	cfg := operatorsettingscli.ConfigureConfig{
		Context: context.Background(),
		HomeDir: t.TempDir(),
		Output:  bytes.NewBuffer(nil),
	}
	boundErr := operation(cfg)
	directErr := operatorsettingscli.Configure(cfg, root)
	if (boundErr == nil) != (directErr == nil) {
		t.Fatalf("bound error = %v, direct error = %v", boundErr, directErr)
	}
	if boundErr != nil && boundErr.Error() != directErr.Error() {
		t.Fatalf("bound error = %q, direct error = %q", boundErr.Error(), directErr.Error())
	}
}

func TestBindResolveOperatorDefaultsRequiresSettingsRoot(t *testing.T) {
	t.Parallel()

	if operation := operatorsettingscli.BindResolveOperatorDefaults(nil); operation != nil {
		t.Fatalf("BindResolveOperatorDefaults(nil) = %T, want nil", operation)
	}
}

func TestBindResolveOperatorDefaultsDelegatesThroughAdapterService(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	root := newCompositionSettingsRoot(map[string]operatorsettings.Document{
		configPath: {
			Defaults: operatorsettings.DocumentDefaults{
				WorkerModelProvider: "codex",
				WorkerModel:         "gpt-5",
			},
			Runtime: operatorsettings.EmptyDocument().Runtime,
		},
	})
	operation := operatorsettingscli.BindResolveOperatorDefaults(root)
	if operation == nil {
		t.Fatal("BindResolveOperatorDefaults(root) = nil, want composition operation")
	}

	resolved, err := operation(homeDir, operatorsettings.Defaults{
		WorkerModelProvider: "gemini",
	}, operatorsettings.FlagOverrides{
		WorkerModel: "flag-model",
	})
	if err != nil {
		t.Fatalf("operation() error = %v", err)
	}
	if resolved.WorkerModelProvider != "GEMINI" || resolved.WorkerModel != "flag-model" {
		t.Fatalf("resolved = %#v, want delegated defaults resolution", resolved)
	}
}

func TestBindResolveOperatorDefaultsMatchesFreeFunctionFacade(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	root := newCompositionSettingsRoot(nil)
	operation := operatorsettingscli.BindResolveOperatorDefaults(root)

	flags := operatorsettings.FlagOverrides{WorkerModelProvider: "DEFAULT"}
	resolvedBound, boundErr := operation(homeDir, operatorsettings.Defaults{}, flags)
	resolvedDirect, directErr := operatorsettingscli.ResolveOperatorDefaults(
		operatorsettingscli.ResolveOperatorDefaultsConfig{
			HomeDir: homeDir,
			Flags:   flags,
		},
		root,
	)
	if (boundErr == nil) != (directErr == nil) {
		t.Fatalf("bound error = %v, direct error = %v", boundErr, directErr)
	}
	if boundErr != nil && boundErr.Error() != directErr.Error() {
		t.Fatalf("bound error = %q, direct error = %q", boundErr.Error(), directErr.Error())
	}
	if boundErr == nil && resolvedBound != resolvedDirect {
		t.Fatalf("bound = %#v, direct = %#v", resolvedBound, resolvedDirect)
	}
}

type compositionSettingsRoot struct {
	documents    map[string]operatorsettings.Document
	configureErr error
	resolveErr   error
}

func newCompositionSettingsRoot(
	documents map[string]operatorsettings.Document,
) *compositionSettingsRoot {
	if documents == nil {
		documents = map[string]operatorsettings.Document{}
	}
	return &compositionSettingsRoot{documents: documents}
}

func (root *compositionSettingsRoot) LoadDocument(
	request operatorsettings.LoadDocumentRequest,
) (operatorsettings.LoadDocumentResult, error) {
	document, ok := root.documents[request.Path]
	if !ok {
		return operatorsettings.LoadDocumentResult{
			Document: operatorsettings.EmptyDocument(),
			Path:     request.Path,
		}, nil
	}
	return operatorsettings.LoadDocumentResult{
		Document: document,
		Path:     request.Path,
		Found:    true,
	}, nil
}

func (root *compositionSettingsRoot) ApplyDocumentUpdate(
	request operatorsettings.ApplyDocumentUpdateRequest,
) (operatorsettings.ApplyDocumentUpdateResult, error) {
	if root.configureErr != nil {
		return operatorsettings.ApplyDocumentUpdateResult{}, root.configureErr
	}
	document := root.documents[request.Path]
	if request.ProviderModel.Provider != nil {
		document.Defaults.WorkerModelProvider = *request.ProviderModel.Provider
	}
	if request.ProviderModel.Model != nil {
		document.Defaults.WorkerModel = *request.ProviderModel.Model
	}
	root.documents[request.Path] = document
	return operatorsettings.ApplyDocumentUpdateResult{
		Document:  document,
		Persisted: true,
	}, nil
}

func (root *compositionSettingsRoot) ResolveEffective(
	request operatorsettings.ResolveEffectiveRequest,
) (operatorsettings.ResolveEffectiveResult, error) {
	if root.resolveErr != nil {
		return operatorsettings.ResolveEffectiveResult{}, root.resolveErr
	}
	provider := request.InvocationOverrides.WorkerModelProvider
	if provider == "" {
		provider = request.EnvironmentOverrides.WorkerModelProvider
	}
	if provider == "" {
		provider = request.DocumentBaseline.WorkerModelProvider
	}
	model := request.InvocationOverrides.WorkerModel
	if model == "" {
		model = request.EnvironmentOverrides.WorkerModel
	}
	if model == "" {
		model = request.DocumentBaseline.WorkerModel
	}
	source := operatorsettings.EffectiveLayerSourceFile
	if request.InvocationOverrides.WorkerModelProvider != "" || request.InvocationOverrides.WorkerModel != "" {
		source = operatorsettings.EffectiveLayerSourceFlag
	} else if request.EnvironmentOverrides.WorkerModelProvider != "" || request.EnvironmentOverrides.WorkerModel != "" {
		source = operatorsettings.EffectiveLayerSourceEnv
	}
	return operatorsettings.ResolveEffectiveResult{
		Selection: operatorsettings.EffectiveSelection{
			WorkerModelProvider:       strings.ToUpper(provider),
			WorkerModel:               model,
			WorkerModelProviderSource: source,
			WorkerModelSource:         source,
			ConfigPath:                request.ConfigPath,
		},
	}, nil
}
