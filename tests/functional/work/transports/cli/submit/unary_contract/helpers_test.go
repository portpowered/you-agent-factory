package unary_contract_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	submitcli "github.com/portpowered/infinite-you/pkg/transports/cli/submit"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	unaryContractWorkType                          = "task"
	unaryContractFileWorkName                      = "unary-file-task"
	unaryContractStdinWorkName                     = "unary-stdin-task"
	unaryContractDefaultSessionWorkName            = "unary-default-session-task"
	unaryContractExplicitSessionWorkName           = "unary-explicit-session-task"
	unaryContractStructuredFailureWorkName         = "unary-structured-failure-task"
	unaryContractStructuredFailureUnsafeMessage    = "payload-secret access-token-secret"
	unaryContractStructuredFailureUnsafeCredential = "sk-proj-secret123"
)

func buildUnaryContractProcess(t *testing.T, edges serviceedges.Edges) support.Process {
	t.Helper()
	return support.BuildProcess(t, edges)
}

func executeUnarySubmitCLI(
	t *testing.T,
	process support.Process,
	serverURL string,
	workName string,
	payloadPath string,
	sessionID string,
	stdin io.Reader,
) string {
	t.Helper()

	home := t.TempDir()
	args := []string{
		"you", "--server", serverURL, "--json",
		"submit",
		"--name", workName,
		"--work-type-name", unaryContractWorkType,
		"--payload", payloadPath,
	}
	if trimmed := strings.TrimSpace(sessionID); trimmed != "" {
		args = append(args, "--session", trimmed)
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = unaryContractHomeEnvironment(home)
	inputs.Input.WorkingDirectory = home
	stdinIsTTY := stdin == nil
	inputs.Input.StdinIsTTY = &stdinIsTTY
	if stdin != nil {
		inputs.Input.Stdin = stdin
	} else {
		inputs.Input.Stdin = strings.NewReader("")
	}
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(unary submit) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	return inputs.Stdout()
}

func executeUnarySubmitExpectingFailure(
	t *testing.T,
	process support.Process,
	serverURL string,
	workName string,
	payloadPath string,
) *support.CapturedInputs {
	t.Helper()

	home := t.TempDir()
	args := []string{
		"you", "--server", serverURL,
		"submit",
		"--name", workName,
		"--work-type-name", unaryContractWorkType,
		"--payload", payloadPath,
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = unaryContractHomeEnvironment(home)
	inputs.Input.WorkingDirectory = home
	stdinIsTTY := true
	inputs.Input.StdinIsTTY = &stdinIsTTY
	inputs.Input.Stdin = strings.NewReader("")
	err := process.Execute(inputs.Input)
	if err == nil {
		t.Fatalf(
			"Process.Execute(unary submit) unexpectedly succeeded\nstdout:\n%s\nstderr:\n%s",
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	return inputs
}

func newUnaryStructuredFailureServer(t *testing.T) (*httptest.Server, func(*testing.T)) {
	t.Helper()

	var observedMethod string
	var observedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observedMethod = r.Method
		observedPath = r.URL.Path
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, fmt.Sprintf(`{
			"message":%q,
			"code":"BAD_REQUEST",
			"family":"BAD_REQUEST",
			"workId":%q
		}`,
			unaryContractStructuredFailureUnsafeMessage,
			unaryContractStructuredFailureUnsafeCredential,
		))
	}))
	t.Cleanup(server.Close)
	return server, func(t *testing.T) {
		t.Helper()
		if observedMethod != http.MethodPost {
			t.Fatalf("unary submit request method = %q, want POST", observedMethod)
		}
		wantPath := "/factory-sessions/~default/work"
		if observedPath != wantPath {
			t.Fatalf("unary submit request path = %q, want %q", observedPath, wantPath)
		}
	}
}

func assertUnarySubmitStructuredFailurePreservesPublicMessage(
	t *testing.T,
	inputs *support.CapturedInputs,
) {
	t.Helper()

	combined := strings.Join([]string{
		inputs.Stderr(),
		inputs.Stdout(),
	}, "\n")
	for _, marker := range []string{
		"submission failed (400)",
		"code=BAD_REQUEST",
		"family=BAD_REQUEST",
	} {
		if !strings.Contains(combined, marker) {
			t.Fatalf("unary submit failure output missing public backend marker %q:\nstdout:\n%s\nstderr:\n%s",
				marker, inputs.Stdout(), inputs.Stderr())
		}
	}
	for _, leaked := range []string{
		unaryContractStructuredFailureUnsafeMessage,
		"access-token",
		unaryContractStructuredFailureUnsafeCredential,
		`"traceId":`,
		`"workId":`,
		"traceId:",
		"workId:",
	} {
		if strings.Contains(combined, leaked) {
			t.Fatalf("unary submit failure output must not contain %q:\nstdout:\n%s\nstderr:\n%s",
				leaked, inputs.Stdout(), inputs.Stderr())
		}
	}
}

