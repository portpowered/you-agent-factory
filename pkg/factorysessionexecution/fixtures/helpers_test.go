package fixtures_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func contractFixtureCatalogPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "api", "testdata", "durable-session-contract-fixtures.json")
}

func assertStringSliceEqual(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %#v, want %#v", label, got, want)
	}
	for index := range got {
		if got[index] != want[index] {
			t.Fatalf("%s[%d] = %q, want %q", label, index, got[index], want[index])
		}
	}
}
func publishedScenarioByPurpose(t *testing.T, purpose fixtures.FixtureScenarioPurpose) fixtures.PublishedFixtureScenario {
	t.Helper()
	for _, row := range fixtures.PublishedFixtureScenarios {
		if row.Purpose == purpose {
			return row
		}
	}
	t.Fatalf("published scenario missing for purpose %q", purpose)
	return fixtures.PublishedFixtureScenario{}
}

func startRequestForPublished(row fixtures.PublishedFixtureScenario) fse.StartRequest {
	switch row.RequestID {
	case "req-js-timeout-001":
		return fse.StartRequest{
			RequestID: row.RequestID,
			Source: fse.Source{
				Kind:         workflowsource.KindWorkflowName,
				WorkflowName: "long-running-audit",
			},
			Wait: &fse.WaitOptions{TimeoutMillis: int64Ptr(30000)},
		}
	case "req-idempotent-replay-001":
		return fse.StartRequest{
			RequestID: row.RequestID,
			Source: fse.Source{
				Kind:         workflowsource.KindWorkflowFile,
				WorkflowFile: ".claude/workflows/idempotent.yaml",
			},
			Args: map[string]any{"task": "replay"},
			RequestedPolicy: map[string]any{
				"policyHash": "req-policy-idempotent",
			},
		}
	default:
		return fse.StartRequest{
			RequestID: row.RequestID,
			Source: fse.Source{
				Kind:      workflowsource.KindFactoryID,
				FactoryID: "customer-support-triage",
			},
		}
	}
}

func containsLiveSessionID(sessions []fse.LiveSessionSummary, sessionID string) bool {
	for _, session := range sessions {
		if session.ID == sessionID {
			return true
		}
	}
	return false
}

func containsDurableSessionID(sessions []fse.DurableSessionListSummary, sessionID string) bool {
	for _, session := range sessions {
		if session.SessionID == sessionID {
			return true
		}
	}
	return false
}

func startPublishedScenario(t *testing.T, service *fse.FakeService, row fixtures.PublishedFixtureScenario) {
	t.Helper()
	req := startRequestForPublished(row)
	if row.Purpose == fixtures.FixturePurposeSyncSuccess || row.Purpose == fixtures.FixturePurposeSyncTimeout {
		if _, err := service.StartSync(context.Background(), req); err != nil {
			t.Fatalf("fse.StartSync(%s): %v", row.Purpose, err)
		}
		return
	}
	if _, err := service.StartAsync(context.Background(), req); err != nil {
		t.Fatalf("fse.StartAsync(%s): %v", row.Purpose, err)
	}
}

func startAwaitingApprovalSession(t *testing.T, service *fse.FakeService) {
	t.Helper()
	_, err := service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-js-awaiting-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowFile,
			WorkflowFile: ".claude/workflows/approval-gate.yaml",
		},
	})
	if err != nil {
		t.Fatalf("fse.StartAsync awaiting approval: %v", err)
	}
}

func startFailedPartialSession(t *testing.T, service *fse.FakeService) {
	t.Helper()
	startAsyncByRequestID(t, service, "req-js-failed-partial-001")
}

func liveSessionCount(t *testing.T, service *fse.FakeService) int {
	t.Helper()
	result, err := service.ListSessions(context.Background(), fse.ListSessionsRequest{
		Scope: fse.SessionListScopeLive,
	})
	if err != nil {
		t.Fatalf("fse.ListSessions live: %v", err)
	}
	return len(result.LiveSessions)
}

func assertTypedFailureHash(t *testing.T, err error, wantHash string) fixtures.TypedFailureIdentity {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want typed failure")
	}
	identity, ok := fixtures.TypedFailureIdentityFromError(err)
	if !ok {
		t.Fatalf("error = %v, want mappable typed failure identity", err)
	}
	hash, err := fixtures.TypedFailureHash(identity)
	if err != nil {
		t.Fatalf("fixtures.TypedFailureHash: %v", err)
	}
	if hash != wantHash {
		t.Fatalf("typed failure hash = %q, want %q (identity=%#v)", hash, wantHash, identity)
	}
	return identity
}

func newContractFakeService(t *testing.T) *fse.FakeService {
	t.Helper()
	service, err := fse.NewFakeServiceFromContractFixtures(contractFixtureCatalogPath(t))
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}
	return service
}

