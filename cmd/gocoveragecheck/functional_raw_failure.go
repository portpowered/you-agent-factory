package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	functionalRawFailureSchemaVersion = "functional-raw-failures.v1"
	functionalRawFailureIndexName     = "index.json"
	functionalRawFailurePackageLimit  = int64(64 << 20)
)

type functionalRawFailureCapture struct {
	mu                 sync.Mutex
	directory          string
	configuredMaxBytes int64
	packageMaxBytes    int64
	capturedBytes      int64
	packages           map[string]*functionalRawFailurePackage
	executedCommands   [][]string
	activeCommand      []string
	activePackages     []string
	captureError       error
	commandExitStatus  int
	write              func(io.Writer, []byte) error
}

type functionalRawFailurePackage struct {
	packageName    string
	file           *os.File
	temporaryPath  string
	capturedBytes  int64
	observedBytes  int64
	capturedEvents int64
	observedEvents int64
	capHit         bool
	failed         bool
	terminal       string
	incomplete     bool
	selectors      map[string]struct{}
	command        []string
	exitStatus     int
}

type functionalRawFailureIndex struct {
	SchemaVersion               string                             `json:"schemaVersion"`
	Head                        string                             `json:"head"`
	ConfiguredMaxBytes          int64                              `json:"configuredMaxBytes"`
	PackageMaxBytes             int64                              `json:"packageMaxBytes"`
	ExecutedCommands            [][]string                         `json:"executedCommands"`
	SuccessfulPackagesDiscarded int                                `json:"successfulPackagesDiscarded"`
	SuccessfulObservedBytes     int64                              `json:"successfulObservedBytes"`
	Failures                    []functionalRawFailurePackageIndex `json:"failures"`
	CaptureStatus               string                             `json:"captureStatus"`
	CaptureError                string                             `json:"captureError,omitempty"`
}

type functionalRawFailurePackageIndex struct {
	Package            string    `json:"package"`
	Selectors          []string  `json:"selectors"`
	SelectorStatus     string    `json:"selectorStatus"`
	Command            []string  `json:"command"`
	ExitStatus         int       `json:"exitStatus"`
	File               string    `json:"file,omitempty"`
	CapturedBytes      int64     `json:"capturedBytes"`
	ObservedBytes      int64     `json:"observedBytes"`
	CapturedEventRange *[2]int64 `json:"capturedEventRange"`
	OmittedEventRange  *[2]int64 `json:"omittedEventRange"`
	OmittedBytesRange  *[2]int64 `json:"omittedBytesRange"`
	CapHit             bool      `json:"capHit"`
	CaptureStatus      string    `json:"captureStatus"`
	Reproduce          string    `json:"reproduce"`
}

func newFunctionalRawFailureCapture(directory string, configuredMaxBytes int64) (*functionalRawFailureCapture, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return nil, errors.New("raw failure capture directory is empty")
	}
	cleanDirectory := filepath.Clean(directory)
	if cleanDirectory == "." || filepath.Dir(cleanDirectory) == cleanDirectory {
		return nil, errors.New("raw failure capture directory cannot be the current directory or a filesystem root")
	}
	if configuredMaxBytes <= 0 {
		return nil, errors.New("raw failure max bytes must be positive")
	}
	if configuredMaxBytes > 512<<20 {
		return nil, errors.New("raw failure max bytes exceeds the supported 512 MiB limit")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, fmt.Errorf("create raw failure directory %s: %w", directory, err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read raw failure directory %s: %w", directory, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if name != functionalRawFailureIndexName && !(strings.HasPrefix(name, "functional-failure-") && strings.HasSuffix(name, ".jsonl")) && !strings.HasPrefix(name, ".raw-spool-") && !strings.HasPrefix(name, ".index-raw-") {
			continue
		}
		if err := os.RemoveAll(filepath.Join(directory, name)); err != nil {
			return nil, fmt.Errorf("remove stale raw failure artifact %s: %w", name, err)
		}
	}
	packageMaxBytes := configuredMaxBytes
	if packageMaxBytes > functionalRawFailurePackageLimit {
		packageMaxBytes = functionalRawFailurePackageLimit
	}
	return &functionalRawFailureCapture{
		directory:          directory,
		configuredMaxBytes: configuredMaxBytes,
		packageMaxBytes:    packageMaxBytes,
		packages:           make(map[string]*functionalRawFailurePackage),
		write:              writeRawFailureBytes,
	}, nil
}

