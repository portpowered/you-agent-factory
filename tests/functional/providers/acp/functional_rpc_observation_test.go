package acp_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const acpObservationTraceMaxBytes = 64 * 1024

var errACPObservationTraceLimit = errors.New("ACP observation trace byte limit reached")

type acpObservationRecord struct {
	Version   int                     `json:"version"`
	PID       int                     `json:"pid"`
	Phase     string                  `json:"phase"`
	Attempt   int                     `json:"attempt,omitempty"`
	Direction string                  `json:"direction,omitempty"`
	Method    string                  `json:"method,omitempty"`
	ID        json.RawMessage         `json:"id,omitempty"`
	Result    string                  `json:"result,omitempty"`
	Error     *acpObservationRPCError `json:"error,omitempty"`
	ExitCode  *int                    `json:"exitCode,omitempty"`
}

type acpObservationRPCError struct {
	Code int               `json:"code"`
	Data map[string]string `json:"data,omitempty"`
}

type acpObservationWriter struct {
	file    *os.File
	path    string
	attempt int
}

func newACPObservationWriter(path string, attempt int) (*acpObservationWriter, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("ACP observation path must be absolute")
	}

	info, err := os.Lstat(path)
	var file *os.File
	if os.IsNotExist(err) {
		file, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_APPEND|os.O_WRONLY, 0o600)
	} else if err != nil {
		return nil, fmt.Errorf("inspect ACP observation path: %w", err)
	} else {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("ACP observation path must name a regular file")
		}
		file, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	}
	if err != nil {
		return nil, fmt.Errorf("open ACP observation trace: %w", err)
	}
	closeOnError := func(cause error) (*acpObservationWriter, error) {
		_ = file.Close()
		return nil, cause
	}
	if err := file.Chmod(0o600); err != nil {
		return closeOnError(fmt.Errorf("restrict ACP observation trace permissions: %w", err))
	}
	info, err = file.Stat()
	if err != nil {
		return closeOnError(fmt.Errorf("stat ACP observation trace: %w", err))
	}
	if !info.Mode().IsRegular() {
		return closeOnError(fmt.Errorf("ACP observation path must name a regular file"))
	}
	if info.Size() > acpObservationTraceMaxBytes {
		return closeOnError(fmt.Errorf("existing ACP observation trace exceeds %d bytes", acpObservationTraceMaxBytes))
	}
	return &acpObservationWriter{file: file, path: path, attempt: attempt}, nil
}

func (w *acpObservationWriter) lifecycle(phase, result string, exitCode *int) error {
	return w.append(acpObservationRecord{
		Version: 1, PID: os.Getpid(), Phase: phase, Attempt: w.attempt, Result: result, ExitCode: exitCode,
	})
}

func (w *acpObservationWriter) rpc(direction, method string, id json.RawMessage, result string, rpcErr *rpcError) error {
	if w == nil {
		return nil
	}
	record := acpObservationRecord{
		Version: 1, PID: os.Getpid(), Phase: "rpc", Attempt: w.attempt,
		Direction: direction, Method: acpObservationMethod(method), ID: acpObservationID(id, direction), Result: result,
	}
	if rpcErr != nil {
		record.Error = &acpObservationRPCError{Code: rpcErr.Code, Data: acpObservationErrorData(rpcErr.Data)}
	}
	return w.append(record)
}

func (w *acpObservationWriter) append(record acpObservationRecord) error {
	if w == nil {
		return nil
	}
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode ACP observation record: %w", err)
	}
	data = append(data, '\n')
	lockPath := w.path + ".lock"
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("acquire ACP observation trace lock: %w", err)
	}
	defer os.Remove(lockPath)
	defer lock.Close()
	info, err := w.file.Stat()
	if err != nil {
		return fmt.Errorf("stat ACP observation trace: %w", err)
	}
	if int64(len(data)) > acpObservationTraceMaxBytes-info.Size() {
		return fmt.Errorf("%w (%d bytes)", errACPObservationTraceLimit, acpObservationTraceMaxBytes)
	}
	n, err := w.file.Write(data)
	if err != nil {
		return fmt.Errorf("write ACP observation trace: %w", err)
	}
	if n != len(data) {
		return fmt.Errorf("write ACP observation trace: %w", io.ErrShortWrite)
	}
	return nil
}

