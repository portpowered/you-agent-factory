package packagedinstallation

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type packagedInstallationLogEntry struct {
	level   string
	message string
	fields  map[string]any
}

type packagedInstallationLogger struct {
	mu      sync.Mutex
	entries []packagedInstallationLogEntry
}

func (logger *packagedInstallationLogger) Debug(message string, fields ...any) {
	logger.record("debug", message, fields...)
}

func (logger *packagedInstallationLogger) Info(message string, fields ...any) {
	logger.record("info", message, fields...)
}

func (logger *packagedInstallationLogger) Warn(message string, fields ...any) {
	logger.record("warn", message, fields...)
}

func (logger *packagedInstallationLogger) Error(message string, fields ...any) {
	logger.record("error", message, fields...)
}

func (logger *packagedInstallationLogger) Verbose(message string, fields ...any) {
	logger.record("verbose", message, fields...)
}

func (logger *packagedInstallationLogger) record(level, message string, fields ...any) {
	values := make(map[string]any, len(fields)/2)
	for index := 0; index+1 < len(fields); index += 2 {
		key, ok := fields[index].(string)
		if ok {
			values[key] = fields[index+1]
		}
	}
	logger.mu.Lock()
	defer logger.mu.Unlock()
	logger.entries = append(logger.entries, packagedInstallationLogEntry{
		level:   level,
		message: message,
		fields:  values,
	})
}

func (logger *packagedInstallationLogger) snapshot() []packagedInstallationLogEntry {
	logger.mu.Lock()
	defer logger.mu.Unlock()
	entries := make([]packagedInstallationLogEntry, len(logger.entries))
	copy(entries, logger.entries)
	return entries
}

func TestInstallPackagedFactory_LogsStructuredScopeAndSuccess(t *testing.T) {
	root := t.TempDir()
	logger := &packagedInstallationLogger{}
	scopeID := "local-diagnostic-scope"
	name := "@test/structured-logging"

	_, err := New(
		&successfulPackagedInstallationPersistence{},
		platformfilesystem.Local{},
		os.Mkdir,
		logger,
	).InstallPackagedFactory(t.Context(), factorydefinitions.PackagedFactoryInstallParams{
		NamedFactoriesRoot: root,
		BackendScopeID:     scopeID,
		Definition: factorydefinitions.PackagedDefinition{
			Name: name,
			JSON: []byte(`{}`),
		},
		Format: factorydefinitions.PackagedFactoryFormatJSON,
	})
	if err != nil {
		t.Fatalf("InstallPackagedFactory() error = %v", err)
	}

	wantOutcomes := map[string]bool{"acquired": false, "success": false}
	for _, entry := range logger.snapshot() {
		outcome, _ := entry.fields["outcome"].(string)
		if _, wanted := wantOutcomes[outcome]; !wanted {
			continue
		}
		if entry.message != "factory_definitions.packaged_installation" {
			t.Fatalf("log message = %q, want packaged-installation operation", entry.message)
		}
		if got := entry.fields["backend_scope_id"]; got != scopeID {
			t.Fatalf("backend_scope_id = %#v, want %q", got, scopeID)
		}
		if resource, ok := entry.fields["resource"].(string); !ok || resource == "" {
			t.Fatalf("resource = %#v, want named resource", entry.fields["resource"])
		}
		if entry.fields["owner_identity"] != "unverified" {
			t.Fatalf("owner_identity = %#v, want unverified", entry.fields["owner_identity"])
		}
		wantOutcomes[outcome] = true
	}
	for outcome, found := range wantOutcomes {
		if !found {
			t.Fatalf("structured log outcome %q was not emitted: %#v", outcome, logger.snapshot())
		}
	}
}

