// backendsizecheck:ignore-file pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
package customer_lifecycles_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	wantPackagedGoalPrimaryResult      = "mock worker accepted"
	packagedGoalAcceptedProviderOutput = `{"decision":"accepted","output":"` + wantPackagedGoalPrimaryResult + `"}`
	packagedGoalProviderModel          = "gpt-5-codex"
	// Race instrumentation and concurrent CI lanes can delay production wiring
	// before listener binding; readiness returns immediately once it is bound.
	crossPackagedServerReadyTimeout = 90 * time.Second
)

type packagedGoalAPIInspection struct {
	session          factoryapi.FactorySession
	status           factoryapi.StatusResponse
	listed           factoryapi.ListWorkResponse
	live             bool
	maxProcessing    int
	maxTerminal      int
	maxTotalTokens   int
	sawFactoryActive bool
}

type packagedGoalFactoryEventCollector struct {
	cancel context.CancelFunc
	done   chan struct{}
	events []factoryapi.FactoryEvent
}

func startPackagedGoalFactoryEventCollector(
	ctx context.Context,
	t *testing.T,
	baseURL string,
) *packagedGoalFactoryEventCollector {
	t.Helper()

	streamCtx, cancel := context.WithCancel(ctx)
	collector := &packagedGoalFactoryEventCollector{
		cancel: cancel,
		done:   make(chan struct{}),
	}
	request, err := http.NewRequestWithContext(
		streamCtx,
		http.MethodGet,
		support.DefaultSessionEventsURL(baseURL),
		nil,
	)
	if err != nil {
		cancel()
		t.Fatalf("build factory event stream request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		cancel()
		t.Fatalf("GET factory event stream: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		response.Body.Close()
		cancel()
		t.Fatalf(
			"GET factory event stream status = %d, want 200: %s",
			response.StatusCode,
			string(payload),
		)
	}

	go func() {
		defer close(collector.done)
		defer response.Body.Close()
		scanner := bufio.NewScanner(response.Body)
		var dataLines []string
		flush := func() {
			if len(dataLines) == 0 {
				return
			}
			var event factoryapi.FactoryEvent
			if err := json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &event); err == nil {
				collector.events = append(collector.events, event)
			}
			dataLines = nil
		}
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				flush()
				continue
			}
			if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
	}()
	return collector
}

func (collector *packagedGoalFactoryEventCollector) stop() {
	if collector == nil || collector.cancel == nil {
		return
	}
	collector.cancel()
	<-collector.done
}

func (collector *packagedGoalFactoryEventCollector) snapshot() []factoryapi.FactoryEvent {
	if collector == nil {
		return nil
	}
	return append([]factoryapi.FactoryEvent(nil), collector.events...)
}

