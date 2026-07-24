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

	inspect       providersessions.InspectResult
	inspectErr    error
	lastInspected providersessions.SessionRef

	project       providersessions.ProjectResult
	projectErr    error
	lastProjected providersessions.SessionRef

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

func (f *rootServiceFake) Inspect(req providersessions.InspectRequest) (providersessions.InspectResult, error) {
	f.lastInspected = req.Session
	if f.inspectErr != nil {
		return providersessions.InspectResult{}, f.inspectErr
	}
	return f.inspect, nil
}

func (f *rootServiceFake) Project(req providersessions.ProjectRequest) (providersessions.ProjectResult, error) {
	f.lastProjected = req.Session
	if f.projectErr != nil {
		return providersessions.ProjectResult{}, f.projectErr
	}
	return f.project, nil
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

func TestRootService_Inspect_Characterization_TypedSessionRefSuccess(t *testing.T) {
	modifiedAt := time.Date(2026, 7, 24, 3, 0, 0, 0, time.UTC)
	ref := providersessions.SessionRef{
		Provider: providersessions.ProviderCodex,
		Kind:     providersessions.SessionIDKind,
		ID:       "session-inspect-1",
	}
	fake := &rootServiceFake{
		inspect: providersessions.InspectResult{
			Session: ref,
			Source: providersessions.SourceMetadata{
				ModifiedAt:   &modifiedAt,
				RelativePath: "2026/07/24/rollout-session-inspect-1.jsonl",
				SizeBytes:    17,
			},
		},
	}

	var svc providersessions.Service = fake
	result, err := svc.Inspect(providersessions.InspectRequest{Session: ref})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if fake.lastInspected != ref {
		t.Fatalf("fake recorded SessionRef = %#v, want %#v", fake.lastInspected, ref)
	}
	if result.Session != ref {
		t.Fatalf("InspectResult.Session = %#v, want %#v", result.Session, ref)
	}
	if result.Source.RelativePath == "" || result.Source.ModifiedAt == nil || result.Source.SizeBytes != 17 {
		t.Fatalf("InspectResult.Source = %#v", result.Source)
	}
}

func TestRootService_Inspect_Characterization_TypedFailures(t *testing.T) {
	ref := providersessions.SessionRef{
		Provider: providersessions.ProviderCursor,
		Kind:     providersessions.SessionIDKind,
		ID:       "missing-session",
	}
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
			fake := &rootServiceFake{inspectErr: test.err}
			var svc providersessions.Service = fake
			_, err := svc.Inspect(providersessions.InspectRequest{Session: ref})
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

func TestRootService_Project_Characterization_NormalizedDetailSuccess(t *testing.T) {
	modifiedAt := time.Date(2026, 7, 24, 4, 0, 0, 0, time.UTC)
	assistantText := "projected assistant reply"
	reasoningText := "projected reasoning"
	toolName := "exec_command"
	toolArgs := `{"cmd":"go test"}`
	toolCallID := "call-project-1"
	toolStatus := "completed"
	inputTokens := 11
	outputTokens := 7
	totalTokens := 18
	ref := providersessions.SessionRef{
		Provider: providersessions.ProviderCodex,
		Kind:     providersessions.SessionIDKind,
		ID:       "session-project-1",
	}
	fake := &rootServiceFake{
		project: providersessions.ProjectResult{
			Session: ref,
			Detail: providersessions.Detail{
				ProviderSession: providersessions.Ref{
					Provider: ref.Provider,
					Kind:     ref.Kind,
					ID:       ref.ID,
				},
				Source: providersessions.SourceMetadata{
					ModifiedAt:   &modifiedAt,
					RelativePath: "2026/07/24/rollout-session-project-1.jsonl",
					SizeBytes:    99,
				},
				Parse: providersessions.ParseSummary{
					EventCount: 3,
					LineCount:  3,
					FunctionCalls: []providersessions.FunctionCallSummary{{
						Arguments: &toolArgs,
						CallID:    &toolCallID,
						Name:      &toolName,
						Order:     1,
						Status:    &toolStatus,
						Type:      "function_call",
					}},
					Reasoning: []providersessions.ReasoningSummary{{
						Order:      1,
						SourceType: "agent_reasoning",
						Text:       &reasoningText,
					}},
					TokenUsage: &providersessions.TokenUsage{
						InputTokens:  &inputTokens,
						OutputTokens: &outputTokens,
						TotalTokens:  &totalTokens,
					},
					Turns: []providersessions.TurnSummary{{
						Index:             0,
						EventCount:        3,
						FunctionCallCount: 1,
						ReasoningCount:    1,
					}},
				},
				Transcript: []providersessions.TranscriptEntry{
					{
						Order: 0,
						Text:  &reasoningText,
						Type:  providersessions.TranscriptReasoning,
					},
					{
						Arguments: &toolArgs,
						CallID:    &toolCallID,
						Name:      &toolName,
						Order:     1,
						Status:    &toolStatus,
						Type:      providersessions.TranscriptToolCall,
					},
					{
						Order: 2,
						Text:  &assistantText,
						Type:  providersessions.TranscriptAssistantMessage,
					},
				},
			},
		},
	}

	var svc providersessions.Service = fake
	result, err := svc.Project(providersessions.ProjectRequest{Session: ref})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if fake.lastProjected != ref {
		t.Fatalf("fake recorded SessionRef = %#v, want %#v", fake.lastProjected, ref)
	}
	if result.Session != ref {
		t.Fatalf("ProjectResult.Session = %#v, want %#v", result.Session, ref)
	}
	detail := result.Detail
	if detail.ProviderSession.Provider != ref.Provider ||
		detail.ProviderSession.Kind != ref.Kind ||
		detail.ProviderSession.ID != ref.ID {
		t.Fatalf("Detail.ProviderSession = %#v", detail.ProviderSession)
	}
	if len(detail.Transcript) != 3 {
		t.Fatalf("Transcript len = %d, want 3", len(detail.Transcript))
	}
	if detail.Transcript[0].Type != providersessions.TranscriptReasoning ||
		detail.Transcript[0].Text == nil || *detail.Transcript[0].Text != reasoningText {
		t.Fatalf("Transcript[0] = %#v", detail.Transcript[0])
	}
	if detail.Transcript[1].Type != providersessions.TranscriptToolCall ||
		detail.Transcript[1].Name == nil || *detail.Transcript[1].Name != toolName {
		t.Fatalf("Transcript[1] = %#v", detail.Transcript[1])
	}
	if detail.Transcript[2].Type != providersessions.TranscriptAssistantMessage ||
		detail.Transcript[2].Text == nil || *detail.Transcript[2].Text != assistantText {
		t.Fatalf("Transcript[2] = %#v", detail.Transcript[2])
	}
	if len(detail.Parse.Reasoning) != 1 || detail.Parse.Reasoning[0].Text == nil ||
		*detail.Parse.Reasoning[0].Text != reasoningText {
		t.Fatalf("Parse.Reasoning = %#v", detail.Parse.Reasoning)
	}
	if len(detail.Parse.FunctionCalls) != 1 || detail.Parse.FunctionCalls[0].Name == nil ||
		*detail.Parse.FunctionCalls[0].Name != toolName {
		t.Fatalf("Parse.FunctionCalls = %#v", detail.Parse.FunctionCalls)
	}
	if detail.Parse.TokenUsage == nil ||
		detail.Parse.TokenUsage.InputTokens == nil || *detail.Parse.TokenUsage.InputTokens != inputTokens ||
		detail.Parse.TokenUsage.OutputTokens == nil || *detail.Parse.TokenUsage.OutputTokens != outputTokens ||
		detail.Parse.TokenUsage.TotalTokens == nil || *detail.Parse.TokenUsage.TotalTokens != totalTokens {
		t.Fatalf("Parse.TokenUsage = %#v", detail.Parse.TokenUsage)
	}
	if detail.Source.RelativePath == "" || detail.Source.ModifiedAt == nil || detail.Source.SizeBytes != 99 {
		t.Fatalf("Source = %#v", detail.Source)
	}
}

func TestRootService_Project_Characterization_TypedFailures(t *testing.T) {
	ref := providersessions.SessionRef{
		Provider: providersessions.ProviderCursor,
		Kind:     providersessions.SessionIDKind,
		ID:       "missing-project-session",
	}
	cases := []struct {
		name string
		err  error
	}{
		{name: "unsupported provider", err: providersessions.ErrUnsupportedProvider},
		{name: "unsupported kind", err: providersessions.ErrUnsupportedKind},
		{name: "session not found", err: providersessions.ErrSessionNotFound},
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
			fake := &rootServiceFake{projectErr: test.err}
			var svc providersessions.Service = fake
			_, err := svc.Project(providersessions.ProjectRequest{Session: ref})
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

// TestRootService_Seal_PublishedSlicesOnSingularRoot proves stories 002-003
// slices are both reachable through one named Service using only Provider
// Sessions root contracts (no Codex/Cursor reader, filesystem/SQL/OS effect,
// Providers catalog/execution, or Workers selection-policy imports).
func TestRootService_Seal_PublishedSlicesOnSingularRoot(t *testing.T) {
	modifiedAt := time.Date(2026, 7, 24, 5, 0, 0, 0, time.UTC)
	assistantText := "sealed assistant reply"
	inputTokens := 5
	ref := providersessions.SessionRef{
		Provider: providersessions.ProviderCodex,
		Kind:     providersessions.SessionIDKind,
		ID:       "session-seal-1",
	}
	fake := &rootServiceFake{
		inspect: providersessions.InspectResult{
			Session: ref,
			Source: providersessions.SourceMetadata{
				ModifiedAt:   &modifiedAt,
				RelativePath: "2026/07/24/rollout-session-seal-1.jsonl",
				SizeBytes:    21,
			},
		},
		project: providersessions.ProjectResult{
			Session: ref,
			Detail: providersessions.Detail{
				ProviderSession: providersessions.Ref{
					Provider: ref.Provider,
					Kind:     ref.Kind,
					ID:       ref.ID,
				},
				Source: providersessions.SourceMetadata{
					ModifiedAt:   &modifiedAt,
					RelativePath: "2026/07/24/rollout-session-seal-1.jsonl",
					SizeBytes:    21,
				},
				Parse: providersessions.ParseSummary{
					EventCount: 1,
					LineCount:  1,
					TokenUsage: &providersessions.TokenUsage{InputTokens: &inputTokens},
				},
				Transcript: []providersessions.TranscriptEntry{{
					Order: 0,
					Text:  &assistantText,
					Type:  providersessions.TranscriptAssistantMessage,
				}},
			},
		},
	}

	var svc providersessions.Service = fake

	inspected, err := svc.Inspect(providersessions.InspectRequest{Session: ref})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if inspected.Session != ref {
		t.Fatalf("InspectResult.Session = %#v, want %#v", inspected.Session, ref)
	}
	if inspected.Source.RelativePath == "" || inspected.Source.ModifiedAt == nil {
		t.Fatalf("InspectResult.Source = %#v", inspected.Source)
	}

	projected, err := svc.Project(providersessions.ProjectRequest{Session: ref})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if projected.Session != ref {
		t.Fatalf("ProjectResult.Session = %#v, want %#v", projected.Session, ref)
	}
	if len(projected.Detail.Transcript) != 1 ||
		projected.Detail.Transcript[0].Text == nil ||
		*projected.Detail.Transcript[0].Text != assistantText {
		t.Fatalf("ProjectResult.Detail.Transcript = %#v", projected.Detail.Transcript)
	}
	if projected.Detail.Parse.TokenUsage == nil ||
		projected.Detail.Parse.TokenUsage.InputTokens == nil ||
		*projected.Detail.Parse.TokenUsage.InputTokens != inputTokens {
		t.Fatalf("ProjectResult.Detail.Parse.TokenUsage = %#v", projected.Detail.Parse.TokenUsage)
	}

	if fake.lastInspected != ref || fake.lastProjected != ref {
		t.Fatalf("fake recorded Inspect=%#v Project=%#v, want both %#v", fake.lastInspected, fake.lastProjected, ref)
	}

	fake.inspectErr = providersessions.ErrSessionNotFound
	fake.projectErr = providersessions.ErrUnsupportedProvider
	if _, err := svc.Inspect(providersessions.InspectRequest{Session: ref}); !errors.Is(err, providersessions.ErrSessionNotFound) {
		t.Fatalf("Inspect typed failure = %v, want ErrSessionNotFound", err)
	}
	if _, err := svc.Project(providersessions.ProjectRequest{Session: ref}); !errors.Is(err, providersessions.ErrUnsupportedProvider) {
		t.Fatalf("Project typed failure = %v, want ErrUnsupportedProvider", err)
	}
}
