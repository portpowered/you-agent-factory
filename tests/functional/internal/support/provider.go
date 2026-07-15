package support

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
)

func ProviderErrorCorpusEntry(t *testing.T, name string) workerprovider.ProviderErrorCorpusEntry {
	t.Helper()

	corpus, err := workerprovider.LoadProviderErrorCorpus()
	if err != nil {
		t.Fatalf("provider.LoadProviderErrorCorpus() error = %v", err)
	}
	entry, ok := corpus.Entry(name)
	if !ok {
		t.Fatalf("provider error corpus entry %q not found", name)
	}
	return entry
}

func AcceptedProviderResponse() workerexecution.InferenceResponse {
	return workerexecution.InferenceResponse{Content: "COMPLETE"}
}

func RejectedProviderResponse(content string) workerexecution.InferenceResponse {
	return workerexecution.InferenceResponse{Content: content}
}

func CursorProviderSuccessStdout(result string) []byte {
	if result == "" {
		result = "Done. COMPLETE"
	}
	systemPayload := map[string]any{
		"type":       "system",
		"subtype":    "init",
		"session_id": "cursor-functional-test-session",
	}
	resultPayload := map[string]any{
		"type":       "result",
		"subtype":    "success",
		"is_error":   false,
		"result":     result,
		"session_id": "cursor-functional-test-session",
	}
	systemEncoded, err := json.Marshal(systemPayload)
	if err != nil {
		panic(err)
	}
	resultEncoded, err := json.Marshal(resultPayload)
	if err != nil {
		panic(err)
	}
	return append(append(systemEncoded, '\n'), resultEncoded...)
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

func ProviderCallsForWorker(provider *testutil.MockProvider, workerType string) []workerexecution.ProviderInferenceRequest {
	var calls []workerexecution.ProviderInferenceRequest
	for _, call := range provider.Calls() {
		if call.WorkerType == workerType {
			calls = append(calls, call)
		}
	}
	return calls
}
