package inference_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type residualHostRoute struct {
	fault     atomic.Bool
	mode      string
	specs     chan serviceedges.HostProcessStartSpec
	starts    atomic.Int64
	stops     atomic.Int64
	exits     atomic.Int64
	probed    chan struct{}
	ready     chan struct{}
	stopped   chan struct{}
	probeOnce sync.Once
	stopOnce  sync.Once
}

type residualHosts struct {
	mu     sync.Mutex
	routes map[string]*residualHostRoute
}

func (hosts *residualHosts) register(name, mode string) *residualHostRoute {
	hosts.mu.Lock()
	defer hosts.mu.Unlock()
	route := &residualHostRoute{specs: make(chan serviceedges.HostProcessStartSpec, 4), mode: mode, probed: make(chan struct{}), ready: make(chan struct{}, 1), stopped: make(chan struct{})}
	route.fault.Store(mode != "ready")
	hosts.routes[name] = route
	return route
}

func (hosts *residualHosts) route(endpoint string) *residualHostRoute {
	hosts.mu.Lock()
	defer hosts.mu.Unlock()
	return hosts.routes[endpoint]
}

func (hosts *residualHosts) Start(_ context.Context, spec serviceedges.HostProcessStartSpec) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	route := hosts.route(spec.HealthEndpoint)
	if route == nil {
		return nil, errors.New("unexpected controlled host route")
	}
	spec.Args = append([]string(nil), spec.Args...)
	route.specs <- spec
	route.starts.Add(1)
	if route.mode == "launch" && route.fault.Load() {
		return nil, errors.New("controlled host launch failure")
	}
	return &residualHostProcess{endpoint: spec.HealthEndpoint, route: route,
		crashed: route.mode == "crash" && route.fault.Load(), done: make(chan struct{})}, nil
}

func (hosts *residualHosts) Negotiate(ctx context.Context, endpoint string, request serviceedges.ModelHostProtocolNegotiationRequest) (serviceedges.ModelHostProtocolNegotiationResult, error) {
	route := hosts.route(endpoint)
	if route == nil {
		return serviceedges.ModelHostProtocolNegotiationResult{}, errors.New("unexpected controlled protocol route")
	}
	route.probeOnce.Do(func() { close(route.probed) })
	result := serviceedges.ModelHostProtocolNegotiationResult{ProtocolVersion: request.ProtocolVersion, Backend: request.Backend}
	if route.fault.Load() {
		if route.mode == "protocol" {
			result.ProtocolVersion = "incompatible-controlled-protocol"
			return result, nil
		}
		if route.mode == "timeout" {
			return result, nil
		}
		if route.mode == "crash" {
			return result, nil
		}
		select {
		case <-route.ready:
		case <-ctx.Done():
			return result, ctx.Err()
		}
	}
	result.Ready = true
	return result, nil
}

type residualHostProcess struct {
	endpoint string
	route    *residualHostRoute
	crashed  bool
	done     chan struct{}
	once     sync.Once
}

func (process *residualHostProcess) HealthEndpoint() string { return process.endpoint }
func (process *residualHostProcess) Wait() error {
	defer process.route.exits.Add(1)
	if process.crashed {
		return errors.New("controlled child exited before readiness")
	}
	<-process.done
	return nil
}
func (process *residualHostProcess) Stop(context.Context) error {
	process.once.Do(func() {
		process.route.stops.Add(1)
		close(process.done)
	})
	process.route.stopOnce.Do(func() { close(process.route.stopped) })
	return nil
}

func startResidualHostServer(t *testing.T, hosts *residualHosts, routes *fixedLeafRoutes, clocks ...genericCLIHostClock) *support.FunctionalAPIServer {
	t.Helper()
	home := t.TempDir()
	body := []byte("controlled-host-embedding-backend")
	selection := story004EmbedBackendSelection(body)
	writeGenericBuiltinModelCache(t, home, story004EmbedSource)
	writeGenericBackendCache(t, home, "localai-llamacpp", selection, body)
	edges := story004EmbedEdges(home, &rejectingModelAssetHTTP{}, nil, nil,
		&joinedProtocolNegotiator{}, &joinedCompatibilityChecker{}, selection, nil)
	edges.ModelHostProcessLauncher = hosts
	edges.ModelHostProtocolNegotiator = hosts
	edges.ModelEmbeddingBackend = nil
	edges.ModelInvocationBackend = routes.invoke
	edges.ModelRuntimeCommandRunner = support.NewRecordingCommandRunner("unexpected controlled host command")
	if len(clocks) != 0 {
		edges.ModelHostClock = clocks[0]
	}
	return functionalStartAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig()),
		Env:        functionalHomeEnvironment(home), Edges: edges,
		BeforeStart: func(t testing.TB, process support.Process, input root.Input) {
			bootstrapFixedLeafProfile(t, process, input)
		},
	})
}

func openResidualHostSession(t *testing.T, baseURL, endpoint string) string {
	t.Helper()
	session, _ := openResidualHostSessionWithClose(t, baseURL, endpoint)
	return session
}

func openResidualHealthSession(t *testing.T, baseURL, endpoint string) string {
	t.Helper()
	config := fixedLeafFactoryConfig()
	worker := config["workers"].([]map[string]any)[0]
	worker["command"] = "controlled-health-embed"
	worker["args"] = []string{"--health-endpoint", endpoint}
	dir := functionalScaffoldFactory(t, config)
	support.WriteWorkstationConfig(t, dir, "embed", "---\ntype: MODEL_INVOKE\n---\nEmbed the selected text.\n")
	session := support.OpenFactorySessionAt(t, baseURL, dir).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, session) })
	return session
}

func openResidualHostSessionWithClose(t *testing.T, baseURL, endpoint string) (string, func()) {
	t.Helper()
	config := fixedLeafFactoryConfig()
	worker := config["workers"].([]map[string]any)[0]
	worker["command"] = "controlled-embed"
	worker["args"] = []string{"--grpc-endpoint", endpoint}
	dir := functionalScaffoldFactory(t, config)
	support.WriteWorkstationConfig(t, dir, "embed", "---\ntype: MODEL_INVOKE\n---\nEmbed the selected text.\n")
	session := support.OpenFactorySessionAt(t, baseURL, dir).Session.Id
	var once sync.Once
	closeSession := func() { once.Do(func() { support.CloseFactorySessionAt(t, baseURL, session) }) }
	t.Cleanup(closeSession)
	return session, closeSession
}

func waitResidualHostSignal(t *testing.T, signal <-chan struct{}, failure string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(30 * time.Second):
		t.Fatal(failure)
	}
}
