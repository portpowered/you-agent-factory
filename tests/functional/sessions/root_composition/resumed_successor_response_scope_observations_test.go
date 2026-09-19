//go:build factoryartifact

package root_composition_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	workersessionscli "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func assertResumedResponseScopeListener(t *testing.T, baseURL string) {
	t.Helper()
	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse functional API URL: %v", err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("functional API URL port %q: %v", parsed.Port(), err)
	}
	if port == 7437 {
		t.Fatal("functional resumed-successor scenario used reserved production port 7437")
	}
}

func assertResumedSuccessorIdentity(t *testing.T, session factoryapi.FactorySession) {
	t.Helper()
	if strings.TrimSpace(session.Id) == "" || !session.IsDefault {
		t.Fatalf("resumed Factory Session = %#v, want a named default session", session)
	}
	if _, err := uuid.Parse(session.Id); err != nil {
		t.Fatalf("resumed successor public session ID = %q, want UUID: %v", session.Id, err)
	}
	if session.Runtime.StreamIdentity == nil {
		t.Fatal("resumed Factory Session has no stream identity")
	}
	if _, err := uuid.Parse(session.Runtime.StreamIdentity.FactorySessionID); err != nil {
		t.Fatalf(
			"resumed successor canonical session ID = %q, want a preallocated UUID: %v",
			session.Runtime.StreamIdentity.FactorySessionID,
			err,
		)
	}
	if session.Runtime.StreamIdentity.StreamGenerationID == "" {
		t.Fatal("resumed successor stream generation ID is empty")
	}
}

func assertResumedSuccessorIdentityStable(t *testing.T, first, second factoryapi.FactorySession) {
	t.Helper()
	if first.Id != second.Id {
		t.Fatalf("resumed successor public session ID changed from %q to %q", first.Id, second.Id)
	}
	if first.Runtime.StreamIdentity == nil || second.Runtime.StreamIdentity == nil {
		t.Fatal("resumed successor identity disappeared between reads")
	}
	if first.Runtime.StreamIdentity.FactorySessionID != second.Runtime.StreamIdentity.FactorySessionID {
		t.Fatalf(
			"resumed successor canonical session ID changed from %q to %q",
			first.Runtime.StreamIdentity.FactorySessionID,
			second.Runtime.StreamIdentity.FactorySessionID,
		)
	}
	if first.Runtime.StreamIdentity.StreamGenerationID != second.Runtime.StreamIdentity.StreamGenerationID {
		t.Fatalf(
			"resumed successor stream generation ID changed from %q to %q",
			first.Runtime.StreamIdentity.StreamGenerationID,
			second.Runtime.StreamIdentity.StreamGenerationID,
		)
	}
}

func assertResumedHistoricalPrefix(
	t testing.TB,
	events []factoryapi.FactoryEvent,
	ledger resumedResponseScopeLedger,
) {
	t.Helper()
	if len(events) < ledger.eventCount {
		t.Fatalf("public resumed event count = %d, want at least %d", len(events), ledger.eventCount)
	}
	seen := make(map[string]struct{}, len(events))
	for _, event := range events {
		if event.Id == "" {
			t.Fatal("public resumed Factory Event has empty ID")
		}
		if _, duplicate := seen[event.Id]; duplicate {
			t.Fatalf("public resumed Factory Events duplicate ID %q", event.Id)
		}
		seen[event.Id] = struct{}{}
	}
	for eventID := range ledger.eventIDs {
		if _, found := seen[eventID]; !found {
			t.Fatalf("public resumed Factory Events omitted historical event %q", eventID)
		}
	}
	if got := support.CountFactoryEvents(events, factoryapi.FactoryEventTypeSessionCompleted); got < ledger.sessionCompleted {
		t.Fatalf("public SESSION_COMPLETED count = %d, want at least %d", got, ledger.sessionCompleted)
	}
}

