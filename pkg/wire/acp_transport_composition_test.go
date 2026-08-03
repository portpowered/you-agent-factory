package wire

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
)

// ResolveNamedFactory extends the sibling staticFactoryDefinitionsService
// fake (declared in chat_sessions_composition_test.go) with the one other
// Factory Definitions collaborator method a "session/new" or
// "session/set_config_option" call reaches: the working-root compatibility
// check the Factory target-catalog operation performs whenever a caller
// supplies a non-blank ClientWorkingRoot. A global-source resolution always
// satisfies that check, matching the default ACP Agent profile's
// "@you/factory-builder" default target, which every case in this file
// selects.
func (s *staticFactoryDefinitionsService) ResolveNamedFactory(
	context.Context,
	factorydefinitions.ResolveNamedFactoryRequest,
) (factorydefinitions.ResolveNamedFactoryResult, error) {
	return factorydefinitions.ResolveNamedFactoryResult{
		Resolution: factorydefinitions.NamedFactoryResolution{
			Source: factorydefinitions.NamedFactoryResolutionSourceGlobal,
		},
	}, nil
}

// rpcTestMessage is the minimal JSON-RPC 2.0 response shape this test reads
// off the real ACP stdio Server's Serve output.
type rpcTestMessage struct {
	ID     json.RawMessage      `json:"id"`
	Result json.RawMessage      `json:"result"`
	Error  *acpsdk.RequestError `json:"error"`
}

// TestACPServerReachesCanonicalChatSessionsAuthorityThroughWireComposition
// proves the exact provider chain this graph registers for the production
// ACP consumer (provideACPServer, consuming the same provideChatSessionsService
// and provideChatSessionsFactoryTargetCatalogService chain every other
// canonical consumer composes through) actually reaches the one process-scoped
// Chat Sessions authority: a real "session/new" call through the constructed
// acp.Server creates a session that is directly observable on the exact
// chatsessions.Service instance injected into that same server, and a real
// "session/set_config_option" call against it performs one target mutation
// through the live catalog -- not a second, independently constructed Chat
// engine and not a hand-rolled transport double.
func TestACPServerReachesCanonicalChatSessionsAuthorityThroughWireComposition(t *testing.T) {
	t.Parallel()

	if _, err := InjectBundle(context.Background(), serviceedges.Edges{}); err != nil {
		t.Fatalf("InjectBundle() error = %v", err)
	}

	logger, chatSessions, catalog := buildACPCompositionTestCollaborators(t)

	home := t.TempDir()
	resolveHomeDir := acpServerResolveHomeDir(func() (string, error) { return home, nil })
	server := provideACPServer(logger, chatSessions, catalog, resolveHomeDir)
	if server == nil {
		t.Fatal("provideACPServer() returned a nil acp.Server")
	}

	cwd := t.TempDir()
	created, initialSession := assertSessionNewCreatesObservableSession(t, server, chatSessions, cwd)
	assertSetConfigOptionMutatesTargetThroughTheSameAuthority(t, server, chatSessions, created, initialSession)
}

// buildACPCompositionTestCollaborators constructs the same canonical logger,
// chatsessions.Service, and chatsessions.FactoryTargetCatalogService chain
// pkg/wire's generated InjectBundle registers for the production ACP
// consumer, with a real Operator Settings root and a focused Factory
// Definitions double standing in for the one collaborator only constructible
// from a live Factory Session (see staticFactoryDefinitionsService's doc
// comment in the sibling chat_sessions_composition_test.go).
func buildACPCompositionTestCollaborators(t *testing.T) (logging.Logger, chatsessions.Service, chatsessions.FactoryTargetCatalogService) {
	t.Helper()

	zapLogger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatalf("logging.NewDefaultLogger() error = %v", err)
	}
	logger := logging.NewZapLogger(zapLogger, false)

	chatSessions, err := provideChatSessionsService(logger)
	if err != nil {
		t.Fatalf("provideChatSessionsService() error = %v", err)
	}

	factoryBuilderLocation := "/factories/@you/factory-builder"
	factoryDefinitions := &staticFactoryDefinitionsService{
		entries: []factorydefinitions.EffectiveFactoryCatalogEntry{
			{
				Name:       "@you/factory-builder",
				Location:   &factoryBuilderLocation,
				Definition: &factorydefinitions.FactoryConfig{Name: "Factory Builder"},
			},
		},
	}
	// A non-existent Operator Settings path resolves the built-in default ACP
	// Agent profile (DefaultTarget "factory:@you/factory-builder"), matching
	// the sole installed entry above, exactly like the sibling
	// provideChatSessionsFactoryTargetCatalogService composition test does.
	edges := serviceedges.Edges{}
	files := provideOperatorSettingsFileSystem(edges)
	providersRoot, err := provideProvidersService(edges)
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}
	providerRegistry, err := provideProviderRegistry(edges, providersRoot)
	if err != nil {
		t.Fatalf("provideProviderRegistry() error = %v", err)
	}
	operatorSettings, err := provideOperatorSettingsService(
		files,
		provideOperatorSettingsCreateTemporaryFile(edges),
		provideOperatorSettingsProviderCatalog(providerRegistry),
		provideOperatorConfigDecoder(),
		provideOperatorConfigEncoder(),
		provideOperatorSettingsIDGenerator(edges),
		providersRoot,
		logger,
	)
	if err != nil {
		t.Fatalf("provideOperatorSettingsService() error = %v", err)
	}
	catalog, err := provideChatSessionsFactoryTargetCatalogService(operatorSettings, factoryDefinitions, logger)
	if err != nil {
		t.Fatalf("provideChatSessionsFactoryTargetCatalogService() error = %v", err)
	}
	return logger, chatSessions, catalog
}

