package cli_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorsettingscli "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/cli"
)

func TestNewRequiresSettingsRoot(t *testing.T) {
	t.Parallel()

	if service := operatorsettingscli.New(nil); service != nil {
		t.Fatalf("New(nil) = %T, want nil", service)
	}
}

func TestConstructedService_ConfigureReportsPersistedDefaults(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	model := "free-form/model:v1"
	root := newFakeSettingsRoot(map[string]operatorsettings.Document{
		configPath: {
			BackendScopeID: "scope-1",
			Defaults: operatorsettings.DocumentDefaults{
				WorkerModelProvider: "codex",
				WorkerModel:         "gpt-5",
			},
			Runtime: operatorsettings.EmptyDocument.Runtime,
		},
	})
	service := operatorsettingscli.New(root)
	if service == nil {
		t.Fatal("New(root) = nil, want Settings CLI service")
	}

	var output bytes.Buffer
	err := service.Configure(operatorsettingscli.ConfigureConfig{
		Context:  context.Background(),
		HomeDir:  homeDir,
		Provider: "CODEX",
		Model:    &model,
		Output:   &output,
	})
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
	for _, want := range []string{
		"codex",
		"free-form/model:v1",
		configPath,
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output = %q, want %q", output.String(), want)
		}
	}
	updated := root.documents[configPath]
	if updated.Defaults.WorkerModelProvider != "codex" || updated.Defaults.WorkerModel != model {
		t.Fatalf("document = %#v, want updated provider/model", updated)
	}
}

func TestConstructedService_ConfigureRejectsMissingProviderBeforePersistence(t *testing.T) {
	t.Parallel()

	root := newFakeSettingsRoot(nil)
	service := operatorsettingscli.New(root)
	if service == nil {
		t.Fatal("New(root) = nil, want Settings CLI service")
	}

	var output bytes.Buffer
	err := service.Configure(operatorsettingscli.ConfigureConfig{
		Context: context.Background(),
		HomeDir: t.TempDir(),
		Output:  &output,
	})
	if err == nil || !strings.Contains(err.Error(), "use --provider") {
		t.Fatalf("Configure() error = %v, want supplied-provider guidance", err)
	}
	if output.Len() != 0 {
		t.Fatalf("output = %q, want empty", output.String())
	}
}

func TestConstructedService_ConfigureRejectsUnsupportedProvider(t *testing.T) {
	t.Parallel()

	root := newFakeSettingsRoot(nil)
	service := operatorsettingscli.New(root)
	if service == nil {
		t.Fatal("New(root) = nil, want Settings CLI service")
	}

	unsupported := "unsupported-provider"
	err := service.Configure(operatorsettingscli.ConfigureConfig{
		Context:  context.Background(),
		HomeDir:  t.TempDir(),
		Provider: unsupported,
		Output:   &bytes.Buffer{},
	})
	if !errors.Is(err, operatorsettings.ErrDocumentUnsupported) {
		t.Fatalf("Configure() error = %v, want ErrDocumentUnsupported", err)
	}
}

func TestConstructedService_ConfigureHonorsPromptCancellation(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	original := operatorsettings.Document{
		BackendScopeID: "scope-1",
		Defaults: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "original",
		},
		Runtime: operatorsettings.EmptyDocument.Runtime,
	}
	root := newFakeSettingsRoot(map[string]operatorsettings.Document{
		configPath: original.Clone(),
	})
	service := operatorsettingscli.New(root)
	if service == nil {
		t.Fatal("New(root) = nil, want Settings CLI service")
	}

	err := service.Configure(operatorsettingscli.ConfigureConfig{
		Context:       context.Background(),
		HomeDir:       homeDir,
		Input:         strings.NewReader("/cancel\n"),
		Output:        &bytes.Buffer{},
		Interactive:   true,
		NewLineReader: testLineReaderFactory,
	})
	if !errors.Is(err, operatorsettings.ErrProviderModelInputCanceled) {
		t.Fatalf("Configure() error = %v, want ErrProviderModelInputCanceled", err)
	}
	got := root.documents[configPath]
	if got.Defaults.WorkerModel != original.Defaults.WorkerModel {
		t.Fatalf("document changed after cancellation: %#v", got.Defaults)
	}
}

