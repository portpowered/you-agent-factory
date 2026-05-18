package support

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func ProviderErrorCorpusEntry(t *testing.T, name string) workers.ProviderErrorCorpusEntry {
	t.Helper()

	corpus, err := workers.LoadProviderErrorCorpus()
	if err != nil {
		t.Fatalf("workers.LoadProviderErrorCorpus() error = %v", err)
	}
	entry, ok := corpus.Entry(name)
	if !ok {
		t.Fatalf("provider error corpus entry %q not found", name)
	}
	return entry
}

func AcceptedProviderResponse() interfaces.InferenceResponse {
	return interfaces.InferenceResponse{Content: "COMPLETE"}
}

func RejectedProviderResponse(content string) interfaces.InferenceResponse {
	return interfaces.InferenceResponse{Content: content}
}

func ProviderCallsForWorker(provider *testutil.MockProvider, workerType string) []interfaces.ProviderInferenceRequest {
	var calls []interfaces.ProviderInferenceRequest
	for _, call := range provider.Calls() {
		if call.WorkerType == workerType {
			calls = append(calls, call)
		}
	}
	return calls
}
