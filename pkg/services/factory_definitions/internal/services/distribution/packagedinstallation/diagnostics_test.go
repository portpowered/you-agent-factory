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
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestInstallPackagedFactory_DeadPIDReclaimsLease(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skip("native incarnation queries require Windows or Linux")
	}
	const deadPID = 2147483647
	if _, err := (platformprocess.IncarnationProbe{}).LookupProcess(deadPID); !errors.Is(err, platformprocess.ErrProcessGone) {
		t.Fatalf("native absence prerequisite: %v", err)
	}
	root := t.TempDir()
	definition := installationDefinitionFixture()
	staging := stagingOwnershipPath(root, definition.Name)
	if err := os.MkdirAll(staging, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, stagingOwnerMetadataName), []byte(`{"pid":2147483647}`), 0600); err != nil {
		t.Fatal(err)
	}
	observed := false
	persistence := &installationPersistenceStub{prepareObserve: func() {
		assertRecoveredNativeOwner(t, staging)
		observed = true
	}}
	logger := &packagedInstallationLogger{}
	result, err := New(persistence, platformfilesystem.Local{}, os.Mkdir, logger).InstallPackagedFactory(t.Context(), factorydefinitions.PackagedFactoryInstallParams{NamedFactoriesRoot: root, Definition: definition, Format: factorydefinitions.PackagedFactoryFormatJSON})
	if err != nil {
		t.Fatal(err)
	}
	if !observed || result.Outcome != factorydefinitions.PackagedFactoryInstallCreated {
		t.Fatalf("result=%+v observed=%v", result, observed)
	}
	data, err := os.ReadFile(filepath.Join(result.FactoryDir, "factory.json"))
	if err != nil || string(data) != string(definition.JSON) {
		t.Fatalf("target=%s error=%v", data, err)
	}
	paths, err := filepath.Glob(staging + "*")
	if err != nil || len(paths) != 0 {
		t.Fatalf("lease cleanup: %v %v", paths, err)
	}
	for _, entry := range logger.snapshot() {
		if entry.fields["outcome"] == "reclaimed-orphan" {
			return
		}
	}
	t.Fatal("missing reclaimed-orphan diagnostic")
}

