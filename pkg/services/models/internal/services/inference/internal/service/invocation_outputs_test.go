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

func TestInvocationArtifactFailuresAreAtomic(t *testing.T) {
	t.Parallel()

	text := "héllo 🌍"
	tests := []struct {
		name      string
		mutate    func(*inference.InvocationRuntimeResult)
		wantClass models.InvocationFailureClass
		wantCause error
	}{
		{
			name:      "missing descriptor",
			mutate:    func(result *inference.InvocationRuntimeResult) { result.Artifacts = nil },
			wantClass: models.InvocationFailureClassArtifact,
			wantCause: models.ErrInferenceArtifactInvalid,
		},
		{
			name: "invalid reference",
			mutate: func(result *inference.InvocationRuntimeResult) {
				result.Artifacts[0].RefValue = "private://backend/cache"
			},
			wantClass: models.InvocationFailureClassArtifact,
			wantCause: models.ErrInferenceArtifactInvalid,
		},
		{
			name: "mismatched name",
			mutate: func(result *inference.InvocationRuntimeResult) {
				result.Artifacts[0].Name = "usage"
			},
			wantClass: models.InvocationFailureClassArtifact,
			wantCause: models.ErrInferenceArtifactInvalid,
		},
		{
			name: "mismatched media",
			mutate: func(result *inference.InvocationRuntimeResult) {
				result.Artifacts[0].MediaType = "application/json"
			},
			wantClass: models.InvocationFailureClassArtifact,
			wantCause: models.ErrInferenceArtifactInvalid,
		},
		{
			name: "negative size",
			mutate: func(result *inference.InvocationRuntimeResult) {
				result.Artifacts[0].SizeBytes = -1
			},
			wantClass: models.InvocationFailureClassArtifact,
			wantCause: models.ErrInferenceArtifactInvalid,
		},
		{
			name: "oversized size",
			mutate: func(result *inference.InvocationRuntimeResult) {
				result.Artifacts[0].SizeBytes = 16<<20 + 1
			},
			wantClass: models.InvocationFailureClassArtifact,
			wantCause: models.ErrInferenceArtifactInvalid,
		},
		{
			name: "mismatched size",
			mutate: func(result *inference.InvocationRuntimeResult) {
				result.Artifacts[0].SizeBytes++
			},
			wantClass: models.InvocationFailureClassArtifact,
			wantCause: models.ErrInferenceArtifactInvalid,
		},
		{
			name: "malformed text",
			mutate: func(result *inference.InvocationRuntimeResult) {
				result.Content[0].Modality = models.ModalityJSON
			},
			wantClass: models.InvocationFailureClassMalformedResponse,
			wantCause: models.ErrInferenceFailed,
		},
		{
			name: "blank text",
			mutate: func(result *inference.InvocationRuntimeResult) {
				result.Content[0].Content = "  "
			},
			wantClass: models.InvocationFailureClassMalformedResponse,
			wantCause: models.ErrInferenceFailed,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			scopes, scope := openInferenceScope(t, "omni-atomic-"+strings.ReplaceAll(test.name, " ", "-"), "scoped-model", models.OperationOMNI)
			lease := mustLeaseRef(t, "omni-lease")
			host := &recordingInferenceHost{leases: map[string]models.ModelLease{
				lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
			}}
			runtimeResult := inference.InvocationRuntimeResult{
				Content: []models.InferenceContent{{
					Name: "text", Modality: models.ModalityText,
					ContentType: "text/plain", MediaType: "text/plain", Content: text,
				}},
				Artifacts: []inference.InvocationArtifactSource{{
					Name: "text", MediaType: "text/plain", SizeBytes: int64(len([]byte(text))),
				}},
			}
			test.mutate(&runtimeResult)
			runtime := &recordingInvocationRuntime{content: runtimeResult.Content, artifactSources: runtimeResult.Artifacts}
			service := newInferenceServiceWithHost(
				t, scopes, capabilityCatalog{detail: omniDetail()}, host, runtime, fixedClock(), nil,
			)

			result, err := service.InvokeModelWithLease(context.Background(), omniInvocationRequest(scope, lease))
			var failure *models.InvocationFailure
			if err == nil || !errors.As(err, &failure) || failure.Class != test.wantClass || failure.Slot != "text" || !errors.Is(err, test.wantCause) {
				t.Fatalf("error = %v, failure = %#v, want class %q/cause %v", err, failure, test.wantClass, test.wantCause)
			}
			if result.Status != models.ModelInvocationStatusFailed || result.LeaseDisposition != models.InvocationLeaseReleased ||
				len(result.Content) != 0 || len(result.Outputs) != 0 || len(result.Artifacts) != 0 {
				t.Fatalf("result = %#v, want atomic failed result without output or artifact", result)
			}
			if runtime.invokeCalls != 1 || host.releaseCalls != 1 {
				t.Fatalf("runtime/release calls = %d/%d, want 1/1", runtime.invokeCalls, host.releaseCalls)
			}
		})
	}
}

