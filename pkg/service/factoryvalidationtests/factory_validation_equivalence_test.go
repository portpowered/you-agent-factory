package factoryvalidationtests

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"go.uber.org/zap"
)

func TestFactoryValidation_EquivalentCanonicalTargetsAcrossPackageConfigAndSavePaths(t *testing.T) {
	t.Parallel()

	factory, err := factoryvalidation.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}
	cfg, err := config.FactoryConfigFromOpenAPI(factory)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}

	explicit := factoryvalidation.Validate(&cfg)
	packageSignatures := factoryvalidation.CanonicalTargetSignatures(explicit.Targets)
	configFindings := config.CanonicalStructuralFindings(&cfg)
	if len(configFindings) != len(explicit.Targets) {
		t.Fatalf("config findings = %d, package targets = %d, want equivalent coverage",
			len(configFindings), len(explicit.Targets))
	}
	for index, target := range explicit.Targets {
		if configFindings[index].Rule != target.Code {
			t.Fatalf("config finding[%d].Rule = %q, want package target code %q",
				index, configFindings[index].Rule, target.Code)
		}
	}

	saveSignatures := canonicalTargetsFromEditableSaveRejection(t, factory)
	if !factoryvalidation.EquivalentCanonicalTargetSignatures(packageSignatures, saveSignatures) {
		t.Fatalf("package signatures = %#v, save signatures = %#v, want equivalent canonical targets",
			packageSignatures, saveSignatures)
	}

	validatorResult := config.NewConfigValidator().Validate(&cfg)
	for _, finding := range configFindings {
		assertConfigFindingExists(t, validatorResult.Findings, finding.Rule)
	}
}

func canonicalTargetsFromEditableSaveRejection(t *testing.T, invalid factoryapi.Factory) []string {
	t.Helper()

	rootDir := t.TempDir()
	initialVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  11,
		Physical: time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC),
	}
	var validPayload map[string]any
	if err := json.Unmarshal([]byte(factoryvalidation.CrossPathValidAlphaFactoryJSON), &validPayload); err != nil {
		t.Fatalf("unmarshal valid alpha fixture: %v", err)
	}
	validPayload["version"] = map[string]any{
		"logical":  initialVersion.Logical,
		"physical": initialVersion.Physical.UTC().Format(time.RFC3339Nano),
	}
	payload, err := json.Marshal(validPayload)
	if err != nil {
		t.Fatalf("marshal valid alpha payload: %v", err)
	}
	if _, err := config.PersistNamedFactory(rootDir, "alpha", payload); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- svc.Run(runCtx)
	}()
	t.Cleanup(func() {
		cancelRun()
		select {
		case err := <-runDone:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("factory service run: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for factory service run to stop")
		}
	})

	waitDeadline := time.Now().Add(time.Second)
	for time.Now().Before(waitDeadline) {
		snap, snapErr := svc.GetEngineStateSnapshotForSession(context.Background(), factorysessions.DefaultSessionID)
		if snapErr == nil && snap.RuntimeStatus == interfaces.RuntimeStatusIdle {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	current, err := svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory: %v", err)
	}
	if current.Version == nil {
		t.Fatal("expected current factory version metadata")
	}

	invalid.Name = factoryapi.FactoryName("alpha")
	invalid.Version = &factoryapi.HybridLogicalTimestamp{
		Logical:  current.Version.Logical + 1,
		Physical: current.Version.Physical.Add(time.Second),
	}

	_, err = svc.SaveFactoryForSession(
		context.Background(),
		factorysessions.DefaultSessionID,
		factoryapi.FactorySaveModeReplaceCurrent,
		invalid,
	)
	var topologyErr *apisurface.TopologyValidationError
	if !errors.As(err, &topologyErr) {
		t.Fatalf("SaveFactoryForSession error = %v, want topology validation error", err)
	}
	return factoryvalidation.CanonicalAPITargetSignatures(topologyErr.Targets)
}

func assertConfigFindingExists(t *testing.T, findings []config.Finding, rule string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Rule == rule {
			return
		}
	}
	t.Fatalf("config findings = %#v, want rule %q", findings, rule)
}
