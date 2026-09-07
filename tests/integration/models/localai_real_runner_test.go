//go:build windows

package models_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/locking"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"golang.org/x/sys/windows"
)

const (
	localAIRealEvidenceSchema  = "localai.windows-real-evidence.v1"
	localAIBudgetSchema        = "localai.windows-real-budget.v1"
	localAIRealHelperModeEnv   = "LOCALAI_REAL_HELPER_MODE"
	localAIRealHelperOutputEnv = "LOCALAI_REAL_HELPER_OUTPUT"
	localAIRealHelperResultEnv = "LOCALAI_REAL_HELPER_RESULT"
	localAIRealOutputToken     = "{output}"
	localAIRealRootToken       = "{root}"
	localAIRealWorkToken       = "{work}"
	localAIRealMaxStreamBytes  = 64 << 10
	localAIRealMaxFailureBytes = 192
	localAIRealCommandTimeout  = 10 * time.Second
)

type localAIRealReport struct {
	Schema       string                    `json:"schema"`
	RunID        string                    `json:"runId"`
	Status       string                    `json:"status"`
	Platform     string                    `json:"platform"`
	Architecture string                    `json:"architecture"`
	Build        localAIRealBuildIdentity  `json:"build"`
	BudgetLedger localAIRealLedgerIdentity `json:"budgetLedger"`
	Redacted     bool                      `json:"redacted"`
	Journeys     []localAIRealJourney      `json:"journeys"`
}

type localAIRealBuildIdentity struct {
	PathIdentity string `json:"pathIdentity"`
	Bytes        int64  `json:"bytes"`
	SHA256       string `json:"sha256"`
	Commit       string `json:"commit"`
	Tree         string `json:"tree"`
}

type localAIRealLedgerIdentity struct {
	PathIdentity string `json:"pathIdentity"`
	SHA256       string `json:"sha256"`
}

type localAIRealJourney struct {
	Selector            string                `json:"selector"`
	Status              string                `json:"status"`
	Artifacts           []localAIRealArtifact `json:"artifacts"`
	CacheIdentitySHA256 string                `json:"cacheIdentitySha256"`
	Offline             bool                  `json:"offline"`
	Semantic            localAIRealSemantic   `json:"semantic"`
	Release             localAIRealRelease    `json:"release"`
	Failure             *localAIRealFailure   `json:"failure"`
	Unproven            []string              `json:"unproven"`
}

type localAIRealArtifact struct {
	Kind      string `json:"kind"`
	Path      string `json:"pathIdentity"`
	MediaType string `json:"mediaType"`
	Bytes     int64  `json:"bytes"`
	SHA256    string `json:"sha256"`
}

type localAIRealSemantic struct {
	Assertion string `json:"assertion"`
	Expected  string `json:"expected"`
	Observed  string `json:"observed"`
	Passed    bool   `json:"passed"`
}

type localAIRealRelease struct {
	ProcessTreeClosed bool `json:"processTreeClosed"`
	OwnedProcesses    int  `json:"ownedProcesses"`
	OwnedListeners    int  `json:"ownedListeners"`
	OwnedLeases       int  `json:"ownedLeases"`
	PartialArtifacts  int  `json:"partialArtifacts"`
}

type localAIRealFailure struct {
	Owner     string `json:"owner"`
	Assertion string `json:"assertion"`
	Expected  string `json:"expected"`
	Observed  string `json:"observed"`
}

type localAIBudgetLedger struct {
	Schema       string                     `json:"schema"`
	RunID        string                     `json:"runId"`
	Limits       localAIBudgetLimits        `json:"limits"`
	Consumed     localAIBudgetConsumed      `json:"consumed"`
	Reservations []localAIBudgetReservation `json:"reservations"`
}

type localAIBudgetLimits struct {
	ModelCalls    int64 `json:"modelCalls"`
	DownloadBytes int64 `json:"downloadBytes"`
}

type localAIBudgetConsumed struct {
	ModelCalls    int64 `json:"modelCalls"`
	DownloadBytes int64 `json:"downloadBytes"`
}

type localAIBudgetReservation struct {
	ID      string `json:"id"`
	Journey string `json:"journey"`
	Kind    string `json:"kind"`
	Amount  int64  `json:"amount"`
	State   string `json:"state"`
}

type localAIRealRunRequest struct {
	Selector            string
	RunID               string
	Root                string
	ReportPath          string
	LedgerPath          string
	Limits              localAIBudgetLimits
	ReservationKind     string
	ReservationAmount   int64
	Command             localAICommandSpec
	Build               localAIRealBuildIdentity
	Offline             bool
	CacheIdentitySHA256 string
	Unproven            []string
	Timeout             time.Duration
}

type localAICommandSpec struct {
	BinaryPath  string
	Arguments   []string
	Environment []string
	OutputName  string
}

type localAIRealRoots struct {
	Root    string
	Work    string
	Profile string
	Cache   string
	Temp    string
	Output  string
	Streams string
}

type localAICommandObservation struct {
	Started             bool
	ProcessExited       bool
	ExitCode            int
	TimedOut            bool
	ProcessTreeAttached bool
	ProcessTreeClosed   bool
	Stdout              []byte
	Stderr              []byte
	StdoutTruncated     bool
	StderrTruncated     bool
}

type localAIRealCommandExecutor interface {
	Execute(context.Context, localAICommandSpec, localAIRealRoots) localAICommandObservation
}

type localAIRealRunner struct {
	executor    localAIRealCommandExecutor
	locks       locking.Service
	writeReport func(string, localAIRealReport) error
}

type localAIBudgetError struct {
	Code     string
	Consumed int64
	Limit    int64
}

func (err *localAIBudgetError) Error() string {
	return fmt.Sprintf("budget ledger %s (%d/%d)", err.Code, err.Consumed, err.Limit)
}

