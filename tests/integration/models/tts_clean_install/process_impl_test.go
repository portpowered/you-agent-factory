package tts_clean_install

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
	"strings"
	"sync"
	"time"
)

const (
	deterministicHelperIdentity               = "tts-clean-install-helper/v2"
	deterministicHelperPathEnvironment        = "INFINITE_YOU_TTS_PREBUILT_HELPER_PATH"
	deterministicHelperHashEnvironment        = "INFINITE_YOU_TTS_PREBUILT_HELPER_SHA256"
	deterministicHelperModeEnvironment        = "TTS_CLEAN_INSTALL_HELPER_MODE"
	deterministicHelperReadyEnvironment       = "TTS_CLEAN_INSTALL_HELPER_READY_PATH"
	deterministicHelperDescendantEnvironment  = "TTS_CLEAN_INSTALL_HELPER_DESCENDANT_READY_PATH"
	deterministicHelperOutputEnvironment      = "TTS_CLEAN_INSTALL_HELPER_OUTPUT_ROOT"
	deterministicHelperArtifactEnvironment    = "TTS_CLEAN_INSTALL_HELPER_ARTIFACT_PATH"
	deterministicHelperOutputBytesEnvironment = "TTS_CLEAN_INSTALL_HELPER_OUTPUT_BYTES"
	deterministicHelperOfflineEnvironment     = "TTS_CLEAN_INSTALL_HELPER_OFFLINE"
	deterministicHelperFailPhaseEnvironment   = "TTS_CLEAN_INSTALL_HELPER_FAIL_PHASE"
	helperProcessWaitCeiling                  = 15 * time.Second
	helperOutputBytes                         = 256 << 10
	forbiddenListenerPort                     = 7437
)

var errHelperEventsClosed = errors.New("deterministic helper exited before readiness")

type helperEvent struct {
	Event           string   `json:"event"`
	Phase           string   `json:"phase"`
	Status          string   `json:"status"`
	Evidence        []string `json:"evidence"`
	Argv            []string `json:"argv"`
	PID             int      `json:"pid"`
	DescendantPID   int      `json:"descendantPid"`
	Address         string   `json:"address"`
	Port            int      `json:"port"`
	ReadinessState  string   `json:"readinessState"`
	LifecycleState  string   `json:"lifecycleState"`
	CacheBytes      int64    `json:"cacheBytes"`
	CacheReused     bool     `json:"cacheReused"`
	NetworkAttempts int      `json:"networkAttempts"`
	AudioName       string   `json:"audioName"`
	AudioPath       string   `json:"audioPath"`

	lineBytes  int64
	lineSHA256 string
}

type helperCapture struct {
	body       bytes.Buffer
	streamHash hashWriter
	line       []byte
	events     chan helperEvent
}

// hashWriter is the small subset of hash.Hash needed by helperCapture. It is
// kept as a local interface so the capture has no dependency on a concrete
// hash implementation after construction.
type hashWriter interface {
	Write([]byte) (int, error)
	Sum([]byte) []byte
}

func newHelperCapture(events chan helperEvent) *helperCapture {
	return &helperCapture{streamHash: sha256.New(), events: events}
}

// Write retains controlled helper output so tests can compare exact bytes and
// hashes. The largest emitted stream is deliberately bounded by the test
// case, not by an ambient process output limit.
func (capture *helperCapture) Write(chunk []byte) (int, error) {
	if _, err := capture.body.Write(chunk); err != nil {
		return 0, err
	}
	if _, err := capture.streamHash.Write(chunk); err != nil {
		return 0, err
	}
	capture.line = append(capture.line, chunk...)
	for {
		index := bytes.IndexByte(capture.line, '\n')
		if index < 0 {
			break
		}
		rawLine := append([]byte(nil), capture.line[:index+1]...)
		line := bytes.TrimSuffix(rawLine[:len(rawLine)-1], []byte{'\r'})
		capture.line = append([]byte(nil), capture.line[index+1:]...)
		var event helperEvent
		if json.Unmarshal(line, &event) != nil || event.Event == "" {
			continue
		}
		event.lineBytes = int64(len(rawLine))
		event.lineSHA256 = sha256Hex(rawLine)
		if capture.events != nil {
			capture.events <- event
		}
	}
	return len(chunk), nil
}