func startAsyncByRequestID(t *testing.T, service *fse.FakeService, requestID string) fse.AsyncStartResult {
	t.Helper()
	result, err := service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: requestID,
		Source:    fse.Source{Kind: workflowsource.KindFactoryID, FactoryID: "customer-support-triage"},
	})
	if err != nil {
		t.Fatalf("StartAsync(%q): %v", requestID, err)
	}
	return result
}

func int64Ptr(value int64) *int64 {
	return &value
}

func startPublishedScenarioWithSync(t *testing.T, service *fse.FakeService, row fixtures.PublishedFixtureScenario, sync bool) {
	t.Helper()
	req := startRequestForPublished(row)
	if sync {
		if _, err := service.StartSync(context.Background(), req); err != nil {
			t.Fatalf("StartSync: %v", err)
		}
		return
	}
	if _, err := service.StartAsync(context.Background(), req); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
}

func assertDispatchListStableSummaries(
	t *testing.T,
	service *fse.FakeService,
	row fixtures.PublishedFixtureScenario,
	wantIDs []string,
	wantHash string,
) {
	t.Helper()
	listed, err := service.ListDispatches(context.Background(), row.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if listed.SessionID != row.SessionID {
		t.Fatalf("sessionId = %q, want %q", listed.SessionID, row.SessionID)
	}
	if len(listed.Dispatches) != len(wantIDs) {
		t.Fatalf("dispatches = %#v, want %d rows", listed.Dispatches, len(wantIDs))
	}
	for index, wantID := range wantIDs {
		got := listed.Dispatches[index]
		if got.ID != wantID {
			t.Fatalf("dispatch[%d].id = %q, want %q", index, got.ID, wantID)
		}
		if got.Status == "" || got.DispatchKind == "" {
			t.Fatalf("dispatch[%d] missing status/kind: %#v", index, got)
		}
	}
	read, err := service.GetSession(context.Background(), row.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if err := fse.ValidateDispatchListMatchesSessionProgress(read, listed.Dispatches); err != nil {
		t.Fatalf("ValidateDispatchListMatchesSessionProgress: %v", err)
	}
	hash, err := fixtures.ListDispatchesResultHash(listed)
	if err != nil {
		t.Fatalf("ListDispatchesResultHash: %v", err)
	}
	if hash != wantHash {
		t.Fatalf("dispatch list hash = %q, want %q", hash, wantHash)
	}
}

func assertArtifactListStableSummaries(
	t *testing.T,
	service *fse.FakeService,
	row fixtures.PublishedFixtureScenario,
	wantIDs []string,
	wantHash string,
) {
	t.Helper()
	listed, err := service.ListArtifacts(context.Background(), row.SessionID)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if listed.SessionID != row.SessionID {
		t.Fatalf("sessionId = %q, want %q", listed.SessionID, row.SessionID)
	}
	if len(listed.Artifacts) != len(wantIDs) {
		t.Fatalf("artifacts = %#v, want %d rows", listed.Artifacts, len(wantIDs))
	}
	for index, wantID := range wantIDs {
		got := listed.Artifacts[index]
		if got.ID != wantID {
			t.Fatalf("artifact[%d].id = %q, want %q", index, got.ID, wantID)
		}
		if got.Kind == "" || got.ContentHash == "" {
			t.Fatalf("artifact[%d] missing kind/contentHash: %#v", index, got)
		}
		if got.RetrievalRef == nil || got.RetrievalRef.Href == "" {
			t.Fatalf("artifact[%d] missing retrieval ref: %#v", index, got)
		}
		wantHref := "/factory-sessions/" + row.SessionID + "/artifacts/" + wantID
		if got.RetrievalRef.Href != wantHref {
			t.Fatalf("retrieval href = %q, want %q", got.RetrievalRef.Href, wantHref)
		}
	}
	hash, err := fixtures.ListArtifactsResultHash(listed)
	if err != nil {
		t.Fatalf("ListArtifactsResultHash: %v", err)
	}
	if hash != wantHash {
		t.Fatalf("artifact list hash = %q, want %q", hash, wantHash)
	}
}

func assertCanonicalEventEnvelope(t *testing.T, raw json.RawMessage, eventType, id string) {
	t.Helper()
	const schemaVersion = "agent-factory.event.v1"
	var envelope struct {
		SchemaVersion string `json:"schemaVersion"`
		ID            string `json:"id"`
		Type          string `json:"type"`
		Context       struct {
			Sequence int `json:"sequence"`
		} `json:"context"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("Unmarshal event: %v", err)
	}
	if envelope.SchemaVersion != schemaVersion {
		t.Fatalf("schemaVersion = %q, want %q", envelope.SchemaVersion, schemaVersion)
	}
	if id != "" && envelope.ID != id {
		t.Fatalf("id = %q, want %q", envelope.ID, id)
	}
	if eventType != "" && envelope.Type != eventType {
		t.Fatalf("type = %q, want %q", envelope.Type, eventType)
	}
	if envelope.Context.Sequence <= 0 {
		t.Fatalf("sequence = %d, want positive", envelope.Context.Sequence)
	}
	if len(envelope.Payload) == 0 {
		t.Fatal("payload missing")
	}
}
