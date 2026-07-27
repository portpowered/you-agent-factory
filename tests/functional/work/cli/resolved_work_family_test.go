package workcli_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	workservice "github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
	"github.com/spf13/cobra"
)

func TestResolvedWorkFamilyExecutesEveryPublicOperationFromStableInputs(t *testing.T) {
	var listed workcli.ListConfig
	var shown workcli.ShowConfig
	var moved workcli.MoveConfig
	handlers := commandregistry.ResolvedWorkHandlers{
		List: commandregistry.ResolvedListRunE(commandregistry.ResolvedListBinding{
			ListWork: func(cfg workcli.ListConfig) error {
				listed = cfg
				_, err := io.WriteString(cfg.Output, "listed\n")
				return err
			},
		}),
		Show: commandregistry.ResolvedShowRunE(commandregistry.ResolvedShowBinding{
			ShowWork: func(cfg workcli.ShowConfig) error {
				shown = cfg
				_, err := io.WriteString(cfg.Output, "shown\n")
				return err
			},
		}),
		Move: commandregistry.ResolvedMoveRunE(commandregistry.ResolvedMoveBinding{
			MoveWork: func(cfg workcli.MoveConfig) error {
				moved = cfg
				_, err := io.WriteString(cfg.Output, "moved\n")
				return err
			},
		}),
		Visualize: commandregistry.ResolvedVisualizeRunE(commandregistry.ResolvedVisualizeBinding{
			VisualizeWork: func(cfg workcli.VisualizeConfig) error {
				return workcli.Visualize(func(request workservice.VisualizationRequest) (string, error) {
					if request.BatchFile != "batch.json" || request.Format != "markdown-mermaid" {
						return "", fmt.Errorf("visualization request = %#v", request)
					}
					return "visualized\n", nil
				}, cfg)
			},
		}),
	}

	assertResolvedWorkExecution(t, handlers, []string{
		"--server", "https://factory.example", "--json", "--debug", "work", "list",
		"--session", "session-a", "--state-name", "review", "--state-type", "PROCESSING",
		"--name", "PRD", "--work-type-name", "story", "--trace-id", "trace-a",
		"--sort-by", "state.type", "--max-results", "7", "--next-token", "cursor-a",
	}, "listed\n")
	assertResolvedWorkExecution(t, handlers, []string{
		"--server", "https://factory.example", "--json", "work", "show",
		"--session", "session-b", "work-b",
	}, "shown\n")
	assertResolvedWorkExecution(t, handlers, []string{
		"--server", "https://factory.example", "--verbose", "work", "move",
		"--session", "session-c", "--request-id", "request-c", "work-c", "complete",
	}, "moved\n")
	assertResolvedWorkExecution(t, handlers, []string{
		"work", "visualize", "--format", "markdown-mermaid", "batch.json",
	}, "visualized\n")

	if listed.Server != "https://factory.example" || listed.SessionID != "session-a" ||
		listed.StateName != "review" || listed.StateType != "PROCESSING" ||
		listed.Name != "PRD" || listed.WorkTypeName != "story" || listed.TraceID != "trace-a" ||
		listed.SortBy != "state.type" || listed.MaxResults != 7 ||
		listed.NextToken != "cursor-a" || !listed.JSON || !listed.Verbose || !listed.Debug {
		t.Fatalf("list config = %#v, want stable local and inherited inputs", listed)
	}
	if shown.Server != "https://factory.example" || shown.SessionID != "session-b" ||
		shown.WorkID != "work-b" || !shown.JSON {
		t.Fatalf("show config = %#v, want stable local and inherited inputs", shown)
	}
	if moved.Server != "https://factory.example" || moved.SessionID != "session-c" ||
		moved.WorkID != "work-c" || moved.StateName != "complete" ||
		moved.RequestID != "request-c" || !moved.Verbose {
		t.Fatalf("move config = %#v, want stable local and inherited inputs", moved)
	}
}

