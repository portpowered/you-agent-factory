package acp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/testutil"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	acpProductArtifactPathEnv     = "INFINITE_YOU_PREBUILT_ARTIFACT"
	acpProductArtifactRequiredEnv = "INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT"
	acpProductSourceHeadEnv       = "INFINITE_YOU_SOURCE_HEAD"
	acpProductMainPackage         = "github.com/portpowered/infinite-you/cmd/factory"
	acpFactoryRunTimeout          = 45 * time.Second
)

// TestACPFactoryRetryJoinsHelperAndFactoryEvidence runs the prebuilt product
// and prebuilt ACP peer in one scenario. It joins redacted helper/RPC evidence
// to the public Factory Session, Work, ModelResponse, Provider Session and
// terminal Factory Event before making the acceptance assertions.
func TestACPFactoryRetryJoinsHelperAndFactoryEvidence(t *testing.T) {
	t.Parallel()
	productPath := requirePrebuiltACPProduct(t)
	helperPath := requirePrebuiltACPHelper(t)
	factorySessionID := uuid.NewString()
	providerSessionID := "acp-session-retry-resume"
	providerID := "retry-acp"

	root := t.TempDir()
	fixtureSource := acpFunctionalFixtureSource(t)
	factoryDir := testutil.CopyFixtureDir(t, fixtureSource)
	testutil.WriteSeedFile(t, factoryDir, "task", []byte(`{"title":"same-run ACP Factory retry"}`))
	writeIntegrationACPWorker(t, factoryDir, providerID)

	home := filepath.Join(root, "home")
	if err := os.MkdirAll(filepath.Join(home, ".you-agent-factory"), 0o700); err != nil {
		t.Fatalf("create invocation home: %v", err)
	}
	retryAttemptDir := filepath.Join(root, "retry-attempts")
	traceDir := filepath.Join(root, "observation")
	if err := os.Mkdir(traceDir, 0o700); err != nil {
		t.Fatalf("create private observation directory: %v", err)
	}
	tracePath := filepath.Join(traceDir, "rpc.jsonl")
	fixture := acpHelperFixture{
		Kind:                  "functional",
		Mode:                  "retry-resume",
		SessionID:             providerSessionID,
		RetryAttemptDirectory: retryAttemptDir,
		ObservationPath:       tracePath,
	}
	encodedFixture, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("encode ACP helper fixture: %v", err)
	}
	helperArgs := strings.Join([]string{
		strconv.Quote(helperPath),
		"-test.run=^TestACPAgentHelperProcess$",
		"--",
		acpHelperFixtureArg + base64.RawURLEncoding.EncodeToString(encodedFixture),
	}, " ")
	config := struct {
		Workers struct {
			ACP struct {
				Integrations []struct {
					ID        string `json:"id"`
					Name      string `json:"name"`
					Transport string `json:"transport"`
					Command   string `json:"command"`
				} `json:"integrations"`
			} `json:"acp"`
		} `json:"workers"`
	}{}
	config.Workers.ACP.Integrations = append(config.Workers.ACP.Integrations, struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Transport string `json:"transport"`
		Command   string `json:"command"`
	}{ID: "retry-entry", Name: providerID, Transport: "stdio", Command: helperArgs})
	configBytes, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("encode ACP operator config: %v", err)
	}
	configPath := filepath.Join(home, ".you-agent-factory", "config.json")
	if err := os.WriteFile(configPath, configBytes, 0o600); err != nil {
		t.Fatalf("write invocation ACP config: %v", err)
	}

	port := reserveLoopbackPort(t)
	baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
	ctx, cancel := context.WithTimeout(t.Context(), acpFactoryRunTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, productPath,
		"run", "--dir", factoryDir,
		"--continuously", "--with-server", "--listen", "127.0.0.1:"+strconv.Itoa(port),
		"--session", factorySessionID, "--no-record", "--quiet",
	)
	command.Dir = factoryDir
	command.Env = withACPEnvironment(os.Environ(), "HOME", home)
	command.Env = withACPEnvironment(command.Env, "USERPROFILE", home)
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start prebuilt product %s: %v", productPath, err)
	}
	waitDone := make(chan error, 1)
	go func() { waitDone <- command.Wait() }()
	defer func() {
		if command.ProcessState != nil {
			return
		}
		_ = command.Process.Kill()
		select {
		case <-waitDone:
		case <-time.After(5 * time.Second):
			t.Errorf("prebuilt product did not exit after test cleanup")
		}
	}()

	if err := waitForACPFactorySession(ctx, baseURL, factorySessionID, waitDone, func() string {
		return stdout.String() + "\n" + stderr.String()
	}); err != nil {
		t.Fatalf("wait for public Factory Session: %v", err)
	}
	terminalEvent, err := waitForACPDispatchResponse(ctx, baseURL, factorySessionID, waitDone, func() string {
		return stdout.String() + "\n" + stderr.String()
	})
	if err != nil {
		t.Fatalf("wait for terminal Factory Event: %v", err)
	}

	session, err := getACPFactorySession(ctx, baseURL, factorySessionID)
	if err != nil {
		t.Fatalf("read public Factory Session: %v", err)
	}
	listed, err := getACPWork(ctx, baseURL, factorySessionID)
	if err != nil {
		t.Fatalf("read session-scoped public Work: %v", err)
	}
	events, err := readACPFactoryEvents(ctx, baseURL, factorySessionID)
	if err != nil {
		t.Fatalf("read session-scoped Factory Event history: %v", err)
	}
	traceRecords, err := readACPIntegrationObservationRecords(tracePath)
	if err != nil {
		t.Fatalf("read same-run ACP helper trace: %v", err)
	}
	joined := summarizeACPFactoryJoin(
		factorySessionID, providerID, providerSessionID, session, listed, events, traceRecords,
	)
	t.Logf("Same-run ACP-to-Factory join: %s", joined)

	if session.Id != factorySessionID {
		t.Fatalf("Factory Session ID = %q, want %q; %s", session.Id, factorySessionID, joined)
	}
	if len(listed.Results) != 1 || customerWorkLocation(listed.Results[0]) != "task:done" {
		t.Fatalf("public Work outcome = %#v, want one task:done Work; %s", listed.Results, joined)
	}
	workID := pointerString(listed.Results[0].WorkId)
	if workID == "" {
		t.Fatalf("public Work ID is missing; %s", joined)
	}
	if !eventHasWork(terminalEvent, workID) || terminalEvent.Id == "" || terminalEvent.Context.DispatchId == nil || *terminalEvent.Context.DispatchId == "" {
		t.Fatalf("terminal Factory Event is not joined to Work %q and a Dispatch ID: %#v; %s", workID, terminalEvent, joined)
	}
	if !eventHasWorkIDWithSession(terminalEvent, workID, factorySessionID) {
		t.Fatalf("terminal Factory Event session/work correlation is incomplete; %s", joined)
	}
	modelResponses, err := joinedModelResponses(events, workID)
	if err != nil {
		t.Fatalf("decode public ModelResponse events: %v; %s", err, joined)
	}
	if len(modelResponses) != 1 {
		t.Fatalf("ModelResponse event count for Work %q = %d, want the final result for the single Factory model invocation; %s", workID, len(modelResponses), joined)
	}
	for _, response := range modelResponses {
		if response.payload.Outcome != factoryapi.InferenceOutcomeSucceeded {
			t.Fatalf("ModelResponse outcome = %q, want SUCCEEDED after the helper retry; %s", response.payload.Outcome, joined)
		}
		if response.event.Context.SessionId == nil || *response.event.Context.SessionId != factorySessionID {
			t.Fatalf("ModelResponse Factory Session = %v, want %q; %s", response.event.Context.SessionId, factorySessionID, joined)
		}
		if response.payload.ProviderSession == nil || pointerString(response.payload.ProviderSession.Provider) != providerID ||
			pointerString(response.payload.ProviderSession.Id) != providerSessionID {
			t.Fatalf("ModelResponse Provider Session = %#v, want %s/%s; %s", response.payload.ProviderSession, providerID, providerSessionID, joined)
		}
	}
	validateRetryObservationTrace(t, tracePath)
	if !terminalEventInHistory(events, terminalEvent.Id, workID, factorySessionID) {
		t.Fatalf("terminal event %q is missing from retained session history; %s", terminalEvent.Id, joined)
	}
	for _, event := range events {
		gotSessionID := eventSessionID(event)
		if gotSessionID == "" {
			if len(eventWorkIDs(event)) > 0 || event.Context.DispatchId != nil {
				t.Fatalf("session-scoped Factory Event %q omitted Factory Session ID; %s", event.Id, joined)
			}
			continue
		}
		if gotSessionID != factorySessionID {
			t.Fatalf("Factory Event %q escaped selected session %q; %s", event.Id, factorySessionID, joined)
		}
	}

	productInfo, err := buildinfo.ReadFile(productPath)
	if err != nil {
		t.Fatalf("read prebuilt product build identity: %v", err)
	}
	productBytes, err := os.ReadFile(productPath)
	if err != nil {
		t.Fatalf("read prebuilt product artifact: %v", err)
	}
	productHash := sha256.Sum256(productBytes)
	t.Logf("Prebuilt product sha256=%s main=%s module=%s vcs.revision=%s vcs.modified=%s", hex.EncodeToString(productHash[:]), productInfo.Path, productInfo.Main.Path, buildInfoSetting(productInfo.Settings, "vcs.revision"), buildInfoSetting(productInfo.Settings, "vcs.modified"))
}

