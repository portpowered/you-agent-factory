package runtime_api

import (
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/scheduler"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
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

	_, err := factoryconfig.LoadRuntimeConfig(dir, nil)
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

	_, err := factoryconfig.LoadRuntimeConfig(dir, nil)
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

	_, err := factoryconfig.LoadRuntimeConfig(dir, nil)
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

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkID:     "runtime-config-alignment-work",
		WorkTypeID: "task",
		TraceID:    "runtime-config-alignment-trace",
		Payload:    []byte(`{"title":"runtime config alignment smoke"}`),
	})
	dueAt := time.Now().UTC().Add(-time.Second)
	expiresAt := dueAt.Add(time.Hour)
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkID:     "runtime-config-alignment-cron-work",
		WorkTypeID: "scheduled",
		TraceID:    "runtime-config-alignment-cron-trace",
		Payload:    []byte(`{"title":"runtime config alignment cron smoke"}`),
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
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
	server := startFunctionalServerWithConfig(t, dir, false, func(cfg *service.FactoryServiceConfig) {
		cfg.ProviderCommandRunnerOverride = providerRunner
		cfg.CommandRunnerOverride = scriptRunner
	}, factory.WithScheduler(scheduler.NewWorkInQueueScheduler(1)))

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

	engineState := server.GetEngineStateSnapshot(t)
	if len(engineState.Marking.PlaceTokens["task:complete"]) != 1 {
		t.Fatalf("completed task token count = %d, want 1; places=%#v", len(engineState.Marking.PlaceTokens["task:complete"]), engineState.Marking.PlaceTokens)
	}
	if len(engineState.Marking.PlaceTokens["task:failed"]) != 0 {
		t.Fatalf("failed task token count = %d, want 0; places=%#v", len(engineState.Marking.PlaceTokens["task:failed"]), engineState.Marking.PlaceTokens)
	}
	if len(engineState.Marking.PlaceTokens["scheduled:complete"]) != 1 {
		t.Fatalf("completed scheduled token count = %d, want 1; places=%#v", len(engineState.Marking.PlaceTokens["scheduled:complete"]), engineState.Marking.PlaceTokens)
	}
	if len(engineState.Marking.PlaceTokens["agent-slot:available"]) != 1 {
		t.Fatalf("agent-slot availability after completion = %d, want 1; places=%#v", len(engineState.Marking.PlaceTokens["agent-slot:available"]), engineState.Marking.PlaceTokens)
	}
	if providerRunner.CallCount() != 2 {
		t.Fatalf("provider runner call count = %d, want 2", providerRunner.CallCount())
	}
	if scriptRunner.CallCount() != 2 {
		t.Fatalf("script runner call count = %d, want 2", scriptRunner.CallCount())
	}
	assertRuntimeConfigAlignmentDispatchHistory(t, engineState.DispatchHistory)
	assertRuntimeConfigAlignmentCompleteTokenPayload(t, engineState.Marking.Tokens)
	assertRuntimeConfigAlignmentEventHistory(t, server)
	assertRuntimeConfigAlignmentTopologyProjection(t, dir)
}

type runtimeConfigAlignmentSummary struct {
	Workers      map[string]runtimeConfigAlignmentWorkerSummary
	Workstations map[string]runtimeConfigAlignmentWorkstationSummary
}

type runtimeConfigAlignmentWorkerSummary struct {
	Type      string
	Resources []interfaces.ResourceConfig
	StopToken string
}

type runtimeConfigAlignmentWorkstationSummary struct {
	WorkerTypeName string
	Kind           interfaces.WorkstationKind
	Type           string
	Cron           *runtimeConfigAlignmentCronSummary
	Limits         interfaces.WorkstationLimits
	Resources      []interfaces.ResourceConfig
	StopWords      []string
}

type runtimeConfigAlignmentCronSummary struct {
	Schedule       string
	TriggerAtStart bool
	Jitter         string
	ExpiryWindow   string
}