func assertResolvedWorkExecution(
	t *testing.T,
	handlers commandregistry.ResolvedWorkHandlers,
	args []string,
	wantOutput string,
) {
	t.Helper()
	root := resolvedWorkFamilyRoot(t, handlers)
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(%v) error = %v", args, err)
	}
	if output.String() != wantOutput {
		t.Fatalf("Execute(%v) output = %q, want %q", args, output.String(), wantOutput)
	}
}

func resolvedWorkFamilyRoot(
	t *testing.T,
	handlers commandregistry.ResolvedWorkHandlers,
) *cobra.Command {
	t.Helper()
	manifest, err := generated.WorkFamilyManifest()
	if err != nil {
		t.Fatalf("WorkFamilyManifest() error = %v", err)
	}
	rootManifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		t.Fatalf("RepresentativeFamilyManifest() error = %v", err)
	}
	rootRecord, err := rootManifest.CommandByID("you")
	if err != nil {
		t.Fatalf("CommandByID(you) error = %v", err)
	}
	manifest.Commands[rootRecord.ID] = rootRecord
	byCommandID := map[string]commandregistry.ResolvedWorkRunE{
		"you.work.list": handlers.List, "you.work.show": handlers.Show,
		"you.work.move": handlers.Move, "you.work.visualize": handlers.Visualize,
	}
	cobraHandlers := make(climanifestcobra.CobraHandlerRegistry, len(byCommandID))
	for commandID, handler := range byCommandID {
		record := manifest.Commands[commandID]
		cobraHandlers[record.Handler.ID] = func(
			cmd *cobra.Command,
			_ []string,
			values map[string]any,
			inherited resolvedinput.Inputs,
		) error {
			local, resolveErr := resolveWorkFunctionalInputs(record, values)
			if resolveErr != nil {
				return resolveErr
			}
			return handler(cmd, local, inherited)
		}
	}
	root, err := (climanifestcobra.GenericConstructor{}).Construct(
		manifest,
		climanifestcobra.GenericBindings{
			Handlers: climanifestcobra.HandlerRegistry{
				rootRecord.Handler.ID: func(context.Context, map[string]any) error { return nil },
			},
			CobraHandlers:           cobraHandlers,
			GuardUnknownSubcommands: true,
		},
	)
	if err != nil {
		t.Fatalf("Construct() error = %v", err)
	}
	return root
}

func resolveWorkFunctionalInputs(
	record climanifest.Command,
	values map[string]any,
) (resolvedinput.Inputs, error) {
	definitions := make([]resolvedinput.Definition, 0, len(record.Arguments)+len(record.Flags))
	candidates := make([]resolvedinput.Candidate, 0, cap(definitions))
	appendInput := func(id string, source resolvedinput.Source) error {
		value, present := values[id]
		if !present {
			return nil
		}
		resolved, err := workFunctionalValue(value)
		if err != nil {
			return fmt.Errorf("resolve %q: %w", id, err)
		}
		definitions = append(definitions, resolvedinput.Definition{
			ID: id, Kind: resolved.Kind(), Precedence: []resolvedinput.Source{source},
		})
		candidates = append(candidates, resolvedinput.Candidate{
			InputID: id, Source: source, Value: resolved,
		})
		return nil
	}
	for id := range record.Arguments {
		if err := appendInput(id, resolvedinput.SourcePositionalArgument); err != nil {
			return resolvedinput.Inputs{}, err
		}
	}
	for id, flag := range record.Flags {
		if flag.Scope == "local" {
			if err := appendInput(id, resolvedinput.SourceCLIFlag); err != nil {
				return resolvedinput.Inputs{}, err
			}
		}
	}
	return resolvedinput.Resolve(definitions, candidates)
}

func workFunctionalValue(value any) (resolvedinput.Value, error) {
	switch typed := value.(type) {
	case string:
		return resolvedinput.StringValue(typed), nil
	case int:
		return resolvedinput.IntValue(typed), nil
	default:
		return resolvedinput.Value{}, fmt.Errorf("unsupported Work input type %T", value)
	}
}
