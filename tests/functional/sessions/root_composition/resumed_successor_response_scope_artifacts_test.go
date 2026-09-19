//go:build factoryartifact

package root_composition_test

import (
	"archive/zip"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type resumedResponseScopeArtifacts struct {
	recordingSource string
	recordingCopy   string
	archiveSource   string
	archiveCopy     string
	extractedRoot   string
	factoryDir      string
	ledger          resumedResponseScopeLedger
}

type resumedResponseScopeLedger struct {
	eventIDs             map[string]struct{}
	historicalWorkIDs    map[string]struct{}
	eventCount           int
	sessionCompleted     int
	historicalWorkMaxima map[string]int
}

type resumedReplayRecord struct {
	RecordType string          `json:"recordType"`
	Event      json.RawMessage `json:"event"`
}

func stageResumedResponseScopeArtifacts(t *testing.T) resumedResponseScopeArtifacts {
	t.Helper()
	recordingSource := strings.TrimSpace(os.Getenv(resumedResponseScopeRecordingEnv))
	archiveSource := strings.TrimSpace(os.Getenv(resumedResponseScopeArchiveEnv))
	if recordingSource == "" || archiveSource == "" {
		t.Fatalf(
			"immutable resumed-successor artifacts are required: set %s and %s",
			resumedResponseScopeRecordingEnv,
			resumedResponseScopeArchiveEnv,
		)
	}
	for label, path := range map[string]string{
		"recording source":       recordingSource,
		"Factory archive source": archiveSource,
	} {
		if !filepath.IsAbs(path) {
			t.Fatalf("%s path must be absolute: %q", label, path)
		}
	}
	recordingIdentity := requireResumedArtifactIdentity(
		t,
		recordingSource,
		resumedResponseScopeRecordingBytes,
		resumedResponseScopeRecordingSHA256,
		"recording source",
	)
	archiveIdentity := requireResumedArtifactIdentity(
		t,
		archiveSource,
		resumedResponseScopeArchiveBytes,
		resumedResponseScopeArchiveSHA256,
		"Factory archive source",
	)

	stageRoot := t.TempDir()
	recordingCopy := filepath.Join(stageRoot, "preserved-before-restart.jsonl")
	archiveCopy := filepath.Join(stageRoot, "factory-8f1e2e5.zip")
	copyResumedArtifact(t, recordingSource, recordingCopy)
	copyResumedArtifact(t, archiveSource, archiveCopy)
	if got := requireResumedArtifactIdentity(t, recordingCopy, recordingIdentity.bytes, recordingIdentity.hash, "recording copy"); got != recordingIdentity {
		t.Fatalf("recording copy identity = %#v, want %#v", got, recordingIdentity)
	}
	if got := requireResumedArtifactIdentity(t, archiveCopy, archiveIdentity.bytes, archiveIdentity.hash, "Factory archive copy"); got != archiveIdentity {
		t.Fatalf("Factory archive copy identity = %#v, want %#v", got, archiveIdentity)
	}
	t.Cleanup(func() {
		verifyResumedArtifactIdentity(t, recordingSource, recordingIdentity, "recording source after scenario")
		verifyResumedArtifactIdentity(t, recordingCopy, recordingIdentity, "recording copy after scenario")
		verifyResumedArtifactIdentity(t, archiveSource, archiveIdentity, "Factory archive source after scenario")
		verifyResumedArtifactIdentity(t, archiveCopy, archiveIdentity, "Factory archive copy after scenario")
	})

	extractedRoot, factoryDir := extractResumedFactoryArchive(t, archiveCopy)
	return resumedResponseScopeArtifacts{
		recordingSource: recordingSource,
		recordingCopy:   recordingCopy,
		archiveSource:   archiveSource,
		archiveCopy:     archiveCopy,
		extractedRoot:   extractedRoot,
		factoryDir:      factoryDir,
		ledger:          readResumedResponseScopeLedger(t, recordingCopy),
	}
}

type resumedArtifactIdentity struct {
	bytes int64
	hash  string
}

func requireResumedArtifactIdentity(
	t testing.TB,
	path string,
	wantBytes int64,
	wantHash string,
	label string,
) resumedArtifactIdentity {
	t.Helper()
	got, err := hashResumedArtifact(path)
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	if got.bytes != wantBytes || got.hash != strings.ToUpper(wantHash) {
		t.Fatalf(
			"%s identity = (%d bytes, %s), want (%d bytes, %s)",
			label, got.bytes, got.hash, wantBytes, strings.ToUpper(wantHash),
		)
	}
	return got
}

func verifyResumedArtifactIdentity(
	t testing.TB,
	path string,
	want resumedArtifactIdentity,
	label string,
) {
	t.Helper()
	got, err := hashResumedArtifact(path)
	if err != nil {
		t.Errorf("%s: %v", label, err)
		return
	}
	if got != want {
		t.Errorf("%s identity = %#v, want %#v", label, got, want)
	}
}

func hashResumedArtifact(path string) (resumedArtifactIdentity, error) {
	file, err := os.Open(path)
	if err != nil {
		return resumedArtifactIdentity{}, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return resumedArtifactIdentity{}, err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return resumedArtifactIdentity{}, err
	}
	return resumedArtifactIdentity{
		bytes: stat.Size(),
		hash:  strings.ToUpper(hex.EncodeToString(hash.Sum(nil))),
	}, nil
}

func copyResumedArtifact(t testing.TB, source, destination string) {
	t.Helper()
	input, err := os.Open(source)
	if err != nil {
		t.Fatalf("open immutable artifact %q: %v", source, err)
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatalf("create immutable artifact copy %q: %v", destination, err)
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		t.Fatalf("copy immutable artifact %q: %v", source, err)
	}
	if err := output.Close(); err != nil {
		t.Fatalf("close immutable artifact copy %q: %v", destination, err)
	}
}

func extractResumedFactoryArchive(t testing.TB, archivePath string) (string, string) {
	t.Helper()
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatalf("open Factory archive: %v", err)
	}
	defer reader.Close()
	extractedRoot := t.TempDir()
	rootAbs, err := filepath.Abs(extractedRoot)
	if err != nil {
		t.Fatalf("resolve Factory archive extraction root: %v", err)
	}
	for _, entry := range reader.File {
		name := filepath.Clean(filepath.FromSlash(entry.Name))
		if name == "." || name == ".." || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(os.PathSeparator)) {
			t.Fatalf("unsafe Factory archive entry %q", entry.Name)
		}
		target := filepath.Join(extractedRoot, name)
		relative, err := filepath.Rel(rootAbs, target)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			t.Fatalf("Factory archive entry escapes extraction root: %q", entry.Name)
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				t.Fatalf("create Factory archive directory %q: %v", entry.Name, err)
			}
			continue
		}
		if entry.FileInfo().Mode()&os.ModeSymlink != 0 {
			t.Fatalf("Factory archive contains unsupported symlink %q", entry.Name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatalf("create Factory archive parent for %q: %v", entry.Name, err)
		}
		input, err := entry.Open()
		if err != nil {
			t.Fatalf("open Factory archive entry %q: %v", entry.Name, err)
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			_ = input.Close()
			t.Fatalf("create extracted Factory archive entry %q: %v", entry.Name, err)
		}
		_, copyErr := io.Copy(output, input)
		closeInputErr := input.Close()
		closeOutputErr := output.Close()
		if copyErr != nil || closeInputErr != nil || closeOutputErr != nil {
			t.Fatalf("extract Factory archive entry %q: copy=%v inputClose=%v outputClose=%v", entry.Name, copyErr, closeInputErr, closeOutputErr)
		}
	}
	factoryDir := filepath.Join(extractedRoot, "factory")
	factoryConfigPath := filepath.Join(factoryDir, "factory.json")
	if _, err := os.Stat(factoryConfigPath); err != nil {
		t.Fatalf("extracted Factory archive missing factory/factory.json: %v", err)
	}
	// The archive is a direct Factory project, while session-scoped definition
	// version lookup resolves named factories beneath the selected Factory
	// root. Preserve the archive bytes and add the minimal derived catalog entry
	// needed by that public definition boundary.
	namedFactoryDir := filepath.Join(factoryDir, "you-agent-factory")
	if err := os.MkdirAll(namedFactoryDir, 0o700); err != nil {
		t.Fatalf("create extracted named Factory catalog entry: %v", err)
	}
	copyResumedArtifact(t, factoryConfigPath, filepath.Join(namedFactoryDir, "factory.json"))
	return extractedRoot, factoryDir
}

