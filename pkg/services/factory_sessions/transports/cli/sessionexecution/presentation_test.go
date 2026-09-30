package sessionexecution_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/sessionexecution"
)

func TestRunCanonicalSyncPresentsActiveDirectJavaScriptResult(t *testing.T) {
	stub := &canonicalSyncPresentationService{result: factorysessions.SessionStartResult{
		Sync: &factorysessions.SyncStartResult{
			AsyncStartResult: factorysessions.AsyncStartResult{
				SessionID: "session-direct", Status: string(factorysessions.LifecycleStatusSucceeded),
			},
			SyncOutcome: "COMPLETED",
		},
	}}
	request := factorysessions.SessionStartRequest{
		Mode:        factorysessions.SessionOperationModeDurable,
		FolderPath:  "/factory",
		Synchronous: true,
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: "run-direct"},
	}
	var output bytes.Buffer
	if err := sessionexecution.RunCanonicalSync(context.Background(), stub, request, false, &output); err != nil {
		t.Fatalf("RunCanonicalSync: %v", err)
	}
	if stub.request.Mode != request.Mode || stub.request.FolderPath != request.FolderPath || stub.request.Correlation.RequestID != request.Correlation.RequestID || !stub.request.Synchronous {
		t.Fatalf("start request = %#v, want %#v", stub.request, request)
	}
	if got := output.String(); !strings.Contains(got, "Factory session session-direct completed (SUCCEEDED).") {
		t.Fatalf("output = %q, want canonical sync completion", got)
	}
}

type canonicalSyncPresentationService struct {
	factorysessions.Service
	request factorysessions.SessionStartRequest
	result  factorysessions.SessionStartResult
}

func (service *canonicalSyncPresentationService) Start(_ context.Context, request factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
	service.request = request
	return service.result, nil
}
