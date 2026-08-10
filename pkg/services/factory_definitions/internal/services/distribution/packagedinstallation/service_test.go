package packagedinstallation

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
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	"github.com/portpowered/infinite-you/pkg/platform/directoryreplace"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/inboxgitkeep"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryauthoredlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/authoredlayout"
	authoringlayoutpersist "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/persist"
	authoringlayoutprepare "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/prepare"
	factorypersistence "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/persistence"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/portableconfig"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

func packagedInstallationTestPersistence() factorydefinitions.Persistence {
	validator := factoryvalidation.New(nil)
	mapper := factorymapping.NewFactoryConfigMapper()
	fileSystem := platformfilesystem.Local{}
	writer := factoryauthoredlayout.NewWriter(
		authoredmapping.RenderWorkerAgentsMarkdown,
		authoredmapping.RenderWorkstationAgentsMarkdown,
		authoredmapping.RenderAgentsBody,
		factoryauthoredlayout.NewAgentsFileWriter(fileSystem),
		authoredmapping.SafeFactoryLayoutSegment,
		authoredmapping.SafePromptFilePath,
		fileSystem,
		inboxgitkeep.NewLocal(fileSystem),
	)
	persistence, err := factorypersistence.New(
		validator,
		func(payload []byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return validationentry.MapFactoryJSONForPersistence(payload, factorydefinitioncomposition.LoadCanonicalJSON)
		},
		func(
			ctx context.Context,
			segment string,
			payload []byte,
			validator factorydefinitions.Validator,
		) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
			return authoringlayoutprepare.FactoryLayout(
				ctx,
				segment,
				payload,
				validator,
				mapper.Expand,
				authoredmapping.AuthoredFactoryConfigForExpandedLayout,
				mapper.Flatten,
			)
		},
		func(
			targetDir string,
			prepared *factorydefinitions.PreparedFactoryLayoutPayload,
			sourcePath string,
		) error {
			return writer.WritePrepared(
				targetDir,
				prepared,
				sourcePath,
				portableconfig.NewMaterializer(platformfilesystem.Local{}),
				factorydefinitioncomposition.PruneRemovedDocs,
			)
		},
		func(targetDir string) error {
			_, err := factorydefinitioncomposition.LoadDirectory(targetDir, nil)
			return err
		},
		nil,
		nil,
		nil,
		platformfilesystem.Local{},
		factorydefinitioncomposition.NamedPaths().RequireDefinitionDir,
		directoryreplace.Local{},
	)
	if err != nil {
		panic(err)
	}
	return persistence
}

func TestEnsurePackagedFactories_InvalidPayloadDoesNotCommitTarget(t *testing.T) {
	root := t.TempDir()
	definition := factorydefinitions.PackagedDefinition{
		Name: "@test/invalid",
		JSON: []byte(`{"id":"invalid","workers":[`),
	}
	_, err := New(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir).
		EnsurePackagedFactories(t.Context(), root, "", []factorydefinitions.PackagedDefinition{definition})
	if err == nil || !strings.Contains(err.Error(), "install packaged factory") {
		t.Fatalf("EnsurePackagedFactories() error = %v", err)
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("committed entries = %v, want none", entries)
	}
}

func TestEnsurePackagedFactories_PreparationFailurePreservesExistingRoot(t *testing.T) {
	root := t.TempDir()
	marker := root + string(os.PathSeparator) + "customer-owned.txt"
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	definition := factorydefinitions.PackagedDefinition{Name: "@test/invalid", JSON: []byte(`{`)}
	if _, err := New(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir).
		EnsurePackagedFactories(t.Context(), root, "", []factorydefinitions.PackagedDefinition{definition}); err == nil {
		t.Fatal("EnsurePackagedFactories() error = nil")
	}
	content, err := os.ReadFile(marker)
	if err != nil || string(content) != "keep" {
		t.Fatalf("customer marker = %q, %v", content, err)
	}
}

func TestEnsurePackagedFactories_FailsClosedWithoutFileSystem(t *testing.T) {
	_, err := New(packagedInstallationTestPersistence(), nil, os.Mkdir).EnsurePackagedFactories(
		t.Context(),
		t.TempDir(),
		"",
		[]factorydefinitions.PackagedDefinition{{Name: "@test/missing-filesystem", JSON: []byte(`{}`)}},
	)
	if err == nil || !strings.Contains(err.Error(), "installation filesystem is required") {
		t.Fatalf("EnsurePackagedFactories() error = %v", err)
	}
}

