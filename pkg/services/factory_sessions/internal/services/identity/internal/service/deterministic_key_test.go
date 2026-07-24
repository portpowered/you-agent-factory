package service_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	identitywire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity/wire"
)

func TestIdentityPath_EquivalentTargetsShareDeterministicLogicalSessionKey(t *testing.T) {
	t.Parallel()

	canonicalFolder := filepath.Clean(filepath.Join(t.TempDir(), "canonical"))
	svc, err := identitywire.NewService(identity.Dependencies{
		ResolveSymlinks: func(string) (string, error) { return canonicalFolder, nil },
		ResolveHome:     func() (string, error) { return "home", nil },
		Directories:     ownershipDirectories{},
	})
	if err != nil {
		t.Fatalf("identitywire.NewService: %v", err)
	}
	ctx := context.Background()

	first, err := svc.Normalize(ctx, factorysessions.IdentityNormalizeRequest{
		BackendScopeID: "  backend-scope  ",
		FolderPath:     "submitted-folder",
		Target:         factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
	})
	if err != nil {
		t.Fatalf("Normalize(whitespace scope): %v", err)
	}
	if !logicaltarget.IsLogicalSessionKeyID(first.LogicalSessionKeyID) {
		t.Fatalf("logical session key = %q, want opaque lsk- format", first.LogicalSessionKeyID)
	}

	equivalentFolderPresentation, err := svc.Normalize(ctx, factorysessions.IdentityNormalizeRequest{
		BackendScopeID: "backend-scope",
		FolderPath:     "submitted-folder" + string(filepath.Separator),
		Target:         factorysessions.TargetRef{},
	})
	if err != nil {
		t.Fatalf("Normalize(equivalent folder presentation): %v", err)
	}
	if equivalentFolderPresentation.LogicalSessionKeyID != first.LogicalSessionKeyID {
		t.Fatalf(
			"equivalent targets produced different keys: %q vs %q",
			first.LogicalSessionKeyID,
			equivalentFolderPresentation.LogicalSessionKeyID,
		)
	}

	secondPass, err := svc.Normalize(ctx, factorysessions.IdentityNormalizeRequest{
		BackendScopeID: "backend-scope",
		FolderPath:     "alias-folder",
		Target:         factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
	})
	if err != nil {
		t.Fatalf("Normalize(second pass): %v", err)
	}
	if secondPass.LogicalSessionKeyID != first.LogicalSessionKeyID {
		t.Fatalf(
			"repeat normalize changed key: %q vs %q",
			first.LogicalSessionKeyID,
			secondPass.LogicalSessionKeyID,
		)
	}
}

