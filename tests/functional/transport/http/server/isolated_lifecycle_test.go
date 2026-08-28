package server_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const c06IsolatedLifecycleTimeout = 15 * time.Second

// c06IsolationExpectation describes the resources a process-scoped witness
// owns. Keeping the expectation beside the row makes an unclassified process
// or listener visible when a lifecycle test is added or changed.
type c06IsolationExpectation struct {
	process       bool
	listener      bool
	portRelease   bool
	rejectedBinds int
}

type c06IsolatedLifecycleRow struct {
	id                   string
	reason               string
	expectation          c06IsolationExpectation
	processConstructed   bool
	processHosted        bool
	processJoined        bool
	processClosed        bool
	listenerBound        bool
	listenerClosed       bool
	portReleased         bool
	rejectedBindAttempts int
}

type c06IsolatedRootState struct {
	removed bool
}

type c06IsolatedLifecycleLedger struct {
	mu    sync.Mutex
	rows  map[string]*c06IsolatedLifecycleRow
	roots map[string]*c06IsolatedRootState
}

var c06IsolatedLifecycle = &c06IsolatedLifecycleLedger{
	rows:  make(map[string]*c06IsolatedLifecycleRow),
	roots: make(map[string]*c06IsolatedRootState),
}

func (ledger *c06IsolatedLifecycleLedger) register(
	id, reason string,
	expectation c06IsolationExpectation,
) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if strings.TrimSpace(id) == "" {
		return errors.New("c06 isolated lifecycle ID is required")
	}
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("c06 isolated lifecycle %q has no isolation reason", id)
	}
	if existing, exists := ledger.rows[id]; exists {
		if !existing.clean() {
			return fmt.Errorf("c06 isolated lifecycle %q is already registered", id)
		}
		// go test -count=N repeats the package in one test process. A completed
		// row is safe to replace for the next iteration; an incomplete row must
		// remain a hard duplicate so the final cleanup assertion cannot be
		// bypassed.
		delete(ledger.rows, id)
	}
	if expectation.rejectedBinds < 0 {
		return fmt.Errorf("c06 isolated lifecycle %q has a negative rejected-bind expectation", id)
	}
	ledger.rows[id] = &c06IsolatedLifecycleRow{
		id:          id,
		reason:      reason,
		expectation: expectation,
	}
	return nil
}

func (row *c06IsolatedLifecycleRow) clean() bool {
	if row == nil {
		return false
	}
	if row.expectation.listener && (!row.listenerBound || !row.listenerClosed) {
		return false
	}
	if row.expectation.portRelease && !row.portReleased {
		return false
	}
	if row.rejectedBindAttempts != row.expectation.rejectedBinds {
		return false
	}
	if row.expectation.process && (!row.processConstructed || !row.processJoined || !row.processClosed) {
		return false
	}
	return true
}

func (ledger *c06IsolatedLifecycleLedger) registerRoot(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("normalize c06 isolated root %q: %w", path, err)
	}
	absolute = filepath.Clean(absolute)
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if _, exists := ledger.roots[absolute]; exists {
		return fmt.Errorf("c06 isolated root %q is already registered", absolute)
	}
	ledger.roots[absolute] = &c06IsolatedRootState{}
	return nil
}

func (ledger *c06IsolatedLifecycleLedger) markRootRemoved(path string) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if root, ok := ledger.roots[filepath.Clean(absolute)]; ok {
		root.removed = true
	}
}

func (ledger *c06IsolatedLifecycleLedger) update(id string, update func(*c06IsolatedLifecycleRow)) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if row, ok := ledger.rows[id]; ok {
		update(row)
	}
}

func (ledger *c06IsolatedLifecycleLedger) processConstructed(id string) {
	ledger.update(id, func(row *c06IsolatedLifecycleRow) { row.processConstructed = true })
}

func (ledger *c06IsolatedLifecycleLedger) processHosted(id string) {
	ledger.update(id, func(row *c06IsolatedLifecycleRow) { row.processHosted = true })
}

