package platform_conformance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultControlledStreamLimit     int64 = 64 << 10
	defaultControlledWaitDelay             = 250 * time.Millisecond
	controlledProductFailureExitCode       = 42

	controlledHelperModeEnv        = "LOCALAI_PLATFORM_CONFORMANCE_HELPER_MODE"
	controlledHelperOutputRootEnv  = "LOCALAI_PLATFORM_CONFORMANCE_OUTPUT_ROOT"
	controlledHelperPartialPathEnv = "LOCALAI_PLATFORM_CONFORMANCE_PARTIAL_PATH"
	controlledHelperPortEnv        = "LOCALAI_PLATFORM_CONFORMANCE_PORT"
	controlledHelperSecretEnv      = "LOCALAI_PLATFORM_CONFORMANCE_SECRET"
	controlledHelperModeSuccess    = "success"
	controlledHelperModeProduct    = "product-failure"
	controlledHelperModeTimeout    = "timeout"
	controlledHelperModeCancel     = "cancel"
	controlledHelperModePartial    = "partial"
	controlledHelperModeTree       = "tree"
	controlledHelperModeListener   = "listener-child"
	controlledHelperModeSecret     = "secret"
	controlledHelperPathEnv        = "LOCALAI_PLATFORM_CONFORMANCE_HELPER_PATH"
)

const (
	controlledFailureOwnerHarness = "harness"
	controlledFailureOwnerProduct = "product"

	controlledAssertionResult    = "controlled command result"
	controlledAssertionSemantic  = "controlled semantic output"
	controlledAssertionCleanup   = "owned resource cleanup"
	controlledAssertionRedaction = "bounded secret redaction"
)

// ControlledRunner is the integration-only process boundary for the private
// platform conformance package. It consumes an already admitted run and never
// resolves a command, model, backend, or network dependency on its own.
type ControlledRunner struct {
	budget      BudgetStore
	streamLimit int64
	waitDelay   time.Duration
	writeReport func(string, Report) error
	starter     controlledProcessStarter
}

type controlledProcessStarter func(*exec.Cmd) error

// ControlledRunOptions carries observations and lifecycle choices owned by a
// single integration scenario. A false FinalizeLedger value is used only by
// the deliberate shared-ledger contention case; its parent finalizes once all
// independent attempts have joined.
type ControlledRunOptions struct {
	CommandName    string
	ReservationID  string
	Secrets        []string
	FinalizeLedger *bool
	OnReady        func()
	OnStarted      func()
}

type controlledAttempt struct {
	commandIndex    int
	command         CommandSpec
	environment     []string
	redactionTokens []string
	partialPath     string
	reservation     BudgetReservation
	process         *exec.Cmd
	tree            controlledProcessTree
	stdout          *controlledCapture
	stderr          *controlledCapture
	started         bool
	treeAttached    bool
	waitCompleted   bool
	timedOut        bool
	cancelled       bool
	startErr        error
	waitErr         error
	cleanupErr      error
	release         ReleaseEvidence
	exitCode        int
	semantic        SemanticObservation
	commandEvidence CommandEvidence
}

// NewControlledRunner constructs the controlled executable adapter from the
// existing durable budget store. The default writer is the package's atomic,
// canonical report writer so callers cannot accidentally publish raw output.
func NewControlledRunner(budget BudgetStore) (ControlledRunner, error) {
	if budget.locker == nil {
		return ControlledRunner{}, errors.New("platform conformance controlled runner budget store is required")
	}
	return ControlledRunner{
		budget: budget, streamLimit: defaultControlledStreamLimit,
		waitDelay: defaultControlledWaitDelay, writeReport: WriteReportAtomic,
		starter: func(command *exec.Cmd) error { return command.Start() },
	}, nil
}