func pollPackagedGoalAPIInspectionUntilCLICompletes(
	ctx context.Context,
	t *testing.T,
	baseURL string,
	execDone <-chan error,
	releaseServerShutdown func(),
) (packagedGoalAPIInspection, error) {
	t.Helper()

	var snapshot packagedGoalAPIInspection
	if err := waitForSessionWorkEndpoint(ctx, baseURL, 30*time.Second); err != nil {
		t.Fatalf("wait for CLI-hosted API server: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return snapshot, ctx.Err()
		default:
		}

		if candidate, err := fetchPackagedGoalAPIInspectionSnapshot(baseURL); err == nil {
			snapshot = preferStrongerPackagedGoalAPIInspection(snapshot, candidate)
			if hasPackagedGoalTerminalWork(snapshot.listed) && releaseServerShutdown != nil {
				releaseServerShutdown()
			}
		}

		select {
		case execErr := <-execDone:
			if candidate, err := fetchPackagedGoalAPIInspectionSnapshot(baseURL); err == nil {
				snapshot = preferStrongerPackagedGoalAPIInspection(snapshot, candidate)
				if hasPackagedGoalTerminalWork(snapshot.listed) && releaseServerShutdown != nil {
					releaseServerShutdown()
				}
			}
			return snapshot, execErr
		case <-ctx.Done():
			return snapshot, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func fetchPackagedGoalAPIInspectionSnapshot(baseURL string) (packagedGoalAPIInspection, error) {
	session, sessionErr := tryGetDefaultSession(baseURL)
	status, statusErr := tryGetStatus(baseURL)
	listed, listErr := tryListDefaultSessionWork(baseURL)
	if sessionErr != nil {
		return packagedGoalAPIInspection{}, sessionErr
	}
	if statusErr != nil {
		return packagedGoalAPIInspection{}, statusErr
	}
	if listErr != nil {
		return packagedGoalAPIInspection{}, listErr
	}
	if strings.TrimSpace(session.Id) == "" || strings.TrimSpace(status.RuntimeStatus) == "" {
		return packagedGoalAPIInspection{}, fmt.Errorf("session/status not ready")
	}

	return packagedGoalAPIInspection{
		session:          session,
		status:           status,
		listed:           listed,
		live:             true,
		maxProcessing:    status.Categories.Processing,
		maxTerminal:      status.Categories.Terminal,
		maxTotalTokens:   status.TotalTokens,
		sawFactoryActive: status.FactoryState == "RUNNING" && status.RuntimeStatus != "",
	}, nil
}

func mergePackagedGoalAPIInspectionMetrics(
	current packagedGoalAPIInspection,
	candidate packagedGoalAPIInspection,
) packagedGoalAPIInspection {
	if candidate.maxProcessing > current.maxProcessing {
		current.maxProcessing = candidate.maxProcessing
	}
	if candidate.maxTerminal > current.maxTerminal {
		current.maxTerminal = candidate.maxTerminal
	}
	if candidate.maxTotalTokens > current.maxTotalTokens {
		current.maxTotalTokens = candidate.maxTotalTokens
	}
	if candidate.sawFactoryActive {
		current.sawFactoryActive = true
	}
	return current
}

func preferStrongerPackagedGoalAPIInspection(
	current packagedGoalAPIInspection,
	candidate packagedGoalAPIInspection,
) packagedGoalAPIInspection {
	if !candidate.live {
		return current
	}
	if !current.live {
		return candidate
	}
	merged := mergePackagedGoalAPIInspectionMetrics(current, candidate)
	if hasPackagedGoalAPIInspectabilityEvidence(candidate) &&
		!hasPackagedGoalAPIInspectabilityEvidence(current) {
		merged.session = candidate.session
		merged.status = candidate.status
		merged.listed = candidate.listed
		return merged
	}
	if len(candidate.listed.Results) > len(current.listed.Results) {
		merged.session = candidate.session
		merged.status = candidate.status
		merged.listed = candidate.listed
	}
	return merged
}

func hasPackagedGoalAPIInspectabilityEvidence(snapshot packagedGoalAPIInspection) bool {
	return hasPackagedGoalTerminalWork(snapshot.listed)
}

func hasPackagedGoalTerminalWork(listed factoryapi.ListWorkResponse) bool {
	for _, work := range listed.Results {
		if work.WorkTypeName == nil || *work.WorkTypeName != "goal" || work.State == nil {
			continue
		}
		if work.State.Type == factoryapi.WorkStateTypeTERMINAL ||
			work.State.Type == factoryapi.WorkStateTypeFAILED {
			return true
		}
	}
	return false
}

func hasPackagedGoalCompleteWork(listed factoryapi.ListWorkResponse) bool {
	for _, work := range listed.Results {
		if work.WorkTypeName == nil || *work.WorkTypeName != "goal" {
			continue
		}
		if support.HasWorkAtCustomerState(listed, stringValue(work.WorkId), "goal:complete") {
			return true
		}
	}
	return false
}

func tryGetDefaultSession(baseURL string) (factoryapi.FactorySession, error) {
	response, err := http.Get(
		strings.TrimSuffix(baseURL, "/") + "/factory-sessions/~default",
	)
	if err != nil {
		return factoryapi.FactorySession{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return factoryapi.FactorySession{}, fmt.Errorf("status = %d", response.StatusCode)
	}
	var decoded factoryapi.FactorySessionGetResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return factoryapi.FactorySession{}, err
	}
	return decoded.AsFactorySession()
}

func tryGetStatus(baseURL string) (factoryapi.StatusResponse, error) {
	response, err := http.Get(strings.TrimSuffix(baseURL, "/") + "/status")
	if err != nil {
		return factoryapi.StatusResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return factoryapi.StatusResponse{}, fmt.Errorf("status = %d", response.StatusCode)
	}
	var decoded factoryapi.StatusResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return factoryapi.StatusResponse{}, err
	}
	return decoded, nil
}

func tryListDefaultSessionWork(baseURL string) (factoryapi.ListWorkResponse, error) {
	return tryListDefaultSessionWorkByTrace(baseURL, "")
}

func tryListDefaultSessionWorkByTrace(
	baseURL string,
	traceID string,
) (factoryapi.ListWorkResponse, error) {
	url := support.DefaultSessionWorkURL(baseURL, "/work")
	if strings.TrimSpace(traceID) != "" {
		url += "?traceId=" + traceID
	}
	response, err := http.Get(url)
	if err != nil {
		return factoryapi.ListWorkResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return factoryapi.ListWorkResponse{}, fmt.Errorf("status = %d", response.StatusCode)
	}
	var decoded factoryapi.ListWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return factoryapi.ListWorkResponse{}, err
	}
	return decoded, nil
}

func mergePackagedGoalFactoryEvents(
	collected []factoryapi.FactoryEvent,
	retained []factoryapi.FactoryEvent,
) []factoryapi.FactoryEvent {
	seen := make(map[string]struct{}, len(collected)+len(retained))
	merged := make([]factoryapi.FactoryEvent, 0, len(collected)+len(retained))
	for _, batch := range [][]factoryapi.FactoryEvent{collected, retained} {
		for _, event := range batch {
			if event.Id == "" {
				continue
			}
			if _, ok := seen[event.Id]; ok {
				continue
			}
			seen[event.Id] = struct{}{}
			merged = append(merged, event)
		}
	}
	return merged
}

func tryGetRetainedFactoryEvents(baseURL string) ([]factoryapi.FactoryEvent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		support.DefaultSessionEventsURL(baseURL),
		nil,
	)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status = %d", response.StatusCode)
	}
	scanner := bufio.NewScanner(response.Body)
	var dataLines []string
	var events []factoryapi.FactoryEvent
	flush := func() {
		if len(dataLines) == 0 {
			return
		}
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &event); err == nil {
			events = append(events, event)
		}
		dataLines = nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	flush()
	return events, scanner.Err()
}

