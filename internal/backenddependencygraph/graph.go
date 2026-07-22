package backenddependencygraph

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// Package is the portion of go list metadata needed to build the graph.
type Package struct {
	ImportPath   string
	Imports      []string
	TestImports  []string
	XTestImports []string
	Module       *Module
}

// Module identifies the module that owns a package.
type Module struct {
	Path string
	Main bool
}

type packageGroup struct {
	id       string
	label    string
	color    string
	bgColor  string
	packages []Package
}

const (
	defaultNodeColor       = "#e2e8f0"
	commandNodeColor       = "#fef3c7"
	rootNodeColor          = "#c4b5fd"
	defaultEdgeColor       = "#64748b"
	serviceRootEdgeColor   = "#16a34a"
	wireEdgeColor          = "#2563eb"
	boundaryViolationColor = "#dc2626"
	testEdgeColor          = "#94a3b8"
)

// Load returns import relationships for packages below cmd, pkg, and tests.
func Load(ctx context.Context, root, goBinary string) ([]Package, string, error) {
	packages, modulePath, err := listPackages(ctx, root, goBinary, "cmd/pkg", false, "./cmd/...", "./pkg/...")
	if err != nil {
		return nil, "", err
	}
	testPackages, _, err := listPackages(ctx, root, goBinary, "tests", true, "./tests/...")
	if err != nil {
		return nil, "", err
	}
	if modulePath == "" {
		return nil, "", fmt.Errorf("go list cmd/pkg packages did not identify the main module")
	}
	return append(packages, testPackages...), modulePath, nil
}

func listPackages(ctx context.Context, root, goBinary, scope string, tolerateErrors bool, patterns ...string) ([]Package, string, error) {
	arguments := []string{"list"}
	if tolerateErrors {
		arguments = append(arguments, "-e")
	}
	arguments = append(arguments, "-json")
	arguments = append(arguments, patterns...)
	command := exec.CommandContext(ctx, goBinary, arguments...)
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return nil, "", fmt.Errorf("go list %s packages: %w: %s", scope, err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, "", fmt.Errorf("go list %s packages: %w", scope, err)
	}

	decoder := json.NewDecoder(bytes.NewReader(output))
	packages := make([]Package, 0)
	modulePath := ""
	for {
		var pkg Package
		if err := decoder.Decode(&pkg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, "", fmt.Errorf("decode go list %s output: %w", scope, err)
		}
		packages = append(packages, pkg)
		if pkg.Module != nil && pkg.Module.Main {
			modulePath = pkg.Module.Path
		}
	}
	return packages, modulePath, nil
}

// RenderDOT renders a deterministic graph containing only the supplied packages.
func RenderDOT(packages []Package, modulePath string) []byte {
	packages = slices.Clone(packages)
	slices.SortFunc(packages, func(left, right Package) int {
		return strings.Compare(left.ImportPath, right.ImportPath)
	})

	known := make(map[string]struct{}, len(packages))
	for _, pkg := range packages {
		known[pkg.ImportPath] = struct{}{}
	}

	var output strings.Builder
	output.WriteString("digraph backend_dependencies {\n")
	output.WriteString("  graph [rankdir=LR, overlap=false, splines=true, bgcolor=\"transparent\"];\n")
	output.WriteString("  node [shape=box, style=\"rounded,filled\", fontname=\"Helvetica\", fontsize=10];\n")
	fmt.Fprintf(&output, "  edge [color=%q, arrowsize=0.6];\n\n", defaultEdgeColor)

	for _, group := range packageGroups(packages, modulePath) {
		fmt.Fprintf(&output, "  subgraph cluster_%s {\n", group.id)
		fmt.Fprintf(&output, "    label=%q; color=%q; bgcolor=%q; style=\"rounded\"; penwidth=1.5;\n", group.label, group.color, group.bgColor)
		for _, pkg := range group.packages {
			label := strings.TrimPrefix(pkg.ImportPath, modulePath+"/")
			fmt.Fprintf(&output, "    %q [label=%q, fillcolor=%q];\n", pkg.ImportPath, label, nodeFillColor(label))
		}
		output.WriteString("  }\n\n")
	}

	for _, pkg := range packages {
		productionImports, testOnlyImports := packageImportsByKind(pkg)
		for _, imported := range productionImports {
			if _, ok := known[imported]; !ok {
				continue
			}
			sourceLabel := strings.TrimPrefix(pkg.ImportPath, modulePath+"/")
			importedLabel := strings.TrimPrefix(imported, modulePath+"/")
			if attributes := edgeAttributes(sourceLabel, importedLabel); attributes != "" {
				fmt.Fprintf(&output, "  %q -> %q %s;\n", pkg.ImportPath, imported, attributes)
			} else {
				fmt.Fprintf(&output, "  %q -> %q;\n", pkg.ImportPath, imported)
			}
		}
		for _, imported := range testOnlyImports {
			if _, ok := known[imported]; !ok {
				continue
			}
			fmt.Fprintf(
				&output,
				"  %q -> %q [color=%q, style=\"dashed\", penwidth=1.0];\n",
				pkg.ImportPath,
				imported,
				testEdgeColor,
			)
		}
	}
	output.WriteString("}\n")
	return []byte(output.String())
}

