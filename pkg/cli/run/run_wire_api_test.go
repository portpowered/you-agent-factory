package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/portpowered/infinite-you/cmd/factory/compose"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"go.uber.org/zap"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRun_WireBuiltFactoryServiceServesStatus(t *testing.T) {
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	writeRunWireTestWorkerAgentsMD(t, dir, "worker-a")
	writeRunWireTestWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("Close listener: %v", err)
	}

	originalStartAPIServer := startAPIServer
	originalServeFactoryAPIServer := serveFactoryAPIServer
	defer func() {
		startAPIServer = originalStartAPIServer
		serveFactoryAPIServer = originalServeFactoryAPIServer
	}()
	serveFactoryAPIServer = compose.ServeAPIServer
	startAPIServer = func(
		ctx context.Context,
		runtime apisurface.APISurface,
		bindPort int,
		logger *zap.Logger,
		markReady func(),
	) error {
		apiListener, listenErr := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", bindPort))
		if listenErr != nil {
			return listenErr
		}
		return serveAPIServer(ctx, runtime, bindPort, logger, markReady, apiListener)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- Run(ctx, RunConfig{
			Dir:                        dir,
			Continuously:               true,
			MockWorkersEnabled:         true,
			Port:                       port,
			SuppressDashboardRendering: true,
			Logger:                     zap.NewNop(),
		})
	}()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	var status factoryapi.StatusResponse
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/status")
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			t.Fatalf("read /status body: %v", readErr)
		}
		if resp.StatusCode != http.StatusOK {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if err := json.Unmarshal(body, &status); err != nil {
			t.Fatalf("decode /status: %v", err)
		}
		break
	}
	if status.FactoryState == "" {
		t.Fatalf("GET /status factory_state empty after polling %s/status", baseURL)
	}

	cancel()
	if err := <-runErrCh; err != nil && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}
}

func writeRunWireTestWorkerAgentsMD(t *testing.T, factoryDir, workerName string) {
	t.Helper()
	workerDir := filepath.Join(factoryDir, "workers", workerName)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("create worker dir: %v", err)
	}
	content := "---\ntype: MODEL_WORKER\nmodel: claude-3-5-haiku-20241022\n---\nYou are a helpful assistant.\n"
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write worker AGENTS.md: %v", err)
	}
}

