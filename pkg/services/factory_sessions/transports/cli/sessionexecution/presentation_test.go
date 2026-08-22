package sessionexecution_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/sessionexecution"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
)

func TestRunNormalizedSyncPresentsActiveDirectJavaScriptResult(t *testing.T) {
	service := &syncPresentationService{result: factorysessions.SyncStartResult{
		AsyncStartResult: factorysessions.AsyncStartResult{
			SessionID: "session-direct", Status: string(factorysessions.LifecycleStatusSucceeded),
		},
		SyncOutcome: "COMPLETED",
	}}
	var output bytes.Buffer
	request := factorysessions.StartRequest{RequestID: "run-direct"}

	if err := sessionexecution.RunNormalizedSync(context.Background(), service, request, false, &output); err != nil {
		t.Fatalf("RunNormalizedSync: %v", err)
	}
	if service.request.RequestID != request.RequestID {
		t.Fatalf("start request = %#v, want %#v", service.request, request)
	}
	if got := output.String(); !strings.Contains(got, "Factory session session-direct completed (SUCCEEDED).") {
		t.Fatalf("output = %q, want active sync completion", got)
	}
}

type syncPresentationService struct {
	factorysessionwire.DurableExecutionService
	request factorysessions.StartRequest
	result  factorysessions.SyncStartResult
}

func (service *syncPresentationService) StartSync(_ context.Context, request factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
	service.request = request
	return service.result, nil
}
