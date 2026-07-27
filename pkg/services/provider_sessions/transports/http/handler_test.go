package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestProviderSessionDetailToAPIPreservesIdentitySourceParseAndTranscript(t *testing.T) {
	modifiedAt := time.Date(2026, 7, 27, 23, 44, 0, 0, time.UTC)
	text := "adapter detail transcript"
	inputTokens := 4
	outputTokens := 8
	detail := providersessions.Detail{
		ProviderSession: providersessions.Ref{
			Provider: providersessions.ProviderCodex,
			Kind:     providersessions.SessionIDKind,
			ID:       "session-http-2",
		},
		Source: providersessions.SourceMetadata{
			ModifiedAt:   &modifiedAt,
			RelativePath: "2026/07/27/rollout-session-http-2.jsonl",
			SizeBytes:    128,
		},
		Parse: providersessions.ParseSummary{
			EventCount: 2,
			LineCount:  2,
			TokenUsage: &providersessions.TokenUsage{
				InputTokens:  &inputTokens,
				OutputTokens: &outputTokens,
				TotalTokens:  ptrInt(12),
			},
			FunctionCalls: []providersessions.FunctionCallSummary{{
				CallID: ptrString("call-1"),
				Name:   ptrString("lookup"),
				Order:  0,
				Type:   "function_call",
			}},
		},
		Transcript: []providersessions.TranscriptEntry{{
			Order: 0,
			Text:  &text,
			Type:  providersessions.TranscriptAssistantMessage,
		}},
	}

	got := providerSessionDetailToAPI(detail)
	if got.ProviderSession.Id != "session-http-2" ||
		got.ProviderSession.Provider != factoryapi.Codex ||
		got.ProviderSession.Kind != factoryapi.LoadableProviderSessionKindSessionID {
		t.Fatalf("providerSession = %#v, want codex session_id identity", got.ProviderSession)
	}
	if got.Source.RelativePath != detail.Source.RelativePath || got.Source.SizeBytes != 128 {
		t.Fatalf("source = %#v, want mapped source metadata", got.Source)
	}
	if got.Parse.EventCount != 2 || got.Parse.TokenUsage == nil || *got.Parse.TokenUsage.TotalTokens != 12 {
		t.Fatalf("parse = %#v, want mapped parse summary", got.Parse)
	}
	if len(got.Parse.FunctionCalls) != 1 || got.Parse.FunctionCalls[0].Name == nil || *got.Parse.FunctionCalls[0].Name != "lookup" {
		t.Fatalf("functionCalls = %#v, want mapped function call summary", got.Parse.FunctionCalls)
	}
	if len(got.Transcript) != 1 || got.Transcript[0].Text == nil || *got.Transcript[0].Text != text {
		t.Fatalf("transcript = %#v, want mapped transcript entry", got.Transcript)
	}
}

func TestDecodeDetailsParamsRejectsBlankIdentifierBeforeRoot(t *testing.T) {
	for _, id := range []string{"", "   ", "\t"} {
		t.Run(id, func(t *testing.T) {
			_, _, _, err := decodeDetailsParams(factoryapi.GetProviderSessionDetailsParams{
				Provider: factoryapi.Codex,
				Kind:     factoryapi.LoadableProviderSessionKindSessionID,
				Id:       id,
			})
			var validationErr requestValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("err = %v, want requestValidationError", err)
			}
		})
	}
}

func TestAdapterGetProviderSessionDetailsDecodesAndMapsSuccess(t *testing.T) {
	modifiedAt := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	text := "mapped detail"
	fake := &rootServiceFake{
		detail: providersessions.Detail{
			ProviderSession: providersessions.Ref{
				Provider: providersessions.ProviderCursor,
				Kind:     providersessions.SessionIDKind,
				ID:       "cursor_sess_01",
			},
			Source: providersessions.SourceMetadata{
				ModifiedAt:   &modifiedAt,
				RelativePath: "workspace/cursor_sess_01/store.db",
				SizeBytes:    42,
			},
			Parse: providersessions.ParseSummary{EventCount: 1, LineCount: 1},
			Transcript: []providersessions.TranscriptEntry{{
				Order: 0,
				Text:  &text,
				Type:  providersessions.TranscriptUserMessage,
			}},
		},
	}
	adapter := NewAdapter(fake)

	response, err := adapter.GetProviderSessionDetails(factoryapi.GetProviderSessionDetailsParams{
		Provider: factoryapi.Cursor,
		Kind:     factoryapi.LoadableProviderSessionKindSessionID,
		Id:       "  cursor_sess_01  ",
	})
	if err != nil {
		t.Fatalf("GetProviderSessionDetails: %v", err)
	}
	if fake.lastProvider != string(factoryapi.Cursor) ||
		fake.lastKind != string(factoryapi.LoadableProviderSessionKindSessionID) ||
		fake.lastID != "cursor_sess_01" {
		t.Fatalf("fake recorded identity = (%q, %q, %q)", fake.lastProvider, fake.lastKind, fake.lastID)
	}
	if response.ProviderSession.Id != "cursor_sess_01" || len(response.Transcript) != 1 {
		t.Fatalf("response = %#v, want encoded detail response", response)
	}
}

