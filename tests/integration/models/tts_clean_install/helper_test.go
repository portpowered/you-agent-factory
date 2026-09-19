package tts_clean_install

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"testing"
)

// TestMain also gives the integration lane a small compiled child artifact.
// The CI/build lane compiles this package once with `go test -c`; the normal
// test process invokes that immutable artifact through this mode branch for
// the OS process, pipe, listener, and filesystem boundary cases.
func TestMain(t *testing.M) {
	if mode := os.Getenv(deterministicHelperModeEnvironment); mode != "" {
		os.Exit(runCompiledHelper(mode))
	}
	os.Exit(t.Run())
}

func TestTTSCleanInstallCompiledHelperTarget(*testing.T) {}

func runCompiledHelper(mode string) int {
	if err := waitForHelperStartGate(); err != nil {
		return 2
	}
	switch mode {
	case "burst":
		return runCompiledBurst()
	case "tree":
		return runCompiledTree()
	case "descendant":
		return runCompiledDescendant()
	case "listener":
		return runCompiledListener("listener-ready")
	case "sentinel":
		return runCompiledSentinel()
	case "journey":
		return runCompiledJourney()
	default:
		return 2
	}
}

func waitForHelperStartGate() error {
	var gate [1]byte
	_, err := io.ReadFull(os.Stdin, gate[:])
	return err
}

func runCompiledBurst() int {
	count, err := strconv.Atoi(os.Getenv(deterministicHelperOutputBytesEnvironment))
	if err != nil || count < 131072 {
		return 2
	}
	payload := expectedBurstBytes(count)
	if _, err := os.Stdout.Write(payload); err != nil {
		return 2
	}
	if _, err := os.Stderr.Write(payload); err != nil {
		return 2
	}
	return 0
}

func runCompiledTree() int {
	readyPath := os.Getenv(deterministicHelperDescendantEnvironment)
	if readyPath == "" {
		return 2
	}
	child := exec.Command(os.Args[0])
	child.Env = setEnvironmentValue(os.Environ(), deterministicHelperModeEnvironment, "descendant")
	child.Stdin = bytes.NewReader([]byte{0})
	child.Stderr = os.Stderr
	childOutput, err := child.StdoutPipe()
	if err != nil {
		return 2
	}
	if err := child.Start(); err != nil {
		return 2
	}
	ready := make(chan struct{}, 1)
	go forwardCompiledChildOutput(childOutput, ready)
	waitCh := make(chan error, 1)
	go func() { waitCh <- child.Wait() }()
	select {
	case <-ready:
		if err := writeHelperEvent(helperEvent{Event: "ready", PID: os.Getpid(), DescendantPID: child.Process.Pid}); err != nil {
			return 2
		}
		waitForHelperTermination()
		<-waitCh
		return 0
	case <-waitCh:
		return 2
	}
}

func forwardCompiledChildOutput(output io.Reader, ready chan<- struct{}) {
	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		_, _ = os.Stdout.Write(append(line, '\n'))
		var event helperEvent
		if json.Unmarshal(line, &event) == nil && event.Event == "descendant-ready" {
			select {
			case ready <- struct{}{}:
			default:
			}
		}
	}
}

func runCompiledDescendant() int {
	readyPath := os.Getenv(deterministicHelperDescendantEnvironment)
	if readyPath == "" {
		return 2
	}
	if err := writeHelperText(readyPath, strconv.Itoa(os.Getpid())); err != nil {
		return 2
	}
	if err := writeHelperEvent(helperEvent{Event: "descendant-ready", PID: os.Getpid()}); err != nil {
		return 2
	}
	waitForHelperTermination()
	return 0
}

