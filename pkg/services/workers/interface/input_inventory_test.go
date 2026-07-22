package mockworkers_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/services/workers/interface"
)

func localMockWorkersConfigLoader(t *testing.T) mockworkers.MockWorkersConfigLoader {
	t.Helper()
	load, err := mockworkers.NewMockWorkersConfigLoader(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("construct mock workers config loader: %v", err)
	}
	return load
}

func TestProjectInputInventory_RecordsUnknownFieldPolicyAndEntrypoints(t *testing.T) {
	t.Parallel()

	inventory := mockworkers.ProjectInputInventory()
	if inventory.FormatVersion != mockworkers.InputInventoryFormatVersion {
		t.Fatalf("FormatVersion = %q, want %q", inventory.FormatVersion, mockworkers.InputInventoryFormatVersion)
	}
	if !strings.Contains(inventory.UnknownFieldPolicy, "DisallowUnknownFields") {
		t.Fatalf("unknown field policy = %q, want DisallowUnknownFields reference", inventory.UnknownFieldPolicy)
	}
	if len(inventory.LoaderEntrypoints) != 2 {
		t.Fatalf("loader entrypoints len = %d, want ParseMockWorkersConfig and LoadMockWorkersConfig", len(inventory.LoaderEntrypoints))
	}
}

func TestProjectInputInventory_HasDocsExampleAndVariantCases(t *testing.T) {
	t.Parallel()

	inventory := mockworkers.ProjectInputInventory()
	byID := indexInputCasesByID(t, inventory.Cases)

	required := []string{
		"valid-empty-default",
		"valid-accept-entry-selectors",
		"valid-reject-without-reject-config",
		"valid-script-minimal-command",
		"valid-unmatched-policy-explicit-accept",
		"docs-example-mock-workers",
		"docs-example-mock-workers-script",
		"docs-example-mock-workers-mixed",
		"valid-load-empty-path",
		"load-docs-example-mock-workers",
		"load-docs-example-mock-workers-script",
		"load-docs-example-mock-workers-mixed",
	}
	for _, id := range required {
		if _, ok := byID[id]; !ok {
			t.Fatalf("missing indexed input case %q", id)
		}
	}
}

func TestProjectInputInventory_HasUnknownFieldAndInvalidUnionRejectCases(t *testing.T) {
	t.Parallel()

	inventory := mockworkers.ProjectInputInventory()
	byID := indexInputCasesByID(t, inventory.Cases)

	required := []string{
		"invalid-unknown-top-level",
		"invalid-unknown-nested-mock-worker",
		"invalid-trailing-json",
		"invalid-unknown-run-type",
		"invalid-unknown-unmatched-policy",
		"invalid-script-without-script-config",
		"invalid-script-without-command",
		"invalid-reject-exit-code-out-of-range",
	}
	for _, id := range required {
		inputCase, ok := byID[id]
		if !ok {
			t.Fatalf("missing indexed invalid input case %q", id)
		}
		if inputCase.Outcome != "reject" {
			t.Fatalf("input case %q outcome = %q, want reject", id, inputCase.Outcome)
		}
		if len(inputCase.ErrorFragments) == 0 {
			t.Fatalf("input case %q missing errorFragments", id)
		}
	}

	var unknownFieldReject bool
	for _, inputCase := range inventory.Cases {
		if inputCase.Category != "parse-unknown-field" || inputCase.Outcome != "reject" {
			continue
		}
		if inputCase.Fixture == "" {
			t.Fatalf("unknown-field case %q missing fixture", inputCase.ID)
		}
		unknownFieldReject = true
		break
	}
	if !unknownFieldReject {
		t.Fatal("missing unknown-field reject case in input inventory")
	}
}

func TestIndexedInputCases_MatchProductionLoaders(t *testing.T) {
	inventory := mockworkers.ProjectInputInventory()
	seen := make(map[string]struct{}, len(inventory.Cases))
	for _, inputCase := range inventory.Cases {
		if inputCase.ID == "" {
			t.Fatal("input case missing id")
		}
		if _, exists := seen[inputCase.ID]; exists {
			t.Fatalf("duplicate input case id %q", inputCase.ID)
		}
		seen[inputCase.ID] = struct{}{}

		t.Run(inputCase.ID, func(t *testing.T) {
			runIndexedInputCase(t, inputCase)
		})
	}
}

