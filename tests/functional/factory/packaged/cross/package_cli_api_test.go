package cross

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
	"runtime"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const wantPackagedGoalPrimaryResult = "mock worker accepted"

// TestPackagedFactoryInvokedByCLICanBeInspectedByAPI proves a packaged @you/goal
// invocation started through the public you run CLI with a run-scoped server
// exposes compatible session, status, work, and invocation identity facts through
// the public Factory Session API while the invocation completes.
func TestPackagedFactoryInvokedByCLICanBeInspectedByAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("slow packaged factory CLI/API cross-surface inspectability")
	}

	homeDir := t.TempDir()
	factoryDir := support.InstallPackagedFactory(
		t,
		homeDir,
		factorydefinitions.PackagedGoalFactoryName,
	)
	factoryPath := filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile)
	mockWorkersPath := writePackagedGoalMockWorkersConfig(t)
	goalText := fmt.Sprintf(
		"functional-packaged-cross-cli-api-inspect-%d",
		time.Now().UnixNano(),
	)

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	requestedURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	server := support.NewProcessAPIServer()
	process := support.BuildProcess(t, serviceedges.Edges{APIServerStarter: server.Start})

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	inputs := support.FakeInputs(ctx, []string{
		"you", "--json", "run",
		"--factory", factoryPath,
		"--with-server",
		"--server", requestedURL,
		"--with-mock-workers",
		"--no-record",
		mockWorkersPath,
		goalText,
	})
	inputs.Input.WorkingDirectory = factoryDir
	inputs.Input.Env = isolatedHomeEnvironment(homeDir)

	execDone := make(chan error, 1)
	go func() {
		execDone <- process.Execute(inputs.Input)
	}()

	baseURL := server.WaitForURL(t)
	eventCollector := startPackagedGoalFactoryEventCollector(ctx, t, baseURL)

	inspection, execErr := pollPackagedGoalAPIInspectionUntilCLICompletes(ctx, t, baseURL, execDone)
	eventCollector.stop()
	events := eventCollector.snapshot()
	if retained, retainErr := tryGetRetainedFactoryEvents(baseURL); retainErr == nil {
		events = mergePackagedGoalFactoryEvents(events, retained)
	}
	if refreshed, refreshErr := fetchPackagedGoalAPIInspectionSnapshot(baseURL); refreshErr == nil {
		inspection = preferStrongerPackagedGoalAPIInspection(inspection, refreshed)
	}

	if execErr != nil {
		t.Fatalf(
			"CLI packaged-factory invocation: %v\nstdout:\n%s\nstderr:\n%s",
			execErr,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	cliResponse := support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	if traceListed, traceErr := tryListDefaultSessionWorkByTrace(baseURL, cliResponse.TraceId); traceErr == nil {
		inspection = preferStrongerPackagedGoalAPIInspection(inspection, packagedGoalAPIInspection{
			session: inspection.session,
			status:  inspection.status,
			listed:  traceListed,
			live:    inspection.live,
		})
	}

	if !inspection.live {
		t.Fatalf("API inspection never observed a live session/status while CLI invocation ran")
	}

	assertPackagedGoalCLIInvocationInspectableByAPI(
		t,
		cliResponse,
		inspection,
		events,
		wantPackagedGoalPrimaryResult,
	)
}

// TestPackagedFactoryCLIAndAPIPrimaryOutcomeShapesAgree proves packaged @you/goal
// CLI and API invocations agree on compatible public primary-outcome facts:
// terminal status, primary-result text on success, and stable public error codes
// on representative empty-input, source-conflict, and unresolved-primary-result
// failures across positional, stdin, and named-factory success paths.
func TestPackagedFactoryCLIAndAPIPrimaryOutcomeShapesAgree(t *testing.T) {
	if testing.Short() {
		t.Skip("slow packaged factory CLI/API primary-outcome parity")
	}

	t.Run("positional success", func(t *testing.T) {
		dir := scaffoldPackagedGoalInvocationFactory(t)
		factoryPath := filepath.Join(dir, factorydefinitions.FactoryConfigFile)
		mockWorkersPath := writeDefaultMockWorkersConfig(t)
		goalText := fmt.Sprintf(
			"functional-packaged-cross-cli-api-parity-positional-%d",
			time.Now().UnixNano(),
		)

		apiResponse := invokePackagedGoalViaAPI(t, dir, mockWorkersPath, goalText)
		cliResponse, _, stderr, err := runPackagedGoalInvocationCLIJSON(
			t,
			dir,
			factoryPath,
			mockWorkersPath,
			isolatedHomeEnvironment(t.TempDir()),
			nil,
			goalText,
		)
		if err != nil {
			t.Fatalf("CLI positional invocation: %v\nstderr:\n%s", err, stderr)
		}
		assertPackagedGoalCLIAPIPrimaryOutcomeParity(
			t,
			apiResponse,
			cliResponse,
			wantPackagedGoalPrimaryResult,
		)
	})

	t.Run("stdin success", func(t *testing.T) {
		dir := scaffoldPackagedGoalInvocationFactory(t)
		factoryPath := filepath.Join(dir, factorydefinitions.FactoryConfigFile)
		mockWorkersPath := writeDefaultMockWorkersConfig(t)
		goalText := fmt.Sprintf(
			"functional-packaged-cross-cli-api-parity-stdin-%d",
			time.Now().UnixNano(),
		)

		apiResponse := invokePackagedGoalViaAPI(t, dir, mockWorkersPath, goalText)
		cliResponse, _, stderr, err := runPackagedGoalInvocationCLIJSON(
			t,
			dir,
			factoryPath,
			mockWorkersPath,
			isolatedHomeEnvironment(t.TempDir()),
			strings.NewReader(goalText),
		)
		if err != nil {
			t.Fatalf("CLI stdin invocation: %v\nstderr:\n%s", err, stderr)
		}
		assertPackagedGoalCLIAPIPrimaryOutcomeParity(
			t,
			apiResponse,
			cliResponse,
			wantPackagedGoalPrimaryResult,
		)
	})

	t.Run("named factory success", func(t *testing.T) {
		homeDir := t.TempDir()
		factoryDir := support.InstallPackagedFactory(
			t,
			homeDir,
			factorydefinitions.PackagedGoalFactoryName,
		)
		mockWorkersPath := writePackagedGoalMockWorkersConfig(t)
		goalText := fmt.Sprintf(
			"functional-packaged-cross-cli-api-parity-named-%d",
			time.Now().UnixNano(),
		)

		apiResponse := invokePackagedGoalViaAPI(t, factoryDir, mockWorkersPath, goalText)
		cliResponse, _, stderr, err := runPackagedGoalNamedInvocationCLIJSON(
			t,
			homeDir,
			mockWorkersPath,
			goalText,
		)
		if err != nil {
			t.Fatalf("CLI named invocation: %v\nstderr:\n%s", err, stderr)
		}
		assertPackagedGoalCLIAPIPrimaryOutcomeParity(
			t,
			apiResponse,
			cliResponse,
			wantPackagedGoalPrimaryResult,
		)
	})

	t.Run("empty input rejected", func(t *testing.T) {
		dir := scaffoldPackagedGoalInvocationFactory(t)
		factoryPath := filepath.Join(dir, factorydefinitions.FactoryConfigFile)
		mockWorkersPath := writeDefaultMockWorkersConfig(t)

		apiErr := postPackagedGoalInvocationExpectError(t, dir, mockWorkersPath, "   ")
		if string(apiErr.Code) != "INVOCATION_INPUT_EMPTY" {
			t.Fatalf("API error code = %q, want INVOCATION_INPUT_EMPTY", apiErr.Code)
		}

		_, _, stderr, err := runPackagedGoalInvocationCLIJSON(
			t,
			dir,
			factoryPath,
			mockWorkersPath,
			isolatedHomeEnvironment(t.TempDir()),
			nil,
			"   ",
		)
		if err == nil {
			t.Fatal("expected CLI empty invocation input to fail")
		}
		if !strings.Contains(err.Error(), "INVOCATION_INPUT_EMPTY") &&
			!strings.Contains(stderr, "INVOCATION_INPUT_EMPTY") {
			t.Fatalf("CLI failure = %v\nstderr = %q, want INVOCATION_INPUT_EMPTY", err, stderr)
		}
	})

	t.Run("source conflict rejected before invocation", func(t *testing.T) {
		dir := scaffoldPackagedGoalInvocationFactory(t)
		factoryPath := filepath.Join(dir, factorydefinitions.FactoryConfigFile)
		mockWorkersPath := writeDefaultMockWorkersConfig(t)
		conflictMessage := "invocation input sources conflict: positional_text, stdin_text"

		_, stdout, stderr, err := runPackagedGoalInvocationCLI(
			t,
			dir,
			factoryPath,
			mockWorkersPath,
			isolatedHomeEnvironment(t.TempDir()),
			strings.NewReader("from stdin"),
			"from positional",
		)
		if err == nil {
			t.Fatal("expected conflicting positional and stdin invocation inputs to fail")
		}
		if stdout != "" {
			t.Fatalf("stdout = %q, want empty on conflict failure", stdout)
		}
		if !strings.Contains(stderr, "INVOCATION_INPUT_SOURCE_CONFLICT") {
			t.Fatalf("stderr = %q, want stable conflict code", stderr)
		}
		if !strings.Contains(stderr, conflictMessage) {
			t.Fatalf("stderr = %q, want conflict detail %q", stderr, conflictMessage)
		}
	})

	t.Run("unresolved primary result reports stable failure", func(t *testing.T) {
		dir := scaffoldPackagedGoalInvocationFactoryWithUnresolvedPrimaryResult(t)
		factoryPath := filepath.Join(dir, factorydefinitions.FactoryConfigFile)
		goalText := fmt.Sprintf(
			"functional-packaged-cross-cli-api-parity-unresolved-%d",
			time.Now().UnixNano(),
		)
		mockWorkersPath := writeDefaultMockWorkersConfig(t)

		apiResponse := invokePackagedGoalViaAPI(t, dir, mockWorkersPath, goalText)
		if apiResponse.Status != factoryapi.InvocationTerminalStatusFailed {
			t.Fatalf("API status = %q, want FAILED", apiResponse.Status)
		}
		if apiResponse.ErrorCode == nil ||
			*apiResponse.ErrorCode != factoryapi.INVOCATIONPRIMARYRESULTUNRESOLVED {
			t.Fatalf(
				"API errorCode = %v, want INVOCATION_PRIMARY_RESULT_UNRESOLVED",
				errorCodeString(apiResponse.ErrorCode),
			)
		}
		if apiResponse.PrimaryResult != nil {
			t.Fatalf("API primaryResult = %#v, want nil on unresolved output", apiResponse.PrimaryResult)
		}

		cliResponse, _, stderr, err := runPackagedGoalInvocationCLIJSON(
			t,
			dir,
			factoryPath,
			mockWorkersPath,
			isolatedHomeEnvironment(t.TempDir()),
			nil,
			goalText,
		)
		if err == nil {
			t.Fatal("expected CLI unresolved invocation primary result to fail")
		}
		if !strings.Contains(err.Error(), "INVOCATION_PRIMARY_RESULT_UNRESOLVED") &&
			!strings.Contains(stderr, "INVOCATION_PRIMARY_RESULT_UNRESOLVED") {
			t.Fatalf(
				"CLI failure = %v\nstderr = %q, want INVOCATION_PRIMARY_RESULT_UNRESOLVED",
				err,
				stderr,
			)
		}
		if cliResponse.Status != factoryapi.InvocationTerminalStatusFailed {
			t.Fatalf("CLI status = %q, want FAILED", cliResponse.Status)
		}
		if cliResponse.ErrorCode == nil ||
			*cliResponse.ErrorCode != factoryapi.INVOCATIONPRIMARYRESULTUNRESOLVED {
			t.Fatalf(
				"CLI errorCode = %v, want INVOCATION_PRIMARY_RESULT_UNRESOLVED",
				errorCodeString(cliResponse.ErrorCode),
			)
		}
		if cliResponse.PrimaryResult != nil {
			t.Fatalf("CLI primaryResult = %#v, want nil on unresolved output", cliResponse.PrimaryResult)
		}
	})
}

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
		}

		select {
		case execErr := <-execDone:
			if candidate, err := fetchPackagedGoalAPIInspectionSnapshot(baseURL); err == nil {
				snapshot = preferStrongerPackagedGoalAPIInspection(snapshot, candidate)
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
	return hasPackagedGoalCompleteWork(snapshot.listed)
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

func writePackagedGoalMockWorkersConfig(t *testing.T) string {
	t.Helper()

	checkerCommand, checkerArgs := mockWorkerEchoCommand("plain")
	reviewerCommand, reviewerArgs := mockWorkerEchoCommand("accepted")
	cfg := workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{
			{
				WorkerName:      "goal-planner",
				WorkstationName: "plan-goal",
				RunType:         workers.MockWorkerRunTypeAccept,
			},
			{
				WorkerName:      "goal-executor",
				WorkstationName: "execute-goal",
				RunType:         workers.MockWorkerRunTypeAccept,
			},
			{
				WorkerName:      "goal-checker",
				WorkstationName: "check-goal",
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: checkerCommand,
					Args:    checkerArgs,
				},
			},
			{
				WorkerName:      "goal-reviewer",
				WorkstationName: "review-goal",
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: reviewerCommand,
					Args:    reviewerArgs,
				},
			},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal packaged goal mock-workers config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "mock-workers-packaged-goal.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write packaged goal mock-workers config: %v", err)
	}
	return path
}

