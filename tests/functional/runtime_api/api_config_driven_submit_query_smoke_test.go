package runtime_api

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestConfigDriven_RESTAPISubmitAndQuery(t *testing.T) {
	support.SkipLongFunctional(t, "slow config-driven runtime API submit/query smoke")

	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	server := startFunctionalServer(t, dir, true, factory.WithServiceMode())
	traceID := submitGeneratedWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         "rest-submit",
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "REST submit"},
	})
	if traceID == "" {
		t.Fatal("POST /work returned an empty trace ID")
	}

	listResp := waitForGeneratedWorkComplete(t, server.URL(), traceID, 10*time.Second)
	if len(listResp.Results) != 1 {
		t.Fatalf("GET /work result count = %d, want 1", len(listResp.Results))
	}
	work := listResp.Results[0]
	if stringPointerValue(work.WorkTypeName) != "task" {
		t.Errorf("GET /work work type = %q, want task", stringPointerValue(work.WorkTypeName))
	}
	if generatedWorkStateName(work.State) != "complete" || generatedWorkStateType(work.State) != factoryapi.WorkStateTypeTERMINAL {
		t.Errorf("GET /work state = %#v, want complete/TERMINAL", work.State)
	}
}