func runIndexedInputCase(t *testing.T, inputCase mockworkers.InputCase) {
	t.Helper()

	switch inputCase.Entrypoint {
	case "ParseMockWorkersConfig":
		runParseMockWorkersConfigCase(t, inputCase)
	case "LoadMockWorkersConfig":
		runLoadMockWorkersConfigCase(t, inputCase)
	default:
		t.Fatalf("unsupported entrypoint %q", inputCase.Entrypoint)
	}
}

func runParseMockWorkersConfigCase(t *testing.T, inputCase mockworkers.InputCase) {
	t.Helper()

	data := readRepoFixture(t, inputCase.Fixture)
	cfg, err := mockworkers.ParseMockWorkersConfig(data)
	if inputCase.Outcome == "accept" {
		if err != nil {
			t.Fatalf("ParseMockWorkersConfig() error = %v, want accept", err)
		}
		assertMockWorkersConfigExpectation(t, cfg, inputCase.ExpectedConfig)
		return
	}
	if err == nil {
		t.Fatal("ParseMockWorkersConfig() error = nil, want reject")
	}
	assertErrorFragments(t, err, inputCase.ErrorFragments)
}

func runLoadMockWorkersConfigCase(t *testing.T, inputCase mockworkers.InputCase) {
	t.Helper()

	var path string
	if inputCase.ID == "valid-load-empty-path" {
		path = ""
	} else {
		path = repoFixturePath(t, inputCase.Fixture)
	}

	cfg, err := localMockWorkersConfigLoader(t)(path)
	if inputCase.Outcome == "accept" {
		if err != nil {
			t.Fatalf("LoadMockWorkersConfig() error = %v, want accept", err)
		}
		assertMockWorkersConfigExpectation(t, cfg, inputCase.ExpectedConfig)
		return
	}
	if err == nil {
		t.Fatal("LoadMockWorkersConfig() error = nil, want reject")
	}
	assertErrorFragments(t, err, inputCase.ErrorFragments)
}

func assertMockWorkersConfigExpectation(t *testing.T, cfg *mockworkers.MockWorkersConfig, want *mockworkers.MockWorkersConfigExpectation) {
	t.Helper()

	if want == nil {
		t.Fatal("accept case missing expectedConfig")
	}
	if cfg == nil {
		t.Fatal("loader returned nil config")
	}
	assertUnmatchedDispatchPolicyExpectation(t, cfg, want)
	if len(cfg.MockWorkers) != want.MockWorkerCount {
		t.Fatalf("mock worker count = %d, want %d", len(cfg.MockWorkers), want.MockWorkerCount)
	}
	for i, wantWorker := range want.MockWorkers {
		if i >= len(cfg.MockWorkers) {
			t.Fatalf("mockWorkers[%d] missing, want %#v", i, wantWorker)
		}
		assertMockWorkerExpectation(t, i, cfg.MockWorkers[i], wantWorker)
	}
}

func assertUnmatchedDispatchPolicyExpectation(t *testing.T, cfg *mockworkers.MockWorkersConfig, want *mockworkers.MockWorkersConfigExpectation) {
	t.Helper()

	if want.UnmatchedDispatchPolicy == "" {
		return
	}
	got := string(cfg.UnmatchedDispatchPolicy)
	if got != want.UnmatchedDispatchPolicy {
		t.Fatalf("unmatchedDispatchPolicy = %q, want %q", got, want.UnmatchedDispatchPolicy)
	}
}

func assertMockWorkerExpectation(t *testing.T, index int, got mockworkers.MockWorkerConfig, want mockworkers.MockWorkerExpectation) {
	t.Helper()

	if want.ID != "" && got.ID != want.ID {
		t.Fatalf("mockWorkers[%d].id = %q, want %q", index, got.ID, want.ID)
	}
	if want.WorkerName != "" && got.WorkerName != want.WorkerName {
		t.Fatalf("mockWorkers[%d].workerName = %q, want %q", index, got.WorkerName, want.WorkerName)
	}
	if want.WorkstationName != "" && got.WorkstationName != want.WorkstationName {
		t.Fatalf("mockWorkers[%d].workstationName = %q, want %q", index, got.WorkstationName, want.WorkstationName)
	}
	if want.RunType != "" && string(got.RunType) != want.RunType {
		t.Fatalf("mockWorkers[%d].runType = %q, want %q", index, got.RunType, want.RunType)
	}
	assertMockWorkerScriptExpectation(t, index, got, want)
	assertMockWorkerRejectExpectation(t, index, got, want)
}

