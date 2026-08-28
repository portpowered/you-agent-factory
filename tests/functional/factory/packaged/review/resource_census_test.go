package review

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type packagedReviewCleanupPath string

const (
	packagedReviewCleanupSuccess          packagedReviewCleanupPath = "success"
	packagedReviewCleanupRejection        packagedReviewCleanupPath = "rejection"
	packagedReviewCleanupFailure          packagedReviewCleanupPath = "failure"
	packagedReviewCleanupCancellation     packagedReviewCleanupPath = "cancellation"
	packagedReviewCleanupTimeout          packagedReviewCleanupPath = "timeout"
	packagedReviewCleanupAssertionFailure packagedReviewCleanupPath = "assertion-failure"
	packagedReviewCleanupPackageTeardown  packagedReviewCleanupPath = "package-teardown"
)

var packagedReviewCleanupPaths = []packagedReviewCleanupPath{
	packagedReviewCleanupSuccess,
	packagedReviewCleanupRejection,
	packagedReviewCleanupFailure,
	packagedReviewCleanupCancellation,
	packagedReviewCleanupTimeout,
	packagedReviewCleanupAssertionFailure,
	packagedReviewCleanupPackageTeardown,
}

var packagedReviewResourceKinds = []string{
	"definition",
	"workspace",
	"selector",
	"runtime",
	"replay",
}

type packagedReviewCensusRecord struct {
	name         string
	rootDir      string
	factoryDir   string
	workspace    string
	selector     string
	requestID    string
	sessionID    string
	workID       string
	eventIDs     []string
	cleaned      bool
	rootAbsent   bool
	sessionGone  bool
	selectorGone bool
	resourceGone map[string]bool
}

type packagedReviewResourceCensus struct {
	mu                   sync.Mutex
	records              []packagedReviewCensusRecord
	cleanupPaths         map[packagedReviewCleanupPath]int
	cleanupPathCensusRun bool
	processStarts        int
}

func newPackagedReviewResourceCensus() *packagedReviewResourceCensus {
	return &packagedReviewResourceCensus{
		cleanupPaths: make(map[packagedReviewCleanupPath]int),
	}
}

func (census *packagedReviewResourceCensus) recordProcessStart() {
	if census == nil {
		return
	}
	census.mu.Lock()
	defer census.mu.Unlock()
	census.processStarts++
}

func (census *packagedReviewResourceCensus) recordPath(path packagedReviewCleanupPath) {
	if census == nil {
		return
	}
	census.mu.Lock()
	defer census.mu.Unlock()
	if census.cleanupPaths == nil {
		census.cleanupPaths = make(map[packagedReviewCleanupPath]int)
	}
	census.cleanupPaths[path]++
}

func (census *packagedReviewResourceCensus) enableCleanupPathCensus() {
	if census == nil {
		return
	}
	census.mu.Lock()
	defer census.mu.Unlock()
	census.cleanupPathCensusRun = true
}

func (census *packagedReviewResourceCensus) cleanupPathSummary() string {
	if census == nil {
		return ""
	}
	census.mu.Lock()
	defer census.mu.Unlock()
	parts := make([]string, 0, len(packagedReviewCleanupPaths))
	for _, path := range packagedReviewCleanupPaths {
		parts = append(parts, fmt.Sprintf("%s=%d", path, census.cleanupPaths[path]))
	}
	return strings.Join(parts, ",")
}

func (census *packagedReviewResourceCensus) register(record packagedReviewCensusRecord) {
	census.mu.Lock()
	defer census.mu.Unlock()
	census.records = append(census.records, record)
}

func (census *packagedReviewResourceCensus) recordEvidence(
	requestID, workID string, eventIDs []string,
) {
	census.mu.Lock()
	defer census.mu.Unlock()
	for index := range census.records {
		if census.records[index].requestID != requestID {
			continue
		}
		census.records[index].workID = workID
		census.records[index].eventIDs = append([]string(nil), eventIDs...)
		return
	}
}

