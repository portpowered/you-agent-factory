package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestNewUnisolatedHomeToucherFails(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"pkg/a/a_test.go": "package a\nvar _ = os.UserHomeDir\nfunc f(){ os.UserHomeDir() }",
		"baseline.txt":    "# none\n",
	})
	err := run(root, "baseline.txt", false, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "pkg/a/a_test.go") {
		t.Fatalf("want violation for pkg/a/a_test.go, got %v", err)
	}
}

func TestBaselinedAndIsolatedPackagesPass(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"pkg/a/a_test.go":    "package a\nfunc f(){ os.UserHomeDir() }",
		"pkg/b/b_test.go":    "package b\nfunc f(){ os.UserHomeDir() }",
		"pkg/b/home_test.go": "package b\nfunc TestMain(){ testhome.IsolateHomeMain() }",
		"baseline.txt":       "pkg/a/a_test.go\npkg/gone/x_test.go\n",
	})
	var out bytes.Buffer
	if err := run(root, "baseline.txt", false, &out); err != nil {
		t.Fatalf("unexpected failure: %v", err)
	}
}
