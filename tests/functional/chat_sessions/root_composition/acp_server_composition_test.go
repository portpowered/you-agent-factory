package root_composition_test

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

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	"github.com/portpowered/infinite-you/pkg/root"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

// rpcMessage is the minimal JSON-RPC 2.0 response shape this test reads off
// the real ACP stdio Server's Serve output.
type rpcMessage struct {
	ID     json.RawMessage      `json:"id"`
	Result json.RawMessage      `json:"result"`
	Error  *acpsdk.RequestError `json:"error"`
}

// TestACPServerReachesCanonicalChatSessionsAuthorityThroughRootBuildProcess
// proves the customer-facing construction path: it seeds a real, isolated
// home directory with a real installed packaged Factory and a real persisted
// ACP Agent profile, calls root.BuildProcess (the exact public entrypoint the
// you binary uses), and drives a real session/new call through
// Process.ACPServer() -- the production ACP stdio server -- observing the
// real Chat Sessions authority root.BuildProcess composed.
func TestACPServerReachesCanonicalChatSessionsAuthorityThroughRootBuildProcess(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	seedInstalledPackagedFactory(t, home, "@you/goal")
	support.SeedACPAgentProfile(t, home, "factory:@you/goal", []string{"factory:@you/goal"})

	process, err := root.BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	server := process.ACPServer()
	if server == nil {
		t.Fatal("Process.ACPServer() returned a nil acp.Server")
	}

	cwd := t.TempDir()
	sessionID := assertSessionNewReturnsDefaultTarget(t, server, cwd, "factory:@you/goal")
	if sessionID == "" {
		t.Fatal("session/new returned a blank sessionId")
	}
}

// seedInstalledPackagedFactory writes the real published JSON for one
// built-in packaged Factory directly under the global named-Factory root
// derived from home, at <globalRoot>/<scope>/<name>/factory.json -- the exact
// layout the production named-Factory catalog and effective-catalog
// discovery both read.
func seedInstalledPackagedFactory(t *testing.T, home, name string) {
	t.Helper()

	globalRoot, err := factorydefinitions.NamedFactoriesRootForHome(home)
	if err != nil {
		t.Fatalf("NamedFactoriesRootForHome() error = %v", err)
	}

	published, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("LoadPublishedDefinitionCatalog() error = %v", err)
	}
	catalog, err := factorydefinitionswire.NewPackagedFactoryCatalog(published.All())
	if err != nil {
		t.Fatalf("NewPackagedFactoryCatalog() error = %v", err)
	}
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

// assertSessionNewReturnsDefaultTarget drives one real "session/new" call on
// its own connection and returns the created sessionId.
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

	resp := decodeRPCMessage(t, &out)
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

func decodeRPCMessage(t *testing.T, out *bytes.Buffer) rpcMessage {
	t.Helper()
	line, err := bufio.NewReader(bytes.NewReader(out.Bytes())).ReadString('\n')
	if err != nil {
		t.Fatalf("read response line: %v", err)
	}
	var msg rpcMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		t.Fatalf("unmarshal response line %q: %v", line, err)
	}
	return msg
}
