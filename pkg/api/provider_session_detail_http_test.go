package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestGetProviderSessionDetails_EventRefRoundTripLoadsCursorAndCodex(t *testing.T) {
	codexRoot := t.TempDir()
	writeProviderSessionFixture(t, codexRoot, "sess_123", strings.Join([]string{
		`{"type":"session_meta","id":"sess_123"}`,
		`{"type":"response_item","item":{"type":"reasoning"}}`,
	}, "\n"))

	cursorRoot, cursorSessionID := writeCursorProviderSessionUUIDFixture(t)
	srv := newTestServerWithProviderSessionRoots(codexRoot, cursorRoot)

	codexEventRef := factoryapi.LoadableProviderSessionRef{
		Provider: factoryapi.Codex,
		Kind:     factoryapi.LoadableProviderSessionKindSessionID,
		Id:       "sess_123",
	}
	assertProviderSessionDetailLoadsFromEventRef(t, srv, codexEventRef, factoryapi.Codex)

	cursorEventRef := factoryapi.LoadableProviderSessionRef{
		Provider: factoryapi.Cursor,
		Kind:     factoryapi.LoadableProviderSessionKindSessionID,
		Id:       cursorSessionID,
	}
	assertProviderSessionDetailLoadsFromEventRef(t, srv, cursorEventRef, factoryapi.Cursor)

	legacyAgentEventRef := factoryapi.LoadableProviderSessionRef{
		Provider: factoryapi.LoadableProviderSessionProvider("agent"),
		Kind:     factoryapi.LoadableProviderSessionKindSessionID,
		Id:       cursorSessionID,
	}
	assertProviderSessionDetailLoadsFromEventRef(t, srv, legacyAgentEventRef, factoryapi.Cursor)

	canonicalizedLegacyRef := loadableProviderSessionRefFromEventMetadata(interfaces.ProviderSessionMetadata{
		Provider: "agent",
		Kind:     "session_id",
		ID:       cursorSessionID,
	})
	if string(canonicalizedLegacyRef.Provider) != string(factoryapi.Cursor) {
		t.Fatalf("canonicalized legacy ref provider = %q, want cursor", canonicalizedLegacyRef.Provider)
	}
	assertProviderSessionDetailLoadsFromEventRef(t, srv, canonicalizedLegacyRef, factoryapi.Cursor)
}

func TestGetProviderSessionDetails_RegressionLoadsCodexAndCursorFromConfiguredRoots(t *testing.T) {
	codexRoot := t.TempDir()
	writeProviderSessionFixture(t, codexRoot, "sess_123", strings.Join([]string{
		`{"type":"session_meta","id":"sess_123"}`,
		`{"type":"response_item","item":{"type":"reasoning"}}`,
	}, "\n"))

	cursorRoot, cursorSessionID := writeCursorProviderSessionUUIDFixture(t)

	srv := newTestServerWithProviderSessionRoots(codexRoot, cursorRoot)

	codexReq := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=sess_123", nil)
	codexRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(codexRec, codexReq)
	if codexRec.Code != http.StatusOK {
		t.Fatalf("codex status = %d, want 200: %s", codexRec.Code, codexRec.Body.String())
	}
	codexResp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, codexRec)
	assertProviderSessionResponseIdentity(t, codexResp)

	cursorReq := httptest.NewRequest("GET", "/provider-sessions/detail?provider=cursor&kind=session_id&id="+cursorSessionID, nil)
	cursorRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(cursorRec, cursorReq)
	if cursorRec.Code != http.StatusOK {
		t.Fatalf("cursor status = %d, want 200: %s", cursorRec.Code, cursorRec.Body.String())
	}
	cursorResp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, cursorRec)
	if string(cursorResp.ProviderSession.Provider) != "cursor" || cursorResp.ProviderSession.Id != cursorSessionID {
		t.Fatalf("cursor provider session = %#v, want cursor session_id %s", cursorResp.ProviderSession, cursorSessionID)
	}
}

func TestGetProviderSessionDetails_LoadsCodexSessionFromConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	writeProviderSessionFixture(t, root, "sess_123", strings.Join([]string{
		`{"type":"session_meta","id":"sess_123"}`,
		`{"type":"response_item","item":{"type":"reasoning"}}`,
		`{"unexpected":true}`,
		`not-json`,
		``,
	}, "\n"))

	srv := newTestServerWithCodexRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=codex&kind=session_id&id=sess_123", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, rec)
	assertProviderSessionResponseIdentity(t, resp)
	assertProviderSessionParseCounts(t, resp.Parse)
	assertProviderSessionTranscriptSummary(t, resp)
	assertProviderSessionParseDiagnostics(t, resp.Parse)
}

