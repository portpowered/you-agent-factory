package workers_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// peerProviderFake is a Runtime-facing consumer that names the Workers root
// Provider port without importing pkg/services/workers/provider/inferencecontract.
type peerProviderFake struct {
	response workers.InferenceResponse
	err      error
	calls    int
}

var _ workers.Provider = (*peerProviderFake)(nil)

func (fake *peerProviderFake) Infer(
	_ context.Context,
	request workers.ProviderInferenceRequest,
) (workers.InferenceResponse, error) {
	fake.calls++
	if request.UserMessage == "" {
		return workers.InferenceResponse{}, workers.NewProviderError(
			workers.WorkFailureTypePermanentBadRequest,
			"user message is required",
			nil,
		)
	}
	return fake.response, fake.err
}

func TestWorkersRootProviderPortIsNamedByPeerConsumers(t *testing.T) {
	t.Parallel()

	var provider workers.Provider = &peerProviderFake{
		response: workers.InferenceResponse{Content: "root-port"},
	}

	response, err := provider.Infer(context.Background(), workers.ProviderInferenceRequest{
		UserMessage: "hello",
	})
	if err != nil {
		t.Fatalf("Infer() error = %v", err)
	}
	if response.Content != "root-port" {
		t.Fatalf("response content = %q, want %q", response.Content, "root-port")
	}
}
