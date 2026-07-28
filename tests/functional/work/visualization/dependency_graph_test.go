package visualization_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
)

const visualizeBatchRequestID = "work-visualize-dependency-graph"

// TestWorkVisualizeProducesDeterministicGraph proves you work visualize renders
// the same Mermaid dependency graph for a local FACTORY_REQUEST_BATCH on every
// invocation without submitting work or contacting a running factory.
func TestWorkVisualizeProducesDeterministicGraph(t *testing.T) {
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)
	batchPath := writeVisualizeDependencyBatchFile(t, session.WorkDir)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	first, err := session.Run(ctx, "work", "visualize", batchPath)
	firstOut := session.RequireSuccess(t, "first visualize", first, err).Stdout

	second, err := session.Run(ctx, "work", "visualize", batchPath)
	secondOut := session.RequireSuccess(t, "second visualize", second, err).Stdout

	if firstOut != secondOut {
		t.Fatalf("visualize output not deterministic:\nfirst:\n%s\nsecond:\n%s", firstOut, secondOut)
	}

	assertVisualizeDependencyGraphOutput(t, firstOut)
}

func writeVisualizeDependencyBatchFile(t *testing.T, dir string) string {
	t.Helper()

	batchPath := filepath.Join(dir, "visualize-dependency-batch.json")
	content := fmt.Sprintf(`{
  "requestId": %q,
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "plan", "workTypeName": "task"},
    {"name": "ship-release", "workTypeName": "task"}
  ],
  "relations": [
    {"type": "DEPENDS_ON", "sourceWorkName": "ship-release", "targetWorkName": "plan"}
  ]
}`, visualizeBatchRequestID)
	if err := os.WriteFile(batchPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write visualize batch file: %v", err)
	}
	return batchPath
}

func assertVisualizeDependencyGraphOutput(t *testing.T, output string) {
	t.Helper()

	if !strings.HasPrefix(output, "flowchart TD\n") {
		t.Fatalf("visualize output missing flowchart header:\n%s", output)
	}
	for _, want := range []string{
		`plan["plan"]`,
		`"ship-release"["ship-release"]`,
		`"ship-release" --> plan`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("visualize output missing %q:\n%s", want, output)
		}
	}
}
