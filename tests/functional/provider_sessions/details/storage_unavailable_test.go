package details

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// These read-only GETs own the API's safe error contract. One idle host shares
// immutable effects; each parallel leaf owns its provider ID and fault route.
// No Factory Session is needed for standalone Provider Session inspection.
func TestProviderSessionStorageFailuresReturnSafeAPIErrors(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	codexID := "session_fixture_codex_open_fault"
	writeCodexGoldenRolloutFixture(t, codexSessionsRoot(home), codexID, "{}\n")
	files := &unavailableSessionFiles{blockedName: "rollout-" + codexID + ".jsonl"}
	openID, pingID := "cursor-fixture-open-fault", "cursor-fixture-ping-fault"
	writeCursorGoldenSuccessStorageFixture(t, home, openID)
	writeCursorGoldenSuccessStorageFixture(t, home, pingID)
	mixedID := "session_fixture_codex_mixed_corrupt"
	// A truncated record ends at EOF; a newline-terminated malformed record
	// intentionally receives the distinct invalid-JSON diagnostic.
	writeCodexGoldenRolloutFixture(t, codexSessionsRoot(home), mixedID,
		"{\"type\":\"event_msg\",\"payload\":{\"type\":\"agent_message\",\"message\":\"retained fixture answer\"}}\n"+
			"{\"type\":\"response_item\",\"payload\":{\"type\":\"message\",\"content\":[{\"text\":\"partial")
	ping := &unavailableSessionConnector{}
	var openCalls atomic.Int32
	edges := serviceedges.Edges{
		ProviderSessionResolveHomeDirectory: func() (string, error) { return home, nil },
		ProviderSessionFileSystem:           files,
		ProviderSessionCursorOpenDatabase: func(name, dsn string) (*sql.DB, error) {
			switch {
			case strings.Contains(dsn, openID):
				openCalls.Add(1)
				return nil, privateStorageFault()
			case strings.Contains(dsn, pingID):
				return sql.OpenDB(ping), nil
			default:
				return sql.Open(name, dsn)
			}
		},
	}
	dir := support.ScaffoldSingleStepFactory(t, "provider-session-storage-failures")
	support.ClearSeedInputs(t, dir)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true, Edges: edges,
		Env: []string{"HOME=" + home, "USERPROFILE=" + home},
	})
	t.Cleanup(func() { server.Stop(t) })
	t.Run("F11-U3 truncated record retains valid transcript", func(t *testing.T) {
		t.Parallel()
		assertMixedCorruptDetail(t, server.URL(), mixedID, home)
	})
	for _, test := range []struct {
		name, provider, id string
		observed           func(*testing.T)
	}{
		{"F11-U5 Codex file open", "codex", codexID, func(t *testing.T) {
			if files.opens.Load() != 1 {
				t.Fatalf("faulted file opens = %d, want 1", files.opens.Load())
			}
		}},
		{"F11-U6 Cursor database open", "cursor", openID, func(t *testing.T) {
			if openCalls.Load() != 1 {
				t.Fatalf("faulted database opens = %d, want 1", openCalls.Load())
			}
		}},
		{"F11-U6 Cursor database Ping", "cursor", pingID, func(t *testing.T) {
			if ping.pings.Load() != 1 || ping.closes.Load() != 1 {
				t.Fatalf("database Ping/Close = %d/%d, want 1/1", ping.pings.Load(), ping.closes.Load())
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			body := getAPIProviderSessionDetailErrorBody(t, server.URL(), test.provider, "session_id", test.id, http.StatusInternalServerError)
			assertStorageFailureResponse(t, body, home)
			test.observed(t)
		})
	}
}

func assertMixedCorruptDetail(t *testing.T, baseURL, id, home string) {
	t.Helper()
	detail := support.GetJSON[factoryapi.ProviderSessionDetailResponse](t, codexProviderSessionDetailURL(baseURL, id))
	assertProviderSessionDetailIdentity(t, detail, id, factoryapi.Codex, factoryapi.LoadableProviderSessionKindSessionID)
	if detail.Parse.MalformedLineCount != 1 || len(detail.Parse.ParseErrors) != 1 ||
		detail.Parse.ParseErrors[0].Message != "truncated JSON event record" {
		t.Fatalf("mixed rollout parse = %#v, want exactly one truncated record", detail.Parse)
	}
	if len(detail.Transcript) != 1 || detail.Transcript[0].Text == nil || *detail.Transcript[0].Text != "retained fixture answer" {
		t.Fatalf("mixed rollout transcript = %#v, want retained valid answer", detail.Transcript)
	}
	encoded, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	assertCodexProviderSessionErrorBodySafe(t, "mixed-corrupt", string(encoded), home)
}

