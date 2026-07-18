package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/portpowered/infinite-you/internal/testutil/replayfixtures"
)

func main() {
	events, err := replayfixtures.CanonicalTopologyReplacementEvents()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(events); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
