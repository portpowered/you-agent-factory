package packagedinstallation

import (
	"context"
	"os"
	"path/filepath"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// These payloads are opaque to installer policy. No published catalog, schema
// validator, renderer, loader, or production persistence participates in a unit.
func installationDefinitionFixture() factorydefinitions.PackagedDefinition {
	return factorydefinitions.PackagedDefinition{
		Name:    "@test/fixture",
		JSON:    []byte(`{"name":"fixture","description":"fixture-v1"}`),
		YAML:    []byte("name: fixture\n"),
		Formats: []factorydefinitions.PackagedFactoryFormat{factorydefinitions.PackagedFactoryFormatJSON},
	}
}

func packagedInstallationTestPersistence() factorydefinitions.PackagedFactoryPersistence {
	return &installationPersistenceStub{}
}

type installationPersistenceStub struct {
	factorydefinitions.PackagedFactoryPersistence
	prepareErr     error
	prepareCancel  context.CancelFunc
	prepareObserve func()
}

func (p *installationPersistenceStub) PreparePackagedFactoryLayout(ctx context.Context, _ string, payload []byte) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	if p.prepareObserve != nil {
		p.prepareObserve()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p.prepareCancel != nil {
		p.prepareCancel()
		return nil, ctx.Err()
	}
	if p.prepareErr != nil {
		return nil, p.prepareErr
	}
	return &factorydefinitions.PreparedFactoryLayoutPayload{Canonical: append([]byte(nil), payload...)}, nil
}

func (*installationPersistenceStub) ValidateFactoryLayout(string) error { return nil }

func (p *installationPersistenceStub) CreateNamedFactory(root, name string, prepared *factorydefinitions.PreparedFactoryLayoutPayload) (string, error) {
	target := filepath.Join(root, filepath.FromSlash(name))
	if err := p.write(target, prepared); err != nil {
		return "", err
	}
	return target, nil
}

func (p *installationPersistenceStub) ReplaceNamedFactory(root, name string, prepared *factorydefinitions.PreparedFactoryLayoutPayload) (string, error) {
	target := filepath.Join(root, filepath.FromSlash(name))
	if _, err := p.ReplaceFactoryLayout(target, prepared); err != nil {
		return "", err
	}
	return target, nil
}

func (p *installationPersistenceStub) ReplaceFactoryLayout(target string, prepared *factorydefinitions.PreparedFactoryLayoutPayload) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	if err := os.RemoveAll(target); err != nil {
		return nil, err
	}
	if err := p.write(target, prepared); err != nil {
		return nil, err
	}
	return &factorydefinitions.FactorySplitLayoutReplaceResult{DiscardBackup: func() {}}, nil
}

func (*installationPersistenceStub) write(target string, prepared *factorydefinitions.PreparedFactoryLayoutPayload) error {
	// One nested opaque file exercises the installer's recursive identity and
	// backup policies without constructing a real authored Factory layout.
	if err := os.MkdirAll(filepath.Join(target, "nested"), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(target, "nested", "content"), []byte("fixture"), 0600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(target, prepared.RootFileName), prepared.Canonical, 0600)
}
