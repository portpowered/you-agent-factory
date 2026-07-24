package service

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
)

const testBackendScope = "backend-scope"

func TestServiceNormalizesDefaultNamedFolderAndProviderTargets(t *testing.T) {
	t.Parallel()

	canonicalFolder := filepath.Clean(filepath.Join(t.TempDir(), "canonical"))
	svc := newTestService(canonicalFolder)
	ctx := context.Background()

	defaultIdentity, err := svc.Normalize(ctx, identity.NormalizeRequest{
		BackendScopeID: "  " + testBackendScope + "  ",
		FolderPath:     "submitted-folder",
		Target:         factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
	})
	if err != nil {
		t.Fatalf("Normalize(default): %v", err)
	}
	if defaultIdentity.Reference.FolderPath != canonicalFolder ||
		defaultIdentity.Reference.Kind != factorysessions.LogicalTargetKindDefault ||
		!logicaltarget.IsLogicalSessionKeyID(defaultIdentity.LogicalSessionKeyID) {
		t.Fatalf("default identity = %#v", defaultIdentity)
	}

	equivalent, err := svc.Normalize(ctx, identity.NormalizeRequest{
		BackendScopeID: testBackendScope,
		FolderPath:     "equivalent-folder",
		Target:         factorysessions.TargetRef{},
	})
	if err != nil {
		t.Fatalf("Normalize(equivalent): %v", err)
	}
	if equivalent.LogicalSessionKeyID != defaultIdentity.LogicalSessionKeyID {
		t.Fatalf("equivalent key = %q, want %q", equivalent.LogicalSessionKeyID, defaultIdentity.LogicalSessionKeyID)
	}

	named, err := svc.Normalize(ctx, identity.NormalizeRequest{
		BackendScopeID: testBackendScope,
		FolderPath:     "submitted-folder",
		Target:         factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "  beta  "},
	})
	if err != nil {
		t.Fatalf("Normalize(named): %v", err)
	}
	if named.Reference.NamedTarget != "beta" || named.LogicalSessionKeyID == defaultIdentity.LogicalSessionKeyID {
		t.Fatalf("named identity = %#v", named)
	}

	provider, err := svc.NormalizeProvider(ctx, identity.NormalizeProviderRequest{
		BackendScopeID: testBackendScope,
		FolderPath:     "submitted-folder",
		Boundary: factorysessions.LogicalTargetProviderBoundary{
			Provider: " Cursor ", Kind: " Workspace ", Boundary: " team-alpha ",
		},
	})
	if err != nil {
		t.Fatalf("NormalizeProvider: %v", err)
	}
	if provider.Reference.Provider == nil || provider.Reference.Provider.Provider != "cursor" ||
		provider.Reference.Provider.Kind != "workspace" || provider.Reference.Provider.Boundary != "team-alpha" {
		t.Fatalf("provider identity = %#v", provider)
	}
}

func TestServiceRejectsMalformedAndAmbiguousTargets(t *testing.T) {
	t.Parallel()

	svc := newTestService(filepath.Clean(t.TempDir()))
	tests := []identity.NormalizeRequest{
		{FolderPath: "folder", Target: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}},
		{BackendScopeID: testBackendScope, Target: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}},
		{BackendScopeID: testBackendScope, FolderPath: "folder", Target: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault, Name: "beta"}},
		{BackendScopeID: testBackendScope, FolderPath: "folder", Target: factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed}},
		{BackendScopeID: testBackendScope, FolderPath: "folder", Target: factorysessions.TargetRef{Kind: "unsupported"}},
	}
	for _, request := range tests {
		if _, err := svc.Normalize(context.Background(), request); err == nil {
			t.Fatalf("Normalize(%#v) error = nil", request)
		}
	}
	_, err := svc.NormalizeProvider(context.Background(), identity.NormalizeProviderRequest{
		BackendScopeID: testBackendScope, FolderPath: "folder",
		Boundary: factorysessions.LogicalTargetProviderBoundary{Provider: "cursor", Kind: "workspace", Boundary: "sk-secret"},
	})
	if !errors.Is(err, factorysessions.ErrLogicalTargetInvalid) {
		t.Fatalf("NormalizeProvider(secret) error = %v, want invalid target", err)
	}
}

func TestServiceResolvesRestartedSessionByLogicalIdentity(t *testing.T) {
	t.Parallel()

	canonicalFolder := filepath.Clean(t.TempDir())
	svc := newTestService(canonicalFolder)
	registry := sessionregistry.New()
	restarted := &livesession.LiveSession{
		ID: "new-runtime-id", SessionState: livesession.SessionState{FolderPath: "submitted-folder"},
		Target: factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "beta"},
	}
	registry.Upsert(restarted, true)
	resolved, err := svc.Normalize(context.Background(), identity.NormalizeRequest{
		BackendScopeID: testBackendScope, FolderPath: restarted.FolderPath, Target: restarted.Target,
	})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got := svc.ResolveLogical(registry, testBackendScope, resolved.LogicalSessionKeyID); got != restarted {
		t.Fatalf("ResolveLogical() = %#v, want restarted session", got)
	}
	if got := svc.ResolveLogical(registry, "other-backend", resolved.LogicalSessionKeyID); got != nil {
		t.Fatalf("ResolveLogical(other backend) = %#v, want nil", got)
	}
}

func newTestService(canonicalFolder string) *Service {
	return New(identity.Dependencies{
		ResolveSymlinks: func(string) (string, error) { return canonicalFolder, nil },
		ResolveHome:     func() (string, error) { return "home", nil },
		Directories:     testDirectories{},
	})
}

type testDirectories struct{}

func (testDirectories) Stat(string) (fs.FileInfo, error)      { return testFileInfo{}, nil }
func (testDirectories) ReadDir(string) ([]fs.DirEntry, error) { return nil, nil }

type testFileInfo struct{}

func (testFileInfo) Name() string       { return "folder" }
func (testFileInfo) Size() int64        { return 0 }
func (testFileInfo) Mode() fs.FileMode  { return fs.ModeDir }
func (testFileInfo) ModTime() time.Time { return time.Time{} }
func (testFileInfo) IsDir() bool        { return true }
func (testFileInfo) Sys() any           { return nil }
