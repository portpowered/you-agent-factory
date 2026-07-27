// Package testutil provides reusable black-box conformance checks for the
// parent-private Providers Execution adapter seam.
package testutil

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
)

const (
	conformanceContent       = "conformance success"
	conformanceSecret        = "conformance-secret"
	conformanceSessionID     = "session-conformance"
	conformanceProgressFacts = 130
	conformanceProgressLimit = 128
)

// Plan describes one controllable native-attempt outcome without prescribing
// how an adapter implements its native protocol.
type Plan struct {
	Result         providers.ExecuteResult
	Failure        error
	WaitForContext bool
	MutateRequest  bool
}

// Observation reports detached facts retained by a conformance adapter.
type Observation struct {
	Calls    int
	Cleanups int
	Requests []providers.ExecuteRequest
}

// Clone returns a detached observation.
func (observation Observation) Clone() Observation {
	cloned := observation
	cloned.Requests = make([]providers.ExecuteRequest, len(observation.Requests))
	for index := range observation.Requests {
		cloned.Requests[index] = observation.Requests[index].Clone()
	}
	return cloned
}

// Adapter exposes one private attempt plus only the controls needed to inspect
// invocation, cancellation propagation, and cleanup.
type Adapter struct {
	Attempt execution.Attempt
	Observe func() Observation
	Started <-chan struct{}
}

// Subject supplies a fresh independently implemented adapter for each
// conformance scenario. Progress-capable subjects receive the ordered and
// bounded progress scenario in addition to the common final-only contract.
type Subject struct {
	NewAdapter       func(Plan) Adapter
	NewRoot          func(execution.Attempt) (providers.Service, error)
	SupportsProgress bool
}

// Run exercises a controllable adapter only through the singular Providers
// root and Providers-owned request, result, progress, diagnostic, SessionRef,
// and failure contracts.
func Run(t *testing.T, subject Subject) {
	t.Helper()
	requireSubject(t, subject)

	t.Run("success with session is detached", func(t *testing.T) {
		runDetachedSuccess(t, subject)
	})
	t.Run("success without session", func(t *testing.T) {
		runSessionlessSuccess(t, subject)
	})
	if subject.SupportsProgress {
		t.Run("progress is ordered bounded and sanitized", func(t *testing.T) {
			runProgressSuccess(t, subject)
		})
	}
	t.Run("declared failure is normalized", func(t *testing.T) {
		runDeclaredFailure(t, subject)
	})
	t.Run("parse failure is normalized and terminal", func(t *testing.T) {
		runParseFailure(t, subject)
	})
	t.Run("cancellation propagates and cleans up", func(t *testing.T) {
		runContextFailure(t, subject, false)
	})
	t.Run("deadline propagates and cleans up", func(t *testing.T) {
		runContextFailure(t, subject, true)
	})
}

func runDetachedSuccess(t *testing.T, subject Subject) {
	t.Helper()
	nativeResult := successResult(true, nil)
	adapter, root := newSubjectRoot(t, subject, Plan{
		Result:        nativeResult,
		MutateRequest: true,
	})
	request := conformanceRequest()
	wantRequest := request.Clone()

	result, err := root.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("Execute(success) error = %v", err)
	}
	assertSuccess(t, result, true)
	if request.ResumeSession.ID != conformanceSessionID {
		t.Fatalf("caller session = %#v, want unchanged", request.ResumeSession)
	}
	assertObservation(t, adapter, 1, 1, wantRequest)

	result.SessionRef.ID = "caller-mutated"
	result.Diagnostics.Metadata["safe"] = "caller-mutated"
	later, err := root.Execute(t.Context(), conformanceRequest())
	if err != nil {
		t.Fatalf("second Execute(success) error = %v", err)
	}
	assertSuccess(t, later, true)
	assertObservation(t, adapter, 2, 2, wantRequest)
}

func runSessionlessSuccess(t *testing.T, subject Subject) {
	t.Helper()
	adapter, root := newSubjectRoot(t, subject, Plan{
		Result: successResult(false, nil),
	})
	result, err := root.Execute(t.Context(), conformanceRequest())
	if err != nil {
		t.Fatalf("Execute(sessionless success) error = %v", err)
	}
	assertSuccess(t, result, false)
	assertObservation(t, adapter, 1, 1, conformanceRequest())
}