func TestInstallPackagedFactory_PreExistingStagingReturnsBoundedContention(t *testing.T) {
	root := t.TempDir()
	name := "@test/contended"
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create Factory root: %v", err)
	}
	stagingPath := filepath.Join(root, authoringlayoutpersist.StagingDirectoryPrefix(name)+"owner")
	if err := os.Mkdir(stagingPath, 0o755); err != nil {
		t.Fatalf("create retained staging resource: %v", err)
	}

	_, err := New(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir).InstallPackagedFactory(
		t.Context(),
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: root,
			BackendScopeID:     "local-contention-scope",
			Definition: factorydefinitions.PackagedDefinition{
				Name: name,
				JSON: []byte(`{}`),
			},
			Format: factorydefinitions.PackagedFactoryFormatJSON,
		},
	)
	if err == nil {
		t.Fatal("InstallPackagedFactory() error = nil, want bounded contention")
	}
	if !errors.Is(err, factorydefinitions.ErrFactoryInstallationContention) {
		t.Fatalf("InstallPackagedFactory() error = %v, want ErrFactoryInstallationContention", err)
	}
	for _, want := range []string{
		stagingPath,
		"backend_scope_id=local-contention-scope",
		"outcome=indeterminate-contention",
		"owner_liveness=indeterminate",
		"verify no you process is still installing",
		"remove only " + stagingPath,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("InstallPackagedFactory() error = %q, want %q", err, want)
		}
	}
	if _, statErr := os.Stat(stagingPath); statErr != nil {
		t.Fatalf("retained staging resource was removed: %v", statErr)
	}
	targetPath := filepath.Join(root, "@test", "contended")
	if _, statErr := os.Stat(targetPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("target path stat error = %v, want target absent", statErr)
	}
}

func TestInstallPackagedFactory_LiveOwnerContentionPreservesLease(t *testing.T) {
	root := t.TempDir()
	persistence := &blockingPackagedInstallationPersistence{
		prepareStarted: make(chan struct{}),
		allowPrepare:   make(chan struct{}),
	}
	installer := New(persistence, platformfilesystem.Local{}, os.Mkdir)
	params := factorydefinitions.PackagedFactoryInstallParams{
		NamedFactoriesRoot: root,
		BackendScopeID:     "local-live-scope",
		Definition: factorydefinitions.PackagedDefinition{
			Name: "@test/live-owner",
			JSON: []byte(`{}`),
		},
		Format: factorydefinitions.PackagedFactoryFormatJSON,
	}
	firstErr := make(chan error, 1)
	go func() {
		_, err := installer.InstallPackagedFactory(t.Context(), params)
		firstErr <- err
	}()
	<-persistence.prepareStarted

	secondDone := make(chan error, 1)
	go func() {
		_, err := installer.InstallPackagedFactory(t.Context(), params)
		secondDone <- err
	}()
	secondInstallErr := <-secondDone
	if secondInstallErr == nil || !errors.Is(secondInstallErr, factorydefinitions.ErrFactoryInstallationContention) {
		t.Fatalf("live-owner successor error = %v, want typed contention", secondInstallErr)
	}
	for _, want := range []string{
		"outcome=active-contention",
		"owner_liveness=active",
		fmt.Sprintf("owner_pid=%d", os.Getpid()),
		"stop or verify that owner",
	} {
		if !strings.Contains(secondInstallErr.Error(), want) {
			t.Fatalf("live-owner successor error = %q, want %q", secondInstallErr, want)
		}
	}
	leasePath := stagingOwnershipPath(root, params.Definition.Name)
	if _, err := os.Stat(leasePath); err != nil {
		t.Fatalf("live owner lease stat error = %v, want retained lease", err)
	}
	close(persistence.allowPrepare)
	if err := <-firstErr; err != nil {
		t.Fatalf("live owner completed installation error = %v", err)
	}
	if _, err := os.Stat(leasePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("released lease stat error = %v, want lease removed", err)
	}
}

func TestInstallPackagedFactory_MalformedOwnerMetadataFailsClosed(t *testing.T) {
	root := t.TempDir()
	name := "@test/malformed-owner"
	leasePath := stagingOwnershipPath(root, name)
	if err := os.MkdirAll(leasePath, 0o755); err != nil {
		t.Fatalf("create retained owner lease: %v", err)
	}
	if err := os.WriteFile(filepath.Join(leasePath, stagingOwnerMetadataName), []byte(`{"pid":"reused"}`), 0o600); err != nil {
		t.Fatalf("write malformed owner metadata: %v", err)
	}

	_, err := New(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir).InstallPackagedFactory(
		t.Context(),
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: root,
			Definition: factorydefinitions.PackagedDefinition{
				Name: name,
				JSON: []byte(`{}`),
			},
			Format: factorydefinitions.PackagedFactoryFormatJSON,
		},
	)
	if err == nil || !errors.Is(err, factorydefinitions.ErrFactoryInstallationContention) {
		t.Fatalf("malformed owner error = %v, want typed contention", err)
	}
	for _, want := range []string{
		leasePath,
		"outcome=indeterminate-contention",
		"owner_liveness=indeterminate",
		"remove only " + leasePath,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("malformed owner error = %q, want %q", err, want)
		}
	}
	if _, statErr := os.Stat(leasePath); statErr != nil {
		t.Fatalf("malformed owner lease was removed: %v", statErr)
	}
}

