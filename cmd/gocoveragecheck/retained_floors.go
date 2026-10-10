package main

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// These obligations are independent of the editable measurement baseline.
// Updating samples or staging a generic floor hold cannot lower them.
var retainedFunctionalFloors = map[string]coverageFloor{
	modulePath + "/pkg/wire":             8006,
	modulePath + "/pkg/platform/metrics": 7466,
	modulePath + "/pkg/services/factory_definitions/transports/mapping/validationentry":        6567,
	modulePath + "/pkg/services/work/internal/services/state_access/wire":                      7647,
	modulePath + "/pkg/services/automations/internal/services/reconciliation/internal/service": 3571,
	modulePath + "/pkg/transports/http":                                                        6559,
	modulePath + "/pkg/transports/cli/clihttp":                                                 6707,
}

var dispositionRangePattern = regexp.MustCompile(`^[1-9][0-9]*\.[1-9][0-9]*,[1-9][0-9]*\.[1-9][0-9]*$`)
var dispositionDigestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var dispositionReviewPathPattern = regexp.MustCompile(`^/portpowered/you-agent-factory/pull/[1-9][0-9]*$`)

func checkRetainedFunctionalFloors(manifest coverageManifest, totals map[string]packageCoverageTotals, blocks map[string]coverageBlock, repoRoot string) []string {
	if manifest.Lane != functionalCoverageSuite {
		return nil
	}
	entries := make(map[string]coverageManifestEntry, len(manifest.Packages))
	for _, entry := range manifest.Packages {
		entries[entry.Package] = entry
	}
	packages := make([]string, 0, len(retainedFunctionalFloors))
	for importPath := range retainedFunctionalFloors {
		packages = append(packages, importPath)
	}
	slices.Sort(packages)
	var failures []string
	for _, importPath := range packages {
		entry := entries[importPath]
		actual := totals[importPath]
		minimum := retainedFunctionalFloors[importPath]
		if err := checkRetainedFunctionalPackage(entry, minimum, actual, blocks, repoRoot); err != nil {
			failures = append(failures, fmt.Sprintf("retained functional coverage regression: package=%s retained-minimum=%s%% manifest-minimum=%s covered=%d/%d statements; %v; restore functional coverage or supply every exact source-bound dead/unreachable block for independent review", importPath, minimum.String(), string(entry.Minimum), actual.coveredStatements, actual.totalStatements, err))
		}
	}
	return failures
}

func checkRetainedFunctionalPackage(entry coverageManifestEntry, minimum coverageFloor, actual packageCoverageTotals, blocks map[string]coverageBlock, repoRoot string) error {
	floor, err := parseCoverageFloor(entry.Minimum)
	if err != nil || entry.Exception != nil {
		return fmt.Errorf("missing numeric retained policy; defaults, measurement exceptions and scope exclusions cannot satisfy admission")
	}
	if actual.totalStatements <= 0 || actual.coveredStatements < 0 || actual.coveredStatements > actual.totalStatements {
		return fmt.Errorf("missing or invalid measurement; zero statements and selected-package omissions cannot satisfy admission")
	}
	// Even when restoration passes, stale or foreign exemption records fail closed.
	if len(entry.DeadCodeDispositions) > 0 {
		if err := checkDeadCodeDispositions(entry, blocks, repoRoot); err != nil {
			return err
		}
	}
	if floor >= minimum && coveragePassesFloor(floor, actual) {
		return nil
	}
	if floor > minimum {
		return fmt.Errorf("stronger existing floor %s%% remains required; holds, epsilon and dispositions cannot lower it", floor.String())
	}
	if len(entry.DeadCodeDispositions) == 0 {
		return fmt.Errorf("retained threshold unmet; holds and epsilon do not apply; %s", formatUncoveredCoverageBlocks(blocks, entry.Package))
	}
	return nil
}

func validateDeadCodeDisposition(record coverageManifestDeadCodeDisposition, importPath string) error {
	file := record.File
	if err := validateDispositionFile(file, importPath); err != nil {
		return err
	}
	if !dispositionRangePattern.MatchString(record.Range) || record.Statements <= 0 || !dispositionDigestPattern.MatchString(record.SourceSHA256) {
		return fmt.Errorf("disposition %s:%s requires exact positive range, statements and sourceSHA256", file, record.Range)
	}
	if record.Classification != "dead" && record.Classification != "unreachable" {
		return fmt.Errorf("disposition %s:%s classification must be dead or unreachable", file, record.Range)
	}
	rationale := strings.TrimSpace(record.Justification)
	if rationale == "" || genericDispositionJustification(rationale) {
		return fmt.Errorf("disposition %s:%s requires block-specific canonical reachability justification, not generic hold/scope text", file, record.Range)
	}
	if !validDispositionReviewReference(record.ReviewReference) {
		return fmt.Errorf("disposition %s:%s requires an independent PR review reference", file, record.Range)
	}
	return nil
}

