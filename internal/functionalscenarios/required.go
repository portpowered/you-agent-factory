package functionalscenarios

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	RequiredManifestFormatVersion = "required-functional-scenarios/v1"
	canonicalManifestPath         = "contracts/functional-scenarios.json"

	ViolationMissingScenario       = "missing-scenario"
	ViolationMissingTest           = "missing-test"
	ViolationRenamedTest           = "renamed-test"
	ViolationSkippedTest           = "skipped-test"
	ViolationLongOnly              = "long-only"
	ViolationInvalidClassification = "invalid-classification"
	ViolationInvalidDisposition    = "invalid-disposition"
)

// RequiredManifest records both the reviewed minimum that must remain runnable
// and the narrow set of reviewed SSE surfaces that are explicitly non-required.
type RequiredManifest struct {
	FormatVersion        string                   `json:"formatVersion"`
	Scenarios            []RequiredScenario       `json:"scenarios"`
	NonRequiredScenarios []NonRequiredDisposition `json:"nonRequiredScenarios,omitempty"`
}

// NonRequiredDisposition records a reviewed exception to required short-lane
// coverage. Only known public SSE surfaces with an explicit product decision
// may appear here; required streams cannot be waived through this list.
type NonRequiredDisposition struct {
	StableID       string `json:"stableId"`
	Interface      string `json:"interface"`
	Disposition    string `json:"disposition"`
	ReviewedReason string `json:"reviewedReason"`
}

// RequiredScenario binds one stable public scenario to its required test and
// qualifying lane/classification. These fields are deliberately explicit so a
// coverage requirement cannot be weakened by moving only the test binding.
type RequiredScenario struct {
	StableID         string `json:"stableId"`
	Test             string `json:"test"`
	Interface        string `json:"interface"`
	Lane             string `json:"lane"`
	ExecutionClass   string `json:"executionClass"`
	CustomerBoundary bool   `json:"customerBoundary"`
}

// RequiredViolation is a stable, actionable required-lane diagnostic.
type RequiredViolation struct {
	StableID string
	Category string
	Detail   string
}

func (violation RequiredViolation) Error() string {
	return fmt.Sprintf(
		"required functional scenario %q [%s]: %s; restore or correct the reviewed required-scenario manifest and short customer-boundary test",
		violation.StableID,
		violation.Category,
		violation.Detail,
	)
}

// DecodeRequiredManifest decodes a reviewed required-lane contract.
func DecodeRequiredManifest(data []byte) (*RequiredManifest, error) {
	manifest := &RequiredManifest{}
	if err := json.Unmarshal(data, manifest); err != nil {
		return nil, fmt.Errorf("decode required functional scenario manifest: %w", err)
	}
	return manifest, nil
}

// CheckRequiredScenarios verifies every reviewed requirement against the exact
// repository-owned Go test it names. It performs no test execution or live IO.
func CheckRequiredScenarios(repositoryRoot string, manifest *RequiredManifest) error {
	if manifest == nil {
		return fmt.Errorf("check required functional scenarios: manifest is nil")
	}
	if manifest.FormatVersion != RequiredManifestFormatVersion {
		return fmt.Errorf("check required functional scenarios: unknown formatVersion %q", manifest.FormatVersion)
	}
	seen := make(map[string]bool, len(manifest.Scenarios)+len(manifest.NonRequiredScenarios))
	required := make(map[string]bool, len(manifest.Scenarios))
	for _, scenario := range manifest.Scenarios {
		if seen[scenario.StableID] {
			return requiredViolation(scenario, ViolationInvalidClassification, "stable ID is declared more than once")
		}
		seen[scenario.StableID] = true
		required[scenario.StableID] = true
		if err := checkRequiredClassification(scenario); err != nil {
			return err
		}
		if err := checkRequiredTest(repositoryRoot, scenario); err != nil {
			return err
		}
	}
	for _, disposition := range manifest.NonRequiredScenarios {
		if seen[disposition.StableID] {
			return nonRequiredViolation(disposition, "stable ID is also declared as a required scenario or non-required disposition")
		}
		seen[disposition.StableID] = true
		if err := checkNonRequiredDisposition(disposition); err != nil {
			return err
		}
	}
	return checkCanonicalRequiredScenarios(repositoryRoot, required)
}

func checkCanonicalRequiredScenarios(repositoryRoot string, required map[string]bool) error {
	filename := filepath.Join(repositoryRoot, filepath.FromSlash(canonicalManifestPath))
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read canonical functional scenario manifest %s: %w", filename, err)
	}
	manifest, err := DecodeManifest(data)
	if err != nil {
		return err
	}
	if err := ValidateManifest(manifest); err != nil {
		return err
	}
	for _, scenario := range manifest.Scenarios {
		if scenario.SSE == nil || !scenario.SSE.Required || required[scenario.StableID] {
			continue
		}
		return RequiredViolation{
			StableID: scenario.StableID,
			Category: ViolationMissingScenario,
			Detail:   "canonical reviewed scenario is required but has no required short-lane entry",
		}
	}
	return nil
}

