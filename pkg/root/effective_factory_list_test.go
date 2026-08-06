package root

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	inference "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
)

func TestBuildProcessFactoryListProjectsEffectiveCatalogWithoutRuntimeOrWrites(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	workingDirectory := t.TempDir()
	projectRoot := filepath.Join(workingDirectory, "factory")
	globalRoot := filepath.Join(home, ".you-agent-factory", "factories")
	prepareRootListCatalog(t, projectRoot, globalRoot)
	before := rootListTreeSnapshot(t, home, workingDirectory)

	integration := &rootRecordingIntegration{identity: "catalog.test.provider"}
	var initializationCalls atomic.Int32
	apiStarts := 0
	process, err := BuildProcess(context.Background(), serviceedges.Edges{
		SystemInitializationInspectPath: func(string) (fs.FileInfo, error) {
			initializationCalls.Add(1)
			return nil, errors.New("factory list must not initialize")
		},
		ProviderRegistrations: []inference.Registration{{
			Manifest:    rootExternalManifest(t, "catalog.test.provider", "catalog-test"),
			Integration: integration,
		}},
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			apiStarts++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	entries, diagnostics := executeRootFactoryList(t, process, home, workingDirectory)
	assertRootEffectiveList(t, entries, diagnostics, projectRoot, globalRoot)
	assertRootFactoryNameCompletion(t, process, home, workingDirectory, entries)
	assertRootSignatureCompletion(t, process, home, workingDirectory)
	assertRootCanceledCompletion(t, process, home, workingDirectory)
	after := rootListTreeSnapshot(t, home, workingDirectory)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("filesystem changed during listing\nbefore: %#v\nafter:  %#v", before, after)
	}
	assertRootListEffects(t, initializationCalls.Load(), apiStarts, integration)
	assertRootCanceledList(t, process, home, workingDirectory)
}

// TestBuildProcessUsesInjectedNamedPathFileSystemForLiveCatalogResolution
// proves the named-path edge reaches the one canonical root/Wire graph. The
// fixture deliberately exposes only virtual Factory roots: neither root nor
// either selected Factory exists on the host, so the process cannot succeed
// through the default local filesystem. Construction remains inert, while
// subsequent public ACP and CLI paths observe the replacement's live named
// Factory and current-pointer state.
func TestBuildProcessUsesInjectedNamedPathFileSystemForLiveCatalogResolution(t *testing.T) {
	home := t.TempDir()
	workingDirectory := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	projectRoot := factorydefinitions.ProjectFactoriesRoot(workingDirectory)
	globalRoot, err := factorydefinitions.NamedFactoriesRootForHome(home)
	if err != nil {
		t.Fatalf("NamedFactoriesRootForHome() error = %v", err)
	}
	assertRootPathAbsent(t, projectRoot)
	assertRootPathAbsent(t, globalRoot)
	writeRootACPAgentProfile(t, home, "factory:@you/alpha")

	catalog := newRootVirtualCatalogFileSystem(t, projectRoot, globalRoot, "@you/alpha")
	apiStarts := 0
	process, err := BuildProcess(context.Background(), serviceedges.Edges{
		FactoryDefinitionNamedPathFileSystem:           catalog.namedPaths(),
		FactoryDefinitionNamedFactoryCatalogFileSystem: catalog.namedFactoryCatalog(),
		FactoryDefinitionAuthoredReaderFileSystem:      catalog.authoredReader(),
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			apiStarts++
			return errors.New("API lifecycle must not start during construction")
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	t.Cleanup(func() { _ = process.Close(context.Background()) })
	if catalog.namedPathCallCount() != 0 || apiStarts != 0 {
		t.Fatalf("construction calls = named-path filesystem:%d api:%d, want both zero", catalog.namedPathCallCount(), apiStarts)
	}

	catalog.resetNamedPathCalls()
	if got := rootACPNewSessionTarget(t, process.ACPServer(), workingDirectory); got != "factory:@you/alpha" {
		t.Fatalf("session/new target = %q, want replacement-defined factory:@you/alpha", got)
	}
	catalog.requireNamedPathCall(t, "Stat", filepath.Join(projectRoot, "@you", "alpha", factorydefinitions.FactoryConfigFile))

	catalog.resetNamedPathCalls()
	entries, _ := executeRootFactoryList(t, process, home, workingDirectory)
	assertRootVirtualCurrentFactory(t, entries, "@you/alpha", filepath.Join(projectRoot, "@you", "alpha"))
	catalog.requireNamedPathCall(t, "ReadFile", filepath.Join(projectRoot, ".current-factory"))

	catalog.setCurrent("@you/beta")
	writeRootACPAgentProfile(t, home, "factory:@you/beta")
	catalog.resetNamedPathCalls()
	if got := rootACPNewSessionTarget(t, process.ACPServer(), workingDirectory); got != "factory:@you/beta" {
		t.Fatalf("second session/new target = %q, want live replacement-defined factory:@you/beta", got)
	}
	catalog.requireNamedPathCall(t, "Stat", filepath.Join(projectRoot, "@you", "beta", factorydefinitions.FactoryConfigFile))

	catalog.resetNamedPathCalls()
	entries, _ = executeRootFactoryList(t, process, home, workingDirectory)
	assertRootVirtualCurrentFactory(t, entries, "@you/beta", filepath.Join(projectRoot, "@you", "beta"))
	catalog.requireNamedPathCall(t, "ReadFile", filepath.Join(projectRoot, ".current-factory"))
}

func assertRootPathAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("host path %q stat error = %v, want not exist", path, err)
	}
}

func writeRootACPAgentProfile(t *testing.T, home, target string) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"workers": map[string]any{"acp": map[string]any{"agentProfile": map[string]any{
			"defaultTarget": target, "allowedTargets": []string{target},
		}}},
	})
	if err != nil {
		t.Fatalf("marshal ACP agent profile: %v", err)
	}
	path := filepath.Join(home, ".you-agent-factory", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir ACP agent profile directory: %v", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write ACP agent profile: %v", err)
	}
}

