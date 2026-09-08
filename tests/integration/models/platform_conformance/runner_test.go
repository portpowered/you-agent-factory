package platform_conformance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPortableControlledRunner(t *testing.T) {
	cases := []struct {
		name string
		mode string
		body func(*testing.T, ControlledRunner, controlledFixture)
	}{
		{name: "I-01-success-and-semantic-output", mode: controlledHelperModeSuccess, body: testControlledSuccess},
		{name: "I-02-typed-product-failure", mode: controlledHelperModeProduct, body: testControlledProductFailure},
		{name: "I-03-timeout-reaps-owned-tree", mode: controlledHelperModeTimeout, body: testControlledTimeout},
		{name: "I-04-cancellation-releases-resources", mode: controlledHelperModeCancel, body: testControlledCancellation},
		{name: "I-05-partial-artifact-is-removed", mode: controlledHelperModePartial, body: testControlledPartial},
		{name: "I-06-tree-and-listener-are-owned", mode: controlledHelperModeTree, body: testControlledTree},
		{name: "I-08-secret-output-is-redacted", mode: controlledHelperModeSecret, body: testControlledSecret},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			fixture := newControlledFixture(t, testCase.mode)
			runner := mustControlledRunner(t)
			testCase.body(t, runner, fixture)
		})
	}
}

func TestPortableControlledRunnerUsesDeclaredTimeout(t *testing.T) {
	fixture := newControlledFixture(t, controlledHelperModeTimeout)
	fixture.spec.TimeoutMillis = 1000
	admitted, err := AdmitWithInspector(fixture.spec, fixture.host, fixture.inspector)
	if err != nil {
		t.Fatalf("admit short-timeout fixture: %v", err)
	}
	runner := mustControlledRunner(t)
	ready := make(chan struct{})
	result := make(chan controlledRunResult, 1)
	go func() {
		report, runErr := runner.Run(context.Background(), admitted, ControlledRunOptions{
			OnReady: func() { close(ready) },
		})
		result <- controlledRunResult{report: report, err: runErr}
	}()
	waitControlledSignal(t, ready, "declared-timeout helper readiness")
	completed := <-result
	if completed.err != nil {
		t.Fatalf("declared-timeout run: %v", completed.err)
	}
	if !completed.report.Commands[0].TimedOut || completed.report.Commands[0].Cancelled {
		t.Fatalf("declared-timeout command evidence = %#v", completed.report.Commands[0])
	}
	assertCleanRelease(t, completed.report)
}

func TestPortableControlledRunnerDoesNotStartWhenCommandSelectionFails(t *testing.T) {
	fixture := newControlledFixture(t, controlledHelperModeSuccess)
	runner := mustControlledRunner(t)
	var starts atomic.Int32
	runner.starter = func(*exec.Cmd) error {
		starts.Add(1)
		return errors.New("unexpected controlled launch")
	}
	if _, err := runner.Run(context.Background(), fixture.admission, ControlledRunOptions{CommandName: "missing"}); err == nil {
		t.Fatal("missing admitted command was accepted")
	}
	if got := starts.Load(); got != 0 {
		t.Fatalf("launch seam calls = %d, want zero", got)
	}
}

func TestPortableControlledRunnerQuiescenceCeilingRetainsAliveCounts(t *testing.T) {
	fixture := newControlledFixture(t, controlledHelperModeTimeout)
	command, tree := startControlledObservationProcess(t, fixture, controlledHelperModeTimeout)
	t.Cleanup(func() {
		_ = terminateControlledProcess(command, tree, true)
		_ = command.Wait()
		closeControlledProcessTree(tree, true)
	})

	attempt := &controlledAttempt{
		process: command, tree: tree, treeAttached: true, waitCompleted: true,
		stdout: newControlledCapture(defaultControlledStreamLimit, nil),
	}
	runner := mustControlledRunner(t)
	runner.waitDelay = 25 * time.Millisecond
	runner.observeControlledQuiescence(fixture.spec, attempt, false)

	var ceiling controlledCleanupCeilingError
	if !errors.As(attempt.cleanupErr, &ceiling) {
		t.Fatalf("quiescence ceiling error = %v, want typed cleanup-ceiling error", attempt.cleanupErr)
	}
	if attempt.release.ProcessTreeClosed || attempt.release.OwnedProcesses == 0 || attempt.release.OwnedListeners != 0 {
		t.Fatalf("quiescence ceiling release = %#v, want retained process count", attempt.release)
	}
	status, failure, _ := classifyControlledResult(
		true, 0, false, true, true, nil, nil, attempt.cleanupErr, attempt.release,
		SemanticObservation{Assertion: controlledAssertionSemantic, Expected: "controlled output", Observed: "controlled output", Passed: true}, false,
	)
	if status != StatusFail || failure == nil || failure.Owner != controlledFailureOwnerHarness || failure.Assertion != controlledAssertionCleanup {
		t.Fatalf("quiescence ceiling classification = status %q failure %#v", status, failure)
	}
	if controlledErrorClass(attempt.cleanupErr) != "quiescence_ceiling" {
		t.Fatalf("quiescence ceiling error class = %q", controlledErrorClass(attempt.cleanupErr))
	}
}

