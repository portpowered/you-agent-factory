package functionaltestviz

import (
	"fmt"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/internal/functionaltestmetadata"
)

// DetailCatalog groups classified records for the detailed per-test Markdown
// section. Customer scenarios are bucketed in browse order; harness records
// are kept separate so they do not read as customer scenarios.
type DetailCatalog struct {
	CustomerBuckets []DetailBucket
	OtherCustomer   []ClassifiedRecord
	Harness         []ClassifiedRecord
}

// DetailBucket is one browse-order domain bucket with stable-sorted customer rows.
type DetailBucket struct {
	Domain  string
	Records []ClassifiedRecord
}

// BuildDetailCatalog partitions classified records into browse-order customer
// buckets, out-of-order customer domains, and harness verification rows.
func BuildDetailCatalog(records []ClassifiedRecord) DetailCatalog {
	bucketRecords := make(map[string][]ClassifiedRecord, len(DomainBrowseOrder))
	var otherCustomer []ClassifiedRecord
	var harness []ClassifiedRecord

	for _, record := range records {
		if record.Record.Classification == functionaltestmetadata.ClassificationHarness {
			harness = append(harness, record)
			continue
		}
		if record.Record.Classification != functionaltestmetadata.ClassificationCustomer {
			continue
		}
		bucket, ok := SummaryBucketForDomain(record.Domain)
		if !ok {
			otherCustomer = append(otherCustomer, record)
			continue
		}
		bucketRecords[bucket] = append(bucketRecords[bucket], record)
	}

	customerBuckets := make([]DetailBucket, 0, len(DomainBrowseOrder))
	for _, bucket := range DomainBrowseOrder {
		rows := bucketRecords[bucket]
		sortDetailRecords(rows)
		customerBuckets = append(customerBuckets, DetailBucket{
			Domain:  bucket,
			Records: rows,
		})
	}

	sortDetailRecords(otherCustomer)
	sortDetailRecords(harness)

	return DetailCatalog{
		CustomerBuckets: customerBuckets,
		OtherCustomer:   otherCustomer,
		Harness:         harness,
	}
}

// RenderDetailCatalogMarkdown renders the detailed per-test catalog section.
func RenderDetailCatalogMarkdown(catalog DetailCatalog) string {
	var b strings.Builder
	b.WriteString("## Test catalog\n")

	for _, bucket := range catalog.CustomerBuckets {
		b.WriteString("\n")
		b.WriteString(renderDetailBucket(bucket))
	}

	if len(catalog.OtherCustomer) > 0 {
		b.WriteString("\n\n### Other domains\n\n")
		for _, record := range catalog.OtherCustomer {
			b.WriteString(renderDetailRecord(record))
			b.WriteString("\n")
		}
	}

	if len(catalog.Harness) > 0 {
		b.WriteString("\n\n## Harness verification\n\n")
		for _, record := range catalog.Harness {
			b.WriteString(renderDetailRecord(record))
			b.WriteString("\n")
		}
	}

	return b.String()
}

func renderDetailBucket(bucket DetailBucket) string {
	var b strings.Builder
	b.WriteString("### ")
	b.WriteString(bucket.Domain)
	b.WriteString("\n")
	if len(bucket.Records) == 0 {
		b.WriteString("\n- _No customer scenarios._\n")
		return b.String()
	}
	b.WriteString("\n")
	for _, record := range bucket.Records {
		b.WriteString(renderDetailRecord(record))
		b.WriteString("\n")
	}
	return b.String()
}

func renderDetailRecord(record ClassifiedRecord) string {
	var b strings.Builder
	b.WriteString("- **")
	b.WriteString(record.Record.Name)
	b.WriteString("** — ")
	b.WriteString(renderDetailDescription(record))
	b.WriteString("\n")
	b.WriteString("  - Source: `")
	b.WriteString(formatSourceLocation(record.Record.File, record.Record.Line))
	b.WriteString("`\n")
	b.WriteString("  - Domain: `")
	b.WriteString(record.Domain)
	b.WriteString("`\n")
	b.WriteString("  - Package: `")
	b.WriteString(record.Package)
	b.WriteString("`\n")
	labels := detailRecordLabels(record)
	if len(labels) > 0 {
		b.WriteString("  - Labels: ")
		b.WriteString(strings.Join(labels, ", "))
		b.WriteString("\n")
	}
	if record.GoldenBacked {
		b.WriteString(renderGoldenProvenance(record.Provenance))
	}
	return b.String()
}

func renderGoldenProvenance(provenance GoldenProvenance) string {
	if !provenance.Present() {
		return "  - Golden provenance: ERROR: missing attached provenance\n"
	}
	var b strings.Builder
	b.WriteString("  - Golden provenance:\n")
	b.WriteString("    - Provider: `")
	b.WriteString(provenance.Provider)
	b.WriteString("`\n")
	b.WriteString("    - Case: `")
	b.WriteString(provenance.Case)
	b.WriteString("`\n")
	b.WriteString("    - Fidelity class: `")
	b.WriteString(provenance.FidelityClass)
	b.WriteString("`\n")
	b.WriteString("    - Golden id: `")
	b.WriteString(provenance.ID)
	b.WriteString("`\n")
	b.WriteString("    - Manifest: `")
	b.WriteString(provenance.ManifestPath)
	b.WriteString("`\n")
	return b.String()
}

func renderDetailDescription(record ClassifiedRecord) string {
	if record.Undocumented {
		return "(undocumented)"
	}
	if strings.TrimSpace(record.Record.Description) == "" {
		return "(undocumented)"
	}
	return record.Record.Description
}

func detailRecordLabels(record ClassifiedRecord) []string {
	labels := make([]string, 0, 4)
	if record.Lane == LaneLongOnly {
		labels = append(labels, "long-only")
	} else {
		labels = append(labels, "short")
	}
	if record.GoldenBacked {
		labels = append(labels, "golden-backed")
	}
	if record.Deprecated {
		labels = append(labels, "deprecated")
	}
	if record.Undocumented {
		labels = append(labels, "undocumented")
	}
	return labels
}

func formatSourceLocation(file string, line int) string {
	if line <= 0 {
		return file
	}
	return fmt.Sprintf("%s:%d", file, line)
}

func sortDetailRecords(records []ClassifiedRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		left := records[i]
		right := records[j]
		if cmp := strings.Compare(left.Domain, right.Domain); cmp != 0 {
			return cmp < 0
		}
		if cmp := strings.Compare(left.Record.File, right.Record.File); cmp != 0 {
			return cmp < 0
		}
		return strings.Compare(left.Record.Name, right.Record.Name) < 0
	})
}
