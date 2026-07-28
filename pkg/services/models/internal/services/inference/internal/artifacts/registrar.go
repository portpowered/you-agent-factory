// Package artifacts owns Inference-internal invocation artifact registration and
// export over an injected filesystem port. It has no independent public
// lifecycle or peer-facing service interface.
package artifacts

import (
	"fmt"
	"io"
	"strings"
	"sync"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

// Registrar materializes detached artifact metadata and exports runtime-owned
// sources through an injected filesystem port.
type Registrar struct {
	filesystem models.InvocationArtifactFileSystem

	mu     sync.Mutex
	nextID int
	sources map[string]string
}

// NewRegistrar constructs an artifact registrar over the exact filesystem port
// used for export. Construction validates the port without opening files.
func NewRegistrar(filesystem models.InvocationArtifactFileSystem) (*Registrar, error) {
	if filesystem == nil {
		return nil, fmt.Errorf(
			"%w: Models Inference artifact filesystem is required",
			models.ErrInvalidInferenceDependencies,
		)
	}
	return &Registrar{
		filesystem: filesystem,
		sources:    make(map[string]string),
	}, nil
}

// Register returns detached artifact metadata for one runtime-owned source.
func (r *Registrar) Register(source inference.InvocationArtifactSource) (models.InferenceArtifact, error) {
	if r == nil {
		return models.InferenceArtifact{}, models.ErrUnavailable
	}
	rawRef := source.RefValue
	if rawRef != "" && strings.TrimSpace(rawRef) == "" {
		return models.InferenceArtifact{}, models.ErrInferenceArtifactInvalid
	}
	refValue := strings.TrimSpace(rawRef)
	if refValue == "" {
		r.mu.Lock()
		r.nextID++
		refValue = fmt.Sprintf("models-inference:artifact:%d", r.nextID)
		r.mu.Unlock()
	}
	ref, err := (models.InferenceArtifactRef{}).Parse(refValue)
	if err != nil {
		return models.InferenceArtifact{}, err
	}
	if strings.TrimSpace(source.SourcePath) != "" {
		r.mu.Lock()
		r.sources[ref.String()] = source.SourcePath
		r.mu.Unlock()
	}
	return models.InferenceArtifact{
		Artifact:   ref,
		Name:       source.Name,
		MediaType:  source.MediaType,
		SizeBytes:  source.SizeBytes,
		Properties: cloneStringMap(source.Properties),
	}.Clone(), nil
}

// ExportInvocationArtifact copies one registered runtime-owned source to a
// caller-selected destination.
func (r *Registrar) ExportInvocationArtifact(
	ref models.InferenceArtifactRef,
	destinationPath string,
) error {
	if r == nil || r.filesystem == nil {
		return fmt.Errorf("model inference artifact registrar is required")
	}
	r.mu.Lock()
	sourcePath, ok := r.sources[ref.String()]
	r.mu.Unlock()
	if !ok || strings.TrimSpace(sourcePath) == "" {
		return models.ErrInferenceArtifactInvalid
	}
	input, err := r.filesystem.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open streamed invocation output: %w", err)
	}
	defer input.Close()

	output, err := r.filesystem.Create(destinationPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer output.Close()

	if _, err := io.Copy(output, input); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}
	return nil
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
