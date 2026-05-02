package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/portpowered/infinite-you/internal/releaseprep"
)

func main() {
	var version string
	flag.StringVar(&version, "version", "", "release semver tag, for example v1.2.3")
	flag.Parse()

	if err := releaseprep.Run(context.Background(), releaseprep.Options{
		Version:        version,
		ProgressWriter: os.Stdout,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
