package httpserver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	runtimepprof "runtime/pprof"
	runtimetrace "runtime/trace"
	"strings"
	"testing"
	"time"

	"github.com/google/pprof/profile"
)

func TestHandlerWithPprofHandlesNilHandlerAndDirectProfileDispatch(t *testing.T) {
	if handler := HandlerWithPprof(nil, true, nil); handler != nil {
		t.Fatalf("HandlerWithPprof(nil) = %T, want nil", handler)
	}

	request := httptest.NewRequest(http.MethodGet, pprofPath+"heap", nil)
	response := httptest.NewRecorder()
	pprofIndexWithWaiter(waitPprofDuration)(response, request)
	if response.Code != http.StatusOK || response.Body.Len() == 0 {
		t.Fatalf("direct pprof profile = (%d, %d bytes), want non-empty 200 response", response.Code, response.Body.Len())
	}

	waitCalls := 0
	controlled := handlerWithPprof(http.NotFoundHandler(), true, nil, func(ctx context.Context, duration time.Duration) error {
		if ctx == nil {
			t.Fatal("controlled pprof waiter received nil context")
		}
		if duration != time.Second {
			t.Fatalf("controlled pprof wait duration = %s, want 1s", duration)
		}
		waitCalls++
		return nil
	})
	controlledResponse := httptest.NewRecorder()
	controlled.ServeHTTP(controlledResponse, httptest.NewRequest(http.MethodGet, pprofPath+"heap?seconds=1", nil))
	if controlledResponse.Code != http.StatusOK || controlledResponse.Body.Len() == 0 {
		t.Fatalf("controlled heap delta = (%d, %d bytes), want non-empty 200 response", controlledResponse.Code, controlledResponse.Body.Len())
	}
	if _, err := profile.Parse(bytes.NewReader(controlledResponse.Body.Bytes())); err != nil {
		t.Fatalf("parse controlled heap delta: %v", err)
	}
	if waitCalls != 1 {
		t.Fatalf("controlled pprof waiter calls = %d, want 1", waitCalls)
	}
}

func TestPprofIndexPreservesStandardHTMLLayout(t *testing.T) {
	response := httptest.NewRecorder()
	pprofIndexWithWaiter(waitPprofDuration)(response, httptest.NewRequest(http.MethodGet, pprofPath, nil))

	body := response.Body.String()
	if !strings.Contains(body, "<ul>\n<li><div class=profile-name>") {
		t.Fatalf("pprof index body = %q, want standard unindented list-item layout", body)
	}
	if strings.Contains(body, "<ul>\n\t<li><div class=profile-name>") {
		t.Fatalf("pprof index body = %q, contains an unintended tab before list items", body)
	}
}

func TestPprofNamedReportsUnknownProfileAndDebugFormat(t *testing.T) {
	unknownResponse := httptest.NewRecorder()
	pprofNamedWithWaiter("does-not-exist", waitPprofDuration).ServeHTTP(unknownResponse, httptest.NewRequest(http.MethodGet, "/debug/pprof/does-not-exist", nil))
	if unknownResponse.Code != http.StatusNotFound || unknownResponse.Header().Get("X-Go-Pprof") != "1" ||
		!strings.Contains(unknownResponse.Body.String(), "Unknown profile") {
		t.Fatalf("unknown profile = (%d, %q, headers=%v), want pprof 404", unknownResponse.Code, unknownResponse.Body.String(), unknownResponse.Header())
	}

	debugResponse := httptest.NewRecorder()
	pprofNamedWithWaiter("heap", waitPprofDuration).ServeHTTP(debugResponse, httptest.NewRequest(http.MethodGet, "/debug/pprof/heap?debug=1&gc=1", nil))
	if debugResponse.Code != http.StatusOK || debugResponse.Body.Len() == 0 {
		t.Fatalf("debug heap = (%d, %d bytes), want non-empty 200 response", debugResponse.Code, debugResponse.Body.Len())
	}
	if got := debugResponse.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("debug heap Content-Type = %q, want text/plain; charset=utf-8", got)
	}
	if got := debugResponse.Header().Get("Content-Disposition"); got != "" {
		t.Fatalf("debug heap Content-Disposition = %q, want empty", got)
	}
}