func assertResumedSuccessorLifecycle(
	t testing.TB,
	events []factoryapi.FactoryEvent,
	canonicalSessionID string,
) {
	t.Helper()
	var startedEvents, completedEvents []factoryapi.FactoryEvent
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeSessionStarted:
			if event.Id == "factory-event/session-started/"+canonicalSessionID {
				startedEvents = append(startedEvents, event)
			}
		case factoryapi.FactoryEventTypeSessionCompleted:
			if event.Id == "factory-event/session-completed/"+canonicalSessionID {
				completedEvents = append(completedEvents, event)
			}
		}
	}
	if len(startedEvents) != 1 || len(completedEvents) != 1 {
		t.Fatalf(
			"successor lifecycle events for %q = started:%d completed:%d, want exactly one of each",
			canonicalSessionID, len(startedEvents), len(completedEvents),
		)
	}
	started, err := startedEvents[0].Payload.AsSessionStartedEventPayload()
	if err != nil {
		t.Fatalf("decode successor SESSION_STARTED %q: %v", startedEvents[0].Id, err)
	}
	completed, err := completedEvents[0].Payload.AsSessionCompletedEventPayload()
	if err != nil {
		t.Fatalf("decode successor SESSION_COMPLETED %q: %v", completedEvents[0].Id, err)
	}
	wantDuration := completed.CompletedAt.Sub(started.StartedAt).Milliseconds()
	if wantDuration < 0 {
		t.Fatalf("successor lifecycle times are reversed: started=%s completed=%s", started.StartedAt, completed.CompletedAt)
	}
	if completed.DurationMillis == nil || *completed.DurationMillis != wantDuration {
		t.Fatalf(
			"successor lifecycle duration = %#v, want %d ms from %s to %s",
			completed.DurationMillis, wantDuration, started.StartedAt, completed.CompletedAt,
		)
	}
}

func assertResumedWorkerSessionIdentity(
	t testing.TB,
	server *support.FunctionalAPIServer,
	sessionID, workID, canonicalSessionID, workerSessionID string,
) {
	t.Helper()
	httpList := support.ListSessionWorkerSessions(t, server.URL(), sessionID, workID)
	assertResumedWorkerSessionList(t, "HTTP", httpList, canonicalSessionID, workID)
	assertResumedWorkerSessionID(t, "HTTP", httpList, workerSessionID)
	stdout, err := listResumedWorkerSessionsThroughCLI(t, server.URL(), sessionID, workID)
	if err != nil {
		t.Fatalf("CLI Worker Session list: %v; stdout=%q", err, stdout)
	}
	var cliList factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &cliList); err != nil {
		t.Fatalf("decode CLI Worker Session list: %v; stdout=%q", err, stdout)
	}
	assertResumedWorkerSessionList(t, "CLI", cliList, canonicalSessionID, workID)
	assertResumedWorkerSessionID(t, "CLI", cliList, workerSessionID)
}

func listResumedWorkerSessionsThroughCLI(
	t testing.TB,
	serverURL, sessionID, workID string,
) (string, error) {
	t.Helper()
	protocol, err := clihttp.NewProtocol(
		&http.Client{Timeout: resumedResponseScopeObservationTimeout},
		platformclock.Real{},
	)
	if err != nil {
		return "", fmt.Errorf("build Worker Sessions CLI HTTP protocol: %w", err)
	}
	var stdout bytes.Buffer
	err = workersessionscli.NewList(protocol)(workersessionscli.ListConfig{
		Context:      t.Context(),
		Server:       serverURL,
		SessionID:    sessionID,
		WorkID:       workID,
		OutputFormat: "json",
		Output:       &stdout,
		Diagnostics:  io.Discard,
	})
	return stdout.String(), err
}

func assertResumedWorkerSessionList(
	t testing.TB,
	boundary string,
	listed factoryapi.ListWorkerSessionsResponse,
	wantCanonicalSessionID, wantWorkID string,
) {
	t.Helper()
	if len(listed.Sessions) == 0 {
		t.Fatalf("%s Worker Session list for Work %q is empty", boundary, wantWorkID)
	}
	for _, observation := range listed.Sessions {
		assertResumedWorkerSessionObservation(t, boundary, observation, wantCanonicalSessionID, wantWorkID)
	}
}

func assertResumedWorkerSessionID(
	t testing.TB,
	boundary string,
	listed factoryapi.ListWorkerSessionsResponse,
	wantWorkerSessionID string,
) {
	t.Helper()
	for _, observation := range listed.Sessions {
		if observation.WorkerSessionId == wantWorkerSessionID {
			return
		}
	}
	t.Fatalf("%s Worker Session list omitted Worker Session %q", boundary, wantWorkerSessionID)
}

