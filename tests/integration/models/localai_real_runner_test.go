//go:build windows

package models_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/locking"
	"golang.org/x/sys/windows"
)

const (
	localAIRealEvidenceSchema            = "localai.windows-real-evidence.v1"
	localAIBudgetSchema                  = "localai.windows-real-budget.v1"
	localAIRealHelperModeEnv             = "LOCALAI_REAL_HELPER_MODE"
	localAIRealHelperOutputEnv           = "LOCALAI_REAL_HELPER_OUTPUT"
	localAIRealHelperResultEnv           = "LOCALAI_REAL_HELPER_RESULT"
	localAIRealHelperTranscriptEnv       = "LOCALAI_REAL_HELPER_TRANSCRIPT"
	localAIRealHelperSegmentsEnv         = "LOCALAI_REAL_HELPER_SEGMENTS"
	localAIRealModelCacheEnv             = "INFINITE_YOU_OMNIVOICE_CACHE_DIR"
	localAIRealOutputToken               = "{output}"
	localAIRealInputToken                = "{input}"
	localAIRealTranscriptToken           = "{transcript}"
	localAIRealSegmentsToken             = "{segments}"
	localAIRealRootToken                 = "{root}"
	localAIRealWorkToken                 = "{work}"
	localAIRealMaxStreamBytes            = 64 << 10
	localAIRealMaxFailureBytes           = 192
	localAIRealMaxAudioBytes       int64 = 512 << 20
	localAIRealMaxAudioDuration          = 5 * time.Minute
	localAIRealCommandTimeout            = 10 * time.Second
	localAIJourneyTTS                    = "tts"
	localAIJourneyASR                    = "asr"
	localAIJourneyTTSASR                 = "tts-asr"
	localAIRealTranscriptFile            = "transcript.txt"
	localAIRealSegmentsFile              = "segments.json"
)

type localAIRealReport struct {
	Schema       string                    `json:"schema"`
	RunID        string                    `json:"runId"`
	Status       string                    `json:"status"`
	Platform     string                    `json:"platform"`
	Architecture string                    `json:"architecture"`
	Build        localAIRealBuildIdentity  `json:"build"`
	BudgetLedger localAIRealLedgerIdentity `json:"budgetLedger"`
	Policy       localAIRealPolicy         `json:"policy"`
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
	Cache               localAIRealCache      `json:"cache"`
	Offline             bool                  `json:"offline"`
	Semantic            localAIRealSemantic   `json:"semantic"`
	Execution           *localAIRealExecution `json:"execution,omitempty"`
	Release             localAIRealRelease    `json:"release"`
	Failure             *localAIRealFailure   `json:"failure"`
	Unproven            []string              `json:"unproven"`
}

type localAIRealPolicy struct {
	WorkRoot          string `json:"workRoot"`
	StateRoot         string `json:"stateRoot"`
	CacheRoot         string `json:"cacheRoot"`
	TempRoot          string `json:"tempRoot"`
	OutputRoot        string `json:"outputRoot"`
	StreamsRoot       string `json:"streamsRoot"`
	PortState         string `json:"portState"`
	NetworkPolicy     string `json:"networkPolicy"`
	Timeout           string `json:"timeout"`
	ModelCallLimit    int64  `json:"modelCallLimit"`
	DownloadLimit     int64  `json:"downloadByteLimit"`
	ChildProcessLimit int    `json:"childProcessLimit"`
	SemanticRetries   int    `json:"semanticRetries"`
}

type localAIRealCache struct {
	BeforeIdentitySHA256 string `json:"beforeIdentitySha256"`
	AfterIdentitySHA256  string `json:"afterIdentitySha256"`
	BeforeEntries        int    `json:"beforeEntries"`
	AfterEntries         int    `json:"afterEntries"`
	BeforeBytes          int64  `json:"beforeBytes"`
	AfterBytes           int64  `json:"afterBytes"`
	FreshAtStart         bool   `json:"freshAtStart"`
	Reused               bool   `json:"reused"`
	PartialArtifacts     int    `json:"partialArtifacts"`
}

type localAIRealExecution struct {
	Commands []localAIRealCommandExecution `json:"commands"`
}

