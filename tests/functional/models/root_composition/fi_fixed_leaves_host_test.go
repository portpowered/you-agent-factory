package root_composition_test

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Admission is sequenced only until each owned effect is accepted, so the first
// launched host belongs to selected and the second to peer. Both then remain
// live across selected cancellation; no global invocation lock is held.
func TestModelsFixedLeavesCancelThenCloseReleasesOwnedHostAndPreservesAcceptedPeer(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	body := []byte("fixed-leaf-configured-embedding-backend")
	selection := story004EmbedBackendSelection(body)
	writeGenericBuiltinModelCache(t, home, story004EmbedSource)
	writeGenericBackendCache(t, home, "localai-llamacpp", selection, body)
	hosts := newFixedLeafConfiguredHosts(t)
	routes := newFixedLeafRoutes()
	network := &rejectingModelAssetHTTP{}
	edges := story004EmbedEdges(home, network, hosts.client,
		&recordingModelHostLauncher{}, &joinedProtocolNegotiator{},
		&joinedCompatibilityChecker{}, selection, nil)
	edges.ModelHostProcessLauncher = hosts
	edges.ModelHostProtocolNegotiator = hosts
	edges.ModelEmbeddingBackend = nil
	edges.ModelInvocationBackend = routes.invoke
	server := functionalStartAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig()),
		Env:        functionalHomeEnvironment(home), Edges: edges, BeforeStart: bootstrapFixedLeafProfile,
	})
	runFixedLeafOwnedHostCancellation(t, server.URL(), routes, hosts, home)
	if network.Calls() != 0 {
		t.Fatalf("cached session invocation attempted %d model downloads", network.Calls())
	}
}

func runFixedLeafOwnedHostCancellation(t *testing.T, baseURL string, routes *fixedLeafRoutes, hosts *fixedLeafConfiguredHosts, home string) {
	t.Helper()
	selected, closeSelected := openFixedLeafOwnedHostSession(t, baseURL)
	peer, closePeer := openFixedLeafOwnedHostSession(t, baseURL)
	selectedRoute, peerRoute := routes.registerBlocked("cancel-selected"), routes.registerBlocked("cancel-peer")
	t.Cleanup(func() { close(selectedRoute.release); close(peerRoute.release) })
	submitFixedLeafWork(t, baseURL, selected, "cancel-selected")
	selectedScope := waitFixedLeafAccepted(t, selectedRoute)
	submitFixedLeafWork(t, baseURL, peer, "cancel-peer")
	peerScope := waitFixedLeafAccepted(t, peerRoute)
	if selectedScope == peerScope {
		t.Fatal("accepted invocations shared a model scope")
	}
	for _, name := range []string{"selected", "peer"} {
		assertFixedLeafConfiguredHost(t, hosts.launchers[name], home)
		select {
		case <-hosts.negotiated[name]:
		default:
			t.Fatalf("%s host endpoint/revision was not negotiated", name)
		}
	}
	ack := postFunctionalJSON[factoryapi.FactorySessionLifecycleControlResponse](t,
		baseURL+"/factory-sessions/"+selected+"/cancel", factoryapi.FactorySessionLifecycleControlRequest{}, "cancel selected host owner")
	if ack.SessionId != selected || ack.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("selected cancellation acknowledgement = %#v", ack)
	}
	select {
	case <-selectedRoute.canceled:
	case <-time.After(30 * time.Second):
		t.Fatal("accepted selected effect did not observe cancellation")
	}
	support.WaitForSessionStopped(t, baseURL, selected, 5*time.Second)
	assertFixedLeafCanceledWork(t, baseURL, selected)
	// Cancel stops the runtime invocation; the default zero idle-unload policy
	// retains its supervised host until the customer closes the session scope.
	if !hosts.launchers["selected"].Active() || hosts.launchers["selected"].Stops() != 0 {
		t.Fatal("cancellation did not preserve the scoped host until session close")
	}
	closeSelected()
	assertLocalAIDirectHostReleased(t, hosts.launchers["selected"])
	if !hosts.launchers["peer"].Active() || hosts.launchers["peer"].Stops() != 0 {
		t.Fatal("selected cancellation stopped the accepted peer host")
	}
	assertFixedLeafOwnedPeerRecovery(t, baseURL, peer, peerScope, peerRoute, routes)
	closePeer()
	assertLocalAIDirectHostReleased(t, hosts.launchers["peer"])
}

