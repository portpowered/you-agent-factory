package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

const (
	defaultArtifactName = "backend-verification-failure-artifacts"
	defaultCommand      = "make test-backend-verification"
	defaultPrimaryLog   = ".artifacts/backend-verification/command.log"
	excerptLineLimit    = 80
	excerptByteLimit    = 12000
)

var (
	goTestFailurePattern             = regexp.MustCompile(`^\s*--- FAIL: ([^\s(]+)`)
	coverageFailurePattern           = regexp.MustCompile(`go coverage ([0-9.]+)% is below minimum ([0-9.]+)%`)
	stdoutWriter           io.Writer = os.Stdout
	stderrWriter           io.Writer = os.Stderr
	exitFunc                         = os.Exit
)

type config struct {
	logPath      string
	command      string
	artifactName string
	primaryLog   string
}

type failureSummary struct {
	Kind         string
	CommandPhase string
	Package      string
	TestName     string
	Measured     string
	Required     string
	Excerpt      []string
	Inferred     bool
}

func main() {
	cfg := parseConfig()
	if err := run(cfg, stdoutWriter); err != nil {
		fmt.Fprintln(stderrWriter, err)
		exitFunc(1)
	}
}

func parseConfig() config {
	cfg := config{
		command:      defaultCommand,
		artifactName: defaultArtifactName,
		primaryLog:   defaultPrimaryLog,
	}
	flag.StringVar(&cfg.logPath, "log", "", "path to the backend verification command log")
	flag.StringVar(&cfg.command, "command", defaultCommand, "canonical local rerun command")
	flag.StringVar(&cfg.artifactName, "artifact-name", defaultArtifactName, "retained failure artifact name")
	flag.StringVar(&cfg.primaryLog, "primary-log", defaultPrimaryLog, "primary command log path to display")
	flag.Parse()
	return cfg
}

func run(cfg config, output io.Writer) error {
	if strings.TrimSpace(cfg.logPath) == "" {
		return fmt.Errorf("-log is required")
	}
	rawLog, err := os.ReadFile(cfg.logPath)
	if err != nil {
		return fmt.Errorf("read backend verification log %s: %w", cfg.logPath, err)
	}

	summary := summarizeBackendVerificationLog(string(rawLog))
	writeMarkdownSummary(output, cfg, summary)
	return nil
}

func summarizeBackendVerificationLog(rawLog string) failureSummary {
	lines := splitLogLines(rawLog)
	if summary, ok := summarizeFirstGoTestFailure(lines); ok {
		return summary
	}
	if summary, ok := summarizeCoverageGateFailure(lines); ok {
		return summary
	}
	return failureSummary{
		Kind:     "unclassified backend verification failure",
		Excerpt:  boundedExcerpt(lines, 0, len(lines)),
		Inferred: false,
	}
}

func summarizeFirstGoTestFailure(lines []string) (failureSummary, bool) {
	for index, line := range lines {
		matches := goTestFailurePattern.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		testName := matches[1]
		start := findGoTestBlockStart(lines, index, testName)
		end := findGoTestBlockEnd(lines, index)
		return failureSummary{
			Kind:         "go test failure",
			CommandPhase: findCommandPhase(lines),
			Package:      findFailedPackage(lines, index),
			TestName:     testName,
			Excerpt:      boundedExcerpt(lines, start, end),
			Inferred:     true,
		}, true
	}
	return failureSummary{}, false
}

func summarizeCoverageGateFailure(lines []string) (failureSummary, bool) {
	for index, line := range lines {
		matches := coverageFailurePattern.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		return failureSummary{
			Kind:         "coverage gate failure",
			CommandPhase: findCommandPhase(lines),
			Measured:     matches[1] + "%",
			Required:     matches[2] + "%",
			Excerpt:      boundedExcerpt(lines, coverageExcerptStart(lines, index), coverageExcerptEnd(lines, index)),
			Inferred:     true,
		}, true
	}
	return failureSummary{}, false
}

func coverageExcerptStart(lines []string, failureIndex int) int {
	for index := failureIndex - 1; index >= 0; index-- {
		if strings.Contains(lines[index], "total:") || strings.HasPrefix(lines[index], "go tool cover") {
			return index
		}
		if strings.HasPrefix(lines[index], "ok  \t") || strings.HasPrefix(lines[index], "?\t") {
			break
		}
	}
	return failureIndex
}

