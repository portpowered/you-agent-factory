package factorydefinitions

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	distributionpackageassets "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/packageassets"
	"gopkg.in/yaml.v3"
)

// PackagedFactoryFormat identifies the authored root representation selected
// when a published Factory is materialized.
type PackagedFactoryFormat string

const (
	PackagedFactoryFormatJSON PackagedFactoryFormat = "JSON"
	PackagedFactoryFormatYAML PackagedFactoryFormat = "YAML"
	PackagedFactoryFormatYML  PackagedFactoryFormat = "YML"
)

// PackagedDefinition is one detached Factory Definition shipped with the
// executable. JSON and YAML retain their published source bytes, while
// Integrity declares the byte-level representation and bundled-artifact facts
// that must hold before the package can be materialized.
type PackagedDefinition struct {
	Name      string
	Project   string
	JSON      []byte
	YAML      []byte
	Formats   []PackagedFactoryFormat
	Integrity PackagedFactoryIntegrity
}

// PackagedFactoryIntegrity declares the exact published root representations
// and bundled artifacts for one package. It contains digests only; callers do
// not receive package locators or implementation-owned filesystem state.
type PackagedFactoryIntegrity struct {
	JSONSHA256   string
	YAMLSHA256   string
	BundledFiles []PackagedFactoryArtifactIntegrity
}

// PackagedFactoryArtifactIntegrity identifies one bundled Factory artifact by
// its Factory-relative target path and exact content digest.
type PackagedFactoryArtifactIntegrity struct {
	TargetPath string
	SHA256     string
}

// BuildPackagedFactoryIntegrity derives detached representation and bundled
// artifact digests from published source bytes. Package publication uses this
// pure helper after validating its generated manifest.
func BuildPackagedFactoryIntegrity(
	jsonPayload []byte,
	yamlPayload []byte,
) (PackagedFactoryIntegrity, error) {
	integrity := PackagedFactoryIntegrity{
		JSONSHA256: digestPackagedFactoryBytes(jsonPayload),
		YAMLSHA256: digestPackagedFactoryBytes(yamlPayload),
	}
	canonical, err := canonicalPackagedFactoryJSON(jsonPayload, yamlPayload)
	if err != nil {
		return PackagedFactoryIntegrity{}, err
	}
	artifacts, err := packagedFactoryArtifacts(canonical)
	if err != nil {
		return PackagedFactoryIntegrity{}, err
	}
	integrity.BundledFiles = artifacts
	return integrity, nil
}

// VerifyPackagedFactoryIntegrity verifies declared representation and bundled
// artifact content before a package may mutate its destination. It is pure and
// leaves source buffers untouched.
func VerifyPackagedFactoryIntegrity(
	definition PackagedDefinition,
	format PackagedFactoryFormat,
) error {
	integrity := definition.Integrity
	if integrity.JSONSHA256 != "" {
		if err := verifyPackagedFactoryDigest(definition.JSON, integrity.JSONSHA256); err != nil {
			return packagedIntegrityError(definition.Name, format, "", err)
		}
	}
	if integrity.YAMLSHA256 != "" {
		if err := verifyPackagedFactoryDigest(definition.YAML, integrity.YAMLSHA256); err != nil {
			return packagedIntegrityError(definition.Name, format, "", err)
		}
	}
	if len(integrity.BundledFiles) == 0 {
		return nil
	}
	canonical, err := canonicalPackagedFactoryJSON(definition.JSON, definition.YAML)
	if err != nil {
		return NewPackagedFactoryInputError(
			PackagedFactoryErrorMalformed,
			definition.Name,
			format,
			"",
			err,
		)
	}
	actual, err := packagedFactoryArtifacts(canonical)
	if err != nil {
		return NewPackagedFactoryInputError(
			PackagedFactoryErrorMalformed,
			definition.Name,
			format,
			"",
			err,
		)
	}
	declared := make(map[string]string, len(integrity.BundledFiles))
	for _, artifact := range integrity.BundledFiles {
		if err := validatePackagedFactoryArtifactPath(artifact.TargetPath); err != nil {
			return NewPackagedFactoryInputError(
				PackagedFactoryErrorMalformed,
				definition.Name,
				format,
				artifact.TargetPath,
				err,
			)
		}
		if _, exists := declared[artifact.TargetPath]; exists {
			return NewPackagedFactoryInputError(
				PackagedFactoryErrorMalformed,
				definition.Name,
				format,
				artifact.TargetPath,
				fmt.Errorf("duplicate declared bundled artifact"),
			)
		}
		declared[artifact.TargetPath] = artifact.SHA256
	}
	if len(declared) != len(actual) {
		return packagedIntegrityError(definition.Name, format, "", fmt.Errorf("declared bundled artifact set differs"))
	}
	for _, artifact := range actual {
		expected, exists := declared[artifact.TargetPath]
		if !exists || !packagedFactoryDigestEqual(expected, artifact.SHA256) {
			return packagedIntegrityError(
				definition.Name,
				format,
				artifact.TargetPath,
				fmt.Errorf("declared bundled artifact digest differs"),
			)
		}
	}
	return nil
}