// A failed walker needs a separate immutable edge shape: walking starts at the
// whole Codex root, rather than a scenario-selected file path.
func TestCodexProviderSessionWalkFailureReturnsSafeAPIError(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	if err := os.MkdirAll(codexSessionsRoot(home), 0o755); err != nil {
		t.Fatal(err)
	}
	var walks atomic.Int32
	edges := serviceedges.Edges{
		ProviderSessionResolveHomeDirectory: func() (string, error) { return home, nil },
		ProviderSessionCodexWalkDirectory: func(string, fs.WalkDirFunc) error {
			walks.Add(1)
			return privateStorageFault()
		},
	}
	dir := support.ScaffoldSingleStepFactory(t, "provider-session-walk-failure")
	support.ClearSeedInputs(t, dir)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true, Edges: edges,
		Env: []string{"HOME=" + home, "USERPROFILE=" + home},
	})
	t.Cleanup(func() { server.Stop(t) })
	body := getAPIProviderSessionDetailErrorBody(t, server.URL(), "codex", "session_id", "session_fixture_codex_walk_fault", http.StatusInternalServerError)
	assertStorageFailureResponse(t, body, home)
	if walks.Load() != 1 {
		t.Fatalf("faulted directory walks = %d, want 1", walks.Load())
	}
}

func assertStorageFailureResponse(t *testing.T, body, home string) {
	t.Helper()
	var failure factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(body), &failure); err != nil {
		t.Fatalf("decode storage error: %v", err)
	}
	if failure.Code != factoryapi.ErrorResponseCodeINTERNALERROR || failure.Message != "failed to load provider session details" {
		t.Fatalf("storage failure = %#v, want safe INTERNAL_ERROR", failure)
	}
	for _, forbidden := range []string{home, "/private/operator/session-store", "private-storage-fault", `"transcript"`, `"providerSession"`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("storage failure leaked private data or fabricated detail: %s", body)
		}
	}
}

func privateStorageFault() error {
	return errors.New("private-storage-fault: /private/operator/session-store")
}

type unavailableSessionFiles struct {
	blockedName string
	opens       atomic.Int32
}

func (files *unavailableSessionFiles) Open(path string) (io.ReadCloser, error) {
	if filepath.Base(path) == files.blockedName {
		files.opens.Add(1)
		return nil, privateStorageFault()
	}
	return os.Open(path)
}

func (*unavailableSessionFiles) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

// OpenDB accepts a connector directly, avoiding process-global SQL driver
// registration. Each faulted database owns its connection and close evidence.
type unavailableSessionConnector struct {
	pings, closes atomic.Int32
}

func (connector *unavailableSessionConnector) Connect(context.Context) (driver.Conn, error) {
	return &unavailableSessionConnection{owner: connector}, nil
}

func (connector *unavailableSessionConnector) Driver() driver.Driver {
	return unavailableSessionDriver{connector: connector}
}

type unavailableSessionDriver struct {
	connector *unavailableSessionConnector
}

func (d unavailableSessionDriver) Open(string) (driver.Conn, error) {
	return d.connector.Connect(context.Background())
}

type unavailableSessionConnection struct {
	owner *unavailableSessionConnector
}

func (connection *unavailableSessionConnection) Ping(context.Context) error {
	connection.owner.pings.Add(1)
	return privateStorageFault()
}

func (connection *unavailableSessionConnection) Close() error {
	connection.owner.closes.Add(1)
	return nil
}

func (*unavailableSessionConnection) Prepare(string) (driver.Stmt, error) {
	return nil, privateStorageFault()
}

func (*unavailableSessionConnection) Begin() (driver.Tx, error) {
	return nil, privateStorageFault()
}
