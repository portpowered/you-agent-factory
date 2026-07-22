package factorysession_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession"
	mcpserver "github.com/portpowered/infinite-you/pkg/transports/mcp/server"
)

func TestProjectResultPolicyInventory_BuildsDocumentShape(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectResultPolicyInventory()
	if err != nil {
		t.Fatalf("ProjectResultPolicyInventory() error = %v", err)
	}
	if inventory.FormatVersion != mcpfactorysession.ToolInventoryFormatVersion {
		t.Fatalf("formatVersion = %q, want %q", inventory.FormatVersion, mcpfactorysession.ToolInventoryFormatVersion)
	}
	if inventory.ProtocolVersion != mcpfactorysession.ToolInventoryProtocolVersion {
		t.Fatalf("protocolVersion = %q, want %q", inventory.ProtocolVersion, mcpfactorysession.ToolInventoryProtocolVersion)
	}
	if len(inventory.Fixtures) == 0 {
		t.Fatal("expected at least one representative success fixture")
	}
}

func TestProjectResultPolicyInventory_SuccessTransportPolicy(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectResultPolicyInventory()
	if err != nil {
		t.Fatalf("ProjectResultPolicyInventory() error = %v", err)
	}
	policy := inventory.SuccessTransport
	if policy.ContentItemCount != 1 {
		t.Fatalf("contentItemCount = %d, want 1", policy.ContentItemCount)

	}
	if !slices.Equal(policy.ContentTypes, []string{"text"}) {
		t.Fatalf("contentTypes = %#v, want [text]", policy.ContentTypes)
	}
	if policy.TextEncoding != mcpfactorysession.SuccessTextEncodingSerializedJSON {
		t.Fatalf("textEncoding = %q, want %q", policy.TextEncoding, mcpfactorysession.SuccessTextEncodingSerializedJSON)
	}
	if policy.IsError {
		t.Fatal("isError = true, want false")
	}
	if policy.HasStructuredContent {
		t.Fatal("hasStructuredContent = true, want false")
	}
	for _, unsupported := range []string{"image", "audio", "resource"} {
		if !slices.Contains(policy.UnsupportedContentTypes, unsupported) {
			t.Fatalf("unsupportedContentTypes missing %q", unsupported)
		}
	}
	for _, unsupported := range []string{"outputSchema", "structuredContent"} {
		if !slices.Contains(policy.UnsupportedFields, unsupported) {
			t.Fatalf("unsupportedFields missing %q", unsupported)
		}
	}
}

func TestProjectResultPolicyInventory_FixturesSortedByName(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectResultPolicyInventory()
	if err != nil {
		t.Fatalf("ProjectResultPolicyInventory() error = %v", err)
	}
	names := make([]string, len(inventory.Fixtures))
	for i, fixture := range inventory.Fixtures {
		names[i] = fixture.Name
	}
	sorted := slices.Clone(names)
	slices.Sort(sorted)
	if !slices.Equal(names, sorted) {
		t.Fatalf("fixture names = %#v, want sorted %#v", names, sorted)
	}
}

func TestProjectResultPolicyInventory_DoesNotAdvertiseUnsupportedCapabilities(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectResultPolicyInventory()
	if err != nil {
		t.Fatalf("ProjectResultPolicyInventory() error = %v", err)
	}
	for _, fixture := range inventory.Fixtures {
		payload := string(fixture.CallToolResult)
		for _, forbidden := range []string{
			"\"image\"",
			"\"audio\"",
			"\"resource\"",
			"\"outputSchema\"",
			"\"structuredContent\"",
		} {
			if strings.Contains(payload, forbidden) {
				t.Fatalf("fixture %q callToolResult advertises unsupported capability %q", fixture.Name, forbidden)
			}
		}
	}
}

