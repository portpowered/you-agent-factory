package functional_test

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/factory/scheduler"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
)

const (
	runtimeConfigAlignmentSignalTimeout     = 10 * time.Second
	runtimeConfigAlignmentCompletionTimeout = 15 * time.Second
	runtimeConfigAlignmentPollInterval      = 50 * time.Millisecond

	runtimeConfigAlignmentCronWorkstation    = "aaa-cron-task"
	runtimeConfigAlignmentExecuteWorkstation = "yyy-execute-task"
	runtimeConfigAlignmentReviewWorkstation  = "zzz-review-task"

	runtimeConfigAlignmentGeneratedBoundaryContext = "decode factory generated-schema boundary"
)

func TestRuntimeConfigAlignmentSmoke_CanonicalOnlyBoundaryStaysAlignedAcrossExecutionAndRejectsRetiredAliases(t *testing.T) {
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

	dir := scaffoldFactory(t, cfg)
	writeRuntimeConfigAlignmentAgentConfigs(t, dir)

	_, err := factoryconfig.LoadRuntimeConfig(dir, nil)
	assertRuntimeConfigAlignmentBoundaryErrorContains(t, err,
		runtimeConfigAlignmentGeneratedBoundaryContext,
		want,
	)
}

func testRuntimeConfigAlignmentRejectsSplitWorkerModelProviderAlias(t *testing.T) {
	dir := setupRuntimeConfigAlignmentFactory(t)
	writeAgentConfig(t, dir, "reviewer", `---
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

func setupRuntimeConfigAlignmentFactory(t *testing.T) string {
	t.Helper()

	dir := scaffoldFactory(t, runtimeConfigAlignmentFactoryJSONConfig())
	writeRuntimeConfigAlignmentAgentConfigs(t, dir)
	return dir
}

func runtimeConfigAlignmentFactoryJSONConfig() map[string]any {
	return map[string]any{
		"name":            "factory",
		"workTypes":       runtimeConfigAlignmentWorkTypes(),
		"resources":       runtimeConfigAlignmentResources(),
		"supportingFiles": runtimeConfigAlignmentResourceManifest(),
		"workers":         runtimeConfigAlignmentWorkers(),
		"workstations":    runtimeConfigAlignmentWorkstations(),
	}
}

func runtimeConfigAlignmentWorkTypes() []map[string]any {
	return []map[string]any{
		{
			"name": "scheduled",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		},
		{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "reviewed", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		},
	}
}

func runtimeConfigAlignmentResources() []map[string]any {
	return []map[string]any{{
		"name":     "agent-slot",
		"capacity": 1,
	}}
}

func runtimeConfigAlignmentResourceManifest() map[string]any {
	return map[string]any{
		"requiredTools": []map[string]any{{
			"name":        "go",
			"command":     "go",
			"purpose":     "Runs portable validation helpers",
			"versionArgs": []string{"version"},
		}},
		"bundledFiles": []map[string]any{{
			"type":       "SCRIPT",
			"targetPath": "factory/scripts/bootstrap.ps1",
			"content": map[string]any{
				"encoding": "utf-8",
				"inline":   "Write-Output 'bootstrap'\n",
			},
		}, {
			"type":       "DOC",
			"targetPath": "factory/docs/usage.md",
			"content": map[string]any{
				"encoding": "utf-8",
				"inline":   "# Runtime config alignment\n",
			},
		}},
	}
}

func runtimeConfigAlignmentWorkers() []map[string]any {
	return []map[string]any{
		{"name": "cron-worker"},
		{"name": "reviewer"},
		{"name": "executor"},
	}
}

func runtimeConfigAlignmentWorkstations() []map[string]any {
	return []map[string]any{
		runtimeConfigAlignmentReviewWorkstationConfig(),
		runtimeConfigAlignmentExecuteWorkstationConfig(),
		runtimeConfigAlignmentCronWorkstationConfig(),
	}
}

func runtimeConfigAlignmentReviewWorkstationConfig() map[string]any {
	return map[string]any{
		"name":    runtimeConfigAlignmentReviewWorkstation,
		"worker":  "reviewer",
		"inputs":  []map[string]string{{"workType": "task", "state": "init"}},
		"outputs": []map[string]string{{"workType": "task", "state": "reviewed"}},
		"resources": []map[string]any{{
			"name":     "agent-slot",
			"capacity": 1,
		}},
	}
}

func runtimeConfigAlignmentExecuteWorkstationConfig() map[string]any {
	return map[string]any{
		"name":      runtimeConfigAlignmentExecuteWorkstation,
		"worker":    "executor",
		"inputs":    []map[string]string{{"workType": "task", "state": "reviewed"}},
		"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
		"onFailure": map[string]string{"workType": "task", "state": "failed"},
		"resources": []map[string]any{{
			"name":     "agent-slot",
			"capacity": 1,
		}},
	}
}

func runtimeConfigAlignmentCronWorkstationConfig() map[string]any {
	return map[string]any{
		"name":      runtimeConfigAlignmentCronWorkstation,
		"worker":    "cron-worker",
		"inputs":    []map[string]string{{"workType": "scheduled", "state": "init"}},
		"outputs":   []map[string]string{{"workType": "scheduled", "state": "complete"}},
		"onFailure": map[string]string{"workType": "scheduled", "state": "failed"},
	}
}

func writeRuntimeConfigAlignmentAgentConfigs(t *testing.T, dir string) {
	t.Helper()

	writeAgentConfig(t, dir, "reviewer", `---
type: MODEL_WORKER
model: claude-sonnet-4-20250514
modelProvider: claude
resources:
  - name: agent-slot
    capacity: 1
stopToken: COMPLETE
---
You are the review worker.
`)
	writeAgentConfig(t, dir, "executor", `---
type: SCRIPT_WORKER
command: echo
resources:
  - name: agent-slot
    capacity: 1
---
You are the execution worker.
`)
	writeAgentConfig(t, dir, "cron-worker", `---
type: MODEL_WORKER
model: gpt-5.4
modelProvider: openai
stopToken: COMPLETE
---
You are the cron worker.
`)
	writeWorkstationConfig(t, dir, runtimeConfigAlignmentReviewWorkstation, `---
behavior: REPEATER
type: MODEL_WORKSTATION
worker: reviewer
stopWords:
  - DONE
---
Review the task and return DONE when it is acceptable.
`)
	writeWorkstationConfig(t, dir, runtimeConfigAlignmentExecuteWorkstation, `---
type: MODEL_WORKSTATION
worker: executor
limits:
  maxExecutionTime: 100ms
  maxRetries: 2
---
Execute the reviewed task.
`)
	writeWorkstationConfig(t, dir, runtimeConfigAlignmentCronWorkstation, `---
behavior: CRON
type: MODEL_WORKSTATION
worker: cron-worker
cron:
  schedule: "0 * * * *"
  expiryWindow: 1h
---
Complete the scheduled task and return COMPLETE.
`)
}

func assertRuntimeConfigAlignmentCanonicalRoundTrip(t *testing.T, dir string) {
	t.Helper()

	loaded, err := factoryconfig.LoadRuntimeConfig(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	assertRuntimeConfigAlignmentResourceManifest(t, loaded.FactoryConfig().ResourceManifest)
	wantSummary := runtimeConfigAlignmentSummaryFromRuntime(t, loaded, loaded)

	flattened, err := factoryconfig.FlattenFactoryConfig(dir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig: %v", err)
	}
	assertRuntimeConfigAlignmentCanonicalJSON(t, flattened)
	flattenedFactory, err := factoryconfig.GeneratedFactoryFromOpenAPIJSON(flattened)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON(flattened): %v", err)
	}
	assertRuntimeConfigAlignmentGeneratedBoundary(t, flattenedFactory)

	generatedFactory, err := replay.GeneratedFactoryFromLoadedConfig(
		loaded,
		replay.WithGeneratedFactorySourceDirectory(loaded.FactoryDir()),
	)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromLoadedConfig: %v", err)
	}
	assertRuntimeConfigAlignmentGeneratedBoundary(t, generatedFactory)
	if !reflect.DeepEqual(
		runtimeConfigAlignmentComparableFactory(flattenedFactory),
		runtimeConfigAlignmentComparableFactory(generatedFactory),
	) {
		t.Fatalf(
			"flattened canonical factory and generated replay factory diverged\nflattened: %#v\ngenerated: %#v",
			runtimeConfigAlignmentComparableFactory(flattenedFactory),
			runtimeConfigAlignmentComparableFactory(generatedFactory),
		)
	}
	assertRuntimeConfigAlignmentGeneratedResourceManifest(t, flattenedFactory.SupportingFiles)
	assertRuntimeConfigAlignmentGeneratedResourceManifest(t, generatedFactory.SupportingFiles)

	replayRuntime, err := replay.RuntimeConfigFromGeneratedFactory(generatedFactory)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}
	if replayRuntime.FactoryDir() != loaded.FactoryDir() {
		t.Fatalf("replay runtime FactoryDir = %q, want %q", replayRuntime.FactoryDir(), loaded.FactoryDir())
	}
	assertRuntimeConfigAlignmentResourceManifest(t, replayRuntime.Factory.ResourceManifest)
	gotSummary := runtimeConfigAlignmentSummaryFromRuntime(t, replayRuntime, replayRuntime)
	if !reflect.DeepEqual(gotSummary, wantSummary) {
		t.Fatalf("replay runtime config summary mismatch\ngot:  %#v\nwant: %#v", gotSummary, wantSummary)
	}
}

func startRuntimeConfigAlignmentSmokeServer(
	t *testing.T,
	dir string,
) (*FunctionalServer, *runtimeConfigAlignmentProviderRunner, *runtimeConfigAlignmentScriptRunner) {
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
	server := StartFunctionalServerWithConfig(t, dir, false, func(cfg *service.FactoryServiceConfig) {
		cfg.ProviderCommandRunnerOverride = providerRunner
		cfg.CommandRunnerOverride = scriptRunner
	}, factory.WithScheduler(scheduler.NewWorkInQueueScheduler(1)))

	return server, providerRunner, scriptRunner
}

func waitForRuntimeConfigAlignmentExecution(
	t *testing.T,
	server *FunctionalServer,
	providerRunner *runtimeConfigAlignmentProviderRunner,
	scriptRunner *runtimeConfigAlignmentScriptRunner,
) {
	t.Helper()

	waitForRuntimeConfigAlignmentStopWordDispatch(t, server)
	waitForRuntimeConfigAlignmentInFlightResourceConsumption(t, server, scriptRunner)
	waitForRuntimeConfigAlignmentTimeoutAndRequeue(t, server, scriptRunner)
	server.WaitForCompleted(t, runtimeConfigAlignmentCompletionTimeout)
}

func assertRuntimeConfigAlignmentFinalState(
	t *testing.T,
	dir string,
	server *FunctionalServer,
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

func runtimeConfigAlignmentSummaryFromRuntime(
	t *testing.T,
	definitionLookup interfaces.RuntimeDefinitionLookup,
	workstationLookup interfaces.RuntimeWorkstationLookup,
) runtimeConfigAlignmentSummary {
	t.Helper()

	return runtimeConfigAlignmentSummary{
		Workers: map[string]runtimeConfigAlignmentWorkerSummary{
			"cron-worker": runtimeConfigAlignmentWorkerSummaryFromLookup(t, definitionLookup.Worker, "cron-worker"),
			"reviewer":    runtimeConfigAlignmentWorkerSummaryFromLookup(t, definitionLookup.Worker, "reviewer"),
			"executor":    runtimeConfigAlignmentWorkerSummaryFromLookup(t, definitionLookup.Worker, "executor"),
		},
		Workstations: map[string]runtimeConfigAlignmentWorkstationSummary{
			runtimeConfigAlignmentCronWorkstation:    runtimeConfigAlignmentWorkstationSummaryFromLookup(t, workstationLookup.Workstation, runtimeConfigAlignmentCronWorkstation),
			runtimeConfigAlignmentReviewWorkstation:  runtimeConfigAlignmentWorkstationSummaryFromLookup(t, workstationLookup.Workstation, runtimeConfigAlignmentReviewWorkstation),
			runtimeConfigAlignmentExecuteWorkstation: runtimeConfigAlignmentWorkstationSummaryFromLookup(t, workstationLookup.Workstation, runtimeConfigAlignmentExecuteWorkstation),
		},
	}
}

func runtimeConfigAlignmentWorkerSummaryFromLookup(
	t *testing.T,
	lookup func(string) (*interfaces.WorkerConfig, bool),
	name string,
) runtimeConfigAlignmentWorkerSummary {
	t.Helper()

	worker, ok := lookup(name)
	if !ok {
		t.Fatalf("expected worker %q", name)
	}

	return runtimeConfigAlignmentWorkerSummary{
		Type:      worker.Type,
		Resources: append([]interfaces.ResourceConfig(nil), worker.Resources...),
		StopToken: worker.StopToken,
	}
}

func runtimeConfigAlignmentWorkstationSummaryFromLookup(
	t *testing.T,
	lookup func(string) (*interfaces.FactoryWorkstationConfig, bool),
	name string,
) runtimeConfigAlignmentWorkstationSummary {
	t.Helper()

	workstation, ok := lookup(name)
	if !ok {
		t.Fatalf("expected workstation %q", name)
	}

	return runtimeConfigAlignmentWorkstationSummary{
		WorkerTypeName: workstation.WorkerTypeName,
		Kind:           workstation.Kind,
		Type:           workstation.Type,
		Cron:           runtimeConfigAlignmentCronSummaryFromConfig(workstation.Cron),
		Limits:         workstation.Limits,
		Resources:      append([]interfaces.ResourceConfig(nil), workstation.Resources...),
		StopWords:      append([]string(nil), workstation.StopWords...),
	}
}

func runtimeConfigAlignmentCronSummaryFromConfig(cron *interfaces.CronConfig) *runtimeConfigAlignmentCronSummary {
	if cron == nil {
		return nil
	}
	return &runtimeConfigAlignmentCronSummary{
		Schedule:       cron.Schedule,
		TriggerAtStart: cron.TriggerAtStart,
		Jitter:         cron.Jitter,
		ExpiryWindow:   cron.ExpiryWindow,
	}
}

func assertRuntimeConfigAlignmentBoundaryErrorContains(t *testing.T, err error, want ...string) {
	t.Helper()

	if err == nil {
		t.Fatal("expected authored boundary to reject retired alias")
	}
	got := err.Error()
	for _, fragment := range want {
		if !strings.Contains(got, fragment) {
			t.Fatalf("boundary error = %q, want fragment %q", got, fragment)
		}
	}
}

func assertRuntimeConfigAlignmentCanonicalJSON(t *testing.T, flattened []byte) {
	t.Helper()

	text := string(flattened)
	for _, want := range []string{
		`"behavior": "CRON"`,
		`"behavior": "REPEATER"`,
		`"stopWords":`,
		`"maxExecutionTime": "100ms"`,
		`"schedule": "0 * * * *"`,
		`"resources":`,
		`"stopToken": "COMPLETE"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("flattened config missing canonical fragment %s: %s", want, text)
		}
	}
	for _, forbidden := range []string{
		`"stop_words"`,
		`"max_execution_time"`,
		`"resource_usage"`,
		`"timeout": "100ms"`,
		`"resource_manifest"`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("flattened config contains legacy fragment %s: %s", forbidden, text)
		}
	}
}

