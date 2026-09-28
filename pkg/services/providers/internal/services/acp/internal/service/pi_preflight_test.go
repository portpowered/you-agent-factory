package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

const piVersionHelperEnv = "YOU_TEST_PI_VERSION_HELPER"

func TestPiVersionHelperProcess(t *testing.T) {
	value := os.Getenv(piVersionHelperEnv)
	if value == "" {
		return
	}
	if value == "sleep" {
		time.Sleep(10 * time.Second)
	} else {
		_, _ = fmt.Fprintln(os.Stdout, value)
	}
	os.Exit(0)
}

type piVersionLocator struct{ missing bool }

func (locator piVersionLocator) LookPath(name string) (string, error) {
	if locator.missing || name != "pi" {
		return "", errors.New("private executable path")
	}
	return name, nil
}

func piVersionCommandFactory(calls *atomic.Int32) platformprocess.CommandFactory {
	return func(name string, args ...string) *exec.Cmd {
		calls.Add(1)
		if name != "pi" || len(args) != 1 || args[0] != "--version" {
			return nil
		}
		return exec.Command(os.Args[0], "-test.run=^TestPiVersionHelperProcess$")
	}
}

func piVersionEnvironment(value string) []string {
	return append(os.Environ(), piVersionHelperEnv+"="+value)
}

func TestPiPreflightVersionRequirement(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		output    string
		wantError bool
	}{
		{name: "older Pi", output: "0.74.2", wantError: true},
		{name: "minimum Pi", output: "0.81.0"},
		{name: "newer Pi", output: "0.87.1"},
		{name: "unparseable output", output: "private token and path", wantError: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var calls atomic.Int32
			err := piPreflight(context.Background(), piVersionCommandFactory(&calls), piVersionLocator{}, t.TempDir(), piVersionEnvironment(testCase.output))
			if calls.Load() != 1 {
				t.Fatalf("version command calls = %d, want 1", calls.Load())
			}
			if !testCase.wantError {
				if err != nil {
					t.Fatalf("piPreflight(%q): %v", testCase.output, err)
				}
				return
			}
			var failure providers.ExecuteFailure
			if !errors.As(err, &failure) || failure.Kind != providers.ExecuteFailureKindMisconfigured {
				t.Fatalf("piPreflight(%q) = %v, want misconfigured", testCase.output, err)
			}
			if failure.Message != piUpgradeAction || strings.Contains(failure.Message, testCase.output) {
				t.Fatalf("unsafe or unactionable failure = %q", failure.Message)
			}
		})
	}
}

func TestPiPreflightMissingExecutable(t *testing.T) {
	var calls atomic.Int32
	err := piPreflight(context.Background(), piVersionCommandFactory(&calls), piVersionLocator{missing: true}, t.TempDir(), nil)
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) || failure.Kind != providers.ExecuteFailureKindDependency ||
		failure.Diagnostics == nil || failure.Diagnostics.Metadata["work-failure-type"] != "missing_executable" {
		t.Fatalf("missing executable failure = %#v", err)
	}
	if calls.Load() != 0 || strings.Contains(failure.Message, "private") {
		t.Fatalf("missing executable launched command or leaked locator error: %#v", failure)
	}
}

func TestPiPreflightWithoutLocatorUsesCommandFactory(t *testing.T) {
	var calls atomic.Int32
	err := piPreflight(context.Background(), piVersionCommandFactory(&calls), nil, t.TempDir(), piVersionEnvironment("0.87.1"))
	if err != nil || calls.Load() != 1 {
		t.Fatalf("preflight without locator = %v; command calls = %d, want success and one call", err, calls.Load())
	}
	err = piPreflight(context.Background(), func(string, ...string) *exec.Cmd {
		return exec.Command("you-pi-does-not-exist-4e6ad0")
	}, nil, t.TempDir(), os.Environ())
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) || failure.Kind != providers.ExecuteFailureKindDependency ||
		failure.Diagnostics == nil || failure.Diagnostics.Metadata["work-failure-type"] != "missing_executable" {
		t.Fatalf("missing executable without locator = %#v, want dependency failure", err)
	}
}

func TestPiPreflightCanceledContextSkipsCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls atomic.Int32
	err := piPreflight(ctx, piVersionCommandFactory(&calls), piVersionLocator{}, t.TempDir(), nil)
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) || failure.Kind != providers.ExecuteFailureKindCanceled || calls.Load() != 0 {
		t.Fatalf("canceled preflight = %#v, command calls = %d", err, calls.Load())
	}
}

func TestPiPreflightCancelsRunningVersionProbe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	var calls atomic.Int32
	started := time.Now()
	err := piPreflight(ctx, piVersionCommandFactory(&calls), piVersionLocator{}, t.TempDir(), piVersionEnvironment("sleep"))
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) || failure.Kind != providers.ExecuteFailureKindTimeout || calls.Load() != 1 {
		t.Fatalf("running probe cancellation = %#v, command calls = %d", err, calls.Load())
	}
	if time.Since(started) > 5*time.Second {
		t.Fatal("version probe did not terminate promptly")
	}
}

func TestParsePiVersion(t *testing.T) {
	for _, testCase := range []struct {
		input string
		want  piVersion
		ok    bool
	}{
		{input: "0.81.0", want: piVersion{0, 81, 0}, ok: true},
		{input: "pi 0.87.1\n", want: piVersion{0, 87, 1}, ok: true},
		{input: "v1.0.0", want: piVersion{1, 0, 0}, ok: true},
		{input: "0.74", ok: false},
		{input: "private data", ok: false},
		{input: "-1.0.0", ok: false},
	} {
		got, ok := parsePiVersion(testCase.input)
		if ok != testCase.ok || (ok && got != testCase.want) {
			t.Fatalf("parsePiVersion(%q) = %#v, %t; want %#v, %t", testCase.input, got, ok, testCase.want, testCase.ok)
		}
	}
}
