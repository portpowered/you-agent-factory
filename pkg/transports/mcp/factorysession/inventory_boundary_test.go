package factorysession_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/fixtures"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession"
	mcpserver "github.com/portpowered/infinite-you/pkg/transports/mcp/server"
)

func TestProjectAliasInventory_BuildsDocumentShape(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectAliasInventory()
	if err != nil {
		t.Fatalf("ProjectAliasInventory() error = %v", err)
	}
	if inventory.FormatVersion != mcpfactorysession.ToolInventoryFormatVersion {
		t.Fatalf("formatVersion = %q, want %q", inventory.FormatVersion, mcpfactorysession.ToolInventoryFormatVersion)
	}
	if inventory.ProtocolVersion != mcpfactorysession.ToolInventoryProtocolVersion {
		t.Fatalf("protocolVersion = %q, want %q", inventory.ProtocolVersion, mcpfactorysession.ToolInventoryProtocolVersion)
	}
	if len(inventory.Aliases) != len(mcpfactorysession.DiscoverCompatibilityAliases()) {
		t.Fatalf("alias count = %d, want %d", len(inventory.Aliases), len(mcpfactorysession.DiscoverCompatibilityAliases()))
	}
}

func TestProjectAliasInventory_AliasesSortedByName(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectAliasInventory()
	if err != nil {
		t.Fatalf("ProjectAliasInventory() error = %v", err)
	}
	names := make([]string, len(inventory.Aliases))
	for i, alias := range inventory.Aliases {
		names[i] = alias.Name
	}
	sorted := slices.Clone(names)
	slices.Sort(sorted)
	if !slices.Equal(names, sorted) {
		t.Fatalf("alias names = %#v, want sorted %#v", names, sorted)
	}
}

func TestProjectAliasInventory_EachAliasHasMappingFields(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectAliasInventory()
	if err != nil {
		t.Fatalf("ProjectAliasInventory() error = %v", err)
	}
	for _, alias := range inventory.Aliases {
		if strings.TrimSpace(alias.Name) == "" {
			t.Fatal("alias name is required")
		}
		if strings.TrimSpace(alias.CanonicalName) == "" {
			t.Fatalf("alias %q canonical name is required", alias.Name)
		}
		if strings.TrimSpace(alias.Description) == "" {
			t.Fatalf("alias %q description is required", alias.Name)
		}
		if !alias.CompatibilityOnly {
			t.Fatalf("alias %q compatibilityOnly = false, want true", alias.Name)
		}
		if !strings.HasPrefix(alias.Name, "you.workflow.") {
			t.Fatalf("alias %q should use workflow vocabulary", alias.Name)
		}
		if !strings.HasPrefix(alias.CanonicalName, "you.factory_session.") {
			t.Fatalf("alias %q canonical target %q should use Factory Session vocabulary", alias.Name, alias.CanonicalName)
		}
	}
}

func TestProjectAliasInventory_MatchesDiscoverCompatibilityAliases(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectAliasInventory()
	if err != nil {
		t.Fatalf("ProjectAliasInventory() error = %v", err)
	}
	byName := aliasInventoryByName(t, inventory)
	for _, discovered := range mcpfactorysession.DiscoverCompatibilityAliases() {
		entry, ok := byName[discovered.Name]
		if !ok {
			t.Fatalf("inventory missing compatibility alias %q", discovered.Name)
		}
		if entry.CanonicalName != discovered.CanonicalName {
			t.Fatalf("alias %q canonicalName = %q, want %q", discovered.Name, entry.CanonicalName, discovered.CanonicalName)
		}
		if entry.CompatibilityOnly != discovered.CompatibilityOnly {
			t.Fatalf("alias %q compatibilityOnly = %v, want %v", discovered.Name, entry.CompatibilityOnly, discovered.CompatibilityOnly)
		}
		if entry.Description != discovered.Description {
			t.Fatalf("alias %q description = %q, want %q", discovered.Name, entry.Description, discovered.Description)
		}
	}
}

