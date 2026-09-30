package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

const argumentRoundTripHelperEnvironment = "YOU_TEST_ACP_ARGUMENT_ROUNDTRIP_HELPER"

// TestACPArgumentRoundTripHelperProcess is the direct child used by
// TestExecuteUsesLosslessQuotedLaunch. It exits before an ACP handshake so the
// test can assert the exact executable and argv received by exec.Command
// without running a shell or a live provider.
func TestACPArgumentRoundTripHelperProcess(t *testing.T) {
	if os.Getenv(argumentRoundTripHelperEnvironment) == "" {
		return
	}
	os.Exit(0)
}

type bridgeExecutableLocator struct {
	current string
	looked  []string
}

func (locator *bridgeExecutableLocator) LookPath(name string) (string, error) {
	locator.looked = append(locator.looked, name)
	if name == "you" {
		return "", errors.New("you is absent from PATH")
	}
	return name, nil
}

func (locator *bridgeExecutableLocator) CurrentExecutable() (string, error) {
	return locator.current, nil
}

func TestBundledPiBridgeUsesCurrentExecutableWithoutYouOnPATH(t *testing.T) {
	current, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	locator := &bridgeExecutableLocator{current: current}
	var launched string
	var arguments []string
	commandFactory := func(name string, args ...string) *exec.Cmd {
		launched, arguments = name, append([]string(nil), args...)
		return exec.Command(current, "-test.run=^TestACPArgumentRoundTripHelperProcess$")
	}
	daemon := newDaemon(Command{Name: "you", Args: []string{"pi-acp"}}, commandFactory, locator)
	t.Cleanup(func() { _ = daemon.close(context.Background()) })
	_ = daemon.ensureStarted(context.Background(), providers.ID("pi"), t.TempDir(),
		append(os.Environ(), argumentRoundTripHelperEnvironment+"=1"), providers.ExecuteRequest{})
	if launched != current {
		t.Fatalf("launch executable = %q, want %q", launched, current)
	}
	if len(arguments) != 1 || arguments[0] != "pi-acp" {
		t.Fatalf("launch arguments = %q, want [pi-acp]", arguments)
	}
	if len(locator.looked) != 0 {
		t.Fatalf("PATH lookups = %q, want none", locator.looked)
	}
}

func TestOtherACPCommandStillUsesConfiguredExecutable(t *testing.T) {
	locator := &bridgeExecutableLocator{current: filepath.Join(t.TempDir(), "you")}
	var launched string
	commandFactory := func(name string, args ...string) *exec.Cmd {
		launched = name
		return exec.Command(os.Args[0], "-test.run=^TestACPArgumentRoundTripHelperProcess$")
	}
	daemon := newDaemon(Command{Name: "other-agent", Args: []string{"acp"}}, commandFactory, locator)
	t.Cleanup(func() { _ = daemon.close(context.Background()) })
	_ = daemon.ensureStarted(context.Background(), providers.ID("other"), t.TempDir(),
		append(os.Environ(), argumentRoundTripHelperEnvironment+"=1"), providers.ExecuteRequest{})
	if launched != "other-agent" || len(locator.looked) != 1 || locator.looked[0] != "other-agent" {
		t.Fatalf("other launch = %q, PATH lookups = %q", launched, locator.looked)
	}
}

func TestExecuteUsesLosslessQuotedLaunch(t *testing.T) {
	wantName := `agent'\tool`
	wantArguments := []string{"hello world", "semi;colon", "quote's"}
	var gotName string
	var gotArguments []string
	commandFactory := func(name string, arguments ...string) *exec.Cmd {
		gotName = name
		gotArguments = append([]string(nil), arguments...)
		return exec.Command(os.Args[0], "-test.run=^TestACPArgumentRoundTripHelperProcess$")
	}

	serviceValue, err := New([]providers.ACPIntegration{{
		Name:      "quoted-acp",
		Transport: "stdio",
		Command:   `'agent'\''\tool' 'hello world' 'semi;colon' 'quote'\''s'`,
		Arguments: wantArguments,
	}}, commandFactory, availableLocator{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = serviceValue.Close(context.Background()) })

	_, err = serviceValue.Execute(context.Background(), "quoted-acp", providers.ExecuteRequest{
		Provider:           "quoted-acp",
		AttemptID:          "quoted-argument-attempt",
		UserMessage:        "exercise quoted arguments",
		WorkingDirectory:   t.TempDir(),
		ProcessEnvironment: append(os.Environ(), argumentRoundTripHelperEnvironment+"=1"),
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want helper process to terminate before ACP initialize")
	}
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Execute() error = %v (%T), want ExecuteFailure", err, err)
	}
	if gotName != wantName {
		t.Fatalf("command executable = %q, want %q", gotName, wantName)
	}
	if !reflect.DeepEqual(gotArguments, wantArguments) {
		t.Fatalf("command arguments = %#v, want %#v", gotArguments, wantArguments)
	}
}