func assertResumedWorkerSessionObservation(
	t testing.TB,
	boundary string,
	observation factoryapi.WorkerSessionObservation,
	wantCanonicalSessionID, wantWorkID string,
) {
	t.Helper()
	if observation.WorkerSessionId == "" {
		t.Fatalf("%s Worker Session observation has empty identity: %#v", boundary, observation)
	}
	if observation.WorkId == nil || *observation.WorkId != wantWorkID {
		t.Fatalf("%s Worker Session %q Work ID = %#v, want %q", boundary, observation.WorkerSessionId, observation.WorkId, wantWorkID)
	}
	if observation.FactorySessionId == nil || *observation.FactorySessionId != wantCanonicalSessionID {
		t.Fatalf(
			"%s Worker Session %q Factory Session = %#v, want canonical %q",
			boundary, observation.WorkerSessionId, observation.FactorySessionId, wantCanonicalSessionID,
		)
	}
	if observation.Direct {
		t.Fatalf("%s Worker Session %q was marked direct, want Factory dispatch attribution", boundary, observation.WorkerSessionId)
	}
}

func resumedWorkerSessionIDForDispatch(
	t testing.TB,
	events []factoryapi.FactoryEvent,
	dispatchID string,
) string {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation ||
			event.Context.DispatchId == nil || *event.Context.DispatchId != dispatchID {
			continue
		}
		payload, err := event.Payload.AsDispatchWorkerSessionAssociationEventPayload()
		if err != nil {
			t.Fatalf("decode Worker Session association %q: %v", event.Id, err)
		}
		if strings.TrimSpace(payload.WorkerSessionId) != "" {
			return payload.WorkerSessionId
		}
	}
	t.Fatalf("generated dispatch %q has no Worker Session association", dispatchID)
	return ""
}

type resumedScopeSnapshot struct {
	workCount               int
	relationCount           int
	workIDsDigest           string
	workDigest              string
	canonicalEventCount     int
	canonicalEventIDsDigest string
	canonicalIdentityDigest string
	responseEventCount      int
	responseEventIDsDigest  string
	responseIdentityDigest  string
}

func captureOpenResumedScopeSnapshot(
	t testing.TB,
	baseURL, sessionID string,
) (resumedScopeSnapshot, []factoryapi.FactoryEvent, []factoryapi.FactoryResponseEvent) {
	t.Helper()
	events := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	responseEvents := readOpenResumedResponseEvents(t, baseURL, sessionID)
	return buildResumedScopeSnapshot(t, baseURL, sessionID, events, responseEvents), events, responseEvents
}

func captureOpenResumedScopeSnapshotOnly(t testing.TB, baseURL, sessionID string) resumedScopeSnapshot {
	t.Helper()
	snapshot, _, _ := captureOpenResumedScopeSnapshot(t, baseURL, sessionID)
	return snapshot
}

func captureClosedResumedScopeSnapshot(
	t testing.TB,
	baseURL, sessionID string,
	responseEvents []factoryapi.FactoryResponseEvent,
) resumedScopeSnapshot {
	t.Helper()
	if responseEvents == nil {
		responseEvents = readClosedResumedResponseEvents(t, baseURL, sessionID)
	}
	events := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	return buildResumedScopeSnapshot(t, baseURL, sessionID, events, responseEvents)
}

