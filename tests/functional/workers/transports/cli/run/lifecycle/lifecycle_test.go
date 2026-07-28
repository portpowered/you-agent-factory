package lifecycle_test

import (
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	wantCleanInvocationPrimaryResult          = "deterministic workers lifecycle primary COMPLETE"
	wantServerAttachedInvocationPrimaryResult = "deterministic workers lifecycle server-attached COMPLETE"
	deterministicProviderFailureExit          = 7
	deterministicProviderFailureStderr        = "deterministic provider rejection"
)

var cleanInvocationForbiddenOperatorChatter = []string{
	"Factory initiated",
	"Dashboard URL",
	"Runtime log",
	"Opening dashboard",
	"Recording saved to",
	"Factory:",
}

// TestCLIRunCleanInvocationCompletesWithoutDashboardStartup proves a
// clean/prompt-style public you run invocation completes with only the Factory
// primary result on stdout and does not emit dashboard open or startup sidecar
// output on that primary-result stream.
func TestCLIRunCleanInvocationCompletesWithoutDashboardStartup(t *testing.T) {
	t.Parallel()

	factoryDir := scaffoldProviderBackedFactory(t)
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)

	edges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(
		t,
		&edges,
		support.NewStaticSuccessCommandRunner(wantCleanInvocationPrimaryResult),
		nil,
	)

	args := []string{
		"you", "run",
		"--factory", factoryPath,
		"--no-record",
		"prove workers-owned clean invocation lifecycle",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.WorkingDirectory = factoryDir
	if err := support.BuildProcess(t, edges).Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s",
			args,
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	stdout := strings.TrimSuffix(inputs.Stdout(), "\n")
	if stdout != wantCleanInvocationPrimaryResult {
		t.Fatalf("stdout = %q, want exact primary clean invocation output %q", stdout, wantCleanInvocationPrimaryResult)
	}
	assertCleanInvocationStdoutFreeOfOperatorChatter(t, stdout)
	if inputs.Stderr() != "" {
		t.Fatalf("stderr = %q, want empty successful-run stderr", inputs.Stderr())
	}
}

// TestCLIRunServerAttachedInvocationTargetsExistingFactorySession proves a
// server-attached public you run invocation completes against an already-open
// Factory Session on a running host rather than starting a separate one-shot
// detached lifecycle.
func TestCLIRunServerAttachedInvocationTargetsExistingFactorySession(t *testing.T) {
	t.Parallel()

	factoryDir := scaffoldProviderBackedFactory(t)
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)

	edges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(
		t,
		&edges,
		support.NewStaticSuccessCommandRunner(wantServerAttachedInvocationPrimaryResult),
		nil,
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
	})
	defer server.Stop(t)

	baseURL := server.URL()
	defaultSessionBefore := getFactorySessionAt(t, baseURL, factorysessions.DefaultSessionID)
	opened := support.OpenFactorySessionAt(t, baseURL, factoryDir)
	explicitSessionID := opened.Session.Id

	process := support.BuildProcess(t, edges)
	args := []string{
		"you", "--server", baseURL,
		"run",
		"--factory", factoryPath,
		"--no-record",
		"prove workers-owned server-attached lifecycle",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.WorkingDirectory = factoryDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s",
			args,
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	stdout := strings.TrimSuffix(inputs.Stdout(), "\n")
	if stdout != wantServerAttachedInvocationPrimaryResult {
		t.Fatalf("stdout = %q, want exact server-attached primary result %q", stdout, wantServerAttachedInvocationPrimaryResult)
	}
	if inputs.Stderr() != "" {
		t.Fatalf("stderr = %q, want empty successful server-attached stderr", inputs.Stderr())
	}

	defaultSessionAfter := getFactorySessionAt(t, baseURL, factorysessions.DefaultSessionID)
	if defaultSessionAfter.Id != defaultSessionBefore.Id {
		t.Fatalf(
			"default Factory Session id after run = %q, want unchanged existing session %q",
			defaultSessionAfter.Id,
			defaultSessionBefore.Id,
		)
	}

	shown := executeSessionShowCLI(t, process, baseURL, defaultSessionBefore.Id)
	if shown.Id != defaultSessionBefore.Id {
		t.Fatalf("session show id = %q, want targeted existing session %q", shown.Id, defaultSessionBefore.Id)
	}

	explicitAfter := getFactorySessionAt(t, baseURL, explicitSessionID)
	if explicitAfter.Id != explicitSessionID {
		t.Fatalf(
			"explicit Factory Session id after run = %q, want pre-opened session %q",
			explicitAfter.Id,
			explicitSessionID,
		)
	}
}

