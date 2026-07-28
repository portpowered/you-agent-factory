package factorydefinitions_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

const (
	definitionLifecycleHostImport = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/definition"
	compilationPackageGlob        = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/..."
)

var transitionalCompileLoadPublicDirs = []string{
	"definition",
}

var deletedCompileLoadPublicDirs = []string{
	"loading",
	"runtimeconfig",
	"loadedsource",
}

func TestCompileLoadFold_TransitionalPublicPackagesRemainPresent(t *testing.T) {
	t.Parallel()

	for _, relativeDir := range transitionalCompileLoadPublicDirs {
		relativeDir := relativeDir
		t.Run(relativeDir, func(t *testing.T) {
			t.Parallel()
			info, err := os.Stat(relativeDir)
			if err != nil {
				t.Fatalf("transitional public package %s must remain present until its owning DEL packet: %v", relativeDir, err)
			}
			if !info.IsDir() {
				t.Fatalf("transitional public path %s must remain a directory until its owning DEL packet", relativeDir)
			}
		})
	}
}

func TestDelDef_DeletedCompileLoadTransitionalPublicPackagesAbsent(t *testing.T) {
	t.Parallel()

	for _, relativeDir := range deletedCompileLoadPublicDirs {
		relativeDir := relativeDir
		t.Run(relativeDir, func(t *testing.T) {
			t.Parallel()
			_, err := os.Stat(relativeDir)
			if !os.IsNotExist(err) {
				t.Fatalf("transitional public package %s must be deleted by DEL-DEF; stat = %v", relativeDir, err)
			}
		})
	}
}

func TestCompileLoadFold_DefinitionLifecycleHostUnchangedInBranch(t *testing.T) {
	t.Parallel()

	baseRef := resolveDefinitionDiffBaseRef(t)
	cmd := exec.Command(
		"git",
		"diff",
		baseRef+"...HEAD",
		"--",
		"pkg/services/factory_definitions/definition/",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 && len(strings.TrimSpace(string(output))) == 0 {
			return
		}
		t.Fatalf("git diff definition/: %v\n%s", err, output)
	}
	if trimmed := strings.TrimSpace(string(output)); trimmed != "" {
		t.Fatalf("definition/ must remain untouched by CLN-DEF-FOLD-COMPILATION; found diff:\n%s", trimmed)
	}
}

func resolveDefinitionDiffBaseRef(t *testing.T) string {
	t.Helper()

	candidates := []string{
		strings.TrimSpace(os.Getenv("PR_BASE_SHA")),
		"origin/main",
		"main",
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		cmd := exec.Command("git", "rev-parse", "--verify", candidate)
		if err := cmd.Run(); err == nil {
			return candidate
		}
	}
	t.Fatal("unable to resolve git base ref for definition/ diff guard; set PR_BASE_SHA or fetch origin/main")
	return ""
}

func TestCompileLoadFold_CompilationDoesNotImportDefinitionLifecycleHost(t *testing.T) {
	t.Parallel()

	for _, packagePath := range listCompilationPackages(t) {
		packagePath := packagePath
		t.Run(packagePath, func(t *testing.T) {
			t.Parallel()
			assertPackageDoesNotImport(t, packagePath, definitionLifecycleHostImport)
		})
	}
}

func listCompilationPackages(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command("go", "list", compilationPackageGlob)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list compilation packages: %v\n%s", err, output)
	}
	packages := strings.Fields(string(output))
	if len(packages) == 0 {
		t.Fatal("expected at least one compilation package")
	}
	return packages
}
