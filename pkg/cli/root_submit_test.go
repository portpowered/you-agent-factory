package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"

	submitcli "github.com/portpowered/infinite-you/pkg/cli/submit"
)

func TestSubmitCommand_HelpAdvertisesRequiredFlags(t *testing.T) {
	root := NewRootCommand()
	submitCmd, _, err := root.Find([]string{"submit"})
	if err != nil {
		t.Fatalf("find submit: %v", err)
	}

	for _, name := range []string{"name", "work-type-name", "payload"} {
		f := submitCmd.Flags().Lookup(name)
		if f == nil {
			t.Errorf("expected --%s flag on submit command", name)
			continue
		}
	}
	for _, name := range []string{"factory", "factory-id", "work-type-id"} {
		if f := submitCmd.Flags().Lookup(name); f != nil {
			t.Fatalf("submit command should not expose --%s", name)
		}
	}

	var out bytes.Buffer
	root = NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"submit", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute submit --help: %v", err)
	}

	help := out.String()
	if !strings.Contains(help, "--name") {
		t.Fatalf("submit help should list --name:\n%s", help)
	}
	if !strings.Contains(help, "authored request name") {
		t.Fatalf("submit help should describe --name:\n%s", help)
	}
	if !strings.Contains(help, "--work-type-name") {
		t.Fatalf("submit help should list --work-type-name:\n%s", help)
	}
	if !strings.Contains(help, "work type name to submit to") {
		t.Fatalf("submit help should describe work type names:\n%s", help)
	}
	for _, disallowed := range []string{"--work-type-id", "--factory-id", "--factory"} {
		if strings.Contains(help, disallowed) {
			t.Fatalf("submit help should not list %s:\n%s", disallowed, help)
		}
	}
}

func TestSubmitCommand_WorkTypeIDFlagIsRejected(t *testing.T) {
	originalSubmitWork := submitWork
	defer func() {
		submitWork = originalSubmitWork
	}()

	called := false
	submitWork = func(cfg submitcli.SubmitConfig) error {
		called = true
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"submit",
		"--work-type-id", "legacy-task",
		"--payload", "request.md",
	})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected removed --work-type-id flag to fail")
	}
	if !strings.Contains(err.Error(), "unknown flag: --work-type-id") {
		t.Fatalf("removed flag error = %q, want unknown flag", err.Error())
	}
	if called {
		t.Fatal("submit command should not run when --work-type-id is supplied")
	}
}

func TestSubmitCommand_MissingWorkTypeNameReturnsLocalValidationError(t *testing.T) {
	originalSubmitWork := submitWork
	defer func() {
		submitWork = originalSubmitWork
	}()

	called := false
	submitWork = func(cfg submitcli.SubmitConfig) error {
		called = true
		return submitcli.Submit(cfg)
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"submit", "--name", "request-name", "--payload", "work.json"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected missing work type name to fail")
	}
	if !called {
		t.Fatal("expected submit validation to run")
	}
	if got := err.Error(); got != "--work-type-name is required" {
		t.Fatalf("missing work type error = %q, want --work-type-name is required", got)
	}
}

func TestSubmitCommand_MissingNameReturnsLocalValidationError(t *testing.T) {
	originalSubmitWork := submitWork
	defer func() {
		submitWork = originalSubmitWork
	}()

	called := false
	submitWork = func(cfg submitcli.SubmitConfig) error {
		called = true
		return submitcli.Submit(cfg)
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"submit", "--work-type-name", "tasks", "--payload", "work.json"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected missing name to fail")
	}
	if !called {
		t.Fatal("expected submit validation to run")
	}
	if got := err.Error(); got != "--name is required" {
		t.Fatalf("missing name error = %q, want --name is required", got)
	}
}

func TestSubmitCommand_MissingPayloadReturnsLocalValidationError(t *testing.T) {
	originalSubmitWork := submitWork
	defer func() {
		submitWork = originalSubmitWork
	}()

	called := false
	submitWork = func(cfg submitcli.SubmitConfig) error {
		called = true
		return submitcli.Submit(cfg)
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"submit", "--name", "request-name", "--work-type-name", "tasks"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected missing payload to fail")
	}
	if !called {
		t.Fatal("expected submit validation to run")
	}
	if got := err.Error(); got != "--payload is required" {
		t.Fatalf("missing payload error = %q, want --payload is required", got)
	}
}

func TestSubmitCommand_WorkTypeNameFlagMapsToSubmitConfig(t *testing.T) {
	originalSubmitWork := submitWork
	defer func() {
		submitWork = originalSubmitWork
	}()

	var got submitcli.SubmitConfig
	submitWork = func(cfg submitcli.SubmitConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"submit",
		"--name", "request-name",
		"--work-type-name", "tasks",
		"--payload", "request.md",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute submit --work-type-name: %v", err)
	}

	if got.Name != "request-name" {
		t.Fatalf("name = %q, want request-name", got.Name)
	}
	if got.WorkTypeName != "tasks" {
		t.Fatalf("work type name = %q, want tasks", got.WorkTypeName)
	}
	if got.Payload != "request.md" {
		t.Fatalf("payload = %q, want request.md", got.Payload)
	}
	if got.Server != "http://localhost:7437" {
		t.Fatalf("server = %q, want http://localhost:7437", got.Server)
	}
}

func TestSubmitCommand_PortFlagRejected(t *testing.T) {
	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"submit",
		"--name", "request-name",
		"--work-type-name", "tasks",
		"--payload", "request.md",
		"--port", "7437",
	})

	if execErr := root.Execute(); execErr == nil {
		t.Fatal("expected --port rejection")
	} else if !strings.Contains(execErr.Error(), "--server") {
		t.Fatalf("error = %v, want --server guidance", execErr)
	}
}

func TestSubmitCommand_DefaultServerMapsToSharedLocalURI(t *testing.T) {
	originalSubmitWork := submitWork
	defer func() {
		submitWork = originalSubmitWork
	}()

	var got submitcli.SubmitConfig
	submitWork = func(cfg submitcli.SubmitConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"submit",
		"--name", "request-name",
		"--work-type-name", "tasks",
		"--payload", "request.md",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute submit: %v", err)
	}

	if got.Server != "http://localhost:7437" {
		t.Fatalf("server = %q, want http://localhost:7437", got.Server)
	}
}
