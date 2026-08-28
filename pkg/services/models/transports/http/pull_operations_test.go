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

func TestAdapter_PullModelInvokesFakeRootWithDecodedName(t *testing.T) {
	t.Parallel()

	var invokedName string
	root := &rootFake{
		pull: func(_ context.Context, name string) (models.PullResult, error) {
			invokedName = name
			return models.PullResult{
				ModelName: name,
				Outcome:   "PULLED",
			}, nil
		},
	}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.PullModel(recorder, httptest.NewRequest(http.MethodPost, "/models/voice/pull", nil), "voice")

	if invokedName != "voice" {
		t.Fatalf("PullModel invoked root with name = %q, want voice", invokedName)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var response factoryapi.ModelPullResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ModelName != "voice" || response.Outcome != factoryapi.ModelPullOutcomePULLED {
		t.Fatalf("response = %#v, want encoded voice pull success", response)
	}
}

func TestAdapter_PullModelRejectsEmptyNameBeforeFakeRoot(t *testing.T) {
	t.Parallel()

	root := &rootFake{
		pull: func(context.Context, string) (models.PullResult, error) {
			t.Fatal("fake root must not be invoked for empty model name")
			return models.PullResult{}, nil
		},
	}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.PullModel(recorder, httptest.NewRequest(http.MethodPost, "/models//pull", nil), "   ")

	assertCatalogHTTPError(t, recorder, http.StatusNotFound, "NOT_FOUND", "model not found")
}

func TestAdapter_PullModelMapsPullErrorToTypedError(t *testing.T) {
	t.Parallel()

	failure := models.PullResult{
		ModelName:          "voice",
		ManagedPullOutcome: "TIMED_OUT",
		ReadinessState:     "FAILED",
	}
	root := &rootFake{
		pull: func(context.Context, string) (models.PullResult, error) {
			return models.PullResult{}, &models.PullError{Result: failure, Cause: errors.New("deadline exceeded")}
		},
	}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.PullModel(recorder, httptest.NewRequest(http.MethodPost, "/models/voice/pull", nil), "voice")

	assertCatalogHTTPError(t, recorder, http.StatusInternalServerError, "INTERNAL_ERROR", pullFailedMessage)
}

func TestAdapter_PullModelProjectsSourceFetchFailureAsTypedError(t *testing.T) {
	t.Parallel()

	failure := models.PullResult{
		ModelName:          "voice",
		ProviderLocality:   string(models.LocalityLocal),
		Outcome:            "PULLED",
		ManagedPullOutcome: "SOURCE_FETCH_FAILED",
		ReadinessState:     "FAILED",
		LifecycleState:     "NOT_INSTALLED",
	}
	root := &rootFake{
		pull: func(context.Context, string) (models.PullResult, error) {
			return models.PullResult{}, &models.PullError{Result: failure, Cause: models.ErrSourceFetchFailed}
		},
	}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.PullModel(recorder, httptest.NewRequest(http.MethodPost, "/models/voice/pull", nil), "voice")

	assertCatalogHTTPError(t, recorder, http.StatusInternalServerError, "INTERNAL_ERROR", pullFailedMessage)
}

func TestModelPullCompatibilityOutcomeProjectsManagedOutcomeTotally(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		managed factoryapi.ManagedRuntimePullOutcome
		want    factoryapi.ModelPullOutcome
	}{
		{name: "installed", managed: factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY, want: factoryapi.ModelPullOutcomePULLED},
		{name: "already present", managed: factoryapi.ManagedRuntimePullOutcomeALREADYPRESENT, want: factoryapi.ModelPullOutcomeALREADYPRESENT},
		{name: "already ready", managed: factoryapi.ManagedRuntimePullOutcomeALREADYREADY, want: factoryapi.ModelPullOutcomeALREADYPRESENT},
		{name: "still loading", managed: factoryapi.ManagedRuntimePullOutcomeSTILLLOADING, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "timed out", managed: factoryapi.ManagedRuntimePullOutcomeTIMEDOUT, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "cancelled", managed: factoryapi.ManagedRuntimePullOutcomeCANCELLED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "source fetch failed", managed: factoryapi.ManagedRuntimePullOutcomeSOURCEFETCHFAILED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "source resolution failed", managed: factoryapi.ManagedRuntimePullOutcomeSOURCERESOLUTIONFAILED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "integrity verification failed", managed: factoryapi.ManagedRuntimePullOutcomeINTEGRITYVERIFICATIONFAILED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "assembly failed", managed: factoryapi.ManagedRuntimePullOutcomeASSEMBLYFAILED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "cache installation failed", managed: factoryapi.ManagedRuntimePullOutcomeCACHEINSTALLATIONFAILED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "readiness evaluation failed", managed: factoryapi.ManagedRuntimePullOutcomeREADINESSEVALUATIONFAILED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "asset preparation failed", managed: factoryapi.ManagedRuntimePullOutcomeASSETPREPARATIONFAILED, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "unsupported", managed: factoryapi.ManagedRuntimePullOutcomeUNSUPPORTEDRUNTIME, want: factoryapi.ModelPullOutcomeFAILED},
		{name: "future outcome", managed: factoryapi.ManagedRuntimePullOutcome("FUTURE_OUTCOME"), want: factoryapi.ModelPullOutcomeFAILED},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := modelPullResponseFromService(models.PullResult{
				ModelName: "voice", Outcome: "PULLED", ManagedPullOutcome: string(tc.managed),
			})
			if result.Outcome != tc.want {
				t.Fatalf("managed outcome %q projected as %q, want %q", tc.managed, result.Outcome, tc.want)
			}
		})
	}
}

