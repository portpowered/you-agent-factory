package automations_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/automations"
)

func TestUnimplementedService_ReturnsTypedNotReadyForEveryRootSlice(t *testing.T) {
	t.Parallel()

	var service automations.Service = automations.UnimplementedService{}
	ctx := context.Background()

	err := service.StartSchedulerSidecarsForRuntime(ctx, nil, "", nil, nil, nil)
	assertTypedAutomationsError(
		t,
		"StartSchedulerSidecarsForRuntime",
		err,
		automations.ErrorCodeNotReady,
		automations.ErrNotReady,
	)
	_, err = service.Ready(ctx, automations.ReadyRequest{})
	assertTypedAutomationsError(t, "Ready", err, automations.ErrorCodeNotReady, automations.ErrNotReady)
	_, err = service.Reconcile(ctx, automations.ReconcileRequest{})
	assertTypedAutomationsError(t, "Reconcile", err, automations.ErrorCodeNotReady, automations.ErrNotReady)
	_, err = service.StartSource(ctx, automations.StartSourceRequest{})
	assertTypedAutomationsError(t, "StartSource", err, automations.ErrorCodeNotReady, automations.ErrNotReady)
	_, err = service.StopSource(ctx, automations.StopSourceRequest{})
	assertTypedAutomationsError(t, "StopSource", err, automations.ErrorCodeNotReady, automations.ErrNotReady)
	_, err = service.WaitSource(ctx, automations.WaitSourceRequest{})
	assertTypedAutomationsError(t, "WaitSource", err, automations.ErrorCodeNotReady, automations.ErrNotReady)
	_, err = service.SourceStatus(ctx, automations.SourceStatusRequest{})
	assertTypedAutomationsError(t, "SourceStatus", err, automations.ErrorCodeNotReady, automations.ErrNotReady)
	_, err = service.GetStatus(ctx, automations.GetStatusRequest{})
	assertTypedAutomationsError(t, "GetStatus", err, automations.ErrorCodeNotReady, automations.ErrNotReady)
	_, err = service.GetCursor(ctx, automations.GetCursorRequest{})
	assertTypedAutomationsError(t, "GetCursor", err, automations.ErrorCodeNotReady, automations.ErrNotReady)

	var readyService automations.Service = automations.ReadyUnimplementedService{}
	ready, err := readyService.Ready(ctx, automations.ReadyRequest{})
	if err != nil {
		t.Fatalf("ReadyUnimplementedService.Ready() unexpected error: %v", err)
	}
	if !ready.Ready {
		t.Fatal("ReadyUnimplementedService.Ready() = false, want true")
	}
	_, err = readyService.Reconcile(ctx, automations.ReconcileRequest{})
	assertTypedAutomationsError(t, "Reconcile", err, automations.ErrorCodeNotReady, automations.ErrNotReady)
}
