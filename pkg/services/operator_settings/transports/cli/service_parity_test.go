package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	internaltestproviders "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testproviders"
	operatorsettingscli "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/cli"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
)

func constructedSettingsCLIService(
	t *testing.T,
	root operatorsettings.Service,
) operatorsettingscli.Service {
	t.Helper()
	service := operatorsettingscli.New(root)
	if service == nil {
		t.Fatal("New(root) = nil, want Settings CLI service")
	}
	return service
}

func assertConfigureParity(
	t *testing.T,
	service operatorsettingscli.Service,
	root operatorsettings.Service,
	cfg operatorsettingscli.ConfigureConfig,
	input string,
	beforeEach func(),
	after func(serviceOut string, serviceErr error),
) {
	t.Helper()

	newCfg := func() operatorsettingscli.ConfigureConfig {
		invocation := cfg
		if cfg.Interactive || input != "" {
			invocation.Input = strings.NewReader(input)
		}
		invocation.Output = &bytes.Buffer{}
		return invocation
	}
	run := func(invoke func(operatorsettingscli.ConfigureConfig) error) (*bytes.Buffer, error) {
		if beforeEach != nil {
			beforeEach()
		}
		invocation := newCfg()
		return invocation.Output.(*bytes.Buffer), invoke(invocation)
	}

	serviceOut, serviceErr := run(service.Configure)
	commandOut, commandErr := run(func(invocation operatorsettingscli.ConfigureConfig) error {
		return operatorsettingscli.Configure(invocation, root)
	})

	if (serviceErr == nil) != (commandErr == nil) {
		t.Fatalf("service error = %v, command error = %v", serviceErr, commandErr)
	}
	if serviceErr != nil && commandErr != nil {
		if !errors.Is(serviceErr, commandErr) && serviceErr.Error() != commandErr.Error() {
			t.Fatalf("service error = %q, command error = %q", serviceErr.Error(), commandErr.Error())
		}
	}
	if serviceOut.String() != commandOut.String() {
		t.Fatalf("service output = %q, command output = %q", serviceOut.String(), commandOut.String())
	}
	if after != nil {
		after(serviceOut.String(), serviceErr)
	}
}

func TestConstructedService_ConfigureReportsCanonicalPersistedDefaultsParity(t *testing.T) {
	t.Parallel()

	root := paritySettingsRoot(t)
	service := constructedSettingsCLIService(t, root)
	homeDir := t.TempDir()
	model := "  free-form/model:v1  "
	cfg := operatorsettingscli.ConfigureConfig{
		Context:  context.Background(),
		HomeDir:  homeDir,
		Provider: "CODEX",
		Model:    &model,
	}

	assertConfigureParity(t, service, root, cfg, "", nil, func(output string, err error) {
		if err != nil {
			t.Fatalf("Configure() error = %v", err)
		}
		for _, want := range []string{
			"codex",
			"free-form/model:v1",
			operatorsettings.DefaultConfigPath(homeDir),
		} {
			if !strings.Contains(output, want) {
				t.Fatalf("output = %q, want %q", output, want)
			}
		}
	})
}

func TestConstructedService_ConfigureRejectsEmptySuppliedModelParity(t *testing.T) {
	t.Parallel()

	root := paritySettingsRoot(t)
	service := constructedSettingsCLIService(t, root)
	model := "  "
	cfg := operatorsettingscli.ConfigureConfig{
		Context:  context.Background(),
		HomeDir:  t.TempDir(),
		Provider: "codex",
		Model:    &model,
	}

	assertConfigureParity(t, service, root, cfg, "", nil, func(output string, err error) {
		if err == nil || !strings.Contains(err.Error(), "model must be non-empty") {
			t.Fatalf("Configure() error = %v, want empty-model rejection", err)
		}
		if output != "" {
			t.Fatalf("output = %q, want empty on validation failure", output)
		}
	})
}

func TestConstructedService_ConfigureRejectsMissingProviderParity(t *testing.T) {
	t.Parallel()

	root := paritySettingsRoot(t)
	service := constructedSettingsCLIService(t, root)
	cfg := operatorsettingscli.ConfigureConfig{
		Context: context.Background(),
		HomeDir: t.TempDir(),
	}

	assertConfigureParity(t, service, root, cfg, "", nil, func(output string, err error) {
		if err == nil || !strings.Contains(err.Error(), "use --provider") {
			t.Fatalf("Configure() error = %v, want supplied-provider guidance", err)
		}
		if output != "" {
			t.Fatalf("output = %q, want empty", output)
		}
	})
}

