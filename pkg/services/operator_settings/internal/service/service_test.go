package service_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/testdeps"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/service"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
	"go.uber.org/zap/zapcore"
)

func TestRootEnsureLocalBackendScopeGeneratesAndThenReuses(t *testing.T) {
	t.Parallel()

	root := newIdentityRoot(t, testCreateTemporaryFile)
	path := filepath.Join(t.TempDir(), "config.json")

	first, err := root.EnsureLocalBackendScope(path)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope() = %v", err)
	}
	if first.Outcome != operatorsettings.BackendScopeOutcomeGenerated {
		t.Fatalf("first outcome = %q, want generated", first.Outcome)
	}
	if !root.IsLocalBackendScopeID(first.BackendScopeID) {
		t.Fatalf("first backend scope = %q, want local UUID", first.BackendScopeID)
	}

	second, err := root.EnsureLocalBackendScope(path)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope() second call = %v", err)
	}
	if second.Outcome != operatorsettings.BackendScopeOutcomeReused || second.BackendScopeID != first.BackendScopeID {
		t.Fatalf("second result = %#v, want reuse of %#v", second, first)
	}
}

func TestRootEnsureLocalBackendScopeRejectsShortWriteWithoutReplacement(t *testing.T) {
	t.Parallel()

	root := newIdentityRoot(t, func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
		return shortWriteTemporaryFile{name: filepath.Join(dir, pattern)}, nil
	})
	path := filepath.Join(t.TempDir(), "config.json")

	_, err := root.EnsureLocalBackendScope(path)
	if err == nil || !strings.Contains(err.Error(), "short write") {
		t.Fatalf("EnsureLocalBackendScope() = %v, want short-write failure", err)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("config path stat error = %v, want destination to remain absent", statErr)
	}
}

