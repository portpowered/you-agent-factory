package functional_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	agentcli "github.com/portpowered/agent-factory/pkg/cli"
	docscli "github.com/portpowered/agent-factory/pkg/cli/docs"
)

type docsSmokeTopic struct {
	name          string
	referenceFile string
}

var docsSmokeTopics = []docsSmokeTopic{
	{name: string(docscli.TopicConfig), referenceFile: "config.md"},
	{name: string(docscli.TopicWorkstation), referenceFile: "workstation.md"},
	{name: string(docscli.TopicWorkers), referenceFile: "workers.md"},
	{name: string(docscli.TopicResources), referenceFile: "resources.md"},
	{name: string(docscli.TopicBatchWork), referenceFile: "batch-work.md"},
	{name: string(docscli.TopicTemplates), referenceFile: "templates.md"},
}

func TestDocsCommandSmoke_PackagedTopicsRemainAvailableOutsideRepositoryDocsTree(t *testing.T) {
	referenceDir := docsSmokeReferenceDir(t)
	workingDir := t.TempDir()
	missingDocsTree := filepath.Join(workingDir, "libraries", "agent-factory", "docs")
	if _, err := os.Stat(missingDocsTree); !os.IsNotExist(err) {
		t.Fatalf("temp working dir unexpectedly contains package docs tree %q", missingDocsTree)
	}

	for _, topic := range docsSmokeTopics {
		topic := topic
		t.Run(topic.name, func(t *testing.T) {
			want := readDocsSmokeReference(t, filepath.Join(referenceDir, topic.referenceFile))
			got := executeDocsSmokeCommand(t, workingDir, "docs", topic.name)
			if got != want {
				t.Fatalf("agent-factory docs %s output did not match packaged reference markdown", topic.name)
			}
		})
	}
}

func docsSmokeReferenceDir(t *testing.T) string {
	t.Helper()

	dir, err := filepath.Abs(filepath.Join("..", "..", "pkg", "cli", "docs", "reference"))
	if err != nil {
		t.Fatalf("resolve packaged docs reference dir: %v", err)
	}
	return dir
}

func readDocsSmokeReference(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read packaged docs reference %s: %v", path, err)
	}
	return string(data)
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
