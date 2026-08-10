package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
)

func TestRunCommand_VerboseFlag(t *testing.T) {
	root := newLegacyTestRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	vFlag := runCmd.Flag("verbose")
	if vFlag == nil {
		t.Fatal("expected --verbose flag on run command")
	}
	if vFlag.DefValue != "false" {
		t.Errorf("default verbose = %q, want %q", vFlag.DefValue, "false")
	}
	if vFlag.Shorthand != "v" {
		t.Errorf("verbose shorthand = %q, want %q", vFlag.Shorthand, "v")
	}
}

func TestRootCommand_SharedDiagnosticsFlagsAvailableOnCoveredCommands(t *testing.T) {
	root := newLegacyTestRootCommand()
	commands := [][]string{
		{},
		{"run"},
		{"submit"},
		{"work", "list"},
		{"factory", "query"},
		{"factory", "list"},
		{"factory", "create"},
		{"factory", "replace-current"},
		{"factory", "update", "staging"},
		{"factory", "delete", "staging"},
		{"models", "list"},
		{"models", "inspect"},
		{"models", "invoke"},
		{"models", "pull"},
		{"factory", "config", "flatten"},
		{"factory", "config", "expand"},
		{"factory", "config", "validate"},
		{"init"},
		{"docs", "config"},
	}

	for _, path := range commands {
		cmd := root
		if len(path) > 0 {
			found, _, err := root.Find(path)
			if err != nil {
				t.Fatalf("find %v: %v", path, err)
			}
			cmd = found
		}
		for name, shorthand := range map[string]string{"verbose": "v", "debug": "d"} {
			flag := cmd.Flag(name)
			if flag == nil {
				t.Fatalf("%v missing shared --%s flag", path, name)
			}
			if flag.DefValue != "false" {
				t.Fatalf("%v --%s default = %q, want false", path, name, flag.DefValue)
			}
			if flag.Shorthand != shorthand {
				t.Fatalf("%v --%s shorthand = %q, want %q", path, name, flag.Shorthand, shorthand)
			}
		}
	}
}

func TestRunCommand_RecordFlagsDocumentDefaultRecordingBehavior(t *testing.T) {
	root := newLegacyTestRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	recordFlag := runCmd.Flags().Lookup("record")
	if recordFlag == nil {
		t.Fatal("expected --record flag on run command")
	}
	if !strings.Contains(recordFlag.Usage, "default live runs record automatically unless --no-record is used") {
		t.Fatalf("--record usage = %q, want default-recording guidance", recordFlag.Usage)
	}
	if !strings.Contains(recordFlag.Usage, "replay artifacts are sensitive") {
		t.Fatalf("--record usage = %q, want sensitivity guidance", recordFlag.Usage)
	}

	noRecordFlag := runCmd.Flags().Lookup("no-record")
	if noRecordFlag == nil {
		t.Fatal("expected --no-record flag on run command")
	}
	if noRecordFlag.DefValue != "false" {
		t.Fatalf("--no-record default = %q, want false", noRecordFlag.DefValue)
	}
	if !strings.Contains(noRecordFlag.Usage, "disable the default replay artifact for this invocation") {
		t.Fatalf("--no-record usage = %q", noRecordFlag.Usage)
	}
	if !strings.Contains(runCmd.Long, "Normal live runs record by default unless you pass --no-record.") {
		t.Fatal("expected run command long help text to document default recording")
	}
	if !strings.Contains(runCmd.Long, "Replay artifacts are sensitive and can contain prompts, payloads, stdout, stderr, and diagnostic metadata.") {
		t.Fatal("expected run command long help text to document replay artifact sensitivity")
	}

	replayFlag := runCmd.Flags().Lookup("replay")
	if replayFlag == nil {
		t.Fatal("expected --replay flag on run command")
	}
	if !strings.Contains(replayFlag.Usage, "existing sensitive replay artifact") {
		t.Fatalf("--replay usage = %q, want sensitivity guidance", replayFlag.Usage)
	}
}

func TestRunCommand_FactoryPromptRejectsEmptyStdinWithStableCode(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := newLegacyTestRootCommandWithInvocationInput(programmedInvocationInput(
		work.PreparedInvocationInput{},
		&work.InputError{
			Code:    work.InputErrorCodeEmpty,
			Message: "invocation stdin input is empty",
			Source:  work.InputSourceStdinText,
		},
	))
	root.SetIn(strings.NewReader(""))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--factory", factoryPath, "-"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected empty stdin rejection")
	}
	if !strings.Contains(err.Error(), "INVOCATION_INPUT_EMPTY") {
		t.Fatalf("error = %q, want stable empty stdin code", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start for empty factory stdin")
	}
}

