package wire_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	internaltestproviders "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testproviders"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func testIDGenerator() operatorsettings.IDGenerator {
	return func() string { return "00000000-0000-4000-8000-000000000001" }
}

func TestNewServiceConstructsPublishedRoot(t *testing.T) {
	t.Parallel()

	providersRoot := internaltestproviders.StandardCatalog()

	service, err := settingswire.NewService(
		&stubFileSystem{},
		stubCreateTemporaryFile,
		stubConfigDecoder,
		stubConfigEncoder,
		stubProviderCatalog,
		providersRoot,
		testIDGenerator(),
		nil,
	)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}

	var root operatorsettings.Service = service
	if root == nil {
		t.Fatal("constructed value is not assignable to operatorsettings.Service")
	}
}

func TestNewServiceServesPublishedPeerBehavior(t *testing.T) {
	t.Parallel()

	providersRoot := internaltestproviders.StandardCatalog()

	service, err := settingswire.NewService(
		&stubFileSystem{},
		stubCreateTemporaryFile,
		stubConfigDecoder,
		stubConfigEncoder,
		stubProviderCatalog,
		providersRoot,
		testIDGenerator(),
		nil,
	)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}

	var root operatorsettings.Service = service
	if root == nil {
		t.Fatal("constructed root is nil")
	}

	configPath := "/home/operator/.you-agent-factory/config.json"
	baseline := operatorsettings.DocumentDefaults{
		WorkerModelProvider: "codex",
		WorkerModel:         "gpt-5",
	}
	resolved, err := root.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: baseline,
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "gemini",
			WorkerModel:         "flag-model",
		},
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("ResolveEffective() = %v", err)
	}
	if resolved.Selection.WorkerModelProvider != "GEMINI" ||
		resolved.Selection.WorkerModel != "flag-model" ||
		resolved.Selection.ConfigPath != configPath {
		t.Fatalf("ResolveEffective() = %#v", resolved.Selection)
	}

	_, err = root.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "unsupported-provider",
		},
		ConfigPath: configPath,
	})
	if !errors.Is(err, operatorsettings.ErrResolutionUnsupportedOverride) {
		t.Fatalf("unsupported override error = %v, want ErrResolutionUnsupportedOverride", err)
	}

	_, err = root.LoadDocument(operatorsettings.LoadDocumentRequest{})
	if !errors.Is(err, operatorsettings.ErrDocumentMalformed) {
		t.Fatalf("empty load path error = %v, want ErrDocumentMalformed", err)
	}
}

func TestNewServiceUsesInjectedIDGeneratorForBackendScope(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("WriteFile(config) = %v", err)
	}

	service, err := settingswire.NewService(
		platformfilesystem.Local{},
		func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		globalconfigmapping.Decode,
		globalconfigmapping.Encode,
		stubProviderCatalog,
		internaltestproviders.StandardCatalog(),
		testIDGenerator(),
		nil,
	)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}

	resolved, err := service.EnsureLocalBackendScope(configPath)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope() = %v", err)
	}
	want := operatorsettings.LocalBackendScopePrefix + "00000000-0000-4000-8000-000000000001"
	if resolved.BackendScopeID != want || resolved.Outcome != operatorsettings.BackendScopeOutcomeGenerated {
		t.Fatalf("EnsureLocalBackendScope() = %#v, want generated scope %q", resolved, want)
	}
}

func TestNewServiceConstructsInertRoot(t *testing.T) {
	t.Parallel()

	files := &recordingFileSystem{}
	createTemp := newRecordingCreateTemporaryFile()
	decoder := newRecordingConfigDecoder()
	encoder := newRecordingConfigEncoder()
	providersCatalog := newRecordingProviderCatalog()
	providersRoot := &recordingProvidersRoot{}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	service, err := settingswire.NewService(
		files,
		createTemp.fn,
		decoder.fn,
		encoder.fn,
		providersCatalog.fn,
		providersRoot,
		testIDGenerator(),
		nil,
	)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}

	var root operatorsettings.Service = service
	if root == nil {
		t.Fatal("constructed value is not assignable to operatorsettings.Service")
	}

	if files.readCalls != 0 {
		t.Fatalf("construction invoked ReadFile %d times, want inert construction", files.readCalls)
	}
	if files.mkdirCalls != 0 || files.removeCalls != 0 || files.chmodCalls != 0 || files.renameCalls != 0 {
		t.Fatalf(
			"construction invoked filesystem mutations (mkdir=%d remove=%d chmod=%d rename=%d), want inert construction",
			files.mkdirCalls,
			files.removeCalls,
			files.chmodCalls,
			files.renameCalls,
		)
	}
	if createTemp.calls != 0 {
		t.Fatalf("construction invoked temp-file creation %d times, want inert construction", createTemp.calls)
	}
	if decoder.calls != 0 {
		t.Fatalf("construction invoked decoder %d times, want inert construction", decoder.calls)
	}
	if encoder.calls != 0 {
		t.Fatalf("construction invoked encoder %d times, want inert construction", encoder.calls)
	}
	if providersCatalog.calls != 0 {
		t.Fatalf("construction invoked provider catalog %d times, want inert construction", providersCatalog.calls)
	}
	if providersRoot.listCalls != 0 || providersRoot.getCalls != 0 || providersRoot.executeCalls != 0 {
		t.Fatalf(
			"construction invoked providers root (list=%d get=%d execute=%d), want inert construction",
			providersRoot.listCalls,
			providersRoot.getCalls,
			providersRoot.executeCalls,
		)
	}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	if leaked := runtime.NumGoroutine() - baseline; leaked > 4 {
		t.Fatalf(
			"goroutine leak after construction: baseline=%d current=%d delta=%d, want no lifecycle goroutines",
			baseline, runtime.NumGoroutine(), leaked,
		)
	}
}

