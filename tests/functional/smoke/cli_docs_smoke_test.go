package smoke

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
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
	{name: "agents", heading: "# Agents", markers: []string{"bare `you` for generated command discovery", "Bare `you` prints help successfully", "## Read order", "factory/docs/overview.md", "factory/docs/README.md", "## CLI-only ingress", "Autonomous agents must submit work only through the CLI", "`you submit batch`", "## Batch submit for agents", "### Idempotency and duplicate work", "requestId", "duplicate batches", "you submit batch ./batches/release-story-set.json", "## Is the factory running?", "you session list", "you server", "you run --continuously --with-server", "you factory show", "you docs sessions", "## Operator loop", "you work list --name", "## Command matrix", "side-effect-free discovery", "Operator-only", "## Planner vs executor", "## Topic router", "`you docs config`", "`you docs templates`", "`you docs resources`", "`you docs models`", "`you docs batch-inputs`", "FACTORY_REQUEST_BATCH", "POST /factory-sessions/{session_id}/work", "## Factory-local docs discovery"}, absent: []string{"start the factory with `you`", "## Start Here", "## Read Order (Any Factory)", "## Submitting Work", "### Batch submit for agents", "Run `you docs agents`", "[Config](config.md)", "[Work](work.md)", "[Batch Inputs](batch-inputs.md)", "[Relationships](relationships.md)", "[Authoring Factories](authoring-factories.md)", "factoryState", "runtimeStatus", "dashboard/ui", "thoughts:init", "idea:init", "plan:init", "task:in-review", "Work enters a running factory through one of these ingress paths", "| Ingress | When to use |", "| Watched `factory/inputs/**` JSON files |", "| `POST /work` | Single submitted work item", "Place batch files under the inbox paths", "## Related Topics"}},
	{name: "authoring-factories", heading: "# Authoring Factories", markers: []string{"factory.json", "workers/<name>/AGENTS.md", "workstations/<name>/AGENTS.md", "you docs factory-validation", "### 3. Validate before the first run", "you factory config validate ./factory.json", "you factory config validate ./factory", "Factory validation passed.", "Runtime taxonomy:", "Run the gate again after any topology", "you run --factory ./factory.json \"Fix the lint issues\"", "handlingBehavior: [\"DEFAULT\"]", "you run --dir ./factory --with-mock-workers --work ./docs/examples/startup-work.json", "you docs mock-workers", "you docs record-replay", "you docs run", "you docs models", "you factory create my-team-review --from ./factory.json", "you factory update my-team-review --from ./factory.json", "`create` refuses to overwrite an existing name", "you run --named @you/goal", "you run --named @you/tts --output primary \"Read the release summary.\"", "`you docs agents`", "--no-record", "requestId", "workTypeName", "~/.you-agent-factory/factories", "you factory list --dir ./alternate-factories", "~/.you-agent-factory/factories/@you/review", "you factory config validate ~/.you-agent-factory/factories/@you/review", "you factory create <factory-name> --from <staged-candidate> --dir ~/.you-agent-factory/factories", "is not read or migrated during startup", "Use `you docs factory-validation` for the complete static-check list"}, absent: []string{"work_type_name", "source_work_name", `"request_id"`, "[Agents](agents.md)", "you docs packaged-fusion", "you docs packaged-goal", "you docs packaged-tts", "~/.you-agent-factory/you-agent-factories/@you/review", "you factory config validate ~/.you-agent-factory/you-agent-factories/@you/review", "you config init"}},
	{name: "packaged-factories", heading: "# Packaged Factories", markers: []string{"## Discovery and first use", "you factory list", "you run --named <factory> --help", "operator defaults", "seventeen", "## Common entry contract", "model-generated prose is not byte-stable", "## Catalog", "@you/subagent", "@you/agy-clip-qa", "@you/agy-cold-watch", "--shot-specification", "--cut-path", "AGY production review composition example", "TestAgyProductionReviewRolesThroughRootBuildProcess", "Do not enable that live smoke in ordinary CI"}, absent: []string{"you docs packaged-goal", "you docs packaged-tts"}},
	{name: "run", heading: "# Run", markers: []string{
		"you run",
		"Factory",
		"Factory Session",
		"you docs config",
		"you docs sessions",
		"you docs providers",
		"### Primary-result mode",
		"### Human Factory Event stream mode (default)",
		"### NDJSON automation mode",
		"--output response-stream",
		"recordType=factory_event",
		"recordType=invocation_result",
		"invocationReturn",
		"<invocation-working-directory>/factory/factory.json",
		"## Server and site lifecycles",
		"--with-server",
		"--with-site",
		"`you server`",
		"## Capture local runtime memory diagnostics",
		"you server --pprof --listen 127.0.0.1:7437",
		"GET /debug/runtime",
		"processCommitBytes",
		"curl -sS http://127.0.0.1:7437/debug/pprof/heap -o heap.pprof",
		"go tool pprof -top http://127.0.0.1:7437/debug/pprof/heap",
		"RSS or working set as the process-commit signal",
		"SERVER_BIND_FAILED",
		"CURRENT_FACTORY_NOT_FOUND",
		"CURRENT_FACTORY_INVALID",
		"INVOCATION_OUTPUT_CONFLICT",
		"INVOCATION_OUTPUT_UNSUPPORTED",
		"RUN_INVOCATION_FAILED",
		"exits `130`",
	}, absent: []string{
		"recordType=progress",
		"recordType=compaction",
		"recordType=primary_result",
		"PROGRESS_FRAGMENT",
		"STREAM_COMPACTION_SIGNAL",
		"bootstraps ./factory",
	}},
	{name: "config", heading: "# Config", markers: []string{"## Configure Provider And Model Defaults", "you init --provider codex", "~/.you-agent-factory/config.json", "~/.you-agent-factory/factories", "YOU_DEFAULT_WORKER_MODEL_PROVIDER", "YOU_DEFAULT_WORKER_MODEL", "file < env < run flag", "## Validate Or Transform A Factory", "you factory config validate ./factory/factory.json", "you factory config flatten ./factory", "you factory config expand ./dist/factory.json", "## Minimum Factory Authoring Contract", "handlingBehavior", "invocationSignature", "invocationReturn", "supportingFiles", "you docs run", "you docs workers", "you docs workstations", "you docs resources"}, absent: []string{"you config init", "you config validate", "you config flatten", "you config expand", "you factory save", "## Run Controls", "## Single-Work API Submission"}},
	{name: "work", heading: "# Submitted Work", markers: []string{"## Single-Work API Submission", "## Submission contract shapes", "SubmitWorkRequest", "WorkRequest", "`items` cannot be combined with `content` or `payload`", "POST /factory-sessions/{session_id}/work", "~default", "workTypeName", "you docs operations", "## Watch Work state transitions", "you work watch", "you.work.watch.v1", "sequence", "--follow", "Ctrl-C", "last accepted event ID and sequence", "does not persist a cursor", "jq -c 'select(.terminal)'", "ConvertFrom-Json", "## Tags And Prompt Templates", "Token.Tags", "`you docs config`", "`you docs batch-inputs`", "FACTORY_REQUEST_BATCH"}, absent: []string{"../internal/development/parent-aware-fan-in.md", "../internal/development/workstation-guards-and-guarded-loop-breakers.md", "## Work Types", "supportingFiles", "## Portability Resource Manifest", "[Config](config.md)", "[Batch Inputs](batch-inputs.md)", "POST /work`", "`POST /work/staged-files`"}},
	{name: "sessions", heading: "# Sessions and Runtime", markers: []string{"you session list", "you session pause", "you session resume", "POST /factory-sessions/{session_id}/pause", "SESSION_LIFECYCLE_CONTROL", "make docs-reference-smoke", "you factory show", "GET /factory-sessions/{session_id}/status", "factoryState", "runtimeStatus", "categories", "INVOCATION_BLOCKED", "INVOCATION_NEEDS_HUMAN", "INVOCATION_PAUSED", "INVOCATION_INTERRUPTED", "INVOCATION_RUNTIME_FAILURE", "INVOCATION_TIMED_OUT", "INVOCATION_CANCELED", "http://localhost:7437/dashboard/ui", "`you docs agents`", "`you docs work`", "`you docs config`", "`you docs javascript-workflows`", "## Response-event stream lifecycle and reconnect", "GET /factory-sessions/{session_id}/response-events", "FactoryResponseEvent", "after_sequence", "STREAM_GAP", "RESPONSE_EVENT_SESSION_NOT_FOUND", "RESPONSE_EVENT_STREAM_EXPIRED", "GET /factory-sessions/{session_id}/events", "canonical Factory events", "durable process-restart replay", "`you docs run`"}, absent: []string{"[Agents](agents.md)", "[Work](work.md)", "[Config](config.md)", "you docs packaged-goal"}},
	{name: "orchestrators", heading: "# Orchestrators and Factory Sessions", markers: []string{"### Beta JavaScript child-agent contract", "`prompt`", "`label`", "`preset`", "`modelProvider`", "`model`", "`reasoningEffort`", "`permissions`", "agent-run-valid", "agent-run-invalid", "`agent.run() does not support field \"writableRoots\"`", "dynamically constructed argument object", "`you docs javascript-workflows`"}},
	{name: "javascript-workflows", heading: "# JavaScript Workflows", markers: []string{"## Operator flow", "POST http://localhost:7437/factories/preview", "POST http://localhost:7437/factory-sessions/sync", "POST http://localhost:7437/factory-sessions/async", "req-js-timeout-001", "long-running-audit", "waitTimeoutMillis", "req-js-run-n-001", "you session show SESSION_ID", "you metrics --session SESSION_ID --group-by worker", "GET /factory-sessions/SESSION_ID/dispatches", "you.factory_session.list_dispatches", "you.factory_session.start_sync", "FactoryArtifact", "FactoryEvent", "## Stable failures and recovery"}, absent: []string{"you workflow", "you.run --workflow", "command?, sandbox?", "writableRoots?", "allowNetwork?", "outputSchema?"}},
	{name: "mock-workers", heading: "# Mock Workers", markers: []string{"--with-mock-workers", "mockWorkers", "unmatchedDispatchPolicy", "passthrough", "runType", "accept", "reject", "script", "scriptConfig", "docs/examples/mock-workers.json", "docs/examples/mock-workers-script.json", "docs/examples/mock-workers-mixed.json", "docs/examples/startup-work.json", "## Reviewer Verification", "Do not rely on a live real-agent passthrough run for signoff", "automated service and runner tests"}},
	{name: "record-replay", heading: "# Record and Replay", markers: []string{"--record", "--replay", "--no-record", "~/.you-agent-factory/recordings/", "docs/examples/sample-run.replay.json", "Recording saved:", "you run --factory ./workflow.js", "you run --record ./recordings/workflow-run.json --factory ./workflow.js", "you run --replay ./recordings/workflow-run.json --factory ./workflow.js", "you session show <session-id>", "/factory-sessions/<session-id>/events", "/factory-sessions/<session-id>/artifacts", "/factory-sessions/<session-id>/results?mode=final", "replayCompatibilityVersion", "raw JavaScript runtime state", "provider transcripts", "child-dispatch lists", "does not invoke a provider, dispatch a child, or execute the JavaScript source", "`--record` with `--replay`", "`--no-record` with `--record`"}},
	{name: "guards", heading: "# Guards", markers: []string{"VISIT_COUNT", "SAME_NAME", "MATCHES_FIELDS", "ALL_CHILDREN_COMPLETE", "ANY_CHILD_FAILED", "INFERENCE_THROTTLE_GUARD", "LOGICAL_MOVE", "limits.maxRetries"}},
	{name: "relationships", heading: "# Relationships", markers: []string{"DEPENDS_ON", "PARENT_CHILD", "SPAWNED_BY", "requiredState", "sourceWorkName", "targetWorkName", "targetWorkId", "workTypeName", "requestId", "FACTORY_REQUEST_BATCH", "## Submitted relation endpoints", "Cross-batch targets and terminal outcomes", "cross-session", "standard dependency failure cascade", "you submit batch --dry-run", "`you docs guards`", "`you docs batch-inputs`"}, absent: []string{"work_type_name", "source_work_name", "target_work_name", "[Guards](guards.md)", "[Batch Inputs](batch-inputs.md)"}},
	{name: "operations", heading: "# Operations", markers: []string{"you run --dir ./factory --with-server --continuously", "Terminal Work", "Error: factory session drained with N non-terminal work items; run is incomplete", "operations-stranded-work.json", "in memory", "same-name resubmit", "six production recoveries", "inclusive threshold", "greater than or equal to", "guarded `LOGICAL_MOVE`", "queued Work does not", "priority inversion", "upstream-slot", "work watch --follow", "worker-sessions list", "worker-sessions show", "worker-sessions stream", "worker-sessions read", "No worker sessions found.", "WORKER_SESSION_NOT_FOUND", "dispatch starts", "prompt snapshot", "`you docs guards`", "`you docs resources`", "`you docs sessions`", "`you docs work`", "`you docs workers`"}},
	{name: "workstations", heading: "# Workstations Reference", markers: []string{"workstation authoring contract", "INFERENCE_RUN", "AGENT_RUN", "MODEL_WORKSTATION", "MODEL_INVOKE", "CLASSIFIER_WORKSTATION", "LOGICAL_MOVE", "`you docs operations`", "`you docs guards`", "`you docs relationships`"}, absent: []string{"../internal/development/parent-aware-fan-in.md", "../internal/development/workstation-guards-and-guarded-loop-breakers.md", "[Guards](guards.md)", "[Relationships](relationships.md)"}},
	{name: "workstation", heading: "# Workstations Reference", markers: []string{"workstation authoring contract", "INFERENCE_RUN", "AGENT_RUN", "MODEL_WORKSTATION", "CLASSIFIER_WORKSTATION", "LOGICAL_MOVE"}, absent: []string{"docs/reference/workstations-and-workers.md"}},
	{name: "workers", heading: "# Workers", markers: []string{"INFERENCE_WORKER", "AGENT_WORKER", "POLLER_WORKER", "MODEL_WORKER", "SCRIPT_WORKER", "HOSTED_WORKER", "auth.secretRef", "secrets/linear-api-key", "INFINITE_YOU_SECRET_SECRETS_LINEAR_API_KEY", "linear.teamIds", "linear.mapping.workType", "modelProvider", "YOU_DEFAULT_WORKER_MODEL_PROVIDER", "--provider", "Authored worker `modelProvider` and `model` values always win", "`you docs operations`", "## Response-stream provider fidelity", "Provider Session", "Factory Session", "FactoryResponseEvent", "Native streaming", "Final-only", "STREAM_GAP", "primaryResult", "byte-identical provider transcripts", "durable process-restart replay", "`you docs run`", "`you docs sessions`", "docs/reference/workers.md"}, absent: []string{"docs/reference/workstations-and-workers.md"}},
	{name: "providers", heading: "# Providers, worker models, and ACP agents", markers: []string{"you providers list", "you providers list --json", "availability`, `readiness`, and `prerequisites", "models` and `efforts", "Directional input/output support", "knownLimits", "gpt-5.6", "gpt-5.6-luna", "gpt-5.6-sol", "gpt-5.6-terra", "claude-sonnet-5", "modelProvider: ANTIGRAVITY", "antigravity", "claude-opus-4-6-thinking", "gemini-3.6-flash-high", "gpt-oss-120b-medium", "low", "medium", "high", "GPT-5.6 does not provide video or audio understanding", "referenced_image_paths", "0–5 paths per call", "file_path", "--add-dir", "executorProvider", "modelProvider", "cursor-acp", "kiro-acp", "opencode-acp", "you workers list", "you workers acp add", "you workers acp delete", "~/.you-agent-factory/config.json", "skipPermissions", "you factory config validate", "--provider cursor-acp", "@you/goal", "agent.run", "you docs javascript-workflows", "Factory validation checks configuration shape", "## Configure a Factory worker or one ad-hoc run", "reasoningEffort: high", "timeout: 45m", "--worker-reasoning-effort", "--to-file", "--add-dir <working-directory>", "--print-timeout", "print_timeout", "final `result` event", "status: SUCCESS", "public Factory and `you run` contract rejects any"}, absent: []string{"The separate AGY effort value may be omitted or set to", "print-mode command adapter accepts a separate `reasoningEffort`"}},
	{name: "acp", heading: "# Providers, worker models, and ACP agents", markers: []string{"you workers acp", "executorProvider", "cursor-acp"}},
	{name: "resources", heading: "# Resources", markers: []string{"capacity", "workstations", "agent-slot", "docs/reference/resources.md"}},
	{name: "batch-inputs", heading: "# Batch Inputs", markers: []string{"## Batch ingress comparison", "`WorkRequest`", "works[].content", "`you submit batch`", "`you submit`", "`you run --work <path>`", "## CLI batch submit (`you submit batch`)", "you submit batch --dry-run", "you submit batch --file", "cat batch.json | you submit batch", "## Quick reference", "## Relation endpoint scope", "unique across the entire `works[]` array", "different `workTypeName` values", "targetWorkId", "Cross-batch dependency outcomes", "rename or remove one entry", "## Before you submit", "factory.json", "factory/docs/overview.md", "factory/docs/README.md", "FACTORY_REQUEST_BATCH", "DEPENDS_ON", "PARENT_CHILD", "factory/inputs/BATCH/default/<request_id>.json", "requestId", "workTypeName", "sourceWorkName", "targetWorkName", "requiredState", "## Work payload size limit", "65,536 bytes", "compact UTF-8 JSON encoding", "payloadBytes=65537", "payloadLimitBytes=65536", "multibyte UTF-8", "staged-file attachments", "aggregate request-body", "Provider-backed", "uses stdin", "## Render batch dependencies (`you work render`)", "you work render batch.json > my-graph.mermaid", "you work render --format markdown-mermaid batch.json > graph.md", "Graph nodes represent work items", "It does not submit", "render diagram images", "`you docs agents`", "`you docs relationships`", "`you docs guards`"}, absent: []string{"../internal/development/parent-aware-fan-in.md", "work_type_name", "source_work_name", "# Batch Work", "batch-work.md", "[Agents](agents.md)", "[Relationships](relationships.md)", "[Guards](guards.md)"}},
	{name: "batch-work", heading: "# Batch Inputs", markers: []string{"## Quick reference", "## Relation endpoint scope", "unique across the entire `works[]` array", "targetWorkId", "Cross-batch dependency outcomes", "rename or remove one entry", "## Before you submit", "factory.json", "factory/docs/overview.md", "`you docs batch-work` is a compatibility alias", "FACTORY_REQUEST_BATCH", "DEPENDS_ON", "PARENT_CHILD", "factory/inputs/BATCH/default/<request_id>.json", "requestId", "workTypeName", "sourceWorkName", "targetWorkName"}, absent: []string{"# Batch Work", "docs/reference/batch-work.md", "batch-work.md", "work_type_name", "source_work_name"}},
	{name: "templates", heading: "# Templates", markers: []string{".Context.Project", ".Context.WorkDir", "docs/reference/templates.md", "text/template", "you docs guards", "you docs relationships"}, absent: []string{"docs/reference/prompt-variables.md"}},
	{name: "models", heading: "# Models", markers: []string{"you models list", "you models inspect llm", "you models inspect asr", "you models inspect tts", "you models inspect embed", "you models pull llm", "you --json models pull embed", "5.0 GB", "148 MB", "18.7 GB", "1.21 GB", "local Models composition", "INFINITE_YOU_OMNIVOICE_CACHE_DIR", "readinessState", "lifecycleState", "you models invoke tts --operation TTS", "you models invoke embed --input text=\"Find similar work\"", "### Generate an embedding", "parameters=json:", "you --json models invoke embed", "MODEL_OFFLINE_CACHE_UNAVAILABLE", "MODEL_BACKEND_FAILURE", "POST /models/invocations", "--output speech.wav", "you --json models invoke", "## Operation contracts", "--input audio=@meeting.wav", "--output transcript=meeting.txt", "--output segments=meeting.json", "application/json", "raw audio bytes", "--text", "unqualified `--output <path>`", "artifact references", "before download or backend activation", "INFERENCE_RUN", "INFERENCE_WORKER", "WorkContent", "`you docs providers`", "`you docs workers`", "`you docs workstations`"}, absent: []string{"docs/reference/workstations-and-workers.md", "## Maintainer Long-Test Expectations", "issue #2201", "The placement is therefore not proven intentional"}},
	{name: "mcp", heading: "# MCP Host Setup", markers: []string{"you server mcp", "mcpServers", `"args": ["server", "mcp"]`, "you.factory_session.validate_source", "you.factory_session.start_async", "## Choose A Backing Mode", "## Run The First-Host Smoke", "## Know What Is Proven", "## Troubleshoot Setup And Calls", "fixture catalog not found", "factory_session.result.not_ready", "serve_smoke_test.go", "serve_runtime_smoke_test.go", "serve_runtime_resume_smoke_test.go", "serve_runtime_resume_non_regression_test.go", "you server mcp --runtime", "Fixture-backed (default)", "Runtime-backed", "`you docs orchestrators`"}, absent: []string{"you docs mcp-hosts", "[Orchestrators](orchestrators.md)", "Follow-Up Cell For Async Install Smoke", "follow-up-cell-mcp-session-serve.md", "HTTP and SSE MCP transports are supported", "you mcp serve", "you serve acp"}},
	{name: "serve-acp", heading: "# Host You As An ACP Agent", markers: []string{"you server acp", `"args": ["server", "acp"]`, "root.BuildProcess", "## Shutdown Behavior", "Clean stdin EOF", "cancellation outcome", "## Configure A Minimal ACP Client", "## Exchange One ACP Prompt", "session/new", "session/prompt", "session/update", "stopReason", "## Distinguish This From Related ACP And MCP Surfaces", "you workers acp add", "you workers acp delete", "you server mcp", "you acp serve", "not a recognized command", "## Know What Is Proven", "root_serve_test.go", "serve_acp_prompt_test.go", "`you docs providers`", "`you docs mcp`", "`you docs sessions`"}, absent: []string{"you docs serve-acp-hosts", "Neither command is an alias for `you workers acp`", "you serve acp", "you mcp serve"}},
	{name: "metrics", heading: "# Metrics", markers: []string{"you session list --scope live", "SESSION_ID=", "jq -er '.sessions[0].id'", "ALL_METRICS=", "SCOPED_METRICS=", "scoped_totals_are_not_larger", "--group-by workstation", "--group-by worker", "--group-by provider", "you --json metrics", "GET /metrics?session_id=", "Input tokens", "Output tokens", "Completed dispatches", "Failures by reason", "p50", "p95", "milliseconds", "METRICS_SESSION_NOT_FOUND", "METRICS_SESSION_SCOPE_UNAVAILABLE", "dur-sess-*", "verified empty success", "Unavailable (provider attribution not proven)", "${...}", "all_factory_sessions", "p50: null", "`you docs sessions`", "`you docs record-replay`"}},
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
		"goal:complete",
		"goal:blocked",
		"goal:failed",
		"REPEATER",
		"`needs_changes` routes back to `goal:init`",
		"decision envelope",
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
		"goal:execute",
	} {
		if strings.Contains(output, stale) {
			t.Fatalf("you docs authoring-factories contains stale goal topology %q:\n%s", stale, output)
		}
	}
}

