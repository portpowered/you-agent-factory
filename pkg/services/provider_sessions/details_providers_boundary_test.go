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

// TestDetails_ProvidersRootBoundary_Success proves Provider Sessions Details
// bridges provider/kind/id strings into Providers-root SessionRef construction
// (providers.ID + providers.SessionIDKind) and returns the existing observable
// Detail for stored Codex and Cursor sessions.
func TestDetails_ProvidersRootBoundary_Success(t *testing.T) {
	t.Run("codex", func(t *testing.T) {
		root := writeCodexSessionFixture(t, "boundary-details-codex")
		svc := newServiceForRoots(t, root, "")

		detail, err := svc.Details("codex", providers.SessionIDKind, "boundary-details-codex")
		if err != nil {
			t.Fatalf("Details: %v", err)
		}
		if detail.ProviderSession.Provider != providersessions.ProviderCodex ||
			detail.ProviderSession.ID != "boundary-details-codex" {
			t.Fatalf("ProviderSession = %#v, want codex boundary-details-codex", detail.ProviderSession)
		}
		if detail.Source.RelativePath == "" {
			t.Fatalf("Detail.Source.RelativePath empty")
		}
	})

	t.Run("cursor", func(t *testing.T) {
		root, sessionID := writeCursorSessionFixture(t)
		svc := newServiceForRoots(t, t.TempDir(), root)

		detail, err := svc.Details("cursor", providers.SessionIDKind, sessionID)
		if err != nil {
			t.Fatalf("Details: %v", err)
		}
		if detail.ProviderSession.Provider != providersessions.ProviderCursor {
			t.Fatalf("ProviderSession = %#v, want Cursor", detail.ProviderSession)
		}
		assertRootCursorProjection(t, detail)
	})
}

// TestDetails_ProvidersRootBoundary_NormalizesProviderAliases proves Details
// normalizes legacy provider strings into Providers-root provider IDs before
// reader dispatch.
func TestDetails_ProvidersRootBoundary_NormalizesProviderAliases(t *testing.T) {
	root := t.TempDir()
	svc := newServiceForRoots(t, t.TempDir(), root)

	for _, provider := range []string{"cursor", "agent", "cursor-agent"} {
		t.Run(provider, func(t *testing.T) {
			_, err := svc.Details(provider, providers.SessionIDKind, "missing-session")
			if !errors.Is(err, providersessions.ErrSessionNotFound) {
				t.Fatalf("Details error = %v, want ErrSessionNotFound", err)
			}
			var lookupErr *providersessions.LookupError
			if !errors.As(err, &lookupErr) {
				t.Fatalf("Details error = %T, want LookupError", err)
			}
			if lookupErr.Provider != providersessions.ProviderCursor || lookupErr.Root != root {
				t.Fatalf("LookupError = %#v, want normalized Cursor root context", lookupErr)
			}
		})
	}
}

// TestDetails_ProvidersRootBoundary_TypedErrors proves Details validates
// Providers-root SessionRef identity before storage access and preserves the
// existing typed Provider Sessions failures.
func TestDetails_ProvidersRootBoundary_TypedErrors(t *testing.T) {
	files := &openRecordingFileSystem{base: platformfilesystem.Local{}}
	svc, err := providersessionswire.NewForRoots(
		files,
		filepath.WalkDir,
		filepath.EvalSymlinks,
		filepath.WalkDir,
		filepath.EvalSymlinks,
		sql.Open,
		t.TempDir(),
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("NewForRoots: %v", err)
	}

	cases := []struct {
		name     string
		provider string
		kind     string
		id       string
		want     error
	}{
		{
			name:     "unsupported provider",
			provider: "openai",
			kind:     providers.SessionIDKind,
			id:       "session-1",
			want:     providersessions.ErrUnsupportedProvider,
		},
		{
			name:     "unsupported kind",
			provider: "codex",
			kind:     "path",
			id:       "session-1",
			want:     providersessions.ErrUnsupportedKind,
		},
		{
			name:     "invalid identifier",
			provider: "codex",
			kind:     providers.SessionIDKind,
			id:       "../secret",
			want:     providersessions.ErrInvalidIdentifier,
		},
		{
			name:     "blank identifier",
			provider: "codex",
			kind:     providers.SessionIDKind,
			id:       "   ",
			want:     providersessions.ErrInvalidIdentifier,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, detailsErr := svc.Details(test.provider, test.kind, test.id)
			if !errors.Is(detailsErr, test.want) {
				t.Fatalf("Details error = %v, want %v", detailsErr, test.want)
			}
		})
	}
	if files.opens != 0 {
		t.Fatalf("native file opens = %d, want 0 before validation passes", files.opens)
	}
}

// TestDetails_ProvidersRootBoundary_MatchesInspectOutcome proves equivalent
// string-keyed Details inputs and Providers-root Inspect requests still yield
// the same observable source metadata.
func TestDetails_ProvidersRootBoundary_MatchesInspectOutcome(t *testing.T) {
	root := writeCodexSessionFixture(t, "boundary-details-equivalence")
	svc := newServiceForRoots(t, root, "")
	ref := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "boundary-details-equivalence",
	}

	detail, err := svc.Details("codex", providers.SessionIDKind, ref.ID)
	if err != nil {
		t.Fatalf("Details: %v", err)
	}
	inspected, err := svc.Inspect(providersessions.InspectRequest{Session: ref})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if inspected.Source.RelativePath != detail.Source.RelativePath ||
		inspected.Source.SizeBytes != detail.Source.SizeBytes {
		t.Fatalf("Inspect source = %#v, want Details source %#v", inspected.Source, detail.Source)
	}
}