func currentFunctionalRawFailureHead(repositoryRoot string) (string, error) {
	command := exec.Command("git", "rev-parse", "HEAD")
	command.Dir = repositoryRoot
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("read raw failure artifact head: %w", err)
	}
	head := strings.TrimSpace(string(output))
	if len(head) != 40 {
		return "", fmt.Errorf("read raw failure artifact head: git returned %q instead of a full commit SHA", head)
	}
	return head, nil
}

func (capture *functionalRawFailureCapture) beginInvocation(invocation commandInvocation) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	capture.activeCommand = append([]string{invocation.name}, invocation.args...)
	capture.executedCommands = append(capture.executedCommands, slices.Clone(capture.activeCommand))
	capture.activePackages = coverageInvocationPackages(invocation.args)
	capture.commandExitStatus = 0
}

func (capture *functionalRawFailureCapture) observeLine(line []byte) error {
	event, ignored, err := parseFunctionalRawFailureEvent(line)
	if err != nil {
		capture.mu.Lock()
		defer capture.mu.Unlock()
		return capture.failLocked(err)
	}
	if ignored {
		return nil
	}

	capture.mu.Lock()
	defer capture.mu.Unlock()
	if capture.captureError != nil {
		return capture.captureError
	}
	return capture.observeEventLocked(event, line)
}

func parseFunctionalRawFailureEvent(line []byte) (goTestTimingEvent, bool, error) {
	var event goTestTimingEvent
	if err := json.Unmarshal(bytes.TrimSpace(line), &event); err != nil {
		return goTestTimingEvent{}, false, errors.New("unattributable or malformed go test JSON output")
	}
	if event.Package == "" {
		// `go test -json -x` emits compiler and linker trace records as
		// build-output events. They are not test package output and should not
		// interrupt the package-keyed failure spool.
		if event.Action == "build-output" {
			return event, true, nil
		}
		return goTestTimingEvent{}, false, errors.New("unattributable or malformed go test JSON output")
	}
	return event, false, nil
}

func (capture *functionalRawFailureCapture) observeEventLocked(event goTestTimingEvent, line []byte) error {
	packageOutput := capture.packages[event.Package]
	if packageOutput == nil {
		packageOutput = &functionalRawFailurePackage{
			packageName: event.Package,
			selectors:   make(map[string]struct{}),
			command:     slices.Clone(capture.activeCommand),
		}
		capture.packages[event.Package] = packageOutput
	}
	packageOutput.observedBytes += int64(len(line))
	packageOutput.observedEvents++
	if event.Action == timingOutcomeFail {
		packageOutput.failed = true
		if event.Test != "" {
			packageOutput.selectors[event.Test] = struct{}{}
		}
	}
	if event.Test == "" {
		switch event.Action {
		case timingOutcomePass, timingOutcomeSkip:
			packageOutput.terminal = event.Action
			if err := capture.discardPackageSpool(packageOutput); err != nil {
				return capture.failLocked(fmt.Errorf("discard successful raw package spool for %s: %w", event.Package, err))
			}
			return nil
		case timingOutcomeFail:
			packageOutput.terminal = event.Action
		}
	}
	return capture.spoolPackageLineLocked(packageOutput, event.Package, line)
}

func (capture *functionalRawFailureCapture) spoolPackageLineLocked(packageOutput *functionalRawFailurePackage, packageName string, line []byte) error {
	if packageOutput.capHit {
		return nil
	}
	packageRemaining := capture.packageMaxBytes - packageOutput.capturedBytes
	globalRemaining := capture.configuredMaxBytes - capture.capturedBytes
	if int64(len(line)) > packageRemaining || int64(len(line)) > globalRemaining {
		packageOutput.capHit = true
		return nil
	}
	if packageOutput.file == nil {
		file, err := os.CreateTemp(capture.directory, ".raw-spool-*")
		if err != nil {
			return capture.failLocked(fmt.Errorf("create raw package spool for %s: %w", packageName, err))
		}
		packageOutput.file = file
		packageOutput.temporaryPath = file.Name()
	}
	if err := capture.write(packageOutput.file, line); err != nil {
		return capture.failLocked(fmt.Errorf("write raw package spool for %s: %w", packageName, err))
	}
	packageOutput.capturedBytes += int64(len(line))
	packageOutput.capturedEvents++
	capture.capturedBytes += int64(len(line))
	return nil
}

