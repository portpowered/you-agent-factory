package baseline

import (
	"bytes"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

// CaptureHelpOutput runs a production Cobra help invocation against root using
// args and returns normalized stdout. stderr is discarded.
func CaptureHelpOutput(root *cobra.Command, args []string) (string, error) {
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs(args)

	if err := root.Execute(); err != nil {
		return "", err
	}
	return NormalizeHelpOutput(out.String()), nil
}

// NormalizeHelpOutput canonicalizes help text so fixtures stay stable across
// platforms and repeated runs.
func NormalizeHelpOutput(output string) string {
	normalized := strings.ReplaceAll(output, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	if normalized != "" && !strings.HasSuffix(normalized, "\n") {
		normalized += "\n"
	}
	return normalized
}
