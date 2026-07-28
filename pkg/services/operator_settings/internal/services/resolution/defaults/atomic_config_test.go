package settingsresolution_test


import (
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	document, err = service.MergeProviderModelDefaults(document, operatorsettings.ProviderModelUpdate{Provider: &provider, Model: &model})
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

func TestConfigDocumentServiceConfigureProviderModel_SerializationFailurePreservesDestination(t *testing.T) {
	t.Parallel()
	path, original, _ := persistedConfigFixture(t)
	service := persistedConfigService(testFiles, testCreateTemp)
	service.Encoder = func(operatorsettings.Config) ([]byte, error) {
		return nil, errors.New("injected serialization failure")
	}
	model := "replacement"
	_, err := service.ConfigureProviderModel(
		context.Background(),
		path,
		operatorsettings.ProviderModelUpdate{Model: &model},
	)
	if err == nil || !strings.Contains(err.Error(), "injected serialization failure") {
		t.Fatalf("ConfigureProviderModel() error = %v, want serialization failure", err)
	}
	assertConfigBytesUnchanged(t, path, original)
	assertNoTemporaryArtifacts(t, filepath.Dir(path))
}

func TestConfigDocumentServicePersist_DeniedDirectoryPermissionsPreserveDestination(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not enforce Unix directory permission bits")
	}
	path, original, document := persistedConfigFixture(t)
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

	service := persistedConfigService(testFiles, testCreateTemp)
	err := service.Persist(context.Background(), path, document)
	if err == nil || !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("Persist() error = %v, want permission denied", err)
	}
	assertConfigSemanticallyUnchanged(t, service, path, original)
	assertNoTemporaryArtifacts(t, dir)
}

