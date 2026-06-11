package smoke

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	agentcli "github.com/portpowered/infinite-you/pkg/cli"
	"github.com/portpowered/infinite-you/pkg/testutil"
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
	"pkg/cli/docs/reference/",
	"pkg/cli/docs/reference",
}

var docsSmokeTopics = []docsSmokeTopic{
	{name: "agents", heading: "# Agents", markers: []string{"## Read order", "factory/docs/overview.md", "factory/docs/README.md", "## CLI-only ingress", "Autonomous agents must submit work only through the CLI", "`you submit batch`", "## Batch submit for agents", "### Idempotency and duplicate work", "requestId", "duplicate batches", "you submit batch ./batches/release-story-set.json", "## Is the factory running?", "you session list", "you factory query", "you docs sessions", "## Operator loop", "you work list --name", "## Command matrix", "Operator-only", "## Planner vs executor", "## Topic router", "`you docs config`", "`you docs templates`", "`you docs resources`", "`you docs models`", "`you docs batch-inputs`", "FACTORY_REQUEST_BATCH", "POST /factory-sessions/{session_id}/work", "## Factory-local docs discovery"}, absent: []string{"## Start Here", "## Read Order (Any Factory)", "## Submitting Work", "### Batch submit for agents", "Run `you docs agents`", "[Config](config.md)", "[Work](work.md)", "[Batch Inputs](batch-inputs.md)", "[Relationships](relationships.md)", "[Authoring Factories](authoring-factories.md)", "factoryState", "runtimeStatus", "dashboard/ui", "thoughts:init", "idea:init", "plan:init", "task:in-review", "Work enters a running factory through one of these ingress paths", "| Ingress | When to use |", "| Watched `factory/inputs/**` JSON files |", "| `POST /work` | Single submitted work item", "Place batch files under the inbox paths", "## Related Topics"}},
	{name: "authoring-factories", heading: "# Authoring Factories", markers: []string{"factory.json", "workers/<name>/AGENTS.md", "workstations/<name>/AGENTS.md", "you run --factory ./factory.json \"Fix the lint issues\"", "handlingBehavior: [\"DEFAULT\"]", "you run --dir ./factory --with-mock-workers", "you docs mock-workers", "you docs record-replay", "you docs packaged-tts", "`you docs agents`", "--no-record", "requestId", "workTypeName"}, absent: []string{"work_type_name", "source_work_name", `"request_id"`, "[Agents](agents.md)"}},
	{name: "packaged-tts", heading: "# Packaged TTS (`@you/tts`)", markers: []string{"you run --named @you/tts", "INFERENCE_WORKER", "INFERENCE_RUN", "~/.you-agent-factory/factories", "@you%2Ftts", "artifactPath", "mediaType", "backend", "editable", "raw audio", "shared invocation contract", "INVOCATION_TTS_MODEL_NOT_READY", "INVOCATION_TTS_GENERATION_FAILED", "`you docs authoring-factories`", "`you docs config`", "`you docs sessions`"}, absent: []string{"[Authoring Factories](authoring-factories.md)", "[Config](config.md)", "[Sessions](sessions.md)"}},
	{name: "config", heading: "# Config", markers: []string{"factory.json", "work types, states, workers, workstations, resources, and routing", "## Work Types", "handlingBehavior: [\"DEFAULT\"]", "## Top-Level Fields", "supportingFiles", "## Portability Resource Manifest", "## Resources", "## How The Pieces Fit", "you docs agents", "you docs work", "you docs mock-workers", "you docs record-replay", "you docs guards", "you docs relationships", "you docs authoring-factories", "--factory", "you run --factory ./factory.json \"Fix the lint issues\"", "--with-mock-workers", "--record", "--replay", "--no-record", "you-agent-factory run"}, absent: []string{"Agent Factory run", "## Single-Work API Submission"}},
	{name: "work", heading: "# Submitted Work", markers: []string{"## Single-Work API Submission", "## Submission contract shapes", "SubmitWorkRequest", "WorkRequest", "`items` cannot be combined with `content` or `payload`", "POST /factory-sessions/{session_id}/work", "~default", "workTypeName", "## Tags And Prompt Templates", "Token.Tags", "`you docs config`", "`you docs batch-inputs`", "FACTORY_REQUEST_BATCH"}, absent: []string{"../internal/development/parent-aware-fan-in.md", "../internal/development/workstation-guards-and-guarded-loop-breakers.md", "## Work Types", "supportingFiles", "## Portability Resource Manifest", "[Config](config.md)", "[Batch Inputs](batch-inputs.md)", "POST /work`", "`POST /work/staged-files`"}},
	{name: "sessions", heading: "# Sessions and Runtime", markers: []string{"you session list", "you factory query", "GET /factory-sessions/{session_id}/status", "factoryState", "runtimeStatus", "categories", "http://localhost:7437/dashboard/ui", "`you docs agents`", "`you docs work`", "`you docs config`"}, absent: []string{"[Agents](agents.md)", "[Work](work.md)", "[Config](config.md)"}},
	{name: "mock-workers", heading: "# Mock Workers", markers: []string{"--with-mock-workers", "mockWorkers", "unmatchedDispatchPolicy", "passthrough", "runType", "accept", "reject", "script", "scriptConfig", "docs/examples/mock-workers.json", "docs/examples/mock-workers-script.json", "docs/examples/mock-workers-mixed.json", "docs/examples/startup-work.json", "## Reviewer Verification", "Do not rely on a live real-agent passthrough run for signoff", "automated service and runner tests"}},
	{name: "record-replay", heading: "# Record and Replay", markers: []string{"--record", "--replay", "--no-record", "~/.you-agent-factory/recordings/", "docs/examples/sample-run.replay.json", "Recording saved:", "`--record` with `--replay`", "`--no-record` with `--record`"}},
	{name: "guards", heading: "# Guards", markers: []string{"VISIT_COUNT", "SAME_NAME", "MATCHES_FIELDS", "ALL_CHILDREN_COMPLETE", "ANY_CHILD_FAILED", "INFERENCE_THROTTLE_GUARD", "LOGICAL_MOVE", "limits.maxRetries"}},
	{name: "relationships", heading: "# Relationships", markers: []string{"DEPENDS_ON", "PARENT_CHILD", "SPAWNED_BY", "requiredState", "sourceWorkName", "targetWorkName", "workTypeName", "requestId", "FACTORY_REQUEST_BATCH", "`you docs guards`", "`you docs batch-inputs`"}, absent: []string{"work_type_name", "source_work_name", "target_work_name", "[Guards](guards.md)", "[Batch Inputs](batch-inputs.md)"}},
	{name: "workstations", heading: "# Workstations Reference", markers: []string{"workstation authoring contract", "INFERENCE_RUN", "AGENT_RUN", "MODEL_WORKSTATION", "MODEL_INVOKE", "CLASSIFIER_WORKSTATION", "LOGICAL_MOVE", "`you docs guards`", "`you docs relationships`"}, absent: []string{"../internal/development/parent-aware-fan-in.md", "../internal/development/workstation-guards-and-guarded-loop-breakers.md", "[Guards](guards.md)", "[Relationships](relationships.md)"}},
	{name: "workstation", heading: "# Workstations Reference", markers: []string{"workstation authoring contract", "INFERENCE_RUN", "AGENT_RUN", "MODEL_WORKSTATION", "CLASSIFIER_WORKSTATION", "LOGICAL_MOVE"}, absent: []string{"docs/reference/workstations-and-workers.md"}},
	{name: "workers", heading: "# Workers", markers: []string{"INFERENCE_WORKER", "AGENT_WORKER", "POLLER_WORKER", "MODEL_WORKER", "SCRIPT_WORKER", "HOSTED_WORKER", "auth.secretRef", "secrets/linear-api-key", "INFINITE_YOU_SECRET_SECRETS_LINEAR_API_KEY", "linear.teamIds", "linear.mapping.workType", "modelProvider", "docs/reference/workers.md"}, absent: []string{"docs/reference/workstations-and-workers.md"}},
	{name: "resources", heading: "# Resources", markers: []string{"capacity", "workstations", "agent-slot", "docs/reference/resources.md"}},
	{name: "batch-inputs", heading: "# Batch Inputs", markers: []string{"## Batch ingress comparison", "`WorkRequest`", "works[].content", "`you submit batch`", "`you submit`", "`you run --work <path>`", "## CLI batch submit (`you submit batch`)", "you submit batch --dry-run", "you submit batch --file", "cat batch.json | you submit batch", "## Quick reference", "## Before you submit", "factory.json", "factory/docs/overview.md", "factory/docs/README.md", "FACTORY_REQUEST_BATCH", "DEPENDS_ON", "PARENT_CHILD", "factory/inputs/BATCH/default/<request_id>.json", "requestId", "workTypeName", "sourceWorkName", "targetWorkName", "requiredState", "`you docs agents`", "`you docs relationships`", "`you docs guards`"}, absent: []string{"../internal/development/parent-aware-fan-in.md", "work_type_name", "source_work_name", "# Batch Work", "batch-work.md", "[Agents](agents.md)", "[Relationships](relationships.md)", "[Guards](guards.md)"}},
	{name: "batch-work", heading: "# Batch Inputs", markers: []string{"## Quick reference", "## Before you submit", "factory.json", "factory/docs/overview.md", "`you docs batch-work` is a compatibility alias", "FACTORY_REQUEST_BATCH", "DEPENDS_ON", "PARENT_CHILD", "factory/inputs/BATCH/default/<request_id>.json", "requestId", "workTypeName", "sourceWorkName", "targetWorkName"}, absent: []string{"# Batch Work", "docs/reference/batch-work.md", "batch-work.md", "work_type_name", "source_work_name"}},
	{name: "templates", heading: "# Templates", markers: []string{".Context.Project", ".Context.WorkDir", "docs/reference/templates.md", "text/template", "you docs guards", "you docs relationships"}, absent: []string{"docs/reference/prompt-variables.md"}},
}