func runCompiledListener(eventName string) int {
	readyPath := os.Getenv(deterministicHelperReadyEnvironment)
	if readyPath == "" {
		return 2
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return 2
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	if port == forbiddenListenerPort {
		return 2
	}
	if err := writeHelperText(readyPath, strconv.Itoa(port)); err != nil {
		return 2
	}
	if err := writeHelperEvent(helperEvent{Event: eventName, PID: os.Getpid(), Address: "127.0.0.1", Port: port}); err != nil {
		return 2
	}
	waitForHelperTermination()
	return 0
}

func runCompiledSentinel() int {
	readyPath := os.Getenv(deterministicHelperReadyEnvironment)
	if readyPath == "" {
		return 2
	}
	if err := writeHelperText(readyPath, strconv.Itoa(os.Getpid())); err != nil {
		return 2
	}
	if err := writeHelperEvent(helperEvent{Event: "sentinel-ready", PID: os.Getpid()}); err != nil {
		return 2
	}
	waitForHelperTermination()
	return 0
}

func runCompiledJourney() int {
	outputRoot := os.Getenv(deterministicHelperOutputEnvironment)
	artifactPath := os.Getenv(deterministicHelperArtifactEnvironment)
	if outputRoot == "" || artifactPath == "" {
		return 2
	}
	if err := createControlledHelperRoots(outputRoot); err != nil {
		return 2
	}
	artifact, err := os.ReadFile(artifactPath)
	if err != nil {
		return 2
	}
	installedPath := filepath.Join(outputRoot, "work", "installed-build.bin")
	if err := os.WriteFile(installedPath, artifact, 0o600); err != nil {
		return 2
	}
	audioRoot := filepath.Join(outputRoot, "runtime", "audio")
	if err := os.MkdirAll(audioRoot, 0o700); err != nil {
		return 2
	}
	offline := os.Getenv(deterministicHelperOfflineEnvironment) == "1"
	failPhase := os.Getenv(deterministicHelperFailPhaseEnvironment)
	for _, phase := range journeyStepOrder {
		if failPhase == phase {
			_ = writeHelperEvent(helperEvent{Event: "phase", Phase: phase, Status: journeyFail, Evidence: []string{"controlled phase failure"}, Argv: helperPublicArguments(phase)})
			return 17
		}
		event := helperEvent{Event: "phase", Phase: phase, Status: journeyPass, Evidence: []string{phase + " observed"}, Argv: helperPublicArguments(phase)}
		switch phase {
		case JourneyStepColdInvoke:
			if err := os.WriteFile(filepath.Join(audioRoot, "cold-tts.wav"), makePCM16WAV([]int16{1, 2, 3, 4, 5, 6, 7, 8}, 8000), 0o600); err != nil {
				return 2
			}
		case JourneyStepColdReadiness:
			event.ReadinessState, event.LifecycleState = "READY", "LOADED"
		case JourneyStepWarmOfflineInvoke:
			if !offline {
				return 2
			}
			if err := os.WriteFile(filepath.Join(outputRoot, "cache", "controlled-cache.marker"), []byte("controlled cache reuse marker"), 0o600); err != nil {
				return 2
			}
			if err := os.WriteFile(filepath.Join(audioRoot, "warm-offline-tts.wav"), makePCM16WAV([]int16{8, 7, 6, 5, 4, 3, 2, 1}, 8000), 0o600); err != nil {
				return 2
			}
		case JourneyStepWarmOfflineReadiness:
			marker, err := os.Stat(filepath.Join(outputRoot, "cache", "controlled-cache.marker"))
			if err != nil {
				return 2
			}
			event.ReadinessState, event.LifecycleState = "READY", "LOADED"
			event.CacheBytes, event.CacheReused = marker.Size(), true
		case JourneyStepRemove:
			if err := os.Remove(installedPath); err != nil {
				return 2
			}
			if err := os.Remove(filepath.Join(outputRoot, "cache", "controlled-cache.marker")); err != nil {
				return 2
			}
		case JourneyStepPostRemove:
			if _, err := os.Stat(installedPath); !errors.Is(err, os.ErrNotExist) {
				return 2
			}
			if _, err := os.Stat(filepath.Join(outputRoot, "cache", "controlled-cache.marker")); !errors.Is(err, os.ErrNotExist) {
				return 2
			}
		}
		if err := writeHelperEvent(event); err != nil {
			return 2
		}
	}
	return 0
}

func createControlledHelperRoots(root string) error {
	for _, name := range []string{"work", "profile", "state", "cache", "temp", "streams", "runtime"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o700); err != nil {
			return err
		}
	}
	return nil
}

func helperPublicArguments(phase string) []string {
	switch phase {
	case JourneyStepInstall:
		return []string{"you", "models", "install"}
	case JourneyStepInstalledIdentity:
		return []string{"you", "version"}
	case JourneyStepHelp:
		return []string{"you", "--help"}
	case JourneyStepDocs:
		return []string{"you", "docs", "models"}
	case JourneyStepModelsList:
		return []string{"you", "models", "list"}
	case JourneyStepModelsInspect:
		return []string{"you", "models", "inspect"}
	case JourneyStepColdInvoke:
		return []string{"you", "models", "invoke", "--text", "<redacted-text>"}
	case JourneyStepColdReadiness:
		return []string{"you", "models", "status"}
	case JourneyStepWarmOfflineInvoke:
		return []string{"you", "models", "invoke", "--offline", "--text", "<redacted-text>"}
	case JourneyStepWarmOfflineReadiness:
		return []string{"you", "models", "status"}
	case JourneyStepRemove:
		return []string{"you", "models", "remove"}
	case JourneyStepPostRemove:
		return []string{"you", "models", "inspect"}
	case JourneyStepCleanup:
		return []string{"probe", "cleanup-owned-roots"}
	default:
		return []string{"you", "models", phase}
	}
}

func writeHelperEvent(event helperEvent) error {
	return json.NewEncoder(os.Stdout).Encode(event)
}

func writeHelperText(path, value string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(value), 0o600)
}

func waitForHelperTermination() {
	// signal.Notify keeps an observable OS signal receiver alive. A bare
	// select-with-no-cases is treated as a runtime deadlock and exits before the
	// parent can exercise timeout or cancellation cleanup.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	defer signal.Stop(signals)
	<-signals
}
