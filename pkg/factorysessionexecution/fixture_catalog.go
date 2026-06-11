package factorysessionexecution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ContractFixtureCatalogRelativePath is the repository-relative path to the durable
// session contract fixture catalog consumed by FakeService and downstream cells.
const ContractFixtureCatalogRelativePath = "pkg/api/testdata/durable-session-contract-fixtures.json"

// Published fixture scenario IDs are stable identifiers downstream CLI, MCP, API,
// and website cells can import for deterministic Factory Session verification.
const (
	FixtureScenarioValidationFailure      = "missing-source"
	FixtureScenarioAsyncRunning           = "javascript-running-n-dispatch"
	FixtureScenarioSyncSuccess            = "petri-succeeded-one-dispatch"
	FixtureScenarioSyncTimeout            = "javascript-sync-timed-out"
	FixtureScenarioFailedRecoverable      = "javascript-interrupted-recoverable"
	FixtureScenarioDispatchInspection     = "petri-succeeded-one-dispatch"
	FixtureScenarioArtifactInspection     = "javascript-paused-two-dispatch"
	FixtureScenarioEventReconnect         = "javascript-running-n-dispatch"
	FixtureScenarioLifecycleControl       = "javascript-paused-two-dispatch"
	FixtureScenarioIdempotentReplay       = "idempotent-replay"
)

// FixtureScenarioPurpose names one reusable provider-fixture outcome category.
type FixtureScenarioPurpose string

const (
	FixturePurposeValidationFailure  FixtureScenarioPurpose = "validation-failure"
	FixturePurposeAsyncRunning       FixtureScenarioPurpose = "async-running"
	FixturePurposeSyncSuccess        FixtureScenarioPurpose = "sync-success"
	FixturePurposeSyncTimeout        FixtureScenarioPurpose = "sync-timeout"
	FixturePurposeFailedRecoverable  FixtureScenarioPurpose = "failed-recoverable-session"
	FixturePurposeDispatchInspection FixtureScenarioPurpose = "dispatch-inspection"
	FixturePurposeArtifactInspection FixtureScenarioPurpose = "artifact-inspection"
	FixturePurposeEventReconnect     FixtureScenarioPurpose = "event-reconnect"
	FixturePurposeLifecycleControl   FixtureScenarioPurpose = "lifecycle-control"
)

// PublishedFixtureScenario documents one canonical contract-backed scenario identity
// for downstream reuse.
type PublishedFixtureScenario struct {
	Purpose        FixtureScenarioPurpose
	ScenarioID     string
	RequestID      string
	SessionID      string
	LifecycleStatus LifecycleStatus
	ResultStatus   ResultStatus
	ProjectionHash string
}

// PublishedFixtureScenarios is the stable catalog of reusable Factory Session fixture
// identities keyed by outcome purpose.
var PublishedFixtureScenarios = []PublishedFixtureScenario{
	{
		Purpose:         FixturePurposeValidationFailure,
		ScenarioID:      FixtureScenarioValidationFailure,
		RequestID:       "req-missing-source-001",
		SessionID:       "dur-sess-missing-source-001",
		LifecycleStatus: LifecycleStatusFailed,
		ResultStatus:    ResultStatusUnavailable,
	},
	{
		Purpose:         FixturePurposeAsyncRunning,
		ScenarioID:      FixtureScenarioAsyncRunning,
		RequestID:       "req-js-run-n-001",
		SessionID:       "dur-sess-js-run-n-001",
		LifecycleStatus: LifecycleStatusRunning,
		ResultStatus:    ResultStatusPartial,
	},
	{
		Purpose:         FixturePurposeSyncSuccess,
		ScenarioID:      FixtureScenarioSyncSuccess,
		RequestID:       "req-petri-success-001",
		SessionID:       "dur-sess-petri-success-001",
		LifecycleStatus: LifecycleStatusSucceeded,
		ResultStatus:    ResultStatusFinal,
	},
	{
		Purpose:         FixturePurposeSyncTimeout,
		ScenarioID:      FixtureScenarioSyncTimeout,
		RequestID:       "req-js-timeout-001",
		SessionID:       "dur-sess-js-timeout-001",
		LifecycleStatus: LifecycleStatusRunning,
		ResultStatus:    ResultStatusNotReady,
	},
	{
		Purpose:         FixturePurposeFailedRecoverable,
		ScenarioID:      FixtureScenarioFailedRecoverable,
		RequestID:       "req-js-interrupted-001",
		SessionID:       "dur-sess-js-interrupted-001",
		LifecycleStatus: LifecycleStatusInterrupted,
		ResultStatus:    ResultStatusPartial,
	},
	{
		Purpose:         FixturePurposeDispatchInspection,
		ScenarioID:      FixtureScenarioDispatchInspection,
		RequestID:       "req-petri-success-001",
		SessionID:       "dur-sess-petri-success-001",
		LifecycleStatus: LifecycleStatusSucceeded,
		ResultStatus:    ResultStatusFinal,
	},
	{
		Purpose:         FixturePurposeArtifactInspection,
		ScenarioID:      FixtureScenarioArtifactInspection,
		RequestID:       "req-js-paused-001",
		SessionID:       "dur-sess-js-paused-001",
		LifecycleStatus: LifecycleStatusPaused,
		ResultStatus:    ResultStatusPartial,
	},
	{
		Purpose:         FixturePurposeEventReconnect,
		ScenarioID:      FixtureScenarioEventReconnect,
		RequestID:       "req-js-run-n-001",
		SessionID:       "dur-sess-js-run-n-001",
		LifecycleStatus: LifecycleStatusRunning,
		ResultStatus:    ResultStatusPartial,
	},
	{
		Purpose:         FixturePurposeLifecycleControl,
		ScenarioID:      FixtureScenarioLifecycleControl,
		RequestID:       "req-js-paused-001",
		SessionID:       "dur-sess-js-paused-001",
		LifecycleStatus: LifecycleStatusPaused,
		ResultStatus:    ResultStatusPartial,
	},
}

