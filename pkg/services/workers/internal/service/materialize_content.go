package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// materializeWorkContent resolves file-backed Work content before a runner is
// selected or invoked. The request was cloned at Execute ingress, so these
// substitutions are attempt-local and do not mutate canonical Work state.
func (s *Service) materializeWorkContent(
	ctx context.Context,
	request *workers.ExecuteRequest,
	cleanup *cleanupRegistry,
) error {
	if s == nil || s.contentMaterializer == nil || request == nil {
		return nil
	}
	workingDirectory := firstNonEmpty(
		request.Target.Environment.WorkingDirectory,
		request.Target.Workspace.WorkingDirectory,
	)
	if request.Input.WorkflowContext != nil {
		workingDirectory = firstNonEmpty(
			workingDirectory,
			request.Input.WorkflowContext.WorkDirectory,
		)
	}

	for inputIndex := range request.Input.Work {
		if err := s.materializeContentParts(
			ctx,
			workingDirectory,
			request.Target.ExecutorProvider,
			request.Input.Work[inputIndex].Content,
			cleanup,
			fmt.Sprintf("Work input %d", inputIndex),
			&request.Input.Work[inputIndex].Content,
		); err != nil {
			return err
		}
	}
	for bindingIndex := range request.Input.ModelBindings {
		if err := s.materializeContentParts(
			ctx,
			workingDirectory,
			request.Target.ExecutorProvider,
			request.Input.ModelBindings[bindingIndex].Content,
			cleanup,
			fmt.Sprintf("model binding %d", bindingIndex),
			&request.Input.ModelBindings[bindingIndex].Content,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) materializeContentParts(
	ctx context.Context,
	workingDirectory string,
	executorProvider string,
	parts []work.WorkContentPart,
	cleanup *cleanupRegistry,
	owner string,
	materialized *[]work.WorkContentPart,
) error {
	for partIndex := range parts {
		part := parts[partIndex]
		switch part.Type.Normalized() {
		case work.WorkContentPartTypeImage,
			work.WorkContentPartTypeAudio,
			work.WorkContentPartTypeBinary:
		default:
			continue
		}

		rawURL := strings.TrimSpace(part.URL)
		if rawURL == "" {
			filePath := strings.TrimSpace(part.File)
			if filePath == "" {
				return fmt.Errorf(
					"%w: %s content part %d has no URL or file path",
					workers.ErrInvalidExecuteRequest,
					owner,
					partIndex,
				)
			}
			var err error
			rawURL, err = work.FilesystemPathToContentURL(filePath)
			if err != nil {
				return fmt.Errorf(
					"%w: %s content part %d: %v",
					workers.ErrInvalidExecuteRequest,
					owner,
					partIndex,
					err,
				)
			}
		}
		resolvedURL, err := work.ResolveDispatchContentURL(workingDirectory, rawURL)
		if err != nil {
			return fmt.Errorf("%s content part %d: resolve URL: %w", owner, partIndex, err)
		}
		if preserveACPRemoteResourceURL(executorProvider, resolvedURL) {
			if validator, ok := s.contentMaterializer.(work.ContentURLSafetyValidator); ok {
				if err := validator.ValidateContentURLSafety(ctx, resolvedURL); err != nil {
					return fmt.Errorf("%s content part %d: validate URL safety: %w", owner, partIndex, err)
				}
			}
			continue
		}
		path, release, err := s.contentMaterializer.MaterializeContentURL(ctx, resolvedURL)
		if release != nil {
			cleanup.add(func() error {
				release()
				return nil
			})
		}
		if err != nil {
			return fmt.Errorf("%s content part %d: %w", owner, partIndex, err)
		}
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf(
				"%s content part %d: materializer returned an empty path",
				owner,
				partIndex,
			)
		}

		part.URL = ""
		part.File = path
		(*materialized)[partIndex] = part
	}
	return nil
}

func preserveACPRemoteResourceURL(executorProvider, rawURL string) bool {
	if !strings.EqualFold(strings.TrimSpace(executorProvider), workers.ExecutorProviderACP) {
		return false
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		// ACP receives the canonical URL as a resource_link. The ACP daemon,
		// rather than the local Workers process, owns retrieval of this remote
		// resource; file and data URLs still follow normal materialization.
		return true
	default:
		return false
	}
}