func mockWorkerEchoCommand(output string) (string, []string) {
	if runtime.GOOS == "windows" {
		literal := strings.ReplaceAll(output, "'", "''")
		return "powershell.exe", []string{
			"-NoProfile",
			"-NonInteractive",
			"-Command",
			"[Console]::Out.Write('" + literal + "')",
		}
	}
	return "/bin/echo", []string{output}
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

func isolatedHomeEnvironment(homeDir string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir)
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
				"name":      "execute-goal",
				"worker":    "goal-executor",
				"inputs":    []map[string]string{{"workType": "goal", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "goal", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "goal", "state": "failed"}},
			},
		},
	}
}

func writeDefaultMockWorkersConfig(t *testing.T) string {
	t.Helper()

	return support.WriteMockWorkersConfig(t, workers.NewEmptyMockWorkersConfig())
}

func errorCodeString(code *factoryapi.InvocationResponseErrorCode) string {
	if code == nil {
		return "<nil>"
	}
	return string(*code)
}

func invokePackagedGoalViaAPI(
	t *testing.T,
	factoryDir string,
	mockWorkersPath string,
	goalText string,
) factoryapi.InvocationResponse {
	t.Helper()
	return postPackagedGoalInvocation(t, factoryDir, mockWorkersPath, textInvocationRequestBody(goalText))
}

