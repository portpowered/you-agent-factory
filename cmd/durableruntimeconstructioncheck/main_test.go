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

func TestScanAcceptsApplicationCompositionAndTransportTest(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/services/factory_sessions/execution/service.go": "testdata/approved_composition.go.txt",
		"pkg/transports/http/transport_test.go":              "testdata/approved_transport_test.go.txt",
	})

	findings, err := scan(root)
	if err != nil {
		t.Fatalf("scan fixture: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("approved ownership produced findings: %v", findings)
	}
}

func TestScanRejectsTransportApplicationComposition(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/transports/cli/run/compose.go": "testdata/prohibited_transport_composition.go.txt",
	})

	findings, err := scan(root)
	if err != nil {
		t.Fatalf("scan fixture: %v", err)
	}
	for _, prohibited := range []string{
		"BuildInvocationBootstrap",
		"NewExecutionService",
		"NewFakeServiceFromContractFixtures",
		"ProjectPersistence",
	} {
		if !containsFinding(findings, prohibited) {
			t.Errorf("findings %#v do not report %s", findings, prohibited)
		}
	}
}

func TestScanRejectsJavaScriptSpecificLiveProviderPath(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/services/factory_sessions/execution/livechild/provider.go": "testdata/prohibited_live_child_provider.go.txt",
	})

	findings, err := scan(root)
	if err != nil {
		t.Fatalf("scan fixture: %v", err)
	}

	for _, prohibited := range []string{providerPackagePath, providerInferenceName, "pkg/services/workers"} {
		if !containsFinding(findings, prohibited) {
			t.Errorf("findings %#v do not report %s", findings, prohibited)
		}
	}
}

func TestScanAllowsLiveChildSharedBoundaryAndTestDoubles(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/services/factory_sessions/execution/livechild/provider.go":      "testdata/approved_live_child_boundary.go.txt",
		"pkg/services/factory_sessions/execution/livechild/provider_test.go": "testdata/prohibited_live_child_provider.go.txt",
	})

	findings, err := scan(root)
	if err != nil {
		t.Fatalf("scan fixture: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("shared boundary or test double produced findings: %v", findings)
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
