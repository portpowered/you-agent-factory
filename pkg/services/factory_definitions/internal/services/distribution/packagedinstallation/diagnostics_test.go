package packagedinstallation

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	foundAcquired := false
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
		foundAcquired = true
	}
	if !foundAcquired {
		t.Fatalf("recovered acquired diagnostic missing: %#v", logger.snapshot())
	}
}
