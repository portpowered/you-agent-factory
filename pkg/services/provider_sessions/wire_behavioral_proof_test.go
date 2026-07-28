package providersessions_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionswire "github.com/portpowered/infinite-you/pkg/services/provider_sessions/wire"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// TestWireBehavioralProof_PublishedRootPreservesObservables constructs Provider
// Sessions exclusively through provider_sessions/wire and proves Details, Inspect,
// and Project preserve observable outcomes for Codex- and Cursor-backed reader
// fixtures on the published providersessions.Service peer surface.
func TestWireBehavioralProof_PublishedRootPreservesObservables(t *testing.T) {
	t.Run("codex success", func(t *testing.T) {
		root := writeCodexSessionFixture(t, "wire-behavioral-codex")
		svc := wireServiceForRoots(t, root, "")

		ref := providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "wire-behavioral-codex",
		}

		detail, err := svc.Details("codex", providers.SessionIDKind, ref.ID)
		if err != nil {
			t.Fatalf("Details: %v", err)
		}
		if detail.ProviderSession.Provider != providersessions.ProviderCodex ||
			detail.ProviderSession.ID != ref.ID ||
			detail.Source.RelativePath == "" {
			t.Fatalf("Details detail = %#v, want codex session with source metadata", detail)
		}

		inspected, err := svc.Inspect(providersessions.InspectRequest{Session: ref})
		if err != nil {
			t.Fatalf("Inspect: %v", err)
		}
		if inspected.Session != ref {
			t.Fatalf("InspectResult.Session = %#v, want %#v", inspected.Session, ref)
		}
		if inspected.Source.RelativePath != detail.Source.RelativePath ||
			inspected.Source.SizeBytes != detail.Source.SizeBytes {
			t.Fatalf("Inspect source = %#v, want Details source %#v", inspected.Source, detail.Source)
		}

		projected, err := svc.Project(providersessions.ProjectRequest{Session: ref})
		if err != nil {
			t.Fatalf("Project: %v", err)
		}
		if projected.Session != ref {
			t.Fatalf("ProjectResult.Session = %#v, want %#v", projected.Session, ref)
		}
		if projected.Detail.ProviderSession != detail.ProviderSession ||
			projected.Detail.Source.RelativePath != detail.Source.RelativePath {
			t.Fatalf("Project detail = %#v, want Details detail %#v", projected.Detail, detail)
		}
	})

	t.Run("cursor success", func(t *testing.T) {
		root, sessionID := writeCursorSessionFixture(t)
		svc := wireServiceForRoots(t, t.TempDir(), root)

		ref := providers.SessionRef{
			Provider: providers.IDCursor,
			Kind:     providers.SessionIDKind,
			ID:       sessionID,
		}

		detail, err := svc.Details("cursor", providers.SessionIDKind, sessionID)
		if err != nil {
			t.Fatalf("Details: %v", err)
		}
		if detail.ProviderSession.Provider != providersessions.ProviderCursor {
			t.Fatalf("Details ProviderSession = %#v, want Cursor", detail.ProviderSession)
		}
		assertRootCursorProjection(t, detail)

		inspected, err := svc.Inspect(providersessions.InspectRequest{Session: ref})
		if err != nil {
			t.Fatalf("Inspect: %v", err)
		}
		if inspected.Session != ref {
			t.Fatalf("InspectResult.Session = %#v, want %#v", inspected.Session, ref)
		}
		if inspected.Source.RelativePath == "" {
			t.Fatalf("InspectResult.Source.RelativePath empty")
		}

		projected, err := svc.Project(providersessions.ProjectRequest{Session: ref})
		if err != nil {
			t.Fatalf("Project: %v", err)
		}
		if projected.Session != ref {
			t.Fatalf("ProjectResult.Session = %#v, want %#v", projected.Session, ref)
		}
		assertRootCursorProjection(t, projected.Detail)
	})

	t.Run("typed failures", func(t *testing.T) {
		codexRoot := writeCodexSessionFixture(t, "wire-behavioral-typed-codex")
		cursorRoot, cursorSessionID := writeCursorSessionFixture(t)
		svc := wireServiceForRoots(t, codexRoot, cursorRoot)

		codexRef := providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "wire-behavioral-typed-codex",
		}
		cursorRef := providers.SessionRef{
			Provider: providers.IDCursor,
			Kind:     providers.SessionIDKind,
			ID:       cursorSessionID,
		}

		if _, err := svc.Details("openai", providers.SessionIDKind, "session-1"); !errors.Is(err, providersessions.ErrUnsupportedProvider) {
			t.Fatalf("Details unsupported provider = %v, want ErrUnsupportedProvider", err)
		}
		if _, err := svc.Inspect(providersessions.InspectRequest{Session: providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     "path",
			ID:       "session-1",
		}}); !errors.Is(err, providersessions.ErrUnsupportedKind) {
			t.Fatalf("Inspect unsupported kind = %v, want ErrUnsupportedKind", err)
		}
		if _, err := svc.Project(providersessions.ProjectRequest{Session: providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "../secret",
		}}); !errors.Is(err, providersessions.ErrInvalidIdentifier) {
			t.Fatalf("Project invalid identifier = %v, want ErrInvalidIdentifier", err)
		}
		if _, err := svc.Details("codex", providers.SessionIDKind, "missing-session"); !errors.Is(err, providersessions.ErrSessionNotFound) {
			t.Fatalf("Details missing codex session = %v, want ErrSessionNotFound", err)
		}
		if _, err := svc.Inspect(providersessions.InspectRequest{Session: providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "missing-session",
		}}); !errors.Is(err, providersessions.ErrSessionNotFound) {
			t.Fatalf("Inspect missing codex session = %v, want ErrSessionNotFound", err)
		}
		if _, err := svc.Project(providersessions.ProjectRequest{Session: codexRef}); err != nil {
			t.Fatalf("Project codex success sanity = %v", err)
		}
		if _, err := svc.Details("cursor", providers.SessionIDKind, "missing-cursor-session"); !errors.Is(err, providersessions.ErrSessionNotFound) {
			t.Fatalf("Details missing cursor session = %v, want ErrSessionNotFound", err)
		}
		if _, err := svc.Inspect(providersessions.InspectRequest{Session: providers.SessionRef{
			Provider: providers.IDCursor,
			Kind:     providers.SessionIDKind,
			ID:       "missing-cursor-session",
		}}); !errors.Is(err, providersessions.ErrSessionNotFound) {
			t.Fatalf("Inspect missing cursor session = %v, want ErrSessionNotFound", err)
		}
		if _, err := svc.Project(providersessions.ProjectRequest{Session: cursorRef}); err != nil {
			t.Fatalf("Project cursor success sanity = %v", err)
		}
	})
}

func wireServiceForRoots(t *testing.T, codexRoot, cursorRoot string) providersessions.Service {
	t.Helper()
	service, err := providersessionswire.NewForRoots(
		platformfilesystem.Local{},
		providersessions.CodexWalkDirectory(filepath.WalkDir),
		providersessions.CodexResolveSymlinks(filepath.EvalSymlinks),
		providersessions.CursorWalkDirectory(filepath.WalkDir),
		providersessions.CursorResolveSymlinks(filepath.EvalSymlinks),
		providersessions.CursorOpenSQLDatabase(sql.Open),
		codexRoot,
		cursorRoot,
	)
	if err != nil {
		t.Fatalf("providersessionswire.NewForRoots: %v", err)
	}
	var root providersessions.Service = service
	return root
}