func TestLocalAIRealHarnessControlledCases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		mode       string
		wantStatus string
		wantOwner  string
		stub       *localAIStubExecutor
		setup      func(testing.TB, string, *localAIRealRunRequest)
	}{
		{name: "IC-01-valid-controlled-tts", mode: "pass", wantStatus: "PASS"},
		{name: "IC-05-malformed-tts-output", mode: "malformed", wantStatus: "FAIL", wantOwner: "product"},
		{name: "IC-08-cleanup-leak", mode: "pass", wantStatus: "FAIL", wantOwner: "harness", stub: &localAIStubExecutor{observation: localAICommandObservation{
			Started: true, ProcessExited: true, ExitCode: 0, ProcessTreeAttached: true,
		}}},
		{name: "IC-09-report-interruption", mode: "pass", wantStatus: "PASS", setup: setupLocalAIReportInterruption},
		{name: "IC-10-budget-exhaustion", mode: "pass", wantStatus: "FAIL", wantOwner: "harness", stub: &localAIStubExecutor{}, setup: setupLocalAIBudgetExhaustion},
		{name: "IC-11-corrupt-ledger", mode: "pass", wantStatus: "FAIL", wantOwner: "harness", stub: &localAIStubExecutor{}, setup: setupLocalAICorruptLedger},
		{name: "IC-14-redacted-invocation-failure", mode: "secret", wantStatus: "FAIL", wantOwner: "product"},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			request := localAIControlledRequest(t, root, testCase.mode)
			if testCase.setup != nil {
				testCase.setup(t, root, &request)
			}
			runner := mustLocalAIRealRunner(t)
			if testCase.name == "IC-09-report-interruption" {
				runner.writeReport = func(path string, report localAIRealReport) error {
					return writeLocalAIRealReportAtomicWithHook(path, report, func() error { return errLocalAIReportInterrupted })
				}
			}
			if testCase.stub != nil {
				runner.executor = testCase.stub
			}
			report, err := runner.Run(t.Context(), request)
			if testCase.name == "IC-09-report-interruption" {
				assertLocalAIReportInterruption(t, request, report, err)
				return
			}
			if err != nil {
				t.Fatalf("runner returned infrastructure error: %v", err)
			}
			assertLocalAIJourneyResult(t, request, report, testCase.wantStatus, testCase.wantOwner)
			if testCase.name == "IC-10-budget-exhaustion" && testCase.stub.calls.Load() != 0 {
				t.Fatal("budget exhaustion launched the controlled command")
			}
			if testCase.name == "IC-11-corrupt-ledger" && testCase.stub.calls.Load() != 0 {
				t.Fatal("corrupt ledger launched the controlled command")
			}
		})
	}
}

func TestLocalAIRealHarnessBudgetPersistence(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ledgerPath := filepath.Join(root, "localai-budget.json")
	limits := localAIBudgetLimits{ModelCalls: 1}
	if _, _, err := reserveLocalAIBudget(t.Context(), mustLocalAILockService(t), ledgerPath, "run-persistence", "known-tts", "modelCall", 1, limits); err != nil {
		t.Fatalf("initial reservation: %v", err)
	}
	resultPath := filepath.Join(root, "restart-result.txt")
	if got := runLocalAIReservationHelper(t, root, ledgerPath, resultPath); got != "BUDGET_EXHAUSTED" {
		t.Fatalf("restart helper result = %q, want BUDGET_EXHAUSTED", got)
	}
	ledger := readLocalAIBudgetForTest(t, ledgerPath)
	if ledger.Consumed.ModelCalls != 1 || len(ledger.Reservations) != 1 || ledger.Reservations[0].State != "RESERVED" {
		t.Fatalf("ledger after helper restart = %#v, want one durable RESERVED reservation", ledger)
	}

	before, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	errInterrupted := errors.New("controlled ledger interruption")
	err = writeLocalAIBudgetAtomicWithHook(ledgerPath, ledger, func() error { return errInterrupted })
	if !errors.Is(err, errInterrupted) {
		t.Fatalf("interrupted ledger write error = %v, want sentinel", err)
	}
	after, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("interrupted ledger write changed the canonical ledger")
	}

	parallelRoot := t.TempDir()
	parallelLedger := filepath.Join(parallelRoot, "localai-budget.json")
	parallelLimits := localAIBudgetLimits{ModelCalls: 3}
	const attempts = 9
	results := make(chan error, attempts)
	var group sync.WaitGroup
	for index := 0; index < attempts; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, _, reserveErr := reserveLocalAIBudget(t.Context(), mustLocalAILockService(t), parallelLedger, "run-race", "known-tts", "modelCall", 1, parallelLimits)
			results <- reserveErr
		}()
	}
	group.Wait()
	close(results)
	var admitted, exhausted int
	for reserveErr := range results {
		if reserveErr == nil {
			admitted++
			continue
		}
		var budgetErr *localAIBudgetError
		if !errors.As(reserveErr, &budgetErr) || budgetErr.Code != "budget_exhausted" {
			t.Fatalf("parallel reservation error = %v, want budget_exhausted", reserveErr)
		}
		exhausted++
	}
	if admitted != 3 || exhausted != attempts-3 {
		t.Fatalf("parallel admissions = %d, exhausted = %d, want 3 and %d", admitted, exhausted, attempts-3)
	}
}

func TestLocalAIRealHarnessControlledHelper(t *testing.T) {
	mode := strings.TrimSpace(os.Getenv(localAIRealHelperModeEnv))
	if mode == "" {
		return
	}
	switch mode {
	case "pass":
		outputPath := os.Getenv(localAIRealHelperOutputEnv)
		if err := writeLocalAIControlledWAV(outputPath); err != nil {
			os.Exit(21)
		}
		_, _ = os.Stdout.Write([]byte(`{"outputs":[{"name":"audio","modality":"AUDIO","mediaType":"audio/wav","content":"controlled"}]}`))
		os.Exit(0)
	case "malformed":
		_, _ = os.Stdout.Write([]byte(`{"outputs":[`))
		os.Exit(0)
	case "secret":
		_, _ = os.Stdout.Write([]byte(`HF_TOKEN=controlled-secret`))
		os.Exit(0)
	case "ledger-reserve":
		writeLocalAIReservationHelperResult(t)
		os.Exit(0)
	default:
		os.Exit(22)
	}
}

func mustLocalAIRealRunner(t testing.TB) localAIRealRunner {
	t.Helper()
	runner, err := newLocalAIRealRunner()
	if err != nil {
		t.Fatalf("new localai runner: %v", err)
	}
	return runner
}

func newLocalAIRealRunner() (localAIRealRunner, error) {
	locks, err := locking.New(locking.LocalFileSystem{})
	if err != nil {
		return localAIRealRunner{}, err
	}
	return localAIRealRunner{
		executor:    localAIProcessExecutor{},
		locks:       locks,
		writeReport: writeLocalAIRealReportAtomic,
	}, nil
}