func rootACPNewSessionTarget(t *testing.T, server acp.Server, workingDirectory string) string {
	t.Helper()
	var output bytes.Buffer
	request := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":%q,"mcpServers":[]}}`+"\n", workingDirectory)
	if err := server.Serve(context.Background(), strings.NewReader(request), &output); err != nil {
		t.Fatalf("ACP Serve(session/new) error = %v", err)
	}
	// A connection emits session/update notifications alongside responses
	// (session/new advertises its available commands), so select the
	// correlated response frame rather than decoding the whole buffer.
	var response struct {
		Method string               `json:"method"`
		Result json.RawMessage      `json:"result"`
		Error  *acpsdk.RequestError `json:"error"`
	}
	found := false
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		response = struct {
			Method string               `json:"method"`
			Result json.RawMessage      `json:"result"`
			Error  *acpsdk.RequestError `json:"error"`
		}{}
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			t.Fatalf("decode session/new response %q: %v", line, err)
		}
		if response.Method == "" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no session/new response frame in %q", output.String())
	}
	if response.Error != nil {
		t.Fatalf("session/new response error = %+v", response.Error)
	}
	var created acpsdk.NewSessionResponse
	if err := json.Unmarshal(response.Result, &created); err != nil {
		t.Fatalf("decode session/new result: %v", err)
	}
	if len(created.ConfigOptions) != 1 || created.ConfigOptions[0].Select == nil {
		t.Fatalf("session/new config options = %#v, want one target select", created.ConfigOptions)
	}
	return string(created.ConfigOptions[0].Select.CurrentValue)
}

