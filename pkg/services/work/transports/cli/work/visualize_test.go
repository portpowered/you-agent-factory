package work

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
)

func TestVisualize_ForwardsRequestAndWritesWorkOwnedResult(t *testing.T) {
	var got workdomain.VisualizationRequest
	operation := func(request workdomain.VisualizationRequest) (string, error) {
		got = request
		return "flowchart TD\n", nil
	}
	var output bytes.Buffer
	err := NewVisualize(operation)(VisualizeConfig{
		BatchFile: "batch.json", Format: "markdown-mermaid", Output: &output,
	})
	if err != nil {
		t.Fatalf("Visualize: %v", err)
	}
	if got.BatchFile != "batch.json" || got.Format != "markdown-mermaid" {
		t.Fatalf("request = %#v", got)
	}
	if output.String() != "flowchart TD\n" {
		t.Fatalf("output = %q", output.String())
	}
}

func TestVisualize_LeavesOutputEmptyWhenWorkOperationFails(t *testing.T) {
	var output bytes.Buffer
	want := errors.New("invalid JSON")
	err := Visualize(func(workdomain.VisualizationRequest) (string, error) {
		return "", want
	}, VisualizeConfig{BatchFile: "batch.json", Output: &output})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if output.Len() != 0 {
		t.Fatalf("output = %q, want empty", output.String())
	}
}

func TestVisualize_RequiresInjectedOperationAndCobraOutput(t *testing.T) {
	if err := Visualize(nil, VisualizeConfig{Output: &bytes.Buffer{}}); err == nil ||
		!strings.Contains(err.Error(), "operation is required") {
		t.Fatalf("missing operation error = %v", err)
	}
	if err := Visualize(func(workdomain.VisualizationRequest) (string, error) { return "", nil }, VisualizeConfig{}); err == nil ||
		!strings.Contains(err.Error(), "output is required") {
		t.Fatalf("missing output error = %v", err)
	}
}