func writeUnaryContractPayloadFile(t *testing.T, content string) string {
	t.Helper()
	payloadPath := filepath.Join(t.TempDir(), "request.md")
	if err := os.WriteFile(payloadPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write unary payload file: %v", err)
	}
	return payloadPath
}

func decodeUnarySubmitJSON(t *testing.T, output, workName string) submitcli.SubmitSuccessResult {
	t.Helper()

	var submitted submitcli.SubmitSuccessResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &submitted); err != nil {
		t.Fatalf("decode unary submit JSON: %v\noutput:\n%s", err, output)
	}
	if submitted.TraceID == "" {
		t.Fatalf("unary submit response missing traceId: %#v", submitted)
	}
	if submitted.WorkID == nil || strings.TrimSpace(*submitted.WorkID) == "" {
		t.Fatalf("unary submit response missing workId: %#v", submitted)
	}
	if submitted.Name != workName {
		t.Fatalf("unary submit response name = %q, want %q", submitted.Name, workName)
	}
	if submitted.WorkTypeName != unaryContractWorkType {
		t.Fatalf("unary submit response workTypeName = %q, want %q", submitted.WorkTypeName, unaryContractWorkType)
	}
	return submitted
}

func assertUnarySubmitAcknowledgment(t *testing.T, output, workName string) submitcli.SubmitSuccessResult {
	t.Helper()
	return assertUnarySubmitAcknowledgmentForSession(
		t,
		output,
		workName,
		factorysessions.DefaultSessionID,
	)
}

func assertUnarySubmitAcknowledgmentForSession(
	t *testing.T,
	output,
	workName,
	sessionID string,
) submitcli.SubmitSuccessResult {
	t.Helper()

	submitted := decodeUnarySubmitJSON(t, output, workName)
	wantEndpointPath := "/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	for _, marker := range []string{
		`"traceId":`,
		`"workId":`,
		`"sessionId":"` + sessionID + `"`,
		`"endpointPath":"` + wantEndpointPath + `"`,
		workName,
		unaryContractWorkType,
	} {
		if !strings.Contains(output, marker) {
			t.Fatalf("unary submit output missing %q:\n%s", marker, output)
		}
	}
	if submitted.SessionID != sessionID {
		t.Fatalf("unary submit response sessionId = %q, want %q", submitted.SessionID, sessionID)
	}
	if submitted.EndpointPath != wantEndpointPath {
		t.Fatalf("unary submit response endpointPath = %q, want %q", submitted.EndpointPath, wantEndpointPath)
	}
	return submitted
}

func assertUnaryWorkListedAfterSubmit(t *testing.T, baseURL, workName, workID string) {
	t.Helper()
	assertUnaryWorkListedInSession(t, baseURL, factorysessions.DefaultSessionID, workName, workID)
}

func assertUnaryWorkListedInSession(t *testing.T, baseURL, sessionID, workName, workID string) {
	t.Helper()

	listed := listSessionWork(t, baseURL, sessionID)
	for _, item := range listed.Results {
		if item.Name != workName {
			continue
		}
		if support.StringPointerValue(item.WorkId) == workID {
			if support.StringPointerValue(item.WorkTypeName) != unaryContractWorkType {
				t.Fatalf(
					"public work list workTypeName = %q, want %q for session=%q name=%q workId=%q",
					support.StringPointerValue(item.WorkTypeName),
					unaryContractWorkType,
					sessionID,
					workName,
					workID,
				)
			}
			return
		}
	}
	t.Fatalf(
		"public work list missing submitted work session=%q name=%q workId=%q: %#v",
		sessionID,
		workName,
		workID,
		listed.Results,
	)
}

func listSessionWork(t *testing.T, baseURL, sessionID string) factoryapi.ListWorkResponse {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	return support.GetJSON[factoryapi.ListWorkResponse](t, endpoint)
}

func unaryContractFactoryConfig() map[string]any {
	return map[string]any{
		"name": "work-cli-submit-unary-contract",
		"workTypes": []map[string]any{
			{
				"name": unaryContractWorkType,
				"states": []map[string]any{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "mock-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process-task",
				"worker":    "mock-worker",
				"inputs":    []map[string]string{{"workType": unaryContractWorkType, "state": "init"}},
				"outputs":   []map[string]string{{"workType": unaryContractWorkType, "state": "complete"}},
				"onFailure": []map[string]string{{"workType": unaryContractWorkType, "state": "failed"}},
			},
		},
	}
}

func unaryContractHomeEnvironment(home string) []string {
	if runtime.GOOS == "windows" {
		return []string{"USERPROFILE=" + home}
	}
	if runtime.GOOS == "plan9" {
		return []string{"home=" + home}
	}
	return []string{"HOME=" + home}
}
