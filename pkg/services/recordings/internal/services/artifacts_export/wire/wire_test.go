package wire_test

import (
	"context"
	"io/fs"
	"testing"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	artifactsexportwire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/wire"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
)

type snapshotSourceStub struct{}

type publicationFileStub struct {
	name    string
	payload []byte
	called  *[]string
}

func (file *publicationFileStub) Write(payload []byte) (int, error) {
	file.payload = append(file.payload[:0], payload...)
	return len(payload), nil
}

func (file *publicationFileStub) Name() string { return file.name }

func (file *publicationFileStub) Chmod(fs.FileMode) error {
	*file.called = append(*file.called, "chmod")
	return nil
}

func (file *publicationFileStub) Sync() error {
	*file.called = append(*file.called, "sync")
	return nil
}

func (file *publicationFileStub) Close() error {
	*file.called = append(*file.called, "close")
	return nil
}

func (snapshotSourceStub) Snapshot(recordings.RecordingID) (recordinglifecycle.Snapshot, error) {
	return recordinglifecycle.Snapshot{}, recordings.ErrMissingRecordingTarget
}

func TestNewServiceConstructsArtifactsExportCapability(t *testing.T) {
	t.Parallel()

	if service := artifactsexportwire.NewService(snapshotSourceStub{}, nil); service == nil {
		t.Fatal("NewService() = nil")
	}
}

func TestNewPublicationUsesInjectedEffects(t *testing.T) {
	t.Parallel()

	calls := []string{}
	temporary := &publicationFileStub{name: "artifact.tmp", called: &calls}
	publication, err := artifactsexportwire.NewPublication(
		func(string, fs.FileMode) error {
			calls = append(calls, "mkdir")
			return nil
		},
		func(string, string) (recordings.RecordingTemporaryFile, error) {
			calls = append(calls, "create")
			return temporary, nil
		},
		func(string) error {
			calls = append(calls, "remove")
			return nil
		},
		func(string, string) error {
			calls = append(calls, "rename")
			return nil
		},
		func(string) ([]byte, error) {
			calls = append(calls, "read")
			return []byte("published"), nil
		},
	)
	if err != nil {
		t.Fatalf("NewPublication() error = %v", err)
	}
	if err := publication.Publish(context.Background(), "artifact.json", []byte("payload")); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if got, err := publication.Read(context.Background(), "artifact.json"); err != nil || string(got) != "published" {
		t.Fatalf("Read() = (%q, %v), want published payload", got, err)
	}
	for _, name := range []string{"mkdir", "create", "chmod", "sync", "close", "rename", "remove", "read"} {
		if !containsString(calls, name) {
			t.Fatalf("injected effect %q was not called; calls = %#v", name, calls)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
