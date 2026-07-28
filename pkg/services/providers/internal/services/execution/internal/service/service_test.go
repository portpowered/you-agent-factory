package service_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
)

const maxTestDiagnosticRunes = 512

func TestNewRejectsInvalidRegistrationSets(t *testing.T) {
	t.Parallel()

	validAttempt := func(
		context.Context,
		providers.ExecuteRequest,
	) (providers.ExecuteResult, error) {
		return providers.ExecuteResult{}, nil
	}
	tests := []struct {
		name          string
		catalog       catalog.Service
		registrations []execution.Registration
	}{
		{name: "nil catalog"},
		{
			name:    "invalid provider",
			catalog: mustCatalog(t),
			registrations: []execution.Registration{{
				Attempt: validAttempt,
			}},
		},
		{
			name:    "nil adapter",
			catalog: mustCatalog(t),
			registrations: []execution.Registration{{
				Provider: providers.IDCodex,
			}},
		},
		{
			name:    "duplicate adapter",
			catalog: mustCatalog(t),
			registrations: []execution.Registration{
				{Provider: providers.IDCodex, Attempt: validAttempt},
				{Provider: providers.IDCodex, Attempt: validAttempt},
			},
		},
		{
			name:    "unknown provider",
			catalog: mustCatalog(t),
			registrations: []execution.Registration{{
				Provider: "missing",
				Attempt:  validAttempt,
			}},
		},
		{
			name:    "non-canonical alias",
			catalog: mustCatalog(t),
			registrations: []execution.Registration{{
				Provider: "cursor",
				Attempt:  validAttempt,
			}},
		},
		{
			name: "unavailable provider",
			catalog: &recordingCatalog{
				registration: func(id providers.ID) (providers.Descriptor, error) {
					return providers.Descriptor{
						ID:           id,
						Availability: providers.AvailabilityCatalogOnly,
					}, nil
				},
			},
			registrations: []execution.Registration{{
				Provider: providers.IDCodex,
				Attempt:  validAttempt,
			}},
		},
		{
			name: "catalog mismatched provider",
			catalog: &recordingCatalog{
				registration: func(providers.ID) (providers.Descriptor, error) {
					return providers.Descriptor{
						ID:           providers.IDClaude,
						Availability: providers.AvailabilitySelectable,
					}, nil
				},
			},
			registrations: []execution.Registration{{
				Provider: providers.IDCodex,
				Attempt:  validAttempt,
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := executionwire.NewService(test.catalog, test.registrations...)
			if err == nil || service != nil {
				t.Fatalf("NewService() = (%v, %v), want construction error", service, err)
			}
		})
	}
}

func TestExecuteValidatesThenResolvesCanonicalAdapterExactlyOnce(t *testing.T) {
	t.Parallel()

	catalogService := mustCatalog(t)
	calls := 0
	var received providers.ExecuteRequest
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{
			Provider: providers.IDCursor,
			Attempt: func(
				_ context.Context,
				request providers.ExecuteRequest,
			) (providers.ExecuteResult, error) {
				calls++
				received = request.Clone()
				request.ResumeSession.ID = "adapter-mutated"
				return providers.ExecuteResult{Content: "done"}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	resume := &providers.SessionRef{
		Provider: providers.IDCursor,
		Kind:     providers.SessionIDKind,
		ID:       "session-1",
	}
	request := providers.ExecuteRequest{
		Provider:         providers.ID("cursor"),
		AttemptID:        "attempt-1",
		SystemPrompt:     "system",
		UserMessage:      "user",
		OutputSchema:     `{"type":"string"}`,
		ResumeSession:    resume,
		WorkingDirectory: "C:/workspace",
		Worktree:         "C:/workspace/tree",
	}

	result, err := executionService.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if result.Content != "done" {
		t.Fatalf("Execute().Content = %q, want done", result.Content)
	}
	if calls != 1 {
		t.Fatalf("adapter calls = %d, want 1", calls)
	}
	want := request.Clone()
	want.Provider = providers.IDCursor
	if !reflect.DeepEqual(received, want) {
		t.Fatalf("adapter request = %#v, want %#v", received, want)
	}
	if received.ResumeSession == request.ResumeSession {
		t.Fatal("adapter received caller-owned ResumeSession pointer")
	}
	if request.ResumeSession.ID != "session-1" {
		t.Fatalf("caller ResumeSession.ID = %q, want session-1", request.ResumeSession.ID)
	}
}

func TestExecuteRejectsInvalidRequestBeforeCatalogOrAdapterIO(t *testing.T) {
	t.Parallel()

	catalogCalls := 0
	adapterCalls := 0
	catalogService := &recordingCatalog{
		get: func(
			_ context.Context,
			_ providers.GetProviderRequest,
		) (providers.GetProviderResult, error) {
			catalogCalls++
			return providers.GetProviderResult{
				Provider: providers.Descriptor{ID: providers.IDCodex},
			}, nil
		},
	}
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(
				_ context.Context,
				_ providers.ExecuteRequest,
			) (providers.ExecuteResult, error) {
				adapterCalls++
				return providers.ExecuteResult{}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	invalidRequests := []providers.ExecuteRequest{
		{AttemptID: "attempt-1"},
		{Provider: providers.IDCodex},
		{
			Provider:  providers.IDCodex,
			AttemptID: "attempt-1",
			ResumeSession: &providers.SessionRef{
				Provider: providers.IDCodex,
				Kind:     providers.SessionIDKind,
			},
		},
	}

	for _, request := range invalidRequests {
		if _, err := executionService.Execute(context.Background(), request); err == nil {
			t.Fatalf("Execute(%#v) error = nil, want validation failure", request)
		}
	}
	if catalogCalls != 0 || adapterCalls != 0 {
		t.Fatalf(
			"invalid requests caused catalog/adapter calls = %d/%d, want 0/0",
			catalogCalls,
			adapterCalls,
		)
	}
}

func TestExecuteNeverFallsBackForUnknownUnavailableOrUnregisteredProvider(t *testing.T) {
	t.Parallel()

	codexCalls := 0
	executionService, err := executionwire.NewService(
		mustCatalog(t),
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(
				_ context.Context,
				_ providers.ExecuteRequest,
			) (providers.ExecuteResult, error) {
				codexCalls++
				return providers.ExecuteResult{}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	tests := []struct {
		name     string
		provider providers.ID
		want     error
	}{
		{name: "unknown", provider: "missing", want: providers.ErrUnknownProvider},
		{name: "catalog unavailable", provider: providers.IDAgy, want: providers.ErrProviderUnavailable},
		{name: "no registered adapter", provider: providers.IDClaude, want: providers.ErrProviderUnavailable},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, executeErr := executionService.Execute(
				context.Background(),
				providers.ExecuteRequest{
					Provider:  test.provider,
					AttemptID: "attempt-1",
				},
			)
			if !errors.Is(executeErr, test.want) {
				t.Fatalf("Execute() error = %v, want %v", executeErr, test.want)
			}
		})
	}
	if codexCalls != 0 {
		t.Fatalf("default codex adapter calls = %d, want 0", codexCalls)
	}
}

func TestExecuteReturnsFirstAdapterFailureWithoutRetry(t *testing.T) {
	t.Parallel()

	calls := 0
	executionService, err := executionwire.NewService(
		mustCatalog(t),
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(
				_ context.Context,
				_ providers.ExecuteRequest,
			) (providers.ExecuteResult, error) {
				calls++
				return providers.ExecuteResult{Content: "must not escape"},
					errors.New("secret native attempt failed")
			},
		},
	)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}

	result, executeErr := executionService.Execute(
		context.Background(),
		providers.ExecuteRequest{
			Provider:  providers.IDCodex,
			AttemptID: "attempt-1",
		},
	)
	if !errors.Is(executeErr, providers.ErrExecuteFailed) {
		t.Fatalf("Execute() error = %v, want ErrExecuteFailed", executeErr)
	}
	var failure providers.ExecuteFailure
	if !errors.As(executeErr, &failure) ||
		failure.Kind != providers.ExecuteFailureKindUnknown {
		t.Fatalf("Execute() error = %#v, want unknown ExecuteFailure", executeErr)
	}
	if strings.Contains(executeErr.Error(), "secret native") {
		t.Fatalf("Execute() error leaked native detail: %v", executeErr)
	}
	if calls != 1 {
		t.Fatalf("adapter calls = %d, want 1", calls)
	}
	if !reflect.DeepEqual(result, providers.ExecuteResult{}) {
		t.Fatalf("failed Execute() result = %#v, want zero result", result)
	}
}

func TestExecuteNormalizesDeclaredFailureKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind providers.ExecuteFailureKind
		want error
	}{
		{kind: providers.ExecuteFailureKindAuthentication, want: providers.ErrExecuteFailed},
		{kind: providers.ExecuteFailureKindInvalidRequest, want: providers.ErrExecuteFailed},
		{kind: providers.ExecuteFailureKindThrottled, want: providers.ErrExecuteFailed},
		{kind: providers.ExecuteFailureKindDependency, want: providers.ErrExecuteFailed},
		{kind: providers.ExecuteFailureKindUnknown, want: providers.ErrExecuteFailed},
		{kind: providers.ExecuteFailureKindCanceled, want: providers.ErrExecuteCancelled},
		{kind: providers.ExecuteFailureKindTimeout, want: providers.ErrExecuteTimeout},
	}

	for _, test := range tests {
		t.Run(string(test.kind), func(t *testing.T) {
			t.Parallel()

			nativeDiagnostics := &providers.ExecuteDiagnostics{
				DurationMillis: -1,
				Metadata: map[string]string{
					"safe":   "kept",
					"stderr": "native payload",
					"detail": "prompt-secret",
				},
			}
			executionService := mustExecutionService(t, func(
				context.Context,
				providers.ExecuteRequest,
			) (providers.ExecuteResult, error) {
				return providers.ExecuteResult{}, providers.ExecuteFailure{
					Kind:        test.kind,
					Message:     strings.Repeat("m", maxTestDiagnosticRunes+1) + " prompt-secret",
					Diagnostics: nativeDiagnostics,
				}
			})
			_, executeErr := executionService.Execute(
				context.Background(),
				providers.ExecuteRequest{
					Provider:     providers.IDCodex,
					AttemptID:    "attempt-1",
					SystemPrompt: "prompt-secret",
				},
			)
			if !errors.Is(executeErr, test.want) {
				t.Fatalf("Execute() error = %v, want %v", executeErr, test.want)
			}
			var failure providers.ExecuteFailure
			if !errors.As(executeErr, &failure) || failure.Kind != test.kind {
				t.Fatalf("Execute() error = %#v, want kind %q", executeErr, test.kind)
			}
			if len([]rune(failure.Message)) > maxTestDiagnosticRunes ||
				strings.Contains(failure.Message, "prompt-secret") {
				t.Fatalf("failure message is unbounded or unsafe: %q", failure.Message)
			}
			if failure.Diagnostics == nil ||
				failure.Diagnostics.DurationMillis != 0 ||
				failure.Diagnostics.Metadata["stderr"] != "<redacted>" ||
				strings.Contains(failure.Diagnostics.Metadata["detail"], "prompt-secret") {
				t.Fatalf("failure diagnostics = %#v, want bounded safe facts", failure.Diagnostics)
			}

			nativeDiagnostics.Metadata["safe"] = "mutated"
			if failure.Diagnostics.Metadata["safe"] != "kept" {
				t.Fatal("normalized failure retained adapter-owned diagnostics")
			}
		})
	}
}

