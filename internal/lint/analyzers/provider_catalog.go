package analyzers

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing/fstest"

	"github.com/portpowered/infinite-you/internal/providercatalog"
	"golang.org/x/tools/go/analysis"
)

const providerInputs = "packages/model-providers/providers"

// ProviderCatalog validates authored provider facts and exact published outputs.
var ProviderCatalog = &analysis.Analyzer{
	Name: "providercatalog",
	Doc:  "validate provider catalog inputs and generated artifacts",
	Run: func(pass *analysis.Pass) (any, error) {
		if unit, ok := unitKey(pass); !ok || unit != "internal/providercatalog" {
			return nil, nil
		}
		return ProviderCatalogForDirectory(filepath.Dir(pass.Fset.Position(pass.Files[0].Package).Filename)).Run(pass)
	},
}

// ProviderCatalogForDirectory binds external data before issue-cache lookup.
// Git supplies the inventory, including ignored populated input directories;
// validation receives only memory and never traverses the working tree.
func ProviderCatalogForDirectory(directory string) *analysis.Analyzer {
	output, err := exec.Command("git", "-C", directory, "rev-parse", "--show-toplevel").CombinedOutput()
	if err != nil {
		return providerCatalogSnapshot(nil, fmt.Errorf("locate provider repository: %w: %s", err, output))
	}
	root := strings.TrimSpace(string(output))
	output, err = exec.Command("git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", providerInputs).CombinedOutput()
	if err != nil {
		return providerCatalogSnapshot(nil, fmt.Errorf("list provider inputs: %w: %s", err, output))
	}
	ignored, err := exec.Command("git", "-C", root, "ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--", providerInputs).CombinedOutput()
	if err != nil {
		return providerCatalogSnapshot(nil, fmt.Errorf("list ignored provider inputs: %w: %s", err, ignored))
	}
	return providerCatalogSnapshot(readProviderSnapshot(root, strings.Split(string(output)+string(ignored), "\x00")))
}

func readProviderSnapshot(root string, listed []string) (fstest.MapFS, error) {
	names := append([]string{"api/openapi.yaml", providercatalog.CatalogPath, providercatalog.ManifestSchemaPath,
		providercatalog.CatalogSchemaPath, providercatalog.RuntimeCatalogPath}, listed...)
	sort.Strings(names)
	snapshot := fstest.MapFS{}
	for _, name := range names {
		if name == "" || snapshot[name] != nil {
			continue
		}
		if !fs.ValidPath(name) || strings.Contains(name, "\\") || strings.Contains(name, ":") {
			return snapshot, fmt.Errorf("unsafe provider input path %q", name)
		}
		target := filepath.Join(root, filepath.FromSlash(name))
		if strings.HasPrefix(name, providerInputs+"/") && filepath.Base(name) != "provider.yaml" && filepath.Base(name) != "harness.yaml" {
			// Preserve populated-directory evidence without reading unrelated content.
			if _, err := os.Stat(target); errors.Is(err, fs.ErrNotExist) {
				continue
			} else if err != nil {
				return snapshot, fmt.Errorf("inspect %s: %w", name, err)
			}
			snapshot[name] = &fstest.MapFile{Mode: 0o644}
			continue
		}
		data, err := os.ReadFile(target)
		if errors.Is(err, fs.ErrNotExist) {
			continue // Deleted tracked files are absent from the working snapshot.
		}
		if err != nil {
			return snapshot, fmt.Errorf("read %s: %w", name, err)
		}
		snapshot[name] = &fstest.MapFile{Data: data, Mode: 0o644}
	}
	return snapshot, nil
}

func providerCatalogSnapshot(source fstest.MapFS, failure error) *analysis.Analyzer {
	// Clone inputs so independent invocations cannot mutate an existing binding.
	snapshot := fstest.MapFS{}
	names := make([]string, 0, len(source))
	for name, file := range source {
		snapshot[name] = &fstest.MapFile{Data: bytes.Clone(file.Data), Mode: file.Mode}
		names = append(names, name)
	}
	sort.Strings(names)
	hash := sha256.New()
	for _, name := range names {
		fmt.Fprintf(hash, "%d:%s:%d:", len(name), name, len(snapshot[name].Data))
		hash.Write(snapshot[name].Data)
	}
	fmt.Fprintf(hash, "failure:%v", failure)
	return &analysis.Analyzer{
		Name: fmt.Sprintf("providercatalog_%x", hash.Sum(nil)[:12]),
		Doc:  "validate provider catalog inputs and generated artifacts",
		Run: func(pass *analysis.Pass) (any, error) {
			if unit, ok := unitKey(pass); !ok || unit != "internal/providercatalog" {
				return nil, nil
			}
			if failure != nil {
				pass.Reportf(pass.Files[0].Package, "provider-catalog: %s", failure)
				return nil, nil
			}
			catalog, err := providercatalog.Build(snapshot)
			if err != nil {
				pass.Reportf(pass.Files[0].Package, "provider-catalog: %s", err)
				return nil, nil
			}
			paths := make([]string, 0, len(catalog.Files))
			for name := range catalog.Files {
				paths = append(paths, name)
			}
			sort.Strings(paths)
			for _, name := range paths {
				actual, exists := snapshot[name]
				kind := "stale"
				if !exists {
					kind = "missing"
				} else if bytes.Equal(actual.Data, catalog.Files[name]) {
					continue
				}
				pass.Reportf(pass.Files[0].Package, "provider-catalog: %s: %s; run `make provider-catalog-generate`", kind, name)
			}
			return nil, nil
		},
	}
}