func TestRootResolveFromHomeWithEnvironmentWarnsForIgnoredConfigFields(t *testing.T) {
	t.Parallel()

	homeDir := "selected-home"
	zapLogger, observed := testdeps.CapturingZapLogger(zapcore.InfoLevel)
	files := &configReadFileSystem{data: []byte("secret-value")}
	root, err := operatorservice.New(&constructionDocument{}, forwardingResolution{
		resolve: func(request operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error) {
			if request.DocumentBaseline.WorkerModel != "gpt-5" || request.ConfigPath != filepath.Join(homeDir, ".you-agent-factory", "config.json") {
				t.Errorf("resolution request = %#v", request)
			}
			return operatorsettings.ResolveEffectiveResult{Selection: operatorsettings.EffectiveSelection{WorkerModelProvider: "CODEX", WorkerModel: "gpt-5"}}, nil
		},
	}, files, rootTestCreateTemporaryFile, rootTestConfigDecoder, rootTestConfigEncoder, rootTestIDGenerator,
		logging.NewZapLogger(zapLogger, false), func(data []byte) (operatorsettings.Config, operatorsettings.ConfigDecodeDiagnostics, error) {
			if string(data) != "secret-value" {
				t.Errorf("decoder input = %q", data)
			}
			return operatorsettings.Config{Defaults: operatorsettings.Defaults{WorkerModelProvider: "codex", WorkerModel: "gpt-5"}},
				operatorsettings.ConfigDecodeDiagnostics{IgnoredJSONPaths: []string{"$.defaults.futureDefault", "$.futureTopLevel"}}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := root.ResolveFromHomeWithEnvironment(homeDir, operatorsettings.Defaults{}, operatorsettings.FlagOverrides{})
	if err != nil || resolved.WorkerModelProvider != "CODEX" || resolved.WorkerModel != "gpt-5" {
		t.Fatalf("resolved defaults = %#v, %v", resolved, err)
	}

	warnings := observed.FilterMessage("operator_settings.config.unknown_fields_ignored").All()
	if len(warnings) != 1 {
		t.Fatalf("compatibility warnings = %d, want one: %#v", len(warnings), warnings)
	}
	fields := warnings[0].ContextMap()
	if fields["operation"] != "load_file_config" {
		t.Fatalf("warning operation = %#v, want load_file_config", fields["operation"])
	}
	rawPaths, ok := fields["json_paths"].([]interface{})
	if !ok {
		t.Fatalf("warning json_paths = %#v, want []string", fields["json_paths"])
	}
	paths := make([]string, len(rawPaths))
	for index, rawPath := range rawPaths {
		paths[index], ok = rawPath.(string)
		if !ok {
			t.Fatalf("warning json_paths[%d] = %#v, want string", index, rawPath)
		}
	}
	wantPaths := []string{"$.defaults.futureDefault", "$.futureTopLevel"}
	if strings.Join(paths, "\n") != strings.Join(wantPaths, "\n") {
		t.Fatalf("warning paths = %#v, want %#v", paths, wantPaths)
	}
	if strings.Contains(strings.Join(paths, "\n"), "secret-value") {
		t.Fatalf("warning paths leaked an ignored value: %#v", paths)
	}
}

func TestRootACPConfigurationAddsDeletesAndMaterializesDefaults(t *testing.T) {
	t.Parallel()

	path := "config.json"
	document := &profileDocument{path: path}
	root := newControlledRoot(t, document, &constructionResolution{})
	integration := operatorsettings.ACPIntegration{
		ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp",
	}

	added, err := root.ConfigureACPIntegrationAdd(context.Background(), path, integration)
	if err != nil {
		t.Fatalf("ConfigureACPIntegrationAdd() = %v", err)
	}
	if len(added.Workers.ACP.Integrations) != 1 || added.Workers.ACP.Integrations[0] != integration {
		t.Fatalf("added document integrations = %#v, want %#v", added.Workers.ACP.Integrations, []operatorsettings.ACPIntegration{integration})
	}

	deleted, err := root.ConfigureACPIntegrationDelete(context.Background(), path, " cursor-acp ")
	if err != nil {
		t.Fatalf("ConfigureACPIntegrationDelete() = %v", err)
	}
	if len(deleted.Workers.ACP.Integrations) != 0 {
		t.Fatalf("deleted document integrations = %#v, want empty", deleted.Workers.ACP.Integrations)
	}
	if _, err := root.ConfigureACPIntegrationDelete(context.Background(), path, "missing"); !errors.Is(err, operatorsettings.ErrACPIntegrationNotFound) {
		t.Fatalf("ConfigureACPIntegrationDelete(missing) = %v, want ErrACPIntegrationNotFound", err)
	}

	defaultsPath := path
	document.document = operatorsettings.Document{}
	defaults := []operatorsettings.ACPIntegration{integration}
	materialized, err := root.EnsurePackagedACPIntegrations(context.Background(), defaultsPath, defaults)
	if err != nil {
		t.Fatalf("EnsurePackagedACPIntegrations() = %v", err)
	}
	if len(materialized.Workers.ACP.Integrations) != 1 || materialized.Workers.ACP.Integrations[0] != integration {
		t.Fatalf("materialized integrations = %#v, want %#v", materialized.Workers.ACP.Integrations, defaults)
	}

	preserved, err := root.EnsurePackagedACPIntegrations(context.Background(), defaultsPath, []operatorsettings.ACPIntegration{{ID: "other", Name: "other"}})
	if err != nil {
		t.Fatalf("EnsurePackagedACPIntegrations() second call = %v", err)
	}
	if len(preserved.Workers.ACP.Integrations) != 1 || preserved.Workers.ACP.Integrations[0] != integration {
		t.Fatalf("preserved integrations = %#v, want original defaults", preserved.Workers.ACP.Integrations)
	}
}

func TestRootUpdatePriceTablePassesNormalizedCandidateAndPreservesUnrelatedSettings(t *testing.T) {
	t.Parallel()
	source := "hf://custom/gemma"
	document := &profileDocument{path: "config.json", document: operatorsettings.Document{
		BackendScopeID: "local-11111111-1111-4111-8111-111111111111",
		Defaults:       operatorsettings.DocumentDefaults{WorkerModelProvider: "CODEX", WorkerModel: "gpt-5"},
		Models:         map[string]operatorsettings.ModelConfig{"llm": {Source: &source}},
		Runtime:        operatorsettings.DocumentRuntimeSettings{Logging: operatorsettings.DocumentRuntimeArtifactSettings{MaxSizeMB: 11}},
		Workers:        operatorsettings.DocumentWorkerSettings{ACP: operatorsettings.DocumentACPSettings{Integrations: []operatorsettings.ACPIntegration{{ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp"}}}},
		WorkerPresets:  []operatorsettings.DocumentWorkerPreset{{ID: "build", ModelProvider: "CODEX", Model: "gpt-5"}},
	}}
	root := newControlledRoot(t, document, &constructionResolution{})
	cached := "0"
	updated, err := root.UpdatePriceTable(context.Background(), document.path, operatorsettings.PriceTable{
		Currency: "USD", Models: []operatorsettings.PriceTableModel{{Provider: " openai ", Model: " gpt-5 ", InputPerMillionTokens: "1.25", OutputPerMillionTokens: "10", CachedInputPerMillionTokens: &cached}},
	})
	if err != nil || !document.published {
		t.Fatalf("UpdatePriceTable() = %#v, %v; published=%v", updated, err, document.published)
	}
	if updated.Currency != operatorsettings.PriceTableCurrencyUSD || len(updated.Models) != 1 || updated.Models[0].Provider != "CODEX" {
		t.Fatalf("normalized price = %#v", updated)
	}
	assertPreservedPriceTableSettings(t, document.document)
}

func assertPreservedPriceTableSettings(t *testing.T, document operatorsettings.Document) {
	t.Helper()
	if document.BackendScopeID != "local-11111111-1111-4111-8111-111111111111" || document.Defaults.WorkerModel != "gpt-5" {
		t.Fatalf("identity/defaults changed after update: %#v", document)
	}
	model := document.Models["llm"]
	if model.Source == nil || *model.Source != "hf://custom/gemma" {
		t.Fatalf("model overlay changed after update: %#v", document.Models)
	}
	if document.Runtime.Logging.MaxSizeMB != 11 || len(document.WorkerPresets) != 1 || len(document.Workers.ACP.Integrations) != 1 {
		t.Fatalf("unrelated settings changed after update: %#v", document)
	}
	price := document.PriceTable.Models[0]
	if price.InputPerMillionTokens != "1.25" || price.CachedInputPerMillionTokens == nil || *price.CachedInputPerMillionTokens != "0" {
		t.Fatalf("persisted price table = %#v, want exact rates", document.PriceTable)
	}
}

// Only backend identity publication owns filesystem effects in these root tests.
func newIdentityRoot(t *testing.T, createTemp operatorsettings.CreateTemporaryFile) operatorsettings.Service {
	t.Helper()
	root, err := operatorservice.New(&constructionDocument{}, &constructionResolution{}, platformfilesystem.Local{}, createTemp,
		globalconfigmapping.Decode, globalconfigmapping.Encode, rootTestIDGenerator, logging.NoopLogger{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func testCreateTemporaryFile(dir, pattern string) (operatorsettings.TemporaryFile, error) {
	return os.CreateTemp(dir, pattern)
}

type shortWriteTemporaryFile struct {
	name string
}

func (file shortWriteTemporaryFile) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	return len(data) - 1, nil
}

func (file shortWriteTemporaryFile) Name() string { return file.name }

func (shortWriteTemporaryFile) Sync() error  { return nil }
func (shortWriteTemporaryFile) Close() error { return nil }

type rootTestFileSystem struct{}

func (rootTestFileSystem) ReadFile(string) ([]byte, error) {
	panic("filesystem read during root service test")
}

func (rootTestFileSystem) MkdirAll(string, fs.FileMode) error {
	panic("filesystem mkdir during root service test")
}

func (rootTestFileSystem) Remove(string) error {
	panic("filesystem remove during root service test")
}

func (rootTestFileSystem) Chmod(string, fs.FileMode) error {
	panic("filesystem chmod during root service test")
}

func (rootTestFileSystem) Rename(string, string) error {
	panic("filesystem rename during root service test")
}

func rootTestCreateTemporaryFile(string, string) (operatorsettings.TemporaryFile, error) {
	panic("temp-file creation during root service test")
}

func rootTestConfigDecoder([]byte) (operatorsettings.Config, error) {
	panic("config decode during root service test")
}

func rootTestConfigEncoder(operatorsettings.Config) ([]byte, error) {
	panic("config encode during root service test")
}