func postPackagedGoalInvocation(
	t *testing.T,
	factoryDir string,
	mockWorkersPath string,
	body []byte,
) factoryapi.InvocationResponse {
	t.Helper()

	server := startPackagedGoalParityAPIServer(t, factoryDir, mockWorkersPath)
	response, err := http.Post(
		strings.TrimSuffix(server.URL(), "/")+
			"/factory-sessions/"+factorysessions.DefaultSessionID+"/invocations",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST /factory-sessions/~default/invocations: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"POST /factory-sessions/~default/invocations status = %d, want 200: %s",
			response.StatusCode,
			string(payload),
		)
	}

	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation response: %v", err)
	}
	return decoded
}

func postPackagedGoalInvocationExpectError(
	t *testing.T,
	factoryDir string,
	mockWorkersPath string,
	goalText string,
) factoryapi.ErrorResponse {
	t.Helper()

	body := textInvocationRequestBody(goalText)
	server := startPackagedGoalParityAPIServer(t, factoryDir, mockWorkersPath)
	response, err := http.Post(
		strings.TrimSuffix(server.URL(), "/")+
			"/factory-sessions/"+factorysessions.DefaultSessionID+"/invocations",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST /factory-sessions/~default/invocations: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"POST /factory-sessions/~default/invocations status = %d, want 400: %s",
			response.StatusCode,
			string(payload),
		)
	}

	var decoded factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation error response: %v", err)
	}
	return decoded
}

