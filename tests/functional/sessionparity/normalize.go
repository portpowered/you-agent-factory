package sessionparity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// NormalizationError identifies a captured customer-boundary value that cannot
// represent the parity contract. It preserves the interface and stable field
// path instead of manufacturing parity from partial observations.
type NormalizationError struct {
	Interface string
	Field     string
	Reason    string
}

func (e *NormalizationError) Error() string {
	return fmt.Sprintf("%s observation %s: %s", e.Interface, e.Field, e.Reason)
}

// A captured observation is a test-only bundle of the separate customer reads
// needed to inspect one durable Factory Session. REST and CLI JSON members
// contain direct response/output bodies; the REST events member is the raw SSE
// body encoded as a JSON string. MCP members contain complete JSON-RPC
// tools/call responses.
type capturedObservation struct {
	Session    json.RawMessage `json:"session"`
	Dispatches json.RawMessage `json:"dispatches"`
	Artifacts  json.RawMessage `json:"artifacts"`
	Result     json.RawMessage `json:"result"`
	Events     json.RawMessage `json:"events"`
}

type rawSession struct {
	SessionID           *string      `json:"sessionId"`
	Status              *string      `json:"status"`
	Phase               *string      `json:"phase"`
	SourceHash          *string      `json:"sourceHash"`
	ResolvedSource      *rawSource   `json:"resolvedSource"`
	RequestedPolicy     *rawPolicy   `json:"requestedPolicy"`
	EffectivePolicy     *rawPolicy   `json:"effectivePolicy"`
	EffectivePolicyHash *string      `json:"effectivePolicyHash"`
	Progress            *rawProgress `json:"progress"`
	FailureDetail       *rawFailure  `json:"failureDetail"`
}

type rawPolicy struct {
	PolicyHash *string `json:"policyHash"`
}

type rawSource struct {
	SourceHash *string `json:"sourceHash"`
}

type rawProgress struct {
	TotalDispatches     *int `json:"totalDispatches"`
	CompletedDispatches *int `json:"completedDispatches"`
	FailedDispatches    *int `json:"failedDispatches"`
	InFlightDispatches  *int `json:"inFlightDispatches"`
}

type rawDispatchList struct {
	SessionID  *string        `json:"sessionId"`
	Dispatches *[]rawDispatch `json:"dispatches"`
}

type rawDispatch struct {
	ID            *string     `json:"id"`
	Status        *string     `json:"status"`
	DispatchKind  *string     `json:"dispatchKind"`
	FailureDetail *rawFailure `json:"failureDetail"`
}

type rawArtifactList struct {
	SessionID *string        `json:"sessionId"`
	Artifacts *[]rawArtifact `json:"artifacts"`
}

type rawArtifact struct {
	ID   *string `json:"id"`
	Kind *string `json:"kind"`
}

type rawResult struct {
	SessionID     *string         `json:"sessionId"`
	ResultStatus  *string         `json:"resultStatus"`
	PrimaryResult json.RawMessage `json:"primaryResult"`
	FailureDetail *rawFailure     `json:"failureDetail"`
}

type rawFailure struct {
	Reason  *string `json:"reason"`
	Message *string `json:"message"`
}

type rawEventResult struct {
	SessionID *string     `json:"sessionId"`
	Events    *[]rawEvent `json:"events"`
}

type rawEvent struct {
	ID      *string          `json:"id"`
	Type    *string          `json:"type"`
	Context *rawEventContext `json:"context"`
}

type rawEventContext struct {
	Sequence  *int64  `json:"sequence"`
	SessionID *string `json:"sessionId"`
}

// NormalizeREST normalizes a bundle of captured REST response bodies. HTTP
// status, headers, request details, and any other bundle metadata are ignored.
func NormalizeREST(observation []byte) (Projection, error) {
	return normalizeCaptured("REST", observation, directCustomerValue, restSSEEvents)
}

// NormalizeCLIJSON normalizes a bundle of captured --json command outputs.
// Human rendering and command diagnostics are outside the bundle members.
func NormalizeCLIJSON(observation []byte) (Projection, error) {
	return normalizeCaptured("CLI JSON", observation, directCustomerValue, directEvents)
}

// NormalizeMCP normalizes a bundle of captured MCP tool responses. JSON-RPC
// envelopes and request-correlation metadata surrounding each result are ignored.
func NormalizeMCP(observation []byte) (Projection, error) {
	return normalizeCaptured("MCP", observation, mcpCustomerValue, mcpEvents)
}