func (runner localAIRealRunner) Run(ctx context.Context, request localAIRealRunRequest) (localAIRealReport, error) {
	roots := localAIRealRootsFor(request.Root)
	report := newLocalAIRealReport(request)
	if err := validateLocalAIRequest(request); err != nil {
		setLocalAIFailure(&report, "harness", "runner admission", "valid isolated request", "invalid request")
		return runner.finish(request, report)
	}
	if err := prepareLocalAIRoots(roots); err != nil {
		setLocalAIFailure(&report, "environment", "isolated roots", "all owned roots are creatable", "root preparation failed")
		return runner.finish(request, report)
	}
	reservation, _, err := reserveLocalAIBudget(ctx, runner.locks, request.LedgerPath, request.RunID, request.Selector, request.ReservationKind, request.ReservationAmount, request.Limits)
	updateLocalAILedgerIdentity(&report, request.LedgerPath)
	if err != nil {
		setLocalAIBudgetFailure(&report, err)
		return runner.finish(request, report)
	}

	observation := runner.execute(ctx, request, roots)
	if failure := localAICommandFailure(observation); failure != nil {
		setLocalAIFailure(&report, failure.Owner, failure.Assertion, failure.Expected, failure.Observed)
		return runner.finish(request, report)
	}
	output, artifact, failure := observeLocalAITTS(request, roots, observation)
	if failure != nil {
		setLocalAIFailure(&report, failure.Owner, failure.Assertion, failure.Expected, failure.Observed)
		return runner.finish(request, report)
	}
	if err := commitLocalAIBudget(ctx, runner.locks, request.LedgerPath, request.RunID, reservation.ID); err != nil {
		setLocalAIFailure(&report, "harness", "budget reservation commit", "reserved allowance is committed", "commit failed")
		return runner.finish(request, report)
	}
	journey := &report.Journeys[0]
	report.Status = "PASS"
	journey.Status = "PASS"
	journey.Artifacts = []localAIRealArtifact{artifact}
	journey.Semantic = output
	journey.Release = localAIRealRelease{ProcessTreeClosed: true}
	return runner.finish(request, report)
}

func (runner localAIRealRunner) execute(ctx context.Context, request localAIRealRunRequest, roots localAIRealRoots) localAICommandObservation {
	if runner.executor == nil {
		return localAICommandObservation{}
	}
	timeout := request.Timeout
	if timeout <= 0 {
		timeout = localAIRealCommandTimeout
	}
	commandContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := request.Command
	command.Arguments = localAIExpandArguments(command.Arguments, roots)
	command.Environment = localAIProcessEnvironment(roots, command.Environment)
	return runner.executor.Execute(commandContext, command, roots)
}

func (runner localAIRealRunner) finish(request localAIRealRunRequest, report localAIRealReport) (localAIRealReport, error) {
	if err := validateLocalAIRealReport(report); err != nil {
		return report, err
	}
	if runner.writeReport == nil {
		return report, errors.New("localai evidence writer is not configured")
	}
	if err := runner.writeReport(request.ReportPath, report); err != nil {
		return report, err
	}
	return report, nil
}

func newLocalAIRealReport(request localAIRealRunRequest) localAIRealReport {
	return localAIRealReport{
		Schema:       localAIRealEvidenceSchema,
		RunID:        request.RunID,
		Status:       "INCONCLUSIVE",
		Platform:     runtime.GOOS,
		Architecture: runtime.GOARCH,
		Build:        request.Build,
		BudgetLedger: localAIRealLedgerIdentity{PathIdentity: filepath.Base(request.LedgerPath)},
		Redacted:     true,
		Journeys: []localAIRealJourney{{
			Selector:            request.Selector,
			Status:              "INCONCLUSIVE",
			CacheIdentitySHA256: request.CacheIdentitySHA256,
			Offline:             request.Offline,
			Unproven:            append([]string(nil), request.Unproven...),
		}},
	}
}

func setLocalAIFailure(report *localAIRealReport, owner, assertion, expected, observed string) {
	journey := &report.Journeys[0]
	report.Status = "FAIL"
	journey.Status = "FAIL"
	journey.Semantic.Passed = false
	journey.Release = localAIRealRelease{}
	journey.Failure = &localAIRealFailure{
		Owner:     owner,
		Assertion: assertion,
		Expected:  expected,
		Observed:  boundedLocalAIValue(observed),
	}
}

func setLocalAIBudgetFailure(report *localAIRealReport, err error) {
	var budgetErr *localAIBudgetError
	if errors.As(err, &budgetErr) {
		switch budgetErr.Code {
		case "budget_exhausted":
			setLocalAIFailure(report, "harness", "durable budget admission", "consumed allowance remains below limit", fmt.Sprintf("consumed=%d limit=%d", budgetErr.Consumed, budgetErr.Limit))
		case "ledger_corrupt":
			setLocalAIFailure(report, "harness", "durable ledger decoding", "strict localai budget schema", "corrupt canonical ledger")
		case "run_id_mismatch":
			setLocalAIFailure(report, "harness", "durable ledger identity", "one run identity", "ledger run identity mismatch")
		case "limits_mismatch":
			setLocalAIFailure(report, "harness", "durable ledger limits", "stable declared allowance", "ledger limits mismatch")
		default:
			setLocalAIFailure(report, "harness", "durable ledger admission", "locked atomic reservation", "ledger transaction failed")
		}
		return
	}
	setLocalAIFailure(report, "harness", "durable ledger admission", "locked atomic reservation", "ledger transaction failed")
}

type localAIObservationFailure = localAIRealFailure

func localAICommandFailure(observation localAICommandObservation) *localAIObservationFailure {
	if observation.StdoutTruncated || observation.StderrTruncated {
		return &localAIObservationFailure{Owner: "harness", Assertion: "bounded command streams", Expected: "streams fit the redaction bound", Observed: "stream limit exceeded"}
	}
	if violation := localAIStreamViolation(observation.Stdout, observation.Stderr); violation != "" {
		return &localAIObservationFailure{Owner: "product", Assertion: "redacted command streams", Expected: "no secret, prompt, address, or raw media", Observed: violation}
	}
	if !observation.Started {
		return &localAIObservationFailure{Owner: "environment", Assertion: "selected command start", Expected: "controlled command starts", Observed: "process did not start"}
	}
	if observation.TimedOut {
		return &localAIObservationFailure{Owner: "environment", Assertion: "selected command timeout", Expected: "command exits within declared timeout", Observed: "command timed out"}
	}
	if !observation.ProcessExited {
		return &localAIObservationFailure{Owner: "harness", Assertion: "process lifecycle", Expected: "process exits", Observed: "process did not exit"}
	}
	if !observation.ProcessTreeAttached || !observation.ProcessTreeClosed {
		return &localAIObservationFailure{Owner: "harness", Assertion: "process-tree cleanup", Expected: "owned process tree is attached and closed", Observed: "cleanup was not proven"}
	}
	if observation.ExitCode != 0 {
		return &localAIObservationFailure{Owner: "product", Assertion: "selected command exit status", Expected: "exitCode=0", Observed: fmt.Sprintf("exitCode=%d", observation.ExitCode)}
	}
	return nil
}

