package functional_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/internal/testpath"
	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
)

const cleanupSmokeProject = "acme-inventory"

func TestProjectAgnosticCleanupSmoke_ReadmeQuickstartAndReleasePathsRemainStandalone(t *testing.T) {
	readmeBytes, err := os.ReadFile(agentFactoryPath(t, "README.md"))
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	readme := string(readmeBytes)
	assertValueDoesNotContainPortOS(t, "README", readme)

	for _, want := range []string{
		"standalone workflow library and CLI",
		"cd ~/src/sample-project",
		"agent-factory",
		"factory/inputs/tasks/default/my-request.md",
		"Created or reused by `agent-factory` or `agent-factory init`.",
		"This repository also ships a richer checked-in starter under",
		"Submit Markdown stories under `factory/inputs/story/default`.",
		"./factory/README.md",
		"./examples/simple-tasks/README.md",
	} {
		if !strings.Contains(readme, want) {
			t.Fatalf("README missing standalone release-flow text %q", want)
		}
	}
	for _, unwanted := range []string{
		"tests/",
		"portos-monolith",
		"portos-backend",
	} {
		if strings.Contains(readme, unwanted) {
			t.Fatalf("README should not send release readers to %q:\n%s", unwanted, readme)
		}
	}

	for _, rel := range []string{
		"factory/README.md",
		"examples/simple-tasks/README.md",
		"examples/thought-idea--plan-work-review/inputs/task/default/prd-agent-factory-workflow-dashboard-redesign.md",
	} {
		if _, err := os.Stat(agentFactoryPath(t, rel)); err != nil {
			t.Fatalf("release surface path %s should exist: %v", rel, err)
		}
	}
}

func TestProjectAgnosticCleanupSmoke_ReadmeStarterLinksMatchReferencedScaffolds(t *testing.T) {
	readmeBytes, err := os.ReadFile(agentFactoryPath(t, "README.md"))
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	readme := string(readmeBytes)

	starterReadmeBytes, err := os.ReadFile(agentFactoryPath(t, "factory/README.md"))
	if err != nil {
		t.Fatalf("read checked-in starter README: %v", err)
	}
	starterReadme := string(starterReadmeBytes)

	factoryJSONBytes, err := os.ReadFile(agentFactoryPath(t, "factory/factory.json"))
	if err != nil {
		t.Fatalf("read checked-in starter factory.json: %v", err)
	}
	factoryJSON := string(factoryJSONBytes)

	for _, want := range []string{
		"### 🏗️ Default init scaffold",
		"Created or reused by `agent-factory` or `agent-factory init`.",
		"factory/inputs/tasks/default",
		"### 🧭 Checked-in review-loop starter",
		"[`./factory/`](./factory/README.md)",
		"not the default `agent-factory init` scaffold",
		"Submit Markdown stories under `factory/inputs/story/default`.",
	} {
		if !strings.Contains(readme, want) {
			t.Fatalf("README missing scaffold-alignment text %q", want)
		}
	}
	if strings.Contains(readme, "See [factory/README.md](./factory/README.md) for the starter input layout.") {
		t.Fatalf("README should not describe the checked-in ./factory tree as the default starter:\n%s", readme)
	}

	for _, want := range []string{
		"# Checked-In Root Workflow Starter",
		"It is not the default `agent-factory` or `agent-factory init` scaffold",
		"`thoughts:init`",
		"`idea:init`",
		"`task:init`",
	} {
		if !strings.Contains(starterReadme, want) {
			t.Fatalf("checked-in starter README missing %q:\n%s", want, starterReadme)
		}
	}
	if !strings.Contains(factoryJSON, `"name": "task"`) {
		t.Fatalf("checked-in starter factory.json should include the task workflow described by README:\n%s", factoryJSON)
	}
	if strings.Contains(factoryJSON, `"name": "tasks"`) {
		t.Fatalf("checked-in starter factory.json should not masquerade as the default init scaffold:\n%s", factoryJSON)
	}
}

