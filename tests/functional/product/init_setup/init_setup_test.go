package init_setup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

const existingOperatorConfig = `{
  "backendScopeId": "scope-existing",
  "defaults": {
    "workerModelProvider": "claude",
    "workerModel": "old-model"
  },
  "runtime": {
    "logging": {
      "directory": "custom-logs",
      "maxSizeMB": 12,
      "maxBackups": 3,
      "maxAgeDays": 7,
      "compress": true
    },
    "metrics": {
      "directory": "custom-metrics",
      "maxSizeMB": 14,
      "maxBackups": 4,
      "maxAgeDays": 8,
      "compress": false
    }
  },
  "workerPresets": [
    {
      "id": "review",
      "modelProvider": "codex",
      "model": "preset-model"
    }
  ]
}`

// TestInitSuppliedInputsConfigureOnlyProviderModelDefaults proves the public
// root-built command accepts a registered provider and free-form model without
// invoking the legacy Factory scaffold path.
func TestInitSuppliedInputsConfigureOnlyProviderModelDefaults(t *testing.T) {
	fixture := newInitFixture(t)
	var stdout bytes.Buffer

	err := fixture.execute(
		serviceedges.Edges{},
		&stdout,
		"you", "init", "--provider", "codex", "--model", "vendor/free-form:v2",
	)
	if err != nil {
		t.Fatalf("Process.Execute() error = %v", err)
	}

	configured := fixture.readConfig()
	for _, want := range []string{
		`"backendScopeID": "scope-existing"`,
		`"workerModelProvider": "codex"`,
		`"workerModel": "vendor/free-form:v2"`,
		`"directory": "custom-logs"`,
		`"directory": "custom-metrics"`,
		`"id": "review"`,
		`"model": "preset-model"`,
	} {
		if !strings.Contains(configured, want) {
			t.Fatalf("configured document omitted %q:\n%s", want, configured)
		}
	}
	if got := stdout.String(); !strings.Contains(got, "Configured default provider codex and model vendor/free-form:v2") {
		t.Fatalf("stdout = %q, want documented success", got)
	}
	if _, statErr := os.Stat(filepath.Join(fixture.workingDir, "factory")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("legacy scaffold path exists or returned unexpected error: %v", statErr)
	}
}

// TestInitSuppliedInputFailuresDoNotWrite proves public validation and output
// mode failures occur before the atomic operator-config commit.
func TestInitSuppliedInputFailuresDoNotWrite(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing provider",
			args:    []string{"you", "init"},
			wantErr: "use --provider",
		},
		{
			name:    "unregistered provider",
			args:    []string{"you", "init", "--provider", "not-registered"},
			wantErr: `unsupported worker model provider "not-registered"`,
		},
		{
			name:    "json rejected",
			args:    []string{"you", "--json", "init", "--provider", "codex"},
			wantErr: "--json is not supported by you init",
		},
		{
			name:    "empty supplied model",
			args:    []string{"you", "init", "--provider", "codex", "--model", "  "},
			wantErr: "model must be non-empty",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newInitFixture(t)
			err := fixture.execute(serviceedges.Edges{}, io.Discard, test.args...)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Process.Execute() error = %v, want %q", err, test.wantErr)
			}
			if got := fixture.readConfig(); got != existingOperatorConfig {
				t.Fatalf("operator config changed after rejected input:\n%s", got)
			}
		})
	}
}

func TestRetiredInitializationPathsAreRejectedWithoutWrites(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "duplicate config init command",
			args:    []string{"you", "config", "init"},
			wantErr: `unknown command "init" for "you config"`,
		},
		{
			name:    "replacement basic factory command",
			args:    []string{"you", "config", "create-basic-factory"},
			wantErr: `unknown command "create-basic-factory" for "you config"`,
		},
		{
			name:    "legacy scaffold directory",
			args:    []string{"you", "init", "--dir", "legacy-factory"},
			wantErr: "unknown flag: --dir",
		},
		{
			name:    "legacy scaffold type",
			args:    []string{"you", "init", "--type", "ralph"},
			wantErr: "unknown flag: --type",
		},
		{
			name:    "legacy scaffold executor",
			args:    []string{"you", "init", "--executor", "claude"},
			wantErr: "unknown flag: --executor",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newInitFixture(t)
			err := fixture.execute(serviceedges.Edges{}, io.Discard, test.args...)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Process.Execute() error = %v, want %q", err, test.wantErr)
			}
			if got := fixture.readConfig(); got != existingOperatorConfig {
				t.Fatalf("operator config changed after retired input:\n%s", got)
			}
			if _, statErr := os.Stat(filepath.Join(fixture.workingDir, "legacy-factory")); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("retired scaffold path exists or returned unexpected error: %v", statErr)
			}
		})
	}
}

