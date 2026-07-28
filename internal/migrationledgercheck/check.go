package migrationledgercheck

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const (
	DefaultLedgerPath    = "docs/temp/functional-tests-expansion/migration-ledger-inventory.json"
	DefaultChecklistPath = "docs/temp/functional-tests-expansion/test-file-checklist.md"
	FunctionalRoot       = "tests/functional"
	InternalPrefix       = "tests/functional/internal/"
)

var checklistPathPattern = regexp.MustCompile("`(tests/functional/[^`]+_test\\.go)`")

// ScenarioKey uniquely identifies a customer functional scenario.
type ScenarioKey struct {
	SourcePath string
	Scenario   string
}

func (k ScenarioKey) String() string {
	return k.SourcePath + "::" + k.Scenario
}

// LiveScenario is a customer Test* discovered in the live tree.
type LiveScenario struct {
	SourcePath string
	Scenario   string
	Lane       string
}

// Row mirrors one machine-readable ledger inventory row.
type Row struct {
	SourcePath        string `json:"source_path"`
	Package           string `json:"package"`
	Scenario          string `json:"scenario"`
	Lane              string `json:"lane"`
	Destination       string `json:"destination"`
	CatchAll          string `json:"catch_all"`
	SpecialtyTargets  string `json:"specialty_targets"`
	DeletionOnlyBatch string `json:"deletion_only_batch"`
}

// Ledger is the machine-readable migration ledger companion.
type Ledger struct {
	Rows     []Row `json:"rows"`
	Summary  struct {
		CustomerTopLevelTestScenarios int `json:"customer_top_level_Test_scenarios"`
		LaneFunctionallong            int `json:"lane_functionallong"`
		LaneShort                     int `json:"lane_short"`
	} `json:"summary"`
	RuntimeAPIMapping struct {
		BatchIDs []string `json:"batch_ids"`
	} `json:"runtime_api_mapping"`
	GuardsBootstrapReplayMapping struct {
		BatchIDs []string `json:"batch_ids"`
	} `json:"guards_bootstrap_replay_mapping"`
	SmokeWorkflowMapping struct {
		SmokeScenarios    int `json:"smoke_scenarios"`
		WorkflowScenarios int `json:"workflow_scenarios"`
	} `json:"smoke_workflow_mapping"`
}

// RequiredSpecialtyTargets are functional specialty Make targets the ledger must document.
var RequiredSpecialtyTargets = []string{
	"api-smoke",
	"cron-time-work-smoke",
	"pr-inference-approval",
	"long-tests-managed-runtime",
	"long-tests-functional-runtime",
	"artifact-contract-closeout",
	"release-surface-smoke",
	"docs-reference-smoke",
	"current-factory-watcher-switch-smoke",
	"test-built-cli-acceptance",
	"script-timeout-companion-smoke-100",
}

// ExpectedDeletionOnlyBatches is the ordered catch-all retirement batch index.
var ExpectedDeletionOnlyBatches = []string{
	"runtime_api-delete-01-transport-http",
	"runtime_api-delete-02-work-submission",
	"runtime_api-delete-04-sessions",
	"runtime_api-delete-05-factory-packaged",
	"runtime_api-delete-06-factory-current",
	"runtime_api-delete-07-events-replay",
	"runtime_api-delete-08-models",
	"runtime_api-delete-09-workers-resilience",
	"runtime_api-delete-10-observability-product",
	"runtime_api-delete-11-wrong-layer",
	"smoke-delete-03-factory-definitions",
	"smoke-delete-04-transport-cli",
	"smoke-delete-05-factory-packaged",
	"smoke-delete-06-factory-packaged-cross",
	"smoke-delete-07-sessions-controls",
	"smoke-delete-09-guards",
	"smoke-delete-10-workers-mock",
	"smoke-delete-11-resilience-process",
	"smoke-delete-12-product-docs",
	"smoke-delete-13-wrong-layer",
	"workflow-delete-01-orchestration-dispatch",
	"workflow-delete-02-work-routing",
	"workflow-delete-03-work-relationships",
	"workflow-delete-07-factory-current",
	"workflow-delete-08-guards",
	"workflow-delete-09-orchestration-javascript",
	"guards_batch-delete-01-work-relationships",
	"guards_batch-delete-02-guards-global",
	"guards_batch-delete-04-resources-concurrency",
	"guards_batch-delete-05-resources-fairness",
	"guards_batch-delete-06-resilience-batch",
	"bootstrap_portability-delete-02-factory-definitions-validation",
	"bootstrap_portability-delete-03-factory-definitions-import-export",
	"bootstrap_portability-delete-04-portable-config",
	"bootstrap_portability-delete-05-factory-current",
	"replay_contracts-delete-01-events-replay",
	"replay_contracts-delete-02-events-factory-events",
	"replay_contracts-delete-03-events-response-events",
	"replay_contracts-delete-04-work-submission",
	"replay_contracts-delete-06-wrong-layer",
}

