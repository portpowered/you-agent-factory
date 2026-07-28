package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const packagedSubagentChildPrimaryResult = "mock worker accepted"

// TestPackagedSubagentReturnsChildResult proves that packaged @you/subagent
// invocation through the public CLI completes with the child's authoritative
// primary agent response instead of echoing the submitted request text,
// including hermetic no-server success without starting an HTTP listener.
func TestPackagedSubagentReturnsChildResult(t *testing.T) {
	t.Run("CLI JSON returns authoritative child primary result", func(t *testing.T) {
		requestText := fmt.Sprintf("functional-packaged-subagent-primary-%d", time.Now().UnixNano())
		response := runPackagedSubagentCLIJSONInvocation(t, requestText)

		if response.Status != factoryapi.InvocationTerminalStatusCompleted {
			t.Fatalf("status = %q, want COMPLETED; response = %#v", response.Status, response)
		}
		if got := invocationPrimaryResultText(t, response); got != packagedSubagentChildPrimaryResult {
			t.Fatalf("primaryResult = %q, want %q", got, packagedSubagentChildPrimaryResult)
		}
		if strings.Contains(invocationResponseJSON(t, response), requestText) {
			t.Fatalf("invocation JSON echoed submitted request text %q", requestText)
		}
	})

	t.Run("hermetic named invocation succeeds without listening server", func(t *testing.T) {
		requestText := "hermetic no-server packaged subagent prompt"
		stdout, listenerStarts := runHermeticPackagedSubagentInvocation(t, requestText)

		if stdout != packagedSubagentChildPrimaryResult {
			t.Fatalf("stdout = %q, want child primary result %q", stdout, packagedSubagentChildPrimaryResult)
		}
		if stdout == requestText {
			t.Fatal("stdout echoed submitted request text instead of child agent response")
		}
		if listenerStarts != 0 {
			t.Fatalf("HTTP listener start calls = %d, want 0", listenerStarts)
		}
	})
}

func runPackagedSubagentCLIJSONInvocation(t *testing.T, requestText string) factoryapi.InvocationResponse {
	t.Helper()

	homeDir := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, factorydefinitions.PackagedSubagentFactoryName)
	mockWorkersPath := support.WriteMockWorkersConfig(t, packagedSubagentAcceptMockWorkersConfig())

	args := []string{
		"you", "--json", "run",
		"--named", factorydefinitions.PackagedSubagentFactoryName,
		"--with-mock-workers", mockWorkersPath,
		"--no-record",
		requestText,
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = t.TempDir()
	if err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s", args, err, inputs.Stdout(), inputs.Stderr())
	}
	if inputs.Stderr() != "" {
		t.Fatalf("stderr = %q, want empty successful-run stderr", inputs.Stderr())
	}

	var response factoryapi.InvocationResponse
	if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &response); decodeErr != nil {
		t.Fatalf("decode invocation JSON stdout: %v\nstdout:\n%s", decodeErr, inputs.Stdout())
	}
	return response
}

func runHermeticPackagedSubagentInvocation(t *testing.T, requestText string) (string, int) {
	t.Helper()

	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	listenerStarts := &listenerStartObservation{}
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		APIServerStarter: listenerStarts.Start,
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	mockWorkersPath := support.WriteMockWorkersConfig(t, packagedSubagentAcceptMockWorkersConfig())
	args := []string{
		"you", "run",
		"--named", factorydefinitions.PackagedSubagentFactoryName,
		"--with-mock-workers", "--no-record", "--quiet",
		mockWorkersPath, requestText,
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = workingDirectory
	if execErr := process.Execute(inputs.Input); execErr != nil {
		t.Fatalf("Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s", args, execErr, inputs.Stdout(), inputs.Stderr())
	}
	if inputs.Stderr() != "" {
		t.Fatalf("named invocation stderr = %q, want empty; stdout=%s", inputs.Stderr(), inputs.Stdout())
	}
	return strings.TrimSpace(inputs.Stdout()), int(listenerStarts.calls.Load())
}

func packagedSubagentAcceptMockWorkersConfig() *workers.MockWorkersConfig {
	return &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      factorydefinitions.PackagedSubagentWorkerName,
			WorkstationName: factorydefinitions.PackagedSubagentRunWorkstationName,
			RunType:         workers.MockWorkerRunTypeAccept,
		}},
	}
}

func invocationPrimaryResultText(t *testing.T, response factoryapi.InvocationResponse) string {
	t.Helper()

	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}
	return part.Text
}

func invocationResponseJSON(t *testing.T, response factoryapi.InvocationResponse) string {
	t.Helper()

	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal invocation response: %v", err)
	}
	return string(encoded)
}

type listenerStartObservation struct {
	calls atomic.Int32
}

func (observation *listenerStartObservation) Start(
	_ context.Context,
	_ platformhttpserver.StartRequest,
) error {
	observation.calls.Add(1)
	return errors.New("hermetic packaged subagent invocation attempted to start an HTTP listener")
}