func TestNewServiceRejectsMissingFileSystem(t *testing.T) {
	t.Parallel()

	assertNewServiceRejectsMissingPort(t, func() (operatorsettings.Service, error) {
		return settingswire.NewService(
			nil,
			stubCreateTemporaryFile,
			stubConfigDecoder,
			stubConfigEncoder,
			stubProviderCatalog,
			internaltestproviders.StandardCatalog(),
			testIDGenerator(),
			nil,
		)
	}, "construct Operator Settings: filesystem is required")
}

func TestNewServiceRejectsMissingTemporaryFileCreator(t *testing.T) {
	t.Parallel()

	assertNewServiceRejectsMissingPort(t, func() (operatorsettings.Service, error) {
		return settingswire.NewService(
			&stubFileSystem{},
			nil,
			stubConfigDecoder,
			stubConfigEncoder,
			stubProviderCatalog,
			internaltestproviders.StandardCatalog(),
			testIDGenerator(),
			nil,
		)
	}, "construct Operator Settings: create temporary file is required")
}

func TestNewServiceRejectsMissingConfigDecoder(t *testing.T) {
	t.Parallel()

	assertNewServiceRejectsMissingPort(t, func() (operatorsettings.Service, error) {
		return settingswire.NewService(
			&stubFileSystem{},
			stubCreateTemporaryFile,
			nil,
			stubConfigEncoder,
			stubProviderCatalog,
			internaltestproviders.StandardCatalog(),
			testIDGenerator(),
			nil,
		)
	}, "construct Operator Settings: config decoder is required")
}

func TestNewServiceRejectsMissingConfigEncoder(t *testing.T) {
	t.Parallel()

	assertNewServiceRejectsMissingPort(t, func() (operatorsettings.Service, error) {
		return settingswire.NewService(
			&stubFileSystem{},
			stubCreateTemporaryFile,
			stubConfigDecoder,
			nil,
			stubProviderCatalog,
			internaltestproviders.StandardCatalog(),
			testIDGenerator(),
			nil,
		)
	}, "construct Operator Settings: config encoder is required")
}

func TestNewServiceRejectsMissingProviderCatalog(t *testing.T) {
	t.Parallel()

	assertNewServiceRejectsMissingPort(t, func() (operatorsettings.Service, error) {
		return settingswire.NewService(
			&stubFileSystem{},
			stubCreateTemporaryFile,
			stubConfigDecoder,
			stubConfigEncoder,
			nil,
			internaltestproviders.StandardCatalog(),
			testIDGenerator(),
			nil,
		)
	}, "construct Operator Settings: provider catalog is required")
}

func TestNewServiceRejectsMissingProvidersRoot(t *testing.T) {
	t.Parallel()

	assertNewServiceRejectsMissingPort(t, func() (operatorsettings.Service, error) {
		return settingswire.NewService(
			&stubFileSystem{},
			stubCreateTemporaryFile,
			stubConfigDecoder,
			stubConfigEncoder,
			stubProviderCatalog,
			nil,
			testIDGenerator(),
			nil,
		)
	}, "construct Operator Settings: providers root is required")
}

func TestNewServiceRejectsMissingIDGenerator(t *testing.T) {
	t.Parallel()

	assertNewServiceRejectsMissingPort(t, func() (operatorsettings.Service, error) {
		return settingswire.NewService(
			&stubFileSystem{},
			stubCreateTemporaryFile,
			stubConfigDecoder,
			stubConfigEncoder,
			stubProviderCatalog,
			internaltestproviders.StandardCatalog(),
			nil,
			nil,
		)
	}, "construct Operator Settings: ID generator is required")
}

