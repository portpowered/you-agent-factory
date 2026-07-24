package workers_test

import (
	"context"
	"errors"
	"testing"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// fakeWorkersPeer is a peer-owned stand-in that depends only on the Workers
// root package (plus approved peer root contracts such as Models and Work). It
// proves cross-service consumers can satisfy the singular root Service without
// importing Workers implementation packages or provider/*.
type fakeWorkersPeer struct {
	lastModelName string
	lastRequest   modelinference.Request
	result        modelinference.Result
	err           error
}

func (f *fakeWorkersPeer) InvokeModel(
	_ context.Context,
	modelName string,
	request modelinference.Request,
) (modelinference.Result, error) {
	f.lastModelName = modelName
	f.lastRequest = request
	return f.result, f.err
}

var _ workers.Service = (*fakeWorkersPeer)(nil)

func TestServiceRootContract_FakeImplementsAndExercisesSingularSeam(t *testing.T) {
	fake := &fakeWorkersPeer{
		result: modelinference.Result{
			ModelName: "MODEL-A",
			Operation: "summarize",
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: "ok",
			}},
		},
	}
	// Peers consume only the singular root Service seam. RuntimeService
	// composition helpers are not required for the published root authority.
	var service workers.Service = fake
	ctx := context.Background()

	result, err := service.InvokeModel(ctx, "MODEL-A", modelinference.Request{
		Operation: "summarize",
		Content: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "hello",
		}},
	})
	if err != nil {
		t.Fatalf("InvokeModel: %v", err)
	}
	if result.ModelName != "MODEL-A" || len(result.Content) != 1 || result.Content[0].Text != "ok" {
		t.Fatalf("result = %#v, want MODEL-A content ok", result)
	}
	if fake.lastModelName != "MODEL-A" || fake.lastRequest.Operation != "summarize" {
		t.Fatalf(
			"routed = (%q, %#v), want MODEL-A summarize",
			fake.lastModelName,
			fake.lastRequest,
		)
	}
}

func TestServiceRootContract_FakeTypedFailureThroughSingularSeam(t *testing.T) {
	wantErr := modelinference.ErrNotFound
	fake := &fakeWorkersPeer{err: wantErr}
	var service workers.Service = fake

	_, err := service.InvokeModel(context.Background(), "missing", modelinference.Request{
		Operation: "chat",
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("InvokeModel error = %v, want %v", err, wantErr)
	}
}

func TestServiceRootContract_RuntimeServiceIsNotRequiredPeerAuthority(t *testing.T) {
	// Compile-time proof: a peer that only implements Service satisfies the
	// published Workers root without also implementing RuntimeService
	// composition methods (WithCommandRunners / WithProgressPublisher /
	// ProviderCommandInjected). Those remain Factory Session opening /
	// construction helpers, not the peer source of truth for runtime-build,
	// workstation-dispatch, or Runner-neutral slices.
	var service workers.Service = &fakeWorkersPeer{}
	if service == nil {
		t.Fatal("expected non-nil Service")
	}
	_ = workers.RuntimeService(nil)
}
