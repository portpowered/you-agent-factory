package contractvalidator

import (
	"fmt"
	"sort"
	"strconv"
)

// RuntimeManifestSemanticsDiagnostics applies runtime-manifest semantic checks
// after schema validation succeeds.
func RuntimeManifestSemanticsDiagnostics(document string, value any) []Diagnostic {
	return runtimeManifestDiagnostics(document, value)
}

func runtimeManifestDiagnostics(document string, value any) []Diagnostic {
	root, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	symbols, ok := root["symbols"].(map[string]any)
	if !ok {
		return nil
	}

	keys := sortedStringKeys(symbols)

	symbolsByID := make(map[string]string, len(keys))
	symbolKindByKey := make(map[string]string, len(keys))
	pathByKey := make(map[string]string, len(keys))
	childrenByParent := make(map[string]map[string]struct{}, len(keys))

	for _, key := range keys {
		symbol, ok := symbols[key].(map[string]any)
		if !ok {
			continue
		}
		id, _ := symbol["id"].(string)
		path, _ := symbol["path"].(string)
		kind, _ := symbol["kind"].(string)
		if id != "" {
			symbolsByID[id] = key
		}
		pathByKey[key] = path
		symbolKindByKey[key] = kind

		parent, hasParent := symbol["parent"].(string)
		name, _ := symbol["name"].(string)
		if hasParent && parent != "" && name != "" {
			if childrenByParent[parent] == nil {
				childrenByParent[parent] = make(map[string]struct{})
			}
			childrenByParent[parent][name] = struct{}{}
		}
	}

	var diagnostics []Diagnostic

	pathToKeys := make(map[string][]string, len(keys))
	for _, key := range keys {
		path := pathByKey[key]
		if path == "" {
			continue
		}
		pathToKeys[path] = append(pathToKeys[path], key)
	}
	paths := make([]string, 0, len(pathToKeys))
	for path := range pathToKeys {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		keysWithPath := pathToKeys[path]
		if len(keysWithPath) <= 1 {
			continue
		}
		message := fmt.Sprintf("symbol path %s appears more than once", strconv.Quote(path))
		for _, key := range keysWithPath {
			diagnostics = append(diagnostics, newDiagnostic(
				"javascript.path.duplicate",
				"/symbols/"+escapeJSONPointerToken(key)+"/path",
				message,
				document,
			))
		}
	}

	for _, key := range keys {
		symbol, ok := symbols[key].(map[string]any)
		if !ok {
			continue
		}
		symbolPath := "/symbols/" + escapeJSONPointerToken(key)

		if parent, ok := symbol["parent"].(string); ok && parent != "" {
			parentKey, exists := symbolsByID[parent]
			if !exists {
				diagnostics = append(diagnostics, newDiagnostic(
					"javascript.parent.unresolved",
					symbolPath+"/parent",
					fmt.Sprintf("parent symbol %s is not declared", strconv.Quote(parent)),
					document,
				))
			} else if symbolKindByKey[parentKey] != "namespace" {
				diagnostics = append(diagnostics, newDiagnostic(
					"javascript.parent.unresolved",
					symbolPath+"/parent",
					fmt.Sprintf("parent symbol %s is not a namespace", strconv.Quote(parent)),
					document,
				))
			}
		}

		members, ok := symbol["members"].([]any)
		if !ok {
			continue
		}
		symbolID, _ := symbol["id"].(string)
		children := childrenByParent[symbolID]
		for index, memberValue := range members {
			member, ok := memberValue.(string)
			if !ok {
				continue
			}
			if _, resolved := children[member]; !resolved {
				diagnostics = append(diagnostics, newDiagnostic(
					"javascript.member.unresolved",
					symbolPath+"/members/"+strconv.Itoa(index),
					fmt.Sprintf("member %s does not resolve to a declared child symbol", strconv.Quote(member)),
					document,
				))
			}
		}
	}

	sortDiagnostics(diagnostics)
	return diagnostics
}