func packageGroups(packages []Package, modulePath string) []packageGroup {
	commands := packageGroup{id: "commands", label: "Commands", color: "#a16207", bgColor: "#fffbeb"}
	testPackages := make(map[string][]Package)
	components := packageGroup{
		id:      "components",
		label:   "Components",
		color:   "#7c3aed",
		bgColor: "#f5f3ff",
	}
	config := packageGroup{id: "config", label: "Config", color: "#1d4ed8", bgColor: "#eff6ff"}
	initializer := packageGroup{id: "initializer", label: "Initializer", color: "#c2410c", bgColor: "#fff7ed"}
	platformPackages := make(map[string][]Package)
	orchestratorPackages := make(map[string][]Package)
	transportPackages := make(map[string][]Package)
	servicePackages := make(map[string][]Package)
	for _, pkg := range packages {
		label := strings.TrimPrefix(pkg.ImportPath, modulePath+"/")
		if strings.HasPrefix(label, "cmd/") {
			commands.packages = append(commands.packages, pkg)
			continue
		}
		if strings.HasPrefix(label, "tests/") {
			name, _ := packageSegment(label, "tests/")
			testPackages[name] = append(testPackages[name], pkg)
			continue
		}
		if service, ok := serviceName(label); ok {
			servicePackages[service] = append(servicePackages[service], pkg)
			continue
		}
		if isPackageFamily(label, "config") {
			config.packages = append(config.packages, pkg)
			continue
		}
		if isPackageFamily(label, "initializer") {
			initializer.packages = append(initializer.packages, pkg)
			continue
		}
		if name, ok := packageSuffix(label, "pkg/platform/"); ok {
			platformPackages[name] = append(platformPackages[name], pkg)
			continue
		}
		if name, ok := packageSegment(label, "pkg/orchestrators/"); ok {
			orchestratorPackages[name] = append(orchestratorPackages[name], pkg)
			continue
		}
		if name, ok := packageSegment(label, "pkg/transports/"); ok {
			transportPackages[transportName(name)] = append(transportPackages[transportName(name)], pkg)
			continue
		}
		components.packages = append(components.packages, pkg)
	}

	groups := make([]packageGroup, 0, len(testPackages)+len(platformPackages)+len(orchestratorPackages)+len(transportPackages)+len(servicePackages)+4)
	if len(commands.packages) > 0 {
		groups = append(groups, commands)
	}
	groups = appendTestPackageGroups(groups, testPackages)
	if len(components.packages) > 0 {
		groups = append(groups, components)
	}
	if len(config.packages) > 0 {
		groups = append(groups, config)
	}
	if len(initializer.packages) > 0 {
		groups = append(groups, initializer)
	}
	groups = appendPackageGroups(groups, platformPackages, "platform", "Platform", "#0f766e", "#f0fdfa")
	groups = appendPackageGroups(groups, orchestratorPackages, "orchestrator", "Orchestrator", "#a21caf", "#fdf4ff")
	groups = appendPackageGroups(groups, transportPackages, "transport", "Transport", "#0369a1", "#f0f9ff")
	serviceNames := make([]string, 0, len(servicePackages))
	for service := range servicePackages {
		serviceNames = append(serviceNames, service)
	}
	slices.Sort(serviceNames)
	for _, service := range serviceNames {
		groups = append(groups, packageGroup{
			id:       "service_" + service,
			label:    "Service: " + service,
			color:    serviceClusterColor(service),
			bgColor:  "#ffffff",
			packages: servicePackages[service],
		})
	}
	return groups
}