func TestNormalCommandInitializesPackagedFactoriesWithoutSetupCommand(t *testing.T) {
	fixture := newInitFixture(t)
	if err := os.Remove(fixture.configPath); err != nil {
		t.Fatalf("remove seeded operator config: %v", err)
	}
	var stdout bytes.Buffer
	args := []string{
		"you", "--json", "factory", "list", "--dir",
		filepath.Join(fixture.homeDir, ".you-agent-factory", "factories"),
	}
	err := fixture.execute(serviceedges.Edges{}, &stdout, args...)
	if err != nil {
		t.Fatalf("Process.Execute(factory list) error = %v; stdout=%q", err, stdout.String())
	}
	packagedFactory := filepath.Join(
		fixture.homeDir,
		".you-agent-factory",
		"factories",
		"@you",
		"goal",
		"factory.json",
	)
	if _, err := os.Stat(packagedFactory); err != nil {
		t.Fatalf("initializer-owned packaged Factory missing at %s: %v", packagedFactory, err)
	}
	if _, statErr := os.Stat(filepath.Join(fixture.workingDir, "factory")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("normal initialization wrote a legacy working-directory scaffold: %v", statErr)
	}

	firstConfig := fixture.readConfig()
	if err := fixture.execute(serviceedges.Edges{}, io.Discard, args...); err != nil {
		t.Fatalf("Process.Execute(factory list repeat) error = %v", err)
	}
	if got := fixture.readConfig(); got != firstConfig {
		t.Fatalf("repeat initialization rewrote operator config:\nfirst:\n%s\nsecond:\n%s", firstConfig, got)
	}
}

// TestInitSuppliedInputsPreserveConfigWhenAtomicWriteCannotStart proves a
// pre-commit temporary-write failure is returned without replacing the file.
func TestInitSuppliedInputsPreserveConfigWhenAtomicWriteCannotStart(t *testing.T) {
	fixture := newInitFixture(t)
	tempFailure := errors.New("temporary target unavailable")
	edges := serviceedges.Edges{
		OperatorSettingsCreateTemporaryFile: func(string, string) (operatorsettings.TemporaryFile, error) {
			return nil, tempFailure
		},
	}
	err := fixture.execute(edges, io.Discard, "you", "init", "--provider", "codex", "--model", "new-model")
	if err == nil || !strings.Contains(err.Error(), "create operator config temp file") ||
		!errors.Is(err, tempFailure) {
		t.Fatalf("Process.Execute() error = %v, want temporary-file failure", err)
	}
	if got := fixture.readConfig(); got != existingOperatorConfig {
		t.Fatalf("operator config changed after pre-commit failure:\n%s", got)
	}
}

// TestInitInteractiveInputsConfigureOnlyProviderModelDefaults proves terminal
// setup presents current defaults and commits complete prompted input through
// the same atomic settings operation as supplied flags.
func TestInitInteractiveInputsConfigureOnlyProviderModelDefaults(t *testing.T) {
	fixture := newInitFixture(t)
	var stdout bytes.Buffer

	err := fixture.executeInteractive(
		serviceedges.Edges{},
		context.Background(),
		strings.NewReader("codex\nprovider/free-form:v3\n"),
		&stdout,
	)
	if err != nil {
		t.Fatalf("Process.Execute() error = %v", err)
	}
	for _, want := range []string{
		"Provider [claude]:",
		"Model [old-model]:",
		"Configured default provider codex and model provider/free-form:v3",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	configured := fixture.readConfig()
	for _, want := range []string{
		`"backendScopeID": "scope-existing"`,
		`"workerModelProvider": "codex"`,
		`"workerModel": "provider/free-form:v3"`,
		`"directory": "custom-logs"`,
		`"directory": "custom-metrics"`,
		`"model": "preset-model"`,
	} {
		if !strings.Contains(configured, want) {
			t.Fatalf("configured document omitted %q:\n%s", want, configured)
		}
	}
}

func TestInitInteractiveExistingDefaultsAreAccepted(t *testing.T) {
	fixture := newInitFixture(t)
	err := fixture.executeInteractive(
		serviceedges.Edges{},
		context.Background(),
		strings.NewReader("\n\n"),
		io.Discard,
	)
	if err != nil {
		t.Fatalf("Process.Execute() error = %v", err)
	}
	configured := fixture.readConfig()
	if !strings.Contains(configured, `"workerModelProvider": "claude"`) ||
		!strings.Contains(configured, `"workerModel": "old-model"`) {
		t.Fatalf("existing defaults were not retained:\n%s", configured)
	}
}

func TestInitInteractiveRejectedOrTerminatedInputDoesNotWrite(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		input   string
		wantErr string
	}{
		{
			name:    "invalid provider",
			ctx:     context.Background(),
			input:   "not-registered\nfree-form\n",
			wantErr: `unsupported worker model provider "not-registered"`,
		},
		{
			name:    "EOF at model",
			ctx:     context.Background(),
			input:   "codex\n",
			wantErr: "setup canceled",
		},
		{
			name:    "user cancellation at provider",
			ctx:     context.Background(),
			input:   "/cancel\n",
			wantErr: "setup canceled",
		},
		{
			name:    "interrupt at model",
			ctx:     context.Background(),
			input:   "codex\n\x03\n",
			wantErr: "setup canceled",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newInitFixture(t)
			err := fixture.executeInteractive(
				serviceedges.Edges{},
				test.ctx,
				strings.NewReader(test.input),
				io.Discard,
			)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Process.Execute() error = %v, want %q", err, test.wantErr)
			}
			if got := fixture.readConfig(); got != existingOperatorConfig {
				t.Fatalf("operator config changed after rejected prompt:\n%s", got)
			}
		})
	}
}

