// Functional owner: sessions/chat_sessions/root_composition.
package root_composition_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	chatCleanupTimeout        = 5 * time.Second
	chatCleanupCloseRequestID = 9001
)

// chatResourceCensus is package-local on purpose. The Chat root-composition
// tests own the resource lifetime they exercise, while the shared functional
// support package remains a read-only harness dependency. The census records
// the public transport boundary (ACP connections and update streams), the
// process/pipe owners around it, and the controlled external effects. It does
// not replace the production Chat Sessions or Events authorities with a fake.
type chatResourceCensus struct {
	mu sync.Mutex

	nextID uint64

	processes       map[string]*chatProcessResource
	serverOwners    map[string]string
	connections     map[string]*chatConnectionResource
	connectionInput map[string]string
	pipes           map[string]*chatPipeResource
	sessions        map[string]*chatSessionResource
	sessionIDs      map[string]string
	turns           map[string]*chatTurnResource
	calls           map[string]*chatCallResource
	peers           map[string]*chatPeerResource
	paths           map[string]*chatPathResource
	violations      []error
}

type chatProcessResource struct {
	id         string
	owner      string
	closeCount int
}

type chatConnectionResource struct {
	id               string
	owner            string
	processID        string
	closeCount       int
	streamCloseCount int
}

type chatPipeResource struct {
	id         string
	owner      string
	closeCount int
}

type chatSessionResource struct {
	id             string
	sessionID      string
	owner          string
	processID      string
	closeCount     int
	closeMode      string
	closeAttempted bool
	closeAllowed   bool
	closeError     string
}

type chatTurnResource struct {
	id         string
	sessionID  string
	owner      string
	closeCount int
}

type chatCallResource struct {
	id         string
	owner      string
	closeCount int
}

type chatPeerResource struct {
	id         string
	owner      string
	closeCount int
}

type chatPathResource struct {
	id         string
	path       string
	kind       string
	owner      string
	closeCount int
	removed    bool
}

var chatCensus = newChatResourceCensus()

func newChatResourceCensus() *chatResourceCensus {
	return &chatResourceCensus{
		processes:       make(map[string]*chatProcessResource),
		serverOwners:    make(map[string]string),
		connections:     make(map[string]*chatConnectionResource),
		connectionInput: make(map[string]string),
		pipes:           make(map[string]*chatPipeResource),
		sessions:        make(map[string]*chatSessionResource),
		sessionIDs:      make(map[string]string),
		turns:           make(map[string]*chatTurnResource),
		calls:           make(map[string]*chatCallResource),
		peers:           make(map[string]*chatPeerResource),
		paths:           make(map[string]*chatPathResource),
	}
}

func (c *chatResourceCensus) nextIDLocked(prefix string) string {
	c.nextID++
	return fmt.Sprintf("%s-%d", prefix, c.nextID)
}

func chatIdentity(value any) string {
	if value == nil {
		return ""
	}
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return ""
	}
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		return fmt.Sprintf("%T:%x", value, reflected.Pointer())
	default:
		return fmt.Sprintf("%T:%v", value, value)
	}
}

func (c *chatResourceCensus) registerProcess(process support.ApplicationProcess, owner string) string {
	serverKey := chatIdentity(process.ACPServer())
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextIDLocked("chat-process")
	if strings.TrimSpace(owner) == "" {
		owner = "unknown"
	}
	c.processes[id] = &chatProcessResource{id: id, owner: owner}
	if serverKey != "" {
		c.serverOwners[serverKey] = id
	}
	return id
}

func (c *chatResourceCensus) processForServer(server support.ACPServer) string {
	key := chatIdentity(server)
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.serverOwners[key]
}

func (c *chatResourceCensus) closeProcess(process support.ApplicationProcess) error {
	processID := c.processForServer(process.ACPServer())
	c.mu.Lock()
	defer c.mu.Unlock()
	resource, ok := c.processes[processID]
	if !ok {
		return fmt.Errorf("owner=%q resource=process has no registered BuildProcess", "chat-root-composition")
	}
	if resource.closeCount != 0 {
		return fmt.Errorf("owner=%q resource=process id=%q close count=%d want exactly once", resource.owner, processID, resource.closeCount+1)
	}
	resource.closeCount = 1
	// A session whose bounded session/close request was rejected remains owned
	// by the root process. The process is the narrowest supported fallback for
	// the current terminalized-session contract, and is deliberately recorded
	// rather than silently treating a rejected session/close as success.
	for _, session := range c.sessions {
		if session.processID != processID || session.closeCount != 0 {
			continue
		}
		session.closeCount = 1
		session.closeMode = "process-fallback"
	}
	return nil
}

