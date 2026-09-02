package commands_test

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
)

var packagedDocsIndexTopicPattern = regexp.MustCompile(`(?m)^- ` + "`" + `([a-z][a-z0-9-]*)` + "`" + ` - `)

// TestCLIDocsCustomerExperience proves customers can discover packaged docs and
// receive an actionable error for an unsupported topic through one reusable
// public CLI process. Per-topic packaged-content conformance belongs to the
// docs smoke/contract lane rather than a functional inventory.
func TestCLIDocsCustomerExperience(t *testing.T) {
	t.Parallel()
	workingDir := isolatedWorkingDirectoryWithoutDocsTree(t)
	processHarness := newLocalReusableProcessHarness(t)

	t.Run("lists packaged topics", func(t *testing.T) {
		output := executeDocsWiringCommand(t, processHarness, workingDir, "docs")
		if strings.TrimSpace(output) == "" {
			t.Fatal("you docs index stdout is empty")
		}

		for _, marker := range []string{
			"# Docs",
			"Packaged reference topics:",
		} {
			if !strings.Contains(output, marker) {
				t.Fatalf("docs index missing packaged discovery marker %q:\n%s", marker, output)
			}
		}

		topics := parsePackagedDocsIndexTopics(output)
		if len(topics) == 0 {
			t.Fatalf("docs index did not expose any discoverable packaged topics:\n%s", output)
		}
		for _, topic := range topics {
			if !strings.Contains(output, "you docs "+topic) {
				t.Fatalf("docs index missing customer-visible retrieval hint for topic %q:\n%s", topic, output)
			}
		}
	})

	t.Run("unknown topic is actionable", func(t *testing.T) {
		const unknownTopic = "unknown"
		stdout, err := executeDocsWiringCommandResult(t, processHarness, workingDir, "docs", unknownTopic)
		if err == nil {
			t.Fatal("expected unknown docs topic to fail")
		}
		if !strings.Contains(err.Error(), `unsupported docs topic "`+unknownTopic+`"`) {
			t.Fatalf("unexpected unsupported topic error %q", err.Error())
		}
		if strings.TrimSpace(stdout) != "" {
			t.Fatalf("unsupported docs topic should not write stdout, got %q", stdout)
		}
	})
}

func isolatedWorkingDirectoryWithoutDocsTree(t *testing.T) string {
	t.Helper()

	workingDir := t.TempDir()
	docsTree := filepath.Join(workingDir, "docs")
	if _, err := os.Stat(docsTree); !os.IsNotExist(err) {
		t.Fatalf("working directory unexpectedly contains docs tree %q", docsTree)
	}
	return workingDir
}

func executeDocsWiringCommand(
	t *testing.T,
	processHarness *builtcliacceptance.Harness,
	workingDir string,
	args ...string,
) string {
	t.Helper()

	stdout, err := executeDocsWiringCommandResult(t, processHarness, workingDir, args...)
	if err != nil {
		t.Fatalf("execute root command %v: %v\nstdout:\n%s", args, err, stdout)
	}
	return stdout
}

func executeDocsWiringCommandResult(
	t *testing.T,
	processHarness *builtcliacceptance.Harness,
	workingDir string,
	args ...string,
) (stdout string, err error) {
	t.Helper()

	command := processHarness.CommandContext(t.Context(), args...)
	command.Dir = workingDir
	var stdoutBuffer, stderrBuffer bytes.Buffer
	command.Stdout = &stdoutBuffer
	command.Stderr = &stderrBuffer
	err = command.Run()
	return stdoutBuffer.String(), err
}

func parsePackagedDocsIndexTopics(index string) []string {
	matches := packagedDocsIndexTopicPattern.FindAllStringSubmatch(index, -1)
	topics := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		topics = append(topics, match[1])
	}
	return topics
}
