package fixtures

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
)

type (
	LifecycleStatus         = fse.LifecycleStatus
	ResultStatus            = fse.ResultStatus
	FakeScenario            = fse.FakeScenario
	ResultReadResult        = fse.ResultReadResult
	ListDispatchesResult    = fse.ListDispatchesResult
	DispatchDetail          = fse.DispatchDetail
	ListArtifactsResult     = fse.ListArtifactsResult
	ArtifactDetail          = fse.ArtifactDetail
	EventReadResult         = fse.EventReadResult
	LifecycleControlResult  = fse.LifecycleControlResult
	SyncStartResult         = fse.SyncStartResult
	LifecycleControlKind    = fse.LifecycleControlKind
	LifecycleControlOutcome = fse.LifecycleControlOutcome
	ValidationError         = fse.ValidationError
	ControlError            = fse.ControlError
)

const (
	LifecycleStatusFailed                  = fse.LifecycleStatusFailed
	LifecycleStatusRunning                 = fse.LifecycleStatusRunning
	LifecycleStatusSucceeded               = fse.LifecycleStatusSucceeded
	LifecycleStatusPaused                  = fse.LifecycleStatusPaused
	LifecycleStatusInterrupted             = fse.LifecycleStatusInterrupted
	ResultStatusUnavailable                = fse.ResultStatusUnavailable
	ResultStatusPartial                    = fse.ResultStatusPartial
	ResultStatusFinal                      = fse.ResultStatusFinal
	ResultStatusNotReady                   = fse.ResultStatusNotReady
	LifecycleControlOutcomeInvalidState    = fse.LifecycleControlOutcomeInvalidState
	LifecycleControlOutcomeTerminalSession = fse.LifecycleControlOutcomeTerminalSession
	LifecycleControlOutcomeConflict        = fse.LifecycleControlOutcomeConflict
)

// ContractFixtureCatalogRelativePath is the repository-relative path to the durable
// session contract fixture catalog consumed by FakeService and downstream cells.
const ContractFixtureCatalogRelativePath = "pkg/transports/http/testdata/durable-session-contract-fixtures.json"

