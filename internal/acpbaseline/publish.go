package acpbaseline

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// MaxCommittedFileBytes bounds any committed baseline artifact, so a capture
// cannot quietly bloat the repository.
const MaxCommittedFileBytes = 512 * 1024

// Publish writes the committable tier of a capture: the structural digest, the
// manifest, and the capability matrix.
//
// The raw and scrubbed tiers deliberately stay behind in the artifacts
// directory. Only what cannot carry a prompt, a file body, or a credential is
// written here.
func Publish(manifest *Manifest, matrix *CapabilityMatrix, sourceDir, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	literals := EnvironmentLiterals()

	for scenario, rawPath := range manifest.TranscriptFiles {
		digestPath := filepath.Join(destDir, scenario+".digest.jsonl")
		if err := digestFile(rawPath, digestPath, literals); err != nil {
			return fmt.Errorf("digest %s: %w", scenario, err)
		}
	}
	if err := writeJSON(filepath.Join(destDir, "manifest.json"), publishable(manifest)); err != nil {
		return err
	}
	return writeJSON(filepath.Join(destDir, "capability-matrix.json"), matrix)
}

func digestFile(sourcePath, destPath string, literals []string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()

	dest, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = dest.Close() }()

	reader := bufio.NewReader(source)
	writer := bufio.NewWriter(dest)
	ordinal := 0
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) > 0 {
			ordinal++
			// Scrub first so a secret cannot survive inside a value the
			// digest happens to preserve.
			scrubbed := Scrub(string(line), literals)
			digested, digestErr := DigestRecordLine([]byte(scrubbed), ordinal)
			if digestErr != nil {
				return digestErr
			}
			if _, err := writer.Write(append(digested, '\n')); err != nil {
				return err
			}
		}
		if readErr != nil {
			break
		}
	}
	return writer.Flush()
}

func writeJSON(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(encoded, '\n'), 0o644)
}

// absolutePathPattern matches machine-specific absolute paths that must never
// reach a committed artifact. Temp roots are included because a capture's own
// artifact directory lives under one, and that path names the operator's
// machine just as surely as a home directory does.
var absolutePathPattern = regexp.MustCompile(
	`(/Users/|/home/|/private/|/tmp/|/var/folders/|C:\\\\Users\\\\)[A-Za-z0-9._\-]+`)

// VerifyCommitted is the enforcement behind the commit policy.
//
// It walks the committed baseline tree and fails on anything that should never
// have been committed: an undigested string, a secret pattern, a machine path,
// or an oversized file. Policy stated only in prose does not survive; this
// does.
func VerifyCommitted(root string) ([]string, error) {
	var findings []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if info.IsDir() || strings.HasSuffix(path, ".md") {
			return nil
		}
		if info.Size() > MaxCommittedFileBytes {
			findings = append(findings, fmt.Sprintf(
				"%s: %d bytes exceeds the %d byte committed-artifact budget",
				path, info.Size(), MaxCommittedFileBytes))
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		findings = append(findings, inspectCommitted(path, data)...)
		return nil
	})
	return findings, err
}

func inspectCommitted(path string, data []byte) []string {
	var findings []string
	text := string(data)
	for _, pattern := range secretPatterns {
		if pattern.MatchString(text) {
			findings = append(findings, path+": matches a secret pattern")
			break
		}
	}
	if absolutePathPattern.MatchString(text) {
		findings = append(findings, path+": contains a machine-specific absolute path")
	}
	if !strings.HasSuffix(path, ".digest.jsonl") {
		return findings
	}
	for index, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if where, found := ContainsRawContent([]byte(line)); found {
			findings = append(findings, fmt.Sprintf(
				"%s line %d: undigested string at %s -- a raw transcript must never be committed",
				path, index+1, where))
			break
		}
	}
	return findings
}

// publishable strips a manifest of anything machine-specific.
//
// The raw transcript locations are absolute paths under the capture artifact
// directory. They are useful locally and meaningless once published, and they
// name the operator's machine, so only the file names survive.
func publishable(manifest *Manifest) *Manifest {
	published := *manifest
	published.TranscriptFiles = map[string]string{}
	for scenario, path := range manifest.TranscriptFiles {
		published.TranscriptFiles[scenario] = filepath.Base(path)
	}
	// The agent argv names a local binary. Publishing the basename keeps the
	// capture reproducible ("this was our own serve acp") without recording
	// where on the operator's disk it happened to live.
	published.Command = nil
	for _, argument := range manifest.Command {
		if strings.HasPrefix(argument, "/") || strings.Contains(argument, `\`) {
			argument = filepath.Base(argument)
		}
		published.Command = append(published.Command, argument)
	}
	published.Errors = nil
	for _, failure := range manifest.Errors {
		published.Errors = append(published.Errors, absolutePathPattern.ReplaceAllString(failure, "<path>"))
	}
	return &published
}
