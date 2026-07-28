package workmcp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	work "github.com/portpowered/infinite-you/pkg/services/work"
	workmcp "github.com/portpowered/infinite-you/pkg/services/work/transports/mcp"
)

const testMoveSessionID = "session-mcp-move-001"
const testMoveWorkID = "work-mcp-move-001"
const testMoveStateName = "review"
const testMoveRequestID = "request-mcp-move-001"

func TestBind_MoveSuccessReturnsDetachedOperatorMoveResultFromInjectedRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeWorkRoot{
		invoked: &invoked,
		moveWorkForSession: func(
			_ context.Context,
			sessionID string,
			workID string,
			stateName string,
			requestID string,
		) (work.OperatorMoveResult, error) {
			if sessionID != testMoveSessionID {
				t.Fatalf("sessionId = %q, want %q", sessionID, testMoveSessionID)
			}
			if workID != testMoveWorkID {
				t.Fatalf("workId = %q, want %q", workID, testMoveWorkID)
			}
			if stateName != testMoveStateName {
				t.Fatalf("stateName = %q, want %q", stateName, testMoveStateName)
			}
			if requestID != testMoveRequestID {
				t.Fatalf("requestId = %q, want %q", requestID, testMoveRequestID)
			}
			return work.OperatorMoveResult{
				WorkID:     testMoveWorkID,
				WorkTypeID: "story",
				FromState:  "draft",
				ToState:    testMoveStateName,
			}, nil
		},
	}
	operation := workmcp.Bind(workmcp.RootDependencies{Work: fake})
	raw, err := operation(
		context.Background(),
		workmcp.ToolMove,
		testMoveInputJSON(),
	)
	if err != nil {
		t.Fatalf("CallTool(move) transport error = %v, want typed tool response", err)
	}
	if !invoked {
		t.Fatal("fake Work root was not invoked")
	}
	var response workmcp.ToolResponse[work.OperatorMoveResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("tool response = %s, want success envelope", raw)
	}
	if response.Result.WorkID != testMoveWorkID {
		t.Fatalf("workId = %q, want %q", response.Result.WorkID, testMoveWorkID)
	}
	if response.Result.FromState != "draft" || response.Result.ToState != testMoveStateName {
		t.Fatalf("move states = %#v, want draft -> %q", response.Result, testMoveStateName)
	}
}

func TestBind_MoveFailuresReturnTypedErrorEnvelopes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		fake          fakeWorkRoot
		wantCode      string
		wantMessage   string
		wantRetryable bool
	}{
		{
			name: "missing work",
			fake: fakeWorkRoot{
				moveWorkForSession: func(context.Context, string, string, string, string) (work.OperatorMoveResult, error) {
					return work.OperatorMoveResult{}, work.ErrWorkNotFound
				},
			},
			wantCode:      "work.state_access.not_found",
			wantMessage:   "Work not found",
			wantRetryable: false,
		},
		{
			name: "already applied",
			fake: fakeWorkRoot{
				moveWorkForSession: func(context.Context, string, string, string, string) (work.OperatorMoveResult, error) {
					return work.OperatorMoveResult{}, work.ErrMoveWorkRequestAlreadyApplied
				},
			},
			wantCode:      "work.state_access.already_applied",
			wantMessage:   "Operator move request was already applied",
			wantRetryable: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var invoked bool
			tc.fake.invoked = &invoked
			operation := workmcp.Bind(workmcp.RootDependencies{Work: tc.fake})
			raw, err := operation(context.Background(), workmcp.ToolMove, testMoveInputJSON())
			if err != nil {
				t.Fatalf("CallTool(move) transport error = %v, want typed tool response", err)
			}
			if !invoked {
				t.Fatal("fake Work root was not invoked")
			}
			envelope := assertTypedToolErrorEnvelope(t, raw, tc.wantCode, tc.wantRetryable)
			if envelope.Message != tc.wantMessage {
				t.Fatalf("error.message = %q, want %q; envelope = %#v", envelope.Message, tc.wantMessage, envelope)
			}
		})
	}
}

