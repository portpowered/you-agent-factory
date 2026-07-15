package globalconfiginventory_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/config/globalconfiginventory"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
)

func TestProjectTopologyInventory_RecordsSharedFileSplitAndPrecedence(t *testing.T) {
	t.Parallel()

	inventory := globalconfiginventory.ProjectTopologyInventory()
	if inventory.FormatVersion != globalconfiginventory.FormatVersion {
		t.Fatalf("FormatVersion = %q, want %q", inventory.FormatVersion, globalconfiginventory.FormatVersion)
	}
	if inventory.PrecedenceChain != operatorconfig.PrecedenceChain {
		t.Fatalf("PrecedenceChain = %q, want %q", inventory.PrecedenceChain, operatorconfig.PrecedenceChain)
	}
	if !strings.Contains(inventory.SharedFileSplit.Summary, "systemconfig owns backendScopeID") {
		t.Fatalf("shared file split summary = %q, want systemconfig ownership", inventory.SharedFileSplit.Summary)
	}
	if !strings.Contains(inventory.SharedFileSplit.Summary, "operatorconfig owns defaults and workerPresets") {
		t.Fatalf("shared file split summary = %q, want operatorconfig ownership", inventory.SharedFileSplit.Summary)
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
	if provider.EnvironmentVariable != operatorconfig.EnvDefaultWorkerModelProvider {
		t.Fatalf("provider env = %q, want %q", provider.EnvironmentVariable, operatorconfig.EnvDefaultWorkerModelProvider)
	}
	if provider.FlagName != "--default-worker-model-provider" {
		t.Fatalf("provider flag = %q, want --default-worker-model-provider", provider.FlagName)
	}
	if got := strings.Join(provider.PrecedenceLayers, ","); got != "file,env,flag" {
		t.Fatalf("provider precedence = %q, want file,env,flag", got)
	}

	model := byID["defaults.workerModel"]
	if model.EnvironmentVariable != operatorconfig.EnvDefaultWorkerModel {
		t.Fatalf("model env = %q, want %q", model.EnvironmentVariable, operatorconfig.EnvDefaultWorkerModel)
	}
	if model.FlagName != "--default-worker-model" {
		t.Fatalf("model flag = %q, want --default-worker-model", model.FlagName)
	}

	backendScope := byID["backendScopeID"]
	if backendScope.PersistenceOwner != "systemconfig" || backendScope.ParseOwner != "systemconfig" {
		t.Fatalf("backendScopeID ownership = parse %q persist %q, want systemconfig/systemconfig", backendScope.ParseOwner, backendScope.PersistenceOwner)
	}
	if len(backendScope.PrecedenceLayers) != 0 {
		t.Fatalf("backendScopeID precedence layers = %#v, want none", backendScope.PrecedenceLayers)
	}

	if len(inventory.UnknownFieldPolicy) < 2 {
		t.Fatalf("unknown field policy len = %d, want operatorconfig and systemconfig entries", len(inventory.UnknownFieldPolicy))
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