func packagedIntegrityError(
	name string,
	format PackagedFactoryFormat,
	artifact string,
	cause error,
) error {
	return NewPackagedFactoryInputError(
		PackagedFactoryErrorIntegrity,
		name,
		format,
		artifact,
		cause,
	)
}

func digestPackagedFactoryBytes(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func verifyPackagedFactoryDigest(payload []byte, declared string) error {
	if len(payload) == 0 {
		return fmt.Errorf("declared representation is missing")
	}
	if len(declared) != sha256.Size*2 {
		return fmt.Errorf("declared digest is not a SHA-256 value")
	}
	if _, err := hex.DecodeString(declared); err != nil {
		return fmt.Errorf("declared digest is not hexadecimal")
	}
	if !packagedFactoryDigestEqual(declared, digestPackagedFactoryBytes(payload)) {
		return fmt.Errorf("declared digest differs from package content")
	}
	return nil
}

func packagedFactoryDigestEqual(left string, right string) bool {
	if len(left) != len(right) {
		return false
	}
	return strings.EqualFold(left, right)
}

type packagedArtifactPayload struct {
	SupportingFiles struct {
		BundledFiles []struct {
			TargetPath string `json:"targetPath"`
			Content    struct {
				Inline string `json:"inline"`
			} `json:"content"`
		} `json:"bundledFiles"`
	} `json:"supportingFiles"`
}

func packagedFactoryArtifacts(canonical []byte) ([]PackagedFactoryArtifactIntegrity, error) {
	var payload packagedArtifactPayload
	if err := json.Unmarshal(canonical, &payload); err != nil {
		return nil, fmt.Errorf("decode Factory bundled artifacts: %w", err)
	}
	artifacts := make([]PackagedFactoryArtifactIntegrity, 0, len(payload.SupportingFiles.BundledFiles))
	seen := make(map[string]struct{}, len(payload.SupportingFiles.BundledFiles))
	for _, bundled := range payload.SupportingFiles.BundledFiles {
		if err := validatePackagedFactoryArtifactPath(bundled.TargetPath); err != nil {
			return nil, err
		}
		if _, exists := seen[bundled.TargetPath]; exists {
			return nil, fmt.Errorf("duplicate bundled artifact %q", bundled.TargetPath)
		}
		seen[bundled.TargetPath] = struct{}{}
		artifacts = append(artifacts, PackagedFactoryArtifactIntegrity{
			TargetPath: bundled.TargetPath,
			SHA256:     digestPackagedFactoryBytes([]byte(bundled.Content.Inline)),
		})
	}
	sort.Slice(artifacts, func(left int, right int) bool {
		return artifacts[left].TargetPath < artifacts[right].TargetPath
	})
	return artifacts, nil
}

func validatePackagedFactoryArtifactPath(target string) error {
	if target == "" || path.IsAbs(target) || strings.Contains(target, "\\") {
		return fmt.Errorf("bundled artifact target must be a Factory-relative slash path")
	}
	cleaned := path.Clean(target)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || cleaned != target {
		return fmt.Errorf("bundled artifact target must be canonical and remain below the Factory root")
	}
	return nil
}

func canonicalPackagedFactoryJSON(jsonPayload []byte, yamlPayload []byte) ([]byte, error) {
	if len(jsonPayload) > 0 {
		var document json.RawMessage
		if err := json.Unmarshal(jsonPayload, &document); err != nil {
			return nil, fmt.Errorf("decode package JSON: %w", err)
		}
		if len(document) == 0 || document[0] != '{' {
			return nil, fmt.Errorf("package JSON must be an object")
		}
		return append([]byte(nil), jsonPayload...), nil
	}
	if len(yamlPayload) == 0 {
		return nil, fmt.Errorf("package has no representation")
	}
	var document any
	if err := yaml.Unmarshal(yamlPayload, &document); err != nil {
		return nil, fmt.Errorf("decode package YAML: %w", err)
	}
	canonical, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode package YAML as JSON: %w", err)
	}
	if len(canonical) == 0 || canonical[0] != '{' {
		return nil, fmt.Errorf("package YAML must be an object")
	}
	return canonical, nil
}

