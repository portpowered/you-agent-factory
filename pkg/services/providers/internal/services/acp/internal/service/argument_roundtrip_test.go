package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
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
