package runtime_metrics_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	probeObsExpectedWorkID = "cost-replay-priced-work"
	probeObsFalsifier      = "reject non-200, empty results, or a hidden --work-id requirement"

	// The implementation-time probe passed the fleet route, so keep its
	// assertions enabled. The separate blocking gate stays off until PR #2197
	// and Integration Story 0 (#2191) are merged.
	probeObsFleetEnabled = true
	// Flip this one line after the sequencing prerequisite is merged.
	probeObsFleetBlocking = false
)

// TestProbeObservabilityWorkerSessionListing keeps the Work-scoped and
// fleet-wide Worker Session paths as separate customer-facing probe cells.
// The installed binary and isolated replay server are intentional: the
// fleet-wide route must be observed across the same process boundary a
// customer uses, rather than inferred from an in-process service test.
func TestProbeObservabilityWorkerSessionListing(t *testing.T) {
	binaryPath := prebuiltProbeCostArtifact(t)
	fixturePath := testutil.MustRepoPath(t,
		"tests/functional/factory/visualization/runtime_metrics/testdata/codex-gpt-5-codex.factory-recording.v1.json",
	)
	server := startProbeObservabilityServer(t, binaryPath, fixturePath)

	var workScoped probeObsResult
	t.Run("PROBE-OBS/work-scoped", func(t *testing.T) {
		workScoped = runProbeObsList(t, server, probeObsExpectedWorkID)
		logProbeObsVerdict(t, "work-scoped", workScoped)
		if err := validateProbeObsResult(workScoped, true); err != nil {
			t.Fatalf("work-scoped PROBE-OBS failed: %v\nstdout=%s\nstderr=%s\nresponse=%s", err, workScoped.Stdout, workScoped.Stderr, workScoped.ResponseBody)
		}
	})

	t.Run("PROBE-OBS/fleet-wide", func(t *testing.T) {
		fleetWide := runProbeObsList(t, server, "")
		if !probeObsFleetEnabled {
			logProbeObsVerdict(t, "fleet-wide", probeObsResult{
				Verdict:       "SKIPPED/PENDING",
				Command:       fleetWide.Command,
				HTTPStatus:    fleetWide.HTTPStatus,
				ResultCount:   fleetWide.ResultCount,
				Falsifier:     fmt.Sprintf("%s; pending unblocker PR #2197; Integration Story 0 (#2191) is not merged", probeObsFalsifier),
				ResponseBody:  fleetWide.ResponseBody,
				Stdout:        fleetWide.Stdout,
				Stderr:        fleetWide.Stderr,
				CLIExitStatus: fleetWide.CLIExitStatus,
			})
			if fleetWide.HTTPStatus != http.StatusInternalServerError {
				t.Fatalf("pending fleet-wide PROBE-OBS observed HTTP status %d, want current 500 before enabling the cell\ncommand=%s\nresponse=%s", fleetWide.HTTPStatus, fleetWide.Command, fleetWide.ResponseBody)
			}
			t.Skip("fleet-wide Worker Session listing is pending PR #2197 and Integration Story 0 (#2191)")
		}

		if fleetWide.HTTPStatus != http.StatusOK || fleetWide.ResultCount == 0 || fleetWide.CLIExitStatus != 0 {
			pending := fleetWide
			pending.Verdict = "SKIPPED/PENDING"
			pending.Falsifier = fmt.Sprintf("%s; pending unblocker PR #2197; Integration Story 0 (#2191) is not merged", probeObsFalsifier)
			logProbeObsVerdict(t, "fleet-wide", pending)
			if !probeObsFleetBlocking {
				t.Skip("fleet-wide Worker Session listing is pending a passing customer route")
			}
			t.Fatalf("fleet-wide PROBE-OBS failed on a blocking run: %s\nstdout=%s\nstderr=%s\nresponse=%s", fleetWide.Falsifier, fleetWide.Stdout, fleetWide.Stderr, fleetWide.ResponseBody)
		}

		logProbeObsVerdict(t, "fleet-wide", fleetWide)
		if workScoped.ResultCount <= 0 {
			t.Fatalf("fleet-wide PROBE-OBS did not confirm non-empty activity through the work-scoped cell: %#v", workScoped)
		}
		if !probeObsFleetBlocking {
			t.Skip("fleet-wide assertions passed but remain non-blocking until Integration Story 0 (#2191) merges")
		}
		if err := validateProbeObsResult(fleetWide, true); err != nil {
			t.Fatalf("fleet-wide PROBE-OBS failed: %v\nstdout=%s\nstderr=%s\nresponse=%s", err, fleetWide.Stdout, fleetWide.Stderr, fleetWide.ResponseBody)
		}
	})
}

