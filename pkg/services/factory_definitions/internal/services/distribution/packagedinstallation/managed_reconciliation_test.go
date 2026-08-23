package packagedinstallation

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
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

func TestEnsurePackagedFactories_ManagedInstallReplacesInvalidEvidenceWithoutReplacingContent(t *testing.T) {
	t.Parallel()

	definition := publishedManagedTestDefinition(t)
	root := t.TempDir()
	installer := New(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir)
	created, err := installer.EnsurePackagedFactories(t.Context(), root, "managed-test", []factorydefinitions.PackagedDefinition{definition})
	if err != nil {
		t.Fatalf("initial ensure: %v", err)
	}
	if err := os.WriteFile(filepath.Join(created[0].FactoryDir, managedStampName), []byte("not-json"), 0o600); err != nil {
		t.Fatalf("write invalid management evidence: %v", err)
	}

	current, err := installer.EnsurePackagedFactories(t.Context(), root, "managed-test", []factorydefinitions.PackagedDefinition{definition})
	if err != nil {
		t.Fatalf("ensure with invalid management evidence: %v", err)
	}
	if len(current) != 1 || current[0].Outcome != factorydefinitions.PackagedFactoryInstallCurrent {
		t.Fatalf("invalid evidence outcome = %#v, want one current result", current)
	}
	if current[0].BackupDir != "" {
		t.Fatalf("invalid evidence backup = %q, want no replacement backup", current[0].BackupDir)
	}
	if _, err := os.Stat(filepath.Join(created[0].FactoryDir, managedStampName)); err != nil {
		t.Fatalf("rewritten management evidence: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(created[0].FactoryDir, managedStampName),
		[]byte(`{"version":0,"factoryName":"@you/goal","publishedContentId":"published","installedContentId":"installed"}`),
		0o600,
	); err != nil {
		t.Fatalf("write obsolete management evidence: %v", err)
	}
	current, err = installer.EnsurePackagedFactories(t.Context(), root, "managed-test", []factorydefinitions.PackagedDefinition{definition})
	if err != nil || len(current) != 1 || current[0].Outcome != factorydefinitions.PackagedFactoryInstallCurrent {
		t.Fatalf("obsolete evidence outcome = %#v, %v, want one current result", current, err)
	}
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

func TestEnsurePackagedFactories_ManagedNilReplacementPreservesActiveAndCleansBackup(t *testing.T) {
	t.Parallel()

	definition := publishedManagedTestDefinition(t)
	root := t.TempDir()
	basePersistence := packagedInstallationTestPersistence()
	installer := New(basePersistence, platformfilesystem.Local{}, os.Mkdir)
	created, err := installer.EnsurePackagedFactories(t.Context(), root, "managed-test", []factorydefinitions.PackagedDefinition{definition})
	if err != nil {
		t.Fatalf("initial ensure: %v", err)
	}
	before := snapshotDirectoryContents(t, created[0].FactoryDir)
	updated := definition
	updated.JSON = bytes.Replace(updated.JSON, []byte("Persists goal progress"), []byte("nil replacement content"), 1)

	failed, err := New(
		&nilManagedReplacementPersistence{PackagedFactoryPersistence: basePersistence},
		platformfilesystem.Local{},
		os.Mkdir,
	).EnsurePackagedFactories(t.Context(), root, "managed-test", []factorydefinitions.PackagedDefinition{updated})
	if err == nil || !strings.Contains(err.Error(), "replacement result is required") {
		t.Fatalf("nil replacement error = %v, want required-result error", err)
	}
	if len(failed) != 1 || failed[0].Outcome != factorydefinitions.PackagedFactoryInstallFailed {
		t.Fatalf("nil replacement result = %#v, want one failed result", failed)
	}
	assertDirectorySnapshotUnchanged(t, created[0].FactoryDir, before)
	backupEntries, readErr := os.ReadDir(filepath.Join(root, managedBackupRoot))
	if readErr == nil && len(backupEntries) != 0 {
		t.Fatalf("backup entries after nil replacement = %v, want none", backupEntries)
	}
}

func TestEnsurePackagedFactories_ManagedStampPublicationFailureReportsFailedOutcome(t *testing.T) {
	t.Parallel()

	definition := publishedManagedTestDefinition(t)
	root := t.TempDir()
	fileSystem := &managedStampFailureFileSystem{Local: platformfilesystem.Local{}}
	installer := New(packagedInstallationTestPersistence(), fileSystem, os.Mkdir)
	created, err := installer.EnsurePackagedFactories(t.Context(), root, "managed-test", []factorydefinitions.PackagedDefinition{definition})
	if err != nil {
		t.Fatalf("initial ensure: %v", err)
	}
	fileSystem.failStampRename = true
	updated := definition
	updated.JSON = bytes.Replace(updated.JSON, []byte("Persists goal progress"), []byte("stamp publication failure content"), 1)

	failed, err := installer.EnsurePackagedFactories(t.Context(), root, "managed-test", []factorydefinitions.PackagedDefinition{updated})
	if err == nil || !strings.Contains(err.Error(), "publish packaged Factory management evidence") {
		t.Fatalf("stamp publication error = %v, want management-evidence error", err)
	}
	if len(failed) != 1 || failed[0].Outcome != factorydefinitions.PackagedFactoryInstallFailed {
		t.Fatalf("stamp publication result = %#v, want one failed result", failed)
	}
	active, readErr := os.ReadFile(filepath.Join(created[0].FactoryDir, factorydefinitions.FactoryConfigFile))
	if readErr != nil || !bytes.Contains(active, []byte("stamp publication failure content")) {
		t.Fatalf("active Factory after stamp failure = %q, %v, want current payload", active, readErr)
	}
}

func TestWaitForManagedRetryHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := waitForManagedRetry(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForManagedRetry() error = %v, want context.Canceled", err)
	}
}

func TestWriteManagedStampFailureCleansTemporaryEvidence(t *testing.T) {
	t.Parallel()

	targetDir := t.TempDir()
	fileSystem := &failingPackagedInstallationFileSystem{
		Local:        platformfilesystem.Local{},
		writeFileErr: errors.New("management evidence write unavailable"),
	}
	service := New(packagedInstallationTestPersistence(), fileSystem, os.Mkdir)
	if err := service.writeManagedStamp(t.Context(), targetDir, "@you/goal", "published", "installed"); err == nil {
		t.Fatal("writeManagedStamp() error = nil, want write failure")
	}
	if _, err := os.Stat(filepath.Join(targetDir, managedStampTemp)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary management evidence stat = %v, want absent", err)
	}
}

func TestWriteManagedStampCancellationCleansTemporaryEvidence(t *testing.T) {
	t.Parallel()

	targetDir := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	fileSystem := &cancelAfterManagedWriteFileSystem{
		Local:  platformfilesystem.Local{},
		cancel: cancel,
	}
	service := New(packagedInstallationTestPersistence(), fileSystem, os.Mkdir)
	err := service.writeManagedStamp(ctx, targetDir, "@you/goal", "published", "installed")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("writeManagedStamp() error = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, managedStampTemp)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary management evidence stat = %v, want absent", err)
	}
}

