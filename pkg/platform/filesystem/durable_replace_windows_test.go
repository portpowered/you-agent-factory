//go:build windows

package filesystem

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestDurableReplacementSharingFailurePreservesFileAndCanRetry(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "journal")
	committed := []byte("old committed bytes")
	if err := os.WriteFile(path, committed, 0o600); err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	// A controlled host handle refuses delete sharing, as an ordinary external
	// reader can. The adapter must decline replacement, never remove first.
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(handle) })
	local := Local{AllowRenameReplacement: true}
	if err := local.ReplaceDurable(path, []byte("new committed bytes")); err == nil {
		t.Fatal("sharing violation reported success")
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, committed) {
		t.Fatal("sharing failure changed original file", err)
	}
	if err := windows.CloseHandle(handle); err != nil {
		t.Fatal(err)
	}
	handle = windows.InvalidHandle
	replacement := []byte("new committed bytes")
	if err := local.ReplaceDurable(path, replacement); err != nil {
		t.Fatal("retry after reader release failed", err)
	}
	got, err = os.ReadFile(path)
	if err != nil || !bytes.Equal(got, replacement) {
		t.Fatal("retry did not replace the complete file", err)
	}
}