// FixtureScenarioIdentity is the resolved identity bundle for one loaded scenario.
type FixtureScenarioIdentity struct {
	ScenarioID      string
	RequestID       string
	SessionID       string
	LifecycleStatus LifecycleStatus
	ResultStatus    ResultStatus
	DispatchIDs     []string
	ArtifactIDs     []string
	EventIDs        []string
	ProjectionHash  string
}

// LoadFixtureScenarioIdentities loads the contract fixture catalog and returns one
// identity bundle per scenario keyed by scenario ID.
func LoadFixtureScenarioIdentities(path string) (map[string]FixtureScenarioIdentity, error) {
	scenarios, err := LoadFakeScenariosFromContractFixtures(path)
	if err != nil {
		return nil, err
	}
	identities := make(map[string]FixtureScenarioIdentity, len(scenarios))
	for _, scenario := range scenarios {
		identity, err := FixtureScenarioIdentityFromScenario(scenario)
		if err != nil {
			return nil, fmt.Errorf("identity for scenario %q: %w", scenario.ID, err)
		}
		identities[scenario.ID] = identity
	}
	return identities, nil
}

// FixtureScenarioIdentityFromScenario derives the stable identity bundle for one
// loaded fake scenario.
func FixtureScenarioIdentityFromScenario(scenario FakeScenario) (FixtureScenarioIdentity, error) {
	dispatchIDs := make([]string, 0, len(scenario.Dispatches))
	for _, dispatch := range scenario.Dispatches {
		if id := strings.TrimSpace(dispatch.ID); id != "" {
			dispatchIDs = append(dispatchIDs, id)
		}
	}
	sort.Strings(dispatchIDs)

	artifactIDs := make([]string, 0, len(scenario.Artifacts))
	for _, artifact := range scenario.Artifacts {
		if id := strings.TrimSpace(artifact.ID); id != "" {
			artifactIDs = append(artifactIDs, id)
		}
	}
	sort.Strings(artifactIDs)

	eventIDs, err := eventIDsFromFixtureEvents(scenario.Events)
	if err != nil {
		return FixtureScenarioIdentity{}, err
	}

	identity := FixtureScenarioIdentity{
		ScenarioID:      scenario.ID,
		RequestID:       scenario.RequestID,
		SessionID:       scenario.Session.SessionID,
		LifecycleStatus: scenario.Session.Status,
		ResultStatus:    scenario.Result.ResultStatus,
		DispatchIDs:     dispatchIDs,
		ArtifactIDs:     artifactIDs,
		EventIDs:        eventIDs,
	}
	hash, err := FixtureScenarioProjectionHash(identity, scenario.Result.PrimaryResult)
	if err != nil {
		return FixtureScenarioIdentity{}, err
	}
	identity.ProjectionHash = hash
	return identity, nil
}

// FixtureScenarioProjectionHash returns a stable digest for one scenario identity
// bundle and optional primary result payload.
func FixtureScenarioProjectionHash(identity FixtureScenarioIdentity, primaryResult json.RawMessage) (string, error) {
	document := map[string]any{
		"scenarioId":      identity.ScenarioID,
		"requestId":       identity.RequestID,
		"sessionId":       identity.SessionID,
		"lifecycleStatus": string(identity.LifecycleStatus),
		"resultStatus":    string(identity.ResultStatus),
		"dispatchIds":     append([]string(nil), identity.DispatchIDs...),
		"artifactIds":     append([]string(nil), identity.ArtifactIDs...),
		"eventIds":        append([]string(nil), identity.EventIDs...),
	}
	if len(primaryResult) > 0 {
		sum := sha256.Sum256(primaryResult)
		document["primaryResultHash"] = "sha256:" + hex.EncodeToString(sum[:])
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("marshal projection identity: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// HydratePublishedFixtureProjectionHashes fills ProjectionHash on each published
// catalog row from one loaded identity map.
func HydratePublishedFixtureProjectionHashes(identities map[string]FixtureScenarioIdentity) []PublishedFixtureScenario {
	hydrated := make([]PublishedFixtureScenario, len(PublishedFixtureScenarios))
	copy(hydrated, PublishedFixtureScenarios)
	for index := range hydrated {
		identity, ok := identities[hydrated[index].ScenarioID]
		if !ok {
			continue
		}
		hydrated[index].ProjectionHash = identity.ProjectionHash
	}
	return hydrated
}

func eventIDsFromFixtureEvents(events []json.RawMessage) ([]string, error) {
	if len(events) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(events))
	for _, raw := range events {
		var envelope struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return nil, fmt.Errorf("decode event id: %w", err)
		}
		if id := strings.TrimSpace(envelope.ID); id != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids, nil
}
