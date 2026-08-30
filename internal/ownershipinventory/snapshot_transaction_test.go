package ownershipinventory

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

type snapshotTransactionFixture struct {
	packages   []string
	inventory  Inventory
	freeze     PathLeaseFreeze
	candidates SnapshotCandidates
	writes     []snapshotWrite
}

func newSnapshotTransactionFixture(t *testing.T) snapshotTransactionFixture {
	t.Helper()
	root, err := FindRepositoryRoot()
	if err != nil {
		t.Fatalf("FindRepositoryRoot() error = %v", err)
	}
	packages, err := ListProductionPackages(root)
	if err != nil {
		t.Fatalf("ListProductionPackages() error = %v", err)
	}
	inventory, err := BuildInventory(root, packages)
	if err != nil {
		t.Fatalf("BuildInventory() error = %v", err)
	}
	candidates, err := BuildSnapshotCandidates(root)
	if err != nil {
		t.Fatalf("BuildSnapshotCandidates() error = %v", err)
	}
	freeze := BuildPathLeaseFreeze()
	writes, err := snapshotGroupWrites(packages, inventory, freeze, candidates)
	if err != nil {
		t.Fatalf("snapshotGroupWrites() error = %v", err)
	}
	return snapshotTransactionFixture{
		packages: packages, inventory: inventory, freeze: freeze,
		candidates: candidates, writes: writes,
	}
}

func (fixture snapshotTransactionFixture) publish(t *testing.T, files fileSystem, root string) error {
	t.Helper()
	return publishSnapshotGroup(files, root, fixture.packages, fixture.inventory, fixture.freeze, fixture.candidates)
}

func TestOwnershipSnapshotGroupStagesEveryPayloadBeforeReplacement(t *testing.T) {
	fixture := newSnapshotTransactionFixture(t)
	root := t.TempDir()
	files := &controlledSnapshotFileSystem{}

	if err := fixture.publish(t, files, root); err != nil {
		t.Fatalf("publishSnapshotGroup() error = %v", err)
	}
	firstInstall := -1
	for index, operation := range files.operations {
		if strings.HasPrefix(operation, "install:") {
			firstInstall = index
			break
		}
	}
	if firstInstall < 0 {
		t.Fatal("no staged snapshot installation was recorded")
	}
	closedStages := 0
	for _, operation := range files.operations[:firstInstall] {
		if strings.HasPrefix(operation, "close-stage:") {
			closedStages++
		}
	}
	if closedStages != len(fixture.writes) {
		t.Fatalf("closed stages before first install = %d, want %d; operations=%v", closedStages, len(fixture.writes), files.operations)
	}
	assertGeneratedSnapshotStates(t, root, fixture.writes)
	assertNoSnapshotTransactionResidue(t, root)
}

func TestOwnershipSnapshotGroupSuccessPreservesExistingModes(t *testing.T) {
	fixture := newSnapshotTransactionFixture(t)
	root := t.TempDir()
	initial := seedSnapshotTransactionTargets(t, root, fixture.writes, func(int) bool { return true })

	if err := fixture.publish(t, osFileSystem{}, root); err != nil {
		t.Fatalf("publishSnapshotGroup() error = %v", err)
	}
	for index, write := range fixture.writes {
		path := snapshotTargetPath(root, write.relativePath)
		assertSnapshotBytesAndMode(t, path, write.payload, initial[index].mode)
	}
	assertNoSnapshotTransactionResidue(t, root)
}

func TestOwnershipSnapshotGroupWindowsExistingTargetsUseBackupFirst(t *testing.T) {
	fixture := newSnapshotTransactionFixture(t)
	root := t.TempDir()
	initial := seedSnapshotTransactionTargets(t, root, fixture.writes, func(int) bool { return true })
	files := &controlledSnapshotFileSystem{}

	if err := fixture.publish(t, files, root); err != nil {
		t.Fatalf("publishSnapshotGroup() error = %v", err)
	}
	if files.installDestinationExisted {
		t.Fatal("staged replacement was attempted over an existing destination")
	}
	for index, write := range fixture.writes {
		assertSnapshotBytesAndMode(t, snapshotTargetPath(root, write.relativePath), write.payload, initial[index].mode)
	}
	assertNoSnapshotTransactionResidue(t, root)
}

