package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionshttp "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func waitForValidationContext(
	ctx context.Context,
	_ factorydefinitions.SubmittedDefinitionValidationRequest,
) (factorydefinitions.ValidationResult, error) {
	<-ctx.Done()
	return factorydefinitions.ValidationResult{}, ctx.Err()
}

func TestValidateFactory_CanceledDuringRootCallCompletesWithoutHang(t *testing.T) {
	t.Parallel()

	validation := &blockingValidationFake{validate: waitForValidationContext}
	handler := factorydefinitionshttp.NewHandlerFromRoot(
		factorydefinitionshttp.RootBinding{Validation: validation},
		zap.NewNop(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(
		http.MethodPost,
		"/factory-validations",
		strings.NewReader(minimalValidationFactoryBody),
	).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.ValidateFactory(recorder, req)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ValidateFactory hung after request cancellation")
	}
	if body := recorder.Body.String(); body != "" {
		t.Fatalf("response body = %q, want empty cancel-oriented outcome", body)
	}
}

func TestValidateFactory_CanceledBeforeRootCallCompletesWithoutBody(t *testing.T) {
	t.Parallel()

	validation := &blockingValidationFake{}
	handler := factorydefinitionshttp.NewHandlerFromRoot(
		factorydefinitionshttp.RootBinding{Validation: validation},
		zap.NewNop(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/factory-validations",
		strings.NewReader(minimalValidationFactoryBody),
	).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	handler.ValidateFactory(recorder, req)

	if validation.invoked {
		t.Fatal("ValidateSubmittedDefinition was invoked after request context was canceled")
	}
	if body := recorder.Body.String(); body != "" {
		t.Fatalf("response body = %q, want empty cancel-oriented outcome", body)
	}
}

func TestValidateFactory_DeadlineExceededReturnsGatewayTimeout(t *testing.T) {
	t.Parallel()

	validation := &blockingValidationFake{
		validate: func(ctx context.Context, _ factorydefinitions.SubmittedDefinitionValidationRequest) (factorydefinitions.ValidationResult, error) {
			<-ctx.Done()
			return factorydefinitions.ValidationResult{}, ctx.Err()
		},
	}
	handler := factorydefinitionshttp.NewHandlerFromRoot(
		factorydefinitionshttp.RootBinding{Validation: validation},
		zap.NewNop(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		req := httptest.NewRequest(
			http.MethodPost,
			"/factory-validations",
			strings.NewReader(minimalValidationFactoryBody),
		).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		handler.ValidateFactory(recorder, req)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ValidateFactory hung after request deadline")
	}
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 Gateway Timeout", recorder.Code)
	}
	assertDefinitionsErrorResponse(
		t,
		recorder.Body.Bytes(),
		factoryapi.ErrorFamilyInternalServerError,
		factoryapi.ErrorResponseCodeINTERNALERROR,
		"factory definitions request timed out",
	)
}

func TestGetCurrentFactoryBySessionId_CanceledDuringRootCallCompletesWithoutHang(t *testing.T) {
	t.Parallel()

	root := &blockingCurrentFactoryRootFake{
		get: func(ctx context.Context, _ string) (factorydefinitions.EditableFactory, error) {
			<-ctx.Done()
			return factorydefinitions.EditableFactory{}, ctx.Err()
		},
	}
	handler := factorydefinitionshttp.NewHandlerFromRoot(
		factorydefinitionshttp.RootBinding{Definitions: root},
		zap.NewNop(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/session-alpha/factory", nil).WithContext(ctx)
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.GetCurrentFactoryBySessionId(recorder, req, "session-alpha")
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("GetCurrentFactoryBySessionId hung after request cancellation")
	}
	if body := recorder.Body.String(); body != "" {
		t.Fatalf("response body = %q, want empty cancel-oriented outcome", body)
	}
}

func TestGetCurrentFactoryBySessionId_DeadlineExceededReturnsGatewayTimeout(t *testing.T) {
	t.Parallel()

	root := &blockingCurrentFactoryRootFake{
		get: func(ctx context.Context, _ string) (factorydefinitions.EditableFactory, error) {
			<-ctx.Done()
			return factorydefinitions.EditableFactory{}, ctx.Err()
		},
	}
	handler := factorydefinitionshttp.NewHandlerFromRoot(
		factorydefinitionshttp.RootBinding{Definitions: root},
		zap.NewNop(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.GetCurrentFactoryBySessionId(
			recorder,
			httptest.NewRequest(http.MethodGet, "/factory-sessions/session-alpha/factory", nil).WithContext(ctx),
			"session-alpha",
		)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("GetCurrentFactoryBySessionId hung after request deadline")
	}
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 Gateway Timeout", recorder.Code)
	}
	assertDefinitionsErrorResponse(
		t,
		recorder.Body.Bytes(),
		factoryapi.ErrorFamilyInternalServerError,
		factoryapi.ErrorResponseCodeINTERNALERROR,
		"factory definitions request timed out",
	)
}

func TestDefinitionsRequestContextErrorResponseForTest(t *testing.T) {
	t.Parallel()

	if status, response, ok := factorydefinitionshttp.DefinitionsRequestContextErrorResponseForTest(context.Canceled); !ok || status != 0 || response != nil {
		t.Fatalf("canceled = (%d, %#v, %v), want (0, nil, true)", status, response, ok)
	}

	status, response, ok := factorydefinitionshttp.DefinitionsRequestContextErrorResponseForTest(context.DeadlineExceeded)
	if !ok || status != http.StatusGatewayTimeout {
		t.Fatalf("deadline status = %d, ok = %v, want 504 true", status, ok)
	}
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if !strings.Contains(string(body), "factory definitions request timed out") {
		t.Fatalf("response = %s, want timeout message", body)
	}
}

type blockingValidationFake struct {
	invoked  bool
	validate func(context.Context, factorydefinitions.SubmittedDefinitionValidationRequest) (factorydefinitions.ValidationResult, error)
}

func (fake *blockingValidationFake) ValidateSubmittedDefinition(
	ctx context.Context,
	request factorydefinitions.SubmittedDefinitionValidationRequest,
) (factorydefinitions.ValidationResult, error) {
	fake.invoked = true
	if fake.validate != nil {
		return fake.validate(ctx, request)
	}
	return factorydefinitions.ValidationResult{}, nil
}

var _ factorydefinitions.SubmittedDefinitionValidationOperation = (*blockingValidationFake)(nil)

type blockingCurrentFactoryRootFake struct {
	httpDefinitionsRootFake
	get func(context.Context, string) (factorydefinitions.EditableFactory, error)
}

func (fake *blockingCurrentFactoryRootFake) GetCurrentFactoryForSession(
	ctx context.Context,
	sessionID string,
) (factorydefinitions.EditableFactory, error) {
	if fake.get != nil {
		return fake.get(ctx, sessionID)
	}
	return factorydefinitions.EditableFactory{}, factorydefinitions.ErrCurrentFactoryNotFound
}

func assertDefinitionsErrorResponse(
	t *testing.T,
	body []byte,
	wantFamily factoryapi.ErrorFamily,
	wantCode factoryapi.ErrorResponseCode,
	wantMessage string,
) {
	t.Helper()

	var errResp factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Family != wantFamily {
		t.Fatalf("family = %q, want %q", errResp.Family, wantFamily)
	}
	if errResp.Code != wantCode {
		t.Fatalf("code = %q, want %q", errResp.Code, wantCode)
	}
	if errResp.Message != wantMessage {
		t.Fatalf("message = %q, want %q", errResp.Message, wantMessage)
	}
}