func TestInstallPackagedFactory_AcquisitionRacePreservesWinnerLease(t *testing.T) {
	root := t.TempDir()
	fileSystem := &racingPackagedInstallationFileSystem{
		firstMkdirStarted: make(chan struct{}),
		releaseFirstMkdir: make(chan struct{}),
	}
	persistence := &blockingPackagedInstallationPersistence{
		prepareStarted: make(chan struct{}),
		allowPrepare:   make(chan struct{}),
	}
	installer := New(persistence, fileSystem, fileSystem.Mkdir)
	params := factorydefinitions.PackagedFactoryInstallParams{
		NamedFactoriesRoot: root,
		Definition: factorydefinitions.PackagedDefinition{
			Name: "@test/acquisition-race",
			JSON: []byte(`{}`),
		},
		Format: factorydefinitions.PackagedFactoryFormatJSON,
	}
	firstDone := make(chan error, 1)
	go func() {
		_, err := installer.InstallPackagedFactory(t.Context(), params)
		firstDone <- err
	}()
	<-fileSystem.firstMkdirStarted

	secondDone := make(chan error, 1)
	go func() {
		_, err := installer.InstallPackagedFactory(t.Context(), params)
		secondDone <- err
	}()
	<-persistence.prepareStarted
	close(fileSystem.releaseFirstMkdir)
	firstInstallErr := <-firstDone
	if firstInstallErr == nil || !errors.Is(firstInstallErr, factorydefinitions.ErrFactoryInstallationContention) {
		t.Fatalf("racing loser error = %v, want typed contention", firstInstallErr)
	}
	for _, want := range []string{
		"outcome=active-contention",
		"owner_liveness=active",
		"owner_pid=",
	} {
		if !strings.Contains(firstInstallErr.Error(), want) {
			t.Fatalf("racing loser error = %q, want %q", firstInstallErr, want)
		}
	}
	leasePath := stagingOwnershipPath(root, params.Definition.Name)
	if _, err := os.Stat(leasePath); err != nil {
		t.Fatalf("winner lease stat error = %v, want retained lease while winner runs", err)
	}
	close(persistence.allowPrepare)
	if err := <-secondDone; err != nil {
		t.Fatalf("racing winner installation error = %v", err)
	}
	if _, err := os.Stat(leasePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("winner lease stat error = %v, want released lease", err)
	}
}

