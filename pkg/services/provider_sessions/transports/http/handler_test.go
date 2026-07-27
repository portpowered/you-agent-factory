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

func ptrInt(value int) *int {
	return &value
}

func ptrString(value string) *string {
	return &value
}