func TestConstructedService_ResolveOperatorDefaultsDelegatesToRoot(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	root := newFakeSettingsRoot(map[string]operatorsettings.Document{
		configPath: {
			Defaults: operatorsettings.DocumentDefaults{
				WorkerModelProvider: "claude",
				WorkerModel:         "file-model",
			},
			Runtime: operatorsettings.EmptyDocument.Runtime,
		},
	})
	service := operatorsettingscli.New(root)
	if service == nil {
		t.Fatal("New(root) = nil, want Settings CLI service")
	}

	got, err := service.ResolveOperatorDefaults(operatorsettingscli.ResolveOperatorDefaultsConfig{
		HomeDir: homeDir,
		Environment: operatorsettings.Defaults{
			WorkerModel: "env-model",
		},
		Flags: operatorsettings.FlagOverrides{
			WorkerModelProvider: "gemini",
			WorkerModel:         "flag-model",
		},
	})
	if err != nil {
		t.Fatalf("ResolveOperatorDefaults() error = %v", err)
	}
	if got.WorkerModelProvider != "GEMINI" ||
		got.WorkerModel != "flag-model" ||
		got.WorkerModelProviderSource != operatorsettings.SourceFlag ||
		got.WorkerModelSource != operatorsettings.SourceFlag ||
		got.ConfigPath != configPath {
		t.Fatalf("resolved = %#v, want flag-layer defaults", got)
	}
}

func TestConfigureFacadeMatchesConstructedService(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	model := "gpt-test"
	root := newFakeSettingsRoot(map[string]operatorsettings.Document{
		configPath: {
			BackendScopeID: "scope-1",
			Defaults: operatorsettings.DocumentDefaults{
				WorkerModelProvider: "codex",
				WorkerModel:         "gpt-5",
			},
			Runtime: operatorsettings.EmptyDocument.Runtime,
		},
	})
	service := operatorsettingscli.New(root)
	if service == nil {
		t.Fatal("New(root) = nil, want Settings CLI service")
	}

	cfg := operatorsettingscli.ConfigureConfig{
		Context:  context.Background(),
		HomeDir:  homeDir,
		Provider: "codex",
		Model:    &model,
	}
	var serviceOut, commandOut bytes.Buffer
	serviceCfg := cfg
	serviceCfg.Output = &serviceOut
	commandCfg := cfg
	commandCfg.Output = &commandOut

	serviceErr := service.Configure(serviceCfg)
	commandErr := operatorsettingscli.Configure(commandCfg, root)
	if (serviceErr == nil) != (commandErr == nil) {
		t.Fatalf("service error = %v, command error = %v", serviceErr, commandErr)
	}
	if serviceOut.String() != commandOut.String() {
		t.Fatalf("service output = %q, command output = %q", serviceOut.String(), commandOut.String())
	}
}

type fakeSettingsRoot struct {
	documents map[string]operatorsettings.Document
}

func newFakeSettingsRoot(entries map[string]operatorsettings.Document) *fakeSettingsRoot {
	copied := make(map[string]operatorsettings.Document, len(entries))
	for path, document := range entries {
		copied[path] = document.Clone()
	}
	return &fakeSettingsRoot{documents: copied}
}

func (fake *fakeSettingsRoot) LoadDocument(
	request operatorsettings.LoadDocumentRequest,
) (operatorsettings.LoadDocumentResult, error) {
	if err := request.Validate(); err != nil {
		return operatorsettings.LoadDocumentResult{}, err
	}
	path := strings.TrimSpace(request.Path)
	document, found := fake.documents[path]
	if !found {
		if request.RequireExisting {
			return operatorsettings.LoadDocumentResult{}, operatorsettings.DocumentFailure{
				Kind: operatorsettings.DocumentFailureKindNotFound,
				Path: path,
			}
		}
		return operatorsettings.LoadDocumentResult{
			Document: operatorsettings.EmptyDocument,
			Path:     path,
			Found:    false,
		}, nil
	}
	return operatorsettings.LoadDocumentResult{
		Document: document.Clone(),
		Path:     path,
		Found:    true,
	}, nil
}

