//go:build functionallong

package support

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func LastFactoryEventTick(events []factoryapi.FactoryEvent) int {
	tick := 0
	for _, event := range events {
		if event.Context.Tick > tick {
			tick = event.Context.Tick
		}
	}
	return tick
}

func UpdateFactoryConfig(t *testing.T, dir string, mutate func(map[string]any)) {
	t.Helper()

	path := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal factory.json: %v", err)
	}

	mutate(cfg)

	updated, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory.json: %v", err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}

func CountFactoryEvents(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
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
