package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// catalogPackagedFactorySeed holds the read-only result of one public system
// bootstrap. Required-input rows still receive a fresh home and selected
// Factory directory, but do not repeatedly materialize the complete packaged
// catalog when the only behavior under test is pre-provider input rejection.
type catalogPackagedFactorySeed struct {
	homeDir     string
	factoryRoot string
}

func newCatalogPackagedFactorySeed(
	t *testing.T,
	process support.Process,
	factoryName string,
) catalogPackagedFactorySeed {
	t.Helper()

	homeDir := t.TempDir()
	env := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	factoryDir := support.InstallPackagedFactoryWithProcess(t, process, env, t.TempDir(), factoryName)
	if _, err := os.Stat(filepath.Join(homeDir, ".you-agent-factory", "config.json")); err != nil {
		t.Fatalf("catalog seed system config: %v", err)
	}
	// Packaged names use the customer-facing @you/<name> layout. The support
	// helper has already validated the returned Factory directory, so retain
	// that public result instead of reaching into Factory Definitions policy.
	factoryRoot := filepath.Dir(filepath.Dir(factoryDir))
	return catalogPackagedFactorySeed{homeDir: homeDir, factoryRoot: factoryRoot}
}

func (seed catalogPackagedFactorySeed) copyInto(
	t *testing.T,
	homeDir string,
	factoryName string,
) {
	t.Helper()

	seedFactoryDir := filepath.Join(seed.factoryRoot, filepath.FromSlash(factoryName))
	if _, err := os.Stat(filepath.Join(seedFactoryDir, "factory.json")); err != nil {
		t.Fatalf("catalog seed packaged Factory %q: %v", factoryName, err)
	}
	support.CopyFactoryAsNamed(t, seedFactoryDir, homeDir, factoryName)

	seedConfigPath := filepath.Join(seed.homeDir, ".you-agent-factory", "config.json")
	config, err := os.ReadFile(seedConfigPath)
	if err != nil {
		t.Fatalf("read catalog seed system config: %v", err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(config, &document); err != nil {
		t.Fatalf("decode catalog seed system config: %v", err)
	}
	document["backendScopeID"], err = json.Marshal("local-" + uuid.NewString())
	if err != nil {
		t.Fatalf("encode catalog row backend scope: %v", err)
	}
	config, err = json.Marshal(document)
	if err != nil {
		t.Fatalf("encode catalog row system config: %v", err)
	}
	targetConfigPath := filepath.Join(homeDir, ".you-agent-factory", "config.json")
	if err := os.MkdirAll(filepath.Dir(targetConfigPath), 0o755); err != nil {
		t.Fatalf("create catalog row system config directory: %v", err)
	}
	if err := os.WriteFile(targetConfigPath, config, 0o600); err != nil {
		t.Fatalf("copy catalog row system config: %v", err)
	}
}

// TestPackagedFactoryRejectsMissingRequiredInput proves a representative
// packaged Factory rejects an omitted required customer argument before
// provider execution and identifies both the Factory and missing input. The
// declaration of required parameters across every packaged source is static
// conformance, not a functional behavior matrix.
func TestPackagedFactoryRejectsMissingRequiredInput(t *testing.T) {
	const factoryName = "@you/fix"
	fixture := sharedCatalogProcess(t)
	process := fixture.process
	seed := newCatalogPackagedFactorySeed(t, process, factoryName)
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	fixture.provider.setDelegate(runner)
	run := runPackagedFactoryMissingRequiredInputInvocation(t, process, seed, factoryName)
	assertPackagedFactoryMissingRequiredInputRejected(t, run, runner)
	assertPackagedFactoryMissingRequiredInputDiagnosticsNameFactoryAndInput(
		t, run, factoryName, []string{"request"},
	)
}

func runPackagedFactoryMissingRequiredInputInvocation(
	t *testing.T,
	process support.Process,
	seed catalogPackagedFactorySeed,
	factoryName string,
) packagedFactoryMissingRequiredInputRun {
	t.Helper()
	return runPackagedFactoryMissingRequiredInputCLIInvocation(t, process, seed, factoryName)
}

func runPackagedFactoryMissingRequiredInputCLIInvocation(
	t *testing.T,
	process support.Process,
	seed catalogPackagedFactorySeed,
	factoryName string,
) packagedFactoryMissingRequiredInputRun {
	t.Helper()

	homeDir := t.TempDir()
	env := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	seed.copyInto(t, homeDir, factoryName)

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
