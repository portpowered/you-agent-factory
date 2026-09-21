package release_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
)

const (
	localAICandidatePacketDirectoryEnvironment   = "INFINITE_YOU_LOCALAI_CANDIDATE_PACKET_DIR"
	localAICandidateInstallReportPathEnvironment = "INFINITE_YOU_LOCALAI_INSTALL_REPORT_PATH"
	localAICandidatePacketExpectedTree           = "1bb0badc9faa244005d81e9255cb44050b4b720f"
	localAICandidateInstallScratchMaximumBytes   = int64(536870912)
)

type localAICandidatePublicInstallResult struct {
	Status                    string                               `json:"status"`
	PreinstallAbsent          bool                                 `json:"preinstallAbsent"`
	StateRootsInitiallyEmpty  bool                                 `json:"stateRootsInitiallyEmpty"`
	ModelCalls                int                                  `json:"modelCalls"`
	ModelBackendDownloadBytes int64                                `json:"modelBackendDownloadBytes"`
	PathResolution            string                               `json:"pathResolution"`
	Version                   string                               `json:"version"`
	ExpectedVersion           string                               `json:"expectedVersion"`
	ExecutableBuildInfo       localAICandidateExecutableBuildInfo  `json:"executableBuildInfo"`
	Budgets                   localAICandidateInstallBudgets       `json:"budgets"`
	Discovery                 localAICandidateDiscoveryEvidence    `json:"discovery"`
	Activity                  localAICandidateActivityEvidence     `json:"activity"`
	Observer                  localAICandidateObserverEvidence     `json:"observer"`
	DistributionObserver      localAICandidateDistributionEvidence `json:"distributionObserver"`
	Cleanup                   localAICandidateInstallCleanup       `json:"cleanup"`
}

type localAICandidateExecutableBuildInfo struct {
	SourceRevision string `json:"sourceRevision"`
	VCSModified    bool   `json:"vcsModified"`
}

type localAICandidateInstallBudgets struct {
	InstallerTimeoutSeconds          int    `json:"installerTimeoutSeconds"`
	DiscoveryCommandTimeoutSeconds   int    `json:"discoveryCommandTimeoutSeconds"`
	CommandOutputMaximumBytes        int    `json:"commandOutputMaximumBytes"`
	MaximumDiscoveryCommands         int    `json:"maximumDiscoveryCommands"`
	MaximumDistributionRequests      int    `json:"maximumDistributionRequests"`
	MaximumModelCalls                int    `json:"maximumModelCalls"`
	MaximumModelBackendDownloadBytes int64  `json:"maximumModelBackendDownloadBytes"`
	MaximumBackendProcessStarts      int    `json:"maximumBackendProcessStarts"`
	AllowedNetwork                   string `json:"allowedNetwork"`
	ForbiddenPort                    int    `json:"forbiddenPort"`
}

type localAICandidateDiscoveryEvidence struct {
	Status                         string                                     `json:"status"`
	WorkingDirectory               string                                     `json:"workingDirectory"`
	WorkingDirectoryInitiallyEmpty bool                                       `json:"workingDirectoryInitiallyEmpty"`
	WorkingDirectoryEmptyAfter     bool                                       `json:"workingDirectoryEmptyAfter"`
	NoCurrentFactory               bool                                       `json:"noCurrentFactory"`
	Commands                       []localAICandidateDiscoveryCommandEvidence `json:"commands"`
}

type localAICandidateDiscoveryCommandEvidence struct {
	Arguments           []string `json:"arguments"`
	ExitCode            int      `json:"exitCode"`
	TimedOut            bool     `json:"timedOut"`
	OutputLimitExceeded bool     `json:"outputLimitExceeded"`
	ElapsedMilliseconds int64    `json:"elapsedMilliseconds"`
	TimeoutSeconds      int      `json:"timeoutSeconds"`
	OutputMaximumBytes  int      `json:"outputMaximumBytes"`
	Stdout              string   `json:"stdout"`
	StdoutEvidence      struct {
		Bytes     int64  `json:"bytes"`
		SHA256    string `json:"sha256"`
		Truncated bool   `json:"truncated"`
	} `json:"stdoutEvidence"`
}

type localAICandidateActivityEvidence struct {
	Observed                  bool     `json:"observed"`
	CacheBytesBefore          int64    `json:"cacheBytesBefore"`
	CacheBytesAfter           int64    `json:"cacheBytesAfter"`
	CacheBytesDelta           int64    `json:"cacheBytesDelta"`
	ModelCalls                int      `json:"modelCalls"`
	ModelBackendDownloadBytes int64    `json:"modelBackendDownloadBytes"`
	RuntimeEvidenceObserved   bool     `json:"runtimeEvidenceObserved"`
	RuntimeEvidenceRecords    int      `json:"runtimeEvidenceRecords"`
	BackendProcessStarts      int      `json:"backendProcessStarts"`
	CacheRoots                []string `json:"cacheRoots"`
}

