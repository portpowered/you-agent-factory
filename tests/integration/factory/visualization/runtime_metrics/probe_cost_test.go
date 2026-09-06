package runtime_metrics_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	probeCostExpectedProvider = "CODEX"
	probeCostExpectedModel    = "gpt-5-codex"
	probeCostExpectedAmount   = "21.25"
	probeCostExpectedInput    = int64(1_000_000)
	probeCostExpectedOutput   = int64(2_000_000)
	// The probe has no in-process readiness hook: it deliberately starts the
	// installed CLI and observes its public server. This is an overall process
	// safety bound, not a replacement for the terminal-status observation.
	probeCostTimeout = 30 * time.Second
	// The replay server uses --continuously so the customer-facing query can
	// run after replay reaches terminal state. This is the bounded cleanup
	// grace period before terminating that deliberately long-lived process.
	probeCostProcessStopWait = 5 * time.Second
	probeCostArtifactEnv     = "INFINITE_YOU_PREBUILT_ARTIFACT"
)

const probeCostFalsifier = "reject UNPRICED, NO_USAGE, zero/null known_cost, missing measured row, or provider/model drift"

// TestProbeCostReplaysTheInstalledCodexFixture is the PROBE-COST cell from
// the program verification plan. It intentionally crosses the OS process
// boundary: the checked-in recording is copied into a blank workspace, a pinned
// prebuilt artifact is run with a blank operator home, and the result is read
// back through the customer-facing metrics costs command.
func TestProbeCostReplaysTheInstalledCodexFixture(t *testing.T) {
	binaryPath := prebuiltProbeCostArtifact(t)
	fixturePath := testutil.MustRepoPath(t,
		"tests/functional/factory/visualization/runtime_metrics/testdata/codex-gpt-5-codex.factory-recording.v1.json",
	)

	passing := runProbeCost(t, binaryPath, fixturePath, nil)
	logProbeCostVerdict(t, passing)
	if passing.Verdict != "PASS" {
		t.Fatalf("PROBE-COST final fixture verdict = %s: %s\nstdout=%s\nstderr=%s", passing.Verdict, passing.Falsifier, passing.Stdout, passing.Stderr)
	}

	// This is an isolated copy of the checked-in fixture. Changing only the
	// resolved model preserves the recorded token usage while removing the
	// shipped price-table match. A guard that only checks for a row or a
	// non-null field would incorrectly pass this run.
	broken := runProbeCost(t, binaryPath, fixturePath, makeProbeCostFixtureUnpriced)
	logProbeCostVerdict(t, broken)
	if broken.Verdict != "FAIL" {
		t.Fatalf("PROBE-COST deliberate unpriced-model verdict = %s, want FAIL: %s\nstdout=%s\nstderr=%s", broken.Verdict, broken.Falsifier, broken.Stdout, broken.Stderr)
	}
}

