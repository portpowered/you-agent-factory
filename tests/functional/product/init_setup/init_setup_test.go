package init_setup

import (
	"bytes"
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

func (fixture initFixture) readConfig() string {
	fixture.t.Helper()
	payload, err := os.ReadFile(fixture.configPath)
	if err != nil {
		fixture.t.Fatalf("ReadFile(config): %v", err)
	}
	return string(payload)
}