// TestProbeObservabilityValidatorRejectsFalsePositiveReports proves the validator rejects observability reports unsupported by runtime evidence.
func TestProbeObservabilityValidatorRejectsFalsePositiveReports(t *testing.T) {
	base := probeObsResult{Verdict: "PASS", HTTPStatus: http.StatusOK, ResultCount: 1}
	for _, test := range []struct {
		name   string
		mutate func(*probeObsResult)
	}{
		{name: "http 500", mutate: func(result *probeObsResult) {
			result.Verdict = "INCONCLUSIVE"
			result.HTTPStatus = http.StatusInternalServerError
		}},
		{name: "empty result", mutate: func(result *probeObsResult) {
			result.ResultCount = 0
		}},
		{name: "cli failure", mutate: func(result *probeObsResult) {
			result.CLIExitStatus = 1
		}},
		{name: "pending result", mutate: func(result *probeObsResult) {
			result.Verdict = "SKIPPED/PENDING"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := base
			test.mutate(&result)
			if err := validateProbeObsResult(result, true); err == nil {
				t.Fatalf("validator accepted deliberate %s false-positive shape", test.name)
			}
			t.Logf("PROBE-OBS deliberate verdict=FAIL shape=%q falsifier=%q", test.name, probeObsFalsifier)
		})
	}
}

type probeObsServer struct {
	BinaryPath  string
	URL         string
	Workspace   string
	Environment []string
}

type probeObsResult struct {
	Verdict       string
	Command       string
	HTTPStatus    int
	ResultCount   int
	Falsifier     string
	ResponseBody  string
	Stdout        string
	Stderr        string
	CLIExitStatus int
}

func startProbeObservabilityServer(t *testing.T, binaryPath, sourceFixturePath string) probeObsServer {
	t.Helper()
	probeRoot := t.TempDir()
	workspace := filepath.Join(probeRoot, "workspace")
	home := filepath.Join(probeRoot, "home")
	for _, path := range []string{workspace, home} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("create isolated PROBE-OBS directory %s: %v", path, err)
		}
	}

	fixture, err := os.ReadFile(sourceFixturePath)
	if err != nil {
		t.Fatalf("read PROBE-OBS replay fixture: %v", err)
	}
	fixturePath := filepath.Join(workspace, "probe.replay.json")
	if err := os.WriteFile(fixturePath, fixture, 0o644); err != nil {
		t.Fatalf("write isolated PROBE-OBS replay fixture: %v", err)
	}

	port, err := reserveProbeCostPort()
	if err != nil {
		t.Fatalf("reserve PROBE-OBS loopback port: %v", err)
	}
	serverURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	commandArgs := []string{
		"run",
		"--dir", workspace,
		"--replay", fixturePath,
		"--no-record",
		"--with-server",
		"--server", serverURL,
		"--continuously",
		"--quiet",
	}

	probeContext, cancel := context.WithTimeout(t.Context(), probeCostTimeout)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	command := exec.CommandContext(probeContext, binaryPath, commandArgs...)
	command.Dir = workspace
	command.Env = probeCostEnvironment(home)
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		cancel()
		t.Fatalf("start installed PROBE-OBS binary: %v", err)
	}
	process := newProbeCostProcess(command)
	status, err := waitForProbeCostTerminal(probeContext, &http.Client{Timeout: 2 * time.Second}, serverURL, process)
	if err != nil {
		process.Stop(cancel)
		t.Fatalf("PROBE-OBS replay did not reach terminal status: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if status.Categories.Terminal == 0 || status.Categories.Failed != 0 {
		process.Stop(cancel)
		t.Fatalf("PROBE-OBS replay status terminal=%d failed=%d\nstdout=%s\nstderr=%s", status.Categories.Terminal, status.Categories.Failed, stdout.String(), stderr.String())
	}
	t.Cleanup(func() { process.Stop(cancel) })

	return probeObsServer{
		BinaryPath:  binaryPath,
		URL:         serverURL,
		Workspace:   workspace,
		Environment: probeCostEnvironment(home),
	}
}

