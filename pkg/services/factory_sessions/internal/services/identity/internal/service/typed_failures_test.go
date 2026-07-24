package service_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	identitywire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity/wire"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionvalidation"
)

func TestIdentityPath_TypedNormalizeFailuresAreDistinct(t *testing.T) {
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

	cases := []struct {
		name    string
		request factorysessions.IdentityNormalizeRequest
		want    error
		reason  string
		field   string
	}{
		{
			name: "required_backend_scope",
			request: factorysessions.IdentityNormalizeRequest{
				FolderPath: "submitted-folder",
				Target:     factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
			},
			want:   factorysessions.ErrLogicalTargetRequired,
			reason: factorysessions.LogicalTargetReasonRequired,
			field:  "backendScopeId",
		},
		{
			name: "ambiguous_default_with_name",
			request: factorysessions.IdentityNormalizeRequest{
				BackendScopeID: "backend-scope",
				FolderPath:     "submitted-folder",
				Target: factorysessions.TargetRef{
					Kind: factorysessions.TargetKindDefault,
					Name: "beta",
				},
			},
			want:   factorysessions.ErrLogicalTargetAmbiguous,
			reason: factorysessions.LogicalTargetReasonAmbiguousTarget,
			field:  "target",
		},
		{
			name: "invalid_unsupported_kind",
			request: factorysessions.IdentityNormalizeRequest{
				BackendScopeID: "backend-scope",
				FolderPath:     "submitted-folder",
				Target:         factorysessions.TargetRef{Kind: "unsupported"},
			},
			want:   factorysessions.ErrLogicalTargetInvalid,
			reason: factorysessions.LogicalTargetReasonInvalidTarget,
			field:  "target.kind",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := svc.Normalize(ctx, tc.request)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Normalize error = %v, want %v", err, tc.want)
			}
			reason, field, ok := logicaltarget.ValidationReasonFromError(err)
			if !ok || reason != tc.reason || field != tc.field {
				t.Fatalf("validation = (%q, %q, %v), want (%q, %q, true)", reason, field, ok, tc.reason, tc.field)
			}
		})
	}

	if errors.Is(factorysessions.ErrLogicalTargetRequired, factorysessions.ErrLogicalTargetInvalid) ||
		errors.Is(factorysessions.ErrLogicalTargetRequired, factorysessions.ErrLogicalTargetAmbiguous) ||
		errors.Is(factorysessions.ErrLogicalTargetInvalid, factorysessions.ErrLogicalTargetAmbiguous) ||
		errors.Is(factorysessions.ErrLogicalTargetInvalid, factorysessions.ErrLogicalTargetNotFound) ||
		errors.Is(factorysessions.ErrLogicalTargetAmbiguous, factorysessions.ErrLogicalTargetNotFound) {
		t.Fatal("CTR-SES identity typed failures must remain distinguishable sentinels")
	}
}

func TestIdentityPath_SelectAndResolveReturnTypedOrEmptyNotFound(t *testing.T) {
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
	var owned identity.Service = svc

	targets := []factorysessions.Target{
		{Ref: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}, Label: "default"},
	}
	_, err = owned.Select(targets, &factorysessions.TargetRef{
		Kind: factorysessions.TargetKindNamed,
		Name: "missing",
	})
	if err == nil {
		t.Fatal("Select(missing named) error = nil, want typed target_not_found")
	}
	reason, field, ok := sessionvalidation.ReasonFromError(err)
	if !ok || reason != factorysessions.ValidationReasonTargetNotFound || field != "target.name" {
		t.Fatalf("Select validation = (%q, %q, %v), want target_not_found/target.name", reason, field, ok)
	}

	registry := sessionregistry.New()
	registry.Upsert(&livesession.LiveSession{
		ID: "runtime-present",
		SessionState: livesession.SessionState{
			FolderPath: "submitted-folder",
		},
		Target: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
	}, true)

	if got := owned.Resolve(registry, "runtime-absent"); got != nil {
		t.Fatalf("Resolve(missing id) = %#v, want nil empty/not-found", got)
	}
	if got := owned.ResolveLogical(registry, "backend-scope", "lsk-ffffffffffffffffffffffffffffffff"); got != nil {
		t.Fatalf("ResolveLogical(missing key) = %#v, want nil empty/not-found", got)
	}
}