func buildChatProcess(
	t testing.TB,
	owner string,
	edges serviceedges.Edges,
) (support.ApplicationProcess, error) {
	t.Helper()
	process, err := support.BuildProcessWithContext(context.Background(), edges)
	if err != nil {
		return nil, err
	}
	chatCensus.registerProcess(process, owner)
	return process, nil
}

func closeChatProcess(process support.ApplicationProcess) error {
	if process == nil {
		return nil
	}
	server := process.ACPServer()
	if err := process.Close(context.Background()); err != nil {
		return err
	}
	chatACPServerHomes.Delete(chatIdentity(server))
	return chatCensus.closeProcess(process)
}

func (c *chatResourceCensus) openConnection(server support.ACPServer, owner string, input *os.File) string {
	processID := c.processForServer(server)
	c.mu.Lock()
	defer c.mu.Unlock()
	if strings.TrimSpace(owner) == "" {
		owner = "unknown"
	}
	id := c.nextIDLocked("chat-connection")
	c.connections[id] = &chatConnectionResource{
		id:        id,
		owner:     owner,
		processID: processID,
	}
	if input != nil {
		c.connectionInput[chatIdentity(input)] = id
	}
	return id
}

func (c *chatResourceCensus) connectionForInput(input *os.File) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connectionInput[chatIdentity(input)]
}

func (c *chatResourceCensus) processForConnection(connectionID string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if resource := c.connections[connectionID]; resource != nil {
		return resource.processID
	}
	return ""
}

func (c *chatResourceCensus) closeConnection(connectionID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	resource, ok := c.connections[connectionID]
	if !ok {
		return fmt.Errorf("owner=%q resource=ACP connection id=%q close is unregistered", "chat-root-composition", connectionID)
	}
	if resource.closeCount != 0 || resource.streamCloseCount != 0 {
		return fmt.Errorf("owner=%q resource=ACP connection id=%q close count=%d stream close count=%d want exactly once", resource.owner, connectionID, resource.closeCount+1, resource.streamCloseCount+1)
	}
	resource.closeCount = 1
	resource.streamCloseCount = 1
	return nil
}

func serveChatRequest(server support.ACPServer, ctx context.Context, in io.Reader, out io.Writer) error {
	connectionID := chatCensus.openConnection(server, "one-shot ACP Serve", nil)
	defer func() {
		if err := chatCensus.closeConnection(connectionID); err != nil {
			chatCensus.recordViolation(err)
		}
	}()
	home := chatACPHomeForServer(server)
	if home == "" {
		return server.Serve(ctx, in, out)
	}
	return withChatACPHomeEnvironment(home, func() error {
		return server.Serve(ctx, in, out)
	})
}

type chatPipeEndpoint struct {
	file       *os.File
	resourceID string
	once       sync.Once
	err        error
}

func newChatPipeEndpoint(file *os.File, owner string) *chatPipeEndpoint {
	c := chatCensus
	c.mu.Lock()
	id := c.nextIDLocked("chat-pipe")
	if strings.TrimSpace(owner) == "" {
		owner = "unknown"
	}
	c.pipes[id] = &chatPipeResource{id: id, owner: owner}
	c.mu.Unlock()
	return &chatPipeEndpoint{file: file, resourceID: id}
}

func (p *chatPipeEndpoint) Close() error {
	if p == nil {
		return nil
	}
	p.once.Do(func() {
		fileErr := normalizeChatPipeCloseError(p.file.Close())
		censusErr := chatCensus.closePipe(p.resourceID)
		p.err = errors.Join(fileErr, censusErr)
	})
	return p.err
}

func normalizeChatPipeCloseError(err error) error {
	if err == nil || errors.Is(err, os.ErrClosed) {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "file already closed") {
		return nil
	}
	return err
}

func (c *chatResourceCensus) closePipe(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	resource, ok := c.pipes[id]
	if !ok {
		return fmt.Errorf("owner=%q resource=pipe id=%q close is unregistered", "chat-root-composition", id)
	}
	if resource.closeCount != 0 {
		return fmt.Errorf("owner=%q resource=pipe id=%q close count=%d want exactly once", resource.owner, id, resource.closeCount+1)
	}
	resource.closeCount = 1
	return nil
}

