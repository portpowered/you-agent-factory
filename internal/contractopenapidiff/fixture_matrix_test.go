package contractopenapidiff_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/contractopenapidiff"
)

type fixtureCategory string

const (
	categoryRoute           fixtureCategory = "route"
	categoryParameter       fixtureCategory = "parameter"
	categoryRequestResponse fixtureCategory = "request_response_schema"
	categoryEnum            fixtureCategory = "enum"
	categoryDocsOnly        fixtureCategory = "docs_only"
	categoryFailClosed      fixtureCategory = "fail_closed"
)

var openAPIDiffFixtureMatrix = []struct {
	name           string
	fixture        string
	category       fixtureCategory
	classification contractopenapidiff.Classification
	failClosed     bool
}{
	{
		name:           "docs-only",
		fixture:        "docs-only",
		category:       categoryDocsOnly,
		classification: contractopenapidiff.ClassificationPatch,
	},
	{
		name:           "add-route",
		fixture:        "add-route",
		category:       categoryRoute,
		classification: contractopenapidiff.ClassificationMinor,
	},
	{
		name:           "add-parameter",
		fixture:        "add-parameter",
		category:       categoryParameter,
		classification: contractopenapidiff.ClassificationMinor,
	},
	{
		name:           "add-schema-property",
		fixture:        "add-schema-property",
		category:       categoryRequestResponse,
		classification: contractopenapidiff.ClassificationMinor,
	},
	{
		name:           "widen-enum",
		fixture:        "widen-enum",
		category:       categoryEnum,
		classification: contractopenapidiff.ClassificationMinor,
	},
	{
		name:           "relax-parameter-required",
		fixture:        "relax-parameter-required",
		category:       categoryParameter,
		classification: contractopenapidiff.ClassificationMinor,
	},
	{
		name:           "relax-schema-required",
		fixture:        "relax-schema-required",
		category:       categoryRequestResponse,
		classification: contractopenapidiff.ClassificationMinor,
	},
	{
		name:           "remove-route",
		fixture:        "remove-route",
		category:       categoryRoute,
		classification: contractopenapidiff.ClassificationMajor,
	},
	{
		name:           "remove-parameter",
		fixture:        "remove-parameter",
		category:       categoryParameter,
		classification: contractopenapidiff.ClassificationMajor,
	},
	{
		name:           "remove-schema-property",
		fixture:        "remove-schema-property",
		category:       categoryRequestResponse,
		classification: contractopenapidiff.ClassificationMajor,
	},
	{
		name:           "narrow-enum",
		fixture:        "narrow-enum",
		category:       categoryEnum,
		classification: contractopenapidiff.ClassificationMajor,
	},
	{
		name:           "narrow-type",
		fixture:        "narrow-type",
		category:       categoryRequestResponse,
		classification: contractopenapidiff.ClassificationMajor,
	},
	{
		name:           "narrow-inline-response-schema",
		fixture:        "narrow-inline-response-schema",
		category:       categoryRequestResponse,
		classification: contractopenapidiff.ClassificationMajor,
	},
	{
		name:           "add-inline-response-schema-property",
		fixture:        "add-inline-response-schema-property",
		category:       categoryRequestResponse,
		classification: contractopenapidiff.ClassificationMinor,
	},
	{
		name:           "major-wins-mixed",
		fixture:        "major-wins-mixed",
		category:       categoryRoute,
		classification: contractopenapidiff.ClassificationMajor,
	},
	{
		name:       "unsupported-operation-id",
		fixture:    "unsupported-operation-id",
		category:   categoryFailClosed,
		failClosed: true,
	},
	{
		name:       "unsupported-nullable-narrow",
		fixture:    "unsupported-nullable-narrow",
		category:   categoryFailClosed,
		failClosed: true,
	},
	{
		name:       "unsupported-additional-properties-narrow",
		fixture:    "unsupported-additional-properties-narrow",
		category:   categoryFailClosed,
		failClosed: true,
	},
	{
		name:       "unsupported-oneof-narrow",
		fixture:    "unsupported-oneof-narrow",
		category:   categoryFailClosed,
		failClosed: true,
	},
	{
		name:       "unsupported-parameter-style",
		fixture:    "unsupported-parameter-style",
		category:   categoryFailClosed,
		failClosed: true,
	},
	{
		name:       "unsupported-response-header-remove",
		fixture:    "unsupported-response-header-remove",
		category:   categoryFailClosed,
		failClosed: true,
	},
	{
		name:       "unsupported-security-scheme",
		fixture:    "unsupported-security-scheme",
		category:   categoryFailClosed,
		failClosed: true,
	},
	{
		name:           "remove-path-parameter",
		fixture:        "remove-path-parameter",
		category:       categoryParameter,
		classification: contractopenapidiff.ClassificationMajor,
	},
	{
		name:       "unsupported-operation-security",
		fixture:    "unsupported-operation-security",
		category:   categoryFailClosed,
		failClosed: true,
	},
	{
		name:       "unsupported-path-servers",
		fixture:    "unsupported-path-servers",
		category:   categoryFailClosed,
		failClosed: true,
	},
	{
		name:       "unsupported-media-type-encoding",
		fixture:    "unsupported-media-type-encoding",
		category:   categoryFailClosed,
		failClosed: true,
	},
	{
		name:       "unsupported-response-links",
		fixture:    "unsupported-response-links",
		category:   categoryFailClosed,
		failClosed: true,
	},
	{
		name:       "unsupported-operation-callbacks",
		fixture:    "unsupported-operation-callbacks",
		category:   categoryFailClosed,
		failClosed: true,
	},
	{
		name:       "unsupported-operation-servers",
		fixture:    "unsupported-operation-servers",
		category:   categoryFailClosed,
		failClosed: true,
	},
	{
		name:       "unsupported-server-variables",
		fixture:    "unsupported-server-variables",
		category:   categoryFailClosed,
		failClosed: true,
	},
	{
		name:       "unsupported-schema-extension",
		fixture:    "unsupported-schema-extension",
		category:   categoryFailClosed,
		failClosed: true,
	},
	{
		name:       "unsupported-operation-extension",
		fixture:    "unsupported-operation-extension",
		category:   categoryFailClosed,
		failClosed: true,
	},
	{
		name:       "unsupported-parameter-extension",
		fixture:    "unsupported-parameter-extension",
		category:   categoryFailClosed,
		failClosed: true,
	},
	{
		name:       "unsupported-component-parameter-remove",
		fixture:    "unsupported-component-parameter-remove",
		category:   categoryFailClosed,
		failClosed: true,
	},
}

func TestCompareYAML_FixtureMatrix_ClassifiesRepresentativeCases(t *testing.T) {
	t.Parallel()

	for _, tc := range openAPIDiffFixtureMatrix {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			before := readFixture(t, tc.fixture, "before.yaml")
			after := readFixture(t, tc.fixture, "after.yaml")

			result, err := contractopenapidiff.CompareYAML(before, after)
			if tc.failClosed {
				if err == nil {
					t.Fatalf("CompareYAML() = %#v, want unsupported diff error", result)
				}
				if !contractopenapidiff.IsUnsupportedDiff(err) {
					t.Fatalf("CompareYAML() error = %v, want unsupported diff refusal", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("CompareYAML() error = %v", err)
			}
			if result.Classification != tc.classification {
				t.Fatalf("Classification = %q, want %q", result.Classification, tc.classification)
			}
			if len(result.Changes) == 0 {
				t.Fatalf("Changes is empty, want at least one classified change")
			}
			for _, change := range result.Changes {
				if change.Code == "" {
					t.Fatalf("change missing code: %#v", change)
				}
				if change.Path == "" {
					t.Fatalf("change missing path: %#v", change)
				}
			}
		})
	}
}