func assertPackagedGoalCLIInvocationInspectableByAPI(
	t *testing.T,
	cliResponse factoryapi.InvocationResponse,
	inspection packagedGoalAPIInspection,
	events []factoryapi.FactoryEvent,
	wantPrimaryResult string,
) {
	t.Helper()

	session := inspection.session
	status := inspection.status
	listed := inspection.listed

	if cliResponse.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("CLI status = %q, want COMPLETED", cliResponse.Status)
	}
	if strings.TrimSpace(cliResponse.RequestId) == "" || strings.TrimSpace(cliResponse.TraceId) == "" {
		t.Fatalf(
			"CLI invocation identity = request %q trace %q, want non-empty public correlation",
			cliResponse.RequestId,
			cliResponse.TraceId,
		)
	}
	cliPrimary := invocationPrimaryResultText(t, cliResponse)
	if cliPrimary != wantPrimaryResult {
		t.Fatalf("CLI primaryResult = %q, want %q", cliPrimary, wantPrimaryResult)
	}

	if strings.TrimSpace(session.Id) == "" {
		t.Fatal("GET /factory-sessions/~default returned empty session id")
	}
	if status.RuntimeStatus == "" {
		t.Fatal("GET /status returned empty runtimeStatus")
	}
	if cliResponse.SessionId != nil && strings.TrimSpace(*cliResponse.SessionId) != "" &&
		session.Id != strings.TrimSpace(*cliResponse.SessionId) {
		t.Fatalf(
			"API session id = %q, want CLI sessionId %q",
			session.Id,
			strings.TrimSpace(*cliResponse.SessionId),
		)
	}

	correlatedWork, correlated := findPackagedGoalWorkCorrelatedWithCLIInvocation(listed, cliResponse)
	if correlated {
		if correlatedWork.RequestId != nil &&
			strings.TrimSpace(*correlatedWork.RequestId) != "" &&
			*correlatedWork.RequestId != cliResponse.RequestId {
			t.Fatalf(
				"correlated API work requestId = %q, want CLI requestId %q",
				*correlatedWork.RequestId,
				cliResponse.RequestId,
			)
		}
		if traceID := packagedGoalWorkTraceID(correlatedWork); traceID != "" && traceID != cliResponse.TraceId {
			t.Fatalf(
				"correlated API work trace = %q, want CLI traceId %q",
				traceID,
				cliResponse.TraceId,
			)
		}
	}

	hasGoalComplete := hasPackagedGoalCompleteWork(listed)
	hasCorrelatedEvent := packagedGoalFactoryEventsCorrelateWithCLIInvocation(events, cliResponse)
	hasTraceCorrelatedWork := packagedGoalListedWorkCorrelatesWithCLIInvocation(listed, cliResponse)
	if !hasGoalComplete && !hasCorrelatedEvent && !hasTraceCorrelatedWork {
		t.Fatalf(
			"API inspectability evidence missing for CLI-started run: need goal:complete work, identity-correlated factory events, or trace/request-correlated listed work; requestId=%q traceId=%q workId=%q events=%d listed=%#v status=%#v inspection=%#v",
			cliResponse.RequestId,
			cliResponse.TraceId,
			stringValue(cliResponse.WorkId),
			len(events),
			listed.Results,
			status,
			inspection,
		)
	}
	if correlated && !support.HasWorkAtCustomerState(
		listed,
		stringValue(correlatedWork.WorkId),
		"goal:complete",
	) && !hasCorrelatedEvent && !hasTraceCorrelatedWork {
		t.Fatalf(
			"correlated API goal work %q is not at goal:complete and no factory events correlated to CLI identity were observed",
			stringValue(correlatedWork.WorkId),
		)
	}
}

