package omni_media_probe

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	ManifestSchemaV1 = "you.localai.omni-media-fixture.v1"

	CodeInvalidJSON        ValidationCode = "invalid_json"
	CodeUnknownField       ValidationCode = "unknown_field"
	CodeDuplicateJSONKey   ValidationCode = "duplicate_json_key"
	CodeInvalidSchema      ValidationCode = "invalid_schema"
	CodeInvalidArtifact    ValidationCode = "invalid_artifact"
	CodeDuplicateArtifact  ValidationCode = "duplicate_artifact"
	CodePathEscape         ValidationCode = "path_escape"
	CodePathAlias          ValidationCode = "path_alias"
	CodeMissingFile        ValidationCode = "missing_file"
	CodeNonRegularFile     ValidationCode = "non_regular_file"
	CodeUnreadableFile     ValidationCode = "unreadable_file"
	CodeSizeMismatch       ValidationCode = "size_mismatch"
	CodeHashMismatch       ValidationCode = "hash_mismatch"
	CodeInvalidMedia       ValidationCode = "invalid_media"
	CodeMetadataMismatch   ValidationCode = "metadata_mismatch"
	CodeProvenanceMismatch ValidationCode = "provenance_mismatch"
	CodeRubricAmbiguous    ValidationCode = "rubric_ambiguity"
	CodeInvalidPhaseRange  ValidationCode = "invalid_phase_range"
)

// ValidationCode identifies a fail-closed manifest or fixture conformance
// result. It is intentionally independent of production model errors because
// this package owns only the immutable test bundle.
type ValidationCode string

// ValidationError is the typed failure returned before a caller can use an
// invalid fixture as a probe input.
type ValidationError struct {
	Code     ValidationCode
	Field    string
	Expected string
	Observed string
	Cause    error
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	detail := ""
	if e.Expected != "" || e.Observed != "" {
		detail = fmt.Sprintf(" (expected %q, observed %q)", e.Expected, e.Observed)
	}
	if e.Cause != nil {
		return fmt.Sprintf("omni media %s at %s%s: %v", e.Code, e.Field, detail, e.Cause)
	}
	return fmt.Sprintf("omni media %s at %s%s", e.Code, e.Field, detail)
}

