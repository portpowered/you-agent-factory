package support

import (
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/pkg/workers"
)

func AcceptedProviderResponse() interfaces.InferenceResponse {
	return interfaces.InferenceResponse{Content: "COMPLETE"}
}

func RejectedProviderResponse(content string) interfaces.InferenceResponse {
	return interfaces.InferenceResponse{Content: content}
}

func AcceptedCommandResults(count int) []workers.CommandResult {
	results := make([]workers.CommandResult, count)
	for i := range results {
		results[i] = workers.CommandResult{Stdout: []byte("Done. COMPLETE")}
	}
	return results
}

func ProviderCommandRequestsForWorker(runner *testutil.ProviderCommandRunner, workerType string) []workers.CommandRequest {
	var requests []workers.CommandRequest
	for _, request := range runner.Requests() {
		if request.WorkerType == workerType {
			requests = append(requests, request)
		}
	}
	return requests
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
