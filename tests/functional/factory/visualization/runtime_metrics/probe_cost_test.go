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
	"runtime"
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
	probeCostTimeout          = 30 * time.Second
	probeCostProcessStopWait  = 5 * time.Second
	probeCostEnableEnv        = "INFINITE_YOU_ENABLE_PROBE_COST"
)

const probeCostFalsifier = "reject UNPRICED, NO_USAGE, zero/null known_cost, missing measured row, or provider/model drift"

// TestProbeCostReplaysTheInstalledCodexFixture is the PROBE-COST cell from
// the program verification plan. It intentionally crosses the OS process
// boundary: the checked-in recording is copied into a blank workspace, an
// installed-style binary is run with a blank operator home, and the result is
// read back through the customer-facing metrics costs command.
func TestProbeCostReplaysTheInstalledCodexFixture(t *testing.T) {
	if os.Getenv(probeCostEnableEnv) != "1" {
		t.Skipf("PROBE-COST is non-blocking until Integration Story 0 (#2191) merges; set %s=1 to run it", probeCostEnableEnv)
	}

	binaryPath := buildProbeCostBinary(t)
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

	probeRoot := t.TempDir()
	workspacePath := filepath.Join(probeRoot, "workspace")
	homePath := filepath.Join(probeRoot, "home")
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		result.Falsifier = fmt.Sprintf("prepare isolated workspace: %v", err)
		return result
	}
	if err := os.MkdirAll(homePath, 0o755); err != nil {
		result.Falsifier = fmt.Sprintf("prepare isolated home: %v", err)
		return result
	}

	fixture, err := os.ReadFile(sourceFixturePath)
	if err != nil {
		result.Falsifier = fmt.Sprintf("read checked-in fixture: %v", err)
		return result
	}
	if mutate != nil {
		fixture, err = mutate(fixture)
		if err != nil {
			result.Falsifier = fmt.Sprintf("prepare isolated fixture copy: %v", err)
			return result
		}
	}
	fixturePath := filepath.Join(workspacePath, "probe.replay.json")
	if err := os.WriteFile(fixturePath, fixture, 0o644); err != nil {
		result.Falsifier = fmt.Sprintf("write isolated fixture copy: %v", err)
		return result
	}

	port, err := reserveProbeCostPort()
	if err != nil {
		result.Falsifier = fmt.Sprintf("reserve isolated loopback port: %v", err)
		return result
	}
	serverURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	commandArgs := []string{
		"run",
		"--dir", workspacePath,
		"--replay", fixturePath,
		"--no-record",
		"--with-server",
		"--server", serverURL,
		"--continuously",
		"--quiet",
	}
	result.Command = "you " + strings.Join(commandArgs, " ") + " ; you --json --server " + serverURL + " metrics costs"

	probeContext, cancel := context.WithTimeout(t.Context(), probeCostTimeout)
	defer cancel()
	serverStdout := &bytes.Buffer{}
	serverStderr := &bytes.Buffer{}
	serverCommand := exec.CommandContext(probeContext, binaryPath, commandArgs...)
	serverCommand.Dir = workspacePath
	serverCommand.Env = probeCostEnvironment(homePath)
	serverCommand.Stdout = serverStdout
	serverCommand.Stderr = serverStderr
	if err := serverCommand.Start(); err != nil {
		result.Falsifier = fmt.Sprintf("start installed binary: %v", err)
		result.Stderr = serverStderr.String()
		return result
	}
	running := newProbeCostProcess(serverCommand)
	defer running.Stop(cancel)

	client := &http.Client{Timeout: 2 * time.Second}
	status, err := waitForProbeCostTerminal(probeContext, client, serverURL, running)
	if err != nil {
		result.Falsifier = fmt.Sprintf("replay did not reach terminal public status: %v", err)
		result.Stdout = serverStdout.String()
		result.Stderr = serverStderr.String()
		return result
	}
	if status.Categories.Terminal == 0 || status.Categories.Failed != 0 {
		result.Falsifier = fmt.Sprintf("replay status terminal=%d failed=%d", status.Categories.Terminal, status.Categories.Failed)
		result.Stdout = serverStdout.String()
		result.Stderr = serverStderr.String()
		return result
	}

	metricsArgs := []string{"--json", "--server", serverURL, "metrics", "costs"}
	metricsCommand := exec.CommandContext(probeContext, binaryPath, metricsArgs...)
	metricsCommand.Dir = workspacePath
	metricsCommand.Env = probeCostEnvironment(homePath)
	metricsStdout := &bytes.Buffer{}
	metricsStderr := &bytes.Buffer{}
	metricsCommand.Stdout = metricsStdout
	metricsCommand.Stderr = metricsStderr
	if err := metricsCommand.Run(); err != nil {
		result.Falsifier = fmt.Sprintf("%s exited with error: %v", "you --json metrics costs", err)
		result.Stdout = metricsStdout.String()
		result.Stderr = metricsStderr.String()
		return result
	}
	result.Stdout = metricsStdout.String()
	result.Stderr = metricsStderr.String()

	var report generatedclient.CostsReport
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &report); err != nil {
		result.Falsifier = fmt.Sprintf("decode metrics costs JSON: %v", err)
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

func buildProbeCostBinary(t *testing.T) string {
	t.Helper()
	binaryName := "you"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.CommandContext(t.Context(), "go", "build", "-buildvcs=false", "-o", binaryPath, "./cmd/factory")
	build.Dir = testutil.MustRepoRoot(t)
	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("build installed-style you binary: %v\n%s", err, output)
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