func observeLocalAITTS(request localAIRealRunRequest, roots localAIRealRoots, observation localAICommandObservation) (localAIRealSemantic, localAIRealArtifact, *localAIObservationFailure) {
	response, err := decodeLocalAIInvocationResponse(observation.Stdout)
	if err != nil {
		return localAIRealSemantic{}, localAIRealArtifact{}, &localAIObservationFailure{Owner: "product", Assertion: "strict TTS response JSON", Expected: "one bounded audio output", Observed: "malformed response"}
	}
	if len(response.Outputs) != 1 || response.Outputs[0].Name != "audio" || response.Outputs[0].Modality != "AUDIO" {
		return localAIRealSemantic{}, localAIRealArtifact{}, &localAIObservationFailure{Owner: "product", Assertion: "TTS output slot", Expected: "one AUDIO output named audio", Observed: "output shape mismatch"}
	}
	output := response.Outputs[0]
	mediaType := strings.ToLower(strings.TrimSpace(output.MediaType))
	if mediaType != "audio/wav" && mediaType != "audio/wave" {
		return localAIRealSemantic{}, localAIRealArtifact{}, &localAIObservationFailure{Owner: "product", Assertion: "TTS output media type", Expected: "audio/wav", Observed: "unsupported media type"}
	}
	if strings.TrimSpace(output.Content) == "" {
		return localAIRealSemantic{}, localAIRealArtifact{}, &localAIObservationFailure{Owner: "product", Assertion: "TTS output materialization", Expected: "non-empty content marker", Observed: "empty content"}
	}
	audio, err := os.ReadFile(filepath.Join(roots.Output, request.Command.OutputName))
	if err != nil {
		return localAIRealSemantic{}, localAIRealArtifact{}, &localAIObservationFailure{Owner: "product", Assertion: "TTS output artifact", Expected: "readable WAV artifact", Observed: "audio artifact missing"}
	}
	metadata, ok := localAIWAVMetadata(audio)
	if !ok {
		return localAIRealSemantic{}, localAIRealArtifact{}, &localAIObservationFailure{Owner: "product", Assertion: "TTS output decode", Expected: "bounded PCM WAV", Observed: "WAV invariant mismatch"}
	}
	artifact := localAIRealArtifact{Kind: "output-audio", Path: filepath.Base(request.Command.OutputName), MediaType: mediaType, Bytes: int64(len(audio)), SHA256: sha256Hex(audio)}
	semantic := localAIRealSemantic{
		Assertion: "decodable PCM WAV output",
		Expected:  "audio/wav with PCM16 frames",
		Observed:  fmt.Sprintf("bytes=%d;sampleRateHz=%d;channels=%d;bits=%d;durationMillis=%d", len(audio), metadata.sampleRate, metadata.channels, metadata.bits, metadata.durationMillis),
		Passed:    true,
	}
	return semantic, artifact, nil
}

type localAIInvocationResponse struct {
	Outputs []localAIInvocationOutput `json:"outputs"`
	Failure any                       `json:"failure"`
}

type localAIInvocationOutput struct {
	Name      string `json:"name"`
	Modality  string `json:"modality"`
	MediaType string `json:"mediaType"`
	Content   string `json:"content"`
}

func decodeLocalAIInvocationResponse(body []byte) (localAIInvocationResponse, error) {
	if len(body) == 0 || len(body) > localAIRealMaxStreamBytes {
		return localAIInvocationResponse{}, errors.New("response outside bound")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var response localAIInvocationResponse
	if err := decoder.Decode(&response); err != nil {
		return localAIInvocationResponse{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return localAIInvocationResponse{}, errors.New("response contained trailing JSON")
	}
	return response, nil
}

func (executor localAIProcessExecutor) Execute(ctx context.Context, spec localAICommandSpec, roots localAIRealRoots) localAICommandObservation {
	result := localAICommandObservation{ExitCode: -1}
	if strings.TrimSpace(spec.BinaryPath) == "" || !filepath.IsAbs(spec.BinaryPath) {
		return result
	}
	command := exec.Command(spec.BinaryPath, spec.Arguments...)
	command.Dir = roots.Work
	command.Env = spec.Environment
	var stdout, stderr localAIBoundedBuffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	platformprocess.ConfigureSubprocessTree(command)
	if err := command.Start(); err != nil {
		return result
	}
	result.Started = true
	tree, attachErr := platformprocess.AttachSubprocessTree(command)
	if attachErr != nil {
		_ = platformprocess.TerminateSubprocessTree(command, tree)
	} else {
		result.ProcessTreeAttached = true
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- command.Wait() }()
	select {
	case waitErr := <-waitCh:
		result = localAICommandResultFromWait(result, command, waitErr)
	case <-ctx.Done():
		result.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		_ = platformprocess.TerminateSubprocessTree(command, tree)
		result = localAICommandResultFromWait(result, command, <-waitCh)
	}
	platformprocess.CloseSubprocessTree(command, tree)
	result.ProcessTreeClosed = result.ProcessTreeAttached
	result.Stdout = append([]byte(nil), stdout.Bytes()...)
	result.Stderr = append([]byte(nil), stderr.Bytes()...)
	result.StdoutTruncated = stdout.Truncated()
	result.StderrTruncated = stderr.Truncated()
	return result
}

func localAICommandResultFromWait(result localAICommandObservation, command *exec.Cmd, waitErr error) localAICommandObservation {
	result.ProcessExited = command.ProcessState != nil
	if waitErr == nil {
		result.ExitCode = 0
		return result
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	}
	return result
}

type localAIBoundedBuffer struct {
	bytes.Buffer
	truncated bool
}

func (buffer *localAIBoundedBuffer) Write(value []byte) (int, error) {
	remaining := localAIRealMaxStreamBytes - buffer.Len()
	if remaining <= 0 {
		buffer.truncated = true
		return len(value), nil
	}
	if len(value) > remaining {
		_, _ = buffer.Buffer.Write(value[:remaining])
		buffer.truncated = true
		return len(value), nil
	}
	return buffer.Buffer.Write(value)
}

func (buffer *localAIBoundedBuffer) Truncated() bool { return buffer.truncated }

func localAIStreamViolation(stdout, stderr []byte) string {
	value := strings.ToLower(string(append(append([]byte(nil), stdout...), stderr...)))
	for _, marker := range []string{
		"hf_token=", "authorization:", "bearer ", "password=", "api_key=", "access_token=",
		"x-amz-signature=", "signed_url=", "127.0.0.1", "localhost:", "grpc://", "tcp://",
		"local ai works on this machine", "raw audio", "riff",
	} {
		if strings.Contains(value, marker) {
			return "forbidden stream marker"
		}
	}
	return ""
}

func prepareLocalAIRoots(roots localAIRealRoots) error {
	for _, path := range []string{roots.Root, roots.Work, roots.Profile, roots.Cache, roots.Temp, roots.Output, roots.Streams} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func localAIRealRootsFor(root string) localAIRealRoots {
	return localAIRealRoots{
		Root: root, Work: filepath.Join(root, "work"), Profile: filepath.Join(root, "profile"),
		Cache: filepath.Join(root, "cache"), Temp: filepath.Join(root, "temp"),
		Output: filepath.Join(root, "output"), Streams: filepath.Join(root, "streams"),
	}
}

func localAIExpandArguments(arguments []string, roots localAIRealRoots) []string {
	values := map[string]string{
		localAIRealOutputToken: filepath.Join(roots.Output, "tts.wav"),
		localAIRealRootToken:   roots.Root,
		localAIRealWorkToken:   roots.Work,
	}
	expanded := make([]string, len(arguments))
	for index, argument := range arguments {
		expanded[index] = argument
		for token, value := range values {
			expanded[index] = strings.ReplaceAll(expanded[index], token, value)
		}
	}
	return expanded
}

func localAIProcessEnvironment(roots localAIRealRoots, overrides []string) []string {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok && !localAIForbiddenEnvironmentKey(key) {
			values[key] = value
		}
	}
	values["TEMP"] = roots.Temp
	values["TMP"] = roots.Temp
	values["USERPROFILE"] = roots.Profile
	values["HOME"] = roots.Profile
	values["LOCALAPPDATA"] = roots.Profile
	values["APPDATA"] = roots.Profile
	values["XDG_CACHE_HOME"] = filepath.Join(roots.Profile, "cache")
	values["XDG_CONFIG_HOME"] = filepath.Join(roots.Profile, "config")
	values["LOCALAI_REAL_OUTPUT_ROOT"] = roots.Output
	for _, entry := range overrides {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || localAIForbiddenEnvironmentOverrideKey(key) {
			continue
		}
		value = strings.ReplaceAll(value, localAIRealOutputToken, filepath.Join(roots.Output, "tts.wav"))
		value = strings.ReplaceAll(value, localAIRealRootToken, roots.Root)
		value = strings.ReplaceAll(value, localAIRealWorkToken, roots.Work)
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	for index := range keys {
		for other := index + 1; other < len(keys); other++ {
			if strings.ToUpper(keys[other]) < strings.ToUpper(keys[index]) {
				keys[index], keys[other] = keys[other], keys[index]
			}
		}
	}
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		environment = append(environment, key+"="+values[key])
	}
	return environment
}

func localAIForbiddenEnvironmentKey(key string) bool {
	upper := strings.ToUpper(strings.TrimSpace(key))
	if strings.HasPrefix(upper, "INFINITE_YOU_") || strings.HasPrefix(upper, "LOCALAI_REAL_") || strings.Contains(upper, "TOKEN") || strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "SECRET") {
		return true
	}
	switch upper {
	case "HF_ENDPOINT", "HUGGINGFACE_HUB_CACHE", "HF_HOME", "HOME", "USERPROFILE", "LOCALAPPDATA", "APPDATA", "TEMP", "TMP", "XDG_CACHE_HOME", "XDG_CONFIG_HOME":
		return true
	default:
		return false
	}
}

func localAIForbiddenEnvironmentOverrideKey(key string) bool {
	upper := strings.ToUpper(strings.TrimSpace(key))
	if strings.Contains(upper, "TOKEN") || strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "SECRET") {
		return true
	}
	switch upper {
	case "HF_ENDPOINT", "HUGGINGFACE_HUB_CACHE", "HF_HOME", "HOME", "USERPROFILE", "LOCALAPPDATA", "APPDATA", "TEMP", "TMP", "XDG_CACHE_HOME", "XDG_CONFIG_HOME":
		return true
	default:
		return false
	}
}

