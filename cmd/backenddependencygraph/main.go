package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/portpowered/infinite-you/internal/backenddependencygraph"
)

func main() {
	root := flag.String("root", ".", "repository root")
	output := flag.String("output", ".artifacts/backend-dependency-graph/backend-dependency-graph.dot", "DOT output path")
	svgOutput := flag.String("svg-output", ".artifacts/backend-dependency-graph/backend-dependency-graph.svg", "SVG output path when Graphviz is available")
	goBinary := flag.String("go", "go", "Go command")
	flag.Parse()

	packages, modulePath, err := backenddependencygraph.Load(context.Background(), *root, *goBinary)
	if err == nil {
		err = backenddependencygraph.WriteDOT(*output, backenddependencygraph.RenderDOT(packages, modulePath))
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "backend dependency graph: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Wrote backend package dependency graph (%d packages) to %s\n", len(packages), *output)

	dotBinary, err := exec.LookPath("dot")
	if err != nil {
		fmt.Printf("Graphviz 'dot' was not found; inspect %s directly or install Graphviz and rerun this command.\n", *output)
		return
	}
	if err := backenddependencygraph.RenderSVG(context.Background(), dotBinary, *output, *svgOutput); err != nil {
		fmt.Fprintf(os.Stderr, "backend dependency graph: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Rendered backend package dependency graph to %s\n", *svgOutput)
}