// Published fixture scenario IDs are stable identifiers downstream CLI, MCP, API,
// and website cells can import for deterministic Factory Session verification.
const (
	FixtureScenarioValidationFailure  = "missing-source"
	FixtureScenarioAsyncRunning       = "javascript-running-n-dispatch"
	FixtureScenarioSyncSuccess        = "petri-succeeded-one-dispatch"
	FixtureScenarioSyncTimeout        = "javascript-sync-timed-out"
	FixtureScenarioFailedRecoverable  = "javascript-interrupted-recoverable"
	FixtureScenarioDispatchInspection = "petri-succeeded-one-dispatch"
	FixtureScenarioArtifactInspection = "javascript-paused-two-dispatch"
	FixtureScenarioEventReconnect     = "javascript-running-n-dispatch"
	FixtureScenarioLifecycleControl   = "javascript-paused-two-dispatch"
	FixtureScenarioIdempotentReplay   = "idempotent-replay"
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
	Purpose         FixtureScenarioPurpose
	ScenarioID      string
	RequestID       string
	SessionID       string
	LifecycleStatus LifecycleStatus
	ResultStatus    ResultStatus
	ProjectionHash  string
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
func LoadFixtureScenarioIdentities(
	path string,
	files fileeffects.ContractFixtureReader,
) (map[string]FixtureScenarioIdentity, error) {
	scenarios, err := fse.LoadFakeScenariosFromContractFixtures(path, files)
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

// ProjectedResultReadHash returns a stable digest for one projected result read so
// downstream cells can assert fixture-backed result behavior without ad hoc comparisons.
func ProjectedResultReadHash(result ResultReadResult) (string, error) {
	document := map[string]any{
		"sessionId":        result.SessionID,
		"resultStatus":     string(result.ResultStatus),
		"sessionStatus":    string(result.SessionStatus),
		"mode":             string(result.Mode),
		"includeArtifacts": result.IncludeArtifacts,
	}
	if len(result.PrimaryResult) > 0 {
		sum := sha256.Sum256(result.PrimaryResult)
		document["primaryResultHash"] = "sha256:" + hex.EncodeToString(sum[:])
	}
	if result.Availability != nil {
		document["availabilityReason"] = result.Availability.Reason
	}
	if len(result.ArtifactIDs) > 0 {
		document["artifactIds"] = append([]string(nil), result.ArtifactIDs...)
	}
	if len(result.ArtifactRefs) > 0 {
		refs := make([]string, 0, len(result.ArtifactRefs))
		for _, ref := range result.ArtifactRefs {
			if id := strings.TrimSpace(ref.ID); id != "" {
				refs = append(refs, id)
			}
		}
		sort.Strings(refs)
		document["artifactRefIds"] = refs
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("marshal projected result read: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// ListDispatchesResultHash returns a stable digest for one dispatch list read.
func ListDispatchesResultHash(result ListDispatchesResult) (string, error) {
	dispatches := make([]map[string]any, 0, len(result.Dispatches))
	for _, dispatch := range result.Dispatches {
		row := map[string]any{
			"id":           dispatch.ID,
			"status":       string(dispatch.Status),
			"dispatchKind": dispatch.DispatchKind,
			"phase":        dispatch.Phase,
			"label":        dispatch.Label,
			"attempt":      dispatch.Attempt,
		}
		if len(dispatch.OutputArtifactIDs) > 0 {
			ids := append([]string(nil), dispatch.OutputArtifactIDs...)
			sort.Strings(ids)
			row["outputArtifactIds"] = ids
		}
		if len(dispatch.ProviderSessionRefs) > 0 {
			refs := make([]string, 0, len(dispatch.ProviderSessionRefs))
			for _, ref := range dispatch.ProviderSessionRefs {
				refs = append(refs, fmt.Sprintf("%s:%s:%s", ref.Provider, ref.Kind, ref.ID))
			}
			sort.Strings(refs)
			row["providerSessionRefs"] = refs
		}
		dispatches = append(dispatches, row)
	}
	document := map[string]any{
		"sessionId":  result.SessionID,
		"dispatches": dispatches,
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("marshal dispatch list read: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// DispatchDetailHash returns a stable digest for one dispatch detail read.
func DispatchDetailHash(detail DispatchDetail) (string, error) {
	document := map[string]any{
		"sessionId":        detail.SessionID,
		"id":               detail.ID,
		"status":           string(detail.Status),
		"dispatchKind":     detail.DispatchKind,
		"orchestratorKind": detail.OrchestratorKind,
		"phase":            detail.Phase,
		"label":            detail.Label,
		"attempt":          detail.Attempt,
	}
	if len(detail.ArtifactIDs) > 0 {
		ids := append([]string(nil), detail.ArtifactIDs...)
		sort.Strings(ids)
		document["artifactIds"] = ids
	}
	if detail.Petri != nil {
		document["petriTransitionId"] = detail.Petri.TransitionID
	}
	if detail.JavaScript != nil {
		document["javascriptTaskKind"] = detail.JavaScript.TaskKind
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("marshal dispatch detail read: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// ListArtifactsResultHash returns a stable digest for one artifact list read.
func ListArtifactsResultHash(result ListArtifactsResult) (string, error) {
	artifacts := make([]map[string]any, 0, len(result.Artifacts))
	for _, artifact := range result.Artifacts {
		row := map[string]any{
			"id":          artifact.ID,
			"kind":        artifact.Kind,
			"visibility":  artifact.Visibility,
			"label":       artifact.Label,
			"contentHash": artifact.ContentHash,
			"sizeBytes":   artifact.SizeBytes,
			"dispatchId":  artifact.DispatchID,
			"auditMode":   artifact.AuditMode,
		}
		if artifact.RetrievalRef != nil {
			row["retrievalHref"] = artifact.RetrievalRef.Href
		}
		artifacts = append(artifacts, row)
	}
	document := map[string]any{
		"sessionId": result.SessionID,
		"artifacts": artifacts,
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("marshal artifact list read: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// ArtifactDetailHash returns a stable digest for one artifact detail read.
func ArtifactDetailHash(detail ArtifactDetail) (string, error) {
	document := map[string]any{
		"sessionId":   detail.SessionID,
		"id":          detail.ID,
		"kind":        detail.Kind,
		"visibility":  detail.Visibility,
		"label":       detail.Label,
		"contentHash": detail.ContentHash,
		"sizeBytes":   detail.SizeBytes,
		"dispatchId":  detail.DispatchID,
		"auditMode":   detail.AuditMode,
	}
	if detail.ContentRef != nil {
		document["contentHref"] = detail.ContentRef.Href
	}
	if len(detail.Content) > 0 {
		sum := sha256.Sum256(detail.Content)
		document["contentPayloadHash"] = "sha256:" + hex.EncodeToString(sum[:])
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("marshal artifact detail read: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// EventReadResultHash returns a stable digest for one event read preserving event order.
func EventReadResultHash(result EventReadResult) (string, error) {
	eventIDs, err := orderedEventIDsFromFixtureEvents(result.Events)
	if err != nil {
		return "", err
	}
	document := map[string]any{
		"sessionId": result.SessionID,
		"eventIds":  eventIDs,
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("marshal event read: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// LifecycleControlResultHash returns a stable digest for one lifecycle control outcome
// so downstream cells can assert fixture-backed control behavior without ad hoc comparisons.
func LifecycleControlResultHash(result LifecycleControlResult) (string, error) {
	document := map[string]any{
		"sessionId": result.SessionID,
		"operation": string(result.Operation),
		"outcome":   string(result.Outcome),
		"status":    string(result.Status),
	}
	if dispatchID := strings.TrimSpace(result.DispatchID); dispatchID != "" {
		document["dispatchId"] = dispatchID
	}
	if retryDispatchID := strings.TrimSpace(result.RetryDispatchID); retryDispatchID != "" {
		document["retryDispatchId"] = retryDispatchID
	}
	if result.Links.Session != "" {
		document["links"] = map[string]any{
			"session":    result.Links.Session,
			"results":    result.Links.Results,
			"dispatches": result.Links.Dispatches,
			"artifacts":  result.Links.Artifacts,
			"events":     result.Links.Events,
		}
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("marshal lifecycle control result: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// SyncStartResultHash returns a stable digest for one sync start outcome.
func SyncStartResultHash(result SyncStartResult) (string, error) {
	document := map[string]any{
		"sessionId":   result.SessionID,
		"status":      result.Status,
		"syncOutcome": string(result.SyncOutcome),
		"timedOut":    result.TimedOut,
	}
	if len(result.Result) > 0 {
		sum := sha256.Sum256(result.Result)
		document["resultHash"] = "sha256:" + hex.EncodeToString(sum[:])
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("marshal sync start result: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func eventIDsFromFixtureEvents(events []json.RawMessage) ([]string, error) {
	ids, err := orderedEventIDsFromFixtureEvents(events)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	return sorted, nil
}

func orderedEventIDsFromFixtureEvents(events []json.RawMessage) ([]string, error) {
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
	return ids, nil
}

// TypedFailureKind names one stable fixture-provider failure category downstream
// API, CLI, MCP, and UI cells can assert against without parsing free-form text.
type TypedFailureKind string

const (
	TypedFailureUnknownScenario          TypedFailureKind = "UNKNOWN_SCENARIO"
	TypedFailureMalformedRequest         TypedFailureKind = "MALFORMED_REQUEST"
	TypedFailureSessionNotFound          TypedFailureKind = "SESSION_NOT_FOUND"
	TypedFailureDispatchNotFound         TypedFailureKind = "DISPATCH_NOT_FOUND"
	TypedFailureArtifactNotFound         TypedFailureKind = "ARTIFACT_NOT_FOUND"
	TypedFailureReconnectCursorNotFound  TypedFailureKind = "RECONNECT_CURSOR_NOT_FOUND"
	TypedFailureExecutionRequestConflict TypedFailureKind = "EXECUTION_REQUEST_ID_CONFLICT"
	TypedFailureLifecycleConflict        TypedFailureKind = "LIFECYCLE_CONFLICT"
	TypedFailureLifecycleInvalidState    TypedFailureKind = "LIFECYCLE_INVALID_STATE"
	TypedFailureLifecycleTerminalSession TypedFailureKind = "LIFECYCLE_TERMINAL_SESSION"
	TypedFailureResumeMissingCheckpoint  TypedFailureKind = "RESUME_MISSING_CHECKPOINT"
	TypedFailureResumeInvalidState       TypedFailureKind = "RESUME_INVALID_STATE"
	TypedFailureResumeCorruptedState     TypedFailureKind = "RESUME_CORRUPTED_PERSISTENCE"
)

// TypedFailureIdentity captures structured context for one fixture-provider failure.
type TypedFailureIdentity struct {
	Kind      TypedFailureKind
	Field     string
	Operation LifecycleControlKind
	Outcome   LifecycleControlOutcome
	Status    LifecycleStatus
}

// TypedFailureIdentityFromError maps one service error to a stable typed identity.
func TypedFailureIdentityFromError(err error) (TypedFailureIdentity, bool) {
	if err == nil {
		return TypedFailureIdentity{}, false
	}

	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		kind := TypedFailureMalformedRequest
		if validationErr.Field == "requestId" && strings.Contains(validationErr.Message, "unknown fake scenario") {
			kind = TypedFailureUnknownScenario
		}
		return TypedFailureIdentity{
			Kind:  kind,
			Field: validationErr.Field,
		}, true
	}

	if identity, ok := typedFailureIdentityFromControlError(err); ok {
		return identity, true
	}
	if identity, ok := typedFailureIdentityFromResumeError(err); ok {
		return identity, true
	}
	return typedFailureIdentityFromSentinelError(err)
}

func typedFailureIdentityFromResumeError(err error) (TypedFailureIdentity, bool) {
	var resumeErr *fse.ResumeError
	if !errors.As(err, &resumeErr) {
		return TypedFailureIdentity{}, false
	}
	kind, ok := typedFailureKindForResumeOutcome(resumeErr.Outcome)
	if !ok {
		return TypedFailureIdentity{}, false
	}
	return TypedFailureIdentity{
		Kind:   kind,
		Field:  resumeErr.Field,
		Status: resumeErr.Status,
	}, true
}

func typedFailureKindForResumeOutcome(outcome fse.ResumeOutcome) (TypedFailureKind, bool) {
	switch outcome {
	case fse.ResumeOutcomeMissingCheckpoint:
		return TypedFailureResumeMissingCheckpoint, true
	case fse.ResumeOutcomeInvalidState:
		return TypedFailureResumeInvalidState, true
	case fse.ResumeOutcomeCorruptedPersistence:
		return TypedFailureResumeCorruptedState, true
	default:
		return "", false
	}
}

func typedFailureIdentityFromControlError(err error) (TypedFailureIdentity, bool) {
	var controlErr *ControlError
	if !errors.As(err, &controlErr) {
		return TypedFailureIdentity{}, false
	}
	kind, ok := typedFailureKindForControlOutcome(controlErr.Outcome)
	if !ok {
		return TypedFailureIdentity{}, false
	}
	return TypedFailureIdentity{
		Kind:      kind,
		Operation: controlErr.Operation,
		Outcome:   controlErr.Outcome,
		Status:    controlErr.Status,
	}, true
}

func typedFailureKindForControlOutcome(outcome LifecycleControlOutcome) (TypedFailureKind, bool) {
	switch outcome {
	case LifecycleControlOutcomeInvalidState:
		return TypedFailureLifecycleInvalidState, true
	case LifecycleControlOutcomeTerminalSession:
		return TypedFailureLifecycleTerminalSession, true
	case LifecycleControlOutcomeConflict:
		return TypedFailureLifecycleConflict, true
	default:
		return "", false
	}
}

func typedFailureIdentityFromSentinelError(err error) (TypedFailureIdentity, bool) {
	switch {
	case errors.Is(err, fse.ErrSessionNotFound):
		return TypedFailureIdentity{Kind: TypedFailureSessionNotFound}, true
	case errors.Is(err, fse.ErrDispatchNotFound):
		return TypedFailureIdentity{Kind: TypedFailureDispatchNotFound}, true
	case errors.Is(err, fse.ErrArtifactNotFound):
		return TypedFailureIdentity{Kind: TypedFailureArtifactNotFound}, true
	case errors.Is(err, fse.ErrReconnectCursorNotFound):
		return TypedFailureIdentity{Kind: TypedFailureReconnectCursorNotFound}, true
	case errors.Is(err, fse.ErrExecutionRequestIDConflict):
		return TypedFailureIdentity{Kind: TypedFailureExecutionRequestConflict}, true
	default:
		return TypedFailureIdentity{}, false
	}
}

// ForbiddenFixtureVocabularyTerms are resource names that must not appear in public
// fixture fields, error names, or downstream-facing test descriptions.
func ForbiddenFixtureVocabularyTerms() []string {
	return []string{"DynamicWorkflowRun", "workflow run"}
}

// TypedFailureHash returns a stable digest for one typed failure identity.
func TypedFailureHash(identity TypedFailureIdentity) (string, error) {
	document := map[string]any{
		"kind": string(identity.Kind),
	}
	if field := strings.TrimSpace(identity.Field); field != "" {
		document["field"] = field
	}
	if identity.Operation != "" {
		document["operation"] = string(identity.Operation)
	}
	if identity.Outcome != "" {
		document["outcome"] = string(identity.Outcome)
	}
	if identity.Status != "" {
		document["status"] = string(identity.Status)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("marshal typed failure identity: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
