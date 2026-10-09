package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type remoteInterruptParityCase struct {
	name, operation, output, body, code, phase string
	candidates                                 []map[string]any
}

func remoteInterruptParityCases() []remoteInterruptParityCase {
	candidates := []map[string]any{
		{"factorySessionId": "11111111-1111-4111-8111-111111111111", "workerSessionId": "legacy-source", "workId": "owner-work", "state": "RUNNING"},
		{"factorySessionId": "22222222-2222-4222-8222-222222222222", "workerSessionId": "legacy-source", "workId": nil, "state": "COMPLETED"},
	}
	ambiguity, _ := json.Marshal(map[string]any{"code": "WORKER_SESSION_AMBIGUOUS", "message": "controlled ambiguity", "details": map[string]any{"candidates": candidates, "private": "private-upstream-data"}})
	var cases []remoteInterruptParityCase
	for _, operation := range []string{"show", "continue", "interrupt"} {
		cases = append(cases, remoteInterruptParityCase{name: "string-" + operation, operation: operation, output: "json", body: `{"code":"EXISTING_STRING_DETAILS_CODE","message":"Controlled legacy error","details":"private-upstream-data"}`, code: "EXISTING_STRING_DETAILS_CODE"})
		for _, output := range []string{"human", "json"} {
			cases = append(cases, remoteInterruptParityCase{name: "private-" + operation + "-" + output, operation: operation, output: output, body: `{"code":"SAFE_CONFLICT","message":"Controlled legacy error","details":{"candidates":[{"factorySessionId":"private-upstream-data"}]}}`, code: "SAFE_CONFLICT"})
		}
		cases = append(cases, remoteInterruptParityCase{name: "ambiguity-" + operation, operation: operation, output: "json", body: string(ambiguity), code: "WORKER_SESSION_AMBIGUOUS", candidates: candidates})
	}
	cases = append(cases, remoteInterruptParityCase{name: "ambiguity-human", operation: "interrupt", output: "human", body: string(ambiguity), code: "WORKER_SESSION_AMBIGUOUS", candidates: candidates})
	for _, phase := range []string{"VALIDATION", "SOURCE_CANCELLATION"} {
		for _, output := range []string{"human", "json"} {
			cases = append(cases, remoteInterruptParityCase{name: "phase-" + phase + "-" + output, operation: "interrupt", output: output, body: `{"code":"EXISTING_STRING_DETAILS_CODE","message":"Controlled legacy error","details":"private-upstream-data","phase":"` + phase + `"}`, code: "EXISTING_STRING_DETAILS_CODE", phase: phase})
		}
	}
	cases = append(cases,
		remoteInterruptParityCase{name: "ambiguity-string", operation: "interrupt", output: "json", body: `{"code":"WORKER_SESSION_AMBIGUOUS","message":"Controlled legacy error","details":"private-upstream-data"}`, code: "WORKER_SESSION_AMBIGUOUS"},
		remoteInterruptParityCase{name: "malformed", operation: "interrupt", output: "json", body: `{private-upstream-data`, code: "WORKER_SESSION_INTERRUPT_CONFLICT"},
		remoteInterruptParityCase{name: "missing-message", operation: "interrupt", output: "json", body: `{"code":"PRIVATE_CODE","details":"private-upstream-data"}`, code: "WORKER_SESSION_INTERRUPT_CONFLICT"},
	)
	return cases
}

func TestRemoteInterruptErrorParity(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	for _, test := range remoteInterruptParityCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			scenario := fixture.scenario(t, "parity-"+test.name)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			source := "parity-source-" + test.name
			request := "parity-request-" + test.name
			successor := "parity-successor-" + test.name
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				assertRemoteParityRequest(t, r, test.operation, scenario.session.id, source, request, successor)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				_, _ = fmt.Fprint(w, test.body)
			}))
			t.Cleanup(server.Close)
			args := []string{"you", "--remote", "--server", server.URL, "worker-sessions", test.operation, source, "--session", scenario.session.id}
			if test.operation == "show" {
				args = []string{"you", "--remote", "--server", server.URL, "worker-sessions", "show", "--worker-session-id", source, "--session", scenario.session.id}
			}
			if test.output == "json" {
				args = append(args, "--output", "json")
			}
			if test.operation != "show" {
				flag := "--user-message"
				if test.operation == "interrupt" {
					flag = "--replacement-message"
				}
				args = append(args, "--request-id", request, "--successor-worker-session-id", successor, flag, "probe", "--async")
			}
			inputs := support.FakeInputs(ctx, args)
			inputs.Input.Env = scenario.environment()
			inputs.Input.WorkingDirectory = scenario.workingDirectory
			err := fixture.process.Execute(inputs.Input)
			if err == nil {
				t.Fatal("controlled refusal returned success")
			}
			if inputs.Stdout() != "" || calls.Load() != 1 || scenario.providerRunner.CallCount() != 0 {
				t.Fatalf("stdout=%q HTTP calls=%d provider calls=%d", inputs.Stdout(), calls.Load(), scenario.providerRunner.CallCount())
			}
			assertRemoteParityDiagnostic(t, test, inputs.Stderr(), err)
			scenario.close(t)
		})
	}
}