func TestPortableControlledRunnerQuiescenceReturnsImmediatelyWhenClosed(t *testing.T) {
	fixture := newControlledFixture(t, controlledHelperModeSuccess)
	command, tree := startControlledObservationProcess(t, fixture, controlledHelperModeSuccess)
	if err := command.Wait(); err != nil {
		t.Fatalf("wait already-closed controlled process: %v", err)
	}
	if err := terminateControlledProcess(command, tree, true); err != nil {
		t.Fatalf("settle already-closed controlled process tree: %v", err)
	}
	t.Cleanup(func() { closeControlledProcessTree(tree, true) })

	attempt := &controlledAttempt{
		process: command, tree: tree, treeAttached: true, waitCompleted: true,
		stdout: newControlledCapture(defaultControlledStreamLimit, nil),
	}
	runner := mustControlledRunner(t)
	runner.waitDelay = time.Hour
	runner.observeControlledQuiescence(fixture.spec, attempt, false)
	if attempt.cleanupErr != nil || attempt.release != (ReleaseEvidence{ProcessTreeClosed: true}) {
		t.Fatalf("already-closed quiescence = release %#v cleanup=%v", attempt.release, attempt.cleanupErr)
	}
}

func TestPortableControlledRunnerRejectsArtifactReplacementBeforeLaunch(t *testing.T) {
	fixture := newReadinessFixture(t)
	admitted, err := AdmitWithInspector(fixture.spec, fixture.host, fixture.inspector)
	if err != nil {
		t.Fatalf("admit replacement fixture: %v", err)
	}
	if err := os.WriteFile(fixture.spec.CLI.Path, []byte("replacement cli\n"), 0o600); err != nil {
		t.Fatalf("replace admitted CLI artifact: %v", err)
	}
	runner := mustControlledRunner(t)
	var starts atomic.Int32
	runner.starter = func(*exec.Cmd) error {
		starts.Add(1)
		return errors.New("unexpected controlled launch")
	}
	_, err = runner.Run(context.Background(), admitted, ControlledRunOptions{})
	requireAdmissionCode(t, err, "artifact_changed")
	if got := starts.Load(); got != 0 {
		t.Fatalf("replacement launch seam calls = %d, want zero", got)
	}
	if _, statErr := os.Stat(fixture.spec.LedgerPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("artifact replacement created a ledger: %v", statErr)
	}
}