func TestDocsCommandSmoke_AuthoringFactoriesDescribesFactoryBuilder(t *testing.T) {
	output := executeDocsSmokeCommand(t, t.TempDir(), "docs", "authoring-factories")
	for _, want := range []string{
		"The seventeen first-party packaged Factories",
		"`@you/factory-builder`",
		"### Built-in `@you/factory-builder` validated creation",
		"--factory-name release-note-review",
		"--orchestrator javascript",
		"you docs agents",
		"you docs config",
		"you docs javascript-workflows",
		"you factory config validate <staged-candidate>",
		"Validation is required before persistence",
		"you factory create <factory-name> --from <staged-candidate>",
		"does not install the candidate, and does not start it",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("you docs authoring-factories missing Factory Builder marker %q:\n%s", want, output)
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
	for _, want := range []string{"# Docs", "`agents` - Agent orientation: read order, work submission, command matrix, planner vs executor, and topic router", "`authoring-factories` - Practical factory authoring workflow", "`run` - Supported local, one-shot, batch, continuous, and mock-worker run shapes", "`config` - Operator initialization and Factory validation, flattening, expansion, and minimum authoring contract", "`factory-validation` - Pre-run static Factory validation gate", "`mock-workers` - Mock-worker runs", "`record-replay` - Record and replay run modes", "`guards` - Workstation, input, and factory guards", "`relationships` - Batch DEPENDS_ON", "`operations` - Real-pipeline lifetime, finite-drain classification, and same-name restart recovery", "`work` - Submitted work: session-scoped work routes, tags, batch cross-links, and submission contracts", "`sessions` - Live factory sessions: session list, session show, pause and resume, factory show, status API, dashboard URL, and run modes", "`metrics` - Factory Runtime token, dispatch, failure, and latency metrics with deterministic grouping and Factory Session scope", "`providers` - Worker/provider selection, model capabilities and limits, Factory configuration, AGY caveats, ACP lifecycle, and JavaScript usage", "`batch-inputs` - Batch input files", "`workstations` - Workstation kinds", "`you docs agents`", "`you docs authoring-factories`", "`you docs run`", "`you docs config`", "`you docs factory-validation`", "`you docs mock-workers`", "`you docs record-replay`", "`you docs guards`", "`you docs relationships`", "`you docs operations`", "`you docs work`", "`you docs sessions`", "`you docs metrics`", "`you docs providers`", "`you docs batch-inputs`", "`you docs workstations`"} {
		if !strings.Contains(index, want) {
			t.Fatalf("docs index missing %q:\n%s", want, index)
		}
	}
	if !strings.Contains(index, "`packaged-factories` - First-party @you/* Factory catalog, live invocation discovery, and operator guide") {
		t.Fatalf("docs index missing packaged-factories topic: %s", index)
	}
	for _, alias := range []string{"`batch-work`", "`workstation`", "`acp`"} {
		if strings.Contains(index, alias) {
			t.Fatalf("docs index should list canonical topics without %s alias noise:\n%s", alias, index)
		}
	}

	unsupportedStdout, err := executeDocsSmokeCommandResult(t, workingDir, "docs", "unknown")
	if err == nil {
		t.Fatal("expected unsupported docs topic to fail")
	}
	if got := err.Error(); !strings.Contains(got, `unsupported docs topic "unknown"`) {
		t.Fatalf("unexpected unsupported topic error %q", got)
	}
	if got := unsupportedStdout; got != "" {
		t.Fatalf("unsupported docs topic should not write stdout, got %q", got)
	}

	for _, retiredTopic := range []string{"packaged-fusion", "packaged-goal", "packaged-tts", "mcp-hosts"} {
		retiredStdout, err := executeDocsSmokeCommandResult(t, workingDir, "docs", retiredTopic)
		if err == nil || !strings.Contains(err.Error(), `unsupported docs topic "`+retiredTopic+`"`) {
			t.Fatalf("execute retired docs topic %s error = %v, want unsupported-topic error", retiredTopic, err)
		}
		if got := retiredStdout; got != "" {
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

	output, err := executeDocsSmokeCommandResult(t, workingDir, args...)
	if err != nil {
		t.Fatalf("execute root command %v: %v", args, err)
	}
	return output
}

func executeDocsSmokeCommandResult(
	t *testing.T,
	workingDir string,
	args ...string,
) (string, error) {
	t.Helper()

	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(
		context.Background(),
		append([]string{"you"}, args...),
	)
	inputs.WorkingDirectory = workingDir
	err := process.Execute(inputs.Input)
	return inputs.Stdout(), err
}
