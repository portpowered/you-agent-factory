package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/portpowered/agent-factory/internal/testpath"
)

func TestFactoryConfigDocsAndExamples_UseCanonicalPublicContractFields(t *testing.T) {
	t.Parallel()

	targetFiles := existingFiles(append([]string{
		repoPath(t, "README.md"),
		repoPath(t, "docs", "authoring-agents-md.md"),
		repoPath(t, "docs", "authoring-workflows.md"),
		repoPath(t, "docs", "work.md"),
		repoPath(t, "docs", "workstations.md"),
		repoPath(t, "docs", "run-timeline.md"),
		repoPath(t, "docs", "guides", "parent-aware-fan-in.md"),
		repoPath(t, "docs", "guides", "workstation-guards-and-guarded-loop-breakers.md"),
		repoPath(t, "factory", "README.md"),
		repoPath(t, "factory", "workers", "executor", "AGENTS.md"),
		repoPath(t, "factory", "workers", "reviewer", "AGENTS.md"),
		repoPath(t, "factory", "old", "workers", "worker-a", "AGENTS.md"),
		repoPath(t, "pkg", "cli", "init", "init.go"),
		repoPath(t, "tests", "adhoc", "factory", "README.md"),
		repoPath(t, "tests", "adhoc", "factory", "factory.json"),
		repoPath(t, "tests", "adhoc", "factory", "workers", "processor", "AGENTS.md"),
	}, activeExampleConfigFiles(t)...))

	retiredPatterns := map[string]*regexp.Regexp{
		"work_types":         regexp.MustCompile(`\bwork_types\b`),
		"work_type":          regexp.MustCompile(`\bwork_type\b`),
		"parent_input":       regexp.MustCompile(`\bparent_input\b`),
		"spawned_by":         regexp.MustCompile(`\bspawned_by\b`),
		"on_failure":         regexp.MustCompile(`\bon_failure\b`),
		"on_rejection":       regexp.MustCompile(`\bon_rejection\b`),
		"resource_usage":     regexp.MustCompile(`\bresource_usage\b`),
		"working_directory":  regexp.MustCompile(`\bworking_directory\b`),
		"exhaustion_rules":   regexp.MustCompile(`\bexhaustion_rules\b`),
		"watch_workstation":  regexp.MustCompile(`\bwatch_workstation\b`),
		"max_visits":         regexp.MustCompile(`\bmax_visits\b`),
		"model_provider":     regexp.MustCompile(`\bmodel_provider\b`),
		"worker_provider":    regexp.MustCompile(`(^provider:\s)|(\|\s*` + "`provider`" + `\s*\|)`),
		"stop_token":         regexp.MustCompile(`\bstop_token\b`),
		"session_id":         regexp.MustCompile(`\bsession_id\b`),
		"sessionId":          regexp.MustCompile(`(^sessionId:\s)|(\|\s*` + "`sessionId`" + `\s*\|)`),
		"skip_permissions":   regexp.MustCompile(`\bskip_permissions\b`),
		"worker_concurrency": regexp.MustCompile(`(^concurrency:\s)|(\|\s*` + "`concurrency`" + `\s*\|)`),
		"prompt_file":        regexp.MustCompile(`\bprompt_file\b`),
		"output_schema":      regexp.MustCompile(`\boutput_schema\b`),
		"runtimeStopWords":   regexp.MustCompile(`\bruntimeStopWords\b`),
		"runtime_stop_words": regexp.MustCompile(`\bruntime_stop_words\b`),
		"max_retries":        regexp.MustCompile(`\bmax_retries\b`),
		"max_execution_time": regexp.MustCompile(`\bmax_execution_time\b`),
		"trigger_at_start":   regexp.MustCompile(`\btrigger_at_start\b`),
		"expiry_window":      regexp.MustCompile(`\bexpiry_window\b`),
	}
	exactStringBans := []string{
		"modelProvider: anthropic",
		"modelProvider: openai",
		`"modelProvider":"anthropic"`,
		`"modelProvider": "anthropic"`,
		`"modelProvider":"openai"`,
		`"modelProvider": "openai"`,
		"Worker configuration (model, provider, system prompt)",
	}

	for _, relPath := range targetFiles {
		offenses := docsExampleContractOffenses(t, relPath, retiredPatterns, exactStringBans)
		if len(offenses) > 0 {
			t.Fatalf("active factory-config docs/examples must use canonical camelCase fields:\n%s", strings.Join(offenses, "\n"))
		}
	}
}