func validateDispositionFile(file, importPath string) error {
	if file == "" || path.IsAbs(file) || path.Clean(file) != file || strings.ContainsAny(file, "\\:") || strings.HasPrefix(file, "../") || !strings.HasSuffix(file, ".go") || path.Dir(modulePath+"/"+file) != importPath {
		return fmt.Errorf("disposition file %q must be a normalized repository-relative Go file owned by package %s", file, importPath)
	}
	return nil
}

func validDispositionReviewReference(value string) bool {
	reference, err := url.Parse(value)
	if err != nil || reference.Scheme != "https" || reference.Host != "github.com" || reference.User != nil {
		return false
	}
	return dispositionReviewPathPattern.MatchString(reference.Path) && reference.Fragment != ""
}

func genericDispositionJustification(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, generic := range []string{"hold", "scope exclusion", "out of scope", "generated-only", "fixture limitation", "fixture unavailable"} {
		if value == generic || (generic != "hold" && strings.Contains(value, generic)) {
			return true
		}
	}
	return false
}

func retainedDispositionEligibilityDiagnostics(manifest coverageManifest, failures []string) []string {
	if manifest.Lane != functionalCoverageSuite {
		return nil
	}
	var diagnostics []string
	for _, entry := range manifest.Packages {
		if _, protected := retainedFunctionalFloors[entry.Package]; !protected || len(entry.DeadCodeDispositions) == 0 || packageCoverageFinding(failures, entry.Package) {
			continue
		}
		diagnostics = append(diagnostics, fmt.Sprintf("retained functional disposition eligibility: package=%s source-bound-blocks=%d raw coverage unchanged; independent review of reachability and review references required", entry.Package, len(entry.DeadCodeDispositions)))
	}
	return diagnostics
}

// A valid complete disposition set is an alternative to a retained floor, not
// a rewritten denominator. Higher existing numeric obligations still apply.
func filterDispositionFloorFindings(manifest coverageManifest, findings, retainedFailures []string) []string {
	if manifest.Lane != functionalCoverageSuite {
		return findings
	}
	var filtered []string
	for _, finding := range findings {
		eligible := false
		for _, entry := range manifest.Packages {
			retained, protected := retainedFunctionalFloors[entry.Package]
			floor, _ := parseCoverageFloor(entry.Minimum)
			if protected && floor <= retained && len(entry.DeadCodeDispositions) > 0 && !packageCoverageFinding(retainedFailures, entry.Package) && packageCoverageFinding([]string{finding}, entry.Package) {
				eligible = true
				break
			}
		}
		if !eligible {
			filtered = append(filtered, finding)
		}
	}
	return filtered
}

func checkDeadCodeDispositions(entry coverageManifestEntry, blocks map[string]coverageBlock, repoRoot string) error {
	uncovered := uncoveredCoverageBlocks(blocks, entry.Package)
	if len(uncovered) == 0 {
		return fmt.Errorf("disposition set has no matching uncovered profile blocks")
	}
	remaining := make(map[string]coverageBlock, len(uncovered))
	for _, block := range uncovered {
		remaining[block.canonicalPath+":"+block.rangeSpec] = block
	}
	for _, record := range entry.DeadCodeDispositions {
		if err := validateDeadCodeDisposition(record, entry.Package); err != nil {
			return err
		}
		key := modulePath + "/" + record.File + ":" + record.Range
		block, found := remaining[key]
		if !found || block.statementCount != record.Statements {
			return fmt.Errorf("disposition %s:%s is duplicate, foreign, covered or mismatches profile range/statements", record.File, record.Range)
		}
		if err := checkDispositionSource(record, repoRoot); err != nil {
			return err
		}
		delete(remaining, key)
	}
	for _, block := range uncovered {
		if _, missing := remaining[block.canonicalPath+":"+block.rangeSpec]; missing {
			return fmt.Errorf("missing disposition for %s:%s (%d statements)", block.canonicalPath, block.rangeSpec, block.statementCount)
		}
	}
	return nil
}

func checkDispositionSource(record coverageManifestDeadCodeDisposition, repoRoot string) error {
	filename := filepath.Join(repoRoot, filepath.FromSlash(record.File))
	resolved, err := filepath.EvalSymlinks(filename)
	if err != nil {
		return fmt.Errorf("read disposition source %s: %w", record.File, err)
	}
	root, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return fmt.Errorf("resolve disposition source root: %w", err)
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("disposition source %s escapes repository root", record.File)
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return fmt.Errorf("read disposition source %s: %w", record.File, err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != record.SourceSHA256 {
		return fmt.Errorf("stale disposition sourceSHA256 for %s:%s", record.File, record.Range)
	}
	return nil
}
