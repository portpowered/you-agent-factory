package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestRemoveModelHandlerMapsDetachedResultAndScope(t *testing.T) {
	t.Parallel()

	scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:http-remove")
	if err != nil {
		t.Fatalf("parse scope: %v", err)
	}
	root := &rootFake{remove: func(_ context.Context, request models.RemoveModelAssetsRequest) (models.RemoveModelAssetsResult, error) {
		if request.Scope != scope || request.Name != "OMNIVOICE_Q4_K_M" {
			t.Fatalf("remove request = %#v, want scoped model request", request)
		}
		return models.RemoveModelAssetsResult{
			ModelName:    "OMNIVOICE_Q4_K_M",
			Revision:     "rev-test",
			CachePath:    `C:\models\OMNIVOICE_Q4_K_M\rev-test`,
			BytesRemoved: 42,
			Readiness:    models.AssetReadinessMissing,
			Outcome:      models.AssetRemovalRemoved,
		}, nil
	}}
	handler := NewHandlerFromRoot(RootBinding{Models: root, Scope: scope}, zap.NewNop())
	request := httptest.NewRequest(http.MethodDelete, "/models/OMNIVOICE_Q4_K_M", nil)
	response := httptest.NewRecorder()
	handler.RemoveModel(response, request, "OMNIVOICE_Q4_K_M")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	var body factoryapi.ModelRemoveResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode remove response: %v", err)
	}
	if body.ModelName != "OMNIVOICE_Q4_K_M" || body.Revision != "rev-test" ||
		body.BytesRemoved != 42 || body.Outcome != factoryapi.REMOVED {
		t.Fatalf("remove response = %#v", body)
	}
}

func TestRemoveModelHandlerUsesSpecificCacheErrors(t *testing.T) {
	t.Parallel()

	scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:http-remove-error")
	if err != nil {
		t.Fatalf("parse scope: %v", err)
	}
	root := &rootFake{remove: func(context.Context, models.RemoveModelAssetsRequest) (models.RemoveModelAssetsResult, error) {
		return models.RemoveModelAssetsResult{}, errors.Join(models.ErrModelCacheInUse, errors.New("held"))
	}}
	handler := NewHandlerFromRoot(RootBinding{Models: root, Scope: scope}, zap.NewNop())
	response := httptest.NewRecorder()
	handler.RemoveModel(response, httptest.NewRequest(http.MethodDelete, "/models/model", nil), "model")
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want conflict; body=%s", response.Code, response.Body.String())
	}
	var body factoryapi.ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Code != factoryapi.ErrorResponseCodeMODELCACHEINUSE {
		t.Fatalf("error code = %q, want MODEL_CACHE_IN_USE", body.Code)
	}
}
