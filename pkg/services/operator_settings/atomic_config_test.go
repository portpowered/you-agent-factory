package operatorsettings

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestConfigDocumentServicePersist_AtomicallyPublishesCompleteConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.json")
	service := persistedConfigService(testFiles, testCreateTemp)
	document, err := service.Parse([]byte(`{"backendScopeID":"local-11111111-1111-4111-8111-111111111111","workerPresets":[{"id":"build","modelProvider":"codex"}]}`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	provider, model := "claude", "provider/model@next"
	document, err = service.MergeProviderModelDefaults(document, ProviderModelUpdate{Provider: &provider, Model: &model})
	if err != nil {
		t.Fatalf("MergeProviderModelDefaults() error = %v", err)
	}
	if err := service.Persist(context.Background(), path, document); err != nil {
		t.Fatalf("Persist() error = %v", err)
	}

	got, err := service.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(got.FileConfig(), document.FileConfig()) || got.BackendScopeID() != document.BackendScopeID() {
		t.Fatalf("persisted document = %#v/%q, want %#v/%q", got.FileConfig(), got.BackendScopeID(), document.FileConfig(), document.BackendScopeID())
	}
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatalf("Stat() error = %v", statErr)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("destination mode = %v, want 0600", info.Mode().Perm())
		}
	}
}

func TestConfigDocumentServicePersist_PreCommitFailuresPreserveDestination(t *testing.T) {
	t.Parallel()
	phases := []struct {
		name       string
		filePhase  string
		tempPhase  string
		shortWrite bool
		want       string
	}{
		{name: "directory", filePhase: "mkdir", want: "create operator config directory"},
		{name: "temporary file", tempPhase: "create", want: "create operator config temp file"},
		{name: "write", tempPhase: "write", want: "write operator config temp file"},
		{name: "short write", shortWrite: true, want: "short write"},
		{name: "sync", tempPhase: "sync", want: "sync operator config temp file"},
		{name: "close", tempPhase: "close", want: "close operator config temp file"},
		{name: "permissions", filePhase: "chmod", want: "set operator config temp file permissions"},
		{name: "replacement", filePhase: "rename", want: "replace operator config with temp file"},
	}
	for _, phase := range phases {
		phase := phase
		t.Run(phase.name, func(t *testing.T) {
			t.Parallel()
			path, original, document := persistedConfigFixture(t)
			files := &faultFileSystem{FileSystem: testFiles, failPhase: phase.filePhase}
			create := faultTemporaryFileCreator(phase.tempPhase, phase.shortWrite)
			service := persistedConfigService(files, create)
			err := service.Persist(context.Background(), path, document)
			if err == nil || !strings.Contains(err.Error(), phase.want) {
				t.Fatalf("Persist() error = %v, want %q", err, phase.want)
			}
			assertConfigBytesUnchanged(t, path, original)
			assertNoTemporaryArtifacts(t, filepath.Dir(path))
		})
	}
}