func TestExecuteUsesDeterministicLifecycleFailurePrecedence(t *testing.T) {
	t.Parallel()

	executionService := mustExecutionService(t, func(
		context.Context,
		providers.ExecuteRequest,
	) (providers.ExecuteResult, error) {
		return providers.ExecuteResult{}, execution.AttemptFailure{
			NativeError:     errors.New("native secret"),
			DecodeError:     errors.New("decode secret"),
			FlushError:      errors.New("flush secret"),
			FinalParseError: errors.New("parse secret"),
		}
	})

	for range 20 {
		_, executeErr := executionService.Execute(
			context.Background(),
			providers.ExecuteRequest{
				Provider:  providers.IDCodex,
				AttemptID: "attempt-1",
			},
		)
		var failure providers.ExecuteFailure
		if !errors.As(executeErr, &failure) ||
			failure.Diagnostics == nil ||
			failure.Diagnostics.Metadata["failure_stage"] != "final_parse" {
			t.Fatalf("Execute() error = %#v, want final_parse precedence", executeErr)
		}
		for _, unsafe := range []string{"native secret", "decode secret", "flush secret", "parse secret"} {
			if strings.Contains(executeErr.Error(), unsafe) {
				t.Fatalf("Execute() leaked %q: %v", unsafe, executeErr)
			}
		}
	}
}