func (fake *fakeSettingsRoot) ApplyDocumentUpdate(
	request operatorsettings.ApplyDocumentUpdateRequest,
) (operatorsettings.ApplyDocumentUpdateResult, error) {
	if err := request.Validate(); err != nil {
		return operatorsettings.ApplyDocumentUpdateResult{}, err
	}
	path := strings.TrimSpace(request.Path)
	document, found := fake.documents[path]
	if !found {
		document = operatorsettings.EmptyDocument
	}
	expected := strings.TrimSpace(request.ExpectedBackendScope)
	if expected != "" && document.BackendScopeID != expected {
		return operatorsettings.ApplyDocumentUpdateResult{}, operatorsettings.DocumentFailure{
			Kind:    operatorsettings.DocumentFailureKindConflict,
			Message: "backend scope mismatch",
			Path:    path,
		}
	}
	updated, err := mergeProviderModelUpdate(document, request.ProviderModel)
	if err != nil {
		return operatorsettings.ApplyDocumentUpdateResult{}, err
	}
	fake.documents[path] = updated
	return operatorsettings.ApplyDocumentUpdateResult{
		Document:  updated.Clone(),
		Path:      path,
		Persisted: true,
	}, nil
}

func (fake *fakeSettingsRoot) ResolveEffective(
	request operatorsettings.ResolveEffectiveRequest,
) (operatorsettings.ResolveEffectiveResult, error) {
	if err := request.Validate(); err != nil {
		return operatorsettings.ResolveEffectiveResult{}, err
	}
	providerRaw, providerSource := winningLayerValue(
		request.DocumentBaseline.WorkerModelProvider,
		request.EnvironmentOverrides.WorkerModelProvider,
		request.InvocationOverrides.WorkerModelProvider,
	)
	modelRaw, modelSource := winningLayerValue(
		request.DocumentBaseline.WorkerModel,
		request.EnvironmentOverrides.WorkerModel,
		request.InvocationOverrides.WorkerModel,
	)
	resolvedProvider, err := resolveWorkerModelProvider(providerRaw, providerSource, request)
	if err != nil {
		return operatorsettings.ResolveEffectiveResult{}, err
	}
	return operatorsettings.ResolveEffectiveResult{
		Selection: operatorsettings.EffectiveSelection{
			WorkerModelProvider:       resolvedProvider,
			WorkerModel:               strings.TrimSpace(modelRaw),
			WorkerModelProviderSource: providerSource,
			WorkerModelSource:         modelSource,
			ConfigPath:                strings.TrimSpace(request.ConfigPath),
		},
	}, nil
}

func (fake *fakeSettingsRoot) DefaultConfigPath(homeDir string) string {
	return filepath.Join(strings.TrimSpace(homeDir), ".you-agent-factory", "config.json")
}

func (fake *fakeSettingsRoot) LoadFileConfig(path string) (operatorsettings.Config, error) {
	loaded, err := fake.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if err != nil {
		return operatorsettings.Config{}, err
	}
	return operatorsettings.Config{
		BackendScopeID: loaded.Document.BackendScopeID,
		Defaults: operatorsettings.Defaults{
			WorkerModelProvider: loaded.Document.Defaults.WorkerModelProvider,
			WorkerModel:         loaded.Document.Defaults.WorkerModel,
		},
	}, nil
}

