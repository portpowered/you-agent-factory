package functionalscenarios

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const (
	ManifestFormatVersion = "functional-scenario-manifest/v1"

	StatusCovered       = "covered"
	StatusPartial       = "partial"
	StatusMissing       = "missing"
	StatusNotApplicable = "not-applicable"

	LaneShort    = "short"
	LaneLong     = "long"
	LaneExternal = "externally-provisioned"

	ExecutionDeterministic = "deterministic"
	ExecutionExternal      = "external"

	SSERequired               = "required"
	SSEDeprecatedLaterRemoval = "deprecated-later-removal"
	SSECurrentlyDeferred      = "currently-deferred"

	sessionEventsStableID  = "sse/getEventsBySessionId"
	globalEventsStableID   = "sse/getEvents"
	responseEventsStableID = "sse/getFactoryResponseEventsBySessionId"

	sessionEventsScope = "session Factory Event stream, including recovery after malformed JSON data"
)

// Manifest is the reviewed functional-coverage projection of canonical public components.
type Manifest struct {
	FormatVersion string     `json:"formatVersion"`
	Scenarios     []Scenario `json:"scenarios"`
}

// Scenario records reviewed customer-boundary evidence or an explicit gap.
type Scenario struct {
	StableID       string          `json:"stableId"`
	Name           string          `json:"name"`
	Interface      string          `json:"interface"`
	Classification string          `json:"classification"`
	Status         string          `json:"status"`
	Lane           string          `json:"lane"`
	ExecutionClass string          `json:"executionClass"`
	Evidence       []Evidence      `json:"evidence,omitempty"`
	Gap            string          `json:"gap,omitempty"`
	ReviewedReason string          `json:"reviewedReason,omitempty"`
	SSE            *SSEDisposition `json:"sse,omitempty"`
}

// Evidence names a repository-owned test and the customer boundary it exercises.
type Evidence struct {
	Test     string `json:"test"`
	Boundary string `json:"boundary"`
}

// SSEDisposition makes required and non-required stream policy explicit.
type SSEDisposition struct {
	Required    bool   `json:"required"`
	Disposition string `json:"disposition"`
	Scope       string `json:"scope"`
}

// DecodeManifest decodes a checked-in reviewed manifest without changing it.
func DecodeManifest(data []byte) (*Manifest, error) {
	manifest := &Manifest{}
	if err := json.Unmarshal(data, manifest); err != nil {
		return nil, fmt.Errorf("decode functional scenario manifest: %w", err)
	}
	return manifest, nil
}

