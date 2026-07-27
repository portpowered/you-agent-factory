package main

import (
	"bytes"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunPassesOnRepositoryFreeze(t *testing.T) {
	root := repositoryRoot(t)
	var stdout, stderr bytes.Buffer
	if code := run(root, &stdout, &stderr); code != 0 {
		t.Fatalf("run() = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "verified") {
		t.Fatalf("stdout = %q, want verified message", stdout.String())
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
