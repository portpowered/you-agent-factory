package providersessions_test

import (
	"errors"
	"testing"
	"time"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

// rootServiceFake is a peer-shaped Provider Sessions root Service that uses
// only Provider Sessions root contracts. It never imports Codex/Cursor reader
// packages, filesystem/SQL/OS effect ports, Providers catalog/execution types,
// or Workers selection-policy types.
type rootServiceFake struct {
	detail    providersessions.Detail
	detailErr error

	lastProvider string
	lastKind     string
	lastID       string
}

var _ providersessions.Service = (*rootServiceFake)(nil)

func (f *rootServiceFake) Details(provider, kind, id string) (providersessions.Detail, error) {
	f.lastProvider = provider
	f.lastKind = kind
	f.lastID = id
	if f.detailErr != nil {
		return providersessions.Detail{}, f.detailErr
	}
	return f.detail, nil
}

func TestRootService_Characterization_FakeImplementsSingularSeam(t *testing.T) {
	modifiedAt := time.Date(2026, 7, 23, 18, 0, 0, 0, time.UTC)
	text := "hello from peer fake"
	inputTokens := 3
	fake := &rootServiceFake{
		detail: providersessions.Detail{
			ProviderSession: providersessions.Ref{
				Provider: providersessions.ProviderCodex,
				Kind:     providersessions.SessionIDKind,
				ID:       "session-root-1",
			},
			Source: providersessions.SourceMetadata{
				ModifiedAt:   &modifiedAt,
				RelativePath: "2026/07/23/rollout-session-root-1.jsonl",
				SizeBytes:    42,
			},
			Parse: providersessions.ParseSummary{
				EventCount: 1,
				LineCount:  1,
				TokenUsage: &providersessions.TokenUsage{InputTokens: &inputTokens},
				Turns: []providersessions.TurnSummary{{
					Index:      0,
					EventCount: 1,
				}},
			},
			Transcript: []providersessions.TranscriptEntry{{
				Order: 0,
				Text:  &text,
				Type:  providersessions.TranscriptAssistantMessage,
			}},
		},
	}

	var svc providersessions.Service = fake
	detail, err := svc.Details("codex", providersessions.SessionIDKind, "session-root-1")
	if err != nil {
		t.Fatalf("Details: %v", err)
	}
	if fake.lastProvider != "codex" || fake.lastKind != providersessions.SessionIDKind || fake.lastID != "session-root-1" {
		t.Fatalf("fake recorded identity = (%q, %q, %q)", fake.lastProvider, fake.lastKind, fake.lastID)
	}
	if detail.ProviderSession.Provider != providersessions.ProviderCodex ||
		detail.ProviderSession.Kind != providersessions.SessionIDKind ||
		detail.ProviderSession.ID != "session-root-1" {
		t.Fatalf("ProviderSession = %#v", detail.ProviderSession)
	}
	if len(detail.Transcript) != 1 || detail.Transcript[0].Text == nil || *detail.Transcript[0].Text != text {
		t.Fatalf("Transcript = %#v", detail.Transcript)
	}
	if detail.Parse.TokenUsage == nil || detail.Parse.TokenUsage.InputTokens == nil ||
		*detail.Parse.TokenUsage.InputTokens != inputTokens {
		t.Fatalf("Parse.TokenUsage = %#v", detail.Parse.TokenUsage)
	}
	if detail.Source.RelativePath == "" || detail.Source.ModifiedAt == nil {
		t.Fatalf("Source = %#v", detail.Source)
	}
}

func TestRootService_Characterization_TypedFailures(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{name: "unsupported provider", err: providersessions.ErrUnsupportedProvider},
		{name: "unsupported kind", err: providersessions.ErrUnsupportedKind},
		{name: "invalid identifier", err: providersessions.ErrInvalidIdentifier},
		{name: "session not found", err: providersessions.ErrSessionNotFound},
		{name: "ambiguous session", err: providersessions.ErrAmbiguousSessionFile},
		{
			name: "lookup wraps session not found",
			err: &providersessions.LookupError{
				Provider: providersessions.ProviderCursor,
				Root:     "/tmp/cursor-root",
				Err:      providersessions.ErrSessionNotFound,
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fake := &rootServiceFake{detailErr: test.err}
			var svc providersessions.Service = fake
			_, err := svc.Details("cursor", providersessions.SessionIDKind, "missing")
			if !errors.Is(err, test.err) {
				t.Fatalf("err = %v, want typed %v", err, test.err)
			}
			var lookupErr *providersessions.LookupError
			if errors.As(test.err, &lookupErr) {
				var got *providersessions.LookupError
				if !errors.As(err, &got) {
					t.Fatalf("err = %T, want LookupError", err)
				}
				if got.Provider != providersessions.ProviderCursor || got.Root != "/tmp/cursor-root" {
					t.Fatalf("LookupError = %#v", got)
				}
				if !errors.Is(err, providersessions.ErrSessionNotFound) {
					t.Fatalf("LookupError should unwrap to ErrSessionNotFound, got %v", err)
				}
			}
		})
	}
}
