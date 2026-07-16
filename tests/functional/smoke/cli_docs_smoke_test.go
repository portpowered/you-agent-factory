package smoke

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	agentcli "github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type docsSmokeTopic struct {
	name    string
	heading string
	markers []string
	absent  []string
}

// retiredDuplicateTreePaths are legacy maintainer paths removed by docs-embed-consolidation.
var retiredDuplicateTreePaths = []string{
	"pkg/transports/cli/docs/reference/",
	"pkg/transports/cli/docs/reference",
}

var docsSmokeTopics = []docsSmokeTopic{
	{name: "agents", heading: "# Agents", markers: []string{"## Read order", "factory/docs/overview.md", "factory/docs/README.md", "## CLI-only ingress", "Autonomous agents must submit work only through the CLI", "`you submit batch`", "## Batch submit for agents", "### Idempotency and duplicate work", "requestId", "duplicate batches", "you submit batch ./batches/release-story-set.json", "## Is the factory running?", "you session list", "you factory query", "you docs sessions", "## Operator loop", "you work list --name", "## Command matrix", "Operator-only", "## Planner vs executor", "## Topic router", "`you docs config`", "`you docs templates`", "`you docs resources`", "`you docs models`", "`you docs batch-inputs`", "FACTORY_REQUEST_BATCH", "POST /factory-sessions/{session_id}/work", "## Factory-local docs discovery"}, absent: []string{"## Start Here", "## Read Order (Any Factory)", "## Submitting Work", "### Batch submit for agents", "Run `you docs agents`", "[Config](config.md)", "[Work](work.md)", "[Batch Inputs](batch-inputs.md)", "[Relationships](relationships.md)", "[Authoring Factories](authoring-factories.md)", "factoryState", "runtimeStatus", "dashboard/ui", "thoughts:init", "idea:init", "plan:init", "task:in-review", "Work enters a running factory through one of these ingress paths", "| Ingress | When to use |", "| Watched `factory/inputs/**` JSON files |", "| `POST /work` | Single submitted work item", "Place batch files under the inbox paths", "## Related Topics"}},
	{name: "authoring-factories", heading: "# Authoring Factories", markers: []string{"factory.json", "workers/<name>/AGENTS.md", "workstations/<name>/AGENTS.md", "you run --factory ./factory.json \"Fix the lint issues\"", "handlingBehavior: [\"DEFAULT\"]", "you run --dir ./factory --with-mock-workers --work ./docs/examples/startup-work.json", "you docs mock-workers", "you docs record-replay", "you docs run", "you docs models", "you run --named @you/goal", "you run --named @you/tts --output primary \"Read the release summary.\"", "`you docs agents`", "--no-record", "requestId", "workTypeName"}, absent: []string{"work_type_name", "source_work_name", `"request_id"`, "[Agents](agents.md)", "you docs packaged-fusion", "you docs packaged-goal", "you docs packaged-tts"}},
	{name: "run", heading: "# Run", markers: []string{
		"you run",
		"Factory",
		"Factory Session",
		"you docs config",
		"you docs sessions",
		"### Primary-result mode (default)",
		"### Human response-stream mode",
		"### NDJSON automation mode",
		"--output response-stream",
		"recordType=response_event",
		"recordType=invocation_result",
		"invocationReturn",
	}, absent: []string{
		"recordType=progress",
		"recordType=compaction",
		"recordType=primary_result",
		"PROGRESS_FRAGMENT",
		"STREAM_COMPACTION_SIGNAL",
	}},
	{name: "config", heading: "# Config", markers: []string{"## Initialize Operator And System Configuration", "you config init", "~/.you-agent-factory/config.json", "classifier-small", "classifier-medium", "classifier-large", "gpt-5-mini", "gpt-5.4", "YOU_DEFAULT_WORKER_MODEL_PROVIDER", "YOU_DEFAULT_WORKER_MODEL", "file < env < flag", "## Validate Or Transform A Factory", "you factory config validate ./factory/factory.json", "you factory config flatten ./factory", "you factory config expand ./dist/factory.json", "## Minimum Factory Authoring Contract", "handlingBehavior", "invocationSignature", "invocationReturn", "supportingFiles", "you docs run", "you docs workers", "you docs workstations", "you docs resources"}, absent: []string{"you config validate", "you config flatten", "you config expand", "you factory save", "## Run Controls", "## Single-Work API Submission"}},
	{name: "work", heading: "# Submitted Work", markers: []string{"## Single-Work API Submission", "## Submission contract shapes", "SubmitWorkRequest", "WorkRequest", "`items` cannot be combined with `content` or `payload`", "POST /factory-sessions/{session_id}/work", "~default", "workTypeName", "## Tags And Prompt Templates", "Token.Tags", "`you docs config`", "`you docs batch-inputs`", "FACTORY_REQUEST_BATCH"}, absent: []string{"../internal/development/parent-aware-fan-in.md", "../internal/development/workstation-guards-and-guarded-loop-breakers.md", "## Work Types", "supportingFiles", "## Portability Resource Manifest", "[Config](config.md)", "[Batch Inputs](batch-inputs.md)", "POST /work`", "`POST /work/staged-files`"}},
	{name: "sessions", heading: "# Sessions and Runtime", markers: []string{"you session list", "you session pause", "you session resume", "POST /factory-sessions/{session_id}/pause", "SESSION_LIFECYCLE_CONTROL", "make docs-reference-smoke", "you factory query", "GET /factory-sessions/{session_id}/status", "factoryState", "runtimeStatus", "categories", "INVOCATION_BLOCKED", "INVOCATION_NEEDS_HUMAN", "INVOCATION_PAUSED", "INVOCATION_INTERRUPTED", "INVOCATION_RUNTIME_FAILURE", "INVOCATION_TIMED_OUT", "INVOCATION_CANCELED", "http://localhost:7437/dashboard/ui", "`you docs agents`", "`you docs work`", "`you docs config`", "`you docs javascript-workflows`", "## Response-event stream lifecycle and reconnect", "GET /factory-sessions/{session_id}/response-events", "FactoryResponseEvent", "after_sequence", "STREAM_GAP", "RESPONSE_EVENT_SESSION_NOT_FOUND", "RESPONSE_EVENT_STREAM_EXPIRED", "GET /factory-sessions/{session_id}/events", "canonical Factory events", "durable process-restart replay", "`you docs run`"}, absent: []string{"[Agents](agents.md)", "[Work](work.md)", "[Config](config.md)", "you docs packaged-goal"}},
	{name: "orchestrators", heading: "# Orchestrators and Factory Sessions", markers: []string{"### Beta JavaScript child-agent contract", "`prompt`", "`label`", "`preset`", "`modelProvider`", "`model`", "`reasoningEffort`", "agent-run-valid", "agent-run-invalid", "`agent.run() does not support field \"writableRoots\"`", "dynamically constructed argument object", "`you docs javascript-workflows`"}},
	{name: "javascript-workflows", heading: "# JavaScript Workflows", markers: []string{"## Operator flow", "you workflow validate --kind INLINE_WORKFLOW", "you workflow validate --kind WORKFLOW_NAME --value release-train --dir .", "you --json workflow run", "--request-id req-js-timeout-001", "--workflow long-running-audit", "--wait-timeout-millis 1000", "you --json workflow start", "--request-id req-js-run-n-001", `--args '{"release":"2026.06"}'`, "you --json workflow status SESSION_ID", "you --json workflow result SESSION_ID --mode final", "you --json workflow dispatches SESSION_ID", "you --json workflow artifacts SESSION_ID", "you --json workflow events SESSION_ID", "--execution-provider javascript-runtime --project-root .", "POST /factory-sessions/sync", "you.factory_session.start_sync", "FactoryArtifact", "FactoryEvent", "## Stable failures and recovery"}, absent: []string{"you run --workflow", "command?, sandbox?", "writableRoots?", "allowNetwork?", "outputSchema?"}},
	{name: "mock-workers", heading: "# Mock Workers", markers: []string{"--with-mock-workers", "mockWorkers", "unmatchedDispatchPolicy", "passthrough", "runType", "accept", "reject", "script", "scriptConfig", "docs/examples/mock-workers.json", "docs/examples/mock-workers-script.json", "docs/examples/mock-workers-mixed.json", "docs/examples/startup-work.json", "## Reviewer Verification", "Do not rely on a live real-agent passthrough run for signoff", "automated service and runner tests"}},
	{name: "record-replay", heading: "# Record and Replay", markers: []string{"--record", "--replay", "--no-record", "~/.you-agent-factory/recordings/", "docs/examples/sample-run.replay.json", "Recording saved:", "you run --factory ./workflow.js", "you run --record ./recordings/workflow-run.json --factory ./workflow.js", "you run --replay ./recordings/workflow-run.json --factory ./workflow.js", "you workflow status <session-id>", "you workflow events <session-id>", "you workflow artifacts <session-id>", "you workflow result <session-id> --mode final", "replayCompatibilityVersion", "raw JavaScript runtime state", "provider transcripts", "child-dispatch lists", "does not invoke a provider, dispatch a child, or execute the JavaScript source", "`--record` with `--replay`", "`--no-record` with `--record`"}},
	{name: "guards", heading: "# Guards", markers: []string{"VISIT_COUNT", "SAME_NAME", "MATCHES_FIELDS", "ALL_CHILDREN_COMPLETE", "ANY_CHILD_FAILED", "INFERENCE_THROTTLE_GUARD", "LOGICAL_MOVE", "limits.maxRetries"}},
	{name: "relationships", heading: "# Relationships", markers: []string{"DEPENDS_ON", "PARENT_CHILD", "SPAWNED_BY", "requiredState", "sourceWorkName", "targetWorkName", "workTypeName", "requestId", "FACTORY_REQUEST_BATCH", "`you docs guards`", "`you docs batch-inputs`"}, absent: []string{"work_type_name", "source_work_name", "target_work_name", "[Guards](guards.md)", "[Batch Inputs](batch-inputs.md)"}},
	{name: "workstations", heading: "# Workstations Reference", markers: []string{"workstation authoring contract", "INFERENCE_RUN", "AGENT_RUN", "MODEL_WORKSTATION", "MODEL_INVOKE", "CLASSIFIER_WORKSTATION", "LOGICAL_MOVE", "`you docs guards`", "`you docs relationships`"}, absent: []string{"../internal/development/parent-aware-fan-in.md", "../internal/development/workstation-guards-and-guarded-loop-breakers.md", "[Guards](guards.md)", "[Relationships](relationships.md)"}},
	{name: "workstation", heading: "# Workstations Reference", markers: []string{"workstation authoring contract", "INFERENCE_RUN", "AGENT_RUN", "MODEL_WORKSTATION", "CLASSIFIER_WORKSTATION", "LOGICAL_MOVE"}, absent: []string{"docs/reference/workstations-and-workers.md"}},
	{name: "workers", heading: "# Workers", markers: []string{"INFERENCE_WORKER", "AGENT_WORKER", "POLLER_WORKER", "MODEL_WORKER", "SCRIPT_WORKER", "HOSTED_WORKER", "auth.secretRef", "secrets/linear-api-key", "INFINITE_YOU_SECRET_SECRETS_LINEAR_API_KEY", "linear.teamIds", "linear.mapping.workType", "modelProvider", "YOU_DEFAULT_WORKER_MODEL_PROVIDER", "--default-worker-model-provider", "Authored worker `modelProvider` and `model` values always win", "## Response-stream provider fidelity", "Provider Session", "Factory Session", "FactoryResponseEvent", "Native streaming", "Final-only", "STREAM_GAP", "primaryResult", "byte-identical provider transcripts", "durable process-restart replay", "`you docs run`", "`you docs sessions`", "docs/reference/workers.md"}, absent: []string{"docs/reference/workstations-and-workers.md"}},
	{name: "resources", heading: "# Resources", markers: []string{"capacity", "workstations", "agent-slot", "docs/reference/resources.md"}},
	{name: "batch-inputs", heading: "# Batch Inputs", markers: []string{"## Batch ingress comparison", "`WorkRequest`", "works[].content", "`you submit batch`", "`you submit`", "`you run --work <path>`", "## CLI batch submit (`you submit batch`)", "you submit batch --dry-run", "you submit batch --file", "cat batch.json | you submit batch", "## Quick reference", "## Before you submit", "factory.json", "factory/docs/overview.md", "factory/docs/README.md", "FACTORY_REQUEST_BATCH", "DEPENDS_ON", "PARENT_CHILD", "factory/inputs/BATCH/default/<request_id>.json", "requestId", "workTypeName", "sourceWorkName", "targetWorkName", "requiredState", "## Visualize batch dependencies (`you work visualize`)", "you work visualize batch.json > my-graph.mermaid", "you work visualize --format markdown-mermaid batch.json > graph.md", "Graph nodes represent work items", "It does not submit", "render diagram images", "`you docs agents`", "`you docs relationships`", "`you docs guards`"}, absent: []string{"../internal/development/parent-aware-fan-in.md", "work_type_name", "source_work_name", "# Batch Work", "batch-work.md", "[Agents](agents.md)", "[Relationships](relationships.md)", "[Guards](guards.md)"}},
	{name: "batch-work", heading: "# Batch Inputs", markers: []string{"## Quick reference", "## Before you submit", "factory.json", "factory/docs/overview.md", "`you docs batch-work` is a compatibility alias", "FACTORY_REQUEST_BATCH", "DEPENDS_ON", "PARENT_CHILD", "factory/inputs/BATCH/default/<request_id>.json", "requestId", "workTypeName", "sourceWorkName", "targetWorkName"}, absent: []string{"# Batch Work", "docs/reference/batch-work.md", "batch-work.md", "work_type_name", "source_work_name"}},
	{name: "templates", heading: "# Templates", markers: []string{".Context.Project", ".Context.WorkDir", "docs/reference/templates.md", "text/template", "you docs guards", "you docs relationships"}, absent: []string{"docs/reference/prompt-variables.md"}},
	{name: "models", heading: "# Models", markers: []string{"you models list", "you models inspect OMNIVOICE_Q4_K_M", "you models pull OMNIVOICE_Q4_K_M", "readinessState", "lifecycleState", "you models invoke OMNIVOICE_Q4_K_M --operation TTS --text", "--output ./speech.wav", "you --json models invoke", "INFERENCE_RUN", "INFERENCE_WORKER", "WorkContent", "`you docs workers`", "`you docs workstations`"}, absent: []string{"docs/reference/workstations-and-workers.md", "## Maintainer Long-Test Expectations"}},
	{name: "mcp", heading: "# MCP Host Setup", markers: []string{"you mcp serve", "mcpServers", `"args": ["mcp", "serve"]`, "you.factory_session.validate_source", "you.factory_session.start_async", "## Choose A Backing Mode", "## Run The First-Host Smoke", "## Know What Is Proven", "## Troubleshoot Setup And Calls", "fixture catalog not found", "factory_session.result.not_ready", "serve_smoke_test.go", "serve_runtime_smoke_test.go", "serve_runtime_resume_smoke_test.go", "serve_runtime_resume_non_regression_test.go", "you mcp serve --runtime", "Fixture-backed (default)", "Runtime-backed", "`you docs orchestrators`"}, absent: []string{"you docs mcp-hosts", "[Orchestrators](orchestrators.md)", "Follow-Up Cell For Async Install Smoke", "follow-up-cell-mcp-session-serve.md", "HTTP and SSE MCP transports are supported"}},
}

var retiredDocsInvocationPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(^|[^[:alnum:]-])infinite-you docs([^[:alnum:]-]|$)`),
	regexp.MustCompile("(^|[^[:alnum:]-])agent-factory run([^[:alnum:]-]|$)"),
	regexp.MustCompile("(^|[^[:alnum:]-])agent-factory config([^[:alnum:]-]|$)"),
}

func TestDocsCommandSmoke_AuthoringFactoriesDescribesMinimalGoalRepeater(t *testing.T) {
	output := executeDocsSmokeCommand(t, t.TempDir(), "docs", "authoring-factories")
	for _, want := range []string{
		"### Built-in `@you/goal` repeater",
		"goal:init",
		"goal:execute",
		"goal:complete",
		"goal:failed",
		"REPEATER",
		"Continue and reject outcomes route back to `goal:init`",
		"invocationReturn",
		"primaryResult",
		"workers/goal-executor/AGENTS.md",
		"workstations/execute-goal/AGENTS.md",
		"INVOCATION_RUNTIME_FAILURE",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("you docs authoring-factories missing minimal-goal marker %q:\n%s", want, output)
		}
	}
	for _, stale := range []string{
		"plan-goal",
		"check-goal",
		"review-goal",
		"structured-review-goal",
		"goal:plan",
		"goal:review",
		"goal:structured-review",
	} {
		if strings.Contains(output, stale) {
			t.Fatalf("you docs authoring-factories contains stale goal topology %q:\n%s", stale, output)
		}
	}
}

func TestDocsCommandSmoke_PackagedTopicsRemainAvailableOutsideRepositoryDocsTree(t *testing.T) {
	workingDir := t.TempDir()
	missingDocsTree := filepath.Join(workingDir, "docs")
	if _, err := os.Stat(missingDocsTree); !os.IsNotExist(err) {
		t.Fatalf("temp working dir unexpectedly contains docs tree %q", missingDocsTree)
	}

	index := executeDocsSmokeCommand(t, workingDir, "docs")
	for _, want := range []string{"# Docs", "`agents` - Agent orientation: read order, work submission, command matrix, planner vs executor, and topic router", "`authoring-factories` - Practical factory authoring workflow", "`run` - Supported local, one-shot, batch, continuous, and mock-worker run shapes", "`config` - Operator initialization and Factory validation, flattening, expansion, and minimum authoring contract", "`mock-workers` - Mock-worker runs", "`record-replay` - Record and replay run modes", "`guards` - Workstation, input, and factory guards", "`relationships` - Batch DEPENDS_ON", "`work` - Submitted work: session-scoped work routes, tags, batch cross-links, and submission contracts", "`sessions` - Live factory sessions: session list, session show, pause and resume, factory query, status API, dashboard URL, and run modes", "`batch-inputs` - Batch input files", "`workstations` - Workstation kinds", "`you docs agents`", "`you docs authoring-factories`", "`you docs run`", "`you docs config`", "`you docs mock-workers`", "`you docs record-replay`", "`you docs guards`", "`you docs relationships`", "`you docs work`", "`you docs sessions`", "`you docs batch-inputs`", "`you docs workstations`"} {
		if !strings.Contains(index, want) {
			t.Fatalf("docs index missing %q:\n%s", want, index)
		}
	}
	for _, alias := range []string{"`batch-work`", "`workstation`"} {
		if strings.Contains(index, alias) {
			t.Fatalf("docs index should list canonical topics without %s alias noise:\n%s", alias, index)
		}
	}

	var unsupportedStdout bytes.Buffer
	runInWorkingDirectory(t, workingDir, func() {
		root := agentcli.NewRootCommand()
		root.SetOut(&unsupportedStdout)
		root.SetErr(io.Discard)
		root.SetArgs([]string{"docs", "unknown"})

		err := root.Execute()
		if err == nil {
			t.Fatal("expected unsupported docs topic to fail")
		}
		if got := err.Error(); got != `unsupported docs topic "unknown" (supported: agents, authoring-factories, run, config, mock-workers, record-replay, guards, relationships, work, sessions, orchestrators, javascript-workflows, mcp, workstations, workers, resources, models, batch-inputs, templates)` {
			t.Fatalf("unexpected unsupported topic error %q", got)
		}
	})
	if got := unsupportedStdout.String(); got != "" {
		t.Fatalf("unsupported docs topic should not write stdout, got %q", got)
	}

	for _, retiredTopic := range []string{"packaged-fusion", "packaged-goal", "packaged-tts", "mcp-hosts"} {
		var retiredStdout bytes.Buffer
		runInWorkingDirectory(t, workingDir, func() {
			root := agentcli.NewRootCommand()
			root.SetOut(&retiredStdout)
			root.SetErr(io.Discard)
			root.SetArgs([]string{"docs", retiredTopic})

			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), `unsupported docs topic "`+retiredTopic+`"`) {
				t.Fatalf("execute retired docs topic %s error = %v, want unsupported-topic error", retiredTopic, err)
			}
		})
		if got := retiredStdout.String(); got != "" {
			t.Fatalf("retired docs topic %s wrote stdout %q", retiredTopic, got)
		}
	}

	for _, topic := range docsSmokeTopics {
		topic := topic
		t.Run(topic.name, func(t *testing.T) {
			output := executeDocsSmokeCommand(t, workingDir, "docs", topic.name)
			if !strings.Contains(output, topic.heading) {
				t.Fatalf("you-agent-factory docs %s missing heading %q", topic.name, topic.heading)
			}
			for _, marker := range topic.markers {
				if !strings.Contains(output, marker) {
					t.Fatalf("you-agent-factory docs %s missing marker %q", topic.name, marker)
				}
			}
			for _, unwanted := range topic.absent {
				if strings.Contains(output, unwanted) {
					t.Fatalf("you-agent-factory docs %s still references retired path %q:\n%s", topic.name, unwanted, output)
				}
			}
			for _, retiredPath := range retiredDuplicateTreePaths {
				if strings.Contains(output, retiredPath) {
					t.Fatalf("you-agent-factory docs %s still references removed duplicate-tree path %q:\n%s", topic.name, retiredPath, output)
				}
			}
			for _, oldInvocation := range retiredDocsInvocationPatterns {
				if oldInvocation.FindStringIndex(output) != nil {
					t.Fatalf("you-agent-factory docs %s still contains old executable invocation %q:\n%s", topic.name, oldInvocation.String(), output)
				}
			}
		})
	}
}

func executeDocsSmokeCommand(t *testing.T, workingDir string, args ...string) string {
	t.Helper()

	var out bytes.Buffer
	runInWorkingDirectory(t, workingDir, func() {
		root := agentcli.NewRootCommand()
		root.SetOut(&out)
		root.SetErr(io.Discard)
		root.SetArgs(args)

		if err := root.Execute(); err != nil {
			t.Fatalf("execute root command %v: %v", args, err)
		}
	})
	return out.String()
}

func runInWorkingDirectory(t *testing.T, dir string, fn func()) {
	t.Helper()

	support.WithWorkingDirectory(t, dir, fn)
}
