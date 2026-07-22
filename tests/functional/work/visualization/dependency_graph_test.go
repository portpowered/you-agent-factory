package visualization

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestDependencyGraphVisualization_RendersCompleteEscapedFlowchart(t *testing.T) {
	request := work.WorkRequest{
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		RequestID: "visualize-release",
		Works: []work.Work{
			{Name: "plan", WorkID: "work-plan"},
			{Name: "ship-release", WorkID: "work-ship"},
		},
		Relations: []work.WorkRelation{
			{
				SourceWorkName: "ship-release",
				TargetWorkName: "plan",
				Type:           work.WorkRelationDependsOn,
			},
		},
	}

	graph, err := work.DeriveFromWorkRequest(request)
	if err != nil {
		t.Fatalf("derive dependency graph: %v", err)
	}

	got := work.RenderMermaidFlowchart(graph)
	for _, want := range []string{
		"flowchart TD\n",
		`plan["plan"]`,
		`"ship-release"["ship-release"]`,
		`"ship-release" --> plan`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("flowchart missing %q:\n%s", want, got)
		}
	}
}
