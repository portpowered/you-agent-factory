package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	workcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
)

type rootTestHTTPClock struct{}

func (rootTestHTTPClock) Now() time.Time { return time.Unix(1, 0) }

func rootTestHTTPProtocol() clihttp.Protocol {
	protocol, err := clihttp.NewProtocol(&http.Client{}, rootTestHTTPClock{})
	if err != nil {
		panic(err)
	}
	return protocol
}

func TestSessionCommand_RegistersSubcommands(t *testing.T) {
	root := newLegacyTestRootCommand()
	for _, path := range [][]string{
		{"session", "list"},
		{"session", "show"},
		{"session", "pause"},
		{"session", "resume"},
		{"session", "create"},
		{"session", "delete"},
	} {
		if _, _, err := root.Find(path); err != nil {
			t.Fatalf("find %v: %v", path, err)
		}
	}
}

func TestSessionCommand_HelpDocumentsSubcommandsAndExamples(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"session", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute session --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"list",
		"show",
		"pause",
		"resume",
		"cancel",
		"terminate",
		"create",
		"delete",
		"you session list",
		"you session show",
		"you session pause",
		"you session resume",
		"you --remote --server http://factory.example:7437 session pause",
		"you session resume session-beta --remote --server http://factory.example:7437",
		"you session list --json",
		"you session create --dir /workspace/fleet --port 9090",
		"you --server http://localhost:9090 session delete session-beta --json",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("session help missing %q:\n%s", want, help)
		}
	}
}

func TestSessionListHelpOutputDocumentsCombinedHistoryWorkflow(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "session list",
			args: []string{"session", "list", "--help"},
			want: []string{
				"default scope is all",
				"--live-only",
				"--history-only",
				"recorded-history artifacts",
			},
		},
		{
			name: "session",
			args: []string{"session", "--help"},
			want: []string{
				"recorded history",
				"--live-only or --history-only",
				"mutually exclusive",
			},
		},
		{
			name: "root",
			args: []string{"--help"},
			want: []string{
				"session",
				"List, open, and close factory sessions on a running host",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			root := newLegacyTestRootCommand()
			root.SetOut(&out)
			root.SetErr(io.Discard)
			root.SetArgs(test.args)

			if err := root.Execute(); err != nil {
				t.Fatalf("execute %s: %v", strings.Join(test.args, " "), err)
			}

			help := out.String()
			for _, want := range test.want {
				if !strings.Contains(help, want) {
					t.Fatalf("help missing %q:\n%s", want, help)
				}
			}
			if strings.Contains(strings.ToLower(help), "recording list") {
				t.Fatalf("help introduced a competing recording-list vocabulary:\n%s", help)
			}
		})
	}
}

func TestSessionListCommand_ConflictingFlagsFailBeforeHTTP(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"--server", "http://127.0.0.1:1",
		"session", "list", "--live-only", "--history-only",
	})

	err := root.Execute()
	if err == nil ||
		!strings.Contains(err.Error(), "cannot be used together") ||
		!strings.Contains(err.Error(), "--live-only") ||
		!strings.Contains(err.Error(), "--history-only") {
		t.Fatalf("conflicting session-list flags error = %v, want actionable mutual-exclusion error", err)
	}
	if out.Len() != 0 {
		t.Fatalf("conflict stdout = %q, want empty output", out.String())
	}
}

