package releasesmoke

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
)

const (
	defaultTimeout     = 20 * time.Second
	processStopTimeout = 5 * time.Second
	maxLogTailBytes    = 8192
)

type Config struct {
	BinaryPath  string
	FixturePath string
	Timeout     time.Duration
}

type Result struct {
	BaseURL            string   `json:"baseUrl"`
	DashboardURL       string   `json:"dashboardUrl"`
	WorkspacePath      string   `json:"workspacePath"`
	ObservedEventTypes []string `json:"observedEventTypes"`
	CompletedWorkCount int      `json:"completedWorkCount"`
}

type Failure struct {
	Phase              string   `json:"phase"`
	Message            string   `json:"message"`
	BaseURL            string   `json:"baseUrl,omitempty"`
	DashboardURL       string   `json:"dashboardUrl,omitempty"`
	BinaryPath         string   `json:"binaryPath"`
	FixturePath        string   `json:"fixturePath"`
	WorkspacePath      string   `json:"workspacePath,omitempty"`
	ObservedEventTypes []string `json:"observedEventTypes,omitempty"`
	StdoutTail         string   `json:"stdoutTail,omitempty"`
	StderrTail         string   `json:"stderrTail,omitempty"`
}

func (f *Failure) Error() string {
	return fmt.Sprintf("%s: %s", f.Phase, f.Message)
}

func Run(ctx context.Context, cfg Config) (Result, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	binaryPath, fixturePath, err := validatePaths(cfg)
	if err != nil {
		return Result{}, err
	}

	workspacePath, err := copyFixtureToTemp(fixturePath)
	if err != nil {
		return Result{}, &Failure{
			Phase:       "prepare_workspace",
			Message:     err.Error(),
			BinaryPath:  binaryPath,
			FixturePath: fixturePath,
		}
	}
	defer os.RemoveAll(workspacePath)

	port, err := reservePort()
	if err != nil {
		return Result{}, &Failure{
			Phase:         "reserve_port",
			Message:       err.Error(),
			BinaryPath:    binaryPath,
			FixturePath:   fixturePath,
			WorkspacePath: workspacePath,
		}
	}

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	dashboardURL := baseURL + "/dashboard/ui"
	stdoutBuf := newLockedBuffer()
	stderrBuf := newLockedBuffer()

	procCtx, stopProcess := context.WithCancel(ctx)
	defer stopProcess()

	cmd := exec.CommandContext(
		procCtx,
		binaryPath,
		"run",
		"--dir", workspacePath,
		"--continuously",
		"--with-mock-workers",
		"--port", fmt.Sprintf("%d", port),
		"--quiet",
	)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf
	cmd.Dir = workspacePath

	waitCh := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return Result{}, &Failure{
			Phase:         "start_binary",
			Message:       err.Error(),
			BaseURL:       baseURL,
			DashboardURL:  dashboardURL,
			BinaryPath:    binaryPath,
			FixturePath:   fixturePath,
			WorkspacePath: workspacePath,
			StdoutTail:    stdoutBuf.Tail(),
			StderrTail:    stderrBuf.Tail(),
		}
	}
	go func() {
		waitCh <- cmd.Wait()
	}()
	defer stopCommand(cmd, stopProcess, waitCh)

	client := &http.Client{Timeout: 2 * time.Second}
	if err := waitForStatus(ctx, client, baseURL, waitCh); err != nil {
		return Result{}, failureFor(baseURL, dashboardURL, binaryPath, fixturePath, workspacePath, stdoutBuf, stderrBuf, nil, "wait_for_status", err)
	}

	stream, err := openEventStream(ctx, client, baseURL+"/events")
	if err != nil {
		return Result{}, failureFor(baseURL, dashboardURL, binaryPath, fixturePath, workspacePath, stdoutBuf, stderrBuf, nil, "open_events", err)
	}
	defer stream.Close()

	observedEvents, err := waitForEventPreludeAndWork(ctx, stream, waitCh)
	if err != nil {
		return Result{}, failureFor(baseURL, dashboardURL, binaryPath, fixturePath, workspacePath, stdoutBuf, stderrBuf, observedEvents, "verify_events", err)
	}

	if err := verifyDashboard(ctx, client, dashboardURL); err != nil {
		return Result{}, failureFor(baseURL, dashboardURL, binaryPath, fixturePath, workspacePath, stdoutBuf, stderrBuf, observedEvents, "verify_dashboard", err)
	}

	workCount, err := waitForCompletedWork(ctx, client, baseURL+"/work", waitCh)
	if err != nil {
		return Result{}, failureFor(baseURL, dashboardURL, binaryPath, fixturePath, workspacePath, stdoutBuf, stderrBuf, observedEvents, "verify_completed_work", err)
	}

	return Result{
		BaseURL:            baseURL,
		DashboardURL:       dashboardURL,
		WorkspacePath:      workspacePath,
		ObservedEventTypes: observedEvents,
		CompletedWorkCount: workCount,
	}, nil
}

