package internal_test

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

// Compile-time proof that ProjectRequest carries the Providers-owned SessionRef
// alias rather than a parallel Provider Sessions identity type.
var _ providers.SessionRef = providersessions.ProjectRequest{}.Session

// TestProject_ProvidersRootBoundary_Success proves Provider Sessions Project
// accepts a Providers-root SessionRef (providers.ID + providers.SessionIDKind)
// and returns the existing observable Detail-shaped ProjectResult for stored
// Codex and Cursor sessions.
func TestProject_ProvidersRootBoundary_Success(t *testing.T) {
	t.Run("codex", func(t *testing.T) {
		root := writeCodexSessionFixture(t, "boundary-project-codex")
		svc := newServiceForRoots(t, root, "")
		ref := providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "boundary-project-codex",
		}

		result, err := svc.Project(providersessions.ProjectRequest{Session: ref})
		if err != nil {
			t.Fatalf("Project: %v", err)
		}
		if result.Session != ref {
			t.Fatalf("ProjectResult.Session = %#v, want %#v", result.Session, ref)
		}
		if result.Detail.ProviderSession.Provider != providersessions.ProviderCodex ||
			result.Detail.ProviderSession.ID != ref.ID {
			t.Fatalf("ProjectResult.Detail.ProviderSession = %#v", result.Detail.ProviderSession)
		}
		if result.Detail.Source.RelativePath == "" {
			t.Fatalf("ProjectResult.Detail.Source.RelativePath empty")
		}
	})

	t.Run("cursor", func(t *testing.T) {
		root, sessionID := writeCursorSessionFixture(t)
		svc := newServiceForRoots(t, t.TempDir(), root)
		ref := providers.SessionRef{
			Provider: providers.IDCursor,
			Kind:     providers.SessionIDKind,
			ID:       sessionID,
		}

		result, err := svc.Project(providersessions.ProjectRequest{Session: ref})
		if err != nil {
			t.Fatalf("Project: %v", err)
		}
		if result.Session != ref {
			t.Fatalf("ProjectResult.Session = %#v, want %#v", result.Session, ref)
		}
		if result.Detail.ProviderSession.Provider != providersessions.ProviderCursor {
			t.Fatalf("ProjectResult.Detail.ProviderSession = %#v, want Cursor", result.Detail.ProviderSession)
		}
		assertRootCursorProjection(t, result.Detail)
	})
}

// TestProject_ProvidersRootBoundary_TypedErrors proves Project validates
// Providers-root SessionRef identity before storage access and preserves the
// existing typed Provider Sessions failures.
func TestProject_ProvidersRootBoundary_TypedErrors(t *testing.T) {
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
		{
			name: "invalid identifier",
			ref:  providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "../secret"},
			want: providersessions.ErrInvalidIdentifier,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, projectErr := svc.Project(providersessions.ProjectRequest{Session: test.ref})
			if !errors.Is(projectErr, test.want) {
				t.Fatalf("Project error = %v, want %v", projectErr, test.want)
			}
		})
	}
	if files.opens != 0 {
		t.Fatalf("native file opens = %d, want 0 before validation passes", files.opens)
	}
}

// TestProject_ProvidersRootBoundary_MatchesDetailsOutcome proves equivalent
// string-keyed Details inputs and Providers-root Project requests still yield
// the same observable normalized detail facts.
func TestProject_ProvidersRootBoundary_MatchesDetailsOutcome(t *testing.T) {
	root := writeCodexSessionFixture(t, "boundary-project-equivalence")
	svc := newServiceForRoots(t, root, "")
	ref := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "boundary-project-equivalence",
	}

	detail, err := svc.Details("codex", providers.SessionIDKind, ref.ID)
	if err != nil {
		t.Fatalf("Details: %v", err)
	}
	projected, err := svc.Project(providersessions.ProjectRequest{Session: ref})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if projected.Detail.Source.RelativePath != detail.Source.RelativePath ||
		projected.Detail.Source.SizeBytes != detail.Source.SizeBytes ||
		projected.Detail.ProviderSession != detail.ProviderSession {
		t.Fatalf("Project detail = %#v, want Details detail %#v", projected.Detail, detail)
	}
}
