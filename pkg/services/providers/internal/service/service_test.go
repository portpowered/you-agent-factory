package service_test

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
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

func TestNew_RejectsNilCatalog(t *testing.T) {
	t.Parallel()

	service, err := providerservice.New(nil)
	if err == nil || service != nil {
		t.Fatalf("New(nil) = (%v, %v), want error", service, err)
	}
}

func TestRootDelegatesListAndGetToCatalog(t *testing.T) {
	t.Parallel()

	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	root, err := providerservice.New(catalogService)
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	list, err := root.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}
	if len(list.Providers) != 8 {
		t.Fatalf("len(Providers) = %d, want 8", len(list.Providers))
	}

	got, err := root.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.IDCodex})
	if err != nil {
		t.Fatalf("GetProvider(codex) = %v", err)
	}
	if got.Provider.ID != providers.IDCodex {
		t.Fatalf("GetProvider(codex).Provider.ID = %q, want codex", got.Provider.ID)
	}

	byAlias, err := root.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.ID("cursor")})
	if err != nil {
		t.Fatalf("GetProvider(cursor) = %v", err)
	}
	if byAlias.Provider.ID != providers.IDCursor {
		t.Fatalf("GetProvider(cursor).Provider.ID = %q, want agent", byAlias.Provider.ID)
	}
}

func TestRootCatalogTypedFailuresMatchPrivateCatalog(t *testing.T) {
	t.Parallel()

	root := mustRootService(t)

	assertGetErrorIs(t, root, providers.GetProviderRequest{}, providers.ErrInvalidID)
	assertGetErrorIs(t, root, providers.GetProviderRequest{ID: providers.IDClaude + "-stale"}, providers.ErrUnknownProvider)
	assertGetErrorIs(t, root, providers.GetProviderRequest{ID: providers.IDAgy}, providers.ErrProviderUnavailable)

	list, err := root.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}
	if _, ok := indexProviders(list.Providers)[providers.IDAgy]; !ok {
		t.Fatal("ListProviders() missing unavailable agy provider")
	}
}

func TestRootCatalogProbeFailureMatchesPrivateCatalog(t *testing.T) {
	t.Parallel()

	root, err := providerswire.NewService(catalogwire.WithProbeQuery(func(
		_ context.Context,
		descriptor providers.Descriptor,
	) (catalog.ProbeFacts, error) {
		if descriptor.ID == providers.IDCodex {
			return catalog.ProbeFacts{}, errors.New("native probe stderr: /Users/customer/.codex/output")
		}
		return catalog.ProbeFacts{
			Readiness:     descriptor.Readiness,
			Prerequisites: descriptor.Prerequisites,
		}, nil
	}))
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}

	list, err := root.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}
	codex := indexProviders(list.Providers)[providers.IDCodex]
	if codex.Readiness != providers.ReadinessUnavailable {
		t.Fatalf("codex readiness = %q, want unavailable after probe failure", codex.Readiness)
	}
	if strings.Contains(codex.Prerequisites[0].Description, "/Users/") {
		t.Fatalf("probe failure description leaked native output: %q", codex.Prerequisites[0].Description)
	}

	assertGetErrorIs(t, root, providers.GetProviderRequest{ID: providers.IDCodex}, providers.ErrProviderUnavailable)
}

func TestRootExecuteRemainsUnimplemented(t *testing.T) {
	t.Parallel()

	root := mustRootService(t)

	_, err := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:    providers.IDCodex,
		AttemptID:   "attempt-1",
		UserMessage: "hello",
	})
	if !errors.Is(err, providers.ErrExecuteFailed) {
		t.Fatalf("Execute() error = %v, want ErrExecuteFailed", err)
	}

	_, err = root.Execute(context.Background(), providers.ExecuteRequest{
		Provider: providers.IDCodex,
	})
	if !errors.Is(err, providers.ErrExecuteFailed) {
		t.Fatalf("invalid Execute() error = %v, want ErrExecuteFailed", err)
	}
}

func TestRootConstructionIsInert(t *testing.T) {
	t.Parallel()

	root, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	if root == nil {
		t.Fatal("NewService() returned nil")
	}
	var _ providers.Service = root
}

func TestPackageBoundary_AvoidsWorkersProviderInternals(t *testing.T) {
	t.Parallel()

	assertPackageAvoidsForbiddenPeers(
		t,
		"github.com/portpowered/infinite-you/pkg/services/providers/internal/service",
	)
}

func TestTransitionalWorkersProviderRemainsInPlace(t *testing.T) {
	t.Parallel()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", "..", "..", ".."))
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

func mustRootService(t *testing.T) providers.Service {
	t.Helper()

	root, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	return root
}

func assertGetErrorIs(
	t *testing.T,
	service providers.Service,
	request providers.GetProviderRequest,
	want error,
) {
	t.Helper()

	_, err := service.GetProvider(context.Background(), request)
	if !errors.Is(err, want) {
		t.Fatalf("GetProvider(%#v) error = %v, want %v", request, err, want)
	}
}

func indexProviders(descriptors []providers.Descriptor) map[providers.ID]providers.Descriptor {
	byID := make(map[providers.ID]providers.Descriptor, len(descriptors))
	for _, descriptor := range descriptors {
		byID[descriptor.ID] = descriptor
	}
	return byID
}

func assertPackageAvoidsForbiddenPeers(t *testing.T, importPath string) {
	t.Helper()

	cmd := exec.Command(
		"go",
		"list",
		"-deps",
		"-f",
		"{{if not .Standard}}{{.ImportPath}}{{end}}",
		importPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list deps for %s: %v\n%s", importPath, err, output)
	}

	forbiddenRoots := []string{
		"github.com/portpowered/infinite-you/pkg/services/workers/provider",
	}
	for _, dep := range strings.Fields(string(output)) {
		for _, forbidden := range forbiddenRoots {
			if dep == forbidden || strings.HasPrefix(dep, forbidden+"/") {
				t.Fatalf("%s must not import %s; found dependency %s", importPath, forbidden, dep)
			}
		}
	}
}