func coverageExcerptEnd(lines []string, failureIndex int) int {
	for index := failureIndex + 1; index < len(lines); index++ {
		line := lines[index]
		if strings.HasPrefix(line, "make") || strings.Contains(line, "run go test coverage lane:") {
			continue
		}
		return index
	}
	return len(lines)
}

func splitLogLines(rawLog string) []string {
	normalized := strings.ReplaceAll(rawLog, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	trimmed := strings.TrimRight(normalized, "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func findGoTestBlockStart(lines []string, failIndex int, testName string) int {
	rootTestName := strings.Split(testName, "/")[0]
	runPrefix := "=== RUN   " + rootTestName
	for index := failIndex; index >= 0; index-- {
		if strings.HasPrefix(lines[index], runPrefix) {
			return index
		}
		if strings.HasPrefix(lines[index], "FAIL\t") || strings.HasPrefix(lines[index], "ok  \t") {
			break
		}
	}
	return failIndex
}

func findGoTestBlockEnd(lines []string, failIndex int) int {
	for index := failIndex + 1; index < len(lines); index++ {
		if packageName, ok := parseFailedPackageLine(lines[index]); ok && packageName != "" {
			return index + 1
		}
		if strings.HasPrefix(lines[index], "=== RUN   ") && index > failIndex+1 {
			return index
		}
	}
	return len(lines)
}

func findFailedPackage(lines []string, failIndex int) string {
	for index := failIndex + 1; index < len(lines); index++ {
		if packageName, ok := parseFailedPackageLine(lines[index]); ok {
			return packageName
		}
	}
	return ""
}

func parseFailedPackageLine(line string) (string, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] != "FAIL" {
		return "", false
	}
	return fields[1], true
}

func findCommandPhase(lines []string) string {
	for _, line := range lines {
		if strings.Contains(line, "run go test coverage lane:") {
			return "run go test coverage lane"
		}
	}
	return ""
}

func boundedExcerpt(lines []string, start int, end int) []string {
	if start < 0 {
		start = 0
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start > end {
		start = end
	}

	excerpt := make([]string, 0, min(end-start, excerptLineLimit))
	bytesUsed := 0
	for index := start; index < end && len(excerpt) < excerptLineLimit; index++ {
		line := lines[index]
		nextBytes := len(line) + 1
		if len(excerpt) > 0 && bytesUsed+nextBytes > excerptByteLimit {
			excerpt = append(excerpt, "... excerpt truncated ...")
			return excerpt
		}
		excerpt = append(excerpt, line)
		bytesUsed += nextBytes
	}
	if end-start > len(excerpt) {
		excerpt = append(excerpt, "... excerpt truncated ...")
	}
	return excerpt
}

func writeMarkdownSummary(output io.Writer, cfg config, summary failureSummary) {
	fmt.Fprintln(output, "### Execution result")
	fmt.Fprintln(output)
	fmt.Fprintf(output, "- Verified: Backend coverage and required short functional checks through `%s`.\n", cfg.command)
	fmt.Fprintln(output, "- Result: `failed`")
	fmt.Fprintf(output, "- Failure type: `%s`\n", summary.Kind)
	if summary.CommandPhase != "" {
		fmt.Fprintf(output, "- Command phase: `%s`\n", summary.CommandPhase)
	}
	if summary.Package != "" {
		fmt.Fprintf(output, "- Package: `%s`\n", summary.Package)
	}
	if summary.TestName != "" {
		fmt.Fprintf(output, "- Test: `%s`\n", summary.TestName)
	}
	if summary.Measured != "" {
		fmt.Fprintf(output, "- Measured coverage: `%s`\n", summary.Measured)
	}
	if summary.Required != "" {
		fmt.Fprintf(output, "- Required coverage: `%s`\n", summary.Required)
	}
	if !summary.Inferred {
		fmt.Fprintln(output, "- Failure identity: specific failing package or test could not be inferred from the command log.")
	}
	fmt.Fprintf(output, "- Local rerun: `%s`\n", cfg.command)
	fmt.Fprintf(output, "- Retained artifact: `%s` for 14 days\n", cfg.artifactName)
	fmt.Fprintf(output, "- Primary log: `%s`\n", cfg.primaryLog)
	fmt.Fprintln(output)
	fmt.Fprintln(output, "#### First actionable failure excerpt")
	fmt.Fprintln(output)
	fmt.Fprintln(output, "```text")
	for _, line := range summary.Excerpt {
		fmt.Fprintln(output, line)
	}
	fmt.Fprintln(output, "```")
}
