package runtime_api

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	runtimeConfigAlignmentSignalTimeout     = 10 * time.Second
	runtimeConfigAlignmentCompletionTimeout = 15 * time.Second
	runtimeConfigAlignmentPollInterval      = 50 * time.Millisecond
	runtimeConfigAlignmentExecuteTimeout    = 100 * time.Millisecond
	runtimeConfigAlignmentTimeoutMinElapsed = 50 * time.Millisecond
	runtimeConfigAlignmentTimeoutMaxElapsed = 1500 * time.Millisecond

	runtimeConfigAlignmentCronWorkstation    = "aaa-cron-task"
	runtimeConfigAlignmentExecuteWorkstation = "yyy-execute-task"
	runtimeConfigAlignmentReviewWorkstation  = "zzz-review-task"

	runtimeConfigAlignmentGeneratedBoundaryContext = "decode factory generated-schema boundary"
)

func TestRuntimeConfigAlignmentSmoke_CanonicalOnlyBoundaryStaysAlignedAcrossExecutionAndRejectsRetiredAliases(t *testing.T) {
	t.Parallel()
	support.SkipLongFunctional(t, "slow runtime config boundary alignment sweep")

	for _, tc := range []struct {
		name string
		run  func(*testing.T)
	}{
		{
			name: "canonical split factory stays aligned across flatten replay and execution",
			run:  testRuntimeConfigAlignmentCanonicalRoundTripAndExecution,
		},
		{
			name: "generated factory json rejects retired worker provider alias",
			run:  testRuntimeConfigAlignmentRejectsGeneratedWorkerProviderAlias,
		},
		{
			name: "generated factory json rejects retired workstation resource_usage alias",
			run:  testRuntimeConfigAlignmentRejectsGeneratedWorkstationResourceUsageAlias,
		},
		{
			name: "split worker frontmatter rejects retired model_provider alias",
			run:  testRuntimeConfigAlignmentRejectsSplitWorkerModelProviderAlias,
		},
		{
			name: "split workstation frontmatter rejects retired runtime_type alias",
			run:  testRuntimeConfigAlignmentRejectsSplitWorkstationRuntimeTypeAlias,
		},
		{
			name: "split workstation frontmatter rejects retired cron trigger_at_start alias",
			run:  testRuntimeConfigAlignmentRejectsSplitWorkstationCronTriggerAtStartAlias,
		},
	} {
		tc := tc
		t.Run(tc.name, tc.run)
	}
}

func testRuntimeConfigAlignmentCanonicalRoundTripAndExecution(t *testing.T) {
	// C06-ISOLATED CASE-40: the canonical flatten/readback witness is currently
	// coupled to the same process execution body; retain the combined case
	// until that process-owned boundary can be split without weakening parity.
	dir := setupRuntimeConfigAlignmentFactory(t)
	assertRuntimeConfigAlignmentCanonicalRoundTrip(t, dir)
	server, providerRunner, scriptRunner := startRuntimeConfigAlignmentSmokeServer(t, dir)

	waitForRuntimeConfigAlignmentExecution(t, server, providerRunner, scriptRunner)
	assertRuntimeConfigAlignmentFinalState(t, dir, server, providerRunner, scriptRunner)
}

func testRuntimeConfigAlignmentRejectsGeneratedWorkerProviderAlias(t *testing.T) {
	assertRuntimeConfigAlignmentRejectsGeneratedFactoryAlias(t, func(cfg map[string]any) {
		cfg["workers"].([]map[string]any)[0]["provider"] = "openai"
	}, "workers[0].provider is not supported; use executorProvider")
}

func testRuntimeConfigAlignmentRejectsGeneratedWorkstationResourceUsageAlias(t *testing.T) {
	assertRuntimeConfigAlignmentRejectsGeneratedFactoryAlias(t, func(cfg map[string]any) {
		workstation := cfg["workstations"].([]map[string]any)[0]
		workstation["resource_usage"] = workstation["resources"]
		delete(workstation, "resources")
	}, "workstations[0].resource_usage is not supported; use resources")
}