func TestOwnershipSnapshotGroupRejectsInvalidCandidatesBeforeFilesystemMutation(t *testing.T) {
	fixture := newSnapshotTransactionFixture(t)
	tests := []struct {
		name   string
		mutate func(*snapshotTransactionFixture)
		want   string
	}{
		{
			name: "S-03 validation",
			mutate: func(fixture *snapshotTransactionFixture) {
				fixture.inventory.SortKey = "unstable"
			},
			want: "validate S-03",
		},
		{
			name: "S-04 validation",
			mutate: func(fixture *snapshotTransactionFixture) {
				fixture.freeze.FormatVersion = "invalid"
			},
			want: "validate S-04",
		},
		{
			name: "S-05 validation",
			mutate: func(fixture *snapshotTransactionFixture) {
				fixture.candidates.OperatorSettingsRootGo.Files[0].Classification = "invalid_classification"
			},
			want: "invalid_classification",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caseFixture := fixture
			test.mutate(&caseFixture)
			files := &controlledSnapshotFileSystem{}
			err := caseFixture.publish(t, files, t.TempDir())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("publishSnapshotGroup() error = %v, want %q", err, test.want)
			}
			if len(files.operations) != 0 {
				t.Fatalf("invalid candidate caused filesystem operations: %v", files.operations)
			}
		})
	}
}

func TestOwnershipSnapshotGroupRejectsNonRegularTargetsBeforeStaging(t *testing.T) {
	fixture := newSnapshotTransactionFixture(t)
	for position, write := range fixture.writes {
		t.Run(fmt.Sprintf("position-%d", position+1), func(t *testing.T) {
			root := t.TempDir()
			seedSnapshotTransactionTargets(t, root, fixture.writes, func(index int) bool { return index != position })
			path := snapshotTargetPath(root, write.relativePath)
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatalf("mkdir non-regular target: %v", err)
			}
			files := &controlledSnapshotFileSystem{}
			err := fixture.publish(t, files, root)
			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("position %d", position+1)) || !strings.Contains(err.Error(), "directory") {
				t.Fatalf("publishSnapshotGroup() error = %v, want position and directory", err)
			}
			if containsSnapshotOperation(files.operations, "create-stage:") || containsSnapshotOperation(files.operations, "install:") {
				t.Fatalf("non-regular target allowed staging: %v", files.operations)
			}
			assertNoSnapshotTransactionResidue(t, root)
		})
	}
}

func TestOwnershipSnapshotGroupStagingFailuresPreserveTargets(t *testing.T) {
	fixture := newSnapshotTransactionFixture(t)
	tests := []struct {
		name      string
		configure func(*controlledSnapshotFileSystem, int)
		want      string
	}{
		{name: "create", configure: func(files *controlledSnapshotFileSystem, position int) { files.failStageCreateAt = position }, want: "create destination-local stage"},
		{name: "write", configure: func(files *controlledSnapshotFileSystem, position int) { files.failStageWriteAt = position }, want: "write destination-local stage"},
		{name: "chmod", configure: func(files *controlledSnapshotFileSystem, position int) { files.failStageChmodAt = position }, want: "set destination-local stage mode"},
		{name: "close", configure: func(files *controlledSnapshotFileSystem, position int) { files.failStageCloseAt = position }, want: "close destination-local stage"},
	}
	for _, test := range tests {
		for position := range fixture.writes {
			position := position + 1
			t.Run(fmt.Sprintf("%s-position-%d", test.name, position), func(t *testing.T) {
				root := t.TempDir()
				initial := seedSnapshotTransactionTargets(t, root, fixture.writes, func(int) bool { return true })
				files := &controlledSnapshotFileSystem{}
				test.configure(files, position)
				err := fixture.publish(t, files, root)
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("publishSnapshotGroup() error = %v, want %q", err, test.want)
				}
				assertSnapshotTransactionStates(t, root, fixture.writes, initial)
				if containsSnapshotOperation(files.operations, "install:") {
					t.Fatalf("staging failure reached replacement: %v", files.operations)
				}
				assertNoSnapshotTransactionResidue(t, root)
			})
		}
	}
}

