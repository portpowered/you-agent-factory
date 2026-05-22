package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFactoryService_OpenFactorySessionFromFolder_AutoOpensSingleTarget(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha"},
	})
	defer harness.stop(t)

	result, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), harness.rootDir, nil)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(single target): %v", err)
	}
	if result == nil || result.SessionID == "" {
		t.Fatalf("single-target open result = %#v, want session id", result)
	}
	if len(result.Targets) != 0 {
		t.Fatalf("single-target open returned picker targets = %#v, want none", result.Targets)
	}
	session := harness.requireSession(t, result.SessionID)
	if session.handle.runtime.dir != harness.factoryDirs["alpha"] {
		t.Fatalf("opened session runtime dir = %q, want %q", session.handle.runtime.dir, harness.factoryDirs["alpha"])
	}
	if got := harness.svc.sessions.count(); got != 2 {
		t.Fatalf("live session count = %d, want 2", got)
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_ReturnsTargetPickerMetadata(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig:     minimalFactoryConfig(),
		namedFactories: []string{"alpha", "beta"},
	})
	defer harness.stop(t)

	result, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), harness.rootDir, nil)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(multi target): %v", err)
	}
	if result == nil {
		t.Fatal("expected target picker result, got nil")
	}
	if result.SessionID != "" {
		t.Fatalf("multi-target open returned session %q, want target picker", result.SessionID)
	}
	if len(result.Targets) != 3 {
		t.Fatalf("target picker count = %d, want 3", len(result.Targets))
	}

	assertSessionTargetMetadata(t, result.Targets[0], FactorySessionTargetKindDefault, "", "default", harness.rootDir, "factory")
	assertSessionTargetMetadata(t, result.Targets[1], FactorySessionTargetKindNamed, "alpha", "alpha", filepath.Join(harness.rootDir, "alpha"), "alpha")
	assertSessionTargetMetadata(t, result.Targets[2], FactorySessionTargetKindNamed, "beta", "beta", filepath.Join(harness.rootDir, "beta"), "beta")
	if got := harness.svc.sessions.count(); got != 1 {
		t.Fatalf("target-picker flow mutated live sessions to %d, want 1", got)
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_OpensExplicitDefaultAndNamedTargets(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig:     minimalFactoryConfig(),
		namedFactories: []string{"beta"},
	})
	defer harness.stop(t)

	defaultOpen, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), harness.rootDir, &FactorySessionTargetRef{
		Kind: FactorySessionTargetKindDefault,
	})
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(default): %v", err)
	}
	betaOpenOne, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), harness.rootDir, &FactorySessionTargetRef{
		Kind: FactorySessionTargetKindNamed,
		Name: "beta",
	})
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(beta one): %v", err)
	}
	betaOpenTwo, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), harness.rootDir, &FactorySessionTargetRef{
		Kind: FactorySessionTargetKindNamed,
		Name: "beta",
	})
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder(beta two): %v", err)
	}
	if betaOpenOne.SessionID == betaOpenTwo.SessionID {
		t.Fatalf("duplicate beta session ids = %q", betaOpenOne.SessionID)
	}

	defaultSession := harness.requireSession(t, defaultOpen.SessionID)
	betaSessionOne := harness.requireSession(t, betaOpenOne.SessionID)
	betaSessionTwo := harness.requireSession(t, betaOpenTwo.SessionID)
	if defaultSession.handle.runtime.dir != harness.rootDir {
		t.Fatalf("default target runtime dir = %q, want %q", defaultSession.handle.runtime.dir, harness.rootDir)
	}
	if betaSessionOne.handle.runtime.dir != harness.factoryDirs["beta"] || betaSessionTwo.handle.runtime.dir != harness.factoryDirs["beta"] {
		t.Fatalf("beta target runtime dirs = %q and %q, want %q", betaSessionOne.handle.runtime.dir, betaSessionTwo.handle.runtime.dir, harness.factoryDirs["beta"])
	}
	if got := harness.svc.sessions.count(); got != 4 {
		t.Fatalf("live session count = %d, want 4", got)
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_RejectsInvalidFolderAndTargetWithoutMutation(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	defer harness.stop(t)

	before := harness.svc.sessions.count()
	if _, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), filepath.Join(harness.rootDir, "missing"), nil); err == nil || !strings.Contains(err.Error(), "stat factory session folder") {
		t.Fatalf("OpenFactorySessionFromFolder(missing folder) error = %v, want folder stat failure", err)
	}
	if got := harness.svc.sessions.count(); got != before {
		t.Fatalf("missing-folder open mutated live sessions to %d, want %d", got, before)
	}

	if _, err := harness.svc.OpenFactorySessionFromFolder(context.Background(), harness.rootDir, &FactorySessionTargetRef{
		Kind: FactorySessionTargetKindNamed,
		Name: "missing",
	}); err == nil || !strings.Contains(err.Error(), `factory session target "missing" was not found`) {
		t.Fatalf("OpenFactorySessionFromFolder(missing target) error = %v, want missing-target failure", err)
	}
	if got := harness.svc.sessions.count(); got != before {
		t.Fatalf("missing-target open mutated live sessions to %d, want %d", got, before)
	}
}

func TestFactoryService_OpenFactorySessionFromFolder_CanceledRequestDoesNotRegisterSession(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "beta",
		namedFactories: []string{"beta"},
	})
	defer harness.stop(t)

	beforeIDs := harness.svc.sessions.ids()
	if len(beforeIDs) != 1 || beforeIDs[0] != defaultFactorySessionID {
		t.Fatalf("session ids before canceled open = %v, want [%s]", beforeIDs, defaultFactorySessionID)
	}

	openCtx, cancelOpen := context.WithCancel(context.Background())
	cancelOpen()

	if _, err := harness.svc.OpenFactorySessionFromFolder(openCtx, harness.factoryDirs["beta"], nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenFactorySessionFromFolder(canceled) error = %v, want context canceled", err)
	}

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if got := harness.svc.sessions.ids(); len(got) == 1 && got[0] == defaultFactorySessionID {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		t.Fatalf("canceled open mutated live sessions to %v, want only [%s]", harness.svc.sessions.ids(), defaultFactorySessionID)
	}
}
