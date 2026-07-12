package factorysession_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
	mcpserver "github.com/portpowered/infinite-you/pkg/mcp/server"
	"github.com/portpowered/infinite-you/pkg/testutil"
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

func TestEncodeSuccessCallToolResult_MatchesServerToolsCallSuccessEncoding(t *testing.T) {
	client := newResultPolicyFixtureMCPClient(t)
	raw, err := client.CallTool(mcpfactorysession.ToolListSessions, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallTool(list_sessions) error = %v", err)
	}

	srv := newResultPolicyTestServer(t, client)
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
	if string(baseline) != string(projected) {
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
	raw, err := client.CallTool(mcpfactorysession.ToolListSessions, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallTool(list_sessions) error = %v", err)
	}
	if string(inventory.Fixtures[0].ToolResponse) != string(raw) {
		t.Fatalf("baseline toolResponse = %s, want %s", inventory.Fixtures[0].ToolResponse, raw)
	}

	srv := newResultPolicyTestServer(t, client)
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

func newResultPolicyFixtureMCPClient(t *testing.T) *mcpfactorysession.Client {
	t.Helper()
	catalogPath := testutil.MustRepoPath(t, fixtures.ContractFixtureCatalogRelativePath)
	service, err := factorysessionexecution.NewFakeServiceFromContractFixtures(catalogPath)
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures() error = %v", err)
	}
	return mcpfactorysession.NewClientWithService(service)
}

func mustProjectResultPolicyInventory(t *testing.T) mcpfactorysession.ResultPolicyInventory {
	t.Helper()
	inventory, err := mcpfactorysession.ProjectResultPolicyInventory()
	if err != nil {
		t.Fatalf("ProjectResultPolicyInventory() error = %v", err)
	}
	return inventory
}

func newResultPolicyTestServer(t *testing.T, client *mcpfactorysession.Client) *mcpserver.Server {
	t.Helper()
	srv, err := mcpserver.New(mcpserver.Options{Client: client})
	if err != nil {
		t.Fatalf("server.New() error = %v", err)
	}
	return srv
}

func runResultPolicyServerHandleLine(t *testing.T, srv *mcpserver.Server, line string) string {
	t.Helper()
	var output bytes.Buffer
	if err := srv.ServeStdio(context.Background(), strings.NewReader(line+"\n"), &output); err != nil {
		t.Fatalf("ServeStdio(%s) error = %v", line, err)
	}
	return strings.TrimSpace(output.String())
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