func (census *packagedReviewResourceCensus) recordCleanup(
	requestID string, sessionGone, rootAbsent, selectorGone bool,
) {
	census.mu.Lock()
	defer census.mu.Unlock()
	for index := range census.records {
		if census.records[index].requestID != requestID {
			continue
		}
		census.records[index].cleaned = true
		census.records[index].rootAbsent = rootAbsent
		census.records[index].sessionGone = sessionGone
		census.records[index].selectorGone = selectorGone
		census.records[index].resourceGone = map[string]bool{
			"definition": pathAbsent(census.records[index].factoryDir),
			"workspace":  pathAbsent(census.records[index].workspace),
			"selector":   pathAbsent(census.records[index].selector),
			"runtime":    pathAbsent(census.records[index].factoryDir),
			"replay":     pathAbsent(census.records[index].factoryDir),
		}
		return
	}
}

func (census *packagedReviewResourceCensus) snapshot() []packagedReviewCensusRecord {
	census.mu.Lock()
	defer census.mu.Unlock()
	records := make([]packagedReviewCensusRecord, len(census.records))
	copy(records, census.records)
	for index := range records {
		records[index].eventIDs = append([]string(nil), records[index].eventIDs...)
		resourceGone := records[index].resourceGone
		records[index].resourceGone = make(map[string]bool, len(records[index].resourceGone))
		for kind, gone := range resourceGone {
			records[index].resourceGone[kind] = gone
		}
	}
	return records
}

func (census *packagedReviewResourceCensus) closedError() error {
	if census == nil {
		return nil
	}
	var errs []error
	for _, record := range census.snapshot() {
		if !record.cleaned {
			errs = append(errs, fmt.Errorf("Review census record %q was not cleaned", record.name))
		}
		if !record.rootAbsent {
			errs = append(errs, fmt.Errorf("Review census root %q remains", record.rootDir))
		}
		if !record.sessionGone {
			errs = append(errs, fmt.Errorf("Review census session %q remains", record.sessionID))
		}
		if !record.selectorGone {
			errs = append(errs, fmt.Errorf("Review census selector %q remains", record.selector))
		}
		for _, kind := range packagedReviewResourceKinds {
			if !record.resourceGone[kind] {
				errs = append(errs, fmt.Errorf("Review census %s resource remains for %q", kind, record.name))
			}
		}
	}
	census.mu.Lock()
	if census.cleanupPathCensusRun {
		for _, path := range packagedReviewCleanupPaths {
			if census.cleanupPaths[path] == 0 {
				errs = append(errs, fmt.Errorf("Review cleanup path %q was not observed", path))
			}
		}
	}
	census.mu.Unlock()
	return errors.Join(errs...)
}