func assertRootVirtualCurrentFactory(t *testing.T, entries []rootListEntry, name, location string) {
	t.Helper()
	for _, entry := range entries {
		if entry.Name != name {
			continue
		}
		if !entry.Current || entry.FactoryDirectory != location {
			t.Fatalf("current entry %q = %#v, want current at virtual location %q", name, entry, location)
		}
		return
	}
	t.Fatalf("current entry %q not found in %#v", name, entries)
}

type rootVirtualCatalogFileSystem struct {
	mu             sync.Mutex
	projectRoot    string
	globalRoot     string
	current        string
	namedPathCalls []rootVirtualCatalogFileSystemCall
}

type rootVirtualCatalogFileSystemCall struct {
	operation string
	path      string
}

func newRootVirtualCatalogFileSystem(t *testing.T, projectRoot, globalRoot, current string) *rootVirtualCatalogFileSystem {
	t.Helper()
	if current != "@you/alpha" && current != "@you/beta" {
		t.Fatalf("virtual current Factory = %q, want @you/alpha or @you/beta", current)
	}
	return &rootVirtualCatalogFileSystem{projectRoot: filepath.Clean(projectRoot), globalRoot: filepath.Clean(globalRoot), current: current}
}

func (f *rootVirtualCatalogFileSystem) namedPaths() rootVirtualNamedPathFileSystem {
	return rootVirtualNamedPathFileSystem{catalog: f}
}

func (f *rootVirtualCatalogFileSystem) namedFactoryCatalog() rootVirtualNamedFactoryCatalogFileSystem {
	return rootVirtualNamedFactoryCatalogFileSystem{catalog: f}
}

func (f *rootVirtualCatalogFileSystem) authoredReader() rootVirtualAuthoredReaderFileSystem {
	return rootVirtualAuthoredReaderFileSystem{catalog: f}
}

type rootVirtualNamedPathFileSystem struct{ catalog *rootVirtualCatalogFileSystem }

func (p rootVirtualNamedPathFileSystem) ReadFile(path string) ([]byte, error) {
	f := p.catalog
	f.mu.Lock()
	defer f.mu.Unlock()
	path = f.recordNamedPathCall("ReadFile", path)
	if path == filepath.Join(f.projectRoot, ".current-factory") {
		return []byte(f.current + "\n"), nil
	}
	return nil, fs.ErrNotExist
}

func (p rootVirtualNamedPathFileSystem) Stat(path string) (fs.FileInfo, error) {
	f := p.catalog
	f.mu.Lock()
	defer f.mu.Unlock()
	path = f.recordNamedPathCall("Stat", path)
	if path == filepath.Join(f.projectRoot, f.current) {
		return rootVirtualFileInfo{name: filepath.Base(path), mode: fs.ModeDir | 0o755}, nil
	}
	if path == filepath.Join(f.projectRoot, f.current, factorydefinitions.FactoryConfigFile) {
		return rootVirtualFileInfo{name: factorydefinitions.FactoryConfigFile, mode: 0o600}, nil
	}
	return nil, fs.ErrNotExist
}

func (p rootVirtualNamedPathFileSystem) MkdirAll(path string, _ fs.FileMode) error {
	f := p.catalog
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recordNamedPathCall("MkdirAll", path)
	return errors.New("virtual catalog is read-only")
}

func (p rootVirtualNamedPathFileSystem) WriteFile(path string, _ []byte, _ fs.FileMode) error {
	f := p.catalog
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recordNamedPathCall("WriteFile", path)
	return errors.New("virtual catalog is read-only")
}

type rootVirtualNamedFactoryCatalogFileSystem struct{ catalog *rootVirtualCatalogFileSystem }

func (c rootVirtualNamedFactoryCatalogFileSystem) Stat(path string) (fs.FileInfo, error) {
	f := c.catalog
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.catalogStat(filepath.Clean(path))
}