func (c *chatResourceCensus) registerSession(sessionID, owner, processID string) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", errors.New("chat session ID is blank")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing := c.sessionIDs[sessionID]; existing != "" {
		return "", fmt.Errorf("chat session %q was registered more than once", sessionID)
	}
	if strings.TrimSpace(owner) == "" {
		owner = "unknown"
	}
	id := c.nextIDLocked("chat-session")
	c.sessions[id] = &chatSessionResource{
		id:        id,
		sessionID: sessionID,
		owner:     owner,
		processID: processID,
	}
	c.sessionIDs[sessionID] = id
	return id, nil
}

func (c *chatResourceCensus) closeSessionExplicit(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	resource, ok := c.sessions[id]
	if !ok {
		return fmt.Errorf("owner=%q resource=Chat Session id=%q close is unregistered", "chat-root-composition", id)
	}
	if resource.closeCount != 0 {
		return nil
	}
	resource.closeCount = 1
	resource.closeMode = "ACP session/close"
	return nil
}

func (c *chatResourceCensus) recordSessionCloseFailure(id string, closeErr error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if resource := c.sessions[id]; resource != nil {
		resource.closeAttempted = true
		resource.closeAllowed = c.sessionCloseFallbackAllowedLocked(resource.sessionID, closeErr)
		resource.closeError = closeErr.Error()
	}
}

func (c *chatResourceCensus) sessionCloseFallbackAllowed(id string, closeErr error) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	resource := c.sessions[id]
	if resource == nil {
		return false
	}
	return c.sessionCloseFallbackAllowedLocked(resource.sessionID, closeErr)
}

func (c *chatResourceCensus) sessionCloseFallbackAllowedLocked(sessionID string, closeErr error) bool {
	if closeErr == nil || !strings.Contains(strings.ToLower(closeErr.Error()), "malformed_request") {
		return false
	}

	for _, turn := range c.turns {
		if turn.sessionID != sessionID {
			continue
		}
		if turn.closeCount != 1 {
			return false
		}
	}
	// The production ACP close contract currently requires an active turn.
	// A session whose observed turns are all terminal therefore has no
	// individually closable control target; its owning root process is the
	// explicit, reasoned fallback for this test lane.
	return true
}

func trackChatSession(
	t testing.TB,
	sessionID, owner, processID string,
	close func() error,
) {
	t.Helper()
	resourceID, err := chatCensus.registerSession(sessionID, owner, processID)
	if err != nil {
		t.Fatalf("register Chat Session cleanup census: %v", err)
	}
	t.Cleanup(func() {
		if err := close(); err == nil {
			if err := chatCensus.closeSessionExplicit(resourceID); err != nil {
				t.Errorf("record Chat Session close census for %q: %v", sessionID, err)
			}
			return
		} else {
			chatCensus.recordSessionCloseFailure(resourceID, err)
			if !chatCensus.sessionCloseFallbackAllowed(resourceID, err) {
				t.Errorf("Chat Session %q session/close failed: %v", sessionID, err)
			}
		}
	})
}

func trackChatSessionOnServer(t testing.TB, server support.ACPServer, sessionID string) {
	t.Helper()
	processID := chatCensus.processForServer(server)
	trackChatSession(t, sessionID, "ACP session/new", processID, func() error {
		return closeChatSessionViaServer(server, sessionID)
	})
}

func trackChatSessionOnConnection(t testing.TB, stdin *os.File, stdout *bufio.Reader, sessionID string) {
	t.Helper()
	connectionID := chatCensus.connectionForInput(stdin)
	processID := chatCensus.processForConnection(connectionID)
	trackChatSession(t, sessionID, "ACP stdio connection", processID, func() error {
		return closeChatSessionOnConnection(stdin, stdout, sessionID)
	})
}