func (w *acpObservationWriter) close() error {
	if w == nil {
		return nil
	}
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("close ACP observation trace: %w", err)
	}
	return nil
}

func acpObservationMethod(method string) string {
	switch method {
	case "initialize", "session/new", "session/load", "session/set_config_option", "session/prompt", "session/cancel", "$/cancel_request", "session/update", "fs/read_text_file", "fs/write_text_file", "terminal/create", "terminal/kill", "terminal/output", "terminal/release", "terminal/wait_for_exit":
		return method
	default:
		return "unlisted"
	}
}

func acpObservationID(raw json.RawMessage, direction string) json.RawMessage {
	if len(raw) == 0 || len(raw) > 24 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil
	}
	switch value := value.(type) {
	case json.Number:
		if _, err := strconv.ParseInt(value.String(), 10, 64); err == nil {
			return json.RawMessage(value.String())
		}
	case string:
		if direction == "peer_to_client" && strings.HasPrefix(value, "peer-") {
			if _, err := strconv.ParseUint(strings.TrimPrefix(value, "peer-"), 10, 16); err == nil {
				encoded, _ := json.Marshal(value)
				return encoded
			}
		}
	}
	return nil
}

func acpObservationErrorData(data any) map[string]string {
	values, ok := data.(map[string]any)
	if !ok {
		return nil
	}
	message, ok := values["error"].(string)
	if !ok {
		return nil
	}
	if message == "temporarily unavailable" {
		return map[string]string{"error": message}
	}
	return map[string]string{"error": "[redacted]"}
}

func readACPObservationRecords(path string) ([]acpObservationRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) > acpObservationTraceMaxBytes {
		return nil, fmt.Errorf("ACP observation trace has %d bytes, exceeds cap %d", len(data), acpObservationTraceMaxBytes)
	}
	var records []acpObservationRecord
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var record acpObservationRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, fmt.Errorf("decode ACP observation record: %w", err)
		}
		records = append(records, record)
	}
	return records, nil
}

type acpPublicWorkObservation struct {
	ID    string `json:"id,omitempty"`
	State string `json:"state,omitempty"`
}

type acpPublicEventObservation struct {
	ID                string   `json:"id,omitempty"`
	Type              string   `json:"type,omitempty"`
	WorkIDs           []string `json:"workIds,omitempty"`
	SessionID         string   `json:"sessionId,omitempty"`
	DispatchID        string   `json:"dispatchId,omitempty"`
	Attempt           int      `json:"attempt,omitempty"`
	Outcome           string   `json:"outcome,omitempty"`
	Provider          string   `json:"provider,omitempty"`
	ProviderSessionID string   `json:"providerSessionId,omitempty"`
}

type acpPublicOutcomeObservation struct {
	WorkCount  int                         `json:"workCount"`
	Works      []acpPublicWorkObservation  `json:"works"`
	EventCount int                         `json:"eventCount"`
	Events     []acpPublicEventObservation `json:"events"`
	Truncated  bool                        `json:"truncated,omitempty"`
}

