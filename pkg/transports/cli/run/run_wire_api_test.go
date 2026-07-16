package run

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responsestream"
	"github.com/portpowered/infinite-you/pkg/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"
)

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
	set            *responsestream.StreamSet
	subscribeCalls []responseStreamSubscribeCall
}

func newRecordingResponseStreamAttachable() *recordingResponseStreamAttachable {
	return &recordingResponseStreamAttachable{
		set: responsestream.NewStreamSet(),
	}
}

func (r *recordingResponseStreamAttachable) ensureDispatch(dispatchID string) {
	r.set.Stream(dispatchID)
}

func (r *recordingResponseStreamAttachable) stream(dispatchID string) *factorysessions.SessionResponseStream {
	return r.set.Stream(dispatchID)
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
	return r.set.Subscribe(dispatchID, afterSequence)
}

func (r *recordingResponseStreamAttachable) SessionResponseStreamDispatchIDs(string) ([]string, error) {
	return r.set.DispatchIDs(), nil
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
	buildInvocationBootstrap = func(_ context.Context, _ *service.FactoryServiceConfig) (sessionInvocationRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					RequestID: "req-1",
					TraceID:   "trace-1",
					Status:    interfaces.InvocationTerminalStatusCompleted,
					PrimaryResult: []work.WorkContentPart{
						{Type: work.WorkContentPartTypeText, Text: "goal completed"},
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

func TestRun_FactoryInvocationHumanResponseStreamRejectsLegacyStreamFallback(t *testing.T) {
	preserveRunGlobals(t)

	text := "goal completed"
	var output strings.Builder
	attachable := newRecordingResponseStreamAttachable()
	attachable.ensureDispatch("dispatch-goal-1")
	buildInvocationBootstrap = func(_ context.Context, _ *service.FactoryServiceConfig) (sessionInvocationRunner, error) {
		return stubResponseStreamInvocationService{
			stubInvocationService: stubInvocationService{
				run: func(ctx context.Context) error {
					<-ctx.Done()
					return nil
				},
				invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
					stream := attachable.stream("dispatch-goal-1")
					if stream != nil {
						stream.Append(responsestream.Event{
							Kind:       responsestream.EventKindProgressFragment,
							Type:       responsestream.EventTypeProgress,
							DispatchID: "dispatch-goal-1",
							Payload:    "planning",
						})
						stream.Append(responsestream.Event{
							Kind:       responsestream.EventKindResponseFragment,
							Type:       responsestream.EventTypeTextDelta,
							DispatchID: "dispatch-goal-1",
							Payload:    text,
						})
					}
					return apisurface.FactoryInvocationResult{
						RequestID: "req-1",
						TraceID:   "trace-1",
						Status:    interfaces.InvocationTerminalStatusCompleted,
						PrimaryResult: []work.WorkContentPart{
							{Type: work.WorkContentPartTypeText, Text: text},
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
	if got := output.String(); got != text {
		t.Fatalf("output = %q, want only authoritative result %q", got, text)
	}
	if len(attachable.subscribeCalls) != 0 {
		t.Fatal("human mode must not subscribe to the legacy response stream")
	}
}

func TestRun_FactoryInvocationResponseStreamJSONEmitsStructuredRecords(t *testing.T) {
	preserveRunGlobals(t)

	text := "goal completed"
	var output strings.Builder
	attachable := newRecordingResponseEventAttachable()
	buildInvocationBootstrap = func(_ context.Context, _ *service.FactoryServiceConfig) (sessionInvocationRunner, error) {
		return stubResponseEventInvocationService{
			stubInvocationService: stubInvocationService{
				run: func(ctx context.Context) error {
					<-ctx.Done()
					return nil
				},
				invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
					select {
					case <-attachable.subscribed:
					case <-time.After(2 * time.Second):
						t.Fatal("canonical response-event subscription was not established")
					}
					if err := attachable.publish(canonicalResponseEventFixture(1, responseevents.KindMessage)); err != nil {
						t.Fatalf("publish canonical response event: %v", err)
					}
					return apisurface.FactoryInvocationResult{
						RequestID: "req-1",
						TraceID:   "trace-1",
						Status:    interfaces.InvocationTerminalStatusCompleted,
						PrimaryResult: []work.WorkContentPart{
							{Type: work.WorkContentPartTypeText, Text: text},
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
	if !strings.Contains(lines[0], `"recordType":"response_event"`) || !strings.Contains(lines[0], `"kind":"MESSAGE"`) {
		t.Fatalf("response event line = %q", lines[0])
	}
	if !strings.Contains(lines[1], `"recordType":"invocation_result"`) || !strings.Contains(lines[1], `"requestId":"req-1"`) {
		t.Fatalf("invocation result line = %q", lines[1])
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
			Status:    interfaces.InvocationTerminalStatusFailed,
			ErrorCode: "INVOCATION_BLOCKED",
			Message:   `goal invocation blocked while work "Review plan" is in state goal:blocked`,
			SessionID: factorysessions.DefaultSessionID,
			WorkID:    "work-review-plan",
			WorkName:  "Review plan",
			WorkState: "goal:blocked",
		},
		wantErrCode: "INVOCATION_BLOCKED",
		wantContains: []string{
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
			Status:    interfaces.InvocationTerminalStatusFailed,
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
			`"recordType":"invocation_result"`,
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
			Status:    interfaces.InvocationTerminalStatusFailed,
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
			Status:    interfaces.InvocationTerminalStatusTimedOut,
			ErrorCode: "INVOCATION_TIMED_OUT",
			Message:   "invocation timed out while waiting for primary result",
			SessionID: factorysessions.DefaultSessionID,
		},
		jsonMode:    true,
		wantErrCode: "INVOCATION_TIMED_OUT",
		wantContains: []string{
			`"recordType":"invocation_result"`,
			`"status":"TIMED_OUT"`,
			`"errorCode":"INVOCATION_TIMED_OUT"`,
		},
	},
	{
		name: "unresolved primary result human",
		result: apisurface.FactoryInvocationResult{
			RequestID: "req-unresolved",
			TraceID:   "trace-unresolved",
			Status:    interfaces.InvocationTerminalStatusFailed,
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
	buildInvocationBootstrap = func(_ context.Context, _ *service.FactoryServiceConfig) (sessionInvocationRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return result, nil
			},
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
		var finalRecord responseStreamJSONInvocationResultRecord
		if err := json.Unmarshal([]byte(lines[len(lines)-1]), &finalRecord); err != nil {
			t.Fatalf("unmarshal final invocation_result record: %v\n%s", err, lines[len(lines)-1])
		}
		assertInvocationResponseMatchesFactoryResult(t, finalRecord.Invocation, result)
	}
}

func slowStdoutResponseEventInvoke(
	attachable *recordingResponseEventAttachable,
	eventsFlooded chan<- struct{},
	primaryText string,
) func(context.Context, string, factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
	return func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
		select {
		case <-attachable.subscribed:
		case <-time.After(2 * time.Second):
			return apisurface.FactoryInvocationResult{}, errors.New("canonical response-event subscription was not established")
		}
		for i := 0; i < defaultResponseStreamProgressQueueCapacity+4; i++ {
			event := humanResponseEvent(
				responseevents.KindProgress,
				responseevents.PhaseUpdated,
				responseevents.ProgressPayload{Label: "working"},
			)
			if err := attachable.publish(event); err != nil {
				return apisurface.FactoryInvocationResult{}, err
			}
		}
		eventsFlooded <- struct{}{}
		return apisurface.FactoryInvocationResult{
			RequestID: "req-1",
			TraceID:   "trace-1",
			Status:    interfaces.InvocationTerminalStatusCompleted,
			PrimaryResult: []work.WorkContentPart{
				{Type: work.WorkContentPartTypeText, Text: primaryText},
			},
		}, nil
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
	if strings.Contains(got, "terminal output backlog") {
		t.Fatalf("human output must not include terminal backlog notice:\n%s", got)
	}
	if !strings.Contains(got, "progress: working") {
		t.Fatalf("canonical progress did not reach slow stdout:\n%s", got)
	}
	if !strings.HasSuffix(strings.TrimSpace(got), text) {
		t.Fatalf("output missing final primary result:\n%s", got)
	}
	if strings.Count(got, text) != 1 {
		t.Fatalf("final primary result must appear exactly once:\n%s", got)
	}
	assertNoProgressAfterFinalMarker(t, got, responseStreamPrimaryResultHeader, "progress: working")
}

func TestRun_FactoryInvocationResponseStreamCompletesWithSlowStdout(t *testing.T) {
	runFactoryInvocationResponseStreamSlowStdoutFixture(t, false)
}

func TestRun_FactoryInvocationResponseStreamJSONCompletesWithSlowStdout(t *testing.T) {
	runFactoryInvocationResponseStreamSlowStdoutFixture(t, true)
}

func runFactoryInvocationResponseStreamSlowStdoutFixture(t *testing.T, jsonMode bool) {
	t.Helper()
	preserveRunGlobals(t)

	text := "goal completed"
	output := &gatedResponseStreamWriter{}
	output.block()
	eventsFlooded := make(chan struct{}, 1)
	attachable := newRecordingResponseEventAttachable()
	buildInvocationBootstrap = func(_ context.Context, _ *service.FactoryServiceConfig) (sessionInvocationRunner, error) {
		return stubResponseEventInvocationService{
			stubInvocationService: stubInvocationService{
				run: func(ctx context.Context) error {
					<-ctx.Done()
					return nil
				},
				invoke: slowStdoutResponseEventInvoke(attachable, eventsFlooded, text),
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
			JSONOutput:               jsonMode,
			StdinIsTTY:               func() bool { return true },
			Output:                   output,
			Port:                     7437,
		})
	}()

	waitForResponseStreamProgressFlood(t, eventsFlooded, done)
	waitForBlockedStdoutWrites(t, output, 2*time.Second)
	output.release()
	waitForResponseStreamRunCompletion(t, done, 2*time.Second)
	if jsonMode {
		assertSlowStdoutJSONResponseStreamOutput(t, output, text)
		return
	}
	assertSlowStdoutResponseStreamOutput(t, output, text)
}

func assertSlowStdoutJSONResponseStreamOutput(t *testing.T, output *gatedResponseStreamWriter, text string) {
	t.Helper()
	got := output.String()
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected NDJSON response_event and invocation_result lines, got:\n%s", got)
	}
	foundProgress := false
	for _, line := range lines[:len(lines)-1] {
		if strings.Contains(line, `"recordType":"response_event"`) {
			foundProgress = true
			break
		}
	}
	if !foundProgress {
		t.Fatalf("NDJSON output missing response_event records:\n%s", got)
	}
	var finalRecord responseStreamJSONInvocationResultRecord
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &finalRecord); err != nil {
		t.Fatalf("unmarshal final invocation_result record: %v\n%s", err, lines[len(lines)-1])
	}
	if finalRecord.RecordType != responseStreamJSONRecordInvocationResult {
		t.Fatalf("final record type = %q, want %q", finalRecord.RecordType, responseStreamJSONRecordInvocationResult)
	}
	assertInvocationResponseMatchesFactoryResult(t, finalRecord.Invocation, apisurface.FactoryInvocationResult{
		RequestID: "req-1",
		TraceID:   "trace-1",
		Status:    interfaces.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: text},
		},
	})
}

func waitForResponseStreamFinalWritePastDrainTimeout(
	t *testing.T,
	done <-chan error,
	output *gatedResponseStreamWriter,
) {
	t.Helper()
	waitForBlockedStdoutWrites(t, output, 2*time.Second)
	time.Sleep(responseStreamProgressDrainTimeout + 50*time.Millisecond)
	output.release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("writeFinalInvocationResult: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for final result after stdout release")
	}
}

func assertNoProgressAfterFinalMarker(t *testing.T, got, marker, progressMarker string) {
	t.Helper()
	markerIdx := strings.Index(got, marker)
	if markerIdx < 0 {
		t.Fatalf("missing final marker %q:\n%s", marker, got)
	}
	tail := got[markerIdx+len(marker):]
	if strings.Contains(tail, progressMarker) {
		t.Fatalf("progress after %q:\n%s", marker, got)
	}
}

func TestResponseStreamRenderer_FinalResultDoesNotInterleavePastDrainTimeout(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		newRenderer    func(io.Writer) responseStreamRenderer
		enqueue        func(responseStreamRenderer, int)
		finalMarker    string
		progressMarker string
	}{
		{
			name:        "human",
			newRenderer: func(output io.Writer) responseStreamRenderer { return newHumanResponseStreamRenderer(output) },
			enqueue: func(renderer responseStreamRenderer, count int) {
				floodCanonicalHumanProgress(renderer.(*humanResponseStreamRenderer), count)
			},
			finalMarker:    responseStreamPrimaryResultHeader,
			progressMarker: "working",
		},
		{
			name:        "json",
			newRenderer: func(output io.Writer) responseStreamRenderer { return newJSONResponseStreamRenderer(output) },
			enqueue: func(renderer responseStreamRenderer, count int) {
				events := make([]responseevents.FactoryResponseEvent, count)
				for index := range events {
					events[index] = canonicalResponseEventFixture(int64(index+1), responseevents.KindMessage)
				}
				renderer.(*jsonResponseStreamRenderer).onResponseEvents(events)
			},
			finalMarker:    `"recordType":"invocation_result"`,
			progressMarker: `"recordType":"response_event"`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			output := &gatedResponseStreamWriter{}
			output.block()
			renderer := tc.newRenderer(output)
			tc.enqueue(renderer, defaultResponseStreamProgressQueueCapacity+4)

			done := make(chan error, 1)
			go func() {
				done <- renderer.writeFinalInvocationResult(responseStreamBacklogSuccessResult)
			}()

			waitForResponseStreamFinalWritePastDrainTimeout(t, done, output)
			assertNoProgressAfterFinalMarker(t, output.String(), tc.finalMarker, tc.progressMarker)
		})
	}
}