type localAICandidateObserverEvidence struct {
	Observed                     bool   `json:"observed"`
	SampleCount                  int    `json:"sampleCount"`
	AttributedProcessCount       int    `json:"attributedProcessCount"`
	NonLoopbackConnections       int    `json:"nonLoopbackConnections"`
	Port7437Accesses             int    `json:"port7437Accesses"`
	BackendProcessStarts         int    `json:"backendProcessStarts"`
	SurvivingTaskProcesses       int    `json:"survivingTaskProcesses"`
	LoopbackDistributionRequests int    `json:"loopbackDistributionRequests"`
	DistributionListenerPort     int    `json:"distributionListenerPort"`
	DistributionListenerStopped  bool   `json:"distributionListenerStopped"`
	DistributionObserverObserved bool   `json:"distributionObserverObserved"`
	DistributionObserverError    string `json:"distributionObserverError"`
	Error                        string `json:"error"`
}

type localAICandidateDistributionEvidence struct {
	Observed               bool                          `json:"observed"`
	ListeningPort          int                           `json:"listeningPort"`
	RequestCount           int                           `json:"requestCount"`
	NonLoopbackConnections int                           `json:"nonLoopbackConnections"`
	Port7437Accesses       int                           `json:"port7437Accesses"`
	ListenerStopped        bool                          `json:"listenerStopped"`
	Requests               []localAICandidateHTTPRequest `json:"requests"`
}

type localAICandidateHTTPRequest struct {
	Method        string `json:"method"`
	Path          string `json:"path"`
	RemoteAddress string `json:"remoteAddress"`
	LocalAddress  string `json:"localAddress"`
	LocalPort     int    `json:"localPort"`
	StatusCode    int    `json:"statusCode"`
}

type localAICandidateInstallCleanup struct {
	Status                  string `json:"status"`
	ListenerStopped         bool   `json:"listenerStopped"`
	InstallDirectoryRemoved bool   `json:"installDirectoryRemoved"`
	PartialFilesRemaining   int    `json:"partialFilesRemaining"`
	Error                   string `json:"error"`
}

func assertLocalAICandidateInstallIdentity(t *testing.T, result localAICandidatePublicInstallResult, installDirectory, version, targetRevision string) {
	t.Helper()
	if result.Status != "PASS" || !result.PreinstallAbsent || !result.StateRootsInitiallyEmpty ||
		result.PathResolution == "" || !strings.EqualFold(filepath.Clean(result.PathResolution), filepath.Clean(filepath.Join(installDirectory, "you.exe"))) ||
		result.Version != version || result.ExpectedVersion != version ||
		result.ExecutableBuildInfo.SourceRevision != targetRevision || result.ExecutableBuildInfo.VCSModified {
		t.Fatalf("public candidate install identity = %#v", result)
	}
}

func assertLocalAICandidateInstallBudgets(t *testing.T, budgets localAICandidateInstallBudgets) {
	t.Helper()
	if budgets.InstallerTimeoutSeconds != 300 || budgets.DiscoveryCommandTimeoutSeconds != 120 ||
		budgets.CommandOutputMaximumBytes != 1048576 || budgets.MaximumDiscoveryCommands != 10 ||
		budgets.MaximumDistributionRequests != 4 || budgets.MaximumModelCalls != 0 ||
		budgets.MaximumModelBackendDownloadBytes != 0 || budgets.MaximumBackendProcessStarts != 0 ||
		budgets.ForbiddenPort != 7437 || budgets.AllowedNetwork == "" {
		t.Fatalf("public candidate install budgets = %#v", budgets)
	}
}

func assertLocalAICandidateInstallDiscovery(t *testing.T, discovery localAICandidateDiscoveryEvidence, budgets localAICandidateInstallBudgets) {
	t.Helper()
	if discovery.Status != "PASS" || discovery.WorkingDirectory == "" ||
		!discovery.WorkingDirectoryInitiallyEmpty || !discovery.WorkingDirectoryEmptyAfter ||
		!discovery.NoCurrentFactory || len(discovery.Commands) != budgets.MaximumDiscoveryCommands {
		t.Fatalf("public candidate discovery context = %#v", discovery)
	}
	want := map[string]bool{
		"--version": true, "--help": true, "docs models": true, "docs agents": true, "models list": true, "models --help": true,
		"--json models inspect llm": true, "--json models inspect asr": true,
		"--json models inspect tts": true, "--json models inspect embed": true,
	}
	for _, command := range discovery.Commands {
		assertLocalAICandidateDiscoveryCommand(t, command, budgets)
		identity := strings.Join(command.Arguments, " ")
		if _, expected := want[identity]; !expected {
			t.Fatalf("unexpected public candidate discovery command %q", identity)
		}
		delete(want, identity)
		if identity == "models list" {
			for _, name := range []string{"llm", "asr", "tts", "embed"} {
				if !strings.Contains(command.Stdout, name) {
					t.Fatalf("models list output did not include %s", name)
				}
			}
		}
	}
	if len(want) != 0 {
		t.Fatalf("public candidate discovery omitted commands: %#v", want)
	}
	assertLocalAICandidateInertModelStates(t, discovery.Commands)
}