func (capture *functionalRawFailureCapture) failLocked(err error) error {
	if capture.captureError == nil {
		capture.captureError = err
	}
	return capture.captureError
}

func writeRawFailureBytes(writer io.Writer, value []byte) error {
	written, err := writer.Write(value)
	if err != nil {
		return err
	}
	if written != len(value) {
		return io.ErrShortWrite
	}
	return nil
}

func (capture *functionalRawFailureCapture) discardPackageSpool(packageOutput *functionalRawFailurePackage) error {
	if packageOutput.file != nil {
		if err := packageOutput.file.Close(); err != nil {
			return err
		}
		packageOutput.file = nil
	}
	if packageOutput.temporaryPath != "" {
		if err := os.Remove(packageOutput.temporaryPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		packageOutput.temporaryPath = ""
	}
	capture.capturedBytes -= packageOutput.capturedBytes
	packageOutput.capturedBytes = 0
	packageOutput.capturedEvents = 0
	packageOutput.capHit = false
	return nil
}

func (capture *functionalRawFailureCapture) finishInvocation(commandErr error) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	capture.commandExitStatus = functionalRawFailureExitStatus(commandErr)
	for _, packageName := range capture.activePackages {
		if !isConcreteCoveragePackage(packageName) {
			continue
		}
		packageOutput := capture.packages[packageName]
		if packageOutput == nil {
			packageOutput = &functionalRawFailurePackage{
				packageName: packageName,
				selectors:   make(map[string]struct{}),
				command:     slices.Clone(capture.activeCommand),
			}
			capture.packages[packageName] = packageOutput
		}
		if packageOutput.terminal == "" {
			packageOutput.incomplete = true
		}
		if packageOutput.failed || packageOutput.incomplete {
			packageOutput.command = slices.Clone(capture.activeCommand)
			packageOutput.exitStatus = capture.commandExitStatus
		}
	}
	for _, packageOutput := range capture.packages {
		if !slices.Equal(packageOutput.command, capture.activeCommand) {
			continue
		}
		if packageOutput.terminal == "" {
			packageOutput.incomplete = true
		}
		if packageOutput.failed || packageOutput.incomplete {
			packageOutput.exitStatus = capture.commandExitStatus
		}
	}
}

func functionalRawFailureExitStatus(commandErr error) int {
	if commandErr == nil {
		return 0
	}
	var exitError *exec.ExitError
	if errors.As(commandErr, &exitError) && exitError.ExitCode() >= 0 {
		return exitError.ExitCode()
	}
	const marker = "exit status "
	message := commandErr.Error()
	if markerIndex := strings.LastIndex(message, marker); markerIndex >= 0 {
		if status, err := strconv.Atoi(strings.TrimSpace(message[markerIndex+len(marker):])); err == nil && status >= 0 {
			return status
		}
	}
	return 1
}

func coverageInvocationPackages(arguments []string) []string {
	if len(arguments) == 0 || arguments[0] != "test" {
		return nil
	}
	packages := make([]string, 0)
	for _, argument := range arguments[1:] {
		if strings.HasPrefix(argument, "-") || argument == "" {
			continue
		}
		packages = append(packages, argument)
	}
	return packages
}

func isConcreteCoveragePackage(packageName string) bool {
	return packageName != "." && !strings.Contains(packageName, "...") && !strings.ContainsAny(packageName, "*?[{")
}

func (capture *functionalRawFailureCapture) publish(head string, testFailure bool) error {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	index, err := capture.buildIndexLocked(head, testFailure)
	if err != nil {
		return err
	}
	if err := writeFunctionalRawFailureIndex(capture.directory, index); err != nil {
		return err
	}
	if index.CaptureStatus == "incomplete" {
		return fmt.Errorf("raw functional failure capture incomplete: %s", index.CaptureError)
	}
	return nil
}

