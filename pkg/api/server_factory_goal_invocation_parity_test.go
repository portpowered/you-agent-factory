package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/invocations"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

const namedGoalParityText = "Plan the sprint from CLI and API parity coverage"

func TestFactorySessionsAPI_NamedGoalInvocationSuccessMatchesCLIContract(t *testing.T) {
	mock := &testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"~default": {},
		},
		InvokeFactoryResult: apisurface.FactoryInvocationResult{
			RequestID: "request-goal-parity-success",
			TraceID:   "trace-goal-parity-success",
			Status:    factoryapi.InvocationTerminalStatusCompleted,
			PrimaryResult: []interfaces.WorkContentPart{{
				Type: interfaces.WorkContentPartTypeText,
				Text: "goal parity completed",
			}},
		},
	}
	srv := newTestServer(mock)

	body := `{"sourceKind":"text","content":[{"type":"text","text":"` + namedGoalParityText + `"}]}`
	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/~default/invocations", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /factory-sessions/~default/invocations status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	response := decodeJSONResponse[factoryapi.InvocationResponse](t, rec)
	if response.RequestId != mock.InvokeFactoryResult.RequestID {
		t.Fatalf("requestId = %q, want %q", response.RequestId, mock.InvokeFactoryResult.RequestID)
	}
	if response.TraceId != mock.InvokeFactoryResult.TraceID {
		t.Fatalf("traceId = %q, want %q", response.TraceId, mock.InvokeFactoryResult.TraceID)
	}
	if response.Status != mock.InvokeFactoryResult.Status {
		t.Fatalf("status = %q, want %q", response.Status, mock.InvokeFactoryResult.Status)
	}
	assertGeneratedWorkContentParts(t, response.PrimaryResult, mock.InvokeFactoryResult.PrimaryResult)

	if len(mock.InvokedFactorySessions) != 1 {
		t.Fatalf("invoked factory sessions = %d, want 1", len(mock.InvokedFactorySessions))
	}
	if got := extractInvocationRequestText(t, &mock.InvokedFactorySessions[0]); got != namedGoalParityText {
		t.Fatalf("invocation text = %q, want %q", got, namedGoalParityText)
	}
}

func TestFactorySessionsAPI_NamedGoalInvocationSourceConflictMatchesCLIContract(t *testing.T) {
	conflictMessage := "invocation input sources conflict: positional_text, stdin_text"
	srv := newTestServer(&testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"~default": {},
		},
		InvokeFactoryErr: &invocations.InputError{
			Code:    invocations.InputErrorCodeSourceConflict,
			Message: conflictMessage,
		},
	})

	body := `{"sourceKind":"text","content":[{"type":"text","text":"` + namedGoalParityText + `"}]}`
	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/~default/invocations", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(
		t,
		rec,
		http.StatusBadRequest,
		string(invocations.InputErrorCodeSourceConflict),
		conflictMessage,
	)
}

func extractInvocationRequestText(t *testing.T, request *factoryapi.InvocationRequest) string {
	t.Helper()

	if request == nil {
		t.Fatal("invocation request = nil")
	}
	if len(request.Content) != 1 {
		t.Fatalf("content parts = %d, want 1", len(request.Content))
	}
	part, err := request.Content[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("AsWorkTextContentPart: %v", err)
	}
	return part.Text
}
