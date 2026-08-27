package root_composition_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
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

// catalogCohort is the immutable no-profile process used by catalog-only
// witnesses. It owns every published packaged Factory in one fixed home, so
// absent-profile enumeration and target switching can share the same root
// without changing profile state or activating a Factory runtime.
type catalogCohort struct {
	home      string
	process   support.ApplicationProcess
	installed []string
}

var catalogCohortState struct {
	sync.Mutex
	cohort *catalogCohort
	err    error
}

func catalogCohortForTest(t *testing.T) *catalogCohort {
	t.Helper()

	catalogCohortState.Lock()
	defer catalogCohortState.Unlock()
	if catalogCohortState.cohort == nil && catalogCohortState.err == nil {
		home, err := os.MkdirTemp("", "you-chat-catalog-cohort-")
		if err != nil {
			catalogCohortState.err = fmt.Errorf("create catalog cohort home: %w", err)
		} else {
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			installed := seedEveryInstalledPackagedFactory(t, home)
			process, buildErr := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{})
			if buildErr != nil {
				catalogCohortState.err = fmt.Errorf("build catalog cohort process: %w", buildErr)
				_ = os.RemoveAll(home)
			} else {
				catalogCohortState.cohort = &catalogCohort{
					home:      home,
					process:   process,
					installed: installed,
				}
			}
		}
	}
	if catalogCohortState.err != nil {
		t.Fatalf("catalog cohort: %v", catalogCohortState.err)
	}

	cohort := catalogCohortState.cohort
	t.Setenv("HOME", cohort.home)
	t.Setenv("USERPROFILE", cohort.home)
	return cohort
}

// closeCatalogCohort releases the process and fixed home after all package
// tests have finished. It is called by the package TestMain next to the
// story-001 controlled cohort cleanup.
func closeCatalogCohort() {
	catalogCohortState.Lock()
	cohort := catalogCohortState.cohort
	catalogCohortState.Unlock()
	if cohort == nil {
		return
	}
	if err := cohort.process.Close(context.Background()); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "catalog cohort close: %v\n", err)
	}
	if err := os.RemoveAll(cohort.home); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "catalog cohort home cleanup: %v\n", err)
	}
}

// newCatalogProfileCohort builds one immutable process for an authored ACP
// Agent profile. Different allowlists are deliberately separate cohorts; the
// profile is read from the fixed home and must never be rewritten after root
// construction to make two catalog cases fit one process.
func newCatalogProfileCohort(
	t *testing.T,
	name, defaultTarget string,
	allowedTargets []string,
) *catalogCohort {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	seedEveryInstalledPackagedFactory(t, home)
	support.SeedACPAgentProfile(t, home, defaultTarget, allowedTargets)
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("build %s catalog profile cohort: %v", name, err)
	}
	cohort := &catalogCohort{home: home, process: process}
	closeProcessCleanly(t, process)
	return cohort
}

// TestACPServerReachesCanonicalChatSessionsAuthorityThroughRootBuildProcess
// proves the customer-facing construction path: it seeds a real, isolated
// home directory with a real installed packaged Factory and a real persisted
// ACP Agent profile, calls root.BuildProcess (the exact public entrypoint the
// you binary uses), and drives a real session/new call through
// Process.ACPServer() -- the production ACP stdio server -- observing the
// real Chat Sessions authority root.BuildProcess composed.
func TestACPServerReachesCanonicalChatSessionsAuthorityThroughRootBuildProcess(t *testing.T) {
	controlledACPHome(t)
	server := controlledACPServer(t)
	cwd := controlledACPWorkingDirectory(t, "canonical-server")
	sessionID := assertSessionNewReturnsDefaultTarget(t, server, cwd, "factory:@you/goal")
	if sessionID == "" {
		t.Fatal("session/new returned a blank sessionId")
	}
	secondCWD := controlledACPWorkingDirectory(t, "canonical-server-second")
	secondSessionID := assertSessionNewReturnsDefaultTarget(t, server, secondCWD, "factory:@you/goal")
	if secondSessionID == "" {
		t.Fatal("second session/new returned a blank sessionId")
	}
	if secondSessionID == sessionID {
		t.Fatalf("two ACP connections reused Chat Session identity %q, want unique session IDs", sessionID)
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

// decodeRPCMessage returns the one correlated response frame in out.
//
// A connection legitimately interleaves session/update notifications with
// responses -- session/new advertises its available commands -- so
// notifications are skipped rather than mistaken for the response.
func decodeRPCMessage(t *testing.T, out *bytes.Buffer) rpcMessage {
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
		var msg rpcMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatalf("unmarshal response line %q: %v", line, err)
		}
		return msg
	}
}