func TestPprofDeltaReportsInvalidUnsupportedAndDebugQueries(t *testing.T) {
	profile := runtimepprof.Lookup("heap")
	if profile == nil {
		t.Fatal("runtime heap profile is unavailable")
	}
	tests := []struct {
		name        string
		profileName string
		query       string
		wantStatus  int
		wantBody    string
	}{
		{name: "invalid seconds", profileName: "heap", query: "seconds=invalid", wantStatus: http.StatusBadRequest, wantBody: "positive integer"},
		{name: "unsupported profile", profileName: "unsupported", query: "seconds=1", wantStatus: http.StatusBadRequest, wantBody: "not supported"},
		{name: "debug combination", profileName: "heap", query: "seconds=1&debug=1", wantStatus: http.StatusBadRequest, wantBody: "incompatible"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/debug/pprof/heap?"+test.query, nil)
			response := httptest.NewRecorder()
			servePprofDeltaProfileWithWaiter(response, request, test.profileName, profile, request.URL.Query().Get("seconds"), waitPprofDuration)
			if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), test.wantBody) {
				t.Fatalf("delta response = (%d, %q), want %d containing %q", response.Code, response.Body.String(), test.wantStatus, test.wantBody)
			}
			if response.Header().Get("X-Go-Pprof") != "1" || response.Header().Get("Content-Disposition") != "" {
				t.Fatalf("delta error headers = %v, want pprof error headers", response.Header())
			}
		})
	}
}

func TestPprofDeltaReportsDeadlineAndCancellation(t *testing.T) {
	profile := runtimepprof.Lookup("heap")
	if profile == nil {
		t.Fatal("runtime heap profile is unavailable")
	}
	waiter := func(ctx context.Context, _ time.Duration) error {
		if ctx == nil {
			return errors.New("waiter context is nil")
		}
		return ctx.Err()
	}

	deadlineContext, cancelDeadline := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelDeadline()
	deadlineRequest := httptest.NewRequest(http.MethodGet, "/debug/pprof/heap", nil).WithContext(deadlineContext)
	deadlineResponse := httptest.NewRecorder()
	servePprofDeltaProfileWithWaiter(deadlineResponse, deadlineRequest, "heap", profile, "1", waiter)
	if deadlineResponse.Code != http.StatusRequestTimeout || !strings.Contains(deadlineResponse.Body.String(), context.DeadlineExceeded.Error()) {
		t.Fatalf("deadline delta = (%d, %q), want 408 deadline error", deadlineResponse.Code, deadlineResponse.Body.String())
	}
	if err := waitPprofDuration(nil, time.Second); err == nil || err.Error() != "pprof duration wait: context is required" {
		t.Fatalf("nil pprof wait context error = %v, want explicit context-required error", err)
	}

	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	canceledRequest := httptest.NewRequest(http.MethodGet, "/debug/pprof/heap", nil).WithContext(canceledContext)
	canceledResponse := httptest.NewRecorder()
	servePprofDeltaProfileWithWaiter(canceledResponse, canceledRequest, "heap", profile, "1", waiter)
	if canceledResponse.Code != http.StatusInternalServerError || !strings.Contains(canceledResponse.Body.String(), context.Canceled.Error()) {
		t.Fatalf("canceled delta = (%d, %q), want 500 cancellation error", canceledResponse.Code, canceledResponse.Body.String())
	}
}

func TestPprofDeltaStopsOnWriterFailure(t *testing.T) {
	profile := runtimepprof.Lookup("heap")
	if profile == nil {
		t.Fatal("runtime heap profile is unavailable")
	}
	response := &failingRuntimeResponseWriter{header: make(http.Header)}
	request := httptest.NewRequest(http.MethodGet, "/debug/pprof/heap", nil)
	servePprofDeltaProfileWithWaiter(response, request, "heap", profile, "1", func(context.Context, time.Duration) error {
		return nil
	})
	if response.writes != 1 {
		t.Fatalf("delta response writes = %d, want one failed write", response.writes)
	}
	if response.Header().Get("Content-Disposition") != `attachment; filename="heap-delta"` {
		t.Fatalf("delta Content-Disposition = %q, want heap-delta attachment", response.Header().Get("Content-Disposition"))
	}
}

func TestPprofCommandLineAndIndexHandleInjectedWriterOutcomes(t *testing.T) {
	commandLineResponse := httptest.NewRecorder()
	pprofCmdline(nil)(commandLineResponse, httptest.NewRequest(http.MethodGet, "/debug/pprof/cmdline", nil))
	if commandLineResponse.Code != http.StatusOK || commandLineResponse.Body.Len() != 0 {
		t.Fatalf("nil command line response = (%d, %d bytes), want empty 200 response", commandLineResponse.Code, commandLineResponse.Body.Len())
	}

	indexResponse := &failingRuntimeResponseWriter{header: make(http.Header)}
	pprofIndexWithWaiter(waitPprofDuration)(indexResponse, httptest.NewRequest(http.MethodGet, pprofPath, nil))
	if indexResponse.writes != 1 {
		t.Fatalf("index response writes = %d, want one failed write", indexResponse.writes)
	}
}

func TestPprofCPUAndTraceApplyDefaultsAndReportActiveRuntimeEffects(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	waitDurations := make([]time.Duration, 0, 2)
	waiter := recordingPprofWaiter(&waitDurations)

	assertControlledPprofCPU(t, ctx, waiter)
	assertActivePprofCPU(t, waiter)
	assertControlledPprofTrace(t, ctx, waiter)
	assertActivePprofTrace(t, waiter)
	if got, want := fmt.Sprint(waitDurations), "[30s 1s]"; fmt.Sprint(waitDurations) != want {
		t.Fatalf("controlled pprof wait durations = %s, want %s", got, want)
	}
}

