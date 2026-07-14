package contractopenapidiff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// CompareYAML loads two OpenAPI YAML documents and classifies their differences.
func CompareYAML(before, after []byte) (Result, error) {
	beforeDoc, err := loadDocument(before)
	if err != nil {
		return Result{}, fmt.Errorf("load before openapi document: %w", err)
	}
	afterDoc, err := loadDocument(after)
	if err != nil {
		return Result{}, fmt.Errorf("load after openapi document: %w", err)
	}
	return CompareDocuments(beforeDoc, afterDoc)
}

// CompareDocuments classifies supported differences between loaded OpenAPI documents.
func CompareDocuments(before, after *openapi3.T) (Result, error) {
	if before == nil || after == nil {
		return Result{}, fmt.Errorf("openapi document is nil")
	}
	structuralChanges, err := collectStructuralChanges(before, after)
	if err != nil {
		return Result{}, err
	}

	changes := append(structuralChanges, collectDocumentationChanges(before, after)...)
	sortChanges(changes)

	classification := classifyStructuralChanges(structuralChanges)

	return Result{
		Classification: classification,
		Changes:        changes,
	}, nil
}

func loadDocument(data []byte) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(data)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func sortChanges(changes []Change) {
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Path != changes[j].Path {
			return changes[i].Path < changes[j].Path
		}
		return changes[i].Code < changes[j].Code
	})
}

func operationPath(method, path string) string {
	return strings.ToUpper(method) + " " + path
}

func appendDocChange(changes []Change, code, path string) []Change {
	return append(changes, Change{Code: code, Path: path})
}

func appendMinorChange(changes []Change, code, path string) []Change {
	return append(changes, Change{Code: code, Path: path})
}

func appendMajorChange(changes []Change, code, path string) []Change {
	return append(changes, Change{Code: code, Path: path})
}

func classifyStructuralChanges(structuralChanges []Change) Classification {
	for _, change := range structuralChanges {
		if isMajorChangeCode(change.Code) {
			return ClassificationMajor
		}
	}
	if len(structuralChanges) > 0 {
		return ClassificationMinor
	}
	return ClassificationPatch
}

func isMajorChangeCode(code string) bool {
	return strings.HasPrefix(code, "openapi.remove.") || strings.HasPrefix(code, "openapi.narrow.")
}

func stringPtrEqual(before, after *string) bool {
	if before == nil && after == nil {
		return true
	}
	if before == nil || after == nil {
		return false
	}
	return *before == *after
}

func externalDocsEqual(before, after *openapi3.ExternalDocs) bool {
	if before == nil && after == nil {
		return true
	}
	if before == nil || after == nil {
		return false
	}
	return before.URL == after.URL && before.Description == after.Description
}

func appendExternalDocsChange(changes []Change, code, path string, before, after *openapi3.ExternalDocs) []Change {
	if externalDocsEqual(before, after) {
		return changes
	}
	return appendDocChange(changes, code, path)
}
