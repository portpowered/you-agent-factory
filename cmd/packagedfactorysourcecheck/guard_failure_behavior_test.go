package main

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPackagedFactorySourceGuardCommandFailsOnOffBoundaryRootDocument(t *testing.T) {
	root := fixtureRepository(t)
	writeFixture(t, root, "pkg/services/factory_definitions/packages/new/factory.yml", "name: '@you/new'\n")

	exitCode, stderr := runSourceGuardCommand(t, root)
	if exitCode == 0 {
		t.Fatalf("source guard exit code = 0, want non-zero failure; stderr = %q", stderr)
	}
	for _, want := range []string{
		diagnosticPrefix,
		"source boundary failed",
		"pkg/services/factory_definitions/packages/new/factory.yml",
		`declares shipped first-party Factory "@you/new"`,
		authoredBoundary,
	} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr = %q, want substring %q", stderr, want)
		}
	}
}

func TestPackagedFactorySourceGuardCommandFailsOnOffBoundaryGoLiteral(t *testing.T) {
	root := fixtureRepository(t)
	writeFixture(
		t,
		root,
		"pkg/services/factory_definitions/packages/new/builtin.go",
		"package new\nvar definition = []byte(`{\"name\":\"@you/new-literal\"}`)\n",
	)

	exitCode, stderr := runSourceGuardCommand(t, root)
	if exitCode == 0 {
		t.Fatalf("source guard exit code = 0, want non-zero failure; stderr = %q", stderr)
	}
	for _, want := range []string{
		diagnosticPrefix,
		"pkg/services/factory_definitions/packages/new/builtin.go",
		`declares shipped first-party Factory "@you/new-literal"`,
		authoredBoundary,
	} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr = %q, want substring %q", stderr, want)
		}
	}
}

func runSourceGuardCommand(t *testing.T, root string) (int, string) {
	t.Helper()
	repoRoot := repositoryRoot(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(
		ctx,
		"go",
		"run",
		"./cmd/packagedfactorysourcecheck",
		"-root",
		root,
	)
	command.Dir = repoRoot
	var stderr bytes.Buffer
	command.Stderr = &stderr
	err := command.Run()
	if err == nil {
		return 0, stderr.String()
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("go run packagedfactorysourcecheck: %v", err)
	}
	return exitErr.ExitCode(), stderr.String()
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	return root
}