func (capture *helperCapture) Bytes() []byte {
	return append([]byte(nil), capture.body.Bytes()...)
}

func (capture *helperCapture) SHA256() string {
	return hex.EncodeToString(capture.streamHash.Sum(nil))
}

type helperResult struct {
	PID            int
	ExitCode       int
	Stdout         []byte
	Stderr         []byte
	StdoutBytes    int64
	StderrBytes    int64
	StdoutSHA256   string
	StderrSHA256   string
	StartedAt      time.Time
	EndedAt        time.Time
	TimedOut       bool
	Cancelled      bool
	Events         []helperEvent
	OwnedSurvivors int
}

type helperRun struct {
	command       *exec.Cmd
	tree          processTree
	treeAttached  bool
	stdout        *helperCapture
	stderr        *helperCapture
	events        chan helperEvent
	waitCh        chan error
	streamDone    chan struct{}
	stdoutDone    chan struct{}
	stderrDone    chan struct{}
	startedAt     time.Time
	owned         bool
	stopRequested bool
	waited        bool
	mu            sync.Mutex
}

func startHelper(plan SealedPlan, mode string, ownedTree bool, arguments ...string) (*helperRun, error) {
	helperPath, err := verifiedHelperPath()
	if err != nil {
		return nil, err
	}
	if _, err := absoluteClean(plan.Isolation.OutputRoot); err != nil {
		return nil, fmt.Errorf("helper working root: %w", err)
	}
	if info, statErr := os.Stat(plan.Isolation.OutputRoot); statErr != nil || !info.IsDir() {
		if statErr != nil {
			return nil, fmt.Errorf("helper working root: %w", statErr)
		}
		return nil, errors.New("helper working root is not a directory")
	}

	command := exec.Command(helperPath)
	command.Dir = plan.Isolation.OutputRoot
	command.Env = helperInvocationEnvironment(plan, mode, arguments...)
	configureProcessCommand(command)

	events := make(chan helperEvent, 64)
	stdout := newHelperCapture(events)
	stderr := newHelperCapture(nil)
	stdoutPipe, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("helper stdout pipe: %w", err)
	}
	stderrPipe, err := command.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("helper stderr pipe: %w", err)
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("helper stdin pipe: %w", err)
	}

	var tree processTree
	if ownedTree {
		tree, err = newProcessTree()
		if err != nil {
			return nil, fmt.Errorf("create helper process tree: %w", err)
		}
	}
	if err := command.Start(); err != nil {
		tree.close()
		return nil, fmt.Errorf("start deterministic helper: %w", err)
	}
	run := &helperRun{
		command: command, tree: tree, stdout: stdout, stderr: stderr,
		events: events, streamDone: make(chan struct{}), startedAt: time.Now().UTC(), owned: ownedTree,
	}
	if ownedTree {
		if err := run.tree.attach(command.Process); err != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
			tree.close()
			return nil, fmt.Errorf("attach helper process tree: %w", err)
		}
		run.treeAttached = true
	}
	// The compiled helper blocks on stdin before it can create a descendant,
	// bind a listener, or write journey state. Release that gate only after the
	// root is attached to its Job Object; this closes the command.Start to
	// AssignProcessToJobObject child-creation race.
	if _, err := stdin.Write([]byte{0}); err != nil {
		_ = run.Stop()
		_ = command.Wait()
		tree.close()
		return nil, fmt.Errorf("release helper start gate: %w", err)
	}
	if err := stdin.Close(); err != nil {
		_ = run.Stop()
		_ = command.Wait()
		tree.close()
		return nil, fmt.Errorf("close helper start gate: %w", err)
	}

	run.stdoutDone = make(chan struct{})
	run.stderrDone = make(chan struct{})
	go drainHelperStream(stdoutPipe, stdout, run.stdoutDone)
	go drainHelperStream(stderrPipe, stderr, run.stderrDone)
	run.waitCh = make(chan error, 1)
	go func() { run.waitCh <- command.Wait() }()
	go func() {
		// Both io.Copy calls have completed before this channel closes, so a
		// caller may safely inspect exact output after helper termination.
		<-run.stdoutDone
		<-run.stderrDone
		close(events)
		close(run.streamDone)
	}()
	return run, nil
}

