package filesystem

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type stagedReplacement struct {
	written  int
	writeErr error
	syncErr  error
	closeErr error
	closed   bool
}

func (file *stagedReplacement) Write([]byte) (int, error) { return file.written, file.writeErr }
func (file *stagedReplacement) Sync() error               { return file.syncErr }
func (file *stagedReplacement) Close() error {
	file.closed = true
	return file.closeErr
}

func TestDurableReplacementStagingFailuresRefusePublication(t *testing.T) {
	t.Parallel()
	fault := errors.New("controlled staging failure")
	for _, test := range []struct {
		name string
		file stagedReplacement
		want error
	}{
		{"write", stagedReplacement{writeErr: fault}, fault},
		{"short write", stagedReplacement{written: 1}, io.ErrShortWrite},
		{"flush", stagedReplacement{written: 2, syncErr: fault}, fault},
		{"close", stagedReplacement{written: 2, closeErr: fault}, fault},
		{"first failure", stagedReplacement{writeErr: fault, closeErr: io.ErrClosedPipe}, fault},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := (Local{}).flushReplacement(&test.file, []byte("{}")); !errors.Is(err, test.want) {
				t.Fatal("staging failure did not refuse publication", err)
			}
			if !test.file.closed {
				t.Fatal("failed staging leaked its descriptor")
			}
		})
	}
}

func TestDurableReplacementPublishesCompletePrivateBytes(t *testing.T) {
	t.Parallel()
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "existing"}[existing], func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "journal")
			if existing {
				if err := os.WriteFile(path, []byte("old committed bytes"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			local := Local{AllowRenameReplacement: false}
			for _, data := range [][]byte{[]byte("snapshot\n"), {}, []byte("later snapshot\n")} {
				if err := local.ReplaceDurable(path, data); err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(got, data) {
					t.Fatal("replacement bytes differ", err)
				}
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
					t.Fatal("replacement grants excess file permissions")
				}
			}
		})
	}
}

func TestDurableReplacementRejectsInvalidTargetsWithoutChangingExistingData(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "existing")
	committed := []byte("committed bytes")
	if err := os.WriteFile(path, committed, 0o600); err != nil {
		t.Fatal(err)
	}
	local := Local{AllowRenameReplacement: true}
	for _, target := range []string{"", " ", filepath.Join(path, "child"), filepath.Join(root, "missing", "child"), path + "\x00"} {
		if err := local.ReplaceDurable(target, []byte("replacement")); err == nil {
			t.Fatal("invalid target reported success")
		}
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, committed) {
			t.Fatal("failed replacement changed committed bytes", err)
		}
	}
	// Replacing an occupied directory must fail without deleting its contents,
	// even when the older RenameReplacing policy permits file removal.
	directory := filepath.Join(root, "occupied")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(directory, "retained")
	if err := os.WriteFile(child, committed, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := local.ReplaceDurable(directory, []byte("replacement")); err == nil {
		t.Fatal("directory replacement reported success")
	}
	got, err := os.ReadFile(child)
	if err != nil || !bytes.Equal(got, committed) {
		t.Fatal("failed replacement removed destination contents", err)
	}
}
