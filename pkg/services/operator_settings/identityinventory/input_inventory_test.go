package identityinventory_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/operator_settings/globalconfiginventory"
	identityinventory "github.com/portpowered/infinite-you/pkg/services/operator_settings/identityinventory"
)

const fixturesRelativeDir = "pkg/services/operator_settings/identityinventory/testdata/fixtures"

func TestProjectInputInventory_RecordsTolerantUnknownFieldPolicy(t *testing.T) {
	t.Parallel()

	inventory := identityinventory.ProjectInputInventory()
	if inventory.FormatVersion != identityinventory.InputInventoryFormatVersion {
		t.Fatalf("FormatVersion = %q, want %q", inventory.FormatVersion, identityinventory.InputInventoryFormatVersion)
	}
	if !strings.Contains(inventory.UnknownFieldPolicy, "ignores other top-level keys on read") {
		t.Fatalf("unknown field policy = %q, want tolerant load reference", inventory.UnknownFieldPolicy)
	}
	if !strings.Contains(inventory.SiblingPreservation, "preserves defaults") {
		t.Fatalf("sibling preservation = %q, want defaults preservation note", inventory.SiblingPreservation)
	}
}

func TestProjectInputInventory_HasTolerantSiblingCase(t *testing.T) {
	t.Parallel()

	inventory := identityinventory.ProjectInputInventory()
	for _, inputCase := range inventory.Cases {
		if inputCase.Category != "tolerant-sibling" || inputCase.Outcome != "accept" {
			continue
		}
		if inputCase.Fixture == "" {
			t.Fatalf("tolerant-sibling case %q missing fixture", inputCase.ID)
		}
		return
	}
	t.Fatal("missing tolerant-sibling accept case in input inventory")
}

func TestIndexedEnsureScopeCases_MatchProductionLoader(t *testing.T) {
	inventory := identityinventory.ProjectInputInventory()
	seen := make(map[string]struct{}, len(inventory.Cases))
	for _, inputCase := range inventory.Cases {
		if inputCase.Entrypoint != "EnsureLocalBackendScope" {
			continue
		}
		if inputCase.ID == "" {
			t.Fatal("input case missing id")
		}
		if _, exists := seen[inputCase.ID]; exists {
			t.Fatalf("duplicate input case id %q", inputCase.ID)
		}
		seen[inputCase.ID] = struct{}{}

		t.Run(inputCase.ID, func(t *testing.T) {
			runEnsureScopeCase(t, inputCase)
		})
	}
}

func runEnsureScopeCase(t *testing.T, inputCase identityinventory.InputCase) {
	t.Helper()

	var configPath string
	switch inputCase.ID {
	case "valid-missing-file":
		configPath = operatorsettings.DefaultConfigPath(t.TempDir())
	case "invalid-empty-config-path":
		configPath = "   "
	default:
		configPath = writeFixtureToTemp(t, inputCase.Fixture)
	}

	resolved, err := operatorsettings.EnsureLocalBackendScope(
		platformfilesystem.Local{},
		func(dir, pattern string) (operatorsettings.TemporaryFile, error) { return os.CreateTemp(dir, pattern) },
		uuid.NewString,
		configPath,
	)
	if inputCase.Outcome == "accept" {
		if err != nil {
			t.Fatalf("EnsureLocalBackendScope() error = %v, want accept", err)
		}
		assertScopeExpectation(t, resolved, inputCase.ExpectedScope)
		if inputCase.PersistedFileExpectation != nil {
			assertPersistedFileExpectation(t, configPath, resolved, inputCase.PersistedFileExpectation)
		}
		return
	}

	if err == nil {
		t.Fatal("EnsureLocalBackendScope() error = nil, want reject")
	}
	assertErrorFragments(t, err, inputCase.ErrorFragments)
}

func assertScopeExpectation(t *testing.T, resolved operatorsettings.ResolvedBackendScope, want *identityinventory.ScopeExpectation) {
	t.Helper()

	if want == nil {
		t.Fatal("accept case missing expectedScope")
	}
	if resolved.Outcome != want.Outcome {
		t.Fatalf("outcome = %q, want %q", resolved.Outcome, want.Outcome)
	}
	if want.BackendScopeID != "" && resolved.BackendScopeID != want.BackendScopeID {
		t.Fatalf("backendScopeID = %q, want %q", resolved.BackendScopeID, want.BackendScopeID)
	}
	if want.RequireLocalUUID && !operatorsettings.IsLocalBackendScopeID(resolved.BackendScopeID) {
		t.Fatalf("backendScopeID = %q, want local-<uuid>", resolved.BackendScopeID)
	}
}

