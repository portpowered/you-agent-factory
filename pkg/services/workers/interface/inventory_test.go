package mockworkers_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/workers/interface"
)

func TestProjectTopologyInventory_RecordsLoaderBoundariesAndRunTypes(t *testing.T) {
	t.Parallel()

	inventory := mockworkers.ProjectTopologyInventory()
	if inventory.FormatVersion != mockworkers.FormatVersion {
		t.Fatalf("FormatVersion = %q, want %q", inventory.FormatVersion, mockworkers.FormatVersion)
	}
	if !strings.Contains(inventory.UnknownFieldPolicy, "DisallowUnknownFields") {
		t.Fatalf("unknown field policy = %q, want strict decode language", inventory.UnknownFieldPolicy)
	}
	if !strings.Contains(inventory.EntrySelectionPolicy, "first matching") {
		t.Fatalf("entry selection policy = %q, want first-match language", inventory.EntrySelectionPolicy)
	}

	runTypes := make(map[string]mockworkers.RunTypeRecord, len(inventory.RunTypeUnion.Values))
	for _, value := range inventory.RunTypeUnion.Values {
		runTypes[value.Value] = value
	}
	for _, want := range []string{"accept", "script", "reject"} {
		if _, ok := runTypes[want]; !ok {
			t.Fatalf("missing runType union value %q", want)
		}
	}
	if runTypes["script"].NestedConfig != "scriptConfig" {
		t.Fatalf("script nested config = %q, want scriptConfig", runTypes["script"].NestedConfig)
	}
	if runTypes["reject"].NestedConfig != "rejectConfig" {
		t.Fatalf("reject nested config = %q, want rejectConfig", runTypes["reject"].NestedConfig)
	}
}

func TestProjectTopologyInventory_RecordsRequiredFieldsAndSelectors(t *testing.T) {
	t.Parallel()

	inventory := mockworkers.ProjectTopologyInventory()
	byID := indexFieldsByID(t, inventory.Fields)

	required := []string{
		"mockWorkers",
		"unmatchedDispatchPolicy",
		"mockWorkers[].id",
		"mockWorkers[].workerName",
		"mockWorkers[].workstationName",
		"mockWorkers[].workInputs",
		"mockWorkers[].runType",
		"mockWorkers[].scriptConfig",
		"mockWorkers[].rejectConfig",
		"mockWorkers[].workInputs[].workId",
		"mockWorkers[].workInputs[].workType",
		"mockWorkers[].workInputs[].state",
		"mockWorkers[].workInputs[].inputName",
		"mockWorkers[].workInputs[].traceId",
		"mockWorkers[].workInputs[].channel",
		"mockWorkers[].workInputs[].payloadHash",
		"mockWorkers[].scriptConfig.command",
		"mockWorkers[].scriptConfig.args",
		"mockWorkers[].scriptConfig.env",
		"mockWorkers[].scriptConfig.workingDirectory",
		"mockWorkers[].scriptConfig.stdin",
		"mockWorkers[].scriptConfig.timeout",
		"mockWorkers[].rejectConfig.stdout",
		"mockWorkers[].rejectConfig.stderr",
		"mockWorkers[].rejectConfig.exitCode",
	}
	for _, id := range required {
		if _, ok := byID[id]; !ok {
			t.Fatalf("missing inventoried field %q", id)
		}
	}

	runType := byID["mockWorkers[].runType"]
	if runType.ValidationOwner != "validate" {
		t.Fatalf("runType validation owner = %q, want validate", runType.ValidationOwner)
	}
	if runType.Required != "required" {
		t.Fatalf("runType required = %q, want required", runType.Required)
	}

	command := byID["mockWorkers[].scriptConfig.command"]
	if command.Required != "required when runType is script" {
		t.Fatalf("script command required = %q, want required when runType is script", command.Required)
	}

	exitCode := byID["mockWorkers[].rejectConfig.exitCode"]
	if exitCode.ValidationOwner != "validate" {
		t.Fatalf("exitCode validation owner = %q, want validate", exitCode.ValidationOwner)
	}

	if len(inventory.UnmatchedDispatchPolicies) < 3 {
		t.Fatalf("unmatched policy len = %d, want accept default plus explicit accept and passthrough", len(inventory.UnmatchedDispatchPolicies))
	}
}

