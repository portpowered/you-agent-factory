// Command packagedfactorysourcecheck enforces the authored source boundary for
// first-party Factory definitions shipped with the executable.
package main

import (
	"context"
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

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	"gopkg.in/yaml.v3"
)

const (
	authoredBoundary = "packages/packaged-factories/factories"
	diagnosticPrefix = "[agent-factory:packaged-factory-source]"
)

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
	inventoryCount, violations := inspectAuthoredBoundary(repoRoot)
	outside, err := inspectOutsideBoundary(repoRoot)
	if err != nil {
		return err
	}
	violations = append(violations, outside...)
	sort.Strings(violations)
	if len(violations) > 0 {
		return fmt.Errorf("%s source boundary failed:\n- %s", diagnosticPrefix, strings.Join(violations, "\n- "))
	}
	fmt.Fprintf(stdout, "%s authored source boundary holds for %d shipped Factories\n", diagnosticPrefix, inventoryCount)
	return nil
}

func inspectAuthoredBoundary(repoRoot string) (int, []string) {
	inventory, err := packagedfactorycatalog.Discover(
		context.Background(),
		os.DirFS(repoRoot),
		authoredBoundary,
	)
	if err != nil {
		return 0, []string{err.Error()}
	}
	return len(inventory.Entries), nil
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
