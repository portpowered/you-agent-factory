package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"

	"golang.org/x/mod/modfile"
)

const (
	baselinePath     = "docs/internal/baselines/deadcode-baseline.txt"
	currentPath      = "bin/deadcode-current.txt"
	deadcodeTool     = "golang.org/x/tools/cmd/deadcode@v0.25.1"
	hostPathFile     = ".artifacts/golangci/host-path.txt"
	repositoryModule = "github.com/portpowered/infinite-you"
	// vendoredSDKDir is the exact repository-relative directory holding the
	// preserved upstream ACP SDK sources. The root module compiles that package
	// tree, so deadcode reports upstream helpers this repository does not
	// author; those findings stay outside the authored report.
	vendoredSDKDir = "third_party/acp-go-sdk/"
)

var (
	commandMain                  = run
	runDeadcodeCommand           = runDeadcode
	execCommand                  = exec.Command
	exitFunc                     = os.Exit
	stdout             io.Writer = os.Stdout
	stderr             io.Writer = os.Stderr
	positionPattern              = regexp.MustCompile(`:(\d+):(\d+):`)
)

func main() {
	exitFunc(commandMain(os.Args[1:], stdout, stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("deadcodecheck", flag.ContinueOnError)
	flags.SetOutput(stderr)
	hostFile := flags.String("golangci-host-file", hostPathFile, "retained custom golangci host path")
	if err := flags.Parse(args); err != nil {
		return 1
	}
	actual, err := runDeadcodeCommand(*hostFile)
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
		currentFindings := countFindings(actual)
		fmt.Fprintf(stderr, "baseline findings: %d, current findings: %d\n", countFindings(baseline), currentFindings)
		fmt.Fprintf(stderr, "LINT_VIOLATION_COUNT: %d\n", currentFindings)
		return 1
	}

	fmt.Fprintln(stdout, "[agent-factory:deadcode] baseline matches")
	return 0
}

func runDeadcode(hostFile string) (string, error) {
	root, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	hostBytes, err := os.ReadFile(hostFile)
	if err != nil {
		return "", fmt.Errorf("read generated golangci host (run make golangci-build): %w", err)
	}
	host := strings.TrimSpace(string(hostBytes))
	if !filepath.IsAbs(host) {
		return "", fmt.Errorf("generated golangci host path must be absolute: %q", host)
	}
	if err := validateHostModule(host, root); err != nil {
		return "", err
	}
	// Each delivered program has its own module dependency graph. Analyze both
	// with real main/init roots, then keep findings dead in every program that
	// compiles the source. Tests never contribute reachability.
	repositoryReport, err := productionDeadcode(root, "./...")
	if err != nil {
		return "", err
	}
	hostReport, err := productionDeadcode(host, "./cmd/golangci-lint")
	if err != nil {
		return "", err
	}
	hostReport, err = repositoryPositions(hostReport, host, root)
	if err != nil {
		return "", err
	}
	packages, err := hostRepositoryPackages(host, root)
	if err != nil {
		return "", err
	}
	return reconcileProductionReports(repositoryReport, hostReport, packages), nil
}

func productionDeadcode(directory, pattern string) (string, error) {
	cmd := execCommand("go", "run", deadcodeTool,
		"-filter=^"+regexp.QuoteMeta(repositoryModule)+"(/|$)", pattern)
	cmd.Dir = directory
	cmd.Env = productionEnv()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("run deadcode in %s: %w\n%s", directory, err, stderr.String())
	}
	if stderr.Len() > 0 {
		_, _ = os.Stderr.Write(stderr.Bytes())
	}
	return stdout.String(), nil
}

func productionEnv() []string {
	// go run tool@version ignores the caller's module toolchain directive.
	// Keep its go/types at least as new as the repository compiler.
	return append(deadcodeEnv(), "GOWORK=off", "GOTOOLCHAIN="+runtime.Version()+"+auto")
}

func hostRepositoryPackages(host, root string) (map[string]bool, error) {
	// Compiler-provided ownership, not a directory exemption or source walker.
	format := `{{if .Module}}{{if eq .Module.Path "` + repositoryModule + `"}}{{.Dir}}{{end}}{{end}}`
	cmd := execCommand("go", "list", "-deps", "-f", format, "./cmd/golangci-lint")
	cmd.Dir = host
	cmd.Env = productionEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("load generated host compiler ownership: %w\n%s", err, stderr.String())
	}
	packages := map[string]bool{}
	for _, directory := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if directory == "" {
			continue
		}
		relative, err := filepath.Rel(root, strings.TrimSpace(directory))
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("host package outside repository: %q", directory)
		}
		packages[filepath.ToSlash(relative)] = true
	}
	if !packages["tools/golangcilintplugin"] || !packages["internal/lint/analyzers"] {
		return nil, fmt.Errorf("generated host does not compile the repository plugin and analyzers")
	}
	return packages, nil
}