func TestExecuteNormalizesEveryPrivateFailureShape(t *testing.T) {
	t.Parallel()

	assertPrivateFailureShapes(t, []privateFailureShape{
		{
			name:       "pointer declared failure",
			attemptErr: &providers.ExecuteFailure{Kind: providers.ExecuteFailureKindAuthentication},
			wantKind:   providers.ExecuteFailureKindAuthentication,
		},
		{
			name: "pointer lifecycle declared failure",
			attemptErr: &execution.AttemptFailure{
				Declared: &providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency},
			},
			wantKind: providers.ExecuteFailureKindDependency,
		},
		{
			name:       "timeout sentinel",
			attemptErr: fmt.Errorf("private wrapper: %w", providers.ErrExecuteTimeout),
			wantKind:   providers.ExecuteFailureKindTimeout,
		},
		{
			name:       "cancel sentinel",
			attemptErr: fmt.Errorf("private wrapper: %w", providers.ErrExecuteCancelled),
			wantKind:   providers.ExecuteFailureKindCanceled,
		},
		{
			name:       "invalid declared kind",
			attemptErr: providers.ExecuteFailure{Kind: "native_private_kind"},
			wantKind:   providers.ExecuteFailureKindUnknown,
		},
		{
			name:       "native lifecycle failure",
			attemptErr: execution.AttemptFailure{NativeError: errors.New("native detail")},
			wantKind:   providers.ExecuteFailureKindUnknown,
			wantStage:  "native",
		},
		{
			name:       "decode lifecycle failure",
			attemptErr: execution.AttemptFailure{DecodeError: errors.New("decode detail")},
			wantKind:   providers.ExecuteFailureKindInvalidRequest,
			wantStage:  "decode",
		},
		{
			name:       "flush lifecycle failure",
			attemptErr: execution.AttemptFailure{FlushError: errors.New("flush detail")},
			wantKind:   providers.ExecuteFailureKindUnknown,
			wantStage:  "flush",
		},
		{
			name:       "empty lifecycle failure",
			attemptErr: execution.AttemptFailure{},
			wantKind:   providers.ExecuteFailureKindUnknown,
		},
		{
			name: "lifecycle deadline wins",
			attemptErr: execution.AttemptFailure{
				NativeError: context.DeadlineExceeded,
				DecodeError: context.Canceled,
			},
			wantKind: providers.ExecuteFailureKindTimeout,
		},
		{
			name: "lifecycle cancellation",
			attemptErr: execution.AttemptFailure{
				FlushError: context.Canceled,
			},
			wantKind: providers.ExecuteFailureKindCanceled,
		},
	})
}

