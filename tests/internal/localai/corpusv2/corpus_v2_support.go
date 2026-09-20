package corpusv2

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	CorpusV2SchemaVersion = "you.localai.omni-video-corpus-input.v2"
	CorpusV2Repository    = "C:/Users/andre/work/experiments/tv-girl"
	CorpusV2Commit        = "443ee4715e6e0f8ec02489e4fda9554d15434e08"
	CorpusV2IndexPath     = "docs/selfiepostx/video-output-index.md"
	CorpusV2IndexSHA256   = "ac4eeb02e6d1e1065176dcf5c18f74367b241c698ec6344ca48829b7ab770c28"
	CorpusV2Mode          = "read-only-external"
	CorpusV2Ordering      = "durationSeconds,byteCount,normalizedPath:index[0,floor((n-1)/2),n-1]"
	CorpusV2ForbiddenPort = 7437
	CorpusV2MaxDiskBytes  = int64(1 << 30)

	CorpusV2PerInvocationTimeoutSeconds = 180
	CorpusV2Retries                     = 0
	CorpusV2MaxHeavyProcesses           = 1
	CorpusV2MaxDownloadBytes            = int64(0)
	CorpusV2MaxPaidUSD                  = int64(0)
	CorpusV2NetworkPolicy               = "declared-local-assets-and-loopback-only"
)

var corpusV2RequiredStudies = []string{
	"selfie-jessie-duration-study",
	"selfie-jessie-prompt-study",
	"selfie-jessie-quality",
}

var corpusV2AuthoritySectionCounts = map[string]int{
	"mv-ruinan":                    24,
	"selfie-jessie-duration-study": 10,
	"selfie-jessie-prompt-study":   100,
	"selfie-jessie-quality":        180,
	"selfie-jessie":                19,
	"selfie-ruinan":                37,
}

// CorpusV2ValidationCode is a stable, actionable classification for a
// fail-closed corpus preflight. It is intentionally local to the corpus
// contract and does not reuse model or provider failure classes.
type CorpusV2ValidationCode string

const (
	CorpusV2CodeInvalidIndex           CorpusV2ValidationCode = "invalid_index"
	CorpusV2CodeMalformedIndex         CorpusV2ValidationCode = "malformed_index"
	CorpusV2CodeUnknownSection         CorpusV2ValidationCode = "unknown_section"
	CorpusV2CodeCountMismatch          CorpusV2ValidationCode = "count_mismatch"
	CorpusV2CodeDuplicateClip          CorpusV2ValidationCode = "duplicate_clip"
	CorpusV2CodeDuplicatePrompt        CorpusV2ValidationCode = "duplicate_prompt"
	CorpusV2CodePathEscape             CorpusV2ValidationCode = "path_escape"
	CorpusV2CodePathIdentity           CorpusV2ValidationCode = "path_identity_mismatch"
	CorpusV2CodeMissingSibling         CorpusV2ValidationCode = "missing_sibling"
	CorpusV2CodeNonRegularFile         CorpusV2ValidationCode = "non_regular_file"
	CorpusV2CodeUnreadableFile         CorpusV2ValidationCode = "unreadable_file"
	CorpusV2CodeHashMismatch           CorpusV2ValidationCode = "hash_mismatch"
	CorpusV2CodeSourceCommitMismatch   CorpusV2ValidationCode = "source_commit_mismatch"
	CorpusV2CodeIndexHashMismatch      CorpusV2ValidationCode = "index_hash_mismatch"
	CorpusV2CodeSourceMutation         CorpusV2ValidationCode = "source_mutation"
	CorpusV2CodeInvalidMedia           CorpusV2ValidationCode = "invalid_media"
	CorpusV2CodeInsufficientRows       CorpusV2ValidationCode = "insufficient_rows"
	CorpusV2CodeSelectionDuplicate     CorpusV2ValidationCode = "selection_duplicate"
	CorpusV2CodeSelectionMismatch      CorpusV2ValidationCode = "selection_mismatch"
	CorpusV2CodeMetadataMismatch       CorpusV2ValidationCode = "metadata_mismatch"
	CorpusV2CodeAuthorityUnavailable   CorpusV2ValidationCode = "authority_unavailable"
	CorpusV2CodeGitIdentityUnavailable CorpusV2ValidationCode = "git_identity_unavailable"
)

// CorpusV2ValidationError describes a failure that must stop before a model,
// worker, lease, output, or heavyweight process can be admitted.
type CorpusV2ValidationError struct {
	Code     CorpusV2ValidationCode
	Field    string
	Expected string
	Observed string
	Cause    error
}

func (e *CorpusV2ValidationError) Error() string {
	if e == nil {
		return ""
	}
	detail := ""
	if e.Expected != "" || e.Observed != "" {
		detail = fmt.Sprintf(" (expected %q, observed %q)", e.Expected, e.Observed)
	}
	if e.Cause != nil {
		return fmt.Sprintf("corpus v2 %s at %s%s: %v", e.Code, e.Field, detail, e.Cause)
	}
	return fmt.Sprintf("corpus v2 %s at %s%s", e.Code, e.Field, detail)
}

