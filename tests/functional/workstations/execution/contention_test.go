//go:build functionallong

package execution

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	contendingPrimaryWorkstation   = "process"
	contendingAlternateWorkstation = "process-alternate"
)

// TestEligibleWorkstationContentionChoosesOneDispatchOnly proves that when
// multiple execution workstations are eligible for the same Work item, the
// factory chooses exactly one dispatch per eligibility cycle and routes the
// Work to the success path without duplicate concurrent dispatches, observed
// through public Work and Factory Event projections.
func TestEligibleWorkstationContentionChoosesOneDispatchOnly(t *testing.T) {
	support.SkipLongFunctional(t, "slow execution-workstation contention sweep")

	t.Run("competing_workstations_choose_exactly_one_dispatch", runCompetingWorkstations)
	t.Run("shared_executor_resolves_distinct_workstations_in_order", runSharedExecutorWorkstations)
	t.Run("distinct_workers_resolve_their_bound_workstations", runDistinctWorkerWorkstations)
}

func runCompetingWorkstations(t *testing.T) {
	dir := support.ScaffoldFactory(t, contendingExecutionFactoryConfig())
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	support.WriteAgentConfig(t, dir, "worker-b", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"item":"contended"}`))

	provider := testutil.NewMockProvider(workerexecution.InferenceResponse{Content: "Done. COMPLETE"})
	session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		15*time.Second,
	)
	dispatches := support.ObserveDispatchEvents(t, events)

	if provider.CallCount() != 1 {
		t.Fatalf("provider call count = %d, want 1 when competing workstations contend", provider.CallCount())
	}
	if session.Runtime.Progress.Categories.Terminal != 1 || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf("session progress categories = %+v, want one terminal and zero failed", session.Runtime.Progress.Categories)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "complete")); got != 1 {
		t.Fatalf("CountWorkAtCustomerState(task:complete) = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "init")); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(task:init) = %d, want 0 after completion", got)
	}

	workID := terminalTaskWorkIDAtState(t, listed, "complete")
	if completed := countCompletedDispatchesForWork(dispatches, workID); completed != 1 {
		t.Fatalf("completed dispatch count for work %q = %d, want 1; dispatches=%#v", workID, completed, dispatches)
	}
	fired := competingWorkstationsThatDispatched(dispatches, workID)
	if len(fired) != 1 {
		t.Fatalf("competing workstations that dispatched work %q = %v, want exactly one", workID, fired)
	}
	if fired[0] != contendingPrimaryWorkstation && fired[0] != contendingAlternateWorkstation {
		t.Fatalf("dispatch workstation = %q, want %q or %q", fired[0], contendingPrimaryWorkstation, contendingAlternateWorkstation)
	}
}

func runSharedExecutorWorkstations(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "stateless_collector"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"item":"shared-executor"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Stage 1 done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Stage 2 done. COMPLETE"},
	)
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		15*time.Second,
	)
	dispatches := support.ObserveDispatchEvents(t, events)

	if provider.CallCount() != 2 {
		t.Fatalf("provider call count = %d, want 2 for shared-executor staged completion", provider.CallCount())
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "done")); got != 1 {
		t.Fatalf("CountWorkAtCustomerState(task:done) = %d, want 1; listed=%#v", got, listed)
	}

	workID := terminalTaskWorkIDAtState(t, listed, "done")
	assertExactlyOneCompletedDispatchPerWorkAtWorkstation(t, dispatches, workID, statelessCollectorStage1Workstation)
	assertExactlyOneCompletedDispatchPerWorkAtWorkstation(t, dispatches, workID, statelessCollectorStage2Workstation)

	calls := provider.Calls()
	if calls[0].WorkstationType != statelessCollectorStage1Workstation {
		t.Fatalf("first dispatch workstation = %q, want %q", calls[0].WorkstationType, statelessCollectorStage1Workstation)
	}
	if calls[1].WorkstationType != statelessCollectorStage2Workstation {
		t.Fatalf("second dispatch workstation = %q, want %q", calls[1].WorkstationType, statelessCollectorStage2Workstation)
	}
	if calls[0].Model != "test-model" || calls[1].Model != "test-model" {
		t.Fatalf("expected shared model test-model, got %q and %q", calls[0].Model, calls[1].Model)
	}
	if !strings.Contains(calls[0].UserMessage, "Step 1 workstation.") {
		t.Fatalf("first dispatch prompt = %q, want step1 workstation prompt", calls[0].UserMessage)
	}
	if !strings.Contains(calls[1].UserMessage, "Step 2 workstation.") {
		t.Fatalf("second dispatch prompt = %q, want step2 workstation prompt", calls[1].UserMessage)
	}
}

func runDistinctWorkerWorkstations(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "stateless_collector"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"item":"different-workers"}`))
	rewriteStatelessCollectorForDifferentWorkers(t, dir)

	work := map[string][]workerexecution.InferenceResponse{
		"agent-a": {{Content: "Stage 1 done. COMPLETE"}},
		"agent-b": {{Content: "Stage 2 done. COMPLETE"}},
	}
	provider := testutil.NewMockWorkerMapProvider(work)
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		15*time.Second,
	)
	dispatches := support.ObserveDispatchEvents(t, events)

	if provider.CallCount("agent-a") != 1 {
		t.Fatalf("agent-a call count = %d, want 1", provider.CallCount("agent-a"))
	}
	if provider.CallCount("agent-b") != 1 {
		t.Fatalf("agent-b call count = %d, want 1", provider.CallCount("agent-b"))
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "done")); got != 1 {
		t.Fatalf("CountWorkAtCustomerState(task:done) = %d, want 1; listed=%#v", got, listed)
	}

	workID := terminalTaskWorkIDAtState(t, listed, "done")
	assertExactlyOneCompletedDispatchPerWorkAtWorkstation(t, dispatches, workID, statelessCollectorStage1Workstation)
	assertExactlyOneCompletedDispatchPerWorkAtWorkstation(t, dispatches, workID, statelessCollectorStage2Workstation)

	first := provider.Calls("agent-a")[0]
	second := provider.Calls("agent-b")[0]
	if first.WorkstationType != statelessCollectorStage1Workstation {
		t.Fatalf("agent-a workstation = %q, want %q", first.WorkstationType, statelessCollectorStage1Workstation)
	}
	if second.WorkstationType != statelessCollectorStage2Workstation {
		t.Fatalf("agent-b workstation = %q, want %q", second.WorkstationType, statelessCollectorStage2Workstation)
	}
	if !strings.Contains(first.UserMessage, "Step 1 workstation.") {
		t.Fatalf("agent-a prompt = %q, want step1 workstation prompt", first.UserMessage)
	}
	if !strings.Contains(second.UserMessage, "Step 2 workstation.") {
		t.Fatalf("agent-b prompt = %q, want step2 workstation prompt", second.UserMessage)
	}
}

