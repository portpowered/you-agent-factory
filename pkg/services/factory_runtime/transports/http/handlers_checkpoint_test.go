package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func TestCaptureCheckpoint_ForwardsDecodedFieldsToRoot(t *testing.T) {
	t.Parallel()

	var got factoryruntime.CaptureCheckpointRequest
	fake := &runtimeRootFake{
		captureCheckpoint: func(_ context.Context, req factoryruntime.CaptureCheckpointRequest) (factoryruntime.CaptureCheckpointResult, error) {
			got = req
			return factoryruntime.CaptureCheckpointResult{
				Outcome: factoryruntime.CheckpointOutcomeCaptured,
				Checkpoint: factoryruntime.Checkpoint{
					CheckpointID:  req.CheckpointID,
					SchemaVersion: 1,
					StrategyKind:  "opaque",
					Payload:       []byte("payload"),
				},
			}, nil
		},
	}
	adapter := NewAdapter(fake)

	body := strings.NewReader(`{"checkpointId":" checkpoint-1 "}`)
	rec := httptest.NewRecorder()
	adapter.CaptureCheckpoint(rec, httptest.NewRequest(http.MethodPost, "/runtime/checkpoint/capture", body))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got.CheckpointID != "checkpoint-1" {
		t.Fatalf("capture request = %#v, want trimmed checkpoint id", got)
	}

	var response runtimeCaptureCheckpointHTTPResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Outcome != factoryruntime.CheckpointOutcomeCaptured {
		t.Fatalf("outcome = %q, want CAPTURED", response.Outcome)
	}
	if response.Checkpoint.CheckpointID != "checkpoint-1" || response.Checkpoint.SchemaVersion != 1 {
		t.Fatalf("checkpoint = %#v, want captured checkpoint fields", response.Checkpoint)
	}
}

func TestLoadCheckpoint_ForwardsDecodedFieldsToRoot(t *testing.T) {
	t.Parallel()

	var got factoryruntime.LoadCheckpointRequest
	fake := &runtimeRootFake{
		loadCheckpoint: func(_ context.Context, req factoryruntime.LoadCheckpointRequest) (factoryruntime.LoadCheckpointResult, error) {
			got = req
			return factoryruntime.LoadCheckpointResult{
				Outcome: factoryruntime.CheckpointOutcomeLoaded,
				Checkpoint: factoryruntime.Checkpoint{
					CheckpointID:  req.CheckpointID,
					SchemaVersion: req.ExpectedSchemaVersion,
				},
				Compatible: true,
			}, nil
		},
	}
	adapter := NewAdapter(fake)

	body := strings.NewReader(`{"checkpointId":"checkpoint-1","expectedSchemaVersion":2}`)
	rec := httptest.NewRecorder()
	adapter.LoadCheckpoint(rec, httptest.NewRequest(http.MethodPost, "/runtime/checkpoint/load", body))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got.CheckpointID != "checkpoint-1" || got.ExpectedSchemaVersion != 2 {
		t.Fatalf("load request = %#v, want decoded checkpoint fields", got)
	}

	var response runtimeLoadCheckpointHTTPResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Outcome != factoryruntime.CheckpointOutcomeLoaded || !response.Compatible {
		t.Fatalf("response = %#v, want LOADED compatible checkpoint", response)
	}
}

func TestRestoreCheckpoint_ForwardsDecodedFieldsToRoot(t *testing.T) {
	t.Parallel()

	var got factoryruntime.RestoreCheckpointRequest
	fake := &runtimeRootFake{
		restoreCheckpoint: func(_ context.Context, req factoryruntime.RestoreCheckpointRequest) (factoryruntime.RestoreCheckpointResult, error) {
			got = req
			return factoryruntime.RestoreCheckpointResult{
				Outcome:      factoryruntime.CheckpointOutcomeRestored,
				CheckpointID: req.Checkpoint.CheckpointID,
			}, nil
		},
	}
	adapter := NewAdapter(fake)

	body := strings.NewReader(`{
		"checkpoint":{
			"checkpointId":"checkpoint-1",
			"schemaVersion":1,
			"strategyKind":"opaque",
			"payload":"cGF5bG9hZA=="
		}
	}`)
	rec := httptest.NewRecorder()
	adapter.RestoreCheckpoint(rec, httptest.NewRequest(http.MethodPost, "/runtime/checkpoint/restore", body))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got.Checkpoint.CheckpointID != "checkpoint-1" || got.Checkpoint.SchemaVersion != 1 ||
		got.Checkpoint.StrategyKind != "opaque" || string(got.Checkpoint.Payload) != "payload" {
		t.Fatalf("restore request = %#v, want decoded checkpoint fields", got)
	}

	var response runtimeRestoreCheckpointHTTPResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Outcome != factoryruntime.CheckpointOutcomeRestored || response.CheckpointID != "checkpoint-1" {
		t.Fatalf("response = %#v, want RESTORED checkpoint id", response)
	}
}

