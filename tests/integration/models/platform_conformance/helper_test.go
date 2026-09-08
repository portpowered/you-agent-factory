package platform_conformance

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"testing"
)

// TestMain turns this package's already compiled test executable into the
// controlled child helper when the runner supplies an explicit helper mode.
// Normal package tests never enter this branch, and the helper does not run
// the package test suite recursively.
func TestMain(t *testing.M) {
	if os.Getenv(controlledHelperModeEnv) != "" {
		os.Exit(runControlledHelper())
	}
	os.Exit(t.Run())
}

// TestPlatformConformanceHelper is the named target used in argv so the
// precompiled test binary remains an ordinary Go test artifact. TestMain
// handles the helper before the testing package starts its own tests.
func TestPlatformConformanceHelper(*testing.T) {}

func runControlledHelper() int {
	switch os.Getenv(controlledHelperModeEnv) {
	case controlledHelperModeSuccess:
		writeControlledHelperReady()
		writeControlledHelperSemantic("controlled embedding response")
		return 0
	case controlledHelperModeProduct:
		writeControlledHelperReady()
		_ = writeControlledLine(os.Stdout, controlledHelperEvent{Event: "product-failure", Text: "controlled backend failure"})
		return controlledProductFailureExitCode
	case controlledHelperModeTimeout, controlledHelperModeCancel:
		writeControlledHelperReady()
		waitControlledHelperTermination()
		return 0
	case controlledHelperModePartial:
		partialPath := os.Getenv(controlledHelperPartialPathEnv)
		if partialPath == "" {
			return 2
		}
		if err := os.WriteFile(partialPath, []byte("partial controlled output"), 0o600); err != nil {
			return 2
		}
		writeControlledHelperReady()
		writeControlledHelperSemantic("controlled response with partial artifact")
		return 0
	case controlledHelperModeTree:
		return runControlledTreeHelper()
	case controlledHelperModeListener:
		return runControlledListenerChild()
	case controlledHelperModeSecret:
		secret := os.Getenv(controlledHelperSecretEnv)
		writeControlledHelperReady()
		writeControlledHelperSemantic(secret)
		_, _ = fmt.Fprintln(os.Stderr, secret)
		return 0
	default:
		return 2
	}
}

func runControlledTreeHelper() int {
	command := exec.Command(os.Args[0], "-test.run=^TestPlatformConformanceHelper$")
	command.Env = setEnvironmentValue(os.Environ(), controlledHelperModeEnv, controlledHelperModeListener)
	childOutput, err := command.StdoutPipe()
	if err != nil {
		return 2
	}
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return 2
	}
	listenerReady := make(chan struct{}, 1)
	go forwardControlledChildOutput(childOutput, listenerReady)
	waitCh := make(chan error, 1)
	go func() { waitCh <- command.Wait() }()
	select {
	case <-listenerReady:
		writeControlledHelperReady()
		waitControlledHelperTermination()
		// The process group receives the same interrupt, but waiting here lets
		// the parent reap its owned child before TestMain exits. The outer
		// runner still has a force-kill fallback if a child ignores the signal.
		<-waitCh
	case <-waitCh:
		return 2
	}
	return 0
}

func forwardControlledChildOutput(output io.Reader, ready chan<- struct{}) {
	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		_, _ = os.Stdout.Write(append(line, '\n'))
		var event controlledHelperEvent
		if json.Unmarshal(line, &event) == nil && event.Event == "listener-ready" {
			select {
			case ready <- struct{}{}:
			default:
			}
		}
	}
}

func runControlledListenerChild() int {
	port, err := strconv.Atoi(os.Getenv(controlledHelperPortEnv))
	if err != nil || port <= 0 {
		return 2
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return 2
	}
	defer listener.Close()
	if err := writeControlledLine(os.Stdout, controlledHelperEvent{Event: "listener-ready"}); err != nil {
		return 2
	}
	waitControlledHelperTermination()
	return 0
}

func waitControlledHelperTermination() {
	// signal.Notify keeps an observable OS signal receiver alive. A bare
	// select-with-no-cases is treated as a Go runtime deadlock and exits before
	// the runner can exercise its timeout or cancellation cleanup.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	defer signal.Stop(signals)
	<-signals
}

func writeControlledHelperReady() {
	_ = writeControlledLine(os.Stdout, controlledHelperEvent{Event: "ready"})
}

func writeControlledHelperSemantic(text string) {
	_ = writeControlledLine(os.Stdout, controlledHelperEvent{
		Event: "semantic", Text: text,
		Values: []float64{0.125, -0.25, 0.375, 0.5, -0.625, 0.75, 0.875, -1},
	})
}
