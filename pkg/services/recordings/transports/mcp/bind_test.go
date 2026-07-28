package recordingmcp_test

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	mcprecording "github.com/portpowered/infinite-you/pkg/services/recordings/transports/mcp"
)

const testRecordingID = "recording-mcp-bind-001"

func TestBind_FakeRootInvokedThroughCanonicalQueryStatusTool(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeRecordingsRoot{
		invoked: &invoked,
		queryStatus: func(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
			if request.RecordingID != testRecordingID {
				t.Fatalf("recordingId = %q, want %q", request.RecordingID, testRecordingID)
			}
			return recordings.RecordingStatusResult{
				Status: recordings.RecordingStatusFacts{
					RecordingID: testRecordingID,
					State:       recordings.RecordingActive,
				},
			}, nil
		},
	}
	operation := mcprecording.Bind(mcprecording.RootDependencies{Recordings: fake})
	raw, err := operation(
		context.Background(),
		mcprecording.ToolQueryStatus,
		json.RawMessage(`{"recordingId":"`+testRecordingID+`"}`),
	)
	if err != nil {
		t.Fatalf("CallTool(query_status) error = %v", err)
	}
	if !invoked {
		t.Fatal("fake recordings root was not invoked")
	}
	var response mcprecording.ToolResponse[recordings.RecordingStatusResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if response.Error != nil || response.Result == nil {
		t.Fatalf("CallTool(query_status) = %s, want success", raw)
	}
	if response.Result.Status.RecordingID != testRecordingID {
		t.Fatalf("recordingId = %q, want %q", response.Result.Status.RecordingID, testRecordingID)
	}
	if response.Result.Status.State != recordings.RecordingActive {
		t.Fatalf("state = %q, want ACTIVE", response.Result.Status.State)
	}
}

func TestCallTool_UnknownToolReturnsStableError(t *testing.T) {
	t.Parallel()

	_, err := mcprecording.CallTool(
		context.Background(),
		fakeRecordingsRoot{},
		"you.recording.unknown_tool",
		json.RawMessage(`{}`),
	)
	if err == nil {
		t.Fatal("CallTool(unknown tool) error = nil, want unsupported-tool error")
	}
	if got := err.Error(); got != `unsupported tool "you.recording.unknown_tool"` {
		t.Fatalf("CallTool(unknown tool) error = %q, want %q", got, `unsupported tool "you.recording.unknown_tool"`)
	}
}

func TestBind_ToolOperationRejectsMissingContext(t *testing.T) {
	t.Parallel()

	operation := mcprecording.BindToolOperation(fakeRecordingsRoot{})
	_, err := operation(nil, mcprecording.ToolQueryStatus, json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "context is required") {
		t.Fatalf("ToolOperation(nil context) error = %v, want required-context error", err)
	}
}

func TestPackageBoundary_DoesNotImportRecordingsInternal(t *testing.T) {
	t.Parallel()

	forbidden := "github.com/portpowered/infinite-you/pkg/services/recordings/internal"
	packagePath := "github.com/portpowered/infinite-you/pkg/services/recordings/transports/mcp"
	assertPackageDirectImportsForbidden(t, packagePath, []string{forbidden})
}

type fakeRecordingsRoot struct {
	recordings.Service
	invoked     *bool
	queryStatus func(recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error)
	appendEvent func(recordings.AppendRecordedEventRequest) (recordings.AppendRecordedEventResult, error)
}

func (fake fakeRecordingsRoot) markInvoked() {
	if fake.invoked != nil {
		*fake.invoked = true
	}
}

func (fake fakeRecordingsRoot) QueryRecordingStatus(
	request recordings.RecordingStatusRequest,
) (recordings.RecordingStatusResult, error) {
	fake.markInvoked()
	if fake.queryStatus == nil {
		panic("unexpected QueryRecordingStatus on fake recordings root")
	}
	return fake.queryStatus(request)
}

func (fake fakeRecordingsRoot) Append(
	request recordings.AppendRecordedEventRequest,
) (recordings.AppendRecordedEventResult, error) {
	fake.markInvoked()
	if fake.appendEvent == nil {
		panic("unexpected Append on fake recordings root")
	}
	return fake.appendEvent(request)
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
