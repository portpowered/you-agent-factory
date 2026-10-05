package analyzers

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	"golang.org/x/tools/go/analysis"
)

const (
	packagedSourceUnit        = "packages/packaged-factories"
	packagedCatalogReportUnit = "internal/lint/analyzers"
)

var PackagedFactorySourceInputs = &analysis.Analyzer{
	Name: "packagedfactorysourceinputs",
	Doc:  "validate compiler-declared shipped Factory source inputs",
	Run: func(pass *analysis.Pass) (any, error) {
		if unit, ok := unitKey(pass); !ok || unit != packagedCatalogReportUnit {
			return nil, nil
		}
		return PackagedFactorySourceInputsForDirectory(filepath.Dir(pass.Fset.Position(pass.Files[0].Package).Filename)).Run(pass)
	},
}

// PackagedFactorySourceInputsForDirectory freezes compiler-owned inputs before the
// host's issue-cache lookup. Its name fingerprints bytes, inventory and errors;
// changing a non-Go asset cannot reuse a previous clean result.
func PackagedFactorySourceInputsForDirectory(directory string) *analysis.Analyzer {
	command := exec.Command("go", "list", "-e", "-json", modulePrefix+packagedSourceUnit)
	command.Dir = directory
	metadata, err := command.Output()
	var snapshot []byte
	if err == nil {
		snapshot, err = packagedCompilerSnapshot(metadata, os.ReadFile)
	}
	return packagedSourceInputsSnapshot(metadata, snapshot, err)
}

func packagedCompilerSnapshot(metadata []byte, read func(string) ([]byte, error)) ([]byte, error) {
	var unit struct {
		Dir, ImportPath string
		EmbedFiles      []string
		Error           *struct{ Err string }
		DepsErrors      []struct{ Err string }
	}
	if err := json.Unmarshal(metadata, &unit); err != nil {
		return nil, fmt.Errorf("invalid compiler metadata: %w", err)
	}
	if unit.Error != nil {
		return nil, fmt.Errorf("incomplete compiler metadata: %s", unit.Error.Err)
	}
	if len(unit.DepsErrors) > 0 {
		return nil, fmt.Errorf("incomplete compiler metadata: %s", unit.DepsErrors[0].Err)
	}
	if unit.ImportPath != modulePrefix+packagedSourceUnit || !filepath.IsAbs(unit.Dir) {
		return nil, fmt.Errorf("compiler metadata lacks the packaged source owner/directory")
	}
	// Validate the entire declaration before performing any read, including
	// non-authored embedded names. Never normalize an unsafe name into safety.
	seen := map[string]bool{}
	for _, name := range unit.EmbedFiles {
		if !fs.ValidPath(name) || strings.ContainsAny(name, "\\:") || seen[name] {
			return nil, fmt.Errorf("unsafe or duplicate compiler embedded path %q", name)
		}
		seen[name] = true
	}
	sort.Strings(unit.EmbedFiles)
	return archivePackagedInputs(unit.Dir, unit.EmbedFiles, read)
}

func archivePackagedInputs(directory string, names []string, read func(string) ([]byte, error)) ([]byte, error) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	count := 0
	for _, name := range names {
		if !under(name, "factories") {
			continue
		}
		payload, err := read(filepath.Join(directory, filepath.FromSlash(name)))
		if err != nil {
			return nil, fmt.Errorf("read compiler input %s: %w", name, err)
		}
		entry, err := writer.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := entry.Write(payload); err != nil {
			return nil, err
		}
		count++
	}
	if count == 0 {
		return nil, fmt.Errorf("compiler metadata contains no authored Factory inputs")
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func packagedSourceInputsSnapshot(metadata, snapshot []byte, failure error) *analysis.Analyzer {
	digest := sha256.Sum256(append(append(append([]byte{}, metadata...), snapshot...), []byte(fmt.Sprint(failure))...))
	// Evaluate once against a private read-only archive. The captured result is
	// immutable even if the compiler-declared host files change during analysis.
	if failure == nil {
		var source *zip.Reader
		source, failure = zip.NewReader(bytes.NewReader(snapshot), int64(len(snapshot)))
		if failure == nil {
			_, failure = packagedfactorycatalog.Discover(context.Background(), source, "factories")
		}
	}
	return &analysis.Analyzer{
		Name: fmt.Sprintf("packagedfactorysourceinputs_%x", digest[:12]),
		Doc:  "validate compiler-declared shipped Factory source inputs",
		Run: func(pass *analysis.Pass) (any, error) {
			// Use a surviving canonical lint owner so removal of the entire
			// shipped package still fails closed instead of losing its observer.
			if unit, ok := unitKey(pass); ok && unit == packagedCatalogReportUnit && failure != nil {
				pass.Reportf(pass.Files[0].Package, "packaged-factory-source: %s: %s", packagedSourceBoundary, failure)
			}
			return nil, nil
		},
	}
}
