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

// TestFactoryTargetReadinessCustomerBehavior proves an explicit Factory
// Session exposes its selected target and malformed customer configuration is
// rejected before any provider dispatch.
func TestFactoryTargetReadinessCustomerBehavior(t *testing.T) {
	t.Parallel()
	t.Run("FT-RDY-01 fully written explicit session exposes exact target", testFactoryTargetReadinessHappy)
	t.Run("FT-RDY-02 malformed target config is an immediate semantic failure", testFactoryTargetReadinessSemanticFailure)
}

func testFactoryTargetReadinessHappy(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	caseFixture := newWorkerSessionsCLICase(t)
	fixture := caseFixture.fixture
	sessionID := caseFixture.openSession(t)
	observed, err := waitForCaseFactoryTargetReadiness(ctx, fixture, caseFixture, sessionID)
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
}

func testFactoryTargetReadinessSemanticFailure(t *testing.T) {
	t.Helper()
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
	providerCalls := 0
	for _, request := range fixture.runner.RequestsSince(callStart) {
		if sameFactoryPath(request.WorkDir, caseFixture.factoryDir) {
			providerCalls++
		}
	}
	assertFactoryTargetReadinessSemanticFailure(t, err, int(checks.Load()), providerCalls)
	if len(caseFixture.sessionIDs) != 0 {
		t.Fatalf("malformed Factory readiness opened a session: %#v", caseFixture.sessionIDs)
	}
	assertFactorySessionFolderAbsent(t, fixture.baseURL, caseFixture.factoryDir)
}

func waitForCaseFactoryTargetReadiness(
	ctx context.Context,
	fixture *workerSessionsCLISharedFixture,
	caseFixture *workerSessionsCLICase,
	sessionID string,
) (factoryTargetReadinessObservation, error) {
	return waitForFactoryTargetReadiness(ctx, sessionID, "default", 15*time.Second,
		func(observeCtx context.Context) (factoryTargetReadinessObservation, error) {
			return observeFactoryTargetReadiness(observeCtx, fixture.baseURL, sessionID, caseFixture.factoryDir)
		})
}

func assertFactoryTargetReadinessSemanticFailure(t *testing.T, err error, checks, providerCalls int) {
	t.Helper()
	if err == nil {
		t.Fatal("malformed Factory target was reported ready")
	}
	var readinessErr *factoryTargetReadinessError
	if !errors.As(err, &readinessErr) || readinessErr.Class != factoryTargetReadinessSemanticFailure {
		t.Fatalf("malformed Factory readiness error = %v, want semantic failure", err)
	}
	if checks != 1 {
		t.Fatalf("malformed Factory readiness checks = %d, want immediate single check", checks)
	}
	assertReadinessErrorMarkers(t, err, "default", "<none>", "FACTORY_SESSION_CONFIG_LOAD_FAILED")
	if providerCalls != 0 {
		t.Fatalf("malformed Factory readiness invoked provider route: calls=%d", providerCalls)
	}
}

func assertReadinessErrorMarkers(t *testing.T, err error, targetID, sessionID, lastResult string) {
	t.Helper()
	for _, marker := range []string{
		"target=\"" + targetID + "\"",
		"session=\"" + sessionID + "\"",
		"first_public_operation=\"" + factoryTargetReadinessOperation + "\"",
	} {
		if !strings.Contains(err.Error(), marker) {
			t.Fatalf("readiness error omitted %q: %v", marker, err)
		}
	}
	if lastResult != "" && !strings.Contains(err.Error(), lastResult) {
		t.Fatalf("readiness error omitted last result %q: %v", lastResult, err)
	}
}