func validateLocalAIRequest(request localAIRealRunRequest) error {
	if strings.TrimSpace(request.Selector) == "" || strings.ContainsAny(request.Selector, "\\/\r\n") {
		return errors.New("selector is not bounded")
	}
	if strings.TrimSpace(request.RunID) == "" || len(request.RunID) > 128 {
		return errors.New("run identity is not bounded")
	}
	for _, path := range []string{request.Root, request.ReportPath, request.LedgerPath, request.Command.BinaryPath} {
		if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) {
			return errors.New("isolated path is not absolute")
		}
	}
	if request.ReservationKind != "modelCall" && request.ReservationKind != "downloadBytes" {
		return errors.New("reservation kind is not bounded")
	}
	if request.ReservationAmount <= 0 || request.Limits.ModelCalls < 0 || request.Limits.DownloadBytes < 0 {
		return errors.New("reservation or limits are invalid")
	}
	if request.Command.OutputName == "" || filepath.Base(request.Command.OutputName) != request.Command.OutputName {
		return errors.New("output name is not a file identity")
	}
	if request.CacheIdentitySHA256 != "" && !isLocalAISHA256(request.CacheIdentitySHA256) {
		return errors.New("cache identity is not a SHA-256")
	}
	return nil
}

func validateLocalAIRealReport(report localAIRealReport) error {
	if report.Schema != localAIRealEvidenceSchema || !localAIStatus(report.Status) || report.Platform == "" || report.Architecture == "" || !report.Redacted {
		return errors.New("report identity or status is invalid")
	}
	if report.RunID == "" || len(report.RunID) > 128 || len(report.Journeys) != 1 {
		return errors.New("report cardinality is invalid")
	}
	if err := validateLocalAIBuild(report.Build); err != nil {
		return err
	}
	journey := report.Journeys[0]
	if journey.Selector == "" || journey.Status != report.Status || !localAIStatus(journey.Status) {
		return errors.New("journey identity or status is invalid")
	}
	if journey.CacheIdentitySHA256 != "" && !isLocalAISHA256(journey.CacheIdentitySHA256) {
		return errors.New("journey cache identity is invalid")
	}
	if report.Status == "PASS" {
		if len(journey.Artifacts) != 1 || journey.Failure != nil || !journey.Semantic.Passed || !journey.Release.ProcessTreeClosed {
			return errors.New("pass report omitted semantic or release proof")
		}
		if err := validateLocalAIArtifact(journey.Artifacts[0]); err != nil {
			return err
		}
	} else if journey.Failure == nil || !localAIFailureOwner(journey.Failure.Owner) {
		return errors.New("failure report omitted bounded ownership")
	} else if err := validateLocalAIFailure(*journey.Failure); err != nil {
		return err
	}
	if report.BudgetLedger.PathIdentity == "" {
		return errors.New("budget ledger identity is missing")
	}
	if report.BudgetLedger.SHA256 != "" && !isLocalAISHA256(report.BudgetLedger.SHA256) {
		return errors.New("budget ledger identity is invalid")
	}
	body, err := json.Marshal(report)
	if err != nil {
		return err
	}
	if localAIStreamViolation(body, nil) != "" {
		return errors.New("report contained unredacted evidence")
	}
	return nil
}

func validateLocalAIBuild(build localAIRealBuildIdentity) error {
	if !localAIPathIdentity(build.PathIdentity) || build.Bytes <= 0 || !isLocalAISHA256(build.SHA256) || build.Commit == "" || build.Tree == "" {
		return errors.New("build identity is invalid")
	}
	return nil
}