func drainHelperStream(reader io.Reader, capture *helperCapture, done chan<- struct{}) {
	_, _ = io.Copy(capture, reader)
	if done != nil {
		close(done)
	}
}

func (run *helperRun) WaitReady(ctx context.Context, eventName string) (helperEvent, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		select {
		case event, ok := <-run.events:
			if !ok {
				return helperEvent{}, errHelperEventsClosed
			}
			if event.Event == eventName {
				return event, nil
			}
		case <-ctx.Done():
			return helperEvent{}, ctx.Err()
		}
	}
}

func (run *helperRun) Stop() error {
	run.mu.Lock()
	defer run.mu.Unlock()
	if run.stopRequested {
		return nil
	}
	run.stopRequested = true
	if !run.owned {
		if run.command.Process == nil {
			return nil
		}
		if err := run.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
		return nil
	}
	return run.tree.terminate(run.command.Process)
}

func (run *helperRun) Wait(ctx context.Context) (helperResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	run.mu.Lock()
	if run.waited {
		run.mu.Unlock()
		return helperResult{}, errors.New("helper was already waited")
	}
	run.mu.Unlock()

	result := helperResult{PID: run.command.Process.Pid, ExitCode: -1, StartedAt: run.startedAt}
	var terminationErr error
	select {
	case waitErr := <-run.waitCh:
		result.ExitCode = helperExitCode(waitErr)
	case <-ctx.Done():
		result.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		result.Cancelled = errors.Is(ctx.Err(), context.Canceled)
		terminationErr = run.Stop()
		select {
		case waitErr := <-run.waitCh:
			result.ExitCode = helperExitCode(waitErr)
		case <-time.After(helperProcessWaitCeiling):
			terminationErr = errors.Join(terminationErr, errors.New("helper did not exit after termination"))
		}
	}

	select {
	case <-run.streamDone:
	case <-time.After(helperProcessWaitCeiling):
		terminationErr = errors.Join(terminationErr, errors.New("helper output streams did not drain"))
	}
	if run.owned && run.treeAttached {
		if err := run.tree.terminate(run.command.Process); err != nil {
			terminationErr = errors.Join(terminationErr, err)
		}
		if run.tree.active() {
			result.OwnedSurvivors = 1
		}
		run.tree.close()
	}
	result.Stdout = run.stdout.Bytes()
	result.Stderr = run.stderr.Bytes()
	result.StdoutBytes = int64(len(result.Stdout))
	result.StderrBytes = int64(len(result.Stderr))
	result.StdoutSHA256 = run.stdout.SHA256()
	result.StderrSHA256 = run.stderr.SHA256()
	result.Events = drainHelperEvents(run.events)
	result.EndedAt = time.Now().UTC()
	run.mu.Lock()
	run.waited = true
	run.mu.Unlock()
	return result, terminationErr
}

func helperExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func drainHelperEvents(events <-chan helperEvent) []helperEvent {
	result := make([]helperEvent, 0)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return result
			}
			result = append(result, event)
		default:
			return result
		}
	}
}

