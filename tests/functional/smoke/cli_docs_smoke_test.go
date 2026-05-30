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

var docsSmokeTopics = []docsSmokeTopic{
	{name: "authoring-factories", heading: "# Authoring Factories", markers: []string{"factory.json", "workers/<name>/AGENTS.md", "workstations/<name>/AGENTS.md", "you run --factory ./factory.json \"Fix the lint issues\"", "handlingBehavior: [\"DEFAULT\"]", "you run --dir ./factory --with-mock-workers", "you docs mock-workers", "you docs record-replay", "--no-record", "requestId", "workTypeName"}, absent: []string{"work_type_name", "source_work_name", `"request_id"`}},
	{name: "config", heading: "# Config", markers: []string{"factory.json", "workTypes", "supportingFiles", "bundledFiles", "share-time starter-work snapshots", "docs/reference/config.md", "docs/reference/work.md", "you docs mock-workers", "you docs record-replay", "you docs guards", "you docs relationships", "you docs authoring-factories", "--factory", "you run --factory ./factory.json \"Fix the lint issues\"", "--with-mock-workers", "--record", "--replay", "--no-record", "you-agent-factory run"}, absent: []string{"Agent Factory run"}},
	{name: "mock-workers", heading: "# Mock Workers", markers: []string{"--with-mock-workers", "mockWorkers", "runType", "accept", "reject", "script", "docs/examples/mock-workers.json", "docs/examples/startup-work.json"}},
	{name: "record-replay", heading: "# Record and Replay", markers: []string{"--record", "--replay", "--no-record", "~/.you-agent-factory/recordings/", "docs/examples/sample-run.replay.json", "Recording saved:", "`--record` with `--replay`", "`--no-record` with `--record`"}},
	{name: "guards", heading: "# Guards", markers: []string{"VISIT_COUNT", "SAME_NAME", "MATCHES_FIELDS", "ALL_CHILDREN_COMPLETE", "ANY_CHILD_FAILED", "INFERENCE_THROTTLE_GUARD", "LOGICAL_MOVE", "limits.maxRetries"}},
	{name: "relationships", heading: "# Relationships", markers: []string{"DEPENDS_ON", "PARENT_CHILD", "SPAWNED_BY", "requiredState", "sourceWorkName", "targetWorkName", "workTypeName", "requestId", "FACTORY_REQUEST_BATCH", "[Guards](guards.md)", "[Batch Inputs](batch-inputs.md)"}, absent: []string{"work_type_name", "source_work_name", "target_work_name"}},
	{name: "work", heading: "# Factory JSON And Work Configuration", markers: []string{"work types, states, workers, workstations, resources, and routing", "supportingFiles", "## Work Types", "handlingBehavior: [\"DEFAULT\"]", "you run --factory", "## Resources", "[Workstations](workstations.md)", "[Guards](guards.md)", "[Relationships](relationships.md)", "batch-inputs.md"}, absent: []string{"../internal/development/parent-aware-fan-in.md", "../internal/development/workstation-guards-and-guarded-loop-breakers.md"}},
	{name: "workstations", heading: "# Workstations Reference", markers: []string{"workstation authoring contract", "MODEL_WORKSTATION", "CLASSIFIER_WORKSTATION", "LOGICAL_MOVE", "[Guards](guards.md)", "[Relationships](relationships.md)"}, absent: []string{"../internal/development/parent-aware-fan-in.md", "../internal/development/workstation-guards-and-guarded-loop-breakers.md"}},
	{name: "workstation", heading: "# Workstations Reference", markers: []string{"workstation authoring contract", "MODEL_WORKSTATION", "CLASSIFIER_WORKSTATION", "LOGICAL_MOVE"}, absent: []string{"docs/reference/workstations-and-workers.md"}},
	{name: "workers", heading: "# Workers", markers: []string{"MODEL_WORKER", "SCRIPT_WORKER", "modelProvider", "docs/reference/workers.md"}, absent: []string{"docs/reference/workstations-and-workers.md"}},
	{name: "resources", heading: "# Resources", markers: []string{"capacity", "workstations", "agent-slot", "docs/reference/resources.md"}},
	{name: "batch-inputs", heading: "# Batch Inputs", markers: []string{"## Quick Reference", "## Before You Submit", "factory.json", "factory/docs/overview.md", "you docs batch-work", "FACTORY_REQUEST_BATCH", "DEPENDS_ON", "PARENT_CHILD", "factory/inputs/BATCH/default/<request_id>.json", "requestId", "workTypeName", "sourceWorkName", "targetWorkName", "requiredState", "you docs relationships", "[Relationships](relationships.md)", "[Guards](guards.md)"}, absent: []string{"../internal/development/parent-aware-fan-in.md", "work_type_name", "source_work_name", "# Batch Work", "docs/reference/batch-work.md"}},
	{name: "batch-work", heading: "# Batch Inputs", markers: []string{"## Quick Reference", "factory/docs/overview.md", "FACTORY_REQUEST_BATCH", "DEPENDS_ON", "PARENT_CHILD", "factory/inputs/BATCH/default/<request_id>.json", "requestId", "workTypeName", "sourceWorkName", "targetWorkName"}, absent: []string{"# Batch Work", "docs/reference/batch-work.md", "work_type_name", "source_work_name"}},
	{name: "templates", heading: "# Templates", markers: []string{".Context.Project", ".Context.WorkDir", "docs/reference/templates.md", "text/template", "you docs guards", "you docs relationships"}, absent: []string{"docs/reference/prompt-variables.md"}},
}

func TestAuthoringFactoriesDocs_LinkMockWorkerReplayExamplesFromCustomerPath(t *testing.T) {
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
		"../examples/startup-work.json",
		"../examples/README.md",
		"you docs mock-workers",
		"you docs record-replay",
		"--no-record",
		"record-replay.md",
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

func TestCLIDocsSmoke_PackagedTopicsRemainAvailableOutsideRepositoryDocsTree(t *testing.T) {
	workingDir := t.TempDir()
	missingDocsTree := filepath.Join(workingDir, "docs")
	if _, err := os.Stat(missingDocsTree); !os.IsNotExist(err) {
		t.Fatalf("temp working dir unexpectedly contains docs tree %q", missingDocsTree)
	}

	index := executeDocsSmokeCommand(t, workingDir, "docs")
	for _, want := range []string{"# Docs", "`authoring-factories` - Practical factory authoring workflow", "`config` - Factory configuration", "`mock-workers` - Mock-worker runs", "`record-replay` - Record and replay run modes", "`guards` - Workstation, input, and factory guards", "`relationships` - Batch DEPENDS_ON", "`work` - Work types", "`batch-inputs` - Batch input files", "`workstations` - Workstation kinds", "`you docs authoring-factories`", "`you docs config`", "`you docs mock-workers`", "`you docs record-replay`", "`you docs guards`", "`you docs relationships`", "`you docs work`", "`you docs batch-inputs`", "`you docs workstations`"} {
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
		if got := err.Error(); got != `unsupported docs topic "unknown" (supported: authoring-factories, config, mock-workers, record-replay, guards, relationships, work, workstations, workers, resources, models, batch-inputs, templates)` {
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