func TestInstallPackagedFactory_ConcurrentOrphanReclaimPreservesWinnerLease(t *testing.T) {
	root := t.TempDir()
	name := "@test/orphan-reclaim-race"
	stagingPath := stagingOwnershipPath(root, name)
	if err := os.MkdirAll(stagingPath, 0o755); err != nil {
		t.Fatalf("create orphan staging path: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stagingPath, stagingOwnerMetadataName), []byte(`{"pid":404}`), 0o600); err != nil {
		t.Fatalf("write orphan owner metadata: %v", err)
	}
	fileSystem := &orphanReclaimRaceFileSystem{
		Local:              platformfilesystem.Local{},
		stagingPath:        stagingPath,
		firstReclaimReady:  make(chan struct{}),
		secondReclaimReady: make(chan struct{}),
		releaseFirst:       make(chan struct{}),
		releaseSecond:      make(chan struct{}),
	}
	persistence := &blockingPackagedInstallationPersistence{
		prepareStarted: make(chan struct{}),
		allowPrepare:   make(chan struct{}),
	}
	params := factorydefinitions.PackagedFactoryInstallParams{
		NamedFactoriesRoot: root,
		BackendScopeID:     "orphan-race-scope",
		Definition: factorydefinitions.PackagedDefinition{
			Name: name,
			JSON: []byte(`{}`),
		},
		Format: factorydefinitions.PackagedFactoryFormatJSON,
	}
	first := newWithOwnerProbe(
		persistence,
		fileSystem,
		os.Mkdir,
		&scriptedOwnerProbe{record: ownerRecord{PID: 101}, liveness: ownerLivenessOrphaned},
	)
	second := newWithOwnerProbe(
		persistence,
		fileSystem,
		os.Mkdir,
		&scriptedOwnerProbe{record: ownerRecord{PID: 202}, liveness: ownerLivenessOrphaned},
	)
	done := make(chan error, 2)
	go func() {
		_, err := first.InstallPackagedFactory(t.Context(), params)
		done <- err
	}()
	go func() {
		_, err := second.InstallPackagedFactory(t.Context(), params)
		done <- err
	}()

	<-fileSystem.firstReclaimReady
	<-fileSystem.secondReclaimReady
	close(fileSystem.releaseFirst)
	<-persistence.prepareStarted

	var recoveredPath string
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read root while winner is held: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "-recovered-") {
			recoveredPath = filepath.Join(root, entry.Name())
			break
		}
	}
	if recoveredPath == "" {
		t.Fatalf("winner recovery lease was not published; entries = %v", entries)
	}
	ownerData, err := os.ReadFile(filepath.Join(recoveredPath, stagingOwnerMetadataName))
	if err != nil {
		t.Fatalf("read winner owner metadata: %v", err)
	}
	var winner ownerRecord
	if err := json.Unmarshal(ownerData, &winner); err != nil {
		t.Fatalf("decode winner owner metadata: %v", err)
	}
	if winner.PID != 101 && winner.PID != 202 {
		t.Fatalf("winner owner PID = %d, want one of the contenders", winner.PID)
	}
	if _, err := os.Stat(stagingPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("original orphan path stat = %v, want absent", err)
	}

	close(fileSystem.releaseSecond)
	loserErr := <-done
	if loserErr == nil || !errors.Is(loserErr, factorydefinitions.ErrFactoryInstallationContention) {
		t.Fatalf("delayed reclaimer error = %v, want typed contention", loserErr)
	}
	for _, want := range []string{"outcome=indeterminate-contention", "owner_liveness=racing", stagingPath} {
		if !strings.Contains(loserErr.Error(), want) {
			t.Fatalf("delayed reclaimer error = %q, want %q", loserErr, want)
		}
	}
	if _, err := os.Stat(filepath.Join(recoveredPath, stagingOwnerMetadataName)); err != nil {
		t.Fatalf("winner owner metadata after delayed reclaimer = %v, want preserved", err)
	}

	close(persistence.allowPrepare)
	if winnerErr := <-done; winnerErr != nil {
		t.Fatalf("winning installation error = %v", winnerErr)
	}
	if _, err := os.Stat(recoveredPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("winner recovery lease stat = %v, want released", err)
	}
}

type blockingPackagedInstallationPersistence struct {
	factorydefinitions.Persistence
	prepareStarted chan struct{}
	allowPrepare   chan struct{}
	prepareOnce    sync.Once
}

func (p *blockingPackagedInstallationPersistence) PrepareFactoryLayout(
	context.Context,
	string,
	[]byte,
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	p.prepareOnce.Do(func() { close(p.prepareStarted) })
	<-p.allowPrepare
	return &factorydefinitions.PreparedFactoryLayoutPayload{}, nil
}

func (p *blockingPackagedInstallationPersistence) CreateNamedFactory(
	rootDir string,
	name string,
	_ *factorydefinitions.PreparedFactoryLayoutPayload,
) (string, error) {
	return filepath.Join(rootDir, name), nil
}

type racingPackagedInstallationFileSystem struct {
	platformfilesystem.Local
	firstMkdirStarted chan struct{}
	releaseFirstMkdir chan struct{}
	mu                sync.Mutex
	mkdirCalls        int
}

type orphanReclaimRaceFileSystem struct {
	platformfilesystem.Local
	stagingPath        string
	firstReclaimReady  chan struct{}
	secondReclaimReady chan struct{}
	releaseFirst       chan struct{}
	releaseSecond      chan struct{}
	reclaimCalls       atomic.Int32
}

func (fileSystem *orphanReclaimRaceFileSystem) Rename(oldPath, newPath string) error {
	if oldPath != fileSystem.stagingPath {
		return fileSystem.Local.Rename(oldPath, newPath)
	}
	switch fileSystem.reclaimCalls.Add(1) {
	case 1:
		close(fileSystem.firstReclaimReady)
		<-fileSystem.releaseFirst
	case 2:
		close(fileSystem.secondReclaimReady)
		<-fileSystem.releaseSecond
	}
	return fileSystem.Local.Rename(oldPath, newPath)
}