func TestPortableControlledReportInterruption(t *testing.T) {
	fixture := newControlledFixture(t, controlledHelperModeSuccess)
	store := mustLocalBudgetStore(t)
	if _, _, err := store.Reserve(context.Background(), ReservationRequest{
		LedgerPath: fixture.spec.LedgerPath, LedgerID: DefaultLedgerID(fixture.spec.RunID), RunID: fixture.spec.RunID,
		ID: "report-interruption", Kind: BudgetKindChildProcesses, Amount: 1, Command: "controlled", Limits: fixture.spec.Limits,
	}); err != nil {
		t.Fatalf("reserve report interruption budget: %v", err)
	}
	if _, err := store.Release(context.Background(), fixture.spec.LedgerPath, fixture.spec.RunID, "report-interruption"); err != nil {
		t.Fatalf("release report interruption budget: %v", err)
	}
	if _, err := store.Finalize(context.Background(), fixture.spec.LedgerPath, fixture.spec.RunID); err != nil {
		t.Fatalf("finalize report interruption budget: %v", err)
	}
	identity, err := LedgerFileIdentity(fixture.spec.LedgerPath)
	if err != nil {
		t.Fatalf("read report interruption ledger identity: %v", err)
	}
	report := fixture.admission.NewReadinessReport(identity)
	if err := WriteReportAtomic(fixture.spec.ReportPath, report); err != nil {
		t.Fatalf("write report preimage: %v", err)
	}
	before, err := os.ReadFile(fixture.spec.ReportPath)
	if err != nil {
		t.Fatalf("read report preimage: %v", err)
	}
	interrupted := errors.New("controlled report interruption")
	if err := writeReportAtomicWithHook(fixture.spec.ReportPath, report, func() error { return interrupted }); !errors.Is(err, interrupted) {
		t.Fatalf("interrupted report write error = %v, want %v", err, interrupted)
	}
	after, err := os.ReadFile(fixture.spec.ReportPath)
	if err != nil {
		t.Fatalf("read report after interruption: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("interrupted report write changed the canonical report")
	}
	entries, err := os.ReadDir(fixture.root)
	if err != nil {
		t.Fatalf("read report parent after interruption: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".platform-conformance-") {
			t.Fatalf("interrupted report write left temporary file %q", entry.Name())
		}
	}
}

func TestPortableControlledReservationContention(t *testing.T) {
	const attempts = 8
	sharedRoot := t.TempDir()
	sharedLedger := filepath.Join(sharedRoot, "contention-budget.json")
	noFinalize := false
	runner := mustControlledRunner(t)
	gate := make(chan struct{})
	results := make(chan controlledRunResult, attempts)
	var started atomic.Int32
	for index := 0; index < attempts; index++ {
		index := index
		fixture := newControlledFixture(t, controlledHelperModeSuccess)
		fixture.spec.RunID = "contention-run"
		fixture.spec.LedgerPath = sharedLedger
		fixture.spec.ReportPath = filepath.Join(fixture.root, fmt.Sprintf("contention-%d-report.json", index))
		admitted, err := AdmitWithInspector(fixture.spec, fixture.host, fixture.inspector)
		if err != nil {
			t.Fatalf("admit contention fixture %d: %v", index, err)
		}
		fixture.admission = admitted
		go func() {
			<-gate
			report, runErr := runner.Run(context.Background(), admitted, ControlledRunOptions{
				ReservationID:  fmt.Sprintf("contention-reservation-%d", index),
				FinalizeLedger: &noFinalize,
				OnStarted:      func() { started.Add(1) },
			})
			results <- controlledRunResult{report: report, err: runErr}
		}()
	}
	close(gate)

	admittedCount := 0
	failedBeforeStart := 0
	for index := 0; index < attempts; index++ {
		result := <-results
		if result.err != nil {
			t.Fatalf("contention attempt returned infrastructure error: %v", result.err)
		}
		if result.report.Status == StatusPass {
			admittedCount++
			continue
		}
		failedBeforeStart++
		if result.report.Failure == nil || result.report.Commands[0].Started {
			t.Fatalf("contention loser was not a pre-launch failure: %#v", result.report)
		}
	}
	if admittedCount != int(MaxChildProcesses) || failedBeforeStart != attempts-int(MaxChildProcesses) {
		t.Fatalf("contention outcomes = admitted %d failedBeforeStart %d, want %d and %d", admittedCount, failedBeforeStart, MaxChildProcesses, attempts-int(MaxChildProcesses))
	}
	if got := started.Load(); int64(got) != MaxChildProcesses {
		t.Fatalf("controlled children started = %d, want %d", got, MaxChildProcesses)
	}
	store := mustLocalBudgetStore(t)
	finalized, err := store.Finalize(context.Background(), sharedLedger, "contention-run")
	if err != nil {
		t.Fatalf("finalize contention ledger: %v", err)
	}
	if !finalized.Finalized || finalized.Consumed.ChildProcesses != MaxChildProcesses || len(finalized.Reservations) != int(MaxChildProcesses) {
		t.Fatalf("invalid settled contention ledger: %#v", finalized)
	}
	if err := finalized.Validate(); err != nil {
		t.Fatalf("settled contention ledger is invalid: %v", err)
	}
}

type controlledRunResult struct {
	report Report
	err    error
}

type controlledFixture struct {
	root      string
	spec      RunSpec
	command   CommandSpec
	host      HostIdentity
	inspector *fixtureInspector
	admission Admission
}

func newControlledFixture(t *testing.T, mode string) controlledFixture {
	t.Helper()
	root := t.TempDir()
	commandPath := os.Getenv(controlledHelperPathEnv)
	if strings.TrimSpace(commandPath) == "" {
		commandPath = os.Args[0]
	}
	commandPath, err := filepath.Abs(commandPath)
	if err != nil {
		t.Fatalf("resolve controlled test executable: %v", err)
	}
	t.Logf("controlled helper artifact: %s", PathIdentity(commandPath))
	cli, err := (LocalArtifactInspector{}).Inspect(commandPath)
	if err != nil {
		t.Fatalf("inspect controlled test executable: %v", err)
	}
	// Windows does not expose the executable bit through os.FileMode. The
	// command is the already compiled test executable, so the controlled
	// inspector supplies the platform-neutral executable fact explicitly.
	cli.Executable = true
	artifactRoot := filepath.Join(root, "declared-artifacts")
	if err := os.MkdirAll(artifactRoot, 0o700); err != nil {
		t.Fatalf("create controlled declared artifacts: %v", err)
	}
	backendPath := filepath.Join(artifactRoot, "backend.fixture")
	modelPath := filepath.Join(artifactRoot, "model.fixture")
	fixturePath := filepath.Join(artifactRoot, "input.fixture")
	backend := writeFixtureArtifact(t, backendPath, []byte("controlled backend\n"))
	model := writeFixtureArtifact(t, modelPath, []byte("controlled model\n"))
	input := writeFixtureArtifact(t, fixturePath, []byte("Find similar work\n"))
	rootBase := filepath.Join(root, "owned-roots")
	command := CommandSpec{
		Name: "invoke", Path: commandPath, Args: []string{"-test.run=^TestPlatformConformanceHelper$"},
		Environment: []string{
			"HOME=" + filepath.Join(root, "home"),
			controlledHelperModeEnv + "=" + mode,
		},
	}
	if mode == controlledHelperModeSecret {
		command.Environment = append(command.Environment, controlledHelperSecretEnv+"=controlled-secret-sentinel")
	}
	spec := RunSpec{
		Schema: RunSchemaV1, RunID: "run-" + strings.ReplaceAll(strings.ToLower(mode), "-", "_"),
		Target:  Target{OS: TargetLinux, Arch: ArchAMD64},
		CLI:     CLIIdentity{Path: commandPath, SHA256: cli.SHA256, SizeBytes: cli.SizeBytes, Version: "v0.0.0-controlled", Commit: "controlled-cli-001"},
		Backend: BackendIdentity{ID: "controlled-backend", Path: backendPath, SHA256: backend.SHA256, SizeBytes: backend.SizeBytes, SourceRevision: "controlled-backend-001"},
		Model:   ModelIdentity{ID: "controlled-model", Path: modelPath, SHA256: model.SHA256, SizeBytes: model.SizeBytes, Revision: "controlled-model-001"},
		Fixture: FixtureIdentity{ID: "controlled-input", Path: fixturePath, SHA256: input.SHA256, SizeBytes: input.SizeBytes, SemanticAssertion: "eight finite numeric values and non-empty controlled text"},
		Roots: Roots{
			Work: filepath.Join(rootBase, "work"), State: filepath.Join(rootBase, "state"), Cache: filepath.Join(rootBase, "cache"),
			Temp: filepath.Join(rootBase, "temp"), Output: filepath.Join(rootBase, "output"),
		},
		Port: freeControlledPort(t), TimeoutMillis: 1200000, NetworkPolicy: NetworkPolicyDeny,
		Limits:   BudgetLimits{DownloadBytes: 0, ModelCalls: 0, NetworkRequests: 0, MaxChildProcesses: MaxChildProcesses, TemporaryBytes: 1024},
		Commands: []CommandSpec{command}, ReportPath: filepath.Join(root, "report.json"), LedgerPath: filepath.Join(root, "budget.json"),
	}
	inspector := &fixtureInspector{artifacts: map[string]ObservedArtifact{
		commandPath: {Regular: cli.Regular, Executable: cli.Executable, SizeBytes: cli.SizeBytes, SHA256: cli.SHA256},
		backendPath: {Regular: true, Executable: true, SizeBytes: backend.SizeBytes, SHA256: backend.SHA256},
		modelPath:   {Regular: true, SizeBytes: model.SizeBytes, SHA256: model.SHA256},
		fixturePath: {Regular: true, SizeBytes: input.SizeBytes, SHA256: input.SHA256},
	}, errors: map[string]error{}}
	host := HostIdentity{OS: spec.Target.OS, Arch: spec.Target.Arch}
	admitted, err := AdmitWithInspector(spec, host, inspector)
	if err != nil {
		t.Fatalf("admit controlled fixture: %v", err)
	}
	return controlledFixture{root: root, spec: spec, command: command, host: host, inspector: inspector, admission: admitted}
}

func mustControlledRunner(t testing.TB) ControlledRunner {
	t.Helper()
	store := mustLocalBudgetStore(t)
	runner, err := NewControlledRunner(store)
	if err != nil {
		t.Fatalf("create controlled runner: %v", err)
	}
	return runner
}

func mustLocalBudgetStore(t testing.TB) BudgetStore {
	t.Helper()
	store, err := NewLocalBudgetStore()
	if err != nil {
		t.Fatalf("create local budget store: %v", err)
	}
	return store
}

func testControlledSuccess(t *testing.T, runner ControlledRunner, fixture controlledFixture) {
	report, err := runner.Run(context.Background(), fixture.admission, ControlledRunOptions{})
	if err != nil {
		t.Fatalf("controlled success: %v", err)
	}
	if report.Status != StatusPass || report.Failure != nil {
		t.Fatalf("success report = %#v failure=%+v", report, report.Failure)
	}
	assertControlledReportIdentity(t, report, fixture)
	command := report.Commands[0]
	if !command.Started || command.ExitCode != 0 || command.StdoutBytes == 0 || !validDigest(command.StdoutSHA256) || !command.Redacted {
		t.Fatalf("success command evidence = %#v", command)
	}
	if report.Release != (ReleaseEvidence{ProcessTreeClosed: true}) {
		t.Fatalf("success release = %#v", report.Release)
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("success report validation: %v", err)
	}
	assertFinalizedChildLedger(t, fixture.spec.LedgerPath, 1)
}

func testControlledProductFailure(t *testing.T, runner ControlledRunner, fixture controlledFixture) {
	report, err := runner.Run(context.Background(), fixture.admission, ControlledRunOptions{})
	if err != nil {
		t.Fatalf("controlled product failure: %v", err)
	}
	if report.Status != StatusFail || report.Failure == nil || report.Failure.Owner != controlledFailureOwnerProduct {
		t.Fatalf("product failure report = %#v", report)
	}
	assertControlledReportIdentity(t, report, fixture)
	command := report.Commands[0]
	if !command.Started || command.ExitCode != controlledProductFailureExitCode || command.TimedOut || command.Cancelled {
		t.Fatalf("product failure command evidence = %#v", command)
	}
	assertCleanRelease(t, report)
	assertFinalizedChildLedger(t, fixture.spec.LedgerPath, 1)
}

func testControlledTimeout(t *testing.T, runner ControlledRunner, fixture controlledFixture) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ready := make(chan struct{})
	result := make(chan controlledRunResult, 1)
	go func() {
		report, err := runner.Run(ctx, fixture.admission, ControlledRunOptions{OnReady: func() { close(ready) }})
		result <- controlledRunResult{report: report, err: err}
	}()
	waitControlledSignal(t, ready, "timeout helper readiness")
	completed := <-result
	if completed.err != nil {
		t.Fatalf("controlled timeout: %v", completed.err)
	}
	if completed.report.Status != StatusFail || completed.report.Failure == nil || completed.report.Failure.Owner != controlledFailureOwnerHarness {
		t.Fatalf("timeout report = %#v", completed.report)
	}
	if !completed.report.Commands[0].TimedOut || completed.report.Commands[0].Cancelled {
		t.Fatalf("timeout command evidence = %#v", completed.report.Commands[0])
	}
	assertCleanRelease(t, completed.report)
	assertFinalizedChildLedger(t, fixture.spec.LedgerPath, 1)
}

