// Command packagedfactorysourcecheck enforces the authored source boundary for
// first-party Factory definitions shipped with the executable.
package main

import (
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	authoredBoundary = "packages/packaged-factories/factories"
	diagnosticPrefix = "[agent-factory:packaged-factory-source]"
)

var requiredFactories = []string{
	"deep-research",
	"fusion",
	"goal",
	"quorum",
	"review",
	"subagent",
	"tts",
}

var rootDocumentNames = map[string]struct{}{
	"factory.json": {},
	"factory.yaml": {},
	"factory.yml":  {},
}

var excludedDirectoryNames = map[string]struct{}{
	".git":         {},
	".artifacts":   {},
	"coverage":     {},
	"dist":         {},
	"examples":     {},
	"fixtures":     {},
	"node_modules": {},
	"testdata":     {},
	"tests":        {},
	"vendor":       {},
}

type config struct {
	root string
}

type factoryIdentity struct {
	Name string `yaml:"name"`
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.root, "root", ".", "repository root to scan")
	flag.Parse()
	if err := run(cfg, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cfg config, stdout io.Writer) error {
	repoRoot, err := filepath.Abs(cfg.root)
	if err != nil {
		return fmt.Errorf("%s resolve repository root: %w", diagnosticPrefix, err)
	}
	var violations []string
	violations = append(violations, inspectAuthoredBoundary(repoRoot)...)
	outside, err := inspectOutsideBoundary(repoRoot)
	if err != nil {
		return err
	}
	violations = append(violations, outside...)
	sort.Strings(violations)
	if len(violations) > 0 {
		return fmt.Errorf("%s source boundary failed:\n- %s", diagnosticPrefix, strings.Join(violations, "\n- "))
	}
	fmt.Fprintf(stdout, "%s authored source boundary holds for %d shipped Factories\n", diagnosticPrefix, len(requiredFactories))
	return nil
}

func inspectAuthoredBoundary(repoRoot string) []string {
	boundaryPath := filepath.Join(repoRoot, filepath.FromSlash(authoredBoundary))
	entries, err := os.ReadDir(boundaryPath)
	if err != nil {
		return []string{fmt.Sprintf("inspect required source boundary %s: %v", authoredBoundary, err)}
	}

	found := make(map[string]struct{}, len(entries))
	var violations []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		found[entry.Name()] = struct{}{}
		violations = append(violations, inspectFactoryDirectory(boundaryPath, entry.Name())...)
	}
	for _, name := range requiredFactories {
		if _, ok := found[name]; !ok {
			violations = append(violations, fmt.Sprintf("missing shipped Factory directory %s/%s", authoredBoundary, name))
		}
	}
	return violations
}

func inspectFactoryDirectory(boundaryPath, name string) []string {
	directory := filepath.Join(boundaryPath, name)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return []string{fmt.Sprintf("inspect %s/%s: %v", authoredBoundary, name, err)}
	}
	var roots []string
	for _, entry := range entries {
		if !entry.IsDir() {
			if _, ok := rootDocumentNames[strings.ToLower(entry.Name())]; ok {
				roots = append(roots, entry.Name())
			}
		}
	}
	sort.Strings(roots)
	if len(roots) != 1 {
		return []string{fmt.Sprintf(
			"%s/%s contains %d root Factory documents (%s); keep exactly one of factory.json, factory.yaml, or factory.yml",
			authoredBoundary,
			name,
			len(roots),
			strings.Join(roots, ", "),
		)}
	}

	path := filepath.Join(directory, roots[0])
	identity, err := readFactoryIdentity(path)
	if err != nil {
		return []string{fmt.Sprintf("read %s/%s/%s: %v", authoredBoundary, name, roots[0], err)}
	}
	wantName := "@you/" + name
	if identity.Name != wantName {
		return []string{fmt.Sprintf(
			"%s/%s/%s declares name %q; want %q so directory and shipped Factory identity agree",
			authoredBoundary,
			name,
			roots[0],
			identity.Name,
			wantName,
		)}
	}
	return nil
}

func inspectOutsideBoundary(repoRoot string) ([]string, error) {
	boundaryPath := filepath.Clean(filepath.Join(repoRoot, filepath.FromSlash(authoredBoundary)))
	var violations []string
	err := filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == boundaryPath {
				return filepath.SkipDir
			}
			if path != repoRoot && excludedDirectory(path, entry.Name(), repoRoot) {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		identities, err := firstPartyIdentities(path, entry.Name())
		if err != nil {
			return fmt.Errorf("inspect %s: %w", relative, err)
		}
		for _, identity := range identities {
			violations = append(violations, outsideBoundaryViolation(relative, identity.Name))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%s scan repository: %w", diagnosticPrefix, err)
	}
	return violations, nil
}

func firstPartyIdentities(path, name string) ([]factoryIdentity, error) {
	if _, ok := rootDocumentNames[strings.ToLower(name)]; ok {
		identity, err := readFactoryIdentity(path)
		if err == nil && strings.HasPrefix(identity.Name, "@you/") {
			return []factoryIdentity{identity}, nil
		}
		return nil, nil
	}
	lowerName := strings.ToLower(name)
	if !strings.HasSuffix(lowerName, ".go") || strings.HasSuffix(lowerName, "_test.go") {
		return nil, nil
	}
	return embeddedFirstPartyIdentities(path)
}

func embeddedFirstPartyIdentities(path string) ([]factoryIdentity, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	if ast.IsGenerated(file) {
		return nil, nil
	}
	byName := make(map[string]factoryIdentity)
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err != nil {
			return true
		}
		identity, err := decodeFactoryIdentity([]byte(value))
		if err == nil && strings.HasPrefix(identity.Name, "@you/") {
			byName[identity.Name] = identity
		}
		return true
	})
	identities := make([]factoryIdentity, 0, len(byName))
	for _, identity := range byName {
		identities = append(identities, identity)
	}
	sort.Slice(identities, func(i, j int) bool { return identities[i].Name < identities[j].Name })
	return identities, nil
}

func outsideBoundaryViolation(relative, name string) string {
	return fmt.Sprintf(
		"%s declares shipped first-party Factory %q outside %s; move the authored definition and owned assets under the required source boundary",
		relative,
		name,
		authoredBoundary,
	)
}

func excludedDirectory(path, name, repoRoot string) bool {
	if _, excluded := excludedDirectoryNames[strings.ToLower(name)]; excluded {
		return true
	}
	relative, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return false
	}
	// The checked-in factory scaffold is customer-authored repository content,
	// not a definition shipped by the packaged Factory catalog.
	return filepath.ToSlash(relative) == "factory"
}

func readFactoryIdentity(path string) (factoryIdentity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return factoryIdentity{}, err
	}
	return decodeFactoryIdentity(data)
}

func decodeFactoryIdentity(data []byte) (factoryIdentity, error) {
	var identity factoryIdentity
	if err := yaml.Unmarshal(data, &identity); err != nil {
		return factoryIdentity{}, err
	}
	identity.Name = strings.TrimSpace(identity.Name)
	if identity.Name == "" {
		return factoryIdentity{}, errors.New("missing non-empty name")
	}
	return identity, nil
}
