package service_test

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/service"
)

type configReadFileSystem struct {
	rootTestFileSystem
	data []byte
	err  error
	path string
}

func (f *configReadFileSystem) ReadFile(path string) ([]byte, error) {
	f.path = path
	return f.data, f.err
}

func TestRootLoadConfigSelectsOptionalDiagnosticsAndPreservesFailures(t *testing.T) {
	t.Parallel()
	failure := errors.New("private-config-sentinel")
	for _, tc := range []struct {
		name                   string
		diagnostic             bool
		readError, decodeError error
	}{
		{name: "required decoder"}, {name: "diagnostic specialization", diagnostic: true},
		{name: "missing", readError: fs.ErrNotExist}, {name: "read unavailable", readError: failure},
		{name: "decode failed", decodeError: failure}, {name: "diagnostic failed", diagnostic: true, decodeError: failure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			files := &configReadFileSystem{data: []byte("raw-secret-value"), err: tc.readError}
			decoded, diagnosed := 0, 0
			config := operatorsettings.Config{Defaults: operatorsettings.Defaults{WorkerModel: "selected-model"}}
			decode := func(data []byte) (operatorsettings.Config, error) {
				decoded++
				if string(data) != "raw-secret-value" {
					t.Errorf("decode data = %q", data)
				}
				return config, tc.decodeError
			}
			var diagnostic operatorsettings.ConfigDiagnosticsDecoder
			if tc.diagnostic {
				diagnostic = func([]byte) (operatorsettings.Config, operatorsettings.ConfigDecodeDiagnostics, error) {
					diagnosed++
					return config, operatorsettings.ConfigDecodeDiagnostics{IgnoredJSONPaths: []string{"$.future"}}, tc.decodeError
				}
			}
			logger := &spyLogger{}
			root, err := operatorservice.New(&constructionDocument{}, &constructionResolution{}, files, rootTestCreateTemporaryFile,
				decode, rootTestConfigEncoder, rootTestIDGenerator, logger, diagnostic)
			if err != nil {
				t.Fatal(err)
			}
			got, err := root.LoadFileConfig(" config.json ")
			wantError := tc.readError
			if wantError == nil {
				wantError = tc.decodeError
			}
			assertLoadedConfig(t, got, config, err, wantError)
			wantDecoded, wantDiagnosed := 0, 0
			if tc.readError == nil {
				if tc.diagnostic {
					wantDiagnosed = 1
				} else {
					wantDecoded = 1
				}
			}
			if decoded != wantDecoded || diagnosed != wantDiagnosed || files.path != "config.json" {
				t.Fatalf("decoder calls = %d/%d, path=%q", decoded, diagnosed, files.path)
			}
			if tc.diagnostic && wantError == nil && !containsKeyValue(logger, "json_paths", []string{"$.future"}) {
				t.Fatal("missing diagnostic paths")
			}
			assertNoSensitiveValuesLogged(t, logger, "raw-secret-value", failure.Error())
		})
	}
}

func assertLoadedConfig(t *testing.T, got, want operatorsettings.Config, err, wantError error) {
	t.Helper()
	if errors.Is(wantError, fs.ErrNotExist) {
		if err != nil || got.Runtime.Logging.MaxSizeMB != operatorsettings.DefaultRuntimeArtifactMaxSizeMB || got.PriceTable.Currency != "USD" {
			t.Fatalf("missing config = %#v, %v", got, err)
		}
	} else if wantError != nil {
		if !errors.Is(err, wantError) {
			t.Fatalf("load error = %v, want %v", err, wantError)
		}
	} else if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("config = %#v, %v", got, err)
	}
}

