package functional_test

import (
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
)

func acceptedProviderResponse() interfaces.InferenceResponse {
	return interfaces.InferenceResponse{Content: "COMPLETE"}
}

func rejectedProviderResponse(content string) interfaces.InferenceResponse {
	return interfaces.InferenceResponse{Content: content}
}

func providerCallsForWorker(provider *testutil.MockProvider, workerType string) []interfaces.ProviderInferenceRequest {
	var calls []interfaces.ProviderInferenceRequest
	for _, call := range provider.Calls() {
		if call.WorkerType == workerType {
			calls = append(calls, call)
		}
	}
	return calls
}
