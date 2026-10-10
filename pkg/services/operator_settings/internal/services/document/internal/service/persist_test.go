package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	internalservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document/internal/service"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
)

func TestPersistDocument_AtomicallyPublishesCompleteDocument(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.json")
	service := newDocumentPersistService(t, testLocalFilesystem, testCreateTemp)

	loaded, err := service.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if err != nil {
		t.Fatalf("LoadDocument() = %v", err)
	}
	document := loaded.Document
	document.Defaults.WorkerModel = "provider/model@next"
	document.Defaults.WorkerModelProvider = "claude"
	document.WorkerPresets = nil

	if err := service.PersistDocument(context.Background(), operatorsettings.PersistDocumentRequest{
		Path:     path,
		Document: document,
	}); err != nil {
		t.Fatalf("PersistDocument() = %v", err)
	}

	reloaded, err := service.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if err != nil {
		t.Fatalf("LoadDocument() after persist = %v", err)
	}
	if reloaded.Document.Defaults != document.Defaults ||
		reloaded.Document.BackendScopeID != document.BackendScopeID ||
		reloaded.Document.Runtime != document.Runtime ||
		len(reloaded.Document.WorkerPresets) != len(document.WorkerPresets) {
		t.Fatalf("reloaded document = %#v, want %#v", reloaded.Document, document)
	}
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatalf("Stat() = %v", statErr)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("destination mode = %v, want 0600", info.Mode().Perm())
		}
	}
}

func TestPersistDocument_PreCommitFailuresPreserveDestination(t *testing.T) {
	t.Parallel()

	phases := []struct {
		name       string
		filePhase  string
		tempPhase  string
		shortWrite bool
		want       string
	}{
		{name: "directory", filePhase: "mkdir", want: "create operator document directory"},
		{name: "temporary file", tempPhase: "create", want: "create operator document temp file"},
		{name: "write", tempPhase: "write", want: "write operator document temp file"},
		{name: "short write", shortWrite: true, want: "short write"},
		{name: "sync", tempPhase: "sync", want: "sync operator document temp file"},
		{name: "close", tempPhase: "close", want: "close operator document temp file"},
		{name: "permissions", filePhase: "chmod", want: "set operator document temp file permissions"},
		{name: "replacement", filePhase: "rename", want: "replace operator document with temp file"},
	}
	for _, phase := range phases {
		phase := phase
		t.Run(phase.name, func(t *testing.T) {
			t.Parallel()
			path, original, document := persistedDocumentFixture(t)
			files := &faultFileSystem{FileSystem: testLocalFilesystem, failPhase: phase.filePhase}
			create := faultTemporaryFileCreator(phase.tempPhase, phase.shortWrite)
			service := newDocumentPersistService(t, files, create)
			err := service.PersistDocument(context.Background(), operatorsettings.PersistDocumentRequest{
				Path:     path,
				Document: document,
			})
			if err == nil || !strings.Contains(err.Error(), phase.want) {
				t.Fatalf("PersistDocument() = %v, want %q", err, phase.want)
			}
			assertDocumentBytesUnchanged(t, path, original)
			assertDocumentSemanticallyUnchanged(t, newDocumentPersistService(t, testLocalFilesystem, testCreateTemp), path, original)
			assertNoTemporaryArtifacts(t, filepath.Dir(path))
		})
	}
}

func TestPersistDocument_DeniedDirectoryPermissionsPreserveDestination(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not enforce Unix directory permission bits")
	}
	path, original, document := persistedDocumentFixture(t)
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Skipf("cannot establish restrictive directory permissions: %v", err)
	}
	defer func() {
		if err := os.Chmod(dir, 0o700); err != nil {
			t.Errorf("restore directory permissions: %v", err)
		}
	}()

	probe, probeErr := os.CreateTemp(dir, "permission-probe-*.tmp")
	if probeErr == nil {
		probePath := probe.Name()
		_ = probe.Close()
		_ = os.Remove(probePath)
		t.Skip("environment does not enforce denied writes for the restrictive directory")
	}
	if !errors.Is(probeErr, fs.ErrPermission) {
		t.Skipf("cannot establish permission-denied behavior: %v", probeErr)
	}

	service := newDocumentPersistService(t, testLocalFilesystem, testCreateTemp)
	err := service.PersistDocument(context.Background(), operatorsettings.PersistDocumentRequest{
		Path:     path,
		Document: document,
	})
	if err == nil || !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("PersistDocument() = %v, want permission denied", err)
	}
	assertDocumentSemanticallyUnchanged(t, service, path, original)
	assertNoTemporaryArtifacts(t, dir)
}

