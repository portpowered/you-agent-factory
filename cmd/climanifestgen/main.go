package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
)

const successMessage = "[agent-factory:cli-manifest-generate] CLI family metadata generated"

func main() {
	root := flag.String("root", ".", "repository root")
	check := flag.Bool("check", false, "verify generated CLI family artifacts are current")
	flag.Parse()
	os.Exit(run(*root, *check, os.Stdout, os.Stderr))
}

func run(root string, check bool, stdout, stderr io.Writer) int {
	if check {
		drift, err := climanifestgen.Check(root)
		if err != nil {
			fmt.Fprintf(stderr, "[agent-factory:cli-manifest-check] check failed: %v\n", err)
			return 1
		}
		if drift.Empty() {
			fmt.Fprintln(stdout, "[agent-factory:cli-manifest-check] CLI family metadata is current")
			return 0
		}
		writeDrift(stderr, drift)
		return 1
	}

	if err := climanifestgen.Generate(root); err != nil {
		fmt.Fprintf(stderr, "[agent-factory:cli-manifest-generate] generation failed: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, successMessage)
	return 0
}

func writeDrift(stderr io.Writer, drift climanifestgen.Drift) {
	fmt.Fprintln(stderr, "[agent-factory:cli-manifest-check] CLI family metadata drift detected")
	for _, category := range []struct {
		label string
		paths []string
	}{
		{label: "missing", paths: drift.Missing},
		{label: "stale", paths: drift.Stale},
		{label: "unexpected", paths: drift.Unexpected},
	} {
		if len(category.paths) == 0 {
			continue
		}
		paths := append([]string(nil), category.paths...)
		sort.Strings(paths)
		fmt.Fprintf(stderr, "  %s:\n", category.label)
		for _, path := range paths {
			fmt.Fprintf(stderr, "    - %s", path)
			if ids := drift.CommandIDs[path]; len(ids) > 0 {
				fmt.Fprintf(stderr, " (command IDs: %v)", ids)
			}
			fmt.Fprintln(stderr)
		}
	}
	fmt.Fprintln(stderr, "  remediation: go run ./cmd/climanifestgen -root .")
}
