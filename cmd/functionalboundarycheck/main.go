// functionalboundarycheck keeps customer-boundary request-batch scenarios from
// regressing into direct runtime assertions.
package main

import (
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultScenarioPath = "tests/functional/runtime_api/api_request_batch_boundary_smoke_test.go"
	diagnosticPrefix    = "[agent-factory:functional-boundary]"
)

var forbiddenRequestBatchImports = []string{
	"github.com/portpowered/infinite-you/pkg/factory/",
	"github.com/portpowered/infinite-you/pkg/service",
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri",
}

type config struct {
	root string
	path string
}

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stderr io.Writer) error {
	cfg, err := parseConfig(args, stderr)
	if err != nil {
		return err
	}
	return checkSource(cfg.root, cfg.path)
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	flags := flag.NewFlagSet("functionalboundarycheck", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root")
	path := flags.String("path", defaultScenarioPath, "request-batch functional scenario source path")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	return config{root: *root, path: *path}, nil
}

func checkSource(root, path string) error {
	relativePath, sourcePath, err := functionalTestPath(root, path)
	if err != nil {
		return err
	}
	file, err := parser.ParseFile(token.NewFileSet(), sourcePath, nil, parser.ImportsOnly)
	if err != nil {
		return fmt.Errorf("%s parse request-batch functional scenario %s: %w", diagnosticPrefix, relativePath, err)
	}
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return fmt.Errorf("%s read import in %s: %w", diagnosticPrefix, filepath.ToSlash(path), err)
		}
		if isForbiddenRequestBatchImport(importPath) {
			return prohibitedInternalImportError(relativePath, importPath)
		}
	}
	return nil
}

func functionalTestPath(root, path string) (string, string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", fmt.Errorf("%s resolve repository root: %w", diagnosticPrefix, err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", fmt.Errorf("%s resolve functional scenario path: %w", diagnosticPrefix, err)
	}
	relativePath, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", "", fmt.Errorf("%s resolve functional scenario location: %w", diagnosticPrefix, err)
	}
	relativePath = filepath.ToSlash(relativePath)
	if relativePath == ".." || strings.HasPrefix(relativePath, "../") || !strings.HasPrefix(relativePath, "tests/functional/") || !strings.HasSuffix(relativePath, "_test.go") {
		return "", "", fmt.Errorf("%s request-batch boundary checks apply only to repository tests/functional/*_test.go sources: %s", diagnosticPrefix, relativePath)
	}
	return relativePath, absPath, nil
}

func isForbiddenRequestBatchImport(importPath string) bool {
	for _, forbidden := range forbiddenRequestBatchImports {
		if importPath == strings.TrimSuffix(forbidden, "/") || strings.HasPrefix(importPath, forbidden) {
			return true
		}
	}
	return false
}

func prohibitedInternalImportError(path, importPath string) error {
	return fmt.Errorf(
		"%s prohibited direct request-batch internal import: %s (%s); use generated REST/SSE customers or tests/functional/internal/support instead",
		diagnosticPrefix,
		importPath,
		filepath.ToSlash(path),
	)
}
