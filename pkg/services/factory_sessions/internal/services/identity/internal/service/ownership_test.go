package service_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	identitywire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity/wire"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
	"go.uber.org/zap"
)

// Compile-time proof: the private identity surface reuses the CTR-SES root
// identity vocabulary rather than a second competing request/result authority.
func TestPrivateIdentityContractsAliasRootVocabulary(t *testing.T) {
	t.Parallel()

	var rootRequest factorysessions.IdentityNormalizeRequest
	var privateRequest identity.NormalizeRequest = rootRequest
	_ = privateRequest

	var rootProvider factorysessions.IdentityNormalizeProviderRequest
	var privateProvider identity.NormalizeProviderRequest = rootProvider
	_ = privateProvider

	var rootResolved factorysessions.ResolvedIdentity
	var privateResolved identity.ResolvedIdentity = rootResolved
	rootResolved = privateResolved
	_ = rootResolved
}

func TestWireServiceOwnsNormalizeDiscoverAndResolve(t *testing.T) {
	t.Parallel()

	canonicalFolder := filepath.Clean(filepath.Join(t.TempDir(), "canonical"))
	svc, err := identitywire.NewService(
		func(string) (string, error) { return canonicalFolder, nil },
		func() (string, error) { return "home", nil },
		ownershipDirectories{},
	)
	if err != nil {
		t.Fatalf("identitywire.NewService: %v", err)
	}
	var owned identity.Service = svc
	ctx := context.Background()

	defaultIdentity, err := owned.Normalize(ctx, factorysessions.IdentityNormalizeRequest{
		BackendScopeID: "backend-scope",
		FolderPath:     "submitted-folder",
		Target:         factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
	})
	if err != nil {
		t.Fatalf("Normalize(default): %v", err)
	}
	if defaultIdentity.Reference.Kind != factorysessions.LogicalTargetKindDefault ||
		defaultIdentity.LogicalSessionKeyID == "" {
		t.Fatalf("default identity = %#v", defaultIdentity)
	}

	named, err := owned.Normalize(ctx, factorysessions.IdentityNormalizeRequest{
		BackendScopeID: "backend-scope",
		FolderPath:     "submitted-folder",
		Target:         factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "beta"},
	})
	if err != nil {
		t.Fatalf("Normalize(named): %v", err)
	}
	if named.Reference.NamedTarget != "beta" || named.LogicalSessionKeyID == defaultIdentity.LogicalSessionKeyID {
		t.Fatalf("named identity = %#v", named)
	}

	provider, err := owned.NormalizeProvider(ctx, factorysessions.IdentityNormalizeProviderRequest{
		BackendScopeID: "backend-scope",
		FolderPath:     "submitted-folder",
		Boundary: factorysessions.LogicalTargetProviderBoundary{
			Provider: "cursor", Kind: "workspace", Boundary: "team-alpha",
		},
	})
	if err != nil {
		t.Fatalf("NormalizeProvider: %v", err)
	}
	if provider.Reference.Provider == nil || provider.LogicalSessionKeyID == "" {
		t.Fatalf("provider identity = %#v", provider)
	}

	folder := t.TempDir()
	if err := os.MkdirAll(filepath.Join(folder, "beta"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	discoverSvc, err := identitywire.NewService(
		func(path string) (string, error) { return filepath.Clean(path), nil },
		func() (string, error) { return "home", nil },
		ownershipRealDirectories{},
	)
	if err != nil {
		t.Fatalf("identitywire.NewService(discover): %v", err)
	}
	targets, err := discoverSvc.Discover(ctx, identity.DiscoverRequest{
		FolderPath:        folder,
		WorkstationLoader: ownershipWorkstationLoader{},
		LoadFactory: func(factoryDir string, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			name := filepath.Base(factoryDir)
			return ownershipLoadedFactory{cfg: &factorydefinitions.FactoryConfig{Name: name, Project: name}}, nil
		},
		Logger: zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(targets) < 1 {
		t.Fatalf("Discover targets = %#v, want at least default", targets)
	}
	selected, err := discoverSvc.Select(targets, &factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault})
	if err != nil || selected == nil {
		t.Fatalf("Select(default): selected=%#v err=%v", selected, err)
	}

	registry := sessionregistry.New()
	live := &livesession.LiveSession{
		ID: "runtime-1",
		SessionState: livesession.SessionState{
			FolderPath: "submitted-folder",
		},
		Target: factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "beta"},
	}
	registry.Upsert(live, true)
	if got := owned.Resolve(registry, "runtime-1"); got != live {
		t.Fatalf("Resolve(id) = %#v, want live session", got)
	}
	if got := owned.ResolveLogical(registry, "backend-scope", named.LogicalSessionKeyID); got != live {
		t.Fatalf("ResolveLogical = %#v, want live session", got)
	}
}

type ownershipDirectories struct{}

func (ownershipDirectories) Stat(string) (fs.FileInfo, error)      { return ownershipFileInfo{}, nil }
func (ownershipDirectories) ReadDir(string) ([]fs.DirEntry, error) { return nil, nil }

type ownershipRealDirectories struct{}

func (ownershipRealDirectories) Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }
func (ownershipRealDirectories) ReadDir(path string) ([]fs.DirEntry, error) {
	return os.ReadDir(path)
}

type ownershipFileInfo struct{}

func (ownershipFileInfo) Name() string       { return "folder" }
func (ownershipFileInfo) Size() int64        { return 0 }
func (ownershipFileInfo) Mode() fs.FileMode  { return fs.ModeDir }
func (ownershipFileInfo) ModTime() time.Time { return time.Time{} }
func (ownershipFileInfo) IsDir() bool        { return true }
func (ownershipFileInfo) Sys() any           { return nil }

type ownershipWorkstationLoader struct{}

func (ownershipWorkstationLoader) Load(string) (*factorydefinitions.FactoryWorkstationConfig, error) {
	return nil, nil
}

type ownershipLoadedFactory struct {
	cfg *factorydefinitions.FactoryConfig
}

func (o ownershipLoadedFactory) FactoryDir() string { return "" }
func (o ownershipLoadedFactory) FactoryConfig() *factorydefinitions.FactoryConfig {
	return o.cfg
}
func (ownershipLoadedFactory) Worker(string) (*workerconfig.Config, bool) { return nil, false }
func (ownershipLoadedFactory) Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}
func (ownershipLoadedFactory) RuntimeBaseDir() string { return "" }
func (ownershipLoadedFactory) SetRuntimeBaseDir(string) {}
func (ownershipLoadedFactory) PortableBundledFileReplacements() []factorydefinitions.PortableBundledFileReplacement {
	return nil
}
func (ownershipLoadedFactory) MutateWorkers(func(*workerconfig.Config) error) error { return nil }