func (c rootVirtualNamedFactoryCatalogFileSystem) ReadDir(path string) ([]os.DirEntry, error) {
	f := c.catalog
	f.mu.Lock()
	defer f.mu.Unlock()
	path = filepath.Clean(path)
	if path == f.projectRoot {
		return []os.DirEntry{rootVirtualDirEntry{name: "@you"}}, nil
	}
	if path == filepath.Join(f.projectRoot, "@you") {
		return []os.DirEntry{rootVirtualDirEntry{name: filepath.Base(f.current)}}, nil
	}
	if path == f.globalRoot {
		return []os.DirEntry{}, nil
	}
	return nil, fs.ErrNotExist
}

func (c rootVirtualNamedFactoryCatalogFileSystem) RemoveAll(path string) error {
	f := c.catalog
	f.mu.Lock()
	defer f.mu.Unlock()
	return errors.New("virtual catalog is read-only")
}

type rootVirtualAuthoredReaderFileSystem struct{ catalog *rootVirtualCatalogFileSystem }

func (r rootVirtualAuthoredReaderFileSystem) ReadFile(path string) ([]byte, error) {
	f := r.catalog
	f.mu.Lock()
	defer f.mu.Unlock()
	path = filepath.Clean(path)
	if path == filepath.Join(f.projectRoot, f.current, factorydefinitions.FactoryConfigFile) {
		return rootVirtualFactoryJSON(f.current), nil
	}
	return nil, fs.ErrNotExist
}

func (r rootVirtualAuthoredReaderFileSystem) Stat(path string) (fs.FileInfo, error) {
	f := r.catalog
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.catalogStat(filepath.Clean(path))
}

func (f *rootVirtualCatalogFileSystem) setCurrent(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.current = name
}

func (f *rootVirtualCatalogFileSystem) namedPathCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.namedPathCalls)
}

func (f *rootVirtualCatalogFileSystem) resetNamedPathCalls() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.namedPathCalls = nil
}

func (f *rootVirtualCatalogFileSystem) requireNamedPathCall(t *testing.T, operation, path string) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	path = filepath.Clean(path)
	for _, call := range f.namedPathCalls {
		if call.operation == operation && call.path == path {
			return
		}
	}
	t.Fatalf("named-path filesystem calls = %#v, want %s(%q)", f.namedPathCalls, operation, path)
}

func (f *rootVirtualCatalogFileSystem) recordNamedPathCall(operation, path string) string {
	path = filepath.Clean(path)
	f.namedPathCalls = append(f.namedPathCalls, rootVirtualCatalogFileSystemCall{operation: operation, path: path})
	return path
}

func (f *rootVirtualCatalogFileSystem) catalogStat(path string) (fs.FileInfo, error) {
	if path == f.projectRoot || path == f.globalRoot || path == filepath.Join(f.projectRoot, "@you") || path == filepath.Join(f.projectRoot, f.current) {
		return rootVirtualFileInfo{name: filepath.Base(path), mode: fs.ModeDir | 0o755}, nil
	}
	if path == filepath.Join(f.projectRoot, f.current, factorydefinitions.FactoryConfigFile) {
		return rootVirtualFileInfo{name: factorydefinitions.FactoryConfigFile, mode: 0o600}, nil
	}
	return nil, fs.ErrNotExist
}

func rootVirtualFactoryJSON(name string) []byte {
	payload, _ := json.Marshal(map[string]any{
		"name": name, "id": name,
		"description": map[string]any{"type": "LOCALIZABLE_ASSET", "value": "Virtual " + name},
		"workTypes":   []any{}, "resources": []any{}, "workers": []any{}, "workstations": []any{},
	})
	return payload
}

type rootVirtualFileInfo struct {
	name string
	mode fs.FileMode
}

func (i rootVirtualFileInfo) Name() string       { return i.name }
func (i rootVirtualFileInfo) Size() int64        { return 0 }
func (i rootVirtualFileInfo) Mode() fs.FileMode  { return i.mode }
func (i rootVirtualFileInfo) ModTime() time.Time { return time.Time{} }
func (i rootVirtualFileInfo) IsDir() bool        { return i.mode.IsDir() }
func (i rootVirtualFileInfo) Sys() any           { return nil }