func assertLocalAICandidateDiscoveryCommand(t *testing.T, command localAICandidateDiscoveryCommandEvidence, budgets localAICandidateInstallBudgets) {
	t.Helper()
	if command.ExitCode != 0 || command.TimedOut || command.OutputLimitExceeded || command.ElapsedMilliseconds < 0 ||
		command.TimeoutSeconds != budgets.DiscoveryCommandTimeoutSeconds || command.OutputMaximumBytes != budgets.CommandOutputMaximumBytes ||
		strings.TrimSpace(command.Stdout) == "" || command.StdoutEvidence.Bytes <= 0 ||
		command.StdoutEvidence.SHA256 == "" || command.StdoutEvidence.Truncated {
		t.Fatalf("public candidate discovery command evidence = %#v", command)
	}
}

func assertLocalAICandidateInertModelStates(t *testing.T, commands []localAICandidateDiscoveryCommandEvidence) {
	t.Helper()
	for _, name := range []string{"llm", "asr", "tts", "embed"} {
		identity := "--json models inspect " + name
		var stdout string
		for _, command := range commands {
			if strings.Join(command.Arguments, " ") == identity {
				stdout = command.Stdout
				break
			}
		}
		var model struct {
			Name           string `json:"name"`
			ManagedRuntime struct {
				Identity       string `json:"identity"`
				ReadinessState string `json:"readinessState"`
				LifecycleState string `json:"lifecycleState"`
			} `json:"managedRuntime"`
			LoadState string `json:"loadState"`
		}
		if err := json.Unmarshal([]byte(stdout), &model); err != nil || model.Name != name || model.ManagedRuntime.Identity != name ||
			model.ManagedRuntime.ReadinessState != "MISSING" || model.ManagedRuntime.LifecycleState != "NOT_INSTALLED" || model.LoadState != "UNLOADED" {
			t.Fatalf("public candidate model %s state = %#v; decode error: %v", name, model, err)
		}
	}
}

func assertLocalAICandidateInstallActivity(t *testing.T, result localAICandidatePublicInstallResult) {
	t.Helper()
	activity := result.Activity
	if !activity.Observed || activity.CacheBytesBefore != 0 || activity.CacheBytesAfter != 0 || activity.CacheBytesDelta != 0 ||
		result.ModelCalls != 0 || result.ModelBackendDownloadBytes != 0 || activity.ModelCalls != 0 ||
		activity.ModelBackendDownloadBytes != 0 || !activity.RuntimeEvidenceObserved || activity.RuntimeEvidenceRecords != 0 ||
		activity.BackendProcessStarts != 0 || len(activity.CacheRoots) != 4 {
		t.Fatalf("public candidate model activity = %#v", activity)
	}
}

func assertLocalAICandidateInstallObserver(t *testing.T, observer localAICandidateObserverEvidence) {
	t.Helper()
	if !observer.Observed || observer.SampleCount <= 0 || observer.AttributedProcessCount <= 0 ||
		observer.NonLoopbackConnections != 0 || observer.Port7437Accesses != 0 || observer.BackendProcessStarts != 0 ||
		observer.SurvivingTaskProcesses != 0 || observer.LoopbackDistributionRequests != 4 ||
		observer.DistributionListenerPort <= 0 || observer.DistributionListenerPort == 7437 ||
		!observer.DistributionListenerStopped || !observer.DistributionObserverObserved ||
		observer.DistributionObserverError != "" || observer.Error != "" {
		t.Fatalf("public candidate observer = %#v", observer)
	}
}