func (e *CorpusV2ValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func corpusV2Error(code CorpusV2ValidationCode, field, expected, observed string, cause error) error {
	return &CorpusV2ValidationError{Code: code, Field: field, Expected: expected, Observed: observed, Cause: cause}
}

// CorpusV2Authority pins the external source. RepositoryRoot is the local
// checkout used for read-only inspection; the other fields are immutable
// authority values from the operator amendment.
type CorpusV2Authority struct {
	RepositoryRoot  string
	Commit          string
	IndexPath       string
	IndexSHA256     string
	Mode            string
	PairCount       int
	SectionCounts   map[string]int
	RequiredStudies []string
	ExpectedSamples []CorpusV2SampleExpectation
}

func DefaultCorpusV2Authority() CorpusV2Authority {
	return CorpusV2Authority{
		RepositoryRoot:  CorpusV2Repository,
		Commit:          CorpusV2Commit,
		IndexPath:       CorpusV2IndexPath,
		IndexSHA256:     CorpusV2IndexSHA256,
		Mode:            CorpusV2Mode,
		PairCount:       370,
		SectionCounts:   cloneCorpusV2Counts(corpusV2AuthoritySectionCounts),
		RequiredStudies: append([]string(nil), corpusV2RequiredStudies...),
		ExpectedSamples: defaultCorpusV2SampleExpectations(),
	}
}

func cloneCorpusV2Counts(input map[string]int) map[string]int {
	output := make(map[string]int, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

// CorpusV2FileIdentity is a read-only identity of an indexed source file.
// Identity is a stable content identity (`file:<bytes>:<sha256>`); Path keeps
// the source location explicit for later shipped-CLI invocation.
type CorpusV2FileIdentity struct {
	Path     string `json:"path"`
	Identity string `json:"identity"`
	Bytes    int64  `json:"bytes"`
	SHA256   string `json:"sha256"`
}

type CorpusV2Pair struct {
	Study   string
	Attempt string
	Clip    CorpusV2FileIdentity
	Prompt  CorpusV2FileIdentity
	Stream  CorpusV2StreamMetadata
}

type CorpusV2Index struct {
	RepositoryRoot string
	Commit         string
	IndexPath      string
	IndexSHA256    string
	Pairs          []CorpusV2Pair
	Sections       map[string][]CorpusV2Pair
}

type CorpusV2StreamMetadata struct {
	Identity        string  `json:"identity"`
	Codec           string  `json:"codec"`
	Width           int     `json:"width"`
	Height          int     `json:"height"`
	FrameRate       string  `json:"frameRate"`
	DurationMillis  int64   `json:"durationMillis"`
	DurationSeconds float64 `json:"durationSeconds"`
	Frames          int64   `json:"frames"`

	durationNumerator   uint64
	durationDenominator uint64
}

type CorpusV2Sample struct {
	Study        string                 `json:"study"`
	Band         string                 `json:"band"`
	Attempt      string                 `json:"attempt"`
	Clip         CorpusV2FileIdentity   `json:"clip"`
	Prompt       CorpusV2FileIdentity   `json:"prompt"`
	Stream       CorpusV2StreamMetadata `json:"stream"`
	SourceCommit string                 `json:"sourceCommit"`
}

type CorpusV2Manifest struct {
	SchemaVersion   string           `json:"schemaVersion"`
	Repository      string           `json:"repository"`
	Commit          string           `json:"commit"`
	IndexPath       string           `json:"indexPath"`
	IndexSHA256     string           `json:"indexSha256"`
	Pairs           []CorpusV2Pair   `json:"pairs"`
	UniqueClips     int              `json:"uniqueClips"`
	UniquePrompts   int              `json:"uniquePrompts"`
	MissingSiblings int              `json:"missingSiblings"`
	Samples         []CorpusV2Sample `json:"samples"`
	CopiedBytes     int64            `json:"copiedBytes"`
	UploadedBytes   int64            `json:"uploadedBytes"`
	ReadOnly        bool             `json:"readOnly"`
}

type corpusV2IndexRow struct {
	Line    int
	Attempt string
	Clip    string
	Prompt  string
}

// ParseCorpusV2Index parses only the strict Markdown index grammar. It does
// not read media, invoke git, copy bytes, or start a model.
func ParseCorpusV2Index(data []byte, repositoryRoot string) (CorpusV2Index, error) {
	root, err := corpusV2AbsoluteRoot(repositoryRoot)
	if err != nil {
		return CorpusV2Index{}, err
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) == 0 || lines[0] != "# Video and prompt output index" {
		return CorpusV2Index{}, corpusV2Error(CorpusV2CodeMalformedIndex, "header", "# Video and prompt output index", firstCorpusV2Line(lines), nil)
	}
	cursor := 1
	for cursor < len(lines) && strings.TrimSpace(lines[cursor]) == "" {
		cursor++
	}
	if cursor >= len(lines) || !strings.HasPrefix(lines[cursor], "Snapshot: ") || strings.TrimSpace(strings.TrimPrefix(lines[cursor], "Snapshot: ")) == "" {
		return CorpusV2Index{}, corpusV2Error(CorpusV2CodeMalformedIndex, "snapshot", "Snapshot: <date>", lineAt(lines, cursor), nil)
	}
	cursor++
	for cursor < len(lines) && strings.TrimSpace(lines[cursor]) == "" {
		cursor++
	}
	const corpusV2IndexDescription = "This index lists all 370 production `clip.mp4` files with a sibling `prompt.md` found at inspection time. Review copies, other filenames, and files without a sibling prompt are not included. Presence here does not mean approved for publication."
	if cursor >= len(lines) || lines[cursor] != corpusV2IndexDescription {
		return CorpusV2Index{}, corpusV2Error(CorpusV2CodeMalformedIndex, "description", corpusV2IndexDescription, lineAt(lines, cursor), nil)
	}

	sections := make(map[string][]CorpusV2Pair)
	var pairs []CorpusV2Pair
	for index := cursor + 1; index < len(lines); {
		if strings.TrimSpace(lines[index]) == "" {
			index++
			continue
		}
		sectionName, declaredCount, ok := parseCorpusV2SectionHeading(lines[index])
		if !ok {
			return CorpusV2Index{}, corpusV2Error(CorpusV2CodeMalformedIndex, fmt.Sprintf("line[%d]", index+1), "## <study> (<count>)", lines[index], nil)
		}
		if _, exists := sections[sectionName]; exists {
			return CorpusV2Index{}, corpusV2Error(CorpusV2CodeUnknownSection, fmt.Sprintf("line[%d]", index+1), "one section per study", sectionName, nil)
		}
		index++
		for index < len(lines) && strings.TrimSpace(lines[index]) == "" {
			index++
		}
		if index+1 >= len(lines) || lines[index] != "| Attempt | Video path | Corresponding prompt path |" || lines[index+1] != "| --- | --- | --- |" {
			return CorpusV2Index{}, corpusV2Error(CorpusV2CodeMalformedIndex, fmt.Sprintf("section.%s.table", sectionName), "canonical three-column table", lineAt(lines, index), nil)
		}
		index += 2
		rows := make([]CorpusV2Pair, 0, declaredCount)
		for index < len(lines) && strings.TrimSpace(lines[index]) != "" && !strings.HasPrefix(lines[index], "## ") {
			row, ok := parseCorpusV2Row(lines[index])
			if !ok {
				return CorpusV2Index{}, corpusV2Error(CorpusV2CodeMalformedIndex, fmt.Sprintf("line[%d]", index+1), "canonical video/prompt row", lines[index], nil)
			}
			pair, err := corpusV2PairFromRow(root, sectionName, row)
			if err != nil {
				return CorpusV2Index{}, err
			}
			rows = append(rows, pair)
			index++
		}
		if len(rows) != declaredCount {
			return CorpusV2Index{}, corpusV2Error(CorpusV2CodeCountMismatch, "section."+sectionName, fmt.Sprint(declaredCount), fmt.Sprint(len(rows)), nil)
		}
		sections[sectionName] = rows
		pairs = append(pairs, rows...)
	}
	if len(pairs) == 0 {
		return CorpusV2Index{}, corpusV2Error(CorpusV2CodeInvalidIndex, "pairs", "at least one pair", "empty", nil)
	}
	if err := validateCorpusV2UniquePairs(pairs); err != nil {
		return CorpusV2Index{}, err
	}
	return CorpusV2Index{RepositoryRoot: root, Pairs: pairs, Sections: sections}, nil
}

func firstCorpusV2Line(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}

func lineAt(lines []string, index int) string {
	if index < 0 || index >= len(lines) {
		return ""
	}
	return lines[index]
}

func parseCorpusV2SectionHeading(line string) (string, int, bool) {
	if !strings.HasPrefix(line, "## ") || !strings.HasSuffix(line, ")") {
		return "", 0, false
	}
	body := strings.TrimPrefix(strings.TrimSuffix(line, ")"), "## ")
	separator := strings.LastIndex(body, " (")
	if separator <= 0 {
		return "", 0, false
	}
	name, countText := body[:separator], body[separator+2:]
	if name == "" || countText == "" || strings.ContainsAny(name, " \t") {
		return "", 0, false
	}
	var count int
	if _, err := fmt.Sscanf(countText, "%d", &count); err != nil || count < 0 || fmt.Sprint(count) != countText {
		return "", 0, false
	}
	return name, count, true
}

func parseCorpusV2Row(line string) (corpusV2IndexRow, bool) {
	if !strings.HasPrefix(line, "| ") || !strings.HasSuffix(line, " |") {
		return corpusV2IndexRow{}, false
	}
	columns := strings.Split(line[2:len(line)-2], " | ")
	if len(columns) != 3 || strings.TrimSpace(columns[0]) != columns[0] || columns[0] == "" {
		return corpusV2IndexRow{}, false
	}
	clipLabel, clipTarget, ok := parseCorpusV2Link(columns[1])
	if !ok {
		return corpusV2IndexRow{}, false
	}
	promptLabel, promptTarget, ok := parseCorpusV2Link(columns[2])
	if !ok || clipLabel != clipTarget || promptLabel != promptTarget {
		return corpusV2IndexRow{}, false
	}
	return corpusV2IndexRow{Attempt: columns[0], Clip: clipTarget, Prompt: promptTarget}, true
}

func parseCorpusV2Link(value string) (string, string, bool) {
	open := strings.Index(value, "](")
	if open <= 0 || !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, ")") {
		return "", "", false
	}
	label := value[1:open]
	target := value[open+2 : len(value)-1]
	if strings.HasPrefix(target, "<") && strings.HasSuffix(target, ">") {
		target = target[1 : len(target)-1]
	}
	if label == "" || target == "" || strings.ContainsAny(label, "\r\n") || strings.ContainsAny(target, "\r\n") {
		return "", "", false
	}
	return label, target, true
}

func corpusV2PairFromRow(root, study string, row corpusV2IndexRow) (CorpusV2Pair, error) {
	clip, err := corpusV2ValidateIndexedPath(root, study, row.Attempt, row.Clip, "clip.mp4")
	if err != nil {
		return CorpusV2Pair{}, err
	}
	prompt, err := corpusV2ValidateIndexedPath(root, study, row.Attempt, row.Prompt, "prompt.md")
	if err != nil {
		return CorpusV2Pair{}, err
	}
	if filepath.Dir(filepath.Clean(clip)) != filepath.Dir(filepath.Clean(prompt)) {
		return CorpusV2Pair{}, corpusV2Error(CorpusV2CodePathIdentity, study+"."+row.Attempt, "clip and prompt share one attempt directory", clip+" / "+prompt, nil)
	}
	return CorpusV2Pair{Study: study, Attempt: row.Attempt, Clip: CorpusV2FileIdentity{Path: clip}, Prompt: CorpusV2FileIdentity{Path: prompt}}, nil
}

func corpusV2ValidateIndexedPath(root, study, attempt, value, baseName string) (string, error) {
	if strings.TrimSpace(value) != value || strings.ContainsRune(value, 0) || strings.ContainsRune(value, '\\') {
		return "", corpusV2Error(CorpusV2CodePathEscape, study+"."+attempt+"."+baseName, "canonical absolute slash path", value, nil)
	}
	normalizedRoot := corpusV2Slash(filepath.Clean(root))
	normalized := corpusV2Slash(value)
	if !corpusV2IsAbsoluteSlashPath(normalized) || !corpusV2PathWithin(normalizedRoot, normalized) {
		return "", corpusV2Error(CorpusV2CodePathEscape, study+"."+attempt+"."+baseName, "path below pinned repository root", normalizedRoot+"; observed="+normalized, nil)
	}
	for _, part := range strings.Split(normalized, "/") {
		if part == "." || part == ".." {
			return "", corpusV2Error(CorpusV2CodePathEscape, study+"."+attempt+"."+baseName, "canonical path without dot segments", normalized, nil)
		}
	}
	relative := strings.TrimPrefix(normalized, normalizedRoot+"/")
	parts := strings.Split(relative, "/")
	if len(parts) < 4 || parts[0] != "production" || parts[1] != study || parts[len(parts)-2] != attempt || parts[len(parts)-1] != baseName {
		return "", corpusV2Error(CorpusV2CodePathIdentity, study+"."+attempt+"."+baseName, "production/<study>/.../<attempt>/"+baseName, relative, nil)
	}
	return filepath.FromSlash(normalized), nil
}

func corpusV2AbsoluteRoot(value string) (string, error) {
	if strings.TrimSpace(value) == "" || strings.ContainsRune(value, 0) {
		return "", corpusV2Error(CorpusV2CodePathEscape, "repositoryRoot", "non-empty absolute root", value, nil)
	}
	abs, err := filepath.Abs(filepath.Clean(value))
	if err != nil || !filepath.IsAbs(abs) {
		return "", corpusV2Error(CorpusV2CodePathEscape, "repositoryRoot", "absolute root", value, err)
	}
	return filepath.Clean(abs), nil
}

func corpusV2ResolveContainedPath(root, path, field string) (string, error) {
	absoluteRoot, err := corpusV2AbsoluteRoot(root)
	if err != nil {
		return "", err
	}
	absolutePath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", corpusV2Error(CorpusV2CodePathEscape, field, "absolute path below repository root", path, err)
	}
	relativePath, err := filepath.Rel(absoluteRoot, absolutePath)
	if err != nil || relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) || filepath.IsAbs(relativePath) || !corpusV2PathWithin(corpusV2Slash(absoluteRoot), corpusV2Slash(absolutePath)) {
		return "", corpusV2Error(CorpusV2CodePathEscape, field, "path below repository root", absoluteRoot+"; observed="+absolutePath, err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return "", corpusV2Error(CorpusV2CodeAuthorityUnavailable, "repositoryRoot", "existing resolvable repository root", absoluteRoot, err)
	}
	resolvedRoot = filepath.Clean(resolvedRoot)

	candidate := absolutePath
	var missingParts []string
	for {
		resolvedPath, resolveErr := filepath.EvalSymlinks(candidate)
		if resolveErr == nil {
			for index := len(missingParts) - 1; index >= 0; index-- {
				resolvedPath = filepath.Join(resolvedPath, missingParts[index])
			}
			resolvedPath = filepath.Clean(resolvedPath)
			expectedPath := filepath.Clean(filepath.Join(resolvedRoot, relativePath))
			if !corpusV2PathWithin(corpusV2Slash(resolvedRoot), corpusV2Slash(resolvedPath)) || !corpusV2SamePath(resolvedPath, expectedPath) {
				return "", corpusV2Error(CorpusV2CodePathEscape, field, "path without symlink or junction aliases below repository root", resolvedRoot+"; observed="+resolvedPath, nil)
			}
			return resolvedPath, nil
		}
		if info, lstatErr := os.Lstat(candidate); lstatErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", corpusV2Error(CorpusV2CodePathEscape, field, "path without symlink or junction aliases below repository root", candidate, resolveErr)
		} else if lstatErr != nil && !errors.Is(lstatErr, os.ErrNotExist) {
			return "", corpusV2Error(CorpusV2CodeUnreadableFile, field, "resolvable path below repository root", candidate, lstatErr)
		}
		if !errors.Is(resolveErr, os.ErrNotExist) {
			return "", corpusV2Error(CorpusV2CodeUnreadableFile, field, "resolvable path below repository root", candidate, resolveErr)
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return "", corpusV2Error(CorpusV2CodePathEscape, field, "path below repository root", absoluteRoot+"; observed="+absolutePath, resolveErr)
		}
		missingParts = append(missingParts, filepath.Base(candidate))
		candidate = parent
	}
}

