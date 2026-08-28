package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	factoryTargetReadinessOperation = "POST /factory-sessions (validateOnly)"
	factoryTargetReadinessPoll      = 10 * time.Millisecond
)

type factoryTargetReadinessObservation struct {
	Target *factoryapi.FactorySessionTarget
	Result string
}

type factoryTargetReadinessFailureClass string

const (
	factoryTargetReadinessSemanticFailure factoryTargetReadinessFailureClass = "semantic_failure"
	factoryTargetReadinessTransient       factoryTargetReadinessFailureClass = "transient_not_ready"
	factoryTargetReadinessDeadline        factoryTargetReadinessFailureClass = "deadline"
	factoryTargetReadinessCanceled        factoryTargetReadinessFailureClass = "canceled"
)

// factoryTargetReadinessError keeps the first public operation and the latest
// public result together so a failed characterization remains useful after
// test-output truncation. It intentionally carries no response body.
type factoryTargetReadinessError struct {
	Class      factoryTargetReadinessFailureClass
	Operation  string
	TargetID   string
	SessionID  string
	LastResult string
	Cause      error
}

func (err *factoryTargetReadinessError) Error() string {
	if err == nil {
		return ""
	}
	message := fmt.Sprintf(
		"Factory target readiness %s: target=%q session=%q first_public_operation=%q last_result_or_error=%q",
		err.Class,
		err.TargetID,
		err.SessionID,
		err.Operation,
		err.LastResult,
	)
	if err.Cause != nil {
		message += ": " + err.Cause.Error()
	}
	return message
}

