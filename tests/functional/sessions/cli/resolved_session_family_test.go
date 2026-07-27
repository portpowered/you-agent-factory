package sessioncli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

func TestResolvedSessionFamilyExecutesEveryPublicOperationFromStableInputs(t *testing.T) {
	var captured sessionOperationCaptures
	services := recordingSessionServices(&captured)

	assertResolvedSessionExecution(t, services, []string{
		"--server", "https://factory.example", "--json", "--debug",
		"session", "create", "--dir", "factory-a", "--init-new-factory",
		"--port", "9042", "--target-kind", "named", "--target-name", "alpha",
	}, "created\n")
	assertResolvedSessionExecution(t, services, []string{
		"--json", "session", "delete", "session-delete",
	}, "deleted\n")
	assertResolvedSessionExecution(t, services, []string{
		"--server", "https://factory.example", "--verbose",
		"session", "list", "--scope", "persisted",
	}, "listed\n")
	assertResolvedSessionExecution(t, services, []string{
		"--server", "https://factory.example", "--json",
		"session", "show", "session-show",
	}, "shown\n")
	assertResolvedSessionExecution(t, services, []string{
		"--server", "https://factory.example", "session", "dispatches",
		"--phase", "execute", "--status", "SUCCEEDED", "session-dispatches",
	}, "dispatches\n")
	assertResolvedSessionExecution(t, services, []string{
		"--server", "https://factory.example", "session", "pause",
	}, "paused\n")
	assertResolvedSessionExecution(t, services, []string{
		"--server", "https://factory.example", "--json",
		"session", "resume", "session-resume",
	}, "resumed\n")
	assertResolvedSessionRejection(t, services, []string{
		"session", "show", "--port", "9043", "session-rejected",
	})
	assertSessionOperationCaptures(t, captured)
}

type sessionOperationCaptures struct {
	created      sessioncli.CreateConfig
	deleted      sessioncli.DeleteConfig
	listed       sessioncli.ListConfig
	shown        sessioncli.ShowConfig
	dispatches   sessioncli.DispatchesConfig
	paused       sessioncli.LifecycleControlConfig
	resumed      sessioncli.LifecycleControlConfig
	preparedList bool
	showCalls    int
}

func recordingSessionServices(captured *sessionOperationCaptures) commandregistry.SessionResolvedServices {
	return commandregistry.SessionResolvedServices{
		CreateSession: func(cfg sessioncli.CreateConfig) error {
			captured.created = cfg
			_, err := io.WriteString(cfg.Output, "created\n")
			return err
		},
		DeleteSession: func(cfg sessioncli.DeleteConfig) error {
			captured.deleted = cfg
			_, err := io.WriteString(cfg.Output, "deleted\n")
			return err
		},
		ListSessions: func(cfg sessioncli.ListConfig) error {
			captured.listed = cfg
			_, err := io.WriteString(cfg.Output, "listed\n")
			return err
		},
		ShowSession: func(cfg sessioncli.ShowConfig) error {
			captured.showCalls++
			captured.shown = cfg
			_, err := io.WriteString(cfg.Output, "shown\n")
			return err
		},
		ListDispatches: func(cfg sessioncli.DispatchesConfig) error {
			captured.dispatches = cfg
			_, err := io.WriteString(cfg.Output, "dispatches\n")
			return err
		},
		PauseSession: func(cfg sessioncli.LifecycleControlConfig) error {
			captured.paused = cfg
			_, err := io.WriteString(cfg.Output, "paused\n")
			return err
		},
		ResumeSession: func(cfg sessioncli.LifecycleControlConfig) error {
			captured.resumed = cfg
			_, err := io.WriteString(cfg.Output, "resumed\n")
			return err
		},
		PrepareList: func(_ context.Context, _ *sessioncli.ListConfig) error {
			captured.preparedList = true
			return nil
		},
		Diagnostics: func(cmd *cobra.Command) io.Writer {
			return cmd.ErrOrStderr()
		},
	}
}