func TestBind_MoveFailuresAreDistinctFromListGetAndAdmissionFailures(t *testing.T) {
	t.Parallel()

	notFoundRaw := mustCallMove(t, fakeWorkRoot{
		moveWorkForSession: func(context.Context, string, string, string, string) (work.OperatorMoveResult, error) {
			return work.OperatorMoveResult{}, work.ErrWorkNotFound
		},
	})
	alreadyAppliedRaw := mustCallMove(t, fakeWorkRoot{
		moveWorkForSession: func(context.Context, string, string, string, string) (work.OperatorMoveResult, error) {
			return work.OperatorMoveResult{}, work.ErrMoveWorkRequestAlreadyApplied
		},
	})
	listNotFoundRaw := mustCallList(t, fakeWorkRoot{
		listWork: func(context.Context, string, work.ListOptions) (work.ListResult, error) {
			return work.ListResult{}, work.ErrWorkNotFound
		},
	})
	admissionRaw := mustCallSubmit(t, fakeWorkRoot{
		submitWorkRequestForSession: func(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
			return work.WorkRequestSubmitResult{}, work.ErrInvalidWorkRequest
		},
	})

	notFoundEnvelope := assertTypedToolErrorEnvelope(t, notFoundRaw, "work.state_access.not_found", false)
	alreadyAppliedEnvelope := assertTypedToolErrorEnvelope(t, alreadyAppliedRaw, "work.state_access.already_applied", false)
	listNotFoundEnvelope := assertTypedToolErrorEnvelope(t, listNotFoundRaw, "work.state_access.not_found", false)
	admissionEnvelope := assertTypedToolErrorEnvelope(t, admissionRaw, "work.admission.invalid", false)

	for _, pair := range [][2]string{
		{alreadyAppliedEnvelope.Code, notFoundEnvelope.Code},
		{alreadyAppliedEnvelope.Code, admissionEnvelope.Code},
		{notFoundEnvelope.Code, admissionEnvelope.Code},
		{alreadyAppliedEnvelope.Code, listNotFoundEnvelope.Code},
	} {
		if pair[0] == pair[1] && pair[0] != "work.state_access.not_found" {
			t.Fatalf("error codes should be distinct: %q and %q", pair[0], pair[1])
		}
	}
	if alreadyAppliedEnvelope.Code == listNotFoundEnvelope.Code {
		t.Fatalf("already-applied code %q should differ from list/get not-found code %q",
			alreadyAppliedEnvelope.Code, listNotFoundEnvelope.Code)
	}
}

func TestBind_MoveMalformedJSONReturnsDecodeErrorWithoutInvokingRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := workmcp.Bind(workmcp.RootDependencies{
		Work: fakeWorkRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		workmcp.ToolMove,
		json.RawMessage(`{"sessionId":`),
	)
	if err != nil {
		t.Fatalf("CallTool(move) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false)
	if !strings.Contains(envelope.Message, "decode move work input") {
		t.Fatalf("error.message = %q, want decode move work input context", envelope.Message)
	}
	if invoked {
		t.Fatal("fake Work root was invoked for malformed JSON")
	}
}

func mustCallMove(t *testing.T, fake fakeWorkRoot) json.RawMessage {
	t.Helper()

	operation := workmcp.Bind(workmcp.RootDependencies{Work: fake})
	raw, err := operation(
		context.Background(),
		workmcp.ToolMove,
		testMoveInputJSON(),
	)
	if err != nil {
		t.Fatalf("CallTool(move) transport error = %v", err)
	}
	return raw
}

func testMoveInputJSON() json.RawMessage {
	return json.RawMessage(`{"sessionId":"` + testMoveSessionID +
		`","workId":"` + testMoveWorkID +
		`","stateName":"` + testMoveStateName +
		`","requestId":"` + testMoveRequestID + `"}`)
}