// Run executes one command from an admitted specification. Expected product,
// timeout, cancellation, cleanup, and budget outcomes are represented in the
// returned report. An error means the harness could not produce durable
// evidence at all.
func (runner ControlledRunner) Run(ctx context.Context, admission Admission, options ControlledRunOptions) (Report, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	runContext, cancel := context.WithTimeout(ctx, time.Duration(admission.Spec.TimeoutMillis)*time.Millisecond)
	defer cancel()
	commandIndex, command, err := admittedCommand(admission, options.CommandName)
	if err != nil {
		return Report{}, err
	}
	if err := prepareControlledRoots(admission.Spec.Roots); err != nil {
		return Report{}, fmt.Errorf("prepare controlled run roots: %w", err)
	}

	commandEnvironment := controlledCommandEnvironment(command)
	partialPath := filepath.Join(admission.Spec.Roots.Output, "partial-"+admission.Spec.RunID+".artifact")
	commandEnvironment = setEnvironmentValue(commandEnvironment, controlledHelperPartialPathEnv, partialPath)
	commandEnvironment = setEnvironmentValue(commandEnvironment, controlledHelperPortEnv, strconv.Itoa(admission.Spec.Port))
	commandEnvironment = setEnvironmentValue(commandEnvironment, controlledHelperOutputRootEnv, admission.Spec.Roots.Output)
	redactionTokens := controlledRedactionTokens(admission, commandEnvironment, options.Secrets)
	stagedPath, cleanupStage, err := stageControlledCommand(admission.Spec, command)
	if err != nil {
		return Report{}, err
	}

	reservationID := options.ReservationID
	if reservationID == "" {
		reservationID = "reservation-" + admission.Spec.RunID + "-" + command.Name
	}
	reservation, _, reserveErr := runner.budget.Reserve(runContext, ReservationRequest{
		LedgerPath: admission.Spec.LedgerPath,
		LedgerID:   DefaultLedgerID(admission.Spec.RunID),
		RunID:      admission.Spec.RunID,
		ID:         reservationID,
		Kind:       BudgetKindChildProcesses,
		Amount:     1,
		Command:    command.Name,
		Limits:     admission.Spec.Limits,
	})
	if reserveErr != nil {
		if cleanupErr := cleanupStage(); cleanupErr != nil {
			return Report{}, errors.Join(reserveErr, fmt.Errorf("clean staged controlled command: %w", cleanupErr))
		}
		return runner.finishUnstarted(admission, commandIndex, command, commandEnvironment, redactionTokens, reserveErr)
	}
	attempt := controlledAttempt{
		commandIndex: commandIndex, command: command, environment: commandEnvironment,
		redactionTokens: redactionTokens, partialPath: partialPath, reservation: reservation,
	}
	runner.executeControlledAttempt(runContext, admission.Spec, options, stagedPath, &attempt)
	attempt.cleanupErr = errors.Join(attempt.cleanupErr, cleanupStage())
	return runner.finishControlledAttempt(admission, options, &attempt)
}

func (runner ControlledRunner) executeControlledAttempt(ctx context.Context, spec RunSpec, options ControlledRunOptions, executablePath string, attempt *controlledAttempt) {
	attempt.stdout = newControlledCapture(runner.streamLimit, runner.readyObserver(options))
	attempt.stderr = newControlledCapture(runner.streamLimit, nil)
	attempt.process = exec.Command(executablePath, attempt.command.Args...)
	attempt.process.Dir = spec.Roots.Work
	attempt.process.Env = attempt.environment
	attempt.process.Stdout = attempt.stdout
	attempt.process.Stderr = attempt.stderr
	attempt.process.WaitDelay = runner.waitDelay
	configureControlledProcessTree(attempt.process)
	waitCh := runner.startControlledAttempt(options, attempt)
	runner.waitControlledAttempt(ctx, waitCh, attempt)
	runner.collectControlledAttempt(spec, attempt)
}

func (runner ControlledRunner) startControlledAttempt(options ControlledRunOptions, attempt *controlledAttempt) <-chan error {
	starter := runner.starter
	if starter == nil {
		starter = func(command *exec.Cmd) error { return command.Start() }
	}
	if err := starter(attempt.process); err != nil {
		attempt.startErr = err
		attempt.cleanupErr = err
		return nil
	}
	attempt.started = true
	if options.OnStarted != nil {
		options.OnStarted()
	}
	var err error
	attempt.tree, err = attachControlledProcessTree(attempt.process)
	if err != nil {
		attempt.startErr = err
		return channelForControlledWait(attempt.process)
	}
	attempt.treeAttached = true
	return channelForControlledWait(attempt.process)
}

func channelForControlledWait(command *exec.Cmd) <-chan error {
	waitCh := make(chan error, 1)
	go func() { waitCh <- command.Wait() }()
	return waitCh
}