type localAIRealCommandExecution struct {
	Started             bool   `json:"started"`
	ProcessExited       bool   `json:"processExited"`
	ExitCode            int    `json:"exitCode"`
	TimedOut            bool   `json:"timedOut"`
	ProcessTreeAttached bool   `json:"processTreeAttached"`
	ProcessTreeClosed   bool   `json:"processTreeClosed"`
	OwnedProcesses      int    `json:"ownedProcesses"`
	OwnedListeners      int    `json:"ownedListeners"`
	OwnedLeases         int    `json:"ownedLeases"`
	StdoutBytes         int64  `json:"stdoutBytes"`
	StdoutSHA256        string `json:"stdoutSha256"`
	StderrBytes         int64  `json:"stderrBytes"`
	StderrSHA256        string `json:"stderrSha256"`
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
	Checked           bool `json:"checked"`
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
	Kind                string
	RunID               string
	Root                string
	CacheRoot           string
	ReportPath          string
	LedgerPath          string
	Limits              localAIBudgetLimits
	ReservationKind     string
	ReservationAmount   int64
	Command             localAICommandSpec
	FollowUp            *localAICommandSpec
	InputPath           string
	ExpectedInputSHA256 string
	ExpectedTranscript  string
	Build               localAIRealBuildIdentity
	Offline             bool
	RequireFreshCache   bool
	RequireCacheReuse   bool
	NetworkPolicy       string
	ChildProcessLimit   int
	SemanticRetries     int
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
	HFHome  string
	HFCache string
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
	OwnedProcesses      int
	OwnedListeners      int
	OwnedLeases         int
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
		{name: "IC-13-partial-artifact-leak", mode: "pass", wantStatus: "FAIL", wantOwner: "harness", setup: setupLocalAIPartialArtifactLeak},
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
			if testCase.name == "IC-08-cleanup-leak" && (report.Journeys[0].Release.ProcessTreeClosed || report.Journeys[0].Release.OwnedProcesses == 0) {
				t.Fatalf("cleanup leak release = %#v, want an owned process remaining", report.Journeys[0].Release)
			}
			if testCase.name == "IC-13-partial-artifact-leak" && report.Journeys[0].Release.PartialArtifacts == 0 {
				t.Fatalf("partial-artifact release = %#v, want a detected partial artifact", report.Journeys[0].Release)
			}
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
	roots := localAIRealRootsForRequest(request)
	report := newLocalAIRealReport(request)
	if err := validateLocalAIRequest(request); err != nil {
		setLocalAIFailure(&report, "harness", "runner admission", "valid isolated request", "invalid request")
		return runner.finish(request, report)
	}
	if err := prepareLocalAIRoots(roots); err != nil {
		setLocalAIFailure(&report, "environment", "isolated roots", "all owned roots are creatable", "root preparation failed")
		return runner.finish(request, report)
	}
	if failure := localAICacheAdmissionFailure(request, report.Journeys[0].Cache); failure != nil {
		setLocalAIFailure(&report, failure.Owner, failure.Assertion, failure.Expected, failure.Observed)
		return runner.finish(request, report)
	}
	reservation, _, err := reserveLocalAIBudget(ctx, runner.locks, request.LedgerPath, request.RunID, request.Selector, request.ReservationKind, request.ReservationAmount, request.Limits)
	updateLocalAILedgerIdentity(&report, request.LedgerPath)
	if err != nil {
		setLocalAIBudgetFailure(&report, err)
		return runner.finish(request, report)
	}

	observations := []localAICommandObservation{runner.execute(ctx, request, request.Command, roots)}
	recordLocalAIExecution(&report, observations[0])
	if failure := localAICommandFailureForJourney(observations[0], request.Kind == localAIJourneyASR); failure != nil {
		setLocalAIFailure(&report, failure.Owner, failure.Assertion, failure.Expected, failure.Observed)
		return runner.finish(request, report)
	}
	if request.FollowUp != nil {
		observations = append(observations, runner.execute(ctx, request, *request.FollowUp, roots))
		recordLocalAIExecution(&report, observations[1])
		if failure := localAICommandFailureForJourney(observations[1], true); failure != nil {
			setLocalAIFailure(&report, failure.Owner, failure.Assertion, failure.Expected, failure.Observed)
			return runner.finish(request, report)
		}
	}
	output, artifacts, failure := observeLocalAI(request, roots, observations)
	if failure != nil {
		report.Journeys[0].Artifacts = artifacts
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
	journey.Artifacts = artifacts
	journey.Semantic = output
	return runner.finish(request, report)
}

func (runner localAIRealRunner) execute(ctx context.Context, request localAIRealRunRequest, spec localAICommandSpec, roots localAIRealRoots) localAICommandObservation {
	if runner.executor == nil {
		return localAICommandObservation{}
	}
	timeout := request.Timeout
	if timeout <= 0 {
		timeout = localAIRealCommandTimeout
	}
	commandContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := spec
	command.Arguments = localAIExpandArguments(command.Arguments, roots)
	command.Environment = localAIProcessEnvironment(roots, command.Environment)
	return runner.executor.Execute(commandContext, command, roots)
}

func (runner localAIRealRunner) finish(request localAIRealRunRequest, report localAIRealReport) (localAIRealReport, error) {
	if err := finalizeLocalAIReport(request, &report); err != nil {
		if report.Status == "PASS" {
			setLocalAIFailure(&report, "harness", "runner release and cache inspection", "owned resources and cache state are observable", "final inspection failed")
		}
	}
	if report.Status == "PASS" && report.Journeys[0].Release.PartialArtifacts > 0 {
		setLocalAIFailure(&report, "harness", "partial artifact cleanup", "no owned partial artifacts remain", "partial artifact remains")
	}
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

func recordLocalAIExecution(report *localAIRealReport, observation localAICommandObservation) {
	if len(report.Journeys) != 1 {
		return
	}
	journey := &report.Journeys[0]
	if journey.Execution == nil {
		journey.Execution = &localAIRealExecution{}
	}
	ownedProcesses := observation.OwnedProcesses
	if observation.Started && (!observation.ProcessExited || !observation.ProcessTreeClosed) && ownedProcesses == 0 {
		ownedProcesses = 1
	}
	journey.Execution.Commands = append(journey.Execution.Commands, localAIRealCommandExecution{
		Started: observation.Started, ProcessExited: observation.ProcessExited, ExitCode: observation.ExitCode,
		TimedOut: observation.TimedOut, ProcessTreeAttached: observation.ProcessTreeAttached,
		ProcessTreeClosed: observation.ProcessTreeClosed, OwnedProcesses: ownedProcesses,
		OwnedListeners: observation.OwnedListeners, OwnedLeases: observation.OwnedLeases,
		StdoutBytes: int64(len(observation.Stdout)), StdoutSHA256: sha256Hex(observation.Stdout),
		StderrBytes: int64(len(observation.Stderr)), StderrSHA256: sha256Hex(observation.Stderr),
	})
}

func newLocalAIRealReport(request localAIRealRunRequest) localAIRealReport {
	roots := localAIRealRootsForRequest(request)
	cache, _ := localAIRealCacheSnapshotForRoots(roots)
	timeout := request.Timeout
	if timeout <= 0 {
		timeout = localAIRealCommandTimeout
	}
	networkPolicy := request.NetworkPolicy
	if networkPolicy == "" {
		networkPolicy = "controlled-no-network"
	}
	childProcessLimit := request.ChildProcessLimit
	if childProcessLimit <= 0 {
		childProcessLimit = 2
	}
	return localAIRealReport{
		Schema:       localAIRealEvidenceSchema,
		RunID:        request.RunID,
		Status:       "INCONCLUSIVE",
		Platform:     runtime.GOOS,
		Architecture: runtime.GOARCH,
		Build:        request.Build,
		BudgetLedger: localAIRealLedgerIdentity{PathIdentity: filepath.Base(request.LedgerPath)},
		Policy: localAIRealPolicy{
			WorkRoot: pathIdentityHash(roots.Work), StateRoot: pathIdentityHash(roots.Profile),
			CacheRoot: pathIdentityHash(roots.Cache), TempRoot: pathIdentityHash(roots.Temp),
			OutputRoot: pathIdentityHash(roots.Output), StreamsRoot: pathIdentityHash(roots.Streams),
			PortState: "selector-owned:" + pathIdentityHash(roots.Root),
			NetworkPolicy: networkPolicy, Timeout: timeout.String(), ModelCallLimit: request.Limits.ModelCalls,
			DownloadLimit: request.Limits.DownloadBytes, ChildProcessLimit: childProcessLimit,
			SemanticRetries: request.SemanticRetries,
		},
		Redacted: true,
		Journeys: []localAIRealJourney{{
			Selector:            request.Selector,
			Status:              "INCONCLUSIVE",
			CacheIdentitySHA256: cache.IdentitySHA256,
			Cache: localAIRealCache{
				BeforeIdentitySHA256: cache.IdentitySHA256, BeforeEntries: cache.Entries,
				BeforeBytes: cache.Bytes, FreshAtStart: cache.Entries == 0 && cache.PartialArtifacts == 0,
				PartialArtifacts: cache.PartialArtifacts,
			},
			Offline:   request.Offline,
			Execution: &localAIRealExecution{Commands: []localAIRealCommandExecution{}},
			Unproven:  append([]string(nil), request.Unproven...),
		}},
	}
}

func pathIdentityHash(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	return "sha256=" + sha256Hex([]byte(filepath.Clean(path)))
}

func setLocalAIFailure(report *localAIRealReport, owner, assertion, expected, observed string) {
	journey := &report.Journeys[0]
	report.Status = "FAIL"
	journey.Status = "FAIL"
	journey.Semantic.Passed = false
	journey.Failure = &localAIRealFailure{
		Owner:     owner,
		Assertion: assertion,
		Expected:  expected,
		Observed:  boundedLocalAIValue(observed),
	}
}

func setLocalAIInconclusive(report *localAIRealReport, owner, assertion, expected, observed string) {
	journey := &report.Journeys[0]
	report.Status = "INCONCLUSIVE"
	journey.Status = "INCONCLUSIVE"
	journey.Semantic.Passed = false
	journey.Failure = &localAIRealFailure{
		Owner: owner, Assertion: assertion, Expected: expected, Observed: boundedLocalAIValue(observed),
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

type localAIProcessExecutor struct{}