func validatePaths(cfg Config) (string, string, error) {
	if strings.TrimSpace(cfg.BinaryPath) == "" {
		return "", "", errors.New("binary path is required")
	}
	if strings.TrimSpace(cfg.FixturePath) == "" {
		return "", "", errors.New("fixture path is required")
	}

	binaryPath, err := filepath.Abs(cfg.BinaryPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve binary path: %w", err)
	}
	fixturePath, err := filepath.Abs(cfg.FixturePath)
	if err != nil {
		return "", "", fmt.Errorf("resolve fixture path: %w", err)
	}

	if info, err := os.Stat(binaryPath); err != nil || info.IsDir() {
		if err == nil {
			err = errors.New("path is a directory")
		}
		return "", "", fmt.Errorf("stat binary path: %w", err)
	}
	if info, err := os.Stat(filepath.Join(fixturePath, "factory.json")); err != nil || info.IsDir() {
		if err == nil {
			err = errors.New("factory.json is a directory")
		}
		return "", "", fmt.Errorf("fixture must contain factory.json: %w", err)
	}

	return binaryPath, fixturePath, nil
}

func copyFixtureToTemp(src string) (string, error) {
	dst, err := os.MkdirTemp("", "agent-factory-release-smoke-*")
	if err != nil {
		return "", fmt.Errorf("create temp workspace: %w", err)
	}

	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		_ = os.RemoveAll(dst)
		return "", fmt.Errorf("copy fixture into temp workspace: %w", err)
	}

	return dst, nil
}

func reservePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address type %T", listener.Addr())
	}
	return addr.Port, nil
}

func waitForStatus(ctx context.Context, client *http.Client, baseURL string, waitCh <-chan error) error {
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/status", nil)
		if err != nil {
			return err
		}

		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		if err := processExit(waitCh); err != nil {
			return err
		}
		if err := sleepOrDone(ctx, 100*time.Millisecond); err != nil {
			return err
		}
	}
}

func verifyDashboard(ctx context.Context, client *http.Client, dashboardURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dashboardURL, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("dashboard status = %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		return fmt.Errorf("dashboard content type = %q", resp.Header.Get("Content-Type"))
	}
	if strings.TrimSpace(string(body)) == "" {
		return errors.New("dashboard shell was empty")
	}
	return nil
}

func waitForCompletedWork(ctx context.Context, client *http.Client, endpoint string, waitCh <-chan error) (int, error) {
	type workResponse struct {
		Results []json.RawMessage `json:"results"`
	}

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return 0, err
		}

		resp, err := client.Do(req)
		if err == nil {
			var decoded workResponse
			decodeErr := json.NewDecoder(resp.Body).Decode(&decoded)
			resp.Body.Close()
			if decodeErr == nil && resp.StatusCode == http.StatusOK && len(decoded.Results) > 0 {
				return len(decoded.Results), nil
			}
		}

		if err := processExit(waitCh); err != nil {
			return 0, err
		}
		if err := sleepOrDone(ctx, 100*time.Millisecond); err != nil {
			return 0, err
		}
	}
}

