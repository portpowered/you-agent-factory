package artifacts

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	modelRoleManifestSchemaVersion = 1
	modelRoleManifestKind          = "localai-model-role-artifacts"
	modelRoleTTS                   = "tts"
	modelRoleModel                 = "model"
	modelRoleTokenizer             = "tokenizer"
	modelRoleVoice                 = "voice"
)

var (
	// ErrModelRoleManifestMalformed classifies invalid private role metadata.
	ErrModelRoleManifestMalformed = errors.New("model role manifest is malformed")
	//go:embed localai-model-role-artifacts.json
	defaultModelRoleManifestData []byte
)

// ModelRoleManifestError describes a private role-manifest validation failure
// without including model paths, URLs, or other resolved runtime values.
type ModelRoleManifestError struct {
	Field  string
	Detail string
}

func (e *ModelRoleManifestError) Error() string {
	if e == nil {
		return ""
	}
	if e.Field == "" {
		return e.Detail
	}
	return fmt.Sprintf("model role manifest %s: %s", e.Field, e.Detail)
}

func (e *ModelRoleManifestError) Unwrap() error {
	return ErrModelRoleManifestMalformed
}

// ModelRoleManifest is the validated private role metadata used by a backend
// layout selector. It is not part of the public Models contract.
type ModelRoleManifest struct {
	SchemaVersion int              `json:"schemaVersion"`
	Kind          string           `json:"kind"`
	Models        []ModelRoleModel `json:"models"`
}

// ModelRoleModel describes one model's exact private artifact roles.
type ModelRoleModel struct {
	Name        string               `json:"name"`
	Publication ModelRolePublication `json:"publication"`
	Source      ModelRoleSource      `json:"source"`
	Backend     ModelRoleBackend     `json:"backend"`
	Protocol    ModelRoleProtocol    `json:"protocol"`
	Targets     []string             `json:"targets"`
	Artifacts   []ModelRoleArtifact  `json:"artifacts"`
}

// ModelRolePublication records the model repository, revision, license, and
// upstream base-model provenance.
type ModelRolePublication struct {
	Repository string `json:"repository"`
	Revision   string `json:"revision"`
	License    string `json:"license"`
	BaseModel  string `json:"baseModel"`
}

// ModelRoleSource records the immutable model source URI.
type ModelRoleSource struct {
	URI string `json:"uri"`
}

// ModelRoleBackend records the private backend and source revisions.
type ModelRoleBackend struct {
	ID            string `json:"id"`
	Repository    string `json:"repository"`
	Commit        string `json:"commit"`
	LocalAICommit string `json:"localAICommit"`
}

// ModelRoleProtocol records the private backend protocol identity.
type ModelRoleProtocol struct {
	Path     string `json:"path"`
	Revision string `json:"revision"`
}

// ModelRoleArtifact records one exact model-role file and its integrity facts.
type ModelRoleArtifact struct {
	Role      string `json:"role"`
	Path      string `json:"path"`
	SizeBytes int64  `json:"sizeBytes"`
	SHA256    string `json:"sha256"`
}

// DefaultModelRoleManifest decodes the authored private role manifest.
func DefaultModelRoleManifest() (ModelRoleManifest, error) {
	return DecodeModelRoleManifest(defaultModelRoleManifestData)
}

// DecodeModelRoleManifest validates one authored private role manifest.
func DecodeModelRoleManifest(data []byte) (ModelRoleManifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest ModelRoleManifest
	if err := decoder.Decode(&manifest); err != nil {
		return ModelRoleManifest{}, roleManifestFailure("document", "must be valid JSON")
	}
	if err := rejectTrailingJSON(decoder); err != nil {
		return ModelRoleManifest{}, roleManifestFailure("document", "must contain one JSON value")
	}
	if err := validateModelRoleManifest(manifest); err != nil {
		return ModelRoleManifest{}, err
	}
	return cloneModelRoleManifest(manifest), nil
}

// Model returns a detached role definition by model name.
func (manifest ModelRoleManifest) Model(name string) (ModelRoleModel, bool) {
	canonical := strings.ToLower(strings.TrimSpace(name))
	for _, model := range manifest.Models {
		if model.Name == canonical {
			return cloneModelRoleModel(model), true
		}
	}
	return ModelRoleModel{}, false
}

// Artifact returns one detached role artifact by role name.
func (model ModelRoleModel) Artifact(role string) (ModelRoleArtifact, bool) {
	canonical := strings.ToLower(strings.TrimSpace(role))
	for _, artifact := range model.Artifacts {
		if artifact.Role == canonical {
			return artifact, true
		}
	}
	return ModelRoleArtifact{}, false
}