func (fileSystem *racingPackagedInstallationFileSystem) Mkdir(path string, mode fs.FileMode) error {
	fileSystem.mu.Lock()
	fileSystem.mkdirCalls++
	call := fileSystem.mkdirCalls
	fileSystem.mu.Unlock()
	if call == 1 {
		close(fileSystem.firstMkdirStarted)
		<-fileSystem.releaseFirstMkdir
	}
	return os.Mkdir(path, mode)
}

type scriptedOwnerProbe struct {
	record     ownerRecord
	currentErr error
	liveness   ownerLiveness
}

func (probe *scriptedOwnerProbe) Current() (ownerRecord, error) {
	return probe.record, probe.currentErr
}

func (probe *scriptedOwnerProbe) Classify(ownerRecord) ownerLiveness {
	return probe.liveness
}

type failingPackagedInstallationFileSystem struct {
	platformfilesystem.Local
	statPath         string
	statErr          error
	readDirErr       error
	mkdirAllErr      error
	mkdirErr         error
	removeErr        error
	readFileErr      error
	readFileData     []byte
	overrideReadFile bool
	writeFileErr     error
	renameErr        error
}

func (fileSystem *failingPackagedInstallationFileSystem) Stat(path string) (fs.FileInfo, error) {
	if fileSystem.statPath == path && fileSystem.statErr != nil {
		return nil, fileSystem.statErr
	}
	return fileSystem.Local.Stat(path)
}

func (fileSystem *failingPackagedInstallationFileSystem) ReadDir(path string) ([]fs.DirEntry, error) {
	if fileSystem.readDirErr != nil {
		return nil, fileSystem.readDirErr
	}
	return fileSystem.Local.ReadDir(path)
}

func (fileSystem *failingPackagedInstallationFileSystem) Mkdir(path string, mode fs.FileMode) error {
	if fileSystem.mkdirErr != nil {
		return fileSystem.mkdirErr
	}
	return os.Mkdir(path, mode)
}

func (fileSystem *failingPackagedInstallationFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	if fileSystem.mkdirAllErr != nil {
		return fileSystem.mkdirAllErr
	}
	return fileSystem.Local.MkdirAll(path, mode)
}

func (fileSystem *failingPackagedInstallationFileSystem) RemoveAll(path string) error {
	if fileSystem.removeErr != nil {
		return fileSystem.removeErr
	}
	return fileSystem.Local.RemoveAll(path)
}

func (fileSystem *failingPackagedInstallationFileSystem) ReadFile(path string) ([]byte, error) {
	if fileSystem.readFileErr != nil {
		return nil, fileSystem.readFileErr
	}
	if fileSystem.overrideReadFile {
		return append([]byte(nil), fileSystem.readFileData...), nil
	}
	return fileSystem.Local.ReadFile(path)
}

func (fileSystem *failingPackagedInstallationFileSystem) WriteFile(path string, data []byte, mode fs.FileMode) error {
	if fileSystem.writeFileErr != nil {
		return fileSystem.writeFileErr
	}
	return fileSystem.Local.WriteFile(path, data, mode)
}

func (fileSystem *failingPackagedInstallationFileSystem) Rename(oldPath, newPath string) error {
	if fileSystem.renameErr != nil {
		return fileSystem.renameErr
	}
	return fileSystem.Local.Rename(oldPath, newPath)
}

func TestInstallPackagedFactory_MaterializesPortableEditableFormats(t *testing.T) {
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("LoadPublishedDefinitionCatalog() error = %v", err)
	}
	definition, ok := catalog.Lookup("@you/full-flow")
	if !ok {
		t.Fatal("published catalog is missing @you/full-flow")
	}
	tests := []struct {
		format   factorydefinitions.PackagedFactoryFormat
		rootFile string
	}{
		{format: factorydefinitions.PackagedFactoryFormatJSON, rootFile: "factory.json"},
		{format: factorydefinitions.PackagedFactoryFormatYAML, rootFile: "factory.yaml"},
		{format: factorydefinitions.PackagedFactoryFormatYML, rootFile: "factory.yml"},
	}
	for _, test := range tests {
		test := test
		t.Run(string(test.format), func(t *testing.T) {
			root := t.TempDir()
			result, installErr := New(
				packagedInstallationTestPersistence(),
				platformfilesystem.Local{},
				os.Mkdir,
			).InstallPackagedFactory(t.Context(), factorydefinitions.PackagedFactoryInstallParams{
				NamedFactoriesRoot: root,
				Definition:         definition,
				Format:             test.format,
			})
			if installErr != nil {
				t.Fatalf("InstallPackagedFactory() error = %v", installErr)
			}
			if result.Outcome != factorydefinitions.PackagedFactoryInstallCreated ||
				result.Format != test.format {
				t.Fatalf("InstallPackagedFactory() = %#v", result)
			}
			assertSingleAuthoredRoot(t, result.FactoryDir, test.rootFile)
			assertBundledAssets(t, definition, result.FactoryDir)
			assertPortableMaterializedContent(t, result.FactoryDir)
			assertCustomerEditIsLoaded(t, result.FactoryDir, test.rootFile, "full-flow")
		})
	}
}

