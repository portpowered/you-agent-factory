package factorysessions_test

import (
	"errors"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// peerIdentitySurfaceFake is a peer-owned stand-in for identity/target
// resolution. It compiles against only the published Factory Sessions root
// identity vocabulary and never imports factory_sessions/internal.
type peerIdentitySurfaceFake struct {
	bySignature map[string]factorysessions.ResolvedIdentity
	failures    map[string]error
}

func newPeerIdentitySurfaceFake() *peerIdentitySurfaceFake {
	return &peerIdentitySurfaceFake{
		bySignature: make(map[string]factorysessions.ResolvedIdentity),
		failures:    make(map[string]error),
	}
}

func identityRequestSignature(request factorysessions.IdentityNormalizeRequest) string {
	return "target|" + request.BackendScopeID + "|" + request.FolderPath + "|" +
		string(request.Target.Kind) + "|" + request.Target.Name
}

func identityProviderSignature(request factorysessions.IdentityNormalizeProviderRequest) string {
	boundary := request.Boundary
	return "provider|" + request.BackendScopeID + "|" + request.FolderPath + "|" +
		boundary.Provider + "|" + boundary.Kind + "|" + boundary.Boundary
}

func (fake *peerIdentitySurfaceFake) Normalize(
	request factorysessions.IdentityNormalizeRequest,
) (factorysessions.ResolvedIdentity, error) {
	signature := identityRequestSignature(request)
	if err, ok := fake.failures[signature]; ok {
		return factorysessions.ResolvedIdentity{}, err
	}
	if resolved, ok := fake.bySignature[signature]; ok {
		return resolved, nil
	}
	return factorysessions.ResolvedIdentity{}, factorysessions.ErrLogicalTargetNotFound
}

func (fake *peerIdentitySurfaceFake) NormalizeProvider(
	request factorysessions.IdentityNormalizeProviderRequest,
) (factorysessions.ResolvedIdentity, error) {
	signature := identityProviderSignature(request)
	if err, ok := fake.failures[signature]; ok {
		return factorysessions.ResolvedIdentity{}, err
	}
	if resolved, ok := fake.bySignature[signature]; ok {
		return resolved, nil
	}
	return factorysessions.ResolvedIdentity{}, factorysessions.ErrLogicalTargetNotFound
}

func defaultIdentityRequest(folder string, target factorysessions.TargetRef) factorysessions.IdentityNormalizeRequest {
	return factorysessions.IdentityNormalizeRequest{
		BackendScopeID: "backend-a",
		FolderPath:     folder,
		Target:         target,
	}
}

func seedDefaultResolvedIdentity(fake *peerIdentitySurfaceFake, folder, key string, requests ...factorysessions.IdentityNormalizeRequest) {
	resolved := factorysessions.ResolvedIdentity{
		Reference: factorysessions.CanonicalLogicalTargetReference{
			BackendScopeID: "backend-a",
			FolderPath:     folder,
			Kind:           factorysessions.LogicalTargetKindDefault,
		},
		LogicalSessionKeyID: key,
		RuntimeTarget: factorysessions.RuntimeLogicalTarget{
			FolderPath: folder,
			Kind:       string(factorysessions.LogicalTargetKindDefault),
		},
	}
	for _, request := range requests {
		fake.bySignature[identityRequestSignature(request)] = resolved
	}
}

func seedNamedResolvedIdentity(fake *peerIdentitySurfaceFake, request factorysessions.IdentityNormalizeRequest, key string) {
	fake.bySignature[identityRequestSignature(request)] = factorysessions.ResolvedIdentity{
		Reference: factorysessions.CanonicalLogicalTargetReference{
			BackendScopeID: request.BackendScopeID,
			FolderPath:     request.FolderPath,
			Kind:           factorysessions.LogicalTargetKindNamed,
			NamedTarget:    request.Target.Name,
		},
		LogicalSessionKeyID: key,
		RuntimeTarget: factorysessions.RuntimeLogicalTarget{
			FolderPath:  request.FolderPath,
			Kind:        string(factorysessions.LogicalTargetKindNamed),
			NamedTarget: ptr(request.Target.Name),
		},
	}
}

func seedProviderResolvedIdentity(fake *peerIdentitySurfaceFake, request factorysessions.IdentityNormalizeProviderRequest, key string) {
	boundary := request.Boundary
	fake.bySignature[identityProviderSignature(request)] = factorysessions.ResolvedIdentity{
		Reference: factorysessions.CanonicalLogicalTargetReference{
			BackendScopeID: request.BackendScopeID,
			FolderPath:     request.FolderPath,
			Kind:           factorysessions.LogicalTargetKindProvider,
			Provider: &factorysessions.LogicalTargetProviderBoundary{
				Provider: boundary.Provider,
				Kind:     boundary.Kind,
				Boundary: boundary.Boundary,
			},
		},
		LogicalSessionKeyID: key,
		RuntimeTarget: factorysessions.RuntimeLogicalTarget{
			FolderPath: request.FolderPath,
			Kind:       string(factorysessions.LogicalTargetKindProvider),
			ProviderBoundary: &factorysessions.RuntimeLogicalProviderBoundary{
				Provider: boundary.Provider,
				Kind:     boundary.Kind,
				Boundary: boundary.Boundary,
			},
		},
	}
}

func TestIdentityRootContract_EquivalentTargetsShareLogicalSessionKey(t *testing.T) {
	t.Parallel()

	fake := newPeerIdentitySurfaceFake()
	sharedKey := "lsk-equivalent-default"
	folder := "/workspace/factories/demo"
	explicitDefault := defaultIdentityRequest(folder, factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault})
	equivalentEmptyKind := defaultIdentityRequest(folder, factorysessions.TargetRef{})
	seedDefaultResolvedIdentity(fake, folder, sharedKey, explicitDefault, equivalentEmptyKind)

	named := defaultIdentityRequest(folder, factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "beta"})
	seedNamedResolvedIdentity(fake, named, "lsk-named-beta")
	provider := factorysessions.IdentityNormalizeProviderRequest{
		BackendScopeID: "backend-a",
		FolderPath:     folder,
		Boundary: factorysessions.LogicalTargetProviderBoundary{
			Provider: "cursor",
			Kind:     "workspace",
			Boundary: "team-alpha",
		},
	}
	seedProviderResolvedIdentity(fake, provider, "lsk-provider-team-alpha")

	first, err := fake.Normalize(explicitDefault)
	if err != nil {
		t.Fatalf("Normalize(explicit default): %v", err)
	}
	second, err := fake.Normalize(equivalentEmptyKind)
	if err != nil {
		t.Fatalf("Normalize(equivalent empty kind): %v", err)
	}
	if first.LogicalSessionKeyID != second.LogicalSessionKeyID || first.LogicalSessionKeyID != sharedKey {
		t.Fatalf("equivalent keys = %q and %q, want %q", first.LogicalSessionKeyID, second.LogicalSessionKeyID, sharedKey)
	}

	namedResolved, err := fake.Normalize(named)
	if err != nil {
		t.Fatalf("Normalize(named): %v", err)
	}
	if namedResolved.LogicalSessionKeyID == sharedKey || namedResolved.Reference.NamedTarget != "beta" {
		t.Fatalf("named identity = %#v, want distinct named key", namedResolved)
	}

	providerResolved, err := fake.NormalizeProvider(provider)
	if err != nil {
		t.Fatalf("NormalizeProvider: %v", err)
	}
	if providerResolved.LogicalSessionKeyID == "" || providerResolved.Reference.Provider == nil {
		t.Fatalf("provider identity = %#v, want provider boundary result", providerResolved)
	}
}

