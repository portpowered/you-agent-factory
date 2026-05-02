package providers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestScriptExecutor_Success(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("input-payload"))

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(successRunner("script-output-ok")),
	)

	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	assertTokenPayload(t, h.Marking(), "task:done", "script-output-ok")
}

func TestScriptExecutor_Failure(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("input-payload"))

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(failureRunner("script broke")),
	)

	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:done")
}

func TestScriptExecutor_PreservesTokenColor(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("original-payload"))

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(successRunner("new-payload")),
	)

	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:init")

	snap := h.Marking()
	for _, tok := range snap.Tokens {
		if tok.PlaceID == "task:done" {
			if got := string(tok.Color.Payload); got != "new-payload" {
				t.Errorf("expected payload %q, got %q", "new-payload", got)
			}
			if tok.Color.WorkTypeID != "task" {
				t.Errorf("expected WorkTypeID 'task', got %q", tok.Color.WorkTypeID)
			}
			return
		}
	}
	t.Error("no token found in task:done")
}

func TestScriptExecutor_SuccessWithColorMetadata(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkID:     "work-seed-001",
		WorkTypeID: "task",
		TraceID:    "trace-seed-001",
		Payload:    []byte("seed-payload"),
		Tags:       map[string]string{"env": "test", "team": "platform"},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(successRunner("success-output")),
	)

	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	snap := h.Marking()
	for _, tok := range snap.Tokens {
		if tok.PlaceID == "task:done" {
			if got := string(tok.Color.Payload); got != "success-output" {
				t.Errorf("expected payload %q, got %q", "success-output", got)
			}
			if tok.Color.WorkID != "work-seed-001" {
				t.Errorf("WorkID: want 'work-seed-001', got %q", tok.Color.WorkID)
			}
			if tok.Color.WorkTypeID != "task" {
				t.Errorf("WorkTypeID: want 'task', got %q", tok.Color.WorkTypeID)
			}
			if tok.Color.TraceID != "trace-seed-001" {
				t.Errorf("TraceID: want 'trace-seed-001', got %q", tok.Color.TraceID)
			}
			if tok.Color.Tags["env"] != "test" {
				t.Errorf("Tags[env]: want 'test', got %q", tok.Color.Tags["env"])
			}
			if tok.Color.Tags["team"] != "platform" {
				t.Errorf("Tags[team]: want 'platform', got %q", tok.Color.Tags["team"])
			}
			return
		}
	}
	t.Error("no token found in task:done")
}

func TestScriptExecutor_FailureRoutesToFailedPlace(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("input-payload"))

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(failureRunner("script-error-output")),
	)

	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:done")

	snap := h.Marking()
	for _, tok := range snap.Tokens {
		if tok.PlaceID == "task:failed" {
			if strings.Contains(tok.History.LastError, "script-error-output") {
				return
			}
			for _, fr := range tok.History.FailureLog {
				if strings.Contains(fr.Error, "script-error-output") {
					return
				}
			}
			t.Errorf("expected token history to contain 'script-error-output', got LastError=%q, FailureLog=%+v",
				tok.History.LastError, tok.History.FailureLog)
			return
		}
	}
	t.Error("no token found in task:failed")
}

func TestScriptExecutor_ArgTemplating(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))

	agentsMD := "---\ntype: SCRIPT_WORKER\ncommand: echo\nargs:\n  - \"{{ (index .Inputs 0).Name }}\"\n  - \"{{ (index .Inputs 0).WorkID }}\"\n---\n"
	agentsPath := filepath.Join(dir, "workers", "script-worker", "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "prd-my-feature",
		WorkID:     "work-abc-123",
		WorkTypeID: "task",
		TraceID:    "trace-tmpl-test",
		Payload:    []byte("template-test-payload"),
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(&echoArgsRunner{}),
	)

	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	assertTokenPayload(t, h.Marking(), "task:done", "prd-my-feature\nwork-abc-123")
}

func TestScriptExecutor_WorkTypeIDFromTargetPlace(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "type-stamp-test",
		WorkID:     "work-type-stamp",
		WorkTypeID: "task",
		TraceID:    "trace-type-stamp",
		Payload:    []byte("payload"),
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(successRunner("output")),
	)

	h.RunUntilComplete(t, 5*time.Second)

	snap := h.Marking()
	for _, tok := range snap.Tokens {
		if tok.PlaceID == "task:done" {
			if tok.Color.WorkTypeID != "task" {
				t.Errorf("WorkTypeID: want 'task', got %q", tok.Color.WorkTypeID)
			}
			if tok.Color.WorkID != "work-type-stamp" {
				t.Errorf("WorkID: want 'work-type-stamp' (preserved for same-type), got %q", tok.Color.WorkID)
			}
			return
		}
	}
	t.Error("no token found in task:done")
}