func TestSessionListCommand_ConflictingFlagsRenderTypedDiagnostic(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := withTestInjectedPlatformRoles(CommandFactory{}).ExecuteCommand(startupcli.CommandInvocation{
		Arguments: []string{
			"--server", "http://127.0.0.1:1",
			"session", "list", "--live-only", "--history-only",
		},
		Stdin:   strings.NewReader(""),
		Stdout:  &stdout,
		Stderr:  &stderr,
		Context: context.Background(),
	})
	if err == nil {
		t.Fatal("conflicting session-list flags error = nil, want failure")
	}
	if stdout.Len() != 0 {
		t.Fatalf("conflict stdout = %q, want empty output", stdout.String())
	}
	for _, want := range []string{
		`"code":"CLI_FLAG_CONFLICT"`,
		`"family":"BAD_REQUEST"`,
		"cannot be used together",
		"--live-only",
		"--history-only",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("conflict stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestSessionPauseCommand_HelpDocumentsOperatorControls(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"session", "pause", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute session pause --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"pause [session-id]",
		"~default",
		"session-beta",
		"you session list --scope all",
		"already-paused",
		"invalid-state",
		"not-found",
		"unreachable-host",
		"Factory Session",
		"you session pause",
		"--json",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("session pause help missing %q:\n%s", want, help)
		}
	}
}

func TestSessionPauseCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalPauseSession := pauseSession
	defer func() {
		pauseSession = originalPauseSession
	}()

	var got session.LifecycleControlConfig
	pauseSession = func(cfg session.LifecycleControlConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "--server", "http://127.0.0.1:9090", "session", "pause"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute session pause with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to LifecycleControlConfig.JSON")
	}
	if got.Server != "http://127.0.0.1:9090" {
		t.Fatalf("server = %q, want http://127.0.0.1:9090", got.Server)
	}
	if got.SessionID != "" {
		t.Fatalf("sessionId = %q, want omitted-session default routing", got.SessionID)
	}
}

func TestSessionShowCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalShowSession := showSession
	defer func() {
		showSession = originalShowSession
	}()

	var got session.ShowConfig
	showSession = func(cfg session.ShowConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "--server", "http://127.0.0.1:9090", "session", "show", "dur-sess-js-run-n-001"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute session show with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to ShowConfig.JSON")
	}
	if got.Server != "http://127.0.0.1:9090" {
		t.Fatalf("server = %q, want http://127.0.0.1:9090", got.Server)
	}
	if got.SessionID != "dur-sess-js-run-n-001" {
		t.Fatalf("sessionId = %q, want dur-sess-js-run-n-001", got.SessionID)
	}
}

func TestSessionDispatchesCommand_IsUnknownAfterRemoval(t *testing.T) {
	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"session", "dispatches", "dur-sess-js-run-n-001"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), `unknown command "dispatches"`) {
		t.Fatalf("execute retired session dispatches = %v, want unknown command", err)
	}
}

func TestSessionResumeCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalResumeSession := resumeSession
	defer func() {
		resumeSession = originalResumeSession
	}()

	var got session.LifecycleControlConfig
	resumeSession = func(cfg session.LifecycleControlConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "--server", "http://127.0.0.1:9090", "session", "resume", "session-beta"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute session resume with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to LifecycleControlConfig.JSON")
	}
	if got.Server != "http://127.0.0.1:9090" {
		t.Fatalf("server = %q, want http://127.0.0.1:9090", got.Server)
	}
	if got.SessionID != "session-beta" {
		t.Fatalf("sessionId = %q, want session-beta", got.SessionID)
	}
}

func TestSessionPauseCommand_AllowsOmittedSessionID(t *testing.T) {
	originalPauseSession := pauseSession
	defer func() {
		pauseSession = originalPauseSession
	}()

	var got session.LifecycleControlConfig
	pauseSession = func(cfg session.LifecycleControlConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"session", "pause"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute session pause without session id: %v", err)
	}
	if got.SessionID != "" {
		t.Fatalf("sessionId = %q, want omitted-session default routing", got.SessionID)
	}
}

func TestSessionResumeCommand_HelpDocumentsOperatorControls(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"session", "resume", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute session resume --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"resume [session-id]",
		"~default",
		"session-beta",
		"you session list --scope all",
		"already-running",
		"invalid-state",
		"not-found",
		"unreachable-host",
		"Factory Session",
		"you session resume",
		"--json",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("session resume help missing %q:\n%s", want, help)
		}
	}
}