func packagedGoalListedWorkCorrelatesWithCLIInvocation(
	listed factoryapi.ListWorkResponse,
	cliResponse factoryapi.InvocationResponse,
) bool {
	_, correlated := findPackagedGoalWorkCorrelatedWithCLIInvocation(listed, cliResponse)
	return correlated
}

func packagedGoalFactoryEventsCorrelateWithCLIInvocation(
	events []factoryapi.FactoryEvent,
	cliResponse factoryapi.InvocationResponse,
) bool {
	for _, event := range events {
		if event.Context.RequestId != nil &&
			*event.Context.RequestId == cliResponse.RequestId {
			return true
		}
		if event.Context.CurrentChainingTraceId != nil &&
			*event.Context.CurrentChainingTraceId == cliResponse.TraceId {
			return true
		}
		if event.Context.TraceIds != nil {
			for _, traceID := range *event.Context.TraceIds {
				if traceID == cliResponse.TraceId {
					return true
				}
			}
		}
		if cliResponse.WorkId != nil && event.Context.WorkIds != nil {
			cliWorkID := strings.TrimSpace(*cliResponse.WorkId)
			if cliWorkID != "" {
				for _, workID := range *event.Context.WorkIds {
					if workID == cliWorkID {
						return true
					}
				}
			}
		}
	}
	return false
}

func findPackagedGoalWorkCorrelatedWithCLIInvocation(
	listed factoryapi.ListWorkResponse,
	cliResponse factoryapi.InvocationResponse,
) (factoryapi.Work, bool) {
	for _, work := range listed.Results {
		if work.WorkTypeName == nil || *work.WorkTypeName != "goal" {
			continue
		}
		if packagedGoalWorkCorrelatesWithCLIInvocation(work, cliResponse) {
			return work, true
		}
	}
	return factoryapi.Work{}, false
}

