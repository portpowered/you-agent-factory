package runtime_api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestServiceModeSmoke_PublicSessionEventsAndWorkStayReachableUntilCanceled(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-mode lifecycle smoke")
	host, release := newPublicServiceModeHost(t)
	stream := support.OpenDefaultSessionFactoryEventStream(t, host.Client(), host.URL())
	requirePublicEventPrelude(t, stream)

	traceID := submitPublicServiceModeWork(t, host)
	request := requireEvent(t, stream, factoryapi.FactoryEventTypeWorkRequest, 10*time.Second)
	dispatch := requireEvent(t, stream, factoryapi.FactoryEventTypeDispatchRequest, 10*time.Second)
	if request.Context.Sequence >= dispatch.Context.Sequence {
		t.Fatalf("public event order = work-request:%d dispatch-request:%d, want increasing", request.Context.Sequence, dispatch.Context.Sequence)
	}
	if !containsString(pointerSliceValue(dispatch.Context.TraceIds), traceID) {
		t.Fatalf("dispatch event trace IDs = %#v, want %q", dispatch.Context.TraceIds, traceID)
	}

	close(release)
	response := requireEvent(t, stream, factoryapi.FactoryEventTypeDispatchResponse, 10*time.Second)
	if dispatch.Context.Sequence >= response.Context.Sequence {
		t.Fatalf("public lifecycle event order = dispatch:%d response:%d, want increasing", dispatch.Context.Sequence, response.Context.Sequence)
	}
	waitForPublicCompletedWork(t, host, traceID, 10*time.Second)
	requirePublicStatus(t, host, http.StatusOK)
	stream.Close()
	host.Stop(t)
	select {
	case <-host.Done():
	default:
		t.Fatal("composed host remains running after explicit cancellation")
	}
}

func newPublicServiceModeHost(t *testing.T) (*support.ComposedFunctionalHTTPHost, chan struct{}) {
	t.Helper()
	dir := support.ScaffoldFactory(t, twoStagePipelineConfig())
	release := make(chan struct{})
	blocking := &publicBlockingExecutor{release: release}
	host := support.StartComposedFunctionalHTTPHost(t, support.ComposedFunctionalHTTPHostConfig{
		FactoryDir:  dir,
		RuntimeMode: interfaces.RuntimeModeService,
		ExtraOptions: []factory.FactoryOption{
			factory.WithServiceMode(),
			factory.WithWorkerExecutor("worker-a", blocking),
			factory.WithWorkerExecutor("worker-b", publicStaticOutcomeExecutor{}),
		},
	})
	return host, release
}

func requirePublicEventPrelude(t *testing.T, stream *support.FactorySessionEventStream) {
	t.Helper()
	first := stream.Next(5 * time.Second)
	second := stream.Next(5 * time.Second)
	if first.Type != factoryapi.FactoryEventTypeRunRequest || second.Type != factoryapi.FactoryEventTypeInitialStructureRequest {
		t.Fatalf("public session prelude = %#v, %#v; want RUN_REQUEST then INITIAL_STRUCTURE_REQUEST", first.Type, second.Type)
	}
	if first.Context.Sequence >= second.Context.Sequence {
		t.Fatalf("public session prelude sequences = %d, %d; want increasing", first.Context.Sequence, second.Context.Sequence)
	}
}

func submitPublicServiceModeWork(t *testing.T, host *support.ComposedFunctionalHTTPHost) string {
	t.Helper()
	payload, err := json.Marshal(factoryapi.SubmitWorkRequest{Name: "service-mode smoke item", WorkTypeName: "task", Payload: map[string]string{"title": "service-mode smoke item"}})
	if err != nil {
		t.Fatalf("marshal submit work: %v", err)
	}
	resp, err := host.Client().Post(support.DefaultSessionWorkURL(host.URL(), "/work"), "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST public work endpoint: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST public work endpoint status = %d, want 201", resp.StatusCode)
	}
	var result factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode submit work response: %v", err)
	}
	if result.TraceId == "" {
		t.Fatal("POST public work endpoint returned an empty trace ID")
	}
	return result.TraceId
}

func requireEvent(t *testing.T, stream *support.FactorySessionEventStream, want factoryapi.FactoryEventType, timeout time.Duration) factoryapi.FactoryEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		event := stream.Next(time.Until(deadline))
		if event.Type == want {
			return event
		}
	}
	t.Fatalf("timed out waiting for public event type %q", want)
	return factoryapi.FactoryEvent{}
}

func waitForPublicCompletedWork(t *testing.T, host *support.ComposedFunctionalHTTPHost, traceID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		resp, err := host.Client().Get(support.DefaultSessionWorkURL(host.URL(), "/work"))
		if err != nil {
			t.Fatalf("GET public work endpoint: %v", err)
		}
		var work factoryapi.ListWorkResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&work)
		status := resp.StatusCode
		resp.Body.Close()
		if decodeErr != nil {
			t.Fatalf("decode public work response: %v", decodeErr)
		}
		if status != http.StatusOK {
			t.Fatalf("GET public work endpoint status = %d, want 200", status)
		}
		if len(work.Results) == 1 && pointerString(work.Results[0].TraceId) == traceID && work.Results[0].State != nil && work.Results[0].State.Name == "complete" && work.Results[0].State.Type == factoryapi.WorkStateTypeTERMINAL {
			return
		}
		select {
		case <-t.Context().Done():
			t.Fatalf("waiting for completed public work canceled: %v", t.Context().Err())
		case <-ticker.C:
		case <-time.After(time.Until(deadline)):
			t.Fatalf("timed out waiting for completed public work for trace %q; last response = %#v", traceID, work.Results)
		}
	}
}

func requirePublicStatus(t *testing.T, host *support.ComposedFunctionalHTTPHost, wantStatus int) {
	t.Helper()
	response, err := host.Client().Get(host.URL() + "/status")
	if err != nil {
		t.Fatalf("GET /status: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != wantStatus {
		t.Fatalf("GET /status status = %d, want %d", response.StatusCode, wantStatus)
	}
}

func pointerSliceValue(values *[]string) []string {
	if values == nil {
		return nil
	}
	return *values
}

func pointerString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type publicBlockingExecutor struct {
	release <-chan struct{}
}

func (executor *publicBlockingExecutor) Execute(ctx context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	select {
	case <-executor.release:
	case <-ctx.Done():
		return interfaces.WorkResult{}, ctx.Err()
	}
	return interfaces.WorkResult{DispatchID: dispatch.DispatchID, TransitionID: dispatch.TransitionID, Outcome: interfaces.OutcomeAccepted}, nil
}

type publicStaticOutcomeExecutor struct{}

func (publicStaticOutcomeExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	return interfaces.WorkResult{DispatchID: dispatch.DispatchID, TransitionID: dispatch.TransitionID, Outcome: interfaces.OutcomeAccepted}, nil
}
