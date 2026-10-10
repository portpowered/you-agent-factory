package inference_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Each immutable cold gallery graph owns its cache and fault selector. The
// installed offline peer must remain usable before the failed edge is repaired.
func TestModelsGalleryBootstrapFailureKeepsOfflinePeer(t *testing.T) {
	requireGalleryPlatform(t)
	t.Parallel()
	for _, fault := range []string{"metadata", "checksum", "command"} {
		t.Run(fault, func(t *testing.T) {
			t.Parallel()
			home, fixture := newGalleryLifecycleFixture(t, fault)
			peer := seedGalleryOfflinePeer(t, fixture)
			fixture.failed.Store(true)
			process := buildPullToReadyProcess(t, galleryLifecycleInferenceEdges(t, fixture, home))
			galleryAssertPullFailure(t, process, home)
			requests, commands := fixture.requests.Load(), fixture.commands.Load()
			assertGalleryLifecycleInference(t, process, peer)
			if fixture.requests.Load() != requests || fixture.commands.Load() != commands {
				t.Fatal("offline peer attempted the still-failing gallery effect")
			}
			fixture.failed.Store(false)
			assertGalleryLifecyclePull(t, process, home, fixture)
			assertGalleryLifecycleInference(t, process, home)
			assertGalleryLifecycleInference(t, process, peer)
		})
	}
}

func seedGalleryOfflinePeer(t *testing.T, fixture *galleryPullFixture) string {
	t.Helper()
	home := t.TempDir()
	writeGenericModelSourceOverride(t, home, pullToReadyModelName, pullToReadySource, "localai-whisper")
	writeGenericBuiltinModelCache(t, home, pullToReadySource)
	if err := os.Rename(filepath.Join(home, ".agent-factory", "models"), filepath.Join(home, "managed-cache")); err != nil {
		t.Fatal(err)
	}
	installation := filepath.Join(fixture.cache, "you", "localai-backends", "cpu-whisper")
	if err := os.MkdirAll(installation, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installation, "run.sh"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return home
}

// Hosted invocation owns explicit Factory Session Work. Both selected and peer
// Sessions use the supported embedding model with the production gallery graph.
// CLI pull repairs the same root process before its host command starts.
func TestModelsGalleryBootstrapSessionInference(t *testing.T) {
	requireGalleryPlatform(t)
	t.Parallel()
	home, fixture := newGalleryLifecycleFixture(t, "metadata")
	writeGenericBuiltinModelCache(t, home, story004EmbedSource)
	fixture.failed.Store(true)
	routes := newFixedLeafRoutes()
	edges := galleryLifecycleInferenceEdges(t, fixture, home)
	edges.ModelInvocationBackend = routes.invoke
	server := functionalStartAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig()),
		Env:        functionalHomeEnvironment(home), Edges: edges,
		BeforeStart: func(_ testing.TB, process support.Process, _ root.Input) {
			composed := process.(rootProcess)
			galleryAssertPullFailure(t, composed, home)
			fixture.failed.Store(false)
			assertGalleryLifecyclePull(t, composed, home, fixture)
			inputs := support.FakeInputs(t.Context(), []string{"you", "--json", "models", "pull", "embed"})
			inputs.Input.Env = functionalHomeEnvironment(home)
			inputs.Input.WorkingDirectory = t.TempDir()
			if err := composed.Execute(inputs.Input); err != nil {
				t.Fatalf("gallery embedding pull: %v; %s", err, inputs.Stderr())
			}
		},
	})
	for _, name := range []string{"selected", "peer"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			session := openFixedLeafSession(t, server.URL())
			for _, requestID := range []string{name + "-first", name + "-reuse"} {
				route := routes.register(requestID, false)
				response := invokeFixedLeafSession(t, server.URL(), session, requestID, requestID)
				assertFixedLeafSuccess(t, response, route.output)
				if (response.SessionId != nil && *response.SessionId != session) || response.RequestId != requestID {
					t.Fatalf("gallery Session identity = %#v", response)
				}
				routes.recordScope(t, name, waitFixedLeafAccepted(t, route))
				assertFixedLeafModelEvent(t, server.URL(), session, requestID, false)
			}
		})
	}
}