func testControlledCancellation(t *testing.T, runner ControlledRunner, fixture controlledFixture) {
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan controlledRunResult, 1)
	ready := make(chan struct{})
	go func() {
		report, err := runner.Run(ctx, fixture.admission, ControlledRunOptions{OnReady: func() { close(ready) }})
		result <- controlledRunResult{report: report, err: err}
	}()
	waitControlledSignal(t, ready, "cancellation helper readiness")
	cancel()
	completed := <-result
	if completed.err != nil {
		t.Fatalf("controlled cancellation: %v", completed.err)
	}
	if completed.report.Status != StatusFail || completed.report.Failure == nil || completed.report.Failure.Owner != controlledFailureOwnerHarness {
		t.Fatalf("cancellation report = %#v", completed.report)
	}
	if !completed.report.Commands[0].Cancelled || completed.report.Commands[0].TimedOut {
		t.Fatalf("cancellation command evidence = %#v", completed.report.Commands[0])
	}
	assertCleanRelease(t, completed.report)
	assertFinalizedChildLedger(t, fixture.spec.LedgerPath, 1)
}

func testControlledPartial(t *testing.T, runner ControlledRunner, fixture controlledFixture) {
	report, err := runner.Run(context.Background(), fixture.admission, ControlledRunOptions{})
	if err != nil {
		t.Fatalf("controlled partial artifact: %v", err)
	}
	if report.Status != StatusFail || report.Failure == nil || report.Failure.Owner != controlledFailureOwnerHarness {
		t.Fatalf("partial report = %#v", report)
	}
	if report.Release.PartialArtifacts != 1 || report.Release.TemporaryBytes != 0 {
		t.Fatalf("partial release = %#v", report.Release)
	}
	partialPath := filepath.Join(fixture.spec.Roots.Output, "partial-"+fixture.spec.RunID+".artifact")
	if _, err := os.Lstat(partialPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial artifact still exists: %v", err)
	}
	assertFinalizedChildLedger(t, fixture.spec.LedgerPath, 1)
}