func readResumedResponseScopeLedger(t testing.TB, path string) resumedResponseScopeLedger {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open copied replay recording: %v", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 64<<20)
	ledger := resumedResponseScopeLedger{
		eventIDs:             make(map[string]struct{}),
		historicalWorkIDs:    make(map[string]struct{}),
		historicalWorkMaxima: make(map[string]int),
	}
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		var record resumedReplayRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("decode replay recording line %d: %v", lineNumber, err)
		}
		switch record.RecordType {
		case "header":
			continue
		case "event":
		default:
			t.Fatalf("replay recording line %d recordType = %q, want header or event", lineNumber, record.RecordType)
		}
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal(record.Event, &event); err != nil {
			t.Fatalf("decode replay recording event line %d: %v", lineNumber, err)
		}
		if event.Id == "" {
			t.Fatalf("replay recording line %d has empty event ID", lineNumber)
		}
		if _, exists := ledger.eventIDs[event.Id]; exists {
			t.Fatalf("replay recording duplicates event ID %q", event.Id)
		}
		ledger.eventIDs[event.Id] = struct{}{}
		ledger.eventCount++
		if event.Type == factoryapi.FactoryEventTypeSessionCompleted {
			ledger.sessionCompleted++
		}
		for _, match := range resumedResponseScopeWorkIDPattern.FindAllStringSubmatch(string(scanner.Bytes()), -1) {
			value, err := strconv.Atoi(match[2])
			if err != nil {
				t.Fatalf("parse historical Work suffix %q: %v", match[0], err)
			}
			if value > ledger.historicalWorkMaxima[match[1]] {
				ledger.historicalWorkMaxima[match[1]] = value
			}
			ledger.historicalWorkIDs[match[0]] = struct{}{}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan replay recording: %v", err)
	}
	if ledger.eventCount != 5801 {
		t.Fatalf("replay recording event count = %d, want 5801", ledger.eventCount)
	}
	if ledger.sessionCompleted == 0 {
		t.Fatal("replay recording has no historical SESSION_COMPLETED event")
	}
	return ledger
}