func assertNewServiceRejectsMissingPort(
	t *testing.T,
	call func() (operatorsettings.Service, error),
	want string,
) {
	t.Helper()

	service, err := call()
	if err == nil {
		t.Fatalf("call = (%v, nil), want error %q", service, want)
	}
	if service != nil {
		t.Fatalf("call = (%v, %v), want nil service", service, err)
	}
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

type stubFileSystem struct{}

func (stubFileSystem) ReadFile(string) ([]byte, error) {
	panic("filesystem read during wire construction test")
}

func (stubFileSystem) MkdirAll(string, fs.FileMode) error {
	panic("filesystem mkdir during wire construction test")
}

func (stubFileSystem) Remove(string) error {
	panic("filesystem remove during wire construction test")
}

func (stubFileSystem) Chmod(string, fs.FileMode) error {
	panic("filesystem chmod during wire construction test")
}

func (stubFileSystem) Rename(string, string) error {
	panic("filesystem rename during wire construction test")
}

func stubCreateTemporaryFile(string, string) (operatorsettings.TemporaryFile, error) {
	panic("temp-file creation during wire construction test")
}

func stubConfigDecoder([]byte) (operatorsettings.Config, error) {
	panic("config decode during wire construction test")
}

func stubConfigEncoder(operatorsettings.Config) ([]byte, error) {
	panic("config encode during wire construction test")
}

func stubProviderCatalog(string) (string, bool) {
	panic("provider catalog during wire construction test")
}

type recordingFileSystem struct {
	readCalls   int
	mkdirCalls  int
	removeCalls int
	chmodCalls  int
	renameCalls int
}

func (files *recordingFileSystem) ReadFile(string) ([]byte, error) {
	files.readCalls++
	panic("filesystem read during inert construction")
}

func (files *recordingFileSystem) MkdirAll(string, fs.FileMode) error {
	files.mkdirCalls++
	panic("filesystem mkdir during inert construction")
}

func (files *recordingFileSystem) Remove(string) error {
	files.removeCalls++
	panic("filesystem remove during inert construction")
}

func (files *recordingFileSystem) Chmod(string, fs.FileMode) error {
	files.chmodCalls++
	panic("filesystem chmod during inert construction")
}

func (files *recordingFileSystem) Rename(string, string) error {
	files.renameCalls++
	panic("filesystem rename during inert construction")
}

type recordingCreateTemporaryFile struct {
	calls int
	fn    operatorsettings.CreateTemporaryFile
}

func newRecordingCreateTemporaryFile() *recordingCreateTemporaryFile {
	recorder := &recordingCreateTemporaryFile{}
	recorder.fn = func(string, string) (operatorsettings.TemporaryFile, error) {
		recorder.calls++
		panic("temp-file creation during inert construction")
	}
	return recorder
}

type recordingConfigDecoder struct {
	calls int
	fn    operatorsettings.ConfigDecoder
}

func newRecordingConfigDecoder() *recordingConfigDecoder {
	recorder := &recordingConfigDecoder{}
	recorder.fn = func([]byte) (operatorsettings.Config, error) {
		recorder.calls++
		panic("config decode during inert construction")
	}
	return recorder
}

type recordingConfigEncoder struct {
	calls int
	fn    operatorsettings.ConfigEncoder
}

func newRecordingConfigEncoder() *recordingConfigEncoder {
	recorder := &recordingConfigEncoder{}
	recorder.fn = func(operatorsettings.Config) ([]byte, error) {
		recorder.calls++
		panic("config encode during inert construction")
	}
	return recorder
}

type recordingProviderCatalog struct {
	calls int
	fn    operatorsettings.ProviderCatalog
}

func newRecordingProviderCatalog() *recordingProviderCatalog {
	recorder := &recordingProviderCatalog{}
	recorder.fn = func(string) (string, bool) {
		recorder.calls++
		panic("provider catalog during inert construction")
	}
	return recorder
}

type recordingProvidersRoot struct {
	providers.Service
	listCalls    int
	getCalls     int
	executeCalls int
}

var _ providers.Service = (*recordingProvidersRoot)(nil)

func (root *recordingProvidersRoot) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	root.listCalls++
	panic("providers list during inert construction")
}

func (root *recordingProvidersRoot) GetProvider(
	context.Context,
	providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	root.getCalls++
	panic("providers get during inert construction")
}

func (root *recordingProvidersRoot) Execute(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	root.executeCalls++
	panic("providers execute during inert construction")
}