func (capture *functionalRawFailureCapture) buildIndexLocked(head string, testFailure bool) (functionalRawFailureIndex, error) {
	index := functionalRawFailureIndex{
		SchemaVersion:      functionalRawFailureSchemaVersion,
		Head:               head,
		ConfiguredMaxBytes: capture.configuredMaxBytes,
		PackageMaxBytes:    capture.packageMaxBytes,
		ExecutedCommands:   cloneStringMatrix(capture.executedCommands),
		Failures:           make([]functionalRawFailurePackageIndex, 0),
		CaptureStatus:      "complete",
	}
	if capture.captureError != nil {
		index.CaptureStatus = "incomplete"
		index.CaptureError = capture.captureError.Error()
	}
	packageNames := make([]string, 0, len(capture.packages))
	for packageName := range capture.packages {
		packageNames = append(packageNames, packageName)
	}
	sort.Strings(packageNames)
	hasIncomplete := capture.captureError != nil
	hasCapped := false
	for _, packageName := range packageNames {
		packageOutput := capture.packages[packageName]
		if !packageOutput.failed && !packageOutput.incomplete {
			if packageOutput.terminal == timingOutcomePass || packageOutput.terminal == timingOutcomeSkip {
				index.SuccessfulPackagesDiscarded++
				index.SuccessfulObservedBytes += packageOutput.observedBytes
			}
			if err := closeAndRemoveRawSpool(packageOutput); err != nil {
				return functionalRawFailureIndex{}, err
			}
			continue
		}
		entry, err := capture.publishPackage(packageOutput)
		if err != nil {
			return functionalRawFailureIndex{}, err
		}
		index.Failures = append(index.Failures, entry)
		hasIncomplete = hasIncomplete || packageOutput.incomplete || entry.CaptureStatus == "incomplete" || capture.captureError != nil
		if packageOutput.capHit {
			hasCapped = true
		}
	}
	if hasIncomplete {
		index.CaptureStatus = "incomplete"
	} else if hasCapped {
		index.CaptureStatus = "capped"
	}
	if capture.captureError != nil {
		index.CaptureError = capture.captureError.Error()
	}
	if testFailure && len(index.Failures) == 0 {
		index.CaptureStatus = "incomplete"
		index.CaptureError = "functional test command failed without attributable package events"
	}
	return index, nil
}