func recordingPprofWaiter(waitDurations *[]time.Duration) pprofDurationWaiter {
	return func(ctx context.Context, duration time.Duration) error {
		if ctx == nil {
			return errors.New("waiter context is nil")
		}
		*waitDurations = append(*waitDurations, duration)
		return ctx.Err()
	}
}

func assertControlledPprofCPU(t *testing.T, ctx context.Context, waiter pprofDurationWaiter) {
	t.Helper()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/debug/pprof/profile?seconds=invalid", nil).WithContext(ctx)
	pprofCPUWithWaiter(waiter)(response, request)
	if response.Code != http.StatusOK || response.Body.Len() == 0 {
		t.Fatalf("default CPU profile = (%d, %d bytes), want non-empty 200 response", response.Code, response.Body.Len())
	}
	parsed, err := profile.Parse(bytes.NewReader(response.Body.Bytes()))
	if err != nil || parsed == nil || len(parsed.SampleType) == 0 {
		t.Fatalf("controlled CPU profile = (%v, %+v), want parseable profile with sample types", err, parsed)
	}
}

func assertActivePprofCPU(t *testing.T, waiter pprofDurationWaiter) {
	t.Helper()
	if err := runtimepprof.StartCPUProfile(io.Discard); err != nil {
		t.Fatalf("start active CPU profile: %v", err)
	}
	defer runtimepprof.StopCPUProfile()

	response := httptest.NewRecorder()
	pprofCPUWithWaiter(waiter)(response, httptest.NewRequest(http.MethodGet, "/debug/pprof/profile", nil))
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "CPU profiling") {
		t.Fatalf("active CPU profile = (%d, %q), want pprof 500 error", response.Code, response.Body.String())
	}
}

func assertControlledPprofTrace(t *testing.T, ctx context.Context, waiter pprofDurationWaiter) {
	t.Helper()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/debug/pprof/trace?seconds=invalid", nil).WithContext(ctx)
	pprofTraceWithWaiter(waiter)(response, request)
	if response.Code != http.StatusOK || response.Body.Len() == 0 {
		t.Fatalf("default trace = (%d, %d bytes), want non-empty 200 response", response.Code, response.Body.Len())
	}
	if !bytes.HasPrefix(response.Body.Bytes(), []byte("go ")) {
		t.Fatalf("controlled trace prefix = %q, want Go trace framing", response.Body.Bytes()[:min(3, response.Body.Len())])
	}
}

func assertActivePprofTrace(t *testing.T, waiter pprofDurationWaiter) {
	t.Helper()
	if err := runtimetrace.Start(io.Discard); err != nil {
		t.Fatalf("start active trace: %v", err)
	}
	defer runtimetrace.Stop()

	response := httptest.NewRecorder()
	pprofTraceWithWaiter(waiter)(response, httptest.NewRequest(http.MethodGet, "/debug/pprof/trace", nil))
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "tracing") {
		t.Fatalf("active trace = (%d, %q), want pprof 500 error", response.Code, response.Body.String())
	}
}

func TestPprofSymbolSupportsQueryAndReportsReadFailures(t *testing.T) {
	pc := reflect.ValueOf(pprofSymbol).Pointer()
	queryRequest := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/debug/pprof/symbol?0x%x+0", pc), nil)
	queryResponse := httptest.NewRecorder()
	pprofSymbol(queryResponse, queryRequest)
	if queryResponse.Code != http.StatusOK || !strings.Contains(queryResponse.Body.String(), "num_symbols: 1") {
		t.Fatalf("query symbol = (%d, %q), want symbol response", queryResponse.Code, queryResponse.Body.String())
	}
	if function := runtime.FuncForPC(pc); function != nil && !strings.Contains(queryResponse.Body.String(), function.Name()) {
		t.Fatalf("query symbol body = %q, want function %q", queryResponse.Body.String(), function.Name())
	}

	postRequest := httptest.NewRequest(http.MethodPost, "/debug/pprof/symbol", nil)
	postRequest.Body = &diagnosticReadCloser{err: errors.New("request body failed")}
	postResponse := httptest.NewRecorder()
	pprofSymbol(postResponse, postRequest)
	if postResponse.Code != http.StatusOK || !strings.Contains(postResponse.Body.String(), "reading request: request body failed") {
		t.Fatalf("failed POST symbol = (%d, %q), want read failure diagnostic", postResponse.Code, postResponse.Body.String())
	}
}

type diagnosticReadCloser struct {
	err error
}

func (reader *diagnosticReadCloser) Read([]byte) (int, error) { return 0, reader.err }
func (*diagnosticReadCloser) Close() error                    { return nil }
