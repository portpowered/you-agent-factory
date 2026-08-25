package service_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai/codecs"
	embeddingruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/runtime"
)

func TestEmbeddingRuntimeMapsFixtureThroughModelsAndReleasesLease(t *testing.T) {
	scopes, scope := openInferenceScope(t, "embed-success", "scoped-model", models.OperationEMBED)
	lease := mustLeaseRef(t, "embed-lease")
	host := &recordingInferenceHost{
		leases: map[string]models.ModelLease{
			lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
		},
	}
	backend := &recordingEmbeddingBackend{
		response: codecs.EmbeddingResponse{Embeddings: []float64{0.1, 0.2, 0.3, 0.4}},
	}
	runtime, err := embeddingruntime.NewEmbedding(func(ctx context.Context, request codecs.EmbeddingRequest) (codecs.EmbeddingResponse, error) {
		return backend.InvokeEmbedding(ctx, request)
	})
	if err != nil {
		t.Fatalf("construct embedding runtime: %v", err)
	}
	service := newInferenceServiceWithHost(
		t,
		scopes,
		embeddingCatalog(),
		host,
		runtime,
		fixedClock(),
		nil,
	)

	request := embeddingRequest(scope, lease)
	result, err := service.InvokeModelWithLease(context.Background(), request)
	if err != nil {
		t.Fatalf("InvokeModelWithLease: %v", err)
	}
	if result.Status != models.ModelInvocationStatusCompleted || result.LeaseDisposition != models.InvocationLeaseReleased {
		t.Fatalf("result = %#v, want completed released invocation", result)
	}
	if len(result.Outputs) != 1 || result.Outputs[0].Name != "embedding" || result.Outputs[0].Modality != models.ModalityJSON || result.Outputs[0].Content != `[0.1,0.2,0.3,0.4]` {
		t.Fatalf("outputs = %#v, want one canonical embedding output", result.Outputs)
	}
	if backend.calls != 1 || backend.request.Prompt != "Find similar work" {
		t.Fatalf("backend calls/request = %d/%#v", backend.calls, backend.request)
	}
	if backend.request.Parameters["normalize"] != true || fmt.Sprint(backend.request.Parameters["dimensions"]) != "4" {
		t.Fatalf("backend parameters = %#v, want preserved supported values", backend.request.Parameters)
	}
	if host.releaseCalls != 1 {
		t.Fatalf("lease release calls = %d, want 1", host.releaseCalls)
	}
}

func TestEmbeddingRuntimeFailuresAreTypedAtomicAndReleaseLease(t *testing.T) {
	tests := []struct {
		name       string
		backendErr error
		response   codecs.EmbeddingResponse
		mutate     func(*models.InvokeModelRequest)
		class      models.InvocationFailureClass
	}{
		{
			name: "invalid parameters",
			mutate: func(request *models.InvokeModelRequest) {
				request.Inputs[1].Content = `{"temperature":0.2}`
			},
			class: models.InvocationFailureClassInvalidParameter,
		},
		{
			name:       "backend failure",
			backendErr: errors.New("backend endpoint https://private.invalid token=secret cache=/private/cache"),
			class:      models.InvocationFailureClassBackendProtocol,
		},
		{
			name:     "malformed response",
			response: codecs.EmbeddingResponse{},
			class:    models.InvocationFailureClassMalformedResponse,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scopes, scope := openInferenceScope(t, "embed-failure-"+strings.ReplaceAll(test.name, " ", "-"), "scoped-model", models.OperationEMBED)
			lease := mustLeaseRef(t, "embed-lease")
			host := &recordingInferenceHost{
				leases: map[string]models.ModelLease{
					lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
				},
			}
			backend := &recordingEmbeddingBackend{backendErr: test.backendErr, response: test.response}
			runtime, err := embeddingruntime.NewEmbedding(func(ctx context.Context, request codecs.EmbeddingRequest) (codecs.EmbeddingResponse, error) {
				return backend.InvokeEmbedding(ctx, request)
			})
			if err != nil {
				t.Fatalf("construct embedding runtime: %v", err)
			}
			service := newInferenceServiceWithHost(t, scopes, embeddingCatalog(), host, runtime, fixedClock(), nil)

			request := embeddingRequest(scope, lease)
			if test.mutate != nil {
				test.mutate(&request)
			}
			result, err := service.InvokeModelWithLease(context.Background(), request)
			if err == nil {
				t.Fatal("InvokeModelWithLease() error = nil")
			}
			var failure *models.InvocationFailure
			if !errors.As(err, &failure) || failure.Class != test.class {
				t.Fatalf("error = %v, failure = %#v, want class %q", err, failure, test.class)
			}
			if strings.Contains(err.Error(), "private.invalid") || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "private/cache") {
				t.Fatalf("failure leaked backend detail: %v", err)
			}
			if result.Status != models.ModelInvocationStatusFailed || result.LeaseDisposition != models.InvocationLeaseReleased || len(result.Content) != 0 || len(result.Outputs) != 0 {
				t.Fatalf("result = %#v, want atomic failed released result", result)
			}
			if host.releaseCalls != 1 {
				t.Fatalf("lease release calls = %d, want 1", host.releaseCalls)
			}
		})
	}
}