func testControlledTree(t *testing.T, runner ControlledRunner, fixture controlledFixture) {
	unrelatedListener, unrelatedProcess, unrelatedTree := startUnrelatedControlledResources(t, fixture)
	readyObserved := make(chan bool, 1)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan controlledRunResult, 1)
	ready := make(chan struct{})
	go func() {
		report, err := runner.Run(ctx, fixture.admission, ControlledRunOptions{OnReady: func() {
			readyObserved <- !controlledListenerReleased(fixture.spec.Port)
			close(ready)
		}})
		result <- controlledRunResult{report: report, err: err}
	}()
	waitControlledSignal(t, ready, "tree helper readiness")
	if !<-readyObserved {
		t.Fatal("tree helper did not hold the declared listener before cancellation")
	}
	cancel()
	completed := <-result
	if completed.err != nil {
		t.Fatalf("controlled tree cleanup: %v", completed.err)
	}
	if completed.report.Status != StatusFail || completed.report.Failure == nil || completed.report.Failure.Owner != controlledFailureOwnerHarness {
		t.Fatalf("tree report = %#v", completed.report)
	}
	if completed.report.Release.OwnedProcesses != 0 || completed.report.Release.OwnedListeners != 0 || !completed.report.Release.ProcessTreeClosed {
		t.Fatalf("tree release = %#v failure=%#v", completed.report.Release, completed.report.Failure)
	}
	assertPortAvailable(t, fixture.spec.Port)
	if !controlledProcessTreeAlive(unrelatedTree) {
		t.Fatal("unrelated controlled process was signaled by owned cleanup")
	}
	if err := assertControlledListenerOccupied(unrelatedListener); err != nil {
		t.Fatalf("unrelated listener ownership changed: %v", err)
	}
	_ = unrelatedProcess
	assertFinalizedChildLedger(t, fixture.spec.LedgerPath, 1)
}

