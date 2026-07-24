package functionaltestviz

import (
	"path"
	"strings"

	"github.com/portpowered/infinite-you/internal/functionaltestmetadata"
)

// Known deprecated / deletion-only functional package roots. Paths under these
// owners are labeled deprecated in the catalog until ownership reaches zero.
var deprecatedFunctionalPackageRoots = map[string]struct{}{
	"runtime_api": {},
}

const (
	// LaneShort is the default functional lane (not gated by functionallong).
	LaneShort = "short"
	// LaneLongOnly is the long-only functional lane gated by functionallong.
	LaneLongOnly = "long-only"
)

// ClassifiedRecord is one inventoried functional Test* with catalog labels.
type ClassifiedRecord struct {
	Record       functionaltestmetadata.Record
	Domain       string
	Package      string
	Lane         string
	GoldenBacked bool
	Undocumented bool
	Deprecated   bool
	// Provenance is attached by AttachGoldenProvenance for golden-backed tests.
	Provenance GoldenProvenance
}

// CatalogInputs is the assembled catalog input set: classified inventory plus
// decoded coverage-summary JSON.
type CatalogInputs struct {
	Records  []ClassifiedRecord
	Coverage CoverageSummary
}

// AssembleCatalogInputs classifies inventoried metadata records and loads the
// required coverage-summary JSON path. Coverage is consumed only by decoding
// that JSON; no coverage-profile parser is used.
func AssembleCatalogInputs(records []functionaltestmetadata.Record, coverageSummaryPath string) (CatalogInputs, error) {
	coverage, err := LoadCoverageSummary(coverageSummaryPath)
	if err != nil {
		return CatalogInputs{}, err
	}
	return CatalogInputs{
		Records:  ClassifyRecords(records),
		Coverage: coverage,
	}, nil
}

// ClassifyRecords derives domain ownership and catalog labels for each record.
// Ordering of the returned slice matches the input order.
func ClassifyRecords(records []functionaltestmetadata.Record) []ClassifiedRecord {
	out := make([]ClassifiedRecord, 0, len(records))
	for _, record := range records {
		out = append(out, ClassifyRecord(record))
	}
	return out
}

// ClassifyRecord derives domain ownership and catalog labels for one record.
func ClassifyRecord(record functionaltestmetadata.Record) ClassifiedRecord {
	domain := DomainFromFile(record.File)
	return ClassifiedRecord{
		Record:       record,
		Domain:       domain,
		Package:      record.Package,
		Lane:         LaneFromBuildTags(record.BuildTags),
		GoldenBacked: strings.TrimSpace(record.Golden) != "",
		Undocumented: record.Undocumented,
		Deprecated:   IsDeprecatedDomain(domain),
	}
}

// DomainFromFile returns the functional domain noun from a repository-relative
// or functional-root-relative source path (tests/functional/<domain>/...).
func DomainFromFile(file string) string {
	normalized := path.Clean("/" + strings.ReplaceAll(file, "\\", "/"))
	normalized = strings.TrimPrefix(normalized, "/")
	parts := strings.Split(normalized, "/")
	if len(parts) == 0 || parts[0] == "" || parts[0] == "." {
		return ""
	}
	if parts[0] == "tests" && len(parts) >= 3 && parts[1] == "functional" {
		return parts[2]
	}
	return parts[0]
}

// LaneFromBuildTags reports short versus long-only from file-level build tags.
// A record is long-only when any constraint expression includes functionallong
// without a leading negation of that tag.
func LaneFromBuildTags(buildTags []string) string {
	for _, tag := range buildTags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" {
			continue
		}
		if strings.Contains(trimmed, "functionallong") && !strings.Contains(trimmed, "!functionallong") {
			return LaneLongOnly
		}
	}
	return LaneShort
}

// IsDeprecatedDomain reports whether a functional domain root is a known
// deletion-only / deprecated package path.
func IsDeprecatedDomain(domain string) bool {
	_, ok := deprecatedFunctionalPackageRoots[domain]
	return ok
}
