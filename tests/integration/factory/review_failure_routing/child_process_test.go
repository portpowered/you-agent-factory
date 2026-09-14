package review_failure_routing

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
)

const (
	childReadyTimeout     = 45 * time.Second
	childOperationTimeout = 30 * time.Second
	childShutdownTimeout  = 20 * time.Second
	childQuiescenceTicks  = 8
	childPollInterval     = 25 * time.Millisecond
)

type routingChild struct {
	cmd        *exec.Cmd
	baseURL    string
	sessionID  string
	recordPath string
	artifact   prebuiltArtifact
	stdout     synchronizedBuffer
	stderr     synchronizedBuffer
	done       chan struct{}

	mu        sync.Mutex
	waitErr   error
	stopped   bool
	cleanExit bool
}

func startRoutingChild(
	t *testing.T,
	artifact prebuiltArtifact,
	factoryDir, homeDir, mockWorkersPath, recordPath string,
) *routingChild {
	t.Helper()
	address := reserveRoutingAddress(t)
	command := exec.CommandContext(t.Context(), artifact.path,
		"run", "--dir", factoryDir,
		"--continuously", "--with-server",
		"--listen", address,
		"--with-mock-workers", mockWorkersPath,
		"--record", recordPath,
	)
	command.Dir = factoryDir
	command.Env = builtcliacceptance.ProcessEnvForIsolatedHome(homeDir)
	configureRoutingCommand(command)

	child := &routingChild{
		cmd:        command,
		baseURL:    "http://" + address,
		recordPath: recordPath,
		artifact:   artifact,
		done:       make(chan struct{}),
	}
	// os/exec writes stdout and stderr from separate goroutines when they are
	// connected directly to writers. The race gate also permits diagnostics
	// while the child is still running, so the buffers synchronize writes and
	// reads rather than relying on Wait having joined the child.
	child.cmd.Stdout = &child.stdout
	child.cmd.Stderr = &child.stderr
	if err := child.cmd.Start(); err != nil {
		t.Fatalf("start externally prebuilt Factory child: %v\n%s", err, child.evidence())
	}
	go func() {
		err := child.cmd.Wait()
		child.mu.Lock()
		child.waitErr = err
		child.mu.Unlock()
		close(child.done)
	}()
	t.Cleanup(func() { child.cleanup(t) })
	return child
}

type synchronizedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (buffer *synchronizedBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.Write(data)
}

func (buffer *synchronizedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.String()
}

func reserveRoutingAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve isolated routing loopback address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release isolated routing loopback address: %v", err)
	}
	return address
}

func (child *routingChild) cleanup(t *testing.T) {
	t.Helper()
	if child == nil {
		return
	}
	child.mu.Lock()
	if child.stopped {
		child.mu.Unlock()
		return
	}
	child.stopped = true
	child.mu.Unlock()

	select {
	case <-child.done:
		return
	default:
	}
	// The child owns a server and may have descendants holding its output
	// handles. Give the CLI its normal console shutdown signal first so it can
	// unwind that tree before the test falls back to a hard kill.
	if child.cmd.Process != nil {
		_ = interruptRoutingProcess(child.cmd)
	}
	select {
	case <-child.done:
		return
	case <-time.After(2 * time.Second):
	}
	if child.cmd.Process != nil {
		if err := child.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			if exited, waitErr := child.exited(); !exited {
				t.Errorf("cleanup kill of Factory child: %v", err)
			} else if !routingCleanExit(waitErr) {
				t.Errorf("Factory child exited during cleanup: %v", waitErr)
			}
		}
	}
	select {
	case <-child.done:
	case <-time.After(childShutdownTimeout):
		t.Errorf("Factory child cleanup did not complete within %s\n%s", childShutdownTimeout, child.evidence())
	}
}

func (child *routingChild) stop(t *testing.T) {
	t.Helper()
	child.mu.Lock()
	if child.stopped {
		child.mu.Unlock()
		return
	}
	child.mu.Unlock()
	if err := interruptRoutingProcess(child.cmd); err != nil {
		t.Fatalf("interrupt externally prebuilt Factory child: %v\n%s", err, child.evidence())
	}
	select {
	case <-child.done:
		child.mu.Lock()
		waitErr := child.waitErr
		child.stopped = true
		child.cleanExit = routingCleanExit(waitErr)
		child.mu.Unlock()
		if !routingCleanExit(waitErr) {
			t.Fatalf("Factory child clean shutdown error = %v\n%s", waitErr, child.evidence())
		}
	case <-time.After(childShutdownTimeout):
		t.Fatalf("Factory child did not stop within %s\n%s", childShutdownTimeout, child.evidence())
	}
}