func testControlledSecret(t *testing.T, runner ControlledRunner, fixture controlledFixture) {
	secret := "controlled-secret-sentinel"
	report, err := runner.Run(context.Background(), fixture.admission, ControlledRunOptions{Secrets: []string{secret}})
	if err != nil {
		t.Fatalf("controlled secret output: %v", err)
	}
	if report.Status != StatusPass || !report.Commands[0].Redacted {
		t.Fatalf("secret report = %#v", report)
	}
	body, err := os.ReadFile(fixture.spec.ReportPath)
	if err != nil {
		t.Fatalf("read redacted report: %v", err)
	}
	if bytes.Contains(body, []byte(secret)) || strings.Contains(fmt.Sprintf("%#v", report), secret) {
		t.Fatalf("secret sentinel survived report evidence")
	}
	assertFinalizedChildLedger(t, fixture.spec.LedgerPath, 1)
}

func waitControlledSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	// This timer is only a safety ceiling for an observable helper readiness
	// channel; it is not a polling delay or a synchronization mechanism.
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case <-signal:
	case <-timer.C:
		t.Fatalf("%s did not arrive", name)
	}
}

func assertCleanRelease(t *testing.T, report Report) {
	t.Helper()
	if !report.Release.ProcessTreeClosed || report.Release.OwnedProcesses != 0 || report.Release.OwnedListeners != 0 || report.Release.PartialArtifacts != 0 || report.Release.TemporaryBytes != 0 {
		t.Fatalf("unclean controlled release = %#v", report.Release)
	}
}

