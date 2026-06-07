package runtime_api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
)

func TestSessionInvocationAPI_ReturnsPrimaryResult(t *testing.T) {
	dir := scaffoldInvocationFactory(t, nil)
	server := startFunctionalServerWithConfig(t, dir, false, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		cfg.ProviderCommandRunnerOverride = support.NewStaticSuccessCommandRunner("primary result COMPLETE")
	})

	response := postInvocation(t, server.URL(), textInvocationRequest(t, "invoke this", nil))
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED", response.Status)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("invocation primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}
	if part.Text != "invoke this" {
		t.Fatalf("primaryResult text = %q, want %q", part.Text, "invoke this")
	}
}

func TestSessionInvocationAPI_UnresolvedPrimaryResultReturnsFailedStatus(t *testing.T) {
	dir := scaffoldInvocationFactory(t, map[string]any{
		"invocationReturn": map[string]any{
			"policy":        "EXPLICIT",
			"workTypeName":  "summary",
			"terminalState": "complete",
		},
		"workTypes": append(simplePipelineConfig()["workTypes"].([]map[string]any), map[string]any{
			"name": "summary",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}),
	})
	server := startFunctionalServerWithConfig(t, dir, false, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		cfg.ProviderCommandRunnerOverride = support.NewStaticSuccessCommandRunner("task output COMPLETE")
	})

	response := postInvocation(t, server.URL(), textInvocationRequest(t, "invoke this", nil))
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED", response.Status)
	}
	if response.ErrorCode == nil || *response.ErrorCode != factoryapi.INVOCATIONPRIMARYRESULTUNRESOLVED {
		t.Fatalf("invocation errorCode = %#v, want INVOCATION_PRIMARY_RESULT_UNRESOLVED", response.ErrorCode)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("invocation primaryResult = %#v, want nil on unresolved output", response.PrimaryResult)
	}
}

func TestSessionInvocationAPI_TimeoutReturnsTimedOutStatus(t *testing.T) {
	dir := scaffoldInvocationFactory(t, nil)
	blocking := newBlockingInvocationRunner()
	server := startFunctionalServerWithConfig(t, dir, false, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		cfg.ProviderCommandRunnerOverride = blocking
	})

	timeoutMillis := int64(10)
	response := postInvocation(t, server.URL(), textInvocationRequest(t, "invoke this", &timeoutMillis))
	if response.Status != factoryapi.InvocationTerminalStatusTimedOut {
		t.Fatalf("invocation status = %q, want TIMED_OUT", response.Status)
	}
	if response.ErrorCode == nil || *response.ErrorCode != factoryapi.INVOCATIONTIMEDOUT {
		t.Fatalf("invocation errorCode = %#v, want INVOCATION_TIMED_OUT", response.ErrorCode)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("invocation primaryResult = %#v, want nil on timeout", response.PrimaryResult)
	}
}

func TestSessionInvocationService_CanceledContextReturnsCanceledStatus(t *testing.T) {
	dir := scaffoldInvocationFactory(t, nil)
	blocking := newBlockingInvocationRunner()
	var svc *service.FactoryService
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		CaptureService: func(captured *service.FactoryService) {
			svc = captured
		},
		Configure: func(cfg *service.FactoryServiceConfig) {
			cfg.RuntimeMode = interfaces.RuntimeModeService
			cfg.ProviderCommandRunnerOverride = blocking
			cfg.Logger = zap.NewNop()
		},
	})
	_ = server
	if svc == nil {
		t.Fatal("expected functional server to capture factory service")
	}

	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan factoryapi.InvocationResponse, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := svc.InvokeFactorySession(ctx, factorysessions.DefaultSessionID, textInvocationRequest(t, "invoke this", nil))
		if err != nil {
			errCh <- err
			return
		}
		response := factoryapi.InvocationResponse{
			RequestId: result.RequestID,
			TraceId:   result.TraceID,
			Status:    result.Status,
		}
		if result.ErrorCode != "" {
			code := factoryapi.InvocationResponseErrorCode(result.ErrorCode)
			response.ErrorCode = &code
		}
		resultCh <- response
	}()

	<-blocking.started
	cancel()

	select {
	case err := <-errCh:
		t.Fatalf("InvokeFactorySession returned error: %v", err)
	case response := <-resultCh:
		if response.Status != factoryapi.InvocationTerminalStatusCanceled {
			t.Fatalf("invocation status = %q, want CANCELED", response.Status)
		}
		if response.ErrorCode == nil || *response.ErrorCode != factoryapi.INVOCATIONCANCELED {
			t.Fatalf("invocation errorCode = %#v, want INVOCATION_CANCELED", response.ErrorCode)
		}
	}
}

type blockingInvocationRunner struct {
	started chan struct{}
}

func newBlockingInvocationRunner() *blockingInvocationRunner {
	return &blockingInvocationRunner{started: make(chan struct{}, 1)}
}

func (r *blockingInvocationRunner) Run(ctx context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	select {
	case r.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return workers.CommandResult{}, ctx.Err()
}

func scaffoldInvocationFactory(t *testing.T, overrides map[string]any) string {
	t.Helper()

	cfg := simplePipelineConfig()
	for key, value := range overrides {
		cfg[key] = value
	}
	workTypes := cfg["workTypes"].([]map[string]any)
	for i := range workTypes {
		if name, _ := workTypes[i]["name"].(string); name == "task" {
			workTypes[i]["handlingBehavior"] = []string{"DEFAULT"}
		}
	}
	dir := support.ScaffoldFactory(t, cfg)
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(interfaces.ModelProviderCodex, "gpt-5-codex"))
	return dir
}

func textInvocationRequest(t *testing.T, text string, timeoutMillis *int64) factoryapi.InvocationRequest {
	t.Helper()

	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: text,
	}); err != nil {
		t.Fatalf("build invocation text content: %v", err)
	}
	return factoryapi.InvocationRequest{
		SourceKind:    factoryapi.InvocationInputSourceKindText,
		Content:       factoryapi.WorkContent{part},
		TimeoutMillis: timeoutMillis,
	}
}

func postInvocation(t *testing.T, serverURL string, request factoryapi.InvocationRequest) factoryapi.InvocationResponse {
	t.Helper()

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal invocation request: %v", err)
	}
	response, err := http.Post(
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+factorysessions.DefaultSessionID+"/invocations",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST /factory-sessions/~default/invocations: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST /factory-sessions/~default/invocations status = %d", response.StatusCode)
	}

	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation response: %v", err)
	}
	return decoded
}

var _ workers.CommandRunner = (*blockingInvocationRunner)(nil)
