package work

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVisualize_WritesMermaidFlowchartToStdout(t *testing.T) {
	path := writeVisualizeBatchFile(t, `{
  "requestId": "visualize-test",
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

	var out bytes.Buffer
	if err := Visualize(VisualizeConfig{BatchFile: path, Output: &out}); err != nil {
		t.Fatalf("Visualize: %v", err)
	}

	got := out.String()
	if !strings.HasPrefix(got, "flowchart TD\n") {
		t.Fatalf("output missing flowchart header:\n%s", got)
	}
	for _, want := range []string{
		`alpha["alpha"]`,
		`beta["beta"]`,
		`gamma["gamma"]`,
		"beta --> alpha",
		"gamma --> beta",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestVisualize_IncludesStandaloneWork(t *testing.T) {
	path := writeVisualizeBatchFile(t, `{
  "requestId": "standalone-visualize",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "solo-a", "workTypeName": "task"},
    {"name": "solo-b", "workTypeName": "task"}
  ]
}`)

	var out bytes.Buffer
	if err := Visualize(VisualizeConfig{BatchFile: path, Output: &out}); err != nil {
		t.Fatalf("Visualize: %v", err)
	}

	got := out.String()
	if strings.Contains(got, "-->") {
		t.Fatalf("standalone batch should not emit edges:\n%s", got)
	}
	for _, want := range []string{`"solo-a"["solo-a"]`, `"solo-b"["solo-b"]`} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestVisualize_WritesMarkdownMermaidToStdout(t *testing.T) {
	path := writeVisualizeBatchFile(t, `{
  "requestId": "markdown-visualize",
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

	var out bytes.Buffer
	if err := Visualize(VisualizeConfig{
		BatchFile: path,
		Format:    "markdown-mermaid",
		Output:    &out,
	}); err != nil {
		t.Fatalf("Visualize: %v", err)
	}

	got := out.String()
	if !strings.HasPrefix(got, "# Work Dependency Graph\n\n") {
		t.Fatalf("output missing markdown title:\n%s", got)
	}
	if !strings.Contains(got, "3 work items and 2 declared dependencies.") {
		t.Fatalf("output missing summary:\n%s", got)
	}

	start := strings.Index(got, "```mermaid\n")
	end := strings.Index(got, "\n```\n")
	if start < 0 || end < 0 || end <= start {
		t.Fatalf("output missing fenced mermaid block:\n%s", got)
	}
	embedded := got[start+len("```mermaid\n") : end]
	for _, want := range []string{
		"flowchart TD",
		`alpha["alpha"]`,
		`beta["beta"]`,
		`gamma["gamma"]`,
		"beta --> alpha",
		"gamma --> beta",
	} {
		if !strings.Contains(embedded, want) {
			t.Fatalf("embedded mermaid missing %q:\n%s", want, embedded)
		}
	}
}

func TestVisualize_UnsupportedFormatListsSupportedValues(t *testing.T) {
	path := writeVisualizeBatchFile(t, `{
  "requestId": "unsupported-format",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [{"name": "alpha", "workTypeName": "task"}]
}`)

	var out bytes.Buffer
	err := Visualize(VisualizeConfig{
		BatchFile: path,
		Format:    "svg",
		Output:    &out,
	})
	if err == nil {
		t.Fatal("expected unsupported format error")
	}
	if !strings.Contains(err.Error(), `unsupported format "svg"`) {
		t.Fatalf("error = %q", err.Error())
	}
	if !strings.Contains(err.Error(), "mermaid, markdown-mermaid") {
		t.Fatalf("error = %q, want supported formats listed", err.Error())
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", out.String())
	}
}

func TestVisualize_MissingFileLeavesStdoutEmpty(t *testing.T) {
	var out bytes.Buffer
	err := Visualize(VisualizeConfig{
		BatchFile: filepath.Join(t.TempDir(), "missing.json"),
		Output:    &out,
	})
	if err == nil {
		t.Fatal("expected missing file error")
	}
	if !strings.Contains(err.Error(), "batch file not found") {
		t.Fatalf("error = %q", err.Error())
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", out.String())
	}
}

func TestVisualize_InvalidBatchLeavesStdoutEmpty(t *testing.T) {
	path := writeVisualizeBatchFile(t, `{not-json`)

	var out bytes.Buffer
	err := Visualize(VisualizeConfig{BatchFile: path, Output: &out})
	if err == nil {
		t.Fatal("expected invalid JSON error")
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", out.String())
	}
}

func TestVisualize_UnknownDependencyLeavesStdoutEmpty(t *testing.T) {
	path := writeVisualizeBatchFile(t, `{
  "requestId": "unknown-dep",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "alpha", "workTypeName": "task"}
  ],
  "relations": [
    {"type": "DEPENDS_ON", "sourceWorkName": "alpha", "targetWorkName": "missing"}
  ]
}`)

	var out bytes.Buffer
	err := Visualize(VisualizeConfig{BatchFile: path, Output: &out})
	if err == nil {
		t.Fatal("expected unknown dependency error")
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", out.String())
	}
}

func writeVisualizeBatchFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
