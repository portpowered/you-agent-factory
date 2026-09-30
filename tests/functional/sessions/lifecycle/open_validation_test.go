package lifecycle_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Customers opening invalid folders receive diagnostics identifying the folder
// or target they must repair, without admitting a Factory Session.
func TestFactorySessionOpenReturnsFolderAndTargetDiagnostics(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(root, "broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "factory.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, folder, code string
		target             map[string]string
	}{
		{name: "missing folder", folder: filepath.Join(root, "missing"), code: "BAD_REQUEST"},
		{name: "file instead of folder", folder: file, code: "BAD_REQUEST"},
		{name: "no runnable factory", folder: t.TempDir(), code: "BAD_REQUEST"},
		{name: "malformed factory", folder: broken, code: "BAD_REQUEST"},
		{name: "unknown named target", folder: lifecycleFixture.factoryDir, code: "BAD_REQUEST", target: map[string]string{"kind": "named", "name": "missing"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			request := map[string]any{"folderPath": tc.folder}
			if tc.target != nil {
				request["target"] = tc.target
			}
			body, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			response, err := http.Post(sharedLifecycleServerURL(t)+"/factory-sessions", "application/json", bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			var diagnostic factoryapi.ErrorResponse
			if err := json.NewDecoder(response.Body).Decode(&diagnostic); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusBadRequest || string(diagnostic.Code) != tc.code || strings.TrimSpace(diagnostic.Message) == "" {
				t.Fatalf("open diagnostic = %d/%#v, want 400/%s and readable message", response.StatusCode, diagnostic, tc.code)
			}

		})
	}
}

// Validation reports a runnable target without publishing a live session.
func TestFactorySessionValidateOnlyReturnsTargetWithoutAdmission(t *testing.T) {
	t.Parallel()
	folder := t.TempDir()
	if err := writeLifecycleFactory(folder); err != nil {
		t.Fatal(err)
	}
	validateOnly := true
	baseURL := sharedLifecycleServerURL(t)
	result := postSessionLifecycleJSON[factoryapi.OpenFactorySessionResponse](t, baseURL+"/factory-sessions", factoryapi.OpenFactorySessionRequest{FolderPath: folder, ValidateOnly: &validateOnly}, "validate Factory Session")
	if result.Session != nil {
		t.Fatalf("validation admitted a session: %#v", result.Session)
	}
	if result.Targets == nil || len(*result.Targets) != 1 {
		t.Fatalf("validation targets = %#v, want one runnable target", result.Targets)
	}
	target := (*result.Targets)[0]
	if target.FolderPath != folder || target.FactoryDir != folder {
		t.Fatalf("validation target = %#v, want folder %q", target, folder)
	}
	listed := support.GetJSON[factoryapi.ListFactorySessionsResponse](t, baseURL+"/factory-sessions")
	for _, session := range listed.Sessions {
		if session.FolderPath == folder {
			t.Fatalf("validation published session %#v", session)
		}
	}
}
