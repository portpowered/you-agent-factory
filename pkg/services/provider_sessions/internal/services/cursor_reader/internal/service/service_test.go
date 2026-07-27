package service

import (
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestReadDiscoversOnlyCanonicalContainedSession(t *testing.T) {
	root, sessionID := writeSessionFixture(t)
	reader, err := New(
		platformfilesystem.Local{},
		filepath.WalkDir,
		filepath.EvalSymlinks,
		sql.Open,
		root,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	detail, err := reader.Read(providers.SessionRef{
		Provider: providers.IDCursor,
		Kind:     providers.SessionIDKind,
		ID:       sessionID,
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if detail.ProviderSession != (providersessions.Ref{
		Provider: providersessions.ProviderCursor,
		Kind:     providersessions.SessionIDKind,
		ID:       sessionID,
	}) {
		t.Fatalf("ProviderSession = %#v", detail.ProviderSession)
	}
	if got, want := detail.Source.RelativePath, filepath.ToSlash(filepath.Join("workspace", sessionID, "store.db")); got != want {
		t.Fatalf("RelativePath = %q, want %q", got, want)
	}
	if filepath.IsAbs(detail.Source.RelativePath) {
		t.Fatalf("RelativePath exposed absolute storage path: %q", detail.Source.RelativePath)
	}
}

func TestReadRejectsInvalidReferencesBeforeStorageIO(t *testing.T) {
	var walks, opens int
	reader, err := New(
		platformfilesystem.Local{},
		func(string, fs.WalkDirFunc) error {
			walks++
			return nil
		},
		filepath.EvalSymlinks,
		func(string, string) (*sql.DB, error) {
			opens++
			return nil, errors.New("unexpected database open")
		},
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tests := []struct {
		name string
		ref  providers.SessionRef
		want error
	}{
		{
			name: "unsupported provider",
			ref:  providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "session-1"},
			want: providersessions.ErrUnsupportedProvider,
		},
		{
			name: "legacy cursor alias is not canonical",
			ref:  providers.SessionRef{Provider: providers.ID("cursor"), Kind: providers.SessionIDKind, ID: "session-1"},
			want: providersessions.ErrUnsupportedProvider,
		},
		{
			name: "wrong kind",
			ref:  providers.SessionRef{Provider: providers.IDCursor, Kind: "path", ID: "session-1"},
			want: providersessions.ErrUnsupportedKind,
		},
		{
			name: "empty id",
			ref:  providers.SessionRef{Provider: providers.IDCursor, Kind: providers.SessionIDKind, ID: "  "},
			want: providersessions.ErrInvalidIdentifier,
		},
		{
			name: "path-like id",
			ref:  providers.SessionRef{Provider: providers.IDCursor, Kind: providers.SessionIDKind, ID: "../other-session"},
			want: providersessions.ErrInvalidIdentifier,
		},
		{
			name: "malformed id",
			ref:  providers.SessionRef{Provider: providers.IDCursor, Kind: providers.SessionIDKind, ID: "session.with.dot"},
			want: providersessions.ErrInvalidIdentifier,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := reader.Read(test.ref); !errors.Is(err, test.want) {
				t.Fatalf("Read error = %v, want %v", err, test.want)
			}
		})
	}
	if walks != 0 || opens != 0 {
		t.Fatalf("rejected references performed storage IO: walks=%d opens=%d", walks, opens)
	}
}

func TestReadMissingAndAmbiguousSessionsNeverOpenDatabase(t *testing.T) {
	for _, test := range []struct {
		name    string
		prepare func(*testing.T) string
		want    error
	}{
		{
			name:    "missing",
			prepare: func(t *testing.T) string { return t.TempDir() },
			want:    providersessions.ErrSessionNotFound,
		},
		{
			name: "ambiguous",
			prepare: func(t *testing.T) string {
				root := t.TempDir()
				for _, workspace := range []string{"a", "b"} {
					path := filepath.Join(root, workspace, "same-session", "store.db")
					if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
						t.Fatalf("mkdir: %v", err)
					}
					if err := os.WriteFile(path, nil, 0o600); err != nil {
						t.Fatalf("write store: %v", err)
					}
				}
				return root
			},
			want: providersessions.ErrAmbiguousSessionFile,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			opens := 0
			reader, err := New(
				platformfilesystem.Local{},
				filepath.WalkDir,
				filepath.EvalSymlinks,
				func(string, string) (*sql.DB, error) {
					opens++
					return nil, errors.New("unexpected database open")
				},
				test.prepare(t),
			)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			_, err = reader.Read(providers.SessionRef{
				Provider: providers.IDCursor,
				Kind:     providers.SessionIDKind,
				ID:       "same-session",
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("Read error = %v, want %v", err, test.want)
			}
			if opens != 0 {
				t.Fatalf("database opens = %d, want 0", opens)
			}
		})
	}
}

func TestReadRejectsCandidateResolvedOutsideRootBeforeDatabaseOpen(t *testing.T) {
	root := t.TempDir()
	sessionID := "replaced-session"
	candidate := filepath.Join(root, "workspace", sessionID, "store.db")
	if err := os.MkdirAll(filepath.Dir(candidate), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(candidate, nil, 0o600); err != nil {
		t.Fatalf("write candidate: %v", err)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs root: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "replacement.db")
	opens := 0
	resolve := func(path string) (string, error) {
		if filepath.Clean(path) == filepath.Clean(absoluteRoot) {
			return absoluteRoot, nil
		}
		return outside, nil
	}
	reader, err := New(
		platformfilesystem.Local{},
		filepath.WalkDir,
		resolve,
		func(string, string) (*sql.DB, error) {
			opens++
			return nil, errors.New("unexpected database open")
		},
		root,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = reader.Read(providers.SessionRef{
		Provider: providers.IDCursor,
		Kind:     providers.SessionIDKind,
		ID:       sessionID,
	})
	if !errors.Is(err, providersessions.ErrInvalidIdentifier) {
		t.Fatalf("Read error = %v, want ErrInvalidIdentifier", err)
	}
	if opens != 0 {
		t.Fatalf("database opens = %d, want 0", opens)
	}
}

func writeSessionFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	sessionID := "canonical-cursor-session"
	path := filepath.Join(root, "workspace", sessionID, "store.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`
CREATE TABLE blobs (key TEXT PRIMARY KEY, value TEXT);
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT);
INSERT INTO blobs (key, value) VALUES ('bubble-1', '{"bubbleId":"bubble-1","text":"hello","timestamp":1000,"type":1}');
INSERT INTO meta (key, value) VALUES ('0', '{"agentId":"canonical-cursor-session","createdAt":1000}');
`); err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	return root, sessionID
}
