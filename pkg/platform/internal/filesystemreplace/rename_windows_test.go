//go:build windows

package filesystemreplace

import (
	"errors"
	"os"
	"testing"
)

func TestRenameReplacingDoesNotFallbackForNonContentionError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("source is not replaceable")
	renameCalls := 0
	removeCalls := 0
	err := RenameReplacing(
		"old",
		"new",
		true,
		func(string, string) error {
			renameCalls++
			return sentinel
		},
		func(string) error {
			removeCalls++
			return nil
		},
		func(string) (os.FileInfo, error) { return nil, nil },
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("rename error = %v, want %v", err, sentinel)
	}
	if renameCalls != 1 || removeCalls != 0 {
		t.Fatalf("fallback calls = rename:%d remove:%d, want rename:1 remove:0", renameCalls, removeCalls)
	}
}

func TestRenameReplacingRetriesWindowsDestinationContention(t *testing.T) {
	t.Parallel()

	renameCalls := 0
	removeCalls := 0
	err := RenameReplacing(
		"old",
		"new",
		true,
		func(string, string) error {
			renameCalls++
			if renameCalls == 1 {
				return os.ErrExist
			}
			return nil
		},
		func(string) error {
			removeCalls++
			return nil
		},
		func(string) (os.FileInfo, error) { return nil, nil },
	)
	if err != nil {
		t.Fatalf("RenameReplacing(contention): %v", err)
	}
	if renameCalls != 2 || removeCalls != 1 {
		t.Fatalf("retry calls = rename:%d remove:%d, want rename:2 remove:1", renameCalls, removeCalls)
	}
}