// Check validates ledger completeness against the live functional tree.
func Check(repoRoot, ledgerPath, checklistPath string) error {
	ledger, err := LoadLedger(filepath.Join(repoRoot, ledgerPath))
	if err != nil {
		return err
	}
	live, err := ScanLiveScenarios(repoRoot)
	if err != nil {
		return err
	}
	checklist, err := LoadChecklistPaths(filepath.Join(repoRoot, checklistPath))
	if err != nil {
		return err
	}

	problems := reconcileLiveAndLedger(live, ledger, checklist)
	problems = append(problems, checkLaneSummary(ledger)...)
	problems = append(problems, checkDeletionOnlyBatches(ledger)...)
	problems = append(problems, checkSpecialtyTargets(ledger)...)
	if err := CheckPackagedFactoryInvocationMatrix(repoRoot, checklistPath); err != nil {
		problems = append(problems, err.Error())
	}
	if len(problems) > 0 {
		slices.Sort(problems)
		return fmt.Errorf("migration ledger completeness check failed (%d problems):\n- %s", len(problems), strings.Join(problems, "\n- "))
	}
	return nil
}

func reconcileLiveAndLedger(live []LiveScenario, ledger *Ledger, checklist map[string]struct{}) []string {
	var problems []string
	liveByKey := map[ScenarioKey]LiveScenario{}
	for _, scenario := range live {
		key := ScenarioKey{SourcePath: scenario.SourcePath, Scenario: scenario.Scenario}
		liveByKey[key] = scenario
	}

	ledgerByKey := map[ScenarioKey]Row{}
	for _, row := range ledger.Rows {
		key := ScenarioKey{SourcePath: row.SourcePath, Scenario: row.Scenario}
		if _, exists := ledgerByKey[key]; exists {
			problems = append(problems, fmt.Sprintf("duplicate ledger row %s", key))
			continue
		}
		ledgerByKey[key] = row
	}

	for key, scenario := range liveByKey {
		row, ok := ledgerByKey[key]
		if !ok {
			problems = append(problems, fmt.Sprintf("live scenario missing from ledger: %s", key))
			continue
		}
		if row.Destination == "" || row.Destination == "TBD" {
			problems = append(problems, fmt.Sprintf("unmapped destination for %s", key))
		}
		if row.DeletionOnlyBatch == "" || row.DeletionOnlyBatch == "TBD" {
			problems = append(problems, fmt.Sprintf("unmapped deletion_only_batch for %s", key))
		}
		if row.Lane != scenario.Lane {
			problems = append(problems, fmt.Sprintf("lane mismatch for %s: ledger=%q live=%q", key, row.Lane, scenario.Lane))
		}
		if !isValidDestination(row.Destination, checklist) {
			problems = append(problems, fmt.Sprintf("invalid destination for %s: %q", key, row.Destination))
		}
	}

	for key := range ledgerByKey {
		if _, ok := liveByKey[key]; !ok {
			problems = append(problems, fmt.Sprintf("stale ledger row not in live tree: %s", key))
		}
	}
	if len(live) != len(ledger.Rows) {
		problems = append(problems, fmt.Sprintf("row count mismatch: live=%d ledger=%d", len(live), len(ledger.Rows)))
	}
	if len(live) != ledger.Summary.CustomerTopLevelTestScenarios {
		problems = append(problems, fmt.Sprintf("summary customer count mismatch: live=%d summary=%d", len(live), ledger.Summary.CustomerTopLevelTestScenarios))
	}
	return problems
}

func checkLaneSummary(ledger *Ledger) []string {
	var problems []string
	shortCount, longCount := 0, 0
	for _, row := range ledger.Rows {
		switch row.Lane {
		case "short":
			shortCount++
		case "functionallong":
			longCount++
		default:
			problems = append(problems, fmt.Sprintf("invalid lane %q on %s", row.Lane, row.Scenario))
		}
	}
	if shortCount != ledger.Summary.LaneShort {
		problems = append(problems, fmt.Sprintf("summary short lane mismatch: rows=%d summary=%d", shortCount, ledger.Summary.LaneShort))
	}
	if longCount != ledger.Summary.LaneFunctionallong {
		problems = append(problems, fmt.Sprintf("summary long lane mismatch: rows=%d summary=%d", longCount, ledger.Summary.LaneFunctionallong))
	}
	return problems
}

