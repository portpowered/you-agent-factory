package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type packagedFactoryRequiredInputCase struct {
	factoryName        string
	requiredParameters []string
}

type packagedFactoryMissingRequiredInputRun struct {
	response      factoryapi.InvocationResponse
	errorResponse factoryapi.ErrorResponse
	execErr       error
}

// TestPackagedFactoriesRejectMissingRequiredInputs proves that omitting required
// invocation inputs for every packaged Factory in the embedded package matrix
// rejects through the public packaged-catalog invocation boundary, without a
// completed success result, and names both the Factory and missing input.
func TestPackagedFactoriesRejectMissingRequiredInputs(t *testing.T) {
	cases, err := packagedFactoriesWithRequiredInvocationInputs()
	if err != nil {
		t.Fatalf("packaged Factory matrix: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("packaged Factory matrix has no required-input cases")
	}

	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: sharedRequiredInputRunner,
	})
	support.CleanupProcess(t, process)

	for _, testcase := range cases {
		testcase := testcase
		t.Run(testcase.factoryName, func(t *testing.T) {
			runner := support.NewRecordingCommandRunner("unexpected live provider execution")
			sharedRequiredInputRunner.setDelegate(runner)
			run := runPackagedFactoryMissingRequiredInputInvocation(
				t,
				process,
				runner,
				testcase.factoryName,
			)
			assertPackagedFactoryMissingRequiredInputRejected(t, run, runner)
			assertPackagedFactoryMissingRequiredInputDiagnosticsNameFactoryAndInput(
				t,
				run,
				testcase.factoryName,
				testcase.requiredParameters,
			)
		})
	}
}

// swappingProviderCommandRunner is an immutable process edge whose per-row
// provider-command recorder is swapped between sequential invocations on the
// shared required-input process. Each row still observes its own zero-call
// assertion without rebuilding the root graph.
type swappingProviderCommandRunner struct {
	mu       sync.Mutex
	delegate platformprocess.CommandRunner
}

func newSwappingProviderCommandRunner() *swappingProviderCommandRunner {
	return &swappingProviderCommandRunner{}
}

func (r *swappingProviderCommandRunner) setDelegate(delegate platformprocess.CommandRunner) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.delegate = delegate
}

func (r *swappingProviderCommandRunner) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	delegate := r.delegate
	r.mu.Unlock()
	if delegate == nil {
		return platformprocess.CommandResult{}, errors.New("no provider command runner installed for this invocation")
	}
	return delegate.Run(ctx, req)
}

// sharedRequiredInputRunner is the immutable swapping edge shared by every
// sequential missing-required-input invocation in this test.
var sharedRequiredInputRunner = newSwappingProviderCommandRunner()

func packagedFactoriesWithRequiredInvocationInputs() ([]packagedFactoryRequiredInputCase, error) {
	inventory, err := packagedfactorycatalog.Discover(
		context.Background(),
		packagedfactories.Source(),
		"factories",
	)
	if err != nil {
		return nil, err
	}

	var cases []packagedFactoryRequiredInputCase
	for _, entry := range inventory.Entries {
		if entry.Factory == nil || entry.Factory.InvocationSignature == nil {
			continue
		}
		var required []string
		for _, parameter := range entry.Factory.InvocationSignature.Parameters {
			if parameter.Required {
				required = append(required, parameter.Name)
			}
		}
		if len(required) == 0 {
			continue
		}
		cases = append(cases, packagedFactoryRequiredInputCase{
			factoryName:        entry.Factory.Name,
			requiredParameters: required,
		})
	}
	return cases, nil
}

func runPackagedFactoryMissingRequiredInputInvocation(
	t *testing.T,
	process support.Process,
	runner *support.RecordingCommandRunner,
	factoryName string,
) packagedFactoryMissingRequiredInputRun {
	t.Helper()

	if packagedFactoryMissingRequiredInputUsesHTTPInvocation(factoryName) {
		return runPackagedFactoryMissingRequiredInputHTTPInvocation(t, runner, factoryName)
	}
	return runPackagedFactoryMissingRequiredInputCLIInvocation(t, process, factoryName)
}

func packagedFactoryMissingRequiredInputUsesHTTPInvocation(factoryName string) bool {
	return false
}

