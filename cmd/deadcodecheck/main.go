package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

const (
	baselinePath = "docs/internal/development/deadcode-baseline.txt"
	currentPath  = "bin/deadcode-current.txt"
	deadcodeTool = "golang.org/x/tools/cmd/deadcode@v0.25.1"
)

var (
	commandMain                  = run
	runDeadcodeCommand           = runDeadcode
	execCommand                  = exec.Command
	exitFunc                     = os.Exit
	stdout             io.Writer = os.Stdout
	stderr             io.Writer = os.Stderr
)

func main() {
	exitFunc(commandMain(os.Args[1:], stdout, stderr))
}

func run(_ []string, stdout io.Writer, stderr io.Writer) int {
	actual, err := runDeadcodeCommand()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	actual = normalizeReport(actual)
	if err := os.MkdirAll("bin", 0o755); err != nil {
		fmt.Fprintf(stderr, "create deadcode output directory: %v\n", err)
		return 1
	}
	if err := os.WriteFile(currentPath, []byte(actual), 0o644); err != nil {
		fmt.Fprintf(stderr, "write current deadcode report: %v\n", err)
		return 1
	}

	baselineBytes, err := os.ReadFile(baselinePath)
	if err != nil {
		fmt.Fprintf(stderr, "read deadcode baseline: %v\n", err)
		return 1
	}
	baseline := normalizeReport(string(baselineBytes))
	if baseline != actual {
		fmt.Fprintf(stderr, "deadcode baseline drift detected; review %s and update %s when intentional\n", currentPath, baselinePath)
		fmt.Fprintf(stderr, "baseline findings: %d, current findings: %d\n", countFindings(baseline), countFindings(actual))
		return 1
	}

	fmt.Fprintln(stdout, "[agent-factory:deadcode] baseline matches")
	return 0
}

func runDeadcode() (string, error) {
	cmd := execCommand("go", "run", deadcodeTool, "-test", "./...")
	cmd.Env = deadcodeEnv()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("run deadcode: %w\n%s", err, stderr.String())
	}
	if stderr.Len() > 0 {
		_, _ = os.Stderr.Write(stderr.Bytes())
	}
	return stdout.String(), nil
}

func deadcodeEnv() []string {
	env := os.Environ()
	for i, entry := range env {
		name, value, ok := strings.Cut(entry, "=")
		if ok && name == "GODEBUG" {
			env[i] = "GODEBUG=" + ensureGoTypesAliasEnabled(value)
			return env
		}
	}
	return append(env, "GODEBUG=gotypesalias=1")
}

func ensureGoTypesAliasEnabled(value string) string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts)+1)
	found := false
	for _, part := range parts {
		if part == "" {
			continue
		}
		name, _, ok := strings.Cut(part, "=")
		if ok && name == "gotypesalias" {
			out = append(out, "gotypesalias=1")
			found = true
			continue
		}
		out = append(out, part)
	}
	if !found {
		out = append(out, "gotypesalias=1")
	}
	return strings.Join(out, ",")
}

func normalizeReport(report string) string {
	report = strings.ReplaceAll(report, "\r\n", "\n")
	report = strings.ReplaceAll(report, "\r", "\n")
	report = strings.ReplaceAll(report, "\\", "/")
	if report != "" && !strings.HasSuffix(report, "\n") {
		report += "\n"
	}
	return report
}

func countFindings(report string) int {
	report = strings.TrimSpace(report)
	if report == "" {
		return 0
	}
	return len(strings.Split(report, "\n"))
}