func validateLocalAIArtifact(artifact localAIRealArtifact) error {
	if artifact.Kind == "" || !localAIPathIdentity(artifact.Path) || artifact.MediaType == "" || artifact.Bytes <= 0 || !isLocalAISHA256(artifact.SHA256) {
		return errors.New("artifact identity is invalid")
	}
	return nil
}

func validateLocalAIFailure(failure localAIRealFailure) error {
	if failure.Assertion == "" || failure.Expected == "" || failure.Observed == "" || len(failure.Observed) > localAIRealMaxFailureBytes {
		return errors.New("failure observation is not bounded")
	}
	if localAIStreamViolation([]byte(failure.Observed), nil) != "" {
		return errors.New("failure observation was not redacted")
	}
	return nil
}

func localAIStatus(value string) bool {
	return value == "PASS" || value == "FAIL" || value == "INCONCLUSIVE"
}

func localAIFailureOwner(value string) bool {
	switch value {
	case "product", "harness", "environment", "unresolved":
		return true
	default:
		return false
	}
}

func localAIPathIdentity(value string) bool {
	return value != "" && filepath.Base(value) == value && !strings.ContainsAny(value, ":\r\n")
}

func boundedLocalAIValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= localAIRealMaxFailureBytes {
		return value
	}
	return "sha256=" + sha256Hex([]byte(value))
}

func isLocalAISHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func writeLocalAIRealReportAtomic(path string, report localAIRealReport) error {
	return writeLocalAIRealReportAtomicWithHook(path, report, nil)
}

func writeLocalAIRealReportAtomicWithHook(path string, report localAIRealReport, beforeReplace func() error) error {
	if err := validateLocalAIRealReport(report); err != nil {
		return err
	}
	return writeLocalAIJSONAtomic(path, report, beforeReplace)
}

func writeLocalAIBudgetAtomic(path string, ledger localAIBudgetLedger) error {
	return writeLocalAIBudgetAtomicWithHook(path, ledger, nil)
}

func writeLocalAIBudgetAtomicWithHook(path string, ledger localAIBudgetLedger, beforeReplace func() error) error {
	if err := validateLocalAIBudgetLedger(ledger); err != nil {
		return err
	}
	return writeLocalAIJSONAtomic(path, ledger, beforeReplace)
}

func writeLocalAIJSONAtomic(path string, value any, beforeReplace func() error) (err error) {
	if strings.TrimSpace(path) == "" {
		return errors.New("atomic path is empty")
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+".*.partial")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err = temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if written, writeErr := temporary.Write(body); writeErr != nil {
		_ = temporary.Close()
		return writeErr
	} else if written != len(body) {
		_ = temporary.Close()
		return io.ErrShortWrite
	}
	if err = temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	if beforeReplace != nil {
		if err = beforeReplace(); err != nil {
			return err
		}
	}
	from, err := windows.UTF16PtrFromString(temporaryPath)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

func reserveLocalAIBudget(ctx context.Context, locks locking.Service, path, runID, journey, kind string, amount int64, limits localAIBudgetLimits) (localAIBudgetReservation, localAIBudgetLedger, error) {
	if locks == nil {
		return localAIBudgetReservation{}, localAIBudgetLedger{}, errors.New("budget lock service is required")
	}
	lock, err := locks.Lock(ctx, path+".lock")
	if err != nil {
		return localAIBudgetReservation{}, localAIBudgetLedger{}, err
	}
	defer lock.Close()
	ledger, err := readOrCreateLocalAIBudget(path, runID, limits)
	if err != nil {
		return localAIBudgetReservation{}, ledger, err
	}
	if ledger.RunID != runID {
		return localAIBudgetReservation{}, ledger, &localAIBudgetError{Code: "run_id_mismatch"}
	}
	if ledger.Limits != limits {
		return localAIBudgetReservation{}, ledger, &localAIBudgetError{Code: "limits_mismatch"}
	}
	limit, consumed := localAIBudgetValues(ledger, kind)
	if amount <= 0 || limit < 0 || consumed < 0 || consumed+amount > limit {
		return localAIBudgetReservation{}, ledger, &localAIBudgetError{Code: "budget_exhausted", Consumed: consumed, Limit: limit}
	}
	reservationID, err := newLocalAIReservationID()
	if err != nil {
		return localAIBudgetReservation{}, ledger, err
	}
	reservation := localAIBudgetReservation{ID: reservationID, Journey: journey, Kind: kind, Amount: amount, State: "RESERVED"}
	localAIBudgetAddConsumed(&ledger, kind, amount)
	ledger.Reservations = append(ledger.Reservations, reservation)
	if err := writeLocalAIBudgetAtomic(path, ledger); err != nil {
		return localAIBudgetReservation{}, ledger, err
	}
	return reservation, ledger, nil
}

func commitLocalAIBudget(ctx context.Context, locks locking.Service, path, runID, reservationID string) error {
	lock, err := locks.Lock(ctx, path+".lock")
	if err != nil {
		return err
	}
	defer lock.Close()
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	ledger, err := decodeLocalAIBudget(body)
	if err != nil {
		return err
	}
	if ledger.RunID != runID {
		return &localAIBudgetError{Code: "run_id_mismatch"}
	}
	for index := range ledger.Reservations {
		if ledger.Reservations[index].ID != reservationID {
			continue
		}
		if ledger.Reservations[index].State == "COMMITTED" {
			return nil
		}
		if ledger.Reservations[index].State != "RESERVED" {
			return &localAIBudgetError{Code: "ledger_corrupt"}
		}
		ledger.Reservations[index].State = "COMMITTED"
		return writeLocalAIBudgetAtomic(path, ledger)
	}
	return &localAIBudgetError{Code: "ledger_corrupt"}
}

func readOrCreateLocalAIBudget(path, runID string, limits localAIBudgetLimits) (localAIBudgetLedger, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		ledger := localAIBudgetLedger{Schema: localAIBudgetSchema, RunID: runID, Limits: limits, Reservations: []localAIBudgetReservation{}}
		return ledger, writeLocalAIBudgetAtomic(path, ledger)
	}
	if err != nil {
		return localAIBudgetLedger{}, err
	}
	ledger, err := decodeLocalAIBudget(body)
	if err != nil {
		return localAIBudgetLedger{}, &localAIBudgetError{Code: "ledger_corrupt"}
	}
	return ledger, nil
}

