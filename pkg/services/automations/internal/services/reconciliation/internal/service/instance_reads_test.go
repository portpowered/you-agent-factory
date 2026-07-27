package service_test

import (
	"context"
	"testing"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	reconciliation "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation"
	reconciliationwire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation/wire"
)

func TestInstanceStatusAndCursorReadAuthoritativeObservation(t *testing.T) {
	t.Parallel()

	identity := sourceIdentity("instance-reads")
	resume := automations.SourceObservation{
		Identity:   identity,
		InstanceID: "instance-read-1",
		State:      automations.ObservedLifecycleRunning,
		Cursor:     "cursor-1",
	}
	service := reconciliationwire.NewService()
	if _, err := service.StartSource(
		context.Background(),
		automations.StartSourceRequest{
			Identity: identity,
			Kind:     "watcher",
			Resume:   &resume,
		},
	); err != nil {
		t.Fatalf("StartSource resume: %v", err)
	}

	status, err := service.GetStatus(
		context.Background(),
		automations.GetStatusRequest{InstanceID: resume.InstanceID},
	)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if status.AutomationID != identity.AutomationID ||
		status.InstanceID != resume.InstanceID ||
		status.Status != resume.State {
		t.Fatalf("GetStatus = %+v, want automation/instance/state %q/%q/%q",
			status, identity.AutomationID, resume.InstanceID, resume.State)
	}

	cursor, err := service.GetCursor(
		context.Background(),
		automations.GetCursorRequest{
			InstanceID:     resume.InstanceID,
			ExpectedCursor: resume.Cursor,
		},
	)
	if err != nil {
		t.Fatalf("GetCursor: %v", err)
	}
	if cursor.AutomationID != identity.AutomationID ||
		cursor.InstanceID != resume.InstanceID ||
		cursor.Cursor != resume.Cursor {
		t.Fatalf("GetCursor = %+v, want automation/instance/cursor %q/%q/%q",
			cursor, identity.AutomationID, resume.InstanceID, resume.Cursor)
	}

	assertInstanceReadFailures(t, service, resume)
}

func assertInstanceReadFailures(
	t *testing.T,
	service reconciliation.Service,
	resume automations.SourceObservation,
) {
	t.Helper()
	_, err := service.GetCursor(
		context.Background(),
		automations.GetCursorRequest{
			InstanceID:     resume.InstanceID,
			ExpectedCursor: "stale-cursor",
		},
	)
	assertLifecycleError(t, err, automations.ErrorCodeConflict, automations.ErrConflict)
	for _, instanceID := range []string{"", " unknown-instance "} {
		_, err = service.GetStatus(
			context.Background(),
			automations.GetStatusRequest{InstanceID: instanceID},
		)
		assertLifecycleError(t, err, automations.ErrorCodeInvalid, automations.ErrInvalidRequest)
	}
	_, err = service.GetStatus(
		context.Background(),
		automations.GetStatusRequest{InstanceID: "unknown-instance"},
	)
	assertLifecycleError(t, err, automations.ErrorCodeNotFound, automations.ErrNotFound)
	_, err = service.GetCursor(
		context.Background(),
		automations.GetCursorRequest{InstanceID: " "},
	)
	assertLifecycleError(t, err, automations.ErrorCodeInvalid, automations.ErrInvalidRequest)
	_, err = service.GetCursor(
		context.Background(),
		automations.GetCursorRequest{InstanceID: "unknown-instance"},
	)
	assertLifecycleError(t, err, automations.ErrorCodeNotFound, automations.ErrNotFound)
}