func TestProjectAliasInventory_RepeatExtractionIsByteIdentical(t *testing.T) {
	first, err := mcpfactorysession.MarshalAliasInventoryJSON(mustProjectAliasInventory(t))
	if err != nil {
		t.Fatalf("first MarshalAliasInventoryJSON() error = %v", err)
	}
	second, err := mcpfactorysession.MarshalAliasInventoryJSON(mustProjectAliasInventory(t))
	if err != nil {
		t.Fatalf("second MarshalAliasInventoryJSON() error = %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("repeat extraction differs:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestVerifyProjectedAliasInventory_PassesForLiveRegistry(t *testing.T) {
	if err := mcpfactorysession.VerifyProjectedAliasInventory(); err != nil {
		t.Fatalf("VerifyProjectedAliasInventory() error = %v", err)
	}
}

func TestVerifyAliasInventory_RejectsAliasInCanonicalToolInventory(t *testing.T) {
	inventory := mcpfactorysession.AliasInventory{
		FormatVersion:   mcpfactorysession.ToolInventoryFormatVersion,
		ProtocolVersion: mcpfactorysession.ToolInventoryProtocolVersion,
		Aliases: []mcpfactorysession.AliasInventoryEntry{{
			Name:              mcpfactorysession.ToolWorkflowValidate,
			CanonicalName:     mcpfactorysession.ToolValidateSource,
			CompatibilityOnly: true,
			Description:       "probe alias that leaked into canonical inventory",
		}},
	}
	toolInventory, err := mcpfactorysession.ProjectToolInventory()
	if err != nil {
		t.Fatalf("ProjectToolInventory() error = %v", err)
	}
	for i := range toolInventory.Tools {
		if toolInventory.Tools[i].Name == mcpfactorysession.ToolWorkflowValidate {
			t.Fatal("canonical tool inventory should not contain workflow alias names")
		}
	}
	if err := mcpfactorysession.VerifyAliasInventory(inventory); err != nil {
		t.Fatalf("VerifyAliasInventory() error = %v", err)
	}
}

func TestVerifyAliasInventory_FailsWhenCanonicalTargetMissing(t *testing.T) {
	inventory := mcpfactorysession.AliasInventory{
		FormatVersion:   mcpfactorysession.ToolInventoryFormatVersion,
		ProtocolVersion: mcpfactorysession.ToolInventoryProtocolVersion,
		Aliases: []mcpfactorysession.AliasInventoryEntry{{
			Name:              "you.workflow.probe",
			CanonicalName:     "you.factory_session.unregistered_probe",
			CompatibilityOnly: true,
			Description:       "probe alias with missing canonical target",
		}},
	}
	err := mcpfactorysession.VerifyAliasInventory(inventory)
	if err == nil {
		t.Fatal("VerifyAliasInventory() error = nil, want failure")
	}
	if !strings.Contains(err.Error(), "you.factory_session.unregistered_probe") {
		t.Fatalf("VerifyAliasInventory() error = %v, want missing canonical target", err)
	}
}

func TestVerifyAliasInventory_FailsWhenResolveToolNameDisagrees(t *testing.T) {
	inventory := mcpfactorysession.AliasInventory{
		FormatVersion:   mcpfactorysession.ToolInventoryFormatVersion,
		ProtocolVersion: mcpfactorysession.ToolInventoryProtocolVersion,
		Aliases: []mcpfactorysession.AliasInventoryEntry{{
			Name:              mcpfactorysession.ToolWorkflowValidate,
			CanonicalName:     mcpfactorysession.ToolStartSync,
			CompatibilityOnly: true,
			Description:       "probe alias with incorrect canonical mapping",
		}},
	}
	err := mcpfactorysession.VerifyAliasInventory(inventory)
	if err == nil {
		t.Fatal("VerifyAliasInventory() error = nil, want failure")
	}
	if !strings.Contains(err.Error(), mcpfactorysession.ToolWorkflowValidate) {
		t.Fatalf("VerifyAliasInventory() error = %v, want alias name", err)
	}
}

func TestAliasBaselineFixtureMatchesProjectedInventory(t *testing.T) {
	baselinePath := testutil.MustRepoPath(t, mcpfactorysession.AliasInventoryBaselineRelativePath)
	baseline, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("read baseline fixture: %v", err)
	}
	projected, err := mcpfactorysession.MarshalAliasInventoryJSON(mustProjectAliasInventory(t))
	if err != nil {
		t.Fatalf("MarshalAliasInventoryJSON() error = %v", err)
	}
	if string(baseline) != string(projected) {
		t.Fatalf("baseline fixture differs from projected inventory:\nbaseline=%s\nprojected=%s", baseline, projected)
	}
}

func TestAliasBaselineFixtureMatchesDiscoverCompatibilityAliases(t *testing.T) {
	baselinePath := testutil.MustRepoPath(t, mcpfactorysession.AliasInventoryBaselineRelativePath)
	baseline, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("read baseline fixture: %v", err)
	}
	var inventory mcpfactorysession.AliasInventory
	if err := json.Unmarshal(baseline, &inventory); err != nil {
		t.Fatalf("unmarshal baseline fixture: %v", err)
	}
	if err := mcpfactorysession.VerifyAliasInventory(inventory); err != nil {
		t.Fatalf("VerifyAliasInventory(baseline) error = %v", err)
	}
	byName := aliasInventoryByName(t, inventory)
	for _, discovered := range mcpfactorysession.DiscoverCompatibilityAliases() {
		entry, ok := byName[discovered.Name]
		if !ok {
			t.Fatalf("baseline missing compatibility alias %q", discovered.Name)
		}
		if entry.CanonicalName != discovered.CanonicalName {
			t.Fatalf("baseline alias %q canonicalName = %q, want %q", discovered.Name, entry.CanonicalName, discovered.CanonicalName)
		}
	}
}

func TestAliasBaselineFixture_AliasNamesAbsentFromCanonicalToolsBaseline(t *testing.T) {
	aliasBaselinePath := testutil.MustRepoPath(t, mcpfactorysession.AliasInventoryBaselineRelativePath)
	toolBaselinePath := testutil.MustRepoPath(t, mcpfactorysession.ToolInventoryBaselineRelativePath)

	aliasBaseline, err := os.ReadFile(aliasBaselinePath)
	if err != nil {
		t.Fatalf("read alias baseline fixture: %v", err)
	}
	toolBaseline, err := os.ReadFile(toolBaselinePath)
	if err != nil {
		t.Fatalf("read tool baseline fixture: %v", err)
	}

	var aliasInventory mcpfactorysession.AliasInventory
	if err := json.Unmarshal(aliasBaseline, &aliasInventory); err != nil {
		t.Fatalf("unmarshal alias baseline fixture: %v", err)
	}
	var toolInventory mcpfactorysession.ToolInventory
	if err := json.Unmarshal(toolBaseline, &toolInventory); err != nil {
		t.Fatalf("unmarshal tool baseline fixture: %v", err)
	}

	canonicalNames := make(map[string]struct{}, len(toolInventory.Tools))
	for _, tool := range toolInventory.Tools {
		canonicalNames[tool.Name] = struct{}{}
	}
	for _, alias := range aliasInventory.Aliases {
		if _, ok := canonicalNames[alias.Name]; ok {
			t.Fatalf("compatibility alias %q must not appear in canonical mcp-tools.json baseline", alias.Name)
		}
	}
}

func mustProjectAliasInventory(t *testing.T) mcpfactorysession.AliasInventory {
	t.Helper()
	inventory, err := mcpfactorysession.ProjectAliasInventory()
	if err != nil {
		t.Fatalf("ProjectAliasInventory() error = %v", err)
	}
	return inventory
}

func aliasInventoryByName(t *testing.T, inventory mcpfactorysession.AliasInventory) map[string]mcpfactorysession.AliasInventoryEntry {
	t.Helper()
	byName := make(map[string]mcpfactorysession.AliasInventoryEntry, len(inventory.Aliases))
	for _, alias := range inventory.Aliases {
		byName[alias.Name] = alias
	}
	return byName
}

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
	raw, err := client.CallTool(mcpfactorysession.ToolGetSession, arguments)
	if err != nil {
		t.Fatalf("CallTool(get_session) error = %v", err)
	}

	srv := newResultPolicyTestServer(t, client)
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
	srv := newResultPolicyTestServer(t, newResultPolicyFixtureMCPClient(t))
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

func TestVerifyProjectedMCPBoundaryInventories_PassesForLiveProjections(t *testing.T) {
	if err := mcpfactorysession.VerifyProjectedMCPBoundaryInventories(); err != nil {
		t.Fatalf("VerifyProjectedMCPBoundaryInventories() error = %v", err)
	}
}

func TestProjectMCPBoundaryInventories_DoesNotMutateDiscovery(t *testing.T) {
	beforeTools := cloneToolDefinitions(t, mcpfactorysession.DiscoverTools())
	beforeAliases := cloneCompatibilityAliases(t, mcpfactorysession.DiscoverCompatibilityAliases())

	if _, err := mcpfactorysession.ProjectToolInventory(); err != nil {
		t.Fatalf("ProjectToolInventory() error = %v", err)
	}
	if _, err := mcpfactorysession.ProjectAliasInventory(); err != nil {
		t.Fatalf("ProjectAliasInventory() error = %v", err)
	}
	if _, err := mcpfactorysession.ProjectResultPolicyInventory(); err != nil {
		t.Fatalf("ProjectResultPolicyInventory() error = %v", err)
	}

	afterTools := mcpfactorysession.DiscoverTools()
	afterAliases := mcpfactorysession.DiscoverCompatibilityAliases()
	if len(beforeTools) != len(afterTools) {
		t.Fatalf("discover tool count changed: before=%d after=%d", len(beforeTools), len(afterTools))
	}
	for i := range beforeTools {
		beforeJSON, err := json.Marshal(beforeTools[i])
		if err != nil {
			t.Fatalf("marshal before tool %q: %v", beforeTools[i].Name, err)
		}
		afterJSON, err := json.Marshal(afterTools[i])
		if err != nil {
			t.Fatalf("marshal after tool %q: %v", afterTools[i].Name, err)
		}
		if string(beforeJSON) != string(afterJSON) {
			t.Fatalf("tool %q mutated by boundary inventory projections", beforeTools[i].Name)
		}
	}
	if len(beforeAliases) != len(afterAliases) {
		t.Fatalf("compatibility alias count changed: before=%d after=%d", len(beforeAliases), len(afterAliases))
	}
	for i := range beforeAliases {
		beforeJSON, err := json.Marshal(beforeAliases[i])
		if err != nil {
			t.Fatalf("marshal before alias %q: %v", beforeAliases[i].Name, err)
		}
		afterJSON, err := json.Marshal(afterAliases[i])
		if err != nil {
			t.Fatalf("marshal after alias %q: %v", afterAliases[i].Name, err)
		}
		if string(beforeJSON) != string(afterJSON) {
			t.Fatalf("compatibility alias %q mutated by boundary inventory projections", beforeAliases[i].Name)
		}
	}
}

func TestMCPBoundaryBaselineFixtures_RepeatExtractionIsByteIdentical(t *testing.T) {
	aliasFirst, err := mcpfactorysession.MarshalAliasInventoryJSON(mustProjectAliasInventory(t))
	if err != nil {
		t.Fatalf("first MarshalAliasInventoryJSON() error = %v", err)
	}
	aliasSecond, err := mcpfactorysession.MarshalAliasInventoryJSON(mustProjectAliasInventory(t))
	if err != nil {
		t.Fatalf("second MarshalAliasInventoryJSON() error = %v", err)
	}
	if string(aliasFirst) != string(aliasSecond) {
		t.Fatalf("alias repeat extraction differs")
	}

	policyFirst, err := mcpfactorysession.MarshalResultPolicyInventoryJSON(mustProjectResultPolicyInventory(t))
	if err != nil {
		t.Fatalf("first MarshalResultPolicyInventoryJSON() error = %v", err)
	}
	policySecond, err := mcpfactorysession.MarshalResultPolicyInventoryJSON(mustProjectResultPolicyInventory(t))
	if err != nil {
		t.Fatalf("second MarshalResultPolicyInventoryJSON() error = %v", err)
	}
	if string(policyFirst) != string(policySecond) {
		t.Fatalf("result-policy repeat extraction differs")
	}

	aliasBaselinePath := testutil.MustRepoPath(t, mcpfactorysession.AliasInventoryBaselineRelativePath)
	aliasBaseline, err := os.ReadFile(aliasBaselinePath)
	if err != nil {
		t.Fatalf("read alias baseline fixture: %v", err)
	}
	if string(aliasBaseline) != string(aliasFirst) {
		t.Fatalf("alias baseline fixture differs from projected inventory")
	}

	policyBaselinePath := testutil.MustRepoPath(t, mcpfactorysession.ResultPolicyInventoryBaselineRelativePath)
	policyBaseline, err := os.ReadFile(policyBaselinePath)
	if err != nil {
		t.Fatalf("read result-policy baseline fixture: %v", err)
	}
	if string(policyBaseline) != string(policyFirst) {
		t.Fatalf("result-policy baseline fixture differs from projected inventory")
	}
}

func TestMCPBoundaryBaselineFixtures_EveryDeclaredAliasResolvesOnce(t *testing.T) {
	inventory, err := mcpfactorysession.ProjectAliasInventory()
	if err != nil {
		t.Fatalf("ProjectAliasInventory() error = %v", err)
	}
	if err := mcpfactorysession.VerifyAliasInventory(inventory); err != nil {
		t.Fatalf("VerifyAliasInventory() error = %v", err)
	}
	if len(inventory.Aliases) != len(mcpfactorysession.DiscoverCompatibilityAliases()) {
		t.Fatalf("alias count = %d, want %d", len(inventory.Aliases), len(mcpfactorysession.DiscoverCompatibilityAliases()))
	}
}

func cloneCompatibilityAliases(t *testing.T, aliases []mcpfactorysession.CompatibilityAlias) []mcpfactorysession.CompatibilityAlias {
	t.Helper()
	encoded, err := json.Marshal(aliases)
	if err != nil {
		t.Fatalf("marshal compatibility aliases: %v", err)
	}
	var cloned []mcpfactorysession.CompatibilityAlias
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		t.Fatalf("unmarshal compatibility aliases: %v", err)
	}
	return cloned
}