type valueDecoder func(string, json.RawMessage) (json.RawMessage, error)
type eventDecoder func(string, json.RawMessage) (rawEventResult, error)

func normalizeCaptured(name string, observation []byte, decode valueDecoder, decodeEvents eventDecoder) (Projection, error) {
	var bundle capturedObservation
	if err := json.Unmarshal(observation, &bundle); err != nil {
		return Projection{}, normalizationError(name, "$", "must be a JSON object")
	}
	for _, member := range []struct {
		field string
		raw   json.RawMessage
	}{
		{"session", bundle.Session}, {"dispatches", bundle.Dispatches}, {"artifacts", bundle.Artifacts},
		{"result", bundle.Result}, {"events", bundle.Events},
	} {
		if missingJSON(member.raw) {
			return Projection{}, normalizationError(name, member.field, "is required")
		}
	}

	sessionJSON, err := decode("session", bundle.Session)
	if err != nil {
		return Projection{}, normalizationError(name, "session", err.Error())
	}
	dispatchJSON, err := decode("dispatches", bundle.Dispatches)
	if err != nil {
		return Projection{}, normalizationError(name, "dispatches", err.Error())
	}
	artifactJSON, err := decode("artifacts", bundle.Artifacts)
	if err != nil {
		return Projection{}, normalizationError(name, "artifacts", err.Error())
	}
	resultJSON, err := decode("result", bundle.Result)
	if err != nil {
		return Projection{}, normalizationError(name, "result", err.Error())
	}
	events, err := decodeEvents(name, bundle.Events)
	if err != nil {
		return Projection{}, err
	}

	return projectCapturedValues(name, sessionJSON, dispatchJSON, artifactJSON, resultJSON, events)
}

func directCustomerValue(_ string, raw json.RawMessage) (json.RawMessage, error) {
	return raw, nil
}

func mcpCustomerValue(_ string, raw json.RawMessage) (json.RawMessage, error) {
	var response struct {
		Result *struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			IsError *bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &response); err != nil || response.Result == nil {
		return nil, fmt.Errorf("result is required")
	}
	if response.Result.IsError == nil || *response.Result.IsError {
		return nil, fmt.Errorf("result must be a successful tools/call response")
	}
	if len(response.Result.Content) != 1 || response.Result.Content[0].Type != "text" || response.Result.Content[0].Text == "" {
		return nil, fmt.Errorf("result.content[0].text is required")
	}
	var toolResponse struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal([]byte(response.Result.Content[0].Text), &toolResponse); err != nil || missingJSON(toolResponse.Result) {
		return nil, fmt.Errorf("result.content[0].text must contain a tool result")
	}
	return toolResponse.Result, nil
}

func directEvents(name string, raw json.RawMessage) (rawEventResult, error) {
	var events []rawEvent
	if err := json.Unmarshal(raw, &events); err != nil {
		return rawEventResult{}, normalizationError(name, "eventCursors", "has incompatible value")
	}
	return rawEventResult{Events: &events}, nil
}

func restSSEEvents(name string, raw json.RawMessage) (rawEventResult, error) {
	var body string
	if err := json.Unmarshal(raw, &body); err != nil {
		return rawEventResult{}, normalizationError(name, "eventCursors", "must contain a captured SSE response body")
	}
	events, err := decodeSSEEvents(body)
	if err != nil {
		return rawEventResult{}, normalizationError(name, "eventCursors", err.Error())
	}
	return rawEventResult{Events: &events}, nil
}

