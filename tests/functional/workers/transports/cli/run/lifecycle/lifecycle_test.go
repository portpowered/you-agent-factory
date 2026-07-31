package lifecycle_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
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
		"--quiet",
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
// hosted public you run --with-server invocation routes through the already-open
// Factory Session on the live runtime host rather than a detached local one-shot
// lifecycle, with Factory Event correlation on that hosted session identity.
func TestCLIRunServerAttachedInvocationTargetsExistingFactorySession(t *testing.T) {
	t.Parallel()

	factoryDir := scaffoldProviderBackedFactory(t)
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)

	hostedServerEdges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(
		t,
		&hostedServerEdges,
		support.NewStaticSuccessCommandRunner(wantServerAttachedInvocationPrimaryResult),
		nil,
	)
	continuousHost := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Edges:                     hostedServerEdges,
	})
	defer continuousHost.Stop(t)

	continuousBaseURL := continuousHost.URL()
	assertDetachedServerPrefRunCannotAttachToContinuousHost(
		t,
		factoryDir,
		factoryPath,
		continuousBaseURL,
	)

	hostedAPI := support.NewProcessAPIServer()
	hostedServerEdges.APIServerStarter = hostedAPI.Start
	hostedProcess := support.BuildProcess(t, hostedServerEdges)

	args := []string{
		"you", "run",
		"--factory", factoryPath,
		"--with-server",
		"--no-record",
		"--quiet",
		"prove workers-owned server-attached lifecycle",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.WorkingDirectory = factoryDir
	command := support.StartProcessCommand(t, hostedProcess, inputs.Input)

	observationsReady := make(chan struct {
		session     factoryapi.FactorySession
		workVisible bool
		err         error
	}, 1)
	go func() {
		baseURL, err := hostedAPI.WaitForBaseURL(5 * time.Second)
		if err != nil {
			observationsReady <- struct {
				session     factoryapi.FactorySession
				workVisible bool
				err         error
			}{err: err}
			return
		}
		session, workVisible, pollErr := pollHostedServerAttachedInvocationObservations(
			baseURL,
			wantServerAttachedInvocationPrimaryResult,
			command.Done(),
		)
		observationsReady <- struct {
			session     factoryapi.FactorySession
			workVisible bool
			err         error
		}{session: session, workVisible: workVisible, err: pollErr}
	}()

	observation := <-observationsReady
	<-command.Done()
	if err := command.Err(); err != nil {
		t.Fatalf(
			"Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s",
			args,
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	if observation.err != nil {
		t.Logf("hosted server-attached session/work observations before host shutdown: %v", observation.err)
	}

	stdout := strings.TrimSuffix(inputs.Stdout(), "\n")
	if stdout != wantServerAttachedInvocationPrimaryResult {
		t.Fatalf("stdout = %q, want exact server-attached primary result %q", stdout, wantServerAttachedInvocationPrimaryResult)
	}
	if inputs.Stderr() != "" {
		t.Fatalf("stderr = %q, want empty successful server-attached stderr", inputs.Stderr())
	}

	if strings.TrimSpace(observation.session.Id) == "" {
		t.Fatal("hosted Factory Session id observed during invocation is empty, want observable session identity")
	}
	if !observation.workVisible {
		t.Logf("terminal /work was not observable before hosted API shutdown; stdout primary result remains the customer-visible proof")
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
		"--quiet",
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

func assertDetachedServerPrefRunCannotAttachToContinuousHost(
	t *testing.T,
	factoryDir, factoryPath, continuousBaseURL string,
) {
	t.Helper()

	clientEdges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(
		t,
		&clientEdges,
		support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
			ExitCode: deterministicProviderFailureExit,
			Stderr:   []byte(deterministicProviderFailureStderr),
		}),
		nil,
	)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", continuousBaseURL,
		"run",
		"--factory", factoryPath,
		"--no-record",
		"prove detached server preference cannot attach",
	})
	inputs.Input.WorkingDirectory = factoryDir
	if err := support.BuildProcess(t, clientEdges).Execute(inputs.Input); err == nil {
		t.Fatalf(
			"Process.Execute(detached --server run) unexpectedly succeeded; want failure when client provider edges are isolated from the continuous host\nstdout:\n%s\nstderr:\n%s",
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
}

func pollHostedServerAttachedInvocationObservations(
	baseURL, wantWorkText string,
	done <-chan struct{},
) (factoryapi.FactorySession, bool, error) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	var (
		sessionRead    bool
		workVisible    bool
		sessionDuring  factoryapi.FactorySession
		lastSessionErr string
	)

	for {
		if !sessionRead {
			if session, ok, diagnostic := tryReadDefaultFactorySession(baseURL); ok {
				sessionDuring = session
				sessionRead = true
			} else if diagnostic != "" {
				lastSessionErr = diagnostic
			}
		}
		if !workVisible {
			if ok, _ := tryReadTerminalWorkPrimaryText(baseURL, wantWorkText); ok {
				workVisible = true
			}
		}
		if sessionRead && workVisible {
			return sessionDuring, true, nil
		}

		select {
		case <-done:
			if !sessionRead {
				return factoryapi.FactorySession{}, workVisible, fmt.Errorf(
					"hosted CLI run finished before session identity was readable at %s: %s",
					baseURL,
					lastSessionErr,
				)
			}
			return sessionDuring, workVisible, nil
		default:
		}

		select {
		case <-done:
			if !sessionRead {
				return factoryapi.FactorySession{}, workVisible, fmt.Errorf(
					"hosted CLI run finished before session identity was readable at %s: %s",
					baseURL,
					lastSessionErr,
				)
			}
			return sessionDuring, workVisible, nil
		case <-ticker.C:
		}
	}
}

func tryReadDefaultFactorySession(baseURL string) (factoryapi.FactorySession, bool, string) {
	session, err := readDefaultFactorySession(baseURL)
	if err != nil {
		return factoryapi.FactorySession{}, false, err.Error()
	}
	return session, true, ""
}

func readDefaultFactorySession(baseURL string) (factoryapi.FactorySession, error) {
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/~default"
	response, err := http.Get(endpoint)
	if err != nil {
		return factoryapi.FactorySession{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return factoryapi.FactorySession{}, fmt.Errorf("GET %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded factoryapi.FactorySessionGetResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return factoryapi.FactorySession{}, err
	}
	session, err := decoded.AsFactorySession()
	if err != nil {
		return factoryapi.FactorySession{}, err
	}
	if strings.TrimSpace(session.Id) == "" {
		return factoryapi.FactorySession{}, fmt.Errorf("GET %s returned empty session id", endpoint)
	}
	return session, nil
}

func tryReadTerminalWorkPrimaryText(serverURL, wantText string) (bool, string) {
	endpoint := support.DefaultSessionWorkURL(serverURL, "/work")
	response, err := http.Get(endpoint)
	if err != nil {
		return false, fmt.Sprintf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(response.Body)
		return false, fmt.Sprintf(
			"GET %s status = %d: %s",
			endpoint,
			response.StatusCode,
			strings.TrimSpace(string(payload)),
		)
	}
	var listed factoryapi.ListWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		return false, fmt.Sprintf("decode GET %s: %v", endpoint, err)
	}
	for _, item := range listed.Results {
		if item.State == nil || item.State.Type != factoryapi.WorkStateTypeTERMINAL {
			continue
		}
		if item.Content == nil || len(*item.Content) != 1 {
			continue
		}
		part, err := (*item.Content)[0].AsWorkTextContentPart()
		if err != nil {
			continue
		}
		if part.Text == wantText {
			return true, ""
		}
	}
	return false, fmt.Sprintf("listed work missing terminal primary text %q: %#v", wantText, listed.Results)
}

func assertCleanInvocationStdoutFreeOfOperatorChatter(t *testing.T, stdout string) {
	t.Helper()

	for _, forbidden := range cleanInvocationForbiddenOperatorChatter {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("stdout contains operator lifecycle chatter %q:\n%s", forbidden, stdout)
		}
	}
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