type acpJoinedModelResponse struct {
	event   factoryapi.FactoryEvent
	payload factoryapi.ModelResponseEventPayload
}

func requirePrebuiltACPProduct(t *testing.T) string {
	t.Helper()
	path := strings.TrimSpace(os.Getenv(acpProductArtifactPathEnv))
	if path == "" {
		if strings.EqualFold(strings.TrimSpace(os.Getenv(acpProductArtifactRequiredEnv)), "1") {
			t.Fatalf("%s is required; ACP integration never builds the product", acpProductArtifactPathEnv)
		}
		t.Skipf("prebuilt product unavailable; set %s", acpProductArtifactPathEnv)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve prebuilt product: %v", err)
	}
	info, err := os.Lstat(absPath)
	if err != nil {
		t.Fatalf("inspect prebuilt product: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		t.Fatalf("prebuilt product %q is not a regular file", absPath)
	}
	build, err := buildinfo.ReadFile(absPath)
	if err != nil {
		t.Fatalf("read prebuilt product build identity: %v", err)
	}
	if build.Path != acpProductMainPackage {
		t.Fatalf("prebuilt product main package = %q, want %q", build.Path, acpProductMainPackage)
	}
	if sourceHead := strings.TrimSpace(os.Getenv(acpProductSourceHeadEnv)); sourceHead != "" {
		if revision := buildInfoSetting(build.Settings, "vcs.revision"); revision != sourceHead {
			t.Fatalf("prebuilt product revision = %q, want source head %q", revision, sourceHead)
		}
	}
	return absPath
}

func acpFunctionalFixtureSource(t *testing.T) string {
	t.Helper()
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve ACP integration test source")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(sourcePath), "..", "..", "..", ".."))
	return filepath.Join(repoRoot, "tests", "functional_test", "testdata", "executor_success")
}

func writeIntegrationACPWorker(t *testing.T, factoryDir, providerID string) {
	t.Helper()
	path := filepath.Join(factoryDir, "workers", "worker", "AGENTS.md")
	content := "---\n" +
		"executorProvider: ACP\n" +
		"modelProvider: " + providerID + "\n" +
		"model: test-model\n" +
		"stopToken: COMPLETE\n" +
		"type: MODEL_WORKER\n" +
		"---\n\nTest ACP worker.\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write ACP worker definition: %v", err)
	}
}

func reserveLoopbackPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release selected loopback port: %v", err)
	}
	return port
}

func withACPEnvironment(environment []string, key, value string) []string {
	updated := make([]string, 0, len(environment)+1)
	prefix := key + "="
	for _, item := range environment {
		if strings.HasPrefix(item, prefix) {
			continue
		}
		updated = append(updated, item)
	}
	return append(updated, prefix+value)
}

func waitForACPFactorySession(
	ctx context.Context,
	baseURL, sessionID string,
	processDone <-chan error,
	processOutput func() string,
) error {
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err == nil {
			response, requestErr := http.DefaultClient.Do(request)
			if requestErr == nil {
				_ = response.Body.Close()
				if response.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
		select {
		case err := <-processDone:
			return fmt.Errorf("product exited before Factory Session readiness: %v; output=%s", err, processOutput())
		case <-ctx.Done():
			return fmt.Errorf("Factory Session endpoint did not become ready: %w; output=%s", ctx.Err(), processOutput())
		case <-ticker.C:
		}
	}
}

func waitForACPDispatchResponse(
	ctx context.Context,
	baseURL, sessionID string,
	processDone <-chan error,
	processOutput func() string,
) (factoryapi.FactoryEvent, error) {
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/events"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return factoryapi.FactoryEvent{}, fmt.Errorf("build Factory Event stream request: %w", err)
	}
	request.Header.Set("Accept", "text/event-stream")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return factoryapi.FactoryEvent{}, fmt.Errorf("open Factory Event stream: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return factoryapi.FactoryEvent{}, fmt.Errorf("Factory Event stream status=%d output=%s", response.StatusCode, processOutput())
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			return factoryapi.FactoryEvent{}, fmt.Errorf("decode live Factory Event: %w", err)
		}
		if event.Type == factoryapi.FactoryEventTypeDispatchResponse && eventSessionID(event) == sessionID && len(eventWorkIDs(event)) > 0 {
			return event, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return factoryapi.FactoryEvent{}, fmt.Errorf("read live Factory Event stream: %w", err)
	}
	select {
	case err := <-processDone:
		return factoryapi.FactoryEvent{}, fmt.Errorf("product exited before terminal dispatch event: %v; output=%s", err, processOutput())
	case <-ctx.Done():
		return factoryapi.FactoryEvent{}, fmt.Errorf("terminal dispatch event not observed: %w; output=%s", ctx.Err(), processOutput())
	default:
		return factoryapi.FactoryEvent{}, fmt.Errorf("Factory Event stream ended before terminal dispatch event; output=%s", processOutput())
	}
}