func decodeSSEEvents(body string) ([]rawEvent, error) {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	events := make([]rawEvent, 0)
	dataLines := make([]string, 0, 1)
	flush := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		var event rawEvent
		if err := json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &event); err != nil {
			return fmt.Errorf("contains incompatible SSE event data")
		}
		events = append(events, event)
		dataLines = dataLines[:0]
		return nil
	}
	for _, line := range lines {
		if line == "" {
			if err := flush(); err != nil {
				return nil, err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			if strings.HasPrefix(data, " ") {
				data = strings.TrimPrefix(data, " ")
			}
			dataLines = append(dataLines, data)
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return events, nil
}

func mcpEvents(name string, raw json.RawMessage) (rawEventResult, error) {
	value, err := mcpCustomerValue("events", raw)
	if err != nil {
		return rawEventResult{}, normalizationError(name, "events", err.Error())
	}
	var result rawEventResult
	if err := json.Unmarshal(value, &result); err != nil {
		return rawEventResult{}, normalizationError(name, "eventCursors", "has incompatible value")
	}
	return result, nil
}

func projectCapturedValues(name string, sessionJSON, dispatchJSON, artifactJSON, resultJSON json.RawMessage, events rawEventResult) (Projection, error) {
	var session rawSession
	var dispatches rawDispatchList
	var artifacts rawArtifactList
	var result rawResult
	for _, input := range []struct {
		field string
		raw   json.RawMessage
		into  any
	}{{"session", sessionJSON, &session}, {"dispatches", dispatchJSON, &dispatches}, {"artifacts", artifactJSON, &artifacts}, {"result", resultJSON, &result}} {
		if err := json.Unmarshal(input.raw, input.into); err != nil {
			return Projection{}, normalizationError(name, input.field, "has incompatible value")
		}
	}

	projection, err := baseProjection(session)
	if err != nil {
		return Projection{}, normalizationError(name, err.Field, err.Reason)
	}
	if err := addDispatches(&projection, dispatches); err != nil {
		return Projection{}, normalizationError(name, err.Field, err.Reason)
	}
	if err := addArtifacts(&projection, artifacts); err != nil {
		return Projection{}, normalizationError(name, err.Field, err.Reason)
	}
	if err := addResult(&projection, result); err != nil {
		return Projection{}, normalizationError(name, err.Field, err.Reason)
	}
	if err := addEvents(&projection, events); err != nil {
		return Projection{}, normalizationError(name, err.Field, err.Reason)
	}
	if err := validateProjection(projection); err != nil {
		return Projection{}, normalizationError(name, err.Field, err.Reason)
	}
	return projection, nil
}

func baseProjection(session rawSession) (Projection, *NormalizationError) {
	if emptyString(session.SessionID) {
		return Projection{}, requiredFact("identity.sessionId")
	}
	if emptyString(session.Status) {
		return Projection{}, requiredFact("lifecycle.status")
	}
	sourceHash, hashErr := equivalentHash(
		"hashes.sourceHash", session.SourceHash, sourceHashFromSource(session.ResolvedSource),
	)
	if hashErr != nil {
		return Projection{}, hashErr
	}
	if sourceHash == nil {
		return Projection{}, requiredFact("hashes.sourceHash")
	}
	if session.Progress == nil {
		return Projection{}, requiredFact("progress")
	}
	progress, progressErr := requiredProgress(*session.Progress)
	if progressErr != nil {
		return Projection{}, progressErr
	}
	effectiveHash, effectiveHashErr := equivalentHash(
		"hashes.effectivePolicyHash", session.EffectivePolicyHash, policyHashFromPolicy(session.EffectivePolicy),
	)
	if effectiveHashErr != nil {
		return Projection{}, effectiveHashErr
	}
	projection := Projection{
		Identity:  FactorySessionIdentity{SessionID: *session.SessionID},
		Lifecycle: LifecycleFacts{Status: *session.Status, Phase: session.Phase},
		Hashes:    HashFacts{SourceHash: *sourceHash, EffectivePolicyHash: effectiveHash},
		Progress:  progress, Dispatches: []DispatchFact{}, Artifacts: []ArtifactFact{},
		Results: []ResultFact{}, Failures: []FailureFact{}, EventCursors: []FactoryEventCursor{},
	}
	if session.RequestedPolicy != nil {
		projection.Hashes.RequestedPolicyHash = session.RequestedPolicy.PolicyHash
	}
	if session.FailureDetail != nil {
		failure, failureErr := mapFailure(*session.SessionID+":failure", *session.SessionID, 1, nil, session.FailureDetail)
		if failureErr != nil {
			return Projection{}, failureErr
		}
		projection.Failures = append(projection.Failures, failure)
	}
	return projection, nil
}

func requiredProgress(progress rawProgress) (ProgressFacts, *NormalizationError) {
	for _, fact := range []struct {
		field string
		value *int
	}{
		{"progress.totalDispatches", progress.TotalDispatches},
		{"progress.completedDispatches", progress.CompletedDispatches},
		{"progress.failedDispatches", progress.FailedDispatches},
		{"progress.inFlightDispatches", progress.InFlightDispatches},
	} {
		if fact.value == nil {
			return ProgressFacts{}, requiredFact(fact.field)
		}
	}
	return ProgressFacts{
		TotalDispatches: *progress.TotalDispatches, CompletedDispatches: *progress.CompletedDispatches,
		FailedDispatches: *progress.FailedDispatches, InFlightDispatches: *progress.InFlightDispatches,
	}, nil
}

func addDispatches(projection *Projection, list rawDispatchList) *NormalizationError {
	if emptyString(list.SessionID) || *list.SessionID != projection.Identity.SessionID {
		return &NormalizationError{Field: "dispatches.sessionId", Reason: "must match identity.sessionId"}
	}
	if list.Dispatches == nil {
		return requiredFact("dispatches")
	}
	for index, row := range *list.Dispatches {
		path := fmt.Sprintf("dispatches[%d]", index)
		if emptyString(row.ID) || emptyString(row.Status) || emptyString(row.DispatchKind) {
			return &NormalizationError{Field: path, Reason: "must contain id, status, and dispatchKind"}
		}
		projection.Dispatches = append(projection.Dispatches, DispatchFact{
			SessionID: *list.SessionID, ID: *row.ID, Order: index + 1, Status: *row.Status, Kind: *row.DispatchKind,
		})
		if row.FailureDetail != nil {
			failure, err := mapFailure(*row.ID+":failure", *list.SessionID, len(projection.Failures)+1, row.ID, row.FailureDetail)
			if err != nil {
				return err
			}
			projection.Failures = append(projection.Failures, failure)
		}
	}
	return nil
}

func addArtifacts(projection *Projection, list rawArtifactList) *NormalizationError {
	if emptyString(list.SessionID) || *list.SessionID != projection.Identity.SessionID {
		return &NormalizationError{Field: "artifacts.sessionId", Reason: "must match identity.sessionId"}
	}
	if list.Artifacts == nil {
		return requiredFact("artifacts")
	}
	for index, row := range *list.Artifacts {
		if emptyString(row.ID) || emptyString(row.Kind) {
			return &NormalizationError{Field: fmt.Sprintf("artifacts[%d]", index), Reason: "must contain id and kind"}
		}
		projection.Artifacts = append(projection.Artifacts, ArtifactFact{
			SessionID: *list.SessionID, ID: *row.ID, Order: index + 1, Kind: *row.Kind,
		})
	}
	return nil
}

func addResult(projection *Projection, result rawResult) *NormalizationError {
	if emptyString(result.SessionID) || *result.SessionID != projection.Identity.SessionID {
		return &NormalizationError{Field: "results.sessionId", Reason: "must match identity.sessionId"}
	}
	if emptyString(result.ResultStatus) {
		return requiredFact("results.status")
	}
	if !missingJSON(result.PrimaryResult) {
		value, err := compactJSON(result.PrimaryResult)
		if err != nil {
			return &NormalizationError{Field: "results[0].value", Reason: "has incompatible value"}
		}
		projection.Results = append(projection.Results, ResultFact{
			SessionID: *result.SessionID, ID: *result.SessionID + ":result", Order: 1,
			Status: *result.ResultStatus, Value: value,
		})
	}
	if result.FailureDetail != nil {
		failure, err := mapFailure(
			*result.SessionID+":result-failure", *result.SessionID, len(projection.Failures)+1, nil, result.FailureDetail,
		)
		if err != nil {
			return err
		}
		projection.Failures = append(projection.Failures, failure)
	}
	return nil
}

func addEvents(projection *Projection, result rawEventResult) *NormalizationError {
	if result.SessionID != nil && *result.SessionID != projection.Identity.SessionID {
		return &NormalizationError{Field: "eventCursors.sessionId", Reason: "must match identity.sessionId"}
	}
	if result.Events == nil {
		return requiredFact("eventCursors")
	}
	for index, event := range *result.Events {
		if emptyString(event.ID) || emptyString(event.Type) || event.Context == nil || event.Context.Sequence == nil ||
			emptyString(event.Context.SessionID) {
			return &NormalizationError{Field: fmt.Sprintf("eventCursors[%d]", index), Reason: "must contain id, type, context.sequence, and context.sessionId"}
		}
		projection.EventCursors = append(projection.EventCursors, FactoryEventCursor{
			SessionID: *event.Context.SessionID, Cursor: *event.ID, Sequence: *event.Context.Sequence, EventType: *event.Type,
		})
	}
	return nil
}

func mapFailure(id, sessionID string, order int, dispatchID *string, detail *rawFailure) (FailureFact, *NormalizationError) {
	if detail == nil || emptyString(detail.Reason) || emptyString(detail.Message) {
		return FailureFact{}, &NormalizationError{Field: fmt.Sprintf("failures[%d]", order-1), Reason: "must contain reason and message"}
	}
	return FailureFact{SessionID: sessionID, ID: id, Order: order, Code: *detail.Reason, Message: *detail.Message, DispatchID: dispatchID}, nil
}

func compactJSON(raw json.RawMessage) (string, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(value)
	return string(canonical), err
}

func sourceHashFromSource(source *rawSource) *string {
	if source == nil {
		return nil
	}
	return source.SourceHash
}

func policyHashFromPolicy(policy *rawPolicy) *string {
	if policy == nil {
		return nil
	}
	return policy.PolicyHash
}

func equivalentHash(field string, first, second *string) (*string, *NormalizationError) {
	if !emptyString(first) && !emptyString(second) && *first != *second {
		return nil, &NormalizationError{Field: field, Reason: "has conflicting customer values"}
	}
	if !emptyString(first) {
		return first, nil
	}
	if !emptyString(second) {
		return second, nil
	}
	return nil, nil
}

func missingJSON(raw json.RawMessage) bool {
	return len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func emptyString(value *string) bool {
	return value == nil || *value == ""
}

func requiredFact(field string) *NormalizationError {
	return &NormalizationError{Field: field, Reason: "is required"}
}

func normalizationError(name, field, reason string) *NormalizationError {
	return &NormalizationError{Interface: name, Field: field, Reason: reason}
}

func validateProjection(projection Projection) *NormalizationError {
	if err := validateDispatches(projection.Identity.SessionID, projection.Dispatches); err != nil {
		return err
	}
	if err := validateArtifacts(projection.Identity.SessionID, projection.Artifacts); err != nil {
		return err
	}
	if err := validateResults(projection.Identity.SessionID, projection.Results); err != nil {
		return err
	}
	if err := validateFailures(projection.Identity.SessionID, projection.Failures); err != nil {
		return err
	}
	return validateEventCursors(projection.Identity.SessionID, projection.EventCursors)
}

func validateDispatches(sessionID string, facts []DispatchFact) *NormalizationError {
	for index, fact := range facts {
		if fact.SessionID != sessionID || fact.ID == "" || fact.Order != index+1 || fact.Status == "" || fact.Kind == "" {
			return &NormalizationError{Field: fmt.Sprintf("dispatches[%d]", index), Reason: "must retain a correlated, ordered dispatch fact"}
		}
	}
	return nil
}

func validateArtifacts(sessionID string, facts []ArtifactFact) *NormalizationError {
	for index, fact := range facts {
		if fact.SessionID != sessionID || fact.ID == "" || fact.Order != index+1 || fact.Kind == "" {
			return &NormalizationError{Field: fmt.Sprintf("artifacts[%d]", index), Reason: "must retain a correlated, ordered artifact fact"}
		}
	}
	return nil
}

func validateResults(sessionID string, facts []ResultFact) *NormalizationError {
	for index, fact := range facts {
		if fact.SessionID != sessionID || fact.ID == "" || fact.Order != index+1 || fact.Status == "" {
			return &NormalizationError{Field: fmt.Sprintf("results[%d]", index), Reason: "must retain a correlated, ordered result fact"}
		}
	}
	return nil
}

func validateFailures(sessionID string, facts []FailureFact) *NormalizationError {
	for index, fact := range facts {
		if fact.SessionID != sessionID || fact.ID == "" || fact.Order != index+1 || fact.Code == "" || fact.Message == "" {
			return &NormalizationError{Field: fmt.Sprintf("failures[%d]", index), Reason: "must retain a correlated, ordered failure fact"}
		}
	}
	return nil
}

func validateEventCursors(sessionID string, facts []FactoryEventCursor) *NormalizationError {
	var previous int64
	for index, fact := range facts {
		if fact.SessionID != sessionID || fact.Cursor == "" || fact.EventType == "" || (index > 0 && fact.Sequence <= previous) {
			return &NormalizationError{Field: fmt.Sprintf("eventCursors[%d]", index), Reason: "must retain a correlated, strictly ordered event cursor"}
		}
		previous = fact.Sequence
	}
	return nil
}