func (err *factoryTargetReadinessError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

// TestFactoryTargetReadinessCharacterization records the earliest public
// Factory-target boundary before FT-B05 starts repeated Worker Session reads.
// The happy and semantic cases use the production-composed application; the
// deadline case drives only the package-local waiter with a classified public
// observation because the valid production fixture resolves immediately.
func TestFactoryTargetReadinessCharacterization(t *testing.T) {
	t.Run("FT-RDY-01 fully written explicit session exposes exact target", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		caseFixture := newWorkerSessionsCLICase(t)
		fixture := caseFixture.fixture
		sessionID := caseFixture.openSession(t)
		observed, err := waitForFactoryTargetReadiness(
			ctx,
			sessionID,
			"default",
			15*time.Second,
			func(observeCtx context.Context) (factoryTargetReadinessObservation, error) {
				return observeFactoryTargetReadiness(observeCtx, fixture.baseURL, sessionID, caseFixture.factoryDir)
			},
		)
		if err != nil {
			t.Fatalf("public Factory-target readiness: %v", err)
		}
		assertFactoryTargetReadinessObservation(t, observed, caseFixture.factoryDir)
		assertExplicitFactorySessionTarget(t, fixture.baseURL, sessionID, caseFixture.factoryDir)

		before := captureWorkerSessionsCLIPublicState(t, fixture, sessionID)

		after := captureWorkerSessionsCLIPublicState(t, fixture, sessionID)
		assertWorkerSessionsCLIPublicStateUnchanged(t, before, after)
		if len(caseFixture.sessionIDs) != 1 {
			t.Fatalf("public readiness opened or closed Factory Sessions: %#v", caseFixture.sessionIDs)
		}
	})

	t.Run("FT-RDY-02 malformed target config is an immediate semantic failure", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		caseFixture := newWorkerSessionsCLICase(t)
		fixture := caseFixture.fixture
		factoryConfig := filepath.Join(caseFixture.factoryDir, "factory.json")
		if err := os.WriteFile(factoryConfig, []byte("{"), 0o644); err != nil {
			t.Fatalf("write malformed Factory config: %v", err)
		}
		callStart := fixture.runner.CallCount()
		var checks atomic.Int32
		_, err := waitForFactoryTargetReadiness(
			ctx,
			"<none>",
			"default",
			15*time.Second,
			func(observeCtx context.Context) (factoryTargetReadinessObservation, error) {
				checks.Add(1)
				return observeFactoryTargetReadiness(observeCtx, fixture.baseURL, "<none>", caseFixture.factoryDir)
			},
		)
		if err == nil {
			t.Fatal("malformed Factory target was reported ready")
		}
		var readinessErr *factoryTargetReadinessError
		if !errors.As(err, &readinessErr) || readinessErr.Class != factoryTargetReadinessSemanticFailure {
			t.Fatalf("malformed Factory readiness error = %v, want semantic failure", err)
		}
		if got := checks.Load(); got != 1 {
			t.Fatalf("malformed Factory readiness checks = %d, want immediate single check", got)
		}
		for _, marker := range []string{
			"target=\"default\"",
			"session=\"<none>\"",
			"first_public_operation=\"" + factoryTargetReadinessOperation + "\"",
			"FACTORY_SESSION_CONFIG_LOAD_FAILED",
		} {
			if !strings.Contains(err.Error(), marker) {
				t.Fatalf("malformed Factory readiness error omitted %q: %v", marker, err)
			}
		}
		if got := fixture.runner.CallCount(); got != callStart {
			t.Fatalf("malformed Factory readiness invoked provider route: before=%d after=%d", callStart, got)
		}
		if len(caseFixture.sessionIDs) != 0 {
			t.Fatalf("malformed Factory readiness opened a session: %#v", caseFixture.sessionIDs)
		}
		assertFactorySessionFolderAbsent(t, fixture.baseURL, caseFixture.factoryDir)
	})

	t.Run("FT-TIME-01 readiness deadline reports last public observation", func(t *testing.T) {
		var checks atomic.Int32
		_, err := waitForFactoryTargetReadiness(
			context.Background(),
			"session-timeout",
			"default",
			50*time.Millisecond,
			func(context.Context) (factoryTargetReadinessObservation, error) {
				checks.Add(1)
				return factoryTargetReadinessObservation{Result: "HTTP 503 code=FACTORY_SESSION_NOT_READY"}, &factoryTargetReadinessError{
					Class:      factoryTargetReadinessTransient,
					Operation:  factoryTargetReadinessOperation,
					TargetID:   "default",
					SessionID:  "session-timeout",
					LastResult: "HTTP 503 code=FACTORY_SESSION_NOT_READY",
					Cause:      errors.New("public target discovery remains not ready"),
				}
			},
		)
		if err == nil {
			t.Fatal("readiness waiter returned nil before its deadline")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("readiness deadline error = %v, want context deadline", err)
		}
		var readinessErr *factoryTargetReadinessError
		if !errors.As(err, &readinessErr) || readinessErr.Class != factoryTargetReadinessDeadline {
			t.Fatalf("readiness deadline error = %v, want deadline classification", err)
		}
		if checks.Load() < 1 {
			t.Fatal("readiness deadline made no public observation")
		}
		for _, marker := range []string{
			"target=\"default\"",
			"session=\"session-timeout\"",
			"first_public_operation=\"" + factoryTargetReadinessOperation + "\"",
			"HTTP 503 code=FACTORY_SESSION_NOT_READY",
		} {
			if !strings.Contains(err.Error(), marker) {
				t.Fatalf("readiness deadline error omitted %q: %v", marker, err)
			}
		}
	})

	t.Run("FT-CAN-01 cancellation stops readiness without public mutation", func(t *testing.T) {
		caseFixture := newWorkerSessionsCLICase(t)
		fixture := caseFixture.fixture
		sessionID := caseFixture.openSession(t)
		before := captureWorkerSessionsCLIPublicState(t, fixture, sessionID)
		ctx, cancel := context.WithCancel(context.Background())
		var checks atomic.Int32
		_, err := waitForFactoryTargetReadiness(
			ctx,
			sessionID,
			"default",
			15*time.Second,
			func(observeCtx context.Context) (factoryTargetReadinessObservation, error) {
				checks.Add(1)
				cancel()
				return observeFactoryTargetReadiness(observeCtx, fixture.baseURL, sessionID, caseFixture.factoryDir)
			},
		)
		if err == nil {
			t.Fatal("canceled readiness returned nil error")
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled readiness error = %v, want context cancellation", err)
		}
		var readinessErr *factoryTargetReadinessError
		if !errors.As(err, &readinessErr) || readinessErr.Class != factoryTargetReadinessCanceled {
			t.Fatalf("canceled readiness error = %v, want canceled classification", err)
		}
		if checks.Load() != 1 {
			t.Fatalf("canceled readiness checks = %d, want one bounded observation", checks.Load())
		}
		for _, marker := range []string{
			"target=\"default\"",
			"session=\"" + sessionID + "\"",
			"first_public_operation=\"" + factoryTargetReadinessOperation + "\"",
		} {
			if !strings.Contains(err.Error(), marker) {
				t.Fatalf("canceled readiness error omitted %q: %v", marker, err)
			}
		}
		after := captureWorkerSessionsCLIPublicState(t, fixture, sessionID)
		assertWorkerSessionsCLIPublicStateUnchanged(t, before, after)
	})
}

