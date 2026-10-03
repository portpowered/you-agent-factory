package acceptance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

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
