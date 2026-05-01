package support

import (
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/pkg/workers"
)

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
