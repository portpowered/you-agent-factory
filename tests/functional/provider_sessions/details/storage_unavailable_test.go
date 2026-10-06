package details

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
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
	openID, pingID := "cursor-fixture-open-fault", "cursor-fixture-ping-fault"
	writeCursorGoldenSuccessStorageFixture(t, home, openID)
	writeCursorGoldenSuccessStorageFixture(t, home, pingID)
	ping := &unavailableSessionConnector{}
	var openCalls atomic.Int32
	edges := serviceedges.Edges{
		ProviderSessionResolveHomeDirectory: func() (string, error) { return home, nil },
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
	server := startStorageFailureHost(t, home, edges)
	for _, test := range []struct {
		name, provider, id string
		observed           func(*testing.T)
	}{
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

func startStorageFailureHost(t *testing.T, home string, edges serviceedges.Edges) *support.FunctionalAPIServer {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "provider-session-storage-failures")
	support.ClearSeedInputs(t, dir)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true, Edges: edges,
		Env: []string{"HOME=" + home, "USERPROFILE=" + home},
		// First-run installation is fixture setup, not the reader behavior.
		// Complete it before starting the host's observable readiness window.
		BeforeStart: func(tb testing.TB, process support.Process, input root.Input) {
			support.InitializeCustomerHomeWithProcess(tb, process, input.Env, input.WorkingDirectory)
		},
	})
	t.Cleanup(func() { server.Stop(t) })
	return server
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
