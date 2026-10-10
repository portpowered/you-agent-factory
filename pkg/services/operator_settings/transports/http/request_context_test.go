package http_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorsettingshttp "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestSettingsRequestContextErrorResponseForTest(t *testing.T) {
	t.Parallel()

	if status, response, ok := operatorsettingshttp.SettingsRequestContextErrorResponseForTest(context.Canceled); !ok || status != 0 || response != nil {
		t.Fatalf("canceled = (%d, %#v, %v), want (0, nil, true)", status, response, ok)
	}

	status, response, ok := operatorsettingshttp.SettingsRequestContextErrorResponseForTest(context.DeadlineExceeded)
	if !ok || status != http.StatusGatewayTimeout {
		t.Fatalf("deadline status = %d, ok = %v, want 504 true", status, ok)
	}
	errResp, ok := response.(factoryapi.ErrorResponse)
	if !ok {
		t.Fatalf("deadline response = %#v, want ErrorResponse", response)
	}
	if errResp.Message != "operator settings request timed out" ||
		errResp.Family != factoryapi.ErrorFamilyInternalServerError ||
		errResp.Code != factoryapi.ErrorResponseCodeINTERNALERROR {
		t.Fatalf("deadline response = %#v, want timeout message", errResp)
	}
}

func TestRootErrorResponse_MapsRequestContextFailures(t *testing.T) {
	t.Parallel()

	status, response, ok := operatorsettingshttp.SettingsRootErrorResponseForTest(context.Canceled)
	if !ok || status != 0 || response.Message != "" {
		t.Fatalf("canceled = (%d, %#v, %v), want handled cancel outcome", status, response, ok)
	}

	status, response, ok = operatorsettingshttp.SettingsRootErrorResponseForTest(context.DeadlineExceeded)
	if !ok || status != http.StatusGatewayTimeout || response.Message != "operator settings request timed out" {
		t.Fatalf("deadline = (%d, %#v, %v), want 504 timeout outcome", status, response, ok)
	}
}

func TestWriteRootOrInternalError_DoesNotMapCancelToInternalError(t *testing.T) {
	t.Parallel()

	adapter := operatorsettingshttp.NewAdapter(&blockingSettingsRootFake{})
	recorder := httptest.NewRecorder()

	operatorsettingshttp.WriteRootOrInternalErrorForTest(adapter, recorder, context.Canceled)

	if body := recorder.Body.String(); body != "" {
		t.Fatalf("response body = %q, want empty cancel-oriented outcome", body)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want default recorder status without encoded body", recorder.Code)
	}
}

func TestWriteRootOrInternalError_MapsDeadlineExceededToGatewayTimeout(t *testing.T) {
	t.Parallel()

	adapter := operatorsettingshttp.NewAdapter(&blockingSettingsRootFake{})
	recorder := httptest.NewRecorder()

	operatorsettingshttp.WriteRootOrInternalErrorForTest(adapter, recorder, context.DeadlineExceeded)

	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 Gateway Timeout", recorder.Code)
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Message != "operator settings request timed out" ||
		response.Code != factoryapi.ErrorResponseCodeINTERNALERROR {
		t.Fatalf("response = %s, want timeout ErrorResponse", recorder.Body.String())
	}
}

type blockingSettingsRootFake struct {
	operatorsettings.Service

	loadDocument        func(operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error)
	applyDocumentUpdate func(operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error)
	resolveEffective    func(operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error)
}

func (fake *blockingSettingsRootFake) LoadDocument(
	request operatorsettings.LoadDocumentRequest,
) (operatorsettings.LoadDocumentResult, error) {
	if fake.loadDocument != nil {
		return fake.loadDocument(request)
	}
	return operatorsettings.LoadDocumentResult{}, operatorsettings.ErrDocumentNotFound
}

func (fake *blockingSettingsRootFake) ApplyDocumentUpdate(
	request operatorsettings.ApplyDocumentUpdateRequest,
) (operatorsettings.ApplyDocumentUpdateResult, error) {
	if fake.applyDocumentUpdate != nil {
		return fake.applyDocumentUpdate(request)
	}
	return operatorsettings.ApplyDocumentUpdateResult{}, operatorsettings.ErrDocumentMalformed
}

func (fake *blockingSettingsRootFake) ResolveEffective(
	request operatorsettings.ResolveEffectiveRequest,
) (operatorsettings.ResolveEffectiveResult, error) {
	if fake.resolveEffective != nil {
		return fake.resolveEffective(request)
	}
	return operatorsettings.ResolveEffectiveResult{}, operatorsettings.ErrResolutionInvalidInput
}