func assertPersistedFileExpectation(
	t *testing.T,
	configPath string,
	resolved operatorsettings.ResolvedBackendScope,
	want *identityinventory.PersistedFileExpectation,
) {
	t.Helper()

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var persisted map[string]json.RawMessage
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if want.BackendScopeIDMatchesResolved {
		scopeRaw, ok := persisted["backendScopeID"]
		if !ok {
			t.Fatal("expected backendScopeID key in persisted config")
		}
		var scopeID string
		if err := json.Unmarshal(scopeRaw, &scopeID); err != nil {
			t.Fatalf("Unmarshal backendScopeID: %v", err)
		}
		if scopeID != resolved.BackendScopeID {
			t.Fatalf("persisted backendScopeID = %q, want %q", scopeID, resolved.BackendScopeID)
		}
	}

	if want.PreservesDefaults {
		defaultsRaw, ok := persisted["defaults"]
		if !ok {
			t.Fatal("expected defaults block to remain in persisted config")
		}
		var defaults struct {
			WorkerModelProvider string `json:"workerModelProvider"`
			WorkerModel         string `json:"workerModel"`
		}
		if err := json.Unmarshal(defaultsRaw, &defaults); err != nil {
			t.Fatalf("Unmarshal defaults: %v", err)
		}
		if defaults.WorkerModelProvider != "codex" || defaults.WorkerModel != "gpt-5-codex" {
			t.Fatalf("defaults = %#v, want codex/gpt-5-codex preserved", defaults)
		}
	}

	for _, key := range want.PreservesSiblingKeys {
		if _, ok := persisted[key]; !ok {
			t.Fatalf("expected sibling key %q to remain in persisted config", key)
		}
	}
}

func assertErrorFragments(t *testing.T, err error, fragments []string) {
	t.Helper()

	for _, fragment := range fragments {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error = %q, want fragment %q", err.Error(), fragment)
		}
	}
}

func readFixture(t *testing.T, rel string) []byte {
	t.Helper()

	path := testutil.MustRepoPath(t, filepath.ToSlash(filepath.Join(fixturesRelativeDir, rel)))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return data
}

func writeFixtureToTemp(t *testing.T, rel string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, readFixture(t, rel), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestMarshalInputInventoryJSON_IsByteIdenticalAcrossRepeatedProjections(t *testing.T) {
	t.Parallel()

	first := identityinventory.ProjectInputInventory()
	second := identityinventory.ProjectInputInventory()

	firstJSON, err := identityinventory.MarshalInputInventoryJSON(first)
	if err != nil {
		t.Fatalf("first MarshalInputInventoryJSON() error = %v", err)
	}
	secondJSON, err := identityinventory.MarshalInputInventoryJSON(second)
	if err != nil {
		t.Fatalf("second MarshalInputInventoryJSON() error = %v", err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("repeated system config input inventory json differs")
	}
	if firstJSON[len(firstJSON)-1] != '\n' {
		t.Fatalf("system config input inventory json missing trailing newline")
	}
}

func TestProjectInputInventory_MatchesCommittedBaseline(t *testing.T) {
	inventory := identityinventory.ProjectInputInventory()
	got, err := identityinventory.MarshalInputInventoryJSON(inventory)
	if err != nil {
		t.Fatalf("MarshalInputInventoryJSON() error = %v", err)
	}

	fixturePath := testutil.MustRepoPath(t, identityinventory.InputIndexBaselineRelativePath)
	want, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read baseline fixture %s: %v", fixturePath, err)
	}
	want = globalconfiginventory.NormalizeFixtureBytes(want)
	if bytes.Equal(got, want) {
		return
	}

	t.Fatalf(
		"system config input index baseline drift detected; update %s when intentional\nwant %d bytes, got %d bytes",
		identityinventory.InputIndexBaselineRelativePath,
		len(want),
		len(got),
	)
}

func TestWriteSystemConfigInputIndexBaseline(t *testing.T) {
	if os.Getenv("UPDATE_SYSTEM_CONFIG_BASELINES") != "1" {
		t.Skip("set UPDATE_SYSTEM_CONFIG_BASELINES=1 to rewrite fixtures")
	}

	inventory := identityinventory.ProjectInputInventory()
	got, err := identityinventory.MarshalInputInventoryJSON(inventory)
	if err != nil {
		t.Fatalf("MarshalInputInventoryJSON() error = %v", err)
	}

	fixturePath := testutil.MustRepoPath(t, identityinventory.InputIndexBaselineRelativePath)
	if err := os.MkdirAll(filepath.Dir(fixturePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(fixturePath, got, 0o644); err != nil {
		t.Fatalf("write baseline fixture %s: %v", fixturePath, err)
	}
}