func TestCheckpointHandlers_MapTypedFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		programmed error
		invoke     func(*Adapter, *httptest.ResponseRecorder)
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{
			name:       "capture checkpoint not found",
			programmed: factoryruntime.ErrCheckpointNotFound,
			invoke: func(adapter *Adapter, rec *httptest.ResponseRecorder) {
				adapter.CaptureCheckpoint(
					rec,
					httptest.NewRequest(
						http.MethodPost,
						"/runtime/checkpoint/capture",
						strings.NewReader(`{"checkpointId":"missing"}`),
					),
				)
			},
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
			wantMsg:    "factory runtime checkpoint not found",
		},
		{
			name:       "load corrupt checkpoint",
			programmed: factoryruntime.ErrCorruptCheckpoint,
			invoke: func(adapter *Adapter, rec *httptest.ResponseRecorder) {
				adapter.LoadCheckpoint(
					rec,
					httptest.NewRequest(
						http.MethodPost,
						"/runtime/checkpoint/load",
						strings.NewReader(`{"checkpointId":"bad"}`),
					),
				)
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
			wantMsg:    "factory runtime checkpoint is corrupt",
		},
		{
			name:       "restore incompatible checkpoint",
			programmed: factoryruntime.ErrIncompatibleCheckpoint,
			invoke: func(adapter *Adapter, rec *httptest.ResponseRecorder) {
				adapter.RestoreCheckpoint(
					rec,
					httptest.NewRequest(
						http.MethodPost,
						"/runtime/checkpoint/restore",
						strings.NewReader(`{"checkpoint":{"checkpointId":"checkpoint-1","schemaVersion":1}}`),
					),
				)
			},
			wantStatus: http.StatusConflict,
			wantCode:   "CONFLICT",
			wantMsg:    "factory runtime checkpoint is incompatible",
		},
		{
			name:       "capture capability unavailable",
			programmed: factoryruntime.ErrCapabilityUnavailable,
			invoke: func(adapter *Adapter, rec *httptest.ResponseRecorder) {
				adapter.CaptureCheckpoint(
					rec,
					httptest.NewRequest(
						http.MethodPost,
						"/runtime/checkpoint/capture",
						strings.NewReader(`{"checkpointId":"checkpoint-1"}`),
					),
				)
			},
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "SERVICE_UNAVAILABLE",
			wantMsg:    "factory runtime capability is unavailable",
		},
		{
			name:       "restore not running",
			programmed: factoryruntime.ErrNotRunning,
			invoke: func(adapter *Adapter, rec *httptest.ResponseRecorder) {
				adapter.RestoreCheckpoint(
					rec,
					httptest.NewRequest(
						http.MethodPost,
						"/runtime/checkpoint/restore",
						strings.NewReader(`{"checkpoint":{"checkpointId":"checkpoint-1","schemaVersion":1}}`),
					),
				)
			},
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "SERVICE_UNAVAILABLE",
			wantMsg:    "factory runtime is not running",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := &runtimeRootFake{
				captureCheckpoint: func(context.Context, factoryruntime.CaptureCheckpointRequest) (factoryruntime.CaptureCheckpointResult, error) {
					return factoryruntime.CaptureCheckpointResult{}, tc.programmed
				},
				loadCheckpoint: func(context.Context, factoryruntime.LoadCheckpointRequest) (factoryruntime.LoadCheckpointResult, error) {
					return factoryruntime.LoadCheckpointResult{}, tc.programmed
				},
				restoreCheckpoint: func(context.Context, factoryruntime.RestoreCheckpointRequest) (factoryruntime.RestoreCheckpointResult, error) {
					return factoryruntime.RestoreCheckpointResult{}, tc.programmed
				},
			}
			adapter := NewAdapter(fake)
			rec := httptest.NewRecorder()
			tc.invoke(adapter, rec)
			assertErrorResponse(t, rec, tc.wantStatus, tc.wantCode, tc.wantMsg)
		})
	}
}

func TestCaptureCheckpoint_RejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&runtimeRootFake{})
	rec := httptest.NewRecorder()
	adapter.CaptureCheckpoint(rec, httptest.NewRequest(http.MethodPost, "/runtime/checkpoint/capture", strings.NewReader("{")))
	assertErrorResponse(t, rec, http.StatusBadRequest, "BAD_REQUEST", "invalid request payload")
}
