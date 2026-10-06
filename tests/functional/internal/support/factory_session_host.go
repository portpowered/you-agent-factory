package support

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// FactorySessionHost owns one initialized process for a customer journey group.
// Each invocation opens its own Factory Session and uses a factory-scoped
// command edge. The parent test owns host cleanup after all parallel children.
type FactorySessionHost struct {
	server   *FunctionalAPIServer
	commands *factorySessionCommands
}

type functionalWorkingDirectory string

func (directory functionalWorkingDirectory) Getwd() (string, error) { return string(directory), nil }

type factorySessionCommands struct {
	mu     sync.RWMutex
	routes map[string]platformprocess.CommandRunner
}

func NewFactorySessionHost(t *testing.T) *FactorySessionHost {
	t.Helper()
	commands := &factorySessionCommands{routes: make(map[string]platformprocess.CommandRunner)}
	idle := ScaffoldFactory(t, map[string]any{"workTypes": []map[string]any{{"name": "idle", "states": []map[string]string{{"name": "init", "type": "INITIAL"}, {"name": "done", "type": "TERMINAL"}}}}})
	ClearSeedInputs(t, idle)
	server := StartFunctionalAPIServer(t, FunctionalAPIServerConfig{
		FactoryDir: idle, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: commands},
	})
	return &FactorySessionHost{server: server, commands: commands}
}

// Run observes terminal Work and retained Factory Events in an explicit live
// session. It leaves peer sessions and the shared host running.
func (host *FactorySessionHost) Run(t *testing.T, dir string, runner platformprocess.CommandRunner, timeout time.Duration) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	t.Helper()
	baseURL, id := host.Open(t, dir, runner)
	WaitForSessionTerminalStatus(t, baseURL, id, timeout)
	endpoint := baseURL + "/factory-sessions/" + url.PathEscape(id)
	detail := GetJSON[factoryapi.FactorySessionGetResponse](t, endpoint)
	session, err := detail.AsFactorySession()
	if err != nil {
		t.Fatal(err)
	}
	work := GetJSON[factoryapi.ListWorkResponse](t, endpoint+"/work")
	events := GetFactoryEventsForSessionAt(t, baseURL, id)
	return session, work, events
}

// Open starts an isolated Factory Session, including journeys that intentionally
// remain blocked. Session cleanup runs before its command route is removed.
func (host *FactorySessionHost) Open(t *testing.T, dir string, runner platformprocess.CommandRunner) (baseURL, sessionID string) {
	t.Helper()
	if runner == nil {
		t.Fatal("Factory Session command runner is required")
	}
	key, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	host.commands.mu.Lock()
	_, exists := host.commands.routes[key]
	if !exists {
		host.commands.routes[key] = runner
	}
	host.commands.mu.Unlock()
	if exists {
		t.Fatalf("Factory Session command route already registered for %s", dir)
	}
	t.Cleanup(func() { host.commands.mu.Lock(); delete(host.commands.routes, key); host.commands.mu.Unlock() })
	opened := OpenFactorySessionAt(t, host.server.URL(), dir)
	id := opened.Session.Id
	t.Cleanup(func() { CloseFactorySessionAt(t, host.server.URL(), id) })
	return host.server.URL(), id
}

func (commands *factorySessionCommands) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	commands.mu.RLock()
	var selected platformprocess.CommandRunner
	selectedLength := 0
	for root, runner := range commands.routes {
		relative, err := filepath.Rel(root, request.WorkDir)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		if len(root) > selectedLength {
			selected = runner
			selectedLength = len(root)
		}
	}
	commands.mu.RUnlock()
	if selected == nil {
		return platformprocess.CommandResult{}, fmt.Errorf("no Factory Session command route for %s", request.WorkDir)
	}
	return selected.Run(ctx, request)
}