func packagedGoalWorkCorrelatesWithCLIInvocation(
	work factoryapi.Work,
	cliResponse factoryapi.InvocationResponse,
) bool {
	if cliResponse.WorkId != nil && work.WorkId != nil &&
		strings.TrimSpace(*cliResponse.WorkId) == strings.TrimSpace(*work.WorkId) {
		return true
	}
	if work.RequestId != nil &&
		strings.TrimSpace(*work.RequestId) != "" &&
		*work.RequestId == cliResponse.RequestId {
		return true
	}
	if traceID := packagedGoalWorkTraceID(work); traceID != "" && traceID == cliResponse.TraceId {
		return true
	}
	return false
}

func packagedGoalWorkTraceID(work factoryapi.Work) string {
	if work.CurrentChainingTraceId != nil && strings.TrimSpace(*work.CurrentChainingTraceId) != "" {
		return strings.TrimSpace(*work.CurrentChainingTraceId)
	}
	if work.TraceId != nil {
		return strings.TrimSpace(*work.TraceId)
	}
	return ""
}

func invocationPrimaryResultText(t *testing.T, response factoryapi.InvocationResponse) string {
	t.Helper()

	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}
	return part.Text
}

func waitForSessionWorkEndpoint(ctx context.Context, baseURL string, timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			support.DefaultSessionWorkURL(baseURL, "/work"),
			nil,
		)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}

	return fmt.Errorf("timed out waiting for GET /work on %s", baseURL)
}

func reserveLocalTCPPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address type %T", listener.Addr())
	}
	return addr.Port, nil
}

// isolatedHomeEnvironment keeps configuration and model discovery in this
// scenario's home without changing the environment used by parallel tests.
func isolatedHomeEnvironment(home string) []string {
	environment := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") ||
			strings.EqualFold(name, runcli.ModelCacheDirEnvironment) {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "HOME="+home, "USERPROFILE="+home,
		runcli.ModelCacheDirEnvironment+"="+filepath.Join(home, ".agent-factory", "models"))
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func scaffoldPackagedGoalInvocationFactory(t *testing.T) string {
	t.Helper()
	return scaffoldPackagedGoalInvocationFactoryWithConfig(t, packagedGoalInvocationFactoryConfig())
}

func scaffoldPackagedGoalInvocationFactoryWithUnresolvedPrimaryResult(t *testing.T) string {
	t.Helper()

	cfg := packagedGoalInvocationFactoryConfig()
	cfg["invocationReturn"] = map[string]any{
		"policy":        "EXPLICIT",
		"workTypeName":  "summary",
		"terminalState": "complete",
	}
	cfg["workTypes"] = append(cfg["workTypes"].([]map[string]any), map[string]any{
		"name": "summary",
		"states": []map[string]string{
			{"name": "init", "type": "INITIAL"},
			{"name": "complete", "type": "TERMINAL"},
			{"name": "failed", "type": "FAILED"},
		},
	})
	return scaffoldPackagedGoalInvocationFactoryWithConfig(t, cfg)
}

func scaffoldPackagedGoalInvocationFactoryWithConfig(t *testing.T, cfg map[string]any) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, cfg)
	support.WriteAgentConfig(
		t,
		dir,
		"goal-executor",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"),
	)
	return dir
}