func validateModelRoleManifest(manifest ModelRoleManifest) error {
	if manifest.SchemaVersion != modelRoleManifestSchemaVersion {
		return roleManifestFailure("schemaVersion", "must be 1")
	}
	if manifest.Kind != modelRoleManifestKind {
		return roleManifestFailure("kind", "must be localai-model-role-artifacts")
	}
	if len(manifest.Models) != 1 {
		return roleManifestFailure("models", "must contain exactly one model")
	}
	seen := make(map[string]struct{}, len(manifest.Models))
	for index, model := range manifest.Models {
		if model.Name != modelRoleTTS {
			return roleManifestFailure(fmt.Sprintf("models[%d].name", index), "must be tts")
		}
		if _, exists := seen[model.Name]; exists {
			return roleManifestFailure("models", "model names must be unique")
		}
		seen[model.Name] = struct{}{}
		if err := validateModelRoleModel(model, index); err != nil {
			return err
		}
	}
	return nil
}

func validateModelRoleModel(model ModelRoleModel, index int) error {
	prefix := fmt.Sprintf("models[%d]", index)
	if !validRepositoryName(model.Publication.Repository) ||
		!commitPattern.MatchString(model.Publication.Revision) ||
		strings.TrimSpace(model.Publication.License) == "" ||
		strings.TrimSpace(model.Publication.BaseModel) == "" {
		return roleManifestFailure(prefix+".publication", "requires repository, revision, license, and baseModel")
	}
	if !validGitHubRepository(model.Backend.Repository) ||
		!commitPattern.MatchString(model.Backend.Commit) ||
		!commitPattern.MatchString(model.Backend.LocalAICommit) ||
		!validToken(model.Backend.ID) {
		return roleManifestFailure(prefix+".backend", "requires a valid backend identity and commits")
	}
	if !validRelativePath(model.Protocol.Path) || !commitPattern.MatchString(model.Protocol.Revision) {
		return roleManifestFailure(prefix+".protocol", "requires a safe path and revision")
	}
	if !sameStrings(model.Targets, []string{"darwin-arm64", "linux-amd64", "windows-amd64"}) {
		return roleManifestFailure(prefix+".targets", "must declare the supported target matrix")
	}
	if len(model.Artifacts) != 3 {
		return roleManifestFailure(prefix+".artifacts", "must contain model, tokenizer, and voice")
	}
	seenRoles := make(map[string]struct{}, len(model.Artifacts))
	seenPaths := make(map[string]struct{}, len(model.Artifacts))
	for artifactIndex, artifact := range model.Artifacts {
		artifactPrefix := fmt.Sprintf("%s.artifacts[%d]", prefix, artifactIndex)
		if artifact.Role != modelRoleModel && artifact.Role != modelRoleTokenizer && artifact.Role != modelRoleVoice {
			return roleManifestFailure(artifactPrefix+".role", "must be model, tokenizer, or voice")
		}
		if _, exists := seenRoles[artifact.Role]; exists {
			return roleManifestFailure(prefix+".artifacts", "roles must be unique")
		}
		seenRoles[artifact.Role] = struct{}{}
		if !validRelativePath(artifact.Path) {
			return roleManifestFailure(artifactPrefix+".path", "must be a safe relative path")
		}
		if _, exists := seenPaths[artifact.Path]; exists {
			return roleManifestFailure(prefix+".artifacts", "paths must be unique")
		}
		seenPaths[artifact.Path] = struct{}{}
		if artifact.SizeBytes <= 0 || !digestPattern.MatchString(artifact.SHA256) {
			return roleManifestFailure(artifactPrefix, "requires a positive size and lowercase SHA-256")
		}
	}
	modelArtifact, ok := model.Artifact(modelRoleModel)
	if !ok || model.Source.URI != "hf://"+model.Publication.Repository+"/"+modelArtifact.Path+"@"+model.Publication.Revision {
		return roleManifestFailure(prefix+".source.uri", "must identify the model artifact at the publication revision")
	}
	return nil
}

func validGitHubRepository(value string) bool {
	return strings.HasPrefix(value, "https://github.com/") && !strings.ContainsAny(value, "?#")
}

func roleManifestFailure(field, detail string) error {
	return &ModelRoleManifestError{Field: field, Detail: detail}
}

func cloneModelRoleManifest(manifest ModelRoleManifest) ModelRoleManifest {
	models := manifest.Models
	manifest.Models = make([]ModelRoleModel, len(models))
	for index, model := range models {
		manifest.Models[index] = cloneModelRoleModel(model)
	}
	return manifest
}

func cloneModelRoleModel(model ModelRoleModel) ModelRoleModel {
	model.Targets = append([]string(nil), model.Targets...)
	model.Artifacts = append([]ModelRoleArtifact(nil), model.Artifacts...)
	return model
}
