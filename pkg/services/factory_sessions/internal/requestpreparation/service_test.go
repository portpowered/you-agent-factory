package requestpreparation_test

import (
	"errors"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/requestpreparation"
)

func TestStartRequestNormalization(t *testing.T) {
	prepared, err := requestpreparation.New().PrepareStart(factorysessions.StartRequest{
		RequestID: " request-1 ",
		Source: factorysessions.Source{
			Kind:      factoryruntime.WorkflowSourceKindFactoryID,
			FactoryID: " factory-1 ",
		},
	})
	if err != nil {
		t.Fatalf("PrepareStart: %v", err)
	}
	if prepared.RequestID != "request-1" || prepared.Source.FactoryID != "factory-1" {
		t.Fatalf("prepared = %#v", prepared)
	}
}

func TestStartRequestRequiresRequestID(t *testing.T) {
	_, err := requestpreparation.New().PrepareStart(factorysessions.StartRequest{
		Source: factorysessions.Source{Kind: factoryruntime.WorkflowSourceKindFactoryID, FactoryID: "factory-1"},
	})
	assertExecutionValidationError(t, err)
}

func TestStartRequestSourceKinds(t *testing.T) {
	prepare := requestpreparation.New()
	t.Run("workflow file", func(t *testing.T) {
		prepared, err := prepare.PrepareStart(factorysessions.StartRequest{
			RequestID: "request-file",
			Source: factorysessions.Source{
				Kind:         factoryruntime.WorkflowSourceKindWorkflowFile,
				WorkflowFile: " workflows/simple.workflow.js ",
			},
		})
		if err != nil || prepared.Source.WorkflowFile != "workflows/simple.workflow.js" {
			t.Fatalf("prepared = %#v, error = %v", prepared, err)
		}
	})
	t.Run("inline workflow", func(t *testing.T) {
		prepared, err := prepare.PrepareStart(factorysessions.StartRequest{
			RequestID: "request-inline",
			Source: factorysessions.Source{
				Kind: factoryruntime.WorkflowSourceKindInlineWorkflow,
				InlineWorkflow: &factorysessions.InlineWorkflowSource{
					Dialect: " dialect ", InlineSource: " return 1; ", Entrypoint: " default ",
				},
			},
		})
		if err != nil || prepared.Source.InlineWorkflow == nil || prepared.Source.InlineWorkflow.InlineSource != "return 1;" {
			t.Fatalf("prepared = %#v, error = %v", prepared, err)
		}
	})
	t.Run("missing inline workflow payload", func(t *testing.T) {
		_, err := prepare.PrepareStart(factorysessions.StartRequest{
			RequestID: "request-inline", Source: factorysessions.Source{Kind: factoryruntime.WorkflowSourceKindInlineWorkflow},
		})
		assertExecutionValidationError(t, err)
	})
}

func TestEventReconnectNormalization(t *testing.T) {
	sequence := 3
	reconnect, err := requestpreparation.New().PrepareEventReconnect(factorysessions.EventReconnectRequest{AfterEventID: " event-1 ", AfterSequence: &sequence})
	if err != nil || reconnect.AfterEventID != "event-1" || reconnect.AfterSequence == nil || *reconnect.AfterSequence != 3 {
		t.Fatalf("reconnect = %#v, error = %v", reconnect, err)
	}
}

func TestControlNormalization(t *testing.T) {
	control, err := requestpreparation.New().PrepareControl(factorysessions.ControlRequest{RequestID: " request ", Reason: " reason "})
	if err != nil || control.RequestID != "request" || control.Reason != "reason" {
		t.Fatalf("control = %#v, error = %v", control, err)
	}
}

func TestApproveNormalization(t *testing.T) {
	approve, err := requestpreparation.New().PrepareApprove(factorysessions.ApproveRequest{
		ControlRequest: factorysessions.ControlRequest{RequestID: " request "}, ApprovalPreviewID: " preview ",
	})
	if err != nil || approve.RequestID != "request" || approve.ApprovalPreviewID != "preview" {
		t.Fatalf("approve = %#v, error = %v", approve, err)
	}
}

func TestRetryDispatchNormalization(t *testing.T) {
	retry, err := requestpreparation.New().PrepareRetryDispatch(factorysessions.RetryDispatchRequest{
		ControlRequest: factorysessions.ControlRequest{RequestID: " request "}, DispatchID: " dispatch ", ForceNewAttempt: true, ResetAttemptCount: true,
	})
	if err != nil || retry.DispatchID != "dispatch" || !retry.ForceNewAttempt || !retry.ResetAttemptCount {
		t.Fatalf("retry = %#v, error = %v", retry, err)
	}
}

func TestInterruptDispatchNormalization(t *testing.T) {
	interrupt, err := requestpreparation.New().PrepareInterruptDispatch(factorysessions.InterruptDispatchRequest{
		ControlRequest: factorysessions.ControlRequest{RequestID: " request ", Reason: " reason "}, DispatchID: " dispatch ",
	})
	if err != nil || interrupt.DispatchID != "dispatch" || interrupt.RequestID != "request" || interrupt.Reason != "reason" {
		t.Fatalf("interrupt = %#v, error = %v", interrupt, err)
	}
}

func TestListSessionsDefaultsToLiveScope(t *testing.T) {
	list, err := requestpreparation.New().PrepareListSessions(factorysessions.ListSessionsRequest{})
	if err != nil || list.Scope != factorysessions.SessionListScopeLive {
		t.Fatalf("list = %#v, error = %v", list, err)
	}
}

func TestRequestPreparationValidation(t *testing.T) {
	prepare := requestpreparation.New()
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "missing retry dispatch", run: func() error {
			_, err := prepare.PrepareRetryDispatch(factorysessions.RetryDispatchRequest{})
			return err
		}},
		{name: "missing interrupt dispatch", run: func() error {
			_, err := prepare.PrepareInterruptDispatch(factorysessions.InterruptDispatchRequest{})
			return err
		}},
		{name: "unsupported list scope", run: func() error {
			_, err := prepare.PrepareListSessions(factorysessions.ListSessionsRequest{Scope: "workspace"})
			return err
		}},
		{name: "invalid result mode", run: func() error {
			_, err := prepare.PrepareResult(factorysessions.ResultRequest{Mode: "invalid"})
			return err
		}},
		{name: "negative reconnect sequence", run: func() error {
			sequence := -1
			_, err := prepare.PrepareEventReconnect(factorysessions.EventReconnectRequest{AfterSequence: &sequence})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) { assertExecutionValidationError(t, test.run()) })
	}
}

func assertExecutionValidationError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want Factory Sessions validation error")
	}
	var validationErr *factorysessions.ExecutionValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T, want ExecutionValidationError", err)
	}
}
