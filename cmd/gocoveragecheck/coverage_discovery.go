package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

// coveragePackageListing is the small portion of go list metadata needed to
// derive both coverage package sets. The listing is intentionally scoped to
// one invocation; it is never persisted between checker runs.
type coveragePackageListing struct {
	importPath   string
	directory    string
	packageName  string
	goFiles      int
	hasGoFiles   bool
	testGoFiles  []string
	xTestGoFiles []string
}

const coverageUnitGoListJSONFields = "-json=ImportPath,Dir,Name,GoFiles,TestGoFiles,XTestGoFiles,Incomplete,Error"

type coverageGoListPackage struct {
	ImportPath   string
	Dir          string
	Name         string
	GoFiles      []string
	TestGoFiles  []string
	XTestGoFiles []string
	Incomplete   bool
	Error        *coverageGoListPackageError
}

type coverageGoListPackageError struct {
	Pos string
	Err string
}

type coveragePackageDiscovery struct {
	allPackages      []string
	unitPackageFiles []coveragePackageListing
}

func resolveCoverageLaneWithDiscovery(cfg config) (coveragePackageDiscovery, []string, []string, error) {
	return resolveCoverageLaneWithDiscoveryForOS(cfg, "")
}

func resolveCoverageLaneWithDiscoveryForOS(cfg config, targetOS string) (coveragePackageDiscovery, []string, []string, error) {
	coverConfigured := strings.TrimSpace(cfg.coverpkg) != ""
	testConfigured := strings.TrimSpace(cfg.packages) != ""
	unitDefaultDiscovery := !testConfigured && (cfg.suite == "" || cfg.suite == unitCoverageSuite)
	patterns := make([]string, 0, 2)
	if !coverConfigured {
		patterns = appendUniqueCoveragePatterns(patterns, defaultCoveragePatterns...)
	}
	if !testConfigured {
		switch cfg.suite {
		case "", unitCoverageSuite:
			patterns = appendUniqueCoveragePatterns(patterns, unitTestPatterns...)
		case functionalCoverageSuite:
			patterns = appendUniqueCoveragePatterns(patterns, functionalTestPatterns...)
		default:
			return coveragePackageDiscovery{}, nil, nil, fmt.Errorf("resolve go coverage lane: unsupported suite %q", cfg.suite)
		}
	}

	var listings []coveragePackageListing
	var err error
	if len(patterns) > 0 {
		if unitDefaultDiscovery {
			listings, err = listUnitGoPackageListings(patterns, targetOS)
		} else {
			listings, err = listGoPackageListings(patterns)
		}
		if err != nil {
			return coveragePackageDiscovery{}, nil, nil, err
		}
	}

	var coverPackages []string
	if coverConfigured {
		coverPackages = splitList(cfg.coverpkg, ",", false)
	} else {
		coverPackages, err = filterCoveragePackageListings(listings, isBackendCoveragePackage, true)
		if err != nil {
			return coveragePackageDiscovery{}, nil, nil, err
		}
	}

	var testPackages []string
	if testConfigured {
		testPackages = splitList(cfg.packages, " ", true)
	} else {
		include := isBackendCoveragePackage
		if cfg.suite == functionalCoverageSuite {
			include = isFunctionalTestPackage
		}
		if unitDefaultDiscovery {
			testPackages, err = filterUnitTestPackageListings(listings, include)
		} else {
			testPackages, err = filterCoveragePackageListings(listings, include, false)
		}
		if err != nil {
			return coveragePackageDiscovery{}, nil, nil, err
		}
	}

	discovery := coveragePackageDiscovery{}
	if unitDefaultDiscovery {
		discovery.allPackages = allListedPackagePaths(listings)
		discovery.unitPackageFiles = append([]coveragePackageListing(nil), listings...)
	}
	return discovery, coverPackages, testPackages, nil
}

func appendUniqueCoveragePatterns(patterns []string, additions ...string) []string {
	for _, addition := range additions {
		if slices.Contains(patterns, addition) {
			continue
		}
		patterns = append(patterns, addition)
	}
	return patterns
}