type rootVirtualDirEntry struct{ name string }

func (e rootVirtualDirEntry) Name() string    { return e.name }
func (rootVirtualDirEntry) IsDir() bool       { return true }
func (rootVirtualDirEntry) Type() fs.FileMode { return fs.ModeDir }
func (e rootVirtualDirEntry) Info() (fs.FileInfo, error) {
	return rootVirtualFileInfo{name: e.name, mode: fs.ModeDir | 0o755}, nil
}

func assertRootSignatureCompletion(
	t *testing.T,
	process interface{ Execute(Input) error },
	home string,
	workingDirectory string,
) {
	t.Helper()
	var first bytes.Buffer
	for index := 0; index < 2; index++ {
		var output bytes.Buffer
		var diagnostics bytes.Buffer
		if err := process.Execute(Input{
			Args: []string{
				"you", "__complete", "run", "--named", "aaa-local", "--req",
			},
			Env:              homeEnvironment(home),
			Stdout:           &output,
			Stderr:           &diagnostics,
			Context:          context.Background(),
			WorkingDirectory: workingDirectory,
		}); err != nil {
			t.Fatalf("Process.Execute(signature completion) error = %v", err)
		}
		if output.String() != "--request\tRequest payload\n:4\n" {
			t.Fatalf("signature completion output = %q", output.String())
		}
		if diagnostics.String() !=
			"Completion ended with directive: ShellCompDirectiveNoFileComp\n" {
			t.Fatalf("signature completion diagnostics = %q", diagnostics.String())
		}
		if index == 0 {
			first.WriteString(output.String())
		} else if output.String() != first.String() {
			t.Fatalf(
				"repeated signature completion differs: first=%q again=%q",
				first.String(),
				output.String(),
			)
		}
	}
}

func assertRootCanceledCompletion(
	t *testing.T,
	process interface{ Execute(Input) error },
	home string,
	workingDirectory string,
) {
	t.Helper()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer
	var diagnostics bytes.Buffer
	err := process.Execute(Input{
		Args: []string{
			"you", "__complete", "run", "--named", "aaa-local", "--req",
		},
		Env:              homeEnvironment(home),
		Stdout:           &output,
		Stderr:           &diagnostics,
		Context:          cancelled,
		WorkingDirectory: workingDirectory,
	})
	if err != nil {
		t.Fatalf("Process.Execute(cancelled completion) error = %v", err)
	}
	if output.String() != ":5\n" {
		t.Fatalf("cancelled completion output = %q, want atomic error directive", output.String())
	}
	if strings.Contains(output.String(), "request") ||
		strings.Contains(diagnostics.String(), "aaa-local") {
		t.Fatalf(
			"cancelled completion leaked candidates or selected Factory: (%q, %q)",
			output.String(),
			diagnostics.String(),
		)
	}
}

