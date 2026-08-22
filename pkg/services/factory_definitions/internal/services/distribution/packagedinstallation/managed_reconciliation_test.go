package packagedinstallation

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestEnsurePackagedFactories_ManagedInstallIsCurrentAndAdoptsEquivalentLegacyContent(t *testing.T) {
	t.Parallel()

	definition := publishedManagedTestDefinition(t)
	root := t.TempDir()
	installer := New(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir)
	legacy, err := installer.InstallPackagedFactory(t.Context(), factorydefinitions.PackagedFactoryInstallParams{
		NamedFactoriesRoot: root,
		Definition:         definition,
		Format:             factorydefinitions.PackagedFactoryFormatJSON,
	})
	if err != nil {
		t.Fatalf("legacy installation: %v", err)
	}
	legacySnapshot := snapshotDirectoryContents(t, legacy.FactoryDir)

	adopted, err := installer.EnsurePackagedFactories(t.Context(), root, "managed-test", []factorydefinitions.PackagedDefinition{definition})
	if err != nil {
		t.Fatalf("adopt legacy installation: %v", err)
	}
	if len(adopted) != 1 || adopted[0].Outcome != factorydefinitions.PackagedFactoryInstallCurrent {
		t.Fatalf("legacy adoption = %#v, want one current result", adopted)
	}
	if adopted[0].BackupDir != "" {
		t.Fatalf("legacy adoption backup = %q, want no replacement backup", adopted[0].BackupDir)
	}
	if _, err := os.Stat(filepath.Join(legacy.FactoryDir, managedStampName)); err != nil {
		t.Fatalf("managed stamp after legacy adoption: %v", err)
	}
	if got := snapshotDirectoryContents(t, legacy.FactoryDir); len(got) != len(legacySnapshot)+1 {
		t.Fatalf("legacy adoption changed %d content entries, want only management evidence added", len(got)-len(legacySnapshot))
	}

	beforeCurrent := snapshotDirectoryContents(t, legacy.FactoryDir)
	current, err := installer.EnsurePackagedFactories(t.Context(), root, "managed-test", []factorydefinitions.PackagedDefinition{definition})
	if err != nil {
		t.Fatalf("repeat ensure: %v", err)
	}
	if current[0].Outcome != factorydefinitions.PackagedFactoryInstallCurrent {
		t.Fatalf("repeat ensure outcome = %q, want current", current[0].Outcome)
	}
	assertDirectorySnapshotUnchanged(t, legacy.FactoryDir, beforeCurrent)
}

func TestEnsurePackagedFactories_ManagedRefreshPreservesStaleActiveDirectory(t *testing.T) {
	t.Parallel()

	definition := publishedManagedTestDefinition(t)
	root := t.TempDir()
	installer := New(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir)
	created, err := installer.EnsurePackagedFactories(t.Context(), root, "managed-test", []factorydefinitions.PackagedDefinition{definition})
	if err != nil {
		t.Fatalf("initial ensure: %v", err)
	}
	if created[0].Outcome != factorydefinitions.PackagedFactoryInstallCreated {
		t.Fatalf("initial outcome = %q, want created", created[0].Outcome)
	}
	oldFactory, err := os.ReadFile(filepath.Join(created[0].FactoryDir, factorydefinitions.FactoryConfigFile))
	if err != nil {
		t.Fatalf("read initial Factory: %v", err)
	}

	updated := definition
	updated.JSON = bytes.Replace(
		updated.JSON,
		[]byte("Persists goal progress"),
		[]byte("Updated packaged goal progress"),
		1,
	)
	if bytes.Equal(updated.JSON, definition.JSON) {
		t.Fatal("test definition did not change")
	}
	refreshed, err := installer.EnsurePackagedFactories(t.Context(), root, "managed-test", []factorydefinitions.PackagedDefinition{updated})
	if err != nil {
		t.Fatalf("stale refresh: %v", err)
	}
	if refreshed[0].Outcome != factorydefinitions.PackagedFactoryInstallRefreshed {
		t.Fatalf("refresh outcome = %q, want refreshed", refreshed[0].Outcome)
	}
	if refreshed[0].BackupDir == "" {
		t.Fatal("refresh backup path is empty")
	}
	backupFactory, err := os.ReadFile(filepath.Join(refreshed[0].BackupDir, factorydefinitions.FactoryConfigFile))
	if err != nil {
		t.Fatalf("read stale backup: %v", err)
	}
	if !bytes.Equal(backupFactory, oldFactory) {
		t.Fatal("stale backup does not preserve the complete prior Factory root file")
	}
	activeFactory, err := os.ReadFile(filepath.Join(refreshed[0].FactoryDir, factorydefinitions.FactoryConfigFile))
	if err != nil {
		t.Fatalf("read refreshed Factory: %v", err)
	}
	if !bytes.Contains(activeFactory, []byte("Updated packaged goal progress")) {
		t.Fatal("active Factory does not contain the current packaged definition")
	}
	if _, err := os.Stat(refreshed[0].BackupDir); err != nil {
		t.Fatalf("preserved backup stat: %v", err)
	}
}