func (e *ValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Provenance records the immutable source identity of one promoted fixture.
type Provenance struct {
	Repository string `json:"repository"`
	Revision   string `json:"revision"`
	SourcePath string `json:"sourcePath"`
	GitBlob    string `json:"gitBlob"`
}

type ImageMetadata struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type VideoMetadata struct {
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	DurationMillis int64  `json:"durationMillis"`
	FrameRate      string `json:"frameRate"`
	Frames         int64  `json:"frames"`
}

// ImagePredicate retains the exact JSON value so future probe code can carry
// the rubric without inventing a second semantic representation.
type ImagePredicate struct {
	Kind  string
	Exact json.RawMessage
}

type ImageRubric struct {
	AllOf []ImagePredicate
}

type VideoPhase struct {
	Label              string `json:"label"`
	Background         string `json:"background"`
	TextColor          string `json:"textColor"`
	StartMillis        int64  `json:"startMillis"`
	EndMillisExclusive int64  `json:"endMillisExclusive"`
}

type VideoTransition struct {
	Kind            string `json:"kind"`
	AtMillis        int64  `json:"atMillis"`
	ToleranceMillis int64  `json:"toleranceMillis"`
}

type VideoRubric struct {
	OrderedPhases []VideoPhase
	Transition    *VideoTransition
}

// Rubric is the media-specific union carried by an Artifact. Exactly one
// branch is populated after strict decoding.
type Rubric struct {
	Image *ImageRubric
	Video *VideoRubric
}

type Artifact struct {
	ID             string
	Path           string
	MediaType      string
	Bytes          int64
	SHA256         string
	Provenance     Provenance
	Image          *ImageMetadata
	Video          *VideoMetadata
	SemanticRubric Rubric

	// ResolvedPath is populated only by LoadManifest. It is not part of the
	// JSON contract and remains inside the test-only package boundary.
	ResolvedPath string `json:"-"`
}

type Manifest struct {
	SchemaVersion string
	Artifacts     []Artifact

	ManifestPath string `json:"-"`
	Root         string `json:"-"`
}

type manifestWire struct {
	SchemaVersion string         `json:"schemaVersion"`
	Artifacts     []artifactWire `json:"artifacts"`
}

type artifactWire struct {
	ID             string          `json:"id"`
	Path           string          `json:"path"`
	MediaType      string          `json:"mediaType"`
	Bytes          int64           `json:"bytes"`
	SHA256         string          `json:"sha256"`
	Provenance     Provenance      `json:"provenance"`
	Image          *ImageMetadata  `json:"image"`
	Video          *VideoMetadata  `json:"video"`
	SemanticRubric json.RawMessage `json:"semanticRubric"`
}

type imageRubricWire struct {
	AllOf []imagePredicateWire `json:"allOf"`
}

type imagePredicateWire struct {
	Kind  string          `json:"kind"`
	Exact json.RawMessage `json:"exact"`
}

type videoRubricWire struct {
	OrderedPhases []VideoPhase     `json:"orderedPhases"`
	Transition    *VideoTransition `json:"transition"`
}

const (
	wantImageID        = "omni-image-v1"
	wantImagePath      = "infinite-you.png"
	wantImageMediaType = "image/png"
	wantImageBytes     = int64(1381559)
	wantImageSHA256    = "6d8f7075d2314a19be2a5b8fe52ffc44f2ab8c0fcc5b68935abbd57788e8c3d8"
	wantImageSource    = "examples/factories/infinite-you.png"
	wantImageBlob      = "06c7e71f4e2cb4c04ce72bfd6857a99652090547"

	wantVideoID        = "omni-video-v1"
	wantVideoPath      = "groundtruth-fixture.mp4"
	wantVideoMediaType = "video/mp4"
	wantVideoBytes     = int64(30039)
	wantVideoSHA256    = "80db7ed6a38cb8de371ef6e732d317102b67b54977d7b0d535a7862f0f05e9b7"
	wantVideoSource    = "tests/functional/providers/agy/testdata/groundtruth-fixture.mp4"
	wantVideoBlob      = "0190b56eccf8c4cb8c4fb8cd3ffd28c2f1420c20"
	wantRepository     = "https://github.com/portpowered/infinite-you.git"
	wantRevision       = "ff194dc02cb66e208988f918bca7386344236c52"
)

// ReadManifest reads and validates only the declarative manifest. It performs
// no fixture or probe effect, and rejects path escape before any artifact is
// opened.
func ReadManifest(path string) (Manifest, error) {
	manifestPath, root, err := normalizeManifestPath(path)
	if err != nil {
		return Manifest{}, err
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		code := CodeUnreadableFile
		if errors.Is(err, os.ErrNotExist) {
			code = CodeMissingFile
		}
		return Manifest{}, validationError(code, "manifest", "readable manifest file", manifestPath, err)
	}

	var wire manifestWire
	if err := decodeStrictJSON(data, &wire); err != nil {
		var typed *ValidationError
		if errors.As(err, &typed) {
			return Manifest{}, err
		}
		code := CodeInvalidJSON
		if strings.Contains(err.Error(), "json: unknown field") {
			code = CodeUnknownField
		}
		return Manifest{}, validationError(code, "manifest", "one strict JSON value", "invalid", err)
	}

	manifest, err := convertManifest(wire)
	if err != nil {
		return Manifest{}, err
	}
	manifest.ManifestPath = manifestPath
	manifest.Root = root
	if err := validateManifestShape(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// LoadManifest performs full local-real asset conformance. The returned
// manifest contains resolved paths only after containment, identity, and
// media metadata all pass.
func LoadManifest(path string) (Manifest, error) {
	manifest, err := ReadManifest(path)
	if err != nil {
		return Manifest{}, err
	}
	for index := range manifest.Artifacts {
		artifact := &manifest.Artifacts[index]
		resolved, err := resolveArtifactPath(manifest.Root, artifact.Path)
		if err != nil {
			return Manifest{}, fmt.Errorf("artifact %d: %w", index, err)
		}
		if err := validateArtifactBytes(*artifact, resolved); err != nil {
			return Manifest{}, fmt.Errorf("artifact %d: %w", index, err)
		}
		artifact.ResolvedPath = resolved
	}
	return manifest, nil
}

// ValidateManifest is an explicit alias for callers that want the conformance
// operation's intent to be visible at the call site.
func ValidateManifest(path string) (Manifest, error) {
	return LoadManifest(path)
}

func normalizeManifestPath(path string) (string, string, error) {
	if strings.TrimSpace(path) == "" || strings.ContainsRune(path, 0) {
		return "", "", validationError(CodeInvalidArtifact, "manifest.path", "non-empty path", path, nil)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", validationError(CodeInvalidArtifact, "manifest.path", "absolute path", path, err)
	}
	abs = filepath.Clean(abs)
	if !filepath.IsAbs(abs) {
		return "", "", validationError(CodeInvalidArtifact, "manifest.path", "absolute path", abs, nil)
	}
	return abs, filepath.Dir(abs), nil
}

func convertManifest(wire manifestWire) (Manifest, error) {
	manifest := Manifest{SchemaVersion: wire.SchemaVersion, Artifacts: make([]Artifact, 0, len(wire.Artifacts))}
	for index, raw := range wire.Artifacts {
		rubric, err := decodeRubric(raw.MediaType, raw.SemanticRubric, fmt.Sprintf("artifacts[%d].semanticRubric", index))
		if err != nil {
			return Manifest{}, err
		}
		manifest.Artifacts = append(manifest.Artifacts, Artifact{
			ID: raw.ID, Path: raw.Path, MediaType: raw.MediaType, Bytes: raw.Bytes,
			SHA256: raw.SHA256, Provenance: raw.Provenance, Image: raw.Image,
			Video: raw.Video, SemanticRubric: rubric,
		})
	}
	return manifest, nil
}

func validateManifestShape(manifest Manifest) error {
	if manifest.SchemaVersion != ManifestSchemaV1 {
		return validationError(CodeInvalidSchema, "schemaVersion", ManifestSchemaV1, manifest.SchemaVersion, nil)
	}
	if len(manifest.Artifacts) != 2 {
		return validationError(CodeInvalidArtifact, "artifacts", "exactly two artifacts", fmt.Sprint(len(manifest.Artifacts)), nil)
	}
	seenIDs := make(map[string]struct{}, len(manifest.Artifacts))
	seenPaths := make(map[string]struct{}, len(manifest.Artifacts))
	for index, artifact := range manifest.Artifacts {
		field := fmt.Sprintf("artifacts[%d]", index)
		if artifact.ID == "" || artifact.Path == "" || artifact.MediaType == "" || artifact.Bytes <= 0 || artifact.SHA256 == "" {
			return validationError(CodeInvalidArtifact, field, "complete artifact identity", "incomplete", nil)
		}
		if _, exists := seenIDs[artifact.ID]; exists {
			return validationError(CodeDuplicateArtifact, field+".id", "unique artifact id", artifact.ID, nil)
		}
		seenIDs[artifact.ID] = struct{}{}
		if _, exists := seenPaths[artifact.Path]; exists {
			return validationError(CodeDuplicateArtifact, field+".path", "unique artifact path", artifact.Path, nil)
		}
		seenPaths[artifact.Path] = struct{}{}
		if _, err := containedArtifactPath(manifest.Root, artifact.Path); err != nil {
			return fmt.Errorf("%s: %w", field, err)
		}
		if err := validateCanonicalArtifact(index, artifact); err != nil {
			return err
		}
	}
	return nil
}

func validateCanonicalArtifact(index int, artifact Artifact) error {
	field := fmt.Sprintf("artifacts[%d]", index)
	wantID, wantPath, wantMedia, wantBytes, wantHash, wantSource, wantBlob := "", "", "", int64(0), "", "", ""
	switch index {
	case 0:
		wantID, wantPath, wantMedia, wantBytes, wantHash = wantImageID, wantImagePath, wantImageMediaType, wantImageBytes, wantImageSHA256
		wantSource, wantBlob = wantImageSource, wantImageBlob
	case 1:
		wantID, wantPath, wantMedia, wantBytes, wantHash = wantVideoID, wantVideoPath, wantVideoMediaType, wantVideoBytes, wantVideoSHA256
		wantSource, wantBlob = wantVideoSource, wantVideoBlob
	default:
		return validationError(CodeInvalidArtifact, field, "known artifact index", fmt.Sprint(index), nil)
	}
	if artifact.ID != wantID || artifact.Path != wantPath || artifact.MediaType != wantMedia || artifact.Bytes != wantBytes || artifact.SHA256 != wantHash {
		return validationError(CodeInvalidArtifact, field, "pinned id/path/media/size/hash", fmt.Sprintf("%s/%s/%s/%d/%s", artifact.ID, artifact.Path, artifact.MediaType, artifact.Bytes, artifact.SHA256), nil)
	}
	if artifact.Provenance != (Provenance{Repository: wantRepository, Revision: wantRevision, SourcePath: wantSource, GitBlob: wantBlob}) {
		return validationError(CodeProvenanceMismatch, field+".provenance", "pinned repository/revision/source/blob", fmt.Sprintf("%+v", artifact.Provenance), nil)
	}
	if index == 0 {
		if artifact.Image == nil || artifact.Video != nil || *artifact.Image != (ImageMetadata{Width: 897, Height: 1672}) {
			return validationError(CodeMetadataMismatch, field+".image", "PNG 897x1672 and no video metadata", "drifted", nil)
		}
		if artifact.SemanticRubric.Image == nil || artifact.SemanticRubric.Video != nil {
			return validationError(CodeRubricAmbiguous, field+".semanticRubric", "one image rubric", "wrong media rubric", nil)
		}
		return nil
	}
	if artifact.Video == nil || artifact.Image != nil || *artifact.Video != (VideoMetadata{Width: 320, Height: 240, DurationMillis: 4000, FrameRate: "25/1", Frames: 100}) {
		return validationError(CodeMetadataMismatch, field+".video", "MP4 320x240/4000ms/25fps/100 frames", "drifted", nil)
	}
	if artifact.SemanticRubric.Video == nil || artifact.SemanticRubric.Image != nil {
		return validationError(CodeRubricAmbiguous, field+".semanticRubric", "one video rubric", "wrong media rubric", nil)
	}
	return nil
}

func validateArtifactBytes(artifact Artifact, path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		code := CodeUnreadableFile
		if errors.Is(err, os.ErrNotExist) {
			code = CodeMissingFile
		}
		return validationError(code, artifact.Path, "existing regular fixture", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return validationError(CodePathAlias, artifact.Path, "non-aliased regular fixture", path, nil)
	}
	if !info.Mode().IsRegular() {
		return validationError(CodeNonRegularFile, artifact.Path, "regular fixture", info.Mode().String(), nil)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return validationError(CodeUnreadableFile, artifact.Path, "readable fixture", path, err)
	}
	if int64(len(data)) != artifact.Bytes {
		return validationError(CodeSizeMismatch, artifact.Path+".bytes", fmt.Sprint(artifact.Bytes), fmt.Sprint(len(data)), nil)
	}
	digest := sha256.Sum256(data)
	gotHash := hex.EncodeToString(digest[:])
	if gotHash != artifact.SHA256 {
		return validationError(CodeHashMismatch, artifact.Path+".sha256", artifact.SHA256, gotHash, nil)
	}

	switch artifact.MediaType {
	case wantImageMediaType:
		config, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || format != "png" {
			return validationError(CodeInvalidMedia, artifact.Path, "decodable PNG", format, err)
		}
		if artifact.Image == nil {
			return validationError(CodeMetadataMismatch, artifact.Path, "declared PNG dimensions", "missing image metadata", nil)
		}
		if config.Width != artifact.Image.Width || config.Height != artifact.Image.Height {
			return validationError(CodeMetadataMismatch, artifact.Path, fmt.Sprintf("PNG %dx%d", artifact.Image.Width, artifact.Image.Height), fmt.Sprintf("PNG %dx%d", config.Width, config.Height), nil)
		}
	case wantVideoMediaType:
		metadata, err := parseMP4Metadata(data)
		if err != nil {
			return validationError(CodeInvalidMedia, artifact.Path, "valid ISO-BMFF MP4 with H.264/AAC tracks", "unreadable", err)
		}
		if artifact.Video == nil {
			return validationError(CodeMetadataMismatch, artifact.Path, "declared MP4 metadata", "missing video metadata", nil)
		}
		if metadata != *artifact.Video {
			return validationError(CodeMetadataMismatch, artifact.Path, fmt.Sprintf("%+v", *artifact.Video), fmt.Sprintf("%+v", metadata), nil)
		}
	default:
		return validationError(CodeInvalidMedia, artifact.Path+".mediaType", "image/png or video/mp4", artifact.MediaType, nil)
	}
	return nil
}

func resolveArtifactPath(root, declared string) (string, error) {
	candidate, err := containedArtifactPath(root, declared)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(candidate)
	if err != nil {
		code := CodeUnreadableFile
		if errors.Is(err, os.ErrNotExist) {
			code = CodeMissingFile
		}
		return "", validationError(code, declared, "existing fixture below manifest root", candidate, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, resolveErr := filepath.EvalSymlinks(candidate)
		if resolveErr != nil {
			return "", validationError(CodePathAlias, declared, "resolvable non-aliased fixture", candidate, resolveErr)
		}
		if !pathWithin(root, resolved) {
			return "", validationError(CodePathEscape, declared, "resolved fixture below manifest root", resolved, nil)
		}
		return "", validationError(CodePathAlias, declared, "non-aliased regular fixture", resolved, nil)
	}
	if !info.Mode().IsRegular() {
		return "", validationError(CodeNonRegularFile, declared, "regular fixture", info.Mode().String(), nil)
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", validationError(CodeUnreadableFile, declared, "resolvable fixture", candidate, err)
	}
	if !pathWithin(root, resolved) {
		return "", validationError(CodePathEscape, declared, "resolved fixture below manifest root", resolved, nil)
	}
	return candidate, nil
}

func containedArtifactPath(root, declared string) (string, error) {
	if root == "" || strings.TrimSpace(declared) != declared || declared == "" || strings.ContainsRune(declared, 0) || filepath.IsAbs(declared) {
		return "", validationError(CodePathEscape, "path", "relative path below manifest root", declared, nil)
	}
	clean := filepath.Clean(declared)
	if clean == "." || filepath.IsAbs(clean) {
		return "", validationError(CodePathEscape, "path", "relative file path below manifest root", declared, nil)
	}
	candidate := filepath.Join(root, clean)
	if !pathWithin(root, candidate) {
		return "", validationError(CodePathEscape, "path", "path below manifest root", declared, nil)
	}
	return candidate, nil
}

func pathWithin(root, candidate string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(rootAbs), filepath.Clean(candidateAbs))
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func decodeRubric(mediaType string, raw json.RawMessage, field string) (Rubric, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return Rubric{}, validationError(CodeRubricAmbiguous, field, "one complete media rubric", "missing or null", nil)
	}
	switch mediaType {
	case wantImageMediaType:
		var wire imageRubricWire
		if err := decodeStrictJSON(raw, &wire); err != nil {
			return Rubric{}, rubricDecodeError(field, err)
		}
		if len(wire.AllOf) != 3 {
			return Rubric{}, validationError(CodeRubricAmbiguous, field+".allOf", "exactly three predicates", fmt.Sprint(len(wire.AllOf)), nil)
		}
		seen := make(map[string]struct{}, len(wire.AllOf))
		predicates := make([]ImagePredicate, 0, len(wire.AllOf))
		wantKinds := []string{"symbol", "text", "accentColors"}
		for index, predicate := range wire.AllOf {
			predicateField := fmt.Sprintf("%s.allOf[%d]", field, index)
			if _, exists := seen[predicate.Kind]; exists || predicate.Kind == "" || predicate.Kind != wantKinds[index] {
				return Rubric{}, validationError(CodeRubricAmbiguous, predicateField+".kind", "one occurrence of each required predicate", predicate.Kind, nil)
			}
			seen[predicate.Kind] = struct{}{}
			if err := validateImagePredicate(predicate, predicateField); err != nil {
				return Rubric{}, err
			}
			predicates = append(predicates, ImagePredicate{Kind: predicate.Kind, Exact: append(json.RawMessage(nil), predicate.Exact...)})
		}
		return Rubric{Image: &ImageRubric{AllOf: predicates}}, nil
	case wantVideoMediaType:
		var wire videoRubricWire
		if err := decodeStrictJSON(raw, &wire); err != nil {
			return Rubric{}, rubricDecodeError(field, err)
		}
		if len(wire.OrderedPhases) != 2 || wire.Transition == nil {
			return Rubric{}, validationError(CodeRubricAmbiguous, field, "two phases and one transition", "incomplete", nil)
		}
		wantPhases := expectedVideoPhases()
		seenLabels := make(map[string]struct{}, len(wire.OrderedPhases))
		for index, phase := range wire.OrderedPhases {
			phaseField := fmt.Sprintf("%s.orderedPhases[%d]", field, index)
			if _, exists := seenLabels[phase.Label]; exists || phase.Label == "" {
				return Rubric{}, validationError(CodeRubricAmbiguous, phaseField+".label", "unique non-empty phase labels", phase.Label, nil)
			}
			seenLabels[phase.Label] = struct{}{}
			if phase.StartMillis < 0 || phase.EndMillisExclusive <= phase.StartMillis || (index > 0 && phase.StartMillis != wire.OrderedPhases[index-1].EndMillisExclusive) {
				return Rubric{}, validationError(CodeInvalidPhaseRange, phaseField, "ordered, contiguous positive phase range", fmt.Sprintf("%d..%d", phase.StartMillis, phase.EndMillisExclusive), nil)
			}
			if phase.StartMillis != wantPhases[index].StartMillis || phase.EndMillisExclusive != wantPhases[index].EndMillisExclusive {
				return Rubric{}, validationError(CodeInvalidPhaseRange, phaseField, "pinned ordered phase range", fmt.Sprintf("%d..%d", phase.StartMillis, phase.EndMillisExclusive), nil)
			}
			if phase != wantPhases[index] {
				return Rubric{}, validationError(CodeRubricAmbiguous, phaseField, "pinned phase label/colors/range", fmt.Sprintf("%+v", phase), nil)
			}
		}
		if wire.Transition.Kind == "" || wire.Transition.AtMillis < 0 || wire.Transition.ToleranceMillis < 0 {
			return Rubric{}, validationError(CodeRubricAmbiguous, field+".transition", "complete transition predicate", "incomplete", nil)
		}
		if *wire.Transition != (VideoTransition{Kind: "hardCut", AtMillis: 2000, ToleranceMillis: 40}) {
			return Rubric{}, validationError(CodeRubricAmbiguous, field+".transition", "pinned hard-cut transition", fmt.Sprintf("%+v", *wire.Transition), nil)
		}
		return Rubric{Video: &VideoRubric{OrderedPhases: append([]VideoPhase(nil), wire.OrderedPhases...), Transition: wire.Transition}}, nil
	default:
		return Rubric{}, validationError(CodeInvalidArtifact, field, "supported media type before rubric decode", mediaType, nil)
	}
}

func expectedVideoPhases() []VideoPhase {
	return []VideoPhase{
		{Label: "PHASE 1", Background: "red", TextColor: "white", StartMillis: 0, EndMillisExclusive: 2000},
		{Label: "PHASE 2", Background: "blue", TextColor: "yellow", StartMillis: 2000, EndMillisExclusive: 4000},
	}
}

func validateImagePredicate(predicate imagePredicateWire, field string) error {
	if len(predicate.Exact) == 0 || bytes.Equal(bytes.TrimSpace(predicate.Exact), []byte("null")) {
		return validationError(CodeRubricAmbiguous, field+".exact", "one exact predicate value", "missing or null", nil)
	}
	switch predicate.Kind {
	case "symbol":
		var exact string
		if err := decodeStrictJSON(predicate.Exact, &exact); err != nil || exact != "infinity" {
			return validationError(CodeRubricAmbiguous, field, "symbol exact infinity", string(predicate.Exact), err)
		}
	case "text":
		var exact string
		if err := decodeStrictJSON(predicate.Exact, &exact); err != nil || exact != "INFINITE YOU" {
			return validationError(CodeRubricAmbiguous, field, "text exact INFINITE YOU", string(predicate.Exact), err)
		}
	case "accentColors":
		var exact []string
		if err := decodeStrictJSON(predicate.Exact, &exact); err != nil || len(exact) != 2 || exact[0] != "blue" || exact[1] != "gold" {
			return validationError(CodeRubricAmbiguous, field, "accentColors exact [blue,gold]", string(predicate.Exact), err)
		}
	default:
		return validationError(CodeRubricAmbiguous, field+".kind", "symbol, text, or accentColors", predicate.Kind, nil)
	}
	return nil
}

func rubricDecodeError(field string, err error) error {
	var typed *ValidationError
	if errors.As(err, &typed) {
		if typed.Code == CodeDuplicateJSONKey {
			return err
		}
	}
	code := CodeInvalidJSON
	if strings.Contains(err.Error(), "json: unknown field") {
		code = CodeUnknownField
	}
	return validationError(code, field, "strict rubric JSON", "invalid", err)
}

func validationError(code ValidationCode, field, expected, observed string, cause error) *ValidationError {
	return &ValidationError{Code: code, Field: field, Expected: expected, Observed: observed, Cause: cause}
}

func decodeStrictJSON(data []byte, destination any) error {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return err
	}
	return nil
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := walkJSONValue(decoder, "$", 0); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return validationError(CodeInvalidJSON, "$", "one JSON value", "trailing value", nil)
		}
		return err
	}
	return nil
}

func walkJSONValue(decoder *json.Decoder, field string, depth int) error {
	if depth > 64 {
		return validationError(CodeInvalidJSON, field, "JSON nesting no deeper than 64 levels", "too deep", nil)
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return validationError(CodeDuplicateJSONKey, field+"."+key, "unique JSON object key", "duplicate", nil)
			}
			seen[key] = struct{}{}
			if err := walkJSONValue(decoder, field+"."+key, depth+1); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return errors.New("object is not closed")
		}
	case '[':
		index := 0
		for decoder.More() {
			if err := walkJSONValue(decoder, fmt.Sprintf("%s[%d]", field, index), depth+1); err != nil {
				return err
			}
			index++
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return errors.New("array is not closed")
		}
	default:
		return errors.New("unexpected JSON delimiter")
	}
	return nil
}