func assertPackagedReviewResourceCensus(
	t *testing.T,
	fixture *packagedReviewSharedFixture,
) {
	t.Helper()
	records := fixture.census.snapshot()
	if len(records) == 0 {
		t.Fatal("Review resource census recorded no shared scenarios")
	}
	assertUniqueReviewCensusField(t, "Factory", records, func(record packagedReviewCensusRecord) string {
		return record.factoryDir
	})
	assertUniqueReviewCensusField(t, "workspace", records, func(record packagedReviewCensusRecord) string {
		return record.workspace
	})
	assertUniqueReviewCensusField(t, "selector", records, func(record packagedReviewCensusRecord) string {
		return record.selector
	})
	assertUniqueReviewCensusField(t, "request", records, func(record packagedReviewCensusRecord) string {
		return record.requestID
	})
	assertUniqueReviewCensusField(t, "Factory Session", records, func(record packagedReviewCensusRecord) string {
		return record.sessionID
	})
	assertUniqueReviewCensusField(t, "Work", records, func(record packagedReviewCensusRecord) string {
		return record.workID
	})
	assertUniqueReviewEventIDs(t, records)
	for _, record := range records {
		if !record.cleaned || !record.rootAbsent || !record.sessionGone || !record.selectorGone {
			t.Fatalf("Review census record = %#v, want cleaned root/session/selector", record)
		}
		for _, kind := range packagedReviewResourceKinds {
			if !record.resourceGone[kind] {
				t.Fatalf("Review census record = %#v, want absent %s resource", record, kind)
			}
		}
	}
	eventCount := 0
	rootsAbsent := 0
	sessionDeletes := 0
	resourceCounts := make(map[string]int, len(packagedReviewResourceKinds))
	for _, record := range records {
		eventCount += len(record.eventIDs)
		if record.rootAbsent {
			rootsAbsent++
		}
		if record.sessionGone {
			sessionDeletes++
		}
		for _, kind := range packagedReviewResourceKinds {
			if record.resourceGone[kind] {
				resourceCounts[kind]++
			}
		}
	}
	registeredSelectors := fixture.providerRunner.registeredCount()
	if registeredSelectors != 0 {
		t.Fatalf("registered Review selectors after scenario cleanup = %d, want 0", registeredSelectors)
	}
	closedSessions := make(map[string]struct{}, len(records))
	for _, record := range records {
		closedSessions[record.sessionID] = struct{}{}
	}
	live := support.GetJSON[factoryapi.ListFactorySessionsResponse](
		t,
		strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions?scope=live",
	)
	defaultSessions := 0
	durableMirrors := 0
	for _, session := range live.Sessions {
		if session.IsDefault {
			defaultSessions++
			continue
		}
		if _, closed := closedSessions[session.Id]; closed &&
			strings.TrimSpace(session.FactoryDir) == "" &&
			strings.TrimSpace(session.FolderPath) == "" && session.Runtime == nil {
			// The current list adapter also projects terminal durable execution
			// rows into the live response with only their session identity. The
			// direct per-session GET above is the live deletion oracle; keep this
			// durable mirror visible in the census rather than mistaking it for a
			// live workspace session.
			durableMirrors++
			continue
		}
		t.Fatalf("unexpected live Review session after scenario cleanup = %#v", session)
	}
	t.Logf(
		"GATE-ISO-002/GATE-CLEAN-004 Review census: scenarios=%d processStarts=%d explicitSessions=%d liveDefaultSessions=%d durableMirrors=%d selectors=%d uniqueFactories=%d uniqueWorkspaces=%d uniqueRequests=%d uniqueWorks=%d eventIDs=%d rootsAbsent=%d sessionDeletes=%d definitionsAbsent=%d workspacesAbsent=%d runtimeArtifactsAbsent=%d replayArtifactsAbsent=%d cleanupPaths=%s",
		len(records), fixture.census.processStarts, len(records), defaultSessions, durableMirrors, registeredSelectors,
		len(records), len(records), len(records), len(records), eventCount, rootsAbsent, sessionDeletes,
		resourceCounts["definition"], resourceCounts["workspace"], resourceCounts["runtime"], resourceCounts["replay"],
		fixture.census.cleanupPathSummary(),
	)
}

func pathAbsent(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return errors.Is(err, os.ErrNotExist)
}

func assertUniqueReviewCensusField(
	t *testing.T,
	label string,
	records []packagedReviewCensusRecord,
	value func(packagedReviewCensusRecord) string,
) {
	t.Helper()
	seen := make(map[string]string, len(records))
	for _, record := range records {
		current := value(record)
		if strings.TrimSpace(current) == "" {
			t.Fatalf("Review census %s identity is empty in %#v", label, record)
		}
		if prior, ok := seen[current]; ok {
			t.Fatalf("Review census %s identity %q is shared by %q and %q", label, current, prior, record.name)
		}
		seen[current] = record.name
	}
}

func assertUniqueReviewEventIDs(t *testing.T, records []packagedReviewCensusRecord) {
	t.Helper()
	seen := make(map[string]string)
	for _, record := range records {
		if len(record.eventIDs) == 0 {
			t.Fatalf("Review census record %q has no Factory Event identities", record.name)
		}
		for _, eventID := range record.eventIDs {
			scopedID := record.sessionID + "\x00" + eventID
			if prior, ok := seen[scopedID]; ok {
				t.Fatalf("Review Factory Event identity %q in session %q is shared by %q and %q", eventID, record.sessionID, prior, record.name)
			}
			seen[scopedID] = record.name
		}
	}
}

