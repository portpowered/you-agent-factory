package omni_media_probe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	preflightRequiredEnvironment = "YOU_OMNI_PREFLIGHT_REQUIRED"
	preflightInputEnvironment    = "YOU_OMNI_PREFLIGHT_INPUT"
	preflightReportEnvironment   = "YOU_OMNI_PREFLIGHT_REPORT"
)

// TestProbeRunnerPrebuiltCLIHandoff is the one opt-in compiled-artifact cell
// for this preparation lane. The invoking build step owns compilation and
// supplies the strict input; this test only launches that exact artifact once.
func TestProbeRunnerPrebuiltCLIHandoff(t *testing.T) {
	required := strings.TrimSpace(os.Getenv(preflightRequiredEnvironment))
	inputPath := strings.TrimSpace(os.Getenv(preflightInputEnvironment))
	reportPath := strings.TrimSpace(os.Getenv(preflightReportEnvironment))
	if required == "" && inputPath == "" && reportPath == "" {
		t.Skip("prebuilt OMNI handoff is opt-in; no artifact input was supplied")
	}
	if required != "1" {
		t.Fatalf("%s must be 1 when any prebuilt handoff variable is set", preflightRequiredEnvironment)
	}
	for name, path := range map[string]string{
		preflightInputEnvironment:  inputPath,
		preflightReportEnvironment: reportPath,
	} {
		if path == "" {
			t.Fatalf("%s is required in strict prebuilt handoff mode", name)
		}
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			t.Fatalf("%s must be an absolute clean path, got %q", name, path)
		}
	}
	if inputPath == reportPath {
		t.Fatalf("%s and %s must be distinct", preflightInputEnvironment, preflightReportEnvironment)
	}
	if _, err := os.Lstat(reportPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("strict prebuilt handoff requires an absent report path, stat=%v", err)
	}

	input, err := ReadProbeInput(inputPath)
	if err != nil {
		t.Fatalf("read strict prebuilt handoff input: %v", err)
	}
	if _, err := os.Stat(input.ProbeRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("strict prebuilt handoff requires a fresh probe root, stat=%v", err)
	}

	executor := &prebuiltCLIExecutor{buildPath: input.Build.Path}
	ctx, cancel := context.WithTimeout(t.Context(), time.Duration(input.Limits.TimeoutSeconds)*time.Second)
	defer cancel()
	report, err := NewRunner(executor).RunPreflight(ctx, inputPath, reportPath)
	if err != nil {
		t.Fatalf("run exact prebuilt CLI preflight: %v", err)
	}
	assertPrebuiltReadyReport(t, input, report, reportPath, executor)
}

func assertPrebuiltReadyReport(t *testing.T, input ProbeInput, report Report, reportPath string, executor *prebuiltCLIExecutor) {
	t.Helper()
	if report.Status != "READY" || report.Failure != nil {
		t.Fatalf("prebuilt report = %#v, want READY without failure", report)
	}
	if report.Build.Identity != input.Build.Identity || report.Build.Bytes <= 0 || report.Build.SHA256 != input.Build.SHA256 {
		t.Fatalf("prebuilt build identity = %#v, want supplied identity and digest", report.Build)
	}
	if report.Dependencies.Model.Identity != input.Dependencies.Model.Identity || report.Dependencies.Projector.Identity != input.Dependencies.Projector.Identity || report.Dependencies.Backend.Identity != input.Dependencies.Backend.Identity {
		t.Fatalf("prebuilt dependency identities = %#v, want supplied identities", report.Dependencies)
	}
	if report.Policy.Port == ProbeForbiddenPort || len(report.Policy.RootIdentities) < 6 || !uniqueStrings(report.Policy.RootIdentities) {
		t.Fatalf("prebuilt policy = %#v, want isolated roots and dynamic non-7437 port", report.Policy)
	}
	if report.Policy.NetworkPolicy != ProbeNetworkPolicy || report.Policy.DownloadBytes != 0 || report.Policy.PaidUSD != 0 || report.Policy.MaxHeavyProcesses != ProbeMaxHeavyProcesses || report.Policy.MaxCalls != input.Limits.MaxCalls || report.Policy.MaxRetries != input.Limits.MaxRetries {
		t.Fatalf("prebuilt activity policy = %#v, want local zero-download one-heavy bounded-call policy", report.Policy)
	}
	if len(report.Journeys) != 2 || report.Journeys[0].Status != JourneyNotRun || report.Journeys[1].Status != JourneyNotRun || len(report.Outputs) != 0 {
		t.Fatalf("prebuilt semantic evidence = journeys=%#v outputs=%#v, want two NOT_RUN journeys and no outputs", report.Journeys, report.Outputs)
	}
	if len(report.Processes) != 1 {
		t.Fatalf("prebuilt process evidence = %#v, want one CLI process", report.Processes)
	}
	process := report.Processes[0]
	if process.Identity != "prebuilt-cli" || process.Kind != "prebuilt-cli" || process.Owner != "omni-media-probe" || process.PID <= 0 || !process.Started || !process.Exited || process.ExitCode != 0 || process.TimedOut {
		t.Fatalf("prebuilt process evidence = %#v, want one exited successful CLI process", process)
	}
	if report.Cleanup != (CleanupEvidence{Checked: true}) {
		t.Fatalf("prebuilt cleanup = %#v, want checked zero-survivor cleanup", report.Cleanup)
	}
	if executor.invocations != 1 || executor.buildPath != input.Build.Path || len(executor.args) != 1 || executor.args[0] != "--version" {
		t.Fatalf("prebuilt invocation = path=%q args=%#v count=%d, want exact supplied path and one --version", executor.buildPath, executor.args, executor.invocations)
	}
	if executor.stdout == "" || strings.ContainsAny(strings.TrimSpace(executor.stdout), "\r\n") || strings.TrimSpace(executor.stderr) != "" {
		t.Fatalf("prebuilt --version output = stdout=%q stderr=%q, want one stdout value and empty stderr", executor.stdout, executor.stderr)
	}
	if executor.workDir == "" || !filepath.IsAbs(executor.workDir) {
		t.Fatalf("prebuilt process work directory = %q, want isolated absolute root", executor.workDir)
	}
	if _, err := os.Stat(input.ProbeRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("prebuilt probe root survived cleanup: %v", err)
	}
	persisted, err := ReadReport(reportPath)
	if err != nil {
		t.Fatalf("strictly reread prebuilt report: %v", err)
	}
	if persisted.Status != "READY" || persisted.Processes[0].PID != process.PID {
		t.Fatalf("persisted prebuilt report = %#v, want exact READY/process evidence", persisted)
	}
	body, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read persisted prebuilt report bytes: %v", err)
	}
	for _, path := range []string{input.ProbeRoot, input.Build.Path, input.Dependencies.Model.Path, input.Dependencies.Projector.Path, input.Dependencies.Backend.Path} {
		if strings.Contains(string(body), path) {
			t.Fatalf("prebuilt report leaked owned path %q", path)
		}
	}
}