type privateFailureShape struct {
	name       string
	attemptErr error
	wantKind   providers.ExecuteFailureKind
	wantStage  string
}

func assertPrivateFailureShapes(t *testing.T, tests []privateFailureShape) {
	t.Helper()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			executionService := mustExecutionService(t, func(
				context.Context,
				providers.ExecuteRequest,
			) (providers.ExecuteResult, error) {
				return providers.ExecuteResult{}, test.attemptErr
			})
			_, executeErr := executionService.Execute(
				context.Background(),
				providers.ExecuteRequest{
					Provider:  providers.IDCodex,
					AttemptID: "attempt-1",
				},
			)
			var failure providers.ExecuteFailure
			if !errors.As(executeErr, &failure) || failure.Kind != test.wantKind {
				t.Fatalf("Execute() error = %#v, want kind %q", executeErr, test.wantKind)
			}
			if failure.Message == "" {
				t.Fatal("Execute() returned an empty safe failure message")
			}
			if test.wantStage == "" {
				if failure.Diagnostics != nil {
					t.Fatalf("failure diagnostics = %#v, want nil", failure.Diagnostics)
				}
				return
			}
			if failure.Diagnostics == nil ||
				failure.Diagnostics.Metadata["failure_stage"] != test.wantStage {
				t.Fatalf(
					"failure diagnostics = %#v, want stage %q",
					failure.Diagnostics,
					test.wantStage,
				)
			}
		})
	}
}

