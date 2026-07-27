package retiredsurfaceguard

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// ScanAllViolations runs the retired-surface guard suite against production snapshots.
func ScanAllViolations(repoRoot string, cli CLIInventory, docs DocsRegistry, mapper NamedFactoryMapper) ([]Violation, error) {
	var violations []Violation
	violations = append(violations, ScanCLIReintroductionViolations(cli)...)
	violations = append(violations, ScanDocsReintroductionViolations(docs)...)
	violations = append(violations, ScanEncodedPathReintroductionViolations(
		mapper,
		filepath.Join(repoRoot, ".retired-surface-guard"),
		SettledScopedNamedFactoryPaths(),
	)...)

	sourceViolations, err := ScanEncodedPathProductionSourceViolations(repoRoot)
	if err != nil {
		return nil, err
	}
	violations = append(violations, sourceViolations...)

	manifestAuthorityViolations, err := ScanCLIManifestAuthoritySourceViolations(repoRoot)
	if err != nil {
		return nil, err
	}
	violations = append(violations, manifestAuthorityViolations...)

	sort.SliceStable(violations, func(i, j int) bool {
		left := violations[i]
		right := violations[j]
		if left.Family != right.Family {
			return left.Family < right.Family
		}
		if left.Surface != right.Surface {
			return left.Surface < right.Surface
		}
		return left.Detail < right.Detail
	})
	return violations, nil
}

// FormatViolation returns one stderr line for a retired-surface guard finding.
func FormatViolation(violation Violation) string {
	return fmt.Sprintf(
		"retired-surface %s guard: %s (%s)",
		violation.Family,
		violation.Surface,
		strings.TrimSpace(violation.Detail),
	)
}
