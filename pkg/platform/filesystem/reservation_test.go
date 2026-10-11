package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveReservedPreservesChangedAndReplacedFiles(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"unchanged", "written", "replaced empty", "replaced bytes", "missing"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "owned.json")
			file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				t.Fatal(err)
			}
			identity, err := file.Stat()
			if err != nil {
				t.Fatal(err)
			}
			if !os.SameFile(identity, identity) {
				t.Fatal("identity unavailable")
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			want := ""
			switch mode {
			case "written":
				want = "actual published history"
				if err := os.WriteFile(path, []byte(want), 0o600); err != nil {
					t.Fatal(err)
				}
			case "replaced empty", "replaced bytes":
				// Retain the original inode under another name so even filesystems
				// that immediately reuse freed identities cannot disguise replacement.
				if err := os.Rename(path, path+".original"); err != nil {
					t.Fatal(err)
				}
				if mode == "replaced bytes" {
					want = "foreign history"
				}
				if err := os.WriteFile(path, []byte(want), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(path, identity.ModTime(), identity.ModTime()); err != nil {
					t.Fatal(err)
				}
			case "missing":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			if err := (Local{}).RemoveReserved(path, identity); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if mode == "unchanged" || mode == "missing" {
				if !os.IsNotExist(err) {
					t.Fatalf("owned placeholder retained: %q %v", data, err)
				}
			} else if err != nil || string(data) != want {
				t.Fatalf("foreign/published file changed: %q %v", data, err)
			}
		})
	}
}