func TestRootResolveFromHomeForwardsLayersAndActionableFailure(t *testing.T) {
	t.Parallel()
	failure := operatorsettings.ResolutionFailure{Kind: operatorsettings.ResolutionFailureKindInvalidInput, Message: "symbolic DEFAULT requires concrete provider"}
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "symbolic failure"}[fail], func(t *testing.T) {
			t.Parallel()
			files := &configReadFileSystem{data: []byte("config")}
			path := filepath.Join("home", ".you-agent-factory", "config.json")
			root, err := operatorservice.New(&constructionDocument{}, forwardingResolution{resolve: func(r operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error) {
				assertResolutionFacts(t, r, path)
				if fail {
					return operatorsettings.ResolveEffectiveResult{}, failure
				}
				return operatorsettings.ResolveEffectiveResult{Selection: operatorsettings.EffectiveSelection{WorkerModel: "flag-model", WorkerModelSource: operatorsettings.EffectiveLayerSourceFlag, ConfigPath: path}}, nil
			}}, files, rootTestCreateTemporaryFile, func([]byte) (operatorsettings.Config, error) {
				return operatorsettings.Config{Defaults: operatorsettings.Defaults{WorkerModel: "file-model"}, WorkerPresets: []operatorsettings.WorkerPreset{{ID: "build"}}}, nil
			}, rootTestConfigEncoder, rootTestIDGenerator, logging.NoopLogger{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := root.ResolveFromHomeWithEnvironment(" home ", operatorsettings.Defaults{WorkerModel: " env-model "}, operatorsettings.FlagOverrides{WorkerModel: " flag-model ", WorkerModelProvider: " DEFAULT "})
			if fail {
				if !errors.Is(err, failure) || !strings.Contains(err.Error(), "YOU_DEFAULT_WORKER_MODEL_PROVIDER") {
					t.Fatalf("symbolic failure = %v", err)
				}
			} else if err != nil || got.WorkerModel != "flag-model" || got.WorkerModelSource != operatorsettings.SourceFlag || got.ConfigPath != path {
				t.Fatalf("resolved = %#v, %v", got, err)
			}
		})
	}
}

func assertResolutionFacts(t *testing.T, r operatorsettings.ResolveEffectiveRequest, path string) {
	t.Helper()
	if r.ConfigPath != path || r.DocumentBaseline.WorkerModel != "file-model" || r.EnvironmentOverrides.WorkerModel != "env-model" || r.InvocationOverrides.WorkerModel != "flag-model" || r.InvocationOverrides.WorkerModelProvider != "DEFAULT" || len(r.WorkerPresets) != 1 {
		t.Errorf("resolution facts = %#v", r)
	}
}

type identityFileSystem struct {
	platformfilesystem.Local
	failAt  string
	failure error
	removed int
}

func (f *identityFileSystem) ReadFile(path string) ([]byte, error) {
	if f.failAt == "read" {
		return nil, f.failure
	}
	return f.Local.ReadFile(path)
}
func (f *identityFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	if f.failAt == "mkdir" {
		return f.failure
	}
	return f.Local.MkdirAll(path, mode)
}
func (f *identityFileSystem) Chmod(path string, mode fs.FileMode) error {
	if f.failAt == "chmod" {
		return f.failure
	}
	return f.Local.Chmod(path, mode)
}
func (f *identityFileSystem) Rename(from, to string) error {
	if f.failAt == "rename" {
		return f.failure
	}
	return f.Local.Rename(from, to)
}
func (f *identityFileSystem) Remove(path string) error {
	f.removed++
	return f.Local.Remove(path)
}

type identityTemporaryFile struct {
	*os.File
	failAt  string
	failure error
}

func (f identityTemporaryFile) Write(data []byte) (int, error) {
	if f.failAt == "write" {
		return 0, f.failure
	}
	if f.failAt == "short write" {
		return len(data) - 1, nil
	}
	return f.File.Write(data)
}
func (f identityTemporaryFile) Sync() error {
	if f.failAt == "sync" {
		return f.failure
	}
	return f.File.Sync()
}
func (f identityTemporaryFile) Close() error {
	err := f.File.Close()
	if f.failAt == "close" {
		return f.failure
	}
	return err
}

func TestRootIdentityPublicationUsesSelectedEffectsAndPreservesDestination(t *testing.T) {
	t.Parallel()
	for _, failAt := range []string{"", "read", "decode", "encode", "mkdir", "temp", "write", "short write", "sync", "close", "chmod", "rename", "invalid identity"} {
		t.Run("effect="+failAt, func(t *testing.T) {
			t.Parallel()
			exerciseIdentityPublication(t, failAt)
		})
	}
}