func writeRunWireTestWorkstationAgentsMD(t *testing.T, factoryDir, workstationName string) {
	t.Helper()
	workstationDir := filepath.Join(factoryDir, "workstations", workstationName)
	if err := os.MkdirAll(workstationDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	content := "---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n"
	if err := os.WriteFile(filepath.Join(workstationDir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}

func TestNormalizeInvocationOutputMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{
			name: "empty defaults to primary",
			raw:  "",
			want: InvocationOutputPrimaryResult,
		},
		{
			name: "primary literal accepted",
			raw:  "primary",
			want: InvocationOutputPrimaryResult,
		},
		{
			name: "response-stream accepted",
			raw:  "response-stream",
			want: InvocationOutputResponseStream,
		},
		{
			name:    "unknown rejected",
			raw:     "sse",
			wantErr: "unsupported --output value",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeInvocationOutputMode(tc.raw)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("NormalizeInvocationOutputMode(%q) error = %v, want %q", tc.raw, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeInvocationOutputMode(%q): %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("mode = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidateInvocationOutputMode_RejectsUnsupportedRunShapes(t *testing.T) {
	t.Parallel()

	text := "Plan the sprint"
	tests := []struct {
		name           string
		cfg            RunConfig
		invocationMode bool
		wantCode       string
	}{
		{
			name: "replay unsupported",
			cfg: RunConfig{
				InvocationOutputMode:     InvocationOutputResponseStream,
				ReplayPath:               "/tmp/replay.json",
				InvocationPositionalText: &text,
			},
			invocationMode: true,
			wantCode:       "INVOCATION_OUTPUT_UNSUPPORTED",
		},
		{
			name: "continuous unsupported",
			cfg: RunConfig{
				InvocationOutputMode:     InvocationOutputResponseStream,
				Continuously:             true,
				InvocationPositionalText: &text,
			},
			invocationMode: true,
			wantCode:       "INVOCATION_OUTPUT_UNSUPPORTED",
		},
		{
			name: "non-invocation run unsupported",
			cfg: RunConfig{
				InvocationOutputMode: InvocationOutputResponseStream,
			},
			invocationMode: false,
			wantCode:       "INVOCATION_OUTPUT_UNSUPPORTED",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateInvocationOutputMode(tc.cfg, tc.invocationMode)
			if err == nil {
				t.Fatal("expected validation error")
			}
			invocationErr, ok := err.(*InvocationError)
			if !ok {
				t.Fatalf("error = %#v, want InvocationError", err)
			}
			if invocationErr.Code != tc.wantCode {
				t.Fatalf("code = %q, want %q", invocationErr.Code, tc.wantCode)
			}
		})
	}
}

func TestValidateInvocationOutputMode_AllowsSupportedInvocation(t *testing.T) {
	t.Parallel()

	text := "Plan the sprint"
	err := validateInvocationOutputMode(RunConfig{
		InvocationOutputMode:     InvocationOutputResponseStream,
		InvocationPositionalText: &text,
	}, true)
	if err != nil {
		t.Fatalf("validateInvocationOutputMode: %v", err)
	}
}

func TestValidateInvocationOutputMode_AllowsJSONResponseStream(t *testing.T) {
	t.Parallel()

	text := "Plan the sprint"
	err := validateInvocationOutputMode(RunConfig{
		InvocationOutputMode:     InvocationOutputResponseStream,
		JSONOutput:               true,
		InvocationPositionalText: &text,
	}, true)
	if err != nil {
		t.Fatalf("validateInvocationOutputMode with JSON: %v", err)
	}
}

type recordingResponseStreamAttachable struct {
	subscribeCalls []responseStreamSubscribeCall
	dispatchIDs    []string
	stream         *factorysessions.SessionResponseStream
}

type responseStreamSubscribeCall struct {
	sessionID  string
	dispatchID string
}

func (r *recordingResponseStreamAttachable) SubscribeSessionResponseStream(
	sessionID string,
	dispatchID string,
	afterSequence int64,
) (*factorysessions.SessionResponseStreamSubscription, error) {
	r.subscribeCalls = append(r.subscribeCalls, responseStreamSubscribeCall{
		sessionID:  sessionID,
		dispatchID: dispatchID,
	})
	if r.stream == nil {
		r.stream = responsestream.NewSessionResponseStream()
	}
	return r.stream.Subscribe(afterSequence)
}

func (r *recordingResponseStreamAttachable) SessionResponseStreamDispatchIDs(string) ([]string, error) {
	return append([]string(nil), r.dispatchIDs...), nil
}

func TestResponseStreamAttachment_SubscribesWhenDispatchAppears(t *testing.T) {
	t.Parallel()

	attachable := &recordingResponseStreamAttachable{
		dispatchIDs: []string{"dispatch-1"},
	}
	sink := &countingResponseStreamSink{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attachment := startResponseStreamAttachment(ctx, attachable, factorysessions.DefaultSessionID, sink)
	if attachment == nil {
		t.Fatal("expected attachment")
	}
	defer attachment.stop()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(attachable.subscribeCalls) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(attachable.subscribeCalls) == 0 {
		t.Fatal("expected internal response-stream subscription")
	}

	attachable.stream.Append(responsestream.Event{
		Kind:    responsestream.EventKindProgressFragment,
		Type:    responsestream.EventTypeProgress,
		Payload: "working",
	})

	for time.Now().Before(deadline) {
		if sink.segments() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if sink.segments() == 0 {
		t.Fatal("expected stream segment delivery after subscription")
	}
	if got := attachable.subscribeCalls[0].dispatchID; got != "dispatch-1" {
		t.Fatalf("dispatchID = %q, want dispatch-1", got)
	}
}

type countingResponseStreamSink struct {
	segmentCount int
}

func (s *countingResponseStreamSink) onStreamSegment(factorysessions.SessionResponseStreamReadResult) {
	s.segmentCount++
}

func (s *countingResponseStreamSink) segments() int {
	return s.segmentCount
}

type stubResponseStreamInvocationService struct {
	stubInvocationService
	attachable *recordingResponseStreamAttachable
}

func (s stubResponseStreamInvocationService) SubscribeSessionResponseStream(
	sessionID string,
	dispatchID string,
	afterSequence int64,
) (*factorysessions.SessionResponseStreamSubscription, error) {
	return s.attachable.SubscribeSessionResponseStream(sessionID, dispatchID, afterSequence)
}

func (s stubResponseStreamInvocationService) SessionResponseStreamDispatchIDs(sessionID string) ([]string, error) {
	return s.attachable.SessionResponseStreamDispatchIDs(sessionID)
}

func TestRun_FactoryInvocationResponseStreamFallsBackWhenStreamAttachmentUnavailable(t *testing.T) {
	preserveRunGlobals(t)

	text := "goal completed"
	var output strings.Builder
	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					RequestID: "req-1",
					TraceID:   "trace-1",
					Status:    factoryapi.InvocationTerminalStatusCompleted,
					PrimaryResult: []interfaces.WorkContentPart{
						{Type: interfaces.WorkContentPartTypeText, Text: "goal completed"},
					},
				}, nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		FactoryConfigPath:        "/tmp/factory.json",
		InvocationPositionalText: &text,
		InvocationOutputMode:     InvocationOutputResponseStream,
		StdinIsTTY:               func() bool { return true },
		Output:                   &output,
		Port:                     7437,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := output.String(); got != text {
		t.Fatalf("output = %q, want primary-result fallback %q", got, text)
	}
}

func TestRun_FactoryInvocationResponseStreamAttachesWhenRunnerSupportsInternalStream(t *testing.T) {
	preserveRunGlobals(t)

	text := "goal completed"
	var output strings.Builder
	attachable := &recordingResponseStreamAttachable{
		dispatchIDs: []string{"dispatch-goal-1"},
	}
	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubResponseStreamInvocationService{
			stubInvocationService: stubInvocationService{
				run: func(ctx context.Context) error {
					<-ctx.Done()
					return nil
				},
				invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
					deadline := time.Now().Add(2 * time.Second)
					for time.Now().Before(deadline) {
						if len(attachable.subscribeCalls) > 0 {
							break
						}
						time.Sleep(10 * time.Millisecond)
					}
					if attachable.stream != nil {
						attachable.stream.Append(responsestream.Event{
							Kind:       responsestream.EventKindProgressFragment,
							Type:       responsestream.EventTypeProgress,
							DispatchID: "dispatch-goal-1",
							Payload:    "planning",
						})
						attachable.stream.Append(responsestream.Event{
							Kind:       responsestream.EventKindResponseFragment,
							Type:       responsestream.EventTypeTextDelta,
							DispatchID: "dispatch-goal-1",
							Payload:    text,
						})
					}
					return apisurface.FactoryInvocationResult{
						RequestID: "req-1",
						TraceID:   "trace-1",
						Status:    factoryapi.InvocationTerminalStatusCompleted,
						PrimaryResult: []interfaces.WorkContentPart{
							{Type: interfaces.WorkContentPartTypeText, Text: text},
						},
					}, nil
				},
			},
			attachable: attachable,
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		FactoryConfigPath:        "/tmp/factory.json",
		InvocationPositionalText: &text,
		InvocationOutputMode:     InvocationOutputResponseStream,
		StdinIsTTY:               func() bool { return true },
		Output:                   &output,
		Port:                     7437,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := output.String()
	if !strings.Contains(got, "[you:progress] planning") {
		t.Fatalf("output missing progress rendering:\n%s", got)
	}
	if strings.Contains(got, "[you:progress] goal completed") {
		t.Fatalf("response fragment leaked into progress output:\n%s", got)
	}
	if !strings.Contains(got, responseStreamPrimaryResultHeader) {
		t.Fatalf("output missing primary-result header:\n%s", got)
	}
	if !strings.HasSuffix(got, text) {
		t.Fatalf("output = %q, want suffix primary result %q", got, text)
	}
	if len(attachable.subscribeCalls) == 0 {
		t.Fatal("expected internal response-stream subscription during invocation")
	}
}

func TestRun_FactoryInvocationResponseStreamJSONEmitsStructuredRecords(t *testing.T) {
	preserveRunGlobals(t)

	text := "goal completed"
	var output strings.Builder
	attachable := &recordingResponseStreamAttachable{
		dispatchIDs: []string{"dispatch-goal-1"},
	}
	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubResponseStreamInvocationService{
			stubInvocationService: stubInvocationService{
				run: func(ctx context.Context) error {
					<-ctx.Done()
					return nil
				},
				invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
					deadline := time.Now().Add(2 * time.Second)
					for time.Now().Before(deadline) {
						if len(attachable.subscribeCalls) > 0 {
							break
						}
						time.Sleep(10 * time.Millisecond)
					}
					if attachable.stream != nil {
						attachable.stream.Append(responsestream.Event{
							Kind:       responsestream.EventKindProgressFragment,
							Type:       responsestream.EventTypeProgress,
							DispatchID: "dispatch-goal-1",
							Payload:    "planning",
						})
					}
					return apisurface.FactoryInvocationResult{
						RequestID: "req-1",
						TraceID:   "trace-1",
						Status:    factoryapi.InvocationTerminalStatusCompleted,
						PrimaryResult: []interfaces.WorkContentPart{
							{Type: interfaces.WorkContentPartTypeText, Text: text},
						},
					}, nil
				},
			},
			attachable: attachable,
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		FactoryConfigPath:        "/tmp/factory.json",
		InvocationPositionalText: &text,
		InvocationOutputMode:     InvocationOutputResponseStream,
		JSONOutput:               true,
		StdinIsTTY:               func() bool { return true },
		Output:                   &output,
		Port:                     7437,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 NDJSON lines, got %d:\n%s", len(lines), output.String())
	}
	if !strings.Contains(lines[0], `"recordType":"progress"`) || !strings.Contains(lines[0], `"payload":"planning"`) {
		t.Fatalf("progress line = %q", lines[0])
	}
	if !strings.Contains(lines[1], `"recordType":"primary_result"`) || !strings.Contains(lines[1], `"requestId":"req-1"`) {
		t.Fatalf("primary result line = %q", lines[1])
	}
}

func TestPrepareRunConfig_ResponseStreamRejectsReplay(t *testing.T) {
	t.Parallel()

	text := "Plan the sprint"
	_, _, _, _, err := prepareRunConfig(RunConfig{
		Dir:                      t.TempDir(),
		FactoryConfigPath:        "factory.json",
		InvocationPositionalText: &text,
		InvocationOutputMode:     InvocationOutputResponseStream,
		ReplayPath:               "/tmp/replay.json",
	})
	if err == nil {
		t.Fatal("expected replay validation error")
	}
	var invocationErr *InvocationError
	if !errors.As(err, &invocationErr) || invocationErr.Code != "INVOCATION_OUTPUT_UNSUPPORTED" {
		t.Fatalf("error = %v, want INVOCATION_OUTPUT_UNSUPPORTED", err)
	}
}

func TestRun_FactoryInvocationResponseStreamRendersTerminalOutcomes(t *testing.T) {
	for _, tc := range responseStreamTerminalOutcomeCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runResponseStreamTerminalOutcomeSubtest(t, tc)
		})
	}
}

type responseStreamTerminalOutcomeCase struct {
	name         string
	result       apisurface.FactoryInvocationResult
	jsonMode     bool
	wantErrCode  string
	wantContains []string
	wantAbsent   []string
}

var responseStreamTerminalOutcomeCases = []responseStreamTerminalOutcomeCase{
	{
		name: "blocked human",
		result: apisurface.FactoryInvocationResult{
			RequestID: "req-blocked",
			TraceID:   "trace-blocked",
			Status:    factoryapi.InvocationTerminalStatusFailed,
			ErrorCode: "INVOCATION_BLOCKED",
			Message:   `goal invocation blocked while work "Review plan" is in state goal:blocked`,
			SessionID: factorysessions.DefaultSessionID,
			WorkID:    "work-review-plan",
			WorkName:  "Review plan",
			WorkState: "goal:blocked",
		},
		wantErrCode: "INVOCATION_BLOCKED",
		wantContains: []string{
			"[you:progress] planning",
			responseStreamInvocationOutcomeHeader,
			"status: FAILED",
			"error: INVOCATION_BLOCKED",
			"workState: goal:blocked",
		},
		wantAbsent: []string{responseStreamPrimaryResultHeader},
	},
	{
		name: "needs-human json",
		result: apisurface.FactoryInvocationResult{
			RequestID: "req-needs-human",
			TraceID:   "trace-needs-human",
			Status:    factoryapi.InvocationTerminalStatusFailed,
			ErrorCode: "INVOCATION_NEEDS_HUMAN",
			Message:   `goal invocation needs human input while work "Review plan" is in state goal:needs-human`,
			SessionID: factorysessions.DefaultSessionID,
			WorkID:    "work-review-plan",
			WorkName:  "Review plan",
			WorkState: "goal:needs-human",
		},
		jsonMode:    true,
		wantErrCode: "INVOCATION_NEEDS_HUMAN",
		wantContains: []string{
			`"recordType":"progress"`,
			`"recordType":"primary_result"`,
			`"status":"FAILED"`,
			`"errorCode":"INVOCATION_NEEDS_HUMAN"`,
		},
		wantAbsent: []string{responseStreamInvocationOutcomeHeader},
	},
	{
		name: "runtime failure human",
		result: apisurface.FactoryInvocationResult{
			RequestID: "req-failed",
			TraceID:   "trace-failed",
			Status:    factoryapi.InvocationTerminalStatusFailed,
			ErrorCode: "INVOCATION_RUNTIME_FAILURE",
			Message:   "goal execution failed before primary result resolved",
			SessionID: factorysessions.DefaultSessionID,
			WorkID:    "work-failed",
			WorkState: "goal:failed",
		},
		wantErrCode: "INVOCATION_RUNTIME_FAILURE",
		wantContains: []string{
			responseStreamInvocationOutcomeHeader,
			"error: INVOCATION_RUNTIME_FAILURE",
			"workState: goal:failed",
		},
	},
	{
		name: "timed out json",
		result: apisurface.FactoryInvocationResult{
			RequestID: "req-timed-out",
			TraceID:   "trace-timed-out",
			Status:    factoryapi.InvocationTerminalStatusTimedOut,
			ErrorCode: "INVOCATION_TIMED_OUT",
			Message:   "invocation timed out while waiting for primary result",
			SessionID: factorysessions.DefaultSessionID,
		},
		jsonMode:    true,
		wantErrCode: "INVOCATION_TIMED_OUT",
		wantContains: []string{
			`"recordType":"primary_result"`,
			`"status":"TIMED_OUT"`,
			`"errorCode":"INVOCATION_TIMED_OUT"`,
		},
	},
	{
		name: "unresolved primary result human",
		result: apisurface.FactoryInvocationResult{
			RequestID: "req-unresolved",
			TraceID:   "trace-unresolved",
			Status:    factoryapi.InvocationTerminalStatusFailed,
			ErrorCode: "INVOCATION_PRIMARY_RESULT_UNRESOLVED",
			Message:   "primary result could not be resolved",
			SessionID: factorysessions.DefaultSessionID,
		},
		wantErrCode: "INVOCATION_PRIMARY_RESULT_UNRESOLVED",
		wantContains: []string{
			responseStreamInvocationOutcomeHeader,
			"error: INVOCATION_PRIMARY_RESULT_UNRESOLVED",
			"message: primary result could not be resolved",
		},
		wantAbsent: []string{responseStreamPrimaryResultHeader},
	},
}

func runResponseStreamTerminalOutcomeSubtest(t *testing.T, tc responseStreamTerminalOutcomeCase) {
	t.Helper()
	preserveRunGlobals(t)

	text := "Plan the sprint"
	var output strings.Builder
	result := tc.result
	attachable := &recordingResponseStreamAttachable{
		dispatchIDs: []string{"dispatch-goal-1"},
	}
	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubResponseStreamInvocationService{
			stubInvocationService: stubInvocationService{
				run: func(ctx context.Context) error {
					<-ctx.Done()
					return nil
				},
				invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
					deadline := time.Now().Add(2 * time.Second)
					for time.Now().Before(deadline) {
						if len(attachable.subscribeCalls) > 0 {
							break
						}
						time.Sleep(10 * time.Millisecond)
					}
					if attachable.stream != nil {
						attachable.stream.Append(responsestream.Event{
							Kind:       responsestream.EventKindProgressFragment,
							Type:       responsestream.EventTypeProgress,
							DispatchID: "dispatch-goal-1",
							Payload:    "planning",
						})
					}
					return result, nil
				},
			},
			attachable: attachable,
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		FactoryConfigPath:        "/tmp/factory.json",
		InvocationPositionalText: &text,
		InvocationOutputMode:     InvocationOutputResponseStream,
		JSONOutput:               tc.jsonMode,
		StdinIsTTY:               func() bool { return true },
		Output:                   &output,
		Port:                     7437,
	})
	if err == nil {
		t.Fatal("expected invocation failure")
	}
	if !strings.Contains(err.Error(), tc.wantErrCode) {
		t.Fatalf("error = %q, want code %q", err.Error(), tc.wantErrCode)
	}

	got := output.String()
	for _, want := range tc.wantContains {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	for _, absent := range tc.wantAbsent {
		if strings.Contains(got, absent) {
			t.Fatalf("output must not contain %q:\n%s", absent, got)
		}
	}

	if tc.jsonMode {
		lines := strings.Split(strings.TrimSpace(got), "\n")
		if len(lines) < 1 {
			t.Fatalf("expected NDJSON output, got empty stdout")
		}
		var finalRecord responseStreamJSONPrimaryResultRecord
		if err := json.Unmarshal([]byte(lines[len(lines)-1]), &finalRecord); err != nil {
			t.Fatalf("unmarshal final primary_result record: %v\n%s", err, lines[len(lines)-1])
		}
		assertInvocationResponseMatchesFactoryResult(t, finalRecord.Invocation, result)
	}
}

