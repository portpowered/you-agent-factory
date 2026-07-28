package providersessions_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionsservice "github.com/portpowered/infinite-you/pkg/services/provider_sessions/service"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// Compile-time proof that InspectRequest carries the Providers-owned SessionRef
// alias rather than a parallel Provider Sessions identity type.
var _ providers.SessionRef = providersessions.InspectRequest{}.Session

// TestInspect_ProvidersRootBoundary_Success proves Provider Sessions Inspect
// accepts a Providers-root SessionRef (providers.ID + providers.SessionIDKind)
// and returns the existing observable InspectResult for stored Codex and Cursor
// sessions.
func TestInspect_ProvidersRootBoundary_Success(t *testing.T) {
	t.Run("codex", func(t *testing.T) {
		root := writeCodexSessionFixture(t, "boundary-inspect-codex")
		svc := newServiceForRoots(t, root, "")
		ref := providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "boundary-inspect-codex",
		}

		result, err := svc.Inspect(providersessions.InspectRequest{Session: ref})
		if err != nil {
			t.Fatalf("Inspect: %v", err)
		}
		if result.Session != ref {
			t.Fatalf("InspectResult.Session = %#v, want %#v", result.Session, ref)
		}
		if result.Source.RelativePath == "" {
			t.Fatalf("InspectResult.Source.RelativePath empty")
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

		result, err := svc.Inspect(providersessions.InspectRequest{Session: ref})
		if err != nil {
			t.Fatalf("Inspect: %v", err)
		}
		if result.Session != ref {
			t.Fatalf("InspectResult.Session = %#v, want %#v", result.Session, ref)
		}
		if result.Source.RelativePath == "" {
			t.Fatalf("InspectResult.Source.RelativePath empty")
		}
	})
}

// TestInspect_ProvidersRootBoundary_TypedErrors proves Inspect validates
// Providers-root SessionRef identity before storage access and preserves the
// existing typed Provider Sessions failures.
func TestInspect_ProvidersRootBoundary_TypedErrors(t *testing.T) {
	files := &openRecordingFileSystem{base: platformfilesystem.Local{}}
	svc, err := providersessionsservice.NewForRoots(
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
			_, inspectErr := svc.Inspect(providersessions.InspectRequest{Session: test.ref})
			if !errors.Is(inspectErr, test.want) {
				t.Fatalf("Inspect error = %v, want %v", inspectErr, test.want)
			}
		})
	}
	if files.opens != 0 {
		t.Fatalf("native file opens = %d, want 0 before validation passes", files.opens)
	}
}

// TestInspect_ProvidersRootBoundary_MatchesDetailsOutcome proves equivalent
// string-keyed Details inputs and Providers-root Inspect requests still yield
// the same observable source metadata.
func TestInspect_ProvidersRootBoundary_MatchesDetailsOutcome(t *testing.T) {
	root := writeCodexSessionFixture(t, "boundary-inspect-equivalence")
	svc := newServiceForRoots(t, root, "")
	ref := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "boundary-inspect-equivalence",
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
