package main

import (
	"fmt"
	"io"
	"slices"
	"strings"
)

func reconcileInventory(sites []spawnSite, inventory inventoryDocument) error {
	findings := []string{}
	byID := make(map[string]inventorySpawnSite, len(inventory.OSSpawnSites))
	for _, site := range inventory.OSSpawnSites {
		byID[site.SiteID] = site
		observed, exists := findObservedSite(sites, site.SiteID)
		if !exists {
			findings = append(findings, fmt.Sprintf("inventory site %s is not present in the AST census", site.SiteID))
			continue
		}
		findings = append(findings, compareSiteMetadata(observed, site)...)
	}
	for _, site := range sites {
		if _, exists := byID[site.SiteID]; exists {
			continue
		}
		findings = append(findings, fmt.Sprintf("AST site %s at %s:%d has no inventory verdict; update the baseline together with an inventory row naming an allowed OS property", site.SiteID, site.SourcePath, site.SourceLine))
	}
	if len(findings) == 0 {
		return nil
	}
	slices.Sort(findings)
	return fmt.Errorf("%s", strings.Join(findings, "\n"))
}

func findObservedSite(sites []spawnSite, siteID string) (spawnSite, bool) {
	for _, site := range sites {
		if site.SiteID == siteID {
			return site, true
		}
	}
	return spawnSite{}, false
}

func compareSiteMetadata(observed spawnSite, recorded inventorySpawnSite) []string {
	var findings []string
	if observed.PackagePath != recorded.PackagePath {
		findings = append(findings, fmt.Sprintf("inventory site %s packagePath=%q does not match AST packagePath=%q", recorded.SiteID, recorded.PackagePath, observed.PackagePath))
	}
	if observed.SourcePath != recorded.SourcePath {
		findings = append(findings, fmt.Sprintf("inventory site %s sourcePath=%q does not match AST sourcePath=%q", recorded.SiteID, recorded.SourcePath, observed.SourcePath))
	}
	if observed.SourceLine != recorded.SourceLine {
		findings = append(findings, fmt.Sprintf("inventory site %s sourceLine=%d does not match AST sourceLine=%d", recorded.SiteID, recorded.SourceLine, observed.SourceLine))
	}
	if observed.EnclosingIdentity != recorded.EnclosingIdentity {
		findings = append(findings, fmt.Sprintf("inventory site %s enclosingIdentity=%q does not match AST enclosingIdentity=%q", recorded.SiteID, recorded.EnclosingIdentity, observed.EnclosingIdentity))
	}
	if observed.Occurrence != recorded.Occurrence {
		findings = append(findings, fmt.Sprintf("inventory site %s occurrence=%d does not match AST occurrence=%d", recorded.SiteID, recorded.Occurrence, observed.Occurrence))
	}
	return findings
}

func evaluateBaseline(sites []spawnSite, inventory inventoryDocument, baseline baselineDocument) []violation {
	observed := packageCounts(sites)
	sitesByPackage := sitesByPackage(sites)
	inventoryByID := inventorySitesByID(inventory)
	baselineByPackage := map[string]baselinePackageRow{}
	recorded := map[string]int{}
	for _, row := range baseline.Packages {
		baselineByPackage[row.PackagePath] = row
		recorded[row.PackagePath] = row.Count
	}
	packages := unionPackagePaths(observed, recorded)
	var violations []violation
	for _, packagePath := range packages {
		actual := observed[packagePath]
		ceiling := recorded[packagePath]
		if actual <= ceiling {
			continue
		}
		baselineIDs := baselineSiteIDs(baselineByPackage[packagePath])
		unadmitted := unadmittedNewSiteIDs(sitesByPackage[packagePath], baselineIDs, inventoryByID)
		if len(unadmitted) == 0 {
			continue
		}
		violations = append(violations, violation{message: fmt.Sprintf("package %s observed %d static OS-spawn site(s), baseline %d; %d new site(s) require paired INTENTIONAL-OS inventory admission naming an allowed OS property (unadmitted=%d; sites=%s)", packagePath, actual, ceiling, actual-ceiling, len(unadmitted), strings.Join(unadmitted, ", "))})
	}
	return violations
}

