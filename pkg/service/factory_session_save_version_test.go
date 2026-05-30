package service

import (
	"context"
	"errors"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/config"
)

func TestFactoryService_SaveFactoryForSession_ReplaceCurrentRejectsMissingVersion(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha"},
	})
	defer harness.stop(t)

	current, err := harness.svc.GetCurrentFactoryForSession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetCurrentFactoryForSession: %v", err)
	}

	replacement := serviceNamedFactoryContractWithWorkType(t, "alpha", "story")
	replacement.Version = nil
	_, err = harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeReplaceCurrent,
		replacement,
	)
	if !errors.Is(err, apisurface.ErrFactoryVersionStale) {
		t.Fatalf("SaveFactoryForSession(replace missing version) error = %v, want stale version", err)
	}

	reloaded, err := harness.svc.GetCurrentFactoryForSession(context.Background(), defaultFactorySessionID)
	if err != nil {
		t.Fatalf("GetCurrentFactoryForSession after rejected save: %v", err)
	}
	assertFactoryWorkType(t, reloaded.WorkTypes, "task", "unchanged work types after missing-version reject")
	if current.Version != nil && reloaded.Version != nil {
		assertMatchingFactoryVersion(t, reloaded.Version, current.Version, "unchanged version after missing-version reject")
	}
}

func TestFactoryService_SaveFactoryForSession_UpsertCreateAllowsOmittedVersion(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha"},
	})
	defer harness.stop(t)

	created, err := harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		serviceNamedFactoryContract(t, "beta"),
	)
	if err != nil {
		t.Fatalf("SaveFactoryForSession(upsert create beta): %v", err)
	}
	if created.Version == nil {
		t.Fatal("expected created factory version metadata when client omitted version")
	}
	if created.Version.Logical.Int64() < 1 {
		t.Fatalf("created version logical = %d, want >= 1", created.Version.Logical.Int64())
	}
	assertFactoryWorkType(t, created.WorkTypes, "task", "created beta work types")
}

func TestFactoryService_SaveFactoryForSession_UpsertReplaceUsesOnDiskVersion(t *testing.T) {
	rootDir := t.TempDir()
	initialVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  3,
		Physical: time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC),
	}
	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayloadWithVersion(t, "alpha", initialVersion)); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	harness := startRunningSessionServiceOnDir(t, rootDir)
	defer harness.stop(t)

	created, err := harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		serviceNamedFactoryContract(t, "beta"),
	)
	if err != nil {
		t.Fatalf("SaveFactoryForSession(upsert create beta): %v", err)
	}
	if created.Version == nil {
		t.Fatal("expected created beta version metadata")
	}

	onDiskVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  created.Version.Logical + 1,
		Physical: created.Version.Physical.Add(time.Second),
	}
	if _, err := config.ReplaceNamedFactory(rootDir, "beta", serviceNamedFactoryPayloadWithVersion(t, "beta", onDiskVersion)); err != nil {
		t.Fatalf("ReplaceNamedFactory(beta newer on-disk version): %v", err)
	}

	stale := serviceNamedFactoryContractWithWorkType(t, "beta", "story")
	stale.Version = created.Version
	_, err = harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		stale,
	)
	if !errors.Is(err, apisurface.ErrFactoryVersionStale) {
		t.Fatalf("SaveFactoryForSession(upsert replace stale) error = %v, want stale version", err)
	}

	fresh := serviceNamedFactoryContractWithWorkType(t, "beta", "story")
	fresh.Version = &factoryapi.HybridLogicalTimestamp{
		Logical:  onDiskVersion.Logical + 1,
		Physical: onDiskVersion.Physical.Add(time.Second),
	}
	replaced, err := harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		fresh,
	)
	if err != nil {
		t.Fatalf("SaveFactoryForSession(upsert replace fresh): %v", err)
	}
	assertFactoryWorkType(t, replaced.WorkTypes, "story", "replaced beta work types")
	assertFactoryVersionAdvanced(t, replaced.Version, onDiskVersion)
}

func TestFactoryService_SaveFactoryForSession_UpsertReplaceDoesNotReturnAlreadyExists(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha"},
	})
	defer harness.stop(t)

	created, err := harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		serviceNamedFactoryContract(t, "beta"),
	)
	if err != nil {
		t.Fatalf("SaveFactoryForSession(upsert create beta): %v", err)
	}
	if created.Version == nil {
		t.Fatal("expected created factory version metadata")
	}

	replacement := serviceNamedFactoryContractWithWorkType(t, "beta", "story")
	replacement.Version = &factoryapi.HybridLogicalTimestamp{
		Logical:  created.Version.Logical + 1,
		Physical: created.Version.Physical.Add(time.Second),
	}
	replaced, err := harness.svc.SaveFactoryForSession(
		context.Background(),
		defaultFactorySessionID,
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		replacement,
	)
	if errors.Is(err, config.ErrNamedFactoryAlreadyExists) {
		t.Fatalf("SaveFactoryForSession(upsert replace beta) error = %v, want replace not FACTORY_ALREADY_EXISTS", err)
	}
	if err != nil {
		t.Fatalf("SaveFactoryForSession(upsert replace beta): %v", err)
	}
	assertFactoryWorkType(t, replaced.WorkTypes, "story", "replaced beta work types")
	if replaced.Version == nil {
		t.Fatal("expected replaced factory version metadata")
	}
	assertFactoryVersionAdvanced(t, replaced.Version, *created.Version)
}
