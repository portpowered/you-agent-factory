package fixtures_test

import (
	"context"
	"errors"
	"testing"

	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestFakeService_PublishedTypedFailures_StartAndReadErrors(t *testing.T) {
	service, _, successRow, reconnectRow, artifactRow := seedTypedFailureService(t)
	runTypedFailureCases(t, startReadTypedFailureCases(service, successRow, reconnectRow, artifactRow))
}

func TestFakeService_PublishedTypedFailures_LifecycleErrors(t *testing.T) {
	service, runningRow, successRow, _, _ := seedTypedFailureService(t)
	runTypedFailureCases(t, lifecycleTypedFailureCases(service, runningRow, successRow))
}

func TestFakeService_MalformedRequests_DoNotMutateFixtureState(t *testing.T) {
	service := newContractFakeService(t)
	before := liveSessionCount(t, service)

	_, err := service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "",
		Source: fse.Source{
			Kind:      workflowsource.KindFactoryID,
			FactoryID: "customer-support-triage",
		},
	})
	var validationErr *fse.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "requestId" {
		t.Fatalf("StartAsync empty requestId error = %v", err)
	}
	if after := liveSessionCount(t, service); after != before {
		t.Fatalf("live sessions after malformed start = %d, want %d", after, before)
	}

	_, err = service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-unknown-scenario-typed-001",
		Source: fse.Source{
			Kind:      workflowsource.KindFactoryID,
			FactoryID: "customer-support-triage",
		},
	})
	if !errors.As(err, &validationErr) || validationErr.Field != "requestId" {
		t.Fatalf("unknown scenario error = %v", err)
	}
	if after := liveSessionCount(t, service); after != before {
		t.Fatalf("live sessions after unknown scenario = %d, want %d", after, before)
	}

	_, err = service.RetryDispatch(context.Background(), "dur-sess-missing-typed-001", fse.RetryDispatchRequest{})
	if !errors.As(err, &validationErr) || validationErr.Field != "dispatchId" {
		t.Fatalf("malformed retry error = %v", err)
	}
	if after := liveSessionCount(t, service); after != before {
		t.Fatalf("live sessions after malformed control = %d, want %d", after, before)
	}

	_, err = service.GetSession(context.Background(), "dur-sess-missing-typed-001")
	if !errors.Is(err, fse.ErrSessionNotFound) {
		t.Fatalf("unknown session error = %v", err)
	}
	if after := liveSessionCount(t, service); after != before {
		t.Fatalf("live sessions after unknown session read = %d, want %d", after, before)
	}
}