func assertRuntimeConfigAlignmentResourceManifest(
	t *testing.T,
	manifest *interfaces.PortableResourceManifestConfig,
) {
	t.Helper()

	if manifest == nil {
		t.Fatal("expected resourceManifest to be preserved")
	}
	if len(manifest.RequiredTools) != 1 {
		t.Fatalf("required tools = %#v, want one entry", manifest.RequiredTools)
	}
	requiredTool := manifest.RequiredTools[0]
	if requiredTool.Name != "go" || requiredTool.Command != "go" || requiredTool.Purpose != "Runs portable validation helpers" {
		t.Fatalf("required tool = %#v", requiredTool)
	}
	if !reflect.DeepEqual(requiredTool.VersionArgs, []string{"version"}) {
		t.Fatalf("required tool version args = %#v, want [version]", requiredTool.VersionArgs)
	}

	if len(manifest.BundledFiles) != 2 {
		t.Fatalf("bundled files = %#v, want two entries", manifest.BundledFiles)
	}
	if bundled := runtimeConfigAlignmentBundledFileByTarget(t, manifest.BundledFiles, "factory/docs/usage.md"); bundled.Type != interfaces.BundledFileTypeDoc ||
		bundled.Content.Encoding != interfaces.BundledFileEncodingUTF8 ||
		bundled.Content.Inline != "# Runtime config alignment\n" {
		t.Fatalf("doc bundled file = %#v", bundled)
	}
	if bundled := runtimeConfigAlignmentBundledFileByTarget(t, manifest.BundledFiles, "factory/scripts/bootstrap.ps1"); bundled.Type != interfaces.BundledFileTypeScript ||
		bundled.Content.Encoding != interfaces.BundledFileEncodingUTF8 ||
		bundled.Content.Inline != "Write-Output 'bootstrap'\n" {
		t.Fatalf("script bundled file = %#v", bundled)
	}
}

