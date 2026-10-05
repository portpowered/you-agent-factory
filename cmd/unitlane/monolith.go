package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type monolithGroup struct {
	Package string   `json:"package"`
	Group   string   `json:"group"`
	Tests   []string `json:"top_level_tests"`
}

// This remains opt-in until full implementation coverage and rebuild profiles
// have been compared. Native Go still validates every build before execution.
func runMonolithUnitTests(cfg config, packages []string) error {
	if cfg.count > 1 {
		return errors.New("experimental monolith supports fresh -count=1 execution only")
	}
	started := time.Now()
	dir, unlock, err := acquireMonolithWorkspace()
	if err != nil {
		return err
	}
	defer unlock()
	dir, err = monolithScopeWorkspace(cfg, dir)
	if err != nil {
		return err
	}
	if cfg.monolithPrepare {
		return prepareMonolithSuite(cfg, packages, dir)
	}
	binary, groups, excluded, err := prepareMonolithBinary(cfg, packages, dir)
	if err != nil {
		return err
	}
	return finishMonolithUnitTests(cfg, packages, binary, groups, excluded, started)
}

func acquireMonolithWorkspace() (string, func(), error) {
	root, err := os.Getwd()
	if err != nil {
		return "", nil, err
	}
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return "", nil, err
	}
	digest := sha256.Sum256([]byte(root))
	dir := filepath.Join(cacheRoot, "you-unit-monolith", fmt.Sprintf("%x", digest[:12]))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", nil, err
	}
	lock, err := os.OpenFile(filepath.Join(dir, "running.lock"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", nil, fmt.Errorf("lock monolith workspace %s (another run may be active): %w", dir, err)
	}
	if err := lock.Close(); err != nil {
		os.Remove(lock.Name())
		return "", nil, err
	}
	return dir, func() { os.Remove(lock.Name()) }, nil
}

func prepareMonolithBinary(cfg config, packages []string, dir string) (string, []monolithGroup, map[string]string, error) {
	metadata := filepath.Join(dir, "packages.json")
	if err := writeMonolithMetadata(cfg, packages, metadata); err != nil {
		return "", nil, nil, err
	}
	python := os.Getenv("PYTHON")
	if python == "" {
		python = "python"
	}
	generate := execCommand(python, filepath.Join("cmd", "unitlane", "monolith_generate.py"), metadata, dir)
	generate.Stdout, generate.Stderr = stderrWriter, stderrWriter
	if err := generate.Run(); err != nil {
		return "", nil, nil, fmt.Errorf("generate monolith overlay: %w", err)
	}
	var groups []monolithGroup
	if err := readMonolithJSON(filepath.Join(dir, "groups.json"), &groups); err != nil {
		return "", nil, nil, err
	}
	var excluded map[string]string
	if err := readMonolithJSON(filepath.Join(dir, "excluded.json"), &excluded); err != nil {
		return "", nil, nil, err
	}
	if err := validateMonolithInventory(packages, groups, excluded); err != nil {
		return "", nil, nil, err
	}
	binary := filepath.Join(dir, "unit.test")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	buildArgs := []string{"test", "-c", fmt.Sprintf("-p=%d", cfg.jobs),
		"-overlay=" + filepath.Join(dir, "overlay.json"), "-o=" + binary}
	if !cfg.vet {
		buildArgs = append(buildArgs, "-vet=off")
	}
	buildArgs = append(buildArgs, "./pkg/monolithpilot")
	build := execCommand("go", buildArgs...)
	build.Stdout, build.Stderr = stderrWriter, stderrWriter
	if err := build.Run(); err != nil {
		return "", nil, nil, fmt.Errorf("validate and build monolith: %w", err)
	}
	return binary, groups, excluded, nil
}

