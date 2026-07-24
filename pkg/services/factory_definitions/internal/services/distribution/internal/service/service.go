package service

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
)

// Service implements the private Definitions distribution capability.
type Service struct {
	catalog   []factorydefinitions.PackagedDefinition
	installer factorydefinitions.PackagedFactoryInstaller
	scaffold  factorydefinitions.ScaffoldInitializer
}

var _ distribution.Service = (*Service)(nil)

// New constructs the distribution implementation from injected collaborators.
func New(
	catalog []factorydefinitions.PackagedDefinition,
	installer factorydefinitions.PackagedFactoryInstaller,
	scaffold factorydefinitions.ScaffoldInitializer,
) *Service {
	if installer == nil || scaffold == nil {
		return nil
	}
	copied := append([]factorydefinitions.PackagedDefinition(nil), catalog...)
	return &Service{catalog: copied, installer: installer, scaffold: scaffold}
}

func (s *Service) ListBuiltInPackagedFactories(
	_ context.Context,
	_ factorydefinitions.ListBuiltInPackagedFactoriesRequest,
) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
	if s == nil {
		return factorydefinitions.ListBuiltInPackagedFactoriesResult{}, fmt.Errorf("packaged factory catalog collaborator is required")
	}
	entries := make([]factorydefinitions.BuiltInPackagedFactoryEntry, 0, len(s.catalog))
	for _, definition := range s.catalog {
		entries = append(entries, factorydefinitions.BuiltInPackagedFactoryEntry{
			Name:    definition.Name,
			Project: definition.Project,
		})
	}
	return factorydefinitions.ListBuiltInPackagedFactoriesResult{Entries: entries}, nil
}

func (s *Service) InstallPackagedFactory(
	ctx context.Context,
	request factorydefinitions.InstallPackagedFactoryRequest,
) (factorydefinitions.InstallPackagedFactoryResult, error) {
	if s == nil || s.installer == nil {
		return factorydefinitions.InstallPackagedFactoryResult{}, factorydefinitions.ErrUnknownPackagedFactoryIdentity
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return factorydefinitions.InstallPackagedFactoryResult{}, factorydefinitions.ErrUnknownPackagedFactoryIdentity
	}
	definition, ok := s.lookup(name)
	if !ok {
		return factorydefinitions.InstallPackagedFactoryResult{}, factorydefinitions.ErrUnknownPackagedFactoryIdentity
	}
	results, err := s.installer.EnsurePackagedFactories(ctx, request.RootDir, []factorydefinitions.PackagedDefinition{definition})
	if err != nil {
		return factorydefinitions.InstallPackagedFactoryResult{}, fmt.Errorf("%w: %v", factorydefinitions.ErrFactoryDistributeFailed, err)
	}
	if len(results) == 0 {
		return factorydefinitions.InstallPackagedFactoryResult{}, factorydefinitions.ErrFactoryDistributeFailed
	}
	installed := results[0]
	return factorydefinitions.InstallPackagedFactoryResult{
		Definition: distributedDefinitionFacts(installed.Name, installed.FactoryDir),
		Outcome:    installed.Outcome,
	}, nil
}

func (s *Service) CreateFactoryScaffold(
	_ context.Context,
	request factorydefinitions.CreateFactoryScaffoldRequest,
) (factorydefinitions.CreateFactoryScaffoldResult, error) {
	if s == nil || s.scaffold == nil {
		return factorydefinitions.CreateFactoryScaffoldResult{}, factorydefinitions.ErrFactoryDistributeFailed
	}
	targetDir := strings.TrimSpace(request.TargetDir)
	if targetDir == "" {
		return factorydefinitions.CreateFactoryScaffoldResult{}, factorydefinitions.ErrFactoryDistributeFailed
	}
	facts := distributedDefinitionFacts("", targetDir)
	if facts.FactoryDir == "" || facts.FactoryDir == "." {
		return factorydefinitions.CreateFactoryScaffoldResult{}, factorydefinitions.ErrFactoryDistributeFailed
	}
	scaffoldType := strings.TrimSpace(request.Type)
	if scaffoldType == "" {
		scaffoldType = string(factorydefinitions.DefaultScaffoldType)
	}
	err := s.scaffold(factorydefinitions.ScaffoldConfig{
		Dir:      facts.FactoryDir,
		Type:     scaffoldType,
		Executor: request.Executor,
		JSON:     true,
		Output:   io.Discard,
	})
	if err != nil {
		return factorydefinitions.CreateFactoryScaffoldResult{}, fmt.Errorf("%w: %v", factorydefinitions.ErrFactoryDistributeFailed, err)
	}
	name := filepath.Base(facts.FactoryDir)
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = scaffoldType
	}
	facts.Name = name
	return factorydefinitions.CreateFactoryScaffoldResult{
		Definition:   facts,
		ScaffoldType: scaffoldType,
	}, nil
}

func (s *Service) lookup(name string) (factorydefinitions.PackagedDefinition, bool) {
	for _, definition := range s.catalog {
		if definition.Name == name {
			return definition, true
		}
	}
	return factorydefinitions.PackagedDefinition{}, false
}

// distributedDefinitionFacts builds the shared CTR-DEF aggregate identity shape
// for install and scaffold success paths so FactoryDir identity stays aligned.
func distributedDefinitionFacts(name, factoryDir string) factorydefinitions.DistributedFactoryDefinitionFacts {
	cleanedDir := strings.TrimSpace(factoryDir)
	if cleanedDir != "" {
		cleanedDir = filepath.Clean(cleanedDir)
	}
	return factorydefinitions.DistributedFactoryDefinitionFacts{
		Name:       strings.TrimSpace(name),
		FactoryDir: cleanedDir,
	}
}
