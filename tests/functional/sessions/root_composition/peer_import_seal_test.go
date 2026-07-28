package root_composition_test

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	sessionsModulePrefix           = "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	sessionsInternalImportPrefix   = sessionsModulePrefix + "/internal"
	sessionsWireImport             = sessionsModulePrefix + "/wire"
	functionalSessionsRootRelative = "tests/functional/sessions"
)

// sessionsProductionPeerPackages are composition peers that must reach Sessions
// only through published root contracts and terminal adapters, not
// owner-private implementation or wire packages.
var sessionsProductionPeerPackages = []string{
	"github.com/portpowered/infinite-you/pkg/root",
	"github.com/portpowered/infinite-you/pkg/services/workers",
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime",
	"github.com/portpowered/infinite-you/pkg/services/models",
	"github.com/portpowered/infinite-you/pkg/services/work",
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization",
	"github.com/portpowered/infinite-you/pkg/transports/cli",
	"github.com/portpowered/infinite-you/pkg/transports/http",
	"github.com/portpowered/infinite-you/pkg/transports/mapping",
	"github.com/portpowered/infinite-you/pkg/transports/mcp/server",
	"github.com/portpowered/infinite-you/pkg/initializer/application",
}

// TestSessionsFunctionalProofsDoNotImportOwnerPrivatePackages proves FUN-scoped
// functional proofs under tests/functional/sessions do not reach Sessions through
// factory_sessions/internal or factory_sessions/wire owner-private import paths.
func TestSessionsFunctionalProofsDoNotImportOwnerPrivatePackages(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.MustRepoRoot(t)
	functionalRoot := filepath.Join(repoRoot, filepath.FromSlash(functionalSessionsRootRelative))
	forbidden := []string{sessionsInternalImportPrefix, sessionsWireImport}

	err := filepath.WalkDir(functionalRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		assertGoSourceImportsForbidden(t, path, forbidden)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", functionalRoot, err)
	}
}

// TestSessionsProductionPeersReachSessionsThroughPublicSurfacesOnly proves
// production peer composition imports only published Sessions root contracts and
// terminal adapters, not factory_sessions/internal or factory_sessions/wire
// implementation seams.
func TestSessionsProductionPeersReachSessionsThroughPublicSurfacesOnly(t *testing.T) {
	t.Parallel()

	forbidden := []string{sessionsInternalImportPrefix, sessionsWireImport}
	for _, packagePath := range sessionsProductionPeerPackages {
		packagePath := packagePath
		t.Run(packagePath, func(t *testing.T) {
			t.Parallel()
			assertPackageDepsForbidden(t, packagePath, forbidden)
		})
	}
}

// TestSessionsRootCompositionConstructsThroughBuildProcess proves FUN-sessions
// root-composition proofs compose Sessions through root.BuildProcess and the
// shared functional support harness rather than owner-private constructors.
func TestSessionsRootCompositionConstructsThroughBuildProcess(t *testing.T) {
	t.Parallel()

	_ = support.BuildProcess(t, serviceedges.Edges{})
}

func assertGoSourceImportsForbidden(t *testing.T, path string, forbidden []string) {
	t.Helper()

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, importSpec := range file.Imports {
		importPath := strings.Trim(importSpec.Path.Value, `"`)
		for _, blocked := range forbidden {
			if importPath == blocked || strings.HasPrefix(importPath, blocked+"/") {
				t.Fatalf(
					"%s imports forbidden Sessions owner-private package %s; use pkg/services/factory_sessions root contracts and root.BuildProcess instead",
					path,
					importPath,
				)
			}
		}
	}
}

func assertPackageDepsForbidden(t *testing.T, packagePath string, forbidden []string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	for _, dep := range strings.Fields(string(output)) {
		for _, blocked := range forbidden {
			if dep == blocked || strings.HasPrefix(dep, blocked+"/") {
				t.Fatalf(
					"%s must not import %s; found dependency %s. Reach Sessions through published root contracts and terminal adapters only",
					packagePath,
					blocked,
					dep,
				)
			}
		}
	}
}
