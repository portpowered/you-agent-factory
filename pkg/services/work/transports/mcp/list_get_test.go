package workmcp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	work "github.com/portpowered/infinite-you/pkg/services/work"
	workmcp "github.com/portpowered/infinite-you/pkg/services/work/transports/mcp"
)

const testListSessionID = "session-mcp-list-001"
const testListWorkID = "work-mcp-list-001"

func TestBind_ListSuccessReturnsDetachedListResultFromInjectedRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	wantOptions := work.ListOptions{
		WorkTypeName: "story",
		MaxResults:   2,
	}
	fake := fakeWorkRoot{
		invoked: &invoked,
		listWork: func(
			_ context.Context,
			sessionID string,
			options work.ListOptions,
		) (work.ListResult, error) {
			if sessionID != testListSessionID {
				t.Fatalf("sessionId = %q, want %q", sessionID, testListSessionID)
			}
			if options != wantOptions {
				t.Fatalf("list options = %#v, want %#v", options, wantOptions)
			}
			return work.ListResult{
				Results: []work.ReadModel{{
					WorkID:       testListWorkID,
					WorkTypeName: "story",
					Name:         "list-story",
				}},
				MaxResults: 2,
				NextToken:  "opaque-token",
			}, nil
		},
	}
	operation := workmcp.Bind(workmcp.RootDependencies{Work: fake})
	raw, err := operation(
		context.Background(),
		workmcp.ToolList,
		testListInputJSON(),
	)
	if err != nil {
		t.Fatalf("CallTool(list) transport error = %v, want typed tool response", err)
	}
	if !invoked {
		t.Fatal("fake Work root was not invoked")
	}
	var response workmcp.ToolResponse[work.ListResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("tool response = %s, want success envelope", raw)
	}
	if len(response.Result.Results) != 1 || response.Result.Results[0].WorkID != testListWorkID {
		t.Fatalf("list results = %#v, want one %q item", response.Result.Results, testListWorkID)
	}
	if response.Result.MaxResults != 2 || response.Result.NextToken != "opaque-token" {
		t.Fatalf("list pagination = %#v, want maxResults=2 nextToken=opaque-token", response.Result)
	}
}

func TestBind_GetSuccessReturnsDetachedReadModelFromInjectedRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeWorkRoot{
		invoked: &invoked,
		getWork: func(_ context.Context, sessionID, workID string) (work.ReadModel, error) {
			if sessionID != testSessionID {
				t.Fatalf("sessionId = %q, want %q", sessionID, testSessionID)
			}
			if workID != testWorkID {
				t.Fatalf("workId = %q, want %q", workID, testWorkID)
			}
			return work.ReadModel{
				WorkID:       testWorkID,
				WorkTypeName: "task",
				Name:         "get-task",
			}, nil
		},
	}
	operation := workmcp.Bind(workmcp.RootDependencies{Work: fake})
	raw, err := operation(
		context.Background(),
		workmcp.ToolGet,
		testGetInputJSON(),
	)
	if err != nil {
		t.Fatalf("CallTool(get) transport error = %v, want typed tool response", err)
	}
	if !invoked {
		t.Fatal("fake Work root was not invoked")
	}
	var response workmcp.ToolResponse[work.ReadModel]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("tool response = %s, want success envelope", raw)
	}
	if response.Result.WorkID != testWorkID || response.Result.Name != "get-task" {
		t.Fatalf("read model = %#v, want workId=%q name=get-task", response.Result, testWorkID)
	}
}

