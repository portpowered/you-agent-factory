package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanRejectsImplicitPersistenceConstruction(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/runtime/implicit.go": "testdata/prohibited_implicit.go.txt",
	})

	findings, err := scan(root)
	if err != nil {
		t.Fatalf("scan fixture: %v", err)
	}

	for _, prohibited := range []string{
		"NewJavaScriptRuntimeService",
		"PersistSessions",
		"DirForProjectRoot",
		"NewDirectoryStore",
		"DirectoryStore literal",
	} {
		if !containsFinding(findings, prohibited) {
			t.Errorf("findings %#v do not report %s", findings, prohibited)
		}
	}
}

func TestScanAcceptsApplicationCompositionAndApprovedHarness(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/factorysessionexecution/service.go":             "testdata/approved_composition.go.txt",
		"pkg/factorysessionexecution/testharness/harness.go": "testdata/approved_harness.go.txt",
		"pkg/api/transport_test.go":                          "testdata/approved_transport_test.go.txt",
	})

	findings, err := scan(root)
	if err != nil {
		t.Fatalf("scan fixture: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("approved ownership produced findings: %v", findings)
	}
}

func fixtureRepository(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for destination, source := range files {
		contents, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("read fixture %s: %v", source, err)
		}
		path := filepath.Join(root, filepath.FromSlash(destination))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create fixture directory: %v", err)
		}
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", destination, err)
		}
	}
	return root
}

func containsFinding(findings []string, expected string) bool {
	for _, finding := range findings {
		if strings.Contains(finding, expected) {
			return true
		}
	}
	return false
}