func TestIdentityRootContract_TypedFailuresAreDistinct(t *testing.T) {
	t.Parallel()

	fake := newPeerIdentitySurfaceFake()
	malformed := factorysessions.IdentityNormalizeRequest{
		BackendScopeID: "backend-a",
		FolderPath:     "/workspace/factories/demo",
		Target: factorysessions.TargetRef{
			Kind: factorysessions.TargetKindDefault,
			Name: "beta",
		},
	}
	ambiguous := factorysessions.IdentityNormalizeRequest{
		BackendScopeID: "backend-a",
		FolderPath:     "/workspace/factories/demo",
		Target:         factorysessions.TargetRef{Kind: "unsupported"},
	}
	missing := factorysessions.IdentityNormalizeRequest{
		BackendScopeID: "backend-a",
		FolderPath:     "/workspace/factories/missing",
		Target:         factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "gone"},
	}

	fake.failures[identityRequestSignature(malformed)] = factorysessions.ErrLogicalTargetInvalid
	fake.failures[identityRequestSignature(ambiguous)] = factorysessions.ErrLogicalTargetAmbiguous

	cases := []struct {
		name    string
		request factorysessions.IdentityNormalizeRequest
		want    error
	}{
		{name: "malformed", request: malformed, want: factorysessions.ErrLogicalTargetInvalid},
		{name: "ambiguous", request: ambiguous, want: factorysessions.ErrLogicalTargetAmbiguous},
		{name: "not_found", request: missing, want: factorysessions.ErrLogicalTargetNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := fake.Normalize(tc.request)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Normalize error = %v, want %v", err, tc.want)
			}
		})
	}

	if errors.Is(factorysessions.ErrLogicalTargetInvalid, factorysessions.ErrLogicalTargetAmbiguous) ||
		errors.Is(factorysessions.ErrLogicalTargetInvalid, factorysessions.ErrLogicalTargetNotFound) ||
		errors.Is(factorysessions.ErrLogicalTargetAmbiguous, factorysessions.ErrLogicalTargetNotFound) {
		t.Fatal("identity typed failures must remain distinguishable sentinels")
	}
}

func ptr(value string) *string { return &value }
