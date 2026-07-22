// Command testlanecheck verifies that every repository Go test package has a
// primary lane in the shared lane policy.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/internal/testlanes"
)

type listedPackage struct {
	ImportPath   string
	TestGoFiles  []string
	XTestGoFiles []string
}

func main() {
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	counts := map[testlanes.Lane]int{}
	var unowned []string
	decoder := json.NewDecoder(stdout)
	for {
		var pkg listedPackage
		if err := decoder.Decode(&pkg); err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintf(os.Stderr, "decode go list output: %v\n", err)
			os.Exit(1)
		}
		if len(pkg.TestGoFiles)+len(pkg.XTestGoFiles) == 0 || isOptInPackage(pkg.ImportPath) {
			continue
		}
		lane, ok := testlanes.ForImportPath(pkg.ImportPath)
		if !ok {
			unowned = append(unowned, pkg.ImportPath)
			continue
		}
		counts[lane]++
	}
	if err := cmd.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "go list: %v\n", err)
		os.Exit(1)
	}
	if len(unowned) > 0 {
		sort.Strings(unowned)
		fmt.Fprintf(os.Stderr, "Go test packages without a primary lane:\n  %s\n", strings.Join(unowned, "\n  "))
		os.Exit(1)
	}

	lanes := make([]string, 0, len(counts))
	for lane, count := range counts {
		lanes = append(lanes, fmt.Sprintf("%s=%d", lane, count))
	}
	sort.Strings(lanes)
	fmt.Printf("Go test lane ownership: %s\n", strings.Join(lanes, " "))
}

func isOptInPackage(importPath string) bool {
	return importPath == testlanes.ModulePath+"/tests/adhoc" ||
		strings.HasPrefix(importPath, testlanes.ModulePath+"/tests/adhoc/")
}