func TestDocsCommandSmoke_AuthoringFactoriesLinkCoverageFromCanonicalTree(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	path := filepath.Join(repoRoot, "docs", "reference", "authoring-factories.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read authoring factories docs: %v", err)
	}
	doc := string(content)

	for _, marker := range []string{
		"you run --factory ./factory.json \"Fix the lint issues\"",
		"handlingBehavior: [\"DEFAULT\"]",
		"## Test Workflows With Mock Workers",
		"you run --dir ./factory --with-mock-workers",
		"you run --dir ./factory --with-mock-workers ./docs/examples/mock-workers.json",
		"../examples/mock-workers.json",
		"../examples/mock-workers-script.json",
		"../examples/mock-workers-mixed.json",
		"../examples/startup-work.json",
		"../examples/README.md",
		"you docs mock-workers",
		"you docs record-replay",
		"--no-record",
	} {
		if !strings.Contains(doc, marker) {
			t.Fatalf("authoring factories docs missing marker %q", marker)
		}
	}
}

var retiredDocsInvocationPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(^|[^[:alnum:]-])infinite-you docs([^[:alnum:]-]|$)`),
	regexp.MustCompile("(^|[^[:alnum:]-])agent-factory run([^[:alnum:]-]|$)"),
	regexp.MustCompile("(^|[^[:alnum:]-])agent-factory config([^[:alnum:]-]|$)"),
}

func TestDocsCommandSmoke_PackagedTopicsRemainAvailableOutsideRepositoryDocsTree(t *testing.T) {
	workingDir := t.TempDir()
	missingDocsTree := filepath.Join(workingDir, "docs")
	if _, err := os.Stat(missingDocsTree); !os.IsNotExist(err) {
		t.Fatalf("temp working dir unexpectedly contains docs tree %q", missingDocsTree)
	}

	index := executeDocsSmokeCommand(t, workingDir, "docs")
	for _, want := range []string{"# Docs", "`agents` - Agent orientation: read order, work submission, command matrix, planner vs executor, and topic router", "`authoring-factories` - Practical factory authoring workflow", "`config` - factory.json topology, work types, states, workers, workstations, resources, and portability", "`mock-workers` - Mock-worker runs", "`record-replay` - Record and replay run modes", "`guards` - Workstation, input, and factory guards", "`relationships` - Batch DEPENDS_ON", "`work` - Submitted work: session-scoped work routes, tags, batch cross-links, and submission contracts", "`sessions` - Live factory sessions: session list, session show, factory query, status API, dashboard URL, and run modes", "`packaged-tts` - Packaged @you/tts invocation", "`batch-inputs` - Batch input files", "`workstations` - Workstation kinds", "`you docs agents`", "`you docs authoring-factories`", "`you docs config`", "`you docs mock-workers`", "`you docs record-replay`", "`you docs guards`", "`you docs relationships`", "`you docs work`", "`you docs sessions`", "`you docs packaged-tts`", "`you docs batch-inputs`", "`you docs workstations`"} {
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
		if got := err.Error(); got != `unsupported docs topic "unknown" (supported: agents, authoring-factories, config, mock-workers, record-replay, guards, relationships, work, sessions, orchestrators, workstations, workers, resources, models, packaged-tts, batch-inputs, templates)` {
			t.Fatalf("unexpected unsupported topic error %q", got)
		}
	})
	if got := unsupportedStdout.String(); got != "" {
		t.Fatalf("unsupported docs topic should not write stdout, got %q", got)
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