func (ledger *c06IsolatedLifecycleLedger) processJoined(id string) {
	ledger.update(id, func(row *c06IsolatedLifecycleRow) { row.processJoined = true })
}

func (ledger *c06IsolatedLifecycleLedger) processClosed(id string) {
	ledger.update(id, func(row *c06IsolatedLifecycleRow) { row.processClosed = true })
}

func (ledger *c06IsolatedLifecycleLedger) listenerBound(id string) {
	ledger.update(id, func(row *c06IsolatedLifecycleRow) { row.listenerBound = true })
}

func (ledger *c06IsolatedLifecycleLedger) listenerClosed(id string) {
	ledger.update(id, func(row *c06IsolatedLifecycleRow) { row.listenerClosed = true })
}

func (ledger *c06IsolatedLifecycleLedger) portReleased(id string) {
	ledger.update(id, func(row *c06IsolatedLifecycleRow) { row.portReleased = true })
}

func (ledger *c06IsolatedLifecycleLedger) rejectedBind(id string) {
	ledger.update(id, func(row *c06IsolatedLifecycleRow) { row.rejectedBindAttempts++ })
}

func (ledger *c06IsolatedLifecycleLedger) summary() string {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	processes, hosted, joined, closed := 0, 0, 0, 0
	listeners, listenerClosed, ports, rejected := 0, 0, 0, 0
	for _, row := range ledger.rows {
		if row.processConstructed {
			processes++
		}
		if row.processHosted {
			hosted++
		}
		if row.processJoined {
			joined++
		}
		if row.processClosed {
			closed++
		}
		if row.listenerBound {
			listeners++
		}
		if row.listenerClosed {
			listenerClosed++
		}
		if row.portReleased {
			ports++
		}
		rejected += row.rejectedBindAttempts
	}
	removed := 0
	for _, root := range ledger.roots {
		if root.removed {
			removed++
		}
	}
	return fmt.Sprintf(
		"c06 isolated lifecycle: process_constructions=%d process_hosts=%d process_joined=%d process_closed=%d successful_listener_binds=%d listener_closed=%d port_releases=%d rejected_bind_attempts=%d isolated_rows=%d roots_removed=%d/%d",
		processes, hosted, joined, closed, listeners, listenerClosed, ports, rejected,
		len(ledger.rows), removed, len(ledger.roots),
	)
}

func (ledger *c06IsolatedLifecycleLedger) assertClean() error {
	ledger.mu.Lock()
	rows := make([]c06IsolatedLifecycleRow, 0, len(ledger.rows))
	for _, row := range ledger.rows {
		rows = append(rows, *row)
	}
	roots := make(map[string]bool, len(ledger.roots))
	for path, root := range ledger.roots {
		roots[path] = root.removed
	}
	ledger.mu.Unlock()

	var errs []error
	for _, row := range rows {
		if row.expectation.listener {
			if !row.listenerBound {
				errs = append(errs, fmt.Errorf("c06 isolated lifecycle %q never observed listener bind", row.id))
			}
			if !row.listenerClosed {
				errs = append(errs, fmt.Errorf("c06 isolated lifecycle %q listener was not directly observed closed", row.id))
			}
		}
		if row.expectation.portRelease && !row.portReleased {
			errs = append(errs, fmt.Errorf("c06 isolated lifecycle %q port was not directly observed reusable", row.id))
		}
		if row.rejectedBindAttempts != row.expectation.rejectedBinds {
			errs = append(errs, fmt.Errorf(
				"c06 isolated lifecycle %q rejected bind attempts = %d, want %d",
				row.id, row.rejectedBindAttempts, row.expectation.rejectedBinds,
			))
		}
		if row.expectation.process {
			if !row.processConstructed {
				errs = append(errs, fmt.Errorf("c06 isolated lifecycle %q process was not recorded", row.id))
			}
			if !row.processJoined {
				errs = append(errs, fmt.Errorf("c06 isolated lifecycle %q process did not join", row.id))
			}
			if !row.processClosed {
				errs = append(errs, fmt.Errorf("c06 isolated lifecycle %q process was not directly closed", row.id))
			}
		}
	}
	for path, removed := range roots {
		if !removed {
			errs = append(errs, fmt.Errorf("c06 isolated temporary root %q remains", path))
		}
	}
	return errors.Join(errs...)
}

