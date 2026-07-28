package unary_contract_test

import (
	"encoding/json"
	"io"
	"runtime"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	submitcli "github.com/portpowered/infinite-you/pkg/transports/cli/submit"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	unaryContractWorkType      = "task"
	unaryContractFileWorkName  = "unary-file-task"
	unaryContractStdinWorkName = "unary-stdin-task"
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
	stdin io.Reader,
) string {
	t.Helper()

	home := t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", serverURL, "--json",
		"submit",
		"--name", workName,
		"--work-type-name", unaryContractWorkType,
		"--payload", payloadPath,
	})
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

	submitted := decodeUnarySubmitJSON(t, output, workName)
	for _, marker := range []string{
		`"traceId":`,
		`"workId":`,
		`"endpointPath":"/factory-sessions/~default/work"`,
		workName,
		unaryContractWorkType,
	} {
		if !strings.Contains(output, marker) {
			t.Fatalf("unary submit output missing %q:\n%s", marker, output)
		}
	}
	return submitted
}

func assertUnaryWorkListedAfterSubmit(t *testing.T, baseURL, workName, workID string) {
	t.Helper()

	listed := support.ListDefaultSessionWork(t, baseURL)
	for _, item := range listed.Results {
		if item.Name != workName {
			continue
		}
		if support.StringPointerValue(item.WorkId) == workID {
			if support.StringPointerValue(item.WorkTypeName) != unaryContractWorkType {
				t.Fatalf(
					"public work list workTypeName = %q, want %q for name=%q workId=%q",
					support.StringPointerValue(item.WorkTypeName),
					unaryContractWorkType,
					workName,
					workID,
				)
			}
			return
		}
	}
	t.Fatalf(
		"public work list missing submitted work name=%q workId=%q: %#v",
		workName,
		workID,
		listed.Results,
	)
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