// assertSessionNewCreatesObservableSession drives one real "session/new"
// call through server and proves the created session is directly
// observable on chatSessions -- the exact instance this test injected into
// provideACPServer -- so the ACP boundary and the canonical Chat Sessions
// authority are proven to be the one singular instance, not two
// independently constructed engines.
func assertSessionNewCreatesObservableSession(
	t *testing.T,
	server acp.Server,
	chatSessions chatsessions.Service,
	cwd string,
) (acpsdk.NewSessionResponse, chatsessions.GetSessionResult) {
	t.Helper()

	var out bytes.Buffer
	newSessionLine := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":%q,"mcpServers":[]}}`+"\n",
		cwd,
	)
	if err := server.Serve(context.Background(), strings.NewReader(newSessionLine), &out); err != nil {
		t.Fatalf("Serve(session/new) error = %v", err)
	}

	resp := decodeRPCTestMessage(t, &out)
	if resp.Error != nil {
		t.Fatalf("session/new response error = %+v, want a successful result", resp.Error)
	}
	var created acpsdk.NewSessionResponse
	if err := json.Unmarshal(resp.Result, &created); err != nil {
		t.Fatalf("unmarshal session/new result: %v", err)
	}
	if created.SessionId == "" {
		t.Fatal("session/new result sessionId is blank")
	}
	if len(created.ConfigOptions) != 1 {
		t.Fatalf("session/new result configOptions = %d, want exactly one Factory target picker option", len(created.ConfigOptions))
	}

	getResult, err := chatSessions.GetSession(context.Background(), chatsessions.GetSessionRequest{
		SessionID: string(created.SessionId),
	})
	if err != nil {
		t.Fatalf("GetSession(%q) on the canonical instance error = %v, want the session the ACP server just created", created.SessionId, err)
	}
	if getResult.Session.WorkingRoot != cwd {
		t.Fatalf("created session WorkingRoot = %q, want the validated editor cwd %q", getResult.Session.WorkingRoot, cwd)
	}
	return created, getResult
}

// assertSetConfigOptionMutatesTargetThroughTheSameAuthority drives one real
// "session/set_config_option" call against the session created above and
// proves it performed exactly one live-catalog-revalidated SetTarget
// mutation, observable through the same canonical chatSessions instance.
func assertSetConfigOptionMutatesTargetThroughTheSameAuthority(
	t *testing.T,
	server acp.Server,
	chatSessions chatsessions.Service,
	created acpsdk.NewSessionResponse,
	initial chatsessions.GetSessionResult,
) {
	t.Helper()

	var out bytes.Buffer
	setConfigLine := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":2,"method":"session/set_config_option","params":{"sessionId":%q,"configId":"target","value":"factory:@you/factory-builder"}}`+"\n",
		created.SessionId,
	)
	if err := server.Serve(context.Background(), strings.NewReader(setConfigLine), &out); err != nil {
		t.Fatalf("Serve(session/set_config_option) error = %v", err)
	}
	resp := decodeRPCTestMessage(t, &out)
	if resp.Error != nil {
		t.Fatalf("session/set_config_option response error = %+v, want a successful result", resp.Error)
	}

	mutated, err := chatSessions.GetSession(context.Background(), chatsessions.GetSessionRequest{
		SessionID: string(created.SessionId),
	})
	if err != nil {
		t.Fatalf("GetSession() after set_config_option error = %v", err)
	}
	if mutated.Session.Version <= initial.Session.Version {
		t.Fatalf("session version after set_config_option = %d, want strictly newer than %d", mutated.Session.Version, initial.Session.Version)
	}
	if mutated.Session.TargetEpisode <= initial.Session.TargetEpisode {
		t.Fatalf("target episode after set_config_option = %d, want strictly newer than %d", mutated.Session.TargetEpisode, initial.Session.TargetEpisode)
	}
}

func decodeRPCTestMessage(t *testing.T, out *bytes.Buffer) rpcTestMessage {
	t.Helper()
	line, err := bufio.NewReader(bytes.NewReader(out.Bytes())).ReadString('\n')
	if err != nil {
		t.Fatalf("read response line: %v", err)
	}
	var msg rpcTestMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		t.Fatalf("unmarshal response line %q: %v", line, err)
	}
	return msg
}