func runPackagedFactoryMissingRequiredInputCLIInvocation(
	t *testing.T,
	process support.Process,
	factoryName string,
) packagedFactoryMissingRequiredInputRun {
	t.Helper()

	homeDir := t.TempDir()
	env := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	support.InstallPackagedFactoryWithProcess(t, process, env, t.TempDir(), factoryName)

	args := []string{
		"you", "--json", "run",
		"--named", factoryName,
		"--no-record",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = env
	inputs.Input.WorkingDirectory = t.TempDir()
	inputs.Input.Stdin = strings.NewReader("")
	stdinIsTTY := true
	inputs.Input.StdinIsTTY = &stdinIsTTY

	run := packagedFactoryMissingRequiredInputRun{
		execErr: process.Execute(inputs.Input),
	}
	if stdout := strings.TrimSpace(inputs.Stdout()); stdout != "" {
		if decodeErr := json.Unmarshal([]byte(stdout), &run.response); decodeErr != nil {
			t.Fatalf("decode invocation JSON stdout: %v\nstdout:\n%s", decodeErr, stdout)
		}
	}
	if stderr := strings.TrimSpace(inputs.Stderr()); stderr != "" {
		if decodeErr := json.Unmarshal([]byte(stderr), &run.errorResponse); decodeErr != nil {
			t.Fatalf("decode ErrorResponse stderr: %v\nstderr:\n%s", decodeErr, stderr)
		}
	}
	return run
}

func runPackagedFactoryMissingRequiredInputHTTPInvocation(
	t *testing.T,
	runner *support.RecordingCommandRunner,
	factoryName string,
) packagedFactoryMissingRequiredInputRun {
	t.Helper()

	factoryDir := support.InstallPackagedFactory(t, t.TempDir(), factoryName)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
		},
	})

	emptyArgs := map[string]any{}
	requestBody, err := json.Marshal(factoryapi.InvocationRequest{
		Args: &emptyArgs,
	})
	if err != nil {
		t.Fatalf("marshal invocation request: %v", err)
	}
	endpoint := strings.TrimSuffix(server.URL(), "/") +
		"/factory-sessions/" + factorysessions.DefaultSessionID + "/invocations"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read invocation response: %v", err)
	}

	run := packagedFactoryMissingRequiredInputRun{}
	if response.StatusCode == http.StatusOK {
		if decodeErr := json.Unmarshal(payload, &run.response); decodeErr != nil {
			t.Fatalf("decode invocation response: %v\npayload:\n%s", decodeErr, string(payload))
		}
		return run
	}
	if decodeErr := json.Unmarshal(payload, &run.errorResponse); decodeErr != nil {
		run.execErr = fmt.Errorf(
			"POST %s status = %d: %s",
			endpoint,
			response.StatusCode,
			string(payload),
		)
		return run
	}
	run.execErr = fmt.Errorf("%s: %s", run.errorResponse.Code, run.errorResponse.Message)
	return run
}

func assertPackagedFactoryMissingRequiredInputRejected(
	t *testing.T,
	run packagedFactoryMissingRequiredInputRun,
	runner *support.RecordingCommandRunner,
) {
	t.Helper()

	if run.response.Status == factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf(
			"invocation status = %q, want non-success rejection for missing required input",
			run.response.Status,
		)
	}
	if run.response.PrimaryResult != nil {
		t.Fatalf(
			"primaryResult = %#v, want no completed success primary result for missing required input",
			run.response.PrimaryResult,
		)
	}
	if runner.CallCount() != 0 {
		t.Fatalf(
			"provider command runner call count = %d, want 0 before missing-required-input validation",
			runner.CallCount(),
		)
	}
	if !packagedFactoryMissingRequiredInputRejected(run) {
		t.Fatalf(
			"missing required input rejection missing from execErr=%v errorResponse=%#v response=%#v",
			run.execErr,
			run.errorResponse,
			run.response,
		)
	}
}

func packagedFactoryMissingRequiredInputRejected(run packagedFactoryMissingRequiredInputRun) bool {
	missingRequiredCode := string(work.ArgumentErrorCodeMissingRequiredInput)
	if run.execErr != nil && strings.Contains(run.execErr.Error(), missingRequiredCode) {
		return true
	}
	if run.errorResponse.Code == factoryapi.ErrorResponseCode(missingRequiredCode) {
		return true
	}
	if run.response.ErrorCode != nil &&
		strings.Contains(string(*run.response.ErrorCode), missingRequiredCode) {
		return true
	}
	return false
}

func assertPackagedFactoryMissingRequiredInputDiagnosticsNameFactoryAndInput(
	t *testing.T,
	run packagedFactoryMissingRequiredInputRun,
	factoryName string,
	requiredParameters []string,
) {
	t.Helper()

	diagnostic := packagedFactoryMissingRequiredInputDiagnostic(run)
	if !strings.Contains(diagnostic, factoryName) {
		t.Fatalf(
			"failure diagnostic = %q, want Factory name %q",
			diagnostic,
			factoryName,
		)
	}
	for _, parameterName := range requiredParameters {
		if packagedFactoryMissingRequiredInputDiagnosticNamesParameter(diagnostic, parameterName) {
			return
		}
	}
	t.Fatalf("failure diagnostic = %q, want at least one missing required input from %#v", diagnostic, requiredParameters)
}

func packagedFactoryMissingRequiredInputDiagnostic(run packagedFactoryMissingRequiredInputRun) string {
	parts := make([]string, 0, 3)
	if run.execErr != nil {
		parts = append(parts, run.execErr.Error())
	}
	if run.errorResponse.Message != "" {
		parts = append(parts, run.errorResponse.Message)
	}
	if run.response.Message != nil {
		parts = append(parts, *run.response.Message)
	}
	return strings.Join(parts, "\n")
}

func packagedFactoryMissingRequiredInputDiagnosticNamesParameter(diagnostic, parameterName string) bool {
	if strings.TrimSpace(parameterName) == "" {
		return false
	}
	return strings.Contains(diagnostic, parameterName) ||
		strings.Contains(diagnostic, fmt.Sprintf("%q", parameterName))
}
