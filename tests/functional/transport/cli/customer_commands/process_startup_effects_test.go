package customer_commands_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// W01/W04/W05: bootstrap is activated through the public command, with only
// its exclusive directory reservation replaced. A retry uses the same completed
// process and customer home; no real provider or standalone host is needed.
func TestCLIInitializationFailureRetryAndCancellation(t *testing.T) {
	t.Parallel()
	unavailable := errors.New("controlled packaged installation unavailable")
	var installationUnavailable atomic.Bool
	installationUnavailable.Store(true)
	process := support.BuildProcess(t, serviceedges.Edges{
		FactoryDefinitionPackagedInstallationDirectoryCreator: func(path string, mode fs.FileMode) error {
			if strings.Contains(path, "unavailable-home") && installationUnavailable.Load() {
				return unavailable
			}
			return os.Mkdir(path, mode)
		},
	})
	t.Run("installation failure and retry", func(t *testing.T) {
		t.Parallel()
		testCLIInstallationRetry(t, process, unavailable, &installationUnavailable)
	})
	t.Run("cancellation has no rollback facts", func(t *testing.T) {
		t.Parallel()
		testCLIInitializationCancellation(t, process)
	})
}

func testCLIInstallationRetry(t *testing.T, process support.Process, unavailable error, installationUnavailable *atomic.Bool) {
	t.Helper()
	home := newCLIHome(t)
	home.home = filepath.Join(home.home, "unavailable-home")
	// Help must remain usable before any bootstrap effect is activated.
	home.requireSuccess(t, process, "--help")
	configPath := filepath.Join(home.home, ".you-agent-factory", "config.json")
	if _, err := os.Stat(configPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("help initialized operator configuration: %v", err)
	}
	args := []string{"--server", "http://127.0.0.1:1", "server", "mcp", "--project-root", home.work}
	failed := home.run(t, process, nil, args...)
	var partial systeminitialization.InitializePartialFailure
	wantFacts := []systeminitialization.RollbackFact{
		{Step: systeminitialization.InitializeStepSystemConfig, Outcome: systeminitialization.RollbackStepRolledBackOrPreserved},
		{Step: systeminitialization.InitializeStepPackagedFactories, Outcome: systeminitialization.RollbackStepUnresolved},
	}
	if !errors.Is(failed.Err, unavailable) || !errors.Is(failed.Err, systeminitialization.ErrInitializePartialFailure) ||
		!errors.As(failed.Err, &partial) || !reflect.DeepEqual(partial.Facts, wantFacts) || failed.Stdout != "" {
		t.Fatalf("bootstrap failure = %#v; facts = %#v", failed, partial.Facts)
	}
	preserved, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("partial initialization did not preserve operator configuration: %v", err)
	}
	installationUnavailable.Store(false)
	home.requireSuccess(t, process, args...)
	current, err := os.ReadFile(configPath)
	if err != nil || string(current) != string(preserved) {
		t.Fatalf("retry changed preserved operator configuration: %v", err)
	}
	requireInstalledSubagentValidation(t, process, home)
	// A repeat bootstrap must leave the installed public definition usable.
	home.requireSuccess(t, process, args...)
	requireInstalledSubagentValidation(t, process, home)
}