func observeFactoryTargetReadiness(ctx context.Context, baseURL, sessionID, factoryDir string) (factoryTargetReadinessObservation, error) {
	targetID := "default"
	validateOnly := true
	payload, err := json.Marshal(factoryapi.OpenFactorySessionRequest{
		FolderPath: factoryDir,
		Target: &factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKindDefault,
		},
		ValidateOnly: &validateOnly,
	})
	if err != nil {
		return factoryTargetReadinessObservation{}, &factoryTargetReadinessError{
			Class:      factoryTargetReadinessSemanticFailure,
			Operation:  factoryTargetReadinessOperation,
			TargetID:   targetID,
			SessionID:  sessionID,
			LastResult: "request payload could not be encoded",
			Cause:      err,
		}
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions",
		bytes.NewReader(payload),
	)
	if err != nil {
		return factoryTargetReadinessObservation{Result: "request could not be built"}, &factoryTargetReadinessError{
			Class:      factoryTargetReadinessSemanticFailure,
			Operation:  factoryTargetReadinessOperation,
			TargetID:   targetID,
			SessionID:  sessionID,
			LastResult: "request could not be built",
			Cause:      err,
		}
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		result := "request failed"
		if ctx.Err() != nil {
			result = "request canceled: " + ctx.Err().Error()
		}
		return factoryTargetReadinessObservation{Result: result}, &factoryTargetReadinessError{
			Class:      factoryTargetReadinessTransient,
			Operation:  factoryTargetReadinessOperation,
			TargetID:   targetID,
			SessionID:  sessionID,
			LastResult: result,
			Cause:      err,
		}
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var apiError factoryapi.ErrorResponse
		decodeErr := json.NewDecoder(io.LimitReader(response.Body, 32*1024)).Decode(&apiError)
		code := string(apiError.Code)
		if code == "" {
			code = "UNKNOWN"
		}
		result := fmt.Sprintf("HTTP %d code=%s", response.StatusCode, code)
		message := strings.TrimSpace(apiError.Message)
		if decodeErr != nil || message == "" {
			message = fmt.Sprintf("public readiness endpoint returned HTTP %d", response.StatusCode)
		}
		failureClass := factoryTargetReadinessTransient
		if response.StatusCode >= http.StatusBadRequest && response.StatusCode < http.StatusInternalServerError {
			failureClass = factoryTargetReadinessSemanticFailure
		}
		return factoryTargetReadinessObservation{Result: result}, &factoryTargetReadinessError{
			Class:      failureClass,
			Operation:  factoryTargetReadinessOperation,
			TargetID:   targetID,
			SessionID:  sessionID,
			LastResult: result,
			Cause:      errors.New(message),
		}
	}

	var opened factoryapi.OpenFactorySessionResponse
	if err := json.NewDecoder(response.Body).Decode(&opened); err != nil {
		result := "successful response could not be decoded"
		return factoryTargetReadinessObservation{Result: result}, &factoryTargetReadinessError{
			Class:      factoryTargetReadinessSemanticFailure,
			Operation:  factoryTargetReadinessOperation,
			TargetID:   targetID,
			SessionID:  sessionID,
			LastResult: result,
			Cause:      err,
		}
	}
	if opened.InitsNewFactory != nil && *opened.InitsNewFactory {
		result := "public validation found no runnable Factory target"
		return factoryTargetReadinessObservation{Result: result}, &factoryTargetReadinessError{
			Class:      factoryTargetReadinessSemanticFailure,
			Operation:  factoryTargetReadinessOperation,
			TargetID:   targetID,
			SessionID:  sessionID,
			LastResult: result,
			Cause:      errors.New(result),
		}
	}
	if opened.Targets == nil || len(*opened.Targets) == 0 {
		result := "public validation returned zero runnable Factory targets"
		return factoryTargetReadinessObservation{Result: result}, &factoryTargetReadinessError{
			Class:      factoryTargetReadinessSemanticFailure,
			Operation:  factoryTargetReadinessOperation,
			TargetID:   targetID,
			SessionID:  sessionID,
			LastResult: result,
			Cause:      errors.New(result),
		}
	}
	for _, candidate := range *opened.Targets {
		if candidate.Ref.Kind != factoryapi.FactorySessionTargetRefKindDefault ||
			!sameFactoryPath(candidate.FolderPath, factoryDir) ||
			!sameFactoryPath(candidate.FactoryDir, factoryDir) {
			continue
		}
		selected := candidate
		return factoryTargetReadinessObservation{
			Target: &selected,
			Result: fmt.Sprintf("public target=%q factoryDir=%q", targetID, candidate.FactoryDir),
		}, nil
	}
	result := fmt.Sprintf("public validation returned %d targets but omitted target %q", len(*opened.Targets), targetID)
	return factoryTargetReadinessObservation{Result: result}, &factoryTargetReadinessError{
		Class:      factoryTargetReadinessSemanticFailure,
		Operation:  factoryTargetReadinessOperation,
		TargetID:   targetID,
		SessionID:  sessionID,
		LastResult: result,
		Cause:      errors.New(result),
	}
}