func assertLocalAICandidateInstallDistribution(t *testing.T, result localAICandidatePublicInstallResult, archiveVersion string) {
	t.Helper()
	distribution := result.DistributionObserver
	if !distribution.Observed || distribution.RequestCount != 4 || distribution.ListeningPort <= 0 ||
		distribution.ListeningPort == 7437 || distribution.ListeningPort != result.Observer.DistributionListenerPort ||
		distribution.NonLoopbackConnections != 0 || distribution.Port7437Accesses != 0 ||
		!distribution.ListenerStopped || len(distribution.Requests) != 4 {
		t.Fatalf("public candidate distribution observer = %#v", distribution)
	}
	routeVersion := strings.TrimPrefix(archiveVersion, "v")
	want := map[string]int{
		"/download/v" + routeVersion + "/install.ps1":                                         200,
		"/releases/download/v" + routeVersion + "/you_" + routeVersion + "_windows_amd64.zip": 200,
		"/releases/download/v" + routeVersion + "/you_" + routeVersion + "_checksums.txt":     200,
		"/stop": 204,
	}
	for _, request := range distribution.Requests {
		remoteAddress := net.ParseIP(request.RemoteAddress)
		localAddress := net.ParseIP(request.LocalAddress)
		status, expected := want[request.Path]
		if !expected || request.Method != "GET" || request.StatusCode != status || remoteAddress == nil || localAddress == nil ||
			!remoteAddress.IsLoopback() || !localAddress.IsLoopback() || request.LocalPort != distribution.ListeningPort {
			t.Fatalf("public candidate distribution request = %#v", request)
		}
		delete(want, request.Path)
	}
	if len(want) != 0 {
		t.Fatalf("public candidate distribution requests are missing: %#v", want)
	}
}

func assertLocalAICandidateInstallCleanup(t *testing.T, cleanup localAICandidateInstallCleanup) {
	t.Helper()
	if cleanup.Status != "PASS" || !cleanup.ListenerStopped || !cleanup.InstallDirectoryRemoved ||
		cleanup.PartialFilesRemaining != 0 || cleanup.Error != "" {
		t.Fatalf("public candidate install cleanup = %#v", cleanup)
	}
}

func TestLocalAICandidateImmutablePacketPublicInstallAndDiscovery(t *testing.T) {
	t.Parallel()
	packetDirectory, targetRevision, packet := loadLocalAICandidatePacketInputs(t)
	tempDirectory := t.TempDir()
	run := runLocalAICandidatePacketInstall(t, tempDirectory, packetDirectory, packet, targetRevision)
	assertLocalAICandidatePublicInstallResult(t, run.resultPath, run.installDirectory, run.version, run.archiveVersion, targetRevision)
	installResult, err := os.ReadFile(run.resultPath)
	if err != nil {
		t.Fatalf("read install result for retained evidence: %v", err)
	}
	installResult = bytes.TrimPrefix(installResult, []byte{0xef, 0xbb, 0xbf})
	scratchBytes, err := localAICandidateDirectoryBytes(tempDirectory)
	if err != nil || scratchBytes > localAICandidateInstallScratchMaximumBytes {
		t.Fatalf("install scratch bytes = %d, limit %d, error = %v", scratchBytes, localAICandidateInstallScratchMaximumBytes, err)
	}
	if err := os.RemoveAll(tempDirectory); err != nil {
		t.Fatalf("remove task-owned install scratch: %v", err)
	}
	if _, err := os.Stat(tempDirectory); !os.IsNotExist(err) {
		t.Fatalf("task-owned install scratch remains after cleanup: %v", err)
	}
	packetAfter, err := verifyLocalAICandidatePacket(t, packetDirectory, targetRevision)
	if err != nil {
		t.Fatal(err)
	}
	if packet.manifestSHA256 != packetAfter.manifestSHA256 || strings.Join(packet.artifactDigests, "\n") != strings.Join(packetAfter.artifactDigests, "\n") {
		t.Fatal("immutable packet bytes changed during the loopback install")
	}
	writeLocalAICandidateInstallReportIfRequested(t, packetDirectory, packetAfter, installResult, scratchBytes)
}

type localAICandidatePacketInstallRun struct {
	resultPath       string
	installDirectory string
	version          string
	archiveVersion   string
}

type localAICandidatePacketInstallInputs struct {
	binaryPath         string
	goPath             string
	version            string
	archiveVersion     string
	installDirectory   string
	candidateDirectory string
	workDirectory      string
	resultPath         string
	environment        []string
}

func loadLocalAICandidatePacketInputs(t *testing.T) (string, string, localAICandidateVerifiedPacket) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell candidate delivery is Windows-only")
	}
	packetDirectory := strings.TrimSpace(os.Getenv(localAICandidatePacketDirectoryEnvironment))
	if packetDirectory == "" {
		t.Skip("set INFINITE_YOU_LOCALAI_CANDIDATE_PACKET_DIR to a finalized immutable packet")
	}
	if !filepath.IsAbs(packetDirectory) {
		t.Fatalf("%s must be absolute: %s", localAICandidatePacketDirectoryEnvironment, packetDirectory)
	}
	packetDirectory = filepath.Clean(packetDirectory)
	targetRevision := localAICandidateSourceCommit(t)
	packet, err := verifyLocalAICandidatePacket(t, packetDirectory, targetRevision)
	if err != nil {
		t.Fatal(err)
	}
	return packetDirectory, targetRevision, packet
}