func waitForEventPreludeAndWork(ctx context.Context, stream *eventStream, waitCh <-chan error) ([]string, error) {
	seen := make(map[factoryapi.FactoryEventType]struct{})
	observed := make([]string, 0, 4)

	for {
		event, err := stream.Next(ctx)
		if err != nil {
			return observed, err
		}
		if _, ok := seen[event.Type]; !ok {
			seen[event.Type] = struct{}{}
			observed = append(observed, string(event.Type))
		}

		_, hasRun := seen[factoryapi.FactoryEventTypeRunRequest]
		_, hasInit := seen[factoryapi.FactoryEventTypeInitialStructureRequest]
		_, hasWork := seen[factoryapi.FactoryEventTypeWorkRequest]
		if hasRun && hasInit && hasWork {
			return observed, nil
		}

		if err := processExit(waitCh); err != nil {
			return observed, err
		}
	}
}

func processExit(waitCh <-chan error) error {
	select {
	case err := <-waitCh:
		if err == nil {
			return errors.New("agent-factory process exited before smoke verification completed")
		}
		return fmt.Errorf("agent-factory process exited: %w", err)
	default:
		return nil
	}
}

func sleepOrDone(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func stopCommand(cmd *exec.Cmd, stopProcess context.CancelFunc, waitCh <-chan error) {
	stopProcess()

	timer := time.NewTimer(processStopTimeout)
	defer timer.Stop()

	select {
	case <-waitCh:
		return
	case <-timer.C:
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-waitCh
	}
}

func failureFor(
	baseURL string,
	dashboardURL string,
	binaryPath string,
	fixturePath string,
	workspacePath string,
	stdoutBuf *lockedBuffer,
	stderrBuf *lockedBuffer,
	observed []string,
	phase string,
	err error,
) error {
	return &Failure{
		Phase:              phase,
		Message:            err.Error(),
		BaseURL:            baseURL,
		DashboardURL:       dashboardURL,
		BinaryPath:         binaryPath,
		FixturePath:        fixturePath,
		WorkspacePath:      workspacePath,
		ObservedEventTypes: append([]string(nil), observed...),
		StdoutTail:         stdoutBuf.Tail(),
		StderrTail:         stderrBuf.Tail(),
	}
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func newLockedBuffer() *lockedBuffer {
	return &lockedBuffer{}
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) Tail() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	data := b.buf.Bytes()
	if len(data) > maxLogTailBytes {
		data = data[len(data)-maxLogTailBytes:]
	}
	return string(data)
}

type eventStream struct {
	cancel context.CancelFunc
	done   chan struct{}
	events chan factoryapi.FactoryEvent
	errs   chan error
}

func openEventStream(ctx context.Context, client *http.Client, endpoint string) (*eventStream, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		cancel()
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("/events status = %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("/events content type = %q", resp.Header.Get("Content-Type"))
	}

	stream := &eventStream{
		cancel: cancel,
		done:   make(chan struct{}),
		events: make(chan factoryapi.FactoryEvent, 64),
		errs:   make(chan error, 1),
	}
	go stream.read(resp)
	return stream, nil
}

func (s *eventStream) read(resp *http.Response) {
	defer close(s.done)
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	var dataLines []string
	flush := func() {
		if len(dataLines) == 0 {
			return
		}
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &event); err != nil {
			select {
			case s.errs <- fmt.Errorf("decode /events payload: %w", err):
			default:
			}
			return
		}
		select {
		case s.events <- event:
		default:
			select {
			case s.errs <- errors.New("/events buffer overflow"):
			default:
			}
		}
		dataLines = nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	flush()

	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "operation was canceled") {
		select {
		case s.errs <- err:
		default:
		}
	}
}

func (s *eventStream) Next(ctx context.Context) (factoryapi.FactoryEvent, error) {
	select {
	case event := <-s.events:
		return event, nil
	case err := <-s.errs:
		return factoryapi.FactoryEvent{}, err
	case <-ctx.Done():
		return factoryapi.FactoryEvent{}, ctx.Err()
	}
}

func (s *eventStream) Close() {
	s.cancel()
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
	}
}