func buildResumedScopeSnapshot(
	t testing.TB,
	baseURL, sessionID string,
	events []factoryapi.FactoryEvent,
	responseEvents []factoryapi.FactoryResponseEvent,
) resumedScopeSnapshot {
	t.Helper()
	works := readAllResumedSessionWork(t, baseURL, sessionID)
	workJSON := make([]json.RawMessage, 0, len(works))
	workIDs := make([]string, 0, len(works))
	relationCount := 0
	for _, item := range works {
		encoded, err := json.Marshal(item)
		if err != nil {
			t.Fatalf("marshal public Work snapshot: %v", err)
		}
		workJSON = append(workJSON, encoded)
		workIDs = append(workIDs, support.StringPointerValue(item.WorkId))
		relationCount += len(support.FactoryRelationsValue(item.Relations))
	}
	sort.Slice(workJSON, func(i, j int) bool { return string(workJSON[i]) < string(workJSON[j]) })
	sort.Strings(workIDs)
	return resumedScopeSnapshot{
		workCount:               len(works),
		relationCount:           relationCount,
		workIDsDigest:           digestResumedJSON(t, workIDs),
		workDigest:              digestResumedJSON(t, workJSON),
		canonicalEventCount:     len(events),
		canonicalEventIDsDigest: digestResumedJSON(t, resumedCanonicalEventIDs(events)),
		canonicalIdentityDigest: digestResumedJSON(t, resumedCanonicalEventIdentities(events)),
		responseEventCount:      len(responseEvents),
		responseEventIDsDigest:  digestResumedJSON(t, resumedResponseEventIDs(responseEvents)),
		responseIdentityDigest:  digestResumedJSON(t, resumedResponseEventIdentities(responseEvents)),
	}
}

func assertResumedScopeSnapshotUnchanged(t testing.TB, before, after resumedScopeSnapshot, boundary string) {
	t.Helper()
	if before != after {
		t.Fatalf("%s mutated public/canonical state: before=%#v after=%#v", boundary, before, after)
	}
}

func readAllResumedSessionWork(t testing.TB, baseURL, sessionID string) []factoryapi.Work {
	t.Helper()
	params := url.Values{}
	params.Set("includeSuperseded", "true")
	params.Set("maxResults", "1000")
	var results []factoryapi.Work
	for {
		page := support.GetJSON[factoryapi.ListWorkResponse](
			t,
			support.SessionWorkURL(baseURL, sessionID, "/work?"+params.Encode()),
		)
		results = append(results, page.Results...)
		if page.PaginationContext == nil || page.PaginationContext.NextToken == nil || strings.TrimSpace(*page.PaginationContext.NextToken) == "" {
			return results
		}
		params.Set("nextToken", *page.PaginationContext.NextToken)
	}
}

func resumedCanonicalEventIDs(events []factoryapi.FactoryEvent) []string {
	ids := make([]string, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.Id)
	}
	return ids
}

type resumedCanonicalEventIdentity struct {
	ID              string                      `json:"id"`
	Type            factoryapi.FactoryEventType `json:"type"`
	Sequence        int                         `json:"sequence"`
	SessionSequence *int                        `json:"sessionSequence,omitempty"`
}

func resumedCanonicalEventIdentities(events []factoryapi.FactoryEvent) []resumedCanonicalEventIdentity {
	identities := make([]resumedCanonicalEventIdentity, 0, len(events))
	for _, event := range events {
		identities = append(identities, resumedCanonicalEventIdentity{
			ID: event.Id, Type: event.Type, Sequence: event.Context.Sequence,
			SessionSequence: event.Context.SessionSequence,
		})
	}
	return identities
}

type resumedResponseEventIdentity struct {
	EventID    string                               `json:"eventId"`
	Sequence   int64                                `json:"sequence"`
	Kind       factoryapi.FactoryResponseEventKind  `json:"kind"`
	Phase      factoryapi.FactoryResponseEventPhase `json:"phase"`
	RunID      string                               `json:"runId"`
	DispatchID string                               `json:"dispatchId,omitempty"`
}

func resumedResponseEventIDs(events []factoryapi.FactoryResponseEvent) []string {
	ids := make([]string, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.EventId)
	}
	return ids
}

func resumedResponseEventIdentities(events []factoryapi.FactoryResponseEvent) []resumedResponseEventIdentity {
	identities := make([]resumedResponseEventIdentity, 0, len(events))
	for _, event := range events {
		identities = append(identities, resumedResponseEventIdentity{
			EventID: event.EventId, Sequence: event.Sequence, Kind: event.Kind,
			Phase: event.Phase, RunID: event.RunId,
			DispatchID: support.StringPointerValue(event.DispatchId),
		})
	}
	return identities
}

func digestResumedJSON(t testing.TB, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal resumed snapshot digest: %v", err)
	}
	digest := sha256.Sum256(encoded)
	return strings.ToUpper(hex.EncodeToString(digest[:]))
}
