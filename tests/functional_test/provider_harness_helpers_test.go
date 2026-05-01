package functional_test

import (
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func acceptedProviderResponse() interfaces.InferenceResponse {
	return interfaces.InferenceResponse{Content: "COMPLETE"}
}

func rejectedProviderResponse(content string) interfaces.InferenceResponse {
	return interfaces.InferenceResponse{Content: content}
}

func acceptedCommandResults(count int) []workers.CommandResult {
	results := make([]workers.CommandResult, count)
	for i := range results {
		results[i] = workers.CommandResult{Stdout: []byte("Done. COMPLETE")}
	}
	return results
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

func providerCommandRequestsForWorker(runner *testutil.ProviderCommandRunner, workerType string) []workers.CommandRequest {
	var requests []workers.CommandRequest
	for _, request := range runner.Requests() {
		if request.WorkerType == workerType {
			requests = append(requests, request)
		}
	}
	return requests
}
