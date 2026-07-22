package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanRejectsInitializerImplementationAndTransportConstruction(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/initializer/application/build.go": `package application
import (
  execution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution"
  runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
)
func build() {
  _ = execution.NewService()
  _ = runcli.NewOperation()
}`,
	})

	findings := scanFixture(t, root)
	assertFinding(t, findings, ruleInitializerServiceImplementation, "/execution")
	assertFinding(t, findings, ruleInitializerTransport, "/cli/run")
	assertFinding(t, findings, ruleInitializerConstruction, "execution.NewService")
	assertFinding(t, findings, ruleInitializerConstruction, "run.NewOperation")
}

func TestScanAllowsInitializerLifecycleAndServiceRootContracts(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/initializer/runtime.go": `package initializer
import (
  "context"
  sessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)
type Open func(context.Context) (sessions.ExecutionService, error)
func coordinate(open Open) (sessions.ExecutionService, error) {
  return open(context.Background())
}`,
		"pkg/wire/wire.go": `package wire
import execution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution"
func inject() { _ = execution.NewService() }`,
	})

	if findings := scanFixture(t, root); len(findings) != 0 {
		t.Fatalf("allowed lifecycle/root/Wire usage produced findings: %#v", findings)
	}
}

func TestScanRejectsPlatformDomainImportsAndAllowsExactLeafPorts(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/platform/runtimeinput/config.go": `package runtimeinput
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
type Config struct { Runtime runtime.Service }`,
		"pkg/platform/http/provider.go": `package http
import inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
type Adapter struct { Invoke inference.Invoker }`,
	})

	findings := scanFixture(t, root)
	if len(findings) != 1 {
		t.Fatalf("platform findings = %#v, want one domain import", findings)
	}
	assertFinding(t, findings, rulePlatformDomainImport, "/factory_runtime")
}

func TestScanAllowsOnlyContentStagingAdapterToImportWorkRootPort(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/platform/contentstaging/adapters.go": `package contentstaging
import work "github.com/portpowered/infinite-you/pkg/services/work"
var _ work.ContentStagingFileSystem`,
		"pkg/platform/contentstaging/broad.go": `package contentstaging
import internal "github.com/portpowered/infinite-you/pkg/services/work/internal"
var _ internal.Service`,
		"pkg/platform/other/adapters.go": `package other
import work "github.com/portpowered/infinite-you/pkg/services/work"
var _ work.ContentStagingFileSystem`,
	})

	findings := scanFixture(t, root)
	if len(findings) != 2 {
		t.Fatalf("platform findings = %#v, want broader Work and other Platform imports rejected", findings)
	}
	assertFinding(t, findings, rulePlatformDomainImport, "/work/internal")
	for _, item := range findings {
		if item.FilePath == "pkg/platform/other/adapters.go" &&
			item.Target == modulePath+"/pkg/services/work" {
			return
		}
	}
	t.Fatalf("findings %#v do not reject Work root import from other Platform package", findings)
}

func TestScanRejectsMappingExternalBehavior(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/transports/mapping/bad.go": `package mapping
import (
  "os"
  "os/exec"
  "time"
)
func mapValue() {
  _ = os.WriteFile("state", nil, 0600)
  _ = exec.Command("worker")
  time.Sleep(time.Second)
  go func() {}()
}`,
		"pkg/transports/mapping/conversion.go": `package mapping
import (
  "time"
)
func convert(value string) {
  _, _ = time.Parse(time.RFC3339, value)
}`,
	})

	findings := scanFixture(t, root)
	assertFinding(t, findings, ruleMappingFilesystem, "os.WriteFile")
	assertFinding(t, findings, ruleMappingProcess, "os/exec")
	assertFinding(t, findings, ruleMappingProcess, "os/exec.Command")
	assertFinding(t, findings, ruleMappingTimer, "time.Sleep")
	assertFinding(t, findings, ruleMappingGoroutine, "go statement")
	for _, item := range findings {
		if item.FilePath == "pkg/transports/mapping/conversion.go" {
			t.Fatalf("pure time conversion produced a finding: %#v", item)
		}
	}
}

func TestScanRejectsMappingFilesystemReads(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/transports/mapping/source.go": `package mapping
import "os"
func load(path string) ([]byte, error) { return os.ReadFile(path) }`,
	})

	findings := scanFixture(t, root)
	assertFinding(t, findings, ruleMappingFilesystem, "os.ReadFile")
}

func TestScanRejectsDotImportsThatHideOwnedBehavior(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/initializer/application/root.go": `package application
import . "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
var _ = NewService`,
		"pkg/transports/mapping/hidden.go": `package mapping
import . "time"
var _ = Sleep`,
	})

	findings := scanFixture(t, root)
	assertFinding(t, findings, ruleInitializerConstruction, "/factory_sessions.*")
	assertFinding(t, findings, ruleMappingTimer, "time.*")
}

func TestRunAcceptsRecordedDebtAndRejectsNewAndStaleEntries(t *testing.T) {
	const sourcePath = "pkg/platform/runtimeinput/config.go"
	const target = modulePath + "/pkg/services/factory_runtime"
	source := `package runtimeinput
import runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
type Config struct { Runtime runtime.Service }`

	t.Run("active", func(t *testing.T) {
		root := fixtureRepository(t, map[string]string{sourcePath: source})
		writeBaseline(t, root, baselineEntryFor(rulePlatformDomainImport, sourcePath, target))
		var stdout, stderr bytes.Buffer
		if err := run(config{root: root}, &stdout, &stderr); err != nil {
			t.Fatalf("run active baseline: %v; stderr=%s", err, stderr.String())
		}
		if !strings.Contains(stdout.String(), "1 deletion-only baseline") {
			t.Fatalf("stdout = %q, want active debt count", stdout.String())
		}
	})

	t.Run("new", func(t *testing.T) {
		root := fixtureRepository(t, map[string]string{sourcePath: source})
		var stdout, stderr bytes.Buffer
		err := run(config{root: root}, &stdout, &stderr)
		if err == nil || !strings.Contains(stderr.String(), "new violation") {
			t.Fatalf("run new violation err=%v stderr=%q", err, stderr.String())
		}
	})

	t.Run("zero debt requires no baseline file", func(t *testing.T) {
		root := fixtureRepository(t, map[string]string{
			"pkg/platform/runtimeinput/config.go": "package runtimeinput\n",
		})
		var stdout, stderr bytes.Buffer
		if err := run(config{root: root}, &stdout, &stderr); err != nil {
			t.Fatalf("run without baseline at zero debt: %v; stderr=%s", err, stderr.String())
		}

		writeBaseline(t, root)
		err := run(config{root: root}, &stdout, &stderr)
		if err == nil || !strings.Contains(err.Error(), "delete the file to record zero debt") {
			t.Fatalf("run empty baseline err=%v, want deletion requirement", err)
		}
	})

	t.Run("stale", func(t *testing.T) {
		root := fixtureRepository(t, map[string]string{
			sourcePath: "package runtimeinput\n",
		})
		writeBaseline(t, root, baselineEntryFor(rulePlatformDomainImport, sourcePath, target))
		var stdout, stderr bytes.Buffer
		err := run(config{root: root}, &stdout, &stderr)
		if err == nil || !strings.Contains(stderr.String(), "stale baseline") {
			t.Fatalf("run stale entry err=%v stderr=%q", err, stderr.String())
		}
	})
}

func TestCreateBaselineRefusesToOverwrite(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/platform/runtimeinput/config.go": `package runtimeinput
import _ "github.com/portpowered/infinite-you/pkg/services/factory_runtime"`,
	})
	var stdout, stderr bytes.Buffer
	if err := run(config{root: root, createBaseline: true}, &stdout, &stderr); err != nil {
		t.Fatalf("create baseline: %v", err)
	}
	if err := run(config{root: root, createBaseline: true}, &stdout, &stderr); err == nil ||
		!strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("second create error = %v, want overwrite refusal", err)
	}
}

func TestCreateBaselineRefusesZeroDebtFile(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/platform/runtimeinput/config.go": "package runtimeinput\n",
	})
	var stdout, stderr bytes.Buffer
	err := run(config{root: root, createBaseline: true}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "absence records zero debt") {
		t.Fatalf("create zero-debt baseline error = %v, want absence requirement", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, baselineFile)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("zero-debt baseline file exists or stat failed unexpectedly: %v", statErr)
	}
}

func scanFixture(t *testing.T, root string) []finding {
	t.Helper()
	findings, err := scan(root)
	if err != nil {
		t.Fatalf("scan fixture: %v", err)
	}
	return findings
}

func assertFinding(t *testing.T, findings []finding, rule, targetPart string) {
	t.Helper()
	for _, item := range findings {
		if item.Rule == rule && strings.Contains(item.Target, targetPart) {
			return
		}
	}
	t.Errorf("findings %#v do not contain rule %q target %q", findings, rule, targetPart)
}

func fixtureRepository(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for relative, contents := range files {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create fixture directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", relative, err)
		}
	}
	return root
}

func writeBaseline(t *testing.T, root string, entries ...baselineEntry) {
	t.Helper()
	data, err := json.Marshal(baseline{Version: baselineVersion, Entries: entries})
	if err != nil {
		t.Fatalf("marshal baseline: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, baselineFile), data, 0o600); err != nil {
		t.Fatalf("write baseline: %v", err)
	}
}

func baselineEntryFor(rule, filePath, target string) baselineEntry {
	return baselineEntry{
		Rule: rule, FilePath: filePath, Target: target,
		Stage: baselineStage, DeletionGate: deletionGates[rule],
	}
}