func TestInstallPackagedFactory_DefaultsToJSONAndRejectsUnsupportedFormat(t *testing.T) {
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("LoadPublishedDefinitionCatalog() error = %v", err)
	}
	definition, ok := catalog.Lookup("@you/goal")
	if !ok {
		t.Fatal("published catalog is missing @you/goal")
	}
	installer := New(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir)
	root := t.TempDir()
	result, err := installer.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: root,
			Definition:         definition,
			Format:             "",
		},
	)
	if err != nil {
		t.Fatalf("default InstallPackagedFactory() error = %v", err)
	}
	if result.Format != factorydefinitions.PackagedFactoryFormatJSON {
		t.Fatalf("default format = %q", result.Format)
	}
	assertSingleAuthoredRoot(t, result.FactoryDir, "factory.json")

	unsupportedRoot := t.TempDir()
	_, err = installer.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: unsupportedRoot,
			Definition:         definition,
			Format:             factorydefinitions.PackagedFactoryFormat("TOML"),
		},
	)
	if err == nil || !strings.Contains(err.Error(), `unsupported packaged Factory format "TOML"`) {
		t.Fatalf("unsupported format error = %v", err)
	}
	entries, readErr := os.ReadDir(unsupportedRoot)
	if readErr != nil {
		t.Fatalf("ReadDir() error = %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("unsupported format created entries: %v", entries)
	}
}

func TestInstallPackagedFactory_MaterializesEveryPublishedFactory(t *testing.T) {
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("LoadPublishedDefinitionCatalog() error = %v", err)
	}
	for _, definition := range catalog.All() {
		definition := definition
		t.Run(definition.Name, func(t *testing.T) {
			result, installErr := New(
				packagedInstallationTestPersistence(),
				platformfilesystem.Local{},
				os.Mkdir,
			).InstallPackagedFactory(
				t.Context(),
				factorydefinitions.PackagedFactoryInstallParams{
					NamedFactoriesRoot: t.TempDir(),
					Definition:         definition,
					Format:             factorydefinitions.PackagedFactoryFormatJSON,
				},
			)
			if installErr != nil {
				t.Fatalf("InstallPackagedFactory() error = %v", installErr)
			}
			if result.Name != definition.Name ||
				result.Outcome != factorydefinitions.PackagedFactoryInstallCreated {
				t.Fatalf("InstallPackagedFactory() = %#v", result)
			}
			if _, loadErr := factorydefinitioncomposition.LoadDirectory(
				result.FactoryDir,
				nil,
			); loadErr != nil {
				t.Fatalf("load materialized Factory: %v", loadErr)
			}
		})
	}
}

func TestInstallPackagedFactory_RepeatSkipsWithoutContentDrift(t *testing.T) {
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("LoadPublishedDefinitionCatalog() error = %v", err)
	}
	definition, ok := catalog.Lookup("@you/goal")
	if !ok {
		t.Fatal("published catalog is missing @you/goal")
	}
	installer := New(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir)
	root := t.TempDir()
	created, err := installer.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: root,
			Definition:         definition,
			Format:             factorydefinitions.PackagedFactoryFormatJSON,
		},
	)
	if err != nil || created.Outcome != factorydefinitions.PackagedFactoryInstallCreated {
		t.Fatalf("initial InstallPackagedFactory() = %#v, %v", created, err)
	}
	marker := filepath.Join(created.FactoryDir, "customer-owned.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshotDirectoryContents(t, created.FactoryDir)

	skipped, err := installer.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: root,
			Definition:         definition,
			Format:             factorydefinitions.PackagedFactoryFormatJSON,
		},
	)
	if err != nil {
		t.Fatalf("repeat InstallPackagedFactory() error = %v", err)
	}
	if skipped.Outcome != factorydefinitions.PackagedFactoryInstallSkipped {
		t.Fatalf("repeat outcome = %q, want skipped", skipped.Outcome)
	}
	assertDirectorySnapshotUnchanged(t, created.FactoryDir, before)
}

