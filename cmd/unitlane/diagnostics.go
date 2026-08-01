package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	diagnosticSchemaVersion = 1
	slowPackageLimit        = 10
)

type unitLaneReport struct {
	SchemaVersion   int                 `json:"schemaVersion"`
	Command         string              `json:"command"`
	PackagePattern  string              `json:"packagePattern"`
	StartedAt       time.Time           `json:"startedAt"`
	FinishedAt      time.Time           `json:"finishedAt"`
	WallTime        string              `json:"wallTime"`
	WallTimeSeconds float64             `json:"wallTimeSeconds"`
	Outcome         string              `json:"outcome"`
	EffectiveJobs   int                 `json:"effectiveJobs"`
	Count           int                 `json:"count"`
	CacheMode       string              `json:"cacheMode"`
	Short           bool                `json:"short"`
	Vet             bool                `json:"vet"`
	Timeout         string              `json:"timeout"`
	Host            unitLaneHost        `json:"host"`
	PackageCount    int                 `json:"packageCount"`
	TestCount       int                 `json:"testCount"`
	Packages        []unitPackageTiming `json:"packages"`
	SlowPackages    []unitPackageTiming `json:"slowPackages"`
}

type unitLaneHost struct {
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	GoVersion  string `json:"goVersion"`
	Hostname   string `json:"hostname,omitempty"`
	CPUs       int    `json:"cpus"`
	GOMAXPROCS int    `json:"gomaxprocs"`
}

type unitBatchReport struct {
	Packages []unitPackageTiming
}

type unitPackageTiming struct {
	Package        string  `json:"package"`
	Elapsed        string  `json:"elapsed"`
	ElapsedSeconds float64 `json:"elapsedSeconds"`
	Outcome        string  `json:"outcome"`
	TestCount      int     `json:"testCount"`
	Cached         bool    `json:"cached"`
	CacheObserved  bool    `json:"cacheObserved"`
	Completed      bool    `json:"completed"`
}

type goTestJSONEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Output  string  `json:"Output"`
	Elapsed float64 `json:"Elapsed"`
	Cached  *bool   `json:"Cached"`
}

type unitTimingCollector struct {
	packages map[string]*unitPackageTiming
}

type unitDiagnosticOutput struct {
	collector *unitTimingCollector
	output    io.Writer
	pending   bytes.Buffer
}

func newUnitLaneReport(cfg config, startedAt time.Time) *unitLaneReport {
	return &unitLaneReport{
		SchemaVersion:  diagnosticSchemaVersion,
		Command:        "go test",
		PackagePattern: cfg.root,
		StartedAt:      startedAt,
		EffectiveJobs:  cfg.jobs,
		Count:          cfg.count,
		CacheMode:      cacheMode(cfg.count),
		Short:          cfg.short,
		Vet:            cfg.vet,
		Timeout:        cfg.timeout.String(),
		Host:           currentUnitLaneHost(),
	}
}

func cacheMode(count int) string {
	switch count {
	case 0:
		return "cached"
	case 1:
		return "fresh"
	default:
		return "counted"
	}
}

func currentUnitLaneHost() unitLaneHost {
	hostname, _ := os.Hostname()
	return unitLaneHost{
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		GoVersion:  runtime.Version(),
		Hostname:   hostname,
		CPUs:       runtime.NumCPU(),
		GOMAXPROCS: runtime.GOMAXPROCS(0),
	}
}

func newUnitTimingCollector() *unitTimingCollector {
	return &unitTimingCollector{packages: make(map[string]*unitPackageTiming)}
}

func (c *unitTimingCollector) accept(event goTestJSONEvent) {
	if event.Package == "" {
		return
	}
	result, ok := c.packages[event.Package]
	if !ok {
		result = &unitPackageTiming{Package: event.Package}
		c.packages[event.Package] = result
	}
	if event.Action == "output" && event.Test == "" && packageOutputIsCached(event.Output) {
		result.Cached = true
		result.CacheObserved = true
	}

	switch event.Action {
	case "run":
		if event.Test != "" {
			result.TestCount++
		}
	case "pass", "fail", "skip":
		if event.Test != "" {
			return
		}
		result.ElapsedSeconds = event.Elapsed
		result.Elapsed = formatElapsed(event.Elapsed)
		result.Outcome = event.Action
		result.Completed = true
		if event.Cached != nil {
			result.Cached = *event.Cached
			result.CacheObserved = true
		}
	}
}

func packageOutputIsCached(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasSuffix(strings.TrimSpace(line), "(cached)") {
			return true
		}
	}
	return false
}