func verifiedHelperPath() (string, error) {
	path := strings.TrimSpace(os.Getenv(deterministicHelperPathEnvironment))
	if path == "" {
		return "", fmt.Errorf("%s is required for compiled helper evidence", deterministicHelperPathEnvironment)
	}
	absolute, err := absoluteClean(path)
	if err != nil {
		return "", fmt.Errorf("compiled helper path: %w", err)
	}
	if err := rejectReparsePath(absolute); err != nil {
		return "", fmt.Errorf("compiled helper path: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", fmt.Errorf("compiled helper: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", errors.New("compiled helper is not a regular non-reparse file")
	}
	want := strings.TrimSpace(os.Getenv(deterministicHelperHashEnvironment))
	if !sha256Pattern.MatchString(want) {
		return "", fmt.Errorf("%s must be lowercase SHA-256", deterministicHelperHashEnvironment)
	}
	body, err := os.ReadFile(absolute)
	if err != nil {
		return "", fmt.Errorf("read compiled helper: %w", err)
	}
	if got := sha256Hex(body); got != want {
		return "", fmt.Errorf("%s SHA-256=%s, want %s", deterministicHelperIdentity, got, want)
	}
	return absolute, nil
}

func helperInvocationEnvironment(plan SealedPlan, mode string, arguments ...string) []string {
	environment := helperEnvironment(plan)
	environment = setEnvironmentValue(environment, deterministicHelperModeEnvironment, mode)
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		name := ""
		value := ""
		switch argument {
		case "-ReadyPath":
			name = deterministicHelperReadyEnvironment
		case "-DescendantReadyPath":
			name = deterministicHelperDescendantEnvironment
		case "-OutputRoot":
			name = deterministicHelperOutputEnvironment
		case "-ArtifactPath":
			name = deterministicHelperArtifactEnvironment
		case "-OutputBytes":
			name = deterministicHelperOutputBytesEnvironment
		case "-FailPhase":
			name = deterministicHelperFailPhaseEnvironment
		case "-Offline":
			environment = setEnvironmentValue(environment, deterministicHelperOfflineEnvironment, "1")
			continue
		default:
			continue
		}
		if index+1 < len(arguments) {
			index++
			value = arguments[index]
		}
		environment = setEnvironmentValue(environment, name, value)
	}
	return environment
}

func setEnvironmentValue(environment []string, name, value string) []string {
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		key, _, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, name) {
			continue
		}
		result = append(result, entry)
	}
	return append(result, name+"="+value)
}

func helperEnvironment(plan SealedPlan) []string {
	allowed := map[string]bool{
		"comspec": true, "path": true, "pathext": true, "psmodulepath": true,
		"programfiles": true, "programfiles(x86)": true, "programw6432": true,
		"systemdrive": true, "systemroot": true, "windir": true,
	}
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok && allowed[strings.ToLower(key)] {
			values[strings.ToLower(key)] = key + "=" + value
		}
	}
	for key, value := range plan.Isolation.Environment {
		values[strings.ToLower(key)] = key + "=" + value
	}
	values["no_proxy"] = "NO_PROXY=*"
	result := make([]string, 0, len(values))
	for _, entry := range values {
		result = append(result, entry)
	}
	sort.Strings(result)
	return result
}

func expectedBurstBytes(size int) []byte {
	result := make([]byte, size)
	for index := range result {
		result[index] = byte((index*31 + 7) % 251)
	}
	return result
}

func startOwnedLoopbackListener(plan SealedPlan) (*helperRun, error) {
	return startHelper(plan, "listener", true, "-ReadyPath", filepath.Join(plan.Isolation.RuntimeRoot, "listener.ready"))
}

func dialAddress(address string) error {
	connection, err := net.DialTimeout("tcp4", address, 500*time.Millisecond)
	if err != nil {
		return err
	}
	return connection.Close()
}

func listenerUnavailable(address string) bool {
	return dialAddress(address) != nil
}

func cleanupOwnedRoots(plan SealedPlan) (CleanupEvidence, error) {
	paths := []string{
		plan.Isolation.WorkRoot, plan.Isolation.ProfileRoot, plan.Isolation.StateRoot,
		plan.Isolation.CacheRoot, plan.Isolation.TempRoot, plan.Isolation.StreamsRoot,
		plan.Isolation.RuntimeRoot,
	}
	removed := 0
	for _, path := range paths {
		if !pathWithin(plan.Isolation.OutputRoot, path) || samePath(path, plan.Isolation.OutputRoot) {
			return CleanupEvidence{}, fmt.Errorf("owned root escaped output root: %s", path)
		}
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return CleanupEvidence{}, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return CleanupEvidence{}, fmt.Errorf("owned root is not a directory: %s", path)
		}
		if err := os.RemoveAll(path); err != nil {
			return CleanupEvidence{}, fmt.Errorf("remove owned root %s: %w", path, err)
		}
		removed++
	}
	if entries, err := os.ReadDir(plan.Isolation.OutputRoot); err != nil {
		return CleanupEvidence{}, err
	} else if len(entries) != 0 {
		retained := make([]string, 0, len(entries))
		for _, entry := range entries {
			retained = append(retained, entry.Name())
		}
		return CleanupEvidence{}, fmt.Errorf("owned output root retained %d entries: %s", len(entries), strings.Join(retained, ", "))
	}
	return CleanupEvidence{OwnedRoots: removed, RemovedRuntimeRoots: removed}, nil
}
