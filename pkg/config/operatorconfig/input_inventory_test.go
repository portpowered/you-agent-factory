package operatorconfig_test

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

const fixturesRelativeDir = "pkg/config/operatorconfig/testdata/fixtures"

func TestProjectInputInventory_RecordsUnknownFieldPolicyAndPrecedence(t *testing.T) {
	t.Parallel()

	inventory := operatorconfig.ProjectInputInventory()
	if inventory.FormatVersion != operatorconfig.InputInventoryFormatVersion {
		t.Fatalf("FormatVersion = %q, want %q", inventory.FormatVersion, operatorconfig.InputInventoryFormatVersion)
	}
	if !strings.Contains(inventory.UnknownFieldPolicy, "DisallowUnknownFields") {
		t.Fatalf("unknown field policy = %q, want DisallowUnknownFields reference", inventory.UnknownFieldPolicy)
	}
	if inventory.PrecedenceChain != operatorconfig.PrecedenceChain {
		t.Fatalf("PrecedenceChain = %q, want %q", inventory.PrecedenceChain, operatorconfig.PrecedenceChain)
	}
}

func TestProjectInputInventory_HasUnknownFieldRejectCase(t *testing.T) {
	t.Parallel()

	inventory := operatorconfig.ProjectInputInventory()
	for _, inputCase := range inventory.Cases {
		if inputCase.Category != "parse-unknown-field" || inputCase.Outcome != "reject" {
			continue
		}
		if inputCase.Fixture == "" {
			t.Fatalf("unknown-field case %q missing fixture", inputCase.ID)
		}
		return
	}
	t.Fatal("missing unknown-field reject case in input inventory")
}

