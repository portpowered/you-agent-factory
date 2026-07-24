// Command ownershipboundarycheck enforces behavior ownership around the
// application initializer, platform adapters, and transport mappings.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	modulePath       = "github.com/portpowered/infinite-you"
	baselineFile     = "ownership-boundary-baseline.json"
	baselineVersion  = 1
	baselineStage    = "wire-injection-full-blow"
	initializerRoot  = "pkg/initializer"
	platformRoot     = "pkg/platform"
	mappingRoot      = "pkg/transports/mapping"
	servicesImport   = modulePath + "/pkg/services/"
	transportsImport = modulePath + "/pkg/transports/"
)

const (
	ruleInitializerServiceImplementation = "initializer-service-implementation-import"
	ruleInitializerTransport             = "initializer-transport-import"
	ruleInitializerConstruction          = "initializer-product-construction"
	rulePlatformDomainImport             = "platform-domain-service-import"
	rulePeerServiceImplementation        = "peer-service-implementation-import"
	ruleMappingFilesystem                = "mapping-filesystem-behavior"
	ruleMappingProcess                   = "mapping-process-behavior"
	ruleMappingTimer                     = "mapping-timer-behavior"
	ruleMappingGoroutine                 = "mapping-goroutine"
)

var deletionGates = map[string]string{
	ruleInitializerServiceImplementation: "replace the implementation import with an already-constructed service-root operation",
	ruleInitializerTransport:             "inject an already-constructed transport operation and remove the transport import from Initializer",
	ruleInitializerConstruction:          "construct the product dependency in pkg/wire and inject its exact operation into Initializer",
	rulePlatformDomainImport:             "move domain policy to its owning service and leave Platform implementing an exact external-effect port",
	rulePeerServiceImplementation:        "import only the peer service root contract (pkg/services/<peer>) instead of a peer implementation or nested subservice",
	ruleMappingFilesystem:                "move filesystem behavior behind an injected service or exact external-effect port",
	ruleMappingProcess:                   "move process discovery or execution behind an injected service-owned port",
	ruleMappingTimer:                     "move timer lifecycle behind an injected clock or owning service",
	ruleMappingGoroutine:                 "move asynchronous lifecycle to an owning service or Initializer lifecycle component",
}

// These packages declare exact external-effect contracts at the leaf that
// performs the effect. Platform may implement those ports but may not depend on
// a composed service, service root, or implementation package.
var approvedPlatformLeafPorts = map[string]struct{}{
	modulePath + "/pkg/services/workers/provider/inferencecontract": {},
}

// approvedPlatformAdapterPorts records narrow source-to-owner exceptions for a
// Platform adapter that implements a port declared at a service root. Do not
// add a service prefix here: each adapter and owner root must be exact.
var approvedPlatformAdapterPorts = map[string]map[string]struct{}{
	"pkg/platform/contentstaging": {
		modulePath + "/pkg/services/work": {},
	},
}

var mappingOSCalls = map[string]struct{}{
	"Chmod": {}, "Chown": {}, "Chtimes": {}, "Create": {}, "CreateTemp": {},
	"FindProcess": {}, "Lchown": {}, "Link": {}, "Mkdir": {}, "MkdirAll": {},
	"MkdirTemp": {}, "Open": {}, "OpenFile": {}, "ReadDir": {}, "ReadFile": {},
	"Lstat": {}, "Stat": {}, "Remove": {}, "RemoveAll": {},
	"Rename": {}, "StartProcess": {}, "Symlink": {}, "Truncate": {},
	"WriteFile": {},
}

var mappingTimerCalls = map[string]struct{}{
	"After": {}, "AfterFunc": {}, "NewTicker": {}, "NewTimer": {},
	"Sleep": {}, "Tick": {},
}

type config struct {
	root           string
	createBaseline bool
}

type finding struct {
	Rule     string `json:"rule"`
	FilePath string `json:"filePath"`
	Target   string `json:"target"`
	Line     int    `json:"-"`
}

type baseline struct {
	Version int             `json:"version"`
	Entries []baselineEntry `json:"entries"`
}

