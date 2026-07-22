package globalconfiginventory_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	operator_settings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/operator_settings/globalconfiginventory"
)

func TestProjectTopologyInventory_RecordsSharedFileSplitAndPrecedence(t *testing.T) {
	t.Parallel()

	inventory := globalconfiginventory.ProjectTopologyInventory()
	if inventory.FormatVersion != globalconfiginventory.FormatVersion {
		t.Fatalf("FormatVersion = %q, want %q", inventory.FormatVersion, globalconfiginventory.FormatVersion)
	}
	if inventory.PrecedenceChain != operator_settings.PrecedenceChain {
		t.Fatalf("PrecedenceChain = %q, want %q", inventory.PrecedenceChain, operator_settings.PrecedenceChain)
	}
	if !strings.Contains(inventory.SharedFileSplit.Summary, "operator_settings owns backendScopeID identity, defaults, and workerPresets") {
		t.Fatalf("shared file split summary = %q, want unified operator_settings ownership", inventory.SharedFileSplit.Summary)
	}
}

func TestProjectTopologyInventory_RecordsRequiredFieldsAndLayers(t *testing.T) {
	t.Parallel()

	inventory := globalconfiginventory.ProjectTopologyInventory()
	byID := indexFieldsByID(t, inventory.Fields)

	required := []string{
		"backendScopeID",
		"defaults",
		"defaults.workerModelProvider",
		"defaults.workerModel",
		"workerPresets",
		"workerPresets[].id",
		"workerPresets[].modelProvider",
		"workerPresets[].model",
		"workerPresets[].reasoningEffort",
	}
	for _, id := range required {
		if _, ok := byID[id]; !ok {
			t.Fatalf("missing inventoried field %q", id)
		}
	}

	provider := byID["defaults.workerModelProvider"]
	if provider.EnvironmentVariable != operator_settings.EnvDefaultWorkerModelProvider {
		t.Fatalf("provider env = %q, want %q", provider.EnvironmentVariable, operator_settings.EnvDefaultWorkerModelProvider)
	}
	if provider.FlagName != "--default-worker-model-provider" {
		t.Fatalf("provider flag = %q, want --default-worker-model-provider", provider.FlagName)
	}
	if got := strings.Join(provider.PrecedenceLayers, ","); got != "file,env,flag" {
		t.Fatalf("provider precedence = %q, want file,env,flag", got)
	}

	model := byID["defaults.workerModel"]
	if model.EnvironmentVariable != operator_settings.EnvDefaultWorkerModel {
		t.Fatalf("model env = %q, want %q", model.EnvironmentVariable, operator_settings.EnvDefaultWorkerModel)
	}
	if model.FlagName != "--default-worker-model" {
		t.Fatalf("model flag = %q, want --default-worker-model", model.FlagName)
	}

	backendScope := byID["backendScopeID"]
	if backendScope.PersistenceOwner != "operator_settings" || backendScope.ParseOwner != "operator_settings" {
		t.Fatalf("backendScopeID ownership = parse %q persist %q, want operator_settings/operator_settings", backendScope.ParseOwner, backendScope.PersistenceOwner)
	}
	if len(backendScope.PrecedenceLayers) != 0 {
		t.Fatalf("backendScopeID precedence layers = %#v, want none", backendScope.PrecedenceLayers)
	}

	if len(inventory.UnknownFieldPolicy) < 2 {
		t.Fatalf("unknown field policy len = %d, want settings and identity loader entries", len(inventory.UnknownFieldPolicy))
	}
}

func TestMarshalCanonicalJSON_IsByteIdenticalAcrossRepeatedProjections(t *testing.T) {
	t.Parallel()

	first := globalconfiginventory.ProjectTopologyInventory()
	second := globalconfiginventory.ProjectTopologyInventory()

	firstJSON, err := globalconfiginventory.MarshalCanonicalJSON(first)
	if err != nil {
		t.Fatalf("first MarshalCanonicalJSON() error = %v", err)
	}
	secondJSON, err := globalconfiginventory.MarshalCanonicalJSON(second)
	if err != nil {
		t.Fatalf("second MarshalCanonicalJSON() error = %v", err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("repeated topology inventory json differs")
	}
	if firstJSON[len(firstJSON)-1] != '\n' {
		t.Fatalf("topology inventory json missing trailing newline")
	}
}

func TestProjectTopologyInventory_MatchesCommittedBaseline(t *testing.T) {
	inventory := globalconfiginventory.ProjectTopologyInventory()
	got, err := globalconfiginventory.MarshalCanonicalJSON(inventory)
	if err != nil {
		t.Fatalf("MarshalCanonicalJSON() error = %v", err)
	}

	fixturePath := testutil.MustRepoPath(t, globalconfiginventory.TopologyBaselineRelativePath)
	want, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read baseline fixture %s: %v", fixturePath, err)
	}
	want = globalconfiginventory.NormalizeFixtureBytes(want)
	if bytes.Equal(got, want) {
		return
	}

	t.Fatalf(
		"global config topology baseline drift detected; update %s when intentional\nwant %d bytes, got %d bytes",
		globalconfiginventory.TopologyBaselineRelativePath,
		len(want),
		len(got),
	)
}

func TestWriteTopologyInventoryBaseline(t *testing.T) {
	if os.Getenv("UPDATE_GLOBAL_CONFIG_BASELINES") != "1" {
		t.Skip("set UPDATE_GLOBAL_CONFIG_BASELINES=1 to rewrite fixtures")
	}

	inventory := globalconfiginventory.ProjectTopologyInventory()
	got, err := globalconfiginventory.MarshalCanonicalJSON(inventory)
	if err != nil {
		t.Fatalf("MarshalCanonicalJSON() error = %v", err)
	}

	fixturePath := testutil.MustRepoPath(t, globalconfiginventory.TopologyBaselineRelativePath)
	if err := os.MkdirAll(filepath.Dir(fixturePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(fixturePath, got, 0o644); err != nil {
		t.Fatalf("write baseline fixture %s: %v", fixturePath, err)
	}
}

func indexFieldsByID(t *testing.T, fields []globalconfiginventory.FieldRecord) map[string]globalconfiginventory.FieldRecord {
	t.Helper()

	indexed := make(map[string]globalconfiginventory.FieldRecord, len(fields))
	for _, field := range fields {
		if field.ID == "" {
			t.Fatal("field record missing id")
		}
		if _, exists := indexed[field.ID]; exists {
			t.Fatalf("duplicate field id %q", field.ID)
		}
		indexed[field.ID] = field
	}
	return indexed
}