func TestConfigDocumentServicePersist_RejectsBeforeFilesystemSideEffects(t *testing.T) {
	t.Parallel()
	files := &faultFileSystem{FileSystem: testFiles}
	service := persistedConfigService(files, testCreateTemp)
	_, err := service.Parse([]byte(`{"workerPresets":[{"id":"invalid","modelProvider":"DEFAULT"}]}`))
	if err == nil {
		t.Fatal("Parse() error = nil, want invalid candidate")
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
			candidate, err := service.MergeProviderModelDefaults(base, operatorsettings.ProviderModelUpdate{Model: &model})
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

func TestConfigDocumentServiceConfigureProviderModel_InputPathsPersistEquivalentResults(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	promptedPath := filepath.Join(dir, "prompted.json")
	suppliedPath := filepath.Join(dir, "supplied.json")
	original := []byte(`{"backendScopeID":"local-11111111-1111-4111-8111-111111111111","defaults":{"workerModelProvider":"CODEX","workerModel":"existing"},"workerPresets":[{"id":"build","modelProvider":"codex"}]}`)
	for _, path := range []string{promptedPath, suppliedPath} {
		if err := os.WriteFile(path, original, 0o600); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", path, err)
		}
	}
	service := persistedConfigService(testFiles, testCreateTemp)
	provider, model := " anthropic ", " provider/model@next "
	update := operatorsettings.ProviderModelUpdate{Provider: &provider, Model: &model}
	var promptedDefaults operatorsettings.Defaults
	prompted, err := service.ConfigureProviderModelPrompted(
		context.Background(),
		promptedPath,
		func(_ context.Context, defaults operatorsettings.Defaults) (operatorsettings.ProviderModelUpdate, error) {
			promptedDefaults = defaults
			return update, nil
		},
	)
	if err != nil {
		t.Fatalf("ConfigureProviderModelPrompted() error = %v", err)
	}
	supplied, err := service.ConfigureProviderModel(context.Background(), suppliedPath, update)
	if err != nil {
		t.Fatalf("ConfigureProviderModel() error = %v", err)
	}
	if promptedDefaults != (operatorsettings.Defaults{WorkerModelProvider: "CODEX", WorkerModel: "existing"}) {
		t.Fatalf("prompt defaults = %#v, want existing defaults", promptedDefaults)
	}
	if !reflect.DeepEqual(prompted.FileConfig(), supplied.FileConfig()) || prompted.BackendScopeID() != supplied.BackendScopeID() {
		t.Fatalf("prompted result = %#v/%q, supplied = %#v/%q", prompted.FileConfig(), prompted.BackendScopeID(), supplied.FileConfig(), supplied.BackendScopeID())
	}
	for _, path := range []string{promptedPath, suppliedPath} {
		persisted, loadErr := service.Load(path)
		if loadErr != nil {
			t.Fatalf("Load(%q) error = %v", path, loadErr)
		}
		if !reflect.DeepEqual(persisted.FileConfig(), supplied.FileConfig()) || persisted.BackendScopeID() != supplied.BackendScopeID() {
			t.Fatalf("persisted result at %q = %#v/%q, want %#v/%q", path, persisted.FileConfig(), persisted.BackendScopeID(), supplied.FileConfig(), supplied.BackendScopeID())
		}
	}
}

func TestConfigDocumentServiceConfigureProviderModel_SerializesReadMergeReplaceTransactions(t *testing.T) {
	t.Parallel()
	path, _, _ := persistedConfigFixture(t)
	files := &readCountingFileSystem{FileSystem: testFiles}
	lock := &observedLocker{attempted: make(chan struct{}, 2)}
	encoderEntered := make(chan struct{})
	releaseEncoder := make(chan struct{})
	var encodeCalls atomic.Int32
	service := persistedConfigService(files, testCreateTemp)
	service.PersistenceLock = lock
	service.Encoder = func(config operatorsettings.Config) ([]byte, error) {
		if encodeCalls.Add(1) == 1 {
			close(encoderEntered)
			<-releaseEncoder
		}
		return encodeTestConfig(config)
	}

	provider := "claude"
	providerResult := make(chan error, 1)
	go func() {
		_, err := service.ConfigureProviderModel(
			context.Background(),
			path,
			operatorsettings.ProviderModelUpdate{Provider: &provider},
		)
		providerResult <- err
	}()
	<-encoderEntered
	<-lock.attempted

	model := "concurrent-model"
	modelResult := make(chan error, 1)
	go func() {
		_, err := service.ConfigureProviderModel(
			context.Background(),
			path,
			operatorsettings.ProviderModelUpdate{Model: &model},
		)
		modelResult <- err
	}()
	<-lock.attempted
	readsBeforeReplacement := files.reads.Load()
	close(releaseEncoder)
	if readsBeforeReplacement != 1 {
		t.Fatalf("reads before first replacement = %d, want 1 serialized transaction", readsBeforeReplacement)
	}
	if err := <-providerResult; err != nil {
		t.Fatalf("provider update error = %v", err)
	}
	if err := <-modelResult; err != nil {
		t.Fatalf("model update error = %v", err)
	}

	document, err := service.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := document.FileConfig().Defaults, (operatorsettings.Defaults{
		WorkerModelProvider: "CLAUDE",
		WorkerModel:         model,
	}); got != want {
		t.Fatalf("defaults = %#v, want both partial updates %#v", got, want)
	}
}

func TestConfigDocumentServiceConfigureProviderModelPrompted_InputStopsBeforePersistence(t *testing.T) {
	t.Parallel()
	promptFailure := errors.New("prompt failed")
	for _, test := range []struct {
		name    string
		prompt  func(context.CancelFunc) operatorsettings.ProviderModelPrompt
		wantErr error
	}{
		{name: "EOF", prompt: func(context.CancelFunc) operatorsettings.ProviderModelPrompt {
			return func(context.Context, operatorsettings.Defaults) (operatorsettings.ProviderModelUpdate, error) { return operatorsettings.ProviderModelUpdate{}, io.EOF }
		}, wantErr: operatorsettings.ErrProviderModelInputCanceled},
		{name: "cancellation", prompt: func(context.CancelFunc) operatorsettings.ProviderModelPrompt {
			return func(context.Context, operatorsettings.Defaults) (operatorsettings.ProviderModelUpdate, error) {
				return operatorsettings.ProviderModelUpdate{}, operatorsettings.ErrProviderModelInputCanceled
			}
		}, wantErr: operatorsettings.ErrProviderModelInputCanceled},
		{name: "interrupt", prompt: func(context.CancelFunc) operatorsettings.ProviderModelPrompt {
			return func(context.Context, operatorsettings.Defaults) (operatorsettings.ProviderModelUpdate, error) {
				return operatorsettings.ProviderModelUpdate{}, operatorsettings.ErrProviderModelInputCanceled
			}
		}, wantErr: operatorsettings.ErrProviderModelInputCanceled},
		{name: "observed context cancellation", prompt: func(cancel context.CancelFunc) operatorsettings.ProviderModelPrompt {
			return func(context.Context, operatorsettings.Defaults) (operatorsettings.ProviderModelUpdate, error) {
				cancel()
				provider := "codex"
				return operatorsettings.ProviderModelUpdate{Provider: &provider}, nil
			}
		}, wantErr: context.Canceled},
		{name: "prompt error", prompt: func(context.CancelFunc) operatorsettings.ProviderModelPrompt {
			return func(context.Context, operatorsettings.Defaults) (operatorsettings.ProviderModelUpdate, error) {
				return operatorsettings.ProviderModelUpdate{}, promptFailure
			}
		}, wantErr: promptFailure},
	} {
		for _, destination := range []string{"existing", "absent"} {
			t.Run(test.name+"/"+destination, func(t *testing.T) {
				t.Parallel()
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				original := []byte(`{"defaults":{"workerModelProvider":"CODEX","workerModel":"existing"}}`)
				if destination == "existing" {
					if err := os.WriteFile(path, original, 0o600); err != nil {
						t.Fatalf("WriteFile() error = %v", err)
					}
				}
				createCalls := 0
				service := persistedConfigService(testFiles, func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
					createCalls++
					return testCreateTemp(dir, pattern)
				})
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				_, err := service.ConfigureProviderModelPrompted(ctx, path, test.prompt(cancel))
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("ConfigureProviderModelPrompted() error = %v, want %v", err, test.wantErr)
				}
				if createCalls != 0 {
					t.Fatalf("temporary-file creations = %d, want zero", createCalls)
				}
				if destination == "existing" {
					assertConfigBytesUnchanged(t, path, original)
				} else if _, statErr := os.Stat(path); !errors.Is(statErr, fs.ErrNotExist) {
					t.Fatalf("absent destination Stat() error = %v, want not exist", statErr)
				}
			})
		}
	}
}