func assertRuntimeConfigAlignmentGeneratedResourceManifest(
	t *testing.T,
	manifest *factoryapi.ResourceManifest,
) {
	t.Helper()

	if manifest == nil {
		t.Fatal("expected generated resourceManifest to be preserved")
	}
	if manifest.RequiredTools == nil || len(*manifest.RequiredTools) != 1 {
		t.Fatalf("generated requiredTools = %#v, want one entry", manifest.RequiredTools)
	}
	requiredTool := (*manifest.RequiredTools)[0]
	if requiredTool.Name != "go" || requiredTool.Command != "go" || stringPointerValue(requiredTool.Purpose) != "Runs portable validation helpers" {
		t.Fatalf("generated required tool = %#v", requiredTool)
	}
	if requiredTool.VersionArgs == nil || !reflect.DeepEqual(*requiredTool.VersionArgs, []string{"version"}) {
		t.Fatalf("generated required tool version args = %#v, want [version]", requiredTool.VersionArgs)
	}

	if manifest.BundledFiles == nil || len(*manifest.BundledFiles) != 2 {
		t.Fatalf("generated bundledFiles = %#v, want two entries", manifest.BundledFiles)
	}
	if bundled := runtimeConfigAlignmentGeneratedBundledFileByTarget(t, *manifest.BundledFiles, "factory/docs/usage.md"); string(bundled.Type) != interfaces.BundledFileTypeDoc ||
		bundled.Content.Encoding != interfaces.BundledFileEncodingUTF8 ||
		bundled.Content.Inline != "# Runtime config alignment\n" {
		t.Fatalf("generated doc bundled file = %#v", bundled)
	}
	if bundled := runtimeConfigAlignmentGeneratedBundledFileByTarget(t, *manifest.BundledFiles, "factory/scripts/bootstrap.ps1"); string(bundled.Type) != interfaces.BundledFileTypeScript ||
		bundled.Content.Encoding != interfaces.BundledFileEncodingUTF8 ||
		bundled.Content.Inline != "Write-Output 'bootstrap'\n" {
		t.Fatalf("generated script bundled file = %#v", bundled)
	}
}

