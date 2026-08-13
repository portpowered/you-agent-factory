package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	workcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestListHumanApprovalsJSONUsesSessionScopedAPIEnvelope(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.ListHumanApprovalsResponse{Approvals: []factoryapi.HumanApproval{{
			ApprovalId: "approval-dispatch-1", SessionId: "session/beta", DispatchId: "dispatch-1",
			WorkstationId: "approve", WorkstationName: "Approve release", Status: "PENDING",
			Decisions: []factoryapi.HumanApprovalDecisions{"APPROVE", "REJECT"}, WorkIds: []string{"work-1"},
		}}})
	}))
	defer server.Close()

	var output, diagnostics bytes.Buffer
	err := workcli.ListHumanApprovals(workcli.ListHumanApprovalsConfig{
		Context: context.Background(), Server: server.URL, SessionID: "session/beta", JSON: true,
		Output: &output, Diagnostics: &diagnostics, Verbose: true, HTTP: testHTTPProtocol(t),
	})
	if err != nil {
		t.Fatalf("ListHumanApprovals: %v", err)
	}
	if gotPath != "/factory-sessions/session%2Fbeta/approvals" {
		t.Fatalf("request path = %q", gotPath)
	}
	var got factoryapi.ListHumanApprovalsResponse
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("JSON output: %v\n%s", err, output.String())
	}
	if len(got.Approvals) != 1 || got.Approvals[0].ApprovalId != "approval-dispatch-1" {
		t.Fatalf("approvals = %#v", got.Approvals)
	}
	if strings.Contains(output.String(), "Approval ID:") {
		t.Fatalf("JSON output contains human text: %q", output.String())
	}
	if !strings.Contains(diagnostics.String(), "human approval list request") {
		t.Fatalf("diagnostics = %q", diagnostics.String())
	}
}

func TestShowHumanApprovalHumanOutputAndNextCommandDiagnostics(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.HumanApproval{
			ApprovalId: "approval/1", SessionId: "session/beta", DispatchId: "dispatch-1",
			WorkstationId: "approve", WorkstationName: "Approve release", Status: "PENDING",
			Decisions: []factoryapi.HumanApprovalDecisions{"APPROVE", "REJECT"}, WorkIds: []string{"work-1", "work-2"},
		})
	}))
	defer server.Close()

	var output, diagnostics bytes.Buffer
	err := workcli.ShowHumanApproval(workcli.ShowHumanApprovalConfig{
		Context: context.Background(), Server: server.URL, SessionID: "session/beta", ApprovalID: "approval/1",
		Output: &output, Diagnostics: &diagnostics, HTTP: testHTTPProtocol(t),
	})
	if err != nil {
		t.Fatalf("ShowHumanApproval: %v", err)
	}
	if gotPath != "/factory-sessions/session%2Fbeta/approvals/approval%2F1" {
		t.Fatalf("request path = %q", gotPath)
	}
	for _, want := range []string{"Approval ID:\tapproval/1", "Workstation:\tApprove release", "Work IDs:\twork-1, work-2", "Status:\tPENDING"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output = %q, want %q", output.String(), want)
		}
	}
	if !strings.Contains(diagnostics.String(), "human approval next command: you work approval show approval/1 --session session/beta") {
		t.Fatalf("diagnostics = %q", diagnostics.String())
	}
}

func TestListHumanApprovalsEmptyResponseIsDeterministic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"approvals":[]}`))
	}))
	defer server.Close()

	var output bytes.Buffer
	err := workcli.ListHumanApprovals(workcli.ListHumanApprovalsConfig{
		Context: context.Background(), Server: server.URL, JSON: true,
		Output: &output, HTTP: testHTTPProtocol(t),
	})
	if err != nil {
		t.Fatalf("ListHumanApprovals: %v", err)
	}
	if output.String() != "{\"approvals\":[]}\n" {
		t.Fatalf("empty output = %q", output.String())
	}
}