func assertSessionOperationCaptures(t *testing.T, captured sessionOperationCaptures) {
	t.Helper()
	if created := captured.created; created.Server != "https://factory.example" ||
		created.Dir != "factory-a" || !created.InitNewFactory ||
		created.TargetKind != "named" || created.TargetName != "alpha" ||
		created.Port != 9042 || !created.PortExplicit || !created.JSON ||
		!created.Verbose || !created.Debug || created.Diagnostics == nil {
		t.Fatalf("create config = %#v, want stable local and inherited inputs", created)
	}
	if captured.deleted.SessionID != "session-delete" || !captured.deleted.JSON {
		t.Fatalf("delete config = %#v, want stable positional and inherited inputs", captured.deleted)
	}
	if captured.listed.Server != "https://factory.example" ||
		captured.listed.Scope != "persisted" || !captured.listed.Verbose ||
		!captured.preparedList {
		t.Fatalf("list config = %#v, want stable local and inherited inputs", captured.listed)
	}
	if captured.shown.Server != "https://factory.example" ||
		captured.shown.SessionID != "session-show" || !captured.shown.JSON {
		t.Fatalf("show config = %#v, want stable positional and inherited inputs", captured.shown)
	}
	if captured.dispatches.Server != "https://factory.example" ||
		captured.dispatches.SessionID != "session-dispatches" ||
		captured.dispatches.Phase != "execute" || captured.dispatches.Status != "SUCCEEDED" {
		t.Fatalf("dispatches config = %#v, want stable local and inherited inputs", captured.dispatches)
	}
	if captured.paused.Server != "https://factory.example" || captured.paused.SessionID != "" {
		t.Fatalf("pause config = %#v, want default compatibility target", captured.paused)
	}
	if captured.resumed.Server != "https://factory.example" ||
		captured.resumed.SessionID != "session-resume" || !captured.resumed.JSON {
		t.Fatalf("resume config = %#v, want named target and inherited inputs", captured.resumed)
	}
	if captured.showCalls != 1 {
		t.Fatalf("show calls = %d, want deprecated port rejection before operation", captured.showCalls)
	}
}

func TestResolvedSessionFamilyRejectsMissingOperations(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"create", []string{"session", "create", "--dir", "factory-a"}, "session create service is required"},
		{"delete", []string{"session", "delete", "session-a"}, "session delete service is required"},
		{"list", []string{"session", "list"}, "session list service is required"},
		{"show", []string{"session", "show", "session-a"}, "session show service is required"},
		{"dispatches", []string{"session", "dispatches", "session-a"}, "session dispatches service is required"},
		{"pause", []string{"session", "pause"}, "session lifecycle control handler is required"},
		{"resume", []string{"session", "resume"}, "session lifecycle control handler is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := resolvedSessionFamilyRoot(t, commandregistry.SessionResolvedServices{})
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(test.args)
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Execute(%v) error = %v, want %q", test.args, err, test.want)
			}
		})
	}
}

func TestResolvedSessionFamilyPreservesListDefaultsAndPreparationFailure(t *testing.T) {
	var defaulted sessioncli.ListConfig
	defaultServices := commandregistry.SessionResolvedServices{
		ListSessions: func(cfg sessioncli.ListConfig) error {
			defaulted = cfg
			_, err := io.WriteString(cfg.Output, "listed-default\n")
			return err
		},
	}
	assertResolvedSessionExecution(
		t, defaultServices, []string{"session", "list"}, "listed-default\n",
	)
	if defaulted.Server != "" || defaulted.Scope != "live" || defaulted.Diagnostics != nil {
		t.Fatalf("default list config = %#v, want compatibility defaults", defaulted)
	}

	prepareErr := errors.New("prepare list")
	var listCalls int
	failingServices := commandregistry.SessionResolvedServices{
		ListSessions: func(sessioncli.ListConfig) error {
			listCalls++
			return nil
		},
		PrepareList: func(context.Context, *sessioncli.ListConfig) error {
			return prepareErr
		},
	}
	root := resolvedSessionFamilyRoot(t, failingServices)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"session", "list"})
	if err := root.Execute(); !errors.Is(err, prepareErr) {
		t.Fatalf("Execute(session list) error = %v, want preparation error", err)
	}
	if listCalls != 0 {
		t.Fatalf("list calls = %d, want preparation failure before operation", listCalls)
	}
}

