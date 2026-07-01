package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestWorkVisualizeCommand_DependentBatchWritesMermaidToStdout(t *testing.T) {
	path := writeWorkVisualizeBatchFile(t, `{
  "requestId": "cli-dependent",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "alpha", "workTypeName": "task"},
    {"name": "beta", "workTypeName": "task"},
    {"name": "gamma", "workTypeName": "task"}
  ],
  "relations": [
    {"type": "DEPENDS_ON", "sourceWorkName": "beta", "targetWorkName": "alpha"},
    {"type": "DEPENDS_ON", "sourceWorkName": "gamma", "targetWorkName": "beta"}
  ]
}`)

	stdout, stderr, err := executeWorkVisualize(t, path)
	if err != nil {
		t.Fatalf("execute work visualize: %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty on success", stderr)
	}
	if !strings.HasPrefix(stdout, "flowchart TD\n") {
		t.Fatalf("stdout missing flowchart header:\n%s", stdout)
	}
	for _, want := range []string{
		`alpha["alpha"]`,
		`beta["beta"]`,
		`gamma["gamma"]`,
		"beta --> alpha",
		"gamma --> beta",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestWorkVisualizeCommand_IndependentWorkBatchHasStandaloneNodes(t *testing.T) {
	path := writeWorkVisualizeBatchFile(t, `{
  "requestId": "cli-independent",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "solo-a", "workTypeName": "task"},
    {"name": "solo-b", "workTypeName": "task"}
  ]
}`)

	stdout, stderr, err := executeWorkVisualize(t, path)
	if err != nil {
		t.Fatalf("execute work visualize: %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty on success", stderr)
	}
	if strings.Contains(stdout, "-->") {
		t.Fatalf("stdout should not contain dependency edges:\n%s", stdout)
	}
	for _, want := range []string{`"solo-a"["solo-a"]`, `"solo-b"["solo-b"]`} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestWorkVisualizeCommand_InvalidDependencyReferenceFailsWithEmptyStdout(t *testing.T) {
	path := writeWorkVisualizeBatchFile(t, `{
  "requestId": "cli-unknown-dep",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "alpha", "workTypeName": "task"}
  ],
  "relations": [
    {"type": "DEPENDS_ON", "sourceWorkName": "alpha", "targetWorkName": "missing"}
  ]
}`)

	stdout, stderr, err := executeWorkVisualize(t, path)
	if err == nil {
		t.Fatal("expected non-zero exit for unknown dependency reference")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty on validation failure", stdout)
	}
	if !strings.Contains(stderr, "unknown") && !strings.Contains(stderr, "missing") {
		t.Fatalf("stderr = %q, want actionable dependency error", stderr)
	}
}

func TestWorkVisualizeCommand_InvalidJSONFailsWithEmptyStdout(t *testing.T) {
	path := writeWorkVisualizeBatchFile(t, `{not-json`)

	stdout, stderr, err := executeWorkVisualize(t, path)
	if err == nil {
		t.Fatal("expected non-zero exit for invalid JSON")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty on validation failure", stdout)
	}
	if stderr == "" {
		t.Fatal("stderr is empty, want validation error message")
	}
}

func TestWorkVisualizeCommand_MissingFileFailsWithEmptyStdout(t *testing.T) {
	stdout, stderr, err := executeWorkVisualize(t, filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("expected non-zero exit for missing batch file")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty on validation failure", stdout)
	}
	if !strings.Contains(stderr, "batch file not found") {
		t.Fatalf("stderr = %q, want missing file error", stderr)
	}
}

func TestWorkVisualizeCommand_MermaidAndMarkdownShareEquivalentEdges(t *testing.T) {
	path := writeWorkVisualizeBatchFile(t, `{
  "requestId": "cli-format-parity",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "alpha", "workTypeName": "task"},
    {"name": "beta", "workTypeName": "task"},
    {"name": "gamma", "workTypeName": "task"}
  ],
  "relations": [
    {"type": "DEPENDS_ON", "sourceWorkName": "beta", "targetWorkName": "alpha"},
    {"type": "DEPENDS_ON", "sourceWorkName": "gamma", "targetWorkName": "beta"}
  ]
}`)

	mermaidStdout, mermaidStderr, err := executeWorkVisualize(t, path)
	if err != nil {
		t.Fatalf("execute work visualize mermaid: %v", err)
	}
	if mermaidStderr != "" {
		t.Fatalf("mermaid stderr = %q, want empty on success", mermaidStderr)
	}

	markdownStdout, markdownStderr, err := executeWorkVisualize(t, "--format", "markdown-mermaid", path)
	if err != nil {
		t.Fatalf("execute work visualize markdown-mermaid: %v", err)
	}
	if markdownStderr != "" {
		t.Fatalf("markdown stderr = %q, want empty on success", markdownStderr)
	}

	mermaidEdges := mermaidEdgeLines(mermaidStdout)
	embedded := mermaidDiagramFromMarkdown(t, markdownStdout)
	markdownEdges := mermaidEdgeLines(embedded)
	if len(mermaidEdges) == 0 {
		t.Fatalf("mermaid output missing edges:\n%s", mermaidStdout)
	}
	if strings.Join(mermaidEdges, "\n") != strings.Join(markdownEdges, "\n") {
		t.Fatalf("edge mismatch:\nmermaid=%v\nmarkdown=%v", mermaidEdges, markdownEdges)
	}
}

func executeWorkVisualize(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	root := NewRootCommand()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	cmdArgs := append([]string{"work", "visualize"}, args...)
	root.SetArgs(cmdArgs)
	err = root.Execute()
	return out.String(), errOut.String(), err
}

func writeWorkVisualizeBatchFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func mermaidEdgeLines(diagram string) []string {
	var edges []string
	for _, line := range strings.Split(diagram, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "-->") {
			edges = append(edges, line)
		}
	}
	sort.Strings(edges)
	return edges
}

func mermaidDiagramFromMarkdown(t *testing.T, markdown string) string {
	t.Helper()
	start := strings.Index(markdown, "```mermaid\n")
	if start < 0 {
		t.Fatalf("markdown missing opening mermaid fence:\n%s", markdown)
	}
	bodyStart := start + len("```mermaid\n")
	rest := markdown[bodyStart:]
	end := strings.Index(rest, "\n```")
	if end < 0 {
		t.Fatalf("markdown missing closing mermaid fence:\n%s", markdown)
	}
	return rest[:end]
}