func TestInitInteractiveContextCancellationAtModelDoesNotWrite(t *testing.T) {
	fixture := newInitFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	input := &blockingAfterProviderReader{
		provider: strings.NewReader("codex\n"),
		release:  release,
	}
	output := &cancelOnModelPromptWriter{cancel: cancel}

	err := fixture.executeInteractive(serviceedges.Edges{}, ctx, input, output)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("Process.Execute() error = %v, want context cancellation", err)
	}
	if got := fixture.readConfig(); got != existingOperatorConfig {
		t.Fatalf("operator config changed after context cancellation:\n%s", got)
	}
}

func TestInitInteractivePreCommitFailurePreservesConfig(t *testing.T) {
	fixture := newInitFixture(t)
	tempFailure := errors.New("prompted temporary target unavailable")
	edges := serviceedges.Edges{
		OperatorSettingsCreateTemporaryFile: func(string, string) (operatorsettings.TemporaryFile, error) {
			return nil, tempFailure
		},
	}
	err := fixture.executeInteractive(
		edges,
		context.Background(),
		strings.NewReader("codex\nnew-model\n"),
		io.Discard,
	)
	if err == nil || !errors.Is(err, tempFailure) {
		t.Fatalf("Process.Execute() error = %v, want temporary-file failure", err)
	}
	if got := fixture.readConfig(); got != existingOperatorConfig {
		t.Fatalf("operator config changed after prompted pre-commit failure:\n%s", got)
	}
}

type initFixture struct {
	t          *testing.T
	homeDir    string
	workingDir string
	configPath string
}

func newInitFixture(t *testing.T) initFixture {
	t.Helper()
	homeDir := t.TempDir()
	workingDir := t.TempDir()
	configPath := filepath.Join(homeDir, ".you-agent-factory", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config directory): %v", err)
	}
	if err := os.WriteFile(configPath, []byte(existingOperatorConfig), 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}
	return initFixture{t: t, homeDir: homeDir, workingDir: workingDir, configPath: configPath}
}

func (fixture initFixture) execute(
	edges serviceedges.Edges,
	stdout io.Writer,
	args ...string,
) error {
	fixture.t.Helper()
	process, err := root.BuildProcess(fixture.t.Context(), edges)
	if err != nil {
		return err
	}
	return process.Execute(root.Input{
		Args: args,
		Env: append(
			os.Environ(),
			"HOME="+fixture.homeDir,
			"USERPROFILE="+fixture.homeDir,
		),
		Stdin:            strings.NewReader(""),
		Stdout:           stdout,
		Stderr:           io.Discard,
		Context:          fixture.t.Context(),
		WorkingDirectory: fixture.workingDir,
	})
}

func (fixture initFixture) executeInteractive(
	edges serviceedges.Edges,
	ctx context.Context,
	stdin io.Reader,
	stdout io.Writer,
) error {
	fixture.t.Helper()
	process, err := root.BuildProcess(fixture.t.Context(), edges)
	if err != nil {
		return err
	}
	isTTY := true
	return process.Execute(root.Input{
		Args: []string{"you", "init"},
		Env: append(
			os.Environ(),
			"HOME="+fixture.homeDir,
			"USERPROFILE="+fixture.homeDir,
		),
		Stdin:            stdin,
		Stdout:           stdout,
		Stderr:           io.Discard,
		Context:          ctx,
		WorkingDirectory: fixture.workingDir,
		StdinIsTTY:       &isTTY,
		StdoutIsTTY:      &isTTY,
	})
}

func (fixture initFixture) readConfig() string {
	fixture.t.Helper()
	payload, err := os.ReadFile(fixture.configPath)
	if err != nil {
		fixture.t.Fatalf("ReadFile(config): %v", err)
	}
	return string(payload)
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
