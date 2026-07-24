//go:build functionallong

package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestMultiOutputColorPropagation(t *testing.T) {
	support.SkipLongFunctional(t, "slow multi-output cross-type color propagation sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_output_color_propagation"))

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "my-feature-plan",
		WorkID:     "work-idea-001",
		WorkTypeID: "idea",
		TraceID:    "trace-multi-out",
		Payload:    []byte("idea payload"),
		Tags:       map[string]string{"priority": "high"},
	})

	_, listedWork := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ScriptCommandRunner: support.NewStaticSuccessCommandRunner("split-output"),
	}, 5*time.Second)
	assertWorkflowSessionPlaces(t, listedWork, map[string]int{
		"idea:complete": 1, "task:complete": 1, "idea:init": 0, "task:init": 0,
	})
	assertCrossTypeTerminalWork(t, listedWork, "my-feature-plan", "work-idea-001", "trace-multi-out")
}

func TestMultiOutputColorPropagation_NameAvailableDownstream(t *testing.T) {
	support.SkipLongFunctional(t, "slow multi-output downstream-name propagation sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_output_color_propagation"))

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "prd-factory-log-levels",
		WorkID:     "work-idea-002",
		WorkTypeID: "idea",
		TraceID:    "trace-name-downstream",
		Payload:    []byte("idea about logging"),
	})

	_, listedWork := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ScriptCommandRunner: support.NewStaticSuccessCommandRunner("downstream-ok"),
	}, 5*time.Second)
	assertWorkflowSessionPlaces(t, listedWork, map[string]int{"task:complete": 1})
	assertCrossTypeTerminalWork(t, listedWork, "prd-factory-log-levels", "work-idea-002", "trace-name-downstream")
}

func assertCrossTypeTerminalWork(t *testing.T, response factoryapi.ListWorkResponse, name, sourceWorkID, traceID string) {
	t.Helper()
	for _, item := range response.Results {
		if item.WorkTypeName == nil || *item.WorkTypeName != "task" || item.State == nil || item.State.Name != "complete" {
			continue
		}
		if item.Name != name {
			t.Errorf("task:complete name = %q, want %q", item.Name, name)
		}
		if item.WorkId == nil || *item.WorkId == "" || *item.WorkId == sourceWorkID {
			t.Errorf("task:complete Work ID = %#v, want generated ID distinct from %q", item.WorkId, sourceWorkID)
		}
		if item.TraceId == nil || *item.TraceId != traceID {
			t.Errorf("task:complete trace ID = %#v, want %q", item.TraceId, traceID)
		}
		if item.Tags != nil && len(*item.Tags) != 0 {
			t.Errorf("task:complete tags = %#v, want empty cross-type tags", *item.Tags)
		}
		return
	}
	t.Error("listed Work has no task:complete item")
}