// TestCLIRunCleanInvocationFailurePreservesPublicError proves a failed
// clean/prompt-style public you run invocation exits unsuccessfully, writes the
// documented public error contract to stderr, and does not emit a false-success
// primary result on stdout.
func TestCLIRunCleanInvocationFailurePreservesPublicError(t *testing.T) {
	t.Parallel()

	factoryDir := scaffoldProviderBackedFactory(t)
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)

	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		ExitCode: deterministicProviderFailureExit,
		Stderr:   []byte(deterministicProviderFailureStderr),
	})
	edges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(t, &edges, runner, nil)

	args := []string{
		"you", "run",
		"--factory", factoryPath,
		"--no-record",
		"prove workers-owned clean invocation failure lifecycle",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.WorkingDirectory = factoryDir
	if err := support.BuildProcess(t, edges).Execute(inputs.Input); err == nil {
		t.Fatal("Process.Execute error = nil, want terminal clean invocation failure")
	}

	stdout := strings.TrimSpace(inputs.Stdout())
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty clean failure stdout without false primary result", stdout)
	}
	if strings.Contains(stdout, wantCleanInvocationPrimaryResult) {
		t.Fatalf("stdout contains false-success primary result %q", wantCleanInvocationPrimaryResult)
	}

	errorResponse := decodeSingleErrorResponse(t, inputs.Stderr())
	if errorResponse.Code == "" || strings.TrimSpace(errorResponse.Message) == "" {
		t.Fatalf("ErrorResponse = %#v, want actionable code and message", errorResponse)
	}
	if errorResponse.Family != factoryapi.ErrorFamilyInternalServerError {
		t.Fatalf("ErrorResponse family = %q, want %q", errorResponse.Family, factoryapi.ErrorFamilyInternalServerError)
	}
}

func scaffoldProviderBackedFactory(t *testing.T) string {
	t.Helper()

	cfg := map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
			"handlingBehavior": []string{"DEFAULT"},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
	dir := support.ScaffoldFactory(t, cfg)
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	return dir
}

func assertCleanInvocationStdoutFreeOfOperatorChatter(t *testing.T, stdout string) {
	t.Helper()

	for _, forbidden := range cleanInvocationForbiddenOperatorChatter {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("stdout contains operator lifecycle chatter %q:\n%s", forbidden, stdout)
		}
	}
}

func getFactorySessionAt(t *testing.T, baseURL, sessionID string) factoryapi.FactorySession {
	t.Helper()

	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + sessionID
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](t, endpoint)
	session, err := response.AsFactorySession()
	if err != nil {
		t.Fatalf("decode Factory Session %q: %v", sessionID, err)
	}
	if strings.TrimSpace(session.Id) == "" {
		t.Fatalf("Factory Session %q response = %#v, want public session id", sessionID, session)
	}
	return session
}

func decodeSingleErrorResponse(t *testing.T, stderr string) factoryapi.ErrorResponse {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(stderr))
	var response factoryapi.ErrorResponse
	if err := decoder.Decode(&response); err != nil {
		t.Fatalf("decode ErrorResponse: %v\nstderr:\n%s", err, stderr)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("stderr contains data after ErrorResponse: %v\nstderr:\n%s", err, stderr)
	}
	return response
}

func executeSessionShowCLI(
	t *testing.T,
	process support.Process,
	baseURL, sessionID string,
) factoryapi.FactorySession {
	t.Helper()

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", baseURL, "--json",
		"session", "show", sessionID,
	})
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(session show) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	var shown factoryapi.FactorySession
	if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &shown); err != nil {
		t.Fatalf("decode session show JSON: %v\nstdout:\n%s", err, inputs.Stdout())
	}
	return shown
}