func docsExampleContractOffenses(t *testing.T, relPath string, retiredPatterns map[string]*regexp.Regexp, exactStringBans []string) []string {
	t.Helper()

	data, err := os.ReadFile(relPath)
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}

	var offenses []string
	lines := strings.Split(string(data), "\n")
	for lineNumber, line := range lines {
		for field, pattern := range retiredPatterns {
			if pattern.MatchString(line) {
				offenses = append(offenses, filepath.Clean(relPath)+":"+strconv.Itoa(lineNumber+1)+": "+field)
			}
		}
		for _, banned := range exactStringBans {
			if strings.Contains(line, banned) {
				offenses = append(offenses, filepath.Clean(relPath)+":"+strconv.Itoa(lineNumber+1)+": "+banned)
			}
		}
	}

	return offenses
}

func TestFactoryConfigDocsAndExamples_UseExecutionLimitsForWorkstationTimeouts(t *testing.T) {
	t.Parallel()

	targetFiles := existingFiles([]string{
		repoPath(t, "docs", "authoring-agents-md.md"),
		repoPath(t, "docs", "authoring-workflows.md"),
		repoPath(t, "docs", "workstations.md"),
		repoPath(t, "docs", "guides", "workstation-guards-and-guarded-loop-breakers.md"),
	})

	workstationTypePattern := regexp.MustCompile(`(?m)^type:\s*(MODEL_WORKSTATION|LOGICAL_MOVE)\s*$`)
	workstationTimeoutPattern := regexp.MustCompile(`(?m)^timeout:`)

	var offenses []string
	for _, relPath := range targetFiles {
		data, err := os.ReadFile(relPath)
		if err != nil {
			t.Fatalf("read %s: %v", relPath, err)
		}
		for _, section := range strings.Split(string(data), "```yaml") {
			block, _, found := strings.Cut(section, "```")
			if !found {
				continue
			}
			if workstationTypePattern.MatchString(block) && workstationTimeoutPattern.MatchString(block) {
				offenses = append(offenses, filepath.Clean(relPath))
				break
			}
		}
	}

	if len(offenses) > 0 {
		t.Fatalf("active workstation docs/examples must author execution limits under limits.maxExecutionTime, not timeout:\n%s", strings.Join(offenses, "\n"))
	}
}

func TestFactoryConfigDocsAndExamples_UseAlignedRuntimeResourceContract(t *testing.T) {
	t.Parallel()

	targetFiles := existingFiles(append([]string{
		repoPath(t, "docs", "authoring-agents-md.md"),
		repoPath(t, "docs", "authoring-workflows.md"),
		repoPath(t, "docs", "work.md"),
		repoPath(t, "docs", "workstations.md"),
		repoPath(t, "docs", "guides", "workstation-guards-and-guarded-loop-breakers.md"),
		repoPath(t, "README.md"),
		repoPath(t, "pkg", "cli", "init", "init.go"),
		repoPath(t, "tests", "adhoc", "factory", "factory.json"),
	}, activeExampleConfigFiles(t)...))

	retiredPatterns := map[string]*regexp.Regexp{
		"resourceUsage":      regexp.MustCompile(`\bresourceUsage\b`),
		"resource_usage":     regexp.MustCompile(`\bresource_usage\b`),
		"worker string list": regexp.MustCompile(`resources:\s*\["`),
	}

	var offenses []string
	for _, relPath := range targetFiles {
		data, err := os.ReadFile(relPath)
		if err != nil {
			t.Fatalf("read %s: %v", relPath, err)
		}

		lines := strings.Split(string(data), "\n")
		for lineNumber, line := range lines {
			for field, pattern := range retiredPatterns {
				if pattern.MatchString(line) {
					offenses = append(offenses, filepath.Clean(relPath)+":"+strconv.Itoa(lineNumber+1)+": "+field)
				}
			}
		}
	}

	if len(offenses) > 0 {
		t.Fatalf("active runtime-resource docs/examples must use the aligned resources contract:\n%s", strings.Join(offenses, "\n"))
	}
}

