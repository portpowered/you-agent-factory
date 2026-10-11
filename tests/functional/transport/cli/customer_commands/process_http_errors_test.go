package customer_commands_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func testCLIHTTPErrorPropagation(t *testing.T) {
	process := plainProcess(t)
	for _, tc := range []cliHTTPScenario{
		{id: "F01", status: 404, code: "NOT_FOUND", family: "NOT_FOUND", message: "factory session not found", hint: true},
		{id: "F02", status: 200, success: true},
		{id: "F03", status: 200, success: true, empty: true},
		{id: "F04", status: 500, code: "INTERNAL_ERROR", family: "INTERNAL_SERVER_ERROR", message: "synthetic internal failure"},
		{id: "F05", status: 500},
		{id: "F06", command: "session", status: 400, code: "BAD_REQUEST", family: "BAD_REQUEST", message: "invalid scope", details: "server guidance"},
		{id: "F07", command: "move", status: 409, code: "CONFLICT", family: "CONFLICT", message: "move rejected"},
		{id: "F08", command: "watch", status: 404, code: "NOT_FOUND", family: "NOT_FOUND", message: "factory session not found"},
		{id: "F09", status: 400, code: "BAD_REQUEST", family: "BAD_REQUEST", message: "page rejected", paginated: true},
		{id: "F10", status: 404, code: "NOT_FOUND", family: "NOT_FOUND", message: "factory session not found", details: "original server guidance"},
		{id: "F11", status: 404, code: "NOT_FOUND", family: "NOT_FOUND", message: "factory session not found", hint: true, human: true},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			sessionID := uuid.NewString()
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tc.serve(t, sessionID, calls.Add(1), w, r)
			}))
			t.Cleanup(server.Close)
			args := tc.args(server.URL, sessionID)
			result := newCLIHome(t).run(t, process, nil, args...)
			if tc.success {
				tc.assertSuccess(t, result)
			} else {
				tc.assertFailure(t, result)
			}
			wantCalls := int32(1)
			if tc.command == "move" || tc.paginated {
				wantCalls = 2
			}
			if calls.Load() != wantCalls {
				t.Fatalf("requests = %d, want %d", calls.Load(), wantCalls)
			}
		})
	}
}

type cliHTTPScenario struct {
	id, command, code, family, message     string
	status                                 int
	details                                any
	hint, human, success, empty, paginated bool
}

func (tc cliHTTPScenario) serve(t *testing.T, sessionID string, call int32, w http.ResponseWriter, r *http.Request) {
	workPath := "/factory-sessions/" + sessionID + "/work"
	w.Header().Set("Content-Type", "application/json")
	expectedPath, method := workPath, http.MethodGet
	switch tc.command {
	case "session":
		expectedPath = "/factory-sessions"
	case "watch":
		expectedPath = "/factory-sessions/" + sessionID + "/events"
	case "move":
		expectedPath += "/work-1"
		if call == 2 {
			expectedPath += "/move"
			method = http.MethodPost
		}
	}
	if r.URL.Path != expectedPath || r.Method != method {
		t.Errorf("request = %s %s, want %s %s", r.Method, r.URL.Path, method, expectedPath)
		w.WriteHeader(500)
		return
	}
	if tc.command == "move" && call == 1 {
		_, _ = io.WriteString(w, `{"name":"sample","workId":"work-1","state":{"name":"init","type":"INITIAL"}}`)
		return
	}
	if tc.paginated && call == 1 {
		_, _ = io.WriteString(w, `{"results":[{"name":"sample"}],"paginationContext":{"maxResults":1,"nextToken":"private-continuation"}}`)
		return
	}
	if tc.paginated && r.URL.Query().Get("nextToken") != "private-continuation" {
		t.Error("continuation was not sent")
	}
	w.WriteHeader(tc.status)
	if tc.success {
		works := []factoryapi.Work{}
		if !tc.empty {
			works = append(works, factoryapi.Work{Name: "sample"})
		}
		_ = json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{Results: works, Counts: &factoryapi.ListWorkCountSummary{Total: len(works)}})
	} else if tc.code == "" {
		_, _ = io.WriteString(w, "private malformed response")
	} else {
		_ = json.NewEncoder(w).Encode(factoryapi.ErrorResponse{Code: factoryapi.ErrorResponseCode(tc.code), Family: factoryapi.ErrorFamily(tc.family), Message: tc.message, Details: tc.details})
	}
}

func (tc cliHTTPScenario) args(serverURL, sessionID string) []string {
	args := []string{"--server", serverURL}
	if !tc.human {
		args = append(args, "--json")
	}
	switch tc.command {
	case "session":
		args = append(args, "session", "list")
	case "move":
		args = append(args, "work", "move", "work-1", "ready", "--session", sessionID)
	case "watch":
		args = append(args, "work", "watch", "--session", sessionID)
	default:
		args = append(args, "work", "list", "--all", "--counts", "--session", sessionID)
	}
	return args
}

func (tc cliHTTPScenario) assertSuccess(t *testing.T, result cliResult) {
	t.Helper()

	if result.Err != nil || result.Stderr != "" {
		t.Fatalf("success result = %#v", result)
	}
	var response factoryapi.ListWorkResponse
	if err := json.Unmarshal([]byte(result.Stdout), &response); err != nil {
		t.Fatal(err)
	}
	want := 1
	if tc.empty {
		want = 0
	}
	if len(response.Results) != want || response.Counts == nil || response.Counts.Total != want {
		t.Fatalf("response = %#v", response)
	}
	if want == 1 && response.Results[0].Name != "sample" {
		t.Fatal("work was lost")
	}
}

func (tc cliHTTPScenario) assertFailure(t *testing.T, result cliResult) {
	t.Helper()

	if result.Err == nil || result.Stdout != "" {
		t.Fatalf("failure result = %#v", result)
	}
	var diagnostic factoryapi.ErrorResponse
	decoder := json.NewDecoder(strings.NewReader(result.Stderr))
	if err := decoder.Decode(&diagnostic); err != nil {
		t.Fatalf("diagnostic: %v; %s", err, result.Stderr)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("extra diagnostic: %v", err)
	}
	code, family, message := tc.code, tc.family, tc.message
	if code == "" {
		code, family, message = "CLI_COMMAND_FAILED", "INTERNAL_SERVER_ERROR", "command failed"
	}
	if string(diagnostic.Code) != code || string(diagnostic.Family) != family || diagnostic.Message != message {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if tc.hint {
		details, ok := diagnostic.Details.(map[string]any)
		if !ok || !strings.Contains(details["hint"].(string), "you session list") {
			t.Fatalf("hint = %#v", diagnostic.Details)
		}
	} else if !reflect.DeepEqual(diagnostic.Details, tc.details) {
		t.Fatalf("details = %#v, want %#v", diagnostic.Details, tc.details)
	}
	if strings.Contains(result.Stderr, "private") {
		t.Fatal("private response or continuation leaked")
	}
}