type prebuiltCLIExecutor struct {
	buildPath   string
	args        []string
	workDir     string
	stdout      string
	stderr      string
	invocations int
}

func (executor *prebuiltCLIExecutor) Execute(ctx context.Context, request ExecutionRequest) (ExecutionObservation, error) {
	executor.invocations++
	executor.args = append([]string(nil), request.Command...)
	executor.workDir = request.Roots.Work
	if request.Journey != probeJourneyCLI || len(request.Command) != 1 || request.Command[0] != "--version" {
		return ExecutionObservation{}, fmt.Errorf("unexpected prebuilt handoff request: %#v", request)
	}
	if request.Roots.Work == "" || !filepath.IsAbs(request.Roots.Work) {
		return ExecutionObservation{}, fmt.Errorf("prebuilt handoff work root is not absolute: %q", request.Roots.Work)
	}
	command := exec.CommandContext(ctx, executor.buildPath, request.Command...)
	command.Dir = request.Roots.Work
	command.Env = prebuiltCLIEnvironment(request.Roots)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	process := ProcessEvidence{Identity: "prebuilt-cli", Kind: "prebuilt-cli", Owner: "omni-media-probe"}
	if err := command.Start(); err != nil {
		return ExecutionObservation{Process: process}, err
	}
	process.Started = true
	process.PID = command.Process.Pid
	waitErr := command.Wait()
	process.Exited = command.ProcessState != nil && command.ProcessState.Exited()
	if command.ProcessState != nil {
		process.ExitCode = command.ProcessState.ExitCode()
	}
	executor.stdout = stdout.String()
	executor.stderr = stderr.String()
	observation := ExecutionObservation{Process: process}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		observation.TimedOut = true
		return observation, nil
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		observation.Cancelled = true
		return observation, nil
	}
	if waitErr != nil || process.ExitCode != 0 {
		observation.Failure = &ReportFailure{
			Owner: "prebuilt-cli", Code: "CLI_COMMAND_FAILED", Expected: "--version exits successfully",
			Observed: "prebuilt CLI returned a non-zero exit", NextAction: "inspect the supplied executable and rerun the handoff",
		}
		return observation, nil
	}
	if strings.TrimSpace(executor.stdout) == "" || strings.ContainsAny(strings.TrimSpace(executor.stdout), "\r\n") || strings.TrimSpace(executor.stderr) != "" {
		observation.Failure = &ReportFailure{
			Owner: "prebuilt-cli", Code: "CLI_VERSION_OUTPUT_INVALID", Expected: "one non-empty stdout version value and empty stderr",
			Observed: "prebuilt CLI version output was invalid", NextAction: "inspect the supplied executable output",
		}
	}
	return observation, nil
}

func prebuiltCLIEnvironment(roots RootPaths) []string {
	allowed := map[string]bool{
		"PATH": true, "PATHEXT": true, "SYSTEMROOT": true, "WINDIR": true, "COMSPEC": true,
		"SYSTEMDRIVE": true, "LANG": true, "LC_ALL": true, "LC_CTYPE": true, "LANGUAGE": true,
	}
	values := make(map[string]string, len(allowed)+10)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok && allowed[strings.ToUpper(key)] {
			values[strings.ToUpper(key)] = value
		}
	}
	set := func(key, value string) {
		if value != "" {
			values[key] = value
		}
	}
	set("HOME", roots.Profile)
	set("USERPROFILE", roots.Profile)
	set("TMPDIR", roots.Work)
	set("TMP", roots.Work)
	set("TEMP", roots.Work)
	set("XDG_CONFIG_HOME", roots.Profile)
	set("XDG_CACHE_HOME", roots.Cache)
	set("XDG_DATA_HOME", roots.Profile)
	if runtime.GOOS == "windows" {
		volume := filepath.VolumeName(roots.Profile)
		set("HOMEDRIVE", volume)
		set("HOMEPATH", strings.TrimPrefix(roots.Profile, volume))
	}
	environment := make([]string, 0, len(values))
	for key, value := range values {
		environment = append(environment, key+"="+value)
	}
	return environment
}