func TestOwnershipSnapshotGroupReplacementFailuresRollBackEveryPosition(t *testing.T) {
	fixture := newSnapshotTransactionFixture(t)
	for position := range fixture.writes {
		t.Run(fmt.Sprintf("position-%d", position+1), func(t *testing.T) {
			root := t.TempDir()
			initial := seedSnapshotTransactionTargets(t, root, fixture.writes, func(int) bool { return true })
			files := &controlledSnapshotFileSystem{failInstallAt: position + 1}
			err := fixture.publish(t, files, root)
			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("replacement position %d", position+1)) || !strings.Contains(err.Error(), fixture.writes[position].relativePath) {
				t.Fatalf("publishSnapshotGroup() error = %v, want forced position/path", err)
			}
			assertSnapshotTransactionStates(t, root, fixture.writes, initial)
			assertNoSnapshotTransactionResidue(t, root)
		})
	}
}

func TestOwnershipSnapshotGroupMixedExistenceRollsBackAbsentTargets(t *testing.T) {
	fixture := newSnapshotTransactionFixture(t)
	root := t.TempDir()
	initial := seedSnapshotTransactionTargets(t, root, fixture.writes, func(index int) bool { return index%2 == 0 })
	files := &controlledSnapshotFileSystem{failInstallAt: 6}

	if err := fixture.publish(t, files, root); err == nil {
		t.Fatal("publishSnapshotGroup() error = nil, want forced replacement failure")
	}
	assertSnapshotTransactionStates(t, root, fixture.writes, initial)
	assertNoSnapshotTransactionResidue(t, root)
}

func TestOwnershipSnapshotGroupRollbackFallbackRetainsDiagnostics(t *testing.T) {
	fixture := newSnapshotTransactionFixture(t)
	root := t.TempDir()
	initial := seedSnapshotTransactionTargets(t, root, fixture.writes, func(int) bool { return true })
	files := &controlledSnapshotFileSystem{failInstallAt: 4, failRollbackRenameOnce: true}

	err := fixture.publish(t, files, root)
	if err == nil || !strings.Contains(err.Error(), "replacement position 4") || !strings.Contains(err.Error(), "restore original from backup") || !strings.Contains(err.Error(), "retained-byte fallback") {
		t.Fatalf("publishSnapshotGroup() error = %v, want primary and rollback diagnostics", err)
	}
	assertSnapshotTransactionStates(t, root, fixture.writes, initial)
	assertNoSnapshotTransactionResidue(t, root)
}

func TestOwnershipSnapshotGroupUnrecoverableRollbackRetainsOriginalBackup(t *testing.T) {
	fixture := newSnapshotTransactionFixture(t)
	root := t.TempDir()
	initial := seedSnapshotTransactionTargets(t, root, fixture.writes, func(int) bool { return true })
	failedPosition := 1
	files := &controlledSnapshotFileSystem{
		failInstallAt:          failedPosition + 1,
		failRollbackRenameOnce: true,
		failFallbackWrite:      true,
	}

	err := fixture.publish(t, files, root)
	if err == nil || !strings.Contains(err.Error(), "retained-byte fallback") || !strings.Contains(err.Error(), "original backup retained") {
		t.Fatalf("publishSnapshotGroup() error = %v, want retained recovery artifact diagnostic", err)
	}
	backupPath := findSnapshotResidue(t, root, snapshotBackupPattern)
	if backupPath == "" {
		t.Fatal("unrecoverable rollback removed the retained backup")
	}
	assertSnapshotBytesAndMode(t, backupPath, initial[failedPosition].payload, initial[failedPosition].mode)
}

