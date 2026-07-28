package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

func TestAdapter_LoadDocumentCanceledBeforeRootCallCompletesWithoutInvoke(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := &blockingSettingsRootFake{
		loadDocument: func(
			operatorsettings.LoadDocumentRequest,
		) (operatorsettings.LoadDocumentResult, error) {
			invoked = true
			return operatorsettings.LoadDocumentResult{}, nil
		},
	}
	adapter := operatorsettingshttp.NewAdapterFromRoot(operatorsettingshttp.RootBinding{Settings: fake})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := adapter.LoadDocument(ctx, operatorsettingshttp.LoadDocumentInput{
		Path:            "/tmp/config.json",
		RequireExisting: true,
	})
	if invoked {
		t.Fatal("LoadDocument invoked fake root after request context was canceled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("LoadDocument error = %v, want context.Canceled", err)
	}
}

func TestAdapter_LoadDocumentCanceledDuringRootCallCompletesWithoutHang(t *testing.T) {
	t.Parallel()

	blocker := make(chan struct{})
	started := make(chan struct{}, 1)
	fake := &blockingSettingsRootFake{
		loadDocument: func(
			operatorsettings.LoadDocumentRequest,
		) (operatorsettings.LoadDocumentResult, error) {
			started <- struct{}{}
			<-blocker
			return operatorsettings.LoadDocumentResult{}, nil
		},
	}
	adapter := operatorsettingshttp.NewAdapterFromRoot(operatorsettingshttp.RootBinding{Settings: fake})

	ctx, cancel := context.WithCancel(context.Background())
	defer close(blocker)

	done := make(chan error, 1)
	go func() {
		_, err := adapter.LoadDocument(ctx, operatorsettingshttp.LoadDocumentInput{
			Path:            "/tmp/config.json",
			RequireExisting: true,
		})
		done <- err
	}()

	<-started
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("LoadDocument error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("LoadDocument hung after request context cancellation")
	}
}

func TestAdapter_LoadDocumentDeadlineExceededDuringRootCallCompletesWithoutHang(t *testing.T) {
	t.Parallel()

	blocker := make(chan struct{})
	started := make(chan struct{}, 1)
	fake := &blockingSettingsRootFake{
		loadDocument: func(
			operatorsettings.LoadDocumentRequest,
		) (operatorsettings.LoadDocumentResult, error) {
			started <- struct{}{}
			<-blocker
			return operatorsettings.LoadDocumentResult{}, nil
		},
	}
	adapter := operatorsettingshttp.NewAdapterFromRoot(operatorsettingshttp.RootBinding{Settings: fake})

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	defer close(blocker)

	done := make(chan error, 1)
	go func() {
		_, err := adapter.LoadDocument(ctx, operatorsettingshttp.LoadDocumentInput{
			Path:            "/tmp/config.json",
			RequireExisting: true,
		})
		done <- err
	}()

	<-started

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("LoadDocument error = %v, want context.DeadlineExceeded", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("LoadDocument hung after request context deadline")
	}
}

func TestWriteRootOrInternalError_DoesNotMapCancelToInternalError(t *testing.T) {
	t.Parallel()

	adapter := operatorsettingshttp.NewAdapterFromRoot(operatorsettingshttp.RootBinding{
		Settings: &blockingSettingsRootFake{},
	})
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

	adapter := operatorsettingshttp.NewAdapterFromRoot(operatorsettingshttp.RootBinding{
		Settings: &blockingSettingsRootFake{},
	})
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