func slowStdoutResponseStreamInvoke(
	attachable *recordingResponseStreamAttachable,
	eventsFlooded chan<- struct{},
	primaryText string,
) func(context.Context, string, factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
	return func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
		waitForResponseStreamSubscribe(attachable, 2*time.Second)
		floodResponseStreamProgressEvents(attachable.stream, "dispatch-goal-1", "working")
		eventsFlooded <- struct{}{}
		return apisurface.FactoryInvocationResult{
			RequestID: "req-1",
			TraceID:   "trace-1",
			Status:    factoryapi.InvocationTerminalStatusCompleted,
			PrimaryResult: []interfaces.WorkContentPart{
				{Type: interfaces.WorkContentPartTypeText, Text: primaryText},
			},
		}, nil
	}
}

func waitForResponseStreamSubscribe(attachable *recordingResponseStreamAttachable, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(attachable.subscribeCalls) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func floodResponseStreamProgressEvents(stream *factorysessions.SessionResponseStream, dispatchID, payload string) {
	if stream == nil {
		return
	}
	for i := 0; i < defaultResponseStreamProgressQueueCapacity+4; i++ {
		stream.Append(responsestream.Event{
			Kind:       responsestream.EventKindProgressFragment,
			Type:       responsestream.EventTypeProgress,
			DispatchID: dispatchID,
			Payload:    payload,
		})
	}
}

func waitForResponseStreamProgressFlood(t *testing.T, eventsFlooded <-chan struct{}, done <-chan error) {
	t.Helper()
	select {
	case <-eventsFlooded:
	case err := <-done:
		t.Fatalf("Run completed before progress events were flooded: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for response-stream progress flood")
	}
}

func waitForBlockedStdoutWrites(t *testing.T, output *gatedResponseStreamWriter, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if output.blockedWriteAttemptsCount() > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if output.blockedWriteAttemptsCount() == 0 {
		t.Fatal("expected progress writer to block on slow stdout before release")
	}
}

func waitForResponseStreamRunCompletion(t *testing.T, done <-chan error, timeout time.Duration) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(timeout):
		t.Fatal("Run blocked after stdout was released")
	}
}