func decodeLocalAIBudget(body []byte) (localAIBudgetLedger, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var ledger localAIBudgetLedger
	if err := decoder.Decode(&ledger); err != nil {
		return localAIBudgetLedger{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return localAIBudgetLedger{}, errors.New("ledger contained trailing JSON")
	}
	if err := validateLocalAIBudgetLedger(ledger); err != nil {
		return localAIBudgetLedger{}, err
	}
	return ledger, nil
}

func validateLocalAIBudgetLedger(ledger localAIBudgetLedger) error {
	if ledger.Schema != localAIBudgetSchema || ledger.RunID == "" || ledger.Limits.ModelCalls < 0 || ledger.Limits.DownloadBytes < 0 || ledger.Consumed.ModelCalls < 0 || ledger.Consumed.DownloadBytes < 0 {
		return errors.New("ledger identity or counters are invalid")
	}
	seen := make(map[string]struct{}, len(ledger.Reservations))
	var modelCalls, downloadBytes int64
	for _, reservation := range ledger.Reservations {
		if reservation.ID == "" || reservation.Journey == "" || reservation.Amount <= 0 || reservation.State != "RESERVED" && reservation.State != "COMMITTED" {
			return errors.New("ledger reservation is invalid")
		}
		if _, exists := seen[reservation.ID]; exists {
			return errors.New("ledger reservation is duplicated")
		}
		seen[reservation.ID] = struct{}{}
		switch reservation.Kind {
		case "modelCall":
			modelCalls += reservation.Amount
		case "downloadBytes":
			downloadBytes += reservation.Amount
		default:
			return errors.New("ledger reservation kind is invalid")
		}
	}
	if ledger.Consumed.ModelCalls != modelCalls || ledger.Consumed.DownloadBytes != downloadBytes || modelCalls > ledger.Limits.ModelCalls || downloadBytes > ledger.Limits.DownloadBytes {
		return errors.New("ledger counters do not match reservations")
	}
	return nil
}

func localAIBudgetValues(ledger localAIBudgetLedger, kind string) (int64, int64) {
	if kind == "downloadBytes" {
		return ledger.Limits.DownloadBytes, ledger.Consumed.DownloadBytes
	}
	return ledger.Limits.ModelCalls, ledger.Consumed.ModelCalls
}

func localAIBudgetAddConsumed(ledger *localAIBudgetLedger, kind string, amount int64) {
	if kind == "downloadBytes" {
		ledger.Consumed.DownloadBytes += amount
		return
	}
	ledger.Consumed.ModelCalls += amount
}

func newLocalAIReservationID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func updateLocalAILedgerIdentity(report *localAIRealReport, path string) {
	report.BudgetLedger.PathIdentity = filepath.Base(path)
	identity, ok := localAIReadFileIdentity(path)
	if ok {
		report.BudgetLedger.SHA256 = identity.SHA256
	}
}

type localAIFileIdentity struct {
	Bytes  int64
	SHA256 string
}

func localAIReadFileIdentity(path string) (localAIFileIdentity, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return localAIFileIdentity{}, false
	}
	file, err := os.Open(path)
	if err != nil {
		return localAIFileIdentity{}, false
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return localAIFileIdentity{}, false
	}
	return localAIFileIdentity{Bytes: info.Size(), SHA256: hex.EncodeToString(hasher.Sum(nil))}, true
}

type localAIWAVDetails struct {
	durationMillis int64
	channels       uint16
	sampleRate     uint32
	bits           uint16
}

func localAIWAVMetadata(audio []byte) (localAIWAVDetails, bool) {
	if len(audio) < 44 || string(audio[:4]) != "RIFF" || string(audio[8:12]) != "WAVE" || string(audio[12:16]) != "fmt " || string(audio[36:40]) != "data" {
		return localAIWAVDetails{}, false
	}
	dataSize := binary.LittleEndian.Uint32(audio[40:44])
	if uint64(dataSize)+44 != uint64(len(audio)) || binary.LittleEndian.Uint32(audio[16:20]) != 16 || binary.LittleEndian.Uint16(audio[20:22]) != 1 {
		return localAIWAVDetails{}, false
	}
	channels := binary.LittleEndian.Uint16(audio[22:24])
	sampleRate := binary.LittleEndian.Uint32(audio[24:28])
	blockAlign := binary.LittleEndian.Uint16(audio[32:34])
	bits := binary.LittleEndian.Uint16(audio[34:36])
	if channels == 0 || sampleRate == 0 || blockAlign == 0 || bits != 16 || blockAlign != channels*bits/8 || binary.LittleEndian.Uint32(audio[28:32]) != sampleRate*uint32(blockAlign) || dataSize == 0 || dataSize%uint32(blockAlign) != 0 {
		return localAIWAVDetails{}, false
	}
	durationMillis := int64(dataSize/uint32(blockAlign)) * 1000 / int64(sampleRate)
	return localAIWAVDetails{durationMillis: durationMillis, channels: channels, sampleRate: sampleRate, bits: bits}, durationMillis > 0
}

func writeLocalAIControlledWAV(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("controlled output path is empty")
	}
	const (
		sampleRate = 8000
		channels   = 1
		bits       = 16
		frames     = 80
	)
	dataBytes := frames * channels * bits / 8
	audio := make([]byte, 44+dataBytes)
	copy(audio[:4], "RIFF")
	binary.LittleEndian.PutUint32(audio[4:8], uint32(len(audio)-8))
	copy(audio[8:12], "WAVE")
	copy(audio[12:16], "fmt ")
	binary.LittleEndian.PutUint32(audio[16:20], 16)
	binary.LittleEndian.PutUint16(audio[20:22], 1)
	binary.LittleEndian.PutUint16(audio[22:24], channels)
	binary.LittleEndian.PutUint32(audio[24:28], sampleRate)
	binary.LittleEndian.PutUint32(audio[28:32], sampleRate*channels*bits/8)
	binary.LittleEndian.PutUint16(audio[32:34], channels*bits/8)
	binary.LittleEndian.PutUint16(audio[34:36], bits)
	copy(audio[36:40], "data")
	binary.LittleEndian.PutUint32(audio[40:44], uint32(dataBytes))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, audio, 0o600)
}

func localAIControlledRequest(t testing.TB, root, mode string) localAIRealRunRequest {
	t.Helper()
	binaryPath, err := os.Executable()
	if err != nil {
		t.Fatalf("test executable: %v", err)
	}
	identity, ok := localAIReadFileIdentity(binaryPath)
	if !ok {
		t.Fatalf("read test executable identity")
	}
	return localAIRealRunRequest{
		Selector:          "controlled-tts",
		RunID:             "controlled-run",
		Root:              root,
		ReportPath:        filepath.Join(root, "evidence.json"),
		LedgerPath:        filepath.Join(root, "localai-budget.json"),
		Limits:            localAIBudgetLimits{ModelCalls: 1},
		ReservationKind:   "modelCall",
		ReservationAmount: 1,
		Command: localAICommandSpec{
			BinaryPath: binaryPath,
			Arguments:  []string{"-test.run=TestLocalAIRealHarnessControlledHelper", "--", mode},
			Environment: []string{
				localAIRealHelperModeEnv + "=" + mode,
				localAIRealHelperOutputEnv + "=" + localAIRealOutputToken,
			},
			OutputName: "tts.wav",
		},
		Build:               localAIRealBuildIdentity{PathIdentity: "controlled-test-binary", Bytes: identity.Bytes, SHA256: identity.SHA256, Commit: "controlled", Tree: "controlled"},
		CacheIdentitySHA256: sha256Hex([]byte("controlled-cache")),
		Unproven:            []string{"real LocalAI model/backend", "public installation and discovery"},
	}
}