func TestInstallPackagedFactory_ExplicitReplaceRestoresPackagedLayout(t *testing.T) {
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("LoadPublishedDefinitionCatalog() error = %v", err)
	}
	definition, ok := catalog.Lookup("@you/goal")
	if !ok {
		t.Fatal("published catalog is missing @you/goal")
	}
	installer := New(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir)
	root := t.TempDir()
	created, err := installer.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: root,
			Definition:         definition,
			Format:             factorydefinitions.PackagedFactoryFormatJSON,
		},
	)
	if err != nil {
		t.Fatalf("initial InstallPackagedFactory() error = %v", err)
	}
	marker := filepath.Join(created.FactoryDir, "customer-owned.txt")
	if err := os.WriteFile(marker, []byte("replace-me"), 0o600); err != nil {
		t.Fatal(err)
	}

	replaced, err := installer.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: root,
			Definition:         definition,
			Format:             factorydefinitions.PackagedFactoryFormatJSON,
			Replace:            true,
		},
	)
	if err != nil {
		t.Fatalf("replace InstallPackagedFactory() error = %v", err)
	}
	if replaced.Outcome != factorydefinitions.PackagedFactoryInstallReplaced {
		t.Fatalf("replace outcome = %q, want replaced", replaced.Outcome)
	}
	if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("customer marker after replace = %v, want absent", statErr)
	}
	if _, loadErr := factorydefinitioncomposition.LoadDirectory(replaced.FactoryDir, nil); loadErr != nil {
		t.Fatalf("load replaced Factory: %v", loadErr)
	}
}

func TestInstallPackagedFactory_RefusesAlternateFormatWithoutReplace(t *testing.T) {
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("LoadPublishedDefinitionCatalog() error = %v", err)
	}
	definition, ok := catalog.Lookup("@you/goal")
	if !ok {
		t.Fatal("published catalog is missing @you/goal")
	}
	installer := New(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir)
	root := t.TempDir()
	if _, err := installer.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: root,
			Definition:         definition,
			Format:             factorydefinitions.PackagedFactoryFormatJSON,
		},
	); err != nil {
		t.Fatalf("initial InstallPackagedFactory() error = %v", err)
	}
	_, err = installer.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: root,
			Definition:         definition,
			Format:             factorydefinitions.PackagedFactoryFormatYAML,
		},
	)
	if err == nil || !errors.Is(err, factorydefinitions.ErrNamedFactoryAlreadyExists) {
		t.Fatalf("alternate format InstallPackagedFactory() error = %v, want %v", err, factorydefinitions.ErrNamedFactoryAlreadyExists)
	}
}

func TestInstallPackagedFactory_CancellationBeforeCommitLeavesTargetAbsent(t *testing.T) {
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("LoadPublishedDefinitionCatalog() error = %v", err)
	}
	definition, ok := catalog.Lookup("@you/goal")
	if !ok {
		t.Fatal("published catalog is missing @you/goal")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	root := t.TempDir()
	_, err = New(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir).
		InstallPackagedFactory(
			ctx,
			factorydefinitions.PackagedFactoryInstallParams{
				NamedFactoriesRoot: root,
				Definition:         definition,
				Format:             factorydefinitions.PackagedFactoryFormatJSON,
			},
		)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("InstallPackagedFactory() error = %v, want cancellation", err)
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("root entries after cancellation = %v, want none", entries)
	}
}

func TestInstallPackagedFactory_FailedReplacePreservesCommittedLayout(t *testing.T) {
	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("LoadPublishedDefinitionCatalog() error = %v", err)
	}
	definition, ok := catalog.Lookup("@you/goal")
	if !ok {
		t.Fatal("published catalog is missing @you/goal")
	}
	installer := New(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir)
	root := t.TempDir()
	created, err := installer.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: root,
			Definition:         definition,
			Format:             factorydefinitions.PackagedFactoryFormatJSON,
		},
	)
	if err != nil {
		t.Fatalf("initial InstallPackagedFactory() error = %v", err)
	}
	before := snapshotDirectoryContents(t, created.FactoryDir)
	invalid := definition
	invalid.JSON = []byte(`{"name":"broken","workers":[`)

	_, err = installer.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.PackagedFactoryInstallParams{
			NamedFactoriesRoot: root,
			Definition:         invalid,
			Format:             factorydefinitions.PackagedFactoryFormatJSON,
			Replace:            true,
		},
	)
	if err == nil {
		t.Fatal("replace with invalid payload error = nil")
	}
	assertDirectorySnapshotUnchanged(t, created.FactoryDir, before)
}

type directoryEntrySnapshot struct {
	Contents []byte
	Mode     fs.FileMode
	IsDir    bool
}