func TestExecuteSuppliesSafeDefaultForEveryDeclaredFailureKind(t *testing.T) {
	t.Parallel()

	for _, kind := range []providers.ExecuteFailureKind{
		providers.ExecuteFailureKindCanceled,
		providers.ExecuteFailureKindTimeout,
		providers.ExecuteFailureKindAuthentication,
		providers.ExecuteFailureKindInvalidRequest,
		providers.ExecuteFailureKindThrottled,
		providers.ExecuteFailureKindDependency,
		providers.ExecuteFailureKindUnknown,
	} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()

			executionService := mustExecutionService(t, func(
				context.Context,
				providers.ExecuteRequest,
			) (providers.ExecuteResult, error) {
				return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: kind}
			})
			_, executeErr := executionService.Execute(
				context.Background(),
				providers.ExecuteRequest{
					Provider:  providers.IDCodex,
					AttemptID: "attempt-1",
				},
			)
			var failure providers.ExecuteFailure
			if !errors.As(executeErr, &failure) || failure.Message == "" {
				t.Fatalf("Execute() error = %#v, want non-empty safe default", executeErr)
			}
		})
	}
}

func TestExecuteClassifiesInvalidRequestBeforeAdapterIO(t *testing.T) {
	t.Parallel()

	calls := 0
	executionService := mustExecutionService(t, func(
		context.Context,
		providers.ExecuteRequest,
	) (providers.ExecuteResult, error) {
		calls++
		return providers.ExecuteResult{}, nil
	})
	_, executeErr := executionService.Execute(context.Background(), providers.ExecuteRequest{
		Provider: providers.IDCodex,
	})
	var failure providers.ExecuteFailure
	if !errors.As(executeErr, &failure) ||
		failure.Kind != providers.ExecuteFailureKindInvalidRequest ||
		!errors.Is(executeErr, providers.ErrExecuteFailed) {
		t.Fatalf("Execute() error = %#v, want invalid-request ExecuteFailure", executeErr)
	}
	if calls != 0 {
		t.Fatalf("adapter calls = %d, want 0", calls)
	}
}