func TestGetProviderSessionDetails_LoadsCursorSessionFromConfiguredRoot(t *testing.T) {
	root, sessionID := writeCursorProviderSessionFixture(t)

	srv := newTestServerWithCursorRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=cursor&kind=session_id&id="+sessionID, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, rec)
	if string(resp.ProviderSession.Provider) != "cursor" || string(resp.ProviderSession.Kind) != "session_id" || resp.ProviderSession.Id != sessionID {
		t.Fatalf("provider session = %#v, want cursor session_id %s", resp.ProviderSession, sessionID)
	}
	if resp.Source.RelativePath != "workspace-hash/"+sessionID+"/store.db" || resp.Source.SizeBytes == 0 {
		t.Fatalf("source = %#v, want rooted cursor store.db metadata", resp.Source)
	}
	if resp.Parse.EventCount != 1 || resp.Parse.LineCount < 1 {
		t.Fatalf("parse summary = %#v, want one readable cursor event", resp.Parse)
	}
	if len(resp.Transcript) != 1 || stringValue(resp.Transcript[0].Text) != "Hello from API fixture" {
		t.Fatalf("transcript = %#v, want one readable cursor transcript entry", resp.Transcript)
	}
	if resp.Parse.TokenUsage == nil || intValue(resp.Parse.TokenUsage.InputTokens) != 100 || intValue(resp.Parse.TokenUsage.TotalTokens) != 175 {
		t.Fatalf("token usage = %#v, want aggregated cursor usage metadata", resp.Parse.TokenUsage)
	}
}

func TestGetProviderSessionDetails_LoadsCursorUUIDSessionFromConfiguredRoot(t *testing.T) {
	root, sessionID := writeCursorProviderSessionUUIDFixture(t)

	srv := newTestServerWithCursorRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=cursor&kind=session_id&id="+sessionID, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, rec)
	if string(resp.ProviderSession.Provider) != "cursor" || string(resp.ProviderSession.Kind) != "session_id" || resp.ProviderSession.Id != sessionID {
		t.Fatalf("provider session = %#v, want cursor session_id %s", resp.ProviderSession, sessionID)
	}
	wantRelativePath := customerCursorWorkspaceHash + "/" + sessionID + "/store.db"
	if resp.Source.RelativePath != wantRelativePath || resp.Source.SizeBytes == 0 {
		t.Fatalf("source = %#v, want rooted cursor store.db metadata at %s", resp.Source, wantRelativePath)
	}
	if resp.Parse.EventCount != 1 || len(resp.Transcript) != 1 || stringValue(resp.Transcript[0].Text) != "Hello from API fixture" {
		t.Fatalf("response = %#v, want readable cursor transcript for UUID session_id", resp)
	}
}

func TestGetProviderSessionDetails_LoadsLegacyAgentCursorSessionFromConfiguredRoot(t *testing.T) {
	root, sessionID := writeCursorProviderSessionFixture(t)

	srv := newTestServerWithCursorRoot(root)
	req := httptest.NewRequest("GET", "/provider-sessions/detail?provider=agent&kind=session_id&id="+sessionID, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ProviderSessionDetailResponse](t, rec)
	if string(resp.ProviderSession.Provider) != "cursor" || string(resp.ProviderSession.Kind) != "session_id" || resp.ProviderSession.Id != sessionID {
		t.Fatalf("provider session = %#v, want canonical cursor session_id %s", resp.ProviderSession, sessionID)
	}
}

func assertProviderSessionResponseIdentity(t *testing.T, resp factoryapi.ProviderSessionDetailResponse) {
	t.Helper()

	if string(resp.ProviderSession.Provider) != "codex" || string(resp.ProviderSession.Kind) != "session_id" || resp.ProviderSession.Id != "sess_123" {
		t.Fatalf("provider session = %#v, want codex session_id sess_123", resp.ProviderSession)
	}
	if resp.Source.RelativePath != "2026/05/18/rollout-sess_123.jsonl" || resp.Source.SizeBytes == 0 {
		t.Fatalf("source = %#v, want rooted rollout path with metadata", resp.Source)
	}
}

func assertProviderSessionParseCounts(t *testing.T, parse factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if parse.LineCount != 4 || parse.EventCount != 3 || parse.MalformedLineCount != 1 || parse.UnknownEventCount != 1 {
		t.Fatalf("parse summary = %#v, want line/event/malformed/unknown counts", parse)
	}
}

func assertProviderSessionTranscriptSummary(t *testing.T, resp factoryapi.ProviderSessionDetailResponse) {
	t.Helper()

	if len(resp.Transcript) != 1 || resp.Transcript[0].Type != factoryapi.Reasoning || resp.Transcript[0].Order != 1 {
		t.Fatalf("transcript = %#v, want one reasoning transcript entry", resp.Transcript)
	}
	if len(resp.Parse.Turns) != 1 || resp.Parse.Turns[0].ReasoningCount != 1 || len(resp.Parse.Reasoning) != 1 || resp.Parse.Reasoning[0].SourceType != "reasoning" {
		t.Fatalf("parse detail = %#v, want reasoning turn summary", resp.Parse)
	}
}

func assertProviderSessionParseDiagnostics(t *testing.T, parse factoryapi.ProviderSessionParseSummary) {
	t.Helper()

	if len(parse.ParseErrors) != 1 || parse.ParseErrors[0].LineNumber != 4 || len(parse.UnknownEvents) != 1 || parse.UnknownEvents[0].LineNumber != 3 {
		t.Fatalf("parse diagnostics = %#v, want malformed line 4 and unknown line 3", parse)
	}
}
