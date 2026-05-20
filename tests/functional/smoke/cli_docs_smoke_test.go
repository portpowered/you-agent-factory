package smoke

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentcli "github.com/portpowered/infinite-you/pkg/cli"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type docsSmokeTopic struct {
	name    string
	heading string
	markers []string
}

var docsSmokeTopics = []docsSmokeTopic{
	{name: "config", heading: "# Config", markers: []string{"factory.json", "workTypes", "docs/reference/config.md", "docs/reference/work.md"}},
	{name: "workstation", heading: "# Workstation", markers: []string{"inputs", "outputs", "LOGICAL_MOVE", "docs/reference/workstations.md"}},
	{name: "workers", heading: "# Workers", markers: []string{"MODEL_WORKER", "SCRIPT_WORKER", "modelProvider", "docs/reference/workers.md"}},
	{name: "resources", heading: "# Resources", markers: []string{"capacity", "workstations", "agent-slot", "docs/reference/resources.md"}},
	{name: "batch-work", heading: "# Batch Work", markers: []string{"FACTORY_REQUEST_BATCH", "DEPENDS_ON", "docs/reference/batch-inputs.md"}},
	{name: "templates", heading: "# Templates", markers: []string{".Context.Project", ".Context.WorkDir", "docs/reference/templates.md", "text/template"}},
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
				t.Fatalf("agent-factory docs %s missing heading %q", topic.name, topic.heading)
			}
			for _, marker := range topic.markers {
				if !strings.Contains(output, marker) {
					t.Fatalf("agent-factory docs %s missing marker %q", topic.name, marker)
				}
			}
			for _, oldInvocation := range []string{
				"infinite-you docs",
				"agent-factory run",
				"agent-factory config",
			} {
				if strings.Contains(output, oldInvocation) {
					t.Fatalf("agent-factory docs %s still contains old executable invocation %q:\n%s", topic.name, oldInvocation, output)
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