func TestContentIdentityReportsFilesystemInspectionFailures(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	statFailure := errors.New("content stat unavailable")
	fileSystem := &failingPackagedInstallationFileSystem{
		Local:    platformfilesystem.Local{},
		statPath: root,
		statErr:  statFailure,
	}
	service := New(packagedInstallationTestPersistence(), fileSystem, os.Mkdir)
	if _, err := service.contentIdentity(root); !errors.Is(err, statFailure) {
		t.Fatalf("contentIdentity() stat error = %v, want %v", err, statFailure)
	}
}

func TestEnsurePackagedFactories_ManagedStampReadFailureIsActionable(t *testing.T) {
	t.Parallel()

	definition := publishedManagedTestDefinition(t)
	root := t.TempDir()
	fileSystem := &managedStampReadFailureFileSystem{Local: platformfilesystem.Local{}}
	installer := New(packagedInstallationTestPersistence(), fileSystem, os.Mkdir)
	if _, err := installer.EnsurePackagedFactories(t.Context(), root, "managed-test", []factorydefinitions.PackagedDefinition{definition}); err != nil {
		t.Fatalf("initial ensure: %v", err)
	}
	fileSystem.failStampRead = true
	_, err := installer.EnsurePackagedFactories(t.Context(), root, "managed-test", []factorydefinitions.PackagedDefinition{definition})
	if err == nil || !strings.Contains(err.Error(), "read packaged Factory management evidence") {
		t.Fatalf("management evidence read error = %v, want actionable read error", err)
	}
}

