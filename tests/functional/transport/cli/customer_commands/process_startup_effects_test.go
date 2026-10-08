package customer_commands_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

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