func c06AssertIsolatedLifecycleClean() error {
	if err := c06IsolatedLifecycle.assertClean(); err != nil {
		return errors.Join(err, errors.New(c06IsolatedLifecycle.summary()))
	}
	return nil
}

type c06IsolatedFactory struct {
	rootDir    string
	factoryDir string
}

func scaffoldC06IsolatedFactory(t *testing.T, cfg map[string]any) c06IsolatedFactory {
	t.Helper()
	rootDir, err := os.MkdirTemp("", "c06-http-isolated-")
	if err != nil {
		t.Fatalf("create c06 isolated root: %v", err)
	}
	if err := c06IsolatedLifecycle.registerRoot(rootDir); err != nil {
		_ = os.RemoveAll(rootDir)
		t.Fatalf("register c06 isolated root: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(rootDir); err != nil {
			t.Errorf("remove c06 isolated root %q: %v", rootDir, err)
			return
		}
		if _, err := os.Stat(rootDir); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("c06 isolated root %q remains after cleanup: %v", rootDir, err)
			return
		}
		c06IsolatedLifecycle.markRootRemoved(rootDir)
	})

	sourceDir := support.ScaffoldFactory(t, cfg)
	factoryDir := filepath.Join(rootDir, "factory")
	if err := copyC06Directory(sourceDir, factoryDir); err != nil {
		t.Fatalf("copy c06 isolated Factory: %v", err)
	}
	installRoutingReachabilityModelWorker(t, factoryDir)
	support.WriteAgentConfig(
		t,
		factoryDir,
		"worker-a",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"),
	)
	return c06IsolatedFactory{rootDir: rootDir, factoryDir: factoryDir}
}

type c06IsolatedProcess struct {
	support.ApplicationProcess
	id        string
	closeOnce sync.Once
}

func c06BuildIsolatedProcess(
	t *testing.T,
	id, reason string,
	expectation c06IsolationExpectation,
	edges serviceedges.Edges,
) *c06IsolatedProcess {
	t.Helper()
	expectation.process = true
	if err := c06IsolatedLifecycle.register(id, reason, expectation); err != nil {
		t.Fatal(err)
	}
	process, err := support.BuildProcessWithContext(context.Background(), edges)
	if err != nil {
		t.Fatalf("build c06 isolated process %q: %v", id, err)
	}
	c06IsolatedLifecycle.processConstructed(id)
	handle := &c06IsolatedProcess{ApplicationProcess: process, id: id}
	support.CleanupProcess(t, process)
	t.Cleanup(func() { handle.close(t) })
	return handle
}

func (process *c06IsolatedProcess) markJoined() {
	if process != nil {
		c06IsolatedLifecycle.processJoined(process.id)
	}
}

func (process *c06IsolatedProcess) close(t testing.TB) {
	if process == nil {
		return
	}
	process.closeOnce.Do(func() {
		closeContext, cancel := context.WithTimeout(context.Background(), c06IsolatedLifecycleTimeout)
		defer cancel()
		if err := process.ApplicationProcess.Close(closeContext); err != nil {
			t.Errorf("close c06 isolated process %q: %v", process.id, err)
		}
		c06IsolatedLifecycle.processClosed(process.id)
	})
}

type c06IsolatedHTTPServer struct {
	process    *c06IsolatedProcess
	command    *support.ProcessCommand
	api        *support.ProcessAPIServer
	baseURL    string
	apiStopped <-chan struct{}
	id         string
	closeOnce  sync.Once
}