func assertRecoveredNativeOwner(t *testing.T, staging string) {
	t.Helper()
	paths, err := filepath.Glob(staging + "-recovered-*")
	if err != nil || len(paths) != 1 {
		t.Fatalf("replacement lease: %v %v", paths, err)
	}
	data, err := os.ReadFile(filepath.Join(paths[0], stagingOwnerMetadataName))
	var owner ownerRecord
	if err != nil || json.Unmarshal(data, &owner) != nil || owner.PID != os.Getpid() {
		t.Fatalf("replacement owner: %s %v", data, err)
	}
	identity, err := (platformprocess.IncarnationProbe{}).CurrentProcess()
	if err != nil || owner.Host != identity.Host || owner.Start != identity.Start {
		t.Fatalf("published identity=%+v native=%+v error=%v", owner, identity, err)
	}
}

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
		logging.NoopLogger{},
	)
	second := newWithOwnerProbe(
		persistence,
		fileSystem,
		os.Mkdir,
		&scriptedOwnerProbe{record: ownerRecord{PID: 202}, liveness: ownerLivenessOrphaned},
		logging.NoopLogger{},
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

// Exact field comparison also protects the diagnostic privacy contract: payloads,
// raw preparation errors and owner metadata never become diagnostic fields.
func assertInstallationDiagnostic(t *testing.T, entry packagedInstallationLogEntry, level, scope, name, resource, outcome string, liveness ownerLiveness, pid any, installOutcome, backup string) {
	t.Helper()
	fields := map[string]any{
		"backend_scope_id": scope, "factory_name": name, "resource": resource,
		"outcome": outcome, "owner_liveness": liveness, "owner_pid": pid,
		"owner_identity": "unverified",
	}
	if installOutcome != "" {
		fields["install_outcome"] = installOutcome
	}
	if backup != "" {
		fields["backup_dir"] = backup
	}
	if entry.level != level || entry.message != "factory_definitions.packaged_installation" || !reflect.DeepEqual(entry.fields, fields) {
		t.Fatalf("diagnostic = %#v, want %s operation with %#v", entry, level, fields)
	}
}

func assertInstallationReleased(t *testing.T, root, name string) {
	t.Helper()
	if path, err := findPreExistingStaging(platformfilesystem.Local{}, root, name); err != nil || path != "" {
		t.Fatalf("remaining staging = %q, %v, want released", path, err)
	}
	entries, err := os.ReadDir(filepath.Join(root, managedScratchRoot))
	if !errors.Is(err, fs.ErrNotExist) && (err != nil || len(entries) != 0) {
		t.Fatalf("remaining managed scratch = %v, %v, want none", entries, err)
	}
}

func assertInstallationPair(t *testing.T, logger *packagedInstallationLogger, root, scope string, result factorydefinitions.PackagedFactoryInstallResult) {
	t.Helper()
	entries := logger.snapshot()
	if len(entries) < 2 {
		t.Fatalf("diagnostics = %#v, want acquisition and terminal outcome", entries)
	}
	entries = entries[len(entries)-2:]
	lease := stagingOwnershipPath(root, result.Name)
	assertInstallationDiagnostic(t, entries[0], "info", scope, result.Name, lease, "acquired", ownerLivenessActive, os.Getpid(), "", "")
	outcome, level := string(result.Outcome), "info"
	switch result.Outcome {
	case factorydefinitions.PackagedFactoryInstallCreated, factorydefinitions.PackagedFactoryInstallReplaced:
		outcome = "success"
	case factorydefinitions.PackagedFactoryInstallCustomerModified:
		level = "warn"
	case factorydefinitions.PackagedFactoryInstallFailed:
		level = "error"
	}
	// Preparation/replacement errors log the lease outcome without a result field.
	installOutcome, backup := string(result.Outcome), result.BackupDir
	if result.Outcome == factorydefinitions.PackagedFactoryInstallFailed {
		installOutcome, backup = "", ""
	}
	assertInstallationDiagnostic(t, entries[1], level, scope, result.Name, lease, outcome, ownerLivenessActive, os.Getpid(), installOutcome, backup)
	assertInstallationReleased(t, root, result.Name)
}

func TestSelectedLoggerInstallCreateSkipReplace(t *testing.T) {
	t.Parallel()
	root, definition := t.TempDir(), installationDefinitionFixture()
	logger := &packagedInstallationLogger{}
	installer := New(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir, logger)
	params := factorydefinitions.PackagedFactoryInstallParams{NamedFactoriesRoot: root, BackendScopeID: "  selected-install  ", Definition: definition}
	created, err := installer.InstallPackagedFactory(t.Context(), params)
	if err != nil || created.Outcome != factorydefinitions.PackagedFactoryInstallCreated {
		t.Fatalf("create = %#v, %v", created, err)
	}
	assertInstallationPair(t, logger, root, "selected-install", created)
	before := snapshotDirectoryContents(t, created.FactoryDir)
	skipped, err := installer.InstallPackagedFactory(t.Context(), params)
	if err != nil || skipped.Outcome != factorydefinitions.PackagedFactoryInstallSkipped {
		t.Fatalf("skip = %#v, %v", skipped, err)
	}
	assertDirectorySnapshotUnchanged(t, created.FactoryDir, before)
	if len(logger.snapshot()) != 2 {
		t.Fatal("ordinary skip emitted diagnostics")
	}
	params.Replace = true
	params.Definition.JSON = []byte("opaque replacement")
	replaced, err := installer.InstallPackagedFactory(t.Context(), params)
	if err != nil || replaced.Outcome != factorydefinitions.PackagedFactoryInstallReplaced {
		t.Fatalf("replace = %#v, %v", replaced, err)
	}
	content, err := os.ReadFile(filepath.Join(replaced.FactoryDir, factorydefinitions.FactoryConfigFile))
	if err != nil || string(content) != "opaque replacement" {
		t.Fatalf("replacement content = %q, %v", content, err)
	}
	assertInstallationPair(t, logger, root, "selected-install", replaced)
}

func TestSelectedLoggerFailurePreservesContentAndOwnership(t *testing.T) {
	for _, managed := range []bool{false, true} {
		for _, mode := range []string{"malformed", "early-cancel", "acquired-cancel"} {
			t.Run(fmt.Sprintf("managed=%t/%s", managed, mode), func(t *testing.T) {
				t.Parallel()
				root, definition := t.TempDir(), installationDefinitionFixture()
				quiet := New(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir, logging.NoopLogger{})
				prior, err := quiet.InstallPackagedFactory(t.Context(), factorydefinitions.PackagedFactoryInstallParams{NamedFactoriesRoot: root, Definition: definition, ManagedRefresh: managed})
				if err != nil {
					t.Fatal(err)
				}
				before := snapshotDirectoryContents(t, prior.FactoryDir)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				cause := errors.New("private malformed payload cause")
				persistence := &installationPersistenceStub{prepareErr: cause}
				if mode != "malformed" {
					cause = context.Canceled
					persistence.prepareErr = nil
				}
				if mode == "early-cancel" {
					cancel()
				}
				if mode == "acquired-cancel" {
					persistence.prepareCancel = cancel
				}
				definition.JSON = []byte("private malformed payload")
				logger := &packagedInstallationLogger{}
				installer := New(persistence, platformfilesystem.Local{}, os.Mkdir, logger)
				var result factorydefinitions.PackagedFactoryInstallResult
				if managed {
					results, installErr := installer.EnsurePackagedFactories(ctx, root, "", []factorydefinitions.PackagedDefinition{definition})
					err, result = installErr, results[0]
				} else {
					result, err = installer.InstallPackagedFactory(ctx, factorydefinitions.PackagedFactoryInstallParams{NamedFactoriesRoot: root, Definition: definition, Replace: true})
				}
				if !errors.Is(err, cause) {
					t.Fatalf("error = %v, want cause %v", err, cause)
				}
				wantOutcome := factorydefinitions.PackagedFactoryInstallOutcome("")
				if managed {
					wantOutcome = factorydefinitions.PackagedFactoryInstallFailed
				}
				if result.Outcome != wantOutcome {
					t.Fatalf("outcome = %q, want %q", result.Outcome, wantOutcome)
				}
				assertDirectorySnapshotUnchanged(t, prior.FactoryDir, before)
				assertInstallationReleased(t, root, definition.Name)
				entries := logger.snapshot()
				if mode == "early-cancel" {
					if len(entries) != 1 {
						t.Fatalf("early cancellation entries = %#v", entries)
					}
					assertInstallationDiagnostic(t, entries[0], "error", "unknown", definition.Name, "", "failed", ownerLivenessIndeterminate, "unavailable", "", "")
				} else {
					if len(entries) != 2 {
						t.Fatalf("acquired failure entries = %#v", entries)
					}
					lease := stagingOwnershipPath(root, definition.Name)
					assertInstallationDiagnostic(t, entries[0], "info", "unknown", definition.Name, lease, "acquired", ownerLivenessActive, os.Getpid(), "", "")
					assertInstallationDiagnostic(t, entries[1], "error", "unknown", definition.Name, lease, "failed", ownerLivenessActive, os.Getpid(), "", "")
				}
			})
		}
	}
}

func TestSelectedLoggerIndeterminateContentionPreservesForeignLease(t *testing.T) {
	for _, metadata := range []string{`{"pid":"private metadata"}`, `{"pid":404}`} {
		t.Run(metadata, func(t *testing.T) {
			t.Parallel()
			root, definition := t.TempDir(), installationDefinitionFixture()
			lease := stagingOwnershipPath(root, definition.Name)
			if err := os.MkdirAll(lease, 0755); err != nil {
				t.Fatal(err)
			}
			ownerPath := filepath.Join(lease, stagingOwnerMetadataName)
			if err := os.WriteFile(ownerPath, []byte(metadata), 0600); err != nil {
				t.Fatal(err)
			}
			logger := &packagedInstallationLogger{}
			installer := newWithOwnerProbe(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir,
				&scriptedOwnerProbe{record: ownerRecord{PID: 101}, liveness: ownerLivenessIndeterminate}, logger)
			_, err := installer.InstallPackagedFactory(t.Context(), factorydefinitions.PackagedFactoryInstallParams{NamedFactoriesRoot: root, Definition: definition})
			if !errors.Is(err, factorydefinitions.ErrFactoryInstallationContention) {
				t.Fatalf("contention = %v", err)
			}
			entries := logger.snapshot()
			if len(entries) != 1 {
				t.Fatalf("entries = %#v", entries)
			}
			pid := any("unavailable")
			if metadata == `{"pid":404}` {
				pid = 404
			}
			assertInstallationDiagnostic(t, entries[0], "warn", "unknown", definition.Name, lease, "indeterminate-contention", ownerLivenessIndeterminate, pid, "", "")
			content, err := os.ReadFile(ownerPath)
			if err != nil || string(content) != metadata {
				t.Fatalf("foreign metadata = %q, %v", content, err)
			}
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(definition.Name))); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("target stat = %v", err)
			}
		})
	}
}