func TestManagedInstallationFailureUsesErrorDiagnostic(t *testing.T) {
	t.Parallel()

	logger := &packagedInstallationLogger{}
	service := New(
		packagedInstallationTestPersistence(),
		platformfilesystem.Local{},
		os.Mkdir,
		logger,
	)
	service.logInstallationOutcome(
		"managed-failure-scope",
		factorydefinitions.PackagedFactoryInstallResult{
			Name:    "@you/goal",
			Outcome: factorydefinitions.PackagedFactoryInstallFailed,
		},
		&stagingLease{path: "managed-lease", owner: ownerRecord{PID: 42}},
	)

	entries := logger.snapshot()
	if len(entries) == 0 || entries[len(entries)-1].level != "error" {
		t.Fatalf("managed failure diagnostics = %#v, want final error entry", entries)
	}
}

func TestInstallPackagedFactory_ReclaimsOnlyRevalidatedOrphan(t *testing.T) {
	root := t.TempDir()
	name := "@test/orphan-recovery"
	stagingPath := stagingOwnershipPath(root, name)
	if err := os.MkdirAll(stagingPath, 0o755); err != nil {
		t.Fatalf("create orphan staging path: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(stagingPath, stagingOwnerMetadataName),
		[]byte(`{"pid":404}`),
		0o600,
	); err != nil {
		t.Fatalf("write orphan owner metadata: %v", err)
	}
	logger := &packagedInstallationLogger{}
	probe := &scriptedOwnerProbe{
		record:   ownerRecord{PID: 101},
		liveness: ownerLivenessOrphaned,
	}
	service := newWithOwnerProbe(
		&successfulPackagedInstallationPersistence{},
		platformfilesystem.Local{},
		os.Mkdir,
		probe,
		logger,
	)
	_, err := service.InstallPackagedFactory(t.Context(), factorydefinitions.PackagedFactoryInstallParams{
		NamedFactoriesRoot: root,
		BackendScopeID:     "local-orphan-scope",
		Definition: factorydefinitions.PackagedDefinition{
			Name: name,
			JSON: []byte(`{}`),
		},
		Format: factorydefinitions.PackagedFactoryFormatJSON,
	})
	if err != nil {
		t.Fatalf("InstallPackagedFactory() error = %v", err)
	}
	if _, err := os.Stat(stagingPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("orphan staging path stat error = %v, want reclaimed", err)
	}

	foundReclaim := false
	for _, entry := range logger.snapshot() {
		if entry.fields["outcome"] != "reclaimed-orphan" {
			continue
		}
		foundReclaim = true
		if entry.fields["backend_scope_id"] != "local-orphan-scope" || entry.fields["resource"] != stagingPath {
			t.Fatalf("reclaim diagnostic = %#v, want scope/resource", entry.fields)
		}
	}
	if !foundReclaim {
		t.Fatalf("reclaimed-orphan diagnostic missing: %#v", logger.snapshot())
	}
	assertRecoveredAcquiredDiagnostic(t, logger)
}

func assertRecoveredAcquiredDiagnostic(t *testing.T, logger *packagedInstallationLogger) {
	t.Helper()
	for _, entry := range logger.snapshot() {
		if entry.fields["outcome"] != "acquired" {
			continue
		}
		if entry.fields["backend_scope_id"] != "local-orphan-scope" {
			t.Fatalf("recovered acquisition diagnostic = %#v, want scope", entry.fields)
		}
		resource, ok := entry.fields["resource"].(string)
		if !ok || !strings.Contains(resource, "-recovered-") {
			t.Fatalf("recovered acquisition resource = %#v, want recovery lease", entry.fields["resource"])
		}
		return
	}
	t.Fatalf("recovered acquired diagnostic missing: %#v", logger.snapshot())
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

	recoveredPath := findRecoveredLeasePath(t, root)
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

func findRecoveredLeasePath(t *testing.T, root string) string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read root while winner is held: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "-recovered-") {
			return filepath.Join(root, entry.Name())
		}
	}
	t.Fatalf("winner recovery lease was not published; entries = %v", entries)
	return ""
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