func TestResolvedSessionFamilyRejectsIncompleteInputSnapshotsBeforeOperation(t *testing.T) {
	tests := []struct {
		name             string
		commandID        string
		args             []string
		clearLocalInputs bool
	}{
		{"create local", "you.session.create", []string{"session", "create", "--dir", "factory-a"}, true},
		{"delete local", "you.session.delete", []string{"session", "delete", "session-a"}, true},
		{"list local", "you.session.list", []string{"session", "list"}, true},
		{"show local", "you.session.show", []string{"session", "show", "session-a"}, true},
		{"dispatches local", "you.session.dispatches", []string{"session", "dispatches", "session-a"}, true},
		{"pause inherited", "you.session.pause", []string{"session", "pause"}, false},
		{"resume inherited", "you.session.resume", []string{"session", "resume"}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var operationCalls int
			operation := func() {
				operationCalls++
			}
			services := commandregistry.SessionResolvedServices{
				CreateSession: func(sessioncli.CreateConfig) error { operation(); return nil },
				DeleteSession: func(sessioncli.DeleteConfig) error { operation(); return nil },
				ListSessions:  func(sessioncli.ListConfig) error { operation(); return nil },
				ShowSession:   func(sessioncli.ShowConfig) error { operation(); return nil },
				ListDispatches: func(sessioncli.DispatchesConfig) error {
					operation()
					return nil
				},
				PauseSession:  func(sessioncli.LifecycleControlConfig) error { operation(); return nil },
				ResumeSession: func(sessioncli.LifecycleControlConfig) error { operation(); return nil },
			}
			root := resolvedSessionFamilyRootWithTransform(
				t,
				services,
				func(commandID string, handler commandregistry.ResolvedRunE) commandregistry.ResolvedRunE {
					if commandID != test.commandID {
						return handler
					}
					return func(
						cmd *cobra.Command,
						local resolvedinput.Inputs,
						inherited resolvedinput.Inputs,
					) error {
						if test.clearLocalInputs {
							local = resolvedinput.Inputs{}
						} else {
							inherited = resolvedinput.Inputs{}
						}
						return handler(cmd, local, inherited)
					}
				},
			)
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(test.args)
			if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "resolve session") {
				t.Fatalf("Execute(%v) error = %v, want resolved-input rejection", test.args, err)
			}
			if operationCalls != 0 {
				t.Fatalf("operation calls = %d, want rejection before operation", operationCalls)
			}
		})
	}
}

func assertResolvedSessionExecution(
	t *testing.T,
	services commandregistry.SessionResolvedServices,
	args []string,
	wantOutput string,
) {
	t.Helper()
	root := resolvedSessionFamilyRoot(t, services)
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

func assertResolvedSessionRejection(
	t *testing.T,
	services commandregistry.SessionResolvedServices,
	args []string,
) {
	t.Helper()
	root := resolvedSessionFamilyRoot(t, services)
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	if err := root.Execute(); err == nil {
		t.Fatalf("Execute(%v) error = nil, want deprecated port rejection", args)
	}
	if output.Len() != 0 {
		t.Fatalf("Execute(%v) output = %q, want empty output", args, output.String())
	}
}

func resolvedSessionFamilyRoot(
	t *testing.T,
	services commandregistry.SessionResolvedServices,
) *cobra.Command {
	t.Helper()
	return resolvedSessionFamilyRootWithTransform(t, services, nil)
}

type sessionHandlerTransform func(
	commandID string,
	handler commandregistry.ResolvedRunE,
) commandregistry.ResolvedRunE

func resolvedSessionFamilyRootWithTransform(
	t *testing.T,
	services commandregistry.SessionResolvedServices,
	transform sessionHandlerTransform,
) *cobra.Command {
	t.Helper()
	manifest, err := generated.SessionFamilyManifest()
	if err != nil {
		t.Fatalf("SessionFamilyManifest() error = %v", err)
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

	handlers := commandregistry.BindSessionResolvedHandlers(services)
	byCommandID := map[string]commandregistry.ResolvedRunE{
		"you.session.create": handlers.Create, "you.session.delete": handlers.Delete,
		"you.session.list": handlers.List, "you.session.show": handlers.Show,
		"you.session.dispatches": handlers.Dispatches,
		"you.session.pause":      handlers.Pause, "you.session.resume": handlers.Resume,
	}
	cobraHandlers := make(climanifestcobra.ResolvedCobraHandlerRegistry, len(byCommandID))
	for commandID, handler := range byCommandID {
		if transform != nil {
			handler = transform(commandID, handler)
		}
		record := manifest.Commands[commandID]
		cobraHandlers[record.Handler.ID] = handler
	}
	root, err := (climanifestcobra.GenericConstructor{}).Construct(
		manifest,
		climanifestcobra.GenericBindings{
			Handlers: climanifestcobra.HandlerRegistry{
				rootRecord.Handler.ID: func(context.Context, map[string]any) error { return nil },
			},
			ResolvedCobraHandlers:   cobraHandlers,
			GuardUnknownSubcommands: true,
		},
	)
	if err != nil {
		t.Fatalf("Construct() error = %v", err)
	}
	root.SilenceUsage = true
	return root
}