func (runner ControlledRunner) waitControlledAttempt(ctx context.Context, waitCh <-chan error, attempt *controlledAttempt) {
	if !attempt.started {
		return
	}
	select {
	case attempt.waitErr = <-waitCh:
		attempt.waitCompleted = true
	case <-ctx.Done():
		attempt.timedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		attempt.cancelled = errors.Is(ctx.Err(), context.Canceled)
		attempt.cleanupErr = requestControlledProcessStop(attempt.process, attempt.tree, attempt.treeAttached)
		attempt.waitErr, attempt.waitCompleted = waitForControlledProcess(waitCh, runner.waitDelay)
		if !attempt.waitCompleted {
			attempt.cleanupErr = errors.Join(attempt.cleanupErr, terminateControlledProcess(attempt.process, attempt.tree, attempt.treeAttached))
			attempt.waitErr, attempt.waitCompleted = waitForControlledProcess(waitCh, runner.waitDelay)
		}
	}
	attempt.cleanupErr = errors.Join(attempt.cleanupErr, terminateControlledProcess(attempt.process, attempt.tree, attempt.treeAttached))
	if !attempt.waitCompleted {
		attempt.waitErr, attempt.waitCompleted = waitForControlledProcess(waitCh, runner.waitDelay)
	}
}

func (runner ControlledRunner) collectControlledAttempt(spec RunSpec, attempt *controlledAttempt) {
	listenerObserved := attempt.stdout.sawEvent("listener-ready")
	runner.observeControlledQuiescence(spec, attempt, listenerObserved)
	partialCount, partialBytes, partialErr := cleanControlledPartial(attempt.partialPath)
	attempt.release.PartialArtifacts = partialCount
	attempt.release.TemporaryBytes = partialBytes
	attempt.cleanupErr = errors.Join(attempt.cleanupErr, partialErr)
	closeControlledProcessTree(attempt.tree, attempt.treeAttached)
	attempt.exitCode = controlledExitCode(attempt.process, attempt.waitErr, attempt.started)
	attempt.commandEvidence = runner.commandEvidence(attempt.command, attempt.environment, attempt.redactionTokens, attempt.started, attempt.exitCode, attempt.timedOut, attempt.cancelled, attempt.stdout, attempt.stderr)
	attempt.semantic = controlledSemanticObservation(attempt.stdout.Bytes(), attempt.redactionTokens)
}

func (runner ControlledRunner) finishControlledAttempt(admission Admission, options ControlledRunOptions, attempt *controlledAttempt) (Report, error) {
	ledger, settlementErr := runner.settleReservation(admission.Spec, attempt.reservation, attempt.started, shouldFinalizeLedger(options))
	attempt.cleanupErr = errors.Join(attempt.cleanupErr, settlementErr)
	status, failure, observations := classifyControlledResult(
		attempt.started, attempt.exitCode, attempt.timedOut, attempt.cancelled, attempt.waitCompleted,
		attempt.startErr, attempt.waitErr, attempt.cleanupErr, attempt.release, attempt.semantic,
		attempt.stdout.truncated || attempt.stderr.truncated,
	)
	if len(options.Secrets) > 0 || hasEnvironmentSecret(attempt.command.Environment) {
		observations = append(observations, SemanticObservation{
			Assertion: controlledAssertionRedaction,
			Expected:  "secret values are absent from returned and persisted evidence",
			Observed:  "stream, argument, and failure evidence was sanitized before hashing",
			Passed:    true,
		})
	}
	return runner.persistControlledReport(admission, attempt, ledger, status, failure, observations)
}

func (runner ControlledRunner) persistControlledReport(admission Admission, attempt *controlledAttempt, ledger BudgetLedger, status string, failure *ReportFailure, observations []SemanticObservation) (Report, error) {
	ledgerIdentity, err := LedgerFileIdentity(admission.Spec.LedgerPath)
	if err != nil && ledger.Schema != "" {
		ledgerIdentity = LedgerIdentity{PathIdentity: PathIdentity(admission.Spec.LedgerPath), SHA256: SHA256Hex(nil), Generation: ledger.Generation}
	}
	if err != nil && ledger.Schema == "" {
		return Report{}, fmt.Errorf("read settled controlled ledger identity: %w", err)
	}
	report := admission.NewReadinessReport(ledgerIdentity)
	report.Status = status
	report.Failure = failure
	report.Commands[attempt.commandIndex] = attempt.commandEvidence
	report.SemanticObservations = append(report.SemanticObservations, observations...)
	report.Release = attempt.release
	if err := report.Validate(); err != nil {
		return Report{}, fmt.Errorf("validate controlled report: %w", err)
	}
	if err := runner.writeReport(admission.Spec.ReportPath, report); err != nil {
		return report, fmt.Errorf("persist controlled report: %w", err)
	}
	return report, nil
}