func startC06IsolatedHTTPServer(
	t *testing.T,
	id, reason, factoryDir string,
	extraArgs []string,
	edges serviceedges.Edges,
) *c06IsolatedHTTPServer {
	t.Helper()
	api := support.NewProcessAPIServer()
	apiStopped := make(chan struct{})
	var apiStopOnce sync.Once
	edges.APIServerStarter = func(ctx context.Context, request platformhttpserver.StartRequest) error {
		c06IsolatedLifecycle.processHosted(id)
		originalOnBound := request.OnBound
		request.OnBound = func(binding platformhttpserver.Binding) {
			c06IsolatedLifecycle.listenerBound(id)
			if originalOnBound != nil {
				originalOnBound(binding)
			}
		}
		err := api.Start(ctx, request)
		apiStopOnce.Do(func() { close(apiStopped) })
		return err
	}
	process := c06BuildIsolatedProcess(t, id, reason, c06IsolationExpectation{
		listener:    true,
		portRelease: true,
	}, edges)
	inputs := support.FakeInputs(context.Background(), append([]string{
		"you", "run", "--dir", factoryDir, "--continuously", "--with-server", "--quiet", "--no-record",
	}, extraArgs...))
	inputs.Input.WorkingDirectory = factoryDir
	homeDir := t.TempDir()
	inputs.Input.Env = c06SharedHTTPEnvironment(homeDir)
	command := support.StartProcessCommand(t, process, inputs.Input)
	server := &c06IsolatedHTTPServer{
		process:    process,
		command:    command,
		api:        api,
		apiStopped: apiStopped,
		id:         id,
	}
	t.Cleanup(func() { server.close(t) })
	baseURL, err := api.WaitForBaseURL(c06IsolatedLifecycleTimeout)
	if err != nil {
		t.Fatalf("wait for c06 isolated HTTP server %q: %v", id, err)
	}
	server.baseURL = baseURL
	support.WaitForStatus(t, baseURL, c06IsolatedLifecycleTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	return server
}

func (server *c06IsolatedHTTPServer) URL() string {
	if server == nil {
		return ""
	}
	return server.baseURL
}

func (server *c06IsolatedHTTPServer) Done() <-chan struct{} {
	if server == nil || server.command == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return server.command.Done()
}

func (server *c06IsolatedHTTPServer) stop(t testing.TB) {
	t.Helper()
	server.close(t)
}

func (server *c06IsolatedHTTPServer) close(t testing.TB) {
	if server == nil {
		return
	}
	server.closeOnce.Do(func() {
		if server.command != nil {
			server.command.Stop(t)
			server.process.markJoined()
		}
		server.process.close(t)
		select {
		case <-server.apiStopped:
		case <-time.After(c06IsolatedLifecycleTimeout):
			t.Errorf("c06 isolated HTTP server %q did not close its listener", server.id)
			return
		}
		c06IsolatedLifecycle.listenerClosed(server.id)
		if err := assertC06ListenerReleased(server.baseURL); err != nil {
			t.Errorf("c06 isolated HTTP server %q listener cleanup: %v", server.id, err)
			return
		}
		c06IsolatedLifecycle.portReleased(server.id)
	})
}

func assertC06ListenerReleased(baseURL string) error {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("parse listener URL %q: %w", baseURL, err)
	}
	client := &http.Client{Timeout: 500 * time.Millisecond}
	response, err := client.Get(strings.TrimSuffix(baseURL, "/") + "/status")
	if err == nil {
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		return fmt.Errorf("listener still served /status: HTTP %d %q", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return rebindC06Listener(parsed.Host)
}

func rebindC06Listener(address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("rebind released listener %s: %w", address, err)
	}
	if err := listener.Close(); err != nil {
		return fmt.Errorf("close listener rebind %s: %w", address, err)
	}
	return nil
}
