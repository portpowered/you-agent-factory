package commandregistry

import (
	"fmt"
	"io"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	docscli "github.com/portpowered/infinite-you/pkg/transports/cli/docs"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

const docsTopicInputID = "you.docs.arg.0"

type DocsBinding struct {
	BinaryName        string
	DiagnosticsWriter func(cmd *cobra.Command) io.Writer
	Verbose           func() bool
}

// DocsResolvedRunE maps the stable manifest topic input into the packaged docs
// operation while retaining Cobra only for transport-owned streams.
func DocsResolvedRunE(
	binding DocsBinding,
) func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	binaryName := binding.BinaryName
	if binaryName == "" {
		binaryName = "you"
	}
	return func(cmd *cobra.Command, inputs, _ resolvedinput.Inputs) error {
		if _, present := inputs.Lookup(docsTopicInputID); !present {
			_, err := io.WriteString(cmd.OutOrStdout(), docscli.IndexMarkdown(binaryName))
			return err
		}
		topic, err := inputs.String(docsTopicInputID)
		if err != nil {
			return fmt.Errorf("read docs topic input: %w", err)
		}
		diagnosticsOutput := cmd.ErrOrStderr()
		if binding.DiagnosticsWriter != nil {
			diagnosticsOutput = binding.DiagnosticsWriter(cmd)
		}
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