func (runner ControlledRunner) finishUnstarted(
	admission Admission,
	commandIndex int,
	command CommandSpec,
	commandEnvironment []string,
	redactionTokens []string,
	reserveErr error,
) (Report, error) {
	ledgerIdentity, err := LedgerFileIdentity(admission.Spec.LedgerPath)
	if err != nil {
		return Report{}, fmt.Errorf("read unstarted controlled ledger identity: %w", err)
	}
	report := admission.NewReadinessReport(ledgerIdentity)
	report.Status = StatusFail
	report.Failure = &ReportFailure{
		Owner: controlledFailureOwnerHarness, Assertion: "child-process budget reservation",
		Expected: "reservation is admitted before executable start", Observed: controlledErrorClass(reserveErr),
	}
	report.Commands[commandIndex] = runner.commandEvidence(
		command, commandEnvironment, redactionTokens, false, -1, false, false,
		newControlledCapture(0, nil), newControlledCapture(0, nil),
	)
	report.SemanticObservations = append(report.SemanticObservations, SemanticObservation{
		Assertion: controlledAssertionResult,
		Expected:  "budget losers fail before process start",
		Observed:  "reservation rejected before command start",
		Passed:    true,
	})
	report.Release = ReleaseEvidence{ProcessTreeClosed: true}
	if err := report.Validate(); err != nil {
		return Report{}, fmt.Errorf("validate unstarted controlled report: %w", err)
	}
	if err := runner.writeReport(admission.Spec.ReportPath, report); err != nil {
		return report, fmt.Errorf("persist unstarted controlled report: %w", err)
	}
	return report, nil
}

func (runner ControlledRunner) settleReservation(spec RunSpec, reservation BudgetReservation, started, finalize bool) (BudgetLedger, error) {
	ctx := context.Background()
	var (
		ledger BudgetLedger
		err    error
	)
	if started {
		ledger, err = runner.budget.Commit(ctx, spec.LedgerPath, spec.RunID, reservation.ID)
	} else {
		ledger, err = runner.budget.Release(ctx, spec.LedgerPath, spec.RunID, reservation.ID)
	}
	if err != nil {
		return BudgetLedger{}, err
	}
	if !finalize {
		return ledger, nil
	}
	return runner.budget.Finalize(ctx, spec.LedgerPath, spec.RunID)
}

func (runner ControlledRunner) commandEvidence(
	command CommandSpec,
	environment []string,
	redactionTokens []string,
	started bool,
	exitCode int,
	timedOut bool,
	cancelled bool,
	stdout, stderr *controlledCapture,
) CommandEvidence {
	stdoutBytes := redactBytes(stdout.Bytes(), redactionTokens)
	stderrBytes := redactBytes(stderr.Bytes(), redactionTokens)
	return CommandEvidence{
		Name: command.Name, PathIdentity: PathIdentity(command.Path),
		Args: redactStrings(command.Args, redactionTokens), EnvironmentKeys: environmentKeys(environment),
		Started: started, ExitCode: exitCode, TimedOut: timedOut, Cancelled: cancelled,
		StdoutBytes: int64(len(stdoutBytes)), StdoutSHA256: SHA256Hex(stdoutBytes),
		StderrBytes: int64(len(stderrBytes)), StderrSHA256: SHA256Hex(stderrBytes), Redacted: true,
	}
}

func (runner ControlledRunner) readyObserver(options ControlledRunOptions) func(string) {
	var ready sync.Once
	return func(line string) {
		var event controlledHelperEvent
		if json.Unmarshal([]byte(line), &event) != nil || event.Event != "ready" {
			return
		}
		ready.Do(func() {
			if options.OnReady != nil {
				options.OnReady()
			}
		})
	}
}

func (runner ControlledRunner) releaseEvidence(
	spec RunSpec,
	command *exec.Cmd,
	tree controlledProcessTree,
	treeAttached, waitCompleted bool,
	cleanupErr error,
	listenerObserved bool,
) ReleaseEvidence {
	release := ReleaseEvidence{
		ProcessTreeClosed: treeAttached && waitCompleted && cleanupErr == nil,
	}
	if treeAttached && controlledProcessTreeAlive(tree) {
		release.ProcessTreeClosed = false
		release.OwnedProcesses = 1
	}
	if listenerObserved && !controlledListenerReleased(spec.Port) {
		release.OwnedListeners = 1
		release.ProcessTreeClosed = false
	}
	if command == nil || command.Process == nil {
		release.ProcessTreeClosed = release.ProcessTreeClosed || !treeAttached
	}
	return release
}