func assertRuntimeConfigAlignmentRejectsGeneratedFactoryAlias(t *testing.T, mutate func(map[string]any), want string) {
	t.Helper()

	cfg := runtimeConfigAlignmentFactoryJSONConfig()
	mutate(cfg)

	dir := support.ScaffoldFactory(t, cfg)
	writeRuntimeConfigAlignmentAgentConfigs(t, dir)

	_, err := support.LoadedFactory(t, dir)
	assertRuntimeConfigAlignmentBoundaryErrorContains(t, err,
		runtimeConfigAlignmentGeneratedBoundaryContext,
		want,
	)
}

func testRuntimeConfigAlignmentRejectsSplitWorkerModelProviderAlias(t *testing.T) {
	dir := setupRuntimeConfigAlignmentFactory(t)
	support.WriteAgentConfig(t, dir, "reviewer", `---
type: MODEL_WORKER
model: claude-sonnet-4-20250514
model_provider: claude
resources:
  - name: agent-slot
    capacity: 1
stopToken: COMPLETE
---
You are the review worker.
`)

	_, err := support.LoadedFactory(t, dir)
	assertRuntimeConfigAlignmentBoundaryErrorContains(t, err,
		`load worker "reviewer" config`,
		"frontmatter.model_provider is not supported; use modelProvider",
	)
}

func testRuntimeConfigAlignmentRejectsSplitWorkstationRuntimeTypeAlias(t *testing.T) {
	assertRuntimeConfigAlignmentRejectsSplitWorkstationAlias(t, runtimeConfigAlignmentReviewWorkstation, `---
behavior: REPEATER
runtime_type: MODEL_WORKSTATION
worker: reviewer
stopWords:
  - DONE
---
Review the task and return DONE when it is acceptable.
`, "frontmatter.runtime_type is not supported; use type")
}

func testRuntimeConfigAlignmentRejectsSplitWorkstationCronTriggerAtStartAlias(t *testing.T) {
	assertRuntimeConfigAlignmentRejectsSplitWorkstationAlias(t, runtimeConfigAlignmentCronWorkstation, `---
behavior: CRON
type: MODEL_WORKSTATION
worker: cron-worker
cron:
  schedule: "0 * * * *"
  trigger_at_start: true
  expiryWindow: 1h
---
Complete the scheduled task and return COMPLETE.
`, "frontmatter.cron.trigger_at_start is not supported; use triggerAtStart")
}

func assertRuntimeConfigAlignmentRejectsSplitWorkstationAlias(t *testing.T, workstationName string, frontmatter string, want string) {
	t.Helper()

	dir := setupRuntimeConfigAlignmentFactory(t)
	writeWorkstationConfig(t, dir, workstationName, frontmatter)

	_, err := support.LoadedFactory(t, dir)
	assertRuntimeConfigAlignmentBoundaryErrorContains(t, err,
		`load workstation "`+workstationName+`" config`,
		want,
	)
}