func runProgressSuccess(t *testing.T, subject Subject) {
	t.Helper()
	progress := make([]providers.ExecuteProgress, conformanceProgressFacts)
	for index := range progress {
		progress[index] = providers.ExecuteProgress{
			Phase:  progressPhase(index),
			Detail: conformanceSecret,
			Metadata: map[string]string{
				"safe":   progressPhase(index),
				"secret": conformanceSecret,
			},
		}
	}
	adapter, root := newSubjectRoot(t, subject, Plan{
		Result: successResult(true, progress),
	})
	result, err := root.Execute(t.Context(), conformanceRequest())
	if err != nil {
		t.Fatalf("Execute(progress success) error = %v", err)
	}
	if result.Diagnostics == nil ||
		len(result.Diagnostics.Progress) != conformanceProgressLimit {
		t.Fatalf("progress count = %#v, want %d", result.Diagnostics, conformanceProgressLimit)
	}
	for index, fact := range result.Diagnostics.Progress {
		if fact.Phase != progressPhase(index) {
			t.Fatalf("progress[%d].Phase = %q", index, fact.Phase)
		}
		if strings.Contains(fact.Detail, conformanceSecret) ||
			strings.Contains(fact.Metadata["secret"], conformanceSecret) {
			t.Fatalf("progress[%d] leaked request/native-sensitive facts: %#v", index, fact)
		}
	}
	assertObservation(t, adapter, 1, 1, conformanceRequest())
}

func runDeclaredFailure(t *testing.T, subject Subject) {
	t.Helper()
	adapter, root := newSubjectRoot(t, subject, Plan{
		Failure: providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindThrottled,
			Message: "throttled " + conformanceSecret,
		},
	})
	result, err := root.Execute(t.Context(), conformanceRequest())
	assertFailure(t, result, err, providers.ErrExecuteFailed, providers.ExecuteFailureKindThrottled)
	if strings.Contains(err.Error(), conformanceSecret) {
		t.Fatalf("Execute() error leaked sensitive request facts: %v", err)
	}
	assertObservation(t, adapter, 1, 1, conformanceRequest())
}

func runParseFailure(t *testing.T, subject Subject) {
	t.Helper()
	adapter, root := newSubjectRoot(t, subject, Plan{
		Result: successResult(true, nil),
		Failure: execution.AttemptFailure{
			FinalParseError: errors.New("raw native parse payload"),
		},
	})
	result, err := root.Execute(t.Context(), conformanceRequest())
	failure := assertFailure(
		t,
		result,
		err,
		providers.ErrExecuteFailed,
		providers.ExecuteFailureKindUnknown,
	)
	if failure.Diagnostics == nil ||
		failure.Diagnostics.Metadata["failure_stage"] != "final_parse" {
		t.Fatalf("parse failure diagnostics = %#v", failure.Diagnostics)
	}
	if strings.Contains(err.Error(), "raw native") {
		t.Fatalf("Execute() error leaked native parse payload: %v", err)
	}
	assertObservation(t, adapter, 1, 1, conformanceRequest())
}

func runContextFailure(t *testing.T, subject Subject, deadline bool) {
	t.Helper()
	adapter, root := newSubjectRoot(t, subject, Plan{WaitForContext: true})
	ctx, cancel := context.WithCancel(t.Context())
	wantSentinel := providers.ErrExecuteCancelled
	wantKind := providers.ExecuteFailureKindCanceled
	if deadline {
		ctx, cancel = context.WithTimeout(t.Context(), 50*time.Millisecond)
		wantSentinel = providers.ErrExecuteTimeout
		wantKind = providers.ExecuteFailureKindTimeout
	}
	defer cancel()

	outcome := make(chan executeOutcome, 1)
	request := conformanceRequest()
	go func() {
		result, err := root.Execute(ctx, request)
		outcome <- executeOutcome{result: result, err: err}
	}()
	awaitStarted(t, adapter.Started)
	request.ResumeSession.ID = "caller-mutated-in-flight"
	if !deadline {
		cancel()
	}
	select {
	case got := <-outcome:
		assertFailure(t, got.result, got.err, wantSentinel, wantKind)
	case <-time.After(time.Second):
		t.Fatal("Execute() did not stop after its context ended")
	}
	assertObservation(t, adapter, 1, 1, conformanceRequest())
}

type executeOutcome struct {
	result providers.ExecuteResult
	err    error
}