func admittedCommand(admission Admission, name string) (int, CommandSpec, error) {
	for index, command := range admission.Spec.Commands {
		if name == "" || command.Name == name {
			return index, command, nil
		}
	}
	return 0, CommandSpec{}, fmt.Errorf("controlled command %q is not present in admitted specification", name)
}

func prepareControlledRoots(roots Roots) error {
	for _, path := range []string{roots.Work, roots.State, roots.Cache, roots.Temp, roots.Output} {
		if err := rejectSymlinkComponents(path); err != nil {
			return err
		}
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.MkdirAll(path, 0o700); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("controlled root %s is not a directory", pathIdentity(path))
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		if len(entries) != 0 {
			return fmt.Errorf("controlled root %s is not empty", pathIdentity(path))
		}
	}
	return nil
}

func stageControlledCommand(spec RunSpec, command CommandSpec) (string, func() error, error) {
	if command.Path != spec.CLI.Path {
		return "", nil, admissionFailure("command_path_drift", "command.path", spec.CLI.Path, command.Path, nil)
	}
	source, sourceInfo, err := openControlledArtifact(command.Path)
	if err != nil {
		return "", nil, admissionFailure("artifact_changed", "cli.path", spec.CLI.SHA256, "unavailable during staging", err)
	}
	defer source.Close()

	stagedPath := filepath.Join(spec.Roots.Temp, "platform-conformance-command"+filepath.Ext(command.Path))
	destination, err := os.OpenFile(stagedPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		return "", nil, fmt.Errorf("create staged controlled command: %w", err)
	}
	if err := destination.Chmod(sourceInfo.Mode().Perm()); err != nil {
		_ = destination.Close()
		_ = os.Remove(stagedPath)
		return "", nil, fmt.Errorf("protect staged controlled command: %w", err)
	}
	digest, size, copyErr := copyControlledArtifact(destination, source)
	if copyErr == nil {
		copyErr = destination.Sync()
	}
	closeErr := destination.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(stagedPath)
		return "", nil, fmt.Errorf("copy controlled command: %w", errors.Join(copyErr, closeErr))
	}
	if size != spec.CLI.SizeBytes || !strings.EqualFold(digest, spec.CLI.SHA256) {
		_ = os.Remove(stagedPath)
		return "", nil, admissionFailure(
			"artifact_changed", "cli.path", spec.CLI.SHA256,
			fmt.Sprintf("sha256:%s size=%d", digest, size), nil,
		)
	}
	if err := verifyControlledSource(command.Path, sourceInfo); err != nil {
		_ = os.Remove(stagedPath)
		return "", nil, admissionFailure("artifact_changed", "cli.path", spec.CLI.SHA256, "source replaced during staging", err)
	}
	return stagedPath, func() error { return removeStagedCommand(stagedPath) }, nil
}

func openControlledArtifact(path string) (*os.File, os.FileInfo, error) {
	if err := rejectSymlinkComponents(path); err != nil {
		return nil, nil, err
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() {
		return nil, nil, errors.New("declared command is not a regular file")
	}
	source, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	sourceInfo, err := source.Stat()
	if err != nil || !os.SameFile(pathInfo, sourceInfo) {
		_ = source.Close()
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, errors.New("declared command changed before staging")
	}
	return source, sourceInfo, nil
}

func copyControlledArtifact(destination, source *os.File) (string, int64, error) {
	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(destination, hasher), source)
	return hex.EncodeToString(hasher.Sum(nil)), size, err
}

func verifyControlledSource(path string, openedInfo os.FileInfo) error {
	if err := rejectSymlinkComponents(path); err != nil {
		return err
	}
	currentInfo, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if currentInfo.Mode()&os.ModeSymlink != 0 || !currentInfo.Mode().IsRegular() {
		return errors.New("declared command is no longer a regular file")
	}
	if !os.SameFile(openedInfo, currentInfo) {
		return errors.New("declared command was replaced during staging")
	}
	return nil
}

func removeStagedCommand(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("staged controlled command is not a regular file")
	}
	return os.Remove(path)
}

