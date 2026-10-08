package script_payload_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/testutil"
)

// This small integration journey consumes a prebuilt CLI and real Python.
// The ingress probe hashes stdin before delegating the unchanged bytes to the
// production router; the recording's public dispatch outputs prove destinations.
func TestPrebuiltCLIStreamsCompleteMissionPayload(t *testing.T) {
	t.Parallel()
	binary := os.Getenv("INFINITE_YOU_INTEGRATION_BINARY")
	if binary == "" {
		t.Skip("set INFINITE_YOU_INTEGRATION_BINARY to a prebuilt CLI")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("artifact=%s sha256=%x", binary, sha256.Sum256(artifact))
	python, err := exec.LookPath("python")
	if err != nil {
		t.Fatal(err)
	}
	project, factory, home := preparePayloadFactory(t, python)
	payloads := map[string]string{"ordinary": `{"title":"ordinary"}`, "large": `{"mission":"` + strings.Repeat("café 😀 $() ", 3000) + `"}`}
	if len(payloads["large"]) <= 40*1024 {
		t.Fatal("fixture does not cross Windows argv limit")
	}
	works := []map[string]any{}
	for name, payload := range payloads {
		works = append(works, map[string]any{"name": name, "workTypeName": "thoughts", "payload": json.RawMessage(payload)})
	}
	batch, err := json.Marshal(map[string]any{"type": "FACTORY_REQUEST_BATCH", "works": works})
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(factory, "inputs", "BATCH", "default")
	if err := os.MkdirAll(input, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(input, "batch.json"), batch, 0600); err != nil {
		t.Fatal(err)
	}
	recording := filepath.Join(project, "recording.json")
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "run", "--dir", factory, "--session", uuid.NewString(), "--quiet", "--record", recording)
	cmd.Dir = project
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "HOME", "USERPROFILE", "YOU_HOME", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_CACHE_HOME":
			continue
		}
		cmd.Env = append(cmd.Env, entry)
	}
	for _, key := range []string{"HOME", "USERPROFILE", "YOU_HOME", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_CACHE_HOME"} {
		cmd.Env = append(cmd.Env, key+"="+home)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("CLI exit: %v; stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	for name, payload := range payloads {
		data, err := os.ReadFile(filepath.Join(project, name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var ingress struct {
			Bytes  int
			Sha256 string
		}
		if err := json.Unmarshal(data, &ingress); err != nil {
			t.Fatal(err)
		}
		if ingress.Bytes != len(payload) || ingress.Sha256 != fmt.Sprintf("%x", sha256.Sum256([]byte(payload))) {
			t.Fatalf("%s ingress=%s", name, data)
		}
		t.Logf("%s ingress bytes=%d sha256=%s", name, ingress.Bytes, ingress.Sha256)
	}
	assertPayloadRecordingRoutes(t, recording)
}

func preparePayloadFactory(t *testing.T, python string) (string, string, string) {
	t.Helper()
	repo, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	factory := filepath.Join(project, "factory")
	home := filepath.Join(project, "home")
	for _, dir := range []string{factory, home} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	probe := filepath.Join(project, "probe.py")
	script := `import sys, hashlib, json, pathlib, subprocess
name, output, router = sys.argv[1:]
payload = sys.stdin.buffer.read()
pathlib.Path(output, name + ".json").write_text(json.dumps({"bytes":len(payload),"sha256":hashlib.sha256(payload).hexdigest()}), encoding="utf-8")
result = subprocess.run([sys.executable, router, "--payload-stdin"], input=payload)
sys.exit(result.returncode)
`
	if err := os.WriteFile(probe, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	config := map[string]any{
		"name":         "real-script-stdin",
		"workTypes":    []any{map[string]any{"name": "thoughts", "states": []any{map[string]any{"name": "init", "type": "INITIAL"}, map[string]any{"name": "mission-ready", "type": "TERMINAL"}, map[string]any{"name": "supervising", "type": "TERMINAL"}, map[string]any{"name": "reporting-failed", "type": "FAILED"}}}},
		"workers":      []any{map[string]any{"name": "router", "type": "SCRIPT_WORKER", "command": python, "args": []string{probe, `{{ (index .Inputs 0).Name }}`, project, filepath.Join(repo, "factory", "scripts", "route-thoughts.py")}, "stdin": `{{ (index .Inputs 0).Payload }}`}},
		"workstations": []any{map[string]any{"name": "route", "type": "CLASSIFIER_WORKSTATION", "worker": "router", "inputs": []any{map[string]any{"workType": "thoughts", "state": "init"}}, "classificationRoutes": []any{map[string]any{"label": "mission", "outputs": []any{map[string]any{"workType": "thoughts", "state": "mission-ready"}}}, map[string]any{"label": "supervision", "outputs": []any{map[string]any{"workType": "thoughts", "state": "supervising"}}}}, "onFailure": []any{map[string]any{"workType": "thoughts", "state": "reporting-failed"}}, "workPropagation": map[string]any{"mode": "PRESERVE_INPUT"}}},
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(factory, "factory.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	return project, factory, home
}

func assertPayloadRecordingRoutes(t *testing.T, recording string) {
	t.Helper()
	observed := map[string]string{}
	for _, event := range testutil.LoadReplayArtifact(t, recording).Events {
		if event.Type != "DISPATCH_RESPONSE" {
			continue
		}
		var response struct {
			OutputWork []struct {
				Name  string
				State struct{ Name string }
			} `json:"outputWork"`
		}
		if err := json.Unmarshal(event.Payload, &response); err != nil {
			t.Fatal(err)
		}
		for _, item := range response.OutputWork {
			observed[item.Name] = item.State.Name
		}
	}
	if observed["large"] != "mission-ready" || observed["ordinary"] != "supervising" {
		t.Fatalf("public output Work destinations=%v", observed)
	}
}