func TestEnsurePackagedFactories_ManagedCustomerModificationIsReportedAndPreserved(t *testing.T) {
	t.Parallel()

	definition := publishedManagedTestDefinition(t)
	root := t.TempDir()
	installer := New(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir)
	created, err := installer.EnsurePackagedFactories(t.Context(), root, "managed-test", []factorydefinitions.PackagedDefinition{definition})
	if err != nil {
		t.Fatalf("initial ensure: %v", err)
	}
	marker := filepath.Join(created[0].FactoryDir, "customer-owned.txt")
	if err := os.WriteFile(marker, []byte("keep this edit"), 0o600); err != nil {
		t.Fatalf("write customer edit: %v", err)
	}

	refreshed, err := installer.EnsurePackagedFactories(t.Context(), root, "managed-test", []factorydefinitions.PackagedDefinition{definition})
	if err != nil {
		t.Fatalf("customer-modified refresh: %v", err)
	}
	if refreshed[0].Outcome != factorydefinitions.PackagedFactoryInstallCustomerModified {
		t.Fatalf("customer-modified outcome = %q, want customer-modified", refreshed[0].Outcome)
	}
	if refreshed[0].BackupDir == "" {
		t.Fatal("customer-modified backup path is empty")
	}
	backupMarker, err := os.ReadFile(filepath.Join(refreshed[0].BackupDir, "customer-owned.txt"))
	if err != nil || string(backupMarker) != "keep this edit" {
		t.Fatalf("preserved customer edit = %q, %v", backupMarker, err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("active customer edit stat = %v, want removed from refreshed active directory", err)
	}
}

func TestEnsurePackagedFactories_ManagedReplacementFailurePreservesActiveAndCleansBackup(t *testing.T) {
	t.Parallel()

	definition := publishedManagedTestDefinition(t)
	root := t.TempDir()
	persistence := &failingManagedReplacementPersistence{
		PackagedFactoryPersistence: packagedInstallationTestPersistence(),
		replaceErr:                 errors.New("replacement unavailable"),
	}
	installer := New(persistence, platformfilesystem.Local{}, os.Mkdir)
	created, err := installer.EnsurePackagedFactories(t.Context(), root, "managed-test", []factorydefinitions.PackagedDefinition{definition})
	if err != nil {
		t.Fatalf("initial ensure: %v", err)
	}
	before := snapshotDirectoryContents(t, created[0].FactoryDir)
	updated := definition
	updated.JSON = bytes.Replace(updated.JSON, []byte("Persists goal progress"), []byte("Replacement failure content"), 1)

	failed, err := installer.EnsurePackagedFactories(t.Context(), root, "managed-test", []factorydefinitions.PackagedDefinition{updated})
	if err == nil || !strings.Contains(err.Error(), "replacement unavailable") {
		t.Fatalf("failed refresh error = %v, want replacement failure", err)
	}
	if len(failed) != 1 || failed[0].Outcome != factorydefinitions.PackagedFactoryInstallFailed {
		t.Fatalf("failed refresh result = %#v, want one failed result", failed)
	}
	assertDirectorySnapshotUnchanged(t, created[0].FactoryDir, before)
	backupRoot := filepath.Join(root, managedBackupRoot)
	entries, readErr := os.ReadDir(backupRoot)
	if readErr == nil && len(entries) != 0 {
		t.Fatalf("backup entries after failed replacement = %v, want none", entries)
	}
}

func TestEnsurePackagedFactories_ConcurrentManagedRefreshesConverge(t *testing.T) {
	t.Parallel()

	definition := publishedManagedTestDefinition(t)
	root := t.TempDir()
	persistence := &blockingManagedReplacementPersistence{
		PackagedFactoryPersistence: packagedInstallationTestPersistence(),
		replacementStarted:         make(chan struct{}),
		allowReplacement:           make(chan struct{}),
	}
	installer := New(persistence, platformfilesystem.Local{}, os.Mkdir)
	if _, err := installer.EnsurePackagedFactories(t.Context(), root, "managed-test", []factorydefinitions.PackagedDefinition{definition}); err != nil {
		t.Fatalf("initial ensure: %v", err)
	}
	updated := definition
	updated.JSON = bytes.Replace(updated.JSON, []byte("Persists goal progress"), []byte("Concurrent refresh content"), 1)

	type ensureResult struct {
		results []factorydefinitions.PackagedFactoryInstallResult
		err     error
	}
	firstDone := make(chan ensureResult, 1)
	go func() {
		results, err := installer.EnsurePackagedFactories(t.Context(), root, "managed-test", []factorydefinitions.PackagedDefinition{updated})
		firstDone <- ensureResult{results: results, err: err}
	}()
	select {
	case <-persistence.replacementStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for first managed replacement")
	}
	secondDone := make(chan ensureResult, 1)
	go func() {
		results, err := installer.EnsurePackagedFactories(t.Context(), root, "managed-test", []factorydefinitions.PackagedDefinition{updated})
		secondDone <- ensureResult{results: results, err: err}
	}()
	close(persistence.allowReplacement)

	var completed []ensureResult
	for range 2 {
		select {
		case result := <-firstDone:
			completed = append(completed, result)
			firstDone = nil
		case result := <-secondDone:
			completed = append(completed, result)
			secondDone = nil
		case <-time.After(10 * time.Second):
			t.Fatal("timed out waiting for concurrent managed refreshes")
		}
	}
	for _, result := range completed {
		if result.err != nil {
			t.Fatalf("concurrent ensure error: %v", result.err)
		}
		if len(result.results) != 1 {
			t.Fatalf("concurrent ensure results = %#v, want one result", result.results)
		}
		if result.results[0].Outcome != factorydefinitions.PackagedFactoryInstallRefreshed &&
			result.results[0].Outcome != factorydefinitions.PackagedFactoryInstallCurrent {
			t.Fatalf("concurrent ensure outcome = %q, want refreshed or current", result.results[0].Outcome)
		}
	}
}

type failingManagedReplacementPersistence struct {
	factorydefinitions.PackagedFactoryPersistence
	replaceErr error
}

type blockingManagedReplacementPersistence struct {
	factorydefinitions.PackagedFactoryPersistence
	replacementStarted chan struct{}
	allowReplacement   chan struct{}
	replacementOnce    sync.Once
}

func (persistence *blockingManagedReplacementPersistence) ReplaceFactoryLayout(
	targetDir string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	persistence.replacementOnce.Do(func() { close(persistence.replacementStarted) })
	<-persistence.allowReplacement
	return persistence.PackagedFactoryPersistence.ReplaceFactoryLayout(targetDir, prepared)
}

func (persistence *failingManagedReplacementPersistence) ReplaceFactoryLayout(
	string,
	*factorydefinitions.PreparedFactoryLayoutPayload,
) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	return nil, persistence.replaceErr
}

func publishedManagedTestDefinition(t *testing.T) factorydefinitions.PackagedDefinition {
	t.Helper()
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("load packaged catalog: %v", err)
	}
	definition, ok := catalog.Lookup("@you/goal")
	if !ok {
		t.Fatal("packaged catalog is missing @you/goal")
	}
	return definition
}