func TestIndexedInputCases_MatchProductionLoaders(t *testing.T) {
	inventory := operatorconfig.ProjectInputInventory()
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

func runIndexedInputCase(t *testing.T, inputCase operatorconfig.InputCase) {
	t.Helper()

	switch inputCase.Entrypoint {
	case "ParseFileConfig":
		runParseFileConfigCase(t, inputCase)
	case "LoadFileConfig":
		runLoadFileConfigCase(t, inputCase)
	case "Resolve":
		runResolveCase(t, inputCase)
	default:
		t.Fatalf("unsupported entrypoint %q", inputCase.Entrypoint)
	}
}

func runParseFileConfigCase(t *testing.T, inputCase operatorconfig.InputCase) {
	t.Helper()

	data := readFixture(t, inputCase.Fixture)
	cfg, err := operatorconfig.ParseFileConfig(data)
	if inputCase.Outcome == "accept" {
		if err != nil {
			t.Fatalf("ParseFileConfig() error = %v, want accept", err)
		}
		assertFileConfigExpectation(t, cfg, inputCase.ExpectedFileConfig)
		return
	}
	if err == nil {
		t.Fatal("ParseFileConfig() error = nil, want reject")
	}
	assertErrorFragments(t, err, inputCase.ErrorFragments)
}

func runLoadFileConfigCase(t *testing.T, inputCase operatorconfig.InputCase) {
	t.Helper()

	var path string
	if inputCase.ID == "valid-missing-file" {
		path = filepath.Join(t.TempDir(), "missing-config.json")
	} else {
		path = writeFixtureToTemp(t, inputCase.Fixture)
	}

	cfg, err := operatorconfig.LoadFileConfig(path)
	if inputCase.Outcome == "accept" {
		if err != nil {
			t.Fatalf("LoadFileConfig() error = %v, want accept", err)
		}
		assertFileConfigExpectation(t, cfg, inputCase.ExpectedFileConfig)
		return
	}
	if err == nil {
		t.Fatal("LoadFileConfig() error = nil, want reject")
	}
	if inputCase.ID == "invalid-load-malformed" && !strings.Contains(err.Error(), path) {
		t.Fatalf("error = %q, want path %q", err.Error(), path)
	}
	assertErrorFragments(t, err, inputCase.ErrorFragments)
}

func runResolveCase(t *testing.T, inputCase operatorconfig.InputCase) {
	t.Helper()

	if inputCase.ResolveLayers == nil {
		t.Fatal("resolve case missing resolveLayers")
	}

	layers := inputCase.ResolveLayers
	fileDefaults := defaultsFromLayers(t, layers)
	for key, value := range layers.Env {
		t.Setenv(key, value)
	}

	resolved, err := operatorconfig.Resolve(operatorconfig.ResolveInput{
		File: fileDefaults,
		Env: operatorconfig.Defaults{
			WorkerModelProvider: strings.TrimSpace(os.Getenv(operatorconfig.EnvDefaultWorkerModelProvider)),
			WorkerModel:         strings.TrimSpace(os.Getenv(operatorconfig.EnvDefaultWorkerModel)),
		},
		Flag: operatorconfig.Defaults{
			WorkerModelProvider: strings.TrimSpace(layers.Flag.WorkerModelProvider),
			WorkerModel:         strings.TrimSpace(layers.Flag.WorkerModel),
		},
	}, "/tmp/operator-config.json")

	if inputCase.Outcome == "accept" {
		if err != nil {
			t.Fatalf("Resolve() error = %v, want accept", err)
		}
		if inputCase.PrecedenceWinners != nil {
			if string(resolved.WorkerModelProviderSource) != inputCase.PrecedenceWinners.WorkerModelProviderSource {
				t.Fatalf("provider source = %q, want %q", resolved.WorkerModelProviderSource, inputCase.PrecedenceWinners.WorkerModelProviderSource)
			}
			if string(resolved.WorkerModelSource) != inputCase.PrecedenceWinners.WorkerModelSource {
				t.Fatalf("model source = %q, want %q", resolved.WorkerModelSource, inputCase.PrecedenceWinners.WorkerModelSource)
			}
		}
		if inputCase.ExpectedResolved != nil {
			if resolved.WorkerModelProvider != inputCase.ExpectedResolved.WorkerModelProvider {
				t.Fatalf("provider = %q, want %q", resolved.WorkerModelProvider, inputCase.ExpectedResolved.WorkerModelProvider)
			}
			if resolved.WorkerModel != inputCase.ExpectedResolved.WorkerModel {
				t.Fatalf("model = %q, want %q", resolved.WorkerModel, inputCase.ExpectedResolved.WorkerModel)
			}
		}
		return
	}

	if err == nil {
		t.Fatal("Resolve() error = nil, want reject")
	}
	assertErrorFragments(t, err, inputCase.ErrorFragments)
}

func defaultsFromLayers(t *testing.T, layers *operatorconfig.ResolveLayers) operatorconfig.Defaults {
	t.Helper()

	if layers.FileFixture != "" {
		cfg, err := operatorconfig.ParseFileConfig(readFixture(t, layers.FileFixture))
		if err != nil {
			t.Fatalf("ParseFileConfig(file fixture) error = %v", err)
		}
		return cfg.Defaults
	}
	return operatorconfig.Defaults{
		WorkerModelProvider: layers.FileDefaults.WorkerModelProvider,
		WorkerModel:         layers.FileDefaults.WorkerModel,
	}
}

func assertFileConfigExpectation(t *testing.T, cfg operatorconfig.FileConfig, want *operatorconfig.FileConfigExpectation) {
	t.Helper()

	if want == nil {
		t.Fatal("accept case missing expectedFileConfig")
	}
	gotDefaults := operatorconfig.DefaultsSnapshot{
		WorkerModelProvider: cfg.Defaults.WorkerModelProvider,
		WorkerModel:         cfg.Defaults.WorkerModel,
	}
	if gotDefaults != want.Defaults {
		t.Fatalf("defaults = %#v, want %#v", gotDefaults, want.Defaults)
	}
	if len(cfg.WorkerPresets) != len(want.WorkerPresets) {
		t.Fatalf("worker presets len = %d, want %d", len(cfg.WorkerPresets), len(want.WorkerPresets))
	}
	for i := range want.WorkerPresets {
		if cfg.WorkerPresets[i] != want.WorkerPresets[i] {
			t.Fatalf("workerPresets[%d] = %#v, want %#v", i, cfg.WorkerPresets[i], want.WorkerPresets[i])
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

	first := operatorconfig.ProjectInputInventory()
	second := operatorconfig.ProjectInputInventory()

	firstJSON, err := operatorconfig.MarshalInputInventoryJSON(first)
	if err != nil {
		t.Fatalf("first MarshalInputInventoryJSON() error = %v", err)
	}
	secondJSON, err := operatorconfig.MarshalInputInventoryJSON(second)
	if err != nil {
		t.Fatalf("second MarshalInputInventoryJSON() error = %v", err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("repeated operator config input inventory json differs")
	}
	if firstJSON[len(firstJSON)-1] != '\n' {
		t.Fatalf("operator config input inventory json missing trailing newline")
	}
}

func TestProjectInputInventory_MatchesCommittedBaseline(t *testing.T) {
	inventory := operatorconfig.ProjectInputInventory()
	got, err := operatorconfig.MarshalInputInventoryJSON(inventory)
	if err != nil {
		t.Fatalf("MarshalInputInventoryJSON() error = %v", err)
	}

	fixturePath := testutil.MustRepoPath(t, operatorconfig.InputIndexBaselineRelativePath)
	want, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read baseline fixture %s: %v", fixturePath, err)
	}
	want = globalconfiginventory.NormalizeFixtureBytes(want)
	if bytes.Equal(got, want) {
		return
	}

	t.Fatalf(
		"operator config input index baseline drift detected; update %s when intentional\nwant %d bytes, got %d bytes",
		operatorconfig.InputIndexBaselineRelativePath,
		len(want),
		len(got),
	)
}

func TestWriteOperatorConfigInputIndexBaseline(t *testing.T) {
	if os.Getenv("UPDATE_OPERATOR_CONFIG_BASELINES") != "1" {
		t.Skip("set UPDATE_OPERATOR_CONFIG_BASELINES=1 to rewrite fixtures")
	}

	inventory := operatorconfig.ProjectInputInventory()
	got, err := operatorconfig.MarshalInputInventoryJSON(inventory)
	if err != nil {
		t.Fatalf("MarshalInputInventoryJSON() error = %v", err)
	}

	fixturePath := testutil.MustRepoPath(t, operatorconfig.InputIndexBaselineRelativePath)
	if err := os.MkdirAll(filepath.Dir(fixturePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(fixturePath, got, 0o644); err != nil {
		t.Fatalf("write baseline fixture %s: %v", fixturePath, err)
	}
}
