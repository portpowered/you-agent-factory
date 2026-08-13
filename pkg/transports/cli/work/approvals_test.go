package work

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestHumanApprovalCompatibilityWrappersDelegateToApprovalTransport(t *testing.T) {
	var requestPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPaths = append(requestPaths, r.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/approvals") {
			_ = json.NewEncoder(w).Encode(factoryapi.ListHumanApprovalsResponse{Approvals: []factoryapi.HumanApproval{{
				ApprovalId: "approval-1", SessionId: "session-1", Status: "PENDING",
			}}})
			return
		}
		_ = json.NewEncoder(w).Encode(factoryapi.HumanApproval{
			ApprovalId: "approval-1", SessionId: "session-1", Status: "PENDING",
		})
	}))
	defer server.Close()

	var listJSON bytes.Buffer
	if err := NewListHumanApprovals(testHTTPProtocol(t))(ListHumanApprovalsConfig{
		Context: context.Background(), Server: server.URL, SessionID: "session-1", JSON: true, Output: &listJSON,
	}); err != nil {
		t.Fatalf("NewListHumanApprovals() = %v", err)
	}
	if !strings.Contains(listJSON.String(), "approval-1") {
		t.Fatalf("wrapped list output = %q, want approval identity", listJSON.String())
	}

	var directList bytes.Buffer
	if err := ListHumanApprovals(ListHumanApprovalsConfig{
		Context: context.Background(), Server: server.URL, SessionID: "session-1", JSON: true,
		Output: &directList, HTTP: testHTTPProtocol(t),
	}); err != nil {
		t.Fatalf("ListHumanApprovals() = %v", err)
	}
	if !strings.Contains(directList.String(), "approval-1") {
		t.Fatalf("direct list output = %q, want approval identity", directList.String())
	}

	var showJSON bytes.Buffer
	if err := NewShowHumanApproval(testHTTPProtocol(t))(ShowHumanApprovalConfig{
		Context: context.Background(), Server: server.URL, SessionID: "session-1", ApprovalID: "approval-1", JSON: true, Output: &showJSON,
	}); err != nil {
		t.Fatalf("NewShowHumanApproval() = %v", err)
	}
	if !strings.Contains(showJSON.String(), "approval-1") {
		t.Fatalf("wrapped show output = %q, want approval identity", showJSON.String())
	}

	var directShow bytes.Buffer
	if err := ShowHumanApproval(ShowHumanApprovalConfig{
		Context: context.Background(), Server: server.URL, SessionID: "session-1", ApprovalID: "approval-1", JSON: true,
		Output: &directShow, HTTP: testHTTPProtocol(t),
	}); err != nil {
		t.Fatalf("ShowHumanApproval() = %v", err)
	}
	if !strings.Contains(directShow.String(), "approval-1") {
		t.Fatalf("direct show output = %q, want approval identity", directShow.String())
	}

	if len(requestPaths) != 4 || requestPaths[0] != "/factory-sessions/session-1/approvals" ||
		requestPaths[1] != "/factory-sessions/session-1/approvals" ||
		requestPaths[2] != "/factory-sessions/session-1/approvals/approval-1" ||
		requestPaths[3] != "/factory-sessions/session-1/approvals/approval-1" {
		t.Fatalf("approval wrapper request paths = %#v", requestPaths)
	}
}