func assertRemoteParityRequest(t *testing.T, r *http.Request, operation, session, source, request, successor string) {
	t.Helper()
	path := "/worker-sessions/" + source + "/" + operation
	method := http.MethodPost
	if operation == "show" {
		path = "/factory-sessions/" + session + "/worker-sessions/" + source
		method = http.MethodGet
	}
	if r.Method != method || r.URL.Path != path {
		t.Errorf("request = %s %s, want %s %s", r.Method, r.URL.Path, method, path)
	}
	if operation == "show" {
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Error(err)
		return
	}
	if body["factorySessionId"] != session || body["requestId"] != request || body["successorWorkerSessionId"] != successor {
		t.Errorf("request scope/identities = %#v", body)
	}
}

func assertRemoteParityDiagnostic(t *testing.T, test remoteInterruptParityCase, stderr string, err error) {
	t.Helper()
	if strings.Contains(stderr+err.Error(), "private-upstream-data") {
		t.Fatal("private details leaked")
	}
	message := "Controlled legacy error"
	if test.candidates != nil {
		message = "controlled ambiguity"
	}
	if test.code == "WORKER_SESSION_INTERRUPT_CONFLICT" {
		message = "remote Worker Session interrupt failed (409)"
	}
	if !strings.Contains(err.Error(), message) || !strings.Contains(stderr, message) {
		t.Fatalf("message lost: stderr=%q error=%v", stderr, err)
	}
	if test.output == "human" {
		if !strings.Contains(stderr, test.code) {
			t.Fatalf("code lost: %q", stderr)
		}
		return
	}
	var diagnostic map[string]any
	decodeDirectWorkerSessionResult(t, stderr, &diagnostic)
	if diagnostic["code"] != test.code || diagnostic["message"] != message {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if test.operation == "interrupt" {
		phase := test.phase
		if phase == "" {
			phase = "VALIDATION"
		}
		if diagnostic["phase"] != phase {
			t.Fatalf("phase = %v, want %s", diagnostic["phase"], phase)
		}
	}
	if test.candidates == nil {
		if _, ok := diagnostic["details"]; ok {
			t.Fatalf("general details exposed: %#v", diagnostic)
		}
		return
	}
	want, _ := json.Marshal(map[string]any{"candidates": test.candidates})
	var expected any
	_ = json.Unmarshal(want, &expected)
	if !reflect.DeepEqual(diagnostic["details"], expected) {
		t.Fatalf("candidates = %#v, want %#v", diagnostic["details"], expected)
	}
}

// C13 protects distinct command-specific errors, interrupt phase, no local
// fallback, and safe endpoint diagnostics through the composed CLI boundary.
func TestT19RemoteWorkerAdmissionRejections(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	for _, command := range []string{"invoke", "continue", "interrupt"} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()
			scenario := fixture.scenario(t, "t19-reject-"+command)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			path, code, phase := t19RemoteRejectionContract(command)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodPost || r.URL.Path != path {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": "controlled admission rejection", "phase": phase})
			}))
			defer server.Close()
			endpoint := server.URL
			inputs := t19RemoteFileInputs(t, scenario, ctx, endpoint, "t19-rejected")
			if command != "invoke" {
				messageFlag := "--user-message"
				if command == "interrupt" {
					messageFlag = "--replacement-message"
				}
				inputs = support.FakeInputs(ctx, []string{
					"you", "--remote", "--server", endpoint, "--json", "worker-sessions", command, "t19-source",
					"--request-id", "t19-rejected-request", "--successor-worker-session-id", "t19-rejected-successor", messageFlag, "replacement prompt",
				})
				inputs.Input.Env = scenario.environment()
				inputs.Input.WorkingDirectory = scenario.workingDirectory
			}
			err := fixture.process.Execute(inputs.Input)
			if err == nil {
				t.Fatal("remote rejection returned success")
			}
			assertDirectWorkerSessionCLIError(t, inputs, code)
			if inputs.Stdout() != "" || calls.Load() != 1 || scenario.providerRunner.CallCount() != 0 {
				t.Fatalf("rejection stdout=%q remote calls=%d local calls=%d", inputs.Stdout(), calls.Load(), scenario.providerRunner.CallCount())
			}
			if strings.Contains(inputs.Stderr()+err.Error(), endpoint) {
				t.Fatalf("rejection disclosed endpoint URL")
			}
			if command == "interrupt" {
				var response struct {
					Phase string `json:"phase"`
				}
				decodeDirectWorkerSessionResult(t, inputs.Stderr(), &response)
				if response.Phase != phase {
					t.Fatalf("interrupt phase = %q, want %q", response.Phase, phase)
				}
			}
			scenario.close(t)
		})
	}
}

func t19RemoteRejectionContract(command string) (path, code, phase string) {
	switch command {
	case "invoke":
		return "/worker-sessions", "WORKER_SESSION_ADMISSION_FAILED", ""
	case "continue":
		return "/worker-sessions/t19-source/continue", "WORKER_SESSION_CONTINUATION_ADMISSION_FAILED", ""
	default:
		return "/worker-sessions/t19-source/interrupt", "WORKER_SESSION_INTERRUPT_SOURCE_CANCELLATION_FAILED", "SOURCE_CANCELLATION"
	}
}
