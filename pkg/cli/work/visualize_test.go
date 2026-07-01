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