func packagedGoalInvocationFactoryConfig() map[string]any {
	return map[string]any{
		"name": "@you/goal",
		"invocationReturn": map[string]any{
			"policy":        "EXPLICIT",
			"workTypeName":  "goal",
			"terminalState": "complete",
		},
		"workTypes": []map[string]any{
			{
				"name":             "goal",
				"handlingBehavior": []string{"DEFAULT"},
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{{"name": "goal-executor"}},
		"workstations": []map[string]any{
			{
				"name":          "execute-goal",
				"worker":        "goal-executor",
				"outcomeFormat": "decision-envelope",
				"inputs":        []map[string]string{{"workType": "goal", "state": "init"}},
				"classificationRoutes": []map[string]any{
					{
						"label":   "accepted",
						"outputs": []map[string]string{{"workType": "goal", "state": "complete"}},
					},
				},
				"onFailure": []map[string]string{{"workType": "goal", "state": "failed"}},
			},
		},
	}
}

func errorCodeString(code *factoryapi.InvocationResponseErrorCode) string {
	if code == nil {
		return "<nil>"
	}
	return string(*code)
}

func invokePackagedGoalViaAPI(
	t *testing.T,
	server *packagedGoalParityAPIServer,
	factoryDir string,
	goalText string,
) (factoryapi.InvocationResponse, packagedGoalInvocationObservation) {
	t.Helper()
	return postPackagedGoalInvocation(t, server, factoryDir, textInvocationRequestBody(goalText))
}

func postPackagedGoalInvocation(
	t *testing.T,
	server *packagedGoalParityAPIServer,
	factoryDir string,
	body []byte,
) (factoryapi.InvocationResponse, packagedGoalInvocationObservation) {
	t.Helper()

	sessionID := openPackagedGoalParitySession(t, server, factoryDir)
	sessionClosed := false
	closeSession := func() {
		if sessionClosed {
			return
		}
		support.CloseFactorySessionAt(t, server.URL(), sessionID)
		assertPackagedGoalFactorySessionAbsent(t, server.URL(), sessionID)

		sessionClosed = true
	}
	t.Cleanup(closeSession)
	response, err := http.Post(
		strings.TrimSuffix(server.URL(), "/")+
			"/factory-sessions/"+sessionID+"/invocations",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST /factory-sessions/%s/invocations: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"POST /factory-sessions/%s/invocations status = %d, want 200: %s",
			sessionID,
			response.StatusCode,
			string(payload),
		)
	}

	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation response: %v", err)
	}
	observation := observePackagedGoalInvocation(t, server.URL(), sessionID)
	closeSession()
	return decoded, observation
}

func postPackagedGoalInvocationExpectError(
	t *testing.T,
	server *packagedGoalParityAPIServer,
	factoryDir string,
	goalText string,
) (factoryapi.ErrorResponse, packagedGoalInvocationObservation) {
	t.Helper()

	body := textInvocationRequestBody(goalText)
	sessionID := openPackagedGoalParitySession(t, server, factoryDir)
	sessionClosed := false
	closeSession := func() {
		if sessionClosed {
			return
		}
		support.CloseFactorySessionAt(t, server.URL(), sessionID)
		assertPackagedGoalFactorySessionAbsent(t, server.URL(), sessionID)

		sessionClosed = true
	}
	t.Cleanup(closeSession)
	response, err := http.Post(
		strings.TrimSuffix(server.URL(), "/")+
			"/factory-sessions/"+sessionID+"/invocations",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST /factory-sessions/%s/invocations: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"POST /factory-sessions/%s/invocations status = %d, want 400: %s",
			sessionID,
			response.StatusCode,
			string(payload),
		)
	}

	var decoded factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation error response: %v", err)
	}
	observation := observePackagedGoalInvocation(t, server.URL(), sessionID)
	closeSession()
	return decoded, observation
}

type packagedGoalParityAPIServer struct {
	command *crossHostedCommand
	url     string
	homeDir string
	close   sync.Once
}

func (server *packagedGoalParityAPIServer) URL() string {
	if server == nil {
		return ""
	}
	return server.url
}

func (server *packagedGoalParityAPIServer) closeServer(t testing.TB) {
	t.Helper()
	if server == nil {
		return
	}
	server.close.Do(func() {
		if server.command != nil {
			server.command.stop(t)
		}

		if server.url != "" {
			assertCrossListenerClosed(t, server.url)
		}
		removeCrossOwnedPath(t, "shared parity API home", server.homeDir)
	})
}

