package functionaltestviz

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/internal/functionaltestmetadata"
)

// DefaultOutputPath is the repository-relative Markdown catalog artifact path.
// Parent directories are created as needed when writing.
const DefaultOutputPath = ".artifacts/functional-test-viz/functional-tests.md"

// DefaultFunctionalRoot is the repository-relative functional test tree parsed
// for catalog inventory.
const DefaultFunctionalRoot = "tests/functional"

// GenerateConfig configures the callable Markdown catalog writer.
type GenerateConfig struct {
	// RepositoryRoot resolves golden manifest paths and default relative
	// functional/output paths. Empty defaults to ".".
	RepositoryRoot string
	// FunctionalRoot is the functional test tree passed to
	// functionaltestmetadata.Parse. Empty defaults to
	// RepositoryRoot/tests/functional.
	FunctionalRoot string
	// CoverageSummaryPath is the required gocoveragecheck coverage-summary JSON
	// path. Relative paths are used as given (process-relative).
	CoverageSummaryPath string
	// OutputPath is the Markdown destination. Empty defaults to
	// RepositoryRoot/DefaultOutputPath. Parent directories are created as needed.
	OutputPath string
}

// Generate loads functionaltestmetadata inventory and coverage-summary JSON,
// attaches golden provenance, renders the catalog, and writes Markdown to the
// configured output path. It does not run the functional suite.
func Generate(cfg GenerateConfig) error {
	normalized, err := normalizeGenerateConfig(cfg)
	if err != nil {
		return err
	}

	records, err := functionaltestmetadata.Parse(normalized.FunctionalRoot)
	if err != nil {
		return fmt.Errorf("parse functional test metadata from %s: %w", normalized.FunctionalRoot, err)
	}

	inputs, err := AssembleCatalogInputs(records, normalized.CoverageSummaryPath)
	if err != nil {
		return err
	}

	withProvenance, err := AttachGoldenProvenance(inputs.Records, normalized.RepositoryRoot)
	if err != nil {
		return err
	}
	inputs.Records = withProvenance

	markdown, err := RenderCatalogMarkdown(inputs)
	if err != nil {
		return err
	}
	return WriteCatalogFile(normalized.OutputPath, markdown)
}

// WriteCatalogFile writes Markdown to path, creating parent directories as needed.
func WriteCatalogFile(path, markdown string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("catalog output path is required")
	}
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create catalog output directory %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(path, []byte(markdown), 0o644); err != nil {
		return fmt.Errorf("write catalog markdown %s: %w", path, err)
	}
	return nil
}

func normalizeGenerateConfig(cfg GenerateConfig) (GenerateConfig, error) {
	root := strings.TrimSpace(cfg.RepositoryRoot)
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return GenerateConfig{}, fmt.Errorf("resolve repository root %q: %w", root, err)
	}

	functionalRoot := strings.TrimSpace(cfg.FunctionalRoot)
	if functionalRoot == "" {
		functionalRoot = filepath.Join(absRoot, filepath.FromSlash(DefaultFunctionalRoot))
	}

	coveragePath := strings.TrimSpace(cfg.CoverageSummaryPath)
	if coveragePath == "" {
		return GenerateConfig{}, fmt.Errorf("coverage-summary path is required")
	}

	outputPath := strings.TrimSpace(cfg.OutputPath)
	if outputPath == "" {
		outputPath = filepath.Join(absRoot, filepath.FromSlash(DefaultOutputPath))
	}

	return GenerateConfig{
		RepositoryRoot:      absRoot,
		FunctionalRoot:      functionalRoot,
		CoverageSummaryPath: coveragePath,
		OutputPath:          outputPath,
	}, nil
}