func TestIdentityPath_DistinctBoundariesProduceDistinctLogicalSessionKeys(t *testing.T) {
	t.Parallel()

	canonicalFolder := filepath.Clean(filepath.Join(t.TempDir(), "canonical"))
	otherFolder := filepath.Clean(filepath.Join(t.TempDir(), "other"))
	ctx := context.Background()

	resolveTo := func(canonical string) identity.Service {
		svc, err := identitywire.NewService(identity.Dependencies{
			ResolveSymlinks: func(string) (string, error) { return canonical, nil },
			ResolveHome:     func() (string, error) { return "home", nil },
			Directories:     ownershipDirectories{},
		})
		if err != nil {
			t.Fatalf("identitywire.NewService: %v", err)
		}
		return svc
	}

	defaultIdentity, err := resolveTo(canonicalFolder).Normalize(ctx, factorysessions.IdentityNormalizeRequest{
		BackendScopeID: "backend-scope",
		FolderPath:     "submitted-folder",
		Target:         factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
	})
	if err != nil {
		t.Fatalf("Normalize(default): %v", err)
	}

	otherScope, err := resolveTo(canonicalFolder).Normalize(ctx, factorysessions.IdentityNormalizeRequest{
		BackendScopeID: "other-scope",
		FolderPath:     "submitted-folder",
		Target:         factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
	})
	if err != nil {
		t.Fatalf("Normalize(other scope): %v", err)
	}

	otherFolderIdentity, err := resolveTo(otherFolder).Normalize(ctx, factorysessions.IdentityNormalizeRequest{
		BackendScopeID: "backend-scope",
		FolderPath:     "submitted-folder",
		Target:         factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
	})
	if err != nil {
		t.Fatalf("Normalize(other folder): %v", err)
	}

	named, err := resolveTo(canonicalFolder).Normalize(ctx, factorysessions.IdentityNormalizeRequest{
		BackendScopeID: "backend-scope",
		FolderPath:     "submitted-folder",
		Target:         factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "beta"},
	})
	if err != nil {
		t.Fatalf("Normalize(named): %v", err)
	}

	provider, err := resolveTo(canonicalFolder).NormalizeProvider(ctx, factorysessions.IdentityNormalizeProviderRequest{
		BackendScopeID: "backend-scope",
		FolderPath:     "submitted-folder",
		Boundary: factorysessions.LogicalTargetProviderBoundary{
			Provider: "cursor", Kind: "workspace", Boundary: "team-alpha",
		},
	})
	if err != nil {
		t.Fatalf("NormalizeProvider: %v", err)
	}

	keys := map[string]string{
		"default":      defaultIdentity.LogicalSessionKeyID,
		"other-scope":  otherScope.LogicalSessionKeyID,
		"other-folder": otherFolderIdentity.LogicalSessionKeyID,
		"named":        named.LogicalSessionKeyID,
		"provider":     provider.LogicalSessionKeyID,
	}
	unique := make(map[string]string, len(keys))
	for label, key := range keys {
		if !logicaltarget.IsLogicalSessionKeyID(key) {
			t.Fatalf("%s key = %q, want opaque lsk- format", label, key)
		}
		if prior, exists := unique[key]; exists {
			t.Fatalf("boundary collision: %s and %s both produced %q", prior, label, key)
		}
		unique[key] = label
	}
	if len(unique) != 5 {
		t.Fatalf("derived %d unique keys, want 5 distinct boundaries", len(unique))
	}
}

func TestIdentityPath_LogicalSessionKeyDoesNotDependOnLiveSessionUUID(t *testing.T) {
	t.Parallel()

	canonicalFolder := filepath.Clean(filepath.Join(t.TempDir(), "canonical"))
	svc, err := identitywire.NewService(identity.Dependencies{
		ResolveSymlinks: func(string) (string, error) { return canonicalFolder, nil },
		ResolveHome:     func() (string, error) { return "home", nil },
		Directories:     ownershipDirectories{},
	})
	if err != nil {
		t.Fatalf("identitywire.NewService: %v", err)
	}

	request := factorysessions.IdentityNormalizeRequest{
		BackendScopeID: "backend-scope",
		FolderPath:     "submitted-folder",
		Target:         factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "beta"},
	}
	first, err := svc.Normalize(context.Background(), request)
	if err != nil {
		t.Fatalf("Normalize(first): %v", err)
	}
	second, err := svc.Normalize(context.Background(), request)
	if err != nil {
		t.Fatalf("Normalize(second): %v", err)
	}
	if first.LogicalSessionKeyID != second.LogicalSessionKeyID {
		t.Fatalf(
			"key changed across independent normalize calls: %q vs %q",
			first.LogicalSessionKeyID,
			second.LogicalSessionKeyID,
		)
	}
	for _, liveUUID := range []string{
		"550e8400-e29b-41d4-a716-446655440000",
		"6ba7b810-9dad-11d1-80b4-00c04fd430c8",
	} {
		if strings.Contains(first.LogicalSessionKeyID, liveUUID) {
			t.Fatalf("logical session key %q embeds live session UUID %q", first.LogicalSessionKeyID, liveUUID)
		}
	}
}