func snapshotDirectoryContents(t *testing.T, root string) map[string]directoryEntrySnapshot {
	t.Helper()
	snapshot := map[string]directoryEntrySnapshot{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		value := directoryEntrySnapshot{Mode: info.Mode(), IsDir: entry.IsDir()}
		if info.Mode().IsRegular() {
			value.Contents, err = os.ReadFile(path)
			if err != nil {
				return err
			}
		}
		snapshot[filepath.ToSlash(relative)] = value
		return nil
	}); err != nil {
		t.Fatalf("snapshot directory: %v", err)
	}
	return snapshot
}

func assertDirectorySnapshotUnchanged(t *testing.T, root string, before map[string]directoryEntrySnapshot) {
	t.Helper()
	after := snapshotDirectoryContents(t, root)
	if reflect.DeepEqual(before, after) {
		return
	}
	for path, want := range before {
		if got, ok := after[path]; !ok {
			t.Errorf("directory entry %q was removed", path)
		} else if !reflect.DeepEqual(want, got) {
			t.Errorf("directory entry %q changed: before=%#v after=%#v", path, want, got)
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			t.Errorf("directory entry %q was added", path)
		}
	}
}

func assertSingleAuthoredRoot(t *testing.T, factoryDir, want string) {
	t.Helper()
	for _, rootFile := range []string{"factory.json", "factory.yaml", "factory.yml"} {
		_, err := os.Stat(filepath.Join(factoryDir, rootFile))
		if rootFile == want {
			if err != nil {
				t.Fatalf("stat selected root %s: %v", rootFile, err)
			}
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unexpected authored root %s: %v", rootFile, err)
		}
	}
	if _, err := factorydefinitioncomposition.LoadDirectory(factoryDir, nil); err != nil {
		t.Fatalf("load materialized Factory: %v", err)
	}
}

func assertBundledAssets(
	t *testing.T,
	definition factorydefinitions.PackagedDefinition,
	factoryDir string,
) {
	t.Helper()
	var published struct {
		SupportingFiles struct {
			BundledFiles []struct {
				TargetPath string `json:"targetPath"`
				Content    struct {
					Inline string `json:"inline"`
				} `json:"content"`
			} `json:"bundledFiles"`
		} `json:"supportingFiles"`
	}
	if err := json.Unmarshal(definition.JSON, &published); err != nil {
		t.Fatalf("decode published definition: %v", err)
	}
	if len(published.SupportingFiles.BundledFiles) == 0 {
		t.Fatal("published definition has no bundled assets")
	}
	for _, bundled := range published.SupportingFiles.BundledFiles {
		relativePath := strings.TrimPrefix(bundled.TargetPath, "factory/")
		content, err := os.ReadFile(filepath.Join(factoryDir, relativePath))
		if err != nil {
			t.Fatalf("read materialized asset %s: %v", relativePath, err)
		}
		if string(content) != bundled.Content.Inline {
			t.Fatalf("materialized asset %s differs from published content", relativePath)
		}
	}
}

func assertPortableMaterializedContent(t *testing.T, factoryDir string) {
	t.Helper()
	err := filepath.WalkDir(factoryDir, func(
		path string,
		entry os.DirEntry,
		walkErr error,
	) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, forbidden := range []string{
			"packages/packaged-factories",
			"generated/factories",
			"node_modules",
			"npm ",
		} {
			if strings.Contains(string(content), forbidden) {
				t.Fatalf("%s contains non-portable reference %q", path, forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk materialized Factory: %v", err)
	}
}

func assertCustomerEditIsLoaded(t *testing.T, factoryDir, rootFile, originalName string) {
	t.Helper()
	path := filepath.Join(factoryDir, rootFile)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read authored root: %v", err)
	}
	var edited []byte
	if rootFile == "factory.json" {
		edited = []byte(strings.Replace(
			string(content),
			`"name": "`+originalName+`"`,
			`"name": "customer-edited"`,
			1,
		))
	} else {
		edited = []byte(strings.Replace(
			string(content),
			"\nname: "+originalName+"\n",
			"\nname: customer-edited\n",
			1,
		))
	}
	if string(edited) == string(content) {
		t.Fatalf("could not locate editable name in %s", rootFile)
	}
	if err := os.WriteFile(path, edited, 0o644); err != nil {
		t.Fatalf("write customer edit: %v", err)
	}
	loaded, err := factorydefinitioncomposition.LoadDirectory(factoryDir, nil)
	if err != nil {
		t.Fatalf("load customer-edited Factory: %v", err)
	}
	if loaded.FactoryConfig().Name != "customer-edited" {
		t.Fatalf("loaded name = %q, want customer-edited", loaded.FactoryConfig().Name)
	}
}
