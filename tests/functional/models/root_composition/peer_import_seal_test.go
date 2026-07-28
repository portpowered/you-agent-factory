package root_composition_test

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

const (
	modelsModulePrefix          = "github.com/portpowered/infinite-you/pkg/services/models"
	modelsInternalImportPrefix  = modelsModulePrefix + "/internal"
	modelsWireImport            = modelsModulePrefix + "/wire"
	modelsServiceRootRelative   = "pkg/services/models"
	functionalModelsRootRelative = "tests/functional/models"
)

var modelsPackagedRootDirectories = []string{"internal", "transports", "wire"}

var modelsInternalSubservices = []string{
	"assets",
	"catalog",
	"inference",
	"runtime_host",
	"runtime_scopes",
}

// modelsProductionPeerPackages are composition peers that must reach Models only
// through published root contracts and terminal adapters, not owner-private
// implementation or wire packages.
var modelsProductionPeerPackages = []string{
	"github.com/portpowered/infinite-you/pkg/root",
	"github.com/portpowered/infinite-you/pkg/services/workers",
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime",
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions",
	"github.com/portpowered/infinite-you/pkg/transports/cli",
	"github.com/portpowered/infinite-you/pkg/transports/http",
	"github.com/portpowered/infinite-you/pkg/initializer/application",
}

// TestModelsPackagedRootShapeMatchesCanonicalServiceLayout proves Models ships
// the canonical packaged-service root: only wire/, internal/, and transports/
// package directories plus thin root contract files.
func TestModelsPackagedRootShapeMatchesCanonicalServiceLayout(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.MustRepoRoot(t)
	serviceRoot := filepath.Join(repoRoot, filepath.FromSlash(modelsServiceRootRelative))
	entries, err := os.ReadDir(serviceRoot)
	if err != nil {
		t.Fatalf("ReadDir(%q) = %v", serviceRoot, err)
	}

	var gotRootDirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			gotRootDirs = append(gotRootDirs, entry.Name())
		}
	}
	slices.Sort(gotRootDirs)
	wantRootDirs := slices.Clone(modelsPackagedRootDirectories)
	slices.Sort(wantRootDirs)
	if !slices.Equal(gotRootDirs, wantRootDirs) {
		t.Fatalf("service root directories = %v, want %v", gotRootDirs, wantRootDirs)
	}

	if _, err := os.Stat(filepath.Join(serviceRoot, "service")); err == nil {
		t.Fatal("pkg/services/models/service must not exist; Models remains packaged as wire/, internal/, transports/")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat service/ = %v", err)
	}

	subservicesRoot := filepath.Join(serviceRoot, "internal", "services")
	subentries, err := os.ReadDir(subservicesRoot)
	if err != nil {
		t.Fatalf("ReadDir(%q) = %v", subservicesRoot, err)
	}
	var gotSubservices []string
	for _, entry := range subentries {
		if entry.IsDir() {
			gotSubservices = append(gotSubservices, entry.Name())
		}
	}
	slices.Sort(gotSubservices)
	wantSubservices := slices.Clone(modelsInternalSubservices)
	slices.Sort(wantSubservices)
	if !slices.Equal(gotSubservices, wantSubservices) {
		t.Fatalf("internal/services directories = %v, want %v", gotSubservices, wantSubservices)
	}
}

// TestModelsFunctionalProofsDoNotImportOwnerPrivatePackages proves FUN-scoped
// functional proofs under tests/functional/models do not reach Models through
// models/internal or models/wire owner-private import paths.
func TestModelsFunctionalProofsDoNotImportOwnerPrivatePackages(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.MustRepoRoot(t)
	functionalRoot := filepath.Join(repoRoot, filepath.FromSlash(functionalModelsRootRelative))
	forbidden := []string{modelsInternalImportPrefix, modelsWireImport}

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

// TestModelsProductionPeersReachModelsThroughPublicSurfacesOnly proves
// production peer composition imports only published Models root contracts and
// terminal adapters, not models/internal or models/wire implementation seams.
func TestModelsProductionPeersReachModelsThroughPublicSurfacesOnly(t *testing.T) {
	t.Parallel()

	forbidden := []string{modelsInternalImportPrefix, modelsWireImport}
	for _, packagePath := range modelsProductionPeerPackages {
		packagePath := packagePath
		t.Run(packagePath, func(t *testing.T) {
			t.Parallel()
			assertPackageDepsForbidden(t, packagePath, forbidden)
		})
	}
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
					"%s imports forbidden Models owner-private package %s; use pkg/services/models root contracts and root.BuildProcess instead",
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
					"%s must not import %s; found dependency %s. Reach Models through published root contracts and terminal adapters only",
					packagePath,
					blocked,
					dep,
				)
			}
		}
	}
}
