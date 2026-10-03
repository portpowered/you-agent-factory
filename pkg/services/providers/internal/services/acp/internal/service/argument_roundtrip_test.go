package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	acpsdk "github.com/portpowered/infinite-you/third_party/acp-go-sdk"
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
	daemon := newTestAttempt(t, "pi", Command{Name: "you", Args: []string{"pi-acp"}}, commandFactory, locator)
	_, _ = daemon.start(context.Background(), t.TempDir(),
		append(os.Environ(), argumentRoundTripHelperEnvironment+"=1"))
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
	daemon := newTestAttempt(t, "other", Command{Name: "other-agent", Args: []string{"acp"}}, commandFactory, locator)
	_, _ = daemon.start(context.Background(), t.TempDir(),
		append(os.Environ(), argumentRoundTripHelperEnvironment+"=1"))
	if launched != "other-agent" || len(locator.looked) != 1 || locator.looked[0] != "other-agent" {
		t.Fatalf("other launch = %q, PATH lookups = %q", launched, locator.looked)
	}
}

// newTestAttempt builds one registered, request-owned attempt directly, for
// launch-argument assertions that need no configured integration set. The
// cleanup releases it exactly as an Execute/Continue terminal path would, so
// the process it owns is torn down rather than left running.
func newTestAttempt(
	t *testing.T,
	id providers.ID,
	command Command,
	commandFactory platformprocess.CommandFactory,
	locator platformprocess.ExecutableLocator,
) *attempt {
	t.Helper()
	target := newProvider(id, providers.ACPIntegration{Name: id, Transport: "stdio"}, command, commandFactory, locator, platformprocess.NewParentOwnedStdio)
	owned := target.newAttempt(providers.ExecuteRequest{AttemptID: "launch-argument-attempt"})
	t.Cleanup(owned.release)
	return owned
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
	}}, commandFactory, availableLocator{}, platformprocess.NewParentOwnedStdio)
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

// TestConfigureRetainsUnchangedProviderAndReplacesChangedCommand proves the
// retention rule Configure applies to a reconfiguration: an integration whose
// launch configuration and ACP behavior fields are unchanged keeps its provider
// object - and with it every attempt still running against it and the
// capability facts its handshakes negotiated - while any changed field replaces
// the provider and drains its attempts.
//
// Every observation here is read-only: retiredState never retires, so asserting
// that a configuration change left an attempt alone cannot be the reason that
// attempt was retired.
func TestConfigureRetainsUnchangedProviderAndReplacesChangedCommand(t *testing.T) {
	serviceValue, err := New([]providers.ACPIntegration{{
		ID: "entry-1", Name: "custom-acp", Aliases: []string{"custom"}, Transport: "stdio",
		Command: "agent acp", RuntimePosture: "installed_executable", ImplementationProfile: "custom-acp",
	}}, nil, nil, platformprocess.NewParentOwnedStdio)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	service := serviceValue.(*Service)
	original := service.providers["custom-acp"]
	// A capability fact only an attempt of this very provider can have
	// negotiated; it must survive an unchanged reconfiguration and must not
	// survive one that replaces the provider.
	original.recordNegotiated(acpsdk.AgentCapabilities{LoadSession: true})
	active := original.newAttempt(providers.ExecuteRequest{AttemptID: "active-attempt"})
	t.Cleanup(active.release)

	unchanged := providers.ACPIntegration{
		ID: "entry-1", Name: "custom-acp", Aliases: []string{"custom"}, Transport: "stdio",
		Command: "agent acp", RuntimePosture: "installed_executable", ImplementationProfile: "custom-acp",
	}
	if err := service.Configure(context.Background(), []providers.ACPIntegration{unchanged}); err != nil {
		t.Fatalf("Configure(unchanged) error = %v", err)
	}
	if service.providers["custom-acp"] != original {
		t.Fatal("unchanged configuration replaced the live provider")
	}
	if active.retiredState() {
		t.Fatal("unchanged configuration retired an active attempt belonging to the retained integration")
	}
	if capabilities, ok := service.NegotiatedCapabilities("custom"); !ok || !capabilities.LoadSession {
		t.Fatalf("NegotiatedCapabilities(unchanged) = (%#v, %t), want the retained provider's own handshake facts", capabilities, ok)
	}

	for _, changed := range []struct {
		name        string
		integration providers.ACPIntegration
	}{
		{name: "changed executable", integration: withLaunch(unchanged, "replacement acp")},
		{name: "changed arguments", integration: withArguments(unchanged, "agent acp --acp")},
		{name: "changed behavior profile", integration: withProfile(unchanged, "replacement-profile")},
		{name: "changed runtime posture", integration: withPosture(unchanged, "package_runner")},
	} {
		t.Run(changed.name, func(t *testing.T) {
			before := service.providers["custom-acp"]
			attempt := before.newAttempt(providers.ExecuteRequest{AttemptID: "replaced-attempt"})
			t.Cleanup(attempt.release)
			if err := service.Configure(context.Background(), []providers.ACPIntegration{changed.integration}); err != nil {
				t.Fatalf("Configure(%s) error = %v", changed.name, err)
			}
			if service.providers["custom-acp"] == before {
				t.Fatalf("%s retained the previous provider", changed.name)
			}
			if !attempt.retiredState() {
				t.Fatalf("%s left an attempt of the replaced integration running", changed.name)
			}
		})
	}
}

