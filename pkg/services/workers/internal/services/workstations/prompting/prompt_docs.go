package prompting

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

const factoryDocsTargetPrefix = "factory/docs/"

// NormalizeFactoryBundledDocTargetPaths returns sorted unique DOC target paths under factory/docs/**.
func NormalizeFactoryBundledDocTargetPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(paths))
	normalized := make([]string, 0, len(paths))
	for _, path := range paths {
		cleaned := strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
		if cleaned == "" || !strings.HasPrefix(cleaned, factoryDocsTargetPrefix) {
			continue
		}
		if len(cleaned) <= len(factoryDocsTargetPrefix) {
			continue
		}
		if _, exists := seen[cleaned]; exists {
			continue
		}
		seen[cleaned] = struct{}{}
		normalized = append(normalized, cleaned)
	}

	sort.Strings(normalized)
	return normalized
}

func bundledDocVariableReferences(docPaths []string) []PromptTemplateVariableReference {
	references := make([]PromptTemplateVariableReference, 0, len(docPaths))
	for _, targetPath := range docPaths {
		references = append(references, PromptTemplateVariableReference{
			Category:    PromptTemplateVariableCategoryDoc,
			Description: fmt.Sprintf("Bundled factory documentation at %s.", targetPath),
			Example:     fmt.Sprintf(`{{ index .Docs %q }}`, targetPath),
			Path:        fmt.Sprintf(`.Docs[%q]`, targetPath),
		})
	}

	return references
}

func bundledDocTargetPathSet(docPaths []string) map[string]struct{} {
	set := make(map[string]struct{}, len(docPaths))
	for _, targetPath := range docPaths {
		set[targetPath] = struct{}{}
	}
	return set
}

func bundledDocPlaceholderContents(docPaths []string) map[string]string {
	if len(docPaths) == 0 {
		return map[string]string{}
	}

	contents := make(map[string]string, len(docPaths))
	for _, targetPath := range docPaths {
		contents[targetPath] = "bundled-doc-content"
	}
	return contents
}

// NewFactoryDocsLoader binds the exact read-only filesystem effect used to
// load bundled Factory documentation.
func NewFactoryDocsLoader(fileSystem platformfilesystem.ReadFileTree) (workerexecution.FactoryDocsLoader, error) {
	if fileSystem == nil {
		return nil, fmt.Errorf("Workers Factory docs filesystem is required")
	}
	return func(factoryDir string) (map[string]string, error) {
		return loadBundledDocContentsFromFactoryDir(factoryDir, fileSystem)
	}, nil
}

func loadBundledDocContentsFromFactoryDir(
	factoryDir string,
	fileSystem platformfilesystem.ReadFileTree,
) (map[string]string, error) {
	if strings.TrimSpace(factoryDir) == "" {
		return map[string]string{}, nil
	}
	if fileSystem == nil {
		return nil, fmt.Errorf("Workers Factory docs filesystem is required")
	}

	docsDir := filepath.Join(factoryDir, "docs")
	info, err := fileSystem.Stat(docsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return map[string]string{}, nil
	}

	contents := make(map[string]string)
	err = fileSystem.WalkDir(docsDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		relativePath, err := filepath.Rel(docsDir, path)
		if err != nil {
			return err
		}

		targetPath := factoryDocsTargetPrefix + filepath.ToSlash(relativePath)
		data, err := fileSystem.ReadFile(path)
		if err != nil {
			return err
		}
		contents[targetPath] = string(data)
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(contents) == 0 {
		return map[string]string{}, nil
	}
	return contents, nil
}

func unavailableBundledDocReason(targetPath string) string {
	return fmt.Sprintf("The current factory does not bundle documentation at %q in this editing context.", targetPath)
}

func buildPromptValidationData(inputCount int, docPaths []string) PromptData {
	inputs := make([]TokenData, 0, inputCount)
	for index := 0; index < inputCount; index++ {
		inputs = append(inputs, TokenData{
			Name:       fmt.Sprintf("input-%d", index),
			WorkID:     fmt.Sprintf("work-%d", index),
			WorkTypeID: "processor",
			DataType:   "work",
			TraceID:    fmt.Sprintf("trace-%d", index),
			ParentID:   "parent",
			Project:    "project",
			Tags: map[string]string{
				"branch": "main",
			},
			Payload: "payload",
			Relations: []work.Relation{{
				Type:          work.RelationDependsOn,
				TargetWorkID:  "target-work",
				RequiredState: "SUCCEEDED",
			}},
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: "content",
			}},
			PreviousOutput:    "previous-output",
			RejectionFeedback: "rejection-feedback",
			History: PromptHistory{
				LastError:    "last-error",
				FailureCount: 1,
				FailureLog: []workerexecution.Failure{{
					TransitionID: "transition",
					Error:        "failure",
					Attempt:      1,
				}},
				TotalVisits:   1,
				AttemptNumber: 2,
			},
		})
	}

	return PromptData{
		Docs:   bundledDocPlaceholderContents(docPaths),
		Inputs: inputs,
		Context: PromptContext{
			WorkDir:     "/tmp/workdir",
			ArtifactDir: "/tmp/artifacts",
			Project:     "project",
			SessionID:   workerexecution.DefaultSessionID,
			Env: map[string]string{
				"API_KEY": "value",
			},
		},
	}
}
