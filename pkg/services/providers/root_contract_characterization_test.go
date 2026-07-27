package providers_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// TestRootContractInvariants_AllSlicesThroughSingularService seals the
// Providers root-contract packet: every published slice (catalog list/get and
// one-attempt Execute) is reachable through one named providers.Service, a
// peer-shaped fake can exercise success and typed-failure paths using only the
// root package, and no second peer-facing Providers authority is required.
func TestRootContractInvariants_AllSlicesThroughSingularService(t *testing.T) {
	t.Parallel()

	codex := providers.Descriptor{
		ID:           providers.IDCodex,
		DisplayName:  "Codex",
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
		Capabilities: []providers.Capability{providers.CapabilityPromptSubmission},
	}
	service := newExecutePeerFake("cancel-attempt", codex)
	var root providers.Service = service

	assertSealCatalogSuccess(t, root)
	assertSealCatalogFailures(t, root, codex)
	assertSealExecuteSuccess(t, root)
	assertSealExecuteFailures(t, root)
}

func assertSealCatalogSuccess(t *testing.T, service providers.Service) {
	t.Helper()

	list, err := service.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}
	if len(list.Providers) != 1 || list.Providers[0].ID != providers.IDCodex {
		t.Fatalf("ListProviders() = %#v, want one codex descriptor", list.Providers)
	}

	got, err := service.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.IDCodex})
	if err != nil {
		t.Fatalf("GetProvider(codex) = %v", err)
	}
	if got.Provider.DisplayName != "Codex" ||
		got.Provider.Availability != providers.AvailabilitySelectable {
		t.Fatalf("GetProvider(codex) = %#v", got.Provider)
	}
}

func assertSealCatalogFailures(t *testing.T, service providers.Service, codex providers.Descriptor) {
	t.Helper()

	assertGetErrorIs(t, service, providers.GetProviderRequest{}, providers.ErrInvalidID)
	assertGetErrorIs(t, service, providers.GetProviderRequest{ID: providers.IDClaude}, providers.ErrUnknownProvider)

	unavailable := providers.Descriptor{
		ID:           providers.IDCursor,
		DisplayName:  "Cursor",
		Availability: providers.AvailabilitySupportedButUnavailable,
		Readiness:    providers.ReadinessUnavailable,
		Prerequisites: []providers.Prerequisite{{
			Kind:   providers.PrerequisiteDependency,
			Name:   "cursor-agent",
			Status: providers.PrerequisiteMissing,
		}},
	}
	blocked := newExecutePeerFake("cancel-attempt", codex, unavailable)
	assertGetErrorIs(
		t,
		blocked,
		providers.GetProviderRequest{ID: providers.IDCursor},
		providers.ErrProviderUnavailable,
	)
}

func assertSealExecuteSuccess(t *testing.T, service providers.Service) {
	t.Helper()

	result, err := service.Execute(context.Background(), providers.ExecuteRequest{
		Provider:    providers.IDCodex,
		AttemptID:   "seal-attempt",
		UserMessage: "hello seal",
	})
	if err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if result.Content != "hello seal-result" {
		t.Fatalf("Execute() content = %q, want hello seal-result", result.Content)
	}
	if result.SessionRef == nil || result.SessionRef.ID != "session-seal-attempt" {
		t.Fatalf("Execute() SessionRef = %#v", result.SessionRef)
	}
}

func assertSealExecuteFailures(t *testing.T, service providers.Service) {
	t.Helper()

	_, err := service.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDCodex,
		AttemptID: "cancel-attempt",
	})
	if !errors.Is(err, providers.ErrExecuteCancelled) {
		t.Fatalf("cancelled Execute() error = %v, want ErrExecuteCancelled", err)
	}
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("cancelled Execute() error = %T(%v), want ExecuteFailure", err, err)
	}

	_, err = service.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDClaude,
		AttemptID: "seal-unknown",
	})
	if !errors.Is(err, providers.ErrUnknownProvider) {
		t.Fatalf("unknown provider Execute() error = %v, want ErrUnknownProvider", err)
	}
}