// seedEveryInstalledPackagedFactory materializes every published built-in
// packaged Factory under home's global named-Factory root, reproducing the
// state a real `you server acp` process reaches after system initialization
// runs. It returns the installed names.
func seedEveryInstalledPackagedFactory(t *testing.T, home string) []string {
	t.Helper()

	published, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("LoadPublishedDefinitionCatalog() error = %v", err)
	}
	names := published.Names()
	if len(names) == 0 {
		t.Fatal("published packaged Factory catalog is empty")
	}
	for _, name := range names {
		seedInstalledPackagedFactory(t, home, name)
	}
	return names
}

// factoryTargetSelectOption drives one real session/new call and returns the
// created session id together with the Factory target select option, so a
// caller can assert over the full choice list rather than only the current
// value.
func factoryTargetSelectOption(
	t *testing.T,
	server acp.Server,
	cwd string,
) (string, acpsdk.SessionConfigOptionSelect) {
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
	if len(created.ConfigOptions) != 1 || created.ConfigOptions[0].Select == nil {
		t.Fatalf("session/new configOptions = %+v, want exactly one Factory target picker option", created.ConfigOptions)
	}
	return string(created.SessionId), *created.ConfigOptions[0].Select
}

// selectOptionChoices returns the flat choice list the Factory target option
// always uses. The ACP select option models its choices as a
// grouped/ungrouped union; this transport only ever emits the ungrouped
// variant.
func selectOptionChoices(t *testing.T, option acpsdk.SessionConfigOptionSelect) []acpsdk.SessionConfigSelectOption {
	t.Helper()
	if option.Options.Ungrouped == nil {
		t.Fatalf("Factory target option choices = %+v, want the ungrouped variant", option.Options)
	}
	return *option.Options.Ungrouped
}

func selectOptionValues(t *testing.T, option acpsdk.SessionConfigOptionSelect) []string {
	t.Helper()
	choices := selectOptionChoices(t, option)
	values := make([]string, 0, len(choices))
	for _, choice := range choices {
		values = append(values, string(choice.Value))
	}
	return values
}

// TestACPServerWithNoAuthoredAgentProfileOffersEveryInstalledFactory proves the
// customer-visible model-enumeration behavior: an operator who never authored
// workers.acp.agentProfile sees every installed Factory in the ACP client's
// picker, not just Factory Builder.
func TestACPServerWithNoAuthoredAgentProfileOffersEveryInstalledFactory(t *testing.T) {
	cohort := catalogCohortForTest(t)
	installed := cohort.installed
	// Deliberately no SeedACPAgentProfile call: an absent profile must mean
	// "unrestricted", which is the whole point of this cell.
	server := cohort.process.ACPServer()
	if server == nil {
		t.Fatal("Process.ACPServer() returned a nil acp.Server")
	}

	sessionID, option := factoryTargetSelectOption(t, server, t.TempDir())
	if sessionID == "" {
		t.Fatal("session/new returned a blank sessionId")
	}

	choices := selectOptionChoices(t, option)
	if len(choices) != len(installed) {
		t.Fatalf("Factory target choices = %d (%v), want one per installed Factory (%d)",
			len(choices), selectOptionValues(t, option), len(installed))
	}
	if string(option.CurrentValue) != "factory:@you/factory-builder" {
		t.Fatalf("currentValue = %q, want factory:@you/factory-builder to start current",
			option.CurrentValue)
	}

	got := make(map[string]bool, len(choices))
	for _, choice := range choices {
		got[string(choice.Value)] = true
		if strings.TrimSpace(choice.Name) == "" {
			t.Fatalf("choice %q has a blank display name", choice.Value)
		}
	}
	for _, name := range installed {
		if !got["factory:"+name] {
			t.Fatalf("installed Factory %q is missing from the ACP picker; got %v",
				name, selectOptionValues(t, option))
		}
	}
	// The four packaged Factories problems.md calls out by name must all be
	// reachable, since that is the reported defect.
	for _, required := range []string{
		"factory:@you/plan-parallel",
		"factory:@you/classify",
		"factory:@you/goal",
		"factory:@you/loop",
	} {
		if !got[required] {
			t.Fatalf("%q is not selectable; got %v", required, selectOptionValues(t, option))
		}
	}
}

// TestACPServerAuthoredAllowedTargetsStillRestrictsCatalog proves
// allowedTargets remains a real opt-in restriction after the default became
// unrestricted.
func TestACPServerAuthoredAllowedTargetsStillRestrictsCatalog(t *testing.T) {
	cohort := newCatalogProfileCohort(t, "restricted", "factory:@you/goal", []string{
		"factory:@you/goal",
		"factory:@you/classify",
	})

	_, option := factoryTargetSelectOption(t, cohort.process.ACPServer(), t.TempDir())
	want := []string{"factory:@you/goal", "factory:@you/classify"}
	got := selectOptionValues(t, option)
	if len(got) != len(want) {
		t.Fatalf("Factory target choices = %v, want exactly %v", got, want)
	}
	if string(option.CurrentValue) != "factory:@you/goal" {
		t.Fatalf("currentValue = %q, want factory:@you/goal", option.CurrentValue)
	}
}

