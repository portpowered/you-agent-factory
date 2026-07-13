package symbolidentity

import (
	"sort"
	"strings"
)

// ProjectInstalledBindings builds a deterministic symbol-identity inventory for the
// currently installed JavaScript runtime surface. The descriptor is pure and
// read-only: it does not construct a goja VM or mutate installed bindings.
func ProjectInstalledBindings() Inventory {
	records := installedBindingRecords()
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].Path < records[j].Path
	})
	return Inventory{
		FormatVersion: FormatVersion,
		Symbols:       records,
	}
}

// ExpectedInstalledPaths returns the sorted full paths for the currently installed
// JavaScript runtime binding surface described by the pure descriptor.
func ExpectedInstalledPaths() []string {
	records := installedBindingRecords()
	paths := make([]string, len(records))
	for i, record := range records {
		paths[i] = record.Path
	}
	sort.Strings(paths)
	return paths
}

func installedBindingRecords() []SymbolRecord {
	workflowMembers := []string{
		"artifact",
		"budget",
		"checkpoint",
		"final",
		"log",
		"resumeState",
	}
	agentMembers := []string{"run"}

	records := []SymbolRecord{
		valueRecord("args"),
		valueRecord("meta"),
		namespaceRecord("workflow", workflowMembers),
		namespaceRecord("agent", agentMembers),
		syncFunctionRecord("phase"),
		syncFunctionRecord("log"),
		asyncFunctionRecord("parallel"),
		asyncFunctionRecord("pipeline"),
	}

	for _, member := range workflowMembers {
		records = append(records, memberFunctionRecord("workflow", member, false))
	}
	records = append(records, memberFunctionRecord("agent", "run", true))

	return records
}

func valueRecord(path string) SymbolRecord {
	return SymbolRecord{
		IDCandidate: idCandidate(path),
		Name:        leafName(path),
		Path:        path,
		Kind:        kindValue,
	}
}

func namespaceRecord(path string, members []string) SymbolRecord {
	sortedMembers := append([]string(nil), members...)
	sort.Strings(sortedMembers)
	return SymbolRecord{
		IDCandidate: idCandidate(path),
		Name:        leafName(path),
		Path:        path,
		Kind:        kindNamespace,
		Members:     sortedMembers,
	}
}

func syncFunctionRecord(path string) SymbolRecord {
	return SymbolRecord{
		IDCandidate: idCandidate(path),
		Name:        leafName(path),
		Path:        path,
		Kind:        kindFunction,
		Callable:    true,
	}
}

func asyncFunctionRecord(path string) SymbolRecord {
	record := syncFunctionRecord(path)
	record.Async = true
	return record
}

func memberFunctionRecord(parent, name string, async bool) SymbolRecord {
	path := parent + "." + name
	record := SymbolRecord{
		IDCandidate: idCandidate(path),
		Name:        name,
		Path:        path,
		Kind:        kindFunction,
		Parent:      parent,
		Callable:    true,
		Async:       async,
	}
	return record
}

func idCandidate(path string) string {
	return strings.ReplaceAll(path, ".", "-")
}

func leafName(path string) string {
	if idx := strings.LastIndex(path, "."); idx >= 0 {
		return path[idx+1:]
	}
	return path
}