func observeFactoryTargetReadiness(ctx context.Context, baseURL, sessionID, factoryDir string) (factoryTargetReadinessObservation, error) {
	const targetID = "default"
	payload, err := marshalFactoryTargetReadinessRequest(factoryDir)
	if err != nil {
		return factoryTargetReadinessFailure(targetID, sessionID, factoryTargetReadinessSemanticFailure, "request payload could not be encoded", err)
	}
	request, err := newFactoryTargetReadinessRequest(ctx, baseURL, payload)
	if err != nil {
		return factoryTargetReadinessFailure(targetID, sessionID, factoryTargetReadinessSemanticFailure, "request could not be built", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		result := "request failed"
		if ctx.Err() != nil {
			result = "request canceled: " + ctx.Err().Error()
		}
		return factoryTargetReadinessFailure(targetID, sessionID, factoryTargetReadinessTransient, result, err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return decodeFactoryTargetReadinessHTTPError(response, targetID, sessionID)
	}
	return decodeFactoryTargetReadinessSuccess(response, targetID, sessionID, factoryDir)
}

func marshalFactoryTargetReadinessRequest(factoryDir string) ([]byte, error) {
	validateOnly := true
	return json.Marshal(factoryapi.OpenFactorySessionRequest{
		FolderPath: factoryDir,
		Target: &factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKindDefault,
		},
		ValidateOnly: &validateOnly,
	})
}

func newFactoryTargetReadinessRequest(ctx context.Context, baseURL string, payload []byte) (*http.Request, error) {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions",
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	return request, nil
}

func decodeFactoryTargetReadinessHTTPError(response *http.Response, targetID, sessionID string) (factoryTargetReadinessObservation, error) {
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
	return factoryTargetReadinessFailure(targetID, sessionID, failureClass, result, errors.New(message))
}

func decodeFactoryTargetReadinessSuccess(response *http.Response, targetID, sessionID, factoryDir string) (factoryTargetReadinessObservation, error) {
	var opened factoryapi.OpenFactorySessionResponse
	if err := json.NewDecoder(response.Body).Decode(&opened); err != nil {
		return factoryTargetReadinessFailure(targetID, sessionID, factoryTargetReadinessSemanticFailure, "successful response could not be decoded", err)
	}
	if opened.InitsNewFactory != nil && *opened.InitsNewFactory {
		result := "public validation found no runnable Factory target"
		return factoryTargetReadinessFailure(targetID, sessionID, factoryTargetReadinessSemanticFailure, result, errors.New(result))
	}
	if opened.Targets == nil || len(*opened.Targets) == 0 {
		result := "public validation returned zero runnable Factory targets"
		return factoryTargetReadinessFailure(targetID, sessionID, factoryTargetReadinessSemanticFailure, result, errors.New(result))
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
	return factoryTargetReadinessFailure(targetID, sessionID, factoryTargetReadinessSemanticFailure, result, errors.New(result))
}

func factoryTargetReadinessFailure(
	targetID, sessionID string,
	class factoryTargetReadinessFailureClass,
	result string,
	cause error,
) (factoryTargetReadinessObservation, error) {
	return factoryTargetReadinessObservation{Result: result}, &factoryTargetReadinessError{
		Class:      class,
		Operation:  factoryTargetReadinessOperation,
		TargetID:   targetID,
		SessionID:  sessionID,
		LastResult: result,
		Cause:      cause,
	}
}

func normalizeFactoryTargetReadinessError(
	observation factoryTargetReadinessObservation,
	err error,
	targetID, sessionID string,
) *factoryTargetReadinessError {
	var readinessErr *factoryTargetReadinessError
	if errors.As(err, &readinessErr) {
		return readinessErr
	}
	return &factoryTargetReadinessError{
		Class:      factoryTargetReadinessTransient,
		Operation:  factoryTargetReadinessOperation,
		TargetID:   targetID,
		SessionID:  sessionID,
		LastResult: observation.Result,
		Cause:      err,
	}
}

func factoryTargetReadinessWaitFailure(
	ctx, waitCtx context.Context,
	lastObservation factoryTargetReadinessObservation,
	firstFailure *factoryTargetReadinessError,
	targetID, sessionID string,
) (factoryTargetReadinessObservation, error) {
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
		readinessErr := normalizeFactoryTargetReadinessError(observation, err, targetID, sessionID)
		if firstFailure == nil {
			firstFailure = readinessErr
		}
		if readinessErr.Class == factoryTargetReadinessSemanticFailure {
			return observation, readinessErr
		}

		select {
		case <-waitCtx.Done():
			return factoryTargetReadinessWaitFailure(
				ctx,
				waitCtx,
				lastObservation,
				firstFailure,
				targetID,
				sessionID,
			)
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
