package unary_contract_test

import (
	"encoding/json"
	"io"
	"net/url"
	"runtime"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	submitcli "github.com/portpowered/infinite-you/pkg/transports/cli/submit"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	unaryContractWorkType                 = "task"
	unaryContractFileWorkName             = "unary-file-task"
	unaryContractStdinWorkName            = "unary-stdin-task"
	unaryContractDefaultSessionWorkName   = "unary-default-session-task"
	unaryContractExplicitSessionWorkName  = "unary-explicit-session-task"
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
