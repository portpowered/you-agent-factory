package workers_test

import (
	"context"
	"testing"

	workers "github.com/portpowered/infinite-you/pkg/services/workers"
	inferencecontract "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

type rootProviderStub struct{}

func (rootProviderStub) Infer(
	context.Context,
	workers.ProviderInferenceRequest,
) (workers.InferenceResponse, error) {
	return workers.InferenceResponse{}, nil
}

func TestProviderPortExposedAtWorkersRoot(t *testing.T) {
	t.Parallel()

	var provider workers.Provider = rootProviderStub{}
	if provider == nil {
		t.Fatal("workers.Provider assignment failed")
	}
}

func TestProviderPortMatchesInferenceContract(t *testing.T) {
	t.Parallel()

	var nested inferencecontract.Provider = rootProviderStub{}
	var root workers.Provider = nested
	if root == nil {
		t.Fatal("workers.Provider is not assignable from inferencecontract.Provider")
	}
}
