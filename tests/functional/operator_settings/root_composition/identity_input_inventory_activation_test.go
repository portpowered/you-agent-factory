package root_composition_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	identityActivationGeneratedUUID = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	identityActivationExistingScope = "local-11111111-1111-4111-8111-111111111111"
)

// TestBackendScopeIdentityGeneratesThroughRootBuildProcessAfterLifecycle proves
// backend-scope identity resolution and persistence activate through public
// Operator Settings surfaces after runtime lifecycle on a process composed only
// via root.BuildProcess with edges.Edges effect replacement.
func TestBackendScopeIdentityGeneratesThroughRootBuildProcessAfterLifecycle(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	recorder := newIdentityActivationRecorder(identityActivationGeneratedUUID)
	process := support.BuildProcess(t, recorder.edges())

	if got := recorder.idGeneratorCalls(); got != 0 {
		t.Fatalf("IDGenerator calls during BuildProcess = %d, want 0", got)
	}

	runOperatorSettingsLifecycleInitialization(t, process, homeDir)

	if got := recorder.idGeneratorCalls(); got == 0 {
		t.Fatalf("IDGenerator calls after lifecycle = %d, want > 0 via edges", got)
	}

	wantScope := operatorsettings.LocalBackendScopePrefix + identityActivationGeneratedUUID
	if got := readBackendScopeIDFromHome(t, homeDir); got != wantScope {
		t.Fatalf("backendScopeID = %q, want generated scope %q", got, wantScope)
	}
}

// TestBackendScopeIdentityReusesExistingScopeThroughRootBuildProcessAfterLifecycle
// proves persisted backend-scope identity is reused through public Operator
// Settings surfaces after runtime lifecycle without regenerating scope IDs.
func TestBackendScopeIdentityReusesExistingScopeThroughRootBuildProcessAfterLifecycle(t *testing.T) {
	t.Parallel()

	homeDir := writeOperatorConfigWithBackendScope(t, identityActivationExistingScope)
	recorder := newIdentityActivationRecorder("should-not-be-used")
	process := support.BuildProcess(t, recorder.edges())

	runOperatorSettingsLifecycleInitialization(t, process, homeDir)

	if got := recorder.idGeneratorCalls(); got != 0 {
		t.Fatalf("IDGenerator calls after lifecycle with existing scope = %d, want 0", got)
	}
	if got := readBackendScopeIDFromHome(t, homeDir); got != identityActivationExistingScope {
		t.Fatalf("backendScopeID = %q, want reused scope %q", got, identityActivationExistingScope)
	}
}

type identityActivationRecorder struct {
	operatorSettingsActivationRecorder
	idGenerator atomic.Int32
	generated   string
}

func newIdentityActivationRecorder(generated string) *identityActivationRecorder {
	return &identityActivationRecorder{generated: generated}
}

func (recorder *identityActivationRecorder) edges() serviceedges.Edges {
	edges := recorder.operatorSettingsActivationRecorder.edges()
	edges.OperatorSettingsIDGenerator = recorder.recordIDGenerator
	return edges
}

func (recorder *identityActivationRecorder) idGeneratorCalls() int32 {
	return recorder.idGenerator.Load()
}

func (recorder *identityActivationRecorder) recordIDGenerator() string {
	recorder.idGenerator.Add(1)
	return recorder.generated
}

func runOperatorSettingsLifecycleInitialization(t *testing.T, process support.Process, homeDir string) {
	t.Helper()

	missingFactory := filepath.Join(homeDir, "missing-lifecycle-factory.json")
	err := process.Execute(root.Input{
		Args: []string{"you", "run", "--factory", missingFactory},
		Env: append(
			os.Environ(),
			"HOME="+homeDir,
			"USERPROFILE="+homeDir,
		),
		Context:          t.Context(),
		WorkingDirectory: homeDir,
	})
	if err == nil || !strings.Contains(err.Error(), filepath.Base(missingFactory)) {
		t.Fatalf("Process.Execute(run missing Factory) error = %v, want missing-factory failure", err)
	}
}

func writeOperatorConfigWithBackendScope(t *testing.T, scopeID string) string {
	t.Helper()

	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir operator config directory: %v", err)
	}
	config := []byte(`{
  "backendScopeID": "` + scopeID + `",
  "defaults": {
    "workerModelProvider": "codex",
    "workerModel": "configured-model"
  }
}`)
	configPath := filepath.Join(configDir, "config.json")
	if err := os.WriteFile(configPath, config, 0o600); err != nil {
		t.Fatalf("write operator config: %v", err)
	}
	return homeDir
}

func readBackendScopeIDFromHome(t *testing.T, homeDir string) string {
	t.Helper()

	configPath := filepath.Join(homeDir, ".you-agent-factory", "config.json")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read operator config at %q: %v", configPath, err)
	}
	var document struct {
		BackendScopeID string `json:"backendScopeID"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode operator config: %v\ncontent:\n%s", err, raw)
	}
	return document.BackendScopeID
}
