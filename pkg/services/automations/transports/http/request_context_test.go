package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	automationshttp "github.com/portpowered/infinite-you/pkg/services/automations/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func waitForAutomationsRootContext(
	ctx context.Context,
) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestAutomationsRequestContextErrorResponseForTest(t *testing.T) {
	t.Parallel()

	if status, response, ok := automationshttp.AutomationsRequestContextErrorResponseForTest(context.Canceled); !ok || status != 0 || response != nil {
		t.Fatalf("canceled = (%d, %#v, %v), want (0, nil, true)", status, response, ok)
	}

	status, response, ok := automationshttp.AutomationsRequestContextErrorResponseForTest(context.DeadlineExceeded)
	if !ok || status != http.StatusGatewayTimeout {
		t.Fatalf("deadline status = %d, ok = %v, want 504 true", status, ok)
	}
	errResp, ok := response.(factoryapi.ErrorResponse)
	if !ok {
		t.Fatalf("deadline response = %#v, want ErrorResponse", response)
	}
	if errResp.Message != "automations request timed out" ||
		errResp.Family != factoryapi.ErrorFamilyInternalServerError ||
		errResp.Code != factoryapi.ErrorResponseCodeINTERNALERROR {
		t.Fatalf("deadline response = %#v, want timeout message", errResp)
	}
}

func TestRootErrorResponse_MapsRequestContextFailures(t *testing.T) {
	t.Parallel()

	status, response, ok := automationshttp.AutomationsRootErrorResponseForTest(context.Canceled)
	if !ok || status != 0 || response.Message != "" {
		t.Fatalf("canceled = (%d, %#v, %v), want handled cancel outcome", status, response, ok)
	}

	status, response, ok = automationshttp.AutomationsRootErrorResponseForTest(context.DeadlineExceeded)
	if !ok || status != http.StatusGatewayTimeout || response.Message != "automations request timed out" {
		t.Fatalf("deadline = (%d, %#v, %v), want 504 timeout outcome", status, response, ok)
	}
}

func TestAdapter_WaitSourceCanceledBeforeRootCallCompletesWithoutInvoke(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := &blockingAutomationsRootFake{
		waitSource: func(context.Context, automations.WaitSourceRequest) (automations.WaitSourceResult, error) {
			invoked = true
			return automations.WaitSourceResult{}, nil
		},
	}
	adapter := automationshttp.NewAdapterFromRoot(automationshttp.RootBinding{
		Automations: automations.Root{Operations: fake},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := adapter.WaitSource(ctx, automationshttp.WaitSourceInput{
		AutomationID: "automation-1",
		SourceID:     "source-1",
		Desired:      "running",
	})
	if invoked {
		t.Fatal("WaitSource invoked fake root after request context was canceled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitSource error = %v, want context.Canceled", err)
	}
}

func TestAdapter_WaitSourceCanceledDuringRootCallCompletesWithoutHang(t *testing.T) {
	t.Parallel()

	fake := &blockingAutomationsRootFake{
		waitSource: func(ctx context.Context, _ automations.WaitSourceRequest) (automations.WaitSourceResult, error) {
			return automations.WaitSourceResult{}, waitForAutomationsRootContext(ctx)
		},
	}
	adapter := automationshttp.NewAdapterFromRoot(automationshttp.RootBinding{
		Automations: automations.Root{Operations: fake},
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := adapter.WaitSource(ctx, automationshttp.WaitSourceInput{
			AutomationID: "automation-1",
			SourceID:     "source-1",
			Desired:      "running",
		})
		done <- err
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("WaitSource error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitSource hung after request context cancellation")
	}
}

func TestAdapter_GetStatusDeadlineExceededDuringRootCallCompletesWithoutHang(t *testing.T) {
	t.Parallel()

	fake := &blockingAutomationsRootFake{
		getStatus: func(ctx context.Context, _ automations.GetStatusRequest) (automations.GetStatusResult, error) {
			return automations.GetStatusResult{}, waitForAutomationsRootContext(ctx)
		},
	}
	adapter := automationshttp.NewAdapterFromRoot(automationshttp.RootBinding{
		Automations: automations.Root{Operations: fake},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := adapter.GetStatus(ctx, automationshttp.GetStatusInput{
			InstanceID: "instance-1",
		})
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("GetStatus error = %v, want context.DeadlineExceeded", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("GetStatus hung after request context deadline")
	}
}

func TestWriteRootOrInternalError_DoesNotMapCancelToInternalError(t *testing.T) {
	t.Parallel()

	adapter := automationshttp.NewAdapterFromRoot(automationshttp.RootBinding{
		Automations: automations.Root{Operations: &blockingAutomationsRootFake{}},
	})
	recorder := httptest.NewRecorder()

	automationshttp.WriteRootOrInternalErrorForTest(adapter, recorder, context.Canceled)

	if body := recorder.Body.String(); body != "" {
		t.Fatalf("response body = %q, want empty cancel-oriented outcome", body)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want default recorder status without encoded body", recorder.Code)
	}
}

func TestWriteRootOrInternalError_MapsDeadlineExceededToGatewayTimeout(t *testing.T) {
	t.Parallel()

	adapter := automationshttp.NewAdapterFromRoot(automationshttp.RootBinding{
		Automations: automations.Root{Operations: &blockingAutomationsRootFake{}},
	})
	recorder := httptest.NewRecorder()

	automationshttp.WriteRootOrInternalErrorForTest(adapter, recorder, context.DeadlineExceeded)

	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 Gateway Timeout", recorder.Code)
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Message != "automations request timed out" ||
		response.Code != factoryapi.ErrorResponseCodeINTERNALERROR {
		t.Fatalf("response = %s, want timeout ErrorResponse", recorder.Body.String())
	}
}

type blockingAutomationsRootFake struct {
	waitSource func(context.Context, automations.WaitSourceRequest) (automations.WaitSourceResult, error)
	getStatus  func(context.Context, automations.GetStatusRequest) (automations.GetStatusResult, error)
}

func (fake *blockingAutomationsRootFake) Reconcile(
	context.Context,
	automations.ReconcileRequest,
) (automations.ReconcileResult, error) {
	return automations.ReconcileResult{}, nil
}

func (fake *blockingAutomationsRootFake) StartSource(
	context.Context,
	automations.StartSourceRequest,
) (automations.StartSourceResult, error) {
	return automations.StartSourceResult{}, nil
}

func (fake *blockingAutomationsRootFake) StopSource(
	context.Context,
	automations.StopSourceRequest,
) (automations.StopSourceResult, error) {
	return automations.StopSourceResult{}, nil
}

func (fake *blockingAutomationsRootFake) WaitSource(
	ctx context.Context,
	request automations.WaitSourceRequest,
) (automations.WaitSourceResult, error) {
	if fake.waitSource != nil {
		return fake.waitSource(ctx, request)
	}
	return automations.WaitSourceResult{}, nil
}

func (fake *blockingAutomationsRootFake) SourceStatus(
	context.Context,
	automations.SourceStatusRequest,
) (automations.SourceStatusResult, error) {
	return automations.SourceStatusResult{}, nil
}

func (fake *blockingAutomationsRootFake) GetStatus(
	ctx context.Context,
	request automations.GetStatusRequest,
) (automations.GetStatusResult, error) {
	if fake.getStatus != nil {
		return fake.getStatus(ctx, request)
	}
	return automations.GetStatusResult{}, nil
}

func (fake *blockingAutomationsRootFake) GetCursor(
	context.Context,
	automations.GetCursorRequest,
) (automations.GetCursorResult, error) {
	return automations.GetCursorResult{}, nil
}