func TestPersistDocument_RejectsBeforeFilesystemSideEffects(t *testing.T) {
	t.Parallel()

	files := &faultFileSystem{FileSystem: testLocalFilesystem}
	service := newDocumentPersistService(t, files, testCreateTemp)
	invalid := operatorsettings.Document{
		WorkerPresets: []operatorsettings.DocumentWorkerPreset{{
			ID:            "invalid",
			ModelProvider: "DEFAULT",
		}},
	}
	if err := service.PersistDocument(context.Background(), operatorsettings.PersistDocumentRequest{
		Path:     "config.json",
		Document: invalid,
	}); err == nil {
		t.Fatal("PersistDocument() = nil, want invalid candidate")
	}
	if files.calls != 0 {
		t.Fatalf("filesystem calls = %d, want zero", files.calls)
	}

	document := operatorsettings.EmptyDocument
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.PersistDocument(cancelled, operatorsettings.PersistDocumentRequest{
		Path:     "config.json",
		Document: document,
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("PersistDocument(cancelled) = %v, want context canceled", err)
	}
	if files.calls != 0 {
		t.Fatalf("filesystem calls after cancellation = %d, want zero", files.calls)
	}
}

func TestPersistDocument_CancellationAtCommitBoundaryPreservesDestination(t *testing.T) {
	t.Parallel()

	path, original, document := persistedDocumentFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	files := &faultFileSystem{FileSystem: testLocalFilesystem, cancelOnChmod: cancel}
	service := newDocumentPersistService(t, files, testCreateTemp)
	if err := service.PersistDocument(ctx, operatorsettings.PersistDocumentRequest{
		Path:     path,
		Document: document,
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("PersistDocument() = %v, want context canceled", err)
	}
	assertDocumentBytesUnchanged(t, path, original)
	assertNoTemporaryArtifacts(t, filepath.Dir(path))
}

func TestPersistDocument_CancellationDuringSuccessfulCommitReportsSuccess(t *testing.T) {
	t.Parallel()

	path, original, document := persistedDocumentFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	files := &faultFileSystem{FileSystem: testLocalFilesystem, cancelOnRename: cancel}
	service := newDocumentPersistService(t, files, testCreateTemp)
	if err := service.PersistDocument(ctx, operatorsettings.PersistDocumentRequest{
		Path:     path,
		Document: document,
	}); err != nil {
		t.Fatalf("PersistDocument() = %v, want committed success", err)
	}
	if ctx.Err() != context.Canceled {
		t.Fatalf("context error = %v, want cancellation observed during replacement", ctx.Err())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() = %v", err)
	}
	if string(got) == string(original) {
		t.Fatal("destination retained original bytes after successful commit")
	}
	if _, err := globalconfigmapping.Decode(got); err != nil {
		t.Fatalf("committed document is invalid: %v", err)
	}
}

func newDocumentPersistService(
	t *testing.T,
	files operatorsettings.FileSystem,
	create operatorsettings.CreateTemporaryFile,
) *internalservice.Service {
	t.Helper()

	return internalservice.NewWithPreserver(
		files,
		create,
		globalconfigmapping.Decode,
		globalconfigmapping.Encode,
		controlledProviderCatalog,
		nil,
		nil,
	)
}

var testLocalFilesystem platformfilesystem.Local

func testCreateTemp(dir, pattern string) (operatorsettings.TemporaryFile, error) {
	return os.CreateTemp(dir, pattern)
}

type faultFileSystem struct {
	operatorsettings.FileSystem
	failPhase      string
	cancelOnChmod  context.CancelFunc
	cancelOnRename context.CancelFunc
	calls          int
}

func (files *faultFileSystem) fail(phase string) error {
	files.calls++
	if files.failPhase == phase {
		return errors.New("injected " + phase + " failure")
	}
	return nil
}

func (files *faultFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	if err := files.fail("mkdir"); err != nil {
		return err
	}
	return files.FileSystem.MkdirAll(path, mode)
}

func (files *faultFileSystem) Remove(path string) error {
	files.calls++
	return files.FileSystem.Remove(path)
}

func (files *faultFileSystem) Chmod(path string, mode fs.FileMode) error {
	if err := files.fail("chmod"); err != nil {
		return err
	}
	if files.cancelOnChmod != nil {
		files.cancelOnChmod()
	}
	return files.FileSystem.Chmod(path, mode)
}

func (files *faultFileSystem) Rename(oldPath, newPath string) error {
	if err := files.fail("rename"); err != nil {
		return err
	}
	if files.cancelOnRename != nil {
		files.cancelOnRename()
	}
	return files.FileSystem.Rename(oldPath, newPath)
}

type faultTemporaryFile struct {
	operatorsettings.TemporaryFile
	failPhase  string
	shortWrite bool
}

func (file *faultTemporaryFile) Write(data []byte) (int, error) {
	if file.failPhase == "write" {
		return 0, errors.New("injected write failure")
	}
	if file.shortWrite {
		return len(data) / 2, nil
	}
	return file.TemporaryFile.Write(data)
}

func (file *faultTemporaryFile) Sync() error {
	if file.failPhase == "sync" {
		return errors.New("injected sync failure")
	}
	return file.TemporaryFile.Sync()
}

func (file *faultTemporaryFile) Close() error {
	if file.failPhase == "close" {
		_ = file.TemporaryFile.Close()
		return errors.New("injected close failure")
	}
	return file.TemporaryFile.Close()
}

func faultTemporaryFileCreator(failPhase string, shortWrite bool) operatorsettings.CreateTemporaryFile {
	return func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
		if failPhase == "create" {
			return nil, errors.New("injected create failure")
		}
		file, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		return &faultTemporaryFile{TemporaryFile: file, failPhase: failPhase, shortWrite: shortWrite}, nil
	}
}

func persistedDocumentFixture(t *testing.T) (string, []byte, operatorsettings.Document) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	original := []byte(`{"defaults":{"workerModelProvider":"claude","workerModel":"before"},"workers":{"acp":{"agentProfile":{"defaultTarget":"factory:@you/reviewer","allowedTargets":["factory:@you/reviewer","factory:@you/factory-builder"]}}}}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}
	service := newDocumentPersistService(t, testLocalFilesystem, testCreateTemp)
	loaded, err := service.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if err != nil {
		t.Fatalf("LoadDocument() = %v", err)
	}
	document := loaded.Document
	document.Defaults.WorkerModel = "replacement-model"
	profile := operatorsettings.ACPAgentProfile{DefaultTarget: "factory:@you/rejected", AllowedTargets: []string{"factory:@you/rejected"}}
	document.Workers.ACP.AgentProfile = &profile
	return path, original, document
}

func assertDocumentBytesUnchanged(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() = %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("destination changed: got %q want %q", got, want)
	}
}

func assertDocumentSemanticallyUnchanged(
	t *testing.T,
	service *internalservice.Service,
	path string,
	want []byte,
) {
	t.Helper()
	wantConfig, err := globalconfigmapping.Decode(want)
	if err != nil {
		t.Fatalf("Decode(original) = %v", err)
	}
	wantDocument := documentFromConfigForTest(wantConfig)
	got, err := service.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if err != nil {
		t.Fatalf("LoadDocument(destination) = %v", err)
	}
	if !reflect.DeepEqual(got.Document, wantDocument) {
		t.Fatalf("destination changed semantically: got %#v want %#v", got.Document, wantDocument)
	}
}

func assertNoTemporaryArtifacts(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "config.json.*.tmp"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary artifacts = %v, error = %v", matches, err)
	}
}

func documentFromConfigForTest(config operatorsettings.Config) operatorsettings.Document {
	document := operatorsettings.Document{
		BackendScopeID: strings.TrimSpace(config.BackendScopeID),
		Defaults: operatorsettings.DocumentDefaults{
			WorkerModelProvider: config.Defaults.WorkerModelProvider,
			WorkerModel:         config.Defaults.WorkerModel,
		},
		PriceTable: config.PriceTable.Clone(),
		Workers:    operatorsettings.DocumentWorkerSettings{ACP: operatorsettings.DocumentACPSettings{AgentProfile: config.Workers.ACP.AgentProfile}},
		Runtime:    operatorsettings.EmptyDocument.Runtime,
	}
	if config.WorkerPresets != nil {
		document.WorkerPresets = make([]operatorsettings.DocumentWorkerPreset, len(config.WorkerPresets))
		for i, preset := range config.WorkerPresets {
			document.WorkerPresets[i] = operatorsettings.DocumentWorkerPreset{
				ID:              preset.ID,
				ModelProvider:   preset.ModelProvider,
				Model:           preset.Model,
				ReasoningEffort: preset.ReasoningEffort,
			}
		}
	}
	return document
}

func TestPersistDocument_EncoderFailurePreservesProfileBytesAndReload(t *testing.T) {
	t.Parallel()
	path, original, candidate := persistedDocumentFixture(t)
	failure := errors.New("injected encode failure")
	service := internalservice.NewWithPreserver(testLocalFilesystem, testCreateTemp, globalconfigmapping.Decode,
		func(operatorsettings.Config) ([]byte, error) { return nil, failure }, controlledProviderCatalog, nil, nil)
	err := service.PersistDocument(context.Background(), operatorsettings.PersistDocumentRequest{Path: path, Document: candidate})
	if !errors.Is(err, failure) {
		t.Fatalf("persist = %v, want encoder failure", err)
	}
	assertDocumentBytesUnchanged(t, path, original)
	assertDocumentSemanticallyUnchanged(t, newDocumentPersistService(t, testLocalFilesystem, testCreateTemp), path, original)
	assertNoTemporaryArtifacts(t, filepath.Dir(path))
}

func TestPersistDocument_ProfileAndPriceTableSurviveFreshOwnerReload(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.json")
	source := "hf://custom/gemma"
	profile := operatorsettings.ACPAgentProfile{DefaultTarget: "factory:@you/reviewer", AllowedTargets: []string{"factory:@you/reviewer", "factory:@you/factory-builder"}}
	cached := "0"
	document := operatorsettings.EmptyDocument.Clone()
	document.BackendScopeID = "local-11111111-1111-4111-8111-111111111111"
	document.Defaults = operatorsettings.DocumentDefaults{WorkerModelProvider: "CODEX", WorkerModel: "gpt-5"}
	document.Models = map[string]operatorsettings.ModelConfig{"llm": {Source: &source}}
	document.Runtime.Logging.MaxSizeMB = 11
	document.Runtime.Metrics.MaxSizeMB = 12
	document.WorkerPresets = []operatorsettings.DocumentWorkerPreset{{ID: "build", ModelProvider: "CODEX", Model: "gpt-5"}}
	document.Workers.ACP.AgentProfile = &profile
	document.Workers.ACP.Integrations = []operatorsettings.ACPIntegration{{ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp"}}
	document.PriceTable = operatorsettings.PriceTable{Currency: "USD", Models: []operatorsettings.PriceTableModel{{Provider: "CODEX", Model: "gpt-5", InputPerMillionTokens: "1.25", OutputPerMillionTokens: "10", CachedInputPerMillionTokens: &cached}}}
	writer := newDocumentPersistService(t, testLocalFilesystem, testCreateTemp)
	if err := writer.PersistDocument(context.Background(), operatorsettings.PersistDocumentRequest{Path: path, Document: document}); err != nil {
		t.Fatal(err)
	}
	reader := newDocumentPersistService(t, testLocalFilesystem, testCreateTemp)
	loaded, err := reader.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path, RequireExisting: true})
	if err != nil || !reflect.DeepEqual(loaded.Document, document) {
		t.Fatalf("fresh owner reload = %#v, %v; want %#v", loaded.Document, err, document)
	}
	model := "gpt-5.2"
	if _, err := writer.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{Path: path, ProviderModel: operatorsettings.DocumentProviderModelUpdate{Model: &model}}); err != nil {
		t.Fatal(err)
	}
	document.Defaults.WorkerModel = model
	loaded, err = reader.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path, RequireExisting: true})
	if err != nil || !reflect.DeepEqual(loaded.Document, document) {
		t.Fatalf("unrelated update reload = %#v, %v; want %#v", loaded.Document, err, document)
	}
}

func TestPersistDocument_OptionalPreserverKeepsUnknownFields(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.json")
	original := []byte(`{"defaults":{"workerModelProvider":"codex","workerModel":"before"},"future":{"token":"retained"}}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	service := internalservice.NewWithPreserver(testLocalFilesystem, testCreateTemp, globalconfigmapping.Decode, globalconfigmapping.Encode, controlledProviderCatalog, globalconfigmapping.PreserveUnknownFields, nil)
	model := "after"
	if _, err := service.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{Path: path, ProviderModel: operatorsettings.DocumentProviderModelUpdate{Model: &model}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	var future map[string]string
	if err := json.Unmarshal(decoded["future"], &future); err != nil || future["token"] != "retained" {
		t.Fatalf("future fields = %s, %v", decoded["future"], err)
	}
	loaded, err := service.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if err != nil || loaded.Document.Defaults.WorkerModel != model {
		t.Fatalf("reload = %#v, %v", loaded, err)
	}
}

func TestPersistDocument_CompatibilityFailuresPreserveOriginalBytes(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"read", "preserve"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			path, original, candidate := persistedDocumentFixture(t)
			failure := errors.New("controlled compatibility failure")
			var files operatorsettings.FileSystem = testLocalFilesystem
			if phase == "read" {
				files = readFailureFileSystem{FileSystem: files, err: failure}
			}
			preserver := func([]byte, []byte) ([]byte, error) { return nil, failure }
			service := internalservice.NewWithPreserver(files, testCreateTemp, globalconfigmapping.Decode, globalconfigmapping.Encode, controlledProviderCatalog, preserver, nil)
			err := service.PersistDocument(context.Background(), operatorsettings.PersistDocumentRequest{Path: path, Document: candidate})
			if !errors.Is(err, failure) {
				t.Fatalf("persist = %v, want compatibility failure", err)
			}
			assertDocumentBytesUnchanged(t, path, original)
			assertNoTemporaryArtifacts(t, filepath.Dir(path))
		})
	}
}