func waitForFactoryTargetReadiness(
	ctx context.Context,
	sessionID, targetID string,
	timeout time.Duration,
	observe func(context.Context) (factoryTargetReadinessObservation, error),
) (factoryTargetReadinessObservation, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if observe == nil {
		return factoryTargetReadinessObservation{}, &factoryTargetReadinessError{
			Class:      factoryTargetReadinessSemanticFailure,
			Operation:  factoryTargetReadinessOperation,
			TargetID:   targetID,
			SessionID:  sessionID,
			LastResult: "readiness observer is unavailable",
			Cause:      errors.New("readiness observer is required"),
		}
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// The public validation endpoint exposes readiness as a state observation,
	// not as a completion subscription, so each bounded attempt re-reads that
	// customer-visible state until success, semantic failure, cancellation, or
	// the deadline.
	ticker := time.NewTicker(factoryTargetReadinessPoll)
	defer ticker.Stop()

	var (
		lastObservation factoryTargetReadinessObservation
		firstFailure    *factoryTargetReadinessError
	)
	for {
		observation, err := observe(waitCtx)
		lastObservation = observation
		if err == nil {
			return observation, nil
		}
		var readinessErr *factoryTargetReadinessError
		if !errors.As(err, &readinessErr) {
			readinessErr = &factoryTargetReadinessError{
				Class:      factoryTargetReadinessTransient,
				Operation:  factoryTargetReadinessOperation,
				TargetID:   targetID,
				SessionID:  sessionID,
				LastResult: observation.Result,
				Cause:      err,
			}
		}
		if firstFailure == nil {
			firstFailure = readinessErr
		}
		if readinessErr.Class == factoryTargetReadinessSemanticFailure {
			return observation, readinessErr
		}

		select {
		case <-waitCtx.Done():
			failureClass := factoryTargetReadinessDeadline
			cause := waitCtx.Err()
			if parentErr := ctx.Err(); parentErr != nil {
				cause = parentErr
				if errors.Is(parentErr, context.Canceled) {
					failureClass = factoryTargetReadinessCanceled
				}
			}
			lastResult := lastObservation.Result
			if strings.TrimSpace(lastResult) == "" {
				lastResult = firstFailure.LastResult
			}
			return lastObservation, &factoryTargetReadinessError{
				Class:      failureClass,
				Operation:  firstFailure.Operation,
				TargetID:   targetID,
				SessionID:  sessionID,
				LastResult: lastResult,
				Cause:      cause,
			}
		case <-ticker.C:
		}
	}
}

func assertExplicitFactorySessionTarget(t *testing.T, baseURL, sessionID, factoryDir string) {
	t.Helper()
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+sessionID,
	)
	session, err := response.AsFactorySession()
	if err != nil {
		t.Fatalf("decode explicit Factory Session %q: %v", sessionID, err)
	}
	if session.Id != sessionID || session.Target.Kind != factoryapi.FactorySessionTargetRefKindDefault ||
		!sameFactoryPath(session.FolderPath, factoryDir) || !sameFactoryPath(session.FactoryDir, factoryDir) {
		t.Fatalf("explicit Factory Session target = %#v, want default target at %q", session, factoryDir)
	}
}

func assertFactoryTargetReadinessObservation(t *testing.T, observation factoryTargetReadinessObservation, factoryDir string) {
	t.Helper()
	if observation.Target == nil {
		t.Fatalf("public Factory-target readiness omitted target: %#v", observation)
	}
	if observation.Target.Ref.Kind != factoryapi.FactorySessionTargetRefKindDefault ||
		!sameFactoryPath(observation.Target.FolderPath, factoryDir) ||
		!sameFactoryPath(observation.Target.FactoryDir, factoryDir) {
		t.Fatalf("public Factory-target readiness = %#v, want exact default target at %q", observation.Target, factoryDir)
	}
	if strings.TrimSpace(observation.Result) == "" {
		t.Fatal("public Factory-target readiness omitted its public result")
	}
}

func sameFactoryPath(left, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}