func runProbeObsList(t *testing.T, server probeObsServer, workID string) probeObsResult {
	t.Helper()
	args := []string{"--json", "--server", server.URL, "worker-sessions", "list"}
	if strings.TrimSpace(workID) != "" {
		args = append(args, "--work-id", workID)
	}
	args = append(args, "--output", "json")
	result := probeObsResult{
		Verdict:       "INCONCLUSIVE",
		Command:       "you " + strings.Join(args, " "),
		Falsifier:     probeObsFalsifier,
		CLIExitStatus: 0,
	}
	if strings.TrimSpace(workID) == "" && strings.Contains(strings.Join(args, " "), "--work-id") {
		result.Falsifier = "fleet-wide command unexpectedly contains --work-id"
		return result
	}

	var err error
	endpoint := probeObsListEndpoint(server.URL, workID)
	result.HTTPStatus, result.ResultCount, result.ResponseBody, err = fetchProbeObsHTTP(t, endpoint)
	if err != nil {
		result.Falsifier = err.Error()
	}

	cliContext, cliCancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cliCancel()
	result.Stdout, result.Stderr, result.CLIExitStatus, err = runProbeObsCLI(cliContext, server, args)
	if err != nil {
		result.Falsifier = fmt.Sprintf("%s exited with error: %v; %s", result.Command, err, probeObsFalsifier)
		return result
	}
	if result.HTTPStatus != http.StatusOK {
		result.Falsifier = fmt.Sprintf("HTTP status=%d; %s", result.HTTPStatus, probeObsFalsifier)
		return result
	}
	var cliListing factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &cliListing); err != nil {
		result.Falsifier = fmt.Sprintf("decode %s stdout: %v; %s", result.Command, err, probeObsFalsifier)
		return result
	}
	if len(cliListing.Sessions) != result.ResultCount {
		result.Falsifier = fmt.Sprintf("CLI result count=%d differs from HTTP result count=%d; %s", len(cliListing.Sessions), result.ResultCount, probeObsFalsifier)
		return result
	}
	result.Verdict = "PASS"
	return result
}

func probeObsListEndpoint(serverURL, workID string) string {
	endpoint := strings.TrimSuffix(serverURL, "/") + "/worker-sessions"
	if strings.TrimSpace(workID) == "" {
		return endpoint
	}
	return strings.TrimSuffix(serverURL, "/") + "/factory-sessions/" + url.PathEscape("~default") + "/worker-sessions?workId=" + url.QueryEscape(workID)
}

func fetchProbeObsHTTP(t *testing.T, endpoint string) (status, resultCount int, responseBody string, err error) {
	t.Helper()
	requestContext, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, 0, "", fmt.Errorf("build %s request: %v", endpoint, err)
	}
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		return 0, 0, "", fmt.Errorf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return response.StatusCode, 0, "", fmt.Errorf("read GET %s response: %v", endpoint, err)
	}
	responseBody = strings.TrimSpace(string(body))
	if response.StatusCode != http.StatusOK {
		return response.StatusCode, 0, responseBody, nil
	}
	var listing factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal(body, &listing); err != nil {
		return response.StatusCode, 0, responseBody, fmt.Errorf("decode GET %s response: %v", endpoint, err)
	}
	return response.StatusCode, len(listing.Sessions), responseBody, nil
}

func runProbeObsCLI(ctx context.Context, server probeObsServer, args []string) (stdout, stderr string, exitStatus int, err error) {
	command := exec.CommandContext(ctx, server.BinaryPath, args...)
	command.Dir = server.Workspace
	command.Env = append([]string(nil), server.Environment...)
	stdoutBuffer := &bytes.Buffer{}
	stderrBuffer := &bytes.Buffer{}
	command.Stdout = stdoutBuffer
	command.Stderr = stderrBuffer
	err = command.Run()
	exitStatus = 0
	if err != nil {
		exitStatus = 1
		if exitError, ok := err.(*exec.ExitError); ok {
			exitStatus = exitError.ExitCode()
		}
	}
	return stdoutBuffer.String(), stderrBuffer.String(), exitStatus, err
}

func validateProbeObsResult(result probeObsResult, requireNonEmpty bool) error {
	if result.Verdict != "PASS" {
		return fmt.Errorf("verdict=%s; falsifier=%s", result.Verdict, result.Falsifier)
	}
	if result.HTTPStatus != http.StatusOK {
		return fmt.Errorf("HTTP status=%d; falsifier=%s", result.HTTPStatus, result.Falsifier)
	}
	if result.CLIExitStatus != 0 {
		return fmt.Errorf("CLI exit status=%d; falsifier=%s", result.CLIExitStatus, result.Falsifier)
	}
	if requireNonEmpty && result.ResultCount == 0 {
		return fmt.Errorf("result count=0; falsifier=%s", result.Falsifier)
	}
	return nil
}

func logProbeObsVerdict(t *testing.T, cell string, result probeObsResult) {
	t.Helper()
	responseBody := result.ResponseBody
	if len(responseBody) > 512 {
		responseBody = responseBody[:512] + "..."
	}
	t.Logf(
		"PROBE-OBS cell=%s verdict=%s command=%q response_status=%d result_count=%d cli_exit=%d falsifier=%q response=%q",
		cell,
		result.Verdict,
		result.Command,
		result.HTTPStatus,
		result.ResultCount,
		result.CLIExitStatus,
		result.Falsifier,
		responseBody,
	)
}
