package tts_clean_install

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPreflightRejectsReparseOutputRootBeforeReportWrite(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "external-target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("create external target: %v", err)
	}
	linked := filepath.Join(root, "linked-output")
	if runtime.GOOS == "windows" {
		output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", linked, target).CombinedOutput()
		if err != nil {
			t.Fatalf("create output junction: %v (%s)", err, strings.TrimSpace(string(output)))
		}
	} else if err := os.Symlink(target, linked); err != nil {
		t.Fatalf("create output symlink: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(linked) })

	f := newFixture(t)
	f.invocation.ReportPath = filepath.Join(linked, "report.json")
	if _, err := Preflight(f.invocation); err == nil {
		t.Fatal("Preflight accepted a reparse-point output root")
	}
	if _, err := os.Stat(filepath.Join(target, "report.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("external target report = %v, want no report write", err)
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatalf("read external target after rejection: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("external target entries after rejection = %d, want zero", len(entries))
	}
}
