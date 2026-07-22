package factorysessions_test

import (
	"errors"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func TestStartRequestFromAPI_NormalizesAsyncAcceptedFixture(t *testing.T) {
	prepared, err := factorysessions.NewRequestPreparation().PrepareStart(factorysessions.StartRequest{
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

func TestStartRequestFromAPI_RejectsMissingRequestID(t *testing.T) {
	_, err := factorysessions.NewRequestPreparation().PrepareStart(factorysessions.StartRequest{
		Source: factorysessions.Source{Kind: factoryruntime.WorkflowSourceKindFactoryID, FactoryID: "factory-1"},
	})
	assertExecutionValidationError(t, err)
}

func TestStartRequestFromAPI_MapsAdditionalSourceKindsAndValidation(t *testing.T) {
	prepare := factorysessions.NewRequestPreparation()
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

func TestEventReconnectRequestFromCLI_MapsAfterEventIDAndSequence(t *testing.T) {
	sequence := 3
	prepared, err := factorysessions.NewRequestPreparation().PrepareEventReconnect(factorysessions.EventReconnectRequest{
		AfterEventID: " event-1 ", AfterSequence: &sequence,
	})
	if err != nil || prepared.AfterEventID != "event-1" || prepared.AfterSequence == nil || *prepared.AfterSequence != 3 {
		t.Fatalf("prepared = %#v, error = %v", prepared, err)
	}
}

func TestRetryDispatchRequestFromAPI_RequiresDispatchID(t *testing.T) {
	_, err := factorysessions.NewRequestPreparation().PrepareRetryDispatch(factorysessions.RetryDispatchRequest{})
	assertExecutionValidationError(t, err)
}

func TestControlRequestFromAPI_NormalizesOptionalMetadata(t *testing.T) {
	prepared, err := factorysessions.NewRequestPreparation().PrepareControl(factorysessions.ControlRequest{RequestID: " request ", Reason: " reason "})
	if err != nil || prepared.RequestID != "request" || prepared.Reason != "reason" {
		t.Fatalf("prepared = %#v, error = %v", prepared, err)
	}
}

func TestApproveRequestFromAPI_NormalizesApprovalFields(t *testing.T) {
	prepared, err := factorysessions.NewRequestPreparation().PrepareApprove(factorysessions.ApproveRequest{
		ControlRequest: factorysessions.ControlRequest{RequestID: " request "}, ApprovalPreviewID: " preview ",
	})
	if err != nil || prepared.RequestID != "request" || prepared.ApprovalPreviewID != "preview" {
		t.Fatalf("prepared = %#v, error = %v", prepared, err)
	}
}

func TestRetryDispatchRequestFromAPI_NormalizesDispatchAndFlags(t *testing.T) {
	prepared, err := factorysessions.NewRequestPreparation().PrepareRetryDispatch(factorysessions.RetryDispatchRequest{
		ControlRequest: factorysessions.ControlRequest{RequestID: " request "}, DispatchID: " dispatch ", ForceNewAttempt: true, ResetAttemptCount: true,
	})
	if err != nil || prepared.DispatchID != "dispatch" || !prepared.ForceNewAttempt || !prepared.ResetAttemptCount {
		t.Fatalf("prepared = %#v, error = %v", prepared, err)
	}
}

func TestInterruptDispatchRequestFromAPI_RequiresDispatchID(t *testing.T) {
	_, err := factorysessions.NewRequestPreparation().PrepareInterruptDispatch(factorysessions.InterruptDispatchRequest{})
	assertExecutionValidationError(t, err)
}

func TestInterruptDispatchRequestFromAPI_NormalizesDispatchAndMetadata(t *testing.T) {
	prepared, err := factorysessions.NewRequestPreparation().PrepareInterruptDispatch(factorysessions.InterruptDispatchRequest{
		ControlRequest: factorysessions.ControlRequest{RequestID: " request ", Reason: " reason "}, DispatchID: " dispatch ",
	})
	if err != nil || prepared.DispatchID != "dispatch" || prepared.RequestID != "request" || prepared.Reason != "reason" {
		t.Fatalf("prepared = %#v, error = %v", prepared, err)
	}
}

func TestListSessionsRequestFromAPI_DefaultsToLiveScope(t *testing.T) {
	prepared, err := factorysessions.NewRequestPreparation().PrepareListSessions(factorysessions.ListSessionsRequest{})
	if err != nil || prepared.Scope != factorysessions.SessionListScopeLive {
		t.Fatalf("prepared = %#v, error = %v", prepared, err)
	}
}

func TestListSessionsRequestFromAPI_RejectsUnsupportedScope(t *testing.T) {
	_, err := factorysessions.NewRequestPreparation().PrepareListSessions(factorysessions.ListSessionsRequest{Scope: "workspace"})
	assertExecutionValidationError(t, err)
}

func TestResultRequestFromAPI_RejectsInvalidMode(t *testing.T) {
	_, err := factorysessions.NewRequestPreparation().PrepareResult(factorysessions.ResultRequest{Mode: "invalid"})
	assertExecutionValidationError(t, err)
}

func TestDurableSessionMapperBoundaryValidation(t *testing.T) {
	prepare := factorysessions.NewRequestPreparation()
	t.Run("invalid result mode", func(t *testing.T) {
		_, err := prepare.PrepareResult(factorysessions.ResultRequest{Mode: "invalid"})
		assertExecutionValidationError(t, err)
	})
	t.Run("unsupported list scope", func(t *testing.T) {
		_, err := prepare.PrepareListSessions(factorysessions.ListSessionsRequest{Scope: "workspace"})
		assertExecutionValidationError(t, err)
	})
	t.Run("negative event reconnect sequence", func(t *testing.T) {
		sequence := -1
		_, err := prepare.PrepareEventReconnect(factorysessions.EventReconnectRequest{AfterSequence: &sequence})
		assertExecutionValidationError(t, err)
	})
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
