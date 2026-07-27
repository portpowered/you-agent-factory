package wire_test

import (
	"errors"
	"io/fs"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingsdocumentwire "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document/wire"
)

func TestNewServiceConstructsInertDocumentOwner(t *testing.T) {
	t.Parallel()

	files := &recordingFileSystem{}
	createTemp := newRecordingCreateTemporaryFile()
	decoder := newRecordingConfigDecoder()
	encoder := newRecordingConfigEncoder()
	providers := newRecordingProviderCatalog()

	service := settingsdocumentwire.NewService(
		files,
		createTemp.fn,
		decoder.fn,
		encoder.fn,
		providers.fn,
	)
	if service == nil {
		t.Fatal("NewService() = nil")
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
	if providers.calls != 0 {
		t.Fatalf("construction invoked provider catalog %d times, want inert construction", providers.calls)
	}

	_, err := service.LoadDocument(operatorsettings.LoadDocumentRequest{
		Path: "/home/operator/.you-agent-factory/config.json",
	})
	if err == nil {
		t.Fatal("LoadDocument() = nil, want error before document load is implemented")
	}
	if files.readCalls != 0 {
		t.Fatalf("LoadDocument() invoked ReadFile during unavailable load, want no filesystem read yet")
	}
	if decoder.calls != 0 {
		t.Fatalf("LoadDocument() invoked decoder during unavailable load, want no codec decode yet")
	}
}

func TestNewServiceRejectsMalformedLoadWithoutFilesystemOrCodecEffects(t *testing.T) {
	t.Parallel()

	service := settingsdocumentwire.NewService(
		&recordingFileSystem{},
		newRecordingCreateTemporaryFile().fn,
		newRecordingConfigDecoder().fn,
		newRecordingConfigEncoder().fn,
		newRecordingProviderCatalog().fn,
	)
	_, err := service.LoadDocument(operatorsettings.LoadDocumentRequest{})
	if !errors.Is(err, operatorsettings.ErrDocumentMalformed) {
		t.Fatalf("LoadDocument() = %v, want ErrDocumentMalformed", err)
	}
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