func TestEnsurePackagedFactories_ManagedPreparationRequiresPreparedLayout(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	definition := publishedManagedTestDefinition(t)
	installer := New(
		&nilManagedPreparationPersistence{PackagedFactoryPersistence: packagedInstallationTestPersistence()},
		platformfilesystem.Local{},
		os.Mkdir,
	)
	_, err := installer.EnsurePackagedFactories(t.Context(), root, "managed-test", []factorydefinitions.PackagedDefinition{definition})
	if err == nil || !strings.Contains(err.Error(), "prepared Factory layout is required") {
		t.Fatalf("nil prepared layout error = %v, want required-layout error", err)
	}
}

func TestEnsurePackagedFactories_ManagedBackupReservationFailureIsActionable(t *testing.T) {
	t.Parallel()

	definition := publishedManagedTestDefinition(t)
	root := t.TempDir()
	directoryCreator := func(path string, mode fs.FileMode) error {
		if strings.Contains(path, managedBackupRoot) {
			return errors.New("backup reservation unavailable")
		}
		return os.Mkdir(path, mode)
	}
	installer := New(packagedInstallationTestPersistence(), platformfilesystem.Local{}, directoryCreator)
	created, err := installer.EnsurePackagedFactories(t.Context(), root, "managed-test", []factorydefinitions.PackagedDefinition{definition})
	if err != nil {
		t.Fatalf("initial ensure: %v", err)
	}
	updated := definition
	updated.JSON = bytes.Replace(updated.JSON, []byte("Persists goal progress"), []byte("backup reservation failure content"), 1)
	_, err = installer.EnsurePackagedFactories(t.Context(), root, "managed-test", []factorydefinitions.PackagedDefinition{updated})
	if err == nil || !strings.Contains(err.Error(), "reserve packaged Factory backup") {
		t.Fatalf("backup reservation error = %v, want actionable reservation error", err)
	}
	if _, statErr := os.Stat(created[0].FactoryDir); statErr != nil {
		t.Fatalf("active Factory after backup reservation failure: %v", statErr)
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

type nilManagedReplacementPersistence struct {
	factorydefinitions.PackagedFactoryPersistence
}

func (persistence *nilManagedReplacementPersistence) ReplaceFactoryLayout(
	string,
	*factorydefinitions.PreparedFactoryLayoutPayload,
) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	return nil, nil
}

type nilManagedPreparationPersistence struct {
	factorydefinitions.PackagedFactoryPersistence
}

func (persistence *nilManagedPreparationPersistence) PreparePackagedFactoryLayout(
	context.Context,
	string,
	[]byte,
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	return nil, nil
}

type managedStampReadFailureFileSystem struct {
	platformfilesystem.Local
	failStampRead bool
}

func (fileSystem *managedStampReadFailureFileSystem) ReadFile(path string) ([]byte, error) {
	if fileSystem.failStampRead && filepath.Base(path) == managedStampName {
		return nil, errors.New("management evidence read unavailable")
	}
	return fileSystem.Local.ReadFile(path)
}

type managedStampFailureFileSystem struct {
	platformfilesystem.Local
	failStampRename bool
}

type cancelAfterManagedWriteFileSystem struct {
	platformfilesystem.Local
	cancel context.CancelFunc
}

func (fileSystem *cancelAfterManagedWriteFileSystem) WriteFile(path string, data []byte, mode fs.FileMode) error {
	if err := fileSystem.Local.WriteFile(path, data, mode); err != nil {
		return err
	}
	fileSystem.cancel()
	return nil
}

func (fileSystem *managedStampFailureFileSystem) Rename(oldPath, newPath string) error {
	if fileSystem.failStampRename && filepath.Base(newPath) == managedStampName {
		return errors.New("management evidence rename unavailable")
	}
	return fileSystem.Local.Rename(oldPath, newPath)
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