func TestRootCommand_HelpDocumentsSessionCommand(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"session",
		"List, open, and close factory sessions on a running host",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}
}

func TestWorkListCommand_HelpDocumentsSessionListDiscoverability(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"work", "list", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute work list --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"you session list",
		"discover live session ids",
		"--name",
		"--work-type-name",
		"--trace-id",
		"before pagination",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("work list help missing %q:\n%s", want, help)
		}
	}
}

func TestWorkMoveCommand_HelpDocumentsOperatorMove(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"work", "move", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute work move --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"work move <work-id> <state-name>",
		"you session list",
		"--session",
		"--request-id",
		"active dispatch",
		"409",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("work move help missing %q:\n%s", want, help)
		}
	}
}

func TestWorkRenderCommand_HelpDocumentsReadOnlyFormatsAndRedirection(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"work", "render", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute work render --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"render <batch-file.json>",
		"read-only",
		"does not submit work",
		"contact a running factory",
		"render diagram images",
		"default: mermaid",
		"markdown-mermaid",
		"work items",
		"dependency relations",
		"> my-graph.mermaid",
		"> graph.md",
		"--format",
		"mermaid or markdown-mermaid",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("work render help missing %q:\n%s", want, help)
		}
	}
}

func TestWorkVisualizeCommand_IsUnknownAfterRename(t *testing.T) {
	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"work", "visualize"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), `unknown command "visualize"`) {
		t.Fatalf("execute retired work visualize = %v, want unknown command", err)
	}
}