func TestBind_StateAccessFailuresReturnTypedErrorEnvelopes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		toolName      string
		input         json.RawMessage
		fake          fakeWorkRoot
		wantCode      string
		wantMessage   string
		wantRetryable bool
	}{
		{
			name:     "get missing work",
			toolName: workmcp.ToolGet,
			input:    testGetInputJSON(),
			fake: fakeWorkRoot{
				getWork: func(context.Context, string, string) (work.ReadModel, error) {
					return work.ReadModel{}, work.ErrWorkNotFound
				},
			},
			wantCode:      "work.state_access.not_found",
			wantMessage:   "Work not found",
			wantRetryable: false,
		},
		{
			name:     "list invalid options",
			toolName: workmcp.ToolList,
			input:    testListInputJSON(),
			fake: fakeWorkRoot{
				listWork: func(context.Context, string, work.ListOptions) (work.ListResult, error) {
					return work.ListResult{}, &work.ValidationError{
						Field:   "state.type",
						Message: "state.type must be one of INITIAL, PROCESSING, TERMINAL, or FAILED",
					}
				},
			},
			wantCode:      "work.state_access.invalid",
			wantMessage:   "state.type must be one of INITIAL, PROCESSING, TERMINAL, or FAILED",
			wantRetryable: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var invoked bool
			tc.fake.invoked = &invoked
			operation := workmcp.Bind(workmcp.RootDependencies{Work: tc.fake})
			raw, err := operation(context.Background(), tc.toolName, tc.input)
			if err != nil {
				t.Fatalf("CallTool(%s) transport error = %v, want typed tool response", tc.toolName, err)
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

func TestBind_StateAccessFailuresAreDistinctFromAdmissionFailures(t *testing.T) {
	t.Parallel()

	notFoundRaw := mustCallGet(t, fakeWorkRoot{
		getWork: func(context.Context, string, string) (work.ReadModel, error) {
			return work.ReadModel{}, work.ErrWorkNotFound
		},
	})
	invalidListRaw := mustCallList(t, fakeWorkRoot{
		listWork: func(context.Context, string, work.ListOptions) (work.ListResult, error) {
			return work.ListResult{}, &work.ValidationError{Field: "sortBy", Message: "sortBy must be state.type"}
		},
	})
	admissionRaw := mustCallSubmit(t, fakeWorkRoot{
		submitWorkRequestForSession: func(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
			return work.WorkRequestSubmitResult{}, work.ErrInvalidWorkRequest
		},
	})

	notFoundEnvelope := assertTypedToolErrorEnvelope(t, notFoundRaw, "work.state_access.not_found", false)
	invalidListEnvelope := assertTypedToolErrorEnvelope(t, invalidListRaw, "work.state_access.invalid", false)
	admissionEnvelope := assertTypedToolErrorEnvelope(t, admissionRaw, "work.admission.invalid", false)

	for _, pair := range [][2]string{
		{notFoundEnvelope.Code, admissionEnvelope.Code},
		{invalidListEnvelope.Code, admissionEnvelope.Code},
		{notFoundEnvelope.Code, invalidListEnvelope.Code},
	} {
		if pair[0] == pair[1] {
			t.Fatalf("error codes should be distinct: %q and %q", pair[0], pair[1])
		}
	}
}

func TestBind_ListMalformedJSONReturnsDecodeErrorWithoutInvokingRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := workmcp.Bind(workmcp.RootDependencies{
		Work: fakeWorkRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		workmcp.ToolList,
		json.RawMessage(`{"sessionId":`),
	)
	if err != nil {
		t.Fatalf("CallTool(list) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false)
	if !strings.Contains(envelope.Message, "decode list work input") {
		t.Fatalf("error.message = %q, want decode list work input context", envelope.Message)
	}
	if invoked {
		t.Fatal("fake Work root was invoked for malformed JSON")
	}
}

func TestBind_GetMalformedJSONReturnsDecodeErrorWithoutInvokingRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := workmcp.Bind(workmcp.RootDependencies{
		Work: fakeWorkRoot{invoked: &invoked},
	})
	raw, err := operation(
		context.Background(),
		workmcp.ToolGet,
		json.RawMessage(`{"workId":`),
	)
	if err != nil {
		t.Fatalf("CallTool(get) transport error = %v, want typed tool response", err)
	}
	envelope := assertTypedToolErrorEnvelope(t, raw, "BAD_REQUEST", false)
	if !strings.Contains(envelope.Message, "decode get work input") {
		t.Fatalf("error.message = %q, want decode get work input context", envelope.Message)
	}
	if invoked {
		t.Fatal("fake Work root was invoked for malformed JSON")
	}
}

func mustCallList(t *testing.T, fake fakeWorkRoot) json.RawMessage {
	t.Helper()

	operation := workmcp.Bind(workmcp.RootDependencies{Work: fake})
	raw, err := operation(
		context.Background(),
		workmcp.ToolList,
		testListInputJSON(),
	)
	if err != nil {
		t.Fatalf("CallTool(list) transport error = %v", err)
	}
	return raw
}

func mustCallGet(t *testing.T, fake fakeWorkRoot) json.RawMessage {
	t.Helper()

	operation := workmcp.Bind(workmcp.RootDependencies{Work: fake})
	raw, err := operation(
		context.Background(),
		workmcp.ToolGet,
		testGetInputJSON(),
	)
	if err != nil {
		t.Fatalf("CallTool(get) transport error = %v", err)
	}
	return raw
}

func testListInputJSON() json.RawMessage {
	return json.RawMessage(`{"sessionId":"` + testListSessionID +
		`","workTypeName":"story","maxResults":2}`)
}

func testGetInputJSON() json.RawMessage {
	return json.RawMessage(`{"sessionId":"` + testSessionID + `","workId":"` + testWorkID + `"}`)
}
