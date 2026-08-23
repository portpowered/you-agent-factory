//go:build functionallong

package runtime_api

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type sleepyExecutor struct{ sleep time.Duration }

func (e *sleepyExecutor) Execute(_ context.Context, d work.WorkDispatch) (workerexecution.WorkResult, error) {
	time.Sleep(e.sleep)
	return workerexecution.WorkResult{DispatchID: d.DispatchID, TransitionID: d.TransitionID, Outcome: workerexecution.OutcomeAccepted}, nil
}

func requireGeneratedSchemaRunStartedPayload(t *testing.T, events []factoryapi.FactoryEvent) factoryapi.RunRequestEventPayload {
	t.Helper()

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeRunRequest {
			continue
		}
		payload, err := event.Payload.AsRunRequestEventPayload()
		if err != nil {
			t.Fatalf("decode run-request payload %q: %v", event.Id, err)
		}
		if payload.Factory.WorkTypes == nil || len(*payload.Factory.WorkTypes) == 0 {
			t.Fatalf("run-request payload factory missing work types: %#v", payload.Factory)
		}
		return payload
	}
	t.Fatalf("recorded events missing RUN_REQUEST: %#v", functionalEventTypes(events))
	return factoryapi.RunRequestEventPayload{}
}