// outcomeContext signals a deadline deterministically; wall time is only a
// generous deadlock ceiling, never the trigger for the behavior under test.
type outcomeContext struct {
	context.Context
	outcome error
}

func (ctx outcomeContext) Err() error {
	if ctx.Context.Err() != nil {
		return ctx.outcome
	}
	return nil
}

func contextOperations() map[string]func(*operatorsettingshttp.Adapter, context.Context) error {
	return map[string]func(*operatorsettingshttp.Adapter, context.Context) error{
		"load": func(a *operatorsettingshttp.Adapter, ctx context.Context) error {
			_, err := a.LoadDocument(ctx, operatorsettingshttp.LoadDocumentInput{Path: "/tmp/config.json", RequireExisting: true})
			return err
		},
		"update": func(a *operatorsettingshttp.Adapter, ctx context.Context) error {
			model := "gpt-5"
			_, err := a.ApplyDocumentUpdate(ctx, operatorsettingshttp.ApplyDocumentUpdateInput{Path: "/tmp/config.json", Model: &model})
			return err
		},
		"resolve": func(a *operatorsettingshttp.Adapter, ctx context.Context) error {
			_, err := a.ResolveEffective(ctx, operatorsettingshttp.ResolveEffectiveInput{})
			return err
		},
	}
}

func contextRoot(invoke func()) *blockingSettingsRootFake {
	return &blockingSettingsRootFake{
		loadDocument: func(operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
			invoke()
			return operatorsettings.LoadDocumentResult{}, nil
		},
		applyDocumentUpdate: func(operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error) {
			invoke()
			return operatorsettings.ApplyDocumentUpdateResult{}, nil
		},
		resolveEffective: func(operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error) {
			invoke()
			return operatorsettings.ResolveEffectiveResult{}, nil
		},
	}
}

func TestAdapter_ContextEndedBeforeOwnerEntry(t *testing.T) {
	t.Parallel()
	for name, invoke := range contextOperations() {
		for _, outcome := range []error{context.Canceled, context.DeadlineExceeded} {
			t.Run(fmt.Sprintf("%s/%s", name, outcome), func(t *testing.T) {
				t.Parallel()
				var calls atomic.Int32
				adapter := operatorsettingshttp.NewAdapter(contextRoot(func() { calls.Add(1) }))
				base, cancel := context.WithCancel(context.Background())
				cancel()
				err := invoke(adapter, outcomeContext{Context: base, outcome: outcome})
				//nolint:errorlint // Exact request-context cause identity is the adapter contract.
				if err != outcome || calls.Load() != 0 {
					t.Fatalf("error = %v, calls = %d; want %v without owner entry", err, calls.Load(), outcome)
				}
			})
		}
	}
}

func TestAdapter_ContextEndsBeforeBlockedOwnerRelease(t *testing.T) {
	t.Parallel()
	for name, invoke := range contextOperations() {
		for _, outcome := range []error{context.Canceled, context.DeadlineExceeded} {
			t.Run(fmt.Sprintf("%s/%s", name, outcome), func(t *testing.T) {
				t.Parallel()
				assertBlockedOwnerContext(t, invoke, outcome)
			})
		}
	}
}

func assertBlockedOwnerContext(t *testing.T, invoke func(*operatorsettingshttp.Adapter, context.Context) error, outcome error) {
	t.Helper()
	entered, release, joined := make(chan struct{}), make(chan struct{}), make(chan struct{})
	adapter := operatorsettingshttp.NewAdapter(contextRoot(func() {
		close(entered)
		<-release
		close(joined)
	}))
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	//nolint:contextcheck // The test wraps its cancellable parent to signal DeadlineExceeded deterministically.
	go func() { done <- invoke(adapter, outcomeContext{Context: base, outcome: outcome}) }()
	// Always release and join the fake, including assertion failures.
	defer func() {
		close(release)
		select {
		case <-joined:
		case <-time.After(10 * time.Second):
			t.Error("owner did not join after release")
		}
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("owner did not enter")
	}
	cancel()
	select {
	case err := <-done:
		//nolint:errorlint // Exact request-context cause identity is the adapter contract.
		if err != outcome {
			t.Fatalf("error = %v, want exact %v", err, outcome)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("adapter did not return before owner release")
	}
	select {
	case <-joined:
		t.Fatal("owner completed before test release")
	default:
	}
}
