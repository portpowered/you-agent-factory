package wire

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
)

// rpcTestMessage is the minimal JSON-RPC 2.0 response shape this test reads
// off the real ACP stdio Server's Serve output.
type rpcTestMessage struct {
	ID     json.RawMessage      `json:"id"`
	Result json.RawMessage      `json:"result"`
	Error  *acpsdk.RequestError `json:"error"`
}

func TestACPServerHomeResolverUsesFactorySessionEdge(t *testing.T) {
	t.Parallel()

	const want = "/isolated/operator-home"
	resolver := provideACPServerResolveHomeDir(serviceedges.Edges{
		FactorySessionResolveHomeDirectory: func() (string, error) { return want, nil },
	})
	got, err := resolver()
	if err != nil {
		t.Fatalf("resolve ACP server home: %v", err)
	}
	if got != want {
		t.Fatalf("ACP server home = %q, want shared Factory Session edge %q", got, want)
	}
}

// TestACPServerReachesCanonicalChatSessionsAuthorityThroughRootBuildProcess
// proves the production construction path, not a hand-replicated provider
// chain: it seeds a real, isolated home directory with two real packaged
// Factories, calls InjectBundle (the exact function root.BuildProcess
// delegates to) to obtain the one canonical *application.Process, and drives
// every assertion through Process.ACPServer() -- the same accessor a real
// embedding entrypoint would use. Two separate Serve calls (distinct
// connections) reusing the identical bare JSON-RPC id 1 produce two distinct
// sessions, proving connection-scoped request identity; a third connection
// mutates the first connection's session, proving both observe the one
// process-scoped Chat Sessions authority Wire composed -- not a second,
// independently constructed engine and not a hand-rolled transport double.
func TestACPServerReachesCanonicalChatSessionsAuthorityThroughRootBuildProcess(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	seedInstalledPackagedFactories(t, home, "@you/goal", "@you/review")
	seedACPAgentProfile(t, home, "factory:@you/goal", []string{"factory:@you/goal", "factory:@you/review"})

	process, err := InjectBundle(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("InjectBundle() error = %v", err)
	}
	server := process.ACPServer()
	if server == nil {
		t.Fatal("Process.ACPServer() returned a nil acp.Server")
	}

	cwd := t.TempDir()
	sessionA := assertSessionNewReturnsDefaultTarget(t, server, cwd, "factory:@you/goal")
	sessionB := assertSessionNewReturnsDefaultTarget(t, server, cwd, "factory:@you/goal")
	if sessionA == sessionB {
		t.Fatalf("session/new on two distinct connections using the identical bare JSON-RPC id 1 produced the same sessionId %q, want request identity to be connection-scoped", sessionA)
	}

	assertSetConfigOptionFromAnotherConnectionMutatesTheSharedAuthority(t, server, sessionA, "factory:@you/review")
}

// seedInstalledPackagedFactories writes the real published JSON for each
// named built-in packaged Factory directly under the global named-Factory
// root derived from home, at <globalRoot>/<scope>/<name>/factory.json --
// the exact layout
// pkg/services/factory_definitions/internal/services/catalog/namedfactories
// and the effective-catalog discovery both read. This reaches the same
// production catalog code the rest of this graph composes, without
// hand-rolling a Factory Definitions double.
func seedInstalledPackagedFactories(t *testing.T, home string, names ...string) {
	t.Helper()

	globalRoot, err := factorydefinitions.NamedFactoriesRootForHome(home)
	if err != nil {
		t.Fatalf("NamedFactoriesRootForHome() error = %v", err)
	}

	definitions, err := providePackagedFactoryDefinitions()
	if err != nil {
		t.Fatalf("providePackagedFactoryDefinitions() error = %v", err)
	}
	catalog, err := providePackagedFactoryCatalog(definitions)
	if err != nil {
		t.Fatalf("providePackagedFactoryCatalog() error = %v", err)
	}

	for _, name := range names {
		resolved, err := catalog.ResolveBuiltInPackagedFactory(
			context.Background(),
			factorydefinitions.ResolveBuiltInPackagedFactoryRequest{Name: name},
		)
		if err != nil {
			t.Fatalf("ResolveBuiltInPackagedFactory(%q) error = %v", name, err)
		}
		scope, leaf, ok := strings.Cut(strings.TrimPrefix(name, "@"), "/")
		if !ok {
			t.Fatalf("packaged Factory name %q is not scoped as @scope/name", name)
		}
		factoryDir := filepath.Join(globalRoot, "@"+scope, leaf)
		if err := os.MkdirAll(factoryDir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", factoryDir, err)
		}
		configPath := filepath.Join(factoryDir, "factory.json")
		if err := os.WriteFile(configPath, resolved.Definition.JSON, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", configPath, err)
		}
	}
}

