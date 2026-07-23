package operatorsettings_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"

	"github.com/portpowered/infinite-you/internal/testutil"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
)

const fixturesRelativeDir = "pkg/services/operator_settings/testdata/fixtures"

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
	case "DecodeGlobalConfig":
		runDecodeGlobalConfigCase(t, inputCase)
	case "LoadFileConfig":
		runLoadFileConfigCase(t, inputCase)
	case "Resolve":
		runResolveCase(t, inputCase)
	default:
		t.Fatalf("unsupported entrypoint %q", inputCase.Entrypoint)
	}
}

func runDecodeGlobalConfigCase(t *testing.T, inputCase operatorconfig.InputCase) {
	t.Helper()

	data := readFixture(t, inputCase.Fixture)
	cfg, err := globalconfigmapping.Decode(data)
	if inputCase.Outcome == "accept" {
		if err != nil {
			t.Fatalf("DecodeGlobalConfig() error = %v, want accept", err)
		}
		assertConfigExpectation(t, cfg, inputCase.ExpectedConfig)
		return
	}
	if err == nil {
		t.Fatal("DecodeGlobalConfig() error = nil, want reject")
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

	cfg, err := operatorconfig.LoadFileConfig(platformfilesystem.Local{}, globalconfigmapping.Decode, path)
	if inputCase.Outcome == "accept" {
		if err != nil {
			t.Fatalf("LoadFileConfig() error = %v, want accept", err)
		}
		assertConfigExpectation(t, cfg, inputCase.ExpectedConfig)
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
		cfg, err := globalconfigmapping.Decode(readFixture(t, layers.FileFixture))
		if err != nil {
			t.Fatalf("DecodeGlobalConfig(file fixture) error = %v", err)
		}
		return cfg.Defaults
	}
	return operatorconfig.Defaults{
		WorkerModelProvider: layers.FileDefaults.WorkerModelProvider,
		WorkerModel:         layers.FileDefaults.WorkerModel,
	}
}

func assertConfigExpectation(t *testing.T, cfg operatorconfig.Config, want *operatorconfig.ConfigExpectation) {
	t.Helper()

	if want == nil {
		t.Fatal("accept case missing expectedConfig")
	}
	if cfg.BackendScopeID != want.BackendScopeID {
		t.Fatalf("backendScopeID = %q, want %q", cfg.BackendScopeID, want.BackendScopeID)
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