func getACPFactorySession(ctx context.Context, baseURL, sessionID string) (factoryapi.FactorySession, error) {
	var response factoryapi.FactorySessionGetResponse
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	if err := getACPJSON(ctx, endpoint, &response); err != nil {
		return factoryapi.FactorySession{}, err
	}
	return response.AsFactorySession()
}

func getACPWork(ctx context.Context, baseURL, sessionID string) (factoryapi.ListWorkResponse, error) {
	var response factoryapi.ListWorkResponse
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	if err := getACPJSON(ctx, endpoint, &response); err != nil {
		return factoryapi.ListWorkResponse{}, err
	}
	return response, nil
}

func getACPJSON(ctx context.Context, endpoint string, result any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build GET %s: %w", endpoint, err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("GET %s: %w", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s status=%d", endpoint, response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(result); err != nil {
		return fmt.Errorf("decode GET %s: %w", endpoint, err)
	}
	return nil
}

func readACPFactoryEvents(ctx context.Context, baseURL, sessionID string) ([]factoryapi.FactoryEvent, error) {
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/events"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build retained Factory Event request: %w", err)
	}
	request.Header.Set("Accept", "text/event-stream")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("read retained Factory Events: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("retained Factory Event status=%d", response.StatusCode)
	}
	retained, err := strconv.Atoi(strings.TrimSpace(response.Header.Get(factorysessions.SessionEventStreamRetainedCountHeader)))
	if err != nil || retained < 0 {
		return nil, fmt.Errorf("retained Factory Event count %q is invalid", response.Header.Get(factorysessions.SessionEventStreamRetainedCountHeader))
	}
	events := make([]factoryapi.FactoryEvent, 0, retained)
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			return nil, fmt.Errorf("decode retained Factory Event: %w", err)
		}
		events = append(events, event)
		if len(events) == retained {
			return events, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan retained Factory Events: %w", err)
	}
	if len(events) != retained {
		return nil, fmt.Errorf("retained Factory Events returned %d of %d", len(events), retained)
	}
	return events, nil
}