func assertMockWorkerScriptExpectation(t *testing.T, index int, got mockworkers.MockWorkerConfig, want mockworkers.MockWorkerExpectation) {
	t.Helper()

	if want.ScriptCommand == "" {
		return
	}
	if got.ScriptConfig == nil {
		t.Fatalf("mockWorkers[%d].scriptConfig = nil, want command %q", index, want.ScriptCommand)
	}
	if got.ScriptConfig.Command != want.ScriptCommand {
		t.Fatalf("mockWorkers[%d].scriptConfig.command = %q, want %q", index, got.ScriptConfig.Command, want.ScriptCommand)
	}
}

func assertMockWorkerRejectExpectation(t *testing.T, index int, got mockworkers.MockWorkerConfig, want mockworkers.MockWorkerExpectation) {
	t.Helper()

	if want.RejectExitCode == nil {
		return
	}
	if got.RejectConfig == nil || got.RejectConfig.ExitCode == nil {
		t.Fatalf("mockWorkers[%d].rejectConfig.exitCode = %#v, want %d", index, got.RejectConfig, *want.RejectExitCode)
	}
	if *got.RejectConfig.ExitCode != *want.RejectExitCode {
		t.Fatalf("mockWorkers[%d].rejectConfig.exitCode = %d, want %d", index, *got.RejectConfig.ExitCode, *want.RejectExitCode)
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

func readRepoFixture(t *testing.T, rel string) []byte {
	t.Helper()

	path := repoFixturePath(t, rel)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return data
}

func repoFixturePath(t *testing.T, rel string) string {
	t.Helper()

	return testutil.MustRepoPath(t, filepath.ToSlash(rel))
}

func indexInputCasesByID(t *testing.T, cases []mockworkers.InputCase) map[string]mockworkers.InputCase {
	t.Helper()

	indexed := make(map[string]mockworkers.InputCase, len(cases))
	for _, inputCase := range cases {
		if inputCase.ID == "" {
			t.Fatal("input case missing id")
		}
		if _, exists := indexed[inputCase.ID]; exists {
			t.Fatalf("duplicate input case id %q", inputCase.ID)
		}
		indexed[inputCase.ID] = inputCase
	}
	return indexed
}

func TestMarshalInputInventoryJSON_IsByteIdenticalAcrossRepeatedProjections(t *testing.T) {
	t.Parallel()

	first := mockworkers.ProjectInputInventory()
	second := mockworkers.ProjectInputInventory()

	firstJSON, err := mockworkers.MarshalInputInventoryJSON(first)
	if err != nil {
		t.Fatalf("first MarshalInputInventoryJSON() error = %v", err)
	}
	secondJSON, err := mockworkers.MarshalInputInventoryJSON(second)
	if err != nil {
		t.Fatalf("second MarshalInputInventoryJSON() error = %v", err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("repeated mock workers input inventory json differs")
	}
	if firstJSON[len(firstJSON)-1] != '\n' {
		t.Fatalf("mock workers input inventory json missing trailing newline")
	}
}

func TestProjectInputInventory_MatchesCommittedBaseline(t *testing.T) {
	inventory := mockworkers.ProjectInputInventory()
	got, err := mockworkers.MarshalInputInventoryJSON(inventory)
	if err != nil {
		t.Fatalf("MarshalInputInventoryJSON() error = %v", err)
	}

	fixturePath := testutil.MustRepoPath(t, mockworkers.InputIndexBaselineRelativePath)
	want, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read baseline fixture %s: %v", fixturePath, err)
	}
	want = mockworkers.NormalizeFixtureBytes(want)
	if bytes.Equal(got, want) {
		return
	}

	t.Fatalf(
		"mock workers input index baseline drift detected; update %s when intentional\nwant %d bytes, got %d bytes",
		mockworkers.InputIndexBaselineRelativePath,
		len(want),
		len(got),
	)
}

func TestWriteMockWorkersInputIndexBaseline(t *testing.T) {
	if os.Getenv("UPDATE_MOCK_WORKERS_BASELINES") != "1" {
		t.Skip("set UPDATE_MOCK_WORKERS_BASELINES=1 to rewrite fixtures")
	}

	inventory := mockworkers.ProjectInputInventory()
	got, err := mockworkers.MarshalInputInventoryJSON(inventory)
	if err != nil {
		t.Fatalf("MarshalInputInventoryJSON() error = %v", err)
	}

	fixturePath := testutil.MustRepoPath(t, mockworkers.InputIndexBaselineRelativePath)
	if err := os.MkdirAll(filepath.Dir(fixturePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(fixturePath, got, 0o644); err != nil {
		t.Fatalf("write baseline fixture %s: %v", fixturePath, err)
	}
}
