package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/generatedartifacts"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
)

const successMessage = "[agent-factory:cli-manifest-generate] CLI family metadata generated"

func main() {
	root := flag.String("root", ".", "repository root")
	check := flag.Bool("check", false, "verify generated CLI family artifacts are current")
	flag.Parse()
	store, err := generatedartifacts.NewLocalStore(platformfilesystem.Local{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[agent-factory:cli-manifest-generate] initialize artifact store: %v\n", err)
		os.Exit(1)
	}
	os.Exit(run(store, *root, *check, os.Stdout, os.Stderr))
}

func run(store generatedartifacts.Store, root string, check bool, stdout, stderr io.Writer) int {
	if store == nil {
		fmt.Fprintln(stderr, "[agent-factory:cli-manifest-generate] artifact store is required")
		return 1
	}
	if check {
		artifacts, err := climanifestgen.Artifacts(store, root)
		if err != nil {
			fmt.Fprintf(stderr, "[agent-factory:cli-manifest-check] check failed: %v\n", err)
			return 1
		}
		baseDrift, err := store.Check(root, artifacts)
		if err != nil {
			fmt.Fprintf(stderr, "[agent-factory:cli-manifest-check] check failed: %v\n", err)
			return 1
		}
		drift := climanifestgen.AnnotateDrift(baseDrift)
		if drift.Empty() {
			fmt.Fprintln(stdout, "[agent-factory:cli-manifest-check] CLI family metadata is current")
			return 0
		}
		writeDrift(stderr, drift)
		return 1
	}

	artifacts, err := climanifestgen.Artifacts(store, root)
	if err != nil {
		fmt.Fprintf(stderr, "[agent-factory:cli-manifest-generate] generation failed: %v\n", err)
		return 1
	}
	if err := store.Write(root, artifacts); err != nil {
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