func TestScriptExecutor_ArgTemplatingWithTags(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))

	agentsMD := "---\ntype: SCRIPT_WORKER\ncommand: echo\nargs:\n  - '{{ index (index .Inputs 0).Tags \"env\" }}'\n  - '{{ index (index .Inputs 0).Tags \"team\" }}'\n---\n"
	agentsPath := filepath.Join(dir, "workers", "script-worker", "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkID:     "work-tag-test",
		WorkTypeID: "task",
		TraceID:    "trace-tag-test",
		Payload:    []byte("tag-test"),
		Tags:       map[string]string{"env": "staging", "team": "infra"},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(&echoArgsRunner{}),
	)

	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:done", 1)

	assertTokenPayload(t, h.Marking(), "task:done", "staging\ninfra")
}

func TestScriptExecutor_RuntimeWorkstationConfigResolvesWorkingDirectoryAndEnv(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	support.SetWorkingDirectory(t, dir)

	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		workstations := cfg["workstations"].([]any)
		workstation := workstations[0].(map[string]any)
		workstation["workingDirectory"] = `/tmp/{{ index (index .Inputs 0).Tags "branch" }}`
		workstation["env"] = map[string]any{
			"TEAM":   `{{ index (index .Inputs 0).Tags "team" }}`,
			"BRANCH": `{{ index (index .Inputs 0).Tags "branch" }}`,
		}
	})

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkID:     "script-runtime-fields",
		WorkTypeID: "task",
		TraceID:    "trace-script-runtime-fields",
		Payload:    []byte("input-payload"),
		Tags: map[string]string{
			"branch": "feature-script",
			"team":   "platform",
		},
	})

	runner := &captureCommandRunner{}
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(runner),
	)

	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:failed")

	if got := runner.LastWorkDir(); got != support.ResolvedRuntimePath(dir, "/tmp/feature-script") {
		t.Fatalf("expected script runner work dir %q, got %q", support.ResolvedRuntimePath(dir, "/tmp/feature-script"), got)
	}

	env := runner.LastEnv()
	if !containsEnv(env, "TEAM=platform") {
		t.Fatalf("expected TEAM env in %v", env)
	}
	if !containsEnv(env, "BRANCH=feature-script") {
		t.Fatalf("expected BRANCH env in %v", env)
	}
}

func TestScriptExecutor_RuntimeConfigMergePreservesCanonicalTopologyAndPromptTemplates(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	support.SetWorkingDirectory(t, dir)

	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		cfg["workTypes"] = []any{
			map[string]any{
				"name": "task",
				"states": []any{
					map[string]any{"name": "init", "type": "INITIAL"},
					map[string]any{"name": "canonical-done", "type": "TERMINAL"},
					map[string]any{"name": "runtime-done", "type": "TERMINAL"},
					map[string]any{"name": "failed", "type": "FAILED"},
				},
			},
		}

		workstations := cfg["workstations"].([]any)
		workstation := workstations[0].(map[string]any)
		workstation["type"] = "MODEL_WORKSTATION"
		workstation["promptTemplate"] = "inline prompt {{ (index .Inputs 0).Name }}"
		workstation["workingDirectory"] = `/inline/{{ (index .Inputs 0).Name }}`
		workstation["env"] = map[string]any{
			"INLINE_ONLY":    "true",
			"RUNTIME_BRANCH": "inline-branch",
		}
		workstation["outputs"] = []any{
			map[string]any{"workType": "task", "state": "canonical-done"},
		}
	})
	writeRuntimeMergeWorkstationConfig(t, dir)
	writeScriptWorkerArgs(t, dir, []string{
		`name={{ (index .Inputs 0).Name }}`,
		`work={{ (index .Inputs 0).WorkID }}`,
		`payload={{ (index .Inputs 0).Payload }}`,
		`workdir={{ .Context.WorkDir }}`,
		`env_branch={{ index .Context.Env "RUNTIME_BRANCH" }}`,
	})

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "runtime-template-name",
		WorkID:     "work-runtime-config",
		WorkTypeID: "task",
		TraceID:    "trace-runtime-config",
		Payload:    []byte("runtime-payload"),
		Tags: map[string]string{
			"branch": "feature-runtime-config",
		},
	})

	runner := &templateCaptureCommandRunner{}
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(runner),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:runtime-done", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:canonical-done").
		HasNoTokenInPlace("task:failed")

	req := runner.LastRequest()
	wantArgs := []string{
		"name=runtime-template-name",
		"work=work-runtime-config",
		"payload=runtime-payload",
		"workdir=" + support.ResolvedRuntimePath(dir, "/runtime/runtime-template-name/feature-runtime-config"),
		"env_branch=feature-runtime-config",
	}
	assertCommandArgs(t, req, wantArgs)
	assertRuntimeMergeCommandRequest(t, dir, req)
	assertTokenPayload(t, h.Marking(), "task:runtime-done", strings.Join(wantArgs, "\n"))
}

