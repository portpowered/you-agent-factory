package wire_test

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	artifactsexportservice "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/internal/service"
	artifactsexportwire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/wire"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
)

type snapshotSourceStub struct{}

type publicationFileStub struct {
	name     string
	calls    *[]string
	payload  []byte
	chmodErr error
	syncErr  error
	closeErr error
	writeErr error
}

func (file *publicationFileStub) Write(payload []byte) (int, error) {
	*file.calls = append(*file.calls, "write")
	if file.writeErr != nil {
		return 0, file.writeErr
	}
	file.payload = append(file.payload[:0], payload...)
	return len(payload), nil
}

func (file *publicationFileStub) Name() string { return file.name }

func (file *publicationFileStub) Chmod(mode fs.FileMode) error {
	*file.calls = append(*file.calls, "chmod")
	if file.chmodErr != nil {
		return file.chmodErr
	}
	if mode.Perm() != 0o600 {
		return errors.New("unexpected temporary-file mode")
	}
	return nil
}

func (file *publicationFileStub) Sync() error {
	*file.calls = append(*file.calls, "sync")
	if file.syncErr != nil {
		return file.syncErr
	}
	return nil
}

func (file *publicationFileStub) Close() error {
	*file.calls = append(*file.calls, "close")
	if file.closeErr != nil {
		return file.closeErr
	}
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

func TestNewPublicationUsesInjectedEffectsInAtomicOrder(t *testing.T) {
	t.Parallel()

	var calls []string
	var mkdirPath string
	var mkdirMode fs.FileMode
	var temporaryDir, temporaryPattern string
	var renameSource, renameDestination, removedPath, readPath string
	temporary := &publicationFileStub{name: filepath.Join("publish", "artifact.json.tmp"), calls: &calls}

	publication, err := artifactsexportwire.NewPublication(
		func(path string, mode fs.FileMode) error {
			calls = append(calls, "mkdir")
			mkdirPath, mkdirMode = path, mode
			return nil
		},
		func(dir, pattern string) (recordings.RecordingTemporaryFile, error) {
			calls = append(calls, "temp")
			temporaryDir, temporaryPattern = dir, pattern
			return temporary, nil
		},
		func(path string) error {
			calls = append(calls, "remove")
			removedPath = path
			return nil
		},
		func(source, destination string) error {
			calls = append(calls, "rename")
			renameSource, renameDestination = source, destination
			return nil
		},
		func(path string) ([]byte, error) {
			calls = append(calls, "read")
			readPath = path
			return []byte("published"), nil
		},
	)
	if err != nil {
		t.Fatalf("NewPublication() error = %v", err)
	}

	destination := filepath.Join("publish", "artifact.json")
	if err := publication.Publish(context.Background(), destination, []byte("payload")); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	read, err := publication.Read(context.Background(), destination)
	if err != nil || string(read) != "published" {
		t.Fatalf("Read() = (%q, %v), want published", read, err)
	}

	if want := []string{"mkdir", "temp", "chmod", "write", "sync", "close", "rename", "remove", "read"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("injected effect order = %#v, want %#v", calls, want)
	}
	if mkdirPath != filepath.Dir(destination) || mkdirMode.Perm() != 0o700 {
		t.Fatalf("mkdir effect = (%q, %v), want destination directory and 0700", mkdirPath, mkdirMode.Perm())
	}
	if temporaryDir != filepath.Dir(destination) || temporaryPattern != "artifact.json.*.tmp" {
		t.Fatalf("temporary effect = (%q, %q), want destination directory and artifact pattern", temporaryDir, temporaryPattern)
	}
	if string(temporary.payload) != "payload" {
		t.Fatalf("temporary payload = %q, want payload", temporary.payload)
	}
	if renameSource != temporary.Name() || renameDestination != destination || removedPath != temporary.Name() {
		t.Fatalf("publish paths = rename(%q, %q), remove(%q); want temp=%q destination=%q", renameSource, renameDestination, removedPath, temporary.Name(), destination)
	}
	if readPath != destination {
		t.Fatalf("read path = %q, want %q", readPath, destination)
	}
}

func TestNewPublicationCleansTemporaryFileWhenInjectedWriteFails(t *testing.T) {
	t.Parallel()

	writeErr := errors.New("write blocked")
	var calls []string
	temporary := &publicationFileStub{
		name:     "artifact.tmp",
		calls:    &calls,
		writeErr: writeErr,
	}
	publication, err := artifactsexportwire.NewPublication(
		func(string, fs.FileMode) error { calls = append(calls, "mkdir"); return nil },
		func(string, string) (recordings.RecordingTemporaryFile, error) {
			calls = append(calls, "temp")
			return temporary, nil
		},
		func(string) error { calls = append(calls, "remove"); return nil },
		func(string, string) error { calls = append(calls, "rename"); return nil },
		func(string) ([]byte, error) { calls = append(calls, "read"); return nil, nil },
	)
	if err != nil {
		t.Fatalf("NewPublication() error = %v", err)
	}
	if err := publication.Publish(context.Background(), "artifact.json", []byte("payload")); !errors.Is(err, writeErr) {
		t.Fatalf("Publish() error = %v, want write error", err)
	}
	if want := []string{"mkdir", "temp", "chmod", "write", "close", "remove"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("write-failure effects = %#v, want %#v", calls, want)
	}
}

func TestNewPublicationPropagatesInjectedOperationErrors(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name       string
		operation  string
		makeErr    error
		createErr  error
		writeErr   error
		renameErr  error
		removeErr  error
		readErr    error
		wantCalls  []string
		wantResult error
	}
	cases := []testCase{
		{
			name:       "mkdir",
			operation:  "publish",
			makeErr:    errors.New("mkdir blocked"),
			wantCalls:  []string{"mkdir"},
			wantResult: errors.New("mkdir blocked"),
		},
		{
			name:       "create",
			operation:  "publish",
			createErr:  errors.New("create blocked"),
			wantCalls:  []string{"mkdir", "temp"},
			wantResult: errors.New("create blocked"),
		},
		{
			name:       "write",
			operation:  "publish",
			writeErr:   errors.New("write blocked"),
			wantCalls:  []string{"mkdir", "temp", "chmod", "write", "close", "remove"},
			wantResult: errors.New("write blocked"),
		},
		{
			name:       "rename",
			operation:  "publish",
			renameErr:  errors.New("rename blocked"),
			wantCalls:  []string{"mkdir", "temp", "chmod", "write", "sync", "close", "rename", "remove"},
			wantResult: errors.New("rename blocked"),
		},
		{
			name:      "remove cleanup is best effort",
			operation: "publish",
			removeErr: errors.New("remove blocked"),
			wantCalls: []string{"mkdir", "temp", "chmod", "write", "sync", "close", "rename", "remove"},
		},
		{
			name:       "read",
			operation:  "read",
			readErr:    errors.New("read blocked"),
			wantCalls:  []string{"read"},
			wantResult: errors.New("read blocked"),
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var calls []string
			file := &publicationFileStub{
				name:     "artifact.tmp",
				calls:    &calls,
				writeErr: testCase.writeErr,
			}
			publication, err := artifactsexportwire.NewPublication(
				func(string, fs.FileMode) error {
					calls = append(calls, "mkdir")
					return testCase.makeErr
				},
				func(string, string) (recordings.RecordingTemporaryFile, error) {
					calls = append(calls, "temp")
					if testCase.createErr != nil {
						return nil, testCase.createErr
					}
					return file, nil
				},
				func(string) error {
					calls = append(calls, "remove")
					return testCase.removeErr
				},
				func(string, string) error {
					calls = append(calls, "rename")
					return testCase.renameErr
				},
				func(string) ([]byte, error) {
					calls = append(calls, "read")
					return nil, testCase.readErr
				},
			)
			if err != nil {
				t.Fatalf("NewPublication() error = %v", err)
			}

			var operationErr error
			if testCase.operation == "publish" {
				operationErr = publication.Publish(context.Background(), "artifact.json", []byte("payload"))
			} else {
				_, operationErr = publication.Read(context.Background(), "artifact.json")
			}
			if testCase.wantResult == nil {
				if operationErr != nil {
					t.Fatalf("operation error = %v, want nil", operationErr)
				}
			} else if operationErr == nil || operationErr.Error() == "" || !strings.Contains(operationErr.Error(), testCase.wantResult.Error()) {
				t.Fatalf("operation error = %v, want %q", operationErr, testCase.wantResult)
			}
			if !reflect.DeepEqual(calls, testCase.wantCalls) {
				t.Fatalf("injected effects = %#v, want %#v", calls, testCase.wantCalls)
			}
		})
	}
}

