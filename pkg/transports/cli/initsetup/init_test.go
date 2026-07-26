package initsetup_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/transports/cli/initsetup"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
)

func TestConfigurerRequiresSuppliedProviderBeforePersistence(t *testing.T) {
	var output bytes.Buffer
	err := initsetup.NewConfigurer(operatorsettings.ConfigDocumentService{})(
		initsetup.Config{
			Context: context.Background(),
			HomeDir: t.TempDir(),
			Output:  &output,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "use --provider") {
		t.Fatalf("configure error = %v, want supplied-provider guidance", err)
	}
	if output.Len() != 0 {
		t.Fatalf("output = %q, want empty", output.String())
	}
}

func TestConfigurerRejectsEmptySuppliedModelBeforePersistence(t *testing.T) {
	model := "  "
	err := initsetup.NewConfigurer(operatorsettings.ConfigDocumentService{})(
		initsetup.Config{
			Context:  context.Background(),
			HomeDir:  t.TempDir(),
			Provider: "codex",
			Model:    &model,
			Output:   &bytes.Buffer{},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "model must be non-empty") {
		t.Fatalf("configure error = %v, want empty-model rejection", err)
	}
}

func TestConfigurerReportsCanonicalPersistedDefaults(t *testing.T) {
	var output bytes.Buffer
	model := "  free-form/model:v1  "
	service := operatorsettings.ConfigDocumentService{
		Files: platformfilesystem.Local{},
		CreateTemp: func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		Providers:       testProviderCatalog,
		Decoder:         globalconfigmapping.Decode,
		Encoder:         globalconfigmapping.Encode,
		PersistenceLock: &sync.Mutex{},
	}
	homeDir := t.TempDir()
	err := initsetup.NewConfigurer(service)(initsetup.Config{
		Context:  context.Background(),
		HomeDir:  homeDir,
		Provider: "CODEX",
		Model:    &model,
		Output:   &output,
	})
	if err != nil {
		t.Fatalf("configure error = %v", err)
	}
	for _, want := range []string{
		"codex",
		"free-form/model:v1",
		filepath.Join(homeDir, ".you-agent-factory", "config.json"),
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output = %q, want %q", output.String(), want)
		}
	}
}

func testProviderCatalog(value string) (string, bool) {
	if strings.EqualFold(strings.TrimSpace(value), "codex") {
		return "codex", true
	}
	return "", false
}