func TestOwnershipSnapshotGroupCleanupFailureReportsResidue(t *testing.T) {
	fixture := newSnapshotTransactionFixture(t)
	root := t.TempDir()
	seedSnapshotTransactionTargets(t, root, fixture.writes, func(int) bool { return true })
	files := &controlledSnapshotFileSystem{failBackupRemovalAt: len(fixture.writes) + 1}

	err := fixture.publish(t, files, root)
	if err == nil || !strings.Contains(err.Error(), "cleanup") || !strings.Contains(err.Error(), "backup") {
		t.Fatalf("publishSnapshotGroup() error = %v, want cleanup diagnostic", err)
	}
	if findSnapshotResidue(t, root, snapshotBackupPattern) == "" {
		t.Fatal("cleanup failure did not leave the reported backup residue")
	}
}

func TestOwnershipSnapshotGroupPublishesDistinctRootsConcurrently(t *testing.T) {
	fixture := newSnapshotTransactionFixture(t)
	roots := []string{t.TempDir(), t.TempDir()}
	errs := make(chan error, len(roots))
	var wait sync.WaitGroup
	for _, root := range roots {
		root := root
		wait.Add(1)
		go func() {
			defer wait.Done()
			errs <- fixture.publish(t, osFileSystem{}, root)
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent publishSnapshotGroup() error = %v", err)
		}
	}
	for _, root := range roots {
		assertGeneratedSnapshotStates(t, root, fixture.writes)
		assertNoSnapshotTransactionResidue(t, root)
	}
}

func TestOwnershipSnapshotWriteSetRejectsShapeDrift(t *testing.T) {
	fixture := newSnapshotTransactionFixture(t)
	tests := []struct {
		name   string
		mutate func([]snapshotWrite) []snapshotWrite
	}{
		{name: "missing entry", mutate: func(writes []snapshotWrite) []snapshotWrite { return writes[:5] }},
		{name: "extra entry", mutate: func(writes []snapshotWrite) []snapshotWrite {
			return append(writes, snapshotWrite{relativePath: "extra.json", payload: []byte("{}\n")})
		}},
		{name: "empty path", mutate: func(writes []snapshotWrite) []snapshotWrite { writes[2].relativePath = ""; return writes }},
		{name: "duplicate", mutate: func(writes []snapshotWrite) []snapshotWrite {
			writes[2].relativePath = writes[1].relativePath
			return writes
		}},
		{name: "out of order", mutate: func(writes []snapshotWrite) []snapshotWrite {
			writes[0], writes[1] = writes[1], writes[0]
			return writes
		}},
		{name: "empty payload", mutate: func(writes []snapshotWrite) []snapshotWrite { writes[3].payload = nil; return writes }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writes := append([]snapshotWrite(nil), fixture.writes...)
			if err := validateSnapshotWriteSet(test.mutate(writes)); err == nil {
				t.Fatal("validateSnapshotWriteSet() error = nil, want fixed-group invariant failure")
			}
		})
	}
}

type seededSnapshotState struct {
	exists  bool
	payload []byte
	mode    fs.FileMode
}

