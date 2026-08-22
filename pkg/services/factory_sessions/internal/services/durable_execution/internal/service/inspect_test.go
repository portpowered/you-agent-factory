package service

import (
	"context"
	"errors"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/fixtures"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
)

func TestDurableInspectReturnsPublishedSuccessShape(t *testing.T) {
	t.Parallel()

	owner, err := New(newFixtureBackedExecution(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	row := publishedScenarioByPurpose(t, fixtures.FixturePurposeAsyncRunning)
	started, err := owner.StartAsync(
		context.Background(),
		factorysessions.DurableStartRequest(startRequestForPublished(row)),
	)
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	inspect, err := owner.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if inspect.SessionID != row.SessionID {
		t.Fatalf("sessionId = %q, want %q", inspect.SessionID, row.SessionID)
	}
	if inspect.Status != row.LifecycleStatus {
		t.Fatalf("status = %q, want %q", inspect.Status, row.LifecycleStatus)
	}
	if inspect.Links.Session != "/factory-sessions/"+row.SessionID {
		t.Fatalf("session link = %q", inspect.Links.Session)
	}
	if inspect.OrchestratorKind == "" {
		t.Fatalf("GetSession = %#v, want durable inspect with orchestrator kind", inspect)
	}
	if inspect.ResultSummary != nil && inspect.ResultSummary.ResultStatus != string(row.ResultStatus) {
		t.Fatalf("resultSummary status = %q, want %q", inspect.ResultSummary.ResultStatus, row.ResultStatus)
	}
}

func TestDurableInspectUnknownSessionReturnsNotFound(t *testing.T) {
	t.Parallel()

	owner, err := New(newFixtureBackedExecution(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = owner.GetSession(context.Background(), "dur-sess-missing-999")
	if !errors.Is(err, factorysessions.ErrDurableSessionNotFound) {
		t.Fatalf("GetSession missing = %v, want ErrDurableSessionNotFound", err)
	}
}

func TestDurableInspectFailuresStayDistinctFromOtherDurableErrors(t *testing.T) {
	t.Parallel()

	owner, err := New(&inspectFailureStub{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()

	_, err = owner.GetSession(ctx, "dur-sess-missing")
	if !errors.Is(err, factorysessions.ErrDurableSessionNotFound) {
		t.Fatalf("GetSession missing = %v, want ErrDurableSessionNotFound", err)
	}

	_, err = owner.ResumeInterruptedSession(ctx, "dur-sess-missing-checkpoint", factorysessions.DurableResumeRequest{RequestID: "resume-1"})
	var missingCheckpoint *factorysessions.DurableResumeError
	if !errors.As(err, &missingCheckpoint) {
		t.Fatalf("ResumeInterruptedSession = %v, want *DurableResumeError", err)
	}
	if missingCheckpoint.Outcome != factorysessions.DurableResumeOutcomeMissingCheckpoint {
		t.Fatalf("resume outcome = %q, want MISSING_CHECKPOINT", missingCheckpoint.Outcome)
	}
	if errors.Is(err, factorysessions.ErrDurableSessionNotFound) {
		t.Fatal("missing checkpoint must stay distinct from ErrDurableSessionNotFound")
	}

	_, err = owner.StartAsync(ctx, factorysessions.DurableStartRequest{RequestID: "req-invalid-source"})
	var invalidSource *factorysessions.DurableValidationError
	if !errors.As(err, &invalidSource) || invalidSource.Field != "source" {
		t.Fatalf("StartAsync invalid source = %v, want *DurableValidationError field=source", err)
	}
	if errors.Is(err, factorysessions.ErrDurableSessionNotFound) {
		t.Fatal("validation failure must stay distinct from ErrDurableSessionNotFound")
	}
}

type inspectFailureStub struct {
	durableexecution.Service
}

func (s *inspectFailureStub) GetSession(_ context.Context, sessionID string) (factorysessions.SessionReadResult, error) {
	if sessionID == "dur-sess-missing" {
		return factorysessions.SessionReadResult{}, factorysessions.ErrDurableSessionNotFound
	}
	return factorysessions.SessionReadResult{}, errors.New("unexpected inspect request")
}

func (s *inspectFailureStub) ResumeInterruptedSession(_ context.Context, sessionID string, _ factorysessions.DurableResumeRequest) (factorysessions.AsyncStartResult, error) {
	if sessionID == "dur-sess-missing-checkpoint" {
		return factorysessions.AsyncStartResult{}, &factorysessions.DurableResumeError{
			Outcome:   factorysessions.DurableResumeOutcomeMissingCheckpoint,
			Status:    factorysessions.LifecycleStatusPaused,
			Field:     "checkpointSummary",
			Message:   string(factorysessions.DurableResumeOutcomeMissingCheckpoint),
			SessionID: sessionID,
		}
	}
	return factorysessions.AsyncStartResult{}, errors.New("unexpected resume request")
}

func (s *inspectFailureStub) StartAsync(_ context.Context, req factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
	if req.RequestID == "req-invalid-source" {
		return factorysessions.AsyncStartResult{}, &factorysessions.DurableValidationError{
			Field:   "source",
			Message: "source is invalid",
		}
	}
	return factorysessions.AsyncStartResult{}, errors.New("unexpected start request")
}