// withLaunch, withArguments, withProfile and withPosture return a copy of the
// integration with exactly one launch or behavior field changed.
func withLaunch(integration providers.ACPIntegration, command string) providers.ACPIntegration {
	integration.Command, integration.Arguments = command, nil
	return integration
}

func withArguments(integration providers.ACPIntegration, command string) providers.ACPIntegration {
	integration.Command, integration.Arguments = command, []string{"acp", "--acp"}
	return integration
}

func withProfile(integration providers.ACPIntegration, profile string) providers.ACPIntegration {
	integration.ImplementationProfile = profile
	return integration
}

func withPosture(integration providers.ACPIntegration, posture string) providers.ACPIntegration {
	integration.RuntimePosture = posture
	return integration
}

// TestConfigureStopsAttemptsOfRemovedIntegration proves Configure stops the
// attempts of an integration that was removed while preserving the attempts of
// integrations it did not change.
func TestConfigureStopsAttemptsOfRemovedIntegration(t *testing.T) {
	serviceValue, err := New([]providers.ACPIntegration{{
		ID: "entry-1", Name: "custom-acp", Transport: "stdio", Command: "agent acp",
	}, {
		ID: "entry-2", Name: "other-acp", Transport: "stdio", Command: "other acp",
	}}, nil, nil, platformprocess.NewParentOwnedStdio)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	service := serviceValue.(*Service)
	removed := service.providers["custom-acp"].newAttempt(providers.ExecuteRequest{AttemptID: "removed-attempt"})
	kept := service.providers["other-acp"].newAttempt(providers.ExecuteRequest{AttemptID: "kept-attempt"})
	t.Cleanup(kept.release)

	if err := service.Configure(context.Background(), []providers.ACPIntegration{{
		ID: "entry-2", Name: "other-acp", Transport: "stdio", Command: "other acp",
	}}); err != nil {
		t.Fatalf("Configure(removal) error = %v", err)
	}
	if !removed.retiredState() {
		t.Fatal("removed integration left its registered attempt running")
	}
	if kept.retiredState() {
		t.Fatal("Configure(removal) stopped an attempt of an unchanged integration")
	}
}

// TestCloseStopsEveryRegisteredAttempt proves Close stops the attempts of every
// configured integration, including attempts registered after the attempt
// itself began but before it published its process.
//
// It also pins the ownership rule that makes that safe: Close is given a
// deadline and must still complete without waiting on a startup that never
// began. An attempt that never launched owns no process, so its teardown is
// immediate; a Close that blocked on such an attempt would return this
// deadline's error instead.
func TestCloseStopsEveryRegisteredAttempt(t *testing.T) {
	serviceValue, err := New([]providers.ACPIntegration{{
		ID: "entry-1", Name: "custom-acp", Transport: "stdio", Command: "agent acp",
	}}, nil, nil, platformprocess.NewParentOwnedStdio)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	service := serviceValue.(*Service)
	first := service.providers["custom-acp"].newAttempt(providers.ExecuteRequest{AttemptID: "first"})
	second := service.providers["custom-acp"].newAttempt(providers.ExecuteRequest{AttemptID: "second"})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := service.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v, want nil: teardown must finish for every attempt that never launched", err)
	}
	if !first.retiredState() || !second.retiredState() {
		t.Fatal("Close() left a registered attempt running")
	}
}

