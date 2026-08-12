package packagedinstallation

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayoutpersist "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/persist"
)

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

type blockingPackagedInstallationPersistence struct {
	factorydefinitions.PackagedFactoryPersistence
	prepareStarted chan struct{}
	allowPrepare   chan struct{}
	prepareOnce    sync.Once
}

func (p *blockingPackagedInstallationPersistence) PreparePackagedFactoryLayout(
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