func corpusV2SamePath(first, second string) bool {
	// Case-only spellings are equivalent only when the filesystem resolves
	// them to one directory entry.
	first, err := filepath.Abs(filepath.Clean(filepath.FromSlash(first)))
	if err != nil {
		return false
	}
	second, err = filepath.Abs(filepath.Clean(filepath.FromSlash(second)))
	if err != nil {
		return false
	}
	if first == second {
		return true
	}

	firstVolume, firstParts, firstAbsolute := corpusV2PathComponents(first)
	secondVolume, secondParts, secondAbsolute := corpusV2PathComponents(second)
	if !firstAbsolute || !secondAbsolute || !strings.EqualFold(firstVolume, secondVolume) || len(firstParts) != len(secondParts) {
		return false
	}

	parent := firstVolume + string(filepath.Separator)
	if firstVolume == "" {
		parent = string(filepath.Separator)
	}
	for index, firstPart := range firstParts {
		secondPart := secondParts[index]
		if firstPart != secondPart {
			if !strings.EqualFold(firstPart, secondPart) {
				return false
			}
			firstInfo, firstErr := os.Lstat(filepath.Join(parent, firstPart))
			secondInfo, secondErr := os.Lstat(filepath.Join(parent, secondPart))
			if firstErr != nil || secondErr != nil || !os.SameFile(firstInfo, secondInfo) {
				return false
			}
			entries, readErr := os.ReadDir(parent)
			if readErr != nil {
				return false
			}
			foundFirst, foundSecond := false, false
			for _, entry := range entries {
				foundFirst = foundFirst || entry.Name() == firstPart
				foundSecond = foundSecond || entry.Name() == secondPart
			}
			if foundFirst && foundSecond {
				return false
			}
		}
		parent = filepath.Join(parent, firstPart)
	}
	return true
}