func assertPackagedGoalParityAPIServerHealthy(
	t testing.TB,
	server *packagedGoalParityAPIServer,
) {
	t.Helper()
	if server == nil || strings.TrimSpace(server.URL()) == "" {
		t.Fatal("shared parity API server is unavailable")
	}
	status := support.GetJSON[factoryapi.StatusResponse](
		t,
		strings.TrimSuffix(server.URL(), "/")+"/status",
	)
	if strings.TrimSpace(status.RuntimeStatus) == "" {
		t.Fatalf("shared parity API runtime status = %q, want non-empty", status.RuntimeStatus)
	}
	if listed := support.ListDefaultSessionWork(t, server.URL()); len(listed.Results) != 0 {
		t.Fatalf("shared parity API default session retained Work = %#v, want empty", listed.Results)
	}
	live := support.GetJSON[factoryapi.ListFactorySessionsResponse](
		t,
		strings.TrimSuffix(server.URL(), "/")+"/factory-sessions?scope=live",
	)
	defaultSessions := 0
	for _, session := range live.Sessions {
		if session.IsDefault {
			defaultSessions++
			continue
		}
		if strings.TrimSpace(session.Id) == "" {
			t.Fatalf("shared parity API live session has no identity: %#v", session)
		}
		assertPackagedGoalFactorySessionAbsent(t, server.URL(), session.Id)
	}
	if defaultSessions != 1 {
		t.Fatalf("shared parity API live sessions = %#v, want one default session and no reachable explicit sessions", live.Sessions)
	}
}

func openPackagedGoalParitySession(
	t *testing.T,
	server *packagedGoalParityAPIServer,
	factoryDir string,
) string {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, server.URL(), factoryDir)
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" {
		t.Fatalf("opened packaged goal session = %#v, want explicit session identity", opened)
	}
	if opened.Session.Id == factorysessions.DefaultSessionID {
		t.Fatalf("opened packaged goal session = %q, want non-default explicit session", opened.Session.Id)
	}

	return opened.Session.Id
}

func startPackagedGoalParityAPIServer(
	t *testing.T,
	factoryDir string,
) *packagedGoalParityAPIServer {
	t.Helper()

	fixture := factorypackagedcrossSharedCrossProcess(t)
	server := support.NewProcessAPIServer()
	fixture.router.set(server)
	homeDir := t.TempDir()
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run",
		"--continuously",
		"--with-server",
		"--quiet",
		"--dir", factoryDir,
		"--no-record",
		"--provider", "CODEX",
		"--model", packagedGoalProviderModel,
	})
	inputs.Input.Env = isolatedHomeEnvironment(homeDir)
	inputs.Input.WorkingDirectory = factoryDir

	command := startCrossHostedCommand(t, fixture.process, inputs)
	baseURL, err := server.WaitForBaseURL(crossPackagedServerReadyTimeout)
	if err != nil {
		t.Fatalf("wait for packaged goal parity API server: %v", err)
	}
	support.WaitForStatus(t, baseURL, crossPackagedServerReadyTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	parityServer := &packagedGoalParityAPIServer{command: command, url: baseURL, homeDir: homeDir}
	t.Cleanup(func() { parityServer.closeServer(t) })
	return parityServer
}

func textInvocationRequestBody(goalText string) []byte {
	sourceKind := factoryapi.InvocationInputSourceKindText
	body, err := json.Marshal(factoryapi.InvocationRequest{
		SourceKind: &sourceKind,
		Content:    invocationTextContentPtr(goalText),
	})
	if err != nil {
		panic(fmt.Sprintf("marshal invocation request: %v", err))
	}
	return body
}

func invocationTextContentPtr(goalText string) *factoryapi.WorkContent {
	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: goalText,
	}); err != nil {
		panic(fmt.Sprintf("build invocation text content: %v", err))
	}
	content := factoryapi.WorkContent{part}
	return &content
}

func runPackagedGoalInvocationCLIJSON(
	t *testing.T,
	factoryDir string,
	factoryPath string,
	env []string,
	stdin io.Reader,
	args ...string,
) (factoryapi.InvocationResponse, string, string, error) {
	t.Helper()
	return runPackagedGoalInvocationCLIWithMode(
		t,
		factoryDir,
		[]string{"--factory", factoryPath},
		env,
		stdin,
		true,
		args...,
	)
}

func runPackagedGoalNamedInvocationCLIJSON(
	t *testing.T,
	homeDir string,
	goalText string,
) (factoryapi.InvocationResponse, string, string, error) {
	t.Helper()
	return runPackagedGoalInvocationCLIWithMode(
		t,
		t.TempDir(),
		[]string{"--named", factorydefinitions.PackagedGoalFactoryName},
		isolatedHomeEnvironment(homeDir),
		nil,
		true,
		goalText,
	)
}