func TestProjectTopologyInventory_RecordsStrictBoundariesAndNonAcceptedCapabilities(t *testing.T) {
	t.Parallel()

	inventory := mockworkers.ProjectTopologyInventory()
	if len(inventory.ValidationBoundaries) < 7 {
		t.Fatalf("validation boundary len = %d, want decode and validate rejections", len(inventory.ValidationBoundaries))
	}

	patterns := make([]string, 0, len(inventory.ValidationBoundaries))
	for _, boundary := range inventory.ValidationBoundaries {
		patterns = append(patterns, boundary.ErrorPattern)
	}
	for _, want := range []string{
		"unknown field",
		`runType must be one of "accept", "script", or "reject"`,
		`scriptConfig is required when runType is "script"`,
		`scriptConfig.command is required when runType is "script"`,
		"rejectConfig.exitCode must be between 1 and 255",
	} {
		if !containsSubstring(patterns, want) {
			t.Fatalf("missing validation boundary pattern %q in %#v", want, patterns)
		}
	}

	categories := make([]string, 0, len(inventory.NotAcceptedCapabilities))
	for _, capability := range inventory.NotAcceptedCapabilities {
		categories = append(categories, capability.Category)
	}
	for _, want := range []string{"media", "artifact payloads", "response sequences"} {
		if !containsSubstring(categories, want) {
			t.Fatalf("missing not-accepted capability category %q in %#v", want, categories)
		}
	}
}

func TestMarshalCanonicalJSON_IsByteIdenticalAcrossRepeatedProjections(t *testing.T) {
	t.Parallel()

	first := mockworkers.ProjectTopologyInventory()
	second := mockworkers.ProjectTopologyInventory()

	firstJSON, err := mockworkers.MarshalCanonicalJSON(first)
	if err != nil {
		t.Fatalf("first MarshalCanonicalJSON() error = %v", err)
	}
	secondJSON, err := mockworkers.MarshalCanonicalJSON(second)
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
	inventory := mockworkers.ProjectTopologyInventory()
	got, err := mockworkers.MarshalCanonicalJSON(inventory)
	if err != nil {
		t.Fatalf("MarshalCanonicalJSON() error = %v", err)
	}

	fixturePath := testutil.MustRepoPath(t, mockworkers.TopologyBaselineRelativePath)
	want, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read baseline fixture %s: %v", fixturePath, err)
	}
	want = mockworkers.NormalizeFixtureBytes(want)
	if bytes.Equal(got, want) {
		return
	}

	t.Fatalf(
		"mock workers topology baseline drift detected; update %s when intentional\nwant %d bytes, got %d bytes",
		mockworkers.TopologyBaselineRelativePath,
		len(want),
		len(got),
	)
}

func TestWriteTopologyInventoryBaseline(t *testing.T) {
	if os.Getenv("UPDATE_MOCK_WORKERS_BASELINES") != "1" {
		t.Skip("set UPDATE_MOCK_WORKERS_BASELINES=1 to rewrite fixtures")
	}

	inventory := mockworkers.ProjectTopologyInventory()
	got, err := mockworkers.MarshalCanonicalJSON(inventory)
	if err != nil {
		t.Fatalf("MarshalCanonicalJSON() error = %v", err)
	}

	fixturePath := testutil.MustRepoPath(t, mockworkers.TopologyBaselineRelativePath)
	if err := os.MkdirAll(filepath.Dir(fixturePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(fixturePath, got, 0o644); err != nil {
		t.Fatalf("write baseline fixture %s: %v", fixturePath, err)
	}
}

func indexFieldsByID(t *testing.T, fields []mockworkers.FieldRecord) map[string]mockworkers.FieldRecord {
	t.Helper()

	indexed := make(map[string]mockworkers.FieldRecord, len(fields))
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

func containsSubstring(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}