func finishMonolithUnitTests(cfg config, packages []string, binary string, groups []monolithGroup, excluded map[string]string, started time.Time) error {
	accumulator := newUnitTimingAccumulator(packages)
	capture, mergedErr := executeMonolithBinary(cfg, binary, groups)
	accumulator.add(capture)
	native := make([]string, 0, len(excluded))
	for pkg := range excluded {
		native = append(native, pkg)
	}
	slices.Sort(native)
	for _, pkg := range native {
		fmt.Fprintf(stderrWriter, "native unit binary: %s (%s)\n", pkg, excluded[pkg])
	}
	// Run exceptions even after a shared-binary failure, so diagnostics include
	// all selected packages and neither lane's failure can disappear.
	var nativeErr error
	if len(native) > 0 {
		fresh := cfg
		fresh.count = 1
		capture, nativeErr = runGoTest(fresh, localPackageArguments(native), native)
		accumulator.add(capture)
	}
	run := unitTimingRun{}
	if strings.TrimSpace(cfg.timingOutput) != "" {
		run = unitTimingRunIdentity(cfg)
	}
	summary := accumulator.summaryWithRun(time.Since(started).Seconds(), run)
	if cfg.wiringIntegration {
		summary.Lane = "wiring integration"
	}
	if !cfg.monolithDetails {
		summary.TestInventoryScope = "merged-top-level/native-all"
	}
	return errors.Join(mergedErr, nativeErr, writeUnitTimingSummary(stdoutWriter, summary), writeUnitTimingSummaryJSON(cfg.timingOutput, summary))
}

func writeMonolithMetadata(cfg config, packages []string, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	args := []string{"list", fmt.Sprintf("-p=%d", cfg.jobs), "-json=ImportPath,Dir,Name,TestGoFiles,XTestGoFiles,XTestEmbedPatterns"}
	batch := append([]string(nil), args...)
	for _, pkg := range localPackageArguments(packages) {
		if commandArgLen(batch)+len(pkg)+1 > maxGoTestCommandLen {
			if err := appendMonolithMetadata(batch, file); err != nil {
				return err
			}
			batch = append([]string(nil), args...)
		}
		batch = append(batch, pkg)
	}
	if len(batch) > len(args) {
		return appendMonolithMetadata(batch, file)
	}
	return nil
}

func appendMonolithMetadata(args []string, output io.Writer) error {
	cmd := execCommand("go", args...)
	var data bytes.Buffer
	cmd.Stdout, cmd.Stderr = &data, stderrWriter
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("discover native Go test registrations: %w", err)
	}
	decoder := json.NewDecoder(&data)
	encoder := json.NewEncoder(output)
	for {
		var pkg monolithPackageMetadata
		if err := decoder.Decode(&pkg); errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return err
		}
		if len(pkg.XTestEmbedPatterns) > 0 {
			if err := resolveMonolithTestEmbeds(&pkg); err != nil {
				return err
			}
		}
		if err := encoder.Encode(pkg); err != nil {
			return err
		}
		registrations, err := monolithRegistrations(pkg)
		if err != nil {
			return err
		}
		// The overlay generator consumes this registration descriptor, rather
		// than asking go list -test to build an additional synthetic package graph.
		digest := sha256.Sum256([]byte(pkg.ImportPath))
		file, ok := output.(*os.File)
		if !ok {
			return errors.New("monolith metadata requires a file workspace")
		}
		path := filepath.Join(filepath.Dir(file.Name()), fmt.Sprintf("registrations-%x.go", digest[:12]))
		if err := os.WriteFile(path, []byte(registrations), 0600); err != nil {
			return err
		}
		if err := encoder.Encode(struct {
			ImportPath, Name string
			GoFiles          []string
		}{pkg.ImportPath + ".test", "main", []string{path}}); err != nil {
			return err
		}
	}
}

type monolithPackageMetadata struct {
	ImportPath, Dir, Name                      string
	TestGoFiles, XTestGoFiles, XTestEmbedFiles []string `json:",omitempty"`
	XTestEmbedPatterns                         []string `json:",omitempty"`
}

