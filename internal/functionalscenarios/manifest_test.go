package functionalscenarios

import (
	"strings"
	"testing"
)

func TestBuildReviewedManifestPublishesTruthfulCoverageAndSSEPolicy(t *testing.T) {
	t.Parallel()

	manifest, err := BuildReviewedManifest(repositoryProjection(t))
	if err != nil {
		t.Fatalf("BuildReviewedManifest() error = %v", err)
	}
	if len(manifest.Scenarios) != 97 {
		t.Fatalf("scenario count = %d, want 97", len(manifest.Scenarios))
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
	assertSSEPolicy(t, byID[sessionEventsStableID], true, SSERequired, StatusPartial)
	assertSSEPolicy(t, byID[globalEventsStableID], false, SSEDeprecatedLaterRemoval, StatusNotApplicable)
	assertSSEPolicy(t, byID[responseEventsStableID], false, SSECurrentlyDeferred, StatusNotApplicable)
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