func assertPackagedReviewSessionDeleted(t testing.TB, baseURL, sessionID string) bool {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Errorf("GET deleted Review Factory Session %q: %v", sessionID, err)
		return false
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return true
	}
	body, _ := io.ReadAll(response.Body)
	t.Errorf(
		"GET deleted Review Factory Session %q status = %d, want 404: %s",
		sessionID,
		response.StatusCode,
		strings.TrimSpace(string(body)),
	)
	return false
}

func assertPackagedReviewPortClosed(baseURL string) error {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("parse Review fixture URL: %w", err)
	}
	connection, err := net.DialTimeout("tcp", parsed.Host, 250*time.Millisecond)
	if err == nil {
		_ = connection.Close()
		return fmt.Errorf("Review fixture port %q still accepts connections", parsed.Host)
	}
	return nil
}

// testPackagedReviewCleanupPathCensus exercises the package-owned cleanup
// ownership with representative terminal causes. The real shared scenarios
// provide the live Factory Session, Work, event/replay, and workspace
// resources; this bounded probe verifies that the same root/selector teardown
// is independent of success, rejection, failure, cancellation, timeout, or an
// assertion-failure cause. It does not substitute for the real runtime edges.
func testPackagedReviewCleanupPathCensus(t *testing.T) {
	t.Helper()
	fixture := sharedPackagedReviewFixture(t)
	fixture.census.enableCleanupPathCensus()
	for _, path := range packagedReviewCleanupPaths[:len(packagedReviewCleanupPaths)-1] {
		path := path
		t.Run(string(path), func(t *testing.T) {
			rootDir, err := os.MkdirTemp("", "you-functional-packaged-review-cleanup-")
			if err != nil {
				t.Fatalf("create cleanup probe root: %v", err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(rootDir) })
			factoryDir := filepath.Join(rootDir, "factory")
			workspaceDir := filepath.Join(factoryDir, "workspace")
			runtimeDir := filepath.Join(rootDir, "runtime")
			replayDir := filepath.Join(rootDir, "replay")
			for _, directory := range []string{workspaceDir, runtimeDir, replayDir} {
				if err := os.MkdirAll(directory, 0o755); err != nil {
					t.Fatalf("create cleanup probe resource %q: %v", directory, err)
				}
			}
			for name, content := range map[string]string{
				filepath.Join(factoryDir, "factory.json"):     "{}",
				filepath.Join(workspaceDir, "workspace.json"): "{}",
				filepath.Join(runtimeDir, "session.json"):     "{}",
				filepath.Join(replayDir, "events.jsonl"):      "event",
			} {
				if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
					t.Fatalf("write cleanup probe resource %q: %v", name, err)
				}
			}
			selector := filepath.Join(factoryDir, "selector")
			before := fixture.providerRunner.registeredCount()
			fixture.providerRunner.register(selector, &packagedReviewCommandRunner{})
			cause := errors.New(string(path))
			switch path {
			case packagedReviewCleanupCancellation:
				cause = context.Canceled
			case packagedReviewCleanupTimeout:
				cause = context.DeadlineExceeded
			case packagedReviewCleanupAssertionFailure:
				cause = errors.New("assertion failed")
			}
			t.Logf("cleanup cause=%v; owned resources remain under %q until teardown", cause, rootDir)
			fixture.providerRunner.unregister(selector)
			if err := os.RemoveAll(rootDir); err != nil {
				t.Fatalf("remove cleanup probe root for %q: %v", path, err)
			}
			if _, err := os.Stat(rootDir); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("cleanup probe root for %q remains: %v", path, err)
			}
			if got := fixture.providerRunner.registeredCount(); got != before {
				t.Fatalf("cleanup probe selectors after %q = %d, want baseline %d", path, got, before)
			}
			fixture.census.recordPath(path)
		})
	}
}