func TestConstructedService_ConfigurePromptsWithExistingDefaultsParity(t *testing.T) {
	t.Parallel()

	root := paritySettingsRoot(t)
	service := constructedSettingsCLIService(t, root)
	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	originalConfig := []byte(`{"defaults":{"workerModelProvider":"codex","workerModel":"existing-model"}}`)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config): %v", err)
	}
	resetConfig := func() {
		if err := os.WriteFile(configPath, originalConfig, 0o600); err != nil {
			t.Fatalf("WriteFile(config): %v", err)
		}
	}
	resetConfig()
	cfg := operatorsettingscli.ConfigureConfig{
		Context:       context.Background(),
		HomeDir:       homeDir,
		Interactive:   true,
		NewLineReader: testLineReaderFactory,
	}

	assertConfigureParity(t, service, root, cfg, "\nfree-form/model:v2\n", resetConfig, func(output string, err error) {
		if err != nil {
			t.Fatalf("Configure() error = %v", err)
		}
		for _, want := range []string{
			"Provider [codex]:",
			"Model [existing-model]:",
			"Configured default provider codex and model free-form/model:v2",
		} {
			if !strings.Contains(output, want) {
				t.Fatalf("output = %q, want %q", output, want)
			}
		}
	})
}

func TestConstructedService_ConfigurePromptTerminationParity(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		input   string
		cancel  bool
		wantErr error
	}{
		{name: "EOF at provider", wantErr: operatorsettings.ErrProviderModelInputCanceled},
		{name: "user cancellation at provider", input: "/cancel\n", wantErr: operatorsettings.ErrProviderModelInputCanceled},
		{name: "interrupt at model", input: "codex\n\x03\n", wantErr: operatorsettings.ErrProviderModelInputCanceled},
		{name: "context cancellation", input: "codex\nmodel\n", cancel: true, wantErr: context.Canceled},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := paritySettingsRoot(t)
			service := constructedSettingsCLIService(t, root)
			homeDir := t.TempDir()
			configPath := operatorsettings.DefaultConfigPath(homeDir)
			if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
				t.Fatalf("MkdirAll(config): %v", err)
			}
			original := []byte(`{"defaults":{"workerModelProvider":"codex","workerModel":"original"}}`)
			resetConfig := func() {
				if err := os.WriteFile(configPath, original, 0o600); err != nil {
					t.Fatalf("WriteFile(config): %v", err)
				}
			}
			resetConfig()
			ctx, cancel := context.WithCancel(context.Background())
			if test.cancel {
				cancel()
			} else {
				defer cancel()
			}
			cfg := operatorsettingscli.ConfigureConfig{
				Context:       ctx,
				HomeDir:       homeDir,
				Interactive:   true,
				NewLineReader: testLineReaderFactory,
			}

			assertConfigureParity(t, service, root, cfg, test.input, resetConfig, func(_ string, err error) {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("Configure() error = %v, want %v", err, test.wantErr)
				}
				got, readErr := os.ReadFile(configPath)
				if readErr != nil {
					t.Fatalf("ReadFile(config): %v", readErr)
				}
				if !bytes.Equal(got, original) {
					t.Fatalf("config changed after prompt termination:\n%s", got)
				}
			})
		})
	}
}

func TestConstructedService_ConfigureRejectsPromptedInvalidProviderParity(t *testing.T) {
	t.Parallel()

	root := paritySettingsRoot(t)
	service := constructedSettingsCLIService(t, root)
	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config): %v", err)
	}
	original := []byte(`{"defaults":{"workerModelProvider":"codex","workerModel":"original"}}`)
	resetConfig := func() {
		if err := os.WriteFile(configPath, original, 0o600); err != nil {
			t.Fatalf("WriteFile(config): %v", err)
		}
	}
	resetConfig()
	cfg := operatorsettingscli.ConfigureConfig{
		Context:       context.Background(),
		HomeDir:       homeDir,
		Interactive:   true,
		NewLineReader: testLineReaderFactory,
	}

	assertConfigureParity(t, service, root, cfg, "unregistered\nfree-form-model\n", resetConfig, func(_ string, err error) {
		if err == nil || !strings.Contains(err.Error(), `unsupported worker model provider "unregistered"`) {
			t.Fatalf("Configure() error = %v, want provider rejection", err)
		}
		got, readErr := os.ReadFile(configPath)
		if readErr != nil {
			t.Fatalf("ReadFile(config): %v", readErr)
		}
		if !bytes.Equal(got, original) {
			t.Fatalf("config changed after invalid provider:\n%s", got)
		}
	})
}

