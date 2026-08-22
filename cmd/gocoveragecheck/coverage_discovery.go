package main

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
)

// coveragePackageListing is the small portion of go list metadata needed to
// derive both coverage package sets. The listing is intentionally scoped to
// one invocation; it is never persisted between checker runs.
type coveragePackageListing struct {
	importPath string
	goFiles    int
	hasGoFiles bool
}

type coveragePackageDiscovery struct {
	allPackages []string
}

func resolveCoverageLaneWithDiscovery(cfg config) (coveragePackageDiscovery, []string, []string, error) {
	coverConfigured := strings.TrimSpace(cfg.coverpkg) != ""
	testConfigured := strings.TrimSpace(cfg.packages) != ""
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
		listings, err = listGoPackageListings(patterns)
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
		testPackages, err = filterCoveragePackageListings(listings, include, false)
		if err != nil {
			return coveragePackageDiscovery{}, nil, nil, err
		}
	}

	discovery := coveragePackageDiscovery{}
	if !testConfigured && (cfg.suite == "" || cfg.suite == unitCoverageSuite) {
		discovery.allPackages = allListedPackagePaths(listings)
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