func TestConfigDocumentServicePersist_RejectsBeforeFilesystemSideEffects(t *testing.T) {
	t.Parallel()
	files := &faultFileSystem{FileSystem: testFiles}
	service := persistedConfigService(files, testCreateTemp)
	invalid := ConfigDocument{fields: map[string]json.RawMessage{"unexpected": []byte("true")}}
	if err := service.Persist(context.Background(), "config.json", invalid); err == nil {
		t.Fatal("Persist() error = nil, want invalid candidate")
	}
	if files.calls != 0 {
		t.Fatalf("filesystem calls = %d, want zero", files.calls)
	}

	document, err := service.Parse([]byte(`{}`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.Persist(cancelled, "config.json", document); !errors.Is(err, context.Canceled) {
		t.Fatalf("Persist(cancelled) error = %v, want context canceled", err)
	}
	if files.calls != 0 {
		t.Fatalf("filesystem calls after cancellation = %d, want zero", files.calls)
	}
}

func TestConfigDocumentServicePersist_CancellationAtCommitBoundaryPreservesDestination(t *testing.T) {
	t.Parallel()
	path, original, document := persistedConfigFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	files := &faultFileSystem{FileSystem: testFiles, cancelOnChmod: cancel}
	service := persistedConfigService(files, testCreateTemp)
	if err := service.Persist(ctx, path, document); !errors.Is(err, context.Canceled) {
		t.Fatalf("Persist() error = %v, want context canceled", err)
	}
	assertConfigBytesUnchanged(t, path, original)
	assertNoTemporaryArtifacts(t, filepath.Dir(path))
}

func TestConfigDocumentServicePersist_CancellationDuringSuccessfulCommitReportsSuccess(t *testing.T) {
	t.Parallel()
	path, original, document := persistedConfigFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	files := &faultFileSystem{FileSystem: testFiles, cancelOnRename: cancel}
	service := persistedConfigService(files, testCreateTemp)
	if err := service.Persist(ctx, path, document); err != nil {
		t.Fatalf("Persist() error = %v, want committed success", err)
	}
	if ctx.Err() != context.Canceled {
		t.Fatalf("context error = %v, want cancellation observed during replacement", ctx.Err())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) == string(original) {
		t.Fatal("destination retained original bytes after successful commit")
	}
	if _, err := service.Parse(got); err != nil {
		t.Fatalf("committed config is invalid: %v", err)
	}
}

func TestConfigDocumentServicePersist_ConcurrentReadersSeeCompleteCandidates(t *testing.T) {
	t.Parallel()
	path, _, base := persistedConfigFixture(t)
	service := persistedConfigService(testFiles, testCreateTemp)
	const writers = 24
	start := make(chan struct{})
	done := make(chan struct{})
	writerErrors := make(chan error, writers)
	readerErrors := make(chan error, 1)
	var group sync.WaitGroup
	group.Add(writers)
	for index := 0; index < writers; index++ {
		index := index
		go func() {
			defer group.Done()
			<-start
			model := "concurrent-model-" + strconv.Itoa(index)
			candidate, err := service.MergeProviderModelDefaults(base, ProviderModelUpdate{Model: &model})
			if err == nil {
				err = service.Persist(context.Background(), path, candidate)
			}
			if err != nil {
				writerErrors <- err
			}
		}()
	}
	go func() {
		<-start
		for {
			select {
			case <-done:
				return
			default:
				if _, err := service.Load(path); err != nil {
					readerErrors <- err
					return
				}
			}
		}
	}()
	close(start)
	group.Wait()
	close(done)
	if len(writerErrors) != 0 {
		t.Fatalf("concurrent writer error = %v", <-writerErrors)
	}
	if len(readerErrors) != 0 {
		t.Fatalf("concurrent reader observed invalid config = %v", <-readerErrors)
	}
	final, err := service.Load(path)
	if err != nil {
		t.Fatalf("Load(final) error = %v", err)
	}
	if final.BackendScopeID() != base.BackendScopeID() || !reflect.DeepEqual(final.FileConfig().WorkerPresets, base.FileConfig().WorkerPresets) {
		t.Fatalf("final document lost unrelated values: %#v/%q", final.FileConfig(), final.BackendScopeID())
	}
}

type faultFileSystem struct {
	FileSystem
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
	TemporaryFile
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

func faultTemporaryFileCreator(failPhase string, shortWrite bool) CreateTemporaryFile {
	return func(dir, pattern string) (TemporaryFile, error) {
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

func persistedConfigService(files FileSystem, create CreateTemporaryFile) ConfigDocumentService {
	return ConfigDocumentService{
		Files:           files,
		CreateTemp:      create,
		Providers:       controlledProviderCatalog,
		PersistenceLock: &sync.Mutex{},
	}
}

func persistedConfigFixture(t *testing.T) (string, []byte, ConfigDocument) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	original := []byte(`{"backendScopeID":"local-11111111-1111-4111-8111-111111111111","defaults":{"workerModelProvider":"CODEX","workerModel":"original"},"workerPresets":[{"id":"build","modelProvider":"codex"}]}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	service := persistedConfigService(testFiles, testCreateTemp)
	document, err := service.Parse(original)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	model := "replacement"
	document, err = service.MergeProviderModelDefaults(document, ProviderModelUpdate{Model: &model})
	if err != nil {
		t.Fatalf("MergeProviderModelDefaults() error = %v", err)
	}
	return path, original, document
}

func assertConfigBytesUnchanged(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("destination changed: got %q want %q", got, want)
	}
}

func assertNoTemporaryArtifacts(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "config.json.*.tmp"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary artifacts = %v, error = %v", matches, err)
	}
}
