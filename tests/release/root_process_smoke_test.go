package release_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

const (
	compiledServiceReadinessTimeout = 15 * time.Second
	compiledServiceJoinTimeout      = 10 * time.Second
	compiledServiceCleanupTimeout   = 10 * time.Second
	compiledServiceOutputLimit      = 8192
	compiledProcessSnapshotLimit    = 1024 * 1024
)

// TestRootProcessCompiledBinaryModeMatrix proves the process-root migration at
// the installed-binary boundary instead of only through Cobra or root fakes.
func TestRootProcessCompiledBinaryModeMatrix(t *testing.T) {
	artifact := requireReleasePrebuiltArtifact(t)
	binaryPath := artifact.Path
	home := t.TempDir()
	environment := builtcliacceptance.ProcessEnvForIsolatedHome(home)

	t.Run("help succeeds", func(t *testing.T) {
		output, err := runBoundedBinary(t, binaryPath, environment, "--help")
		if err != nil || !strings.Contains(output, "Usage:") {
			t.Fatalf("you --help = (%v, %q), want successful usage output", err, output)
		}
	})

	t.Run("non-startup docs command succeeds", func(t *testing.T) {
		output, err := runBoundedBinary(t, binaryPath, environment, "docs", "config")
		if err != nil || !strings.Contains(output, "# Config") {
			t.Fatalf("you docs config = (%v, %q), want packaged docs", err, output)
		}
	})

	t.Run("invalid command fails once", func(t *testing.T) {
		output, err := runBoundedBinary(t, binaryPath, environment, "not-a-command")
		if err == nil || strings.Count(output, `unknown command "not-a-command"`) != 1 {
			t.Fatalf("invalid command = (%v, %q), want one diagnostic and non-zero exit", err, output)
		}
	})

	factoryDir := testutil.MustRepoPath(t, "tests/release/testdata/cli_smoke_factory")
	t.Run("explicit local run succeeds", func(t *testing.T) {
		serverURL := reserveRootProcessSmokeURL(t)
		output, err := runBoundedBinary(
			t, binaryPath, environment,
			"run", "--dir", factoryDir, "--server", serverURL,
			"--with-mock-workers", "--quiet", "--no-record",
		)
		if err != nil {
			t.Fatalf("local run failed: %v\n%s", err, output)
		}
	})

	t.Run("bare invocation prints help and exits successfully", func(t *testing.T) {
		output, err := runBoundedBinary(t, binaryPath, environment)
		if err != nil || !strings.Contains(output, "Usage:") {
			t.Fatalf("bare you = (%v, %q), want successful usage output", err, output)
		}
	})

	t.Run("bare invocation ignores malformed operator config", func(t *testing.T) {
		malformedHome := t.TempDir()
		configDirectory := filepath.Join(malformedHome, ".you-agent-factory")
		if err := os.MkdirAll(configDirectory, 0o700); err != nil {
			t.Fatalf("create malformed operator config directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(configDirectory, "config.json"), []byte("{not-json"), 0o600); err != nil {
			t.Fatalf("write malformed operator config: %v", err)
		}
		malformedEnvironment := builtcliacceptance.ProcessEnvForIsolatedHome(malformedHome)
		output, err := runBoundedBinary(t, binaryPath, malformedEnvironment)
		if err != nil || !strings.Contains(output, "Usage:") {
			t.Fatalf("bare you with malformed operator config = (%v, %q), want successful usage output", err, output)
		}
	})

	t.Run("API service serves until cancellation", func(t *testing.T) {
		serverURL := reserveRootProcessSmokeURL(t)
		serverAddress := strings.TrimPrefix(serverURL, "http://")
		assertCompiledServiceMode(t, binaryPath, factoryDir, serverURL, []string{
			"run", "--dir", factoryDir, "--continuously", "--with-mock-workers",
			"--quiet", "--no-record", "--with-server", "--listen", serverAddress,
		})
	})

	t.Run("MCP server processes stdio and exits", func(t *testing.T) {
		fixturePath := testutil.MustRepoPath(t, "pkg/transports/http/testdata/durable-session-contract-fixtures.json")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binaryPath, "server", "mcp", "--fixture-catalog", fixturePath)
		cmd.Env = environment
		cmd.Stdin = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"root-smoke","version":"test"}}}` + "\n")
		output, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(output), `"protocolVersion":"2024-11-05"`) {
			t.Fatalf("MCP server = (%v, %q), want initialize response and clean EOF", err, string(output))
		}
	})
}

func runBoundedBinary(t *testing.T, binaryPath string, environment []string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binaryPath, args...)
	cmd.Env = environment
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("you %s exceeded 10 second bound", strings.Join(args, " "))
	}
	return string(output), err
}

func reserveRootProcessSmokeURL(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve smoke port: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release smoke port: %v", err)
	}
	return "http://" + address
}

type compiledServiceScenario struct {
	homeDir    string
	profileDir string
	workDir    string
	stateDir   string
	factoryDir string
	tmpDir     string
}

func assertCompiledServiceMode(t *testing.T, binaryPath string, fixtureDir string, serverURL string, args []string) {
	t.Helper()
	scenario := newCompiledServiceScenario(t, fixtureDir)
	parsedURL, err := url.Parse(serverURL)
	if err != nil || parsedURL.Host == "" {
		t.Fatalf("parse service URL %q: %v", serverURL, err)
	}

	serviceArgs := replaceCompiledServiceArgument(args, fixtureDir, scenario.factoryDir)
	serviceArgs = append(serviceArgs,
		"--runtime-log-dir", filepath.Join(scenario.stateDir, "logs"),
		"--runtime-metrics-dir", filepath.Join(scenario.stateDir, "metrics"),
	)
	service, err := startCompiledServiceProcess(t, binaryPath, scenario, serviceArgs)
	if err != nil {
		t.Fatalf("start service mode: %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := service.stopAndJoin(); cleanupErr != nil {
			t.Errorf("service cleanup: %v", cleanupErr)
		}
	})

	readinessCtx, cancelReadiness := context.WithTimeout(t.Context(), compiledServiceReadinessTimeout)
	defer cancelReadiness()
	readyAt, readinessErr := waitForCompiledServiceReadiness(readinessCtx, service, serverURL)
	observedPIDs, processObservationErr := observeCompiledServiceTree(t.Context(), service.rootPID)
	cleanupErr := service.stopAndJoin()
	listenerErr := observeCompiledServiceListenerStopped(t.Context(), parsedURL.Host)
	processGoneErr := observeCompiledServiceProcessesGone(t.Context(), service.rootPID, observedPIDs)

	if readinessErr != nil || processObservationErr != nil || cleanupErr != nil || listenerErr != nil || processGoneErr != nil {
		t.Fatalf(
			"compiled service mode failed: readiness=%v process_observation=%v cleanup=%v listener=%v processes=%v\n%s",
			readinessErr,
			processObservationErr,
			cleanupErr,
			listenerErr,
			processGoneErr,
			service.diagnostic("readiness", serverURL, readinessErr),
		)
	}
	t.Logf(
		"compiled service ready at %s (ready_at=%s, elapsed=%v); root_pid=%d observed_descendants=%v listener_closed=true process_tree_joined=true state_root=%s",
		serverURL,
		readyAt.Format(time.RFC3339Nano),
		readyAt.Sub(service.startedAt).Round(time.Millisecond),
		service.rootPID,
		observedPIDs,
		scenario.stateDir,
	)
}

func newCompiledServiceScenario(t *testing.T, fixtureDir string) compiledServiceScenario {
	t.Helper()
	rootDir := t.TempDir()
	scenario := compiledServiceScenario{
		homeDir:    filepath.Join(rootDir, "home"),
		profileDir: filepath.Join(rootDir, "profile"),
		workDir:    filepath.Join(rootDir, "work"),
		stateDir:   filepath.Join(rootDir, "state"),
		tmpDir:     filepath.Join(rootDir, "tmp"),
	}
	scenario.factoryDir = filepath.Join(scenario.workDir, "factory")
	for _, directory := range []string{
		scenario.homeDir,
		scenario.profileDir,
		scenario.workDir,
		scenario.stateDir,
		scenario.tmpDir,
	} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatalf("create isolated service directory %q: %v", directory, err)
		}
	}
	copyReleaseFixture(t, fixtureDir, scenario.factoryDir)
	return scenario
}

func compiledServiceEnvironment(scenario compiledServiceScenario) []string {
	removed := map[string]struct{}{
		"HOME": {}, "USERPROFILE": {}, "HOMEDRIVE": {}, "HOMEPATH": {},
		"APPDATA": {}, "LOCALAPPDATA": {}, "XDG_CONFIG_HOME": {}, "XDG_CACHE_HOME": {},
		"TMP": {}, "TEMP": {},
	}
	environment := make([]string, 0, len(os.Environ())+12)
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, skip := removed[strings.ToUpper(key)]; skip {
			continue
		}
		if strings.EqualFold(key, "YOU_NO_BROWSER_OPEN") {
			continue
		}
		environment = append(environment, entry)
	}
	profileVolume := filepath.VolumeName(scenario.profileDir)
	profilePath := strings.TrimPrefix(scenario.profileDir, profileVolume)
	if profilePath == "" {
		profilePath = string(os.PathSeparator)
	}
	environment = append(environment,
		"HOME="+scenario.homeDir,
		"USERPROFILE="+scenario.profileDir,
		"HOMEDRIVE="+profileVolume,
		"HOMEPATH="+profilePath,
		"APPDATA="+filepath.Join(scenario.profileDir, "AppData", "Roaming"),
		"LOCALAPPDATA="+filepath.Join(scenario.profileDir, "AppData", "Local"),
		"XDG_CONFIG_HOME="+filepath.Join(scenario.homeDir, ".config"),
		"XDG_CACHE_HOME="+filepath.Join(scenario.homeDir, ".cache"),
		"TMP="+scenario.tmpDir,
		"TEMP="+scenario.tmpDir,
		"YOU_NO_BROWSER_OPEN=1",
	)
	return environment
}

func replaceCompiledServiceArgument(args []string, from string, to string) []string {
	replaced := append([]string(nil), args...)
	for index, arg := range replaced {
		if arg == from {
			replaced[index] = to
		}
	}
	return replaced
}

func copyReleaseFixture(t *testing.T, sourceDir string, destinationDir string) {
	t.Helper()
	err := filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		destination := destinationDir
		if relative != "." {
			destination = filepath.Join(destinationDir, relative)
		}
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, contents, info.Mode().Perm())
	})
	if err != nil {
		t.Fatalf("copy release fixture %q to %q: %v", sourceDir, destinationDir, err)
	}
}

type compiledServiceProcess struct {
	command   *exec.Cmd
	scenario  compiledServiceScenario
	tree      platformprocess.SubprocessTree
	stdout    *compiledBoundedBuffer
	stderr    *compiledBoundedBuffer
	waitDone  chan struct{}
	startedAt time.Time
	rootPID   int

	mu          sync.Mutex
	waitErr     error
	cleanupOnce sync.Once
	cleanupErr  error
}

func startCompiledServiceProcess(
	t *testing.T,
	binaryPath string,
	scenario compiledServiceScenario,
	args []string,
) (*compiledServiceProcess, error) {
	t.Helper()
	stdout := &compiledBoundedBuffer{limit: compiledServiceOutputLimit}
	stderr := &compiledBoundedBuffer{limit: compiledServiceOutputLimit}
	command := exec.Command(binaryPath, args...)
	command.Dir = scenario.workDir
	command.Env = compiledServiceEnvironment(scenario)
	command.Stdout = stdout
	command.Stderr = stderr
	command.WaitDelay = compiledServiceJoinTimeout
	platformprocess.ConfigureSubprocessTree(command)
	if err := command.Start(); err != nil {
		return nil, err
	}
	tree, err := platformprocess.AttachSubprocessTree(command)
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, fmt.Errorf("attach process-tree supervisor: %w", err)
	}
	service := &compiledServiceProcess{
		command:   command,
		scenario:  scenario,
		tree:      tree,
		stdout:    stdout,
		stderr:    stderr,
		waitDone:  make(chan struct{}),
		startedAt: time.Now(),
		rootPID:   command.Process.Pid,
	}
	go func() {
		err := command.Wait()
		service.mu.Lock()
		service.waitErr = err
		service.mu.Unlock()
		close(service.waitDone)
	}()
	return service, nil
}

func (service *compiledServiceProcess) exited() (bool, error) {
	select {
	case <-service.waitDone:
		service.mu.Lock()
		defer service.mu.Unlock()
		return true, service.waitErr
	default:
		return false, nil
	}
}

func (service *compiledServiceProcess) stopAndJoin() error {
	service.cleanupOnce.Do(func() {
		if err := platformprocess.TerminateSubprocessTree(service.command, service.tree); err != nil {
			service.cleanupErr = fmt.Errorf("terminate process tree: %w", err)
		}
		joinTimer := time.NewTimer(compiledServiceJoinTimeout)
		defer joinTimer.Stop()
		select {
		case <-service.waitDone:
		case <-joinTimer.C:
			if service.cleanupErr == nil {
				service.cleanupErr = fmt.Errorf("process %d did not join within %s", service.rootPID, compiledServiceJoinTimeout)
			}
		}
		platformprocess.CloseSubprocessTree(service.command, service.tree)
	})
	return service.cleanupErr
}

func (service *compiledServiceProcess) diagnostic(phase string, serverURL string, cause error) string {
	return redactCompiledServiceDiagnostic(
		fmt.Sprintf(
			"phase=%s url=%s cause=%v root_pid=%d stdout=%q stderr=%q",
			phase,
			serverURL,
			cause,
			service.rootPID,
			service.stdout.Tail(),
			service.stderr.Tail(),
		),
		serverURL,
		service.command.Dir,
		service.command.Path,
		service.scenario.homeDir,
		service.scenario.profileDir,
		service.scenario.workDir,
		service.scenario.stateDir,
		service.scenario.factoryDir,
		service.scenario.tmpDir,
	)
}

func waitForCompiledServiceReadiness(
	ctx context.Context,
	service *compiledServiceProcess,
	serverURL string,
) (time.Time, error) {
	client := &http.Client{Timeout: time.Second}
	lastStatus := 0
	var lastErr error
	probeTimer := time.NewTimer(0)
	defer probeTimer.Stop()
	for {
		select {
		case <-ctx.Done():
			return time.Time{}, fmt.Errorf("phase=readiness safety ceiling: last_status=%d last_error=%v: %w", lastStatus, lastErr, ctx.Err())
		case <-probeTimer.C:
		}

		request, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/status", nil)
		if err != nil {
			return time.Time{}, fmt.Errorf("phase=readiness request: %w", err)
		}
		response, err := client.Do(request)
		if err == nil {
			lastStatus = response.StatusCode
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return time.Now(), nil
			}
		} else {
			lastErr = err
		}

		if exited, exitErr := service.exited(); exited {
			return time.Time{}, fmt.Errorf("phase=early-exit exit=%v last_status=%d last_error=%v", exitErr, lastStatus, lastErr)
		}
		// This timer only bounds the loopback probe rate. Correctness is driven
		// by the /status response, early process exit, or the safety context.
		probeTimer.Reset(50 * time.Millisecond)
	}
}

func observeCompiledServiceListenerStopped(ctx context.Context, address string) error {
	observer := platformhttpserver.NewListenerStopObserver(
		(&net.Dialer{}).DialContext,
		platformhttpserver.DefaultListenerStopObservationInterval,
	)
	if err := observer.Wait(ctx, address, compiledServiceCleanupTimeout); err != nil {
		return fmt.Errorf("listener at %s remained reachable: %w", address, err)
	}
	return nil
}

type compiledProcessRecord struct {
	PID       int `json:"ProcessId"`
	ParentPID int `json:"ParentProcessId"`
}

func observeCompiledServiceTree(ctx context.Context, rootPID int) ([]int, error) {
	records, err := compiledProcessSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	children := make(map[int][]int)
	seenRoot := false
	for _, record := range records {
		if record.PID == rootPID {
			seenRoot = true
		}
		if record.PID > 0 && record.ParentPID > 0 {
			children[record.ParentPID] = append(children[record.ParentPID], record.PID)
		}
	}
	if !seenRoot {
		return nil, fmt.Errorf("process snapshot did not contain running root pid %d", rootPID)
	}
	seen := map[int]struct{}{rootPID: {}}
	queue := []int{rootPID}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		for _, child := range children[parent] {
			if _, ok := seen[child]; ok {
				continue
			}
			seen[child] = struct{}{}
			queue = append(queue, child)
		}
	}
	observed := make([]int, 0, len(seen)-1)
	for pid := range seen {
		if pid != rootPID {
			observed = append(observed, pid)
		}
	}
	sort.Ints(observed)
	return observed, nil
}

func observeCompiledServiceProcessesGone(ctx context.Context, rootPID int, observedPIDs []int) error {
	watched := append([]int{rootPID}, observedPIDs...)
	deadlineCtx, cancel := context.WithTimeout(ctx, compiledServiceCleanupTimeout)
	defer cancel()
	probeTimer := time.NewTimer(0)
	defer probeTimer.Stop()
	for {
		select {
		case <-deadlineCtx.Done():
			return fmt.Errorf("processes still present after cleanup: %v: %w", watched, deadlineCtx.Err())
		case <-probeTimer.C:
		}
		records, err := compiledProcessSnapshot(deadlineCtx)
		if err != nil {
			return err
		}
		active := make(map[int]struct{}, len(records))
		for _, record := range records {
			active[record.PID] = struct{}{}
		}
		remaining := make([]int, 0)
		for _, pid := range watched {
			if _, ok := active[pid]; ok {
				remaining = append(remaining, pid)
			}
		}
		if len(remaining) == 0 {
			return nil
		}
		probeTimer.Reset(50 * time.Millisecond)
	}
}

func compiledProcessSnapshot(ctx context.Context) ([]compiledProcessRecord, error) {
	snapshotCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	stdout := &compiledBoundedBuffer{limit: compiledProcessSnapshotLimit}
	stderr := &compiledBoundedBuffer{limit: compiledServiceOutputLimit}
	var command *exec.Cmd
	if runtime.GOOS == "windows" {
		command = exec.CommandContext(
			snapshotCtx,
			"powershell.exe",
			"-NoProfile",
			"-NonInteractive",
			"-Command",
			"Get-CimInstance Win32_Process | Select-Object ProcessId,ParentProcessId | ConvertTo-Json -Compress",
		)
	} else {
		command = exec.CommandContext(snapshotCtx, "ps", "-eo", "pid=,ppid=")
	}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("process snapshot command: %w (stderr=%q)", err, stderr.Tail())
	}
	if runtime.GOOS == "windows" {
		return parseWindowsCompiledProcessSnapshot(stdout.Value())
	}
	return parseUnixCompiledProcessSnapshot(stdout.Value())
}

func parseWindowsCompiledProcessSnapshot(output string) ([]compiledProcessRecord, error) {
	var records []compiledProcessRecord
	if err := json.Unmarshal([]byte(output), &records); err == nil {
		return records, nil
	}
	var record compiledProcessRecord
	if err := json.Unmarshal([]byte(output), &record); err != nil {
		return nil, fmt.Errorf("parse Windows process snapshot: %w", err)
	}
	return []compiledProcessRecord{record}, nil
}

func parseUnixCompiledProcessSnapshot(output string) ([]compiledProcessRecord, error) {
	var records []compiledProcessRecord
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, fmt.Errorf("parse process pid %q: %w", fields[0], err)
		}
		parentPID, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("parse process parent pid %q: %w", fields[1], err)
		}
		records = append(records, compiledProcessRecord{PID: pid, ParentPID: parentPID})
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("process snapshot was empty")
	}
	return records, nil
}

type compiledBoundedBuffer struct {
	mu        sync.Mutex
	data      []byte
	limit     int
	truncated bool
}

func (buffer *compiledBoundedBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	buffer.data = append(buffer.data, data...)
	if len(buffer.data) > buffer.limit {
		buffer.data = append([]byte(nil), buffer.data[len(buffer.data)-buffer.limit:]...)
		buffer.truncated = true
	}
	return len(data), nil
}

func (buffer *compiledBoundedBuffer) Tail() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	value := string(buffer.data)
	if buffer.truncated {
		value = "...[truncated]" + value
	}
	return value
}

func (buffer *compiledBoundedBuffer) Value() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return string(buffer.data)
}

func redactCompiledServiceDiagnostic(value string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "<redacted>")
		}
	}
	return value
}