func (fake *fakeSettingsRoot) ResolveFromHomeWithEnvironment(
	homeDir string,
	environment operatorsettings.Defaults,
	flags operatorsettings.FlagOverrides,
) (operatorsettings.ResolvedDefaults, error) {
	configPath := fake.DefaultConfigPath(homeDir)
	config, err := fake.LoadFileConfig(configPath)
	if err != nil {
		return operatorsettings.ResolvedDefaults{}, err
	}
	provider, providerSource := cliWinningDefaultsValue(
		config.Defaults.WorkerModelProvider,
		environment.WorkerModelProvider,
		flags.WorkerModelProvider,
	)
	model, modelSource := cliWinningDefaultsValue(
		config.Defaults.WorkerModel,
		environment.WorkerModel,
		flags.WorkerModel,
	)
	canonicalProvider, ok := canonicalizeProvider(provider)
	if !ok {
		return operatorsettings.ResolvedDefaults{}, operatorsettings.ResolutionFailure{
			Kind:    operatorsettings.ResolutionFailureKindUnsupportedOverride,
			Message: provider,
			Field:   "workerModelProvider",
		}
	}
	if canonicalProvider == "DEFAULT" {
		return operatorsettings.ResolvedDefaults{}, operatorsettings.ResolutionFailure{
			Kind: operatorsettings.ResolutionFailureKindInvalidInput,
			Message: "worker model provider DEFAULT requires a concrete provider from file or environment; " +
				"set defaults.workerModelProvider, YOU_DEFAULT_WORKER_MODEL_PROVIDER, or you run --provider",
			Field: "workerModelProvider",
		}
	}
	return operatorsettings.ResolvedDefaults{
		WorkerModelProvider:       canonicalProvider,
		WorkerModel:               model,
		WorkerModelProviderSource: providerSource,
		WorkerModelSource:         modelSource,
		ConfigPath:                configPath,
	}, nil
}

func (fake *fakeSettingsRoot) EnsureLocalBackendScope(string) (operatorsettings.ResolvedBackendScope, error) {
	return operatorsettings.ResolvedBackendScope{}, errors.New("test settings root does not implement identity")
}

func (fake *fakeSettingsRoot) ProjectInputInventory() operatorsettings.InputInventory {
	return operatorsettings.InputInventory{}
}

func (fake *fakeSettingsRoot) DeriveProviderBackendScopeID(provider, kind, boundary string) string {
	return "provider-" + provider + "-" + kind + "-" + boundary
}

func (fake *fakeSettingsRoot) IsLocalBackendScopeID(string) bool { return false }

func (fake *fakeSettingsRoot) ConfigureACPIntegrationAdd(
	context.Context,
	string,
	operatorsettings.ACPIntegration,
) (operatorsettings.Document, error) {
	return operatorsettings.Document{}, errors.New("test settings root does not implement ACP")
}

func (fake *fakeSettingsRoot) ConfigureACPIntegrationDelete(
	context.Context,
	string,
	string,
) (operatorsettings.Document, error) {
	return operatorsettings.Document{}, errors.New("test settings root does not implement ACP")
}

func (fake *fakeSettingsRoot) EnsurePackagedACPIntegrations(
	context.Context,
	string,
	[]operatorsettings.ACPIntegration,
) (operatorsettings.Document, error) {
	return operatorsettings.Document{}, errors.New("test settings root does not implement ACP")
}

func cliWinningDefaultsValue(fileValue, environmentValue, flagValue string) (string, operatorsettings.Source) {
	switch {
	case strings.TrimSpace(flagValue) != "":
		return strings.TrimSpace(flagValue), operatorsettings.SourceFlag
	case strings.TrimSpace(environmentValue) != "":
		return strings.TrimSpace(environmentValue), operatorsettings.SourceEnv
	case strings.TrimSpace(fileValue) != "":
		return strings.TrimSpace(fileValue), operatorsettings.SourceFile
	default:
		return "", ""
	}
}

func mergeProviderModelUpdate(
	document operatorsettings.Document,
	update operatorsettings.DocumentProviderModelUpdate,
) (operatorsettings.Document, error) {
	if update.Provider != nil {
		provider := strings.TrimSpace(*update.Provider)
		if provider == "" {
			return operatorsettings.Document{}, operatorsettings.DocumentFailure{
				Kind:    operatorsettings.DocumentFailureKindMalformed,
				Message: "worker model provider is required",
			}
		}
		if provider == "unsupported-provider" {
			return operatorsettings.Document{}, operatorsettings.DocumentFailure{
				Kind:    operatorsettings.DocumentFailureKindUnsupported,
				Message: provider,
			}
		}
		if canonical, ok := canonicalizeProvider(provider); ok && canonical != "DEFAULT" {
			document.Defaults.WorkerModelProvider = strings.ToLower(canonical)
		} else {
			document.Defaults.WorkerModelProvider = provider
		}
	}
	if update.Model != nil {
		document.Defaults.WorkerModel = strings.TrimSpace(*update.Model)
	}
	return document, nil
}

