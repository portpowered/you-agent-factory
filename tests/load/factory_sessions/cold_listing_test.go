package factory_sessions_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/pkg/initializer/application"
	platformhttp "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// One isolated profile/root process, 500 candidates (<192 MiB), no workers or
// builds. Cold means the first inventory request, not a flushed OS page cache.
// Run explicitly with -run TestColdAllListingWithinFiveSeconds -timeout=2m.
func TestColdAllListingWithinFiveSeconds(t *testing.T) {
	dir, home := t.TempDir(), t.TempDir()
	seedColdProfile(t, home)
	writeColdFile(t, filepath.Join(dir, "factory.json"), `{"name":"cold-inventory","workTypes":[],"workers":[],"workstations":[]}`)
	handler := coldListingHost(t, dir, home)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := &http.Client{Timeout: time.Minute}
	started := time.Now()
	response, err := client.Get(server.URL + "/factory-sessions?scope=all")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	elapsed := time.Since(started)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("ALL: HTTP %d, read error %v", response.StatusCode, err)
	}
	var result factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	assertColdListingContents(t, result)
	t.Logf("cold complete ALL: candidates=500 rows=490 warnings=10 bytes=%d elapsed=%s", len(body), elapsed)
	if elapsed >= 5*time.Second {
		t.Fatalf("cold ALL took %s; bound is strictly under 5s", elapsed)
	}
}

func assertColdListingContents(t *testing.T, result factoryapi.ListFactorySessionsResponse) {
	t.Helper()
	if result.RecordedSessions == nil || len(*result.RecordedSessions) != 490 || result.Warnings == nil || len(*result.Warnings) != 10 {
		t.Fatal("ALL did not return exactly 490 recorded rows and 10 warnings")
	}
	assertColdListingRows(t, *result.RecordedSessions)
	for i, warning := range *result.Warnings {
		if warning.ArtifactReference != fmt.Sprintf("2026/10/08/bad-%03d.json", i) || warning.Code != "UNREADABLE_RECORDING" || strings.Contains(warning.Reason, "planted-secret") {
			t.Fatalf("warning %d: %#v", i, warning)
		}
	}
}

func assertColdListingRows(t *testing.T, rows []factoryapi.FactorySessionRecordedSummary) {
	t.Helper()
	seen := map[string]bool{}
	for i, row := range rows {
		wantID := fmt.Sprintf("00000000-0000-4000-8000-%012d", i)
		if row.SessionId != wantID || seen[row.ArtifactReference] {
			t.Fatalf("row %d identity/order/duplicate: %#v", i, row)
		}
		seen[row.ArtifactReference] = true
	}
}

func seedColdProfile(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".you-agent-factory", "recordings", "2026", "10", "08")
	// A realistic leading snapshot forces metadata readers to skip nested
	// content; a trailing history must never be reconstructed for inventory.
	snapshot := `{"workTypes":[` + strings.Repeat(`{"name":"fixture","states":[{"name":"ready"}]},`, 1024) + `{}]}`
	for i := range 490 {
		id := fmt.Sprintf("00000000-0000-4000-8000-%012d", i)
		legacyEvents, streamEvents := coldEventHistory(id)
		var data, extension string
		if i%2 == 0 {
			extension = ".json"
			data = `{"schemaVersion":"agent-factory.replay.v1","recordedAt":"2026-10-08T00:00:00Z","factory":` + snapshot + `,"events":[` + legacyEvents + `]}`
		} else {
			extension = ".jsonl"
			data = `{"recordType":"header","schemaVersion":"agent-factory.replay.v2","recordedAt":"2026-10-08T00:00:00Z","sessionId":"` + id + `","factoryIdentity":{"id":"fixture","name":"fixture","factoryDirectory":"fixture","sourceDirectory":"fixture"},"hashes":{"factory_hash":"sha256:fixture","workers_hash":"sha256:fixture","workstations_hash":"sha256:fixture","runtime_config_hash":"sha256:fixture"}}` + "\n" + streamEvents
		}
		writeColdFile(t, filepath.Join(dir, id+extension), data)
	}
	for i := range 10 {
		writeColdFile(t, filepath.Join(dir, fmt.Sprintf("bad-%03d.json", i)), `{"private":"planted-secret","events":[`)
	}
}

func coldEventHistory(sessionID string) (string, string) {
	var legacy, stream strings.Builder
	for i := range 128 {
		event := fmt.Sprintf(`{"schemaVersion":"agent-factory.event.v1","id":"event-%d","type":"factory.tick","context":{"sessionId":"%s","eventTime":"2026-10-08T00:00:00Z"},"payload":{"text":"%s"}}`, i, sessionID, strings.Repeat("x", 1024))
		if i > 0 {
			legacy.WriteByte(',')
		}
		legacy.WriteString(event)
		stream.WriteString(`{"recordType":"event","event":` + event + "}\n")
	}
	return legacy.String(), stream.String()
}

func writeColdFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func coldListingHost(t *testing.T, dir, home string) http.Handler {
	t.Helper()
	ready := make(chan http.Handler, 1)
	process, err := root.BuildProcess(t.Context(), edges.Edges{
		FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil },
		APIServerStarter: func(ctx context.Context, req platformhttp.StartRequest) error {
			ready <- req.Handler
			if req.OnBound != nil {
				req.OnBound(platformhttp.Binding{Port: req.Port})
			}
			<-ctx.Done()
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Close(context.Background()) })
	env := builtcliacceptance.ProcessEnvForIsolatedHome(home)
	_ = process.Execute(application.Input{Args: []string{"you", "run", "--no-record", "--factory", filepath.Join(dir, "missing.json")}, Env: env, WorkingDirectory: dir, Context: t.Context(), Stdout: io.Discard, Stderr: io.Discard})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	var hostErr error
	go func() {
		hostErr = process.Execute(application.Input{Args: []string{"you", "run", "--no-record", "--dir", dir, "--continuously", "--with-server", "--quiet"}, Env: env, WorkingDirectory: dir, Context: ctx, Stdout: io.Discard, Stderr: io.Discard})
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
		if hostErr != nil && !errors.Is(hostErr, context.Canceled) {
			t.Errorf("host cleanup: %v", hostErr)
		}
	})
	select {
	case handler := <-ready:
		return handler
	case <-done:
		t.Fatalf("host exited before readiness: %v", hostErr)
	case <-t.Context().Done():
		t.Fatal("host readiness canceled")
	}
	return nil
}
