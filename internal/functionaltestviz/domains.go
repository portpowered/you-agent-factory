package functionaltestviz

import (
	"path"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/internal/functionaltestmetadata"
)

// Browse-order domain summary bucket display names from
// docs/temp/functional-tests-expansion/plan.md.
const (
	DomainBucketTransport                      = "transport"
	DomainBucketWorkers                        = "workers"
	DomainBucketOrchestration                  = "orchestration"
	DomainBucketWorkstations                   = "workstations"
	DomainBucketWork                           = "work"
	DomainBucketSessions                       = "sessions"
	DomainBucketFactory                        = "factory"
	DomainBucketProviderSessions               = "provider_sessions"
	DomainBucketEvents                         = "events"
	DomainBucketModels                         = "models"
	DomainBucketGuardsResources                = "guards / resources"
	DomainBucketObservabilityProductResilience = "observability / product / resilience"
)

// DomainBrowseOrder is the fixed prioritized summary order for the Markdown
// catalog opening. Domains with zero customer scenarios still appear.
var DomainBrowseOrder = []string{
	DomainBucketTransport,
	DomainBucketWorkers,
	DomainBucketOrchestration,
	DomainBucketWorkstations,
	DomainBucketWork,
	DomainBucketSessions,
	DomainBucketFactory,
	DomainBucketProviderSessions,
	DomainBucketEvents,
	DomainBucketModels,
	DomainBucketGuardsResources,
	DomainBucketObservabilityProductResilience,
}

// leafDomainToSummaryBucket maps functional path domain nouns onto browse-order
// summary buckets. Coalesced domains share one bucket display name.
var leafDomainToSummaryBucket = map[string]string{
	"transport":         DomainBucketTransport,
	"workers":           DomainBucketWorkers,
	"orchestration":     DomainBucketOrchestration,
	"workstations":      DomainBucketWorkstations,
	"work":              DomainBucketWork,
	"sessions":          DomainBucketSessions,
	"factory":           DomainBucketFactory,
	"provider_sessions": DomainBucketProviderSessions,
	"events":            DomainBucketEvents,
	"models":            DomainBucketModels,
	"guards":            DomainBucketGuardsResources,
	"resources":         DomainBucketGuardsResources,
	"observability":     DomainBucketObservabilityProductResilience,
	"product":           DomainBucketObservabilityProductResilience,
	"resilience":        DomainBucketObservabilityProductResilience,
}

// NamedCount is a stable-sorted name/count pair for package or subsection tallies.
type NamedCount struct {
	Name  string
	Count int
}

// DomainSummary is one prioritized domain bucket in the catalog opening.
type DomainSummary struct {
	Domain            string
	CustomerScenarios int
	Packages          []NamedCount
	Subsections       []NamedCount
}

// SummaryBucketForDomain returns the browse-order summary bucket for a leaf
// domain noun. ok is false when the domain is outside the prioritized catalog
// opening (for example deletion-only catch-alls).
func SummaryBucketForDomain(domain string) (string, bool) {
	bucket, ok := leafDomainToSummaryBucket[strings.TrimSpace(domain)]
	return bucket, ok
}

// SubsectionFromFile returns the first path segment under the leaf domain, or
// empty when the file sits directly under the domain root.
func SubsectionFromFile(file string) string {
	normalized := path.Clean("/" + strings.ReplaceAll(file, "\\", "/"))
	normalized = strings.TrimPrefix(normalized, "/")
	parts := strings.Split(normalized, "/")
	if len(parts) == 0 || parts[0] == "" || parts[0] == "." {
		return ""
	}
	start := 0
	if parts[0] == "tests" && len(parts) >= 3 && parts[1] == "functional" {
		start = 2
	}
	if len(parts) <= start+1 {
		return ""
	}
	candidate := parts[start+1]
	if candidate == "" || strings.HasSuffix(candidate, ".go") {
		return ""
	}
	return candidate
}

// BuildDomainSummaries aggregates customer scenario counts (plus package and
// subsection tallies) into the fixed browse-order buckets. Harness/internal
// records do not increment customer totals.
func BuildDomainSummaries(records []ClassifiedRecord) []DomainSummary {
	type mutableSummary struct {
		customerScenarios int
		packages          map[string]int
		subsections       map[string]int
	}
	byBucket := make(map[string]*mutableSummary, len(DomainBrowseOrder))
	for _, bucket := range DomainBrowseOrder {
		byBucket[bucket] = &mutableSummary{
			packages:    map[string]int{},
			subsections: map[string]int{},
		}
	}

	for _, record := range records {
		if record.Record.Classification != functionaltestmetadata.ClassificationCustomer {
			continue
		}
		bucket, ok := SummaryBucketForDomain(record.Domain)
		if !ok {
			continue
		}
		summary := byBucket[bucket]
		summary.customerScenarios++
		if pkg := strings.TrimSpace(record.Package); pkg != "" {
			summary.packages[pkg]++
		}
		if subsection := SubsectionFromFile(record.Record.File); subsection != "" {
			summary.subsections[subsection]++
		}
	}

	out := make([]DomainSummary, 0, len(DomainBrowseOrder))
	for _, bucket := range DomainBrowseOrder {
		mutable := byBucket[bucket]
		out = append(out, DomainSummary{
			Domain:            bucket,
			CustomerScenarios: mutable.customerScenarios,
			Packages:          sortedNamedCounts(mutable.packages),
			Subsections:       sortedNamedCounts(mutable.subsections),
		})
	}
	return out
}

func sortedNamedCounts(counts map[string]int) []NamedCount {
	if len(counts) == 0 {
		return nil
	}
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]NamedCount, 0, len(names))
	for _, name := range names {
		out = append(out, NamedCount{Name: name, Count: counts[name]})
	}
	return out
}