func TestProjectAgnosticCleanupSmoke_CheckedInStarterScaffoldFilesRemainNeutralAndCanonical(t *testing.T) {
	files := []string{
		"factory/README.md",
		"factory/factory.json",
		"factory/workers/processor/AGENTS.md",
		"factory/workers/workspace-setup/AGENTS.md",
	}

	for _, rel := range files {
		data, err := os.ReadFile(agentFactoryPath(t, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		assertValueDoesNotContainPortOS(t, rel, string(data))
	}

	factoryJSONBytes, err := os.ReadFile(agentFactoryPath(t, "factory/factory.json"))
	if err != nil {
		t.Fatalf("read starter factory.json: %v", err)
	}
	factoryJSON := string(factoryJSONBytes)
	for _, want := range []string{`"workTypes"`, `"workType"`, `"onFailure"`, `"onRejection"`, `"resources"`, `"maxVisits"`} {
		if !strings.Contains(factoryJSON, want) {
			t.Fatalf("starter factory.json missing canonical key %s:\n%s", want, factoryJSON)
		}
	}
	for _, retired := range []string{`"work_types"`, `"work_type"`, `"on_failure"`, `"on_rejection"`, `"resource_usage"`, `"max_visits"`} {
		if strings.Contains(factoryJSON, retired) {
			t.Fatalf("starter factory.json should not contain retired key %s:\n%s", retired, factoryJSON)
		}
	}

	for _, rel := range []string{"factory/workers/processor/AGENTS.md"} {
		data, err := os.ReadFile(agentFactoryPath(t, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		content := string(data)
		if strings.Contains(content, "stop_token:") {
			t.Fatalf("%s should not contain retired stop_token key:\n%s", rel, content)
		}
		if !strings.Contains(content, "stopToken:") {
			t.Fatalf("%s missing canonical stopToken key:\n%s", rel, content)
		}
	}

}

func TestProjectAgnosticCleanupSmoke_RequestDispatchAndRuntimeContext(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "tags_test"))
	setWorkingDirectory(t, dir)
	clearFactoryProject(t, dir)

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"checker": {{Content: "project cleanup smoke COMPLETE"}},
	})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	request := interfaces.WorkRequest{
		RequestID: "request-project-cleanup-smoke",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "project-cleanup-smoke",
			WorkID:     "work-project-cleanup-smoke",
			WorkTypeID: "task",
			TraceID:    "trace-project-cleanup-smoke",
			Payload:    "verify project-agnostic cleanup",
			Tags: map[string]string{
				"branch":  "feature/acme-cleanup",
				"project": cleanupSmokeProject,
			},
		}},
	}
	h.SubmitWorkRequest(context.Background(), request)
	h.RunUntilComplete(t, 10*time.Second)

	calls := provider.Calls("checker")
	if len(calls) != 1 {
		t.Fatalf("checker provider calls = %d, want 1", len(calls))
	}
	assertCleanupSmokeRuntimeContext(t, dir, calls[0])
	assertInferenceRequestsDoNotContainPortOS(t, calls)

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	var foundTerminalToken bool
	for _, token := range snapshot.Marking.Tokens {
		if token == nil || token.Color.WorkID != "work-project-cleanup-smoke" {
			continue
		}
		foundTerminalToken = true
		if token.PlaceID != "task:complete" {
			t.Fatalf("cleanup smoke token place = %q, want task:complete", token.PlaceID)
		}
		assertCleanupSmokeTags(t, "terminal token", token.Color.Tags)
	}
	if !foundTerminalToken {
		t.Fatal("missing cleanup smoke terminal token")
	}

	events, err := h.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	assertCleanupSmokeEvents(t, events)
}