func controlledCommandEnvironment(command CommandSpec) []string {
	environment := append([]string(nil), command.Environment...)
	environment = setEnvironmentValue(environment, controlledHelperModeEnv, helperModeFromCommand(command))
	return environment
}

func helperModeFromCommand(command CommandSpec) string {
	for _, entry := range command.Environment {
		key, value, ok := strings.Cut(entry, "=")
		if ok && key == controlledHelperModeEnv {
			return value
		}
	}
	return ""
}

func setEnvironmentValue(environment []string, key, value string) []string {
	for index, entry := range environment {
		entryKey, _, ok := strings.Cut(entry, "=")
		if ok && entryKey == key {
			environment[index] = key + "=" + value
			return environment
		}
	}
	return append(environment, key+"="+value)
}

func controlledRedactionTokens(admission Admission, environment, secrets []string) []string {
	tokens := make([]string, 0, len(secrets)+len(environment)+10)
	explicit := make(map[string]struct{}, len(secrets))
	for _, secret := range secrets {
		if secret != "" {
			explicit[secret] = struct{}{}
			tokens = append(tokens, secret)
		}
	}
	for _, entry := range environment {
		_, value, ok := strings.Cut(entry, "=")
		if ok && len(value) >= 3 {
			tokens = append(tokens, value)
		}
	}
	tokens = append(tokens,
		admission.Spec.CLI.Path, admission.Spec.Backend.Path, admission.Spec.Model.Path,
		admission.Spec.Fixture.Path, admission.Spec.ReportPath, admission.Spec.LedgerPath,
		admission.Spec.Roots.Work, admission.Spec.Roots.State, admission.Spec.Roots.Cache,
		admission.Spec.Roots.Temp, admission.Spec.Roots.Output,
	)
	unique := make(map[string]struct{}, len(tokens))
	filtered := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if token == "" {
			continue
		}
		if len(token) < 3 {
			if _, isExplicitSecret := explicit[token]; !isExplicitSecret {
				continue
			}
		}
		if _, exists := unique[token]; exists {
			continue
		}
		unique[token] = struct{}{}
		filtered = append(filtered, token)
	}
	sort.Slice(filtered, func(first, second int) bool { return len(filtered[first]) > len(filtered[second]) })
	return filtered
}

func hasEnvironmentSecret(environment []string) bool {
	for _, entry := range environment {
		key, _, ok := strings.Cut(entry, "=")
		if ok && key == controlledHelperSecretEnv {
			return true
		}
	}
	return false
}

func shouldFinalizeLedger(options ControlledRunOptions) bool {
	return options.FinalizeLedger == nil || *options.FinalizeLedger
}

func terminateControlledProcess(command *exec.Cmd, tree controlledProcessTree, treeAttached bool) error {
	if !treeAttached {
		if command == nil || command.Process == nil {
			return nil
		}
		err := command.Process.Kill()
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return err
	}
	return terminateControlledProcessTree(command, tree)
}

func requestControlledProcessStop(command *exec.Cmd, tree controlledProcessTree, treeAttached bool) error {
	if !treeAttached {
		if command == nil || command.Process == nil {
			return nil
		}
		err := command.Process.Signal(os.Interrupt)
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return err
	}
	return requestControlledProcessTreeStop(command, tree)
}

func waitForControlledProcess(waitCh <-chan error, delay time.Duration) (error, bool) {
	if delay <= 0 {
		select {
		case err := <-waitCh:
			return err, true
		default:
			return errors.New("controlled process did not reach a terminal wait signal"), false
		}
	}
	timer := time.NewTimer(controlledCleanupCeiling(delay))
	defer timer.Stop()
	select {
	case err := <-waitCh:
		return err, true
	case <-timer.C:
		return errors.New("controlled process wait exceeded cleanup ceiling"), false
	}
}