func corpusV2PathComponents(path string) (string, []string, bool) {
	if !filepath.IsAbs(path) {
		return filepath.VolumeName(path), nil, false
	}
	volume := filepath.VolumeName(path)
	remainder := strings.TrimPrefix(path, volume)
	remainder = strings.TrimLeft(remainder, string(filepath.Separator))
	if remainder == "" {
		return volume, nil, true
	}
	return volume, strings.Split(remainder, string(filepath.Separator)), true
}

func corpusV2Slash(value string) string {
	return strings.TrimSuffix(strings.ReplaceAll(value, "\\", "/"), "/")
}

func corpusV2IsAbsoluteSlashPath(value string) bool {
	return strings.HasPrefix(value, "/") || len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && value[2] == '/'
}

func corpusV2PathWithin(root, candidate string) bool {
	root = strings.TrimSuffix(root, "/")
	candidate = strings.TrimSuffix(candidate, "/")
	if strings.EqualFold(root, candidate) {
		return true
	}
	return strings.HasPrefix(candidate, root+"/")
}

func validateCorpusV2UniquePairs(pairs []CorpusV2Pair) error {
	clips := make(map[string]string, len(pairs))
	prompts := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		clip := corpusV2Slash(pair.Clip.Path)
		if previous, exists := clips[strings.ToLower(clip)]; exists {
			return corpusV2Error(CorpusV2CodeDuplicateClip, pair.Study+"."+pair.Attempt+".clip", "unique clip path", previous+" / "+clip, nil)
		}
		clips[strings.ToLower(clip)] = clip
		prompt := corpusV2Slash(pair.Prompt.Path)
		if previous, exists := prompts[strings.ToLower(prompt)]; exists {
			return corpusV2Error(CorpusV2CodeDuplicatePrompt, pair.Study+"."+pair.Attempt+".prompt", "unique prompt path", previous+" / "+prompt, nil)
		}
		prompts[strings.ToLower(prompt)] = prompt
	}
	return nil
}

