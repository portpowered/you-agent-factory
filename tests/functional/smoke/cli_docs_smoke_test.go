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
	{name: "authoring-factories", heading: "# Authoring Factories", markers: []string{"factory.json", "workers/<name>/AGENTS.md", "workstations/<name>/AGENTS.md", "you run --dir ./factory --with-mock-workers", "--record ./docs/examples/sample-run.replay.json", "--replay ./docs/examples/sample-run.replay.json"}},
	{name: "config", heading: "# Config", markers: []string{"factory.json", "workTypes", "supportingFiles", "bundledFiles", "share-time starter-work snapshots", "docs/reference/config.md", "docs/reference/work.md", "docs/reference/authoring-factories.md", "--with-mock-workers", "--record", "--replay", "--no-record", "you-agent-factory run"}, absent: []string{"Agent Factory run"}},
	{name: "work", heading: "# Factory JSON And Work Configuration", markers: []string{"work types, states, workers, workstations, resources, and routing", "supportingFiles", "## Work Types", "## Resources", "[Workstations](workstations.md)", "batch-inputs.md"}},
	{name: "workstations", heading: "# Workstations Reference", markers: []string{"workstation authoring contract", "MODEL_WORKSTATION", "CLASSIFIER_WORKSTATION", "LOGICAL_MOVE"}},
	{name: "workstation", heading: "# Workstations Reference", markers: []string{"workstation authoring contract", "MODEL_WORKSTATION", "CLASSIFIER_WORKSTATION", "LOGICAL_MOVE"}, absent: []string{"docs/reference/workstations-and-workers.md"}},
	{name: "workers", heading: "# Workers", markers: []string{"MODEL_WORKER", "SCRIPT_WORKER", "modelProvider", "docs/reference/workers.md"}, absent: []string{"docs/reference/workstations-and-workers.md"}},
	{name: "resources", heading: "# Resources", markers: []string{"capacity", "workstations", "agent-slot", "docs/reference/resources.md"}},
	{name: "batch-inputs", heading: "# Batch Inputs", markers: []string{"FACTORY_REQUEST_BATCH", "DEPENDS_ON", "PARENT_CHILD", "factory/inputs/BATCH/default/<request_id>.json"}},
	{name: "batch-work", heading: "# Batch Inputs", markers: []string{"FACTORY_REQUEST_BATCH", "DEPENDS_ON", "PARENT_CHILD", "factory/inputs/BATCH/default/<request_id>.json"}, absent: []string{"# Batch Work", "docs/reference/batch-work.md"}},
	{name: "templates", heading: "# Templates", markers: []string{".Context.Project", ".Context.WorkDir", "docs/reference/templates.md", "text/template"}, absent: []string{"docs/reference/prompt-variables.md"}},
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
		"## Test Workflows With Mock Workers",
		"you run --dir ./factory --with-mock-workers",
		"you run --dir ./factory --with-mock-workers ./docs/examples/mock-workers.json",
		"../examples/mock-workers.json",
		"../examples/startup-work.json",
		"../examples/README.md",
		"--record ./docs/examples/sample-run.replay.json",
		"--replay ./docs/examples/sample-run.replay.json",
		"--no-record",
		"docs/internal/development/record-replay.md",
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
	for _, want := range []string{"# Docs", "`authoring-factories` - Practical factory authoring workflow", "`config` - Factory configuration", "`work` - Work types", "`batch-inputs` - Batch input files", "`workstations` - Workstation kinds", "`you docs authoring-factories`", "`you docs config`", "`you docs work`", "`you docs batch-inputs`", "`you docs workstations`"} {
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
		if got := err.Error(); got != `unsupported docs topic "unknown" (supported: authoring-factories, config, mock-workers, record-replay, work, workstations, workers, resources, models, batch-inputs, templates)` {
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