func TestAdapterGetProviderSessionDetailsRejectsBlankIdentifierBeforeRoot(t *testing.T) {
	fake := &rootServiceFake{
		detailErr: errors.New("root must not be called"),
	}
	adapter := NewAdapter(fake)

	_, err := adapter.GetProviderSessionDetails(factoryapi.GetProviderSessionDetailsParams{
		Provider: factoryapi.Codex,
		Kind:     factoryapi.LoadableProviderSessionKindSessionID,
		Id:       "   ",
	})
	var validationErr requestValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("err = %v, want requestValidationError before root call", err)
	}
	if fake.lastID != "" {
		t.Fatalf("fake.lastID = %q, want root not invoked", fake.lastID)
	}
}

func TestNewHandlerRequiresInjectedAdapter(t *testing.T) {
	if handler := NewHandler(nil, zap.NewNop()); handler != nil {
		t.Fatalf("NewHandler(nil) = %#v, want nil", handler)
	}
	if handler := NewHandler(NewAdapter(&rootServiceFake{}), nil); handler != nil {
		t.Fatalf("NewHandler(adapter, nil) = %#v, want nil", handler)
	}
}

func TestHandlerGetProviderSessionDetailsEncodesFakeRootSuccess(t *testing.T) {
	modifiedAt := time.Date(2026, 7, 27, 15, 30, 0, 0, time.UTC)
	text := "handler success"
	fake := &rootServiceFake{
		detail: providersessions.Detail{
			ProviderSession: providersessions.Ref{
				Provider: providersessions.ProviderCodex,
				Kind:     providersessions.SessionIDKind,
				ID:       "sess_123",
			},
			Source: providersessions.SourceMetadata{
				ModifiedAt:   &modifiedAt,
				RelativePath: "2026/05/18/rollout-sess_123.jsonl",
				SizeBytes:    96,
			},
			Parse: providersessions.ParseSummary{EventCount: 3, LineCount: 3},
			Transcript: []providersessions.TranscriptEntry{{
				Order: 0,
				Text:  &text,
				Type:  providersessions.TranscriptAssistantMessage,
			}},
		},
	}
	handler := NewHandler(NewAdapter(fake), zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.GetProviderSessionDetails(recorder, httptest.NewRequest(http.MethodGet, "/provider-sessions/detail", nil), factoryapi.GetProviderSessionDetailsParams{
		Provider: factoryapi.Codex,
		Kind:     factoryapi.LoadableProviderSessionKindSessionID,
		Id:       "sess_123",
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ProviderSessionDetailResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ProviderSession.Id != "sess_123" ||
		response.Source.RelativePath != "2026/05/18/rollout-sess_123.jsonl" ||
		response.Parse.EventCount != 3 ||
		len(response.Transcript) != 1 {
		t.Fatalf("response = %#v, want encoded provider session detail", response)
	}
}

func TestHandlerGetProviderSessionDetailsRejectsBlankIdentifierBeforeRoot(t *testing.T) {
	fake := &rootServiceFake{
		detailErr: errors.New("root must not be called"),
	}
	handler := NewHandler(NewAdapter(fake), zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.GetProviderSessionDetails(recorder, httptest.NewRequest(http.MethodGet, "/provider-sessions/detail", nil), factoryapi.GetProviderSessionDetailsParams{
		Provider: factoryapi.Codex,
		Kind:     factoryapi.LoadableProviderSessionKindSessionID,
		Id:       " ",
	})

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"BAD_REQUEST"`) {
		t.Fatalf("body = %s, want BAD_REQUEST error response", recorder.Body.String())
	}
	if fake.lastID != "" {
		t.Fatalf("fake.lastID = %q, want root not invoked", fake.lastID)
	}
}

func TestHandlerGetProviderSessionDetailsMapsTypedRootErrors(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name        string
		provider    factoryapi.LoadableProviderSessionProvider
		kind        factoryapi.LoadableProviderSessionKind
		rootErr     error
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{
			name:        "session not found",
			provider:    factoryapi.Codex,
			rootErr:     providersessions.ErrSessionNotFound,
			wantStatus:  http.StatusNotFound,
			wantCode:    "NOT_FOUND",
			wantMessage: "provider session not found",
		},
		{
			name:        "cursor session not found",
			provider:    factoryapi.Cursor,
			rootErr:     providersessions.ErrSessionNotFound,
			wantStatus:  http.StatusNotFound,
			wantCode:    "NOT_FOUND",
			wantMessage: "provider session not found",
		},
		{
			name:     "codex invalid identifier",
			provider: factoryapi.Codex,
			rootErr: &providersessions.LookupError{
				Provider: providersessions.ProviderCodex,
				Err:      providersessions.ErrInvalidIdentifier,
			},
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "provider session must be a codex session_id identifier without path separators",
		},
		{
			name:     "cursor invalid identifier",
			provider: factoryapi.Cursor,
			rootErr: &providersessions.LookupError{
				Provider: providersessions.ProviderCursor,
				Err:      providersessions.ErrInvalidIdentifier,
			},
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "provider session must be a cursor session_id identifier without path separators",
		},
		{
			name:        "unsupported provider",
			provider:    factoryapi.LoadableProviderSessionProvider("openai"),
			rootErr:     providersessions.ErrUnsupportedProvider,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "invalid request parameter",
		},
		{
			name:        "unsupported kind",
			provider:    factoryapi.Codex,
			kind:        factoryapi.LoadableProviderSessionKind("path"),
			rootErr:     providersessions.ErrUnsupportedKind,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "invalid request parameter",
		},
		{
			name:        "ambiguous session file",
			provider:    factoryapi.Codex,
			rootErr:     providersessions.ErrAmbiguousSessionFile,
			wantStatus:  http.StatusInternalServerError,
			wantCode:    "INTERNAL_ERROR",
			wantMessage: "multiple provider session files match session identifier",
		},
		{
			name:     "unmapped internal failure",
			provider: factoryapi.Codex,
			rootErr: &providersessions.LookupError{
				Provider: providersessions.ProviderCodex,
				Root:     root,
				Err:      errors.New("pkg/services/provider_sessions/internal/codex_reader: stat failed"),
			},
			wantStatus:  http.StatusInternalServerError,
			wantCode:    "INTERNAL_ERROR",
			wantMessage: "failed to load provider session details",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kind := test.kind
			if kind == "" {
				kind = factoryapi.LoadableProviderSessionKindSessionID
			}
			fake := &rootServiceFake{detailErr: test.rootErr}
			handler := NewHandler(NewAdapter(fake), zap.NewNop())
			recorder := httptest.NewRecorder()

			handler.GetProviderSessionDetails(recorder, httptest.NewRequest(http.MethodGet, "/provider-sessions/detail", nil), factoryapi.GetProviderSessionDetailsParams{
				Provider: test.provider,
				Kind:     kind,
				Id:       "sess-123",
			})

			assertHandlerJSONError(t, recorder, test.wantStatus, test.wantCode, test.wantMessage)
			if test.name == "unmapped internal failure" {
				body := recorder.Body.String()
				for _, forbidden := range []string{
					root,
					"pkg/services/provider_sessions/internal",
					"codex_reader",
					"stat failed",
				} {
					if strings.Contains(body, forbidden) {
						t.Fatalf("body = %s, must not leak internal detail %q", body, forbidden)
					}
				}
			}
		})
	}
}

func TestHandlerGetProviderSessionDetailsCursorNotFoundLogsDiagnostic(t *testing.T) {
	root := t.TempDir()
	core, logs := observer.New(zap.InfoLevel)
	fake := &rootServiceFake{
		detailErr: &providersessions.LookupError{
			Provider: providersessions.ProviderCursor,
			Root:     root,
			Err:      providersessions.ErrSessionNotFound,
		},
	}
	handler := NewHandler(NewAdapter(fake), zap.New(core))
	recorder := httptest.NewRecorder()

	handler.GetProviderSessionDetails(recorder, httptest.NewRequest(http.MethodGet, "/provider-sessions/detail", nil), factoryapi.GetProviderSessionDetailsParams{
		Provider: factoryapi.Cursor,
		Kind:     factoryapi.LoadableProviderSessionKindSessionID,
		Id:       "missing-session",
	})

	assertHandlerJSONError(t, recorder, http.StatusNotFound, "NOT_FOUND", "provider session not found")

	entries := logs.FilterMessage("cursor provider session lookup not found").AllUntimed()
	if len(entries) != 1 {
		t.Fatalf("cursor not-found diagnostic count = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["provider"] != "cursor" {
		t.Fatalf("provider field = %#v, want cursor", fields["provider"])
	}
	if fields["lookup_kind"] != "session_id" {
		t.Fatalf("lookup_kind field = %#v, want session_id", fields["lookup_kind"])
	}
	if fields["requested_id"] != "missing-session" {
		t.Fatalf("requested_id field = %#v, want missing-session", fields["requested_id"])
	}
	if fields["searched_root"] != root {
		t.Fatalf("searched_root field = %#v, want %q", fields["searched_root"], root)
	}
	if fields["root_configured"] != true {
		t.Fatalf("root_configured field = %#v, want true", fields["root_configured"])
	}
}

func assertHandlerJSONError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string, wantMessage string) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d: %s", rec.Code, wantStatus, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var resp factoryapi.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if string(resp.Code) != wantCode {
		t.Fatalf("error code = %q, want %q", resp.Code, wantCode)
	}
	if resp.Message != wantMessage {
		t.Fatalf("error message = %q, want %q", resp.Message, wantMessage)
	}
	if resp.Family != errorFamilyForStatus(wantStatus) {
		t.Fatalf("error family = %q, want %q", resp.Family, errorFamilyForStatus(wantStatus))
	}
}

func ptrInt(value int) *int {
	return &value
}

func ptrString(value string) *string {
	return &value
}