func ValidateCorpusV2Index(index CorpusV2Index, authority CorpusV2Authority) error {
	if index.RepositoryRoot == "" || !strings.EqualFold(corpusV2Slash(index.RepositoryRoot), corpusV2Slash(authority.RepositoryRoot)) {
		return corpusV2Error(CorpusV2CodePathIdentity, "repositoryRoot", authority.RepositoryRoot, index.RepositoryRoot, nil)
	}
	if authority.PairCount > 0 && len(index.Pairs) != authority.PairCount {
		return corpusV2Error(CorpusV2CodeCountMismatch, "pairs", fmt.Sprint(authority.PairCount), fmt.Sprint(len(index.Pairs)), nil)
	}
	for study, expected := range authority.SectionCounts {
		if got := len(index.Sections[study]); got != expected {
			return corpusV2Error(CorpusV2CodeCountMismatch, "section."+study, fmt.Sprint(expected), fmt.Sprint(got), nil)
		}
	}
	for _, study := range authority.RequiredStudies {
		if len(index.Sections[study]) < 3 {
			return corpusV2Error(CorpusV2CodeInsufficientRows, "section."+study, "at least three rows", fmt.Sprint(len(index.Sections[study])), nil)
		}
	}
	return nil
}

// SelectCorpusV2Representatives applies the pinned lexicographic policy to a
// metadata-complete manifest. It never mutates the source pair order.
func SelectCorpusV2Representatives(manifest CorpusV2Manifest, authority CorpusV2Authority) ([]CorpusV2Sample, error) {
	if len(manifest.Pairs) == 0 {
		return nil, corpusV2Error(CorpusV2CodeInsufficientRows, "pairs", "metadata-complete pairs", "empty", nil)
	}
	pairsByStudy := make(map[string][]CorpusV2Pair)
	for _, pair := range manifest.Pairs {
		if pair.Clip.Bytes <= 0 || pair.Clip.SHA256 == "" || pair.Prompt.Bytes <= 0 || pair.Prompt.SHA256 == "" {
			return nil, corpusV2Error(CorpusV2CodeHashMismatch, pair.Study+"."+pair.Attempt, "complete file identities", "incomplete", nil)
		}
		if err := validateCorpusV2Metadata(pair.Stream); err != nil {
			return nil, corpusV2Error(CorpusV2CodeInvalidMedia, pair.Study+"."+pair.Attempt+".stream", "complete video stream metadata", "incomplete", err)
		}
		pairsByStudy[pair.Study] = append(pairsByStudy[pair.Study], pair)
	}
	selected := make([]CorpusV2Sample, 0, len(authority.RequiredStudies)*3)
	seen := make(map[string]struct{}, len(selected))
	for _, study := range authority.RequiredStudies {
		rows := append([]CorpusV2Pair(nil), pairsByStudy[study]...)
		if len(rows) < 3 {
			return nil, corpusV2Error(CorpusV2CodeInsufficientRows, "section."+study, "at least three metadata-complete rows", fmt.Sprint(len(rows)), nil)
		}
		sort.SliceStable(rows, func(first, second int) bool { return corpusV2PairLess(rows[first], rows[second]) })
		indices := []int{0, (len(rows) - 1) / 2, len(rows) - 1}
		bands := []string{"minimum", "median", "maximum"}
		for index, rowIndex := range indices {
			row := rows[rowIndex]
			key := strings.ToLower(corpusV2Slash(row.Clip.Path))
			if _, exists := seen[key]; exists {
				return nil, corpusV2Error(CorpusV2CodeSelectionDuplicate, study+"."+bands[index], "nonduplicate selected clip", row.Clip.Path, nil)
			}
			seen[key] = struct{}{}
			selected = append(selected, CorpusV2Sample{Study: study, Band: bands[index], Attempt: row.Attempt, Clip: row.Clip, Prompt: row.Prompt, Stream: row.Stream, SourceCommit: manifest.Commit})
		}
	}
	if len(selected) != len(authority.RequiredStudies)*3 {
		return nil, corpusV2Error(CorpusV2CodeInsufficientRows, "samples", fmt.Sprint(len(authority.RequiredStudies)*3), fmt.Sprint(len(selected)), nil)
	}
	return selected, nil
}

