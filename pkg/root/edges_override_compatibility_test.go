package root

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// TestBuildProcessAppliesTypedEdgesExternalEffectOverride proves the approved
// root.BuildProcess edges.Edges seam still replaces a representative external
// effect after construction (operator identity generation during normal
// initialization).
func TestBuildProcessAppliesTypedEdgesExternalEffectOverride(t *testing.T) {
	t.Parallel()

	const generated = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	home := t.TempDir()
	process, err := BuildProcess(context.Background(), serviceedges.Edges{
		OperatorSettingsIDGenerator: func() string { return generated },
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	configPath := runNormalInitialization(t, process, home)
	persisted := readBackendScopeID(t, configPath)
	want := operatorsettings.LocalBackendScopePrefix + generated
	if persisted != want {
		t.Fatalf("backendScopeID = %q, want edges override %q", persisted, want)
	}
}

// TestBuildProcessEmptyEdgesSelectProductionExternalEffectDefaults proves {}
// edges keep production defaults for the same representative effect.
func TestBuildProcessEmptyEdgesSelectProductionExternalEffectDefaults(t *testing.T) {
	t.Parallel()

	const overrideSentinel = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	home := t.TempDir()
	process, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	configPath := runNormalInitialization(t, process, home)
	persisted := readBackendScopeID(t, configPath)
	if !operatorsettings.IsLocalBackendScopeID(persisted) {
		t.Fatalf("backendScopeID = %q, want production local-<uuid> default", persisted)
	}
	if persisted == operatorsettings.LocalBackendScopePrefix+overrideSentinel {
		t.Fatalf("empty edges unexpectedly used override sentinel backendScopeID %q", persisted)
	}
}

// TestBuildProcessKeepsFunctionalTypedEdgesReplacementCompatible proves
// functional-style typed edges.Edges replacements still construct through the
// same BuildProcess bag without an alternate override seam, and that a
// filesystem replacement is applied during normal initialization.
func TestBuildProcessKeepsFunctionalTypedEdgesReplacementCompatible(t *testing.T) {
	t.Parallel()

	var initializationCalls atomic.Int32
	apiStarts := 0
	initializationErr := errors.New("system initialization override selected")
	process, err := BuildProcess(context.Background(), serviceedges.Edges{
		SystemInitializationInspectPath: func(string) (fs.FileInfo, error) {
			initializationCalls.Add(1)
			return nil, initializationErr
		},
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			apiStarts++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	if apiStarts != 0 || initializationCalls.Load() != 0 {
		t.Fatalf(
			"construction side effects = api:%d initialization:%d, want zero",
			apiStarts,
			initializationCalls.Load(),
		)
	}

	home := t.TempDir()
	var stderr bytes.Buffer
	err = process.Execute(Input{
		Args: []string{
			"you", "--json", "factory", "list", "--dir",
			filepath.Join(home, ".you-agent-factory", "factories"),
		},
		Env:              homeEnvironment(home),
		Stderr:           &stderr,
		Context:          context.Background(),
		WorkingDirectory: home,
	})
	if err == nil || !strings.Contains(err.Error(), initializationErr.Error()) {
		t.Fatalf(
			"Process.Execute(factory list) error = %v stderr=%q, want initialization override %v",
			err,
			stderr.String(),
			initializationErr,
		)
	}
	if initializationCalls.Load() == 0 {
		t.Fatal("SystemInitializationInspectPath override was not used after construction")
	}
	if apiStarts != 0 {
		t.Fatalf("APIServerStarter calls = %d during factory list, want 0", apiStarts)
	}
}

func runNormalInitialization(t *testing.T, process *initializerapplication.Process, home string) string {
	t.Helper()

	var output bytes.Buffer
	if err := process.Execute(Input{
		Args: []string{
			"you", "--json", "factory", "list", "--dir",
			filepath.Join(home, ".you-agent-factory", "factories"),
		},
		Env:              homeEnvironment(home),
		Stdout:           &output,
		Context:          context.Background(),
		WorkingDirectory: home,
	}); err != nil {
		t.Fatalf("Process.Execute(factory list) error = %v; stdout=%q", err, output.String())
	}
	return filepath.Join(home, ".you-agent-factory", "config.json")
}

func readBackendScopeID(t *testing.T, configPath string) string {
	t.Helper()

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", configPath, err)
	}
	var document struct {
		BackendScopeID string `json:"backendScopeID"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode operator config: %v\ncontent:\n%s", err, raw)
	}
	return document.BackendScopeID
}
