package inference_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Advance only the supplied model clock. Canonical Wire disables idle unload
// (duration zero); changing that policy to create a timer would widen this slice.
type fixedLeafClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fixedLeafTimer
}

type fixedLeafTimer struct {
	due     time.Time
	events  chan time.Time
	stopped atomic.Bool
}

func (c *fixedLeafClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fixedLeafClock) NewTimer(duration time.Duration) interface {
	C() <-chan time.Time
	Stop() bool
} {
	c.mu.Lock()
	defer c.mu.Unlock()
	timer := &fixedLeafTimer{due: c.now.Add(duration), events: make(chan time.Time, 1)}
	c.timers = append(c.timers, timer)
	return timer
}

func (c *fixedLeafClock) advance(duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(duration)
	for _, timer := range c.timers {
		if !c.now.Before(timer.due) && timer.stopped.CompareAndSwap(false, true) {
			timer.events <- c.now
		}
	}
}

func (t *fixedLeafTimer) C() <-chan time.Time { return t.events }
func (t *fixedLeafTimer) Stop() bool          { return t.stopped.CompareAndSwap(false, true) }

// The root keeps one immutable host edge. Observations are keyed by the public
// scenario-owned worker command, so parallel sessions cannot overwrite observations.
type fixedLeafHostEffects struct {
	*recordingModelHostLauncher
	specMu sync.Mutex
	specs  map[string]serviceedges.HostProcessStartSpec
}

func (effects *fixedLeafHostEffects) Start(ctx context.Context, spec serviceedges.HostProcessStartSpec) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	process, err := effects.recordingModelHostLauncher.Start(ctx, spec)
	if err != nil {
		return nil, err
	}
	effects.specMu.Lock()
	defer effects.specMu.Unlock()
	if effects.specs == nil {
		effects.specs = make(map[string]serviceedges.HostProcessStartSpec)
	}
	spec.ModelFiles = append([]string(nil), spec.ModelFiles...)
	spec.BackendFiles = append([]string(nil), spec.BackendFiles...)
	spec.Args = append([]string(nil), spec.Args...)
	effects.specs[spec.Command] = spec
	return process, nil
}

func (effects *fixedLeafHostEffects) assertConfiguration(t *testing.T, scope models.RuntimeScopeRef, name, home string) {
	t.Helper()
	effects.specMu.Lock()
	spec, found := effects.specs["fixed-leaf-"+name]
	effects.specMu.Unlock()
	if !found || spec.Command != "fixed-leaf-"+name || spec.Backend != "localai-llamacpp" ||
		spec.HealthEndpoint != effects.endpoint {
		t.Fatalf("selected host spec = %#v, want %s command/backend/endpoint for scope %s", spec, name, scope.String())
	}
	cache := filepath.Join(home, ".agent-factory", "models")
	assertFixedLeafHostAssetPaths(t, spec, cache)
	assertFixedLeafHostArguments(t, spec)
	assertFixedLeafHostAssetContents(t, spec)
}

func assertFixedLeafHostAssetPaths(t *testing.T, spec serviceedges.HostProcessStartSpec, cache string) {
	t.Helper()
	if len(spec.ModelFiles) != 1 || len(spec.BackendFiles) != 1 || spec.ModelPath != spec.ModelFiles[0] ||
		!pathWithinLocalAIRoot(cache, spec.ModelPath) || !pathWithinLocalAIRoot(cache, spec.BackendFiles[0]) {
		t.Fatalf("selected host asset paths = %#v, want verified model/backend below %s", spec, cache)
	}
	if !strings.Contains(filepath.ToSlash(spec.ModelPath), strings.SplitN(story004EmbedSource, "@", 2)[1]) {
		t.Fatalf("host model path = %s, want the selected pinned revision", spec.ModelPath)
	}
}

func assertFixedLeafHostArguments(t *testing.T, spec serviceedges.HostProcessStartSpec) {
	t.Helper()
	if len(spec.Args) != 5 || spec.Args[0] != "serve" || spec.Args[1] != "--cache-path" ||
		spec.Args[2] != filepath.Dir(spec.ModelPath) || spec.Args[3] != "--backend-cache-path" || spec.Args[4] != filepath.Dir(spec.BackendFiles[0]) {
		t.Fatalf("selected host launch arguments = %#v, want exact selected model/backend cache paths", spec.Args)
	}
}

func assertFixedLeafHostAssetContents(t *testing.T, spec serviceedges.HostProcessStartSpec) {
	t.Helper()
	for _, asset := range []struct{ path, body string }{{spec.ModelPath, "joined built-in tts fixture"}, {spec.BackendFiles[0], "fixed-leaf-embedding-backend"}} {
		if body, err := os.ReadFile(asset.path); err != nil || string(body) != asset.body {
			t.Fatalf("selected host asset %s = %q, %v, want %q", asset.path, body, err, asset.body)
		}
	}
}

func openConfiguredFixedLeafSession(t *testing.T, baseURL, name, endpoint string) string {
	t.Helper()
	config := fixedLeafFactoryConfig()
	resource := "fixed-leaf-" + name + "-cache"
	config["resources"].([]map[string]any)[0]["name"] = resource
	worker := config["workers"].([]map[string]any)[0]
	worker["resources"] = []map[string]any{{"name": resource, "capacity": 1}}
	worker["command"] = "fixed-leaf-" + name
	worker["args"] = []string{"--grpc-endpoint", endpoint}
	dir := functionalScaffoldFactory(t, config)
	support.WriteWorkstationConfig(t, dir, "embed", "---\ntype: MODEL_INVOKE\n---\nEmbed the selected text.\n")
	session := support.OpenFactorySessionAt(t, baseURL, dir).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, session) })
	return session
}

func assertFixedLeafOwnedPeerRecovery(t *testing.T, baseURL, peer, peerScope string, peerRoute *fixedLeafRoute, routes *fixedLeafRoutes) {
	t.Helper()
	select {
	case <-peerRoute.canceled:
		t.Fatal("selected cancellation canceled the accepted peer effect")
	default:
	}
	peerRoute.release <- struct{}{}
	status := support.WaitForSessionTerminalStatus(t, baseURL, peer, 5*time.Second)
	if status.Categories.Terminal != 1 || status.Categories.Failed != 0 {
		t.Fatalf("peer terminal state = %#v", status)
	}
	works := support.GetJSON[factoryapi.ListWorkResponse](t, baseURL+"/factory-sessions/"+peer+"/work")
	if len(works.Results) != 1 {
		t.Fatalf("peer Work = %#v, want one completed result", works)
	}
	assertFixedLeafSuccess(t, factoryapi.InvocationResponse{Status: factoryapi.InvocationTerminalStatusCompleted, PrimaryResult: works.Results[0].Content}, peerRoute.output)
	retry := routes.register("owned-host-peer-retry", false)
	assertFixedLeafSuccess(t, invokeFixedLeafSession(t, baseURL, peer, "owned-host-peer-retry", "owned-host-peer-retry"), retry.output)
	if scope := waitFixedLeafAccepted(t, retry); scope != peerScope {
		t.Fatalf("peer retry scope = %s, want %s", scope, peerScope)
	}
}