func assertControlledReportIdentity(t *testing.T, report Report, fixture controlledFixture) {
	t.Helper()
	if report.Target != fixture.spec.Target || report.Identities.CLI.SHA256 != fixture.spec.CLI.SHA256 ||
		report.Identities.Backend.SHA256 != fixture.spec.Backend.SHA256 || report.Identities.Model.SHA256 != fixture.spec.Model.SHA256 ||
		report.Identities.Fixture.SHA256 != fixture.spec.Fixture.SHA256 || report.Ledger.PathIdentity != PathIdentity(fixture.spec.LedgerPath) ||
		!validDigest(report.Ledger.SHA256) || report.Ledger.Generation <= 0 {
		t.Fatalf("controlled report identities = %#v", report)
	}
	command := report.Commands[0]
	if command.PathIdentity != PathIdentity(fixture.command.Path) || len(command.Args) != len(fixture.command.Args) {
		t.Fatalf("controlled command identity = %#v", command)
	}
	for index := range command.Args {
		if command.Args[index] != fixture.command.Args[index] {
			t.Fatalf("controlled command args = %#v", command.Args)
		}
	}
}

func assertFinalizedChildLedger(t *testing.T, path string, want int64) {
	t.Helper()
	ledger, err := ReadBudgetLedger(path)
	if err != nil {
		t.Fatalf("read settled child ledger: %v", err)
	}
	if !ledger.Finalized || ledger.Consumed.ChildProcesses != want || len(ledger.Reservations) != int(want) {
		t.Fatalf("settled child ledger = %#v", ledger)
	}
}

func freeControlledPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve controlled port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release controlled port: %v", err)
	}
	return port
}

func assertPortAvailable(t *testing.T, port int) {
	t.Helper()
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("controlled listener port %d remains occupied: %v", port, err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close controlled listener probe: %v", err)
	}
}

func startControlledObservationProcess(t testing.TB, fixture controlledFixture, mode string) (*exec.Cmd, controlledProcessTree) {
	t.Helper()
	command := exec.Command(fixture.command.Path, fixture.command.Args...)
	command.Env = setEnvironmentValue(append([]string(nil), fixture.command.Environment...), controlledHelperModeEnv, mode)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	configureControlledProcessTree(command)
	if err := command.Start(); err != nil {
		t.Fatalf("start controlled observation process: %v", err)
	}
	tree, err := attachControlledProcessTree(command)
	if err != nil {
		_ = terminateControlledProcess(command, tree, false)
		_ = command.Wait()
		t.Fatalf("attach controlled observation process: %v", err)
	}
	return command, tree
}

func startUnrelatedControlledResources(t testing.TB, fixture controlledFixture) (net.Listener, *exec.Cmd, controlledProcessTree) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start unrelated listener: %v", err)
	}
	command, tree := startControlledObservationProcess(t, fixture, controlledHelperModeTimeout)
	t.Cleanup(func() {
		_ = terminateControlledProcess(command, tree, true)
		_ = command.Wait()
		closeControlledProcessTree(tree, true)
		_ = listener.Close()
	})
	return listener, command, tree
}

func assertControlledListenerOccupied(listener net.Listener) error {
	address := listener.Addr().String()
	probe, err := net.Listen("tcp4", address)
	if err == nil {
		_ = probe.Close()
		return fmt.Errorf("listener %s was reusable while its owner remained active", address)
	}
	return nil
}