func summarizeACPFactoryJoin(
	factorySessionID, providerID, providerSessionID string,
	session factoryapi.FactorySession,
	listed factoryapi.ListWorkResponse,
	events []factoryapi.FactoryEvent,
	trace []acpObservationRecord,
) string {
	var helpers, rpc, models, terminals []string
	for _, record := range trace {
		switch record.Phase {
		case "start":
			helpers = append(helpers, fmt.Sprintf("attempt=%d pid=%d start", record.Attempt, record.PID))
		case "exit":
			code := "missing"
			if record.ExitCode != nil {
				code = strconv.Itoa(*record.ExitCode)
			}
			helpers = append(helpers, fmt.Sprintf("attempt=%d pid=%d exit=%s/%s", record.Attempt, record.PID, record.Result, code))
		case "rpc":
			id := strings.TrimSpace(string(record.ID))
			if id == "" {
				id = "<missing>"
			}
			errorCode := ""
			if record.Error != nil {
				errorCode = fmt.Sprintf(" error=%d", record.Error.Code)
			}
			rpc = append(rpc, fmt.Sprintf("attempt=%d %s id=%s %s %s%s", record.Attempt, record.Direction, id, record.Method, record.Result, errorCode))
		}
	}
	workID := "<missing>"
	workState := "<missing>"
	workFailure := ""
	if len(listed.Results) == 1 {
		workID = pointerString(listed.Results[0].WorkId)
		workState = customerWorkLocation(listed.Results[0])
		if listed.Results[0].FailureDetail != nil {
			workFailure = fmt.Sprintf("%s:%s", listed.Results[0].FailureDetail.Reason, listed.Results[0].FailureDetail.Message)
		}
	}
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeModelResponse && eventHasWork(event, workID) {
			payload, err := event.Payload.AsModelResponseEventPayload()
			if err != nil {
				models = append(models, fmt.Sprintf("event=%s decode-error=%v", event.Id, err))
				continue
			}
			providerSession := "<missing>"
			if payload.ProviderSession != nil {
				providerSession = pointerString(payload.ProviderSession.Provider) + "/" + pointerString(payload.ProviderSession.Id)
			}
			models = append(models, fmt.Sprintf("event=%s attempt=%d outcome=%s providerSession=%s", event.Id, payload.Attempt, payload.Outcome, providerSession))
		}
		if event.Type == factoryapi.FactoryEventTypeDispatchResponse && eventHasWork(event, workID) {
			dispatchID := "<missing>"
			if event.Context.DispatchId != nil {
				dispatchID = *event.Context.DispatchId
			}
			terminals = append(terminals, fmt.Sprintf("event=%s dispatch=%s work=%s", event.Id, dispatchID, workID))
		}
	}
	if len(models) == 0 {
		models = []string{"<missing>"}
	}
	if len(terminals) == 0 {
		terminals = []string{"<missing>"}
	}
	if len(helpers) == 0 {
		helpers = []string{"<missing>"}
	}
	if len(rpc) == 0 {
		rpc = []string{"<missing>"}
	}
	return fmt.Sprintf(
		"helper=[%s] rpc=[%s] FactorySession=%s/%s Work=%s/%s failure=%q providerSession=%s/%s ModelResponse=[%s] terminalFactoryEvent=[%s]",
		strings.Join(helpers, "; "), strings.Join(rpc, "; "), session.Id, factorySessionID,
		workID, workState, workFailure, providerID, providerSessionID,
		strings.Join(models, "; "), strings.Join(terminals, "; "),
	)
}

func joinedModelResponses(events []factoryapi.FactoryEvent, workID string) ([]acpJoinedModelResponse, error) {
	var responses []acpJoinedModelResponse
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse || !eventHasWork(event, workID) {
			continue
		}
		payload, err := event.Payload.AsModelResponseEventPayload()
		if err != nil {
			return nil, fmt.Errorf("Factory Event %q: %w", event.Id, err)
		}
		responses = append(responses, acpJoinedModelResponse{event: event, payload: payload})
	}
	return responses, nil
}

func eventHasWork(event factoryapi.FactoryEvent, workID string) bool {
	for _, candidate := range eventWorkIDs(event) {
		if candidate == workID {
			return true
		}
	}
	return false
}

func eventHasWorkIDWithSession(event factoryapi.FactoryEvent, workID, sessionID string) bool {
	return eventSessionID(event) == sessionID && eventHasWork(event, workID)
}

func eventWorkIDs(event factoryapi.FactoryEvent) []string {
	if event.Context.WorkIds == nil {
		return nil
	}
	return *event.Context.WorkIds
}

func eventSessionID(event factoryapi.FactoryEvent) string {
	if event.Context.SessionId == nil {
		return ""
	}
	return *event.Context.SessionId
}

func terminalEventInHistory(events []factoryapi.FactoryEvent, eventID, workID, sessionID string) bool {
	for _, event := range events {
		if event.Id == eventID && event.Type == factoryapi.FactoryEventTypeDispatchResponse && eventHasWorkIDWithSession(event, workID, sessionID) {
			return true
		}
	}
	return false
}

func customerWorkLocation(item factoryapi.Work) string {
	if item.WorkTypeName == nil || item.State == nil {
		return ""
	}
	return *item.WorkTypeName + ":" + item.State.Name
}

func pointerString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func readACPIntegrationObservationRecords(path string) ([]acpObservationRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, acpObservationMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data) > acpObservationMaxBytes {
		return nil, fmt.Errorf("ACP observation trace size %d is outside 1..%d bytes", len(data), acpObservationMaxBytes)
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

func buildInfoSetting(settings []debug.BuildSetting, key string) string {
	for _, setting := range settings {
		if setting.Key == key {
			return setting.Value
		}
	}
	return ""
}