func (c *unitTimingCollector) report() unitBatchReport {
	packages := make([]unitPackageTiming, 0, len(c.packages))
	for _, result := range c.packages {
		packages = append(packages, *result)
	}
	return unitBatchReport{Packages: sortPackageTimings(packages)}
}

func (o *unitDiagnosticOutput) Write(data []byte) (int, error) {
	if _, err := o.pending.Write(data); err != nil {
		return 0, err
	}
	for {
		lineEnd := bytes.IndexByte(o.pending.Bytes(), '\n')
		if lineEnd < 0 {
			return len(data), nil
		}
		line := o.pending.Next(lineEnd + 1)
		if err := o.consume(line); err != nil {
			return 0, err
		}
	}
}

func (o *unitDiagnosticOutput) flush() error {
	if o.pending.Len() == 0 {
		return nil
	}
	return o.consume(o.pending.Next(o.pending.Len()))
}

func (o *unitDiagnosticOutput) consume(line []byte) error {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return nil
	}
	var event goTestJSONEvent
	if err := json.Unmarshal(line, &event); err != nil {
		return fmt.Errorf("decode go test JSON event: %w", err)
	}
	o.collector.accept(event)
	if event.Output == "" {
		return nil
	}
	if _, err := io.WriteString(o.output, event.Output); err != nil {
		return fmt.Errorf("replay go test output: %w", err)
	}
	return nil
}

func (r *unitLaneReport) addPackages(batch unitBatchReport) {
	r.Packages = append(r.Packages, batch.Packages...)
}

func finishUnitLaneReport(cfg config, report *unitLaneReport, startedAt time.Time, runErr error) error {
	report.FinishedAt = time.Now()
	report.WallTimeSeconds = report.FinishedAt.Sub(startedAt).Seconds()
	report.WallTime = formatElapsed(report.WallTimeSeconds)
	report.Outcome = "pass"
	if runErr != nil {
		report.Outcome = "fail"
	}
	report.Packages = sortPackageTimings(report.Packages)
	report.PackageCount = len(report.Packages)
	for _, packageTiming := range report.Packages {
		report.TestCount += packageTiming.TestCount
	}
	report.SlowPackages = slowPackages(report.Packages)

	var finishErr error
	if err := writeDiagnosticSummary(stderrWriter, cfg, *report); err != nil {
		finishErr = errors.Join(finishErr, err)
	}
	if cfg.reportPath != "" {
		if err := writeUnitLaneReport(cfg.reportPath, *report); err != nil {
			finishErr = errors.Join(finishErr, err)
		}
	}
	return errors.Join(runErr, finishErr)
}

func sortPackageTimings(packages []unitPackageTiming) []unitPackageTiming {
	sorted := append([]unitPackageTiming(nil), packages...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Package < sorted[j].Package
	})
	return sorted
}

func slowPackages(packages []unitPackageTiming) []unitPackageTiming {
	sorted := append([]unitPackageTiming(nil), packages...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].ElapsedSeconds != sorted[j].ElapsedSeconds {
			return sorted[i].ElapsedSeconds > sorted[j].ElapsedSeconds
		}
		return sorted[i].Package < sorted[j].Package
	})
	if len(sorted) > slowPackageLimit {
		sorted = sorted[:slowPackageLimit]
	}
	return sorted
}

func formatElapsed(seconds float64) string {
	if seconds <= 0 {
		return "0s"
	}
	return (time.Duration(seconds * float64(time.Second))).String()
}

func writeDiagnosticSummary(w io.Writer, cfg config, report unitLaneReport) error {
	if _, err := fmt.Fprintf(w, "unit lane diagnostics: outcome=%s wall=%s packages=%d tests=%d jobs=%d cache=%s\n", report.Outcome, report.WallTime, report.PackageCount, report.TestCount, report.EffectiveJobs, report.CacheMode); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "slow packages:"); err != nil {
		return err
	}
	for _, packageTiming := range report.SlowPackages {
		if _, err := fmt.Fprintf(w, "  %-7s %-9s %s\n", packageTiming.Elapsed, packageTiming.Outcome, packageTiming.Package); err != nil {
			return err
		}
	}
	if cfg.reportPath == "" {
		return nil
	}
	_, err := fmt.Fprintf(w, "unit lane diagnostics report: %s\n", cfg.reportPath)
	return err
}

func writeUnitLaneReport(path string, report unitLaneReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode diagnostic report: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create diagnostic report directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write diagnostic report %q: %w", path, err)
	}
	return nil
}