func assertSlowStdoutResponseStreamOutput(t *testing.T, output *gatedResponseStreamWriter, text string) {
	t.Helper()
	got := output.String()
	if !strings.Contains(got, "[you:progress] terminal output backlog") {
		t.Fatalf("output missing terminal backlog notice:\n%s", got)
	}
	if !strings.HasSuffix(strings.TrimSpace(got), text) {
		t.Fatalf("output missing final primary result:\n%s", got)
	}
}

func TestRun_FactoryInvocationResponseStreamCompletesWithSlowStdout(t *testing.T) {
	preserveRunGlobals(t)

	text := "goal completed"
	output := &gatedResponseStreamWriter{}
	output.block()
	eventsFlooded := make(chan struct{}, 1)
	attachable := &recordingResponseStreamAttachable{
		dispatchIDs: []string{"dispatch-goal-1"},
	}
	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubResponseStreamInvocationService{
			stubInvocationService: stubInvocationService{
				run: func(ctx context.Context) error {
					<-ctx.Done()
					return nil
				},
				invoke: slowStdoutResponseStreamInvoke(attachable, eventsFlooded, text),
			},
			attachable: attachable,
		}, nil
	}

	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), RunConfig{
			FactoryConfigPath:        "/tmp/factory.json",
			InvocationPositionalText: &text,
			InvocationOutputMode:     InvocationOutputResponseStream,
			StdinIsTTY:               func() bool { return true },
			Output:                   output,
			Port:                     7437,
		})
	}()

	waitForResponseStreamProgressFlood(t, eventsFlooded, done)
	waitForBlockedStdoutWrites(t, output, 2*time.Second)
	output.release()
	waitForResponseStreamRunCompletion(t, done, 2*time.Second)
	assertSlowStdoutResponseStreamOutput(t, output, text)
}