func runPackagedGoalInvocationCLI(
	t *testing.T,
	factoryDir string,
	factoryPath string,
	env []string,
	stdin io.Reader,
	args ...string,
) (factoryapi.InvocationResponse, string, string, error) {
	t.Helper()
	return runPackagedGoalInvocationCLIWithMode(
		t,
		factoryDir,
		[]string{"--factory", factoryPath},
		env,
		stdin,
		false,
		args...,
	)
}

func runPackagedGoalInvocationCLIWithMode(
	t *testing.T,
	workingDirectory string,
	sourceArgs []string,
	env []string,
	stdin io.Reader,
	jsonMode bool,
	args ...string,
) (factoryapi.InvocationResponse, string, string, error) {
	t.Helper()

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	cmdArgs := []string{"you"}
	if jsonMode {
		cmdArgs = append(cmdArgs, "--json")
	}
	cmdArgs = append(cmdArgs, "run")
	cmdArgs = append(cmdArgs, sourceArgs...)
	cmdArgs = append(
		cmdArgs,
		"--no-record",
		"--session", uuid.NewString(),
		"--server", baseURL,
		"--provider", "CODEX",
		"--model", packagedGoalProviderModel,
	)
	if jsonMode {
		cmdArgs = append(cmdArgs, "--output", "primary")
	}
	cmdArgs = append(cmdArgs, args...)

	inputs := support.FakeInputs(ctx, cmdArgs)
	inputs.Input.WorkingDirectory = workingDirectory
	inputs.Input.Env = env
	if stdin != nil {
		inputs.Input.Stdin = stdin
		stdinIsTTY := false
		inputs.Input.StdinIsTTY = &stdinIsTTY
	}

	fixture := factorypackagedcrossSharedCrossProcess(t)
	runErr := fixture.process.Execute(inputs.Input)

	var response factoryapi.InvocationResponse
	if jsonMode && strings.TrimSpace(inputs.Stdout()) != "" {
		response = support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	}
	removeCrossOwnedPath(t, "parity CLI working directory", workingDirectory)
	if home := crossHomeFromEnvironment(env); home != fixture.homeDir {
		removeCrossOwnedPath(t, "parity CLI home", home)
	}
	return response, inputs.Stdout(), inputs.Stderr(), runErr
}

func assertPackagedGoalCLIAPIPrimaryOutcomeParity(
	t *testing.T,
	apiResponse factoryapi.InvocationResponse,
	cliResponse factoryapi.InvocationResponse,
	wantPrimaryResult string,
) {
	t.Helper()

	if apiResponse.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("API status = %q, want COMPLETED", apiResponse.Status)
	}
	if cliResponse.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("CLI status = %q, want COMPLETED", cliResponse.Status)
	}
	if strings.TrimSpace(apiResponse.RequestId) == "" || strings.TrimSpace(apiResponse.TraceId) == "" {
		t.Fatalf(
			"API submission identity = request %q trace %q, want non-empty invocation scope",
			apiResponse.RequestId,
			apiResponse.TraceId,
		)
	}
	if strings.TrimSpace(cliResponse.RequestId) == "" || strings.TrimSpace(cliResponse.TraceId) == "" {
		t.Fatalf(
			"CLI submission identity = request %q trace %q, want non-empty invocation scope",
			cliResponse.RequestId,
			cliResponse.TraceId,
		)
	}

	apiText := invocationPrimaryResultText(t, apiResponse)
	cliText := invocationPrimaryResultText(t, cliResponse)
	if apiText != wantPrimaryResult {
		t.Fatalf("API primaryResult = %q, want %q", apiText, wantPrimaryResult)
	}
	if cliText != wantPrimaryResult {
		t.Fatalf("CLI primaryResult = %q, want %q", cliText, wantPrimaryResult)
	}
	if apiText != cliText {
		t.Fatalf("primaryResult mismatch: API = %q, CLI = %q", apiText, cliText)
	}
	if apiResponse.PrimaryResult == nil || cliResponse.PrimaryResult == nil {
		t.Fatal("expected both API and CLI success responses to include primaryResult")
	}
}