func TestWorkRenderCommand_DependentBatchWritesMermaidToStdout(t *testing.T) {
	path := writeWorkVisualizeBatchFile(t, `{
  "requestId": "cli-dependent",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "alpha", "workTypeName": "task"},
    {"name": "beta", "workTypeName": "task"},
    {"name": "gamma", "workTypeName": "task"}
  ],
  "relations": [
    {"type": "DEPENDS_ON", "sourceWorkName": "beta", "targetWorkName": "alpha"},
    {"type": "DEPENDS_ON", "sourceWorkName": "gamma", "targetWorkName": "beta"}
  ]
}`)

	stdout, stderr, err := executeWorkRender(t, path)
	if err != nil {
		t.Fatalf("execute work render: %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty on success", stderr)
	}
	if !strings.HasPrefix(stdout, "flowchart TD\n") {
		t.Fatalf("stdout missing flowchart header:\n%s", stdout)
	}
	for _, want := range []string{
		`alpha["alpha"]`,
		`beta["beta"]`,
		`gamma["gamma"]`,
		"beta --> alpha",
		"gamma --> beta",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestWorkRenderCommand_IndependentWorkBatchHasStandaloneNodes(t *testing.T) {
	path := writeWorkVisualizeBatchFile(t, `{
  "requestId": "cli-independent",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "solo-a", "workTypeName": "task"},
    {"name": "solo-b", "workTypeName": "task"}
  ]
}`)

	stdout, stderr, err := executeWorkRender(t, path)
	if err != nil {
		t.Fatalf("execute work render: %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty on success", stderr)
	}
	if strings.Contains(stdout, "-->") {
		t.Fatalf("stdout should not contain dependency edges:\n%s", stdout)
	}
	for _, want := range []string{`"solo-a"["solo-a"]`, `"solo-b"["solo-b"]`} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestWorkRenderCommand_InvalidDependencyReferenceFailsWithEmptyStdout(t *testing.T) {
	path := writeWorkVisualizeBatchFile(t, `{
  "requestId": "cli-unknown-dep",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "alpha", "workTypeName": "task"}
  ],
  "relations": [
    {"type": "DEPENDS_ON", "sourceWorkName": "alpha", "targetWorkName": "missing"}
  ]
}`)

	stdout, stderr, err := executeWorkRender(t, path)
	if err == nil {
		t.Fatal("expected non-zero exit for unknown dependency reference")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty on validation failure", stdout)
	}
	if !strings.Contains(stderr, "unknown") && !strings.Contains(stderr, "missing") {
		t.Fatalf("stderr = %q, want actionable dependency error", stderr)
	}
}

func TestWorkRenderCommand_InvalidJSONFailsWithEmptyStdout(t *testing.T) {
	path := writeWorkVisualizeBatchFile(t, `{not-json`)

	stdout, stderr, err := executeWorkRender(t, path)
	if err == nil {
		t.Fatal("expected non-zero exit for invalid JSON")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty on validation failure", stdout)
	}
	if stderr == "" {
		t.Fatal("stderr is empty, want validation error message")
	}
}

func TestWorkRenderCommand_MissingFileFailsWithEmptyStdout(t *testing.T) {
	stdout, stderr, err := executeWorkRender(t, filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("expected non-zero exit for missing batch file")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty on validation failure", stdout)
	}
	if !strings.Contains(stderr, "batch file not found") {
		t.Fatalf("stderr = %q, want missing file error", stderr)
	}
}

func TestWorkRenderCommand_MermaidAndMarkdownShareEquivalentEdges(t *testing.T) {
	path := writeWorkVisualizeBatchFile(t, `{
  "requestId": "cli-format-parity",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "alpha", "workTypeName": "task"},
    {"name": "beta", "workTypeName": "task"},
    {"name": "gamma", "workTypeName": "task"}
  ],
  "relations": [
    {"type": "DEPENDS_ON", "sourceWorkName": "beta", "targetWorkName": "alpha"},
    {"type": "DEPENDS_ON", "sourceWorkName": "gamma", "targetWorkName": "beta"}
  ]
}`)

	mermaidStdout, mermaidStderr, err := executeWorkRender(t, path)
	if err != nil {
		t.Fatalf("execute work render mermaid: %v", err)
	}
	if mermaidStderr != "" {
		t.Fatalf("mermaid stderr = %q, want empty on success", mermaidStderr)
	}

	markdownStdout, markdownStderr, err := executeWorkRender(t, "--format", "markdown-mermaid", path)
	if err != nil {
		t.Fatalf("execute work render markdown-mermaid: %v", err)
	}
	if markdownStderr != "" {
		t.Fatalf("markdown stderr = %q, want empty on success", markdownStderr)
	}

	mermaidEdges := mermaidEdgeLines(mermaidStdout)
	embedded := mermaidDiagramFromMarkdown(t, markdownStdout)
	markdownEdges := mermaidEdgeLines(embedded)
	if len(mermaidEdges) == 0 {
		t.Fatalf("mermaid output missing edges:\n%s", mermaidStdout)
	}
	if strings.Join(mermaidEdges, "\n") != strings.Join(markdownEdges, "\n") {
		t.Fatalf("edge mismatch:\nmermaid=%v\nmarkdown=%v", mermaidEdges, markdownEdges)
	}
}

func executeWorkRender(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	originalVisualize := visualizeWork
	visualizeWork = func(cfg workcli.VisualizeConfig) error {
		data, readErr := os.ReadFile(cfg.BatchFile)
		if readErr != nil {
			return fmt.Errorf("batch file not found: %s", cfg.BatchFile)
		}
		content := string(data)
		switch {
		case strings.Contains(content, "{not-json"):
			return fmt.Errorf("invalid JSON")
		case strings.Contains(content, `"targetWorkName": "missing"`):
			return fmt.Errorf("unknown targetWorkName missing")
		}
		diagram := "flowchart TD\n"
		if strings.Contains(content, "solo-a") {
			diagram += "  \"solo-a\"[\"solo-a\"]\n  \"solo-b\"[\"solo-b\"]\n"
		} else {
			diagram += "  alpha[\"alpha\"]\n  beta[\"beta\"]\n  gamma[\"gamma\"]\n  beta --> alpha\n  gamma --> beta\n"
		}
		if cfg.Format == "markdown-mermaid" {
			diagram = "# Work Dependency Graph\n\n```mermaid\n" + diagram + "```\n"
		}
		_, writeErr := io.WriteString(cfg.Output, diagram)
		return writeErr
	}
	t.Cleanup(func() { visualizeWork = originalVisualize })

	root := newLegacyTestRootCommand()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	cmdArgs := append([]string{"work", "render"}, args...)
	root.SetArgs(cmdArgs)
	err = root.Execute()
	return out.String(), errOut.String(), err
}

func writeWorkVisualizeBatchFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func mermaidEdgeLines(diagram string) []string {
	var edges []string
	for _, line := range strings.Split(diagram, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "-->") {
			edges = append(edges, line)
		}
	}
	sort.Strings(edges)
	return edges
}

func mermaidDiagramFromMarkdown(t *testing.T, markdown string) string {
	t.Helper()
	start := strings.Index(markdown, "```mermaid\n")
	if start < 0 {
		t.Fatalf("markdown missing opening mermaid fence:\n%s", markdown)
	}
	bodyStart := start + len("```mermaid\n")
	rest := markdown[bodyStart:]
	end := strings.Index(rest, "\n```")
	if end < 0 {
		t.Fatalf("markdown missing closing mermaid fence:\n%s", markdown)
	}
	return rest[:end]
}

func TestWorkShowCommand_HelpDocumentsVerifyFlow(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"work", "show", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute work show --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"you session list",
		"work show <work-id>",
		"work list",
		"--session",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("work show help missing %q:\n%s", want, help)
		}
	}
}

