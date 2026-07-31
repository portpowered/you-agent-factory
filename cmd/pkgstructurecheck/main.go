// Command pkgstructurecheck enforces the recursive service package shape and
// domain-mirrored functional-test layout.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	baselinePath    = "docs/internal/baselines/package-structure-baseline.json"
	baselineVersion = 1
	baselineStage   = "packaged-structure"

	ruleServiceInterfaceCount        = "service-root-interface-count"
	ruleServiceExportedFunction      = "service-root-exported-function"
	ruleServiceUnexpectedDir         = "service-root-unexpected-directory"
	ruleServiceMissingInternal       = "service-root-missing-internal"
	ruleServiceContainerGoFile       = "service-container-go-file"
	ruleFunctionalShallowFile        = "functional-test-missing-subsection"
	ruleFunctionalUnclassifiedDomain = "functional-test-unclassified-domain"
	ruleRuntimeAPIFile               = "deprecated-runtime-api-file"
	ruleRuntimeAPITest               = "deprecated-runtime-api-test"
)

var deletionGates = map[string]string{
	ruleServiceInterfaceCount:        "reduce the service root to exactly one named interface and delete this exact entry",
	ruleServiceExportedFunction:      "move the exported function behind the service interface or into internal implementation and delete this exact entry",
	ruleServiceUnexpectedDir:         "move the package under wire, internal, transports/<protocol>, or internal/services/<subservice> and delete this exact entry",
	ruleServiceMissingInternal:       "add the subservice's private internal implementation directory and delete this exact entry",
	ruleServiceContainerGoFile:       "move the Go file into a named subservice below internal/services and delete this exact entry",
	ruleFunctionalShallowFile:        "move the functional source into tests/functional/<domain>/<subsection>/... and delete this exact entry",
	ruleFunctionalUnclassifiedDomain: "move the functional source into tests/functional/<domain>/<subsection>/... and delete this exact entry",
	ruleRuntimeAPIFile:               "move the runtime_api source to its durable domain/subsection owner and delete this exact entry",
	ruleRuntimeAPITest:               "move the runtime_api scenario to its durable domain/subsection owner and delete this exact entry",
}

var allowedServiceRootDirectories = map[string]struct{}{
	"internal":   {},
	"transports": {},
	"wire":       {},
}

// allowedFunctionalDomains are the durable product-domain nouns for
// tests/functional/<domain>/<subsection>/... scenario sources. Provider-neutral
// and provider-specific scenarios share providers/<subsection>/..., while
// root-level providers/*.go remains deletion-only aggregate debt.
var allowedFunctionalDomains = map[string]struct{}{
	"transport":         {},
	"workers":           {},
	"orchestration":     {},
	"workstations":      {},
	"work":              {},
	"sessions":          {},
	"factory":           {},
	"providers":         {},
	"provider_sessions": {},
	"events":            {},
	"models":            {},
	"guards":            {},
	"resources":         {},
	"observability":     {},
	"product":           {},
	"resilience":        {},
}

type config struct {
	root           string
	createBaseline bool
}

type finding struct {
	Rule     string
	FilePath string
	Target   string
	Line     int
}

type baseline struct {
	Version int             `json:"version"`
	Stage   string          `json:"stage"`
	Entries []baselineEntry `json:"entries"`
}

