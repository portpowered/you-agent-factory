package gallery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// Each cell targets the installer's one staging operation with controlled
// file/HTTP collaborators. No application graph or host process participates.
func TestLocalAIBinaryStagingFaultsDoNotPublishAndRecover(t *testing.T) {
	t.Parallel()
	for _, fault := range []string{"mkdir", "create", "write", "chmod", "close", "rename", "cancel"} {
		t.Run(fault, func(t *testing.T) {
			t.Parallel()
			installer := fixtureInstaller()
			directory := filepath.Join(t.TempDir(), "release")
			target := filepath.Join(directory, "binary")
			cause := errors.New("controlled staging failure")
			failed := true
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			configureStagingFault(installer, fault, &failed, cause, cancel)
			body := []byte("test")
			digest := sha256.Sum256(body)
			client := localAISelectedHTTPClient{do: func(request *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}, nil
			}}
			binary := localAIReleaseAsset{Name: "binary", URL: "https://selected.invalid/binary", Size: int64(len(body))}
			path, err := installer.stageLocalAIBinary(ctx, client, directory, target, binary, fmt.Sprintf("%x", digest))
			want := cause
			if fault == "cancel" {
				want = context.Canceled
			}
			if path != "" || !errors.Is(err, want) {
				t.Fatalf("fault %s: path=%q error=%v", fault, path, err)
			}
			if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("fault published target: %v", err)
			}
			staged, err := filepath.Glob(filepath.Join(directory, ".local-ai-*"))
			if err != nil || len(staged) != 0 {
				t.Fatalf("staging remains: %v %v", staged, err)
			}
			failed = false
			path, err = installer.stageLocalAIBinary(t.Context(), client, directory, target, binary, fmt.Sprintf("%x", digest))
			if err != nil || path != target {
				t.Fatalf("repair = %q %v", path, err)
			}
			got, err := os.ReadFile(target)
			if err != nil || !bytes.Equal(got, body) {
				t.Fatalf("published bytes = %q %v", got, err)
			}
		})
	}
}

type stagingFaultFile struct {
	*os.File
	fault  string
	cause  error
	failed bool
	cancel context.CancelFunc
}

func (file *stagingFaultFile) Write(data []byte) (int, error) {
	if file.failed && file.fault == "write" {
		return 0, file.cause
	}
	n, err := file.File.Write(data)
	if file.failed && file.fault == "cancel" {
		file.cancel()
	}
	return n, err
}
func (file *stagingFaultFile) Chmod(mode os.FileMode) error {
	if file.failed && file.fault == "chmod" {
		return file.cause
	}
	return file.File.Chmod(mode)
}
func (file *stagingFaultFile) Close() error {
	err := file.File.Close()
	if file.failed && file.fault == "close" {
		return file.cause
	}
	return err
}

func configureStagingFault(installer *Installer, fault string, failed *bool, cause error, cancel context.CancelFunc) {
	installer.MakeDirectories = func(path string, mode os.FileMode) error {
		if *failed && fault == "mkdir" {
			return cause
		}
		return os.MkdirAll(path, mode)
	}
	installer.CreateTempFile = func(directory, pattern string) (interface {
		io.Writer
		io.Closer
		Name() string
		Chmod(os.FileMode) error
	}, error) {
		if *failed && fault == "create" {
			return nil, cause
		}
		file, err := os.CreateTemp(directory, pattern)
		if err != nil {
			return nil, err
		}
		return &stagingFaultFile{File: file, fault: fault, cause: cause, failed: *failed, cancel: cancel}, nil
	}
	installer.RenamePath = func(from, to string) error {
		if *failed && fault == "rename" {
			return cause
		}
		return os.Rename(from, to)
	}
}
