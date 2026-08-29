package submit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const submitBatchStdinLimit = 16 << 20

// TestSubmitInputDependencyAndCleanupMatrix characterizes the public submit
// input, HTTP, output, cancellation, ordering, and same-process cleanup edges.
func TestSubmitInputDependencyAndCleanupMatrix(t *testing.T) {
	fixture := packageSubmitFixture

	t.Run("SUB-005 batch file dry-run", func(t *testing.T) {
		fileRoot := fixture.tempDir(t)
		batchPath := filepath.Join(fileRoot, "batch.json")
		if err := writeSubmitFile(batchPath, oneWorkBatch("file-request", "file-review", "file payload")); err != nil {
			t.Fatalf("write batch file: %v", err)
		}
		result := fixture.execute(t, batchCommand("http://127.0.0.1:1", batchPath, true, false), context.Background(), "", true)
		if result.err != nil {
			t.Fatalf("file batch dry-run error = %v", result.err)
		}
		for _, marker := range []string{"requestId: file-request", "works: file-review", "batchSource: file", "dry-run: no request sent"} {
			if !strings.Contains(result.stdout, marker) {
				t.Fatalf("file batch output omitted %q: %q", marker, result.stdout)
			}
		}
	})

	t.Run("SUB-006 non-TTY stdin dry-run", func(t *testing.T) {
		result := fixture.execute(t, batchCommand("http://127.0.0.1:1", "", true, false), context.Background(), functionalBatch, false)
		if result.err != nil {
			t.Fatalf("stdin batch dry-run error = %v", result.err)
		}
		for _, marker := range []string{"requestId: functional-submit-batch", "batchSource: stdin", "dry-run: no request sent"} {
			if !strings.Contains(result.stdout, marker) {
				t.Fatalf("stdin batch output omitted %q: %q", marker, result.stdout)
			}
		}
	})

	t.Run("SUB-007 malformed and invalid Work input precede HTTP", func(t *testing.T) {
		cases := []struct {
			name string
			data string
			want string
		}{
			{name: "malformed JSON", data: "{not-json", want: "parse inline JSON"},
			{name: "invalid Work", data: `{"requestId":"invalid-work","type":"FACTORY_REQUEST_BATCH","works":[{"name":"","workTypeName":"task"}]}`, want: "work_request:"},
		}
		for _, testCase := range cases {
			t.Run(testCase.name, func(t *testing.T) {
				server := newSubmitHTTPServer(t, fixture.ledger, func(w http.ResponseWriter, _ *http.Request) {
					submitJSONResponse(w, http.StatusCreated, submitBatchAcceptedResponse("unexpected", "trace", "unexpected"))
				})
				result := fixture.execute(t, batchCommand(server.URL(), testCase.data, false, false), context.Background(), "", true)
				if result.err == nil || !strings.Contains(result.err.Error(), testCase.want) {
					t.Fatalf("input error = %v, want %q", result.err, testCase.want)
				}
				if result.stdout != "" {
					t.Fatalf("invalid input stdout = %q, want empty", result.stdout)
				}
				if got := len(server.requestsSnapshot()); got != 0 {
					t.Fatalf("invalid input HTTP requests = %d, want 0", got)
				}
			})
		}

		t.Run("valid input recovers on the same process", func(t *testing.T) {
			result := fixture.execute(t, batchCommand("http://127.0.0.1:1", "", true, false), context.Background(), functionalBatch, false)
			if result.err != nil || !strings.Contains(result.stdout, "functional-submit-batch") {
				t.Fatalf("recovery result = %#v", result)
			}
		})
	})

	t.Run("SUB-008 unary required inputs avoid payload and HTTP", func(t *testing.T) {
		missingPayload := filepath.Join(fixture.tempDir(t), "must-not-be-read.txt")
		cases := []struct {
			name string
			args []string
			want string
		}{
			{name: "name", args: unaryCommand("http://127.0.0.1:1", "", "task", missingPayload, ""), want: "--name"},
			{name: "work type", args: unaryCommand("http://127.0.0.1:1", "missing-name", "", missingPayload, ""), want: "--work-type-name"},
			{name: "payload", args: unaryCommand("http://127.0.0.1:1", "missing-payload", "task", "", ""), want: "--payload"},
		}
		for _, testCase := range cases {
			t.Run(testCase.name, func(t *testing.T) {
				result := fixture.execute(t, testCase.args, context.Background(), "", true)
				if result.err == nil || !strings.Contains(result.err.Error(), testCase.want) {
					t.Fatalf("required input error = %v, want %q", result.err, testCase.want)
				}
				if result.stdout != "" {
					t.Fatalf("required input stdout = %q, want empty", result.stdout)
				}
			})
		}
	})

	t.Run("SUB-009 TTY and whitespace stdin are rejected cleanly", func(t *testing.T) {
		cases := []struct {
			name  string
			stdin string
			tty   bool
			want  string
		}{
			{name: "TTY no input", stdin: "", tty: true, want: "batch input required"},
			{name: "non-TTY whitespace", stdin: " \n\t ", tty: false, want: "batch stdin input is empty"},
		}
		for _, testCase := range cases {
			t.Run(testCase.name, func(t *testing.T) {
				result := fixture.execute(t, batchCommand("http://127.0.0.1:1", "", true, false), context.Background(), testCase.stdin, testCase.tty)
				if result.err == nil || !strings.Contains(result.err.Error(), testCase.want) {
					t.Fatalf("empty input error = %v, want %q", result.err, testCase.want)
				}
				if result.stdout != "" {
					t.Fatalf("empty input stdout = %q, want empty", result.stdout)
				}
			})
		}
	})

	t.Run("SUB-010 smallest valid batch dry-run and live admission", func(t *testing.T) {
		batch := oneWorkBatch("smallest-request", "smallest", "{}")
		dryRun := fixture.execute(t, batchCommand("http://127.0.0.1:1", batch, true, true), context.Background(), "", true)
		if dryRun.err != nil {
			t.Fatalf("smallest dry-run error = %v", dryRun.err)
		}
		var dryRunJSON map[string]any
		if err := json.Unmarshal([]byte(dryRun.stdout), &dryRunJSON); err != nil {
			t.Fatalf("decode smallest dry-run JSON: %v", err)
		}
		if dryRunJSON["requestId"] != "smallest-request" || dryRunJSON["workCount"] != float64(1) {
			t.Fatalf("smallest dry-run JSON = %#v", dryRunJSON)
		}

		server := newSubmitHTTPServer(t, fixture.ledger, func(w http.ResponseWriter, _ *http.Request) {
			submitJSONResponse(w, http.StatusCreated, submitBatchAcceptedResponse("smallest-request", "smallest-trace", "smallest"))
		})
		live := fixture.execute(t, batchCommand(server.URL(), batch, false, false), context.Background(), "", true)
		if live.err != nil {
			t.Fatalf("smallest live error = %v", live.err)
		}
		requests := server.requestsSnapshot()
		if len(requests) != 1 || requests[0].Method != http.MethodPut || requests[0].Path != "/factory-sessions/~default/work-requests/smallest-request" {
			t.Fatalf("smallest live requests = %#v", requests)
		}
		if !strings.Contains(live.stdout, "smallest (task)") {
			t.Fatalf("smallest live output = %q", live.stdout)
		}
	})

	t.Run("SUB-011 inclusive stdin boundary and overflow sentinel", func(t *testing.T) {
		base := oneWorkBatch("boundary-request", "boundary", "{}")
		if len(base) >= submitBatchStdinLimit {
			t.Fatalf("boundary fixture is %d bytes, exceeds test limit", len(base))
		}
		exact := strings.Repeat(" ", submitBatchStdinLimit-len(base)) + base
		exactResult := fixture.execute(t, batchCommand("http://127.0.0.1:1", "", true, false), context.Background(), exact, false)
		if exactResult.err != nil || !strings.Contains(exactResult.stdout, "requestId: boundary-request") {
			t.Fatalf("exact boundary result = %#v", exactResult)
		}
		overflow := exact + "x"
		overflowResult := fixture.execute(t, batchCommand("http://127.0.0.1:1", "", true, false), context.Background(), overflow, false)
		if overflowResult.err == nil || !strings.Contains(overflowResult.err.Error(), "batch stdin exceeds the 16777216-byte limit") {
			t.Fatalf("overflow error = %v", overflowResult.err)
		}
		if overflowResult.stdout != "" {
			t.Fatalf("overflow stdout = %q, want empty", overflowResult.stdout)
		}
	})

	t.Run("SUB-012 duplicate names reject without success", func(t *testing.T) {
		server := newSubmitHTTPServer(t, fixture.ledger, func(w http.ResponseWriter, _ *http.Request) {
			submitJSONResponse(w, http.StatusConflict, `{"message":"work_request: duplicate name \"release\"","code":"EXECUTION_REQUEST_ID_CONFLICT","family":"CONFLICT"}`)
		})
		result := fixture.execute(t, batchCommand(server.URL(), duplicateNameBatch(), false, false), context.Background(), "", true)
		if result.err == nil || !strings.Contains(result.err.Error(), "duplicate name") {
			t.Fatalf("duplicate result error = %v", result.err)
		}
		if strings.Contains(result.stdout, "requestId:") || strings.Contains(result.stdout, "release (task)") {
			t.Fatalf("duplicate success output = %q", result.stdout)
		}
		if got := len(server.requestsSnapshot()); got > 1 {
			t.Fatalf("duplicate HTTP requests = %d, want at most one", got)
		}
	})

	t.Run("SUB-013 auth failures are surfaced once", func(t *testing.T) {
		for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
			t.Run(fmt.Sprintf("status-%d", status), func(t *testing.T) {
				server := newSubmitHTTPServer(t, fixture.ledger, func(w http.ResponseWriter, _ *http.Request) {
					submitJSONResponse(w, status, `{"message":"do not expose this detail","code":"UNSAFE","family":"UNSAFE"}`)
				})
				payloadPath := writeUnaryPayload(t, fixture, "auth failure")
				result := fixture.execute(t, unaryCommand(server.URL(), "auth-work", "task", payloadPath, "auth-session"), context.Background(), "", true)
				if result.err == nil || !strings.Contains(result.err.Error(), fmt.Sprintf("(%d)", status)) {
					t.Fatalf("auth error = %v, want status %d", result.err, status)
				}
				if len(server.requestsSnapshot()) != 1 || strings.Contains(result.stdout, "Submitted:") {
					t.Fatalf("auth result = %#v", result)
				}
			})
		}
	})

	t.Run("SUB-014 structured rejection is safe and not retried", func(t *testing.T) {
		server := newSubmitHTTPServer(t, fixture.ledger, func(w http.ResponseWriter, _ *http.Request) {
			submitJSONResponse(w, http.StatusInternalServerError, `{"message":"work_request: downstream dependency rejected","code":"INTERNAL_ERROR","family":"INTERNAL_SERVER_ERROR","secret":"must-not-leak"}`)
		})
		result := fixture.execute(t, batchCommand(server.URL(), oneWorkBatch("structured-failure", "structured", "{}"), false, false), context.Background(), "", true)
		if result.err == nil || !strings.Contains(result.err.Error(), "work_request: downstream dependency rejected") {
			t.Fatalf("structured rejection error = %v", result.err)
		}
		if strings.Contains(result.err.Error(), "must-not-leak") || len(server.requestsSnapshot()) != 1 || result.stdout != "" {
			t.Fatalf("structured rejection result = %#v requests=%d", result, len(server.requestsSnapshot()))
		}
	})

	t.Run("SUB-015 deadline reaches handler and joins", func(t *testing.T) {
		observed := make(chan struct{})
		handlerDone := make(chan struct{})
		var observedOnce sync.Once
		server := newSubmitHTTPServer(t, fixture.ledger, func(_ http.ResponseWriter, r *http.Request) {
			observedOnce.Do(func() { close(observed) })
			<-r.Context().Done()
			close(handlerDone)
		})
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		defer cancel()
		command := fixture.startInvocation(t, fixture.newInvocation(t, unaryCommand(server.URL(), "deadline-work", "task", writeUnaryPayload(t, fixture, "deadline"), "deadline-session"), ctx, "", true, "", nil))
		waitForSignal(t, observed, "deadline handler observation")
		result := command.result(t)
		if !errors.Is(result.err, context.DeadlineExceeded) {
			t.Fatalf("deadline error = %v, want context deadline exceeded", result.err)
		}
		waitForSignal(t, handlerDone, "deadline handler shutdown")
		if server.active.Load() != 0 {
			t.Fatalf("deadline active handlers = %d, want 0", server.active.Load())
		}
	})

	t.Run("SUB-016 caller cancellation is scoped and next request succeeds", func(t *testing.T) {
		observed := make(chan struct{})
		handlerDone := make(chan struct{})
		var observedOnce sync.Once
		server := newSubmitHTTPServer(t, fixture.ledger, func(_ http.ResponseWriter, r *http.Request) {
			observedOnce.Do(func() { close(observed) })
			<-r.Context().Done()
			close(handlerDone)
		})
		ctx, cancel := context.WithCancel(context.Background())
		command := fixture.startInvocation(t, fixture.newInvocation(t, unaryCommand(server.URL(), "cancel-work", "task", writeUnaryPayload(t, fixture, "cancel"), "cancel-session"), ctx, "", true, "", nil))
		waitForSignal(t, observed, "cancel handler observation")
		cancel()
		result := command.result(t)
		if !errors.Is(result.err, context.Canceled) {
			t.Fatalf("cancel error = %v, want context canceled", result.err)
		}
		waitForSignal(t, handlerDone, "cancel handler shutdown")
		if server.active.Load() != 0 {
			t.Fatalf("cancel active handlers = %d, want 0", server.active.Load())
		}

		success := newSubmitHTTPServer(t, fixture.ledger, func(w http.ResponseWriter, _ *http.Request) {
			submitJSONResponse(w, http.StatusCreated, submitAcceptedResponse("survivor-request", "survivor-trace", "survivor-work", "survivor", "task"))
		})
		survivor := fixture.execute(t, unaryCommand(success.URL(), "survivor", "task", writeUnaryPayload(t, fixture, "survivor"), "survivor-session"), context.Background(), "", true)
		if survivor.err != nil || !strings.Contains(survivor.stdout, "Submitted: survivor (task)") {
			t.Fatalf("survivor result = %#v", survivor)
		}
	})

	t.Run("SUB-017 batch request and rendered identities preserve order", func(t *testing.T) {
		server := newSubmitHTTPServer(t, fixture.ledger, func(w http.ResponseWriter, r *http.Request) {
			var request struct {
				Works []struct {
					Name string `json:"name"`
				} `json:"works"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode ordered request: %v", err)
				return
			}
			names := make([]string, 0, len(request.Works))
			for _, work := range request.Works {
				names = append(names, work.Name)
			}
			submitJSONResponse(w, http.StatusCreated, submitBatchAcceptedResponse("ordered-request", "ordered-trace", names...))
		})
		result := fixture.execute(t, batchCommand(server.URL(), orderedBatch(), false, true), context.Background(), "", true)
		if result.err != nil {
			t.Fatalf("ordered batch error = %v", result.err)
		}
		var output struct {
			Works []struct {
				Name string `json:"name"`
			} `json:"works"`
		}
		if err := json.Unmarshal([]byte(result.stdout), &output); err != nil {
			t.Fatalf("decode ordered output: %v", err)
		}
		want := []string{"first", "second", "third"}
		if len(output.Works) != len(want) {
			t.Fatalf("ordered output works = %#v", output.Works)
		}
		for index, name := range want {
			if output.Works[index].Name != name {
				t.Fatalf("ordered output[%d] = %q, want %q", index, output.Works[index].Name, name)
			}
		}
	})

	t.Run("SUB-018 dry-run is byte stable and live duplicate is not retried", func(t *testing.T) {
		batch := oneWorkBatch("stable-request", "stable", "{}")
		first := fixture.execute(t, batchCommand("http://127.0.0.1:1", batch, true, true), context.Background(), "", true)
		second := fixture.execute(t, batchCommand("http://127.0.0.1:1", batch, true, true), context.Background(), "", true)
		if first.err != nil || second.err != nil || first.stdout != second.stdout {
			t.Fatalf("stable dry-runs = %#v and %#v", first, second)
		}
		var calls atomic.Int32
		server := newSubmitHTTPServer(t, fixture.ledger, func(w http.ResponseWriter, _ *http.Request) {
			if calls.Add(1) == 1 {
				submitJSONResponse(w, http.StatusCreated, submitBatchAcceptedResponse("stable-request", "stable-trace", "stable"))
				return
			}
			submitJSONResponse(w, http.StatusConflict, `{"message":"work_request: duplicate request","code":"EXECUTION_REQUEST_ID_CONFLICT","family":"CONFLICT"}`)
		})
		accepted := fixture.execute(t, batchCommand(server.URL(), batch, false, false), context.Background(), "", true)
		duplicate := fixture.execute(t, batchCommand(server.URL(), batch, false, false), context.Background(), "", true)
		if accepted.err != nil || duplicate.err == nil || !strings.Contains(duplicate.err.Error(), "duplicate request") {
			t.Fatalf("stable live results = %#v and %#v", accepted, duplicate)
		}
		if calls.Load() != 2 || len(server.requestsSnapshot()) != 2 || strings.Contains(duplicate.stdout, "requestId:") {
			t.Fatalf("stable live calls=%d requests=%d duplicate stdout=%q", calls.Load(), len(server.requestsSnapshot()), duplicate.stdout)
		}
	})

	t.Run("SUB-019 failures recover with fresh state in one process", func(t *testing.T) {
		invalid := fixture.execute(t, batchCommand("http://127.0.0.1:1", "{", true, false), context.Background(), "", true)
		if invalid.err == nil {
			t.Fatal("invalid preparation succeeded")
		}
		failureServer := newSubmitHTTPServer(t, fixture.ledger, func(w http.ResponseWriter, _ *http.Request) {
			submitJSONResponse(w, http.StatusBadGateway, `{"message":"work_request: temporary dependency failure","family":"INTERNAL_SERVER_ERROR"}`)
		})
		failed := fixture.execute(t, batchCommand(failureServer.URL(), oneWorkBatch("recovery-request", "recovery", "{}"), false, false), context.Background(), "", true)
		if failed.err == nil || len(failureServer.requestsSnapshot()) != 1 {
			t.Fatalf("dependency failure result = %#v requests=%d", failed, len(failureServer.requestsSnapshot()))
		}
		successServer := newSubmitHTTPServer(t, fixture.ledger, func(w http.ResponseWriter, _ *http.Request) {
			submitJSONResponse(w, http.StatusCreated, submitBatchAcceptedResponse("recovery-request", "recovery-trace", "recovery"))
		})
		success := fixture.execute(t, batchCommand(successServer.URL(), oneWorkBatch("recovery-request", "recovery", "{}"), false, false), context.Background(), "", true)
		if success.err != nil || !strings.Contains(success.stdout, "recovery (task)") {
			t.Fatalf("recovery success = %#v", success)
		}

		t.Run("timeout then valid submit", func(t *testing.T) {
			observed := make(chan struct{})
			handlerDone := make(chan struct{})
			var observedOnce sync.Once
			timeoutServer := newSubmitHTTPServer(t, fixture.ledger, func(_ http.ResponseWriter, r *http.Request) {
				observedOnce.Do(func() { close(observed) })
				<-r.Context().Done()
				close(handlerDone)
			})
			ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
			command := fixture.startInvocation(t, fixture.newInvocation(t, batchCommand(timeoutServer.URL(), oneWorkBatch("timeout-request", "timeout", "{}"), false, false), ctx, "", true, "", nil))
			waitForSignal(t, observed, "recovery timeout handler observation")
			timeoutResult := command.result(t)
			cancel()
			if timeoutResult.err == nil || !strings.Contains(timeoutResult.err.Error(), "context deadline exceeded") {
				t.Fatalf("recovery timeout result = %#v", timeoutResult)
			}
			waitForSignal(t, handlerDone, "recovery timeout handler shutdown")
			recoveryServer := newSubmitHTTPServer(t, fixture.ledger, func(w http.ResponseWriter, _ *http.Request) {
				submitJSONResponse(w, http.StatusCreated, submitBatchAcceptedResponse("timeout-recovery", "timeout-recovery-trace", "timeout-recovery"))
			})
			recovered := fixture.execute(t, batchCommand(recoveryServer.URL(), oneWorkBatch("timeout-recovery", "timeout-recovery", "{}"), false, false), context.Background(), "", true)
			if recovered.err != nil || !strings.Contains(recovered.stdout, "timeout-recovery (task)") {
				t.Fatalf("timeout recovery result = %#v", recovered)
			}
		})
	})

	t.Run("SUB-020 output failure does not retry and next buffer is clean", func(t *testing.T) {
		server := newSubmitHTTPServer(t, fixture.ledger, func(w http.ResponseWriter, _ *http.Request) {
			submitJSONResponse(w, http.StatusCreated, submitBatchAcceptedResponse("writer-request", "writer-trace", "writer"))
		})
		writer := &boundedSubmitWriter{limit: 8, err: errors.New("functional stdout failure")}
		failed := fixture.executeWithWriter(t, batchCommand(server.URL(), oneWorkBatch("writer-request", "writer", "{}"), false, false), context.Background(), "", true, writer)
		if failed.err == nil || !errors.Is(failed.err, writer.err) {
			t.Fatalf("writer failure = %v, want %v", failed.err, writer.err)
		}
		if len(server.requestsSnapshot()) != 1 {
			t.Fatalf("writer failure requests = %d, want 1", len(server.requestsSnapshot()))
		}
		recovered := fixture.execute(t, batchCommand("http://127.0.0.1:1", oneWorkBatch("writer-recovery", "writer-recovery", "{}"), true, false), context.Background(), "", true)
		if recovered.err != nil || !strings.Contains(recovered.stdout, "writer-recovery") || strings.Contains(recovered.stdout, "writer-request") {
			t.Fatalf("writer recovery = %#v", recovered)
		}
		if len(server.requestsSnapshot()) != 1 {
			t.Fatalf("writer recovery requests = %d, want one accepted request", len(server.requestsSnapshot()))
		}
	})

	// SUB-021: concurrent explicit sessions isolate cancellation.
	t.Run("concurrent_server_and_clients", func(t *testing.T) {
		observed := make(chan string, 2)
		handlerDone := make(chan string, 2)
		releaseSurvivor := make(chan struct{})
		server := newSubmitHTTPServer(t, fixture.ledger, func(w http.ResponseWriter, r *http.Request) {
			sessionID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/factory-sessions/"), "/work")
			observed <- sessionID
			if sessionID == "cancel-session" {
				<-r.Context().Done()
				handlerDone <- sessionID
				return
			}
			select {
			case <-releaseSurvivor:
				submitJSONResponse(w, http.StatusCreated, submitAcceptedResponse("survivor-request", "survivor-trace", "survivor-work", "survivor", "task"))
			case <-r.Context().Done():
			}
			handlerDone <- sessionID
		})

		cancelContext, cancel := context.WithCancel(context.Background())
		cancelInvocation := fixture.newInvocation(t, unaryCommand(server.URL(), "cancel-work", "task", writeUnaryPayload(t, fixture, "cancel"), "cancel-session"), cancelContext, "", true, "", nil)
		survivorInvocation := fixture.newInvocation(t, withJSON(unaryCommand(server.URL(), "survivor", "task", writeUnaryPayload(t, fixture, "survivor"), "survivor-session")), context.Background(), "", true, "", nil)
		cancelCommand := fixture.startInvocation(t, cancelInvocation)
		survivorCommand := fixture.startInvocation(t, survivorInvocation)
		seen := map[string]bool{}
		for len(seen) < 2 {
			select {
			case sessionID := <-observed:
				seen[sessionID] = true
			case <-time.After(submitFixtureTimeout):
				t.Fatal("timed out waiting for both session requests")
			}
		}
		cancel()
		cancelResult := cancelCommand.result(t)
		close(releaseSurvivor)
		survivorResult := survivorCommand.result(t)
		if !errors.Is(cancelResult.err, context.Canceled) {
			t.Fatalf("concurrent canceled result = %v", cancelResult.err)
		}
		if survivorResult.err != nil || !strings.Contains(survivorResult.stdout, `"sessionId":"survivor-session"`) {
			t.Fatalf("concurrent survivor result = %#v", survivorResult)
		}
		joined := map[string]bool{}
		for len(joined) < 2 {
			select {
			case sessionID := <-handlerDone:
				joined[sessionID] = true
			case <-time.After(submitFixtureTimeout):
				t.Fatal("timed out waiting for both session handlers")
			}
		}
		requests := server.requestsSnapshot()
		if len(requests) != 2 {
			t.Fatalf("concurrent requests = %#v", requests)
		}
	})
}

func batchCommand(server, source string, dryRun, jsonOutput bool) []string {
	args := []string{"you", "--server", server}
	if jsonOutput {
		args = append(args, "--json")
	}
	args = append(args, "submit", "batch")
	if dryRun {
		args = append(args, "--dry-run")
	}
	if source != "" {
		args = append(args, source)
	}
	return args
}

func unaryCommand(server, name, workType, payload, session string) []string {
	args := []string{"you", "--server", server}
	if session != "" {
		args = append(args, "--session", session)
	}
	args = append(args, "submit")
	if name != "" {
		args = append(args, "--name", name)
	}
	if workType != "" {
		args = append(args, "--work-type-name", workType)
	}
	if payload != "" {
		args = append(args, "--payload", payload)
	}
	return args
}

func withJSON(args []string) []string {
	withJSON := make([]string, 0, len(args)+1)
	for _, arg := range args {
		if arg == "submit" {
			withJSON = append(withJSON, "--json")
		}
		withJSON = append(withJSON, arg)
	}
	return withJSON
}

func duplicateNameBatch() string {
	return `{"requestId":"duplicate-request","type":"FACTORY_REQUEST_BATCH","works":[{"name":"release","workTypeName":"task","payload":"one"},{"name":"release","workTypeName":"task","payload":"two"}]}`
}

func orderedBatch() string {
	return `{"requestId":"ordered-request","type":"FACTORY_REQUEST_BATCH","works":[{"name":"first","workTypeName":"task","payload":"1"},{"name":"second","workTypeName":"task","payload":"2"},{"name":"third","workTypeName":"task","payload":"3"}]}`
}

func writeUnaryPayload(t testing.TB, fixture *submitFixture, content string) string {
	t.Helper()
	path := filepath.Join(fixture.tempDir(t), "payload.txt")
	if err := writeSubmitFile(path, content); err != nil {
		t.Fatalf("write unary payload: %v", err)
	}
	return path
}

func waitForSignal(t testing.TB, signal <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(submitFixtureTimeout):
		t.Fatalf("timed out waiting for %s", label)
	}
}

type boundedSubmitWriter struct {
	mu    sync.Mutex
	data  bytes.Buffer
	limit int
	err   error
}

func (writer *boundedSubmitWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	remaining := writer.limit - writer.data.Len()
	if remaining <= 0 {
		return 0, writer.err
	}
	if len(data) > remaining {
		_, _ = writer.data.Write(data[:remaining])
		return remaining, writer.err
	}
	return writer.data.Write(data)
}

var _ io.Writer = (*boundedSubmitWriter)(nil)