func winningLayerValue(
	baselineValue, envValue, flagValue string,
) (string, operatorsettings.EffectiveLayerSource) {
	switch {
	case strings.TrimSpace(flagValue) != "":
		return strings.TrimSpace(flagValue), operatorsettings.EffectiveLayerSourceFlag
	case strings.TrimSpace(envValue) != "":
		return strings.TrimSpace(envValue), operatorsettings.EffectiveLayerSourceEnv
	case strings.TrimSpace(baselineValue) != "":
		return strings.TrimSpace(baselineValue), operatorsettings.EffectiveLayerSourceFile
	default:
		return "", ""
	}
}

func resolveWorkerModelProvider(
	raw string,
	winningSource operatorsettings.EffectiveLayerSource,
	request operatorsettings.ResolveEffectiveRequest,
) (string, error) {
	if raw == "" {
		return "", nil
	}
	canonical, ok := canonicalizeProvider(raw)
	if !ok {
		return "", operatorsettings.ResolutionFailure{
			Kind:    operatorsettings.ResolutionFailureKindUnsupportedOverride,
			Message: raw,
			Field:   "workerModelProvider",
		}
	}
	if canonical != "DEFAULT" {
		return canonical, nil
	}
	concreteRaw := concreteProviderBelowSource(winningSource, request)
	if concreteRaw == "" {
		return "", operatorsettings.ResolutionFailure{
			Kind:    operatorsettings.ResolutionFailureKindInvalidInput,
			Message: "symbolic DEFAULT requires a concrete provider from file or environment",
			Field:   "workerModelProvider",
		}
	}
	concreteCanonical, ok := canonicalizeProvider(concreteRaw)
	if !ok || concreteCanonical == "DEFAULT" {
		return "", operatorsettings.ResolutionFailure{
			Kind:    operatorsettings.ResolutionFailureKindInvalidInput,
			Message: "symbolic DEFAULT requires a concrete provider from file or environment",
			Field:   "workerModelProvider",
		}
	}
	return concreteCanonical, nil
}

func concreteProviderBelowSource(
	winningSource operatorsettings.EffectiveLayerSource,
	request operatorsettings.ResolveEffectiveRequest,
) string {
	type layer struct {
		source operatorsettings.EffectiveLayerSource
		value  string
	}
	layers := []layer{
		{source: operatorsettings.EffectiveLayerSourceFile, value: request.DocumentBaseline.WorkerModelProvider},
		{source: operatorsettings.EffectiveLayerSourceEnv, value: request.EnvironmentOverrides.WorkerModelProvider},
		{source: operatorsettings.EffectiveLayerSourceFlag, value: request.InvocationOverrides.WorkerModelProvider},
	}
	below := make([]layer, 0, 2)
	for _, layer := range layers {
		if layer.source == winningSource {
			break
		}
		below = append(below, layer)
	}
	for i := len(below) - 1; i >= 0; i-- {
		value := strings.TrimSpace(below[i].value)
		if value == "" || strings.EqualFold(value, "DEFAULT") {
			continue
		}
		return value
	}
	return ""
}

func canonicalizeProvider(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	switch strings.ToLower(value) {
	case "", "default":
		return "DEFAULT", true
	case "codex", "openai":
		return "CODEX", true
	case "claude", "anthropic":
		return "CLAUDE", true
	case "gemini":
		return "GEMINI", true
	default:
		if strings.Contains(value, ".") {
			return value, true
		}
		return "", false
	}
}

type testLineReader struct {
	scanner   *bufio.Scanner
	remaining int
}

func testLineReaderFactory(input io.Reader, maxLines int) (operatorsettingscli.ContextLineReader, error) {
	return &testLineReader{scanner: bufio.NewScanner(input), remaining: maxLines}, nil
}

func (reader *testLineReader) ReadLine(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if reader.remaining == 0 || !reader.scanner.Scan() {
		if err := reader.scanner.Err(); err != nil {
			return "", err
		}
		return "", io.EOF
	}
	reader.remaining--
	return reader.scanner.Text(), nil
}
