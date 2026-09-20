package cancel_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
)

type cancelFixture struct {
	root        string
	factoryDir  string
	stateDir    string
	recordPath  string
	homeDir     string
	port        int
	serverURL   string
	environment []string
}

func writeCancelFixture(t *testing.T) (cancelFixture, error) {
	t.Helper()
	root := t.TempDir()
	fixture := cancelFixture{
		root:       root,
		factoryDir: filepath.Join(root, "factory"),
		stateDir:   filepath.Join(root, "process-state"),
		recordPath: filepath.Join(root, "recording.json"),
		homeDir:    filepath.Join(root, "home"),
	}
	for _, path := range []string{fixture.factoryDir, filepath.Join(fixture.factoryDir, "scripts"), fixture.stateDir, fixture.homeDir} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return cancelFixture{}, fmt.Errorf("create %s: %w", path, err)
		}
	}
	if err := writeCancelWorkerScripts(fixture.factoryDir); err != nil {
		return cancelFixture{}, err
	}
	definition, err := cancelFactoryDefinition(fixture.factoryDir, fixture.stateDir)
	if err != nil {
		return cancelFixture{}, err
	}
	if err := os.WriteFile(filepath.Join(fixture.factoryDir, "factory.json"), definition, 0o600); err != nil {
		return cancelFixture{}, fmt.Errorf("write Factory definition: %w", err)
	}
	port, err := reserveCancelPort()
	if err != nil {
		return cancelFixture{}, err
	}
	fixture.port = port
	fixture.serverURL = "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	fixture.environment = builtcliacceptance.ProcessEnvForIsolatedHome(fixture.homeDir)
	fixture.environment = setCancelEnvironment(fixture.environment, "FACTORY_RELIABILITY_CANCEL_STATE", fixture.stateDir)
	return fixture, nil
}

