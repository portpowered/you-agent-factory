package configcontractsmoke

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndexedAcceptanceCasesMatchProductionLoadersAndSchemas(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	cases := AcceptanceCases()
	families := FamiliesWithParser(testGlobalParser)
	evidence, diagnostics := CheckAcceptanceParity(repositoryRoot, families, cases)
	if len(diagnostics) != 0 {
		t.Fatalf("CheckAcceptanceParity() diagnostics = %v", diagnostics)
	}
	if len(evidence) != len(cases) {
		t.Fatalf("evidence count = %d, want %d", len(evidence), len(cases))
	}

	counts := make(map[FamilyID]map[string]int)
	for _, result := range evidence {
		if result.LoaderOutcome != result.SchemaOutcome {
			t.Errorf("case %q loader=%s schema=%s", result.CaseID, result.LoaderOutcome, result.SchemaOutcome)
		}
		if counts[result.Family] == nil {
			counts[result.Family] = make(map[string]int)
		}
		counts[result.Family][result.LoaderOutcome]++
	}
	for _, family := range families {
		if counts[family.ID][OutcomeAccept] == 0 || counts[family.ID][OutcomeReject] == 0 {
			t.Errorf("configuration family %q outcomes = %#v, want accepted and rejected evidence", family.ID, counts[family.ID])
		}
		for _, category := range []string{CategoryMalformedDocument, CategoryUnknownField, CategoryIncompatibleValue} {
			if !hasAcceptanceCategory(cases, family.ID, category) {
				t.Errorf("configuration family %q has no %q acceptance case", family.ID, category)
			}
		}
	}
}

func hasAcceptanceCategory(cases []AcceptanceCase, family FamilyID, category string) bool {
	for _, acceptanceCase := range cases {
		if acceptanceCase.Family == family && acceptanceCase.Category == category {
			return true
		}
	}
	return false
}

func TestAcceptanceMismatchDiagnosticsNameFamilyFixtureAndOutcomes(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	family := Families()[0]
	schema, err := compileFamilySchema(repositoryRoot, family)
	if err != nil {
		t.Fatalf("compileFamilySchema() error = %v", err)
	}
	payload, err := os.ReadFile(filepath.Join(repositoryRoot, "pkg", "services", "operator_settings", "testdata", "fixtures", "valid", "defaults-only.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	family.parser = func([]byte) error { return os.ErrInvalid }
	acceptanceCase := AcceptanceCase{ID: "schema-only-acceptance", Family: FamilyGlobal, FixturePath: "fixtures/schema-only.json", Outcome: OutcomeAccept}
	_, diagnostics := checkAcceptanceCase(family, schema, acceptanceCase, payload)
	assertAcceptanceDiagnostic(t, diagnostics, "config.acceptance.mismatch", acceptanceCase, "loader=reject schema=accept expected=accept")
}

func TestParserOnlyAcceptanceProducesMismatchDiagnostic(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	family := Families()[0]
	schema, err := compileFamilySchema(repositoryRoot, family)
	if err != nil {
		t.Fatalf("compileFamilySchema() error = %v", err)
	}
	payload, err := os.ReadFile(filepath.Join(repositoryRoot, "pkg", "services", "operator_settings", "testdata", "fixtures", "invalid", "unknown-top-level.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	family.parser = func([]byte) error { return nil }
	acceptanceCase := AcceptanceCase{ID: "parser-only-acceptance", Family: FamilyGlobal, FixturePath: "fixtures/parser-only.json", Outcome: OutcomeReject}
	_, diagnostics := checkAcceptanceCase(family, schema, acceptanceCase, payload)
	assertAcceptanceDiagnostic(t, diagnostics, "config.acceptance.mismatch", acceptanceCase, "loader=accept schema=reject expected=reject")
}

func TestAcceptanceRejectPathMismatchIsStable(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	family := Families()[0]
	schema, err := compileFamilySchema(repositoryRoot, family)
	if err != nil {
		t.Fatalf("compileFamilySchema() error = %v", err)
	}
	payload, err := os.ReadFile(filepath.Join(repositoryRoot, "pkg", "services", "operator_settings", "testdata", "fixtures", "invalid", "unknown-top-level.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	acceptanceCase := AcceptanceCase{
		ID: "wrong-rejection-path", Family: FamilyGlobal, FixturePath: "fixtures/wrong-path.json",
		Outcome: OutcomeReject, DocumentPath: "/differentField",
	}
	_, diagnostics := checkAcceptanceCase(family, schema, acceptanceCase, payload)
	assertAcceptanceDiagnostic(t, diagnostics, "config.acceptance.rejection_path", acceptanceCase, "/differentField")
}

func assertAcceptanceDiagnostic(t *testing.T, diagnostics []Diagnostic, code string, acceptanceCase AcceptanceCase, fragments ...string) {
	t.Helper()
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics = %v, want one", diagnostics)
	}
	diagnostic := diagnostics[0]
	if diagnostic.Code != code || diagnostic.Family != acceptanceCase.Family || diagnostic.Path != acceptanceCase.FixturePath {
		t.Fatalf("diagnostic = %#v, want code=%q family=%q path=%q", diagnostic, code, acceptanceCase.Family, acceptanceCase.FixturePath)
	}
	message := diagnostic.Error()
	for _, fragment := range fragments {
		if !strings.Contains(message, fragment) {
			t.Fatalf("diagnostic %q does not contain %q", message, fragment)
		}
	}
}