func TestAdapter_BuiltInPullSurfacesThroughHTTP(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		models.BuiltInModelNameASR,
		models.BuiltInModelNameTTS,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := &rootFake{
				pull: func(_ context.Context, requested string) (models.PullResult, error) {
					return models.PullResult{
						ModelName:          requested,
						ProviderLocality:   string(models.LocalityLocal),
						Outcome:            "PULLED",
						ManagedPullOutcome: "INSTALLED_SUCCESSFULLY",
						ReadinessState:     "READY",
						LifecycleState:     "INSTALLED",
					}, nil
				},
			}
			handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
			recorder := httptest.NewRecorder()

			handler.PullModel(recorder, httptest.NewRequest(http.MethodPost, "/models/"+name+"/pull", nil), name)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
			}
			var response factoryapi.ModelPullResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if response.ModelName != name || response.Outcome != factoryapi.ModelPullOutcomePULLED ||
				response.ManagedRuntimePull.PullOutcome != factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY ||
				response.ManagedRuntimePull.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
				t.Fatalf("response = %#v, want successful built-in pull", response)
			}
		})
	}
}

func TestAdapter_UnknownPullRemainsNotFound(t *testing.T) {
	t.Parallel()

	root := &rootFake{
		pull: func(context.Context, string) (models.PullResult, error) {
			return models.PullResult{}, models.ErrNotFound
		},
	}
	handler := NewHandlerFromRoot(testRootBinding(root), zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.PullModel(recorder, httptest.NewRequest(http.MethodPost, "/models/unknown-model/pull", nil), "unknown-model")

	assertCatalogHTTPError(t, recorder, http.StatusNotFound, "NOT_FOUND", "model not found")
}

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
			ModelName: "OMNIVOICE_Q4_K_M", Revision: "rev-test",
			CachePath: `C:\models\OMNIVOICE_Q4_K_M\rev-test`, BytesRemoved: 42,
			Readiness: models.AssetReadinessMissing, Outcome: models.AssetRemovalRemoved,
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

func TestAdapter_ScopedPullDecodesIntoRootRequest(t *testing.T) {
	t.Parallel()

	scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:http-scope")
	if err != nil {
		t.Fatalf("parse Models scope: %v", err)
	}
	var pullRequest models.PullModelRequest
	root := &rootFake{
		pullForScope: func(_ context.Context, request models.PullModelRequest) (models.PullResult, error) {
			pullRequest = request
			return models.PullResult{ModelName: request.Name, Outcome: "PULLED"}, nil
		},
	}
	handler := NewHandlerFromRoot(RootBinding{Models: root, Scope: scope}, zap.NewNop())
	recorder := httptest.NewRecorder()

	handler.PullModel(recorder, httptest.NewRequest(http.MethodPost, "/models/voice/pull", nil), "voice")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if pullRequest.Scope != scope || pullRequest.Name != "voice" {
		t.Fatalf("PullModelForScope request = %#v, want scoped voice", pullRequest)
	}
}
