package functional_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentcli "github.com/portpowered/agent-factory/pkg/cli"
)

type docsSmokeTopic struct {
	name    string
	heading string
	markers []string
}

var docsSmokeTopics = []docsSmokeTopic{
	{name: "config", heading: "# Config", markers: []string{"factory.json", "workTypes", "infinite-you docs workstation"}},
	{name: "workstation", heading: "# Workstation", markers: []string{"inputs", "outputs", "LOGICAL_MOVE"}},
	{name: "workers", heading: "# Workers", markers: []string{"MODEL_WORKER", "SCRIPT_WORKER", "modelProvider"}},
	{name: "resources", heading: "# Resources", markers: []string{"capacity", "workstations", "agent-slot"}},
	{name: "batch-work", heading: "# Batch Work", markers: []string{"FACTORY_REQUEST_BATCH", "DEPENDS_ON", "PARENT_CHILD"}},
	{name: "templates", heading: "# Templates", markers: []string{".Context.Project", ".Context.WorkDir", "text/template"}},
}

func TestDocsCommandSmoke_PackagedTopicsRemainAvailableOutsideRepositoryDocsTree(t *testing.T) {
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
				t.Fatalf("infinite-you docs %s missing heading %q", topic.name, topic.heading)
			}
			for _, marker := range topic.markers {
				if !strings.Contains(output, marker) {
					t.Fatalf("infinite-you docs %s missing marker %q", topic.name, marker)
				}
			}
		})
	}
}

func executeDocsSmokeCommand(t *testing.T, workingDir string, args ...string) string {
	t.Helper()

	var out bytes.Buffer
	withWorkingDirectory(t, workingDir, func() {
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
