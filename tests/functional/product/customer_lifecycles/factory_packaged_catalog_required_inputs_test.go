package customer_lifecycles_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type packagedFactoryMissingRequiredInputRun struct {
	response      factoryapi.InvocationResponse
	errorResponse factoryapi.ErrorResponse
	execErr       error
}

// TestPackagedFactoryRejectsMissingRequiredInput proves a representative
// packaged Factory rejects an omitted required customer argument before
// provider execution and identifies both the Factory and missing input. The
// declaration of required parameters across every packaged source is static
// conformance, not a functional behavior matrix.
func testFactorypackagedcatalogPackagedFactoryRejectsMissingRequiredInput(t *testing.T) {
	const factoryName = "@you/fix"
	fixture := sharedCatalogProcess(t)
	process := fixture.process
	// Input rejection uses the same test-owned installed home, with an explicit
	// Factory Session. Installation and backend-scope copying are not its subject.
	homeDir := t.TempDir()
	support.InstallPackagedFactoryWithProcess(t, process, isolatedHomeEnvironment(homeDir), t.TempDir(), factoryName)
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	fixture.provider.setDelegate(runner)
	run := runPackagedFactoryMissingRequiredInputCLIInvocation(t, process, homeDir, factoryName)
	assertPackagedFactoryMissingRequiredInputRejected(t, run, runner)
	assertPackagedFactoryMissingRequiredInputDiagnosticsNameFactoryAndInput(
		t, run, factoryName, []string{"request"},
	)
}

func runPackagedFactoryMissingRequiredInputCLIInvocation(
	t *testing.T,
	process support.Process,
	homeDir string,
	factoryName string,
) packagedFactoryMissingRequiredInputRun {
	t.Helper()

	env := isolatedHomeEnvironment(homeDir)

	args := []string{
		"you", "--json", "run",
		"--session", uuid.NewString(),
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
