// Command acpbaseline captures ACP wire baselines and compares them.
//
// It is a maintenance tool, deliberately not a `you` subcommand: making a
// diagnostic a public CLI surface would commit us to maintaining it as one.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mattn/go-shellwords"

	"github.com/portpowered/infinite-you/internal/acpbaseline"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "capture":
		err = runCapture(os.Args[2:])
	case "compare":
		err = runCompare(os.Args[2:])
	case "verify":
		err = runVerify(os.Args[2:])
	case "scenarios":
		err = runScenarios()
	default:
		usage()
		os.Exit(2)
	}
	if err == nil {
		return
	}
	var operator *acpbaseline.OperatorActionError
	if ok := asOperatorAction(err, &operator); ok {
		fmt.Fprintf(os.Stderr, "acpbaseline: operator action required: %v\n", operator)
		os.Exit(acpbaseline.ExitOperatorAction)
	}
	fmt.Fprintf(os.Stderr, "acpbaseline: %v\n", err)
	os.Exit(1)
}

func asOperatorAction(err error, target **acpbaseline.OperatorActionError) bool {
	if typed, ok := err.(*acpbaseline.OperatorActionError); ok {
		*target = typed
		return true
	}
	return false
}

func usage() {
	fmt.Fprint(os.Stderr, `acpbaseline captures and compares ACP agent wire baselines.

  capture   -agent '<argv>' -name <label> -out <dir> [-publish <dir>] [-scenario all]
  compare   -matrix <a.json> -matrix <b.json> ... -out <comparison.md>
  verify    -dir <committed baselines dir>
  scenarios

Raw transcripts contain full prompt and response content. They are written
under -out and must never be committed; only the -publish tier is committable.
`)
}

func runScenarios() error {
	scenarios, err := acpbaseline.LoadScenarios()
	if err != nil {
		return err
	}
	for _, scenario := range scenarios {
		fmt.Printf("%-26s %s\n", scenario.Name, scenario.Description)
	}
	return nil
}

func runCapture(args []string) error {
	flags := flag.NewFlagSet("capture", flag.ExitOnError)
	agent := flags.String("agent", "", "agent argv, e.g. 'cursor-agent acp'")
	name := flags.String("name", "", "label for this capture")
	out := flags.String("out", ".artifacts/acp-baseline", "raw artifact directory (never committed)")
	publish := flags.String("publish", "", "optional committable output directory")
	scenario := flags.String("scenario", "all", "scenario name, or all")
	timeout := flags.Duration("step-timeout", 90*time.Second, "per-request timeout")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*agent) == "" || strings.TrimSpace(*name) == "" {
		return fmt.Errorf("-agent and -name are required")
	}

	argv, err := shellwords.Parse(*agent)
	if err != nil || len(argv) == 0 {
		return fmt.Errorf("invalid -agent command %q", *agent)
	}

	outputDir := filepath.Join(*out, *name)
	manifest, err := acpbaseline.Capture(acpbaseline.CaptureRequest{
		AgentCommand: argv,
		AgentName:    *name,
		OutputDir:    outputDir,
		Scenarios:    scenarioList(*scenario),
		StepTimeout:  *timeout,
	}, time.Now)
	if err != nil {
		return err
	}

	matrix := acpbaseline.BuildMatrix(manifest)
	encoded, err := json.MarshalIndent(matrix, "", "  ")
	if err != nil {
		return err
	}
	matrixPath := filepath.Join(outputDir, "capability-matrix.json")
	if err := os.WriteFile(matrixPath, append(encoded, '\n'), 0o600); err != nil {
		return err
	}

	fmt.Printf("captured %s: %d scenario(s), matrix at %s\n",
		*name, len(manifest.Scenarios), matrixPath)
	for _, failure := range manifest.Errors {
		fmt.Fprintf(os.Stderr, "  scenario error: %s\n", failure)
	}
	if strings.TrimSpace(*publish) == "" {
		return nil
	}
	if err := acpbaseline.Publish(manifest, matrix, outputDir, *publish); err != nil {
		return err
	}
	fmt.Printf("published digest, manifest, and matrix to %s\n", *publish)
	return nil
}

func scenarioList(value string) []string {
	if strings.TrimSpace(value) == "" || value == "all" {
		return nil
	}
	return strings.Split(value, ",")
}

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func runCompare(args []string) error {
	flags := flag.NewFlagSet("compare", flag.ExitOnError)
	var paths stringList
	flags.Var(&paths, "matrix", "path to a capability-matrix.json (repeatable)")
	out := flags.String("out", "", "output Markdown path; stdout when empty")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("at least one -matrix is required")
	}

	matrices := make([]*acpbaseline.CapabilityMatrix, 0, len(paths))
	for _, path := range paths {
		matrix, err := acpbaseline.LoadMatrix(path)
		if err != nil {
			return err
		}
		matrices = append(matrices, matrix)
	}

	rendered := acpbaseline.RenderComparison(matrices, acpbaseline.Compare(matrices))
	if strings.TrimSpace(*out) == "" {
		fmt.Print(rendered)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	return os.WriteFile(*out, []byte(rendered), 0o644)
}

func runVerify(args []string) error {
	flags := flag.NewFlagSet("verify", flag.ExitOnError)
	dir := flags.String("dir", "docs/internal/projects/acp-program/baselines", "committed baselines directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	findings, err := acpbaseline.VerifyCommitted(*dir)
	if err != nil {
		return err
	}
	if len(findings) == 0 {
		fmt.Printf("[acpbaseline] committed baselines under %s are digested and secret-free\n", *dir)
		return nil
	}
	for _, finding := range findings {
		fmt.Fprintln(os.Stderr, "  "+finding)
	}
	return fmt.Errorf("found %d committed-baseline violation(s)", len(findings))
}