func writeFunctionalRawFailureIndex(directory string, index functionalRawFailureIndex) error {
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("encode raw failure index: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".index-raw-*")
	if err != nil {
		return fmt.Errorf("create raw failure index: %w", err)
	}
	temporaryPath := temporary.Name()
	if err := writeRawFailureBytes(temporary, append(data, '\n')); err != nil {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("write raw failure index: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("sync raw failure index: %w", err)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("close raw failure index: %w", err)
	}
	if err := os.Rename(temporaryPath, filepath.Join(directory, functionalRawFailureIndexName)); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("publish raw failure index: %w", err)
	}
	return nil
}

func (capture *functionalRawFailureCapture) publishPackage(packageOutput *functionalRawFailurePackage) (functionalRawFailurePackageIndex, error) {
	entry := functionalRawFailurePackageIndex{
		Package:       packageOutput.packageName,
		Selectors:     sortedRawFailureSelectors(packageOutput.selectors),
		Command:       slices.Clone(packageOutput.command),
		ExitStatus:    packageOutput.exitStatus,
		CapturedBytes: packageOutput.capturedBytes,
		ObservedBytes: packageOutput.observedBytes,
		CapHit:        packageOutput.capHit,
		CaptureStatus: "complete",
	}
	if len(entry.Selectors) == 0 {
		entry.SelectorStatus = "unavailable"
	} else {
		entry.SelectorStatus = "observed"
	}
	if packageOutput.capturedEvents > 0 {
		entry.CapturedEventRange = &[2]int64{1, packageOutput.capturedEvents}
	}
	if packageOutput.observedEvents > packageOutput.capturedEvents {
		entry.OmittedEventRange = &[2]int64{packageOutput.capturedEvents + 1, packageOutput.observedEvents}
		entry.OmittedBytesRange = &[2]int64{packageOutput.capturedBytes + 1, packageOutput.observedBytes}
	}
	if packageOutput.incomplete {
		entry.CaptureStatus = "incomplete"
	}
	if packageOutput.capHit {
		entry.CaptureStatus = "capped"
	}
	if packageOutput.temporaryPath != "" {
		if packageOutput.file != nil {
			if err := packageOutput.file.Sync(); err != nil {
				capture.captureError = errors.Join(capture.captureError, fmt.Errorf("sync raw package spool for %s: %w", packageOutput.packageName, err))
				entry.CaptureStatus = "incomplete"
			}
			if err := packageOutput.file.Close(); err != nil {
				capture.captureError = errors.Join(capture.captureError, fmt.Errorf("close raw package spool for %s: %w", packageOutput.packageName, err))
				entry.CaptureStatus = "incomplete"
			}
			packageOutput.file = nil
		}
		fileName := rawFailurePackageFileName(packageOutput.packageName)
		finalPath := filepath.Join(capture.directory, fileName)
		if err := os.Rename(packageOutput.temporaryPath, finalPath); err != nil {
			capture.captureError = errors.Join(capture.captureError, fmt.Errorf("publish raw package spool for %s: %w", packageOutput.packageName, err))
			entry.CaptureStatus = "incomplete"
		} else {
			entry.File = fileName
		}
	}
	entry.Reproduce = reproduceFunctionalRawFailure(entry.Command, entry.Package, entry.Selectors)
	return entry, nil
}

func closeAndRemoveRawSpool(packageOutput *functionalRawFailurePackage) error {
	if packageOutput.file != nil {
		if err := packageOutput.file.Close(); err != nil {
			return fmt.Errorf("close successful raw package spool for %s: %w", packageOutput.packageName, err)
		}
		packageOutput.file = nil
	}
	if packageOutput.temporaryPath != "" {
		if err := os.Remove(packageOutput.temporaryPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove successful raw package spool for %s: %w", packageOutput.packageName, err)
		}
	}
	return nil
}

func sortedRawFailureSelectors(selectors map[string]struct{}) []string {
	result := make([]string, 0, len(selectors))
	for selector := range selectors {
		result = append(result, selector)
	}
	sort.Strings(result)
	return result
}

func cloneStringMatrix(values [][]string) [][]string {
	result := make([][]string, 0, len(values))
	for _, value := range values {
		result = append(result, slices.Clone(value))
	}
	return result
}

func rawFailurePackageFileName(packageName string) string {
	segments := strings.Split(strings.Trim(packageName, "/"), "/")
	base := segments[len(segments)-1]
	var safe strings.Builder
	for _, character := range base {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_' {
			safe.WriteRune(character)
		}
	}
	if safe.Len() == 0 {
		safe.WriteString("package")
	}
	if safe.Len() > 80 {
		safeString := safe.String()[:80]
		safe.Reset()
		safe.WriteString(safeString)
	}
	hash := sha256.Sum256([]byte(packageName))
	return "functional-failure-" + safe.String() + "-" + hex.EncodeToString(hash[:8]) + ".jsonl"
}

var goTestSelectorSegmentPattern = regexp.MustCompile(`[^/]+`)

func reproduceFunctionalRawFailure(command []string, packageName string, selectors []string) string {
	if len(command) == 0 {
		return ""
	}
	arguments := make([]string, 0, len(command)+2)
	arguments = append(arguments, command[0])
	for _, argument := range command[1:] {
		if strings.HasPrefix(argument, "-coverprofile=") || strings.HasPrefix(argument, "-coverpkg=") || strings.HasPrefix(argument, "-run=") {
			continue
		}
		if argument == packageName || (!strings.HasPrefix(argument, "-") && argument != "test") {
			continue
		}
		arguments = append(arguments, argument)
	}
	if len(selectors) > 0 {
		patterns := make([]string, 0, len(selectors))
		for _, selector := range selectors {
			segments := goTestSelectorSegmentPattern.FindAllString(selector, -1)
			for index := range segments {
				segments[index] = "^" + regexp.QuoteMeta(segments[index]) + "$"
			}
			patterns = append(patterns, strings.Join(segments, "/"))
		}
		arguments = append(arguments, "-run="+strings.Join(patterns, "|"))
	}
	arguments = append(arguments, packageName)
	quoted := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		quoted = append(quoted, strconv.Quote(argument))
	}
	return strings.Join(quoted, " ")
}