func exerciseIdentityPublication(t *testing.T, failAt string) {
	t.Helper()
	failure := errors.New("selected-effect-failure")
	files := &identityFileSystem{failAt: failAt, failure: failure}
	path := filepath.Join(t.TempDir(), "config.json")
	original := []byte(`{}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	generated, encoded, created := 0, 0, 0
	root, err := operatorservice.New(&constructionDocument{}, &constructionResolution{}, files,
		func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
			created++
			if failAt == "temp" {
				return nil, failure
			}
			file, err := os.CreateTemp(dir, pattern)
			if err != nil {
				return nil, err
			}
			return identityTemporaryFile{File: file, failAt: failAt, failure: failure}, nil
		}, func(data []byte) (operatorsettings.Config, error) {
			if failAt == "decode" {
				return operatorsettings.Config{}, failure
			}
			var config operatorsettings.Config
			err := json.Unmarshal(data, &config)
			return config, err
		}, func(config operatorsettings.Config) ([]byte, error) {
			encoded++
			if failAt == "encode" {
				return nil, failure
			}
			return json.Marshal(config)
		}, func() string {
			generated++
			if failAt == "invalid identity" {
				return "invalid"
			}
			return rootTestIDGenerator()
		}, logging.NoopLogger{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := root.EnsureLocalBackendScope(path)
	if failAt == "" {
		assertGeneratedIdentity(t, result, err, generated, encoded, created)
		reloaded, reloadError := root.EnsureLocalBackendScope(path)
		if reloadError != nil || reloaded.BackendScopeID != result.BackendScopeID || reloaded.Outcome != operatorsettings.BackendScopeOutcomeReused {
			t.Fatalf("reused = %#v, %v", reloaded, reloadError)
		}
		assertGeneratedIdentity(t, result, err, generated, encoded, created)
		return
	}
	assertIdentityFailure(t, failAt, result, err, failure, path, original, files.removed)
}

func assertGeneratedIdentity(t *testing.T, result operatorsettings.ResolvedBackendScope, err error, generated, encoded, created int) {
	t.Helper()
	if err != nil || result.Outcome != operatorsettings.BackendScopeOutcomeGenerated || generated != 1 || encoded != 1 || created != 1 {
		t.Fatalf("generated = %#v, %v; effects %d/%d/%d", result, err, generated, encoded, created)
	}
}

func assertIdentityFailure(t *testing.T, failAt string, result operatorsettings.ResolvedBackendScope, err, failure error, path string, original []byte, removed int) {
	t.Helper()
	if err == nil || result.BackendScopeID != "" {
		t.Fatalf("failed generation = %#v, %v", result, err)
	}
	if failAt == "short write" {
		failure = io.ErrShortWrite
	}
	if failAt != "invalid identity" && !errors.Is(err, failure) {
		t.Fatalf("error = %v, want %v", err, failure)
	}
	data, readError := os.ReadFile(path)
	if readError != nil || string(data) != string(original) {
		t.Fatalf("destination = %q, %v", data, readError)
	}
	wantCleanup := 0
	switch failAt {
	case "write", "short write", "sync", "close", "chmod", "rename":
		wantCleanup = 1
	}
	if removed != wantCleanup {
		t.Fatalf("cleanup calls = %d, want %d", removed, wantCleanup)
	}
}

func TestRootRejectsEmptyPathsBeforeRequiredEffects(t *testing.T) {
	t.Parallel()
	root := newControlledRoot(t, &constructionDocument{}, &constructionResolution{})
	if _, err := root.LoadFileConfig(" "); err == nil {
		t.Fatal("empty config path accepted")
	}
	if _, err := root.EnsureLocalBackendScope(" "); err == nil {
		t.Fatal("empty identity path accepted")
	}
	if inventory := root.ProjectInputInventory(); inventory.FormatVersion != operatorsettings.InputInventoryFormatVersion || len(inventory.Cases) == 0 {
		t.Fatalf("inventory = %#v", inventory)
	}
}