func runLocalAICandidatePacketInstall(t *testing.T, tempDirectory, packetDirectory string, packet localAICandidateVerifiedPacket, targetRevision string) localAICandidatePacketInstallRun {
	t.Helper()
	inputs := prepareLocalAICandidatePacketInstall(t, tempDirectory, packetDirectory, packet, targetRevision)
	for _, directory := range []string{inputs.candidateDirectory, inputs.workDirectory} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatalf("create task-owned install root: %v", err)
		}
	}
	harnessPath := filepath.Join(tempDirectory, "install-immutable-packet.ps1")
	harness := localAICandidatePacketInstallHarness(t, packetDirectory, packet, targetRevision, inputs)
	runLocalAICandidateHarness(t, harnessPath, harness, inputs.environment, tempDirectory)
	return localAICandidatePacketInstallRun{
		resultPath: inputs.resultPath, installDirectory: inputs.installDirectory,
		version: inputs.version, archiveVersion: inputs.archiveVersion,
	}
}

func prepareLocalAICandidatePacketInstall(t *testing.T, tempDirectory, packetDirectory string, packet localAICandidateVerifiedPacket, targetRevision string) localAICandidatePacketInstallInputs {
	t.Helper()
	binaryPath := filepath.Join(packetDirectory, packet.artifacts["windows-amd64-executable"].File)
	assertLocalAICandidateBuildInfo(t, binaryPath, targetRevision)
	version, archiveVersion, goPath, environment := localAICandidatePacketVersion(t, tempDirectory, binaryPath, packet)
	return localAICandidatePacketInstallInputs{
		binaryPath: binaryPath, goPath: goPath, version: version, archiveVersion: archiveVersion, environment: environment,
		installDirectory: filepath.Join(tempDirectory, "install"), candidateDirectory: filepath.Join(tempDirectory, "candidate-output"),
		workDirectory: filepath.Join(tempDirectory, "work"), resultPath: filepath.Join(tempDirectory, "install-result.json"),
	}
}

func localAICandidatePacketVersion(t *testing.T, tempDirectory, binaryPath string, packet localAICandidateVerifiedPacket) (string, string, string, []string) {
	t.Helper()
	versionRoot, environment := localAICandidatePrepareIsolatedEnvironment(t, tempDirectory)
	versionContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(versionContext, binaryPath, "--version")
	command.Dir = versionRoot
	command.Env = environment
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("read immutable packet executable version: %v\n%s", err, output)
	}
	version := strings.TrimSpace(string(output))
	if version == "" || strings.ContainsAny(version, "\r\n") {
		t.Fatalf("immutable packet executable returned invalid version %q", version)
	}
	archiveVersion, ok := localAICandidateArchiveVersion(packet.artifacts["windows-amd64-archive"].File)
	if !ok {
		t.Fatalf("immutable packet archive has an unexpected name: %s", packet.artifacts["windows-amd64-archive"].File)
	}
	goPath, err := exec.LookPath("go.exe")
	if err != nil {
		t.Fatalf("locate Go for installed build-info inspection: %v", err)
	}
	return version, archiveVersion, goPath, environment
}

func localAICandidatePacketInstallHarness(t *testing.T, packetDirectory string, packet localAICandidateVerifiedPacket, targetRevision string, inputs localAICandidatePacketInstallInputs) string {
	t.Helper()
	return fmt.Sprintf(
		". %s -InstallDir %s\n$result = Invoke-InstalledCandidateSmoke -CandidateDirectory %s -ArchivePath %s -ChecksumPath %s -InstallerPath %s -ArchiveExecutablePath %s -Version %s -ExpectedExecutableVersion %s -ExpectedSourceCommit %s -GoExecutablePath %s -RequestedInstallDir %s -WorkDirectory %s\n$result | ConvertTo-Json -Depth 16 | Set-Content -LiteralPath %s -Encoding UTF8\n",
		localAICandidatePowerShellLiteral(localAICandidateScriptPath(t)), localAICandidatePowerShellLiteral(inputs.installDirectory),
		localAICandidatePowerShellLiteral(inputs.candidateDirectory),
		localAICandidatePowerShellLiteral(filepath.Join(packetDirectory, packet.artifacts["windows-amd64-archive"].File)),
		localAICandidatePowerShellLiteral(filepath.Join(packetDirectory, packet.artifacts["detached-checksums"].File)),
		localAICandidatePowerShellLiteral(filepath.Join(packetDirectory, packet.artifacts["windows-installer"].File)),
		localAICandidatePowerShellLiteral(inputs.binaryPath), localAICandidatePowerShellLiteral(inputs.archiveVersion),
		localAICandidatePowerShellLiteral(inputs.version), localAICandidatePowerShellLiteral(targetRevision),
		localAICandidatePowerShellLiteral(inputs.goPath), localAICandidatePowerShellLiteral(inputs.installDirectory),
		localAICandidatePowerShellLiteral(inputs.workDirectory), localAICandidatePowerShellLiteral(inputs.resultPath),
	)
}