func resolveMonolithTestEmbeds(pkg *monolithPackageMetadata) error {
	// Ordinary go list exposes patterns but resolves test embeds only with
	// -test. Ask Go for that resolution only for packages that actually embed
	// external-test fixtures, preserving its glob and hidden-file semantics.
	cmd := execCommand("go", "list", "-test", "-json=ImportPath,ForTest,XTestEmbedFiles", pkg.ImportPath)
	var data bytes.Buffer
	cmd.Stdout, cmd.Stderr = &data, stderrWriter
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("resolve test embeds for %s: %w", pkg.ImportPath, err)
	}
	decoder := json.NewDecoder(&data)
	for {
		var candidate struct {
			ImportPath, ForTest string
			XTestEmbedFiles     []string
		}
		if err := decoder.Decode(&candidate); err != nil {
			return fmt.Errorf("resolve test embeds for %s: %w", pkg.ImportPath, err)
		}
		if candidate.ImportPath == pkg.ImportPath && candidate.ForTest == "" {
			pkg.XTestEmbedFiles = candidate.XTestEmbedFiles
			if len(pkg.XTestEmbedFiles) == 0 {
				return fmt.Errorf("no resolved test embeds for %s", pkg.ImportPath)
			}
			return nil
		}
	}
}

func monolithRegistrations(pkg monolithPackageMetadata) (string, error) {
	entries := map[string][]string{"tests": {}, "benchmarks": {}, "fuzzTargets": {}, "examples": {}}
	mainAlias := ""
	for _, group := range []struct {
		alias string
		files []string
	}{{"_test", pkg.TestGoFiles}, {"_xtest", pkg.XTestGoFiles}} {
		for _, name := range group.files {
			file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(pkg.Dir, name), nil, parser.ParseComments)
			if err != nil {
				return "", err
			}
			for _, declaration := range file.Decls {
				fn, ok := declaration.(*ast.FuncDecl)
				if !ok || fn.Recv != nil {
					continue
				}
				name := fn.Name.Name
				if name == "TestMain" {
					mainAlias = group.alias
					continue
				}
				for _, kind := range []struct{ prefix, section string }{{"Test", "tests"}, {"Benchmark", "benchmarks"}, {"Fuzz", "fuzzTargets"}} {
					if isMonolithTestName(name, kind.prefix) {
						entries[kind.section] = append(entries[kind.section], fmt.Sprintf("{%s, %s.%s},", strconv.Quote(name), group.alias, name))
					}
				}
			}
			// Packages with examples keep native registration and output matching.
			// Conservatively retain even examples without an output oracle.
			for _, example := range doc.Examples(file) {
				name := "Example" + example.Name
				entries["examples"] = append(entries["examples"], fmt.Sprintf("{%s, %s.%s, %s, false},", strconv.Quote(name), group.alias, name, strconv.Quote(example.Output)))
			}
		}
	}
	var source strings.Builder
	for _, kind := range []struct{ name, typ string }{{"tests", "InternalTest"}, {"benchmarks", "InternalBenchmark"}, {"fuzzTargets", "InternalFuzzTarget"}, {"examples", "InternalExample"}} {
		fmt.Fprintf(&source, "var %s = []testing.%s{\n%s\n}\n", kind.name, kind.typ, strings.Join(entries[kind.name], "\n"))
	}
	if mainAlias != "" {
		fmt.Fprintf(&source, "func main() { %s.TestMain(m) }\n", mainAlias)
	}
	return source.String(), nil
}

func isMonolithTestName(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	suffix := strings.TrimPrefix(name, prefix)
	for _, char := range suffix {
		return !unicode.IsLower(char)
	}
	return true
}

func readMonolithJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func validateMonolithInventory(packages []string, groups []monolithGroup, excluded map[string]string) error {
	seen := make(map[string]bool, len(packages))
	for _, group := range groups {
		if seen[group.Package] {
			return fmt.Errorf("duplicate monolith package %s", group.Package)
		}
		seen[group.Package] = true
	}
	for pkg := range excluded {
		if seen[pkg] {
			return fmt.Errorf("package both merged and native: %s", pkg)
		}
		seen[pkg] = true
	}
	for _, pkg := range packages {
		if !seen[pkg] {
			return fmt.Errorf("monolith omitted selected package %s", pkg)
		}
		delete(seen, pkg)
	}
	if len(seen) != 0 {
		return fmt.Errorf("monolith included unselected packages: %v", seen)
	}
	return nil
}