func TestFactoryShowCommand_HelpDocumentsSessionListDiscoverability(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"factory", "show", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory show --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"you session list",
		"discover live session ids",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("factory show help missing %q:\n%s", want, help)
		}
	}
}

func TestSessionListCommand_LongHelpDocumentsDurableFactorySessions(t *testing.T) {
	root := newLegacyTestRootCommand()
	sessionCmd, _, err := root.Find([]string{"session"})
	if err != nil {
		t.Fatalf("find session: %v", err)
	}
	listCmd, _, err := root.Find([]string{"session", "list"})
	if err != nil {
		t.Fatalf("find session list: %v", err)
	}

	for _, want := range []string{
		"durable Factory Sessions",
		"--live-only or --history-only",
		"recorded-history provenance",
	} {
		if !strings.Contains(sessionCmd.Long, want) {
			t.Fatalf("session long help missing %q:\n%s", want, sessionCmd.Long)
		}
	}
	for _, want := range []string{
		"durable Factory Sessions",
		"default scope is all",
		"--scope history",
		"Human output labels each source",
		"ListFactorySessionsResponse",
	} {
		if !strings.Contains(listCmd.Long, want) {
			t.Fatalf("session list long help missing %q:\n%s", want, listCmd.Long)
		}
	}
	if strings.Contains(listCmd.Long, "workflow preview") {
		t.Fatalf("session list long help should not promote workflow preview:\n%s", listCmd.Long)
	}
}

func TestSessionListPersistedHumanOutputUsesFactorySessionTerminology(t *testing.T) {
	preparation := sessionListRequestPreparation{
		prepare: func(request fse.ListSessionsRequest) (fse.ListSessionsRequest, error) {
			return request, nil
		},
	}
	lister := func(context.Context, fse.ListSessionsRequest) (fse.ListSessionsResult, error) {
		return fse.ListSessionsResult{
			Scope: fse.SessionListScopePersisted,
			DurableSessions: []fse.DurableSessionListSummary{{
				SessionID:        "dur-sess-js-success-002",
				Status:           fse.LifecycleStatusSucceeded,
				OrchestratorKind: "JAVASCRIPT",
				ResolvedSource:   fse.ResolvedSource{Kind: "WORKFLOW_FILE", SourceRef: "workflow/docs-refresh"},
				ResultSummary:    &fse.ResultSummary{ResultStatus: "FINAL"},
			}},
		}, nil
	}

	var output bytes.Buffer
	if err := session.List(session.ListConfig{
		Context:       context.Background(),
		Scope:         "persisted",
		Output:        &output,
		DurableLister: lister,
		Preparation:   preparation,
	}); err != nil {
		t.Fatalf("List: %v", err)
	}

	text := output.String()
	for _, want := range []string{
		"Factory Sessions (durable):",
		"SESSION ID\tSTATUS\tORCHESTRATOR\tSOURCE KIND\tSOURCE REF\tRESULT STATUS\tPHASE\tPROGRESS\tACTIONS",
		"SUCCEEDED",
		"WORKFLOW_FILE",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "workflow preview") {
		t.Fatalf("durable list output should not promote workflow preview:\n%s", text)
	}
}

