package functionalscenarios

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestBuildReviewedManifestPublishesTruthfulCoverageAndSSEPolicy(t *testing.T) {
	t.Parallel()

	manifest, err := BuildReviewedManifest(repositoryProjection(t))
	if err != nil {
		t.Fatalf("BuildReviewedManifest() error = %v", err)
	}
	byID := make(map[string]Scenario, len(manifest.Scenarios))
	for _, scenario := range manifest.Scenarios {
		byID[scenario.StableID] = scenario
	}
	if got := byID["cli/you.session"]; got.Status != StatusNotApplicable || got.Classification != ClassificationGrouping {
		t.Fatalf("CLI grouping scenario = %#v", got)
	}
	if got := byID["cli/you.work.move"]; got.Status != StatusCovered || got.Lane != LaneLong || len(got.Evidence) != 1 {
		t.Fatalf("covered CLI scenario = %#v", got)
	}
	if got := byID["cli/you.session.dispatches"]; got.Status != StatusMissing || len(got.Evidence) != 0 {
		t.Fatalf("CLI dispatches scenario = %#v, want truthful missing status", got)
	}
	assertSSEPolicy(t, byID[sessionEventsStableID], true, SSERequired, StatusPartial)
	assertSSEPolicy(t, byID[responseEventsStableID], false, SSECurrentlyDeferred, StatusNotApplicable)
}

func TestCheckEvidenceReferencesRejectsStaleAndInternalOnlyCitations(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	functionalPath := filepath.Join(root, "tests", "functional", "api_test.go")
	if err := os.MkdirAll(filepath.Dir(functionalPath), 0o755); err != nil {
		t.Fatalf("create functional fixture directory: %v", err)
	}
	if err := os.WriteFile(functionalPath, []byte(`package functional

import (
	"testing"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

func TestGetOne(t *testing.T) { functionalevidence.Covers(t, "rest/getOne") }
func TestGetTwo(t *testing.T) { functionalevidence.Covers(t, "rest/getTwo") }
func TestNoDeclaration(t *testing.T) {}
func TestHelper() {}
`), 0o644); err != nil {
		t.Fatalf("write functional fixture: %v", err)
	}
	writeEvidenceRegistryFixture(t, root, []EvidenceDeclaration{{
		Test: "tests/functional/api_test.go::TestGetOne", StableIDs: []string{"rest/getOne"},
	}})

	base := &Manifest{FormatVersion: ManifestFormatVersion, Scenarios: []Scenario{{
		StableID: "rest/getOne", Interface: InterfaceREST,
		Evidence: []Evidence{{Test: "tests/functional/api_test.go::TestGetOne", Boundary: InterfaceREST}},
	}}}
	if err := CheckEvidenceReferences(root, base); err != nil {
		t.Fatalf("CheckEvidenceReferences() error = %v", err)
	}

	tests := []struct {
		name      string
		reference string
		want      string
	}{
		{name: "nonexistent symbol", reference: "tests/functional/api_test.go::TestDeleted", want: `cited executable test symbol "TestDeleted" does not exist`},
		{name: "non-test helper", reference: "tests/functional/api_test.go::TestHelper", want: `cited executable test symbol "TestHelper" does not exist`},
		{name: "nonexistent file", reference: "tests/functional/missing_test.go::TestMissing", want: "cited test file does not exist"},
		{name: "internal package test", reference: "internal/service/service_test.go::TestHandler", want: "not an internal package test"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := cloneManifest(t, base)
			manifest.Scenarios[0].Evidence[0].Test = test.reference
			err := CheckEvidenceReferences(root, manifest)
			if err == nil || !strings.Contains(err.Error(), `scenario "rest/getOne"`) || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), "add or correct") {
				t.Fatalf("CheckEvidenceReferences() error = %v, want stable ID, %q, and remediation", err, test.want)
			}
		})
	}

	writeEvidenceRegistryFixture(t, root, []EvidenceDeclaration{
		{Test: "tests/functional/api_test.go::TestGetOne", StableIDs: []string{"rest/getOne"}},
		{Test: "tests/functional/api_test.go::TestGetTwo", StableIDs: []string{"rest/getTwo"}},
	})
	substituted := cloneManifest(t, base)
	substituted.Scenarios[0].Evidence[0].Test = "tests/functional/api_test.go::TestGetTwo"
	err := CheckEvidenceReferences(root, substituted)
	if err == nil || !strings.Contains(err.Error(), `scenario "rest/getOne"`) || !strings.Contains(err.Error(), "not declared by its customer-boundary test for this component") || !strings.Contains(err.Error(), "rest/getTwo") {
		t.Fatalf("CheckEvidenceReferences() substitution error = %v, want exact component binding diagnostic", err)
	}

	writeEvidenceRegistryFixture(t, root, []EvidenceDeclaration{{
		Test: "tests/functional/api_test.go::TestNoDeclaration", StableIDs: []string{"rest/getOne"},
	}})
	missingDeclaration := cloneManifest(t, base)
	missingDeclaration.Scenarios[0].Evidence[0].Test = "tests/functional/api_test.go::TestNoDeclaration"
	err = CheckEvidenceReferences(root, missingDeclaration)
	if err == nil || !strings.Contains(err.Error(), `scenario "rest/getOne"`) || !strings.Contains(err.Error(), "must contain exactly one direct functionalevidence.Covers call") {
		t.Fatalf("CheckEvidenceReferences() missing declaration error = %v, want cited-test ownership diagnostic", err)
	}
}

