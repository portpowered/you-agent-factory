package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
)

func TestSessionCommand_RegistersSubcommands(t *testing.T) {
	root := newLegacyTestRootCommand()
	for _, path := range [][]string{
		{"session", "list"},
		{"session", "show"},
		{"session", "dispatches"},
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
		"dispatches",
		"pause",
		"resume",
		"create",
		"delete",
		"you session list",
		"you session show",
		"you session pause",
		"you session resume",
		"you session list --json",
		"you session create --dir /workspace/fleet --port 9090",
		"you session delete session-beta --port 9090 --json",
		"same default --port as work list",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("session help missing %q:\n%s", want, help)
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

func TestSessionDispatchesCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalListSessionDispatches := listSessionDispatches
	defer func() {
		listSessionDispatches = originalListSessionDispatches
	}()

	var got session.DispatchesConfig
	listSessionDispatches = func(cfg session.DispatchesConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "--server", "http://127.0.0.1:9090", "session", "dispatches", "dur-sess-js-run-n-001"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute session dispatches with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to DispatchesConfig.JSON")
	}
	if got.Server != "http://127.0.0.1:9090" {
		t.Fatalf("server = %q, want http://127.0.0.1:9090", got.Server)
	}
	if got.SessionID != "dur-sess-js-run-n-001" {
		t.Fatalf("sessionId = %q, want dur-sess-js-run-n-001", got.SessionID)
	}
}

func TestSessionDispatchesCommand_HelpDocumentsDurableInspection(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"session", "dispatches", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute session dispatches --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"dispatches <session-id>",
		"dur-sess-",
		"FactorySession",
		"Dispatch",
		"FactoryArtifact",
		"ListFactorySessionDispatchesResponse",
		"you session dispatches",
		"--json",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("session dispatches help missing %q:\n%s", want, help)
		}
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

func TestWorkVisualizeCommand_HelpDocumentsReadOnlyFormatsAndRedirection(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"work", "visualize", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute work visualize --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"visualize <batch-file.json>",
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
			t.Fatalf("work visualize help missing %q:\n%s", want, help)
		}
	}
}