func TestProjectResultPolicyInventory_RepeatExtractionIsByteIdentical(t *testing.T) {
	first, err := mcpfactorysession.MarshalResultPolicyInventoryJSON(mustProjectResultPolicyInventory(t))
	if err != nil {
		t.Fatalf("first MarshalResultPolicyInventoryJSON() error = %v", err)
	}
	second, err := mcpfactorysession.MarshalResultPolicyInventoryJSON(mustProjectResultPolicyInventory(t))
	if err != nil {
		t.Fatalf("second MarshalResultPolicyInventoryJSON() error = %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("repeat extraction differs:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestVerifyProjectedResultPolicyInventory_PassesForLiveProjection(t *testing.T) {
	if err := mcpfactorysession.VerifyProjectedResultPolicyInventory(); err != nil {
		t.Fatalf("VerifyProjectedResultPolicyInventory() error = %v", err)
	}
}

func TestVerifyResultPolicyInventory_FailsWhenCallToolResultEncodingDrifts(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectResultPolicyInventory()
	if err != nil {
		t.Fatalf("ProjectResultPolicyInventory() error = %v", err)
	}
	inventory.Fixtures[0].CallToolResult = json.RawMessage(`{"content":[{"type":"text","text":"{}"}],"isError":true}`)
	err = mcpfactorysession.VerifyResultPolicyInventory(inventory)
	if err == nil {
		t.Fatal("VerifyResultPolicyInventory() error = nil, want failure")
	}
}

func TestProjectResultPolicyInventory_DomainErrorTransportPolicy(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectResultPolicyInventory()
	if err != nil {
		t.Fatalf("ProjectResultPolicyInventory() error = %v", err)
	}
	policy := inventory.DomainErrorTransport

	if policy.FailureClass != mcpfactorysession.FailureClassDomain {
		t.Fatalf("failureClass = %q, want %q", policy.FailureClass, mcpfactorysession.FailureClassDomain)
	}
	if policy.IsError {
		t.Fatal("isError = true, want false for typed ToolErrorEnvelope payloads")
	}
	if !slices.Equal(policy.StableEnvelopeFields, []string{
		"error.code",
		"error.message",
		"error.retryable",
		"error.sessionId",
		"error.details",
	}) {
		t.Fatalf("stableEnvelopeFields = %#v, want shared error stable fields", policy.StableEnvelopeFields)
	}
}

func TestProjectResultPolicyInventory_ProtocolErrorTransportPolicy(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectResultPolicyInventory()
	if err != nil {
		t.Fatalf("ProjectResultPolicyInventory() error = %v", err)
	}
	policy := inventory.ProtocolErrorTransport
	if policy.FailureClass != mcpfactorysession.FailureClassProtocol {
		t.Fatalf("failureClass = %q, want %q", policy.FailureClass, mcpfactorysession.FailureClassProtocol)
	}
	if policy.Transport != mcpfactorysession.ProtocolTransportJSONRPCError {
		t.Fatalf("transport = %q, want %q", policy.Transport, mcpfactorysession.ProtocolTransportJSONRPCError)
	}
}

func TestProjectResultPolicyInventory_DomainAndProtocolFixturesSortedByName(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectResultPolicyInventory()
	if err != nil {
		t.Fatalf("ProjectResultPolicyInventory() error = %v", err)
	}
	for _, fixtures := range [][]string{
		namesFromDomainErrorFixtures(inventory.DomainErrorFixtures),
		namesFromProtocolErrorFixtures(inventory.ProtocolErrorFixtures),
	} {
		sorted := slices.Clone(fixtures)
		slices.Sort(sorted)
		if !slices.Equal(fixtures, sorted) {
			t.Fatalf("fixture names = %#v, want sorted %#v", fixtures, sorted)
		}
	}
}

func TestDomainErrorFixture_MatchesServerToolsCallEncoding(t *testing.T) {
	client := newResultPolicyFixtureMCPClient(t)
	arguments := json.RawMessage(`{"sessionId":"dur-sess-missing-999"}`)
	raw, err := client.CallTool(context.Background(), mcpfactorysession.ToolGetSession, arguments)
	if err != nil {
		t.Fatalf("CallTool(get_session) error = %v", err)
	}

	srv := newResultPolicyTestServer(t, raw)
	result := decodeToolsCallResult(t, runResultPolicyServerHandleLine(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"you.factory_session.get","arguments":{"sessionId":"dur-sess-missing-999"}}}`,
	))

	projected, err := mcpfactorysession.MarshalSuccessCallToolResultJSON(raw)
	if err != nil {
		t.Fatalf("MarshalSuccessCallToolResultJSON() error = %v", err)
	}
	serverEncoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal server result: %v", err)
	}
	if string(projected) != string(serverEncoded) {
		t.Fatalf("projected domain-error envelope differs from server:\nprojected=%s\nserver=%s", projected, serverEncoded)
	}

	inventory, err := mcpfactorysession.ProjectResultPolicyInventory()
	if err != nil {
		t.Fatalf("ProjectResultPolicyInventory() error = %v", err)
	}
	if string(inventory.DomainErrorFixtures[0].ToolResponse) != string(raw) {
		t.Fatalf("fixture toolResponse = %s, want %s", inventory.DomainErrorFixtures[0].ToolResponse, raw)
	}
	if string(inventory.DomainErrorFixtures[0].CallToolResult) != string(serverEncoded) {
		t.Fatalf("fixture callToolResult = %s, want %s", inventory.DomainErrorFixtures[0].CallToolResult, serverEncoded)
	}
}

func TestProtocolErrorFixtures_MatchServerJSONRPCResponses(t *testing.T) {
	srv := newResultPolicyTestServer(t, nil)
	inventory, err := mcpfactorysession.ProjectResultPolicyInventory()
	if err != nil {
		t.Fatalf("ProjectResultPolicyInventory() error = %v", err)
	}
	for _, fixture := range inventory.ProtocolErrorFixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			line := runResultPolicyServerHandleLine(t, srv, fixture.RequestLine)
			if string(fixture.JSONRPCResponse) != line {
				t.Fatalf("fixture jsonRpcResponse = %s, want server %s", fixture.JSONRPCResponse, line)
			}
		})
	}
}

func TestResultPolicyBaselineFixtureIncludesDomainAndProtocolErrors(t *testing.T) {
	baselinePath := testutil.MustRepoPath(t, mcpfactorysession.ResultPolicyInventoryBaselineRelativePath)
	baseline, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("read baseline fixture: %v", err)
	}
	var inventory mcpfactorysession.ResultPolicyInventory
	if err := json.Unmarshal(baseline, &inventory); err != nil {
		t.Fatalf("unmarshal baseline fixture: %v", err)
	}
	if len(inventory.DomainErrorFixtures) == 0 {
		t.Fatal("baseline domainErrorFixtures is empty")
	}
	if len(inventory.ProtocolErrorFixtures) < 2 {
		t.Fatalf("protocolErrorFixtures = %d, want at least 2", len(inventory.ProtocolErrorFixtures))
	}
	if err := mcpfactorysession.VerifyResultPolicyInventory(inventory); err != nil {
		t.Fatalf("VerifyResultPolicyInventory(baseline) error = %v", err)
	}
}

func TestEncodeSuccessCallToolResult_MatchesServerToolsCallSuccessEncoding(t *testing.T) {
	client := newResultPolicyFixtureMCPClient(t)
	raw, err := client.CallTool(context.Background(), mcpfactorysession.ToolListSessions, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallTool(list_sessions) error = %v", err)
	}

	srv := newResultPolicyTestServer(t, raw)
	result := decodeToolsCallResult(t, runResultPolicyServerHandleLine(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"you.factory_session.list","arguments":{}}}`,
	))

	projected, err := mcpfactorysession.MarshalSuccessCallToolResultJSON(raw)
	if err != nil {
		t.Fatalf("MarshalSuccessCallToolResultJSON() error = %v", err)
	}
	serverEncoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal server result: %v", err)
	}
	if string(projected) != string(serverEncoded) {
		t.Fatalf("projected success envelope differs from server:\nprojected=%s\nserver=%s", projected, serverEncoded)
	}
}