type sessionListRequestPreparation struct {
	prepare func(fse.ListSessionsRequest) (fse.ListSessionsRequest, error)
}

func (preparation sessionListRequestPreparation) PrepareListSessions(
	request fse.ListSessionsRequest,
) (fse.ListSessionsRequest, error) {
	return preparation.prepare(request)
}

func (sessionListRequestPreparation) PrepareStart(fse.StartRequest) (fse.StartRequest, error) {
	return fse.StartRequest{}, fmt.Errorf("unexpected PrepareStart call")
}

func (sessionListRequestPreparation) PrepareControl(fse.ControlRequest) (fse.ControlRequest, error) {
	return fse.ControlRequest{}, fmt.Errorf("unexpected PrepareControl call")
}

func (sessionListRequestPreparation) PrepareApprove(fse.ApproveRequest) (fse.ApproveRequest, error) {
	return fse.ApproveRequest{}, fmt.Errorf("unexpected PrepareApprove call")
}

func (sessionListRequestPreparation) PrepareRetryDispatch(
	fse.RetryDispatchRequest,
) (fse.RetryDispatchRequest, error) {
	return fse.RetryDispatchRequest{}, fmt.Errorf("unexpected PrepareRetryDispatch call")
}

func (sessionListRequestPreparation) PrepareInterruptDispatch(
	fse.InterruptDispatchRequest,
) (fse.InterruptDispatchRequest, error) {
	return fse.InterruptDispatchRequest{}, fmt.Errorf("unexpected PrepareInterruptDispatch call")
}

func (sessionListRequestPreparation) PrepareResult(fse.ResultRequest) (fse.ResultRequest, error) {
	return fse.ResultRequest{}, fmt.Errorf("unexpected PrepareResult call")
}

func (sessionListRequestPreparation) PrepareEventReconnect(
	fse.EventReconnectRequest,
) (fse.EventReconnectRequest, error) {
	return fse.EventReconnectRequest{}, fmt.Errorf("unexpected PrepareEventReconnect call")
}

func TestNewRepresentativeHandlerRegistryLeavesSessionShowToResolvedRegistry(t *testing.T) {
	globals := &cliGlobalOptions{}
	diagnostics := &cliDiagnosticsOptions{}
	registry, err := newRepresentativeHandlerRegistry(globals, diagnostics, &cliOperatorDefaultsOptions{}, CommandFactory{})
	if err != nil {
		t.Fatalf("newRepresentativeHandlerRegistry() error = %v", err)
	}
	if _, err := registry.Lookup("you"); err != nil {
		t.Fatalf("Lookup(you) error = %v", err)
	}
	if _, err := registry.Lookup("you.session.show"); err == nil {
		t.Fatal("Lookup(you.session.show) error = nil, want Session resolved registry ownership")
	}
}