func TestRootContract_ContractValuesStayInertWhenHeld(t *testing.T) {
	t.Parallel()

	descriptor := providers.Descriptor{
		ID:          providers.IDCodex,
		Aliases:     []string{"openai-codex"},
		DisplayName: "Codex",
	}
	clonedDescriptor := descriptor.Clone()
	descriptor.DisplayName = "mutated"
	if clonedDescriptor.DisplayName == "mutated" {
		t.Fatal("Descriptor.Clone() shares mutable display name state")
	}

	request := providers.ExecuteRequest{
		Provider:    providers.IDCodex,
		AttemptID:   "inert-attempt",
		UserMessage: "hello",
	}
	clonedRequest := request.Clone()
	request.UserMessage = "mutated"
	if clonedRequest.UserMessage == "mutated" {
		t.Fatal("ExecuteRequest.Clone() shares mutable user message state")
	}

	session := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "session-inert",
	}
	if err := session.Validate(); err != nil {
		t.Fatalf("SessionRef.Validate() = %v", err)
	}
	if cloned := session.Clone(); cloned != session {
		t.Fatalf("SessionRef.Clone() = %#v, want %#v", cloned, session)
	}

	// Holding contract values must not require a Service implementation or
	// perform adapter/registry/conductor/process work.
	var (
		_ providers.Descriptor     = descriptor
		_ providers.ExecuteRequest = request
		_ providers.SessionRef     = session
		_ providers.ListProvidersRequest
		_ providers.GetProviderRequest
		_ providers.ExecuteResult
	)
}

func TestRootContract_FakePeerConstructionIsInert(t *testing.T) {
	t.Parallel()

	fake := newExecutePeerFake("cancel-attempt")
	if fake.providers == nil {
		t.Fatal("fake peer construction returned nil catalog map")
	}
	if len(fake.providers) != 0 {
		t.Fatalf("fake peer construction initialized catalog entries = %d, want 0", len(fake.providers))
	}

	var service providers.Service = fake
	if service == nil {
		t.Fatal("constructed Service is nil")
	}
}

func TestRootContract_PackageBoundary_AvoidsForbiddenPeers(t *testing.T) {
	t.Parallel()

	assertPackageAvoidsForbiddenPeers(
		t,
		"github.com/portpowered/infinite-you/pkg/services/providers",
		false,
	)
}

func TestRootContract_TestPackageBoundary_AvoidsForbiddenPeers(t *testing.T) {
	t.Parallel()

	assertPackageAvoidsForbiddenPeers(
		t,
		"github.com/portpowered/infinite-you/pkg/services/providers",
		true,
	)
}

func assertPackageAvoidsForbiddenPeers(t *testing.T, importPath string, includeTestDeps ...bool) {
	t.Helper()

	args := []string{
		"go",
		"list",
		"-deps",
		"-f",
		"{{if not .Standard}}{{.ImportPath}}{{end}}",
	}
	if len(includeTestDeps) > 0 && includeTestDeps[0] {
		args = append(args, "-test")
	}
	args = append(args, importPath)

	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list deps for %s: %v\n%s", importPath, err, output)
	}

	forbiddenRoots := []string{
		"github.com/portpowered/infinite-you/pkg/services/workers/provider",
		"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri",
		"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/javascript",
		"github.com/portpowered/infinite-you/pkg/transports",
		"github.com/portpowered/infinite-you/ui",
		"github.com/portpowered/infinite-you/pkg/wire",
		"github.com/portpowered/infinite-you/pkg/root",
		"github.com/portpowered/infinite-you/pkg/initializer",
	}
	for _, dep := range strings.Fields(string(output)) {
		for _, forbidden := range forbiddenRoots {
			if dep == forbidden || strings.HasPrefix(dep, forbidden+"/") {
				t.Fatalf("%s must not import %s; found dependency %s", importPath, forbidden, dep)
			}
		}
	}
}

func TestRootContract_TransitionalWorkersProviderRemainsInPlace(t *testing.T) {
	t.Parallel()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	requiredPaths := []string{
		"pkg/services/workers/provider/registry/registry.go",
		"pkg/services/workers/provider/conductor/conductor.go",
	}
	for _, relative := range requiredPaths {
		path := filepath.Join(repoRoot, filepath.FromSlash(relative))
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("transitional workers provider path %s must remain present: %v", relative, err)
		}
	}
}