func startRuntimeConfigAlignmentSmokeServer(
	t *testing.T,
	dir string,
) (*functionalAPIServer, *runtimeConfigAlignmentProviderRunner, *runtimeConfigAlignmentScriptRunner) {
	t.Helper()

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     "runtime-config-alignment-work",
		WorkTypeID: "task",
		TraceID:    "runtime-config-alignment-trace",
		Payload:    []byte(`{"title":"runtime config alignment smoke"}`),
	})
	dueAt := time.Now().UTC().Add(-time.Second)
	expiresAt := dueAt.Add(time.Hour)
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     "runtime-config-alignment-cron-work",
		WorkTypeID: "scheduled",
		TraceID:    "runtime-config-alignment-cron-trace",
		Payload:    []byte(`{"title":"runtime config alignment cron smoke"}`),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:      "runtime-config-alignment-cron-time",
		Name:        "cron:" + runtimeConfigAlignmentCronWorkstation,
		WorkTypeID:  interfaces.SystemTimeWorkTypeID,
		TargetState: interfaces.SystemTimePendingState,
		TraceID:     "runtime-config-alignment-cron-time",
		Payload:     []byte(`{"source":"cron"}`),
		Tags: map[string]string{
			interfaces.TimeWorkTagKeySource:          interfaces.TimeWorkSourceCron,
			interfaces.TimeWorkTagKeyCronWorkstation: runtimeConfigAlignmentCronWorkstation,
			interfaces.TimeWorkTagKeyNominalAt:       dueAt.Format(time.RFC3339Nano),
			interfaces.TimeWorkTagKeyDueAt:           dueAt.Format(time.RFC3339Nano),
			interfaces.TimeWorkTagKeyExpiresAt:       expiresAt.Format(time.RFC3339Nano),
			interfaces.TimeWorkTagKeyJitter:          "0s",
		},
	})
	providerRunner := newRuntimeConfigAlignmentProviderRunner()
	scriptRunner := newRuntimeConfigAlignmentScriptRunner()
	server := startSharedFunctionalServer(t, dir, runtimeAPIScenario{
		providerRunner: providerRunner,
		scriptRunner:   scriptRunner,
		models: []string{
			"claude-sonnet-4-20250514",
			"gpt-5.4",
		},
	})

	return server, providerRunner, scriptRunner
}

func waitForRuntimeConfigAlignmentExecution(
	t *testing.T,
	server *functionalAPIServer,
	providerRunner *runtimeConfigAlignmentProviderRunner,
	scriptRunner *runtimeConfigAlignmentScriptRunner,
) {
	t.Helper()

	waitForRuntimeConfigAlignmentStopWordDispatch(t, server)
	waitForRuntimeConfigAlignmentInFlightResourceConsumption(t, server, scriptRunner)
	waitForRuntimeConfigAlignmentTimeoutAndRequeue(t, server, scriptRunner)

	close(scriptRunner.releaseSecondAttempt)
	waitForRuntimeConfigAlignmentServerCompletion(t, server, runtimeConfigAlignmentCompletionTimeout)
}

func assertRuntimeConfigAlignmentFinalState(
	t *testing.T,
	dir string,
	server *functionalAPIServer,
	providerRunner *runtimeConfigAlignmentProviderRunner,
	scriptRunner *runtimeConfigAlignmentScriptRunner,
) {
	t.Helper()

	session := server.Session(t)
	listed := server.ListWork(t)
	if got := support.CountWorkAtCustomerState(listed, "task:complete"); got != 1 {
		t.Fatalf("completed task token count = %d, want 1; work=%#v", got, listed.Results)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed task token count = %d, want 0; work=%#v", got, listed.Results)
	}
	if got := support.CountWorkAtCustomerState(listed, "scheduled:complete"); got != 1 {
		t.Fatalf("completed scheduled token count = %d, want 1; work=%#v", got, listed.Results)
	}
	if available, total, ok := runtimeConfigAlignmentResourceUsage(session, "agent-slot"); !ok || available != 1 || total != 1 {
		t.Fatalf("agent-slot usage after completion = available:%d total:%d found:%t, want 1/1", available, total, ok)
	}
	if providerRunner.CallCount() != 2 {
		t.Fatalf("provider runner call count = %d, want 2", providerRunner.CallCount())
	}
	if scriptRunner.CallCount() != 2 {
		t.Fatalf("script runner call count = %d, want 2", scriptRunner.CallCount())
	}
	assertRuntimeConfigAlignmentDispatchHistory(
		t,
		support.ObserveDispatchEvents(t, server.GetFactoryEvents(t)),
	)
	assertRuntimeConfigAlignmentCompleteWorkPayload(t, server.ListWork(t))
	assertRuntimeConfigAlignmentEventHistory(t, server)
	assertRuntimeConfigAlignmentTopologyProjection(t, server)
}