func TestExecuteReturnsDetachedNormalizedSuccess(t *testing.T) {
	t.Parallel()

	const (
		systemPrompt = "system-prompt-secret"
		userMessage  = "user-message-secret"
	)
	session := &providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "session-1",
	}
	progress := make([]providers.ExecuteProgress, 130)
	for i := range progress {
		phase := "phase-" + strconv.Itoa(i)
		if i == 0 {
			phase = strings.Repeat("p", 65)
		}
		progress[i] = providers.ExecuteProgress{
			Phase:  phase,
			Detail: "safe " + userMessage,
			Metadata: map[string]string{
				"sequence":  "kept",
				"api-token": "native-token",
			},
		}
	}
	metadata := map[string]string{
		"duration_source": "adapter",
		"raw_stdout":      "native output",
		"summary":         "completed " + systemPrompt,
	}
	native := providers.ExecuteResult{
		Content:    "final content",
		SessionRef: session,
		Diagnostics: &providers.ExecuteDiagnostics{
			DurationMillis: 42,
			Progress:       progress,
			Metadata:       metadata,
		},
	}
	executionService, err := executionwire.NewService(
		mustCatalog(t),
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(
				_ context.Context,
				_ providers.ExecuteRequest,
			) (providers.ExecuteResult, error) {
				return native, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	request := providers.ExecuteRequest{
		Provider:     providers.IDCodex,
		AttemptID:    "attempt-1",
		SystemPrompt: systemPrompt,
		UserMessage:  userMessage,
	}

	first, executeErr := executionService.Execute(context.Background(), request)
	if executeErr != nil {
		t.Fatalf("Execute() = %v", executeErr)
	}
	assertNormalizedSuccess(t, first, systemPrompt, userMessage)
	first.SessionRef.ID = "caller-mutated"
	first.Diagnostics.Progress[0].Metadata["sequence"] = "caller-mutated"
	first.Diagnostics.Metadata["duration_source"] = "caller-mutated"
	second, executeErr := executionService.Execute(context.Background(), request)
	if executeErr != nil {
		t.Fatalf("second Execute() = %v", executeErr)
	}
	assertResultIsolation(t, second, native)
}

func assertNormalizedSuccess(
	t *testing.T,
	result providers.ExecuteResult,
	systemPrompt string,
	userMessage string,
) {
	t.Helper()

	if result.Content != "final content" ||
		result.SessionRef == nil ||
		result.SessionRef.ID != "session-1" ||
		result.Diagnostics == nil ||
		result.Diagnostics.DurationMillis != 42 {
		t.Fatalf("Execute() result = %#v, want normalized success", result)
	}
	if len(result.Diagnostics.Progress) != 128 {
		t.Fatalf("progress count = %d, want 128", len(result.Diagnostics.Progress))
	}
	if result.Diagnostics.Progress[1].Phase != "phase-1" ||
		result.Diagnostics.Progress[127].Phase != "phase-127" {
		t.Fatalf("progress order changed: %#v", result.Diagnostics.Progress)
	}
	progress := result.Diagnostics.Progress[0]
	if len([]rune(progress.Phase)) != 64 {
		t.Fatalf("progress phase = %q, want 64 runes", progress.Phase)
	}
	if strings.Contains(progress.Detail, userMessage) {
		t.Fatalf("progress detail leaked prompt: %q", progress.Detail)
	}
	if got := progress.Metadata["api-token"]; got != "<redacted>" {
		t.Fatalf("progress token = %q, want redacted", got)
	}
	if got := result.Diagnostics.Metadata["raw_stdout"]; got != "<redacted>" {
		t.Fatalf("raw stdout = %q, want redacted", got)
	}
	if strings.Contains(result.Diagnostics.Metadata["summary"], systemPrompt) {
		t.Fatalf("diagnostic summary leaked prompt: %q", result.Diagnostics.Metadata["summary"])
	}
}

func assertResultIsolation(
	t *testing.T,
	second providers.ExecuteResult,
	native providers.ExecuteResult,
) {
	t.Helper()

	if second.SessionRef.ID != "session-1" ||
		second.Diagnostics.Progress[0].Metadata["sequence"] != "kept" ||
		second.Diagnostics.Metadata["duration_source"] != "adapter" {
		t.Fatalf("second result retained caller mutation: %#v", second)
	}
	if native.SessionRef.ID != "session-1" ||
		native.Diagnostics.Progress[0].Metadata["sequence"] != "kept" ||
		native.Diagnostics.Metadata["duration_source"] != "adapter" {
		t.Fatalf("adapter-owned result was mutated: %#v", native)
	}
}

func TestExecuteNormalizesDiagnosticEdgeValues(t *testing.T) {
	t.Parallel()

	metadata := map[string]string{"": "discarded"}
	for i := range 33 {
		metadata["fact-"+strconv.Itoa(i)] = "value"
	}
	executionService, err := executionwire.NewService(
		mustCatalog(t),
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(
				_ context.Context,
				request providers.ExecuteRequest,
			) (providers.ExecuteResult, error) {
				if request.AttemptID == "plain" {
					return providers.ExecuteResult{Content: "plain"}, nil
				}
				return providers.ExecuteResult{
					Content: "diagnostic",
					Diagnostics: &providers.ExecuteDiagnostics{
						DurationMillis: -1,
						Progress: []providers.ExecuteProgress{{
							Detail:   "resume session-1 \xff",
							Metadata: nil,
						}},
						Metadata: metadata,
					},
				}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	plain, executeErr := executionService.Execute(
		context.Background(),
		providers.ExecuteRequest{
			Provider:  providers.IDCodex,
			AttemptID: "plain",
		},
	)
	if executeErr != nil || plain.Content != "plain" || plain.Diagnostics != nil {
		t.Fatalf("plain Execute() = (%#v, %v)", plain, executeErr)
	}
	result, executeErr := executionService.Execute(
		context.Background(),
		providers.ExecuteRequest{
			Provider:  providers.IDCodex,
			AttemptID: "diagnostic",
			ResumeSession: &providers.SessionRef{
				Provider: providers.IDCodex,
				Kind:     providers.SessionIDKind,
				ID:       "session-1",
			},
		},
	)
	if executeErr != nil {
		t.Fatalf("diagnostic Execute() = %v", executeErr)
	}
	if result.Diagnostics.DurationMillis != 0 {
		t.Fatalf("DurationMillis = %d, want 0", result.Diagnostics.DurationMillis)
	}
	if len(result.Diagnostics.Metadata) != 31 {
		t.Fatalf("metadata count = %d, want 31 bounded non-empty facts", len(result.Diagnostics.Metadata))
	}
	detail := result.Diagnostics.Progress[0].Detail
	if strings.Contains(detail, "session-1") || !strings.Contains(detail, "<redacted>") {
		t.Fatalf("progress detail = %q, want valid UTF-8 redacted session", detail)
	}
}

func TestExecuteRejectsInvalidOrCrossProviderSuccessSession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		session providers.SessionRef
	}{
		{
			name: "invalid",
			session: providers.SessionRef{
				Provider: providers.IDCodex,
				Kind:     providers.SessionIDKind,
			},
		},
		{
			name: "cross provider",
			session: providers.SessionRef{
				Provider: providers.IDClaude,
				Kind:     providers.SessionIDKind,
				ID:       "session-1",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			executionService, err := executionwire.NewService(
				mustCatalog(t),
				execution.Registration{
					Provider: providers.IDCodex,
					Attempt: func(
						_ context.Context,
						_ providers.ExecuteRequest,
					) (providers.ExecuteResult, error) {
						return providers.ExecuteResult{
							Content:    "must not escape",
							SessionRef: &test.session,
						}, nil
					},
				},
			)
			if err != nil {
				t.Fatalf("NewService() = %v", err)
			}
			result, executeErr := executionService.Execute(
				context.Background(),
				providers.ExecuteRequest{
					Provider:  providers.IDCodex,
					AttemptID: "attempt-1",
				},
			)
			if !errors.Is(executeErr, providers.ErrExecuteFailed) {
				t.Fatalf("Execute() error = %v, want ErrExecuteFailed", executeErr)
			}
			if !reflect.DeepEqual(result, providers.ExecuteResult{}) {
				t.Fatalf("Execute() result = %#v, want zero result", result)
			}
		})
	}
}

func mustCatalog(t *testing.T) catalog.Service {
	t.Helper()

	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	return catalogService
}

func mustExecutionService(
	t *testing.T,
	attempt execution.Attempt,
) execution.Service {
	t.Helper()

	executionService, err := executionwire.NewService(
		mustCatalog(t),
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt:  attempt,
		},
	)
	if err != nil {
		t.Fatalf("executionwire.NewService() = %v", err)
	}
	return executionService
}

type recordingCatalog struct {
	get func(
		context.Context,
		providers.GetProviderRequest,
	) (providers.GetProviderResult, error)
	registration func(providers.ID) (providers.Descriptor, error)
}

func (*recordingCatalog) ResolveProviderID(id providers.ID) (providers.ID, error) {
	return id, nil
}

func (catalog *recordingCatalog) RegistrationProvider(
	id providers.ID,
) (providers.Descriptor, error) {
	if catalog.registration != nil {
		return catalog.registration(id)
	}
	return providers.Descriptor{
		ID:           id,
		Availability: providers.AvailabilitySelectable,
	}, nil
}

func (*recordingCatalog) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (catalog *recordingCatalog) GetProvider(
	ctx context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	return catalog.get(ctx, request)
}