func listGoPackageListings(patterns []string) ([]coveragePackageListing, error) {
	args := append([]string{"list", "-f", "{{.ImportPath}}\t{{len .GoFiles}}"}, patterns...)
	rootDir, err := repoRootDir()
	if err != nil {
		return nil, err
	}
	stdout, stderr, err := runCommand(commandInvocation{
		name: "go",
		args: args,
		env:  os.Environ(),
		dir:  rootDir,
	})
	if err != nil {
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = strings.TrimSpace(stdout)
		}
		if detail != "" {
			return nil, fmt.Errorf("list go packages: %w\n%s", err, detail)
		}
		return nil, fmt.Errorf("list go packages: %w", err)
	}

	listings := make([]coveragePackageListing, 0)
	for _, line := range strings.Split(stdout, "\n") {
		importPath, goFiles, hasGoFiles := parseGoListPackageLine(line)
		if strings.TrimSpace(importPath) == "" {
			continue
		}
		listings = append(listings, coveragePackageListing{
			importPath: importPath,
			goFiles:    goFiles,
			hasGoFiles: hasGoFiles,
		})
	}
	return listings, nil
}

func listUnitGoPackageListings(patterns []string, targetOS string) ([]coveragePackageListing, error) {
	args := []string{"list", "-e", coverageUnitGoListJSONFields, "-find"}
	args = append(args, patterns...)
	rootDir, err := repoRootDir()
	if err != nil {
		return nil, err
	}
	stdout, stderr, err := runCommand(commandInvocation{
		name: "go",
		args: args,
		env:  coverageDiscoveryEnvironment(targetOS),
		dir:  rootDir,
	})
	if err != nil {
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = strings.TrimSpace(stdout)
		}
		if detail != "" {
			return nil, fmt.Errorf("list build-aware unit packages: %w\n%s", err, detail)
		}
		return nil, fmt.Errorf("list build-aware unit packages: %w", err)
	}

	listings, err := decodeUnitGoListPackages(stdout)
	if err != nil {
		return nil, err
	}
	return mergeUnitGoListPackages(listings)
}

func coverageDiscoveryEnvironment(targetOS string) []string {
	if strings.TrimSpace(targetOS) == "" {
		return os.Environ()
	}
	environment := os.Environ()
	prefix := "GOOS="
	for index, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			environment[index] = prefix + targetOS
			return environment
		}
	}
	return append(environment, prefix+targetOS)
}

func decodeUnitGoListPackages(stdout string) ([]coveragePackageListing, error) {
	decoder := json.NewDecoder(strings.NewReader(stdout))
	listings := make([]coveragePackageListing, 0)
	for {
		var pkg coverageGoListPackage
		if err := decoder.Decode(&pkg); err != nil {
			if errors.Is(err, io.EOF) {
				if len(listings) == 0 {
					return nil, errors.New("list build-aware unit packages: go list returned no package metadata")
				}
				return listings, nil
			}
			return nil, fmt.Errorf("list build-aware unit packages: decode go list metadata: %w", err)
		}
		if pkg.Error != nil {
			detail := strings.TrimSpace(pkg.Error.Err)
			if detail == "" {
				detail = "package listing failed"
			}
			if position := strings.TrimSpace(pkg.Error.Pos); position != "" {
				return nil, fmt.Errorf("list build-aware unit packages: go list package %q at %s: %s", pkg.ImportPath, position, detail)
			}
			return nil, fmt.Errorf("list build-aware unit packages: go list package %q: %s", pkg.ImportPath, detail)
		}
		if strings.TrimSpace(pkg.ImportPath) == "" {
			return nil, errors.New("list build-aware unit packages: go list returned package metadata without an import path")
		}
		if pkg.Incomplete {
			return nil, fmt.Errorf("list build-aware unit packages: go list returned incomplete metadata for package %q", pkg.ImportPath)
		}
		if err := validateUnitGoListFiles(pkg); err != nil {
			return nil, err
		}
		listings = append(listings, coveragePackageListing{
			importPath:   pkg.ImportPath,
			directory:    pkg.Dir,
			packageName:  pkg.Name,
			goFiles:      len(pkg.GoFiles),
			hasGoFiles:   true,
			testGoFiles:  append([]string(nil), pkg.TestGoFiles...),
			xTestGoFiles: append([]string(nil), pkg.XTestGoFiles...),
		})
	}
}

