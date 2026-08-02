package service_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

func TestInputEchoInvocationRuntimeReturnsDetachedContent(t *testing.T) {
	t.Parallel()

	result, err := inference.InputEchoInvocationRuntime{}.Invoke(
		context.Background(),
		inference.InvocationRuntimeRequest{
			Request: models.InvokeModelRequest{
				Input: models.InferenceInput{ContentType: "text/plain", Content: "hello"},
			},
		},
	)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(result.Content) != 1 || result.Content[0].Content != "hello" {
		t.Fatalf("content = %#v, want echoed input", result.Content)
	}
}

func TestInvokeModelWithLeaseReusesWarmHostSlotForConsecutiveInvokes(t *testing.T) {
	t.Parallel()

	scopes, scope := openInferenceScope(t, "invoke-handle-reuse", "scoped-model", "generate")
	catalog := mustCatalog(t, scopes)
	lease := mustLeaseRef(t, "lease-1")
	leaseTwo := mustLeaseRef(t, "lease-2")
	host := &recordingInferenceHost{
		leases: map[string]models.ModelLease{
			lease.String():    activeLease(scope, lease, "scoped-model", "worker-1"),
			leaseTwo.String(): activeLease(scope, leaseTwo, "scoped-model", "worker-1"),
		},
		warmHosts: make(map[string]bool),
	}
	runtime := &recordingInvocationRuntime{}
	service := newInferenceServiceWithHost(t, scopes, catalog, host, runtime, fixedClock(), nil)

	_, err := service.InvokeModelWithLease(
		context.Background(),
		invokeRequest(scope, lease, "worker-1", "scoped-model", "generate"),
	)
	if err != nil {
		t.Fatalf("first invoke: %v", err)
	}
	_, err = service.InvokeModelWithLease(
		context.Background(),
		invokeRequest(scope, leaseTwo, "worker-1", "scoped-model", "generate"),
	)
	if err != nil {
		t.Fatalf("second invoke: %v", err)
	}
	if host.ensureCalls != 1 {
		t.Fatalf("ensure calls = %d, want 1 shared host slot", host.ensureCalls)
	}
	if runtime.reusedHostSlots != 1 {
		t.Fatalf("reused host slots = %d, want 1", runtime.reusedHostSlots)
	}
}

func TestInvokeModelWithLeaseReturnsDetachedArtifactMetadata(t *testing.T) {
	t.Parallel()

	scopes, scope := openInferenceScope(t, "invoke-artifact", "scoped-model", "generate")
	catalog := mustCatalog(t, scopes)
	lease := mustLeaseRef(t, "lease-1")
	host := &recordingInferenceHost{
		leases: map[string]models.ModelLease{
			lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
		},
		warmHosts: make(map[string]bool),
	}
	runtime := &recordingInvocationRuntime{
		artifactSources: []inference.InvocationArtifactSource{{
			RefValue:   "models-inference:artifact:detached",
			SourcePath: "runtime/output.txt",
			Name:       "result.txt",
			MediaType:  "text/plain",
			SizeBytes:  19,
			Properties: map[string]string{"digest": "sha256:detached"},
		}},
	}
	service := newInferenceServiceWithHost(t, scopes, catalog, host, runtime, fixedClock(), nil)

	result, err := service.InvokeModelWithLease(
		context.Background(),
		invokeRequest(scope, lease, "worker-1", "scoped-model", "generate"),
	)
	if err != nil {
		t.Fatalf("InvokeModelWithLease: %v", err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Artifact.IsZero() {
		t.Fatalf("artifacts = %#v, want detached opaque artifact", result.Artifacts)
	}
	if result.Artifacts[0].Name != "result.txt" ||
		result.Artifacts[0].Properties["digest"] != "sha256:detached" {
		t.Fatalf("artifact metadata = %#v, want detached facts", result.Artifacts[0])
	}
	cloned := result.Clone()
	cloned.Artifacts[0].Properties["digest"] = "peer-mutated"
	if result.Artifacts[0].Properties["digest"] != "sha256:detached" {
		t.Fatalf("artifact metadata retained peer mutation: %#v", result.Artifacts)
	}
}

func TestInvokeModelWithLeaseRejectsInvalidArtifactReference(t *testing.T) {
	t.Parallel()

	scopes, scope := openInferenceScope(t, "invoke-artifact-invalid", "scoped-model", "generate")
	catalog := mustCatalog(t, scopes)
	lease := mustLeaseRef(t, "lease-1")
	host := &recordingInferenceHost{
		leases: map[string]models.ModelLease{
			lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
		},
		warmHosts: make(map[string]bool),
	}
	runtime := &recordingInvocationRuntime{
		artifactSources: []inference.InvocationArtifactSource{{
			RefValue: "   ",
			Name:     "result.txt",
		}},
	}
	service := newInferenceServiceWithHost(t, scopes, catalog, host, runtime, fixedClock(), nil)

	result, err := service.InvokeModelWithLease(
		context.Background(),
		invokeRequest(scope, lease, "worker-1", "scoped-model", "generate"),
	)
	if !errors.Is(err, models.ErrInferenceArtifactInvalid) {
		t.Fatalf("invalid artifact = %v, want ErrInferenceArtifactInvalid", err)
	}
	for _, other := range []error{
		models.ErrInferenceTimeout,
		models.ErrInferenceCancelled,
		models.ErrAssetUnavailable,
	} {
		if errors.Is(err, other) {
			t.Fatalf("invalid artifact must stay distinct from %v: %v", other, err)
		}
	}
	if result.Status != models.ModelInvocationStatusFailed {
		t.Fatalf("result status = %q, want FAILED", result.Status)
	}
}

type memoryArtifactFilesystem struct {
	files map[string]string
}

func (fs *memoryArtifactFilesystem) Open(path string) (io.ReadCloser, error) {
	content, ok := fs.files[path]
	if !ok {
		return nil, fmt.Errorf("open %q: not found", path)
	}
	return io.NopCloser(strings.NewReader(content)), nil
}

func (fs *memoryArtifactFilesystem) Create(path string) (io.WriteCloser, error) {
	return &memoryArtifactWriter{fs: fs, path: path}, nil
}

type memoryArtifactWriter struct {
	fs   *memoryArtifactFilesystem
	path string
	buf  strings.Builder
}

func (w *memoryArtifactWriter) Write(p []byte) (int, error) {
	return w.buf.Write(p)
}

func (w *memoryArtifactWriter) Close() error {
	w.fs.files[w.path] = w.buf.String()
	return nil
}
