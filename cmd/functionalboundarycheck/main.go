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

type config struct{ path string }

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
	return checkSource(cfg.path)
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	flags := flag.NewFlagSet("functionalboundarycheck", flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("path", defaultScenarioPath, "request-batch functional scenario source path")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	return config{path: *path}, nil
}

func checkSource(path string) error {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		return fmt.Errorf("%s parse request-batch functional scenario %s: %w", diagnosticPrefix, filepath.ToSlash(path), err)
	}
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return fmt.Errorf("%s read import in %s: %w", diagnosticPrefix, filepath.ToSlash(path), err)
		}
		if isForbiddenRequestBatchImport(importPath) {
			return prohibitedInternalImportError(path, importPath)
		}
	}
	return nil
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