func controlledExitCode(command *exec.Cmd, waitErr error, started bool) int {
	if !started {
		return -1
	}
	if command != nil && command.ProcessState != nil {
		return command.ProcessState.ExitCode()
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func classifyControlledResult(
	started bool,
	exitCode int,
	timedOut, cancelled, waitCompleted bool,
	startErr, waitErr, cleanupErr error,
	release ReleaseEvidence,
	semantic SemanticObservation,
	truncated bool,
) (string, *ReportFailure, []SemanticObservation) {
	observations := []SemanticObservation{semantic}
	if !started {
		return StatusFail, &ReportFailure{
			Owner: controlledFailureOwnerHarness, Assertion: "controlled command start",
			Expected: "executable starts after reservation", Observed: controlledErrorClass(startErr),
		}, observations
	}
	if isControlledCleanupCeiling(cleanupErr) {
		return StatusFail, &ReportFailure{
			Owner: controlledFailureOwnerHarness, Assertion: controlledAssertionCleanup,
			Expected: "owned process tree and listeners close before the cleanup ceiling", Observed: controlledReleaseObservation(release, cleanupErr, waitErr),
		}, observations
	}
	if timedOut {
		return StatusFail, &ReportFailure{
			Owner: controlledFailureOwnerHarness, Assertion: "controlled command timeout",
			Expected: "owned process tree terminates at the context deadline", Observed: "context deadline exceeded",
		}, observations
	}
	if cancelled {
		return StatusFail, &ReportFailure{
			Owner: controlledFailureOwnerHarness, Assertion: "controlled command cancellation",
			Expected: "owned process tree terminates after cancellation", Observed: "context canceled",
		}, observations
	}
	if truncated {
		return StatusFail, &ReportFailure{
			Owner: controlledFailureOwnerHarness, Assertion: "bounded command streams",
			Expected: "stdout and stderr stay within the capture limit", Observed: "stream capture limit exceeded",
		}, observations
	}
	if cleanupErr != nil || !waitCompleted || !release.ProcessTreeClosed || release.OwnedProcesses != 0 || release.OwnedListeners != 0 {
		return StatusFail, &ReportFailure{
			Owner: controlledFailureOwnerHarness, Assertion: controlledAssertionCleanup,
			Expected: "owned process tree and listeners are closed", Observed: controlledReleaseObservation(release, cleanupErr, waitErr),
		}, observations
	}
	if release.PartialArtifacts > 0 || release.TemporaryBytes > 0 {
		return StatusFail, &ReportFailure{
			Owner: controlledFailureOwnerHarness, Assertion: "partial artifact cleanup",
			Expected: "owned partial artifacts are detected and removed", Observed: fmt.Sprintf("detected=%d remainingBytes=%d", release.PartialArtifacts, release.TemporaryBytes),
		}, observations
	}
	if exitCode == controlledProductFailureExitCode {
		observations = append(observations, SemanticObservation{
			Assertion: controlledAssertionResult,
			Expected:  "typed product failure is classified as product-owned",
			Observed:  "controlled product failure exit code 42",
			Passed:    true,
		})
		return StatusFail, &ReportFailure{
			Owner: controlledFailureOwnerProduct, Assertion: controlledAssertionResult,
			Expected: "controlled product failure is surfaced without harness misclassification", Observed: "typed product failure exit code 42",
		}, observations
	}
	if exitCode != 0 {
		return StatusFail, &ReportFailure{
			Owner: controlledFailureOwnerHarness, Assertion: controlledAssertionResult,
			Expected: "zero exit status or the declared product failure status", Observed: "bounded non-zero exit status",
		}, observations
	}
	if !semantic.Passed {
		return StatusFail, &ReportFailure{
			Owner: controlledFailureOwnerProduct, Assertion: controlledAssertionSemantic,
			Expected: "declared finite semantic values are emitted", Observed: semantic.Observed,
		}, observations
	}
	return StatusPass, nil, observations
}

func controlledSemanticObservation(body []byte, redactionTokens []string) SemanticObservation {
	sanitized := redactBytes(body, redactionTokens)
	for _, line := range strings.Split(string(sanitized), "\n") {
		var event controlledHelperEvent
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &event) != nil || event.Event != "semantic" {
			continue
		}
		finite := len(event.Values) == 8
		for _, value := range event.Values {
			finite = finite && value == value && value > -1e308 && value < 1e308
		}
		if finite && strings.TrimSpace(event.Text) != "" {
			return SemanticObservation{
				Assertion: controlledAssertionSemantic,
				Expected:  "eight finite numeric values and non-empty controlled text",
				Observed:  "eight finite numeric values and controlled semantic text",
				Passed:    true,
			}
		}
		return SemanticObservation{
			Assertion: controlledAssertionSemantic,
			Expected:  "eight finite numeric values and non-empty controlled text",
			Observed:  "semantic event was malformed or non-finite",
			Passed:    false,
		}
	}
	return SemanticObservation{
		Assertion: controlledAssertionSemantic,
		Expected:  "eight finite numeric values and non-empty controlled text",
		Observed:  "semantic event was not emitted",
		Passed:    false,
	}
}

func controlledReleaseObservation(release ReleaseEvidence, cleanupErr, waitErr error) string {
	parts := []string{
		fmt.Sprintf("treeClosed=%t", release.ProcessTreeClosed),
		fmt.Sprintf("ownedProcesses=%d", release.OwnedProcesses),
		fmt.Sprintf("ownedListeners=%d", release.OwnedListeners),
	}
	if cleanupErr != nil {
		parts = append(parts, "cleanup="+controlledErrorClass(cleanupErr))
	}
	if waitErr != nil {
		parts = append(parts, "wait="+controlledErrorClass(waitErr))
	}
	return strings.Join(parts, ",")
}

func controlledErrorClass(err error) string {
	if err == nil {
		return "none"
	}
	if isControlledCleanupCeiling(err) {
		return "quiescence_ceiling"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, os.ErrNotExist) {
		return "missing"
	}
	return "bounded_failure"
}

func cleanControlledPartial(path string) (int, int64, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 1, 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return 1, 0, errors.New("controlled partial artifact is not a regular file")
	}
	bytesRemaining := info.Size()
	if err := os.Remove(path); err != nil {
		return 1, bytesRemaining, err
	}
	if _, err := os.Lstat(path); err == nil {
		return 1, bytesRemaining, errors.New("controlled partial artifact remained after removal")
	} else if !errors.Is(err, os.ErrNotExist) {
		return 1, bytesRemaining, err
	}
	return 1, 0, nil
}