func startPackagedGoalParityAPIServer(
	t *testing.T,
	factoryDir string,
	mockWorkersPath string,
) *support.FunctionalAPIServer {
	t.Helper()

	payload, err := os.ReadFile(mockWorkersPath)
	if err != nil {
		t.Fatalf("read customer mock-workers config: %v", err)
	}
	var mockWorkersConfig workers.MockWorkersConfig
	if err := json.Unmarshal(payload, &mockWorkersConfig); err != nil {
		t.Fatalf("decode customer mock-workers config: %v", err)
	}

	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		MockWorkersConfig:         &mockWorkersConfig,
	})
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
	mockWorkersPath string,
	env []string,
	stdin io.Reader,
	args ...string,
) (factoryapi.InvocationResponse, string, string, error) {
	t.Helper()
	return runPackagedGoalInvocationCLIWithMode(
		t,
		factoryDir,
		[]string{"--factory", factoryPath},
		mockWorkersPath,
		env,
		stdin,
		true,
		args...,
	)
}

func runPackagedGoalNamedInvocationCLIJSON(
	t *testing.T,
	homeDir string,
	mockWorkersPath string,
	goalText string,
) (factoryapi.InvocationResponse, string, string, error) {
	t.Helper()
	return runPackagedGoalInvocationCLIWithMode(
		t,
		t.TempDir(),
		[]string{"--named", factorydefinitions.PackagedGoalFactoryName},
		mockWorkersPath,
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
	mockWorkersPath string,
	env []string,
	stdin io.Reader,
	args ...string,
) (factoryapi.InvocationResponse, string, string, error) {
	t.Helper()
	return runPackagedGoalInvocationCLIWithMode(
		t,
		factoryDir,
		[]string{"--factory", factoryPath},
		mockWorkersPath,
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
	mockWorkersPath string,
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
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		mockWorkersPath,
	)
	cmdArgs = append(cmdArgs, args...)

	inputs := support.FakeInputs(ctx, cmdArgs)
	inputs.Input.WorkingDirectory = workingDirectory
	inputs.Input.Env = env
	if stdin != nil {
		inputs.Input.Stdin = stdin
		stdinIsTTY := false
		inputs.Input.StdinIsTTY = &stdinIsTTY
	}

	runErr := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input)

	var response factoryapi.InvocationResponse
	if jsonMode && strings.TrimSpace(inputs.Stdout()) != "" {
		response = support.DecodeInvocationResponseJSON(t, inputs.Stdout())
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