func TestExplicitNoopInstallationResultsAndFailures(t *testing.T) {
	for _, mode := range []string{"success", "malformed", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			root, definition := t.TempDir(), installationDefinitionFixture()
			persistence := &installationPersistenceStub{}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var cause error
			if mode == "malformed" {
				cause = errors.New("private preparation error")
				persistence.prepareErr = cause
			}
			if mode == "cancelled" {
				cause = context.Canceled
				cancel()
			}
			installer := New(persistence, platformfilesystem.Local{}, os.Mkdir, logging.NoopLogger{})
			result, err := installer.InstallPackagedFactory(ctx, factorydefinitions.PackagedFactoryInstallParams{NamedFactoriesRoot: root, Definition: definition})
			if !errors.Is(err, cause) {
				t.Fatalf("installation = %#v, %v, want %v", result, err, cause)
			}
			target := filepath.Join(root, filepath.FromSlash(definition.Name))
			if cause == nil {
				if result.Outcome != factorydefinitions.PackagedFactoryInstallCreated {
					t.Fatalf("outcome = %q", result.Outcome)
				}
				content, err := os.ReadFile(filepath.Join(target, factorydefinitions.FactoryConfigFile))
				if err != nil || string(content) != string(definition.JSON) {
					t.Fatalf("content = %q, %v", content, err)
				}
			} else if _, err := os.Stat(target); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("failed target stat = %v", err)
			}
			assertInstallationReleased(t, root, definition.Name)
		})
	}
}