// portos:func-length-exception owner=agent-factory reason=project-agnostic-functional-smoke review=2026-07-18 removal=split-example-setup-runtime-and-provider-assertions-before-next-project-agnostic-change
func TestProjectAgnosticCleanupSmoke_ModifiedExamplesExecuteWithNeutralProjectConfig(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow example cleanup smoke")
	tests := []struct {
		name                 string
		dir                  string
		request              interfaces.SubmitRequest
		responses            map[string][]interfaces.InferenceResponse
		wantPlace            string
		wantWorker           string
		seedMarkdownPath     string
		seedMarkdownWorkType string
		seedMarkdownName     string
	}{
		{
			name: "basic example",
			dir:  "examples/basic/factory",
			request: interfaces.SubmitRequest{
				WorkTypeID: "task",
				WorkID:     "basic-example-cleanup-smoke",
				TraceID:    "trace-basic-example-cleanup-smoke",
				Name:       "basic example cleanup smoke",
				Payload:    []byte("basic example cleanup smoke"),
			},
			responses:  map[string][]interfaces.InferenceResponse{"processor": {{Content: "basic example complete DONE"}}},
			wantPlace:  "task:complete",
			wantWorker: "processor",
		},
		{
			name: "simple tasks example",
			dir:  "examples/simple-tasks",
			request: interfaces.SubmitRequest{
				WorkTypeID: "story",
				WorkID:     "simple-tasks-cleanup-smoke",
				TraceID:    "trace-simple-tasks-cleanup-smoke",
				Name:       "simple tasks cleanup smoke",
				Payload:    []byte("simple tasks cleanup smoke"),
			},
			responses: map[string][]interfaces.InferenceResponse{
				"executor": {{Content: "executed <result>ACCEPTED</result>"}},
				"reviewer": {{Content: "reviewed <result>ACCEPTED</result>"}},
			},
			wantPlace:  "story:complete",
			wantWorker: "executor",
		},
		{
			name: "write code review example",
			dir:  "examples/write-code-review",
			request: interfaces.SubmitRequest{
				WorkTypeID: "story",
				WorkID:     "write-code-review-cleanup-smoke",
				TraceID:    "trace-write-code-review-cleanup-smoke",
				Name:       "write code review cleanup smoke",
				Payload:    []byte("write code review cleanup smoke"),
				Tags: map[string]string{
					"branch":   "feature/example-cleanup",
					"worktree": ".worktrees/example-cleanup/write-code-review-cleanup-smoke",
				},
			},
			responses: map[string][]interfaces.InferenceResponse{
				"executor": {{Content: "executed <result>ACCEPTED</result>"}},
				"reviewer": {{Content: "reviewed <result>ACCEPTED</result>"}},
			},
			wantPlace:  "story:complete",
			wantWorker: "executor",
		},
		{
			name: "thought plan review example",
			dir:  "examples/thought-idea--plan-work-review",
			responses: map[string][]interfaces.InferenceResponse{
				"processor": {{Content: "processed <COMPLETE>"}, {Content: "reviewed <COMPLETE>"}},
			},
			wantPlace:            "task:complete",
			wantWorker:           "processor",
			seedMarkdownPath:     "examples/thought-idea--plan-work-review/inputs/task/default/prd-agent-factory-workflow-dashboard-redesign.md",
			seedMarkdownWorkType: "task",
			seedMarkdownName:     "prd-agent-factory-workflow-dashboard-redesign",
		},
		{
			name: "adhoc sample factory",
			dir:  "tests/adhoc/factory",
			request: interfaces.SubmitRequest{
				WorkTypeID: "task",
				WorkID:     "adhoc-factory-cleanup-smoke",
				TraceID:    "trace-adhoc-factory-cleanup-smoke",
				Name:       "adhoc factory cleanup smoke",
				Payload:    []byte("adhoc factory cleanup smoke"),
			},
			responses: map[string][]interfaces.InferenceResponse{
				"processor": {{Content: "processed <COMPLETE>"}, {Content: "reviewed <COMPLETE>"}},
			},
			wantPlace:  "task:complete",
			wantWorker: "processor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := testutil.CopyFixtureDir(t, agentFactoryPath(t, tt.dir))
			setWorkingDirectory(t, dir)
			clearSeedInputs(t, dir)
			setFactoryProject(t, dir, "neutral-example-project")

			provider := testutil.NewMockWorkerMapProvider(tt.responses)
			h := testutil.NewServiceTestHarness(t, dir,
				testutil.WithProvider(provider),
				testutil.WithFullWorkerPoolAndScriptWrap(),
			)

			if tt.seedMarkdownPath != "" {
				samplePayload, err := os.ReadFile(agentFactoryPath(t, tt.seedMarkdownPath))
				if err != nil {
					t.Fatalf("read sample payload: %v", err)
				}
				assertValueDoesNotContainPortOS(t, tt.name+" sample payload", string(samplePayload))
				testutil.WriteSeedMarkdownFile(t, dir, tt.seedMarkdownWorkType, tt.seedMarkdownName, samplePayload)
			} else {
				h.SubmitFull(context.Background(), []interfaces.SubmitRequest{tt.request})
			}
			h.RunUntilComplete(t, 15*time.Second)

			if provider.CallCount(tt.wantWorker) == 0 {
				t.Fatalf("provider did not receive calls for worker %q", tt.wantWorker)
			}
			assertSmokeTokenInPlace(t, h, tt.wantPlace)
			for workerName := range tt.responses {
				assertInferenceRequestsDoNotContainPortOS(t, provider.Calls(workerName))
			}
			if tt.seedMarkdownPath != "" {
				events, err := h.GetFactoryEvents(context.Background())
				if err != nil {
					t.Fatalf("GetFactoryEvents: %v", err)
				}
				assertFactoryEventsDoNotContainPortOS(t, events)
			}
		})
	}
}

