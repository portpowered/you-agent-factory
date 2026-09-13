package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

const (
	naturalNonzeroOutputBytes = 70 << 10
	readinessTimeout          = 15 * time.Second
	readinessPoll             = 10 * time.Millisecond
)

func main() {
	if len(os.Args) < 2 {
		fail("missing helper case")
	}
	switch os.Args[1] {
	case "--case":
		runCase(requiredArgument(2, "case"))
	case "--descendant":
		runDescendant(requiredOption("ready-file"))
	default:
		fail("unknown helper mode %q", os.Args[1])
	}
}

func runCase(name string) {
	switch name {
	case "natural-nonzero":
		if err := writeRepeated(os.Stdout, 'o', naturalNonzeroOutputBytes); err != nil {
			fail("write stdout: %v", err)
		}
		if err := writeRepeated(os.Stderr, 'e', naturalNonzeroOutputBytes); err != nil {
			fail("write stderr: %v", err)
		}
		os.Exit(7)
	case "forced-stop":
		runForcedStop(requiredOption("pid-file"), requiredOption("ready-file"))
	default:
		fail("unknown helper case %q", name)
	}
}

func runForcedStop(pidPath, readyPath string) {
	if err := writeAll(os.Stdout, []byte("managed-process forced-stop stdout\n")); err != nil {
		fail("write forced-stop stdout: %v", err)
	}
	if err := writeAll(os.Stderr, []byte("managed-process forced-stop stderr\n")); err != nil {
		fail("write forced-stop stderr: %v", err)
	}
	child := exec.Command(os.Args[0], "--descendant", "--ready-file", readyPath)
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if err := child.Start(); err != nil {
		fail("start descendant: %v", err)
	}
	if !waitForFile(readyPath) {
		_ = child.Process.Kill()
		fail("descendant did not publish readiness")
	}
	if err := writePIDFile(pidPath, child.Process.Pid); err != nil {
		_ = child.Process.Kill()
		fail("write descendant PID: %v", err)
	}
	blockUntilSignal()
}

func runDescendant(readyPath string) {
	if err := os.WriteFile(readyPath, []byte("ready\n"), 0o600); err != nil {
		fail("write descendant readiness: %v", err)
	}
	blockUntilSignal()
}

func writeRepeated(writer io.Writer, value byte, total int) error {
	chunk := bytes.Repeat([]byte{value}, 8<<10)
	for remaining := total; remaining > 0; {
		count := len(chunk)
		if count > remaining {
			count = remaining
		}
		if err := writeAll(writer, chunk[:count]); err != nil {
			return err
		}
		remaining -= count
	}
	return nil
}

func writeAll(writer io.Writer, value []byte) error {
	for len(value) > 0 {
		written, err := writer.Write(value)
		if err != nil {
			return err
		}
		if written <= 0 {
			return io.ErrShortWrite
		}
		value = value[written:]
	}
	return nil
}

func writePIDFile(path string, pid int) error {
	if pid <= 0 {
		return fmt.Errorf("descendant PID %d is not positive", pid)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, []byte(fmt.Sprintf("%d\n", pid)), 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func waitForFile(path string) bool {
	deadline := time.NewTimer(readinessTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(readinessPoll)
	defer ticker.Stop()
	for {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return true
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			return false
		}
	}
}

func blockUntilSignal() {
	// A signal channel keeps the helper alive without the Go runtime treating a
	// bare empty select as a deadlock. The managed process tree supplies the
	// actual terminal signal on every supported host.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	defer signal.Stop(signals)
	<-signals
}

func requiredOption(name string) string {
	for index := 2; index < len(os.Args); index++ {
		if os.Args[index] == "--"+name && index+1 < len(os.Args) {
			value := strings.TrimSpace(os.Args[index+1])
			if value != "" {
				return filepath.Clean(value)
			}
		}
	}
	fail("missing --%s", name)
	return ""
}

func requiredArgument(index int, name string) string {
	if index < len(os.Args) && strings.TrimSpace(os.Args[index]) != "" {
		return strings.TrimSpace(os.Args[index])
	}
	fail("missing %s argument", name)
	return ""
}

func fail(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "managed process helper: "+format+"\n", args...)
	os.Exit(2)
}