func writeLocalAICandidateInstallReportIfRequested(t *testing.T, packetDirectory string, packet localAICandidateVerifiedPacket, installResult []byte, scratchBytes int64) {
	t.Helper()
	reportPath := strings.TrimSpace(os.Getenv(localAICandidateInstallReportPathEnvironment))
	if reportPath == "" {
		return
	}
	reportPath = filepath.Clean(reportPath)
	if !filepath.IsAbs(reportPath) || localAICandidatePathWithin(packetDirectory, reportPath) {
		t.Fatalf("install report path must be absolute and outside the immutable packet: %s", reportPath)
	}
	driverRoot := testutil.MustRepoRoot(t)
	if status := localAICandidateGit(t, driverRoot, "status", "--porcelain"); status != "" {
		t.Fatalf("retained install report requires a clean driver worktree, got %s", status)
	}
	driverRevision := localAICandidateGit(t, driverRoot, "rev-parse", "HEAD")
	if err := writeLocalAICandidateInstallLoopbackReport(reportPath, packet, installResult, scratchBytes, driverRevision); err != nil {
		t.Fatalf("write install report: %v", err)
	}
}

type localAICandidatePacketArtifact struct {
	Role   string `json:"role"`
	File   string `json:"file"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type localAICandidatePacketManifest struct {
	SchemaVersion string `json:"schemaVersion"`
	Status        string `json:"status"`
	Source        struct {
		Repository  string `json:"repository"`
		Commit      string `json:"commit"`
		Tree        string `json:"tree"`
		VCSModified bool   `json:"vcsModified"`
	} `json:"source"`
	Target struct {
		OS   string `json:"os"`
		Arch string `json:"arch"`
	} `json:"target"`
	Build struct {
		MaximumWorkBytes int64 `json:"maximumWorkBytes"`
		MaximumChildren  int   `json:"maximumChildren"`
		WorkBytes        int64 `json:"workBytes"`
	} `json:"build"`
	Artifacts []localAICandidatePacketArtifact `json:"artifacts"`
}

type localAICandidateVerifiedPacket struct {
	manifest        localAICandidatePacketManifest
	manifestSHA256  string
	artifacts       map[string]localAICandidatePacketArtifact
	artifactDigests []string
}

func verifyLocalAICandidatePacket(t *testing.T, packetDirectory, targetRevision string) (localAICandidateVerifiedPacket, error) {
	t.Helper()
	var verified localAICandidateVerifiedPacket
	manifest, digest, err := readLocalAICandidatePacketManifest(t, packetDirectory, targetRevision)
	if err != nil {
		return verified, err
	}
	artifacts, artifactDigests, err := verifyLocalAICandidatePacketArtifacts(packetDirectory, manifest.Artifacts)
	if err != nil {
		return verified, err
	}
	verified.manifest = manifest
	verified.manifestSHA256 = digest
	verified.artifacts = artifacts
	verified.artifactDigests = artifactDigests
	return verified, nil
}

func readLocalAICandidatePacketManifest(t *testing.T, packetDirectory, targetRevision string) (localAICandidatePacketManifest, string, error) {
	t.Helper()
	var manifest localAICandidatePacketManifest
	manifestBytes, err := os.ReadFile(filepath.Join(packetDirectory, "candidate-report.json"))
	if err != nil {
		return manifest, "", fmt.Errorf("read packet manifest: %w", err)
	}
	manifestHash := sha256.Sum256(manifestBytes)
	digest := fmt.Sprintf("%x", manifestHash)
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return manifest, "", fmt.Errorf("decode packet manifest: %w", err)
	}
	if manifest.SchemaVersion != "local-windows-candidate/v2" || manifest.Status != "PASS" ||
		manifest.Source.Repository != "https://github.com/portpowered/you-agent-factory.git" ||
		manifest.Source.Commit != targetRevision || manifest.Source.Tree != localAICandidatePacketExpectedTree || manifest.Source.VCSModified ||
		manifest.Target.OS != "windows" || manifest.Target.Arch != "amd64" ||
		manifest.Build.MaximumWorkBytes <= 0 || manifest.Build.WorkBytes <= 0 || manifest.Build.WorkBytes > manifest.Build.MaximumWorkBytes ||
		manifest.Build.MaximumChildren < 1 || manifest.Build.MaximumChildren > 4 {
		return manifest, "", fmt.Errorf("packet manifest identity or budget is invalid: source=%s/%s target=%s/%s work=%d/%d children=%d",
			manifest.Source.Commit, manifest.Source.Tree, manifest.Target.OS, manifest.Target.Arch,
			manifest.Build.WorkBytes, manifest.Build.MaximumWorkBytes, manifest.Build.MaximumChildren)
	}
	if tree := localAICandidateGit(t, testutil.MustRepoRoot(t), "rev-parse", targetRevision+"^{tree}"); tree != localAICandidatePacketExpectedTree {
		return manifest, "", fmt.Errorf("source commit tree is %s, want %s", tree, localAICandidatePacketExpectedTree)
	}
	sidecarBytes, err := os.ReadFile(filepath.Join(packetDirectory, "candidate-report.sha256"))
	if err != nil {
		return manifest, "", fmt.Errorf("read manifest digest sidecar: %w", err)
	}
	sidecar := strings.Fields(string(sidecarBytes))
	if len(sidecar) != 2 || !strings.EqualFold(sidecar[0], digest) || sidecar[1] != "candidate-report.json" {
		return manifest, "", fmt.Errorf("manifest digest sidecar does not match candidate-report.json")
	}
	return manifest, digest, nil
}

func verifyLocalAICandidatePacketArtifacts(packetDirectory string, artifacts []localAICandidatePacketArtifact) (map[string]localAICandidatePacketArtifact, []string, error) {
	wantRoles := map[string]bool{
		"windows-amd64-archive": true, "windows-amd64-executable": true, "windows-installer": true,
		"detached-checksums": true, "public-doc-snapshot": true,
	}
	if len(artifacts) != len(wantRoles) {
		return nil, nil, fmt.Errorf("packet has %d artifacts, want %d", len(artifacts), len(wantRoles))
	}
	verified := make(map[string]localAICandidatePacketArtifact, len(artifacts))
	digests := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		if !wantRoles[artifact.Role] || artifact.File == "" || filepath.Base(artifact.File) != artifact.File || artifact.Bytes <= 0 || len(artifact.SHA256) != 64 {
			return nil, nil, fmt.Errorf("invalid packet artifact: %#v", artifact)
		}
		if _, exists := verified[artifact.Role]; exists {
			return nil, nil, fmt.Errorf("packet artifact role %q is duplicated", artifact.Role)
		}
		bytes, digest, err := localAICandidatePacketFileDigest(filepath.Join(packetDirectory, artifact.File))
		if err != nil {
			return nil, nil, fmt.Errorf("hash packet artifact %s: %w", artifact.File, err)
		}
		if bytes != artifact.Bytes || !strings.EqualFold(digest, artifact.SHA256) {
			return nil, nil, fmt.Errorf("packet artifact %s identity differs from manifest", artifact.File)
		}
		verified[artifact.Role] = artifact
		digests = append(digests, artifact.Role+":"+artifact.File+":"+digest)
		delete(wantRoles, artifact.Role)
	}
	if len(wantRoles) != 0 {
		return nil, nil, fmt.Errorf("packet is missing artifact roles: %#v", wantRoles)
	}
	archive := verified["windows-amd64-archive"]
	executable := verified["windows-amd64-executable"]
	if err := verifyLocalAICandidateArchiveExecutable(filepath.Join(packetDirectory, archive.File), executable.Bytes, executable.SHA256); err != nil {
		return nil, nil, err
	}
	if err := verifyLocalAICandidateArchiveChecksum(packetDirectory, verified["detached-checksums"], archive); err != nil {
		return nil, nil, err
	}
	return verified, digests, nil
}

func verifyLocalAICandidateArchiveChecksum(packetDirectory string, checksumArtifact, archive localAICandidatePacketArtifact) error {
	checksumBytes, err := os.ReadFile(filepath.Join(packetDirectory, checksumArtifact.File))
	if err != nil {
		return fmt.Errorf("read packet checksum file: %w", err)
	}
	for _, line := range strings.Split(string(checksumBytes), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.TrimPrefix(fields[len(fields)-1], "*") == archive.File {
			if !strings.EqualFold(fields[0], archive.SHA256) {
				return fmt.Errorf("detached checksum does not match %s", archive.File)
			}
			return nil
		}
	}
	return fmt.Errorf("detached checksums do not include %s", archive.File)
}
func verifyLocalAICandidateArchiveExecutable(archivePath string, expectedBytes int64, expectedSHA256 string) error {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open packet archive: %w", err)
	}
	defer archive.Close()
	found := false
	for _, member := range archive.File {
		if !strings.EqualFold(filepath.Base(filepath.FromSlash(member.Name)), "you.exe") {
			continue
		}
		if found {
			return fmt.Errorf("packet archive contains multiple you.exe members")
		}
		found = true
		reader, err := member.Open()
		if err != nil {
			return fmt.Errorf("open archive you.exe: %w", err)
		}
		hash := sha256.New()
		bytes, copyErr := io.Copy(hash, reader)
		closeErr := reader.Close()
		if copyErr != nil {
			return fmt.Errorf("read archive you.exe: %w", copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close archive you.exe: %w", closeErr)
		}
		if bytes != expectedBytes || !strings.EqualFold(fmt.Sprintf("%x", hash.Sum(nil)), expectedSHA256) {
			return fmt.Errorf("archive you.exe identity differs from retained executable")
		}
	}
	if !found {
		return fmt.Errorf("packet archive does not contain you.exe")
	}
	return nil
}

func localAICandidateArchiveVersion(archiveFile string) (string, bool) {
	if !strings.HasPrefix(archiveFile, "you_") || !strings.HasSuffix(archiveFile, "_windows_amd64.zip") {
		return "", false
	}
	version := strings.TrimSuffix(strings.TrimPrefix(archiveFile, "you_"), "_windows_amd64.zip")
	return version, version != ""
}

func localAICandidatePacketFileDigest(path string) (int64, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer file.Close()
	hash := sha256.New()
	bytes, err := io.Copy(hash, file)
	if err != nil {
		return 0, "", err
	}
	return bytes, fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func localAICandidateDirectoryBytes(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	return total, err
}

func localAICandidatePathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == "." || relative == ".." || !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func writeLocalAICandidateInstallLoopbackReport(path string, packet localAICandidateVerifiedPacket, installResult []byte, scratchBytes int64, driverRevision string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("install report already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect install report path: %w", err)
	}
	sidecarPath := path + ".sha256"
	if _, err := os.Stat(sidecarPath); err == nil {
		return fmt.Errorf("install report sidecar already exists: %s", sidecarPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect install report sidecar path: %w", err)
	}
	if !json.Valid(installResult) {
		return fmt.Errorf("install result is not valid JSON")
	}
	report := map[string]any{
		"schemaVersion": "local-windows-install-loopback/v1", "status": "PASS", "recordedAt": time.Now().UTC().Format(time.RFC3339Nano),
		"driverRevision": driverRevision,
		"source":         packet.manifest.Source, "target": packet.manifest.Target,
		"packet": map[string]any{
			"candidateManifestSha256": packet.manifestSHA256, "artifacts": packet.manifest.Artifacts,
			"manifestStableAfterInstall": true, "artifactBytesStableAfterInstall": true,
		},
		"budgets": map[string]any{
			"maximumCandidateWorkBytes": packet.manifest.Build.MaximumWorkBytes, "candidateWorkBytesUsed": packet.manifest.Build.WorkBytes,
			"maximumCandidateChildren":           packet.manifest.Build.MaximumChildren,
			"maximumInstallScratchRetainedBytes": localAICandidateInstallScratchMaximumBytes, "installScratchRetainedBytes": scratchBytes,
			"installerTimeoutSeconds": 300, "discoveryCommandTimeoutSeconds": 120, "commandOutputMaximumBytes": 1048576,
			"maximumDiscoveryCommands": 10, "maximumDistributionRequests": 4,
			"maximumModelCalls": 0, "maximumModelBackendDownloadBytes": 0, "maximumBackendProcessStarts": 0,
			"allowedNetwork": "IPv4 loopback only for installer distribution requests", "forbiddenPort": 7437,
		},
		"install": json.RawMessage(installResult),
		"cleanup": map[string]any{"workDirectoryRemoved": true, "scratchDirectoryRemoved": true},
	}
	contents, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode install report: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create install report: %w", err)
	}
	if _, err := file.Write(contents); err != nil {
		file.Close()
		os.Remove(path)
		return fmt.Errorf("write install report: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return fmt.Errorf("close install report: %w", err)
	}
	digest := sha256.Sum256(contents)
	sidecar := []byte(fmt.Sprintf("%x  %s\n", digest, filepath.Base(path)))
	sidecarFile, err := os.OpenFile(sidecarPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		os.Remove(path)
		return fmt.Errorf("create install report sidecar: %w", err)
	}
	if _, err := sidecarFile.Write(sidecar); err != nil {
		sidecarFile.Close()
		os.Remove(path)
		os.Remove(sidecarPath)
		return fmt.Errorf("write install report sidecar: %w", err)
	}
	if err := sidecarFile.Close(); err != nil {
		os.Remove(path)
		os.Remove(sidecarPath)
		return fmt.Errorf("close install report sidecar: %w", err)
	}
	return nil
}
