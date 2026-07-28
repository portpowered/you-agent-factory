package internal_test

import (
	"testing"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// peerConstructProvidersRootIdentity builds Inspect/Project/Details inputs using
// only Provider Sessions root contracts and Providers-root SessionRef vocabulary.
func peerConstructProvidersRootIdentity(
	provider, kind, id string,
) (providersessions.InspectRequest, providersessions.ProjectRequest, providers.SessionRef) {
	ref := providers.SessionRef{
		Provider: providers.ID(provider),
		Kind:     kind,
		ID:       id,
	}
	return providersessions.InspectRequest{Session: ref},
		providersessions.ProjectRequest{Session: ref},
		ref
}

// TestRootContracts_ProvidersRootBoundary_IdentityConstruction proves peers can
// construct Inspect, Project, and Details identity through Provider Sessions
// root contracts and Providers-root SessionRef without Workers provider or
// Providers implementation types.
func TestRootContracts_ProvidersRootBoundary_IdentityConstruction(t *testing.T) {
	t.Run("codex", func(t *testing.T) {
		root := writeCodexSessionFixture(t, "boundary-root-contract-codex")
		svc := newServiceForRoots(t, root, "")
		inspectReq, projectReq, ref := peerConstructProvidersRootIdentity(
			string(providers.IDCodex),
			providers.SessionIDKind,
			"boundary-root-contract-codex",
		)

		detail, err := svc.Details("codex", providers.SessionIDKind, ref.ID)
		if err != nil {
			t.Fatalf("Details: %v", err)
		}
		if detail.ProviderSession.Provider != providersessions.ProviderCodex ||
			detail.ProviderSession.Kind != providers.SessionIDKind ||
			detail.ProviderSession.ID != ref.ID {
			t.Fatalf("Detail.ProviderSession = %#v, want codex identity for %q", detail.ProviderSession, ref.ID)
		}

		inspected, err := svc.Inspect(inspectReq)
		if err != nil {
			t.Fatalf("Inspect: %v", err)
		}
		if inspected.Session != ref {
			t.Fatalf("InspectResult.Session = %#v, want %#v", inspected.Session, ref)
		}

		projected, err := svc.Project(projectReq)
		if err != nil {
			t.Fatalf("Project: %v", err)
		}
		if projected.Session != ref {
			t.Fatalf("ProjectResult.Session = %#v, want %#v", projected.Session, ref)
		}
		if projected.Detail.ProviderSession.ID != ref.ID {
			t.Fatalf("ProjectResult.Detail.ProviderSession = %#v, want id %q", projected.Detail.ProviderSession, ref.ID)
		}
	})

	t.Run("cursor", func(t *testing.T) {
		root, sessionID := writeCursorSessionFixture(t)
		svc := newServiceForRoots(t, t.TempDir(), root)
		inspectReq, projectReq, ref := peerConstructProvidersRootIdentity(
			string(providers.IDCursor),
			providers.SessionIDKind,
			sessionID,
		)

		detail, err := svc.Details("cursor", providers.SessionIDKind, sessionID)
		if err != nil {
			t.Fatalf("Details: %v", err)
		}
		if detail.ProviderSession.Provider != providersessions.ProviderCursor ||
			detail.ProviderSession.ID != sessionID {
			t.Fatalf("Detail.ProviderSession = %#v, want cursor %q", detail.ProviderSession, sessionID)
		}

		inspected, err := svc.Inspect(inspectReq)
		if err != nil {
			t.Fatalf("Inspect: %v", err)
		}
		if inspected.Session != ref {
			t.Fatalf("InspectResult.Session = %#v, want %#v", inspected.Session, ref)
		}

		projected, err := svc.Project(projectReq)
		if err != nil {
			t.Fatalf("Project: %v", err)
		}
		if projected.Session != ref {
			t.Fatalf("ProjectResult.Session = %#v, want %#v", projected.Session, ref)
		}
		assertRootCursorProjection(t, projected.Detail)
	})
}