type baselineEntry struct {
	Rule     string `json:"rule"`
	FilePath string `json:"filePath"`
	Target   string `json:"target"`
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.root, "root", ".", "repository root to scan")
	flag.BoolVar(&cfg.createBaseline, "create-baseline", false, "create the deletion-only package-structure baseline; refuses to overwrite an existing file")
	flag.Parse()
	if err := run(cfg, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cfg config, stdout, stderr io.Writer) error {
	repoRoot, err := filepath.Abs(cfg.root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	findings, err := scan(repoRoot)
	if err != nil {
		return err
	}
	if cfg.createBaseline {
		return createBaseline(repoRoot, findings, stdout)
	}
	recorded, err := loadBaseline(repoRoot)
	if err != nil {
		return err
	}
	active, unrecorded, stale, err := partition(findings, recorded)
	if err != nil {
		return err
	}
	for _, item := range unrecorded {
		writeFinding(stderr, "new violation", item)
	}
	for _, item := range stale {
		fmt.Fprintf(stderr, "[agent-factory:pkg-structure] stale baseline: %s %s -> %s; remove this entry from %s\n", item.Rule, item.FilePath, item.Target, baselinePath)
	}
	if len(unrecorded) > 0 || len(stale) > 0 {
		return fmt.Errorf("[agent-factory:pkg-structure] found %d new violation(s) and %d stale baseline entries", len(unrecorded), len(stale))
	}
	fmt.Fprintf(stdout, "[agent-factory:pkg-structure] service and functional package structure holds (%d deletion-only baseline entries remain)\n", len(active))
	return nil
}

func scan(repoRoot string) ([]finding, error) {
	var findings []finding
	serviceFindings, err := scanServices(repoRoot)
	if err != nil {
		return nil, err
	}
	findings = append(findings, serviceFindings...)
	functionalFindings, err := scanFunctionalTests(repoRoot)
	if err != nil {
		return nil, err
	}
	findings = append(findings, functionalFindings...)
	slices.SortFunc(findings, func(left, right finding) int {
		return strings.Compare(findingKey(left.Rule, left.FilePath, left.Target), findingKey(right.Rule, right.FilePath, right.Target))
	})
	return slices.CompactFunc(findings, func(left, right finding) bool {
		return findingKey(left.Rule, left.FilePath, left.Target) == findingKey(right.Rule, right.FilePath, right.Target)
	}), nil
}

func scanServices(repoRoot string) ([]finding, error) {
	servicesRoot := filepath.Join(repoRoot, "pkg", "services")
	entries, err := os.ReadDir(servicesRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read service root: %w", err)
	}
	var findings []finding
	for _, entry := range entries {
		if !entry.IsDir() || ignoredDirectory(entry.Name()) {
			continue
		}
		root := filepath.Join(servicesRoot, entry.Name())
		collected, scanErr := scanServiceRoot(repoRoot, root, false)
		if scanErr != nil {
			return nil, scanErr
		}
		findings = append(findings, collected...)
	}
	return findings, nil
}

func scanServiceRoot(repoRoot, serviceRoot string, requireInternal bool) ([]finding, error) {
	relativeRoot, err := relativePath(repoRoot, serviceRoot)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(serviceRoot)
	if err != nil {
		return nil, fmt.Errorf("read service package %s: %w", relativeRoot, err)
	}
	var findings []finding
	var interfaces []string
	hasInternal := false
	for _, entry := range entries {
		path := filepath.Join(serviceRoot, entry.Name())
		if entry.IsDir() {
			if ignoredDirectory(entry.Name()) {
				continue
			}
			if entry.Name() == "internal" {
				hasInternal = true
			}
			if _, allowed := allowedServiceRootDirectories[entry.Name()]; !allowed {
				childPath, relErr := relativePath(repoRoot, path)
				if relErr != nil {
					return nil, relErr
				}
				findings = append(findings, finding{Rule: ruleServiceUnexpectedDir, FilePath: childPath, Target: relativeRoot})
			}
			continue
		}
		if !isProductionGoFile(entry.Name()) {
			continue
		}
		fileInterfaces, fileFindings, scanErr := scanServiceRootFile(repoRoot, path)
		if scanErr != nil {
			return nil, scanErr
		}
		interfaces = append(interfaces, fileInterfaces...)
		findings = append(findings, fileFindings...)
	}
	slices.Sort(interfaces)
	if len(interfaces) != 1 {
		target := "<none>"
		if len(interfaces) > 0 {
			target = strings.Join(interfaces, ",")
		}
		findings = append(findings, finding{Rule: ruleServiceInterfaceCount, FilePath: relativeRoot, Target: target})
	}
	if requireInternal && !hasInternal {
		internalPath := relativeRoot + "/internal"
		findings = append(findings, finding{Rule: ruleServiceMissingInternal, FilePath: internalPath, Target: relativeRoot})
	}

	// Public sibling services/ is non-canonical (flagged above as unexpected)
	// but still walked so existing nested debt remains reviewable until deleted.
	for _, container := range []string{
		filepath.Join(serviceRoot, "services"),
		filepath.Join(serviceRoot, "internal", "services"),
	} {
		collected, scanErr := scanSubserviceContainer(repoRoot, relativeRoot, container)
		if scanErr != nil {
			return nil, scanErr
		}
		findings = append(findings, collected...)
	}
	return findings, nil
}

func scanSubserviceContainer(repoRoot, relativeRoot, container string) ([]finding, error) {
	subentries, err := os.ReadDir(container)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read subservice container %s: %w", filepath.ToSlash(container), err)
	}
	var findings []finding
	for _, entry := range subentries {
		path := filepath.Join(container, entry.Name())
		if entry.IsDir() {
			if ignoredDirectory(entry.Name()) {
				continue
			}
			collected, scanErr := scanServiceRoot(repoRoot, path, true)
			if scanErr != nil {
				return nil, scanErr
			}
			findings = append(findings, collected...)
			continue
		}
		if isGoFile(entry.Name()) {
			relative, relErr := relativePath(repoRoot, path)
			if relErr != nil {
				return nil, relErr
			}
			findings = append(findings, finding{Rule: ruleServiceContainerGoFile, FilePath: relative, Target: relativeRoot})
		}
	}
	return findings, nil
}

