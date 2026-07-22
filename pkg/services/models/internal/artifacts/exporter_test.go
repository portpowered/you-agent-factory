package artifacts

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

type readCloser struct{ io.Reader }

func (readCloser) Close() error { return nil }

type writeCloser struct{ bytes.Buffer }

func (*writeCloser) Close() error { return nil }

type fileSystemStub struct {
	openedPath  string
	createdPath string
	input       io.ReadCloser
	output      *writeCloser
	openErr     error
	createErr   error
}

func (s *fileSystemStub) Open(path string) (io.ReadCloser, error) {
	s.openedPath = path
	return s.input, s.openErr
}

func (s *fileSystemStub) Create(path string) (io.WriteCloser, error) {
	s.createdPath = path
	return s.output, s.createErr
}

func TestExporterCopiesInvocationArtifactThroughInjectedFileSystem(t *testing.T) {
	t.Parallel()

	output := &writeCloser{}
	filesystem := &fileSystemStub{
		input:  readCloser{Reader: strings.NewReader("RIFF....WAVE")},
		output: output,
	}
	exporter, err := NewExporter(filesystem)
	if err != nil {
		t.Fatalf("NewExporter: %v", err)
	}
	if err := exporter.ExportInvocationArtifact("runtime.wav", "customer.wav"); err != nil {
		t.Fatalf("ExportInvocationArtifact: %v", err)
	}
	if filesystem.openedPath != "runtime.wav" || filesystem.createdPath != "customer.wav" {
		t.Fatalf("paths = (%q, %q), want runtime and customer paths", filesystem.openedPath, filesystem.createdPath)
	}
	if got := output.String(); got != "RIFF....WAVE" {
		t.Fatalf("output = %q", got)
	}
}

func TestExporterReportsInjectedFileSystemFailures(t *testing.T) {
	t.Parallel()

	if exporter, err := NewExporter(nil); exporter != nil || err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("NewExporter(nil) = (%v, %v), want dependency error", exporter, err)
	}
	exporter, err := NewExporter(&fileSystemStub{openErr: errors.New("denied")})
	if err != nil {
		t.Fatalf("NewExporter: %v", err)
	}
	if err := exporter.ExportInvocationArtifact("runtime.wav", "customer.wav"); err == nil || !strings.Contains(err.Error(), "open streamed invocation output: denied") {
		t.Fatalf("ExportInvocationArtifact error = %v", err)
	}
}