// TestProbeMetricsSessionUsesInstalledArtifact proves the restored session
// command crosses the delivered executable boundary from a blank query
// directory. The replay server owns the fixture; the customer query does not
// read that workspace or any local metrics artifact.
func TestProbeMetricsSessionUsesInstalledArtifact(t *testing.T) {
	binaryPath := prebuiltProbeCostArtifact(t)
	fixturePath := testutil.MustRepoPath(t,
		"tests/functional/factory/visualization/runtime_metrics/testdata/codex-gpt-5-codex.factory-recording.v1.json",
	)
	fixture, err := prepareProbeCostFixture(t, fixturePath, nil)
	if err != nil {
		t.Fatalf("prepare installed session fixture: %v", err)
	}
	replay, err := startProbeCostReplay(t, binaryPath, fixture)
	if err != nil {
		t.Fatalf("start installed session replay: %v", err)
	}
	defer replay.stop()

	status, err := waitForProbeCostTerminal(replay.ctx, &http.Client{Timeout: 2 * time.Second}, replay.serverURL, replay.process)
	if err != nil {
		t.Fatalf("installed session replay did not reach terminal status: %v", err)
	}
	if status.Categories.Terminal == 0 || status.Categories.Failed != 0 {
		t.Fatalf("installed session replay status terminal=%d failed=%d", status.Categories.Terminal, status.Categories.Failed)
	}

	queryDirectory := t.TempDir()
	stdout, stderr, exitStatus, err := runProbeInstalledCLI(replay, binaryPath, queryDirectory, []string{
		"--json", "--server", replay.serverURL, "metrics", "session", "~default",
		"--lens", "cost", "--by-worker", "--by-dispatch",
	})
	if err != nil || exitStatus != 0 {
		t.Fatalf("installed metrics session command failed: exit=%d err=%v\nstdout=%s\nstderr=%s", exitStatus, err, stdout, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("installed metrics session stderr = %q, want empty", stderr)
	}
	var report struct {
		FactorySessionID string `json:"factory_session_id"`
		Attempts         []struct {
			DispatchID      string `json:"dispatch_id"`
			WorkID          string `json:"work_id"`
			WorkerSessionID string `json:"worker_session_id"`
		} `json:"attempts"`
		Cost struct {
			Status    string  `json:"status"`
			KnownCost *string `json:"known_cost"`
		} `json:"cost"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &report); err != nil {
		t.Fatalf("decode installed metrics session JSON: %v\nstdout=%s", err, stdout)
	}
	if report.FactorySessionID != "~default" || len(report.Attempts) != 1 || report.Cost.Status != "PRICED" || report.Cost.KnownCost == nil || *report.Cost.KnownCost != probeCostExpectedAmount {
		t.Fatalf("installed metrics session report = %#v, want one selected priced attempt", report)
	}
	attempt := report.Attempts[0]
	if attempt.DispatchID != "cost-replay-priced-dispatch" || attempt.WorkID != "cost-replay-priced-work" || attempt.WorkerSessionID != "cost-replay-priced-worker-session" {
		t.Fatalf("installed metrics session attempt = %#v, want canonical replay identities", attempt)
	}
	if entries, err := os.ReadDir(queryDirectory); err != nil {
		t.Fatalf("read blank query directory: %v", err)
	} else if len(entries) != 0 {
		t.Fatalf("blank query directory contains %d entries, want none", len(entries))
	}
}

// TestProbeCostValidatorRejectsFalsePositiveReports keeps the adversarial
// assertions independent from the production valuation implementation. The
// runtime probe above proves the customer-facing process path; these cases
// prove that each false-positive shape is rejected by the probe oracle.
func TestProbeCostValidatorRejectsFalsePositiveReports(t *testing.T) {
	base := validProbeCostReport()
	cases := []struct {
		name   string
		mutate func(*generatedclient.CostsReport)
	}{
		{name: "unpriced", mutate: func(report *generatedclient.CostsReport) {
			report.Status = generatedclient.CostsReportStatus("UNPRICED")
		}},
		{name: "no usage", mutate: func(report *generatedclient.CostsReport) {
			report.Status = generatedclient.CostsReportStatus("NO_USAGE")
		}},
		{name: "zero known cost", mutate: func(report *generatedclient.CostsReport) {
			report.KnownCost = stringPointer("0")
		}},
		{name: "null known cost", mutate: func(report *generatedclient.CostsReport) {
			report.KnownCost = nil
		}},
		{name: "missing measured row", mutate: func(report *generatedclient.CostsReport) {
			report.LineItems = nil
		}},
		{name: "different provider model", mutate: func(report *generatedclient.CostsReport) {
			report.LineItems[0].Model = stringPointer("unpriced-model")
			report.ProviderModels[0].Model = "unpriced-model"
		}},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			report := cloneProbeCostReport(base)
			test.mutate(&report)
			if err := validateProbeCostReport(report); err == nil {
				t.Fatalf("validator accepted %s false-positive shape", test.name)
			}
		})
	}
}

type probeCostResult struct {
	Verdict           string
	Command           string
	ObservedStatus    string
	ObservedKnownCost string
	Provider          string
	Model             string
	Falsifier         string
	Stdout            string
	Stderr            string
}

type probeCostFixture struct {
	workspacePath string
	homePath      string
	fixturePath   string
}

type probeCostReplay struct {
	ctx           context.Context
	cancel        context.CancelFunc
	serverURL     string
	workspacePath string
	environment   []string
	process       *probeCostProcess
	serverStdout  *bytes.Buffer
	serverStderr  *bytes.Buffer
}

func runProbeCost(
	t *testing.T,
	binaryPath string,
	sourceFixturePath string,
	mutate func([]byte) ([]byte, error),
) probeCostResult {
	t.Helper()
	result := probeCostResult{
		Verdict:   "INCONCLUSIVE",
		Falsifier: probeCostFalsifier,
	}

	fixture, err := prepareProbeCostFixture(t, sourceFixturePath, mutate)
	if err != nil {
		result.Falsifier = err.Error()
		return result
	}

	replay, err := startProbeCostReplay(t, binaryPath, fixture)
	if err != nil {
		result.Falsifier = err.Error()
		return result
	}
	defer replay.stop()
	result.Command = replay.command(fixture.fixturePath)

	client := &http.Client{Timeout: 2 * time.Second}
	status, err := waitForProbeCostTerminal(replay.ctx, client, replay.serverURL, replay.process)
	if err != nil {
		result.Falsifier = fmt.Sprintf("replay did not reach terminal public status: %v", err)
		result.Stdout, result.Stderr = replay.serverOutput()
		return result
	}
	if status.Categories.Terminal == 0 || status.Categories.Failed != 0 {
		result.Falsifier = fmt.Sprintf("replay status terminal=%d failed=%d", status.Categories.Terminal, status.Categories.Failed)
		result.Stdout, result.Stderr = replay.serverOutput()
		return result
	}

	report, stdout, stderr, err := queryProbeCostReport(replay, binaryPath)
	result.Stdout, result.Stderr = stdout, stderr
	if err != nil {
		result.Falsifier = err.Error()
		return result
	}
	result.ObservedStatus, result.ObservedKnownCost, result.Provider, result.Model = observeProbeCostReport(report)
	if err := validateProbeCostReport(report); err != nil {
		result.Verdict = "FAIL"
		result.Falsifier = err.Error()
		return result
	}
	result.Verdict = "PASS"
	return result
}

func prepareProbeCostFixture(
	t *testing.T,
	sourceFixturePath string,
	mutate func([]byte) ([]byte, error),
) (probeCostFixture, error) {
	t.Helper()
	probeRoot := t.TempDir()
	fixture := probeCostFixture{
		workspacePath: filepath.Join(probeRoot, "workspace"),
		homePath:      filepath.Join(probeRoot, "home"),
	}
	for _, path := range []string{fixture.workspacePath, fixture.homePath} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return probeCostFixture{}, fmt.Errorf("prepare isolated directory %s: %v", path, err)
		}
	}

	contents, err := os.ReadFile(sourceFixturePath)
	if err != nil {
		return probeCostFixture{}, fmt.Errorf("read checked-in fixture: %v", err)
	}
	if mutate != nil {
		contents, err = mutate(contents)
		if err != nil {
			return probeCostFixture{}, fmt.Errorf("prepare isolated fixture copy: %v", err)
		}
	}
	fixture.fixturePath = filepath.Join(fixture.workspacePath, "probe.replay.json")
	if err := os.WriteFile(fixture.fixturePath, contents, 0o644); err != nil {
		return probeCostFixture{}, fmt.Errorf("write isolated fixture copy: %v", err)
	}
	return fixture, nil
}

func startProbeCostReplay(t *testing.T, binaryPath string, fixture probeCostFixture) (*probeCostReplay, error) {
	t.Helper()
	port, err := reserveProbeCostPort()
	if err != nil {
		return nil, fmt.Errorf("reserve isolated loopback port: %v", err)
	}
	serverURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	commandArgs := []string{
		"run", "--dir", fixture.workspacePath, "--replay", fixture.fixturePath,
		"--no-record", "--with-server", "--server", serverURL, "--continuously", "--quiet",
	}
	ctx, cancel := context.WithTimeout(t.Context(), probeCostTimeout)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	command := exec.CommandContext(ctx, binaryPath, commandArgs...)
	command.Dir = fixture.workspacePath
	command.Env = probeCostEnvironment(fixture.homePath)
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start installed binary: %v", err)
	}
	return &probeCostReplay{
		ctx:           ctx,
		cancel:        cancel,
		serverURL:     serverURL,
		workspacePath: fixture.workspacePath,
		environment:   probeCostEnvironment(fixture.homePath),
		process:       newProbeCostProcess(command),
		serverStdout:  stdout,
		serverStderr:  stderr,
	}, nil
}

func (replay *probeCostReplay) command(fixturePath string) string {
	return fmt.Sprintf("you run --dir %s --replay %s --no-record --with-server --server %s --continuously --quiet ; you --json --server %s metrics costs",
		filepath.Dir(fixturePath), fixturePath, replay.serverURL, replay.serverURL)
}

func (replay *probeCostReplay) serverOutput() (string, string) {
	return replay.serverStdout.String(), replay.serverStderr.String()
}

func (replay *probeCostReplay) stop() {
	// The replay server is intentionally long-lived so a second installed-binary
	// command can query it after replay. There is no in-band shutdown event, and
	// replacing this process cleanup with an edge mock would skip the OS boundary
	// this probe exists to verify, so wait briefly before forcing termination.
	replay.process.Stop(replay.cancel)
}

func queryProbeCostReport(replay *probeCostReplay, binaryPath string) (generatedclient.CostsReport, string, string, error) {
	metricsArgs := []string{"--json", "--server", replay.serverURL, "metrics", "costs"}
	command := exec.CommandContext(replay.ctx, binaryPath, metricsArgs...)
	command.Dir = replay.workspacePath
	command.Env = append([]string(nil), replay.environment...)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		return generatedclient.CostsReport{}, stdout.String(), stderr.String(), fmt.Errorf("you --json metrics costs exited with error: %v", err)
	}
	var report generatedclient.CostsReport
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &report); err != nil {
		return generatedclient.CostsReport{}, stdout.String(), stderr.String(), fmt.Errorf("decode metrics costs JSON: %v", err)
	}
	return report, stdout.String(), stderr.String(), nil
}

func runProbeInstalledCLI(replay *probeCostReplay, binaryPath, workingDirectory string, args []string) (stdout, stderr string, exitStatus int, err error) {
	command := exec.CommandContext(replay.ctx, binaryPath, args...)
	command.Dir = workingDirectory
	command.Env = append([]string(nil), replay.environment...)
	stdoutBuffer := &bytes.Buffer{}
	stderrBuffer := &bytes.Buffer{}
	command.Stdout = stdoutBuffer
	command.Stderr = stderrBuffer
	err = command.Run()
	stdout = stdoutBuffer.String()
	stderr = stderrBuffer.String()
	if err == nil {
		return stdout, stderr, 0, nil
	}
	exitStatus = 1
	if exitError, ok := err.(*exec.ExitError); ok {
		exitStatus = exitError.ExitCode()
	}
	return stdout, stderr, exitStatus, err
}

func logProbeCostVerdict(t *testing.T, result probeCostResult) {
	t.Helper()
	knownCost := result.ObservedKnownCost
	if knownCost == "" {
		knownCost = "null"
	}
	t.Logf(
		"PROBE-COST verdict=%s command=%q observed_status=%q observed_known_cost=%s provider=%q model=%q falsifier=%q",
		result.Verdict,
		result.Command,
		result.ObservedStatus,
		knownCost,
		result.Provider,
		result.Model,
		result.Falsifier,
	)
}

func prebuiltProbeCostArtifact(t *testing.T) string {
	t.Helper()
	binaryPath := strings.TrimSpace(os.Getenv(probeCostArtifactEnv))
	if binaryPath == "" {
		t.Skipf("PROBE-COST requires a pinned prebuilt artifact; set %s to its path", probeCostArtifactEnv)
	}
	info, err := os.Stat(binaryPath)
	if err != nil {
		t.Fatalf("stat pinned PROBE-COST artifact %q: %v", binaryPath, err)
	}
	if info.IsDir() {
		t.Fatalf("pinned PROBE-COST artifact %q is a directory", binaryPath)
	}
	return binaryPath
}

func reserveProbeCostPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("listener address has type %T", listener.Addr())
	}
	return address.Port, nil
}

func probeCostEnvironment(homePath string) []string {
	environment := make([]string, 0, len(os.Environ())+4)
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		switch key {
		case "HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH":
			continue
		}
		environment = append(environment, entry)
	}
	environment = append(environment,
		"HOME="+homePath,
		"USERPROFILE="+homePath,
		"HOMEDRIVE="+filepath.VolumeName(homePath),
		"HOMEPATH="+string(filepath.Separator),
	)
	return environment
}

type probeCostProcess struct {
	command *exec.Cmd
	done    chan struct{}

	mu  sync.Mutex
	err error
}

func newProbeCostProcess(command *exec.Cmd) *probeCostProcess {
	process := &probeCostProcess{command: command, done: make(chan struct{})}
	go func() {
		err := command.Wait()
		process.mu.Lock()
		process.err = err
		process.mu.Unlock()
		close(process.done)
	}()
	return process
}

func (process *probeCostProcess) ExitError() (bool, error) {
	select {
	case <-process.done:
		process.mu.Lock()
		defer process.mu.Unlock()
		if process.err != nil {
			return true, process.err
		}
		return true, errors.New("installed binary exited before the cost probe completed")
	default:
		return false, nil
	}
}

// Stop cleans up the deliberately long-lived --continuously replay process.
// The public metrics query must run after terminal replay, so a deterministic
// completion event cannot replace this shutdown path; the process boundary is
// the behavior under test. Give graceful cancellation a bounded chance before
// killing the child so a failed probe cannot leak a server into later tests.
func (process *probeCostProcess) Stop(cancel context.CancelFunc) {
	cancel()
	select {
	case <-process.done:
		return
	case <-time.After(probeCostProcessStopWait):
		if process.command.Process != nil {
			_ = process.command.Process.Kill()
		}
		<-process.done
	}
}

// waitForProbeCostTerminal observes readiness through the replay server's
// customer-facing /status endpoint. The installed CLI exposes no readiness
// channel, and replacing this poll with an edge mock would skip the OS/process
// boundary that PROBE-COST and PROBE-OBS are required to verify.
func waitForProbeCostTerminal(
	ctx context.Context,
	client *http.Client,
	serverURL string,
	process *probeCostProcess,
) (factoryapi.StatusResponse, error) {
	endpoint := strings.TrimSuffix(serverURL, "/") + "/status"
	var lastError error
	for {
		if exited, err := process.ExitError(); exited {
			return factoryapi.StatusResponse{}, err
		}

		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return factoryapi.StatusResponse{}, err
		}
		response, err := client.Do(request)
		if err != nil {
			lastError = err
		} else {
			var status factoryapi.StatusResponse
			decodeErr := json.NewDecoder(response.Body).Decode(&status)
			response.Body.Close()
			if decodeErr != nil {
				lastError = decodeErr
			} else if response.StatusCode != http.StatusOK {
				lastError = fmt.Errorf("GET /status returned HTTP %d", response.StatusCode)
			} else if isProbeCostTerminal(status) {
				return status, nil
			}
		}

		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			if lastError != nil {
				return factoryapi.StatusResponse{}, fmt.Errorf("%w (last status error: %v)", ctx.Err(), lastError)
			}
			return factoryapi.StatusResponse{}, ctx.Err()
		case <-timer.C:
		}
	}
}

func isProbeCostTerminal(status factoryapi.StatusResponse) bool {
	return status.Categories.Terminal+status.Categories.Failed > 0 &&
		status.Categories.Initial == 0 &&
		status.Categories.Processing == 0 &&
		(status.RuntimeStatus == "IDLE" || status.RuntimeStatus == "FINISHED")
}

func makeProbeCostFixtureUnpriced(contents []byte) ([]byte, error) {
	const pricedModel = `"gpt-5-codex"`
	const unpricedModel = `"probe-unpriced-model"`
	if count := bytes.Count(contents, []byte(pricedModel)); count != 2 {
		return nil, fmt.Errorf("fixture model occurrence count = %d, want 2", count)
	}
	return bytes.ReplaceAll(contents, []byte(pricedModel), []byte(unpricedModel)), nil
}

func validateProbeCostReport(report generatedclient.CostsReport) error {
	if report.Status != generatedclient.CostsReportStatus("PRICED") {
		return fmt.Errorf("status=%q; expected PRICED, falsifier=%s", report.Status, probeCostFalsifier)
	}
	if report.KnownCost == nil {
		return fmt.Errorf("known_cost=null; expected non-zero exact %s, falsifier=%s", probeCostExpectedAmount, probeCostFalsifier)
	}
	if strings.TrimSpace(*report.KnownCost) == "" || strings.Trim(*report.KnownCost, "0.") == "" {
		return fmt.Errorf("known_cost=%q; expected non-zero exact %s, falsifier=%s", *report.KnownCost, probeCostExpectedAmount, probeCostFalsifier)
	}
	if *report.KnownCost != probeCostExpectedAmount {
		return fmt.Errorf("known_cost=%q; expected exact %s, falsifier=%s", *report.KnownCost, probeCostExpectedAmount, probeCostFalsifier)
	}

	measured := false
	for _, item := range report.LineItems {
		if item.Provider == nil || item.Model == nil || *item.Provider != probeCostExpectedProvider || *item.Model != probeCostExpectedModel {
			continue
		}
		if item.Status != generatedclient.CostsLineItemStatus("PRICED") || item.InputTokens == nil || item.OutputTokens == nil ||
			*item.InputTokens != probeCostExpectedInput || *item.OutputTokens != probeCostExpectedOutput ||
			item.PricedAmount == nil || *item.PricedAmount != probeCostExpectedAmount {
			return fmt.Errorf("measured row for %s/%s is incomplete or false-valued: %#v; falsifier=%s", probeCostExpectedProvider, probeCostExpectedModel, item, probeCostFalsifier)
		}
		measured = true
	}
	if !measured {
		return fmt.Errorf("missing measured row for %s/%s; falsifier=%s", probeCostExpectedProvider, probeCostExpectedModel, probeCostFalsifier)
	}

	resolved := false
	for _, rollup := range report.ProviderModels {
		if rollup.Provider == probeCostExpectedProvider && rollup.Model == probeCostExpectedModel {
			if rollup.Status != generatedclient.CostsProviderModelRollupStatus("PRICED") || rollup.KnownCost == nil || *rollup.KnownCost != probeCostExpectedAmount {
				return fmt.Errorf("provider/model rollup %s/%s is not exactly priced: %#v; falsifier=%s", probeCostExpectedProvider, probeCostExpectedModel, rollup, probeCostFalsifier)
			}
			resolved = true
		}
	}
	if !resolved {
		return fmt.Errorf("provider/model resolution did not produce %s/%s; falsifier=%s", probeCostExpectedProvider, probeCostExpectedModel, probeCostFalsifier)
	}
	return nil
}

func observeProbeCostReport(report generatedclient.CostsReport) (status, knownCost, provider, model string) {
	status = string(report.Status)
	if report.KnownCost != nil {
		knownCost = *report.KnownCost
	}
	if len(report.ProviderModels) > 0 {
		provider = report.ProviderModels[0].Provider
		model = report.ProviderModels[0].Model
	}
	if (provider == "" || model == "") && len(report.LineItems) > 0 {
		item := report.LineItems[0]
		if item.Provider != nil {
			provider = *item.Provider
		}
		if item.Model != nil {
			model = *item.Model
		}
	}
	return status, knownCost, provider, model
}

func validProbeCostReport() generatedclient.CostsReport {
	knownCost := probeCostExpectedAmount
	pricedAmount := probeCostExpectedAmount
	input := probeCostExpectedInput
	output := probeCostExpectedOutput
	return generatedclient.CostsReport{
		Status:    generatedclient.CostsReportStatus("PRICED"),
		KnownCost: &knownCost,
		LineItems: []generatedclient.CostsLineItem{{
			Provider:     stringPointer(probeCostExpectedProvider),
			Model:        stringPointer(probeCostExpectedModel),
			Status:       generatedclient.CostsLineItemStatus("PRICED"),
			InputTokens:  &input,
			OutputTokens: &output,
			PricedAmount: &pricedAmount,
		}},
		ProviderModels: []generatedclient.CostsProviderModelRollup{{
			Provider:  probeCostExpectedProvider,
			Model:     probeCostExpectedModel,
			Status:    generatedclient.CostsProviderModelRollupStatus("PRICED"),
			KnownCost: &knownCost,
		}},
	}
}

func cloneProbeCostReport(report generatedclient.CostsReport) generatedclient.CostsReport {
	encoded, err := json.Marshal(report)
	if err != nil {
		panic(err)
	}
	var clone generatedclient.CostsReport
	if err := json.Unmarshal(encoded, &clone); err != nil {
		panic(err)
	}
	return clone
}

func stringPointer(value string) *string {
	return &value
}
