package providersessions_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"
	codexreader "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/codex_reader"
	codexreaderwire "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/codex_reader/wire"
	cursorreaderwire "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/cursor_reader/wire"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// Compile-time proof that parent-private Codex and Cursor reader contracts
// accept only Providers-owned SessionRef at their production call sites.
var (
	_ providers.SessionRef = providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "compile-time-codex",
	}
	_ providers.SessionRef = providers.SessionRef{
		Provider: providers.IDCursor,
		Kind:     providers.SessionIDKind,
		ID:       "compile-time-cursor",
	}
)

func newCodexReaderForRoot(t *testing.T, root string) codexreader.Service {
	t.Helper()
	reader, err := codexreaderwire.NewService(codexreader.Dependencies{
		Files:           platformfilesystem.Local{},
		WalkDirectory:   providersessionsinternal.CodexWalkDirectory(filepath.WalkDir),
		ResolveSymlinks: providersessionsinternal.CodexResolveSymlinks(filepath.EvalSymlinks),
		SessionsRoot:    root,
	})
	if err != nil {
		t.Fatalf("codexreaderwire.NewService: %v", err)
	}
	return reader
}

func newCursorReaderForRoot(t *testing.T, root string) interface {
	Read(context.Context, providers.SessionRef) (providersessions.Detail, error)
} {
	t.Helper()
	reader, err := cursorreaderwire.NewService(
		platformfilesystem.Local{},
		providersessionsinternal.CursorWalkDirectory(filepath.WalkDir),
		providersessionsinternal.CursorResolveSymlinks(filepath.EvalSymlinks),
		providersessionsinternal.CursorOpenSQLDatabase(sql.Open),
		root,
	)
	if err != nil {
		t.Fatalf("cursorreaderwire.NewService: %v", err)
	}
	return reader
}

// TestCodexReader_ProvidersRootBoundary_Success proves the Codex reader handoff
// accepts a Providers-root SessionRef and returns the existing observable Detail.
func TestCodexReader_ProvidersRootBoundary_Success(t *testing.T) {
	root := writeCodexSessionFixture(t, "boundary-reader-codex")
	reader := newCodexReaderForRoot(t, root)
	ref := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "boundary-reader-codex",
	}

	detail, err := reader.Details(context.Background(), ref)
	if err != nil {
		t.Fatalf("Codex Details: %v", err)
	}
	if detail.ProviderSession.Provider != providersessions.ProviderCodex ||
		detail.ProviderSession.ID != ref.ID {
		t.Fatalf("ProviderSession = %#v, want codex %s", detail.ProviderSession, ref.ID)
	}
	if detail.Source.RelativePath == "" {
		t.Fatalf("Detail.Source.RelativePath empty")
	}
}

// TestCursorReader_ProvidersRootBoundary_Success proves the Cursor reader handoff
// accepts a Providers-root SessionRef and returns the existing observable Detail.
func TestCursorReader_ProvidersRootBoundary_Success(t *testing.T) {
	root, sessionID := writeCursorSessionFixture(t)
	reader := newCursorReaderForRoot(t, root)
	ref := providers.SessionRef{
		Provider: providers.IDCursor,
		Kind:     providers.SessionIDKind,
		ID:       sessionID,
	}

	detail, err := reader.Read(context.Background(), ref)
	if err != nil {
		t.Fatalf("Cursor Read: %v", err)
	}
	if detail.ProviderSession.Provider != providersessions.ProviderCursor {
		t.Fatalf("ProviderSession = %#v, want Cursor", detail.ProviderSession)
	}
	assertRootCursorProjection(t, detail)
}