// BuildReviewedManifest applies the reviewed coverage decisions to a projection.
func BuildReviewedManifest(projection *Projection) (*Manifest, error) {
	if projection == nil {
		return nil, fmt.Errorf("build reviewed manifest: component projection is nil")
	}
	scenarios := make([]Scenario, 0, len(projection.Components))
	for _, component := range projection.Components {
		scenario := missingScenario(component)
		if component.Classification == ClassificationGrouping {
			scenario.Status = StatusNotApplicable
			scenario.ReviewedReason = "CLI grouping node has no independently runnable customer behavior; its child commands remain separately inventoried."
		}
		applyReviewedEvidence(&scenario)
		scenarios = append(scenarios, scenario)
	}
	manifest := &Manifest{FormatVersion: ManifestFormatVersion, Scenarios: scenarios}
	if err := ValidateManifest(manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func missingScenario(component Component) Scenario {
	return Scenario{
		StableID: component.StableID, Name: component.Name, Interface: component.Interface,
		Classification: component.Classification, Status: StatusMissing, Lane: LaneShort,
		ExecutionClass: ExecutionDeterministic,
		ReviewedReason: "No qualifying repository-owned functional test currently invokes and observes this component through its customer boundary.",
	}
}

func applyReviewedEvidence(scenario *Scenario) {
	switch scenario.StableID {
	case "cli/you.docs":
		markCovered(scenario, LaneShort, "tests/release/release_smoke_test.go::TestGoInstallSmoke_InstallsCmdFactoryBinaryIntoCleanGOBIN", InterfaceCLI)
	case "cli/you.run":
		markCovered(scenario, LaneShort, "tests/functional/transport/cli/commands/run_wiring_test.go::TestCLIRunFactoryByPath", InterfaceCLI)
	case "cli/you.submit.batch":
		scenario.Status = StatusCovered
		scenario.Lane = LaneLong
		scenario.ReviewedReason = ""
		scenario.Evidence = []Evidence{
			{Test: "tests/functional/transport/cli/commands/submit_wiring_test.go::TestCLISubmitBatchFile", Boundary: InterfaceCLI},
			{Test: "tests/functional/work/transports/cli/submit/batch_contract/batch_contract_test.go::TestCLISubmitBatchSuccessHumanAndJSONShapes", Boundary: InterfaceCLI},
		}
	case "cli/you.work.move":
		markCovered(scenario, LaneLong, "tests/functional/transport/cli/commands/work_wiring_test.go::TestCLIWorkMoveChangesState", InterfaceCLI)
	case "rest/submitWorkBySessionId", "rest/listWorkBySessionId", "rest/getStatusBySessionId":
		markCovered(scenario, LaneLong, "tests/functional/transport/http/server/generated_client_test.go::TestGeneratedClientAndServerSchemaStayAligned", InterfaceREST)
	case "rest/upsertWorkRequestBySessionId":
		markCovered(scenario, LaneLong, "tests/functional/runtime_api/api_generated_smoke_test.go::TestGeneratedAPIIntegrationSmoke_BatchUpsertAcceptsWorksContent", InterfaceREST)
	case "rest/moveWorkBySessionId":
		markCovered(scenario, LaneLong, "tests/functional/work/recovery/manual_move_test.go::TestFailedCascadeCanBeRecoveredByPublicWorkMove", InterfaceREST)
	case "rest/getFactorySessionDispatch", "rest/listFactorySessionDispatches":
		markCovered(scenario, LaneLong, "tests/functional/sessions/execution/results_dispatches_test.go::TestAPIDispatchListAndDetailExposePublicCorrelation", InterfaceREST)
	case "rest/getFactorySessionResults":
		markCovered(scenario, LaneLong, "tests/functional/sessions/execution/results_dispatches_test.go::TestAPIResultAndResultsExposeTerminalInvocationData", InterfaceREST)
	case "rest/invokeFactorySessionBySessionId":
		markCovered(scenario, LaneLong, "tests/functional/sessions/execution/results_dispatches_test.go::TestAPIResultAndResultsExposeTerminalInvocationData", InterfaceREST)
	case "rest/getEventsBySessionId":
		scenario.Status = StatusPartial
		scenario.Lane = LaneLong
		scenario.ReviewedReason = ""
		scenario.Evidence = []Evidence{{Test: "tests/functional/transport/http/server/generated_client_test.go::TestGeneratedClientAndServerSchemaStayAligned", Boundary: InterfaceREST}}
		scenario.Gap = "The generated REST boundary opens and observes canonical session Factory Events, but malformed-JSON recovery is not proven."
	case sessionEventsStableID:
		scenario.Status = StatusPartial
		scenario.Lane = LaneShort
		scenario.ReviewedReason = ""
		scenario.Evidence = []Evidence{{Test: "tests/release/release_smoke_test.go::TestReleaseSmokeHarness_RunsBuiltBinaryAgainstCanonicalFixture", Boundary: InterfaceSSE}}
		scenario.Gap = "The customer SSE client decodes and observes session Factory Events, but no qualifying functional test proves that it recovers and continues after malformed JSON data."
		scenario.SSE = &SSEDisposition{Required: true, Disposition: SSERequired, Scope: sessionEventsScope}
	case globalEventsStableID:
		scenario.Status = StatusNotApplicable
		scenario.ReviewedReason = "Deprecated global Factory Event compatibility stream is non-required and retained only until later removal."
		scenario.SSE = &SSEDisposition{Required: false, Disposition: SSEDeprecatedLaterRemoval, Scope: "deprecated global Factory Event compatibility stream"}
	case responseEventsStableID:
		scenario.Status = StatusNotApplicable
		scenario.ReviewedReason = "Factory response-event stream functional coverage is non-required and currently deferred."
		scenario.SSE = &SSEDisposition{Required: false, Disposition: SSECurrentlyDeferred, Scope: "session Factory response-event stream"}
	}
}

func markCovered(scenario *Scenario, lane, test, boundary string) {
	scenario.Status = StatusCovered
	scenario.Lane = lane
	scenario.ReviewedReason = ""
	scenario.Evidence = []Evidence{{Test: test, Boundary: boundary}}
}

// ValidateManifest rejects semantically inconsistent reviewed coverage records.
func ValidateManifest(manifest *Manifest) error {
	if manifest == nil {
		return fmt.Errorf("validate functional scenario manifest: manifest is nil")
	}
	if manifest.FormatVersion != ManifestFormatVersion {
		return fmt.Errorf("validate functional scenario manifest: unknown formatVersion %q", manifest.FormatVersion)
	}
	for _, scenario := range manifest.Scenarios {
		if err := validateScenario(scenario); err != nil {
			return err
		}
	}
	return nil
}

// CheckManifest verifies that reviewed scenarios exactly cover the current
// canonical projection and still carry its public identity metadata.
func CheckManifest(projection *Projection, manifest *Manifest) error {
	if projection == nil {
		return fmt.Errorf("check functional scenario manifest: component projection is nil")
	}
	if err := ValidateManifest(manifest); err != nil {
		return withManifestRemediation(err)
	}

	components := make(map[string]Component, len(projection.Components))
	for _, component := range projection.Components {
		components[component.StableID] = component
	}
	scenarios := make(map[string]Scenario, len(manifest.Scenarios))
	for index, scenario := range manifest.Scenarios {
		if index > 0 && manifest.Scenarios[index-1].StableID > scenario.StableID {
			return manifestCheckError(scenario.Interface, scenario.StableID, "manifest records are not sorted by stable ID")
		}
		if first, exists := scenarios[scenario.StableID]; exists {
			return manifestCheckError(scenario.Interface, scenario.StableID,
				"duplicate manifest record (first declared for interface %q)", first.Interface)
		}
		scenarios[scenario.StableID] = scenario
	}

	for _, component := range projection.Components {
		scenario, exists := scenarios[component.StableID]
		if !exists {
			return manifestCheckError(component.Interface, component.StableID, "canonical component is unmapped")
		}
		if scenario.Interface != component.Interface {
			return manifestCheckError(component.Interface, component.StableID,
				"interface is %q, want canonical value %q", scenario.Interface, component.Interface)
		}
		if scenario.Name != component.Name {
			return manifestCheckError(component.Interface, component.StableID,
				"name is %q, want canonical value %q", scenario.Name, component.Name)
		}
		if scenario.Classification != component.Classification {
			return manifestCheckError(component.Interface, component.StableID,
				"classification is %q, want canonical value %q", scenario.Classification, component.Classification)
		}
	}
	for _, scenario := range manifest.Scenarios {
		if _, exists := components[scenario.StableID]; !exists {
			return manifestCheckError(scenario.Interface, scenario.StableID, "manifest record is no longer canonical")
		}
	}
	return nil
}

func manifestCheckError(customerInterface, stableID, format string, args ...any) error {
	detail := fmt.Sprintf(format, args...)
	return fmt.Errorf("check %s interface scenario %q: %s; add or correct the reviewed manifest record", customerInterface, stableID, detail)
}

func withManifestRemediation(err error) error {
	return fmt.Errorf("%w; add or correct the reviewed manifest record", err)
}

func validateScenario(scenario Scenario) error {
	prefix := fmt.Sprintf("validate %s interface scenario %q", scenario.Interface, scenario.StableID)
	if !slices.Contains([]string{StatusCovered, StatusPartial, StatusMissing, StatusNotApplicable}, scenario.Status) {
		return fmt.Errorf("%s: unknown status %q", prefix, scenario.Status)
	}
	if !slices.Contains([]string{LaneShort, LaneLong, LaneExternal}, scenario.Lane) {
		return fmt.Errorf("%s: unknown lane %q", prefix, scenario.Lane)
	}
	if !slices.Contains([]string{ExecutionDeterministic, ExecutionExternal}, scenario.ExecutionClass) {
		return fmt.Errorf("%s: unknown executionClass %q", prefix, scenario.ExecutionClass)
	}
	if (scenario.ExecutionClass == ExecutionExternal) != (scenario.Lane == LaneExternal) {
		return fmt.Errorf("%s: external executionClass and externally-provisioned lane must be assigned together", prefix)
	}
	if err := validateStatusEvidence(prefix, scenario); err != nil {
		return err
	}
	if scenario.Interface == InterfaceSSE {
		return validateSSE(prefix, scenario)
	}
	if scenario.SSE != nil {
		return fmt.Errorf("%s: SSE disposition is only valid for the sse interface", prefix)
	}
	return nil
}

func validateStatusEvidence(prefix string, scenario Scenario) error {
	for _, evidence := range scenario.Evidence {
		if strings.TrimSpace(evidence.Test) == "" || !strings.Contains(evidence.Test, "::Test") {
			return fmt.Errorf("%s: evidence must name a repository test as path::TestName", prefix)
		}
		if !slices.Contains([]string{InterfaceCLI, InterfaceREST, InterfaceMCP, InterfaceSSE}, evidence.Boundary) {
			return fmt.Errorf("%s: evidence boundary %q is not a customer boundary", prefix, evidence.Boundary)
		}
		if evidence.Boundary != scenario.Interface {
			return fmt.Errorf("%s: evidence boundary %q does not exercise the scenario interface", prefix, evidence.Boundary)
		}
	}
	switch scenario.Status {
	case StatusCovered:
		if len(scenario.Evidence) == 0 || scenario.Gap != "" || scenario.ReviewedReason != "" {
			return fmt.Errorf("%s: covered status requires named customer-boundary evidence and forbids gap or reviewedReason", prefix)
		}
	case StatusPartial:
		if len(scenario.Evidence) == 0 || strings.TrimSpace(scenario.Gap) == "" || scenario.ReviewedReason != "" {
			return fmt.Errorf("%s: partial status requires named evidence and a concrete gap", prefix)
		}
	case StatusMissing, StatusNotApplicable:
		if len(scenario.Evidence) != 0 || scenario.Gap != "" || strings.TrimSpace(scenario.ReviewedReason) == "" {
			return fmt.Errorf("%s: %s status requires a reviewedReason and forbids evidence or gap", prefix, scenario.Status)
		}
	}
	return nil
}

func validateSSE(prefix string, scenario Scenario) error {
	if scenario.SSE == nil {
		return fmt.Errorf("%s: SSE scenario requires an explicit disposition", prefix)
	}
	wantRequired, wantDisposition, wantScope := false, "", ""
	switch scenario.StableID {
	case sessionEventsStableID:
		wantRequired, wantDisposition, wantScope = true, SSERequired, sessionEventsScope
	case globalEventsStableID:
		wantDisposition, wantScope = SSEDeprecatedLaterRemoval, "deprecated global Factory Event compatibility stream"
	case responseEventsStableID:
		wantDisposition, wantScope = SSECurrentlyDeferred, "session Factory response-event stream"
	default:
		return fmt.Errorf("%s: unknown public SSE operation; add a reviewed required or non-required disposition", prefix)
	}
	if scenario.SSE.Required != wantRequired || scenario.SSE.Disposition != wantDisposition || scenario.SSE.Scope != wantScope {
		return fmt.Errorf("%s: SSE disposition must be required=%t disposition=%q scope=%q", prefix, wantRequired, wantDisposition, wantScope)
	}
	return nil
}
