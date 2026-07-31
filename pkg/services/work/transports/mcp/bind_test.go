package workmcp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	work "github.com/portpowered/infinite-you/pkg/services/work"
	workmcp "github.com/portpowered/infinite-you/pkg/services/work/transports/mcp"
)

const testSessionID = "session-mcp-bind-001"
const testWorkID = "work-mcp-bind-001"

func TestBind_FakeRootInvokedThroughGetTool(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeWorkRoot{
		getWork: func(_ context.Context, sessionID, workID string) (work.ReadModel, error) {
			invoked = true
			if sessionID != testSessionID {
				t.Fatalf("sessionId = %q, want %q", sessionID, testSessionID)
			}
			if workID != testWorkID {
				t.Fatalf("workId = %q, want %q", workID, testWorkID)
			}
			return work.ReadModel{
				WorkID:       testWorkID,
				WorkTypeName: "task",
			}, nil
		},
	}
	operation := workmcp.NewFromRoot(workmcp.RootDependencies{Work: fake})
	raw, err := operation(
		context.Background(),
		workmcp.ToolGet,
		json.RawMessage(`{"sessionId":"`+testSessionID+`","workId":"`+testWorkID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(get) error = %v", err)
	}
	if !invoked {
		t.Fatal("fake Work root was not invoked")
	}
	var response workmcp.ToolResponse[work.ReadModel]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("CallTool(get) = %s, want success", raw)
	}
	if response.Result.WorkID != testWorkID {
		t.Fatalf("workId = %q, want %q", response.Result.WorkID, testWorkID)
	}
}

func TestBind_UnsupportedToolReturnsStableErrorWithoutInvokingFakeRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	operation := workmcp.Bind(workmcp.RootDependencies{
		Work: fakeWorkRoot{invoked: &invoked},
	})
	_, err := operation(context.Background(), "you.work.unknown", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("CallTool(unknown) error = nil, want unsupported tool error")
	}
	if !strings.Contains(err.Error(), "unsupported tool") {
		t.Fatalf("CallTool(unknown) error = %v, want unsupported tool error", err)
	}
	if invoked {
		t.Fatal("fake Work root was invoked for unknown tool")
	}
}

func TestBind_ToolOperationRejectsMissingContext(t *testing.T) {
	t.Parallel()

	operation := workmcp.BindToolOperation(fakeWorkRoot{})
	_, err := operation(nil, workmcp.ToolGet, json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "MCP request context is required") {
		t.Fatalf("ToolOperation(nil context) error = %v, want required-context error", err)
	}
}

type fakeWorkRoot struct {
	work.Service
	invoked                     *bool
	getWork                     func(context.Context, string, string) (work.ReadModel, error)
	listWork                    func(context.Context, string, work.ListOptions) (work.ListResult, error)
	moveWorkForSession          func(context.Context, string, string, string, string) (work.OperatorMoveResult, error)
	submitWorkRequestForSession func(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error)
}

func (fake fakeWorkRoot) markInvoked() {
	if fake.invoked != nil {
		*fake.invoked = true
	}
}

func (fake fakeWorkRoot) GetWork(
	ctx context.Context,
	sessionID string,
	workID string,
) (work.ReadModel, error) {
	fake.markInvoked()
	if fake.getWork == nil {
		panic("unexpected GetWork on fake Work root")
	}
	return fake.getWork(ctx, sessionID, workID)
}

func (fake fakeWorkRoot) ListWork(
	ctx context.Context,
	sessionID string,
	options work.ListOptions,
) (work.ListResult, error) {
	fake.markInvoked()
	if fake.listWork == nil {
		panic("unexpected ListWork on fake Work root")
	}
	return fake.listWork(ctx, sessionID, options)
}

func (fake fakeWorkRoot) SubmitWorkRequestForSession(
	ctx context.Context,
	sessionID string,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	fake.markInvoked()
	if fake.submitWorkRequestForSession == nil {
		panic("unexpected SubmitWorkRequestForSession on fake Work root")
	}
	return fake.submitWorkRequestForSession(ctx, sessionID, request)
}

func (fake fakeWorkRoot) MoveWorkForSession(
	ctx context.Context,
	sessionID string,
	workID string,
	stateName string,
	requestID string,
) (work.OperatorMoveResult, error) {
	fake.markInvoked()
	if fake.moveWorkForSession == nil {
		panic("unexpected MoveWorkForSession on fake Work root")
	}
	return fake.moveWorkForSession(ctx, sessionID, workID, stateName, requestID)
}