func assertRunStdoutFreeOfOperatorChatter(t *testing.T, stdout string) {
	t.Helper()
	for _, forbidden := range []string{
		"Factory initiated",
		"Dashboard URL",
		"Runtime log",
		"Opening dashboard",
		"Factory:",
		"Recording saved",
	} {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("stdout = %q, want no %q chatter", stdout, forbidden)
		}
	}
}

func TestRunCommand_FactoryPromptRejectsAmbiguousPositionalAndStdin(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := newLegacyTestRootCommandWithInvocationInput(programmedInvocationInput(
		work.PreparedInvocationInput{},
		&work.InputError{
			Code:               work.InputErrorCodeSourceConflict,
			Message:            "invocation input sources conflict: positional_text, stdin_text",
			ConflictingSources: []work.InputSourceLabel{work.InputSourcePositionalText, work.InputSourceStdinText},
		},
	))
	root.SetIn(strings.NewReader("Fix from stdin\n"))
	root.SetOut(io.Discard)
	var stderr bytes.Buffer
	root.SetErr(&stderr)
	root.SetArgs([]string{"run", "--factory", factoryPath, "--quiet", "Fix from args", "-"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected ambiguous positional and stdin prompt rejection")
	}
	for _, want := range []string{"INVOCATION_INPUT_SOURCE_CONFLICT", "positional_text", "stdin_text"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	}
	for _, want := range []string{"INVOCATION_INPUT_SOURCE_CONFLICT", "positional_text", "stdin_text"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want %q", stderr.String(), want)
		}
	}
	if runCalled {
		t.Fatal("run should not start for ambiguous factory prompt input")
	}
}

func TestRunCommand_FactoryPromptRejectsWorkFlagConflict(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := newLegacyTestRootCommandWithInvocationInput(programmedTextInvocationInput(
		work.InputSourcePositionalText,
		"Fix the lint issues",
	))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--factory", factoryPath, "--work", "work.json", "Fix the lint issues"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected conflict between positional prompt and --work")
	}
	if !strings.Contains(err.Error(), "cannot be used with --work") {
		t.Fatalf("error = %q, want --work conflict message", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start when prompt conflicts with --work")
	}
}

func TestRunCommand_PositionalPromptRequiresFactoryFlag(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := newLegacyTestRootCommandWithInvocationInput(programmedTextInvocationInput(
		work.InputSourcePositionalText,
		"Fix the lint issues",
	))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--dir", "factory", "Fix the lint issues"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected positional prompt without --factory to fail")
	}
	if !strings.Contains(err.Error(), "require --factory") {
		t.Fatalf("error = %q, want --factory requirement", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start for positional prompt without --factory")
	}
}

func TestRunCommand_CleanInvocationFailureWritesSingleErrorResponseToStderr(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	var stdout, stderr bytes.Buffer
	runCLI = func(context.Context, runcli.RunConfig) error {
		return &runcli.InvocationError{
			Code:    runcli.InvocationErrorCodeFailed,
			Message: "clean invocation failed: mock worker rejected",
		}
	}

	root := newLegacyTestRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"run", "--factory", factoryPath, "Fix the lint issues"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected clean invocation failure")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	var payload struct {
		Code    string `json:"code"`
		Family  string `json:"family"`
		Message string `json:"message"`
	}
	if decodeErr := json.Unmarshal(stderr.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("stderr is not one ErrorResponse: %v\n%s", decodeErr, stderr.String())
	}
	if payload.Code != runcli.InvocationErrorCodeFailed || payload.Family != "INTERNAL_SERVER_ERROR" ||
		payload.Message != "clean invocation failed: mock worker rejected" {
		t.Fatalf("ErrorResponse = %#v", payload)
	}
}

func TestRunCommand_CleanInvocationJSONFailureWritesSingleErrorObjectToStderr(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	var stdout, stderr bytes.Buffer
	runCLI = func(context.Context, runcli.RunConfig) error {
		return &runcli.InvocationError{
			Code:    runcli.InvocationErrorCodeTimeout,
			Message: "clean invocation timed out",
		}
	}

	root := newLegacyTestRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--json", "run", "--factory", factoryPath, "Fix the lint issues"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected clean invocation timeout")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}

	var payload map[string]string
	if decodeErr := json.Unmarshal(stderr.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("stderr is not one JSON object: %v\n%s", decodeErr, stderr.String())
	}
	if payload["code"] != runcli.InvocationErrorCodeTimeout {
		t.Fatalf("code = %q, want %q", payload["code"], runcli.InvocationErrorCodeTimeout)
	}
	if payload["message"] != "clean invocation timed out" {
		t.Fatalf("message = %q", payload["message"])
	}
	if payload["family"] != "INTERNAL_SERVER_ERROR" {
		t.Fatalf("family = %q", payload["family"])
	}
}