func runtimeConfigAlignmentBundledFileByTarget(t *testing.T, bundledFiles []interfaces.BundledFileConfig, targetPath string) interfaces.BundledFileConfig {
	t.Helper()

	for _, bundledFile := range bundledFiles {
		if bundledFile.TargetPath == targetPath {
			return bundledFile
		}
	}
	t.Fatalf("expected bundled file %q in %#v", targetPath, bundledFiles)
	return interfaces.BundledFileConfig{}
}

func runtimeConfigAlignmentGeneratedBundledFileByTarget(t *testing.T, bundledFiles []factoryapi.BundledFile, targetPath string) factoryapi.BundledFile {
	t.Helper()

	for _, bundledFile := range bundledFiles {
		if bundledFile.TargetPath == targetPath {
			return bundledFile
		}
	}
	t.Fatalf("expected generated bundled file %q in %#v", targetPath, bundledFiles)
	return factoryapi.BundledFile{}
}

func assertRuntimeConfigAlignmentGeneratedBoundary(t *testing.T, generated factoryapi.Factory) {
	t.Helper()

	if generated.Workers == nil || len(*generated.Workers) != 3 {
		t.Fatalf("generated workers = %#v, want three workers", generated.Workers)
	}
	cronWorker := runtimeConfigAlignmentRequireGeneratedWorker(t, *generated.Workers, "cron-worker")
	if stringPointerValue(cronWorker.Type) != interfaces.WorkerTypeModel {
		t.Fatalf("cron-worker type = %q, want %q", stringPointerValue(cronWorker.Type), interfaces.WorkerTypeModel)
	}
	if stringPointerValue(cronWorker.StopToken) != "COMPLETE" {
		t.Fatalf("cron-worker stop token = %q, want COMPLETE", stringPointerValue(cronWorker.StopToken))
	}
	reviewer := runtimeConfigAlignmentRequireGeneratedWorker(t, *generated.Workers, "reviewer")
	if stringPointerValue(reviewer.Type) != interfaces.WorkerTypeModel {
		t.Fatalf("reviewer type = %q, want %q", stringPointerValue(reviewer.Type), interfaces.WorkerTypeModel)
	}
	if stringPointerValue(reviewer.StopToken) != "COMPLETE" {
		t.Fatalf("reviewer stop token = %q, want COMPLETE", stringPointerValue(reviewer.StopToken))
	}
	if !runtimeConfigAlignmentHasGeneratedResource(reviewer.Resources, "agent-slot", 1) {
		t.Fatalf("reviewer resources = %#v, want agent-slot capacity 1", reviewer.Resources)
	}
	executor := runtimeConfigAlignmentRequireGeneratedWorker(t, *generated.Workers, "executor")
	if stringPointerValue(executor.Type) != interfaces.WorkerTypeScript {
		t.Fatalf("executor type = %q, want %q", stringPointerValue(executor.Type), interfaces.WorkerTypeScript)
	}
	if !runtimeConfigAlignmentHasGeneratedResource(executor.Resources, "agent-slot", 1) {
		t.Fatalf("executor resources = %#v, want agent-slot capacity 1", executor.Resources)
	}

	if generated.Workstations == nil || len(*generated.Workstations) != 3 {
		t.Fatalf("generated workstations = %#v, want three workstations", generated.Workstations)
	}
	cron := runtimeConfigAlignmentRequireGeneratedWorkstation(t, *generated.Workstations, runtimeConfigAlignmentCronWorkstation)
	if cron.Worker != "cron-worker" {
		t.Fatalf("%s worker = %q, want cron-worker", runtimeConfigAlignmentCronWorkstation, cron.Worker)
	}
	if cron.Behavior == nil || *cron.Behavior != interfaces.GeneratedPublicWorkstationKind(interfaces.WorkstationKindCron) {
		t.Fatalf("%s behavior = %#v, want CRON", runtimeConfigAlignmentCronWorkstation, cron.Behavior)
	}
	if cron.Cron == nil || cron.Cron.Schedule != "0 * * * *" {
		t.Fatalf("%s cron = %#v, want schedule 0 * * * *", runtimeConfigAlignmentCronWorkstation, cron.Cron)
	}
	review := runtimeConfigAlignmentRequireGeneratedWorkstation(t, *generated.Workstations, runtimeConfigAlignmentReviewWorkstation)
	if review.Worker != "reviewer" {
		t.Fatalf("%s worker = %q, want reviewer", runtimeConfigAlignmentReviewWorkstation, review.Worker)
	}
	if stringPointerValue(review.Type) != interfaces.WorkstationTypeModel {
		t.Fatalf("%s type = %q, want %q", runtimeConfigAlignmentReviewWorkstation, stringPointerValue(review.Type), interfaces.WorkstationTypeModel)
	}
	if review.Behavior == nil || *review.Behavior != interfaces.GeneratedPublicWorkstationKind(interfaces.WorkstationKindRepeater) {
		t.Fatalf("%s behavior = %#v, want REPEATER", runtimeConfigAlignmentReviewWorkstation, review.Behavior)
	}
	if !reflect.DeepEqual(stringSliceValue(review.StopWords), []string{"DONE"}) {
		t.Fatalf("%s stopWords = %#v, want [DONE]", runtimeConfigAlignmentReviewWorkstation, review.StopWords)
	}
	if !runtimeConfigAlignmentHasGeneratedResource(review.Resources, "agent-slot", 1) {
		t.Fatalf("%s resources = %#v, want agent-slot capacity 1", runtimeConfigAlignmentReviewWorkstation, review.Resources)
	}
	execute := runtimeConfigAlignmentRequireGeneratedWorkstation(t, *generated.Workstations, runtimeConfigAlignmentExecuteWorkstation)
	if execute.Worker != "executor" {
		t.Fatalf("%s worker = %q, want executor", runtimeConfigAlignmentExecuteWorkstation, execute.Worker)
	}
	if execute.Limits == nil || stringPointerValue(execute.Limits.MaxExecutionTime) != "100ms" {
		t.Fatalf("%s limits = %#v, want maxExecutionTime 100ms", runtimeConfigAlignmentExecuteWorkstation, execute.Limits)
	}
	if !runtimeConfigAlignmentHasGeneratedResource(execute.Resources, "agent-slot", 1) {
		t.Fatalf("%s resources = %#v, want agent-slot capacity 1", runtimeConfigAlignmentExecuteWorkstation, execute.Resources)
	}
}