func TestResultPolicyBaselineFixtureMatchesProjectedInventory(t *testing.T) {
	baselinePath := testutil.MustRepoPath(t, mcpfactorysession.ResultPolicyInventoryBaselineRelativePath)
	baseline, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("read baseline fixture: %v", err)
	}
	projected, err := mcpfactorysession.MarshalResultPolicyInventoryJSON(mustProjectResultPolicyInventory(t))
	if err != nil {
		t.Fatalf("MarshalResultPolicyInventoryJSON() error = %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(baseline), projected) {
		t.Fatalf("baseline fixture differs from projected inventory:\nbaseline=%s\nprojected=%s", baseline, projected)
	}
}

func TestResultPolicyBaselineFixtureMatchesLiveListSessionsToolsCall(t *testing.T) {
	baselinePath := testutil.MustRepoPath(t, mcpfactorysession.ResultPolicyInventoryBaselineRelativePath)
	baseline, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("read baseline fixture: %v", err)
	}
	var inventory mcpfactorysession.ResultPolicyInventory
	if err := json.Unmarshal(baseline, &inventory); err != nil {
		t.Fatalf("unmarshal baseline fixture: %v", err)
	}
	if err := mcpfactorysession.VerifyResultPolicyInventory(inventory); err != nil {
		t.Fatalf("VerifyResultPolicyInventory(baseline) error = %v", err)
	}

	client := newResultPolicyFixtureMCPClient(t)
	raw, err := client.CallTool(context.Background(), mcpfactorysession.ToolListSessions, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallTool(list_sessions) error = %v", err)
	}
	if string(inventory.Fixtures[0].ToolResponse) != string(raw) {
		t.Fatalf("baseline toolResponse = %s, want %s", inventory.Fixtures[0].ToolResponse, raw)
	}

	srv := newResultPolicyTestServer(t, raw)
	result := decodeToolsCallResult(t, runResultPolicyServerHandleLine(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"you.factory_session.list","arguments":{}}}`,
	))
	serverEncoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal server result: %v", err)
	}
	if string(inventory.Fixtures[0].CallToolResult) != string(serverEncoded) {
		t.Fatalf("baseline callToolResult = %s, want %s", inventory.Fixtures[0].CallToolResult, serverEncoded)
	}
}

func newResultPolicyFixtureMCPClient(t *testing.T) *testClient {
	t.Helper()
	return newTestClientWithService(resultPolicyExecutionScript{}, canonicalMCPRequestPreparation)
}

type resultPolicyExecutionScript struct {
	factorysessions.ExecutionService
}

func (resultPolicyExecutionScript) ListSessions(
	context.Context,
	factorysessions.ListSessionsRequest,
) (factorysessions.ListSessionsResult, error) {
	return factorysessions.ListSessionsResult{
		Scope:           factorysessions.SessionListScopeLive,
		LiveSessions:    []factorysessions.LiveSessionSummary{},
		DurableSessions: []factorysessions.DurableSessionListSummary{},
	}, nil
}

func (resultPolicyExecutionScript) GetSession(
	context.Context,
	string,
) (factorysessions.SessionReadResult, error) {
	return factorysessions.SessionReadResult{}, factorysessions.ErrDurableSessionNotFound
}

func mustProjectResultPolicyInventory(t *testing.T) mcpfactorysession.ResultPolicyInventory {
	t.Helper()
	inventory, err := mcpfactorysession.ProjectResultPolicyInventory()
	if err != nil {
		t.Fatalf("ProjectResultPolicyInventory() error = %v", err)
	}
	return inventory
}

func newResultPolicyTestServer(t *testing.T, result json.RawMessage) *mcpserver.Server {
	t.Helper()
	srv, err := mcpserver.New(mcpserver.Options{
		ToolOperation: func(context.Context, string, json.RawMessage) (json.RawMessage, error) {
			return append(json.RawMessage(nil), result...), nil
		},
	})
	if err != nil {
		t.Fatalf("server.New() error = %v", err)
	}
	return srv
}

func runResultPolicyServerHandleLine(t *testing.T, srv *mcpserver.Server, line string) string {
	t.Helper()
	inputReader, inputWriter := io.Pipe()
	outputReader, outputWriter := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- srv.ServeStdio(ctx, inputReader, outputWriter) }()

	scanner := bufio.NewScanner(outputReader)
	_, _ = io.WriteString(inputWriter,
		`{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`+"\n",
	)
	if !scanner.Scan() {
		t.Fatalf("read initialize response: %v", scanner.Err())
	}
	_, _ = io.WriteString(inputWriter, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n"+line+"\n")
	if !scanner.Scan() {
		t.Fatalf("read response: %v", scanner.Err())
	}
	response := scanner.Text()
	_ = inputWriter.Close()
	if err := <-done; err != nil {
		t.Fatalf("ServeStdio(%s) error = %v", line, err)
	}
	return response
}

func decodeToolsCallResult(t *testing.T, line string) map[string]any {
	t.Helper()
	var response struct {
		Result map[string]any `json:"result"`
		Error  any            `json:"error"`
	}
	if err := json.Unmarshal([]byte(line), &response); err != nil {
		t.Fatalf("unmarshal tools/call response: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("tools/call response error = %#v, want success result", response.Error)
	}
	if response.Result == nil {
		t.Fatal("tools/call response result is nil")
	}
	return response.Result
}

func namesFromDomainErrorFixtures(fixtures []mcpfactorysession.DomainErrorFixture) []string {
	names := make([]string, len(fixtures))
	for i, fixture := range fixtures {
		names[i] = fixture.Name
	}
	return names
}

func namesFromProtocolErrorFixtures(fixtures []mcpfactorysession.ProtocolErrorFixture) []string {
	names := make([]string, len(fixtures))
	for i, fixture := range fixtures {
		names[i] = fixture.Name
	}
	return names
}
