package http

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestAdapter_ExecuteInvokesFakeRootAndEncodesSuccess(t *testing.T) {
	t.Parallel()

	var invoked providers.ExecuteRequest
	session := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "session-attempt-1",
	}
	fake := &rootFake{
		execute: func(_ context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
			invoked = request
			return providers.ExecuteResult{
				Content:    "hello-result",
				SessionRef: &session,
				Diagnostics: &providers.ExecuteDiagnostics{
					DurationMillis: 42,
					Progress: []providers.ExecuteProgress{{
						Phase:  "completed",
						Detail: "one attempt finished",
					}},
				},
			}, nil
		},
	}
	adapter := NewAdapter(fake)

	response, err := adapter.Execute(context.Background(), ExecuteInput{
		ProviderID: "codex",
		Body:       strings.NewReader(`{"attemptId":"attempt-1","reasoningEffort":"xhigh","userMessage":"hello"}`),
	})
	if err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if invoked.Provider != providers.IDCodex ||
		invoked.AttemptID != "attempt-1" ||
		invoked.ReasoningEffort != "xhigh" ||
		invoked.UserMessage != "hello" {
		t.Fatalf("invoked request = %#v, want decoded execute request", invoked)
	}
	if response.Content != "hello-result" {
		t.Fatalf("Content = %q, want hello-result", response.Content)
	}
	if response.SessionRef == nil ||
		response.SessionRef.Provider != "codex" ||
		response.SessionRef.Kind != providers.SessionIDKind ||
		response.SessionRef.ID != "session-attempt-1" {
		t.Fatalf("SessionRef = %#v", response.SessionRef)
	}
	if response.Diagnostics == nil ||
		response.Diagnostics.DurationMillis != 42 ||
		len(response.Diagnostics.Progress) != 1 ||
		response.Diagnostics.Progress[0].Phase != "completed" {
		t.Fatalf("Diagnostics = %#v", response.Diagnostics)
	}
}

func TestAdapter_ExecuteRejectsInvalidProviderIDBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
			t.Fatal("fake root must not be invoked for invalid provider id")
			return providers.ExecuteResult{}, nil
		},
	}
	adapter := NewAdapter(fake)

	_, err := adapter.Execute(context.Background(), ExecuteInput{
		ProviderID: "   ",
		Body:       strings.NewReader(`{"attemptId":"attempt-1"}`),
	})
	if err == nil || !errors.Is(err, providers.ErrInvalidID) {
		t.Fatalf("Execute error = %v, want ErrInvalidID", err)
	}
}

func TestAdapter_ExecuteRejectsMissingAttemptIDBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
			t.Fatal("fake root must not be invoked for missing attempt id")
			return providers.ExecuteResult{}, nil
		},
	}
	adapter := NewAdapter(fake)

	_, err := adapter.Execute(context.Background(), ExecuteInput{
		ProviderID: "codex",
		Body:       strings.NewReader(`{"attemptId":"   "}`),
	})
	if err == nil || !errors.Is(err, ErrInvalidExecuteRequest) {
		t.Fatalf("Execute error = %v, want ErrInvalidExecuteRequest", err)
	}
}

func TestAdapter_ExecuteRejectsMalformedBodyBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
			t.Fatal("fake root must not be invoked for malformed body")
			return providers.ExecuteResult{}, nil
		},
	}
	adapter := NewAdapter(fake)

	_, err := adapter.Execute(context.Background(), ExecuteInput{
		ProviderID: "codex",
		Body:       strings.NewReader(`{"attemptId":`),
	})
	if err == nil || !errors.Is(err, ErrInvalidExecuteRequest) {
		t.Fatalf("Execute error = %v, want ErrInvalidExecuteRequest", err)
	}
}

func TestAdapter_ExecuteMapsUnknownProviderFromFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
			return providers.ExecuteResult{}, providers.ErrUnknownProvider
		},
	}
	adapter := NewAdapter(fake)

	_, err := adapter.Execute(context.Background(), ExecuteInput{
		ProviderID: "codex",
		Body:       strings.NewReader(`{"attemptId":"attempt-1"}`),
	})
	if err == nil || !errors.Is(err, providers.ErrUnknownProvider) {
		t.Fatalf("Execute error = %v, want ErrUnknownProvider", err)
	}
}

func TestAdapter_ExecuteMapsExecuteFailureFromFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
			return providers.ExecuteResult{}, providers.ExecuteFailure{
				Kind:    providers.ExecuteFailureKindInvalidRequest,
				Message: "missing user message",
			}
		},
	}
	adapter := NewAdapter(fake)

	_, err := adapter.Execute(context.Background(), ExecuteInput{
		ProviderID: "codex",
		Body:       strings.NewReader(`{"attemptId":"attempt-1"}`),
	})
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) || failure.Kind != providers.ExecuteFailureKindInvalidRequest {
		t.Fatalf("Execute error = %v, want ExecuteFailure invalid_request", err)
	}
}

func TestAdapter_ExecuteMapsExecuteFailedFromFakeRoot(t *testing.T) {
	t.Parallel()

	fake := &rootFake{
		execute: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
			return providers.ExecuteResult{}, providers.ErrExecuteFailed
		},
	}
	adapter := NewAdapter(fake)

	_, err := adapter.Execute(context.Background(), ExecuteInput{
		ProviderID: "codex",
		Body:       strings.NewReader(`{"attemptId":"attempt-1"}`),
	})
	if err == nil || !errors.Is(err, providers.ErrExecuteFailed) {
		t.Fatalf("Execute error = %v, want ErrExecuteFailed", err)
	}
}

func TestAdapter_ExecuteDecodesResumeSession(t *testing.T) {
	t.Parallel()

	var invoked providers.ExecuteRequest
	fake := &rootFake{
		execute: func(_ context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
			invoked = request
			return providers.ExecuteResult{Content: "ok"}, nil
		},
	}
	adapter := NewAdapter(fake)

	_, err := adapter.Execute(context.Background(), ExecuteInput{
		ProviderID: "codex",
		Body: strings.NewReader(`{
			"attemptId":"attempt-1",
			"resumeSession":{"provider":"codex","kind":"session_id","id":"session-prev"}
		}`),
	})
	if err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if invoked.ResumeSession == nil ||
		invoked.ResumeSession.Provider != providers.IDCodex ||
		invoked.ResumeSession.Kind != providers.SessionIDKind ||
		invoked.ResumeSession.ID != "session-prev" {
		t.Fatalf("ResumeSession = %#v", invoked.ResumeSession)
	}
}