func runtimeConfigAlignmentRequireGeneratedWorker(
	t *testing.T,
	workers []factoryapi.Worker,
	name string,
) factoryapi.Worker {
	t.Helper()

	for _, worker := range workers {
		if worker.Name == name {
			return worker
		}
	}
	t.Fatalf("generated workers missing %q: %#v", name, workers)
	return factoryapi.Worker{}
}

func runtimeConfigAlignmentRequireGeneratedWorkstation(
	t *testing.T,
	workstations []factoryapi.Workstation,
	name string,
) factoryapi.Workstation {
	t.Helper()

	for _, workstation := range workstations {
		if workstation.Name == name {
			return workstation
		}
	}
	t.Fatalf("generated workstations missing %q: %#v", name, workstations)
	return factoryapi.Workstation{}
}

func runtimeConfigAlignmentComparableFactory(factory factoryapi.Factory) factoryapi.Factory {
	comparable := factory
	comparable.FactoryDirectory = nil
	comparable.SourceDirectory = nil
	comparable.Metadata = nil
	return comparable
}

func runtimeConfigAlignmentHasGeneratedResource(resources *[]factoryapi.ResourceRequirement, name string, capacity int) bool {
	if resources == nil {
		return false
	}
	for _, resource := range *resources {
		if resource.Name == name && resource.Capacity == capacity {
			return true
		}
	}
	return false
}

