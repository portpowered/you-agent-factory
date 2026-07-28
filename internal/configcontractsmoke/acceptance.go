package configcontractsmoke

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	OutcomeAccept = "accept"
	OutcomeReject = "reject"

	CategoryValid             = "valid"
	CategoryCompatibility     = "compatibility"
	CategoryMalformedDocument = "malformed-document"
	CategoryUnknownField      = "unknown-field"
	CategoryIncompatibleValue = "incompatible-value"
)

// AcceptanceCase records one stable production-loader/schema parity example.
type AcceptanceCase struct {
	ID           string
	Family       FamilyID
	Category     string
	FixturePath  string
	Outcome      string
	DocumentPath string
}

// AcceptanceEvidence records the independently observed boundary outcomes.
type AcceptanceEvidence struct {
	CaseID        string
	Family        FamilyID
	FixturePath   string
	LoaderOutcome string
	SchemaOutcome string
	DocumentPath  string
}

// AcceptanceCases returns representative indexed compatibility, malformed,
// unknown-field, and incompatible-value cases for every configuration root.
func AcceptanceCases() []AcceptanceCase {
	return []AcceptanceCase{
		{ID: "global-valid-defaults", Family: FamilyGlobal, Category: CategoryValid, FixturePath: "pkg/services/operator_settings/testdata/fixtures/valid/defaults-only.json", Outcome: OutcomeAccept},
		{ID: "global-compat-missing-presets", Family: FamilyGlobal, Category: CategoryCompatibility, FixturePath: "pkg/services/operator_settings/testdata/fixtures/valid/worker-presets-missing.json", Outcome: OutcomeAccept},
		{ID: "global-malformed-json", Family: FamilyGlobal, Category: CategoryMalformedDocument, FixturePath: "pkg/services/operator_settings/testdata/fixtures/invalid/malformed-json.json", Outcome: OutcomeReject, DocumentPath: "/"},
		{ID: "global-unknown-field", Family: FamilyGlobal, Category: CategoryUnknownField, FixturePath: "pkg/services/operator_settings/testdata/fixtures/invalid/unknown-top-level.json", Outcome: OutcomeReject, DocumentPath: "/unexpectedTopLevel"},
		{ID: "global-incompatible-provider", Family: FamilyGlobal, Category: CategoryIncompatibleValue, FixturePath: "pkg/services/operator_settings/testdata/fixtures/invalid/preset-unsupported-provider.json", Outcome: OutcomeReject, DocumentPath: "/workerPresets/0/modelProvider"},

		{ID: "mock-worker-valid-empty", Family: FamilyMockWorker, Category: CategoryValid, FixturePath: "pkg/services/workers/internal/interface/testdata/fixtures/valid/empty-accept.json", Outcome: OutcomeAccept},
		{ID: "mock-worker-compat-reject-default", Family: FamilyMockWorker, Category: CategoryCompatibility, FixturePath: "pkg/services/workers/internal/interface/testdata/fixtures/valid/reject-without-reject-config.json", Outcome: OutcomeAccept},
		{ID: "mock-worker-malformed-json", Family: FamilyMockWorker, Category: CategoryMalformedDocument, FixturePath: "pkg/services/workers/internal/interface/testdata/fixtures/invalid/trailing-json.json", Outcome: OutcomeReject, DocumentPath: "/"},
		{ID: "mock-worker-unknown-field", Family: FamilyMockWorker, Category: CategoryUnknownField, FixturePath: "pkg/services/workers/internal/interface/testdata/fixtures/invalid/unknown-top-level.json", Outcome: OutcomeReject, DocumentPath: "/unexpectedTopLevel"},
		{ID: "mock-worker-incompatible-run-type", Family: FamilyMockWorker, Category: CategoryIncompatibleValue, FixturePath: "pkg/services/workers/internal/interface/testdata/fixtures/invalid/unknown-run-type.json", Outcome: OutcomeReject, DocumentPath: "/mockWorkers/0/runType"},

		{ID: "factory-valid-minimal", Family: FamilyFactory, Category: CategoryValid, FixturePath: "internal/configcontractsmoke/testdata/factory-valid-minimal.json", Outcome: OutcomeAccept},
		{ID: "factory-malformed-json", Family: FamilyFactory, Category: CategoryMalformedDocument, FixturePath: "internal/configcontractsmoke/testdata/factory-malformed.json", Outcome: OutcomeReject, DocumentPath: "/"},
		{ID: "factory-unknown-field", Family: FamilyFactory, Category: CategoryUnknownField, FixturePath: "internal/configcontractsmoke/testdata/factory-unknown-field.json", Outcome: OutcomeReject, DocumentPath: "/unexpectedTopLevel"},
		{ID: "factory-incompatible-worker-type", Family: FamilyFactory, Category: CategoryIncompatibleValue, FixturePath: "pkg/transports/mapping/factoryconfig/openapitests/testdata/fixtures/reject/miscased-worker-type.json", Outcome: OutcomeReject, DocumentPath: "/workers/0/type"},
		{ID: "factory-malformed-layout", Family: FamilyFactory, Category: CategoryMalformedDocument, FixturePath: "pkg/transports/mapping/factoryconfig/openapitests/testdata/fixtures/reject/malformed-layout-missing-schema-version.json", Outcome: OutcomeReject, DocumentPath: "/layout/schemaVersion"},
	}
}