// TestContentionMakesProgressAcrossRepeatedWork proves that submitting
// repeated Work items while multiple execution workstations remain eligible
// still drives each item forward to the Factory success path without stalls
// or unbounded duplicate dispatches, observed through public Work and
// Factory Event projections.
func TestContentionMakesProgressAcrossRepeatedWork(t *testing.T) {
	support.SkipLongFunctional(t, "slow execution-workstation repeated-contention sweep")

	const repeatedWorkCount = 3

	dir := support.ScaffoldFactory(t, contendingExecutionFactoryConfig())
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	support.WriteAgentConfig(t, dir, "worker-b", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	for i := 1; i <= repeatedWorkCount; i++ {
		testutil.WriteSeedFile(t, dir, "task", []byte(fmt.Sprintf(`{"item":"contended-%d"}`, i)))
	}

	responses := make([]workerexecution.InferenceResponse, repeatedWorkCount)
	for i := range responses {
		responses[i] = workerexecution.InferenceResponse{Content: "Done. COMPLETE"}
	}
	provider := testutil.NewMockProvider(responses...)

	session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		30*time.Second,
	)
	dispatches := support.ObserveDispatchEvents(t, events)

	if provider.CallCount() != repeatedWorkCount {
		t.Fatalf("provider call count = %d, want %d when repeated work contends", provider.CallCount(), repeatedWorkCount)
	}
	if session.Runtime.Progress.Categories.Terminal != repeatedWorkCount || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf(
			"session progress categories = %+v, want %d terminal and zero failed",
			session.Runtime.Progress.Categories,
			repeatedWorkCount,
		)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "complete")); got != repeatedWorkCount {
		t.Fatalf("CountWorkAtCustomerState(task:complete) = %d, want %d; listed=%#v", got, repeatedWorkCount, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "init")); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(task:init) = %d, want 0 after all items complete", got)
	}

	workIDs := terminalTaskWorkIDsAtState(t, listed, "complete")
	if len(workIDs) != repeatedWorkCount {
		t.Fatalf("terminal task work IDs = %v, want %d completed items", workIDs, repeatedWorkCount)
	}
	for _, workID := range workIDs {
		if completed := countCompletedDispatchesForWork(dispatches, workID); completed != 1 {
			t.Fatalf("completed dispatch count for work %q = %d, want 1; dispatches=%#v", workID, completed, dispatches)
		}
		fired := competingWorkstationsThatDispatched(dispatches, workID)
		if len(fired) != 1 {
			t.Fatalf("competing workstations that dispatched work %q = %v, want exactly one", workID, fired)
		}
		if fired[0] != contendingPrimaryWorkstation && fired[0] != contendingAlternateWorkstation {
			t.Fatalf("dispatch workstation = %q, want %q or %q", fired[0], contendingPrimaryWorkstation, contendingAlternateWorkstation)
		}
	}
}