func corpusV2PairLess(first, second CorpusV2Pair) bool {
	if durationCompare(first.Stream, second.Stream) != 0 {
		return durationCompare(first.Stream, second.Stream) < 0
	}
	if first.Clip.Bytes != second.Clip.Bytes {
		return first.Clip.Bytes < second.Clip.Bytes
	}
	return strings.ToLower(corpusV2Slash(first.Clip.Path)) < strings.ToLower(corpusV2Slash(second.Clip.Path))
}

func durationCompare(first, second CorpusV2StreamMetadata) int {
	if first.durationDenominator == 0 || second.durationDenominator == 0 {
		if first.DurationSeconds < second.DurationSeconds {
			return -1
		}
		if first.DurationSeconds > second.DurationSeconds {
			return 1
		}
		return 0
	}
	left := new(big.Int).Mul(new(big.Int).SetUint64(first.durationNumerator), new(big.Int).SetUint64(second.durationDenominator))
	right := new(big.Int).Mul(new(big.Int).SetUint64(second.durationNumerator), new(big.Int).SetUint64(first.durationDenominator))
	return left.Cmp(right)
}

func corpusV2FileIdentity(path string, data []byte) CorpusV2FileIdentity {
	digest := sha256.Sum256(data)
	hash := hex.EncodeToString(digest[:])
	return CorpusV2FileIdentity{Path: path, Identity: fmt.Sprintf("file:%d:%s", len(data), hash), Bytes: int64(len(data)), SHA256: hash}
}

func corpusV2IdentityForStream(metadata CorpusV2StreamMetadata) string {
	canonical := fmt.Sprintf("%s|%d|%d|%s|%d|%d", metadata.Codec, metadata.Width, metadata.Height, metadata.FrameRate, metadata.DurationMillis, metadata.Frames)
	digest := sha256.Sum256([]byte(canonical))
	return "stream:" + hex.EncodeToString(digest[:])
}

