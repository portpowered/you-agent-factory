package root_composition_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	"github.com/portpowered/infinite-you/pkg/platform/process"
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

var packagedFactoryCatalogState struct {
	sync.Once
	catalog factorydefinitions.PackagedFactoryCatalogOperations
	names   []string
	err     error
}

func catalogCohortForTest(t *testing.T) *catalogCohort {
	t.Helper()

	catalogCohortState.Lock()
	defer catalogCohortState.Unlock()
	if catalogCohortState.cohort == nil && catalogCohortState.err == nil {
		home, err := chatPersistentMkdirTemp("catalog cohort", "you-chat-catalog-cohort-")
		if err != nil {
			catalogCohortState.err = fmt.Errorf("create catalog cohort home: %w", err)
		} else {
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			installed := seedEveryInstalledPackagedFactory(t, home)
			process, buildErr := buildChatProcess(t, "catalog cohort", serviceedges.Edges{})
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
func closeCatalogCohort() error {
	catalogCohortState.Lock()
	cohort := catalogCohortState.cohort
	catalogCohortState.Unlock()
	if cohort == nil {
		return nil
	}
	var errs []error
	if err := closeChatProcess(cohort.process); err != nil {
		errs = append(errs, fmt.Errorf("catalog cohort close: %w", err))
	}
	if err := chatRemoveRoot(cohort.home); err != nil {
		errs = append(errs, fmt.Errorf("catalog cohort home cleanup: %w", err))
	}
	return errors.Join(errs...)
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
	home := chatTempDir(t, "catalog profile "+name, "catalog-profile-")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	installed := make([]string, 0, len(allowedTargets))
	for _, target := range allowedTargets {
		installed = append(installed, strings.TrimPrefix(target, "factory:"))
	}
	seedInstalledPackagedFactories(t, home, installed)
	support.SeedACPAgentProfile(t, home, defaultTarget, allowedTargets)
	process, err := buildChatProcess(t, "catalog profile "+name, serviceedges.Edges{})
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

	catalog, _ := packagedFactoryCatalogForTest(t)
	seedInstalledPackagedFactoryFromCatalog(t, globalRoot, catalog, name)
}

func seedInstalledPackagedFactories(t *testing.T, home string, names []string) {
	t.Helper()

	globalRoot, err := factorydefinitions.NamedFactoriesRootForHome(home)
	if err != nil {
		t.Fatalf("NamedFactoriesRootForHome() error = %v", err)
	}
	catalog, _ := packagedFactoryCatalogForTest(t)
	for _, name := range names {
		seedInstalledPackagedFactoryFromCatalog(t, globalRoot, catalog, name)
	}
}

func seedInstalledPackagedFactoryFromCatalog(
	t *testing.T,
	globalRoot string,
	catalog factorydefinitions.PackagedFactoryCatalogOperations,
	name string,
) {
	t.Helper()
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
	registerChatFactoryPath(t, factoryDir)
	configPath := filepath.Join(factoryDir, "factory.json")
	if err := os.WriteFile(configPath, resolved.Definition.JSON, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", configPath, err)
	}
}

func packagedFactoryCatalogForTest(
	t *testing.T,
) (factorydefinitions.PackagedFactoryCatalogOperations, []string) {
	t.Helper()

	packagedFactoryCatalogState.Do(func() {
		published, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
		if err != nil {
			packagedFactoryCatalogState.err = fmt.Errorf("load published catalog: %w", err)
			return
		}
		catalog, err := factorydefinitionswire.NewPackagedFactoryCatalog(published.All())
		if err != nil {
			packagedFactoryCatalogState.err = fmt.Errorf("build packaged catalog: %w", err)
			return
		}
		packagedFactoryCatalogState.catalog = catalog
		packagedFactoryCatalogState.names = published.Names()
	})
	if packagedFactoryCatalogState.err != nil {
		t.Fatalf("packaged Factory catalog: %v", packagedFactoryCatalogState.err)
	}
	return packagedFactoryCatalogState.catalog, append([]string(nil), packagedFactoryCatalogState.names...)
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
	if err := serveChatRequest(server, context.Background(), strings.NewReader(newSessionLine), &out); err != nil {
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
	trackChatSessionOnServer(t, server, string(created.SessionId))
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

	_, names := packagedFactoryCatalogForTest(t)
	if len(names) == 0 {
		t.Fatal("published packaged Factory catalog is empty")
	}
	seedInstalledPackagedFactories(t, home, names)
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
	if err := serveChatRequest(server, context.Background(), strings.NewReader(newSessionLine), &out); err != nil {
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
	trackChatSessionOnServer(t, server, string(created.SessionId))
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

	sessionID, option := factoryTargetSelectOption(t, server, chatTempDir(t, "catalog-unrestricted", "catalog-cwd-"))
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

	_, option := factoryTargetSelectOption(t, cohort.process.ACPServer(), chatTempDir(t, "catalog-restricted", "catalog-cwd-"))
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

	cwd := chatTempDir(t, "catalog-switch", "catalog-cwd-")
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
	if err := serveChatRequest(server, context.Background(), strings.NewReader(line), &out); err != nil {
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
	_, option := factoryTargetSelectOption(t, cohort.process.ACPServer(), chatTempDir(t, "catalog-authored-order", "catalog-cwd-"))
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
	_, option := factoryTargetSelectOption(t, cohort.process.ACPServer(), chatTempDir(t, "catalog-unrestricted-order", "catalog-cwd-"))
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

const controlledACPFactory = "@you/goal"

// controlledACPCohort is an immutable-profile process fixture. The shared
// package cohort is used for ACP witnesses that do not activate a Factory
// runtime; activation-owning witnesses use newControlledACPCohort so their
// retained ~default Factory Definitions binding cannot leak into another
// scenario. Both paths use the same root.BuildProcess composition and the
// same request-keyed command-runner edge.
type controlledACPCohort struct {
	home                  string
	process               support.ApplicationProcess
	runner                *controlledACPCommandRunner
	factorySessionIDCalls atomic.Int32
	workingDirectoryRoot  string
}

var controlledCohortState struct {
	sync.Mutex
	cohort *controlledACPCohort
	err    error
}

// TestMain closes the package-scoped process only after every test invocation
// has stopped. Per-test ACP command executions close their own pipes and
// contexts, while this final close releases runtimes retained by the shared
// process before its fixed home is removed.
func TestMain(m *testing.M) {
	code := m.Run()

	if err := closeCatalogCohort(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "catalog cohort cleanup: %v\n", err)
		code = 1
	}

	controlledCohortState.Lock()
	cohort := controlledCohortState.cohort
	controlledCohortState.Unlock()
	if cohort != nil {
		if err := closeChatProcess(cohort.process); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "controlled ACP cohort close: %v\n", err)
			code = 1
		}
		if err := chatRemoveRoot(cohort.home); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "controlled ACP cohort home cleanup: %v\n", err)
			code = 1
		}
	}
	if err := chatCensus.assertClean(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "chat root-composition cleanup census: %v\n", err)
		code = 1
	}
	_, _ = fmt.Fprintf(os.Stderr, "chat root-composition cleanup census: %s\n", chatCensus.summary())

	os.Exit(code)
}

func controlledACPCohortForTest(t *testing.T) *controlledACPCohort {
	t.Helper()

	controlledCohortState.Lock()
	defer controlledCohortState.Unlock()
	if controlledCohortState.cohort == nil && controlledCohortState.err == nil {
		home, err := chatPersistentMkdirTemp("controlled ACP cohort", "you-chat-sessions-cohort-")
		if err != nil {
			controlledCohortState.err = fmt.Errorf("create controlled ACP cohort home: %w", err)
		} else {
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			workingDirectoryRoot := filepath.Join(home, "workdirs")
			if err := os.MkdirAll(workingDirectoryRoot, 0o755); err != nil {
				controlledCohortState.err = fmt.Errorf("create controlled ACP cohort workdirs: %w", err)
				_ = os.RemoveAll(home)
			} else {
				runner := &controlledACPCommandRunner{}
				seedInstalledPackagedFactory(t, home, controlledACPFactory)
				support.SeedACPAgentProfile(t, home, "factory:"+controlledACPFactory, []string{"factory:" + controlledACPFactory})

				cohort := &controlledACPCohort{
					home:                 home,
					runner:               runner,
					workingDirectoryRoot: workingDirectoryRoot,
				}
				process, err := buildChatProcess(t, "controlled ACP cohort", serviceedges.Edges{
					ProviderCommandRunner: runner,
					FactorySessionIDGenerator: func() string {
						n := cohort.factorySessionIDCalls.Add(1)
						return fmt.Sprintf("acp-cohort-factory-session-%d", n)
					},
				})
				if err != nil {
					controlledCohortState.err = fmt.Errorf("build controlled ACP cohort process: %w", err)
					_ = os.RemoveAll(home)
				} else {
					cohort.process = process
					controlledCohortState.cohort = cohort
				}
			}
		}
	}
	if controlledCohortState.err != nil {
		t.Fatalf("controlled ACP cohort: %v", controlledCohortState.err)
	}

	cohort := controlledCohortState.cohort
	t.Setenv("HOME", cohort.home)
	t.Setenv("USERPROFILE", cohort.home)
	return cohort
}

// newControlledACPCohort builds one fixed-profile root for a scenario whose
// real Factory activation remains retained by the on-demand ACP target. The
// production runtime currently binds Factory Definitions under the fixed
// ~default session scope and the public ACP close path cannot close a
// terminalized session, so sharing that retained activation across scenarios
// would make later tests fail with dependency_unavailable. This isolated
// process is the smallest faithful boundary until the production activation
// owner gains a supported release/reopen capability.
func newControlledACPCohort(t *testing.T, name string) *controlledACPCohort {
	t.Helper()
	home := chatMkdirTemp(t, "controlled ACP "+name, "", "you-chat-sessions-"+name+"-")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	workingDirectoryRoot := filepath.Join(home, "workdirs")
	if err := os.MkdirAll(workingDirectoryRoot, 0o755); err != nil {
		_ = os.RemoveAll(home)
		t.Fatalf("create controlled ACP %s workdirs: %v", name, err)
	}

	runner := &controlledACPCommandRunner{}
	seedInstalledPackagedFactory(t, home, controlledACPFactory)
	support.SeedACPAgentProfile(t, home, "factory:"+controlledACPFactory, []string{"factory:" + controlledACPFactory})

	cohort := &controlledACPCohort{
		home:                 home,
		runner:               runner,
		workingDirectoryRoot: workingDirectoryRoot,
	}
	process, err := buildChatProcess(t, "controlled ACP "+name, serviceedges.Edges{
		ProviderCommandRunner: runner,
		FactorySessionIDGenerator: func() string {
			n := cohort.factorySessionIDCalls.Add(1)
			return fmt.Sprintf("acp-%s-factory-session-%d", name, n)
		},
	})
	if err != nil {
		_ = os.RemoveAll(home)
		t.Fatalf("build controlled ACP %s process: %v", name, err)
	}
	cohort.process = process
	t.Cleanup(func() {
		if err := closeChatProcess(cohort.process); err != nil {
			t.Errorf("close controlled ACP %s process: %v", name, err)
		}
	})
	return cohort
}

func controlledACPHome(t *testing.T) string {
	t.Helper()
	return controlledACPCohortForTest(t).home
}

func controlledACPWorkingDirectory(t *testing.T, name string) string {
	t.Helper()
	cohort := controlledACPCohortForTest(t)
	return controlledACPWorkingDirectoryForCohort(t, cohort, name)
}

func controlledACPWorkingDirectoryForCohort(t *testing.T, cohort *controlledACPCohort, name string) string {
	t.Helper()
	return chatMkdirTemp(t, "controlled ACP "+name+" working directory", cohort.workingDirectoryRoot, name+"-")
}

func controlledACPServer(t *testing.T) support.ACPServer {
	t.Helper()
	cohort := controlledACPCohortForTest(t)
	return controlledACPServerForCohort(t, cohort)
}

func controlledACPServerForCohort(t *testing.T, cohort *controlledACPCohort) support.ACPServer {
	t.Helper()
	server := cohort.process.ACPServer()
	if server == nil {
		t.Fatal("controlled ACP cohort Process.ACPServer() returned nil")
	}
	return server
}

// controlledACPCommandRunner routes from the provider request itself. The
// request contains the current user turn, so the result does not depend on
// which compatible scenario ran first or how many provider calls a previous
// turn made. The busy route is armed only by its owning witness and can be
// released by the witness without changing the process edge.
type controlledACPCommandRunner struct {
	mu              sync.Mutex
	requests        []process.CommandRequest
	busyStarted     chan struct{}
	busyRelease     chan struct{}
	busyActive      bool
	busyStartedOnce sync.Once
	busyReleaseOnce sync.Once
}

func (runner *controlledACPCommandRunner) Run(
	ctx context.Context,
	request process.CommandRequest,
) (process.CommandResult, error) {
	callID := beginChatCall("controlled ACP provider")
	defer func() {
		if err := closeChatCall(callID); err != nil {
			chatCensus.recordViolation(err)
		}
	}()
	runner.mu.Lock()
	runner.requests = append(runner.requests, request)
	busyStarted := runner.busyStarted
	busyRelease := runner.busyRelease
	busyActive := runner.busyActive
	runner.mu.Unlock()

	prompt := string(request.Stdin)
	switch {
	case strings.Contains(prompt, "[cohort-failure]"):
		return controlledACPResult("not a decision envelope"), nil
	case strings.Contains(prompt, "[cohort-busy-concurrent]"):
		return controlledACPResult(`{"decision":"accepted","feedback":"","output":"busy concurrent route"}`), nil
	case strings.Contains(prompt, "[cohort-busy-later]"):
		return controlledACPResult(`{"decision":"accepted","feedback":"","output":"busy later route"}`), nil
	case strings.Contains(prompt, "[cohort-busy]") && busyActive:
		runner.busyStartedOnce.Do(func() { close(busyStarted) })
		select {
		case <-busyRelease:
		case <-ctx.Done():
			return process.CommandResult{}, ctx.Err()
		}
		return controlledACPResult(`{"decision":"accepted","feedback":"","output":"busy first route"}`), nil
	case strings.Contains(prompt, "pursue the third goal"):
		return controlledACPResult(`{"decision":"accepted","feedback":"","output":"third turn answer"}`), nil
	case strings.Contains(prompt, "pursue the second goal"):
		return controlledACPResult(`{"decision":"accepted","feedback":"","output":"second turn answer"}`), nil
	case strings.Contains(prompt, "pursue the first goal"):
		return controlledACPResult(`{"decision":"accepted","feedback":"","output":"first turn answer"}`), nil
	case strings.Contains(prompt, "please pursue this goal"):
		return controlledACPResult(`{"decision":"accepted","feedback":"","output":"goal genuinely completed through you server acp"}`), nil
	default:
		return controlledACPResult(`{"decision":"accepted","feedback":"","output":"goal reached over ACP"}`), nil
	}
}

func (runner *controlledACPCommandRunner) requestCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.requests)
}

func controlledACPResult(output string) process.CommandResult {
	return process.CommandResult{Stdout: support.CodexSuccessStdout(output)}
}

func (runner *controlledACPCommandRunner) armBusy() (<-chan struct{}, func()) {
	runner.mu.Lock()
	runner.busyStarted = make(chan struct{})
	runner.busyRelease = make(chan struct{})
	runner.busyActive = true
	runner.busyStartedOnce = sync.Once{}
	runner.busyReleaseOnce = sync.Once{}
	started := runner.busyStarted
	release := runner.busyRelease
	runner.mu.Unlock()

	return started, func() {
		runner.busyReleaseOnce.Do(func() {
			close(release)
			runner.mu.Lock()
			runner.busyActive = false
			runner.mu.Unlock()
		})
	}
}
