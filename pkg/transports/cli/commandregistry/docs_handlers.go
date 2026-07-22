package commandregistry

import (
	"fmt"
	"io"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	docscli "github.com/portpowered/infinite-you/pkg/transports/cli/docs"
	"github.com/spf13/cobra"
)

type DocsHandlers struct{ DocsRunE RunE }

func NewDocsRegistry(handlers DocsHandlers) (*Registry, error) {
	if handlers.DocsRunE == nil {
		return nil, fmt.Errorf("build docs handler registry: you.docs handler is required")
	}
	registry := NewRegistry()
	if err := registry.Register("you.docs", handlers.DocsRunE); err != nil {
		return nil, fmt.Errorf("build docs handler registry: %w", err)
	}
	return registry, nil
}

type DocsBinding struct {
	BinaryName        string
	DiagnosticsWriter func(cmd *cobra.Command) io.Writer
	Verbose           func() bool
}

func DocsRunE(binding DocsBinding) RunE {
	binaryName := binding.BinaryName
	if binaryName == "" {
		binaryName = "you"
	}
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			_, err := io.WriteString(cmd.OutOrStdout(), docscli.IndexMarkdown(binaryName))
			return err
		}
		topic := args[0]
		diagnosticsOutput := binding.DiagnosticsWriter(cmd)
		clidiag.Printf(diagnosticsOutput, binding.Verbose(), "docs request topic=%s", topic)
		markdown, err := docscli.Markdown(topic)
		if err != nil {
			clidiag.Printf(diagnosticsOutput, binding.Verbose(), "docs failed topic=%s phase=resolve-topic", topic)
			return err
		}
		clidiag.Printf(diagnosticsOutput, binding.Verbose(), "docs resolved topic=%s contentBytes=%d", topic, len(markdown))
		_, err = io.WriteString(cmd.OutOrStdout(), markdown)
		if err != nil {
			clidiag.Printf(diagnosticsOutput, binding.Verbose(), "docs failed topic=%s phase=write-output", topic)
		}
		return err
	}
}
