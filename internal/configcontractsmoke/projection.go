package configcontractsmoke

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/portpowered/infinite-you/internal/contractstaging"
)

const familyConfiguration FamilyID = "configuration"

type projectionArtifact struct {
	family           Family
	firstProjection  []byte
	secondProjection []byte
	canonical        []byte
	staged           []byte
	canonicalReadErr error
	stagedReadErr    error
}

// CheckProjectionParity proves that each configuration projection is
// deterministic and synchronized with its canonical owner and approved export.
// It composes contractstaging.Check so the existing staging allowlist remains
// the only ownership policy for generated package artifacts.
func CheckProjectionParity(repositoryRoot string, families []Family) ([]Diagnostic, error) {
	first, err := contractstaging.Artifacts(repositoryRoot)
	if err != nil {
		return nil, fmt.Errorf("build first configuration projection: %w", err)
	}
	second, err := contractstaging.Artifacts(repositoryRoot)
	if err != nil {
		return nil, fmt.Errorf("build second configuration projection: %w", err)
	}

	artifacts := make([]projectionArtifact, 0, len(families))
	for _, family := range families {
		canonical, canonicalErr := readProjectionFile(repositoryRoot, family.SchemaProjectionPath)
		staged, stagedErr := readProjectionFile(repositoryRoot, family.ExportPath)
		artifacts = append(artifacts, projectionArtifact{
			family: family, firstProjection: first[family.ExportPath], secondProjection: second[family.ExportPath],
			canonical: canonical, staged: staged, canonicalReadErr: canonicalErr, stagedReadErr: stagedErr,
		})
	}
	diagnostics := compareProjectionArtifacts(artifacts)

	drift, err := contractstaging.Check(repositoryRoot)
	if err != nil {
		return nil, fmt.Errorf("check reviewed contract staging: %w", err)
	}
	for _, path := range drift.Unexpected {
		diagnostics = append(diagnostics, forbiddenProjectionDiagnostic(path))
	}
	return diagnostics, nil
}

func forbiddenProjectionDiagnostic(path string) Diagnostic {
	return Diagnostic{
		Code: "config.projection.forbidden", Family: familyConfiguration, Path: path,
		Message: "artifact is outside the reviewed contract staging allowlist",
	}
}

func readProjectionFile(repositoryRoot, path string) ([]byte, error) {
	payload, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(path)))
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func compareProjectionArtifacts(artifacts []projectionArtifact) []Diagnostic {
	var diagnostics []Diagnostic
	for _, artifact := range artifacts {
		expected := artifact.firstProjection
		if !bytes.Equal(expected, artifact.secondProjection) {
			diagnostics = append(diagnostics, projectionDiagnostic(
				"config.projection.nondeterministic", artifact.family, artifact.family.ExportPath,
				firstJSONDifference(expected, artifact.secondProjection), "repeated projection produced different bytes",
			))
		}
		if artifact.canonicalReadErr != nil {
			diagnostics = append(diagnostics, projectionDiagnostic(
				"config.projection.missing", artifact.family, artifact.family.SchemaProjectionPath, "/",
				"canonical projection is missing or unreadable",
			))
		} else if !bytes.Equal(expected, artifact.canonical) {
			diagnostics = append(diagnostics, projectionDiagnostic(
				"config.projection.stale", artifact.family, artifact.family.SchemaProjectionPath,
				firstJSONDifference(expected, artifact.canonical), "canonical projection is stale",
			))
		}
		if artifact.stagedReadErr != nil {
			diagnostics = append(diagnostics, projectionDiagnostic(
				"config.projection.missing", artifact.family, artifact.family.ExportPath, "/",
				fmt.Sprintf("approved export is missing or unreadable; canonical owner %q", artifact.family.CanonicalOwnerPath),
			))
		} else if !bytes.Equal(expected, artifact.staged) {
			diagnostics = append(diagnostics, projectionDiagnostic(
				"config.projection.stale", artifact.family, artifact.family.ExportPath,
				firstJSONDifference(expected, artifact.staged),
				fmt.Sprintf("approved export is stale relative to canonical owner %q", artifact.family.CanonicalOwnerPath),
			))
		}
	}
	return diagnostics
}

func projectionDiagnostic(code string, family Family, path, documentPath, message string) Diagnostic {
	return Diagnostic{
		Code: code, Family: family.ID, Path: path,
		Message: fmt.Sprintf("%s at JSON path %q", message, documentPath),
	}
}

func firstJSONDifference(expectedPayload, actualPayload []byte) string {
	var expected, actual any
	if json.Unmarshal(expectedPayload, &expected) != nil || json.Unmarshal(actualPayload, &actual) != nil {
		return "/"
	}
	if path, different := jsonDifference(expected, actual, ""); different {
		if path == "" {
			return "/"
		}
		return path
	}
	return "/"
}

func jsonDifference(expected, actual any, path string) (string, bool) {
	expectedObject, expectedIsObject := expected.(map[string]any)
	actualObject, actualIsObject := actual.(map[string]any)
	if expectedIsObject && actualIsObject {
		keys := make(map[string]struct{}, len(expectedObject)+len(actualObject))
		for key := range expectedObject {
			keys[key] = struct{}{}
		}
		for key := range actualObject {
			keys[key] = struct{}{}
		}
		ordered := make([]string, 0, len(keys))
		for key := range keys {
			ordered = append(ordered, key)
		}
		sort.Strings(ordered)
		for _, key := range ordered {
			expectedChild, expectedOK := expectedObject[key]
			actualChild, actualOK := actualObject[key]
			childPath := path + "/" + escapeJSONPointer(key)
			if !expectedOK || !actualOK {
				return childPath, true
			}
			if difference, ok := jsonDifference(expectedChild, actualChild, childPath); ok {
				return difference, true
			}
		}
		return "", false
	}
	expectedArray, expectedIsArray := expected.([]any)
	actualArray, actualIsArray := actual.([]any)
	if expectedIsArray && actualIsArray {
		limit := len(expectedArray)
		if len(actualArray) < limit {
			limit = len(actualArray)
		}
		for index := 0; index < limit; index++ {
			childPath := path + "/" + strconv.Itoa(index)
			if difference, ok := jsonDifference(expectedArray[index], actualArray[index], childPath); ok {
				return difference, true
			}
		}
		if len(expectedArray) != len(actualArray) {
			return path + "/" + strconv.Itoa(limit), true
		}
		return "", false
	}
	return path, !jsonValuesEqual(expected, actual)
}

func jsonValuesEqual(left, right any) bool {
	leftPayload, leftErr := json.Marshal(left)
	rightPayload, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftPayload, rightPayload)
}

func escapeJSONPointer(value string) string {
	result := bytes.ReplaceAll([]byte(value), []byte("~"), []byte("~0"))
	result = bytes.ReplaceAll(result, []byte("/"), []byte("~1"))
	return string(result)
}