func TestScriptExecutor_RuntimeWorkstationTimeoutRequeuesAndRetriesOnLaterTick(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))

	workstationAgentsPath := filepath.Join(dir, "workstations", "run-script", "AGENTS.md")
	agentsMD := "---\ntype: MODEL_WORKSTATION\nlimits:\n  maxExecutionTime: 10ms\n---\nExecute the script.\n"
	if err := os.WriteFile(workstationAgentsPath, []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}

	testutil.WriteSeedFile(t, dir, "task", []byte("input-payload"))

	runner := newTimeoutThenSuccessCommandRunner()
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(runner),
	)

	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	if runner.CallCount() < 2 {
		t.Fatalf("expected script runner to be called at least twice, got %d", runner.CallCount())
	}

	engineState, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot() error = %v", err)
	}
	if len(engineState.DispatchHistory) < 2 {
		t.Fatalf("DispatchHistory length = %d, want at least 2", len(engineState.DispatchHistory))
	}
	if engineState.DispatchHistory[0].Outcome != interfaces.OutcomeFailed {
		t.Fatalf("first DispatchHistory outcome = %s, want %s", engineState.DispatchHistory[0].Outcome, interfaces.OutcomeFailed)
	}
	if engineState.DispatchHistory[0].Reason != "execution timeout" {
		t.Fatalf("first DispatchHistory reason = %q, want %q", engineState.DispatchHistory[0].Reason, "execution timeout")
	}
	if engineState.DispatchHistory[len(engineState.DispatchHistory)-1].Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("last DispatchHistory outcome = %s, want %s", engineState.DispatchHistory[len(engineState.DispatchHistory)-1].Outcome, interfaces.OutcomeAccepted)
	}
}

func TestScriptExecutor_RuntimeWorkerTimeoutFromLoadedConfigRequeuesAndRetriesOnLaterTick(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))

	workerAgentsPath := filepath.Join(dir, "workers", "script-worker", "AGENTS.md")
	agentsMD := "---\ntype: SCRIPT_WORKER\ncommand: echo\ntimeout: 10ms\n---\nExecute the script.\n"
	if err := os.WriteFile(workerAgentsPath, []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write worker AGENTS.md: %v", err)
	}

	testutil.WriteSeedFile(t, dir, "task", []byte("input-payload"))

	runner := newTimeoutThenSuccessCommandRunner()
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(runner),
	)

	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	if runner.CallCount() < 2 {
		t.Fatalf("expected script runner to be called at least twice, got %d", runner.CallCount())
	}

	engineState, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot() error = %v", err)
	}
	if len(engineState.DispatchHistory) < 2 {
		t.Fatalf("DispatchHistory length = %d, want at least 2", len(engineState.DispatchHistory))
	}
	if engineState.DispatchHistory[0].Reason != "execution timeout" {
		t.Fatalf("first DispatchHistory reason = %q, want %q", engineState.DispatchHistory[0].Reason, "execution timeout")
	}
}

func TestScriptExecutor_AsyncWorkerPoolTemplateFallbackScenarios(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow async worker-pool template fallback sweep")

	t.Run("SingleFileInputWithTemplateAndPayload_Completes", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
		writeWorkstationPromptTemplate(t, dir, "payload: {{ (index .Inputs 0).Payload }}")
		testutil.WriteSeedFile(t, dir, "task", []byte("template-input"))

		h := testutil.NewServiceTestHarness(t, dir,
			testutil.WithFullWorkerPoolAndScriptWrap(),
			testutil.WithCommandRunner(successRunner("template-case-ok")),
		)

		h.RunUntilComplete(t, 10*time.Second)

		h.Assert().
			PlaceTokenCount("task:done", 1).
			HasNoTokenInPlace("task:init").
			HasNoTokenInPlace("task:failed")

		assertTokenPayload(t, h.Marking(), "task:done", "template-case-ok")
	})

	t.Run("NoTemplateWithPayload_Completes", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
		testutil.WriteSeedFile(t, dir, "task", []byte("payload-only"))

		h := testutil.NewServiceTestHarness(t, dir,
			testutil.WithFullWorkerPoolAndScriptWrap(),
			testutil.WithCommandRunner(successRunner("payload-only-ok")),
		)

		h.RunUntilComplete(t, 10*time.Second)

		h.Assert().
			PlaceTokenCount("task:done", 1).
			HasNoTokenInPlace("task:init").
			HasNoTokenInPlace("task:failed")

		assertTokenPayload(t, h.Marking(), "task:done", "payload-only-ok")
	})

	t.Run("NoTemplateAndNoPayload_Completes", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_no_template"))

		h := testutil.NewServiceTestHarness(t, dir,
			testutil.WithFullWorkerPoolAndScriptWrap(),
			testutil.WithCommandRunner(successRunner("empty-input-ok")),
		)

		if err := h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
			WorkID:     "work-no-template-no-payload",
			WorkTypeID: "task",
			TraceID:    "trace-no-template-no-payload",
			Payload:    nil,
		}}); err != nil {
			t.Fatalf("submit nil-payload work: %v", err)
		}

		h.RunUntilComplete(t, 10*time.Second)

		h.Assert().
			PlaceTokenCount("task:done", 1).
			HasNoTokenInPlace("task:init").
			HasNoTokenInPlace("task:failed")

		assertTokenPayload(t, h.Marking(), "task:done", "empty-input-ok")
	})
}
