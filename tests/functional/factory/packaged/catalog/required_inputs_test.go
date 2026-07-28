package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
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
// rejects through the public packaged-catalog invocation boundary without a
// completed success primary result attributable to the missing-input attempt.
func TestPackagedFactoriesRejectMissingRequiredInputs(t *testing.T) {
	cases, err := packagedFactoriesWithRequiredInvocationInputs()
	if err != nil {
		t.Fatalf("packaged Factory matrix: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("packaged Factory matrix has no required-input cases")
	}

	for _, testcase := range cases {
		testcase := testcase
		t.Run(testcase.factoryName, func(t *testing.T) {
			runner := support.NewRecordingCommandRunner("unexpected live provider execution")
			run := runPackagedFactoryMissingRequiredInputInvocation(
				t,
				runner,
				testcase.factoryName,
			)
			assertPackagedFactoryMissingRequiredInputRejected(t, run, runner)
		})
	}
}

// TestPackagedFactoriesNameMissingInputAndFactory proves that omitting required
// invocation inputs for every packaged Factory in the embedded package matrix
// surfaces customer-visible failure diagnostics naming both the missing input and
// the Factory under test through the public packaged-catalog invocation boundary.
func TestPackagedFactoriesNameMissingInputAndFactory(t *testing.T) {
	cases, err := packagedFactoriesWithRequiredInvocationInputs()
	if err != nil {
		t.Fatalf("packaged Factory matrix: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("packaged Factory matrix has no required-input cases")
	}

	for _, testcase := range cases {
		testcase := testcase
		t.Run(testcase.factoryName, func(t *testing.T) {
			runner := support.NewRecordingCommandRunner("unexpected live provider execution")
			run := runPackagedFactoryMissingRequiredInputInvocation(
				t,
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
	runner *support.RecordingCommandRunner,
	factoryName string,
) packagedFactoryMissingRequiredInputRun {
	t.Helper()

	if packagedFactoryMissingRequiredInputUsesHTTPInvocation(factoryName) {
		return runPackagedFactoryMissingRequiredInputHTTPInvocation(t, runner, factoryName)
	}
	return runPackagedFactoryMissingRequiredInputCLIInvocation(t, runner, factoryName)
}

func packagedFactoryMissingRequiredInputUsesHTTPInvocation(factoryName string) bool {
	// @you/fusion exposes an optional "output" parameter that collides with the
	// public you run --output flag, so the packaged Factory cannot compose on the
	// CLI boundary even when required inputs are present.
	return factoryName == factorydefinitions.PackagedFusionFactoryName
}

func runPackagedFactoryMissingRequiredInputCLIInvocation(
	t *testing.T,
	runner *support.RecordingCommandRunner,
	factoryName string,
) packagedFactoryMissingRequiredInputRun {
	t.Helper()

	homeDir := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, factoryName)

	args := []string{
		"you", "--json", "run",
		"--named", factoryName,
		"--no-record",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = t.TempDir()
	inputs.Input.Stdin = strings.NewReader("")
	stdinIsTTY := true
	inputs.Input.StdinIsTTY = &stdinIsTTY

	run := packagedFactoryMissingRequiredInputRun{
		execErr: support.BuildProcess(t, serviceedges.Edges{
			ProviderCommandRunner: runner,
		}).Execute(inputs.Input),
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
		if !packagedFactoryMissingRequiredInputDiagnosticNamesParameter(diagnostic, parameterName) {
			t.Fatalf(
				"failure diagnostic = %q, want missing required input name %q",
				diagnostic,
				parameterName,
			)
		}
	}
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