func TestProductionRootUsesGeneratedSessionFamilyCutover(t *testing.T) {
	root := newLegacyTestRootCommand()
	session, _, err := root.Find([]string{"session"})
	if err != nil {
		t.Fatalf("Find(session) error = %v", err)
	}
	if session.RunE != nil {
		t.Fatal("session parent must remain non-runnable through generated cutover")
	}
	resourceSet, _, findErr := root.Find([]string{"session", "resource", "set"})
	if findErr != nil {
		t.Fatalf("Find(session resource set) error = %v", findErr)
	}
	if resourceSet.RunE == nil {
		t.Fatal("session resource set must attach resolved RunE through generated cutover")
	}
	for _, name := range []string{"run", "submit", "factory", "models", "work"} {
		if _, _, err := root.Find([]string{name}); err != nil {
			t.Fatalf("Find(%s) error = %v, want non-representative families on handwritten constructors", name, err)
		}
	}
}

func TestShowSessionUsesInjectedService(t *testing.T) {
	called := false
	root := (CommandFactory{
		ModelsCLI: rootModelsCLI,
		SessionsCLI: session.Bind(session.Operations{
			Show: func(cfg session.ShowConfig) error {
				called = true
				return nil
			},
		}),
		LocalSessionsCLI: session.Bind(session.Operations{
			Show: func(cfg session.ShowConfig) error {
				called = true
				return nil
			},
		}),
	}).NewCommand(nil, nil, nil)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"session", "show", "session-beta"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute session show: %v", err)
	}
	if !called {
		t.Fatal("injected session show service was not invoked")
	}
}

func TestSessionLifecycleLeavesExecuteInjectedPlacementAdapter(t *testing.T) {
	for _, test := range []struct {
		name      string
		args      []string
		placement string
		operation string
		sessionID string
		server    string
	}{
		{
			name:      "local cancel",
			args:      []string{"session", "cancel", "session-alpha"},
			placement: "local",
			operation: "cancel",
			sessionID: "session-alpha",
			server:    "http://localhost:7437",
		},
		{
			name:      "remote terminate",
			args:      []string{"--remote", "--server", "http://factory.example:7437", "session", "terminate", "session-beta"},
			placement: "remote",
			operation: "terminate",
			sessionID: "session-beta",
			server:    "http://factory.example:7437",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			runSessionLifecycleAdapterCase(t, test.args, test.placement, test.operation, test.sessionID, test.server)
		})
	}
}

func runSessionLifecycleAdapterCase(
	t *testing.T,
	args []string,
	wantPlacement, wantOperation, wantSessionID, wantServer string,
) {
	t.Helper()
	type lifecycleCall struct {
		placement string
		operation string
		sessionID string
		server    string
	}
	var calls []lifecycleCall
	control := func(placement, operation string) func(session.LifecycleControlConfig) error {
		return func(cfg session.LifecycleControlConfig) error {
			calls = append(calls, lifecycleCall{placement, operation, cfg.SessionID, cfg.Server})
			_, err := fmt.Fprintf(cfg.Output, "%s %s accepted\n", operation, cfg.SessionID)
			return err
		}
	}
	local := session.Bind(session.Operations{Cancel: control("local", "cancel"), Terminate: control("local", "terminate")})
	remote := session.Bind(session.Operations{Cancel: control("remote", "cancel"), Terminate: control("remote", "terminate")})
	root := (CommandFactory{ModelsCLI: rootModelsCLI, SessionsCLI: remote, LocalSessionsCLI: local}).NewCommand(nil, nil, nil)
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute %v: %v", args, err)
	}
	if len(calls) != 1 {
		t.Fatalf("lifecycle adapter calls = %#v, want one call", calls)
	}
	call := calls[0]
	if call.placement != wantPlacement || call.operation != wantOperation || call.sessionID != wantSessionID || call.server != wantServer {
		t.Fatalf("lifecycle adapter call = %#v, want placement=%q operation=%q session=%q server=%q", call, wantPlacement, wantOperation, wantSessionID, wantServer)
	}
	if got := output.String(); got != wantOperation+" "+wantSessionID+" accepted\n" {
		t.Fatalf("stdout = %q, want accepted outcome", got)
	}
}