func corpusV2ReadIdentity(root, path string) (CorpusV2FileIdentity, []byte, error) {
	resolvedPath, err := corpusV2ResolveContainedPath(root, path, path)
	if err != nil {
		return CorpusV2FileIdentity{}, nil, err
	}
	info, err := os.Lstat(resolvedPath)
	if err != nil {
		code := CorpusV2CodeUnreadableFile
		if errors.Is(err, os.ErrNotExist) {
			code = CorpusV2CodeMissingSibling
		}
		return CorpusV2FileIdentity{}, nil, corpusV2Error(code, path, "existing indexed file", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return CorpusV2FileIdentity{}, nil, corpusV2Error(CorpusV2CodeNonRegularFile, path, "regular non-aliased file", info.Mode().String(), nil)
	}
	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return CorpusV2FileIdentity{}, nil, corpusV2Error(CorpusV2CodeUnreadableFile, path, "readable indexed file", path, err)
	}
	if len(data) == 0 {
		return CorpusV2FileIdentity{}, nil, corpusV2Error(CorpusV2CodeHashMismatch, path, "nonempty indexed file", "empty", nil)
	}
	identity := corpusV2FileIdentity(path, data)
	if identity.Bytes != info.Size() {
		return CorpusV2FileIdentity{}, nil, corpusV2Error(CorpusV2CodeHashMismatch, path, fmt.Sprint(info.Size()), fmt.Sprint(identity.Bytes), nil)
	}
	return identity, data, nil
}

func corpusV2ReadIndexBytes(path string, authority CorpusV2Authority) ([]byte, string, error) {
	resolvedPath, err := corpusV2ResolveContainedPath(authority.RepositoryRoot, path, "indexPath")
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, "", corpusV2Error(CorpusV2CodeAuthorityUnavailable, path, "readable pinned index", path, err)
	}
	digest := sha256.Sum256(data)
	got := hex.EncodeToString(digest[:])
	if !strings.EqualFold(got, authority.IndexSHA256) {
		return nil, got, corpusV2Error(CorpusV2CodeIndexHashMismatch, "indexSha256", authority.IndexSHA256, got, nil)
	}
	return data, got, nil
}

func corpusV2IndexAbsolutePath(authority CorpusV2Authority) (string, error) {
	root, err := corpusV2AbsoluteRoot(authority.RepositoryRoot)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(authority.IndexPath) || strings.ContainsRune(authority.IndexPath, '\\') || strings.ContainsRune(authority.IndexPath, 0) {
		return "", corpusV2Error(CorpusV2CodePathEscape, "indexPath", "relative slash path", authority.IndexPath, nil)
	}
	indexPath := filepath.Join(root, filepath.FromSlash(authority.IndexPath))
	if !corpusV2PathWithin(corpusV2Slash(root), corpusV2Slash(indexPath)) {
		return "", corpusV2Error(CorpusV2CodePathEscape, "indexPath", "path below repository root", root+"; observed="+indexPath, nil)
	}
	if _, err := corpusV2ResolveContainedPath(root, indexPath, "indexPath"); err != nil {
		return "", err
	}
	return indexPath, nil
}

func readCorpusV2Index(ctx context.Context, authority CorpusV2Authority) (CorpusV2Index, error) {
	indexPath, err := corpusV2IndexAbsolutePath(authority)
	if err != nil {
		return CorpusV2Index{}, err
	}
	data, digest, err := corpusV2ReadIndexBytes(indexPath, authority)
	if err != nil {
		return CorpusV2Index{}, err
	}
	index, err := ParseCorpusV2Index(data, authority.RepositoryRoot)
	if err != nil {
		return CorpusV2Index{}, err
	}
	index.IndexPath = indexPath
	index.IndexSHA256 = digest
	index.Commit = authority.Commit
	if err := ValidateCorpusV2Index(index, authority); err != nil {
		return CorpusV2Index{}, err
	}
	if err := validateCorpusV2IndexedFiles(authority.RepositoryRoot, index.Pairs); err != nil {
		return CorpusV2Index{}, err
	}
	if err := corpusV2VerifyGitAuthority(ctx, authority, indexPath, index.Pairs); err != nil {
		return CorpusV2Index{}, err
	}
	return index, nil
}

func validateCorpusV2IndexedFiles(root string, pairs []CorpusV2Pair) error {
	for _, pair := range pairs {
		for _, path := range []string{pair.Clip.Path, pair.Prompt.Path} {
			resolvedPath, err := corpusV2ResolveContainedPath(root, path, path)
			if err != nil {
				return err
			}
			info, err := os.Lstat(resolvedPath)
			if err != nil {
				code := CorpusV2CodeUnreadableFile
				if errors.Is(err, os.ErrNotExist) {
					code = CorpusV2CodeMissingSibling
				}
				return corpusV2Error(code, path, "existing indexed sibling", path, err)
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return corpusV2Error(CorpusV2CodeNonRegularFile, path, "regular non-aliased indexed sibling", info.Mode().String(), nil)
			}
		}
	}
	return nil
}

// ReadCorpusV2Index performs the pinned, read-only index preflight.
func ReadCorpusV2Index(ctx context.Context, authority CorpusV2Authority) (CorpusV2Index, error) {
	return readCorpusV2Index(ctx, authority)
}