// TestRetiredAttemptRefusesToLaunch proves a retired attempt cannot spawn an
// unowned peer. Close or Configure may retire an attempt after it registered but
// before it reached startup; a spawn that could still happen afterwards would be
// a process nothing owns.
func TestRetiredAttemptRefusesToLaunch(t *testing.T) {
	started := 0
	factory := platformprocess.CommandFactory(func(_ string, _ ...string) *exec.Cmd {
		started++
		return exec.Command(os.Args[0], "-test.run=^TestACPArgumentRoundTripHelperProcess$")
	})
	owned := newTestAttempt(t, "custom-acp", Command{Name: "agent", Args: []string{"acp"}}, factory, availableLocator{})
	if !owned.retire() {
		t.Fatal("retire() = false on a live attempt, want true")
	}
	if _, err := owned.start(context.Background(), t.TempDir(), os.Environ()); err == nil {
		t.Fatal("start() error = nil, want the retired attempt to refuse to launch")
	}
	if err := owned.stop(context.Background()); err != nil {
		t.Fatalf("stop() after a refused launch error = %v, want nil: nothing was owned", err)
	}
	if started != 0 {
		t.Fatalf("ACP processes started = %d, want none after retirement", started)
	}
}

// TestExecuteCanceledBeforeStartupCompletes proves a request rejected before it
// starts a peer still finishes: the attempt's own release must not wait on a
// startup that never began, or Execute would never return.
func TestExecuteCanceledBeforeStartupCompletes(t *testing.T) {
	serviceValue, err := New([]providers.ACPIntegration{{
		ID: "entry-1", Name: "custom-acp", Transport: "stdio", Command: "agent acp",
	}}, nil, nil, platformprocess.NewParentOwnedStdio)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = serviceValue.Close(context.Background()) })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	returned := make(chan error, 1)
	go func() {
		_, err := serviceValue.Execute(ctx, "custom-acp", providers.ExecuteRequest{
			Provider: "custom-acp", AttemptID: "canceled-attempt",
			UserMessage: "never starts", WorkingDirectory: t.TempDir(),
		})
		returned <- err
	}()
	select {
	case err := <-returned:
		var failure providers.ExecuteFailure
		if !errors.As(err, &failure) || failure.Kind != providers.ExecuteFailureKindCanceled {
			t.Fatalf("Execute(canceled) error = %#v, want ExecuteFailureKindCanceled", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Execute() of an already-canceled request never returned: teardown waited on a startup that never began")
	}
}

// TestExecuteAfterCloseFailsAsUnavailable proves an attempt cannot escape a
// Close by resolving the provider it no longer has.
func TestExecuteAfterCloseFailsAsUnavailable(t *testing.T) {
	serviceValue, err := New([]providers.ACPIntegration{{
		ID: "entry-1", Name: "custom-acp", Transport: "stdio", Command: "agent acp",
	}}, nil, nil, platformprocess.NewParentOwnedStdio)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := serviceValue.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	_, err = serviceValue.Execute(context.Background(), "custom-acp", providers.ExecuteRequest{
		Provider: "custom-acp", AttemptID: "after-close", WorkingDirectory: t.TempDir(),
	})
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) || failure.Kind != providers.ExecuteFailureKindDependency {
		t.Fatalf("Execute(after Close) error = %#v, want an unavailable dependency failure", err)
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
	}}, nil, nil, platformprocess.NewParentOwnedStdio)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	service := serviceValue.(*Service)
	original := service.providers["custom-acp"]

	err = service.Configure(context.Background(), []providers.ACPIntegration{{
		ID: "entry-1", Name: "custom-acp", Transport: "http", Command: "agent acp",
	}})
	if err == nil {
		t.Fatal("Configure(malformed) error = nil")
	}
	if service.providers["custom-acp"] != original {
		t.Fatal("malformed replacement mutated the live provider set")
	}
}
