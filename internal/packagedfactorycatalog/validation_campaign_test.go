package packagedfactorycatalog_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
)

const validationLedgerPath = "docs/internal/packaged-factory-validation/ledger.json"

type validationLedger struct {
	SchemaVersion string                    `json:"schemaVersion"`
	Factories     []validationFactoryRecord `json:"factories"`
}

type validationFactoryRecord struct {
	Name                 string   `json:"name"`
	ContractTestPackage  string   `json:"contractTestPackage"`
	ContractTests        []string `json:"contractTests"`
	CanaryStatus         string   `json:"canaryStatus"`
	RepresentativeStatus string   `json:"representativeStatus"`
	GoalStatus           string   `json:"goalStatus"`
	ExperimentRecords    []string `json:"experimentRecords"`
	Limitations          []string `json:"limitations"`
}

func TestPackagedFactoryValidationLedgerCoversPublishedCatalog(t *testing.T) {
	root := validationRepositoryRoot(t)
	ledger := readValidationLedger(t, root)
	artifacts, err := packagedfactorycatalog.GenerateArtifacts(
		t.Context(), packagedfactories.Source(), "factories", "schemas/factory.schema.json",
	)
	if err != nil {
		t.Fatalf("generate packaged Factory artifacts: %v", err)
	}

	published := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		published = append(published, artifact.PublicName)
	}
	recorded := make([]string, 0, len(ledger.Factories))
	seen := make(map[string]struct{}, len(ledger.Factories))
	for _, record := range ledger.Factories {
		if _, duplicate := seen[record.Name]; duplicate {
			t.Fatalf("validation ledger contains duplicate Factory %q", record.Name)
		}
		seen[record.Name] = struct{}{}
		recorded = append(recorded, record.Name)
		assertValidationEvidence(t, root, record)
	}
	sort.Strings(published)
	sort.Strings(recorded)
	if strings.Join(recorded, "\n") != strings.Join(published, "\n") {
		t.Fatalf("validation ledger inventory = %v, want published catalog %v", recorded, published)
	}
}

func assertValidationEvidence(t *testing.T, root string, record validationFactoryRecord) {
	t.Helper()
	validTrialStatuses := map[string]bool{"NOT_RUN": true, "RUNNING": true, "PASSED": true, "FAILED": true, "INCONCLUSIVE": true}
	validGoalStatuses := map[string]bool{"UNVALIDATED": true, "RUNNING": true, "NEEDS_ITERATION": true, "BLOCKED": true, "MEETS_EXPECTATIONS": true}
	if !validTrialStatuses[record.CanaryStatus] || !validTrialStatuses[record.RepresentativeStatus] {
		t.Fatalf("validation ledger Factory %q has invalid trial statuses %q/%q", record.Name, record.CanaryStatus, record.RepresentativeStatus)
	}
	if !validGoalStatuses[record.GoalStatus] {
		t.Fatalf("validation ledger Factory %q has invalid goal status %q", record.Name, record.GoalStatus)
	}
	testRoot := filepath.Join(root, filepath.FromSlash(record.ContractTestPackage))
	testPayload := readGoTestPackage(t, testRoot)
	for _, testName := range record.ContractTests {
		if !strings.Contains(testPayload, "func "+testName+"(") {
			t.Fatalf("validation ledger Factory %q references missing contract test %q in %s", record.Name, testName, record.ContractTestPackage)
		}
	}
	for _, evidencePath := range record.ExperimentRecords {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(evidencePath))); err != nil {
			t.Fatalf("validation ledger Factory %q experiment record %q: %v", record.Name, evidencePath, err)
		}
	}
	if record.GoalStatus == "MEETS_EXPECTATIONS" &&
		(record.CanaryStatus != "PASSED" || record.RepresentativeStatus != "PASSED" || len(record.ExperimentRecords) == 0) {
		t.Fatalf("validation ledger Factory %q claims MEETS_EXPECTATIONS without passed live canary, representative workload, and experiment records", record.Name)
	}
	if record.GoalStatus == "NEEDS_ITERATION" && len(record.Limitations) == 0 {
		t.Fatalf("validation ledger Factory %q needs iteration without a documented limitation", record.Name)
	}
}

func readValidationLedger(t *testing.T, root string) validationLedger {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(validationLedgerPath)))
	if err != nil {
		t.Fatalf("read validation ledger: %v", err)
	}
	var ledger validationLedger
	if err := json.Unmarshal(payload, &ledger); err != nil {
		t.Fatalf("decode validation ledger: %v", err)
	}
	if ledger.SchemaVersion != "1" {
		t.Fatalf("validation ledger schemaVersion = %q, want 1", ledger.SchemaVersion)
	}
	return ledger
}

func readGoTestPackage(t *testing.T, directory string) string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read contract test package %s: %v", directory, err)
	}
	var combined strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		payload, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatalf("read contract test %s: %v", entry.Name(), err)
		}
		combined.Write(payload)
	}
	return combined.String()
}

func validationRepositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}