func closeChatSessionViaServer(server support.ACPServer, sessionID string) error {
	params, err := json.Marshal(map[string]string{"sessionId": sessionID})
	if err != nil {
		return fmt.Errorf("marshal session/close params: %w", err)
	}
	line := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"session/close","params":%s}`+"\n", chatCleanupCloseRequestID, params)
	var out bytes.Buffer
	if err := serveChatRequest(server, context.Background(), strings.NewReader(line), &out); err != nil {
		return fmt.Errorf("Serve(session/close): %w", err)
	}
	responses := responseLinesOnlyErr(&out)
	if len(responses) != 1 {
		return fmt.Errorf("session/close response count = %d, want exactly 1", len(responses))
	}
	if responses[0].Error != nil {
		return fmt.Errorf("session/close RPC error: %s", responses[0].Error.Error())
	}
	return nil
}

func closeChatSessionOnConnection(stdin *os.File, stdout *bufio.Reader, sessionID string) error {
	params, err := json.Marshal(map[string]string{"sessionId": sessionID})
	if err != nil {
		return fmt.Errorf("marshal session/close params: %w", err)
	}
	line := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"session/close","params":%s}`+"\n", chatCleanupCloseRequestID, params)
	if _, err := stdin.Write([]byte(line)); err != nil {
		return fmt.Errorf("write session/close: %w", err)
	}
	result := make(chan error, 1)
	go func() {
		for {
			raw, err := stdout.ReadBytes('\n')
			if err != nil {
				result <- fmt.Errorf("read session/close response: %w", err)
				return
			}
			var decoded serveACPLine
			if err := json.Unmarshal(bytes.TrimSpace(raw), &decoded); err != nil {
				result <- fmt.Errorf("decode session/close response: %w", err)
				return
			}
			if decoded.Method != "" {
				continue
			}
			if strings.TrimSpace(string(decoded.ID)) != fmt.Sprint(chatCleanupCloseRequestID) {
				result <- fmt.Errorf("session/close response ID = %s, want %d", decoded.ID, chatCleanupCloseRequestID)
				return
			}
			if decoded.Error != nil {
				result <- fmt.Errorf("session/close RPC error: %s", decoded.Error.Error())
				return
			}
			result <- nil
			return
		}
	}()
	select {
	case err := <-result:
		return err
	case <-time.After(chatCleanupTimeout):
		// Closing the caller-owned input is the supported way to interrupt the
		// ACP read loop when a close response cannot be produced. The enclosing
		// invocation cleanup then joins Process.Execute and closes the remaining
		// pipe endpoints.
		_ = stdin.Close()
		return fmt.Errorf("timed out waiting for session/close response")
	}
}

func beginChatTurn(sessionID, owner string) string {
	chatCensus.mu.Lock()
	defer chatCensus.mu.Unlock()
	if strings.TrimSpace(owner) == "" {
		owner = "unknown"
	}
	id := chatCensus.nextIDLocked("chat-turn")
	chatCensus.turns[id] = &chatTurnResource{id: id, sessionID: sessionID, owner: owner}
	return id
}

func closeChatTurn(id string) error {
	chatCensus.mu.Lock()
	defer chatCensus.mu.Unlock()
	resource, ok := chatCensus.turns[id]
	if !ok {
		return fmt.Errorf("owner=%q resource=turn id=%q close is unregistered", "chat-root-composition", id)
	}
	if resource.closeCount != 0 {
		return fmt.Errorf("owner=%q resource=turn id=%q close count=%d want exactly once", resource.owner, id, resource.closeCount+1)
	}
	resource.closeCount = 1
	return nil
}

func beginChatCall(owner string) string {
	chatCensus.mu.Lock()
	defer chatCensus.mu.Unlock()
	if strings.TrimSpace(owner) == "" {
		owner = "unknown"
	}
	id := chatCensus.nextIDLocked("chat-call")
	chatCensus.calls[id] = &chatCallResource{id: id, owner: owner}
	return id
}

func closeChatCall(id string) error {
	chatCensus.mu.Lock()
	defer chatCensus.mu.Unlock()
	resource, ok := chatCensus.calls[id]
	if !ok {
		return fmt.Errorf("owner=%q resource=active-call id=%q close is unregistered", "chat-root-composition", id)
	}
	if resource.closeCount != 0 {
		return fmt.Errorf("owner=%q resource=active-call id=%q close count=%d want exactly once", resource.owner, id, resource.closeCount+1)
	}
	resource.closeCount = 1
	return nil
}

func beginChatPeer(owner string) string {
	chatCensus.mu.Lock()
	defer chatCensus.mu.Unlock()
	if strings.TrimSpace(owner) == "" {
		owner = "unknown"
	}
	id := chatCensus.nextIDLocked("chat-peer")
	chatCensus.peers[id] = &chatPeerResource{id: id, owner: owner}
	return id
}