// TestCodexReader_ProvidersRootBoundary_TypedErrors proves Codex reader validates
// Providers-root SessionRef identity and preserves existing typed failures.
func TestCodexReader_ProvidersRootBoundary_TypedErrors(t *testing.T) {
	reader := newCodexReaderForRoot(t, t.TempDir())

	cases := []struct {
		name string
		ref  providers.SessionRef
		want error
	}{
		{
			name: "unsupported provider",
			ref:  providers.SessionRef{Provider: "openai", Kind: providers.SessionIDKind, ID: "session-1"},
			want: providersessions.ErrUnsupportedProvider,
		},
		{
			name: "unsupported kind",
			ref:  providers.SessionRef{Provider: providers.IDCodex, Kind: "path", ID: "session-1"},
			want: providersessions.ErrUnsupportedKind,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := reader.Details(context.Background(), test.ref)
			if !errors.Is(err, test.want) {
				t.Fatalf("Codex Details error = %v, want %v", err, test.want)
			}
		})
	}
}

// TestCursorReader_ProvidersRootBoundary_TypedErrors proves Cursor reader validates
// Providers-root SessionRef identity and preserves existing typed failures.
func TestCursorReader_ProvidersRootBoundary_TypedErrors(t *testing.T) {
	reader := newCursorReaderForRoot(t, t.TempDir())

	cases := []struct {
		name string
		ref  providers.SessionRef
		want error
	}{
		{
			name: "unsupported provider",
			ref:  providers.SessionRef{Provider: "openai", Kind: providers.SessionIDKind, ID: "session-1"},
			want: providersessions.ErrUnsupportedProvider,
		},
		{
			name: "codex provider on cursor reader",
			ref:  providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "session-1"},
			want: providersessions.ErrUnsupportedProvider,
		},
		{
			name: "unsupported kind",
			ref:  providers.SessionRef{Provider: providers.IDCursor, Kind: "path", ID: "session-1"},
			want: providersessions.ErrUnsupportedKind,
		},
		{
			name: "invalid identifier",
			ref:  providers.SessionRef{Provider: providers.IDCursor, Kind: providers.SessionIDKind, ID: "   "},
			want: providersessions.ErrInvalidIdentifier,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := reader.Read(context.Background(), test.ref)
			if !errors.Is(err, test.want) {
				t.Fatalf("Cursor Read error = %v, want %v", err, test.want)
			}
		})
	}
}

// TestReaders_ProvidersRootBoundary_ServiceHandoff proves Provider Sessions
// dispatches Providers-root SessionRef to Codex and Cursor readers without
// reintroducing Workers provider or Providers implementation types.
func TestReaders_ProvidersRootBoundary_ServiceHandoff(t *testing.T) {
	t.Run("codex via Details", func(t *testing.T) {
		root := writeCodexSessionFixture(t, "boundary-handoff-codex")
		svc := newServiceForRoots(t, root, "")
		ref := providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "boundary-handoff-codex",
		}

		detail, err := svc.Details("codex", providers.SessionIDKind, ref.ID)
		if err != nil {
			t.Fatalf("Details: %v", err)
		}
		projected, err := svc.Project(providersessions.ProjectRequest{Session: ref})
		if err != nil {
			t.Fatalf("Project: %v", err)
		}
		if projected.Detail.Source.RelativePath != detail.Source.RelativePath {
			t.Fatalf("Project source = %#v, want Details source %#v", projected.Detail.Source, detail.Source)
		}
	})

	t.Run("cursor via Details", func(t *testing.T) {
		root, sessionID := writeCursorSessionFixture(t)
		svc := newServiceForRoots(t, t.TempDir(), root)
		ref := providers.SessionRef{
			Provider: providers.IDCursor,
			Kind:     providers.SessionIDKind,
			ID:       sessionID,
		}

		detail, err := svc.Details("cursor", providers.SessionIDKind, sessionID)
		if err != nil {
			t.Fatalf("Details: %v", err)
		}
		projected, err := svc.Project(providersessions.ProjectRequest{Session: ref})
		if err != nil {
			t.Fatalf("Project: %v", err)
		}
		assertRootCursorProjection(t, projected.Detail)
		if projected.Detail.Source.RelativePath != detail.Source.RelativePath {
			t.Fatalf("Project source = %#v, want Details source %#v", projected.Detail.Source, detail.Source)
		}
	})
}
