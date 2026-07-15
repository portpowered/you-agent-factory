package contracts_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractstaging"
	"github.com/portpowered/infinite-you/internal/testpath"
)

var forbiddenContractToolingImports = []string{
	"github.com/portpowered/infinite-you/internal/contract",
	"github.com/portpowered/infinite-you/internal/javascriptcontractsmoke",
	"github.com/portpowered/infinite-you/contracts",
}

const (
	repositoryImportPrefix          = "github.com/portpowered/infinite-you/"
	javascriptRuntimePackagePattern = repositoryImportPrefix + "pkg/orchestrators/javascript/..."
)

var forbiddenRuntimeManifestNames = []string{
	"runtime-api.json",
	"runtime-manifest.schema.json",
}

type listedJavaScriptPackage struct {
	Dir        string
	ImportPath string
	GoFiles    []string
	CgoFiles   []string
}

func TestJavaScriptRuntimePackagesDoNotImportContractTooling(t *testing.T) {
	t.Parallel()

	cmd := exec.Command(
		"go",
		"list",
		"-deps",
		"-f",
		"{{if not .Standard}}{{.ImportPath}}{{end}}",
		javascriptRuntimePackagePattern,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list JavaScript runtime dependencies: %v\n%s", err, output)
	}

	for _, dependency := range strings.Fields(string(output)) {
		for _, forbidden := range forbiddenContractToolingImports {
			if dependency == forbidden || strings.HasPrefix(dependency, forbidden+"/") {
				t.Fatalf(
					"JavaScript runtime packages must not depend on contract tooling %s; found %s",
					forbidden,
					dependency,
				)
			}
		}
	}
}

func TestJavaScriptRuntimePackagesDoNotLoadContractManifests(t *testing.T) {
	t.Parallel()

	for _, pkg := range listJavaScriptRuntimePackages(t) {
		files := append(slices.Clone(pkg.GoFiles), pkg.CgoFiles...)
		slices.Sort(files)
		for _, file := range files {
			path := filepath.Join(pkg.Dir, file)
			repositoryPath := filepath.ToSlash(filepath.Join(strings.TrimPrefix(pkg.ImportPath, repositoryImportPrefix), file))
			for _, reference := range runtimeManifestReferences(t, path) {
				t.Errorf(
					"JavaScript runtime package %s loads contract manifest %q in %s; remove the runtime dependency and keep contract comparison in structural tooling",
					pkg.ImportPath,
					reference,
					repositoryPath,
				)
			}
		}
	}
}

func listJavaScriptRuntimePackages(t *testing.T) []listedJavaScriptPackage {
	t.Helper()

	cmd := exec.Command("go", "list", "-json", javascriptRuntimePackagePattern)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list JavaScript runtime packages: %v", err)
	}

	decoder := json.NewDecoder(strings.NewReader(string(output)))
	var packages []listedJavaScriptPackage
	for {
		var pkg listedJavaScriptPackage
		err := decoder.Decode(&pkg)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode listed JavaScript runtime package: %v", err)
		}
		packages = append(packages, pkg)
	}
	slices.SortFunc(packages, func(left, right listedJavaScriptPackage) int {
		return strings.Compare(left.ImportPath, right.ImportPath)
	})
	return packages
}

func runtimeManifestReferences(t *testing.T, path string) []string {
	t.Helper()

	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse JavaScript runtime source %s: %v", filepath.ToSlash(path), err)
	}

	var references []string
	ast.Inspect(parsed, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err != nil {
			return true
		}
		normalized := strings.ReplaceAll(value, `\`, "/")
		for _, forbiddenName := range forbiddenRuntimeManifestNames {
			if normalized == forbiddenName || strings.HasSuffix(normalized, "/"+forbiddenName) {
				references = append(references, value)
				break
			}
		}
		return true
	})
	slices.Sort(references)
	return references
}

func TestJavaScriptAuthoredCatalogBoundary(t *testing.T) {
	t.Parallel()

	repositoryRoot := testpath.MustRepoRootFromCaller(t, 0)
	authoredSchema := testpath.MustRepoPathFromCaller(t, 0, "contracts", "javascript", "runtime-manifest.schema.json")
	authoredCatalog := testpath.MustRepoPathFromCaller(t, 0, "contracts", "javascript", "runtime-api.json")

	if _, err := os.Stat(authoredSchema); err != nil {
		t.Fatalf("authored runtime-manifest schema missing: %v", err)
	}
	if _, err := os.Stat(authoredCatalog); err != nil {
		t.Fatalf("authored JavaScript runtime API catalog missing: %v", err)
	}

	allowed := map[string]struct{}{
		"runtime-manifest.schema.json": {},
		"runtime-api.json":             {},
	}
	entries, err := os.ReadDir(testpath.MustRepoPathFromCaller(t, 0, "contracts", "javascript"))
	if err != nil {
		t.Fatalf("read contracts/javascript: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("contracts/javascript must contain only authored contract files, found directory %s", entry.Name())
		}
		if _, ok := allowed[entry.Name()]; !ok {
			t.Fatalf("unexpected authored javascript contract %s under %s", entry.Name(), repositoryRoot)
		}
	}
}

func TestJavaScriptStagedRuntimeAPIProjectsFromAuthoredCatalog(t *testing.T) {
	t.Parallel()

	const (
		wantSource = "contracts/javascript/runtime-api.json"
		wantTarget = "packages/api/generated/javascript/runtime-api.json"
	)

	found := false
	for _, artifact := range contractstaging.RawArtifacts() {
		if artifact.Target != wantTarget {
			continue
		}
		found = true
		if artifact.Source != wantSource {
			t.Fatalf("staged runtime-api source = %q, want authored catalog %q", artifact.Source, wantSource)
		}
	}
	if !found {
		t.Fatalf("missing staged runtime-api projection in RawArtifacts()")
	}
}