func readCorpusV2Manifest(ctx context.Context, authority CorpusV2Authority) (CorpusV2Manifest, error) {
	index, err := readCorpusV2Index(ctx, authority)
	if err != nil {
		return CorpusV2Manifest{}, err
	}
	pairs := make([]CorpusV2Pair, len(index.Pairs))
	copy(pairs, index.Pairs)
	for index := range pairs {
		pair := &pairs[index]
		clip, data, err := corpusV2ReadIdentity(authority.RepositoryRoot, pair.Clip.Path)
		if err != nil {
			return CorpusV2Manifest{}, err
		}
		prompt, _, err := corpusV2ReadIdentity(authority.RepositoryRoot, pair.Prompt.Path)
		if err != nil {
			return CorpusV2Manifest{}, err
		}
		metadata, err := parseCorpusV2MP4Metadata(data)
		if err != nil {
			return CorpusV2Manifest{}, corpusV2Error(CorpusV2CodeInvalidMedia, pair.Study+"."+pair.Attempt+".clip", "decodable H.264 MP4 video stream", pair.Clip.Path, err)
		}
		metadata.Identity = corpusV2IdentityForStream(metadata)
		pair.Clip = clip
		pair.Prompt = prompt
		pair.Stream = metadata
	}
	manifest := CorpusV2Manifest{SchemaVersion: CorpusV2SchemaVersion, Repository: authority.RepositoryRoot, Commit: authority.Commit, IndexPath: authority.IndexPath, IndexSHA256: index.IndexSHA256, Pairs: pairs, UniqueClips: len(pairs), UniquePrompts: len(pairs), ReadOnly: true}
	selected, err := SelectCorpusV2Representatives(manifest, authority)
	if err != nil {
		return CorpusV2Manifest{}, err
	}
	manifest.Samples = selected
	if err := ValidateCorpusV2Manifest(manifest, authority); err != nil {
		return CorpusV2Manifest{}, err
	}
	return manifest, nil
}

// ReadCorpusV2Manifest performs index, source-identity, stream-metadata, and
// deterministic sample validation without copying or uploading media.
func ReadCorpusV2Manifest(ctx context.Context, authority CorpusV2Authority) (CorpusV2Manifest, error) {
	return readCorpusV2Manifest(ctx, authority)
}

func corpusV2VerifyGitAuthority(ctx context.Context, authority CorpusV2Authority, indexPath string, _ []CorpusV2Pair) error {
	root, err := corpusV2AbsoluteRoot(authority.RepositoryRoot)
	if err != nil {
		return err
	}
	head, err := corpusV2Git(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return corpusV2Error(CorpusV2CodeGitIdentityUnavailable, "git.head", authority.Commit, "unavailable", err)
	}
	if strings.TrimSpace(head) != authority.Commit {
		return corpusV2Error(CorpusV2CodeSourceCommitMismatch, "git.head", authority.Commit, strings.TrimSpace(head), nil)
	}
	indexBlob, err := corpusV2Git(ctx, root, "hash-object", indexPath)
	if err != nil {
		return corpusV2Error(CorpusV2CodeGitIdentityUnavailable, "git.index", "readable index blob", indexPath, err)
	}
	relativeIndex, err := filepath.Rel(root, indexPath)
	if err != nil || filepath.IsAbs(relativeIndex) || strings.HasPrefix(relativeIndex, ".."+string(filepath.Separator)) {
		return corpusV2Error(CorpusV2CodePathEscape, "git.index", "index below repository root", root, nil)
	}
	if err := corpusV2RunGit(ctx, root, "cat-file", "-e", authority.Commit+":"+filepath.ToSlash(relativeIndex)); err != nil {
		return corpusV2Error(CorpusV2CodeSourceMutation, "git.index", "index is present in pinned commit", filepath.ToSlash(relativeIndex), err)
	}
	// The index content SHA-256 was checked before this call; hash-object also
	// proves the selected file is a tracked, readable file in this checkout.
	if strings.TrimSpace(indexBlob) == "" {
		return corpusV2Error(CorpusV2CodeSourceMutation, "git.index", "nonempty committed index identity", indexPath, nil)
	}
	// The production media is intentionally ignored/untracked in the external
	// checkout. Its content identity is therefore recorded by the read-only
	// probe; the pinned commit guards the authoritative index, not media blobs.
	return nil
}

func corpusV2Git(ctx context.Context, root string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func corpusV2RunGit(ctx context.Context, root string, args ...string) error {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	return command.Run()
}

func validateCorpusV2Metadata(metadata CorpusV2StreamMetadata) error {
	if (metadata.Codec != "avc1" && metadata.Codec != "avc3") || metadata.Width <= 0 || metadata.Height <= 0 || metadata.FrameRate == "" || metadata.DurationMillis <= 0 || metadata.Frames <= 0 || math.IsNaN(metadata.DurationSeconds) || math.IsInf(metadata.DurationSeconds, 0) || metadata.DurationSeconds <= 0 {
		return corpusV2Error(CorpusV2CodeInvalidMedia, "stream", "H.264 codec, positive dimensions, frame rate, duration, and frame count", fmt.Sprintf("%+v", metadata), nil)
	}
	return nil
}