func TestConstructedService_ConfigureSurfacesDocumentConflictParity(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	root := newConflictSettingsRoot(map[string]operatorsettings.Document{
		configPath: {
			BackendScopeID: "scope-1",
			Defaults: operatorsettings.DocumentDefaults{
				WorkerModelProvider: "codex",
				WorkerModel:         "gpt-5",
			},
			Runtime: operatorsettings.EmptyDocument.Runtime,
		},
	})
	service := constructedSettingsCLIService(t, root)
	model := "replacement-model"
	cfg := operatorsettingscli.ConfigureConfig{
		Context:  context.Background(),
		HomeDir:  homeDir,
		Provider: "codex",
		Model:    &model,
	}

	assertConfigureParity(t, service, root, cfg, "", nil, func(output string, err error) {
		if !errors.Is(err, operatorsettings.ErrDocumentConflict) {
			t.Fatalf("Configure() error = %v, want ErrDocumentConflict", err)
		}
		if output != "" {
			t.Fatalf("output = %q, want empty on document failure", output)
		}
	})
}

func TestConstructedService_ConfigureHonorsMidPromptContextCancellationParity(t *testing.T) {
	t.Parallel()

	root := paritySettingsRoot(t)
	service := constructedSettingsCLIService(t, root)
	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config): %v", err)
	}
	original := []byte(`{"defaults":{"workerModelProvider":"codex","workerModel":"original"}}`)
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	cfg := operatorsettingscli.ConfigureConfig{
		Context:       ctx,
		HomeDir:       homeDir,
		Interactive:   true,
		Input:         &blockingAfterProviderReader{provider: strings.NewReader("codex\n"), release: release},
		Output:        &cancelOnModelPromptWriter{cancel: cancel},
		NewLineReader: testLineReaderFactory,
	}

	serviceErr := service.Configure(cfg)
	commandErr := operatorsettingscli.Configure(cfg, root)
	if (serviceErr == nil) != (commandErr == nil) {
		t.Fatalf("service error = %v, command error = %v", serviceErr, commandErr)
	}
	for _, err := range []error{serviceErr, commandErr} {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Configure() error = %v, want context.Canceled", err)
		}
	}
	got, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("ReadFile(config): %v", readErr)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("config changed after mid-prompt cancellation:\n%s", got)
	}
}

func TestConstructedService_ConfigureHonorsContextCancellationOnSuppliedProviderPathParity(t *testing.T) {
	t.Parallel()

	root := paritySettingsRoot(t)
	service := constructedSettingsCLIService(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	model := "gpt-test"
	cfg := operatorsettingscli.ConfigureConfig{
		Context:  ctx,
		HomeDir:  t.TempDir(),
		Provider: "codex",
		Model:    &model,
	}

	assertConfigureParity(t, service, root, cfg, "", nil, func(output string, err error) {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Configure() error = %v, want context.Canceled", err)
		}
		if output != "" {
			t.Fatalf("output = %q, want empty on cancellation", output)
		}
	})
}

func paritySettingsRoot(t *testing.T) operatorsettings.Service {
	t.Helper()
	root, err := settingswire.NewServiceFromConfigDocument(
		parityTestConfigService(),
		internaltestproviders.StandardCatalog(),
	)
	if err != nil {
		t.Fatalf("NewServiceFromConfigDocument() error = %v", err)
	}
	return root
}

func parityTestConfigService() operatorsettings.ConfigDocumentService {
	return settingswire.NewConfigDocumentService(
		platformfilesystem.Local{},
		func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		globalconfigmapping.Decode,
		globalconfigmapping.Encode,
		parityTestProviderCatalog,
		&sync.Mutex{},
	)
}

func parityTestProviderCatalog(value string) (string, bool) {
	if strings.EqualFold(strings.TrimSpace(value), "codex") {
		return "codex", true
	}
	return "", false
}

type conflictSettingsRoot struct {
	*fakeSettingsRoot
}

func newConflictSettingsRoot(
	entries map[string]operatorsettings.Document,
) *conflictSettingsRoot {
	return &conflictSettingsRoot{fakeSettingsRoot: newFakeSettingsRoot(entries)}
}

type blockingAfterProviderReader struct {
	provider *strings.Reader
	release  <-chan struct{}
}

func (reader *blockingAfterProviderReader) Read(payload []byte) (int, error) {
	if reader.provider.Len() > 0 {
		return reader.provider.Read(payload)
	}
	<-reader.release
	return 0, io.EOF
}

type cancelOnModelPromptWriter struct {
	cancel context.CancelFunc
}

func (writer *cancelOnModelPromptWriter) Write(payload []byte) (int, error) {
	if strings.Contains(string(payload), "Model") {
		writer.cancel()
	}
	return len(payload), nil
}

func (root *conflictSettingsRoot) ApplyDocumentUpdate(
	request operatorsettings.ApplyDocumentUpdateRequest,
) (operatorsettings.ApplyDocumentUpdateResult, error) {
	path := strings.TrimSpace(request.Path)
	return operatorsettings.ApplyDocumentUpdateResult{}, operatorsettings.DocumentFailure{
		Kind:    operatorsettings.DocumentFailureKindConflict,
		Message: "backend scope mismatch",
		Path:    path,
	}
}