func checkDeletionOnlyBatches(ledger *Ledger) []string {
	var problems []string
	batchCounts := map[string]int{}
	for _, row := range ledger.Rows {
		if row.DeletionOnlyBatch == "" || row.DeletionOnlyBatch == "n/a" {
			continue
		}
		batchCounts[row.DeletionOnlyBatch]++
	}
	for _, batchID := range ExpectedDeletionOnlyBatches {
		if batchCounts[batchID] == 0 {
			problems = append(problems, fmt.Sprintf("deletion-only batch %q has zero ledger rows", batchID))
		}
	}
	for batchID := range batchCounts {
		if !slices.Contains(ExpectedDeletionOnlyBatches, batchID) {
			problems = append(problems, fmt.Sprintf("unexpected deletion-only batch id %q", batchID))
		}
	}
	return problems
}

func checkSpecialtyTargets(ledger *Ledger) []string {
	var problems []string
	for _, row := range ledger.Rows {
		for _, target := range specialtyTargetTokens(row.SpecialtyTargets) {
			if !slices.Contains(RequiredSpecialtyTargets, target) {
				problems = append(problems, fmt.Sprintf("unknown specialty target %q on %s", target, row.Scenario))
			}
		}
	}
	return problems
}

func specialtyTargetTokens(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "none" {
		return nil
	}
	var tokens []string
	for _, part := range strings.Split(trimmed, ",") {
		token := strings.TrimSpace(part)
		if token != "" && token != "none" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

// LoadLedger reads the machine-readable migration ledger companion.
func LoadLedger(path string) (*Ledger, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read ledger %s: %w", path, err)
	}
	var ledger Ledger
	if err := json.Unmarshal(data, &ledger); err != nil {
		return nil, fmt.Errorf("decode ledger %s: %w", path, err)
	}
	if len(ledger.Rows) == 0 {
		return nil, fmt.Errorf("ledger %s has no rows", path)
	}
	return &ledger, nil
}

// LoadChecklistPaths returns checklist destination cells from test-file-checklist.md.
func LoadChecklistPaths(path string) (map[string]struct{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read checklist %s: %w", path, err)
	}
	paths := map[string]struct{}{}
	for _, match := range checklistPathPattern.FindAllStringSubmatch(string(data), -1) {
		paths[match[1]] = struct{}{}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("checklist %s has no destination cells", path)
	}
	return paths, nil
}

// ScanLiveScenarios inventories customer top-level Test* under tests/functional.
func ScanLiveScenarios(repoRoot string) ([]LiveScenario, error) {
	functionalRoot := filepath.Join(repoRoot, filepath.FromSlash(FunctionalRoot))
	var scenarios []LiveScenario

	err := filepath.WalkDir(functionalRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, InternalPrefix) {
			return nil
		}

		fileScenarios, lane, err := scanTestFile(path)
		if err != nil {
			return err
		}
		for _, scenario := range fileScenarios {
			scenarios = append(scenarios, LiveScenario{
				SourcePath: rel,
				Scenario:   scenario,
				Lane:       lane,
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", FunctionalRoot, err)
	}

	slices.SortFunc(scenarios, func(a, b LiveScenario) int {
		if cmp := strings.Compare(a.SourcePath, b.SourcePath); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.Scenario, b.Scenario)
	})
	return scenarios, nil
}

func scanTestFile(path string) ([]string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	lane := detectLane(string(data))

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, data, parser.ParseComments)
	if err != nil {
		return nil, "", fmt.Errorf("parse %s: %w", path, err)
	}

	var scenarios []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || !strings.HasPrefix(fn.Name.Name, "Test") {
			continue
		}
		if fn.Name.Name == "TestMain" {
			continue
		}
		scenarios = append(scenarios, fn.Name.Name)
	}
	return scenarios, lane, nil
}

func detectLane(source string) string {
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "//go:build") && !strings.HasPrefix(trimmed, "// +build") {
			if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
				break
			}
			continue
		}
		if strings.Contains(trimmed, "functionallong") && !strings.Contains(trimmed, "!functionallong") {
			return "functionallong"
		}
	}
	return "short"
}

func isValidDestination(destination string, checklist map[string]struct{}) bool {
	if destination == "" || destination == "TBD" {
		return false
	}
	if strings.HasPrefix(destination, "wrong-layer:") {
		return true
	}
	_, ok := checklist[destination]
	return ok
}