type baselineEntry struct {
	Rule         string `json:"rule"`
	FilePath     string `json:"filePath"`
	Target       string `json:"target"`
	Stage        string `json:"stage"`
	DeletionGate string `json:"deletionGate"`
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.root, "root", ".", "repository root to scan")
	flag.BoolVar(
		&cfg.createBaseline,
		"create-baseline",
		false,
		"create the deletion-only ownership baseline; refuses to overwrite an existing file",
	)
	flag.Parse()
	if err := run(cfg, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cfg config, stdout, stderr io.Writer) error {
	repoRoot, err := filepath.Abs(cfg.root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	findings, err := scan(repoRoot)
	if err != nil {
		return err
	}
	if cfg.createBaseline {
		return createBaseline(repoRoot, findings, stdout)
	}
	recorded, err := loadBaseline(repoRoot)
	if err != nil {
		return err
	}
	active, unrecorded, stale, err := partition(findings, recorded)
	if err != nil {
		return err
	}
	for _, item := range unrecorded {
		writeFinding(stderr, "new violation", item)
	}
	for _, item := range stale {
		fmt.Fprintf(
			stderr,
			"[agent-factory:ownership-boundary] stale baseline: %s %s -> %s; remove this entry from %s\n",
			item.Rule,
			item.FilePath,
			item.Target,
			baselineFile,
		)
	}
	if len(unrecorded) > 0 || len(stale) > 0 {
		return fmt.Errorf(
			"[agent-factory:ownership-boundary] found %d new violation(s) and %d stale baseline entries",
			len(unrecorded),
			len(stale),
		)
	}
	fmt.Fprintf(
		stdout,
		"[agent-factory:ownership-boundary] ownership boundaries hold (%d deletion-only baseline entries remain)\n",
		len(active),
	)
	return nil
}

func scan(repoRoot string) ([]finding, error) {
	inventory, err := loadOwnerInventory(repoRoot)
	if err != nil {
		return nil, err
	}
	var findings []finding
	for _, scanRoot := range []string{initializerRoot, platformRoot, mappingRoot} {
		path := filepath.Join(repoRoot, filepath.FromSlash(scanRoot))
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("inspect %s: %w", scanRoot, err)
		}
		err := filepath.WalkDir(path, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if entry.Name() == "testdata" || entry.Name() == "vendor" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			relative, err := filepath.Rel(repoRoot, path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			fileFindings, err := scanFile(path, relative, inventory)
			if err != nil {
				return err
			}
			findings = append(findings, fileFindings...)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan %s: %w", scanRoot, err)
		}
	}
	peerFindings, err := scanPeerServiceImports(repoRoot, inventory)
	if err != nil {
		return nil, fmt.Errorf("scan peer service imports: %w", err)
	}
	findings = append(findings, peerFindings...)
	findings = uniqueFindings(findings)
	sort.Slice(findings, func(i, j int) bool { return findingKey(findings[i]) < findingKey(findings[j]) })
	return findings, nil
}

func scanFile(path, relative string, inventory ownerInventory) ([]finding, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", relative, err)
	}
	if ast.IsGenerated(file) {
		return nil, nil
	}
	aliases, imports := importInventory(file)
	var findings []finding
	switch {
	case within(relative, initializerRoot):
		findings = append(findings, scanInitializerImports(fileSet, imports, relative, inventory)...)
		findings = append(findings, scanInitializerCalls(fileSet, file, aliases, relative)...)
	case within(relative, platformRoot):
		findings = append(findings, scanPlatformImports(fileSet, imports, relative)...)
	case within(relative, mappingRoot):
		findings = append(findings, scanMappingBehavior(fileSet, file, aliases, imports, relative)...)
	}
	return findings, nil
}

type importedPackage struct {
	Path string
	Pos  token.Pos
	Name string
}

func importInventory(file *ast.File) (map[string]string, []importedPackage) {
	aliases := make(map[string]string)
	imports := make([]importedPackage, 0, len(file.Imports))
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := filepath.Base(path)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		imports = append(imports, importedPackage{Path: path, Pos: spec.Pos(), Name: name})
		if name != "." && name != "_" {
			aliases[name] = path
		}
	}
	return aliases, imports
}

func scanInitializerImports(
	fileSet *token.FileSet,
	imports []importedPackage,
	relative string,
	inventory ownerInventory,
) []finding {
	var findings []finding
	for _, imported := range imports {
		rule := ""
		switch {
		case strings.HasPrefix(imported.Path, transportsImport):
			rule = ruleInitializerTransport
		case serviceImplementationImport(imported.Path, inventory):
			rule = ruleInitializerServiceImplementation
		}
		if rule != "" {
			findings = append(findings, finding{
				Rule: rule, FilePath: relative, Target: imported.Path,
				Line: fileSet.Position(imported.Pos).Line,
			})
		}
		if imported.Name == "." &&
			(strings.HasPrefix(imported.Path, servicesImport) ||
				strings.HasPrefix(imported.Path, transportsImport)) {
			findings = append(findings, finding{
				Rule: ruleInitializerConstruction, FilePath: relative,
				Target: imported.Path + ".*",
				Line:   fileSet.Position(imported.Pos).Line,
			})
		}
	}
	return findings
}