func newSubjectRoot(
	t *testing.T,
	subject Subject,
	plan Plan,
) (Adapter, providers.Service) {
	t.Helper()
	adapter := subject.NewAdapter(plan)
	requireAdapter(t, adapter)
	root, err := subject.NewRoot(adapter.Attempt)
	if err != nil {
		t.Fatalf("construct conformance Providers root: %v", err)
	}
	return adapter, root
}

func conformanceRequest() providers.ExecuteRequest {
	return providers.ExecuteRequest{
		Provider:         providers.IDCodex,
		AttemptID:        "attempt-conformance",
		SystemPrompt:     conformanceSecret,
		UserMessage:      "execute the fixture",
		OutputSchema:     `{"type":"string"}`,
		WorkingDirectory: "C:/conformance",
		Worktree:         "C:/conformance/tree",
		ResumeSession: &providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       conformanceSessionID,
		},
	}
}

func successResult(
	withSession bool,
	progress []providers.ExecuteProgress,
) providers.ExecuteResult {
	result := providers.ExecuteResult{
		Content: conformanceContent,
		Diagnostics: &providers.ExecuteDiagnostics{
			DurationMillis: 42,
			Progress:       progress,
			Metadata:       map[string]string{"safe": "retained"},
		},
	}
	if withSession {
		result.SessionRef = &providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       conformanceSessionID,
		}
	}
	return result
}

func assertSuccess(t *testing.T, result providers.ExecuteResult, withSession bool) {
	t.Helper()
	if result.Content != conformanceContent ||
		result.Diagnostics == nil ||
		result.Diagnostics.DurationMillis != 42 ||
		result.Diagnostics.Metadata["safe"] != "retained" {
		t.Fatalf("Execute() result = %#v", result)
	}
	if withSession {
		if result.SessionRef == nil ||
			result.SessionRef.Provider != providers.IDCodex ||
			result.SessionRef.Kind != providers.SessionIDKind ||
			result.SessionRef.ID != conformanceSessionID {
			t.Fatalf("Execute() SessionRef = %#v", result.SessionRef)
		}
	} else if result.SessionRef != nil {
		t.Fatalf("Execute() SessionRef = %#v, want nil", result.SessionRef)
	}
}

func assertFailure(
	t *testing.T,
	result providers.ExecuteResult,
	err error,
	wantSentinel error,
	wantKind providers.ExecuteFailureKind,
) providers.ExecuteFailure {
	t.Helper()
	if !reflect.DeepEqual(result, providers.ExecuteResult{}) {
		t.Fatalf("failed Execute() result = %#v, want zero result", result)
	}
	if !errors.Is(err, wantSentinel) {
		t.Fatalf("Execute() error = %v, want %v", err, wantSentinel)
	}
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) || failure.Kind != wantKind {
		t.Fatalf("Execute() error = %#v, want ExecuteFailure kind %q", err, wantKind)
	}
	return failure
}

func assertObservation(
	t *testing.T,
	adapter Adapter,
	wantCalls int,
	wantCleanups int,
	wantRequest providers.ExecuteRequest,
) {
	t.Helper()
	observation := adapter.Observe()
	if observation.Calls != wantCalls || observation.Cleanups != wantCleanups {
		t.Fatalf("adapter observation = %#v, want calls/cleanups %d/%d", observation, wantCalls, wantCleanups)
	}
	for index, request := range observation.Requests {
		if !reflect.DeepEqual(request, wantRequest) {
			t.Fatalf("adapter request[%d] = %#v, want %#v", index, request, wantRequest)
		}
		if request.ResumeSession == wantRequest.ResumeSession {
			t.Fatalf("adapter request[%d] retained caller SessionRef pointer", index)
		}
	}
}

func awaitStarted(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("adapter attempt did not start")
	}
}

func progressPhase(index int) string {
	const digits = "0123456789"
	return "phase-" +
		string(digits[(index/100)%10]) +
		string(digits[(index/10)%10]) +
		string(digits[index%10])
}

func requireSubject(t *testing.T, subject Subject) {
	t.Helper()
	if subject.NewAdapter == nil || subject.NewRoot == nil {
		t.Fatal("conformance Subject.NewAdapter and NewRoot are required")
	}
}

func requireAdapter(t *testing.T, adapter Adapter) {
	t.Helper()
	if adapter.Attempt == nil || adapter.Observe == nil || adapter.Started == nil {
		t.Fatal("conformance adapter requires Attempt, Observe, and Started")
	}
}