func controlledListenerReleased(port int) bool {
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	return listener.Close() == nil
}

type controlledCapture struct {
	limit     int64
	data      bytes.Buffer
	truncated bool
	onLine    func(string)
	partial   []byte
	events    map[string]bool
}

func newControlledCapture(limit int64, onLine func(string)) *controlledCapture {
	return &controlledCapture{limit: limit, onLine: onLine, events: make(map[string]bool)}
}

func (capture *controlledCapture) Write(body []byte) (int, error) {
	if capture.limit <= 0 {
		capture.truncated = len(body) > 0
		return len(body), nil
	}
	remaining := capture.limit - int64(capture.data.Len())
	if remaining <= 0 {
		capture.truncated = true
	} else if int64(len(body)) > remaining {
		_, _ = capture.data.Write(body[:remaining])
		capture.truncated = true
	} else {
		_, _ = capture.data.Write(body)
	}
	remainingPartial := capture.limit - int64(len(capture.partial))
	if remainingPartial <= 0 {
		capture.truncated = true
	} else if int64(len(body)) > remainingPartial {
		capture.partial = append(capture.partial, body[:remainingPartial]...)
		capture.truncated = true
	} else {
		capture.partial = append(capture.partial, body...)
	}
	for {
		newline := bytes.IndexByte(capture.partial, '\n')
		if newline < 0 {
			break
		}
		line := strings.TrimSuffix(string(capture.partial[:newline]), "\r")
		capture.observeLine(line)
		capture.partial = append([]byte(nil), capture.partial[newline+1:]...)
	}
	return len(body), nil
}

func (capture *controlledCapture) Flush() {
	if len(capture.partial) > 0 {
		capture.observeLine(strings.TrimSuffix(string(capture.partial), "\r"))
		capture.partial = nil
	}
}

func (capture *controlledCapture) observeLine(line string) {
	var event controlledHelperEvent
	if json.Unmarshal([]byte(strings.TrimSpace(line)), &event) == nil && event.Event != "" {
		capture.events[event.Event] = true
	}
	if capture.onLine != nil {
		capture.onLine(line)
	}
}

func (capture *controlledCapture) sawEvent(event string) bool { return capture.events[event] }

func (capture *controlledCapture) Bytes() []byte {
	capture.Flush()
	return append([]byte(nil), capture.data.Bytes()...)
}

type controlledHelperEvent struct {
	Event  string    `json:"event"`
	Text   string    `json:"text"`
	Values []float64 `json:"values"`
}

func redactBytes(body []byte, tokens []string) []byte {
	result := append([]byte(nil), body...)
	for _, token := range tokens {
		result = bytes.ReplaceAll(result, []byte(token), []byte("[REDACTED]"))
	}
	return result
}

func redactStrings(values, tokens []string) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = string(redactBytes([]byte(value), tokens))
	}
	return result
}

func writeControlledLine(writer io.Writer, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(writer, string(body))
	return err
}