func checkNonRequiredDisposition(disposition NonRequiredDisposition) error {
	wantDisposition := ""
	switch disposition.StableID {
	case globalEventsStableID:
		wantDisposition = SSEDeprecatedLaterRemoval
	case responseEventsStableID:
		wantDisposition = SSECurrentlyDeferred
	default:
		return nonRequiredViolation(disposition, "stable ID is not a reviewed non-required SSE surface")
	}
	if disposition.Interface != InterfaceSSE || disposition.Disposition != wantDisposition || strings.TrimSpace(disposition.ReviewedReason) == "" {
		return nonRequiredViolation(
			disposition,
			fmt.Sprintf("interface=%q disposition=%q reviewedReasonPresent=%t; want interface=%q disposition=%q and a reviewed reason", disposition.Interface, disposition.Disposition, strings.TrimSpace(disposition.ReviewedReason) != "", InterfaceSSE, wantDisposition),
		)
	}
	return nil
}

func checkRequiredClassification(scenario RequiredScenario) error {
	if scenario.Lane != LaneShort {
		return requiredViolation(scenario, ViolationLongOnly, fmt.Sprintf("lane is %q, want %q", scenario.Lane, LaneShort))
	}
	if !slices.Contains([]string{InterfaceCLI, InterfaceREST, InterfaceMCP, InterfaceSSE}, scenario.Interface) ||
		!strings.HasPrefix(scenario.StableID, scenario.Interface+"/") ||
		scenario.ExecutionClass != ExecutionDeterministic ||
		!scenario.CustomerBoundary {
		return requiredViolation(
			scenario,
			ViolationInvalidClassification,
			fmt.Sprintf("interface=%q executionClass=%q customerBoundary=%t; want a matching CLI, REST, MCP, or SSE interface with executionClass=%q and customerBoundary=true", scenario.Interface, scenario.ExecutionClass, scenario.CustomerBoundary, ExecutionDeterministic),
		)
	}
	return nil
}

func checkRequiredTest(repositoryRoot string, scenario RequiredScenario) error {
	testPath, testName, found := strings.Cut(scenario.Test, "::")
	if !found || testPath == "" || testName == "" {
		return requiredViolation(scenario, ViolationRenamedTest, fmt.Sprintf("test identity %q must use path::TestName", scenario.Test))
	}
	normalizedPath := filepath.ToSlash(filepath.Clean(testPath))
	if filepath.IsAbs(testPath) || !strings.HasPrefix(normalizedPath, "tests/functional/") {
		return requiredViolation(scenario, ViolationInvalidClassification, fmt.Sprintf("test %q must be under tests/functional", scenario.Test))
	}
	if strings.HasSuffix(normalizedPath, "_long_test.go") {
		return requiredViolation(scenario, ViolationLongOnly, fmt.Sprintf("test %q is stored in a long-lane file", scenario.Test))
	}

	filename := filepath.Join(repositoryRoot, filepath.FromSlash(normalizedPath))
	parsed, err := parser.ParseFile(token.NewFileSet(), filename, nil, parser.ParseComments)
	if err != nil {
		if os.IsNotExist(err) {
			return requiredViolation(scenario, ViolationMissingTest, fmt.Sprintf("test file %q does not exist", normalizedPath))
		}
		return fmt.Errorf("parse required functional test %s: %w", normalizedPath, err)
	}
	if hasFunctionalLongBuildConstraint(parsed) {
		return requiredViolation(scenario, ViolationLongOnly, fmt.Sprintf("test %q is guarded by the functionallong build tag", scenario.Test))
	}

	function := findRequiredTest(parsed, testName)
	if function == nil {
		return requiredViolation(scenario, ViolationRenamedTest, fmt.Sprintf("test symbol %q does not exist in %q", testName, normalizedPath))
	}
	if testIsSkipped(function) {
		return requiredViolation(scenario, ViolationSkippedTest, fmt.Sprintf("test %q contains a skip path and cannot guarantee required short-lane execution", scenario.Test))
	}
	return nil
}

func findRequiredTest(file *ast.File, name string) *ast.FuncDecl {
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == name && hasGoTestSignature(function) {
			return function
		}
	}
	return nil
}

func testIsSkipped(function *ast.FuncDecl) bool {
	skipped := false
	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if ok && (selector.Sel.Name == "Skip" || selector.Sel.Name == "Skipf" || selector.Sel.Name == "SkipNow" || selector.Sel.Name == "SkipLongFunctional") {
			skipped = true
			return false
		}
		return true
	})
	return skipped
}

func hasFunctionalLongBuildConstraint(file *ast.File) bool {
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if (strings.HasPrefix(comment.Text, "//go:build") || strings.HasPrefix(comment.Text, "// +build")) && strings.Contains(comment.Text, "functionallong") {
				return true
			}
		}
	}
	return false
}

func requiredViolation(scenario RequiredScenario, category, detail string) error {
	return RequiredViolation{StableID: scenario.StableID, Category: category, Detail: detail}
}

func nonRequiredViolation(disposition NonRequiredDisposition, detail string) error {
	return fmt.Errorf(
		"non-required functional scenario %q [%s]: %s; record only the reviewed deprecated or deferred SSE disposition, or move the scenario into the required short customer-boundary list",
		disposition.StableID,
		ViolationInvalidDisposition,
		detail,
	)
}