func contendingExecutionFactoryConfig() map[string]any {
	return map[string]any{
		"name": "execution-contention",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{
			{"name": "worker-a"},
			{"name": "worker-b"},
		},
		"workstations": []map[string]any{
			{
				"name":      contendingPrimaryWorkstation,
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
			{
				"name":      contendingAlternateWorkstation,
				"worker":    "worker-b",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}

func terminalTaskWorkIDAtState(t *testing.T, listed factoryapi.ListWorkResponse, state string) string {
	t.Helper()

	ids := terminalTaskWorkIDsAtState(t, listed, state)
	if len(ids) != 1 {
		t.Fatalf("task work at state %q count = %d, want 1; ids=%v listed=%#v", state, len(ids), ids, listed)
	}
	return ids[0]
}

func terminalTaskWorkIDsAtState(t *testing.T, listed factoryapi.ListWorkResponse, state string) []string {
	t.Helper()

	ids := make([]string, 0, len(listed.Results))
	for _, item := range listed.Results {
		if item.WorkTypeName == nil || *item.WorkTypeName != "task" {
			continue
		}
		if item.State == nil || item.State.Name != state {
			continue
		}
		workID := support.StringPointerValue(item.WorkId)
		if workID == "" {
			t.Fatalf("task work at %q has empty work ID: %#v", state, item)
		}
		ids = append(ids, workID)
	}
	return ids
}

func countCompletedDispatchesForWork(
	dispatches []support.DispatchEventObservation,
	workID string,
) int {
	count := 0
	for _, dispatch := range dispatches {
		if dispatch.Response == nil {
			continue
		}
		if !support.DispatchObservationIncludesWork(dispatch, workID) {
			continue
		}
		count++
	}
	return count
}

func competingWorkstationsThatDispatched(
	dispatches []support.DispatchEventObservation,
	workID string,
) []string {
	workstations := make([]string, 0, 2)
	seen := make(map[string]bool)
	for _, dispatch := range dispatches {
		if dispatch.Response == nil {
			continue
		}
		if !support.DispatchObservationIncludesWork(dispatch, workID) {
			continue
		}
		workstation := dispatch.Request.TransitionId
		if workstation == "" || seen[workstation] {
			continue
		}
		seen[workstation] = true
		workstations = append(workstations, workstation)
	}
	return workstations
}

func rewriteStatelessCollectorForDifferentWorkers(t *testing.T, dir string) {
	t.Helper()

	factoryPath := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(factoryPath)
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal factory.json: %v", err)
	}

	cfg["workers"] = []map[string]any{
		{"name": "agent-a"},
		{"name": "agent-b"},
	}

	workstations := cfg["workstations"].([]any)
	workstations[0].(map[string]any)["worker"] = "agent-a"
	workstations[1].(map[string]any)["worker"] = "agent-b"

	updated, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory.json: %v", err)
	}
	if err := os.WriteFile(factoryPath, updated, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}

	source := filepath.Join(dir, "workers", "agent", "AGENTS.md")
	agentConfig, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read worker AGENTS.md: %v", err)
	}

	for _, workerName := range []string{"agent-a", "agent-b"} {
		workerDir := filepath.Join(dir, "workers", workerName)
		if err := os.MkdirAll(workerDir, 0o755); err != nil {
			t.Fatalf("create worker dir %s: %v", workerName, err)
		}
		if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), agentConfig, 0o644); err != nil {
			t.Fatalf("write worker AGENTS.md %s: %v", workerName, err)
		}
	}

	if err := os.RemoveAll(filepath.Join(dir, "workers", "agent")); err != nil {
		t.Fatalf("remove original worker dir: %v", err)
	}
}
