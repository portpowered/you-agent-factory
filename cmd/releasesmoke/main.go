package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/portpowered/infinite-you/internal/releasesmoke"
)

func main() {
	binaryPath := flag.String("binary", "", "path to the extracted agent-factory binary to smoke test")
	fixturePath := flag.String("fixture", "", "path to the canonical release smoke fixture directory")
	timeout := flag.Duration("timeout", 60*time.Second, "overall smoke timeout")
	flag.Parse()

	result, err := releasesmoke.Run(context.Background(), releasesmoke.Config{
		BinaryPath:  *binaryPath,
		FixturePath: *fixturePath,
		Timeout:     *timeout,
	})
	if err != nil {
		writeJSON(os.Stderr, err)
		os.Exit(1)
	}
	writeJSON(os.Stdout, result)
}

func writeJSON(file *os.File, value any) {
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintf(os.Stderr, "encode JSON output: %v\n", err)
	}
}