func appendTestPackageGroups(groups []packageGroup, packagesByName map[string][]Package) []packageGroup {
	requestedGroups := []string{"functional", "release", "stress", "adhoc"}
	for _, name := range requestedGroups {
		packages := packagesByName[name]
		if len(packages) == 0 {
			continue
		}
		groups = append(groups, packageGroup{
			id:       "tests_" + name,
			label:    "Tests: " + name,
			color:    "#be123c",
			bgColor:  "#fff1f2",
			packages: packages,
		})
	}

	requested := make(map[string]struct{}, len(requestedGroups))
	for _, name := range requestedGroups {
		requested[name] = struct{}{}
	}
	var supportPackages []Package
	for name, packages := range packagesByName {
		if _, ok := requested[name]; !ok {
			supportPackages = append(supportPackages, packages...)
		}
	}
	slices.SortFunc(supportPackages, func(left, right Package) int {
		return strings.Compare(left.ImportPath, right.ImportPath)
	})
	if len(supportPackages) > 0 {
		groups = append(groups, packageGroup{
			id:       "tests_support",
			label:    "Tests: support",
			color:    "#be123c",
			bgColor:  "#fff1f2",
			packages: supportPackages,
		})
	}
	return groups
}

func packageImportsByKind(pkg Package) (production []string, testOnly []string) {
	production = slices.Clone(pkg.Imports)
	slices.Sort(production)
	production = slices.Compact(production)

	productionSet := make(map[string]struct{}, len(production))
	for _, imported := range production {
		productionSet[imported] = struct{}{}
	}
	tests := make([]string, 0, len(pkg.TestImports)+len(pkg.XTestImports))
	tests = append(tests, pkg.TestImports...)
	tests = append(tests, pkg.XTestImports...)
	slices.Sort(tests)
	tests = slices.Compact(tests)
	for _, imported := range tests {
		if _, exists := productionSet[imported]; !exists {
			testOnly = append(testOnly, imported)
		}
	}
	return production, testOnly
}

func appendPackageGroups(groups []packageGroup, packagesByName map[string][]Package, idPrefix, labelPrefix, color, bgColor string) []packageGroup {
	names := make([]string, 0, len(packagesByName))
	for name := range packagesByName {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		groups = append(groups, packageGroup{
			id:       idPrefix + "_" + strings.ReplaceAll(name, "/", "_"),
			label:    labelPrefix + ": " + name,
			color:    color,
			bgColor:  bgColor,
			packages: packagesByName[name],
		})
	}
	return groups
}

func packageSuffix(label, prefix string) (string, bool) {
	if !strings.HasPrefix(label, prefix) {
		return "", false
	}
	name := strings.TrimPrefix(label, prefix)
	return name, name != ""
}

func packageSegment(label, prefix string) (string, bool) {
	name, ok := packageSuffix(label, prefix)
	if !ok {
		return "", false
	}
	segment, _, _ := strings.Cut(name, "/")
	return segment, segment != ""
}

func transportName(name string) string {
	if name == "http" {
		return "rest"
	}
	return name
}