// TestACPServerSetConfigOptionSelectsAnotherInstalledFactory proves an
// unrestricted operator can actually switch the session onto a different
// installed Factory, not merely see it listed.
func TestACPServerSetConfigOptionSelectsAnotherInstalledFactory(t *testing.T) {
	cohort := catalogCohortForTest(t)
	server := cohort.process.ACPServer()
	if server == nil {
		t.Fatal("Process.ACPServer() returned a nil acp.Server")
	}

	cwd := t.TempDir()
	sessionID, option := factoryTargetSelectOption(t, server, cwd)
	if string(option.CurrentValue) == "factory:@you/plan-parallel" {
		t.Fatal("precondition: plan-parallel must not already be current")
	}

	var out bytes.Buffer
	line := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"session/set_config_option","params":`+
			`{"sessionId":%q,"configId":"target","value":"factory:@you/plan-parallel"}}`+"\n",
		sessionID,
	)
	if err := server.Serve(context.Background(), strings.NewReader(line), &out); err != nil {
		t.Fatalf("Serve(session/set_config_option) error = %v", err)
	}

	resp := decodeRPCMessage(t, &out)
	if resp.Error != nil {
		t.Fatalf("session/set_config_option error = %+v, want success", resp.Error)
	}
	var updated acpsdk.SetSessionConfigOptionResponse
	if err := json.Unmarshal(resp.Result, &updated); err != nil {
		t.Fatalf("unmarshal set_config_option result: %v", err)
	}
	if len(updated.ConfigOptions) != 1 || updated.ConfigOptions[0].Select == nil {
		t.Fatalf("configOptions = %+v, want the refreshed Factory target option", updated.ConfigOptions)
	}
	if string(updated.ConfigOptions[0].Select.CurrentValue) != "factory:@you/plan-parallel" {
		t.Fatalf("currentValue = %q, want factory:@you/plan-parallel",
			updated.ConfigOptions[0].Select.CurrentValue)
	}
}

// TestACPServerAuthoredAllowedTargetsAreOfferedInAuthoredOrder proves the
// ordering half of the allowedTargets contract in docs/reference/serve-acp.md:
// "A non-empty list | Only those targets are selectable, in the authored
// order."
//
// Order is the whole reason an operator writes a list rather than relying on
// the unrestricted default. It is what puts the Factory their team reaches for
// first at the top of the client's picker, and re-sorting it alphabetically
// silently discards that choice. The authored order deliberately disagrees
// with both alphabetical order and current-target-first order here, so neither
// can satisfy this cell by coincidence.
func TestACPServerAuthoredAllowedTargetsAreOfferedInAuthoredOrder(t *testing.T) {
	cohort := newCatalogProfileCohort(t, "authored-order", "factory:@you/goal", []string{
		"factory:@you/loop",
		"factory:@you/goal",
		"factory:@you/classify",
	})
	// loop/goal/classify: not alphabetical, and the default is authored
	// second rather than first.
	_, option := factoryTargetSelectOption(t, cohort.process.ACPServer(), t.TempDir())
	want := []string{"factory:@you/loop", "factory:@you/goal", "factory:@you/classify"}
	got := selectOptionValues(t, option)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("Factory target choices = %v, want the authored order %v", got, want)
	}
	if string(option.CurrentValue) != "factory:@you/goal" {
		t.Fatalf("currentValue = %q, want factory:@you/goal", option.CurrentValue)
	}
}

// TestACPServerUnrestrictedTargetsAreOfferedCurrentFirstThenSorted pins the
// other half: with no authored allowlist there is no authored order to
// preserve, so the catalog stays deterministic on its own terms -- the current
// target first, then every other installed Factory in canonical reference
// order.
func TestACPServerUnrestrictedTargetsAreOfferedCurrentFirstThenSorted(t *testing.T) {
	cohort := catalogCohortForTest(t)
	_, option := factoryTargetSelectOption(t, cohort.process.ACPServer(), t.TempDir())
	got := selectOptionValues(t, option)
	if len(got) < 3 {
		t.Fatalf("Factory target choices = %v, want the full installed catalog", got)
	}
	if got[0] != string(option.CurrentValue) {
		t.Fatalf("choices[0] = %q, want the current target %q first", got[0], option.CurrentValue)
	}
	if !sort.StringsAreSorted(got[1:]) {
		t.Fatalf("choices after the current target = %v, want canonical reference order", got[1:])
	}
}