func sitesByPackage(sites []spawnSite) map[string][]spawnSite {
	result := map[string][]spawnSite{}
	for _, site := range sites {
		result[site.PackagePath] = append(result[site.PackagePath], site)
	}
	for packagePath := range result {
		slices.SortFunc(result[packagePath], func(left, right spawnSite) int {
			return strings.Compare(left.SiteID, right.SiteID)
		})
	}
	return result
}

func inventorySitesByID(inventory inventoryDocument) map[string]inventorySpawnSite {
	result := make(map[string]inventorySpawnSite, len(inventory.OSSpawnSites))
	for _, site := range inventory.OSSpawnSites {
		result[site.SiteID] = site
	}
	return result
}

func baselineSiteIDs(row baselinePackageRow) map[string]struct{} {
	result := make(map[string]struct{}, len(row.SiteIDs))
	for _, siteID := range row.SiteIDs {
		result[siteID] = struct{}{}
	}
	return result
}

func unadmittedNewSiteIDs(sites []spawnSite, baselineIDs map[string]struct{}, inventoryByID map[string]inventorySpawnSite) []string {
	var result []string
	for _, site := range sites {
		if _, existed := baselineIDs[site.SiteID]; existed {
			continue
		}
		record, exists := inventoryByID[site.SiteID]
		if !exists || record.Verdict != intentionalVerdict {
			result = append(result, site.SiteID)
		}
	}
	return result
}

func unionPackagePaths(left, right map[string]int) []string {
	seen := make(map[string]struct{}, len(left)+len(right))
	for packagePath := range left {
		seen[packagePath] = struct{}{}
	}
	for packagePath := range right {
		seen[packagePath] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for packagePath := range seen {
		result = append(result, packagePath)
	}
	slices.Sort(result)
	return result
}

func reportViolations(stderr io.Writer, violations []violation) error {
	sortViolations(violations)
	for _, violation := range violations {
		fmt.Fprintf(stderr, "[agent-factory:functional-os-boundary] %s\n", violation.message)
	}
	fmt.Fprintf(stderr, "LINT_VIOLATION_COUNT: %d\n", len(violations))
	return fmt.Errorf("[agent-factory:functional-os-boundary] found %d baseline violation(s); update the baseline together with an inventory row naming the OS property", len(violations))
}

func writeSuccess(stdout io.Writer, sites []spawnSite, baseline baselineDocument, inventory inventoryDocument) {
	counts := packageCounts(sites)
	recordedByPackage := make(map[string]int, len(baseline.Packages))
	recorded := 0
	for _, row := range baseline.Packages {
		recordedByPackage[row.PackagePath] = row.Count
		recorded += row.Count
	}
	decreased := 0
	for _, packagePath := range unionPackagePaths(counts, recordedByPackage) {
		if ceiling, actual := recordedByPackage[packagePath], counts[packagePath]; ceiling > actual {
			decreased += ceiling - actual
		}
	}
	intentional := 0
	accidental := 0
	for _, site := range inventory.OSSpawnSites {
		if site.Verdict == intentionalVerdict {
			intentional++
		} else if site.Verdict == accidentalVerdict {
			accidental++
		}
	}
	fmt.Fprintf(stdout, "[agent-factory:functional-os-boundary] static OS-spawn baseline holds: observed=%d baseline=%d packages=%d intentional=%d accidental=%d decreased=%d\n", len(sites), recorded, len(counts), intentional, accidental, decreased)
	fmt.Fprintf(stdout, "[agent-factory:functional-os-boundary] reconciled %d inventory OS-spawn records\n", len(inventory.OSSpawnSites))
}