func seedSnapshotTransactionTargets(t *testing.T, root string, writes []snapshotWrite, present func(int) bool) []seededSnapshotState {
	t.Helper()
	states := make([]seededSnapshotState, len(writes))
	modes := []fs.FileMode{0o601, 0o623, 0o645, 0o667, 0o604, 0o626}
	for index, write := range writes {
		if !present(index) {
			continue
		}
		path := snapshotTargetPath(root, write.relativePath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", write.relativePath, err)
		}
		payload := []byte(fmt.Sprintf("sentinel-%d\n", index+1))
		if err := os.WriteFile(path, payload, 0o644); err != nil {
			t.Fatalf("write %s: %v", write.relativePath, err)
		}
		if err := os.Chmod(path, modes[index]); err != nil {
			t.Fatalf("chmod %s: %v", write.relativePath, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", write.relativePath, err)
		}
		states[index] = seededSnapshotState{exists: true, payload: payload, mode: info.Mode().Perm()}
	}
	return states
}

func assertSnapshotTransactionStates(t *testing.T, root string, writes []snapshotWrite, states []seededSnapshotState) {
	t.Helper()
	for index, write := range writes {
		path := snapshotTargetPath(root, write.relativePath)
		info, err := os.Lstat(path)
		if !states[index].exists {
			if !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("position %d path %s = %v, want absent", index+1, write.relativePath, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("stat restored position %d path %s: %v", index+1, write.relativePath, err)
			continue
		}
		assertSnapshotBytesAndMode(t, path, states[index].payload, states[index].mode)
		if !info.Mode().IsRegular() {
			t.Errorf("restored position %d is not regular: %s", index+1, info.Mode())
		}
	}
}

func assertGeneratedSnapshotStates(t *testing.T, root string, writes []snapshotWrite) {
	t.Helper()
	for _, write := range writes {
		assertSnapshotBytesAndMode(t, snapshotTargetPath(root, write.relativePath), write.payload, expectedNewSnapshotMode())
	}
}

func expectedNewSnapshotMode() fs.FileMode {
	if runtime.GOOS == "windows" {
		return 0o666
	}
	return snapshotTargetMode
}

func assertSnapshotBytesAndMode(t *testing.T, path string, want []byte, mode fs.FileMode) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("read %s: %v", path, err)
		return
	}
	if !bytes.Equal(got, want) {
		t.Errorf("bytes at %s = %q, want %q", path, got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Errorf("stat %s: %v", path, err)
		return
	}
	if info.Mode().Perm() != mode {
		t.Errorf("mode at %s = %o, want %o", path, info.Mode().Perm(), mode)
	}
}

func snapshotTargetPath(root, relativePath string) string {
	return filepath.Join(root, filepath.FromSlash(relativePath))
}

func assertNoSnapshotTransactionResidue(t *testing.T, root string) {
	t.Helper()
	if path := findSnapshotResidue(t, root, ".ownership-snapshot-"); path != "" {
		t.Errorf("transaction residue remains at %s", path)
	}
}

func findSnapshotResidue(t *testing.T, root, pattern string) string {
	t.Helper()
	pattern = strings.TrimSuffix(pattern, "*")
	var found string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.Contains(entry.Name(), pattern) {
			found = path
			return fs.SkipAll
		}
		return nil
	}); err != nil && !errors.Is(err, fs.SkipAll) {
		t.Fatalf("scan transaction residue: %v", err)
	}
	return found
}

func containsSnapshotOperation(operations []string, prefix string) bool {
	for _, operation := range operations {
		if strings.HasPrefix(operation, prefix) {
			return true
		}
	}
	return false
}

type controlledSnapshotFileSystem struct {
	base                      osFileSystem
	operations                []string
	stageCreates              int
	failStageCreateAt         int
	failStageWriteAt          int
	failStageChmodAt          int
	failStageCloseAt          int
	installRenames            int
	installDestinationExisted bool
	failInstallAt             int
	failRollbackRenameOnce    bool
	rollbackRenameFailed      bool
	failFallbackWrite         bool
	backupRemovalCalls        int
	failBackupRemovalAt       int
}

func (files *controlledSnapshotFileSystem) mkdirAll(path string, perm fs.FileMode) error {
	files.operations = append(files.operations, "mkdir:"+path)
	return files.base.mkdirAll(path, perm)
}

func (files *controlledSnapshotFileSystem) writeFile(path string, payload []byte, perm fs.FileMode) error {
	files.operations = append(files.operations, "write-file:"+path)
	if files.failFallbackWrite {
		return errors.New("forced fallback write failure")
	}
	return files.base.writeFile(path, payload, perm)
}

