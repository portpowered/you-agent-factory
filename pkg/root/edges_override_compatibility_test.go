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
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// TestBuildProcessAppliesTypedEdgesExternalEffectOverride proves the approved
// root.BuildProcess edges.Edges seam still replaces a representative external
// effect after construction (operator identity generation via config init).
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

	configPath := runConfigInit(t, process, home)
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

	configPath := runConfigInit(t, process, home)
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
// filesystem replacement is applied when scaffolding.
func TestBuildProcessKeepsFunctionalTypedEdgesReplacementCompatible(t *testing.T) {
	t.Parallel()

	var scaffoldCalls atomic.Int32
	apiStarts := 0
	scaffoldErr := errors.New("scaffold filesystem override selected")
	process, err := BuildProcess(context.Background(), serviceedges.Edges{
		FactoryDefinitionScaffoldFileSystem: countingScaffoldFileSystem{
			calls: &scaffoldCalls,
			err:   scaffoldErr,
		},
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			apiStarts++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	if apiStarts != 0 || scaffoldCalls.Load() != 0 {
		t.Fatalf(
			"construction side effects = api:%d scaffold:%d, want zero",
			apiStarts,
			scaffoldCalls.Load(),
		)
	}

	home := t.TempDir()
	factoryDir := filepath.Join(t.TempDir(), "override-factory")
	var stderr bytes.Buffer
	err = process.Execute(Input{
		Args:             []string{"you", "init", "--dir", factoryDir},
		Env:              homeEnvironment(home),
		Stderr:           &stderr,
		Context:          context.Background(),
		WorkingDirectory: filepath.Dir(factoryDir),
	})
	if err == nil || !strings.Contains(err.Error(), scaffoldErr.Error()) {
		t.Fatalf(
			"Process.Execute(init) error = %v stderr=%q, want scaffold override %v",
			err,
			stderr.String(),
			scaffoldErr,
		)
	}
	if scaffoldCalls.Load() == 0 {
		t.Fatal("FactoryDefinitionScaffoldFileSystem override was not used after construction")
	}
	if apiStarts != 0 {
		t.Fatalf("APIServerStarter calls = %d during init, want 0", apiStarts)
	}
}

func runConfigInit(t *testing.T, process *initializerapplication.Process, home string) string {
	t.Helper()

	var output bytes.Buffer
	if err := process.Execute(Input{
		Args:             []string{"you", "config", "init", "--json"},
		Env:              homeEnvironment(home),
		Stdout:           &output,
		Context:          context.Background(),
		WorkingDirectory: home,
	}); err != nil {
		t.Fatalf("Process.Execute(config init) error = %v; stdout=%q", err, output.String())
	}
	var outcome struct {
		ConfigPath string `json:"configPath"`
	}
	if err := json.Unmarshal(output.Bytes(), &outcome); err != nil {
		t.Fatalf("decode config init output: %v\noutput:\n%s", err, output.String())
	}
	if outcome.ConfigPath == "" {
		t.Fatal("config init omitted configPath")
	}
	return outcome.ConfigPath
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

type countingScaffoldFileSystem struct {
	calls *atomic.Int32
	err   error
}

var _ factorydefinitions.ScaffoldFileSystem = countingScaffoldFileSystem{}

func (fileSystem countingScaffoldFileSystem) Stat(string) (fs.FileInfo, error) {
	fileSystem.calls.Add(1)
	return nil, fileSystem.err
}

func (fileSystem countingScaffoldFileSystem) MkdirAll(string, fs.FileMode) error {
	fileSystem.calls.Add(1)
	return fileSystem.err
}

func (fileSystem countingScaffoldFileSystem) WriteFile(string, []byte, fs.FileMode) error {
	fileSystem.calls.Add(1)
	return fileSystem.err
}