func scanServiceRootFile(repoRoot, path string) ([]string, []finding, error) {
	relative, err := relativePath(repoRoot, path)
	if err != nil {
		return nil, nil, err
	}
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("parse service root file %s: %w", relative, err)
	}
	if ast.IsGenerated(file) {
		return nil, nil, nil
	}
	var interfaces []string
	var findings []finding
	for _, declaration := range file.Decls {
		switch value := declaration.(type) {
		case *ast.FuncDecl:
			if value.Recv == nil && ast.IsExported(value.Name.Name) {
				findings = append(findings, finding{Rule: ruleServiceExportedFunction, FilePath: relative, Target: value.Name.Name, Line: fileSet.Position(value.Pos()).Line})
			}
		case *ast.GenDecl:
			if value.Tok != token.TYPE {
				continue
			}
			for _, specification := range value.Specs {
				typeSpec, ok := specification.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if _, ok := typeSpec.Type.(*ast.InterfaceType); ok {
					interfaces = append(interfaces, relative+":"+typeSpec.Name.Name)
				}
			}
		}
	}
	return interfaces, findings, nil
}

func scanFunctionalTests(repoRoot string) ([]finding, error) {
	functionalRoot := filepath.Join(repoRoot, "tests", "functional")
	if _, err := os.Stat(functionalRoot); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("inspect functional test root: %w", err)
	}
	var findings []finding
	err := filepath.WalkDir(functionalRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != functionalRoot && ignoredDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isGoFile(entry.Name()) {
			return nil
		}
		relative, relErr := relativePath(repoRoot, path)
		if relErr != nil {
			return relErr
		}
		fromFunctional, relErr := filepath.Rel(functionalRoot, path)
		if relErr != nil {
			return relErr
		}
		parts := strings.Split(filepath.ToSlash(fromFunctional), "/")
		if len(parts) == 0 {
			return nil
		}
		domain := parts[0]
		// Only tests/functional/internal/support/... is the shared harness exception.
		// Other internal/* roots (for example restclient) are structure debt.
		if domain == "internal" {
			if len(parts) >= 3 && parts[1] == "support" {
				return nil
			}
			if len(parts) < 3 {
				findings = append(findings, finding{Rule: ruleFunctionalShallowFile, FilePath: relative, Target: domain})
				return nil
			}
			findings = append(findings, finding{Rule: ruleFunctionalUnclassifiedDomain, FilePath: relative, Target: domain})
			return nil
		}
		if domain == "runtime_api" {
			findings = append(findings, finding{Rule: ruleRuntimeAPIFile, FilePath: relative, Target: "tests/functional/runtime_api"})
			tests, scanErr := functionalTestNames(path)
			if scanErr != nil {
				return fmt.Errorf("scan deprecated functional test %s: %w", relative, scanErr)
			}
			for _, test := range tests {
				findings = append(findings, finding{Rule: ruleRuntimeAPITest, FilePath: relative, Target: test.name, Line: test.line})
			}
			return nil
		}
		// Conforming scenario sources require tests/functional/<domain>/<subsection>/...
		// Approved domain nouns with that depth are accepted and are not structure debt.
		if len(parts) < 3 {
			findings = append(findings, finding{Rule: ruleFunctionalShallowFile, FilePath: relative, Target: domain})
			return nil
		}
		if isAllowedFunctionalDomain(domain) {
			return nil
		}
		// Deep paths under catch-all or unclassified roots are deletion-only debt.
		// New files under those roots fail immediately rather than becoming durable owners.
		findings = append(findings, finding{Rule: ruleFunctionalUnclassifiedDomain, FilePath: relative, Target: domain})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan functional tests: %w", err)
	}
	return findings, nil
}

type namedLine struct {
	name string
	line int
}

func isAllowedFunctionalDomain(domain string) bool {
	_, ok := allowedFunctionalDomains[domain]
	return ok
}

func functionalTestNames(path string) ([]namedLine, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		return nil, err
	}
	if ast.IsGenerated(file) {
		return nil, nil
	}
	var names []namedLine
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv != nil || !strings.HasPrefix(function.Name.Name, "Test") {
			continue
		}
		names = append(names, namedLine{name: function.Name.Name, line: fileSet.Position(function.Pos()).Line})
	}
	slices.SortFunc(names, func(left, right namedLine) int { return strings.Compare(left.name, right.name) })
	return names, nil
}

