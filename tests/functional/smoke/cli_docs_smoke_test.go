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
	{name: "config", heading: "# Config", markers: []string{"factory.json", "workTypes", "supportingFiles", "bundledFiles", "share-time starter-work snapshots", "docs/reference/config.md", "docs/reference/work.md", "docs/reference/authoring-factories.md", "--with-mock-workers", "--record", "--replay", "--no-record", "you-agent-factory run"}, absent: []string{"Agent Factory run"}},
	{name: "workstation", heading: "# Workstation", markers: []string{"inputs", "outputs", "LOGICAL_MOVE", "docs/reference/workstations.md"}, absent: []string{"docs/reference/workstations-and-workers.md"}},
	{name: "workers", heading: "# Workers", markers: []string{"MODEL_WORKER", "SCRIPT_WORKER", "modelProvider", "docs/reference/workers.md"}, absent: []string{"docs/reference/workstations-and-workers.md"}},
	{name: "resources", heading: "# Resources", markers: []string{"capacity", "workstations", "agent-slot", "docs/reference/resources.md"}},
	{name: "batch-work", heading: "# Batch Work", markers: []string{"FACTORY_REQUEST_BATCH", "DEPENDS_ON", "docs/reference/batch-inputs.md"}, absent: []string{"docs/reference/batch-work.md"}},
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

	help := executeDocsSmokeCommand(t, workingDir, "docs")
	for _, want := range []string{"Print packaged markdown reference topics", "Use one of the supported topic subcommands"} {
		if !strings.Contains(help, want) {
			t.Fatalf("docs help missing %q:\n%s", want, help)
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