func assertSmokeTokenInPlace(t *testing.T, h *testutil.ServiceTestHarness, placeID string) {
	t.Helper()

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	for _, token := range snapshot.Marking.Tokens {
		if token != nil && token.PlaceID == placeID {
			return
		}
	}
	places := make([]string, 0, len(snapshot.Marking.Tokens))
	for _, token := range snapshot.Marking.Tokens {
		if token == nil {
			continue
		}
		places = append(places, fmt.Sprintf("%s:%s:%s", token.Color.WorkID, token.PlaceID, token.History.LastError))
	}
	t.Fatalf("expected token in place %q; observed places: %s", placeID, strings.Join(places, ", "))
}

func assertCleanupSmokeRuntimeContext(t *testing.T, dir string, call interfaces.ProviderInferenceRequest) {
	t.Helper()

	if call.WorkingDirectory != resolvedRuntimePath(dir, "/workspaces/acme-inventory/feature/acme-cleanup") {
		t.Fatalf("working directory = %q, want acme project path", call.WorkingDirectory)
	}
	if call.EnvVars["PROJECT"] != cleanupSmokeProject {
		t.Fatalf("PROJECT env = %q, want %s", call.EnvVars["PROJECT"], cleanupSmokeProject)
	}
	if call.EnvVars["CONTEXT_PROJECT"] != cleanupSmokeProject {
		t.Fatalf("CONTEXT_PROJECT env = %q, want %s", call.EnvVars["CONTEXT_PROJECT"], cleanupSmokeProject)
	}
	if call.EnvVars["BRANCH"] != "feature/acme-cleanup" {
		t.Fatalf("BRANCH env = %q, want feature/acme-cleanup", call.EnvVars["BRANCH"])
	}
	if len(call.InputTokens) != 1 {
		t.Fatalf("provider input tokens = %d, want 1", len(call.InputTokens))
	}
	assertCleanupSmokeTags(t, "provider input token", firstInputToken(call.InputTokens).Color.Tags)
}

func assertCleanupSmokeEvents(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	var sawRequest bool
	var sawWorkInput bool
	var sawDispatch bool
	var sawTerminalOutput bool
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest:
			payload, err := event.Payload.AsWorkRequestEventPayload()
			if err != nil || stringPointerValue(event.Context.RequestId) != "request-project-cleanup-smoke" || payload.Works == nil {
				continue
			}
			sawRequest = true
			if len(*payload.Works) != 1 {
				t.Fatalf("cleanup smoke request works = %d, want 1", len(*payload.Works))
			}
			sawWorkInput = true
			assertCleanupSmokeTags(t, "WORK_REQUEST item", generatedTags((*payload.Works)[0].Tags))
		case factoryapi.FactoryEventTypeDispatchRequest:
			payload, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil || payload.TransitionId != "process" {
				continue
			}
			for _, input := range dispatchInputWorksFromHistory(t, events, event, payload) {
				if stringPointerValue(input.WorkId) != "work-project-cleanup-smoke" {
					continue
				}
				sawDispatch = true
				assertCleanupSmokeTags(t, "DISPATCH_CREATED input", generatedTags(input.Tags))
			}
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil || payload.OutputWork == nil {
				continue
			}
			for _, output := range *payload.OutputWork {
				if stringPointerValue(output.WorkId) != "work-project-cleanup-smoke" {
					continue
				}
				sawTerminalOutput = true
				assertCleanupSmokeTags(t, "DISPATCH_COMPLETED output work", generatedTags(output.Tags))
			}
		}
	}
	if !sawRequest || !sawWorkInput || !sawDispatch || !sawTerminalOutput {
		t.Fatalf(
			"cleanup smoke missing event boundary: request=%v input=%v dispatch=%v terminal=%v",
			sawRequest,
			sawWorkInput,
			sawDispatch,
			sawTerminalOutput,
		)
	}
}