func assertRootFactoryNameCompletion(
	t *testing.T,
	process interface{ Execute(Input) error },
	home string,
	workingDirectory string,
	listed []rootListEntry,
) {
	t.Helper()
	var output bytes.Buffer
	var diagnostics bytes.Buffer
	if err := process.Execute(Input{
		Args:             []string{"you", "__complete", "run", "--named", ""},
		Env:              homeEnvironment(home),
		Stdout:           &output,
		Stderr:           &diagnostics,
		Context:          context.Background(),
		WorkingDirectory: workingDirectory,
	}); err != nil {
		t.Fatalf("Process.Execute(factory-name completion) error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) == 0 || lines[len(lines)-1] != ":4" {
		t.Fatalf("completion output = %q, want no-file directive", output.String())
	}
	lines = lines[:len(lines)-1]
	names := make([]string, len(lines))
	for index, line := range lines {
		names[index] = strings.SplitN(line, "\t", 2)[0]
	}
	want := make([]string, len(listed))
	for index, entry := range listed {
		want[index] = entry.Name
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("completed names = %v, listed names = %v", names, want)
	}
	if !strings.Contains(output.String(), "aaa-local\tLocal description") {
		t.Fatalf("completion output = %q, want effective description", output.String())
	}
	if strings.Contains(output.String(), "broken") ||
		strings.Contains(output.String(), "do-not-leak") {
		t.Fatalf("completion output leaked malformed entry: %q", output.String())
	}
	if strings.Contains(diagnostics.String(), "do-not-leak") {
		t.Fatalf("completion diagnostics leaked malformed entry: %q", diagnostics.String())
	}
}

type rootListEntry struct {
	Name              string `json:"name"`
	FactoryDirectory  string `json:"factoryDirectory"`
	Current           bool   `json:"current"`
	Description       string `json:"description"`
	InvocationExample string `json:"invocationExample"`
}

func executeRootFactoryList(
	t *testing.T,
	process interface{ Execute(Input) error },
	home string,
	workingDirectory string,
) ([]rootListEntry, string) {
	t.Helper()
	var output bytes.Buffer
	var diagnostics bytes.Buffer
	if err := process.Execute(Input{
		Args:             []string{"you", "--json", "factory", "list"},
		Env:              homeEnvironment(home),
		Stdout:           &output,
		Stderr:           &diagnostics,
		Context:          context.Background(),
		WorkingDirectory: workingDirectory,
	}); err != nil {
		t.Fatalf("Process.Execute(factory list) error = %v", err)
	}
	var entries []rootListEntry
	if err := json.Unmarshal(output.Bytes(), &entries); err != nil {
		t.Fatalf("decode factory list: %v\n%s", err, output.String())
	}
	return entries, diagnostics.String()
}

func assertRootEffectiveList(
	t *testing.T,
	entries []rootListEntry,
	diagnostics string,
	projectRoot string,
	globalRoot string,
) {
	t.Helper()
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Name
	}
	if !slices.IsSorted(names) {
		t.Fatalf("factory names = %v, want ascending order", names)
	}
	assertRootListEntry(t, entries, "aaa-local", filepath.Join(projectRoot, "aaa-local"), false, "Local description", true)
	assertRootListEntry(t, entries, "mmm-global", filepath.Join(globalRoot, "mmm-global"), false, "Global description", false)
	assertRootListEntry(t, entries, "zzz-shared", filepath.Join(projectRoot, "zzz-shared"), true, "Project override", true)
	if strings.Count(strings.Join(names, "\n"), "zzz-shared") != 1 {
		t.Fatalf("effective names = %v, want one shared entry", names)
	}
	packagedOnly := false
	for _, entry := range entries {
		if entry.FactoryDirectory == "-" {
			packagedOnly = true
			break
		}
	}
	if !packagedOnly {
		t.Fatalf("entries = %#v, want an unmaterialized packaged Factory", entries)
	}
	if !strings.Contains(diagnostics, "global broken (malformed)") ||
		strings.Contains(diagnostics, "do-not-leak") {
		t.Fatalf("diagnostics = %q, want safe malformed-entry context", diagnostics)
	}
}

func assertRootListEffects(
	t *testing.T,
	initializationCalls int32,
	apiStarts int,
	integration *rootRecordingIntegration,
) {
	t.Helper()
	if initializationCalls != 0 || apiStarts != 0 ||
		integration.discoverCalls != 0 || integration.capabilityCalls != 0 || integration.invokeCalls != 0 {
		t.Fatalf(
			"forbidden effects = initialization:%d api:%d provider:(%d,%d,%d)",
			initializationCalls,
			apiStarts,
			integration.discoverCalls,
			integration.capabilityCalls,
			integration.invokeCalls,
		)
	}
}