func requireInstalledSubagentValidation(t *testing.T, process support.Process, home cliHome) {
	t.Helper()
	result := home.requireSuccess(t, process, "--json", "factory", "list")
	var entries []struct {
		Name             string `json:"name"`
		FactoryDirectory string `json:"factoryDirectory"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &entries); err != nil {
		t.Fatalf("decode installed Factory catalog: %v; stdout = %q", err, result.Stdout)
	}
	for _, entry := range entries {
		if entry.Name != "@you/subagent" {
			continue
		}
		if !filepath.IsAbs(entry.FactoryDirectory) {
			t.Fatalf("subagent has no installed directory: %#v", entry)
		}
		home.requireSuccess(t, process, "factory", "config", "validate", filepath.Join(entry.FactoryDirectory, "factory.json"))
		return
	}
	t.Fatalf("installed Factory catalog is missing @you/subagent: %#v", entries)
}

func testCLIInitializationCancellation(t *testing.T, process support.Process) {
	t.Helper()
	home := newCLIHome(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	inputs := support.FakeInputs(ctx, []string{"you", "server", "mcp", "--project-root", home.work})
	inputs.Input.Env = []string{"HOME=" + home.home, "USERPROFILE=" + home.home}
	inputs.Input.WorkingDirectory = home.work
	inputs.Input.Stdin = strings.NewReader("")
	err := process.Execute(inputs.Input)
	var partial systeminitialization.InitializePartialFailure
	if !errors.Is(err, context.Canceled) || !errors.Is(err, systeminitialization.ErrInitializeCancelled) ||
		errors.As(err, &partial) || inputs.Stdout() != "" {
		t.Fatalf("canceled bootstrap = %v; stdout = %q; facts = %#v", err, inputs.Stdout(), partial.Facts)
	}
	if _, statErr := os.Stat(filepath.Join(home.home, ".you-agent-factory", "config.json")); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("canceled bootstrap published configuration: %v", statErr)
	}
}

func TestCLIStartupCauseEffects(t *testing.T) {
	t.Parallel()
	rootCause := errors.New(`controlled startup unavailable token="PRIVATE_TOKEN" prompt="PRIVATE_PROMPT" body="PRIVATE_BODY" open /PRIVATE_DIRECTORY/config`)
	readEntered, releaseRead := make(chan struct{}), make(chan struct{})
	process := support.BuildProcess(t, serviceedges.Edges{
		SystemInitializationInspectPath: func(path string) (fs.FileInfo, error) {
			if strings.Contains(path, "failed-profile") {
				return nil, fmt.Errorf("inspect operator configuration: %w", rootCause)
			}
			return os.Stat(path)
		},
		FactorySessionReplayRecordingReader: func(path string) ([]byte, error) {
			if filepath.Base(path) == "cancel-read.json" {
				close(readEntered)
				<-releaseRead
				return nil, context.Canceled
			}
			return os.ReadFile(path)
		},
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			return fmt.Errorf("start API listener: %w", rootCause)
		},
	})
	// These local startup failures can acquire Current Factory/~default before
	// failing. Keep this smallest cohort ordered on the reusable process.
	for _, scenario := range []string{"preparation", "application startup", "listener startup"} {
		t.Run(scenario, func(t *testing.T) {
			testStartupEffect(t, process, rootCause, scenario)
		})
	}
	t.Run("cancel held recording read", func(t *testing.T) {
		testCancelledStartupRead(t, process, readEntered, releaseRead)
	})
}

func testStartupEffect(t *testing.T, process support.Process, rootCause error, scenario string) {
	t.Helper()
	home := newCLIHome(t)
	args := []string{"run", "--quiet"}
	want := "controlled startup unavailable"
	switch scenario {
	case "preparation":
		args = append(args, "--dir", filepath.Join(home.work, "absent-factory"))
		want = factorydefinitions.ErrFactoryLayoutNotFound.Error()
	case "application startup":
		home.home = filepath.Join(home.home, "failed-profile")
		args = append(args, "--dir", startupEffectFactory(t), "--continuously", "--with-server")
	case "listener startup":
		args = append(args, "--dir", startupEffectFactory(t), "--continuously", "--with-server")
	}
	result := home.run(t, process, nil, args...)
	if result.Err == nil || !strings.Contains(result.Stderr, "cause[0]=") || strings.Count(result.Stderr, "cause[0]=") != 1 {
		t.Fatalf("missing single startup chain: err=%v stderr=%q", result.Err, result.Stderr)
	}
	if scenario == "preparation" {
		_, causes, _ := strings.Cut(result.Stderr, "\n")
		if !errors.Is(result.Err, factorydefinitions.ErrFactoryLayoutNotFound) || !strings.Contains(causes, want) {
			t.Fatalf("preparation hid filesystem cause: %q", result.Stderr)
		}
	} else if !errors.Is(result.Err, rootCause) || !strings.Contains(result.Stderr, want) {
		t.Fatalf("startup lost cause/identity: err=%v stderr=%q", result.Err, result.Stderr)
	}
	if strings.Contains(result.Stderr, "PRIVATE_") || strings.Contains(result.Stdout, "Factory initiated:") {
		t.Fatalf("startup leaked sensitive contents or reported success: stdout=%q stderr=%q", result.Stdout, result.Stderr)
	}
}

func testCancelledStartupRead(t *testing.T, process support.Process, readEntered, releaseRead chan struct{}) {
	t.Helper()
	t.Parallel()
	home := newCLIHome(t)
	path := filepath.Join(home.work, "cancel-read.json")
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	inputs := support.FakeInputs(ctx, []string{"you", "run", "--resume", path, "--quiet"})
	inputs.Input.Env = append(os.Environ(), "HOME="+home.home, "USERPROFILE="+home.home)
	inputs.Input.WorkingDirectory = home.work
	done := make(chan error, 1)
	go func() { done <- process.Execute(inputs.Input) }()
	select {
	case <-readEntered:
	case <-time.After(30 * time.Second):
		t.Fatal("recording read not reached")
	}
	cancel()
	close(releaseRead)
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || strings.Contains(inputs.Stdout()+inputs.Stderr(), "Factory initiated:") {
			t.Fatalf("cancelled read lost identity or reported readiness: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("cancelled opening did not join")
	}
}

func startupEffectFactory(t *testing.T) string {
	t.Helper()
	return support.ScaffoldFactory(t, map[string]any{
		"name": "startup-effects", "workTypes": []map[string]any{{"name": "task", "states": []map[string]string{
			{"name": "init", "type": "INITIAL"}, {"name": "complete", "type": "TERMINAL"},
		}}},
		"workstations": []map[string]any{{"name": "finish", "type": "LOGICAL_MOVE",
			"inputs":  []map[string]string{{"workType": "task", "state": "init"}},
			"outputs": []map[string]string{{"workType": "task", "state": "complete"}},
		}},
	})
}