func TestInvocationArtifactFailureRedactsRuntimeDetails(t *testing.T) {
	t.Parallel()

	const sentinel = "OMNI_PRIVATE_ENDPOINT_SECRET_CACHE_SENTINEL"
	scopes, scope := openInferenceScope(t, "omni-redact", "scoped-model", models.OperationOMNI)
	lease := mustLeaseRef(t, "omni-lease")
	host := &recordingInferenceHost{leases: map[string]models.ModelLease{
		lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
	}}
	text := "safe answer"
	runtime := &recordingInvocationRuntime{
		content: []models.InferenceContent{{
			Name: "text", Modality: models.ModalityText,
			ContentType: "text/plain", MediaType: "text/plain", Content: text,
		}},
		artifactSources: []inference.InvocationArtifactSource{{
			Name: "text", MediaType: "text/plain", SizeBytes: int64(len([]byte(text))),
			SourcePath: sentinel, Properties: map[string]string{"endpoint": sentinel, "secret": sentinel},
		}},
	}
	service := newInferenceServiceWithHost(
		t, scopes, capabilityCatalog{detail: omniDetail()}, host, runtime, fixedClock(), nil,
	)

	result, err := service.InvokeModelWithLease(context.Background(), omniInvocationRequest(scope, lease))
	if err == nil || !errors.Is(err, models.ErrInferenceArtifactInvalid) || strings.Contains(err.Error(), sentinel) {
		t.Fatalf("redacted artifact failure = %v, want safe typed failure", err)
	}
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassArtifact || failure.Message == "" {
		t.Fatalf("failure = %#v, want safe artifact classification", failure)
	}
	if result.Status != models.ModelInvocationStatusFailed || host.releaseCalls != 1 {
		t.Fatalf("result/release = %#v/%d, want failed and one release", result, host.releaseCalls)
	}
}

func TestOMNIBackendFailureRedactsRuntimeDetails(t *testing.T) {
	t.Parallel()

	const sentinel = "OMNI_RAW_PROTOCOL_ENDPOINT_SECRET_SENTINEL"
	scopes, scope := openInferenceScope(t, "omni-backend-redact", "scoped-model", models.OperationOMNI)
	lease := mustLeaseRef(t, "omni-lease")
	host := &recordingInferenceHost{leases: map[string]models.ModelLease{
		lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
	}}
	runtime := &recordingInvocationRuntime{invokeErr: errors.New(sentinel)}
	service := newInferenceServiceWithHost(
		t, scopes, capabilityCatalog{detail: omniDetail()}, host, runtime, fixedClock(), nil,
	)

	result, err := service.InvokeModelWithLease(context.Background(), omniInvocationRequest(scope, lease))
	if err == nil || strings.Contains(err.Error(), sentinel) {
		t.Fatalf("raw OMNI backend failure = %v, want redacted error", err)
	}
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassBackendProtocol || failure.Message == "" {
		t.Fatalf("failure = %#v, want safe backend protocol classification", failure)
	}
	if result.Status != models.ModelInvocationStatusFailed || host.releaseCalls != 1 {
		t.Fatalf("result/release = %#v/%d, want failed and one release", result, host.releaseCalls)
	}
}

func omniDetail() models.Detail {
	operation, _ := (models.GenericOperationCatalog{}).GenericOperationContract(models.OperationOMNI)
	return models.Detail{
		Summary:      models.Summary{Name: "scoped-model"},
		Capabilities: []models.Capability{{Operations: []models.Operation{operation}}},
	}
}

func omniInvocationRequest(scope models.RuntimeScopeRef, lease models.ModelLeaseRef) models.InvokeModelRequest {
	return models.InvokeModelRequest{
		Scope: scope, Lease: lease, Holder: "worker-1", ModelName: "scoped-model",
		Model: models.ModelReference{NameOrURI: "scoped-model"}, Operation: models.OperationOMNI,
		Inputs: []models.InferenceInput{{
			Name: "prompt", Modality: models.ModalityText,
			ContentType: "text/plain", MediaType: "text/plain", Content: "hello",
		}},
	}
}

func TestInvokeModelWithLeaseRegistersValidOMNIArtifactAtomically(t *testing.T) {
	t.Parallel()

	text := "héllo 🌍"
	scopes, scope := openInferenceScope(t, "omni-success", "scoped-model", models.OperationOMNI)
	lease := mustLeaseRef(t, "omni-lease")
	host := &recordingInferenceHost{leases: map[string]models.ModelLease{
		lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
	}}
	runtime := &recordingInvocationRuntime{
		content: []models.InferenceContent{{
			Name: "text", Modality: models.ModalityText,
			ContentType: "text/plain", MediaType: "text/plain", Content: text,
		}},
		artifactSources: []inference.InvocationArtifactSource{{
			Name: "text", MediaType: "text/plain", SizeBytes: int64(len([]byte(text))),
		}},
	}
	service := newInferenceServiceWithHost(
		t, scopes, capabilityCatalog{detail: omniDetail()}, host, runtime, fixedClock(), nil,
	)

	result, err := service.InvokeModelWithLease(context.Background(), omniInvocationRequest(scope, lease))
	if err != nil {
		t.Fatalf("InvokeModelWithLease: %v", err)
	}
	if result.Status != models.ModelInvocationStatusCompleted || result.LeaseDisposition != models.InvocationLeaseReleased ||
		len(result.Outputs) != 1 || result.Outputs[0].Name != "text" || result.Outputs[0].Content != text {
		t.Fatalf("result = %#v, want one semantic completed text output", result)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Artifact.IsZero() || result.Artifacts[0].MediaType != "text/plain" ||
		result.Artifacts[0].SizeBytes != int64(len([]byte(text))) {
		t.Fatalf("artifacts = %#v, want one opaque UTF-8-sized text artifact", result.Artifacts)
	}
	if result.Outputs[0].Artifact == nil || result.Outputs[0].Artifact.Artifact != result.Artifacts[0].Artifact {
		t.Fatalf("output artifact = %#v, want registered artifact identity", result.Outputs[0].Artifact)
	}
	if host.releaseCalls != 1 {
		t.Fatalf("lease release calls = %d, want 1", host.releaseCalls)
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
