package inference

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

// Keep this bound aligned with Work's default materialization limit. Local
// file URLs bypass Work's remote download bound, so the reader must enforce it.
const maxInferenceMediaBytes int64 = 32 << 20

func (r *runner) materializeMediaInputs(ctx context.Context, inputs []models.InferenceInput) error {
	for index := range inputs {
		input := &inputs[index]
		if input.Modality != models.ModalityImage && input.Modality != models.ModalityAudio && input.Modality != models.ModalityVideo {
			continue
		}
		// Work media URLs name content to load, while raw bytes and artifact-only
		// references already satisfy the Models contract.
		if !isMediaContentURL(input.Content) {
			continue
		}
		if r.contentMaterializer == nil || r.mediaFiles == nil {
			return misconfigured("inference media materializer and file reader are required", nil)
		}
		content, err := r.readMediaURL(ctx, input.Content)
		if err != nil {
			return badRequest(fmt.Sprintf("inference media input %q cannot be read", input.Name), err)
		}
		input.Content = string(content)
	}
	return ctx.Err()
}

func isMediaContentURL(content string) bool {
	value := strings.ToLower(strings.TrimSpace(content))
	return strings.HasPrefix(value, "file://") || strings.HasPrefix(value, "https://") ||
		strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "data:")
}

func (r *runner) readMediaURL(ctx context.Context, rawURL string) ([]byte, error) {
	path, cleanup, err := r.contentMaterializer.MaterializeContentURL(ctx, rawURL)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := r.mediaFiles.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	stop := context.AfterFunc(ctx, func() { _ = file.Close() })
	defer stop()
	content, err := io.ReadAll(io.LimitReader(file, maxInferenceMediaBytes+1))
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maxInferenceMediaBytes {
		return nil, fmt.Errorf("media input exceeds %d byte limit", maxInferenceMediaBytes)
	}
	return content, nil
}
