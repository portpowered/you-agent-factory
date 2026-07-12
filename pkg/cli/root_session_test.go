package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/cli/session"
	"github.com/portpowered/infinite-you/pkg/cli/workflow"
	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestSessionCommand_RegistersSubcommands(t *testing.T) {
	root := NewRootCommand()
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
	root := NewRootCommand()
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
	root := NewRootCommand()
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

	root := NewRootCommand()
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

	root := NewRootCommand()
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

	root := NewRootCommand()
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
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"session", "dispatches", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute session dispatches --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"dispatches [session-id]",
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

	root := NewRootCommand()
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

	root := NewRootCommand()
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
	root := NewRootCommand()
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
	root := NewRootCommand()
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
	root := NewRootCommand()
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
	root := NewRootCommand()
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
	root := NewRootCommand()
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

	root := NewRootCommand()
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
	root := NewRootCommand()
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
	root := NewRootCommand()
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

func TestWorkflowCommand_LongHelpPresentsCanonicalFactorySessionSuccessorsFirst(t *testing.T) {
	root := NewRootCommand()
	workflowCmd, _, err := root.Find([]string{"workflow"})
	if err != nil {
		t.Fatalf("find workflow: %v", err)
	}
	previewCmd, _, err := root.Find([]string{"workflow", "preview"})
	if err != nil {
		t.Fatalf("find workflow preview: %v", err)
	}
	validateCmd, _, err := root.Find([]string{"workflow", "validate"})
	if err != nil {
		t.Fatalf("find workflow validate: %v", err)
	}

	for _, want := range []string{
		"Compatibility-only workflow spellings",
		"POST /factories/preview",
		"POST /factory-sessions/{sync|async}",
		"canonical you session commands",
		"run        compatibility sync start; successor: POST /factory-sessions/sync",
		"status     compatibility read; successor: GET /factory-sessions/{session_id}",
	} {
		if !strings.Contains(workflowCmd.Long, want) {
			t.Fatalf("workflow long help missing %q:\n%s", want, workflowCmd.Long)
		}
	}
	if !strings.Contains(previewCmd.Long, "Compatibility command for the Factory preview contract") {
		t.Fatalf("preview long help missing compatibility wording:\n%s", previewCmd.Long)
	}
	if !strings.Contains(previewCmd.Long, "workflow validate") {
		t.Fatalf("preview long help should steer to validate:\n%s", previewCmd.Long)
	}
	if !strings.Contains(validateCmd.Long, "shared workflow validation contract") {
		t.Fatalf("validate long help should preserve its validation behavior:\n%s", validateCmd.Long)
	}
}

func TestSessionListCommand_LongHelpDocumentsDurableFactorySessions(t *testing.T) {
	root := NewRootCommand()
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

func TestWorkflowValidateHumanOutputUsesFactorySessionTerminology(t *testing.T) {
	projectRoot := t.TempDir()
	writeTerminologyWorkflow(t, projectRoot, "review.js", validTerminologyWorkflowSource)

	var output bytes.Buffer
	if err := workflow.Validate(workflow.ValidateConfig{
		SourceConfig: workflow.SourceConfig{
			Dir:         projectRoot,
			SourceKind:  string(workflowsource.KindWorkflowName),
			SourceValue: "review",
		},
		Output: &output,
	}); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	text := output.String()
	for _, want := range []string{"Workflow validation passed.", "Source ref:", "Source hash:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
	assertForbiddenWorkflowRunVocabulary(t, text)
}

func TestWorkflowPreviewHumanOutputRemainsCompatibilityOnly(t *testing.T) {
	projectRoot := t.TempDir()
	writeTerminologyWorkflow(t, projectRoot, "review.js", validTerminologyWorkflowSource)

	var output bytes.Buffer
	if err := workflow.Preview(workflow.PreviewConfig{
		SourceConfig: workflow.SourceConfig{
			Dir:         projectRoot,
			SourceKind:  string(workflowsource.KindWorkflowName),
			SourceValue: "review",
		},
		Output: &output,
	}); err != nil {
		t.Fatalf("Preview: %v", err)
	}

	text := output.String()
	for _, want := range []string{"Factory preview passed.", "Policy hash:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Workflow validation passed.") {
		t.Fatalf("preview output should keep Factory preview compatibility wording:\n%s", text)
	}
	assertForbiddenWorkflowRunVocabulary(t, text)
}

func TestSessionListPersistedHumanOutputUsesFactorySessionTerminology(t *testing.T) {
	service, err := fse.NewFakeServiceFromContractFixtures(contractFixtureCatalogPathForTerminology(t))
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}

	var output bytes.Buffer
	if err := session.List(session.ListConfig{
		Scope:         "persisted",
		Output:        &output,
		DurableLister: service.ListSessions,
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
	assertForbiddenWorkflowRunVocabulary(t, text)
}

func TestWorkflowSessionCLI_NewBehaviorSurfacesCovered(t *testing.T) {
	cases := []struct {
		name string
		path []string
	}{
		{name: "workflow validate human", path: []string{"workflow", "validate"}},
		{name: "workflow preview compatibility", path: []string{"workflow", "preview"}},
		{name: "session list durable", path: []string{"session", "list"}},
	}
	root := NewRootCommand()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd, _, err := root.Find(tc.path)
			if err != nil {
				t.Fatalf("find %v: %v", tc.path, err)
			}
			if strings.TrimSpace(cmd.Short) == "" || strings.TrimSpace(cmd.Long) == "" {
				t.Fatalf("command %v missing short/long help", tc.path)
			}
		})
	}

	for _, term := range fixtures.ForbiddenFixtureVocabularyTerms() {
		if strings.Contains(strings.ToLower("Factory Sessions (durable)"), strings.ToLower(term)) {
			t.Fatalf("forbidden vocabulary helper collides with canonical wording: %q", term)
		}
	}
}

const validTerminologyWorkflowSource = `
meta({ name: "review", version: 1 });
phase("setup");
log("starting");
`

func contractFixtureCatalogPathForTerminology(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "api", "testdata", "durable-session-contract-fixtures.json")
}

func writeTerminologyWorkflow(t *testing.T, projectRoot, name, content string) {
	t.Helper()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
}

func assertForbiddenWorkflowRunVocabulary(t *testing.T, text string) {
	t.Helper()
	lower := strings.ToLower(text)
	for _, term := range fixtures.ForbiddenFixtureVocabularyTerms() {
		if strings.Contains(lower, strings.ToLower(term)) {
			t.Fatalf("output introduced forbidden vocabulary %q:\n%s", term, text)
		}
	}
}