func reconcileProductionReports(repositoryReport, hostReport string, hostPackages map[string]bool) string {
	hostDead := map[string]bool{}
	for _, finding := range strings.Split(strings.TrimSpace(normalizeReport(hostReport)), "\n") {
		hostDead[finding] = true
	}
	var result strings.Builder
	for _, finding := range strings.Split(strings.TrimSpace(normalizeReport(repositoryReport)), "\n") {
		if finding == "" {
			continue
		}
		source, _, _ := strings.Cut(finding, ".go:")
		if !hostPackages[path.Dir(source+".go")] || hostDead[finding] {
			result.WriteString(finding + "\n")
		}
	}
	return result.String()
}

func validateHostModule(host, root string) error {
	data, err := os.ReadFile(filepath.Join(host, "go.mod"))
	if err != nil {
		return fmt.Errorf("read generated golangci host module: %w", err)
	}
	module, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return fmt.Errorf("parse generated golangci host module: %w", err)
	}
	if module.Module == nil || module.Module.Mod.Path != "github.com/golangci/golangci-lint/v2" {
		return fmt.Errorf("generated host is not the golangci module")
	}
	for _, replacement := range module.Replace {
		if replacement.Old.Path != repositoryModule || replacement.New.Version != "" {
			continue
		}
		source := replacement.New.Path
		if !filepath.IsAbs(source) {
			source = filepath.Join(host, source)
		}
		sourceInfo, sourceErr := os.Stat(source)
		rootInfo, rootErr := os.Stat(root)
		if sourceErr == nil && rootErr == nil && os.SameFile(sourceInfo, rootInfo) {
			return nil
		}
	}
	return fmt.Errorf("generated golangci host must replace %s with this checkout", repositoryModule)
}

// Keep baseline identity relative to the authored checkout, regardless of the
// generated host's temporary location. No package or function is excluded.
func repositoryPositions(report, host, root string) (string, error) {
	var result strings.Builder
	for _, line := range strings.Split(strings.TrimSpace(report), "\n") {
		if line == "" {
			continue
		}
		source, diagnostic, ok := strings.Cut(line, ".go:")
		if !ok {
			return "", fmt.Errorf("malformed deadcode finding: %q", line)
		}
		source += ".go"
		if !filepath.IsAbs(source) {
			source = filepath.Join(host, source)
		}
		relative, err := filepath.Rel(root, source)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("deadcode source outside repository: %q", source)
		}
		result.WriteString(filepath.ToSlash(relative) + ":" + diagnostic + "\n")
	}
	return result.String(), nil
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
	report = strings.TrimSpace(report)
	if report == "" {
		return ""
	}

	lines := strings.Split(report, "\n")
	portable := lines[:0]
	for _, line := range lines {
		line = positionPattern.ReplaceAllString(strings.TrimSpace(line), ":")
		if !platformSpecificFinding(line) && !vendoredSDKFinding(line) {
			portable = append(portable, line)
		}
	}
	slices.Sort(portable)
	if len(portable) == 0 {
		return ""
	}
	return strings.Join(portable, "\n") + "\n"
}

func platformSpecificFinding(line string) bool {
	source, _, ok := strings.Cut(line, ".go:")
	if !ok {
		return false
	}
	base := path.Base(source)
	for _, suffix := range []string{
		"windows", "linux", "darwin", "freebsd", "netbsd", "openbsd", "dragonfly",
		"solaris", "aix", "illumos", "android", "ios", "plan9", "js", "wasip1", "unix",
		"amd64", "386", "arm", "arm64", "riscv64", "ppc64", "ppc64le", "s390x",
		"loong64", "mips", "mipsle", "mips64", "mips64le", "wasm",
	} {
		if strings.HasSuffix(base, "_"+suffix) {
			return true
		}
	}
	return false
}

// Exclude only the preserved upstream SDK's repository-relative source path.
// The trailing separator preserves sibling and nested authored directories.
func vendoredSDKFinding(line string) bool {
	source, _, ok := strings.Cut(line, ".go:")
	if !ok {
		return false
	}
	return strings.HasPrefix(source, vendoredSDKDir)
}

func countFindings(report string) int {
	report = strings.TrimSpace(report)
	if report == "" {
		return 0
	}
	return len(strings.Split(report, "\n"))
}