func TestWorkVisualizeCommand_DependentBatchWritesMermaidToStdout(t *testing.T) {
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

	stdout, stderr, err := executeWorkVisualize(t, path)
	if err != nil {
		t.Fatalf("execute work visualize: %v", err)
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

func TestWorkVisualizeCommand_IndependentWorkBatchHasStandaloneNodes(t *testing.T) {
	path := writeWorkVisualizeBatchFile(t, `{
  "requestId": "cli-independent",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "solo-a", "workTypeName": "task"},
    {"name": "solo-b", "workTypeName": "task"}
  ]
}`)

	stdout, stderr, err := executeWorkVisualize(t, path)
	if err != nil {
		t.Fatalf("execute work visualize: %v", err)
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

func TestWorkVisualizeCommand_InvalidDependencyReferenceFailsWithEmptyStdout(t *testing.T) {
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

	stdout, stderr, err := executeWorkVisualize(t, path)
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

func TestWorkVisualizeCommand_InvalidJSONFailsWithEmptyStdout(t *testing.T) {
	path := writeWorkVisualizeBatchFile(t, `{not-json`)

	stdout, stderr, err := executeWorkVisualize(t, path)
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

func TestWorkVisualizeCommand_MissingFileFailsWithEmptyStdout(t *testing.T) {
	stdout, stderr, err := executeWorkVisualize(t, filepath.Join(t.TempDir(), "missing.json"))
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

func TestWorkVisualizeCommand_MermaidAndMarkdownShareEquivalentEdges(t *testing.T) {
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

	mermaidStdout, mermaidStderr, err := executeWorkVisualize(t, path)
	if err != nil {
		t.Fatalf("execute work visualize mermaid: %v", err)
	}
	if mermaidStderr != "" {
		t.Fatalf("mermaid stderr = %q, want empty on success", mermaidStderr)
	}

	markdownStdout, markdownStderr, err := executeWorkVisualize(t, "--format", "markdown-mermaid", path)
	if err != nil {
		t.Fatalf("execute work visualize markdown-mermaid: %v", err)
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

func executeWorkVisualize(t *testing.T, args ...string) (stdout, stderr string, err error) {
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
	cmdArgs := append([]string{"work", "visualize"}, args...)
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

func TestFactoryQueryCommand_HelpDocumentsSessionListDiscoverability(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"factory", "query", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory query --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"you session list",
		"discover live session ids",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("factory query help missing %q:\n%s", want, help)
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
		"--scope live|persisted|all",
		"source identity",
		"result availability",
		"action availability",
	} {
		if !strings.Contains(sessionCmd.Long, want) {
			t.Fatalf("session long help missing %q:\n%s", want, sessionCmd.Long)
		}
	}
	for _, want := range []string{
		"durable Factory Sessions",
		"Factory Session table",
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
	if len(session.Commands()) != 7 {
		t.Fatalf("session child count = %d, want exactly 7 generated leaves", len(session.Commands()))
	}
	for _, name := range []string{"create", "list", "show", "delete", "pause", "resume", "dispatches"} {
		command, _, findErr := root.Find([]string{"session", name})
		if findErr != nil {
			t.Fatalf("Find(session %s) error = %v", name, findErr)
		}
		if command.RunE == nil {
			t.Fatalf("session %s must attach resolved RunE through generated cutover", name)
		}
	}
	for _, name := range []string{"run", "submit", "factory", "models", "work"} {
		if _, _, err := root.Find([]string{name}); err != nil {
			t.Fatalf("Find(%s) error = %v, want non-representative families on handwritten constructors", name, err)
		}
	}
}

func TestSessionCommandCompositionUsesTypedSessionsCLIAdapter(t *testing.T) {
	t.Parallel()

	called := false
	factory := NewCommandFactory(CommandOperations{
		ShowSession: func(cfg session.ShowConfig) error {
			called = true
			if cfg.SessionID != "session-beta" {
				t.Fatalf("SessionID = %q, want session-beta", cfg.SessionID)
			}
			return nil
		},
	})
	if factory.SessionsCLI == nil {
		t.Fatal("SessionsCLI adapter is missing from composed factory")
	}

	root := factory.NewCommand(nil, nil, nil)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"session", "show", "session-beta"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute session show: %v", err)
	}
	if !called {
		t.Fatal("typed Sessions adapter was not invoked through production composition")
	}
}

func TestSessionDataCommandsPreserveBehaviorThroughProductionComposition(t *testing.T) {
	t.Parallel()

	operationFailure := errors.New("session operation failed")
	tests := []struct {
		name       string
		args       []string
		wantError  error
		operations func(*testing.T, error) CommandOperations
	}{
		{
			name: "create",
			args: []string{
				"--verbose", "--json", "--server", "https://factory.example",
				"session", "create", "--dir", "/workspace/fleet",
				"--validate-only", "--target-kind", "named", "--target-name", "alpha",
			},
			wantError: operationFailure,
			operations: func(t *testing.T, result error) CommandOperations {
				return CommandOperations{CreateSession: func(cfg session.CreateConfig) error {
					if cfg.Server != "https://factory.example" || cfg.Dir != "/workspace/fleet" ||
						!cfg.ValidateOnly || cfg.TargetKind != "named" || cfg.TargetName != "alpha" ||
						!cfg.JSON || !cfg.Verbose {
						t.Fatalf("create config = %#v", cfg)
					}
					return writeSessionCompositionOutput(cfg.Output, cfg.Diagnostics, result)
				}}
			},
		},
		{
			name:      "delete",
			args:      []string{"--verbose", "--json", "session", "delete", "session-beta", "--port", "9444"},
			wantError: operationFailure,
			operations: func(t *testing.T, result error) CommandOperations {
				return CommandOperations{DeleteSession: func(cfg session.DeleteConfig) error {
					if cfg.SessionID != "session-beta" || cfg.Port != 9444 || !cfg.JSON || !cfg.Verbose {
						t.Fatalf("delete config = %#v", cfg)
					}
					return writeSessionCompositionOutput(cfg.Output, cfg.Diagnostics, result)
				}}
			},
		},
		{
			name: "list",
			args: []string{
				"--verbose", "--json", "--server", "https://factory.example",
				"session", "list", "--scope", "live",
			},
			wantError: operationFailure,
			operations: func(t *testing.T, result error) CommandOperations {
				return CommandOperations{ListSessions: func(cfg session.ListConfig) error {
					if cfg.Context == nil || cfg.Server != "https://factory.example" ||
						cfg.Scope != "live" || !cfg.JSON || !cfg.Verbose {
						t.Fatalf("list config = %#v", cfg)
					}
					return writeSessionCompositionOutput(cfg.Output, cfg.Diagnostics, result)
				}}
			},
		},
		{
			name:      "show",
			args:      []string{"--verbose", "--json", "--server", "https://factory.example", "session", "show", "session-beta"},
			wantError: context.Canceled,
			operations: func(t *testing.T, result error) CommandOperations {
				return CommandOperations{ShowSession: func(cfg session.ShowConfig) error {
					if cfg.Context == nil || cfg.Server != "https://factory.example" ||
						cfg.SessionID != "session-beta" || !cfg.JSON || !cfg.Verbose {
						t.Fatalf("show config = %#v", cfg)
					}
					return writeSessionCompositionOutput(cfg.Output, cfg.Diagnostics, result)
				}}
			},
		},
		{
			name: "dispatches",
			args: []string{
				"--verbose", "--json", "--server", "https://factory.example",
				"session", "dispatches", "dur-sess-review-001",
				"--phase", "review", "--status", "COMPLETED",
			},
			wantError: operationFailure,
			operations: func(t *testing.T, result error) CommandOperations {
				return CommandOperations{ListSessionDispatches: func(cfg session.DispatchesConfig) error {
					if cfg.Context == nil || cfg.Server != "https://factory.example" ||
						cfg.SessionID != "dur-sess-review-001" || cfg.Phase != "review" ||
						cfg.Status != "COMPLETED" || !cfg.JSON || !cfg.Verbose {
						t.Fatalf("dispatches config = %#v", cfg)
					}
					return writeSessionCompositionOutput(cfg.Output, cfg.Diagnostics, result)
				}}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name+" success", func(t *testing.T) {
			stdout, stderr, err := executeSessionComposition(t, test.operations(t, nil), test.args)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if stdout != "session-ok\n" || stderr != "session-diagnostic\n" {
				t.Fatalf("stdout = %q, stderr = %q", stdout, stderr)
			}
		})
		t.Run(test.name+" failure", func(t *testing.T) {
			stdout, stderr, err := executeSessionComposition(t, test.operations(t, test.wantError), test.args)
			if !errors.Is(err, test.wantError) {
				t.Fatalf("Execute() error = %v, want %v", err, test.wantError)
			}
			if stdout != "" || stderr != fmt.Sprintf("Error: %v\n", test.wantError) {
				t.Fatalf("failure stdout = %q, stderr = %q", stdout, stderr)
			}
		})
	}
}

func executeSessionComposition(
	t *testing.T,
	operations CommandOperations,
	args []string,
) (string, string, error) {
	t.Helper()
	factory := NewCommandFactory(operations)
	if factory.SessionsCLI == nil {
		t.Fatal("SessionsCLI adapter is missing from production composition")
	}
	root := factory.NewCommand(nil, nil, nil)
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

func writeSessionCompositionOutput(output, diagnostics io.Writer, result error) error {
	if result != nil {
		return result
	}
	if _, err := fmt.Fprintln(output, "session-ok"); err != nil {
		return err
	}
	_, err := fmt.Fprintln(diagnostics, "session-diagnostic")
	return err
}

func TestShowSessionUsesInjectedService(t *testing.T) {
	called := false
	root := (CommandFactory{
		ModelsCLI: legacyModelsCLIService{},
		SessionsCLI: session.Bind(session.Operations{
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