func summarizeACPPublicOutcome(work factoryapi.ListWorkResponse, events []factoryapi.FactoryEvent) string {
	const maxItems = 12
	summary := acpPublicOutcomeObservation{
		WorkCount: len(work.Results), EventCount: len(events),
		Works:  make([]acpPublicWorkObservation, 0, min(maxItems, len(work.Results))),
		Events: make([]acpPublicEventObservation, 0, min(maxItems, len(events))),
	}
	for i, item := range work.Results {
		if i >= maxItems {
			summary.Truncated = true
			break
		}
		observed := acpPublicWorkObservation{}
		if item.WorkId != nil {
			observed.ID = boundedACPSummaryValue(*item.WorkId)
		}
		if item.State != nil {
			observed.State = boundedACPSummaryValue(item.State.Name)
		}
		summary.Works = append(summary.Works, observed)
	}
	start := max(0, len(events)-maxItems)
	if start > 0 {
		summary.Truncated = true
	}
	for _, event := range events[start:] {
		observed := acpPublicEventObservation{ID: boundedACPSummaryValue(event.Id), Type: boundedACPSummaryValue(string(event.Type))}
		if event.Context.SessionId != nil {
			observed.SessionID = boundedACPSummaryValue(*event.Context.SessionId)
		}
		if event.Context.DispatchId != nil {
			observed.DispatchID = boundedACPSummaryValue(*event.Context.DispatchId)
		}
		if event.Context.WorkIds != nil {
			for i, id := range *event.Context.WorkIds {
				if i >= maxItems {
					summary.Truncated = true
					break
				}
				observed.WorkIDs = append(observed.WorkIDs, boundedACPSummaryValue(id))
			}
		}
		if event.Type == factoryapi.FactoryEventTypeModelResponse {
			payload, err := event.Payload.AsModelResponseEventPayload()
			if err == nil {
				observed.Attempt = payload.Attempt
				observed.Outcome = boundedACPSummaryValue(string(payload.Outcome))
				if payload.ProviderSession != nil {
					if payload.ProviderSession.Provider != nil {
						observed.Provider = boundedACPSummaryValue(*payload.ProviderSession.Provider)
					}
					if payload.ProviderSession.Id != nil {
						observed.ProviderSessionID = boundedACPSummaryValue(*payload.ProviderSession.Id)
					}
				}
			}
		}
		summary.Events = append(summary.Events, observed)
	}
	encoded, err := json.Marshal(summary)
	if err != nil || len(encoded) > 8*1024 {
		return `{"summary":"bounded public observation unavailable"}`
	}
	return string(encoded)
}

func boundedACPSummaryValue(value string) string {
	if len(value) > 128 || strings.ContainsAny(value, "\r\n\x00") {
		return "[omitted]"
	}
	return value
}

func TestACPFixtureObservationOptIn(t *testing.T) {
	t.Run("disabled helper does not create a trace", testACPObservationDisabled)
	t.Run("enabled retry helper records bounded redacted RPC", testACPObservationEnabled)
	t.Run("invalid paths and trace-write failures fail explicitly", testACPObservationPathFailures)
	t.Run("trace stops at its byte cap", testACPObservationByteCap)
}

