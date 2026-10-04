package initsetup_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorsettingscli "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/cli/initsetup"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

func TestConfigurerRequiresSuppliedProviderBeforePersistence(t *testing.T) {
	var output bytes.Buffer
	err := initsetup.NewConfigurer(nil, nil, testLineReaderFactory)(
		initsetup.Config{
			Context: context.Background(),
			HomeDir: t.TempDir(),
			Output:  &output,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "use --provider") {
		t.Fatalf("configure error = %v, want supplied-provider guidance", err)
	}
	if output.Len() != 0 {
		t.Fatalf("output = %q, want empty", output.String())
	}
}

func TestConfigurerRejectsEmptySuppliedModelBeforePersistence(t *testing.T) {
	model := "  "
	err := initsetup.NewConfigurer(nil, nil, testLineReaderFactory)(
		initsetup.Config{
			Context:  context.Background(),
			HomeDir:  t.TempDir(),
			Provider: "codex",
			Model:    &model,
			Output:   &bytes.Buffer{},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "model must be non-empty") {
		t.Fatalf("configure error = %v, want empty-model rejection", err)
	}
}

func TestConfigurerReportsCanonicalPersistedDefaults(t *testing.T) {
	var output bytes.Buffer
	model := "  free-form/model:v1  "
	service := testConfigService()
	homeDir := t.TempDir()
	err := initsetup.NewConfigurer(service, settingswire.NewCLIService(service), testLineReaderFactory)(initsetup.Config{
		Context:  context.Background(),
		HomeDir:  homeDir,
		Provider: "CODEX",
		Model:    &model,
		Output:   &output,
	})
	if err != nil {
		t.Fatalf("configure error = %v", err)
	}
	for _, want := range []string{
		"codex",
		"free-form/model:v1",
		filepath.Join(homeDir, ".you-agent-factory", "config.json"),
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output = %q, want %q", output.String(), want)
		}
	}
}

func TestConfigurerPromptsWithExistingDefaultsAndPersistsCompleteInput(t *testing.T) {
	var output bytes.Buffer
	homeDir := t.TempDir()
	configPath := filepath.Join(homeDir, ".you-agent-factory", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config): %v", err)
	}
	if err := os.WriteFile(
		configPath,
		[]byte(`{"defaults":{"workerModelProvider":"codex","workerModel":"existing-model"}}`),
		0o600,
	); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}
	service := testConfigService()

	err := initsetup.NewConfigurer(service, settingswire.NewCLIService(service), testLineReaderFactory)(initsetup.Config{
		Context:     context.Background(),
		HomeDir:     homeDir,
		Input:       strings.NewReader("\nfree-form/model:v2\n"),
		Output:      &output,
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("configure error = %v", err)
	}
	for _, want := range []string{
		"Provider [codex]:",
		"Model [existing-model]:",
		"Configured default provider codex and model free-form/model:v2",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output = %q, want %q", output.String(), want)
		}
	}
}

func TestConfigurerPromptTerminationDoesNotPersist(t *testing.T) {
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
		t.Run(test.name, func(t *testing.T) {
			homeDir := t.TempDir()
			configPath := filepath.Join(homeDir, ".you-agent-factory", "config.json")
			if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
				t.Fatalf("MkdirAll(config): %v", err)
			}
			original := []byte(`{"defaults":{"workerModelProvider":"codex","workerModel":"original"}}`)
			if err := os.WriteFile(configPath, original, 0o600); err != nil {
				t.Fatalf("WriteFile(config): %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			if test.cancel {
				cancel()
			} else {
				defer cancel()
			}
			err := testConfigurer()(initsetup.Config{
				Context:     ctx,
				HomeDir:     homeDir,
				Input:       strings.NewReader(test.input),
				Output:      &bytes.Buffer{},
				Interactive: true,
			})
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("configure error = %v, want %v", err, test.wantErr)
			}
			got, readErr := os.ReadFile(configPath)
			if readErr != nil {
				t.Fatalf("ReadFile(config): %v", readErr)
			}
			if !bytes.Equal(got, original) {
				t.Fatalf("config changed after prompt termination:\n%s", got)
			}
		})
	}
}

