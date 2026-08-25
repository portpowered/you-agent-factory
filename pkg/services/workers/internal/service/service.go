// Package service implements the request-scoped Workers Execute path.
package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
)

// Service executes one isolated Workers attempt through the private runner
// registry. It retains no Factory Session, Runtime, dispatch, or attempt state
// after Execute returns.
type Service struct {
	runners             runners.Service
	providers           providers.Service
	providerOverride    providers.Service
	contentMaterializer work.ContentMaterializer
	observe             workers.ObservationSink
	logger              logging.Logger
	clock               func() time.Time
	worktree            workers.FactoryWorktreePreparer
	worktreeRelease     func(context.Context, workers.FactoryWorktreePreparation) error
	temporaryFiles      workers.TemporaryFileSystem
	factoryDocs         workers.FactoryDocsLoader
	// agentRunHarness runs the agent/tool loop for an attempt whose target
	// declares Tools.AgentLoop. It is immutable process configuration; the loop
	// itself keeps no state between Execute calls.
	agentRunHarness agentrun.HarnessAdapter
	// decisionEnvelopes is the Factory Definitions owner used when a detached
	// provider override replaces the registered Agent runner.
	decisionEnvelopes factorydefinitions.DecisionEnvelopeService
}

// New constructs an inert Execute capability. Construction performs no runner
// execution, Worktree preparation, or observation delivery.
func New(
	runnerService runners.Service,
	providersService providers.Service,
	observe workers.ObservationSink,
	logger logging.Logger,
	clock func() time.Time,
	worktree workers.FactoryWorktreePreparer,
	worktreeRelease func(context.Context, workers.FactoryWorktreePreparation) error,
	temporaryFiles workers.TemporaryFileSystem,
	factoryDocs ...workers.FactoryDocsLoader,
) (*Service, error) {
	return newService(
		runnerService,
		providersService,
		observe,
		logger,
		clock,
		worktree,
		worktreeRelease,
		temporaryFiles,
		nil,
		nil,
		nil,
		nil,
		factoryDocs...,
	)
}

// NewWithProviderOverrideAndContentMaterializer constructs an inert Execute
// capability with the Work-owned content materialization edge used by
// request-scoped Worker dispatch. The materializer remains a peer capability;
// Workers does not import Work's private implementation packages.
func NewWithProviderOverrideAndContentMaterializer(
	runnerService runners.Service,
	providersService providers.Service,
	observe workers.ObservationSink,
	logger logging.Logger,
	clock func() time.Time,
	worktree workers.FactoryWorktreePreparer,
	worktreeRelease func(context.Context, workers.FactoryWorktreePreparation) error,
	temporaryFiles workers.TemporaryFileSystem,
	providerOverride providers.Service,
	agentRunHarness agentrun.HarnessAdapter,
	decisionEnvelopes factorydefinitions.DecisionEnvelopeService,
	contentMaterializer work.ContentMaterializer,
	factoryDocs ...workers.FactoryDocsLoader,
) (*Service, error) {
	return newService(
		runnerService,
		providersService,
		observe,
		logger,
		clock,
		worktree,
		worktreeRelease,
		temporaryFiles,
		contentMaterializer,
		providerOverride,
		agentRunHarness,
		decisionEnvelopes,
		factoryDocs...,
	)
}

func newService(
	runnerService runners.Service,
	providersService providers.Service,
	observe workers.ObservationSink,
	logger logging.Logger,
	clock func() time.Time,
	worktree workers.FactoryWorktreePreparer,
	worktreeRelease func(context.Context, workers.FactoryWorktreePreparation) error,
	temporaryFiles workers.TemporaryFileSystem,
	contentMaterializer work.ContentMaterializer,
	providerOverride providers.Service,
	agentRunHarness agentrun.HarnessAdapter,
	decisionEnvelopes factorydefinitions.DecisionEnvelopeService,
	factoryDocs ...workers.FactoryDocsLoader,
) (*Service, error) {
	if runnerService == nil {
		return nil, errMisconfigured("runners service is required")
	}
	if clock == nil {
		return nil, errMisconfigured("clock is required")
	}
	var docsLoader workers.FactoryDocsLoader
	if len(factoryDocs) > 0 {
		docsLoader = factoryDocs[0]
	}
	return &Service{
		runners:             runnerService,
		providers:           providersService,
		providerOverride:    providerOverride,
		contentMaterializer: contentMaterializer,
		observe:             observe,
		logger:              logging.EnsureLogger(logger),
		clock:               clock,
		worktree:            worktree,
		worktreeRelease:     worktreeRelease,
		temporaryFiles:      temporaryFiles,
		factoryDocs:         docsLoader,
		agentRunHarness:     agentRunHarness,
		decisionEnvelopes:   decisionEnvelopes,
	}, nil
}

// contentURLSafetyValidator is an optional capability on the Work materializer
// role. It stays private to Workers because the public Work root owns the
// safety policy; it becomes required before an ACP provider receives a remote
// URL that Workers intentionally preserves for provider-side retrieval.
type contentURLSafetyValidator interface {
	ValidateContentURLSafety(context.Context, string) error
}

// materializeWorkContent resolves file-backed Work content before a runner is
// selected or invoked. The request was cloned at Execute ingress, so these
// substitutions are attempt-local and do not mutate canonical Work state.
func (s *Service) materializeWorkContent(
	ctx context.Context,
	request *workers.ExecuteRequest,
	cleanup *cleanupRegistry,
) error {
	if s == nil || request == nil {
		return nil
	}
	if s.contentMaterializer == nil {
		if requestHasMaterializableContent(request) {
			return fmt.Errorf(
				"%w: Work content materializer is required for media content",
				workers.ErrInvalidExecuteRequest,
			)
		}
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

func requestHasMaterializableContent(request *workers.ExecuteRequest) bool {
	for _, input := range request.Input.Work {
		if contentPartsNeedMaterialization(input.Content) {
			return true
		}
	}
	for _, binding := range request.Input.ModelBindings {
		if contentPartsNeedMaterialization(binding.Content) {
			return true
		}
	}
	return false
}

func contentPartsNeedMaterialization(parts []work.WorkContentPart) bool {
	for _, part := range parts {
		switch part.Type.Normalized() {
		case work.WorkContentPartTypeImage,
			work.WorkContentPartTypeAudio,
			work.WorkContentPartTypeBinary:
			return true
		}
	}
	return false
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
			validator, ok := s.contentMaterializer.(contentURLSafetyValidator)
			if !ok {
				return fmt.Errorf(
					"%w: %s content part %d: Work content materializer cannot validate ACP remote URL safety",
					workers.ErrInvalidExecuteRequest,
					owner,
					partIndex,
				)
			}
			if err := validator.ValidateContentURLSafety(ctx, resolvedURL); err != nil {
				return fmt.Errorf("%s content part %d: validate URL safety: %w", owner, partIndex, err)
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