func testACPObservationDisabled(t *testing.T) {
	traceDirectory := t.TempDir()
	fixture := functionalACPFixture("1")
	var stdout, stderr bytes.Buffer
	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n")
	if err := runFunctionalRPCPeer(fixture, input, &stdout, &stderr); err != nil {
		t.Fatalf("run ordinary fixture peer: %v", err)
	}
	entries, err := os.ReadDir(traceDirectory)
	if err != nil {
		t.Fatalf("read observation directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("peer without observation path created files: %#v", entries)
	}
	if stdout.Len() == 0 {
		t.Fatal("ordinary helper response was not written")
	}
	if stderr.Len() != 0 {
		t.Fatalf("ordinary helper wrote observation output to stderr: %q", stderr.String())
	}
}

func testACPObservationEnabled(t *testing.T) {
	dir := t.TempDir()
	attemptDir := filepath.Join(dir, "attempts")
	tracePath := filepath.Join(dir, "rpc.jsonl")
	fixture := functionalACPFixture("retry-resume")
	fixture.SessionID = "acp-observation-test-session"
	fixture.RetryAttemptDirectory = attemptDir
	fixture.ObservationPath = tracePath
	encoded := encodeACPFixture(fixture)
	decoded, err := decodeACPFixture(encoded)
	if err != nil {
		t.Fatalf("decode enabled fixture payload: %v", err)
	}
	fixture = decoded
	var stdout, stderr bytes.Buffer
	input := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"token":"sentinel-token"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"environment":"sentinel-environment"}}`,
		`{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"prompt":"sentinel-prompt"}}`,
	}, "\n") + "\n")
	if err := runFunctionalRPCPeer(fixture, input, &stdout, &stderr); err != nil {
		t.Fatalf("run observed retry peer: %v", err)
	}
	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	if len(data) == 0 || len(data) > acpObservationTraceMaxBytes {
		t.Fatalf("trace size = %d, want 1..%d bytes", len(data), acpObservationTraceMaxBytes)
	}
	for _, secret := range []string{"sentinel-token", "sentinel-environment", "sentinel-prompt"} {
		if bytes.Contains(data, []byte(secret)) {
			t.Fatalf("trace contains sensitive sentinel %q", secret)
		}
	}
	assertACPObservationFileMode(t, tracePath)
	assertACPObservationRetryRecords(t, tracePath)
	assertACPObservationErrorDataRedacted(t)
}

func assertACPObservationFileMode(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat trace: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("trace permissions = %04o, want 0600", got)
	}
}

func assertACPObservationRetryRecords(t *testing.T, path string) {
	t.Helper()
	records, err := readACPObservationRecords(path)
	if err != nil {
		t.Fatalf("decode trace: %v", err)
	}
	var rpcRecords int
	firstPromptFailure := false
	for _, record := range records {
		if record.Phase != "rpc" {
			continue
		}
		rpcRecords++
		if record.Direction == "peer_to_client" && record.Method == "session/prompt" && record.Error != nil {
			if record.Error.Code != -32001 || record.Result != "error" || record.Attempt != 1 {
				t.Fatalf("first retry prompt error = %#v, want code -32001", record.Error)
			}
			firstPromptFailure = true
		}
	}
	if rpcRecords != 6 {
		t.Fatalf("RPC trace records = %d, want three request/response pairs", rpcRecords)
	}
	if !firstPromptFailure {
		t.Fatal("trace omitted the first retry prompt error")
	}
}

func assertACPObservationErrorDataRedacted(t *testing.T) {
	t.Helper()
	redacted, err := json.Marshal(acpObservationErrorData(map[string]any{"error": "sentinel-error-token"}))
	if err != nil {
		t.Fatalf("encode sanitized error data: %v", err)
	}
	if strings.Contains(string(redacted), "sentinel-error-token") || string(redacted) != `{"error":"[redacted]"}` {
		t.Fatalf("sanitized error data = %s", redacted)
	}
}

func testACPObservationPathFailures(t *testing.T) {
	fixture := functionalACPFixture("1")
	fixture.ObservationPath = "relative/rpc.jsonl"
	if _, err := decodeACPFixture(encodeACPFixture(fixture)); err == nil {
		t.Fatal("relative observation path was accepted")
	}
	fixture.ObservationPath = filepath.Join(t.TempDir(), "missing", "rpc.jsonl")
	var stdout, stderr bytes.Buffer
	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n")
	err := runFunctionalRPCPeer(fixture, input, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "open ACP observation trace") {
		t.Fatalf("trace-write error = %v, want explicit observation failure", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Factory-facing response bytes = %d, want none after trace failure", stdout.Len())
	}
}

func testACPObservationByteCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rpc.jsonl")
	writer, err := newACPObservationWriter(path, 1)
	if err != nil {
		t.Fatalf("open trace: %v", err)
	}
	defer writer.close()
	var limitErr error
	for i := 0; i < 2000; i++ {
		limitErr = writer.rpc("client_to_peer", "session/prompt", json.RawMessage("3"), "request", nil)
		if limitErr != nil {
			break
		}
	}
	if !errors.Is(limitErr, errACPObservationTraceLimit) {
		t.Fatalf("trace cap error = %v, want explicit byte-limit error", limitErr)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat trace: %v", err)
	}
	if info.Size() > acpObservationTraceMaxBytes {
		t.Fatalf("trace size = %d, exceeds cap %d", info.Size(), acpObservationTraceMaxBytes)
	}
}