func validateUnitGoListFiles(pkg coverageGoListPackage) error {
	seen := make(map[string]struct{}, len(pkg.GoFiles)+len(pkg.TestGoFiles)+len(pkg.XTestGoFiles))
	for kind, files := range map[string][]string{
		"GoFiles":      pkg.GoFiles,
		"TestGoFiles":  pkg.TestGoFiles,
		"XTestGoFiles": pkg.XTestGoFiles,
	} {
		for _, file := range files {
			file = strings.TrimSpace(file)
			if file == "" {
				return fmt.Errorf("list build-aware unit packages: go list package %q has an empty %s entry", pkg.ImportPath, kind)
			}
			if _, exists := seen[file]; exists {
				return fmt.Errorf("list build-aware unit packages: go list package %q reports file %q in more than one file set", pkg.ImportPath, file)
			}
			seen[file] = struct{}{}
		}
	}
	return nil
}

func mergeUnitGoListPackages(listings []coveragePackageListing) ([]coveragePackageListing, error) {
	byImportPath := make(map[string]coveragePackageListing, len(listings))
	for _, listing := range listings {
		previous, exists := byImportPath[listing.importPath]
		if !exists {
			byImportPath[listing.importPath] = listing
			continue
		}
		if previous.directory != listing.directory || previous.packageName != listing.packageName || previous.goFiles != listing.goFiles || !slices.Equal(previous.testGoFiles, listing.testGoFiles) || !slices.Equal(previous.xTestGoFiles, listing.xTestGoFiles) {
			return nil, fmt.Errorf("list build-aware unit packages: go list returned contradictory metadata for package %q", listing.importPath)
		}
	}

	merged := make([]coveragePackageListing, 0, len(byImportPath))
	for _, listing := range byImportPath {
		merged = append(merged, listing)
	}
	slices.SortFunc(merged, func(left, right coveragePackageListing) int {
		return strings.Compare(left.importPath, right.importPath)
	})
	return merged, nil
}

func filterUnitTestPackageListings(listings []coveragePackageListing, include func(string) bool) ([]string, error) {
	selected := make([]string, 0, len(listings))
	for _, listing := range listings {
		if include(listing.importPath) && (len(listing.testGoFiles) > 0 || len(listing.xTestGoFiles) > 0) {
			selected = append(selected, listing.importPath)
		}
	}
	if len(selected) == 0 {
		return nil, errors.New("resolve go coverage lane: no packages with build-selected unit tests matched")
	}
	slices.Sort(selected)
	return selected, nil
}

func filterCoveragePackageListings(listings []coveragePackageListing, include func(string) bool, requireNonTestGoFiles bool) ([]string, error) {
	seen := make(map[string]struct{}, len(listings))
	packages := make([]string, 0, len(listings))
	for _, listing := range listings {
		if !include(listing.importPath) {
			continue
		}
		if requireNonTestGoFiles && listing.hasGoFiles && listing.goFiles == 0 {
			continue
		}
		if _, ok := seen[listing.importPath]; ok {
			continue
		}
		seen[listing.importPath] = struct{}{}
		packages = append(packages, listing.importPath)
	}
	slices.Sort(packages)
	if len(packages) == 0 {
		return nil, errors.New("resolve go coverage lane: no packages matched")
	}
	return packages, nil
}

func allListedPackagePaths(listings []coveragePackageListing) []string {
	seen := make(map[string]struct{}, len(listings))
	packages := make([]string, 0, len(listings))
	for _, listing := range listings {
		if _, ok := seen[listing.importPath]; ok {
			continue
		}
		seen[listing.importPath] = struct{}{}
		packages = append(packages, listing.importPath)
	}
	slices.Sort(packages)
	return packages
}