func trackChatPeerOwner(t testing.TB, owner string) {
	t.Helper()
	t.Cleanup(func() {
		if err := closeChatPeers(owner); err != nil {
			t.Errorf("close ACP peer process census for %q: %v", owner, err)
		}
	})
}

func closeChatPeers(owner string) error {
	chatCensus.mu.Lock()
	defer chatCensus.mu.Unlock()
	for _, resource := range chatCensus.peers {
		if resource.owner != owner || resource.closeCount != 0 {
			continue
		}
		resource.closeCount = 1
	}
	return nil
}

func (c *chatResourceCensus) registerPath(owner, kind, path string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if strings.TrimSpace(owner) == "" {
		owner = "unknown"
	}
	id := c.nextIDLocked("chat-path")
	c.paths[id] = &chatPathResource{
		id:    id,
		path:  filepath.Clean(path),
		kind:  kind,
		owner: owner,
	}
	return id
}

func chatTempDir(t testing.TB, owner, prefix string) string {
	t.Helper()
	path, err := os.MkdirTemp("", prefix)
	if err != nil {
		t.Fatalf("create temporary path %q: %v", prefix, err)
	}
	chatCensus.registerPath(owner, "temporary-root", path)
	t.Cleanup(func() {
		if err := os.RemoveAll(path); err != nil {
			t.Errorf("remove temporary path %q: %v", path, err)
		}
		if err := chatCensus.closePathsUnder(path); err != nil {
			t.Errorf("temporary path cleanup for %q: %v", path, err)
		}
	})
	return path
}

func chatMkdirTemp(t testing.TB, owner, parent, prefix string) string {
	t.Helper()
	path, err := os.MkdirTemp(parent, prefix)
	if err != nil {
		t.Fatalf("create temporary path %q: %v", prefix, err)
	}
	chatCensus.registerPath(owner, "temporary-root", path)
	t.Cleanup(func() {
		if err := os.RemoveAll(path); err != nil {
			t.Errorf("remove temporary path %q: %v", path, err)
		}
		if err := chatCensus.closePathsUnder(path); err != nil {
			t.Errorf("temporary path cleanup for %q: %v", path, err)
		}
	})
	return path
}

func chatPersistentMkdirTemp(owner, prefix string) (string, error) {
	path, err := os.MkdirTemp("", prefix)
	if err != nil {
		return "", err
	}
	chatCensus.registerPath(owner, "temporary-root", path)
	return path, nil
}

func registerChatFactoryPath(t testing.TB, path string) {
	t.Helper()
	chatCensus.registerPath(t.Name(), "temporary-factory", path)
}

func (c *chatResourceCensus) closePathsUnder(root string) error {
	root = filepath.Clean(root)
	c.mu.Lock()
	defer c.mu.Unlock()
	var errs []error
	for _, resource := range c.paths {
		if resource.closeCount != 0 || resource.path == "" || !chatPathWithin(root, resource.path) {
			continue
		}
		removed := false
		if _, err := os.Stat(resource.path); errors.Is(err, os.ErrNotExist) {
			removed = true
		} else if err != nil {
			errs = append(errs, fmt.Errorf("owner=%q resource=%s path=%q absence probe: %w", resource.owner, resource.kind, resource.path, err))
		}
		resource.closeCount = 1
		resource.removed = removed
		if !removed {
			errs = append(errs, fmt.Errorf("owner=%q resource=%s path=%q remains", resource.owner, resource.kind, resource.path))
		}
	}
	return errors.Join(errs...)
}

func chatPathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func chatRemoveRoot(root string) error {
	if err := os.RemoveAll(root); err != nil {
		return err
	}
	return chatCensus.closePathsUnder(root)
}

func (c *chatResourceCensus) recordViolation(err error) {
	if err == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.violations = append(c.violations, err)
}