func stringSliceValue(values *[]string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), (*values)...)
}

type runtimeConfigAlignmentProviderRunner struct {
	mu        sync.Mutex
	callCount int
}

func newRuntimeConfigAlignmentProviderRunner() *runtimeConfigAlignmentProviderRunner {
	return &runtimeConfigAlignmentProviderRunner{}
}

func (r *runtimeConfigAlignmentProviderRunner) Run(_ context.Context, request workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	r.callCount++
	r.mu.Unlock()

	switch request.WorkstationName {
	case runtimeConfigAlignmentReviewWorkstation:
		return workers.CommandResult{Stdout: []byte("review complete DONE")}, nil
	case runtimeConfigAlignmentCronWorkstation:
		return workers.CommandResult{Stdout: []byte("cron task COMPLETE")}, nil
	default:
		return workers.CommandResult{Stdout: []byte("unexpected workstation COMPLETE")}, nil
	}
}

func (r *runtimeConfigAlignmentProviderRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.callCount
}

type runtimeConfigAlignmentScriptRunner struct {
	mu                   sync.Mutex
	callCount            int
	firstDispatchStarted chan struct{}
	firstTimeout         chan struct{}
	releaseSecondAttempt chan struct{}
	firstStartedOnce     sync.Once
	firstTimeoutOnce     sync.Once
}

func newRuntimeConfigAlignmentScriptRunner() *runtimeConfigAlignmentScriptRunner {
	return &runtimeConfigAlignmentScriptRunner{
		firstDispatchStarted: make(chan struct{}),
		firstTimeout:         make(chan struct{}),
		releaseSecondAttempt: make(chan struct{}),
	}
}

func (r *runtimeConfigAlignmentScriptRunner) Run(ctx context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	r.callCount++
	call := r.callCount
	r.mu.Unlock()

	if call == 1 {
		r.firstStartedOnce.Do(func() { close(r.firstDispatchStarted) })
		<-ctx.Done()
		r.firstTimeoutOnce.Do(func() { close(r.firstTimeout) })
		return workers.CommandResult{}, ctx.Err()
	}

	if call == 2 {
		select {
		case <-r.releaseSecondAttempt:
		case <-ctx.Done():
			return workers.CommandResult{}, ctx.Err()
		}
	}

	return workers.CommandResult{Stdout: []byte("script-output-after-retry")}, nil
}

func (r *runtimeConfigAlignmentScriptRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.callCount
}