func cancelFactoryDefinition(factoryDir, stateDir string) ([]byte, error) {
	command := "powershell.exe"
	args := []any{
		"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-File", filepath.Join(factoryDir, "scripts", "cancel-worker.ps1"),
		"{{ (index .Inputs 0).WorkID }}", stateDir,
	}
	if runtime.GOOS == "windows" {
		path, err := exec.LookPath(command)
		if err != nil {
			return nil, fmt.Errorf("locate Windows PowerShell for real child fixture: %w", err)
		}
		command = path
	} else {
		command = "sh"
		path, err := exec.LookPath(command)
		if err != nil {
			return nil, fmt.Errorf("locate shell for real child fixture: %w", err)
		}
		command = path
		args = []any{
			filepath.Join(factoryDir, "scripts", "cancel-worker.sh"),
			"{{ (index .Inputs 0).WorkID }}", stateDir,
		}
	}
	definition := map[string]any{
		"name":      "factory-reliability-cancel-process-witness",
		"resources": []any{map[string]any{"id": cancelFixtureResourceName, "name": cancelFixtureResourceName, "capacity": 2}},
		"workTypes": []any{map[string]any{
			"name": "task",
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "complete", "type": "TERMINAL"},
				map[string]any{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []any{map[string]any{
			"name": "process-worker", "type": "SCRIPT_WORKER", "command": command,
			"args": args, "timeout": "2m",
		}},
		"workstations": []any{map[string]any{
			"name": "process", "type": "SCRIPT_RUN", "worker": "process-worker",
			"inputs":    []any{map[string]any{"workType": "task", "state": "init"}},
			"outputs":   []any{map[string]any{"workType": "task", "state": "complete"}},
			"onFailure": []any{map[string]any{"workType": "task", "state": "failed"}},
			"resources": []any{map[string]any{"name": cancelFixtureResourceName, "capacity": 1}},
		}},
	}
	encoded, err := json.MarshalIndent(definition, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode Factory definition: %w", err)
	}
	return encoded, nil
}

func writeCancelWorkerScripts(factoryDir string) error {
	scriptDir := filepath.Join(factoryDir, "scripts")
	if runtime.GOOS == "windows" {
		workerPath := filepath.Join(scriptDir, "cancel-worker.ps1")
		childPath := filepath.Join(scriptDir, "cancel-child.ps1")
		if err := os.WriteFile(workerPath, []byte(cancelWorkerPowerShell), 0o600); err != nil {
			return fmt.Errorf("write worker PowerShell fixture: %w", err)
		}
		if err := os.WriteFile(childPath, []byte(cancelChildPowerShell), 0o600); err != nil {
			return fmt.Errorf("write child PowerShell fixture: %w", err)
		}
		return nil
	}
	path := filepath.Join(scriptDir, "cancel-worker.sh")
	if err := os.WriteFile(path, []byte(cancelWorkerShell), 0o700); err != nil {
		return fmt.Errorf("write worker shell fixture: %w", err)
	}
	return nil
}

const cancelWorkerPowerShell = `param([string]$WorkID, [string]$StateRoot)
$ErrorActionPreference = "Stop"
$safeWork = [System.Text.RegularExpressions.Regex]::Replace($WorkID, "[^A-Za-z0-9._-]", "_")
$markerDir = Join-Path (Join-Path $StateRoot $safeWork) ("run-" + $PID)
New-Item -ItemType Directory -Force -Path $markerDir | Out-Null
Set-Content -LiteralPath (Join-Path $markerDir "root.pid") -Value ([string]$PID) -NoNewline
$childScript = Join-Path $PSScriptRoot "cancel-child.ps1"
$childArgs = '-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "' + $childScript + '" "' + $markerDir + '" "' + $PID + '"'
$child = Start-Process -FilePath (Join-Path $PSHOME "powershell.exe") -ArgumentList $childArgs -PassThru -WindowStyle Hidden
Set-Content -LiteralPath (Join-Path $markerDir "ready") -Value "ready" -NoNewline
[Console]::Out.WriteLine("cancel-child-late-output $WorkID")
[Console]::Out.Flush()
Set-Content -LiteralPath (Join-Path $markerDir "late-output.ready") -Value "ready" -NoNewline
$child.WaitForExit()
`

const cancelChildPowerShell = `param([string]$MarkerDir, [string]$ParentPID)
$ErrorActionPreference = "Stop"
Set-Content -LiteralPath (Join-Path $MarkerDir "child.pid") -Value ([string]$PID) -NoNewline
Set-Content -LiteralPath (Join-Path $MarkerDir "child.parent.pid") -Value $ParentPID -NoNewline
Set-Content -LiteralPath (Join-Path $MarkerDir "child.started") -Value "started" -NoNewline
Start-Sleep -Seconds 600
`

const cancelWorkerShell = `#!/bin/sh
set -u
work_id="$1"
state_root="$2"
safe_work=$(printf '%s' "$work_id" | tr -c 'A-Za-z0-9._-' '_')
marker_dir="$state_root/$safe_work/run-$$"
mkdir -p "$marker_dir"
printf '%s' "$$" > "$marker_dir/root.pid"
sh -c 'marker_dir="$1"; parent_pid="$2"; printf "%s" "$$" > "$marker_dir/child.pid"; printf "%s" "$parent_pid" > "$marker_dir/child.parent.pid"; : > "$marker_dir/child.started"; exec sleep 600' sh "$marker_dir" "$$" &
child_pid=$!
printf '%s' "$child_pid" > "$marker_dir/child.pid"
: > "$marker_dir/ready"
printf '%s\n' "cancel-child-late-output $work_id"
: > "$marker_dir/late-output.ready"
wait "$child_pid" || true
`

func cancelFixtureSHA256(factoryDir string) (string, error) {
	var paths []string
	err := filepath.WalkDir(factoryDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(factoryDir, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	hash := sha256.New()
	for _, relative := range paths {
		content, err := os.ReadFile(filepath.Join(factoryDir, filepath.FromSlash(relative)))
		if err != nil {
			return "", err
		}
		if _, err := fmt.Fprintf(hash, "%s\x00%d\x00", relative, len(content)); err != nil {
			return "", err
		}
		if _, err := hash.Write(content); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func reserveCancelPort() (int, error) {
	for attempts := 0; attempts < 3; attempts++ {
		port, err := builtcliacceptance.ReserveLocalTCPPort()
		if err != nil {
			return 0, err
		}
		if port != 7437 {
			return port, nil
		}
	}
	return 0, errors.New("could not reserve a dynamic local listener other than port 7437")
}

func setCancelEnvironment(environment []string, name, value string) []string {
	filtered := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		key, _, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, name) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return append(filtered, name+"="+value)
}
