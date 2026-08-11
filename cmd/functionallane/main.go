package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type config struct {
	count        int
	jobs         int
	root         string
	short        bool
	timeout      time.Duration
	timingOutput string
}

var executeFunctionalLane = run
var execCommand = exec.Command
var stdoutWriter io.Writer = os.Stdout
var stderrWriter io.Writer = os.Stderr
var exitFunc = os.Exit

const functionalTimingReportVersion = 1

type goTestEvent struct {
	Action  string  `json:"Action"`
	Elapsed float64 `json:"Elapsed"`
	Output  string  `json:"Output"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
}

type functionalTimingEntry struct {
	Name    string  `json:"name"`
	Package string  `json:"package,omitempty"`
	Seconds float64 `json:"seconds"`
}

type functionalTimingReport struct {
	Version     int                     `json:"version"`
	WallSeconds float64                 `json:"wallSeconds"`
	Packages    []functionalTimingEntry `json:"packages"`
	Tests       []functionalTimingEntry `json:"tests"`
}

func main() {
	if err := executeFunctionalLane(); err != nil {
		failf("%v\n", err)
	}
}

func run() error {
	cfg := parseConfig()
	if err := runFunctionalTests(cfg); err != nil {
		return fmt.Errorf("run functional lane: %w", err)
	}

	return nil
}

func parseConfig() config {
	var cfg config
	flag.IntVar(&cfg.count, "count", 0, "go test -count value; zero preserves Go's content-addressed test cache")
	flag.IntVar(&cfg.jobs, "jobs", 8, "go test -p value")
	flag.StringVar(&cfg.root, "root", "./tests/functional/...", "go test package pattern for the functional lane")
	flag.BoolVar(&cfg.short, "short", true, "run with go test -short")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Minute, "go test timeout")
	flag.StringVar(&cfg.timingOutput, "timing-output", "", "optional path for ranked package and test timing JSON")
	flag.Parse()
	if cfg.jobs < 1 {
		cfg.jobs = 1
	}
	return cfg
}

func runFunctionalTests(cfg config) error {
	if cfg.count < 0 {
		return fmt.Errorf("invalid -count=%d: value must be zero or greater", cfg.count)
	}

	args := []string{"test", fmt.Sprintf("-p=%d", cfg.jobs)}
	if strings.TrimSpace(cfg.timingOutput) != "" {
		args = append(args, "-json")
	}
	if cfg.short {
		args = append(args, "-short")
	}
	args = append(args, cfg.root)
	if cfg.count > 0 {
		args = append(args, fmt.Sprintf("-count=%d", cfg.count))
	}
	args = append(args, fmt.Sprintf("-timeout=%s", cfg.timeout))

	cmd := execCommand("go", args...)
	cmd.Env = os.Environ()
	cmd.Stderr = os.Stderr
	if strings.TrimSpace(cfg.timingOutput) == "" {
		cmd.Stdout = os.Stdout
		return cmd.Run()
	}
	return runFunctionalTestsWithTiming(cmd, cfg.timingOutput)
}

func runFunctionalTestsWithTiming(cmd *exec.Cmd, outputPath string) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("capture go test JSON output: %w", err)
	}
	started := time.Now()
	if err := cmd.Start(); err != nil {
		return err
	}
	report, scanErr := collectFunctionalTimingReport(stdout)
	runErr := cmd.Wait()
	report.WallSeconds = roundedSeconds(time.Since(started).Seconds())
	if scanErr != nil {
		return scanErr
	}
	if err := writeFunctionalTimingReport(outputPath, report); err != nil {
		return err
	}
	return runErr
}

func collectFunctionalTimingReport(reader io.Reader) (functionalTimingReport, error) {
	report := functionalTimingReport{Version: functionalTimingReportVersion}
	packageTimes := make(map[string]float64)
	testTimes := make(map[string]functionalTimingEntry)
	packageOutput := make(map[string]*strings.Builder)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var event goTestEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return functionalTimingReport{}, fmt.Errorf("decode go test JSON event: %w", err)
		}
		if event.Output != "" {
			buffer := packageOutput[event.Package]
			if buffer == nil {
				buffer = &strings.Builder{}
				packageOutput[event.Package] = buffer
			}
			buffer.WriteString(event.Output)
			if event.Test == "" && isConciseGoTestOutput(event.Output) {
				fmt.Fprint(stdoutWriter, event.Output)
			}
		}
		if event.Action == "fail" && event.Test == "" {
			if output := packageOutput[event.Package]; output != nil {
				fmt.Fprint(stdoutWriter, output.String())
			}
			delete(packageOutput, event.Package)
		}
		if event.Action != "pass" || event.Elapsed < 0 {
			continue
		}
		if event.Test == "" {
			packageTimes[event.Package] = event.Elapsed
			delete(packageOutput, event.Package)
			continue
		}
		if strings.Contains(event.Test, "/") {
			continue
		}
		key := event.Package + "\x00" + event.Test
		testTimes[key] = functionalTimingEntry{
			Name:    event.Test,
			Package: event.Package,
			Seconds: event.Elapsed,
		}
	}
	if err := scanner.Err(); err != nil {
		return functionalTimingReport{}, fmt.Errorf("read go test JSON output: %w", err)
	}
	for packageName, seconds := range packageTimes {
		report.Packages = append(report.Packages, functionalTimingEntry{Name: packageName, Seconds: seconds})
	}
	for _, entry := range testTimes {
		report.Tests = append(report.Tests, entry)
	}
	sortFunctionalTimingEntries(report.Packages)
	sortFunctionalTimingEntries(report.Tests)
	return report, nil
}

func isConciseGoTestOutput(output string) bool {
	trimmed := strings.TrimSpace(output)
	return strings.HasPrefix(trimmed, "ok  \t") ||
		strings.HasPrefix(trimmed, "?   \t")
}

func sortFunctionalTimingEntries(entries []functionalTimingEntry) {
	slices.SortFunc(entries, func(left, right functionalTimingEntry) int {
		if left.Seconds > right.Seconds {
			return -1
		}
		if left.Seconds < right.Seconds {
			return 1
		}
		if result := strings.Compare(left.Package, right.Package); result != 0 {
			return result
		}
		return strings.Compare(left.Name, right.Name)
	})
}

func writeFunctionalTimingReport(outputPath string, report functionalTimingReport) error {
	if directory := filepath.Dir(outputPath); directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("create functional timing output directory: %w", err)
		}
	}
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create functional timing report: %w", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	writeErr := encoder.Encode(report)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write functional timing report: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close functional timing report: %w", closeErr)
	}
	return nil
}

func roundedSeconds(seconds float64) float64 {
	return float64(int64(seconds*100+0.5)) / 100
}

func failf(format string, args ...any) {
	fmt.Fprintf(stderrWriter, format, args...)
	exitFunc(1)
}