func TestConfigurerRejectsPromptedInvalidProviderWithoutPersisting(t *testing.T) {
	homeDir := t.TempDir()
	configPath := filepath.Join(homeDir, ".you-agent-factory", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config): %v", err)
	}
	original := []byte(`{"defaults":{"workerModelProvider":"codex","workerModel":"original"}}`)
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}
	err := testConfigurer()(initsetup.Config{
		Context:     context.Background(),
		HomeDir:     homeDir,
		Input:       strings.NewReader("unregistered\nfree-form-model\n"),
		Output:      &bytes.Buffer{},
		Interactive: true,
	})
	if err == nil || !strings.Contains(err.Error(), `unsupported worker model provider "unregistered"`) {
		t.Fatalf("configure error = %v, want provider rejection", err)
	}
	got, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("ReadFile(config): %v", readErr)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("config changed after invalid provider:\n%s", got)
	}
}

func testConfigService() operatorsettings.Service {
	providersRoot, err := providerswire.NewService(providerswire.IdentityCatalogProbe,
		platformclock.Real{},
		platformclock.Real{},
		logging.NoopLogger{}, nil, nil, nil)
	if err != nil {
		panic(err)
	}
	files := platformfilesystem.Local{}
	createTemp := operatorsettings.CreateTemporaryFile(func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
		return os.CreateTemp(dir, pattern)
	})
	document := settingswire.NewDocumentService(files, createTemp, globalconfigmapping.Decode,
		globalconfigmapping.Encode, testProviderCatalog, globalconfigmapping.PreserveUnknownFields,
		globalconfigmapping.DecodeWithDiagnostics)
	resolution, err := settingswire.NewResolutionService(providersRoot)
	if err != nil {
		panic(err)
	}
	service, err := settingswire.NewService(document, resolution, files, createTemp,
		globalconfigmapping.Decode, globalconfigmapping.Encode,
		func() string { return "00000000-0000-4000-8000-000000000001" },
		logging.NoopLogger{}, globalconfigmapping.DecodeWithDiagnostics)
	if err != nil {
		panic(err)
	}
	return service
}

type testLineReader struct {
	scanner   *bufio.Scanner
	remaining int
}

func testLineReaderFactory(input io.Reader, maxLines int) (initsetup.ContextLineReader, error) {
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

func testProviderCatalog(value string) (string, bool) {
	if strings.EqualFold(strings.TrimSpace(value), "codex") {
		return "codex", true
	}
	return "", false
}

func testConfigurer() func(initsetup.Config) error {
	root := testConfigService()
	return initsetup.NewConfigurer(root, settingswire.NewCLIService(root), testLineReaderFactory)
}

// The supplied adapter owns rendering and failure propagation. An inert root
// cannot service configure requests, so reacquiring an adapter would fail here.
func TestConfigurerUsesInjectedAdapter(t *testing.T) {
	t.Parallel()
	failure := errors.New("controlled adapter failure")
	adapter := &recordingCLIAdapter{failure: failure}
	root := &inertSettingsRoot{}
	var output bytes.Buffer
	model := "model-v1"
	home := t.TempDir()
	err := initsetup.NewConfigurer(root, adapter, testLineReaderFactory)(initsetup.Config{
		Context: t.Context(), HomeDir: home, Provider: "codex", Model: &model, Output: &output,
	})
	if !errors.Is(err, failure) {
		t.Fatalf("Configure error = %v, want injected failure", err)
	}
	if adapter.calls != 1 || adapter.config.HomeDir != home || adapter.config.Model != &model ||
		adapter.config.Provider != "codex" || adapter.config.Output != &output || adapter.config.Context != t.Context() {
		t.Fatalf("injected adapter received %d calls with %#v", adapter.calls, adapter.config)
	}
	if output.Len() != 0 {
		t.Fatalf("false success output = %q", output.String())
	}
}

type inertSettingsRoot struct{ operatorsettings.Service }
type recordingCLIAdapter struct {
	operatorsettingscli.Service
	calls   int
	config  operatorsettingscli.ConfigureConfig
	failure error
}

func (adapter *recordingCLIAdapter) Configure(config operatorsettingscli.ConfigureConfig) error {
	adapter.calls++
	adapter.config = config
	return adapter.failure
}