func serviceClusterColor(service string) string {
	switch service {
	case "automations":
		return "#db2777"
	case "factory_definitions":
		return "#2563eb"
	case "factory_runtime":
		return "#16a34a"
	case "factory_sessions":
		return "#7c3aed"
	case "models":
		return "#db2777"
	case "provider_sessions":
		return "#0891b2"
	case "recordings":
		return "#ca8a04"
	case "work":
		return "#ea580c"
	case "workers":
		return "#65a30d"
	default:
		return "#64748b"
	}
}

func nodeFillColor(label string) string {
	if strings.HasPrefix(label, "cmd/") {
		return commandNodeColor
	}
	if label == "pkg/root" {
		return rootNodeColor
	}
	if serviceName, ok := serviceName(label); ok {
		return serviceColor(serviceName)
	}
	switch {
	case isPackageFamily(label, "config"):
		return "#bfdbfe"
	case isPackageFamily(label, "initializer"):
		return "#fed7aa"
	case isPackageFamily(label, "orchestrators"):
		return "#bbf7d0"
	case isPackageFamily(label, "platform"):
		return "#bae6fd"
	case isPackageFamily(label, "transports"):
		return "#fbcfe8"
	case isPackageFamily(label, "wire"):
		return "#ddd6fe"
	}
	return defaultNodeColor
}

// edgeAttributes distinguishes service-root imports, Wire construction imports,
// and imports that bypass a service's root package.
func edgeAttributes(source, target string) string {
	targetService, targetIsService := serviceName(target)
	if !targetIsService {
		return ""
	}
	if target == "pkg/services/"+targetService {
		return edgeAttributesForColor(serviceRootEdgeColor)
	}
	if !isServiceInternalPackage(target, targetService) {
		return ""
	}
	if isPackageFamily(source, "wire") {
		return edgeAttributesForColor(wireEdgeColor)
	}

	if sourceService, sourceIsService := serviceName(source); sourceIsService && sourceService == targetService {
		return ""
	}
	return edgeAttributesForColor(boundaryViolationColor)
}

func edgeAttributesForColor(color string) string {
	return fmt.Sprintf("[color=%q, penwidth=1.4]", color)
}

func isServiceInternalPackage(label, service string) bool {
	return strings.HasPrefix(label, "pkg/services/"+service+"/")
}

func isPackageFamily(label, family string) bool {
	packagePath := "pkg/" + family
	return label == packagePath || strings.HasPrefix(label, packagePath+"/")
}

func serviceColor(service string) string {
	switch service {
	case "automations":
		return "#fecdd3"
	case "bundle":
		return "#ddd6fe"
	case "edges":
		return "#fdba74"
	case "factory_definitions":
		return "#bfdbfe"
	case "factory_runtime":
		return "#bbf7d0"
	case "factory_sessions":
		return "#e9d5ff"
	case "models":
		return "#fbcfe8"
	case "provider_sessions":
		return "#a5f3fc"
	case "recordings":
		return "#fde68a"
	case "work":
		return "#fed7aa"
	case "workers":
		return "#d9f99d"
	default:
		return "#e9d5ff"
	}
}

func serviceName(label string) (string, bool) {
	const servicesPrefix = "pkg/services/"
	if !strings.HasPrefix(label, servicesPrefix) {
		return "", false
	}
	remaining := strings.TrimPrefix(label, servicesPrefix)
	service, _, _ := strings.Cut(remaining, "/")
	return service, service != ""
}

// WriteDOT creates parent directories and writes a graph artifact.
func WriteDOT(path string, graph []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create dependency graph directory: %w", err)
	}
	if err := os.WriteFile(path, graph, 0o644); err != nil {
		return fmt.Errorf("write dependency graph: %w", err)
	}
	return nil
}

// RenderSVG uses Graphviz to render a DOT artifact as SVG.
func RenderSVG(ctx context.Context, dotBinary, dotPath, svgPath string) error {
	if err := os.MkdirAll(filepath.Dir(svgPath), 0o755); err != nil {
		return fmt.Errorf("create dependency graph directory: %w", err)
	}
	command := exec.CommandContext(ctx, dotBinary, "-Tsvg", dotPath, "-o", svgPath)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("render dependency graph with Graphviz: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