func TestNewPublicationRejectsMissingEffect(t *testing.T) {
	t.Parallel()

	validMakeDirectories := func(string, fs.FileMode) error { return nil }
	validCreateTemporaryFile := func(string, string) (recordings.RecordingTemporaryFile, error) {
		return nil, nil
	}
	validRemove := func(string) error { return nil }
	validRename := func(string, string) error { return nil }
	validRead := func(string) ([]byte, error) { return nil, nil }
	cases := []struct {
		name string
		make func() (artifactsexportservice.PortableArtifactPublication, error)
	}{
		{name: "mkdir", make: func() (artifactsexportservice.PortableArtifactPublication, error) {
			return artifactsexportwire.NewPublication(nil, validCreateTemporaryFile, validRemove, validRename, validRead)
		}},
		{name: "temp", make: func() (artifactsexportservice.PortableArtifactPublication, error) {
			return artifactsexportwire.NewPublication(validMakeDirectories, nil, validRemove, validRename, validRead)
		}},
		{name: "remove", make: func() (artifactsexportservice.PortableArtifactPublication, error) {
			return artifactsexportwire.NewPublication(validMakeDirectories, validCreateTemporaryFile, nil, validRename, validRead)
		}},
		{name: "rename", make: func() (artifactsexportservice.PortableArtifactPublication, error) {
			return artifactsexportwire.NewPublication(validMakeDirectories, validCreateTemporaryFile, validRemove, nil, validRead)
		}},
		{name: "read", make: func() (artifactsexportservice.PortableArtifactPublication, error) {
			return artifactsexportwire.NewPublication(validMakeDirectories, validCreateTemporaryFile, validRemove, validRename, nil)
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if publication, err := testCase.make(); err == nil || publication != nil {
				t.Fatalf("NewPublication() = (%#v, %v), want missing %s effect error", publication, err, testCase.name)
			}
		})
	}
}