func createBaseline(repoRoot string, findings []finding, stdout io.Writer) error {
	if len(findings) == 0 {
		return fmt.Errorf("refusing to create empty %s; absence records zero debt", baselinePath)
	}
	path := filepath.Join(repoRoot, baselinePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create baseline directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("refusing to overwrite existing %s", baselinePath)
	}
	if err != nil {
		return fmt.Errorf("create %s: %w", baselinePath, err)
	}
	entries := make([]baselineEntry, 0, len(findings))
	for _, item := range findings {
		entries = append(entries, baselineEntry{Rule: item.Rule, FilePath: item.FilePath, Target: item.Target})
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(baseline{Version: baselineVersion, Stage: baselineStage, Entries: entries}); err != nil {
		_ = file.Close()
		return fmt.Errorf("encode %s: %w", baselinePath, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", baselinePath, err)
	}
	fmt.Fprintf(stdout, "[agent-factory:pkg-structure] created %s with %d deletion-only entries\n", baselinePath, len(entries))
	return nil
}

func loadBaseline(repoRoot string) (baseline, error) {
	data, err := os.ReadFile(filepath.Join(repoRoot, baselinePath))
	if errors.Is(err, os.ErrNotExist) {
		return baseline{}, nil
	}
	if err != nil {
		return baseline{}, fmt.Errorf("read %s: %w", baselinePath, err)
	}
	var result baseline
	if err := json.Unmarshal(data, &result); err != nil {
		return baseline{}, fmt.Errorf("decode %s: %w", baselinePath, err)
	}
	if result.Version != baselineVersion {
		return baseline{}, fmt.Errorf("%s version = %d, want %d", baselinePath, result.Version, baselineVersion)
	}
	if result.Stage != baselineStage {
		return baseline{}, fmt.Errorf("%s stage = %q, want %q", baselinePath, result.Stage, baselineStage)
	}
	if len(result.Entries) == 0 {
		return baseline{}, fmt.Errorf("%s is empty; delete the file to record zero debt", baselinePath)
	}
	return result, nil
}

func partition(findings []finding, recorded baseline) ([]finding, []finding, []baselineEntry, error) {
	recordedByKey := make(map[string]baselineEntry, len(recorded.Entries))
	for index, entry := range recorded.Entries {
		if err := validateBaselineEntry(entry); err != nil {
			return nil, nil, nil, fmt.Errorf("%s entry %d: %w", baselinePath, index, err)
		}
		key := findingKey(entry.Rule, entry.FilePath, entry.Target)
		if _, duplicate := recordedByKey[key]; duplicate {
			return nil, nil, nil, fmt.Errorf("%s contains duplicate entry for %s", baselinePath, key)
		}
		recordedByKey[key] = entry
	}
	seen := make(map[string]struct{}, len(findings))
	var active, unrecorded []finding
	for _, item := range findings {
		key := findingKey(item.Rule, item.FilePath, item.Target)
		seen[key] = struct{}{}
		if _, exists := recordedByKey[key]; exists {
			active = append(active, item)
		} else {
			unrecorded = append(unrecorded, item)
		}
	}
	var stale []baselineEntry
	for key, entry := range recordedByKey {
		if _, exists := seen[key]; !exists {
			stale = append(stale, entry)
		}
	}
	slices.SortFunc(stale, func(left, right baselineEntry) int {
		return strings.Compare(findingKey(left.Rule, left.FilePath, left.Target), findingKey(right.Rule, right.FilePath, right.Target))
	})
	return active, unrecorded, stale, nil
}

func validateBaselineEntry(entry baselineEntry) error {
	if strings.TrimSpace(entry.Rule) == "" || strings.TrimSpace(entry.FilePath) == "" || strings.TrimSpace(entry.Target) == "" {
		return errors.New("rule, filePath, and target are required")
	}
	if _, known := deletionGates[entry.Rule]; !known {
		return fmt.Errorf("unknown rule %q", entry.Rule)
	}
	for _, value := range []string{entry.FilePath, entry.Target} {
		if strings.ContainsAny(value, "*?[]") {
			return errors.New("wildcards are not permitted")
		}
	}
	return nil
}

func writeFinding(writer io.Writer, prefix string, item finding) {
	location := item.FilePath
	if item.Line > 0 {
		location = fmt.Sprintf("%s:%d", item.FilePath, item.Line)
	}
	fmt.Fprintf(writer, "[agent-factory:pkg-structure] %s: %s %s -> %s\n", prefix, item.Rule, location, item.Target)
	fmt.Fprintf(writer, "  deletion gate: %s\n", deletionGates[item.Rule])
}

func findingKey(rule, filePath, target string) string {
	return rule + "\x00" + filepath.ToSlash(filePath) + "\x00" + target
}

func relativePath(repoRoot, path string) (string, error) {
	relative, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return "", fmt.Errorf("resolve relative path for %s: %w", filepath.ToSlash(path), err)
	}
	return filepath.ToSlash(relative), nil
}

func isGoFile(name string) bool {
	return strings.HasSuffix(name, ".go")
}

func isProductionGoFile(name string) bool {
	return isGoFile(name) && !strings.HasSuffix(name, "_test.go")
}

func ignoredDirectory(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor":
		return true
	default:
		return false
	}
}
