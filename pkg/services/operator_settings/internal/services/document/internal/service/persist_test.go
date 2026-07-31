package service_test

import (
	"context"
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
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
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

	document := operatorsettings.EmptyDocument()
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

	return internalservice.New(
		files,
		create,
		globalconfigmapping.Decode,
		globalconfigmapping.Encode,
		controlledProviderCatalog,
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
	original := readFixture(t, "valid/load-defaults.json")
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
		Runtime: operatorsettings.EmptyDocument().Runtime,
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