func (files *controlledSnapshotFileSystem) chmod(path string, perm fs.FileMode) error {
	files.operations = append(files.operations, "chmod:"+path)
	return files.base.chmod(path, perm)
}

func (files *controlledSnapshotFileSystem) readFile(path string) ([]byte, error) {
	return files.base.readFile(path)
}

func (files *controlledSnapshotFileSystem) lstat(path string) (fs.FileInfo, error) {
	return files.base.lstat(path)
}

func (files *controlledSnapshotFileSystem) createTemp(directory, pattern string) (snapshotFile, error) {
	if strings.HasPrefix(pattern, ".ownership-snapshot-stage-") {
		files.stageCreates++
		if files.failStageCreateAt == files.stageCreates {
			return nil, errors.New("forced stage create failure")
		}
		stage, err := files.base.createTemp(directory, pattern)
		if err != nil {
			return nil, err
		}
		files.operations = append(files.operations, "create-stage:"+stage.Name())
		return &controlledSnapshotFile{snapshotFile: stage, owner: files, ordinal: files.stageCreates}, nil
	}
	backup, err := files.base.createTemp(directory, pattern)
	if err == nil {
		files.operations = append(files.operations, "create-backup:"+backup.Name())
	}
	return backup, err
}

func (files *controlledSnapshotFileSystem) rename(oldPath, newPath string) error {
	if strings.Contains(filepath.Base(oldPath), ".ownership-snapshot-stage-") {
		files.installRenames++
		files.operations = append(files.operations, "install:"+oldPath+"->"+newPath)
		if _, err := files.base.lstat(newPath); err == nil {
			files.installDestinationExisted = true
		}
		if files.failInstallAt == files.installRenames {
			return errors.New("forced replacement failure")
		}
	} else if strings.Contains(filepath.Base(oldPath), ".ownership-snapshot-backup-") {
		files.operations = append(files.operations, "rollback-rename:"+oldPath+"->"+newPath)
		if files.failRollbackRenameOnce && !files.rollbackRenameFailed {
			files.rollbackRenameFailed = true
			return errors.New("forced rollback rename failure")
		}
	} else {
		files.operations = append(files.operations, "rename:"+oldPath+"->"+newPath)
	}
	return files.base.rename(oldPath, newPath)
}

func (files *controlledSnapshotFileSystem) remove(path string) error {
	if strings.Contains(filepath.Base(path), ".ownership-snapshot-backup-") {
		files.backupRemovalCalls++
		if files.failBackupRemovalAt == files.backupRemovalCalls {
			return errors.New("forced backup cleanup failure")
		}
	}
	files.operations = append(files.operations, "remove:"+path)
	return files.base.remove(path)
}

type controlledSnapshotFile struct {
	snapshotFile
	owner   *controlledSnapshotFileSystem
	ordinal int
}

func (file *controlledSnapshotFile) Write(payload []byte) (int, error) {
	file.owner.operations = append(file.owner.operations, "write-stage:"+file.Name())
	if file.owner.failStageWriteAt == file.ordinal {
		return 0, errors.New("forced stage write failure")
	}
	return file.snapshotFile.Write(payload)
}

func (file *controlledSnapshotFile) Chmod(mode fs.FileMode) error {
	file.owner.operations = append(file.owner.operations, "chmod-stage:"+file.Name())
	if file.owner.failStageChmodAt == file.ordinal {
		return errors.New("forced stage chmod failure")
	}
	return file.snapshotFile.Chmod(mode)
}

func (file *controlledSnapshotFile) Close() error {
	file.owner.operations = append(file.owner.operations, "close-stage:"+file.Name())
	err := file.snapshotFile.Close()
	if file.owner.failStageCloseAt == file.ordinal {
		return errors.Join(err, errors.New("forced stage close failure"))
	}
	return err
}