func serviceImplementationImport(importPath string, inventory ownerInventory) bool {
	if !strings.HasPrefix(importPath, servicesImport) {
		return false
	}
	repositoryPath := "pkg/services/" + strings.TrimPrefix(importPath, servicesImport)
	_, surface := inventory.classify(repositoryPath)
	switch surface {
	case surfaceNonRoot:
		return true
	case surfaceRoot:
		return false
	default:
		// Fixtures without a committed service tree keep the path-shape rule so
		// existing initializer implementation findings remain actionable.
		remainder := strings.TrimPrefix(importPath, servicesImport)
		return strings.Contains(remainder, "/")
	}
}

func scanInitializerCalls(
	fileSet *token.FileSet,
	file *ast.File,
	aliases map[string]string,
	relative string,
) []finding {
	var findings []finding
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		qualifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		importPath, ok := aliases[qualifier.Name]
		if !ok || (!strings.HasPrefix(importPath, servicesImport) &&
			!strings.HasPrefix(importPath, transportsImport)) {
			return true
		}
		if !constructionName(selector.Sel.Name) {
			return true
		}
		findings = append(findings, finding{
			Rule: ruleInitializerConstruction, FilePath: relative,
			Target: importPath + "." + selector.Sel.Name,
			Line:   fileSet.Position(call.Pos()).Line,
		})
		return true
	})
	return findings
}

func constructionName(name string) bool {
	for _, prefix := range []string{"Build", "Create", "Inject", "New", "Open", "Provide"} {
		if strings.HasPrefix(name, prefix) && len(name) > len(prefix) {
			return true
		}
	}
	return false
}

func scanPlatformImports(fileSet *token.FileSet, imports []importedPackage, relative string) []finding {
	var findings []finding
	for _, imported := range imports {
		if !strings.HasPrefix(imported.Path, servicesImport) {
			continue
		}
		if _, approved := approvedPlatformLeafPorts[imported.Path]; approved {
			continue
		}
		sourcePackage := filepath.ToSlash(filepath.Dir(relative))
		if targets := approvedPlatformAdapterPorts[sourcePackage]; targets != nil {
			if _, approved := targets[imported.Path]; approved {
				continue
			}
		}
		findings = append(findings, finding{
			Rule: rulePlatformDomainImport, FilePath: relative, Target: imported.Path,
			Line: fileSet.Position(imported.Pos).Line,
		})
	}
	return findings
}

