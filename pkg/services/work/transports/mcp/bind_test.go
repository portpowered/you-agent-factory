package workmcp_test

import (
	"context"
	"encoding/json"
	"os/exec"
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
				WorkID:   testWorkID,
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

func TestPackageBoundary_DoesNotImportWorkInternal(t *testing.T) {
	t.Parallel()

	forbidden := "github.com/portpowered/infinite-you/pkg/services/work/internal"
	packagePath := "github.com/portpowered/infinite-you/pkg/services/work/transports/mcp"
	assertPackageDirectImportsForbidden(t, packagePath, []string{forbidden})
}

type fakeWorkRoot struct {
	work.Service
	invoked *bool
	getWork func(context.Context, string, string) (work.ReadModel, error)
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

func assertPackageDirectImportsForbidden(t *testing.T, packagePath string, forbiddenRoots []string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		for _, forbidden := range forbiddenRoots {
			if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
				t.Fatalf("%s must not import forbidden ownership %s; found direct import %s", packagePath, forbidden, importPath)
			}
		}
	}
}