func executeMonolithBinary(cfg config, binary string, groups []monolithGroup) (unitTimingCapture, error) {
	args := []string{"-test.count=1", "-test.timeout=" + cfg.timeout.String()}
	if cfg.short {
		args = append(args, "-test.short")
	}
	cmd := execCommand(binary, args...)
	if cfg.monolithDetails {
		args = append([]string{"tool", "test2json", "-t", "-p", modulePath + "/pkg/monolithpilot", binary, "-test.v=test2json"}, args...)
		cmd = execCommand("go", args...)
	} else {
		cmd.Env = append(os.Environ(), "UNIT_MONOLITH_COMPACT=1")
	}
	cmd.Stderr = stderrWriter
	output, err := cmd.StdoutPipe()
	if err != nil {
		return unitTimingCapture{}, err
	}
	if err := cmd.Start(); err != nil {
		return unitTimingCapture{}, err
	}
	reader, writer := io.Pipe()
	mappingErrors := make(chan error, 1)
	go func() {
		var err error
		if cfg.monolithDetails {
			err = remapMonolithEvents(output, writer, groups)
		} else {
			err = remapCompactMonolithEvents(output, writer)
		}
		writer.CloseWithError(err)
		mappingErrors <- err
	}()
	defer reader.Close()
	expected := make([]string, 0, len(groups))
	for _, group := range groups {
		expected = append(expected, group.Package)
	}
	capture, captureErr := collectUnitTimingCapture(reader, expected, stdoutWriter)
	if !capture.Complete {
		captureErr = errors.Join(captureErr, errors.New("merged test execution did not complete its prepared package inventory"))
	}
	if !cfg.monolithDetails {
		// Direct execution always uses -test.count=1 and cannot serve a cached
		// test result; compact terminal records need no artificial output lines.
		for index := range capture.Packages {
			capture.Packages[index].Cache = unitCacheExecuted
		}
	}
	runErr := cmd.Wait()
	return capture, errors.Join(captureErr, runErr, <-mappingErrors)
}

func remapCompactMonolithEvents(input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	encoder := json.NewEncoder(output)
	for scanner.Scan() {
		line := scanner.Text()
		event := goTestUnitTimingEvent{Action: "output", Output: line + "\n"}
		if data, found := strings.CutPrefix(line, "UNIT_EVENT "); found {
			event = goTestUnitTimingEvent{}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				return err
			}
		}
		if err := encoder.Encode(event); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// Restore original package/test identities for the existing timing collector.
// Three native test names currently occur in both internal and external tests;
// only those top-level duplicates have Go's added #NN suffix normalized.
func remapMonolithEvents(input io.Reader, output io.Writer, groups []monolithGroup) error {
	byName := make(map[string]monolithGroup, len(groups))
	for _, group := range groups {
		byName[group.Group] = group
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	encoder := json.NewEncoder(output)
	for scanner.Scan() {
		var event goTestUnitTimingEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return err
		}
		parts := strings.SplitN(event.Test, "/", 3)
		if len(parts) < 2 || parts[0] != "TestUnitPackages" {
			if event.Output != "" {
				event.Action, event.Package, event.Test = "output", "", ""
				if err := encoder.Encode(event); err != nil {
					return err
				}
			}
			continue
		}
		group, ok := byName[parts[1]]
		if !ok {
			return fmt.Errorf("unknown monolith test group %s", parts[1])
		}
		event.Package, event.Test = group.Package, ""
		if len(parts) == 3 {
			event.Test = originalMonolithTestName(parts[2], group.Tests)
		}
		if err := encoder.Encode(event); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func originalMonolithTestName(name string, topLevel []string) string {
	head, tail, nested := strings.Cut(name, "/")
	base, suffix, numbered := strings.Cut(head, "#")
	if !numbered || suffix == "" {
		return name
	}
	for _, char := range suffix {
		if char < '0' || char > '9' {
			return name
		}
	}
	count := 0
	for _, test := range topLevel {
		if test == base {
			count++
		}
	}
	if count < 2 {
		return name
	}
	if nested {
		return base + "/" + tail
	}
	return base
}
