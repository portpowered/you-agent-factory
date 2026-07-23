// Package configcontractsmoke verifies the repository's published configuration
// contract family. It is build tooling and must not be imported by runtime packages.
package configcontractsmoke

import (
	"fmt"
	"reflect"
	"sort"

	workers "github.com/portpowered/infinite-you/pkg/services/workers"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
)

// FamilyID is the stable identifier for one separately loadable configuration root.
type FamilyID string

const (
	FamilyGlobal     FamilyID = "global"
	FamilyMockWorker FamilyID = "mock-worker"
	FamilyFactory    FamilyID = "factory"
)

// ParseFunc invokes the production parser for one configuration root.
type ParseFunc func([]byte) error

// Family binds a production parser to its canonical schema owner, deterministic
// projection, and approved package export.
type Family struct {
	ID                   FamilyID
	ParserID             string
	parser               ParseFunc
	CanonicalOwnerPath   string
	SchemaProjectionPath string
	ExportPath           string
}

// Diagnostic is a stable family/path failure emitted by registry validation.
type Diagnostic struct {
	Code    string
	Family  FamilyID
	Path    string
	Message string
}

func (d Diagnostic) Error() string {
	return fmt.Sprintf("%s: configuration family %q path %q: %s", d.Code, d.Family, d.Path, d.Message)
}

var expectedFamilies = []Family{
	{
		ID:                   FamilyGlobal,
		ParserID:             "pkg/transports/mapping/globalconfig.Decode",
		parser:               parseGlobal,
		CanonicalOwnerPath:   "contracts/config/you-config.schema.json",
		SchemaProjectionPath: "contracts/config/you-config.schema.json",
		ExportPath:           "packages/api/generated/schemas/you-config.schema.json",
	},
	{
		ID:                   FamilyMockWorker,
		ParserID:             "pkg/services/workers/interface.ParseMockWorkersConfig",
		parser:               parseMockWorkers,
		CanonicalOwnerPath:   "contracts/config/mock-workers.schema.json",
		SchemaProjectionPath: "contracts/config/mock-workers.schema.json",
		ExportPath:           "packages/api/generated/schemas/mock-workers.schema.json",
	},
	{
		ID:                   FamilyFactory,
		ParserID:             "pkg/transports/mapping/factoryconfig.FactoryConfigMapper.Expand",
		parser:               parseFactory,
		CanonicalOwnerPath:   "api/openapi.yaml#/components/schemas/Factory",
		SchemaProjectionPath: "contracts/config/factory.schema.json",
		ExportPath:           "packages/api/generated/schemas/factory.schema.json",
	},
}

// Families returns independent registry entries in stable family order.
func Families() []Family {
	return append([]Family(nil), expectedFamilies...)
}

// Parse invokes the registered production parser.
func (f Family) Parse(payload []byte) error {
	if f.parser == nil {
		return fmt.Errorf("configuration family %q has no production parser", f.ID)
	}
	return f.parser(payload)
}

// ValidateFamilies fails closed when a root is omitted, duplicated, or wired to
// another root's parser or schema paths.
func ValidateFamilies(families []Family) []Diagnostic {
	byID := make(map[FamilyID][]Family, len(families))
	for _, family := range families {
		byID[family.ID] = append(byID[family.ID], family)
	}

	var diagnostics []Diagnostic
	for _, expected := range expectedFamilies {
		entries := byID[expected.ID]
		switch len(entries) {
		case 0:
			diagnostics = append(diagnostics, registryDiagnostic("config.family.missing", expected, expected.CanonicalOwnerPath, "registered root is missing"))
			continue
		case 1:
		default:
			diagnostics = append(diagnostics, registryDiagnostic("config.family.duplicate", expected, expected.CanonicalOwnerPath, "registered root appears more than once"))
			continue
		}
		actual := entries[0]
		diagnostics = append(diagnostics, compareFamily(expected, actual)...)
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	for _, rawID := range ids {
		id := FamilyID(rawID)
		entries := byID[id]
		if !isExpectedFamily(id) {
			for _, entry := range entries {
				diagnostics = append(diagnostics, registryDiagnostic("config.family.unexpected", entry, entry.CanonicalOwnerPath, "unrecognized configuration root"))
			}
		}
	}
	return diagnostics
}

func compareFamily(expected, actual Family) []Diagnostic {
	checks := []struct {
		actual   string
		expected string
		path     string
		label    string
	}{
		{actual.ParserID, expected.ParserID, actual.CanonicalOwnerPath, "production parser"},
		{actual.CanonicalOwnerPath, expected.CanonicalOwnerPath, actual.CanonicalOwnerPath, "canonical schema owner"},
		{actual.SchemaProjectionPath, expected.SchemaProjectionPath, actual.SchemaProjectionPath, "schema projection"},
		{actual.ExportPath, expected.ExportPath, actual.ExportPath, "approved export"},
	}
	var diagnostics []Diagnostic
	if actual.parser == nil {
		diagnostics = append(diagnostics, registryDiagnostic("config.family.cross_wired", expected, expected.CanonicalOwnerPath, "production parser is nil"))
	} else if reflect.ValueOf(actual.parser).Pointer() != reflect.ValueOf(expected.parser).Pointer() {
		diagnostics = append(diagnostics, registryDiagnostic("config.family.cross_wired", expected, actual.CanonicalOwnerPath, "production parser does not match its registered root"))
	}
	for _, check := range checks {
		if check.actual != check.expected {
			diagnostics = append(diagnostics, registryDiagnostic(
				"config.family.cross_wired", expected, check.path,
				fmt.Sprintf("%s is %q, want %q", check.label, check.actual, check.expected),
			))
		}
	}
	return diagnostics
}

func registryDiagnostic(code string, family Family, path, message string) Diagnostic {
	if path == "" {
		path = family.CanonicalOwnerPath
	}
	return Diagnostic{Code: code, Family: family.ID, Path: path, Message: message}
}

func isExpectedFamily(id FamilyID) bool {
	for _, family := range expectedFamilies {
		if family.ID == id {
			return true
		}
	}
	return false
}

func parseGlobal(payload []byte) error {
	_, err := globalconfigmapping.Decode(payload)
	return err
}

func parseMockWorkers(payload []byte) error {
	_, err := workers.ParseMockWorkersConfig(payload)
	return err
}

func parseFactory(payload []byte) error {
	_, err := factorymapping.NewFactoryConfigMapper().Expand(payload)
	return err
}