func generatedTags(tags *factoryapi.StringMap) map[string]string {
	if tags == nil {
		return nil
	}
	return map[string]string(*tags)
}

func assertCleanupSmokeTags(t *testing.T, label string, tags map[string]string) {
	t.Helper()

	if tags["project"] != cleanupSmokeProject {
		t.Fatalf("%s project tag = %q, want %s", label, tags["project"], cleanupSmokeProject)
	}
	if tags["branch"] != "feature/acme-cleanup" {
		t.Fatalf("%s branch tag = %q, want feature/acme-cleanup", label, tags["branch"])
	}
	assertMapDoesNotContainPortOS(t, label, tags)
}

func assertInferenceRequestsDoNotContainPortOS(t *testing.T, calls []interfaces.ProviderInferenceRequest) {
	t.Helper()

	if len(calls) == 0 {
		t.Fatal("expected at least one provider request")
	}
	for i, call := range calls {
		data, err := json.Marshal(call)
		if err != nil {
			t.Fatalf("marshal provider request %d: %v", i, err)
		}
		assertValueDoesNotContainPortOS(t, fmt.Sprintf("provider request %d", i), string(data))
	}
}

func assertFactoryEventsDoNotContainPortOS(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	if len(events) == 0 {
		t.Fatal("expected at least one factory event")
	}
	for i, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal factory event %d: %v", i, err)
		}
		assertValueDoesNotContainPortOS(t, fmt.Sprintf("factory event %d (%s)", i, event.Type), string(data))
	}
}

func assertMapDoesNotContainPortOS(t *testing.T, label string, values map[string]string) {
	t.Helper()

	for key, value := range values {
		assertValueDoesNotContainPortOS(t, label+" key", key)
		assertValueDoesNotContainPortOS(t, label+" value", value)
	}
}

func assertValueDoesNotContainPortOS(t *testing.T, label string, value string) {
	t.Helper()

	normalized := strings.ToLower(value)
	if strings.Contains(normalized, "portos") ||
		strings.Contains(normalized, "port os") ||
		strings.Contains(normalized, "port_os") {
		t.Fatalf("%s contains Port OS coupling: %q", label, value)
	}
}

func agentFactoryPath(t *testing.T, rel string) string {
	t.Helper()
	return testpath.MustRepoPathFromCaller(t, 0, filepath.FromSlash(rel))
}

func clearSeedInputs(t *testing.T, dir string) {
	t.Helper()

	if err := os.RemoveAll(filepath.Join(dir, interfaces.InputsDir)); err != nil {
		t.Fatalf("clear seed inputs: %v", err)
	}
}

func setFactoryProject(t *testing.T, dir string, project string) {
	t.Helper()

	updateFactoryProject(t, dir, func(config map[string]any) {
		config["project"] = project
	})
}

func clearFactoryProject(t *testing.T, dir string) {
	t.Helper()

	updateFactoryProject(t, dir, func(config map[string]any) {
		delete(config, "project")
	})
}

func updateFactoryProject(t *testing.T, dir string, update func(map[string]any)) {
	t.Helper()

	factoryPath := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(factoryPath)
	if err != nil {
		t.Fatalf("read factory config: %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse factory config: %v", err)
	}
	update(config)
	data, err = json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory config: %v", err)
	}
	if err := os.WriteFile(factoryPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write factory config: %v", err)
	}
}
