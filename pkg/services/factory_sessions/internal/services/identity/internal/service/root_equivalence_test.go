package service_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	identitywire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity/wire"
)

// peerRootIdentitySurface is the peer-facing CTR-SES identity slice callable
// shape. It is intentionally defined only with factorysessions root vocabulary
// so the exercise sites below compile as a peer would: no identity subservice
// imports and no other Sessions internals.
type peerRootIdentitySurface struct {
	Normalize func(
		context.Context,
		factorysessions.IdentityNormalizeRequest,
	) (factorysessions.ResolvedIdentity, error)
	NormalizeProvider func(
		context.Context,
		factorysessions.IdentityNormalizeProviderRequest,
	) (factorysessions.ResolvedIdentity, error)
	Select func(
		[]factorysessions.Target,
		*factorysessions.TargetRef,
	) (*factorysessions.Target, error)
}

// newPeerRootIdentitySurface binds the private identity implementation behind
// the peer-facing root vocabulary surface. Construction stays in the owner
// test harness; peer call sites only see factorysessions types.
func newPeerRootIdentitySurface(
	t *testing.T,
	canonicalFolder string,
) peerRootIdentitySurface {
	t.Helper()
	svc, err := identitywire.NewService(
		func(string) (string, error) { return canonicalFolder, nil },
		func() (string, error) { return "home", nil },
		ownershipDirectories{},
	)
	if err != nil {
		t.Fatalf("identitywire.NewService: %v", err)
	}
	return peerRootIdentitySurface{
		Normalize:         svc.Normalize,
		NormalizeProvider: svc.NormalizeProvider,
		Select:            svc.Select,
	}
}

func TestRootIdentityEquivalence_EquivalentTargetsShareLogicalSessionKey(t *testing.T) {
	t.Parallel()

	canonicalFolder := filepath.Clean(filepath.Join(t.TempDir(), "canonical"))
	peer := newPeerRootIdentitySurface(t, canonicalFolder)
	ctx := context.Background()
	folder := "submitted-folder"

	explicitDefault := factorysessions.IdentityNormalizeRequest{
		BackendScopeID: "backend-a",
		FolderPath:     folder,
		Target:         factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
	}
	equivalentEmptyKind := factorysessions.IdentityNormalizeRequest{
		BackendScopeID: "backend-a",
		FolderPath:     folder,
		Target:         factorysessions.TargetRef{},
	}

	first, err := peer.Normalize(ctx, explicitDefault)
	if err != nil {
		t.Fatalf("Normalize(explicit default): %v", err)
	}
	second, err := peer.Normalize(ctx, equivalentEmptyKind)
	if err != nil {
		t.Fatalf("Normalize(equivalent empty kind): %v", err)
	}
	if first.LogicalSessionKeyID == "" || first.LogicalSessionKeyID != second.LogicalSessionKeyID {
		t.Fatalf(
			"equivalent keys = %q and %q, want one shared opaque logical session key",
			first.LogicalSessionKeyID,
			second.LogicalSessionKeyID,
		)
	}

	named := factorysessions.IdentityNormalizeRequest{
		BackendScopeID: "backend-a",
		FolderPath:     folder,
		Target:         factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "beta"},
	}
	namedResolved, err := peer.Normalize(ctx, named)
	if err != nil {
		t.Fatalf("Normalize(named): %v", err)
	}
	if namedResolved.LogicalSessionKeyID == first.LogicalSessionKeyID ||
		namedResolved.Reference.NamedTarget != "beta" {
		t.Fatalf("named identity = %#v, want distinct named key", namedResolved)
	}

	provider := factorysessions.IdentityNormalizeProviderRequest{
		BackendScopeID: "backend-a",
		FolderPath:     folder,
		Boundary: factorysessions.LogicalTargetProviderBoundary{
			Provider: "cursor",
			Kind:     "workspace",
			Boundary: "team-alpha",
		},
	}
	providerResolved, err := peer.NormalizeProvider(ctx, provider)
	if err != nil {
		t.Fatalf("NormalizeProvider: %v", err)
	}
	if providerResolved.LogicalSessionKeyID == "" ||
		providerResolved.LogicalSessionKeyID == first.LogicalSessionKeyID ||
		providerResolved.Reference.Provider == nil {
		t.Fatalf("provider identity = %#v, want distinct provider boundary result", providerResolved)
	}
}

func TestRootIdentityEquivalence_TypedFailuresRemainDistinct(t *testing.T) {
	t.Parallel()

	canonicalFolder := filepath.Clean(filepath.Join(t.TempDir(), "canonical"))
	peer := newPeerRootIdentitySurface(t, canonicalFolder)
	ctx := context.Background()
	folder := "submitted-folder"

	// Real private-impl mapping (not the CTR-SES vocabulary demo fake table):
	// default+name → Ambiguous; unsupported kind → Invalid.
	cases := []struct {
		name    string
		request factorysessions.IdentityNormalizeRequest
		want    error
	}{
		{
			name: "required",
			request: factorysessions.IdentityNormalizeRequest{
				FolderPath: folder,
				Target:     factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
			},
			want: factorysessions.ErrLogicalTargetRequired,
		},
		{
			name: "ambiguous_default_with_name",
			request: factorysessions.IdentityNormalizeRequest{
				BackendScopeID: "backend-a",
				FolderPath:     folder,
				Target: factorysessions.TargetRef{
					Kind: factorysessions.TargetKindDefault,
					Name: "beta",
				},
			},
			want: factorysessions.ErrLogicalTargetAmbiguous,
		},
		{
			name: "invalid_unsupported_kind",
			request: factorysessions.IdentityNormalizeRequest{
				BackendScopeID: "backend-a",
				FolderPath:     folder,
				Target:         factorysessions.TargetRef{Kind: "unsupported"},
			},
			want: factorysessions.ErrLogicalTargetInvalid,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := peer.Normalize(ctx, tc.request)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Normalize error = %v, want %v", err, tc.want)
			}
		})
	}

	targets := []factorysessions.Target{
		{Ref: factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}, Label: "default"},
	}
	_, err := peer.Select(targets, &factorysessions.TargetRef{
		Kind: factorysessions.TargetKindNamed,
		Name: "gone",
	})
	if err == nil {
		t.Fatal("Select(missing named) error = nil, want not-found outcome")
	}
	// Peer distinguishes not-found from normalize typed failures without
	// importing Sessions internals: it is a non-nil error that is not the
	// published required/invalid/ambiguous normalize sentinels.
	if errors.Is(err, factorysessions.ErrLogicalTargetRequired) ||
		errors.Is(err, factorysessions.ErrLogicalTargetInvalid) ||
		errors.Is(err, factorysessions.ErrLogicalTargetAmbiguous) {
		t.Fatalf("Select not-found collapsed into normalize sentinel: %v", err)
	}

	if errors.Is(factorysessions.ErrLogicalTargetInvalid, factorysessions.ErrLogicalTargetAmbiguous) ||
		errors.Is(factorysessions.ErrLogicalTargetInvalid, factorysessions.ErrLogicalTargetNotFound) ||
		errors.Is(factorysessions.ErrLogicalTargetAmbiguous, factorysessions.ErrLogicalTargetNotFound) ||
		errors.Is(factorysessions.ErrLogicalTargetRequired, factorysessions.ErrLogicalTargetNotFound) {
		t.Fatal("identity typed failures must remain distinguishable root sentinels")
	}
}