func (r *runtimeConfigAlignmentScriptRunner) waitForFirstDispatch(timeout time.Duration) bool {
	select {
	case <-r.firstDispatchStarted:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (r *runtimeConfigAlignmentScriptRunner) waitForFirstTimeout(timeout time.Duration) bool {
	select {
	case <-r.firstTimeout:
		return true
	case <-time.After(timeout):
		return false
	}
}

func waitForRuntimeConfigAlignmentStopWordDispatch(
	t *testing.T,
	server *FunctionalServer,
) {
	t.Helper()

	deadline := time.Now().Add(runtimeConfigAlignmentSignalTimeout)
	for time.Now().Before(deadline) {
		snapshot := server.GetEngineStateSnapshot(t)
		for _, dispatch := range snapshot.DispatchHistory {
			if dispatch.WorkstationName == runtimeConfigAlignmentReviewWorkstation && dispatch.Outcome == interfaces.OutcomeAccepted {
				return
			}
		}
		time.Sleep(runtimeConfigAlignmentPollInterval)
	}

	snapshot := server.GetEngineStateSnapshot(t)
	t.Fatalf("expected %s to accept via stopWords before timeout stage; history=%#v", runtimeConfigAlignmentReviewWorkstation, snapshot.DispatchHistory)
}

func waitForRuntimeConfigAlignmentInFlightResourceConsumption(
	t *testing.T,
	server *FunctionalServer,
	runner *runtimeConfigAlignmentScriptRunner,
) {
	t.Helper()

	if !runner.waitForFirstDispatch(runtimeConfigAlignmentSignalTimeout) {
		t.Fatalf("timed out waiting for %s to start", runtimeConfigAlignmentExecuteWorkstation)
	}

	deadline := time.Now().Add(runtimeConfigAlignmentSignalTimeout)
	for time.Now().Before(deadline) {
		snapshot := server.GetEngineStateSnapshot(t)
		if snapshot.InFlightCount > 0 && len(snapshot.Marking.PlaceTokens["agent-slot:available"]) == 0 {
			return
		}
		time.Sleep(runtimeConfigAlignmentPollInterval)
	}

	snapshot := server.GetEngineStateSnapshot(t)
	t.Fatalf(
		"expected %s to consume agent-slot while in flight; in_flight=%d places=%#v",
		runtimeConfigAlignmentExecuteWorkstation,
		snapshot.InFlightCount,
		snapshot.Marking.PlaceTokens,
	)
}

func waitForRuntimeConfigAlignmentTimeoutAndRequeue(
	t *testing.T,
	server *FunctionalServer,
	runner *runtimeConfigAlignmentScriptRunner,
) {
	t.Helper()

	if !runner.waitForFirstTimeout(runtimeConfigAlignmentSignalTimeout) {
		t.Fatalf("timed out waiting for %s to hit limits.maxExecutionTime", runtimeConfigAlignmentExecuteWorkstation)
	}
	close(runner.releaseSecondAttempt)

	deadline := time.Now().Add(runtimeConfigAlignmentSignalTimeout)
	for time.Now().Before(deadline) {
		snapshot := server.GetEngineStateSnapshot(t)
		if dispatch, ok := runtimeConfigAlignmentFindDispatch(snapshot.DispatchHistory, runtimeConfigAlignmentExecuteWorkstation, interfaces.OutcomeFailed, "execution timeout"); ok {
			if runtimeConfigAlignmentHasMutationToPlace(dispatch.OutputMutations, "task:reviewed") &&
				runtimeConfigAlignmentHasMutationToPlace(dispatch.OutputMutations, "agent-slot:available") {
				return
			}
		}
		time.Sleep(runtimeConfigAlignmentPollInterval)
	}

	snapshot := server.GetEngineStateSnapshot(t)
	t.Fatalf(
		"expected timed-out %s dispatch to requeue task:reviewed and restore agent-slot; history=%#v places=%#v",
		runtimeConfigAlignmentExecuteWorkstation,
		snapshot.DispatchHistory,
		snapshot.Marking.PlaceTokens,
	)
}

func runtimeConfigAlignmentFindDispatch(
	history []interfaces.CompletedDispatch,
	workstation string,
	outcome interfaces.WorkOutcome,
	reason string,
) (interfaces.CompletedDispatch, bool) {
	for _, dispatch := range history {
		if dispatch.WorkstationName != workstation {
			continue
		}
		if dispatch.Outcome != outcome {
			continue
		}
		if reason != "" && dispatch.Reason != reason {
			continue
		}
		return dispatch, true
	}
	return interfaces.CompletedDispatch{}, false
}

func runtimeConfigAlignmentHasDispatch(
	history []interfaces.CompletedDispatch,
	workstation string,
	outcome interfaces.WorkOutcome,
	reason string,
) bool {
	_, ok := runtimeConfigAlignmentFindDispatch(history, workstation, outcome, reason)
	return ok
}

func runtimeConfigAlignmentHasMutationToPlace(mutations []interfaces.TokenMutationRecord, placeID string) bool {
	for _, mutation := range mutations {
		if mutation.ToPlace == placeID {
			return true
		}
	}
	return false
}

func assertRuntimeConfigAlignmentDispatchHistory(t *testing.T, history []interfaces.CompletedDispatch) {
	t.Helper()

	if len(history) < 4 {
		t.Fatalf("dispatch history length = %d, want at least 4", len(history))
	}
	if !runtimeConfigAlignmentHasDispatch(history, runtimeConfigAlignmentReviewWorkstation, interfaces.OutcomeAccepted, "") {
		t.Fatalf("dispatch history missing accepted %s: %#v", runtimeConfigAlignmentReviewWorkstation, history)
	}
	if !runtimeConfigAlignmentHasDispatch(history, runtimeConfigAlignmentExecuteWorkstation, interfaces.OutcomeFailed, "execution timeout") {
		t.Fatalf("dispatch history missing execution-timeout failure for %s: %#v", runtimeConfigAlignmentExecuteWorkstation, history)
	}
	if !runtimeConfigAlignmentHasDispatch(history, runtimeConfigAlignmentExecuteWorkstation, interfaces.OutcomeAccepted, "") {
		t.Fatalf("dispatch history missing accepted retry for %s: %#v", runtimeConfigAlignmentExecuteWorkstation, history)
	}
	if !runtimeConfigAlignmentHasDispatch(history, runtimeConfigAlignmentCronWorkstation, interfaces.OutcomeAccepted, "") {
		t.Fatalf("dispatch history missing accepted %s: %#v", runtimeConfigAlignmentCronWorkstation, history)
	}
	if !runtimeConfigAlignmentDispatchConsumedPlace(history, runtimeConfigAlignmentCronWorkstation, interfaces.SystemTimePendingPlaceID) {
		t.Fatalf("dispatch history missing %s consumption of %s: %#v", runtimeConfigAlignmentCronWorkstation, interfaces.SystemTimePendingPlaceID, history)
	}
}

func assertRuntimeConfigAlignmentEventHistory(t *testing.T, server *FunctionalServer) {
	t.Helper()

	events, err := server.service.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	for _, eventType := range []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeRunRequest,
		factoryapi.FactoryEventTypeInitialStructureRequest,
		factoryapi.FactoryEventTypeWorkRequest,
		factoryapi.FactoryEventTypeDispatchRequest,
		factoryapi.FactoryEventTypeDispatchResponse,
	} {
		if runtimeConfigAlignmentCountFactoryEvents(events, eventType) == 0 {
			t.Fatalf("GetFactoryEvents missing %s in canonical history", eventType)
		}
	}
	if got := runtimeConfigAlignmentCountFactoryEvents(events, factoryapi.FactoryEventTypeDispatchResponse); got < 4 {
		t.Fatalf("DISPATCH_RESPONSE events = %d, want at least 4", got)
	}

	worldState, err := projections.ReconstructFactoryWorldState(events, runtimeConfigAlignmentMaxTick(events))
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	assertRuntimeConfigAlignmentProjectedWorkstationKind(
		t,
		worldState.Topology,
		runtimeConfigAlignmentCronWorkstation,
		interfaces.CanonicalPublicWorkstationKind(interfaces.WorkstationKindCron),
	)
	assertRuntimeConfigAlignmentProjectedWorkstationKind(
		t,
		worldState.Topology,
		runtimeConfigAlignmentReviewWorkstation,
		interfaces.CanonicalPublicWorkstationKind(interfaces.WorkstationKindRepeater),
	)

	worldView := projections.BuildFactoryWorldView(worldState)
	if got := worldView.Runtime.PlaceTokenCounts["task:complete"]; got != 1 {
		t.Fatalf("canonical world view task:complete count = %d, want 1", got)
	}
	if got := worldView.Runtime.PlaceTokenCounts["scheduled:complete"]; got != 1 {
		t.Fatalf("canonical world view scheduled:complete count = %d, want 1", got)
	}
}

func assertRuntimeConfigAlignmentCompleteTokenPayload(t *testing.T, tokens map[string]*interfaces.Token) {
	t.Helper()

	for _, token := range tokens {
		if token == nil || token.PlaceID != "task:complete" {
			continue
		}
		if string(token.Color.Payload) != "script-output-after-retry" {
			t.Fatalf("completed token payload = %q, want script-output-after-retry", string(token.Color.Payload))
		}
		return
	}

	t.Fatal("expected completed token payload for task:complete")
}

func runtimeConfigAlignmentCountFactoryEvents(
	events []factoryapi.FactoryEvent,
	eventType factoryapi.FactoryEventType,
) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func runtimeConfigAlignmentMaxTick(events []factoryapi.FactoryEvent) int {
	maxTick := 0
	for _, event := range events {
		if event.Context.Tick > maxTick {
			maxTick = event.Context.Tick
		}
	}
	return maxTick
}

func runtimeConfigAlignmentDispatchConsumedPlace(
	history []interfaces.CompletedDispatch,
	workstation string,
	placeID string,
) bool {
	for _, dispatch := range history {
		if dispatch.WorkstationName != workstation {
			continue
		}
		for _, token := range dispatch.ConsumedTokens {
			if token.PlaceID == placeID {
				return true
			}
		}
	}
	return false
}

func assertRuntimeConfigAlignmentTopologyProjection(t *testing.T, dir string) {
	t.Helper()

	replayProjection := projectReplayInitialStructureFromEmbeddedConfig(t, dir)
	assertRuntimeConfigAlignmentTopologyPayload(t, replayProjection)
}

func assertRuntimeConfigAlignmentTopologyPayload(t *testing.T, payload interfaces.InitialStructurePayload) {
	t.Helper()

	assertRuntimeConfigAlignmentProjectedWorkstationKind(t, payload, runtimeConfigAlignmentCronWorkstation, interfaces.CanonicalPublicWorkstationKind(interfaces.WorkstationKindCron))
	assertRuntimeConfigAlignmentProjectedWorkstationKind(t, payload, runtimeConfigAlignmentReviewWorkstation, interfaces.CanonicalPublicWorkstationKind(interfaces.WorkstationKindRepeater))
	assertRuntimeConfigAlignmentConstraint(t, payload.Constraints, "workstation/"+runtimeConfigAlignmentExecuteWorkstation+"/limits", "workstation_limit", map[string]string{
		"max_execution_time": "100ms",
		"max_retries":        "2",
	})
	assertRuntimeConfigAlignmentConstraint(t, payload.Constraints, "workstation/"+runtimeConfigAlignmentReviewWorkstation+"/stop-words", "stop_words", map[string]string{
		"words": "DONE",
	})
	assertRuntimeConfigAlignmentConstraint(t, payload.Constraints, "workstation/"+runtimeConfigAlignmentCronWorkstation+"/cron", "cron_trigger", map[string]string{
		"schedule":      "0 * * * *",
		"expiry_window": "1h",
	})
}

func assertRuntimeConfigAlignmentProjectedWorkstationKind(
	t *testing.T,
	payload interfaces.InitialStructurePayload,
	workstationID string,
	wantKind string,
) {
	t.Helper()

	for _, workstation := range payload.Workstations {
		if workstation.ID != workstationID {
			continue
		}
		if workstation.Kind != wantKind {
			t.Fatalf("workstation %s kind = %q, want %q in %#v", workstationID, workstation.Kind, wantKind, payload.Workstations)
		}
		return
	}

	t.Fatalf("missing workstation %s in %#v", workstationID, payload.Workstations)
}

func assertRuntimeConfigAlignmentConstraint(
	t *testing.T,
	constraints []interfaces.FactoryConstraint,
	id string,
	wantType string,
	wantValues map[string]string,
) {
	t.Helper()

	matches := 0
	for _, constraint := range constraints {
		if constraint.ID != id {
			continue
		}
		matches++
		if constraint.Type != wantType || !reflect.DeepEqual(constraint.Values, wantValues) {
			t.Fatalf("constraint %s = %#v, want type=%s values=%#v", id, constraint, wantType, wantValues)
		}
	}
	if matches != 1 {
		t.Fatalf("constraint %s count = %d, want 1 in %#v", id, matches, constraints)
	}
}