func (c *chatResourceCensus) summary() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	processesClosed := 0
	connectionsClosed := 0
	streamsClosed := 0
	pipesClosed := 0
	sessionsClosed := 0
	explicitSessions := 0
	fallbackSessions := 0
	turnsClosed := 0
	callsClosed := 0
	peersClosed := 0
	pathsRemoved := 0
	for _, resource := range c.processes {
		if resource.closeCount == 1 {
			processesClosed++
		}
	}
	for _, resource := range c.connections {
		if resource.closeCount == 1 {
			connectionsClosed++
		}
		if resource.streamCloseCount == 1 {
			streamsClosed++
		}
	}
	for _, resource := range c.pipes {
		if resource.closeCount == 1 {
			pipesClosed++
		}
	}
	for _, resource := range c.sessions {
		if resource.closeCount == 1 {
			sessionsClosed++
		}
		switch resource.closeMode {
		case "ACP session/close":
			explicitSessions++
		case "process-fallback":
			fallbackSessions++
		}
	}
	for _, resource := range c.turns {
		if resource.closeCount == 1 {
			turnsClosed++
		}
	}
	for _, resource := range c.calls {
		if resource.closeCount == 1 {
			callsClosed++
		}
	}
	for _, resource := range c.peers {
		if resource.closeCount == 1 {
			peersClosed++
		}
	}
	for _, resource := range c.paths {
		if resource.removed {
			pathsRemoved++
		}
	}
	return fmt.Sprintf(
		"processes=%d/%d connections=%d/%d response-streams=%d/%d pipes=%d/%d sessions=%d/%d sessions-explicit=%d sessions-process-fallback=%d turns=%d/%d active-calls=%d/%d peer-processes=%d/%d paths-removed=%d/%d listeners=0/0 violations=%d",
		processesClosed, len(c.processes),
		connectionsClosed, len(c.connections),
		streamsClosed, len(c.connections),
		pipesClosed, len(c.pipes),
		sessionsClosed, len(c.sessions), explicitSessions, fallbackSessions,
		turnsClosed, len(c.turns),
		callsClosed, len(c.calls),
		peersClosed, len(c.peers),
		pathsRemoved, len(c.paths),
		len(c.violations),
	)
}

func (c *chatResourceCensus) assertClean() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var errs []error
	for _, resource := range c.processes {
		if resource.closeCount != 1 {
			errs = append(errs, fmt.Errorf("owner=%q resource=process id=%q close count=%d want exactly once", resource.owner, resource.id, resource.closeCount))
		}
	}
	for _, resource := range c.connections {
		if resource.closeCount != 1 || resource.streamCloseCount != 1 {
			errs = append(errs, fmt.Errorf("owner=%q resource=ACP connection id=%q close=%d stream-close=%d want exactly once", resource.owner, resource.id, resource.closeCount, resource.streamCloseCount))
		}
	}
	for _, resource := range c.pipes {
		if resource.closeCount != 1 {
			errs = append(errs, fmt.Errorf("owner=%q resource=pipe id=%q close count=%d want exactly once", resource.owner, resource.id, resource.closeCount))
		}
	}
	for _, resource := range c.sessions {
		if resource.closeCount != 1 {
			errs = append(errs, fmt.Errorf("owner=%q resource=Chat Session id=%q close count=%d want exactly once", resource.owner, resource.sessionID, resource.closeCount))
			continue
		}
		if resource.closeMode == "process-fallback" {
			if !resource.closeAttempted || !resource.closeAllowed {
				errs = append(errs, fmt.Errorf("owner=%q resource=Chat Session id=%q used process fallback without a bounded close rejection and no open observed turn: %q", resource.owner, resource.sessionID, resource.closeError))
			}
			process, ok := c.processes[resource.processID]
			if !ok || process.closeCount != 1 {
				errs = append(errs, fmt.Errorf("owner=%q resource=Chat Session id=%q process fallback owner=%q is not closed", resource.owner, resource.sessionID, resource.processID))
			}
		}
	}
	for _, resource := range c.turns {
		if resource.closeCount != 1 {
			errs = append(errs, fmt.Errorf("owner=%q resource=turn id=%q close count=%d want exactly once", resource.owner, resource.id, resource.closeCount))
		}
	}
	for _, resource := range c.calls {
		if resource.closeCount != 1 {
			errs = append(errs, fmt.Errorf("owner=%q resource=active-call id=%q close count=%d want exactly once", resource.owner, resource.id, resource.closeCount))
		}
	}
	for _, resource := range c.peers {
		if resource.closeCount != 1 {
			errs = append(errs, fmt.Errorf("owner=%q resource=peer-process id=%q close count=%d want exactly once", resource.owner, resource.id, resource.closeCount))
		}
	}
	for _, resource := range c.paths {
		if resource.closeCount != 1 || !resource.removed {
			errs = append(errs, fmt.Errorf("owner=%q resource=%s path=%q close=%d removed=%t want closed and absent", resource.owner, resource.kind, resource.path, resource.closeCount, resource.removed))
		}
	}
	errs = append(errs, c.violations...)
	return errors.Join(errs...)
}