func assertLocalAIJourneyResult(t testing.TB, request localAIRealRunRequest, report localAIRealReport, wantStatus, wantOwner string) {
	t.Helper()
	if report.Status != wantStatus || len(report.Journeys) != 1 || report.Journeys[0].Selector != request.Selector {
		t.Fatalf("report = %#v, want one %s %q journey", report, wantStatus, request.Selector)
	}
	if wantOwner != "" {
		if report.Journeys[0].Failure == nil || report.Journeys[0].Failure.Owner != wantOwner {
			t.Fatalf("failure = %#v, want owner %q", report.Journeys[0].Failure, wantOwner)
		}
		if err := validateLocalAIFailure(*report.Journeys[0].Failure); err != nil {
			t.Fatalf("failure validation: %v", err)
		}
	}
	body, err := os.ReadFile(request.ReportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if err := validateLocalAIRealReport(report); err != nil {
		t.Fatalf("report validation: %v", err)
	}
	if !bytes.Contains(body, []byte(`"redacted": true`)) {
		t.Fatalf("report did not record redaction: %s", body)
	}
	if bytes.Contains(body, []byte("controlled-secret")) || bytes.Contains(body, []byte("HF_TOKEN=")) {
		t.Fatalf("report leaked forbidden data: %s", body)
	}
}

func setupLocalAIReportInterruption(t testing.TB, _ string, request *localAIRealRunRequest) {
	t.Helper()
	old := newLocalAIRealReport(*request)
	old.Status = "PASS"
	old.Journeys[0].Status = "PASS"
	old.Journeys[0].Artifacts = []localAIRealArtifact{{Kind: "output-audio", Path: "tts.wav", MediaType: "audio/wav", Bytes: 44, SHA256: sha256Hex(bytes.Repeat([]byte{'a'}, 44))}}
	old.Journeys[0].Semantic = localAIRealSemantic{Assertion: "controlled", Expected: "valid", Observed: "valid", Passed: true}
	old.Journeys[0].Release = localAIRealRelease{ProcessTreeClosed: true}
	old.BudgetLedger = localAIRealLedgerIdentity{PathIdentity: filepath.Base(request.LedgerPath), SHA256: sha256Hex([]byte("old-ledger"))}
	if err := writeLocalAIRealReportAtomic(request.ReportPath, old); err != nil {
		t.Fatalf("seed canonical report: %v", err)
	}
}

func assertLocalAIReportInterruption(t testing.TB, request localAIRealRunRequest, report localAIRealReport, err error) {
	t.Helper()
	if !errors.Is(err, errLocalAIReportInterrupted) {
		t.Fatalf("runner error = %v, want report interruption", err)
	}
	if report.Status != "PASS" {
		t.Fatalf("interrupted report result = %#v, want completed in-memory PASS", report)
	}
	body, readErr := os.ReadFile(request.ReportPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Contains(body, []byte(`"status": "PASS"`)) {
		t.Fatalf("previous canonical report was not preserved: %s", body)
	}
}

var errLocalAIReportInterrupted = errors.New("controlled report interruption")

func setupLocalAIBudgetExhaustion(t testing.TB, _ string, request *localAIRealRunRequest) {
	t.Helper()
	locks := mustLocalAILockService(t)
	if _, _, err := reserveLocalAIBudget(t.Context(), locks, request.LedgerPath, request.RunID, request.Selector, request.ReservationKind, request.ReservationAmount, request.Limits); err != nil {
		t.Fatalf("seed budget reservation: %v", err)
	}
}

func setupLocalAICorruptLedger(t testing.TB, _ string, request *localAIRealRunRequest) {
	t.Helper()
	if err := os.WriteFile(request.LedgerPath, []byte(`{"schema":"wrong"}`), 0o600); err != nil {
		t.Fatalf("write corrupt ledger: %v", err)
	}
}

func mustLocalAILockService(t testing.TB) locking.Service {
	t.Helper()
	locks, err := locking.New(locking.LocalFileSystem{})
	if err != nil {
		t.Fatalf("new lock service: %v", err)
	}
	return locks
}

func readLocalAIBudgetForTest(t testing.TB, path string) localAIBudgetLedger {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read budget: %v", err)
	}
	ledger, err := decodeLocalAIBudget(body)
	if err != nil {
		t.Fatalf("decode budget: %v", err)
	}
	return ledger
}

func runLocalAIReservationHelper(t testing.TB, root, ledgerPath, resultPath string) string {
	t.Helper()
	binaryPath, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	roots := localAIRealRootsFor(root)
	if err := prepareLocalAIRoots(roots); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), binaryPath, "-test.run=TestLocalAIRealHarnessControlledHelper", "--")
	command.Dir = roots.Work
	command.Env = localAIProcessEnvironment(roots, []string{
		localAIRealHelperModeEnv + "=ledger-reserve",
		"LOCALAI_REAL_HELPER_LEDGER=" + ledgerPath,
		"LOCALAI_REAL_HELPER_RUN_ID=run-persistence",
		localAIRealHelperResultEnv + "=" + resultPath,
	})
	if err := command.Run(); err != nil {
		t.Fatalf("reservation helper: %v", err)
	}
	body, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(body))
}

func writeLocalAIReservationHelperResult(t *testing.T) {
	ledgerPath := os.Getenv("LOCALAI_REAL_HELPER_LEDGER")
	resultPath := os.Getenv(localAIRealHelperResultEnv)
	locks, err := locking.New(locking.LocalFileSystem{})
	result := "ERROR"
	if err == nil {
		_, _, reserveErr := reserveLocalAIBudget(context.Background(), locks, ledgerPath, "run-persistence", "known-tts", "modelCall", 1, localAIBudgetLimits{ModelCalls: 1})
		var budgetErr *localAIBudgetError
		if errors.As(reserveErr, &budgetErr) && budgetErr.Code == "budget_exhausted" {
			result = "BUDGET_EXHAUSTED"
		} else if reserveErr == nil {
			result = "RESERVED"
		}
	}
	_ = os.WriteFile(resultPath, []byte(result), 0o600)
}

type localAIStubExecutor struct {
	observation localAICommandObservation
	calls       atomic.Int32
}

func (stub *localAIStubExecutor) Execute(context.Context, localAICommandSpec, localAIRealRoots) localAICommandObservation {
	stub.calls.Add(1)
	return stub.observation
}

type localAIProcessExecutor struct{}