func openFixedLeafOwnedHostSession(t *testing.T, baseURL string) (string, func()) {
	t.Helper()
	dir := functionalScaffoldFactory(t, fixedLeafFactoryConfig())
	support.WriteWorkstationConfig(t, dir, "embed", "---\ntype: MODEL_INVOKE\n---\nEmbed the selected text.\n")
	session := support.OpenFactorySessionAt(t, baseURL, dir).Session.Id
	var once sync.Once
	closeSession := func() { once.Do(func() { support.CloseFactorySessionAt(t, baseURL, session) }) }
	t.Cleanup(closeSession)
	return session, closeSession
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

func assertFixedLeafConfiguredHost(t *testing.T, launcher *localAIHostLauncher, home string) {
	t.Helper()
	spec, ok := launcher.LastSpec()
	if !ok || spec.Backend != "localai-llamacpp" || len(spec.ModelFiles) != 1 || spec.ModelFiles[0] != spec.ModelPath {
		t.Fatalf("configured host facts = %#v, observed=%t", spec, ok)
	}
	cacheRoot := filepath.Join(home, ".agent-factory", "models")
	if !pathWithinLocalAIRoot(cacheRoot, spec.ModelPath) || filepath.Base(spec.ModelPath) != "Qwen3-Embedding-0.6B-Q8_0.gguf" {
		t.Fatalf("selected model path = %q, want cached EMBED artifact under %s", spec.ModelPath, cacheRoot)
	}
	if len(spec.BackendFiles) != 1 || !pathWithinLocalAIRoot(filepath.Join(cacheRoot, "backend-artifacts"), spec.BackendFiles[0]) {
		t.Fatalf("selected backend cache files = %#v", spec.BackendFiles)
	}
}

type fixedLeafConfiguredHosts struct {
	mu         sync.Mutex
	started    int
	launchers  map[string]*localAIHostLauncher
	negotiated map[string]chan struct{}
	client     *http.Client
}

func newFixedLeafConfiguredHosts(t *testing.T) *fixedLeafConfiguredHosts {
	t.Helper()
	hosts := &fixedLeafConfiguredHosts{launchers: map[string]*localAIHostLauncher{}, negotiated: map[string]chan struct{}{}}
	for _, name := range []string{"selected", "peer"} {
		server := story004HostServer(t)
		hosts.client = server.Client()
		hosts.launchers[name] = &localAIHostLauncher{endpoint: server.URL}
		hosts.negotiated[name] = make(chan struct{}, 1)
	}
	return hosts
}

func (hosts *fixedLeafConfiguredHosts) Start(ctx context.Context, spec serviceedges.HostProcessStartSpec) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	hosts.mu.Lock()
	index := hosts.started
	hosts.started++
	hosts.mu.Unlock()
	names := []string{"selected", "peer"}
	if index >= len(names) {
		return nil, fmt.Errorf("unexpected additional host acquisition")
	}
	return hosts.launchers[names[index]].Start(ctx, spec)
}

func (hosts *fixedLeafConfiguredHosts) Negotiate(_ context.Context, endpoint string, request serviceedges.ModelHostProtocolNegotiationRequest) (serviceedges.ModelHostProtocolNegotiationResult, error) {
	for name, launcher := range hosts.launchers {
		if endpoint != launcher.endpoint {
			continue
		}
		if request.ModelName != models.BuiltInModelNameEmbed || request.Revision != "370f27d7550e0def9b39c1f16d3fbaa13aa67728" ||
			request.Backend != "localai-llamacpp" || filepath.Base(request.ModelPath) != "Qwen3-Embedding-0.6B-Q8_0.gguf" {
			return serviceedges.ModelHostProtocolNegotiationResult{}, fmt.Errorf("unexpected configured negotiation request %#v", request)
		}
		select {
		case hosts.negotiated[name] <- struct{}{}:
		default:
		}
		return serviceedges.ModelHostProtocolNegotiationResult{ProtocolVersion: request.ProtocolVersion, Backend: request.Backend, Ready: true}, nil
	}
	return serviceedges.ModelHostProtocolNegotiationResult{}, fmt.Errorf("unexpected negotiated endpoint %q", endpoint)
}