func TestEmbeddingRuntimeCancellationReleasesLease(t *testing.T) {
	scopes, scope := openInferenceScope(t, "embed-cancel", "scoped-model", models.OperationEMBED)
	lease := mustLeaseRef(t, "embed-lease")
	host := &recordingInferenceHost{
		leases: map[string]models.ModelLease{
			lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
		},
	}
	backend := &recordingEmbeddingBackend{waitForCancellation: true}
	runtime, err := embeddingruntime.NewEmbedding(func(ctx context.Context, request codecs.EmbeddingRequest) (codecs.EmbeddingResponse, error) {
		return backend.InvokeEmbedding(ctx, request)
	})
	if err != nil {
		t.Fatalf("construct embedding runtime: %v", err)
	}
	service := newInferenceServiceWithHost(t, scopes, embeddingCatalog(), host, runtime, fixedClock(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	backend.started = started
	go func() {
		<-started
		cancel()
	}()

	result, err := service.InvokeModelWithLease(ctx, embeddingRequest(scope, lease))
	if !errors.Is(err, models.ErrInferenceCancelled) {
		t.Fatalf("InvokeModelWithLease() error = %v, want cancellation", err)
	}
	if result.Status != models.ModelInvocationStatusCancelled || result.LeaseDisposition != models.InvocationLeaseReleased || host.releaseCalls != 1 {
		t.Fatalf("result = %#v, release calls = %d, want cancelled released invocation", result, host.releaseCalls)
	}
}

type recordingEmbeddingBackend struct {
	request             codecs.EmbeddingRequest
	response            codecs.EmbeddingResponse
	backendErr          error
	waitForCancellation bool
	started             chan struct{}
	calls               int
}

func (backend *recordingEmbeddingBackend) InvokeEmbedding(ctx context.Context, request codecs.EmbeddingRequest) (codecs.EmbeddingResponse, error) {
	backend.calls++
	backend.request = request
	if backend.started != nil {
		close(backend.started)
		backend.started = nil
	}
	if backend.waitForCancellation {
		<-ctx.Done()
		return codecs.EmbeddingResponse{}, ctx.Err()
	}
	if backend.backendErr != nil {
		return codecs.EmbeddingResponse{}, backend.backendErr
	}
	return backend.response, nil
}

func embeddingCatalog() *capabilityCatalog {
	return &capabilityCatalog{detail: models.Detail{
		Summary: models.Summary{Name: "scoped-model"},
		Capabilities: []models.Capability{{Operations: []models.Operation{{
			Name: models.OperationEMBED,
			Inputs: []models.OperationSlot{
				{Name: "text", Modality: models.ModalityText, Required: boolPointer(true), MediaTypes: []string{"text/plain"}},
				{Name: "parameters", Modality: models.ModalityJSON, MediaTypes: []string{"application/json"}},
			},
			Outputs: []models.OperationSlot{{Name: "embedding", Modality: models.ModalityJSON, Required: boolPointer(true), MediaTypes: []string{"application/json"}}},
		}}}},
	}}
}

func embeddingRequest(scope models.RuntimeScopeRef, lease models.ModelLeaseRef) models.InvokeModelRequest {
	return models.InvokeModelRequest{
		Scope:     scope,
		Lease:     lease,
		Holder:    "worker-1",
		ModelName: "scoped-model",
		Model:     models.ModelReference{NameOrURI: "scoped-model"},
		Operation: models.OperationEMBED,
		Inputs: []models.InferenceInput{
			{
				Name:        "text",
				Modality:    models.ModalityText,
				ContentType: "text/plain",
				MediaType:   "text/plain",
				Content:     "Find similar work",
			},
			{
				Name:        "parameters",
				Modality:    models.ModalityJSON,
				ContentType: "application/json",
				MediaType:   "application/json",
				Content:     `{"normalize":true,"dimensions":4}`,
			},
		},
	}
}
