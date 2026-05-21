package runtime_api

import (
	"reflect"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func setupRuntimeConfigAlignmentFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, runtimeConfigAlignmentFactoryJSONConfig())
	writeRuntimeConfigAlignmentAgentConfigs(t, dir)
	return dir
}

func runtimeConfigAlignmentFactoryJSONConfig() map[string]any {
	return map[string]any{
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
		"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
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
		"onFailure": []map[string]string{{"workType": "scheduled", "state": "failed"}},
	}
}

func writeRuntimeConfigAlignmentAgentConfigs(t *testing.T, dir string) {
	t.Helper()

	support.WriteAgentConfig(t, dir, "reviewer", `---
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
	support.WriteAgentConfig(t, dir, "executor", `---
type: SCRIPT_WORKER
command: echo
resources:
  - name: agent-slot
    capacity: 1
---
You are the execution worker.
`)
	support.WriteAgentConfig(t, dir, "cron-worker", `---
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
  triggerAtStart: true
  jitter: 5s
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

func stringSliceValue(values *[]string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), (*values)...)
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
