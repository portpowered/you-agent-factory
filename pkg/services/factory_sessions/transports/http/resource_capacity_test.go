package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestSetFactorySessionResourceCapacityMapsStableIDAndResponse(t *testing.T) {
	root := &httpSessionsRootFake{
		onApplyLiveChange: func(_ context.Context, sessionID string, request factorysessions.LiveChangeRequest) (factorysessions.LiveChangeResult, error) {
			if sessionID != "session-beta" || request.TargetID != "reviewers" || request.Operation != "resource.capacity.set" || string(request.RequestedValue) != "8" {
				t.Fatalf("request = %#v, want stable resource ID capacity operation", request)
			}
			return factorysessions.LiveChangeResult{
				SessionID: "session-beta", RequestID: request.RequestID, ChangeID: request.ChangeID,
				Outcome: factorysessions.LiveChangeOutcomeApplied, NewRevision: 3,
				ResourceCapacity: &factoryruntime.ResourceCapacityResult{
					ResourceID: "reviewers", ResourceName: "Review Pool", PreviousCapacity: 1,
					RequestedCapacity: 8, EffectiveCapacity: 8, InUseCount: 1, AvailableCount: 7,
					MinimumCapacity: 1, Outcome: factoryruntime.ResourceCapacityOutcomeApplied,
				},
			}, nil
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()
	body := `{"requestId":"raise-1","expectedRevision":2,"capacity":8,"reason":"raise throughput"}`
	request := httptest.NewRequest(http.MethodPost, "/factory-sessions/session-beta/resources/reviewers/capacity", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	handler.SetFactorySessionResourceCapacity(recorder, request, "session-beta", "reviewers")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.FactorySessionResourceCapacityResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ResourceId != "reviewers" || response.ResourceName == nil || *response.ResourceName != "Review Pool" || response.EffectiveCapacity != 8 || response.Revision != 3 {
		t.Fatalf("response = %#v", response)
	}
	if response.Links == nil || response.Links.Events == nil || *response.Links.Events != "/factory-sessions/session-beta/events" {
		t.Fatalf("links = %#v", response.Links)
	}
}

func TestSetFactorySessionResourceCapacityMapsCapacityConflict(t *testing.T) {
	root := &httpSessionsRootFake{
		onApplyLiveChange: func(context.Context, string, factorysessions.LiveChangeRequest) (factorysessions.LiveChangeResult, error) {
			return factorysessions.LiveChangeResult{}, &factorysessions.LiveChangeError{
				Code: factorysessions.LiveChangeErrorCapacityInUse, Message: "reviewers capacity cannot be reduced below 2 units in use",
				ResourceCapacity: &factoryruntime.ResourceCapacityResult{
					ResourceID: "reviewers", PreviousCapacity: 3, RequestedCapacity: 1,
					InUseCount: 2, AvailableCount: 1, MinimumCapacity: 2,
				},
			}
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/factory-sessions/session-beta/resources/reviewers/capacity", strings.NewReader(`{"requestId":"lower-1","expectedRevision":2,"capacity":1}`))
	request.Header.Set("Content-Type", "application/json")
	handler.SetFactorySessionResourceCapacity(recorder, request, "session-beta", "reviewers")

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Code != factoryapi.ErrorResponseCodeRESOURCECAPACITYINUSE {
		t.Fatalf("error code = %q, want RESOURCE_CAPACITY_IN_USE", response.Code)
	}
	if response.ResourceCapacity == nil || response.ResourceCapacity.ResourceId != "reviewers" ||
		response.ResourceCapacity.CurrentCapacity != 3 || response.ResourceCapacity.RequestedCapacity != 1 ||
		response.ResourceCapacity.InUseCount != 2 || response.ResourceCapacity.AvailableCount != 1 ||
		response.ResourceCapacity.MinimumCapacity != 2 {
		t.Fatalf("capacity error details = %#v", response.ResourceCapacity)
	}
}

func TestSetFactorySessionResourceCapacityMapsNoOpAndAPIAttribution(t *testing.T) {
	root := &httpSessionsRootFake{
		onApplyLiveChange: func(_ context.Context, sessionID string, request factorysessions.LiveChangeRequest) (factorysessions.LiveChangeResult, error) {
			if sessionID != "session-beta" || request.Actor != "operator" || request.Source != "api" {
				t.Fatalf("request attribution = %#v, want operator/api", request)
			}
			return factorysessions.LiveChangeResult{
				SessionID: sessionID, RequestID: request.RequestID, ChangeID: request.ChangeID,
				Outcome: factorysessions.LiveChangeOutcomeNoOp, PreviousRevision: 2, NewRevision: 2,
				ResourceCapacity: &factoryruntime.ResourceCapacityResult{
					ResourceID: "reviewers", PreviousCapacity: 2, RequestedCapacity: 2,
					EffectiveCapacity: 2, AvailableCount: 2, Outcome: factoryruntime.ResourceCapacityOutcomeNoOp,
				},
			}, nil
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/factory-sessions/session-beta/resources/reviewers/capacity", strings.NewReader(`{"requestId":"noop-1","expectedRevision":2,"capacity":2}`))
	request.Header.Set("Content-Type", "application/json")
	handler.SetFactorySessionResourceCapacity(recorder, request, "session-beta", "reviewers")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.FactorySessionResourceCapacityResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Outcome != factoryapi.FactorySessionResourceCapacityOutcomeNOOP || response.Revision != 2 {
		t.Fatalf("response = %#v, want NO_OP at unchanged revision", response)
	}
}

func TestSetFactorySessionResourceCapacityMapsRequestConflict(t *testing.T) {
	root := &httpSessionsRootFake{
		onApplyLiveChange: func(context.Context, string, factorysessions.LiveChangeRequest) (factorysessions.LiveChangeResult, error) {
			return factorysessions.LiveChangeResult{}, &factorysessions.LiveChangeError{Code: factorysessions.LiveChangeErrorRequestConflict, Message: "request conflict"}
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/factory-sessions/session-beta/resources/reviewers/capacity", strings.NewReader(`{"requestId":"conflict-1","expectedRevision":2,"capacity":2}`))
	request.Header.Set("Content-Type", "application/json")
	handler.SetFactorySessionResourceCapacity(recorder, request, "session-beta", "reviewers")
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != factoryapi.ErrorResponseCodeREQUESTCONFLICT {
		t.Fatalf("error code = %q, want REQUEST_CONFLICT", response.Code)
	}
}

func TestSetFactorySessionResourceCapacityMapsCLIAttribution(t *testing.T) {
	root := &httpSessionsRootFake{
		onApplyLiveChange: func(_ context.Context, _ string, request factorysessions.LiveChangeRequest) (factorysessions.LiveChangeResult, error) {
			if request.Actor != "operator" || request.Source != "cli" {
				t.Fatalf("request attribution = %#v, want operator/cli", request)
			}
			return factorysessions.LiveChangeResult{
				SessionID: "session-beta", RequestID: request.RequestID, ChangeID: request.ChangeID,
				Outcome: factorysessions.LiveChangeOutcomeNoOp, PreviousRevision: 2, NewRevision: 2,
				ResourceCapacity: &factoryruntime.ResourceCapacityResult{
					ResourceID: "reviewers", PreviousCapacity: 2, RequestedCapacity: 2,
					EffectiveCapacity: 2, AvailableCount: 2, Outcome: factoryruntime.ResourceCapacityOutcomeNoOp,
				},
			}, nil
		},
	}
	handler := factorysessionshttp.NewHandlerFromRoot(factorysessionshttp.RootBinding{Sessions: root}, zap.NewNop())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/factory-sessions/session-beta/resources/reviewers/capacity", strings.NewReader(`{"requestId":"cli-1","expectedRevision":2,"capacity":2}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-You-Source", "cli")
	handler.SetFactorySessionResourceCapacity(recorder, request, "session-beta", "reviewers")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