// PackagedGoalPromptFileSystem is the exact filesystem effect used by the
// packaged Goal drift check to read one already-resolved prompt path.
type PackagedGoalPromptFileSystem interface {
	ReadFile(string) ([]byte, error)
}

const (
	PackagedDeepResearchFactoryName      = "@you/deep-research"
	PackagedFusionFactoryName            = "@you/fusion"
	PackagedGoalFactoryName              = "@you/goal"
	PackagedGoalWorkTypeName             = "goal"
	PackagedGoalExecuteWorkstationName   = "execute-goal"
	PackagedGoalPlanWorkstationName      = "plan-goal"
	PackagedGoalCheckWorkstationName     = "check-goal"
	PackagedGoalReviewWorkstationName    = "review-goal"
	PackagedReviewFactoryName            = "@you/review"
	PackagedReviewExecuteWorkstationName = "execute-review-work"
	PackagedReviewWorkstationName        = "review-review-work"
	PackagedTournamentFactoryName        = "@you/tournament"
	PackagedSpawnFactoryName             = "@you/spawn"
	PackagedLoopFactoryName              = "@you/loop"
	PackagedPlanExecuteFactoryName       = "@you/plan-execute"
	PackagedPlanParallelFactoryName      = "@you/plan-parallel"
	PackagedClassifyFactoryName          = "@you/classify"
	PackagedFullFlowFactoryName          = "@you/full-flow"
)

// CustomerVisibleFactoryName returns the customer-facing Factory identifier for
// diagnostics when runtime configs use authored or generated short names.
func CustomerVisibleFactoryName(cfg *FactoryConfig) string {
	if cfg == nil {
		return ""
	}
	name := strings.TrimSpace(cfg.Name)
	if strings.HasPrefix(name, "@you/") {
		return name
	}
	project := strings.TrimSpace(cfg.Project)
	if strings.HasPrefix(project, "builtin-") {
		return "@you/" + strings.TrimPrefix(project, "builtin-")
	}
	return name
}

// PackagedFactoryAssetDefinition describes one authored packaged Factory and
// the assets available beneath its package-owned asset root.
type PackagedFactoryAssetDefinition = distributionpackageassets.Definition

// AssemblePackagedFactoryAssets resolves package-owned assets and returns a
// canonical JSON payload without persisting or installing the definition.
func AssemblePackagedFactoryAssets(definition PackagedFactoryAssetDefinition) ([]byte, error) {
	return distributionpackageassets.Assemble(definition)
}

// PackagedFactoryAssetFileSystem is the exact filesystem effect used when
// assembling packaged Factory assets from an authored package directory.
type PackagedFactoryAssetFileSystem = fs.FS