func assertRootCanceledList(
	t *testing.T,
	process interface{ Execute(Input) error },
	home string,
	workingDirectory string,
) {
	t.Helper()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer
	var diagnostics bytes.Buffer
	err := process.Execute(Input{
		Args:             []string{"you", "factory", "list"},
		Env:              homeEnvironment(home),
		Stdout:           &output,
		Stderr:           &diagnostics,
		Context:          canceled,
		WorkingDirectory: workingDirectory,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled factory list error = %v, want context canceled", err)
	}
	if output.Len() != 0 || diagnostics.String() != "Error: context canceled\n" {
		t.Fatalf(
			"canceled factory list output = (%q, %q), want no listing and standard cancellation diagnostic",
			output.String(),
			diagnostics.String(),
		)
	}
}

func prepareRootListCatalog(t *testing.T, projectRoot string, globalRoot string) {
	t.Helper()
	writeRootListFactory(t, projectRoot, "aaa-local", "Local description", true)
	writeRootListFactory(t, projectRoot, "zzz-shared", "Project override", true)
	writeRootListFactory(t, globalRoot, "mmm-global", "Global description", false)
	writeRootListFactory(t, globalRoot, "zzz-shared", "Shadowed global", false)
	writeRootListPayload(t, globalRoot, "broken", []byte(`{"secret":"do-not-leak"`))
	if err := os.WriteFile(filepath.Join(globalRoot, ".current-factory"), []byte("zzz-shared\n"), 0o600); err != nil {
		t.Fatalf("write current pointer: %v", err)
	}
}

func writeRootListFactory(
	t *testing.T,
	root string,
	name string,
	description string,
	signature bool,
) {
	t.Helper()
	definition := map[string]any{
		"name": name,
		"id":   name,
		"description": map[string]any{
			"type":  "LOCALIZABLE_ASSET",
			"value": description,
		},
		"workTypes":    []any{},
		"resources":    []any{},
		"workers":      []any{},
		"workstations": []any{},
	}
	if signature {
		definition["invocationSignature"] = map[string]any{
			"parameters": []map[string]any{{
				"name": "prompt", "externalName": "request", "required": true,
				"description": "Request payload",
				"bindings":    []map[string]any{{"kind": "NAMED"}},
			}},
		}
	}
	payload, err := json.Marshal(definition)
	if err != nil {
		t.Fatalf("marshal %s: %v", name, err)
	}
	writeRootListPayload(t, root, name, payload)
}

func writeRootListPayload(t *testing.T, root string, name string, payload []byte) {
	t.Helper()
	factoryDirectory := filepath.Join(root, name)
	if err := os.MkdirAll(factoryDirectory, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", factoryDirectory, err)
	}
	if err := os.WriteFile(filepath.Join(factoryDirectory, "factory.json"), payload, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func assertRootListEntry(
	t *testing.T,
	entries []rootListEntry,
	name string,
	location string,
	current bool,
	description string,
	hasSignature bool,
) {
	t.Helper()
	for _, entry := range entries {
		if entry.Name != name {
			continue
		}
		if entry.FactoryDirectory != location || entry.Current != current ||
			entry.Description != description {
			t.Fatalf("entry %s = %#v", name, entry)
		}
		if hasSignature != (entry.InvocationExample != "") {
			t.Fatalf("entry %s invocation example = %q, signature=%t", name, entry.InvocationExample, hasSignature)
		}
		return
	}
	t.Fatalf("entry %s not found", name)
}

func rootListTreeSnapshot(t *testing.T, roots ...string) map[string]string {
	t.Helper()
	snapshot := make(map[string]string)
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			key := root + ":" + relative
			if entry.IsDir() {
				snapshot[key] = "directory"
				return nil
			}
			payload, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			snapshot[key] = string(payload)
			return nil
		})
		if err != nil {
			t.Fatalf("snapshot %s: %v", root, err)
		}
	}
	return snapshot
}