func (child *routingChild) exited() (bool, error) {
	child.mu.Lock()
	defer child.mu.Unlock()
	select {
	case <-child.done:
		return true, child.waitErr
	default:
		return false, nil
	}
}

func (child *routingChild) evidence() string {
	if child == nil {
		return "child=nil"
	}
	child.mu.Lock()
	waitErr := child.waitErr
	child.mu.Unlock()
	recordDigest, recordSize := digestFile(child.recordPath)
	return fmt.Sprintf(
		"ROUTING-CHILD artifact_path=%q artifact_size=%d artifact_sha256=%s record_path=%q record_size=%d record_sha256=%s wait_error=%v stdout=%q stderr=%q record_excerpt=%q",
		child.artifact.path, child.artifact.size, child.artifact.sha256,
		child.recordPath, recordSize, recordDigest, waitErr,
		boundedOutput(child.stdout.String()), boundedOutput(child.stderr.String()), recordExcerpt(child.recordPath),
	)
}

func recordExcerpt(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "unavailable:" + err.Error()
	}
	const maxExcerpt = 8192
	if len(data) <= maxExcerpt {
		return string(data)
	}
	return string(data[:maxExcerpt/2]) + "...<middle omitted>..." + string(data[len(data)-maxExcerpt/2:])
}

func boundedOutput(value string) string {
	const maxOutput = 32 * 1024
	if len(value) <= maxOutput {
		return value
	}
	return value[:maxOutput] + "...<truncated>"
}

func digestFile(path string) (string, int64) {
	if strings.TrimSpace(path) == "" {
		return "", 0
	}
	file, err := os.Open(path)
	if err != nil {
		return "unavailable:" + err.Error(), 0
	}
	defer file.Close()
	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return "unavailable:" + err.Error(), size
	}
	return hex.EncodeToString(hasher.Sum(nil)), size
}

func waitForRoutingChildReady(t *testing.T, child *routingChild) {
	t.Helper()
	deadline := time.NewTimer(childReadyTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(childPollInterval)
	defer ticker.Stop()
	for {
		response, err := httpClient.Get(child.baseURL + "/status")
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode == 200 {
				return
			}
		}
		if exited, waitErr := child.exited(); exited {
			t.Fatalf("Factory child exited before readiness: %v\n%s", waitErr, child.evidence())
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for Factory child readiness\n%s", child.evidence())
		}
	}
}

var httpClient = newHTTPClient()

func newHTTPClient() *http.Client {
	return &http.Client{Timeout: childOperationTimeout}
}

func waitForRoutingEndpointClosed(t *testing.T, child *routingChild) {
	t.Helper()
	deadline := time.NewTimer(childShutdownTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(childPollInterval)
	defer ticker.Stop()
	for {
		response, err := httpClient.Get(child.baseURL + "/status")
		if err != nil {
			return
		}
		_ = response.Body.Close()
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("Factory child listener remains reachable after clean shutdown: status=%d\n%s", response.StatusCode, child.evidence())
		}
	}
}

func waitForStableRoutingEvents(t *testing.T, child *routingChild, wantTicks int) []factoryEvent {
	t.Helper()
	if wantTicks <= 0 {
		wantTicks = childQuiescenceTicks
	}
	deadline := time.NewTimer(childOperationTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(childPollInterval)
	defer ticker.Stop()
	stable := 0
	lastCount := -1
	var last []factoryEvent
	for {
		events, err := readRoutingEvents(child.baseURL, child.sessionID)
		if err == nil {
			last = events
			if len(events) == lastCount {
				stable++
			} else {
				lastCount = len(events)
				stable = 1
			}
			if stable >= wantTicks {
				return events
			}
		}
		if exited, waitErr := child.exited(); exited {
			t.Fatalf("Factory child exited while waiting for event quiescence: %v\n%s", waitErr, child.evidence())
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for stable Factory Event history; last_count=%d last_error=%v\n%s", len(last), err, child.evidence())
		}
	}
}

func assertRecordWritten(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("recorded Factory artifact %q: %v", path, err)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		t.Fatalf("recorded Factory artifact %q = mode=%s size=%d, want non-empty regular file", path, info.Mode(), info.Size())
	}
	digest, size := digestFile(path)
	t.Logf("ROUTING-RECORD path=%q size=%d sha256=%s", path, size, digest)
}