func TestParseACPCommandPreservesWindowsExecutablePath(t *testing.T) {
	for _, test := range []struct {
		command string
		want    []string
	}{
		{`C:\Users\andre\.bun\bin\opencode.exe acp`, []string{`C:\Users\andre\.bun\bin\opencode.exe`, "acp"}},
		{`"C:\Program Files\OpenCode\opencode.exe" acp`, []string{`C:\Program Files\OpenCode\opencode.exe`, "acp"}},
		{`npx -y opencode-ai acp`, []string{"npx", "-y", "opencode-ai", "acp"}},
	} {
		got, err := parseACPCommand(test.command)
		if err != nil || !reflect.DeepEqual(got, test.want) {
			t.Fatalf("parseACPCommand(%q) = %#v, %v; want %#v", test.command, got, err, test.want)
		}
	}
}

func TestConfigureRetainsUnchangedDaemonAndReplacesChangedCommand(t *testing.T) {
	serviceValue, err := New([]providers.ACPIntegration{{
		ID: "entry-1", Name: "custom-acp", Transport: "stdio", Command: "agent acp",
	}}, nil, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	service := serviceValue.(*Service)
	original := service.daemons["custom-acp"]

	if err := service.Configure(context.Background(), []providers.ACPIntegration{{
		ID: "entry-1", Name: "custom-acp", Transport: "stdio", Command: "agent acp",
	}}); err != nil {
		t.Fatalf("Configure(unchanged) error = %v", err)
	}
	if service.daemons["custom-acp"] != original {
		t.Fatal("unchanged configuration replaced the live daemon")
	}

	if err := service.Configure(context.Background(), []providers.ACPIntegration{{
		ID: "entry-1", Name: "custom-acp", Transport: "stdio", Command: "replacement acp",
	}}); err != nil {
		t.Fatalf("Configure(replacement) error = %v", err)
	}
	if service.daemons["custom-acp"] == original {
		t.Fatal("changed command retained the old daemon")
	}
}

func TestOpenCodeEnvironment(t *testing.T) {
	tests := []struct {
		name        string
		provider    providers.ID
		environment []string
		want        []string
	}{
		{
			name:        "OpenCode default preserves other entries",
			provider:    providers.IDOpenCode,
			environment: []string{"OTHER=value"},
			want:        []string{"OTHER=value", `OPENCODE_CONFIG_CONTENT={"snapshots":false}`},
		},
		{
			name:        "operator inline config is preserved case insensitively",
			provider:    providers.IDOpenCode,
			environment: []string{"OTHER=value", `opencode_config_content={"snapshots":true}`, "LAST=entry"},
			want:        []string{"OTHER=value", `opencode_config_content={"snapshots":true}`, "LAST=entry"},
		},
		{
			name:        "other provider unchanged",
			provider:    providers.IDCodex,
			environment: []string{"OTHER=value"},
			want:        []string{"OTHER=value"},
		},
		{
			name:     "nil OpenCode environment",
			provider: providers.IDOpenCode,
			want:     []string{`OPENCODE_CONFIG_CONTENT={"snapshots":false}`},
		},
		{
			name:        "empty OpenCode environment",
			provider:    providers.IDOpenCode,
			environment: []string{},
			want:        []string{`OPENCODE_CONFIG_CONTENT={"snapshots":false}`},
		},
		{
			name:     "nil other provider environment",
			provider: providers.IDCodex,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := openCodeEnvironment(test.provider, test.environment)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("openCodeEnvironment() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestConfigureRejectsMalformedReplacementWithoutChangingLiveSet(t *testing.T) {
	serviceValue, err := New([]providers.ACPIntegration{{
		ID: "entry-1", Name: "custom-acp", Transport: "stdio", Command: "agent acp",
	}}, nil, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	service := serviceValue.(*Service)
	original := service.daemons["custom-acp"]

	err = service.Configure(context.Background(), []providers.ACPIntegration{{
		ID: "entry-1", Name: "custom-acp", Transport: "http", Command: "agent acp",
	}})
	if err == nil {
		t.Fatal("Configure(malformed) error = nil")
	}
	if service.daemons["custom-acp"] != original {
		t.Fatal("malformed replacement mutated the live daemon set")
	}
}