func writeEvidenceRegistryFixture(t *testing.T, root string, declarations []EvidenceDeclaration) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(EvidenceRegistryRelativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create evidence registry fixture directory: %v", err)
	}
	payload, err := json.Marshal(EvidenceRegistry{FormatVersion: EvidenceRegistryFormatVersion, Declarations: declarations})
	if err != nil {
		t.Fatalf("marshal evidence registry fixture: %v", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write evidence registry fixture: %v", err)
	}
}

func TestValidateManifestRejectsInvalidEvidenceAndSSEDispositions(t *testing.T) {
	t.Parallel()

	base := Scenario{
		StableID: "rest/getOne", Name: "getOne", Interface: InterfaceREST,
		Classification: ClassificationOperation, Status: StatusCovered,
		Lane: LaneShort, ExecutionClass: ExecutionDeterministic,
		Evidence: []Evidence{{Test: "tests/functional/api_test.go::TestGetOne", Boundary: InterfaceREST}},
	}
	tests := []struct {
		name string
		edit func(*Scenario)
		want string
	}{
		{name: "covered without named test", edit: func(s *Scenario) { s.Evidence = nil }, want: "covered status requires named customer-boundary evidence"},
		{name: "package boundary is not customer evidence", edit: func(s *Scenario) { s.Evidence[0].Boundary = "service" }, want: `evidence boundary "service" is not a customer boundary`},
		{name: "different customer boundary does not qualify", edit: func(s *Scenario) { s.Evidence[0].Boundary = InterfaceCLI }, want: `evidence boundary "cli" does not exercise the scenario interface`},
		{name: "external classification requires external lane", edit: func(s *Scenario) { s.ExecutionClass = ExecutionExternal }, want: "must be assigned together"},
		{name: "required session SSE cannot be waived", edit: func(s *Scenario) {
			s.StableID, s.Name, s.Interface = sessionEventsStableID, "getEventsBySessionId", InterfaceSSE
			s.Status, s.Evidence, s.Gap = StatusPartial, []Evidence{{Test: "tests/release/release_smoke_test.go::TestReleaseSmokeHarness", Boundary: InterfaceSSE}}, "JSON recovery missing"
			s.SSE = &SSEDisposition{Required: false, Disposition: SSECurrentlyDeferred, Scope: "wrong"}
		}, want: "SSE disposition must be required=true"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			scenario := base
			scenario.Evidence = append([]Evidence(nil), base.Evidence...)
			test.edit(&scenario)
			err := ValidateManifest(&Manifest{FormatVersion: ManifestFormatVersion, Scenarios: []Scenario{scenario}})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateManifest() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCheckManifestRejectsUnmappedCanonicalComponentsForEveryInterface(t *testing.T) {
	t.Parallel()

	baseProjection := repositoryProjection(t)
	baseManifest, err := BuildReviewedManifest(baseProjection)
	if err != nil {
		t.Fatalf("BuildReviewedManifest() error = %v", err)
	}
	components := []Component{
		newComponent(InterfaceCLI, "you.future", "you future", ClassificationRunnable),
		newComponent(InterfaceREST, "getFuture", "getFuture", ClassificationOperation),
		newComponent(InterfaceMCP, "mcp.future", "future", ClassificationTool),
		newComponent(InterfaceSSE, "getFutureEvents", "getFutureEvents", ClassificationEventStream),
	}
	for _, component := range components {
		component := component
		t.Run(component.Interface, func(t *testing.T) {
			t.Parallel()
			projection := cloneProjection(baseProjection)
			projection.Components = append(projection.Components, component)
			slices.SortFunc(projection.Components, func(left, right Component) int {
				return strings.Compare(left.StableID, right.StableID)
			})
			err := CheckManifest(projection, baseManifest)
			want := `scenario "` + component.StableID + `": canonical component is unmapped`
			if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "add or correct") {
				t.Fatalf("CheckManifest() error = %v, want %q and remediation", err, want)
			}
		})
	}
}

func TestCheckManifestRejectsInventoryAndEvidenceDrift(t *testing.T) {
	t.Parallel()

	projection := repositoryProjection(t)
	base, err := BuildReviewedManifest(projection)
	if err != nil {
		t.Fatalf("BuildReviewedManifest() error = %v", err)
	}
	tests := []struct {
		name string
		edit func(*Manifest)
		want string
	}{
		{name: "duplicate stable ID", edit: func(m *Manifest) { m.Scenarios = append(m.Scenarios, m.Scenarios[len(m.Scenarios)-1]) }, want: "duplicate manifest record"},
		{name: "no longer canonical", edit: func(m *Manifest) {
			m.Scenarios = append(m.Scenarios, Scenario{StableID: "rest/removedOperation", Name: "removedOperation", Interface: InterfaceREST, Classification: ClassificationOperation, Status: StatusMissing, Lane: LaneShort, ExecutionClass: ExecutionDeterministic, ReviewedReason: "No qualifying evidence exists."})
			slices.SortFunc(m.Scenarios, func(left, right Scenario) int { return strings.Compare(left.StableID, right.StableID) })
		}, want: "manifest record is no longer canonical"},
		{name: "unknown status", edit: func(m *Manifest) { m.Scenarios[0].Status = "unknown" }, want: `unknown status "unknown"`},
		{name: "unknown lane", edit: func(m *Manifest) { m.Scenarios[0].Lane = "unknown" }, want: `unknown lane "unknown"`},
		{name: "unknown execution class", edit: func(m *Manifest) { m.Scenarios[0].ExecutionClass = "unknown" }, want: `unknown executionClass "unknown"`},
		{name: "invalid status evidence", edit: func(m *Manifest) { m.Scenarios[0].Status = StatusCovered }, want: "covered status requires named customer-boundary evidence"},
		{name: "incorrect SSE disposition", edit: func(m *Manifest) {
			for index := range m.Scenarios {
				if m.Scenarios[index].StableID == sessionEventsStableID {
					m.Scenarios[index].SSE.Required = false
					return
				}
			}
		}, want: "SSE disposition must be required=true"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			manifest := cloneManifest(t, base)
			test.edit(manifest)
			err := CheckManifest(projection, manifest)
			if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), "add or correct") {
				t.Fatalf("CheckManifest() error = %v, want %q and remediation", err, test.want)
			}
		})
	}
}

func cloneProjection(value *Projection) *Projection {
	result := *value
	result.Components = append([]Component(nil), value.Components...)
	return &result
}

func cloneManifest(t *testing.T, value *Manifest) *Manifest {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal manifest clone: %v", err)
	}
	result, err := DecodeManifest(payload)
	if err != nil {
		t.Fatalf("decode manifest clone: %v", err)
	}
	return result
}

func repositoryProjection(t *testing.T) *Projection {
	t.Helper()
	root := repositoryRoot(t)
	projection, err := Project(
		readFile(t, root+"/contracts/cli/commands.json"),
		readFile(t, root+"/api/openapi.yaml"),
		readFile(t, root+"/contracts/mcp/tools.json"),
	)
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	return projection
}

func assertSSEPolicy(t *testing.T, scenario Scenario, required bool, disposition, status string) {
	t.Helper()
	if scenario.Status != status || scenario.SSE == nil || scenario.SSE.Required != required || scenario.SSE.Disposition != disposition {
		t.Fatalf("SSE scenario %q = %#v", scenario.StableID, scenario)
	}
}