func TestConfigDocumentServiceConfigureProviderModel_PreCanceledContextHasNoFilesystemEffects(t *testing.T) {
	t.Parallel()
	files := &faultFileSystem{FileSystem: testFiles}
	createCalls := 0
	service := persistedConfigService(files, func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
		createCalls++
		return testCreateTemp(dir, pattern)
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	provider := "codex"
	_, err := service.ConfigureProviderModel(ctx, filepath.Join(t.TempDir(), "config.json"), operatorsettings.ProviderModelUpdate{Provider: &provider})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ConfigureProviderModel() error = %v, want context canceled", err)
	}
	if files.calls != 0 || createCalls != 0 {
		t.Fatalf("filesystem calls = %d, temporary-file creations = %d; want zero", files.calls, createCalls)
	}
}

func TestConfigDocumentServiceOperations_RejectMissingBoundaries(t *testing.T) {
	t.Parallel()
	valid := persistedConfigService(testFiles, testCreateTemp)
	document, err := valid.Parse([]byte(`{}`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	prompt := func(context.Context, operatorsettings.Defaults) (operatorsettings.ProviderModelUpdate, error) {
		return operatorsettings.ProviderModelUpdate{}, nil
	}
	for _, test := range []struct {
		name    string
		invoke  func() error
		wantErr string
	}{
		{name: "configure context", invoke: func() error {
			_, err := valid.ConfigureProviderModel(nil, "config.json", operatorsettings.ProviderModelUpdate{})
			return err
		}, wantErr: "context is required"},
		{name: "prompt", invoke: func() error {
			_, err := valid.ConfigureProviderModelPrompted(context.Background(), "config.json", nil)
			return err
		}, wantErr: "prompt is required"},
		{name: "prompt context", invoke: func() error {
			_, err := valid.ConfigureProviderModelPrompted(nil, "config.json", prompt)
			return err
		}, wantErr: "context is required"},
		{name: "persist context", invoke: func() error {
			return valid.Persist(nil, "config.json", document)
		}, wantErr: "context is required"},
		{name: "filesystem", invoke: func() error {
			return (operatorsettings.ConfigDocumentService{}).Persist(context.Background(), "config.json", document)
		}, wantErr: "filesystem is required"},
		{name: "temporary file creator", invoke: func() error {
			service := operatorsettings.ConfigDocumentService{Files: testFiles}
			return service.Persist(context.Background(), "config.json", document)
		}, wantErr: "temporary-file creator is required"},
		{name: "persistence lock", invoke: func() error {
			service := operatorsettings.ConfigDocumentService{Files: testFiles, CreateTemp: testCreateTemp}
			return service.Persist(context.Background(), "config.json", document)
		}, wantErr: "persistence lock is required"},
		{name: "path", invoke: func() error {
			return valid.Persist(context.Background(), "  ", document)
		}, wantErr: "path is required"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.invoke(); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("operation error = %v, want %q", err, test.wantErr)
			}
		})
	}
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

type readCountingFileSystem struct {
	operatorsettings.FileSystem
	reads atomic.Int32
}

func (files *readCountingFileSystem) ReadFile(path string) ([]byte, error) {
	files.reads.Add(1)
	return files.FileSystem.ReadFile(path)
}

type observedLocker struct {
	mutex     sync.Mutex
	attempted chan struct{}
}

func (lock *observedLocker) Lock() {
	lock.attempted <- struct{}{}
	lock.mutex.Lock()
}

func (lock *observedLocker) Unlock() {
	lock.mutex.Unlock()
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

func persistedConfigService(files operatorsettings.FileSystem, create operatorsettings.CreateTemporaryFile) operatorsettings.ConfigDocumentService {
	return operatorsettings.ConfigDocumentService{
		Files:           files,
		CreateTemp:      create,
		Providers:       controlledProviderCatalog,
		Decoder:         decodeTestConfig,
		Encoder:         encodeTestConfig,
		PersistenceLock: &sync.Mutex{},
	}
}

func persistedConfigFixture(t *testing.T) (string, []byte, operatorsettings.ConfigDocument) {
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
	document, err = service.MergeProviderModelDefaults(document, operatorsettings.ProviderModelUpdate{Model: &model})
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

func assertConfigSemanticallyUnchanged(t *testing.T, service operatorsettings.ConfigDocumentService, path string, want []byte) {
	t.Helper()
	wantDocument, err := service.Parse(want)
	if err != nil {
		t.Fatalf("Parse(original) error = %v", err)
	}
	gotDocument, err := service.Load(path)
	if err != nil {
		t.Fatalf("Load(destination) error = %v", err)
	}
	if !reflect.DeepEqual(gotDocument.FileConfig(), wantDocument.FileConfig()) || gotDocument.BackendScopeID() != wantDocument.BackendScopeID() {
		t.Fatalf("destination changed semantically: got %#v/%q want %#v/%q", gotDocument.FileConfig(), gotDocument.BackendScopeID(), wantDocument.FileConfig(), wantDocument.BackendScopeID())
	}
}

func assertNoTemporaryArtifacts(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "config.json.*.tmp"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary artifacts = %v, error = %v", matches, err)
	}
}