func TestFactoryConfigDocsAndExamples_ActiveSplitExamplesShowCanonicalWorkstationRuntimeContract(t *testing.T) {
	t.Parallel()

	targetFiles := existingFiles([]string{
		repoPath(t, "examples", "basic", "factory", "workstations", "process", "AGENTS.md"),
	})

	requiredPatterns := map[string]*regexp.Regexp{
		"limits.maxExecutionTime": regexp.MustCompile(`(?m)^limits:\s*\n(?:[^\n]*\n)*?\s+maxExecutionTime:`),
		"stopWords":               regexp.MustCompile(`(?m)^stopWords:\s*\n`),
	}

	var offenses []string
	for _, relPath := range targetFiles {
		data, err := os.ReadFile(relPath)
		if err != nil {
			t.Fatalf("read %s: %v", relPath, err)
		}

		for field, pattern := range requiredPatterns {
			if !pattern.Match(data) {
				offenses = append(offenses, filepath.Clean(relPath)+": missing "+field)
			}
		}
	}

	if len(offenses) > 0 {
		t.Fatalf("active split examples must show canonical workstation stop words and execution limits:\n%s", strings.Join(offenses, "\n"))
	}
}

func TestFactoryConfigDocsAndExamples_StopWordsExamplesDoNotAdvertiseRejectedRouting(t *testing.T) {
	t.Parallel()

	targetFiles := existingFiles([]string{
		repoPath(t, "docs", "authoring-agents-md.md"),
		repoPath(t, "docs", "workstations.md"),
	})

	var offenses []string
	for _, relPath := range targetFiles {
		data, err := os.ReadFile(relPath)
		if err != nil {
			t.Fatalf("read %s: %v", relPath, err)
		}

		for _, section := range strings.Split(string(data), "```yaml") {
			block, _, found := strings.Cut(section, "```")
			if !found || !strings.Contains(block, "stopWords:") {
				continue
			}
			if strings.Contains(block, "REJECTED") {
				offenses = append(offenses, filepath.Clean(relPath))
				break
			}
		}
	}

	if len(offenses) > 0 {
		t.Fatalf("stopWords workstation docs/examples must not advertise REJECTED routing in the same prompt example:\n%s", strings.Join(offenses, "\n"))
	}
}

func activeExampleConfigFiles(t *testing.T) []string {
	t.Helper()

	roots := []string{
		repoPath(t, "examples"),
		repoPath(t, "factory"),
	}
	var files []string
	for _, root := range roots {
		if _, err := os.Stat(root); err != nil {
			continue
		}
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if filepath.Clean(path) == filepath.Clean(filepath.Join(root, "old")) {
					return filepath.SkipDir
				}
				if filepath.Base(path) == "inputs" {
					return filepath.SkipDir
				}
				return nil
			}
			name := filepath.Base(path)
			if name == "factory.json" || name == "AGENTS.md" {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	return existingFiles(files)
}

func repoPath(t *testing.T, parts ...string) string {
	t.Helper()
	return testpath.MustRepoPathFromCaller(t, 0, parts...)
}

func existingFiles(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			out = append(out, path)
		}
	}
	return out
}
