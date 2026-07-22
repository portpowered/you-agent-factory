package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPassesForCompleteREADMEWithValidLocalReferences(t *testing.T) {
	t.Parallel()

	repoRoot := writeFixtureRepo(t, validREADMEFixture())

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, readmePath: "README.md"}, stdout, stderr)
	if err != nil {
		t.Fatalf("run() error = %v, want nil", err)
	}
	if got := stdout.String(); !strings.Contains(got, "[agent-factory:readme-check] README structure and local references passed") {
		t.Fatalf("run() stdout = %q, want success message", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("run() stderr = %q, want empty", got)
	}
}

func TestRunFailsWhenRequiredSectionRemoved(t *testing.T) {
	t.Parallel()

	content := strings.Replace(validREADMEFixture(), "## License\n", "", 1)
	repoRoot := writeFixtureRepo(t, content)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, readmePath: "README.md"}, stdout, stderr)
	if err == nil {
		t.Fatal("run() unexpectedly succeeded")
	}
	if got := stderr.String(); !strings.Contains(got, "missing required section: License") {
		t.Fatalf("run() stderr = %q, want missing License section", got)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("run() stdout = %q, want empty", got)
	}
}

func TestRunFailsWhenLocalReferenceMissing(t *testing.T) {
	t.Parallel()

	content := strings.Replace(validREADMEFixture(), "./LICENSE.md", "./MISSING.md", 1)
	repoRoot := writeFixtureRepo(t, content)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, readmePath: "README.md"}, stdout, stderr)
	if err == nil {
		t.Fatal("run() unexpectedly succeeded")
	}
	if got := stderr.String(); !strings.Contains(got, "missing local reference: MISSING.md") {
		t.Fatalf("run() stderr = %q, want missing local reference", got)
	}
}

func validREADMEFixture() string {
	return strings.Join([]string{
		"# infinite-you",
		"",
		"![hero](./docs/internal/resources/dashboard.png)",
		"",
		"## Installation",
		"[install script](./scripts/install.sh)",
		"",
		"## Quick start",
		"Run `you`.",
		"",
		"## Features",
		"[Authoring factories](./docs/reference/authoring-factories.md)",
		"",
		"## Comparison",
		"[Comparing systems](./docs/comparatives/comparing-systems.md)",
		"",
		"## References",
		"[Architecture](./docs/architecture/architecture.md)",
		"",
		"## License",
		"[MIT License](./LICENSE.md)",
		"",
	}, "\n")
}

func writeFixtureRepo(t *testing.T, readme string) string {
	t.Helper()

	repoRoot := t.TempDir()
	files := map[string]string{
		"README.md":                              readme,
		"LICENSE.md":                             "MIT",
		"scripts/install.sh":                     "#!/bin/sh",
		"docs/internal/resources/dashboard.png":  "png",
		"docs/reference/authoring-factories.md":  "# authoring",
		"docs/comparatives/comparing-systems.md": "# compare",
		"docs/architecture/architecture.md":      "# architecture",
	}
	for relativePath, body := range files {
		fullPath := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("mkdir fixture path: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(body), 0o644); err != nil {
			t.Fatalf("write fixture file: %v", err)
		}
	}
	return repoRoot
}
