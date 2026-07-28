package artifacts_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference/internal/artifacts"
)

func TestRegistrarExportsInvocationArtifactThroughInjectedFilesystem(t *testing.T) {
	t.Parallel()

	filesystem := &memoryFilesystem{files: map[string]string{
		"runtime/output.wav": "models-owned-bytes",
	}}
	registrar, err := artifacts.NewRegistrar(filesystem)
	if err != nil {
		t.Fatalf("NewRegistrar: %v", err)
	}
	artifact, err := registrar.Register(inference.InvocationArtifactSource{
		RefValue:   "models-inference:artifact:1",
		SourcePath: "runtime/output.wav",
		Name:       "output.wav",
		MediaType:  "audio/wav",
		SizeBytes:  17,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if artifact.Artifact.IsZero() || artifact.Name != "output.wav" {
		t.Fatalf("artifact = %#v, want detached metadata", artifact)
	}
	if err := registrar.ExportInvocationArtifact(artifact.Artifact, "customer/output.wav"); err != nil {
		t.Fatalf("ExportInvocationArtifact: %v", err)
	}
	if filesystem.files["customer/output.wav"] != "models-owned-bytes" {
		t.Fatalf("exported content = %q, want models-owned-bytes", filesystem.files["customer/output.wav"])
	}
}

func TestRegistrarRejectsInvalidArtifactReference(t *testing.T) {
	t.Parallel()

	registrar, err := artifacts.NewRegistrar(&memoryFilesystem{files: map[string]string{}})
	if err != nil {
		t.Fatalf("NewRegistrar: %v", err)
	}
	_, err = registrar.Register(inference.InvocationArtifactSource{RefValue: "  "})
	if !errors.Is(err, models.ErrInferenceArtifactInvalid) {
		t.Fatalf("Register invalid ref = %v, want ErrInferenceArtifactInvalid", err)
	}
}

type memoryFilesystem struct {
	files map[string]string
}

func (fs *memoryFilesystem) Open(path string) (io.ReadCloser, error) {
	content, ok := fs.files[path]
	if !ok {
		return nil, errors.New("not found")
	}
	return io.NopCloser(strings.NewReader(content)), nil
}

func (fs *memoryFilesystem) Create(path string) (io.WriteCloser, error) {
	return &memoryWriter{fs: fs, path: path}, nil
}

type memoryWriter struct {
	fs   *memoryFilesystem
	path string
	buf  strings.Builder
}

func (w *memoryWriter) Write(p []byte) (int, error) {
	return w.buf.Write(p)
}

func (w *memoryWriter) Close() error {
	w.fs.files[w.path] = w.buf.String()
	return nil
}