func scanMappingBehavior(
	fileSet *token.FileSet,
	file *ast.File,
	aliases map[string]string,
	imports []importedPackage,
	relative string,
) []finding {
	var findings []finding
	for _, imported := range imports {
		if imported.Path == "os/exec" {
			findings = append(findings, finding{
				Rule: ruleMappingProcess, FilePath: relative, Target: "os/exec",
				Line: fileSet.Position(imported.Pos).Line,
			})
		}
		if imported.Name != "." {
			continue
		}
		rule := ""
		switch imported.Path {
		case "os":
			rule = ruleMappingFilesystem
		case "time":
			rule = ruleMappingTimer
		}
		if rule != "" {
			findings = append(findings, finding{
				Rule: rule, FilePath: relative, Target: imported.Path + ".*",
				Line: fileSet.Position(imported.Pos).Line,
			})
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		if statement, ok := node.(*ast.GoStmt); ok {
			findings = append(findings, finding{
				Rule: ruleMappingGoroutine, FilePath: relative, Target: "go statement",
				Line: fileSet.Position(statement.Pos()).Line,
			})
			return true
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		qualifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		importPath, ok := aliases[qualifier.Name]
		if !ok {
			return true
		}
		rule := ""
		switch importPath {
		case "os":
			if _, prohibited := mappingOSCalls[selector.Sel.Name]; prohibited {
				rule = ruleMappingFilesystem
			}
		case "os/exec":
			rule = ruleMappingProcess
		case "time":
			if _, prohibited := mappingTimerCalls[selector.Sel.Name]; prohibited {
				rule = ruleMappingTimer
			}
		}
		if rule != "" {
			findings = append(findings, finding{
				Rule: rule, FilePath: relative,
				Target: importPath + "." + selector.Sel.Name,
				Line:   fileSet.Position(call.Pos()).Line,
			})
		}
		return true
	})
	return findings
}

func within(relative, root string) bool {
	return relative == root || strings.HasPrefix(relative, root+"/")
}

func uniqueFindings(items []finding) []finding {
	byKey := make(map[string]finding, len(items))
	for _, item := range items {
		key := findingKey(item)
		current, exists := byKey[key]
		if !exists || item.Line < current.Line {
			byKey[key] = item
		}
	}
	result := make([]finding, 0, len(byKey))
	for _, item := range byKey {
		result = append(result, item)
	}
	return result
}

func findingKey(item finding) string {
	return item.Rule + "\x00" + item.FilePath + "\x00" + item.Target
}

func entryKey(item baselineEntry) string {
	return item.Rule + "\x00" + item.FilePath + "\x00" + item.Target
}

func createBaseline(repoRoot string, findings []finding, stdout io.Writer) error {
	if len(findings) == 0 {
		return fmt.Errorf("refusing to create empty %s; absence records zero debt", baselineFile)
	}
	path := filepath.Join(repoRoot, baselineFile)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("refusing to overwrite existing %s", baselineFile)
		}
		return fmt.Errorf("create %s: %w", baselineFile, err)
	}
	entries := make([]baselineEntry, 0, len(findings))
	for _, item := range findings {
		entries = append(entries, baselineEntry{
			Rule: item.Rule, FilePath: item.FilePath, Target: item.Target,
			Stage: baselineStage, DeletionGate: deletionGates[item.Rule],
		})
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(baseline{Version: baselineVersion, Entries: entries})
	closeErr := file.Close()
	if encodeErr != nil {
		return fmt.Errorf("encode %s: %w", baselineFile, encodeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", baselineFile, closeErr)
	}
	fmt.Fprintf(stdout, "[agent-factory:ownership-boundary] created %s with %d entries\n", baselineFile, len(entries))
	return nil
}

func loadBaseline(repoRoot string) (baseline, error) {
	path := filepath.Join(repoRoot, baselineFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return baseline{}, nil
	}
	if err != nil {
		return baseline{}, fmt.Errorf("read %s: %w", baselineFile, err)
	}
	var result baseline
	if err := json.Unmarshal(data, &result); err != nil {
		return baseline{}, fmt.Errorf("decode %s: %w", baselineFile, err)
	}
	if result.Version != baselineVersion {
		return baseline{}, fmt.Errorf("%s version = %d, want %d", baselineFile, result.Version, baselineVersion)
	}
	if len(result.Entries) == 0 {
		return baseline{}, fmt.Errorf("%s is empty; delete the file to record zero debt", baselineFile)
	}
	return result, nil
}

func partition(
	findings []finding,
	recorded baseline,
) (active []finding, unrecorded []finding, stale []baselineEntry, err error) {
	entries := make(map[string]baselineEntry, len(recorded.Entries))
	for _, item := range recorded.Entries {
		if err := validateEntry(item); err != nil {
			return nil, nil, nil, err
		}
		key := entryKey(item)
		if _, duplicate := entries[key]; duplicate {
			return nil, nil, nil, fmt.Errorf("%s contains duplicate entry for %s", baselineFile, item.Target)
		}
		entries[key] = item
	}
	seen := make(map[string]struct{}, len(findings))
	for _, item := range findings {
		key := findingKey(item)
		seen[key] = struct{}{}
		if _, exists := entries[key]; exists {
			active = append(active, item)
		} else {
			unrecorded = append(unrecorded, item)
		}
	}
	for key, item := range entries {
		if _, exists := seen[key]; !exists {
			stale = append(stale, item)
		}
	}
	sort.Slice(stale, func(i, j int) bool { return entryKey(stale[i]) < entryKey(stale[j]) })
	return active, unrecorded, stale, nil
}

func validateEntry(item baselineEntry) error {
	if item.Rule == "" || item.FilePath == "" || item.Target == "" {
		return fmt.Errorf("%s contains an entry with a missing rule, filePath, or target", baselineFile)
	}
	gate, known := deletionGates[item.Rule]
	if !known {
		return fmt.Errorf("%s contains unknown rule %q", baselineFile, item.Rule)
	}
	if item.Stage != baselineStage {
		return fmt.Errorf("%s entry %s has stage %q, want %q", baselineFile, item.Target, item.Stage, baselineStage)
	}
	if item.DeletionGate != gate {
		return fmt.Errorf("%s entry %s has an invalid deletion gate", baselineFile, item.Target)
	}
	return nil
}

func writeFinding(writer io.Writer, label string, item finding) {
	remediation := deletionGates[item.Rule]
	if item.Rule == rulePeerServiceImplementation {
		remediation = peerViolationMessage(item)
	}
	fmt.Fprintf(
		writer,
		"[agent-factory:ownership-boundary] %s: %s:%d %s -> %s; %s\n",
		label,
		item.FilePath,
		item.Line,
		item.Rule,
		item.Target,
		remediation,
	)
}
