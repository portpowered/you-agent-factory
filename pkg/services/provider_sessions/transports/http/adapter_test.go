package http

import (
	"errors"
	"testing"
	"time"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

// rootServiceFake is a peer-shaped Provider Sessions root Service used to prove
// the HTTP adapter seam without constructing Codex/Cursor readers, filesystem/SQL
// effect ports, or service-local Wire graphs.
type rootServiceFake struct {
	providersessions.Service

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

func (f *rootServiceFake) Inspect(providersessions.InspectRequest) (providersessions.InspectResult, error) {
	panic("unexpected Inspect call in HTTP adapter root seam test")
}

func (f *rootServiceFake) Project(providersessions.ProjectRequest) (providersessions.ProjectResult, error) {
	panic("unexpected Project call in HTTP adapter root seam test")
}

func TestNewAdapterRequiresInjectedRoot(t *testing.T) {
	if adapter := NewAdapter(nil); adapter != nil {
		t.Fatalf("NewAdapter(nil) = %#v, want nil", adapter)
	}
}

func TestAdapterDetailsInvokesInjectedRoot(t *testing.T) {
	modifiedAt := time.Date(2026, 7, 27, 23, 44, 0, 0, time.UTC)
	text := "adapter seam detail"
	inputTokens := 4
	fake := &rootServiceFake{
		detail: providersessions.Detail{
			ProviderSession: providersessions.Ref{
				Provider: providersessions.ProviderCodex,
				Kind:     providersessions.SessionIDKind,
				ID:       "session-http-1",
			},
			Source: providersessions.SourceMetadata{
				ModifiedAt:   &modifiedAt,
				RelativePath: "2026/07/27/rollout-session-http-1.jsonl",
				SizeBytes:    64,
			},
			Parse: providersessions.ParseSummary{
				EventCount: 1,
				LineCount:  1,
				TokenUsage: &providersessions.TokenUsage{InputTokens: &inputTokens},
			},
			Transcript: []providersessions.TranscriptEntry{{
				Order: 0,
				Text:  &text,
				Type:  providersessions.TranscriptAssistantMessage,
			}},
		},
	}
	adapter := NewAdapter(fake)

	detail, err := adapter.Details("codex", providersessions.SessionIDKind, "session-http-1")
	if err != nil {
		t.Fatalf("Details: %v", err)
	}
	if fake.lastProvider != "codex" || fake.lastKind != providersessions.SessionIDKind || fake.lastID != "session-http-1" {
		t.Fatalf("fake recorded identity = (%q, %q, %q)", fake.lastProvider, fake.lastKind, fake.lastID)
	}
	if detail.ProviderSession.ID != "session-http-1" {
		t.Fatalf("Detail.ProviderSession = %#v", detail.ProviderSession)
	}
	if len(detail.Transcript) != 1 || detail.Transcript[0].Text == nil || *detail.Transcript[0].Text != text {
		t.Fatalf("Detail.Transcript = %#v", detail.Transcript)
	}
}

func TestAdapterDetailsPropagatesTypedRootFailures(t *testing.T) {
	fake := &rootServiceFake{detailErr: providersessions.ErrSessionNotFound}
	adapter := NewAdapter(fake)

	_, err := adapter.Details("cursor", providersessions.SessionIDKind, "missing")
	if !errors.Is(err, providersessions.ErrSessionNotFound) {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
}

func TestAdapterDetailsRequiresInjectedRoot(t *testing.T) {
	var adapter *Adapter

	_, err := adapter.Details("codex", providersessions.SessionIDKind, "session-http-1")
	if err == nil {
		t.Fatal("Details on nil adapter = nil, want error")
	}
}