// CheckAcceptanceParity runs each indexed document through its registered
// production parser and matching Draft 2020-12 schema.
func CheckAcceptanceParity(repositoryRoot string, families []Family, cases []AcceptanceCase) ([]AcceptanceEvidence, []Diagnostic) {
	byID := make(map[FamilyID]Family, len(families))
	for _, family := range families {
		byID[family.ID] = family
	}

	schemas := make(map[FamilyID]*jsonschema.Schema, len(families))
	var evidence []AcceptanceEvidence
	var diagnostics []Diagnostic
	for _, acceptanceCase := range cases {
		family, ok := byID[acceptanceCase.Family]
		if !ok {
			diagnostics = append(diagnostics, acceptanceDiagnostic("config.acceptance.family_missing", acceptanceCase, "indexed family is not registered"))
			continue
		}
		schema := schemas[family.ID]
		if schema == nil {
			compiled, err := compileFamilySchema(repositoryRoot, family)
			if err != nil {
				diagnostics = append(diagnostics, acceptanceDiagnostic("config.acceptance.schema", acceptanceCase, err.Error()))
				continue
			}
			schema = compiled
			schemas[family.ID] = schema
		}

		payload, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(acceptanceCase.FixturePath)))
		if err != nil {
			diagnostics = append(diagnostics, acceptanceDiagnostic("config.acceptance.fixture", acceptanceCase, "fixture could not be read"))
			continue
		}
		result, caseDiagnostics := checkAcceptanceCase(family, schema, acceptanceCase, payload)
		evidence = append(evidence, result)
		diagnostics = append(diagnostics, caseDiagnostics...)
	}
	return evidence, diagnostics
}

func checkAcceptanceCase(family Family, schema *jsonschema.Schema, acceptanceCase AcceptanceCase, payload []byte) (AcceptanceEvidence, []Diagnostic) {
	loaderErr := family.Parse(payload)
	schemaErr := validateSchemaPayload(schema, payload)
	loaderOutcome := outcome(loaderErr)
	schemaOutcome := outcome(schemaErr)
	evidence := AcceptanceEvidence{
		CaseID: acceptanceCase.ID, Family: family.ID, FixturePath: acceptanceCase.FixturePath,
		LoaderOutcome: loaderOutcome, SchemaOutcome: schemaOutcome, DocumentPath: acceptanceCase.DocumentPath,
	}

	if loaderOutcome != schemaOutcome || loaderOutcome != acceptanceCase.Outcome {
		message := fmt.Sprintf("loader=%s schema=%s expected=%s", loaderOutcome, schemaOutcome, acceptanceCase.Outcome)
		return evidence, []Diagnostic{acceptanceDiagnostic("config.acceptance.mismatch", acceptanceCase, message)}
	}
	if acceptanceCase.Outcome == OutcomeReject && acceptanceCase.DocumentPath != "" {
		loaderPathOK := rejectionIdentifiesPath(loaderErr, acceptanceCase.DocumentPath)
		schemaPathOK := rejectionIdentifiesPath(schemaErr, acceptanceCase.DocumentPath)
		if !loaderPathOK || !schemaPathOK {
			message := fmt.Sprintf("rejection path %q not identified by loader=%t schema=%t", acceptanceCase.DocumentPath, loaderPathOK, schemaPathOK)
			return evidence, []Diagnostic{acceptanceDiagnostic("config.acceptance.rejection_path", acceptanceCase, message)}
		}
	}
	return evidence, nil
}

func compileFamilySchema(repositoryRoot string, family Family) (*jsonschema.Schema, error) {
	payload, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(family.SchemaProjectionPath)))
	if err != nil {
		return nil, fmt.Errorf("read schema projection %q", family.SchemaProjectionPath)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("parse schema projection %q", family.SchemaProjectionPath)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	resourceID := "config-family-" + string(family.ID) + ".json"
	if err := compiler.AddResource(resourceID, document); err != nil {
		return nil, fmt.Errorf("register schema projection %q", family.SchemaProjectionPath)
	}
	compiled, err := compiler.Compile(resourceID)
	if err != nil {
		return nil, fmt.Errorf("compile schema projection %q: %w", family.SchemaProjectionPath, err)
	}
	return compiled, nil
}

func validateSchemaPayload(schema *jsonschema.Schema, payload []byte) error {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(payload))
	if err != nil {
		return err
	}
	return schema.Validate(document)
}

func outcome(err error) string {
	if err == nil {
		return OutcomeAccept
	}
	return OutcomeReject
}

func rejectionIdentifiesPath(err error, documentPath string) bool {
	if err == nil {
		return false
	}
	if documentPath == "/" {
		return true
	}
	var validationErr *jsonschema.ValidationError
	if errors.As(err, &validationErr) {
		for _, path := range validationPaths(validationErr) {
			if path == documentPath || strings.HasPrefix(path, documentPath+"/") || strings.HasPrefix(documentPath, path+"/") {
				return true
			}
		}
	}
	leaf := documentPath[strings.LastIndex(documentPath, "/")+1:]
	return leaf != "" && strings.Contains(err.Error(), leaf)
}

func validationPaths(err *jsonschema.ValidationError) []string {
	if len(err.Causes) == 0 {
		return []string{jsonPointer(err.InstanceLocation)}
	}
	var paths []string
	for _, cause := range err.Causes {
		paths = append(paths, validationPaths(cause)...)
	}
	return paths
}

func jsonPointer(segments []string) string {
	if len(segments) == 0 {
		return "/"
	}
	escaped := make([]string, len(segments))
	for i, segment := range segments {
		escaped[i] = strings.NewReplacer("~", "~0", "/", "~1").Replace(segment)
	}
	return "/" + strings.Join(escaped, "/")
}

func acceptanceDiagnostic(code string, acceptanceCase AcceptanceCase, message string) Diagnostic {
	if acceptanceCase.DocumentPath != "" {
		message += fmt.Sprintf(" at document path %q", acceptanceCase.DocumentPath)
	}
	return Diagnostic{Code: code, Family: acceptanceCase.Family, Path: acceptanceCase.FixturePath, Message: message}
}