// seedACPAgentProfile persists a real ACP Agent profile at the production
// Operator Settings config path for home, through the same
// operatorsettings.Service.UpdateACPAgentProfile production callers use, so
// InjectBundle's real Operator Settings root resolves it unmodified.
func seedACPAgentProfile(t *testing.T, home, defaultTarget string, allowedTargets []string) {
	t.Helper()

	service, _ := newACPCLIOwnerRoots(t)

	configPath := operatorsettings.DefaultConfigPath(home)
	if _, err := service.UpdateACPAgentProfile(context.Background(), configPath, operatorsettings.ACPAgentProfile{
		DefaultTarget:  defaultTarget,
		AllowedTargets: allowedTargets,
	}); err != nil {
		t.Fatalf("UpdateACPAgentProfile() error = %v", err)
	}
}

// assertSessionNewReturnsDefaultTarget drives one real "session/new" call on
// its own connection (a fresh Serve invocation) using the fixed bare
// JSON-RPC id 1, and returns the created sessionId.
func assertSessionNewReturnsDefaultTarget(t *testing.T, server acp.Server, cwd, wantCurrent string) string {
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
	if len(created.ConfigOptions) != 1 || created.ConfigOptions[0].Select == nil {
		t.Fatalf("session/new configOptions = %+v, want exactly one Factory target picker option", created.ConfigOptions)
	}
	if string(created.ConfigOptions[0].Select.CurrentValue) != wantCurrent {
		t.Fatalf("session/new currentValue = %q, want %q", created.ConfigOptions[0].Select.CurrentValue, wantCurrent)
	}
	return string(created.SessionId)
}

// assertSetConfigOptionFromAnotherConnectionMutatesTheSharedAuthority drives
// one real "session/set_config_option" call, on a third, independent Serve
// connection reusing the same bare JSON-RPC id 1, addressing the session a
// different connection created. Success is observable only if both
// connections share the one process-scoped Chat Sessions authority Wire
// composed through provideACPServer.
func assertSetConfigOptionFromAnotherConnectionMutatesTheSharedAuthority(
	t *testing.T, server acp.Server, sessionID, newTarget string,
) {
	t.Helper()

	var out bytes.Buffer
	setConfigLine := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"session/set_config_option","params":{"sessionId":%q,"configId":"target","value":%q}}`+"\n",
		sessionID, newTarget,
	)
	if err := server.Serve(context.Background(), strings.NewReader(setConfigLine), &out); err != nil {
		t.Fatalf("Serve(session/set_config_option) error = %v", err)
	}
	resp := decodeRPCTestMessage(t, &out)
	if resp.Error != nil {
		t.Fatalf("session/set_config_option response error = %+v, want a successful result", resp.Error)
	}
	var mutated acpsdk.SetSessionConfigOptionResponse
	if err := json.Unmarshal(resp.Result, &mutated); err != nil {
		t.Fatalf("unmarshal session/set_config_option result: %v", err)
	}
	if len(mutated.ConfigOptions) != 1 || mutated.ConfigOptions[0].Select == nil {
		t.Fatalf("session/set_config_option configOptions = %+v, want exactly one Factory target picker option", mutated.ConfigOptions)
	}
	if string(mutated.ConfigOptions[0].Select.CurrentValue) != newTarget {
		t.Fatalf("session/set_config_option currentValue = %q, want %q", mutated.ConfigOptions[0].Select.CurrentValue, newTarget)
	}
}

// decodeRPCTestMessage returns the one correlated response frame in out.
//
// A connection legitimately interleaves session/update notifications with
// responses -- session/new advertises its available commands -- so
// notifications are skipped rather than mistaken for the response.
func decodeRPCTestMessage(t *testing.T, out *bytes.Buffer) rpcTestMessage {
	t.Helper()
	reader := bufio.NewReader(bytes.NewReader(out.Bytes()))
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read response line: %v", err)
		}
		var frame struct {
			Method string `json:"method"`
		}
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatalf("unmarshal response line %q: %v", line, err)
		}
		if frame.Method != "" {
			continue
		}
		var msg rpcTestMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatalf("unmarshal response line %q: %v", line, err)
		}
		return msg
	}
}
