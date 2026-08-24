package commandregistry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"

	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	"github.com/portpowered/infinite-you/pkg/services/work/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

// RunnableRepresentativeCommandIDs returns contracted runnable command IDs for
// the representative family in stable sorted order.
func RunnableRepresentativeCommandIDs(manifest climanifest.Manifest) ([]string, error) {
	ids := make([]string, 0, len(climanifestgen.RepresentativeFamilyCommandIDs))
	for _, commandID := range climanifestgen.RepresentativeFamilyCommandIDs {
		if err := climanifestgen.AssertRepresentativeFamilyCommandID(commandID); err != nil {
			return nil, err
		}
		record, err := manifest.CommandByID(commandID)
		if err != nil {
			return nil, err
		}
		if record.Runnable {
			ids = append(ids, commandID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// VerifyRepresentativeRunnableCoverage fails when any contracted runnable
// representative-family command ID lacks a registered handwritten handler.
func (r *Registry) VerifyRepresentativeRunnableCoverage(manifest climanifest.Manifest) error {
	runnableIDs, err := RunnableRepresentativeCommandIDs(manifest)
	if err != nil {
		return err
	}
	var missing []string
	for _, commandID := range runnableIDs {
		if _, lookupErr := r.Lookup(commandID); lookupErr != nil {
			missing = append(missing, commandID)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("representative runnable command handlers missing for: %v", missing)
	}
	return nil
}

// RunnableSessionHandlerIDs returns the manifest-declared handler IDs for the
// complete runnable Session family. The manifest, rather than command names or
// operation IDs, is the authority for executable binding identity.
func RunnableSessionHandlerIDs(manifest climanifest.Manifest) ([]string, error) {
	if len(manifest.Commands) != len(climanifestgen.SessionFamilyCommandIDs) {
		return nil, fmt.Errorf(
			"session manifest command count = %d, want %d",
			len(manifest.Commands),
			len(climanifestgen.SessionFamilyCommandIDs),
		)
	}
	for commandID := range manifest.Commands {
		if err := climanifestgen.AssertSessionFamilyCommandID(commandID); err != nil {
			return nil, err
		}
	}

	owners := make(map[string]string)
	handlerIDs := make([]string, 0, len(climanifestgen.SessionFamilyCommandIDs)-1)
	for _, commandID := range climanifestgen.SessionFamilyCommandIDs {
		record, err := manifest.CommandByID(commandID)
		if err != nil {
			return nil, err
		}
		if !record.Runnable {
			continue
		}
		if record.Handler == nil || record.Handler.ID == "" {
			return nil, fmt.Errorf("session runnable command %q has no stable handler ID", commandID)
		}
		if owner, exists := owners[record.Handler.ID]; exists {
			return nil, fmt.Errorf(
				"session handler ID %q is duplicated by commands %q and %q",
				record.Handler.ID,
				owner,
				commandID,
			)
		}
		owners[record.Handler.ID] = commandID
		handlerIDs = append(handlerIDs, record.Handler.ID)
	}
	sort.Strings(handlerIDs)
	return handlerIDs, nil
}

// VerifySessionHandlerIDCoverage rejects missing, extra, cross-family, and nil
// executable bindings before a Session command tree can be returned.
func (r *Registry) VerifySessionHandlerIDCoverage(manifest climanifest.Manifest) error {
	handlerIDs, err := RunnableSessionHandlerIDs(manifest)
	if err != nil {
		return err
	}
	expected := make(map[string]struct{}, len(handlerIDs))
	var missing []string
	for _, handlerID := range handlerIDs {
		expected[handlerID] = struct{}{}
		if _, lookupErr := r.LookupHandlers(handlerID); lookupErr != nil {
			missing = append(missing, handlerID)
		}
	}

	var extra []string
	if r != nil {
		for handlerID := range r.handlers {
			if _, ok := expected[handlerID]; !ok {
				extra = append(extra, handlerID)
			}
		}
	}
	sort.Strings(extra)
	if len(missing) > 0 || len(extra) > 0 {
		return fmt.Errorf(
			"session handler ID coverage mismatch: missing=%v extra=%v",
			missing,
			extra,
		)
	}
	return nil
}

// RepresentativeHandlers carries handwritten RunE handlers for contracted runnable
// representative-family command IDs.
type RepresentativeHandlers struct {
	RootRunE RunE
}

const (
	deprecatedSessionPortFlagMessage = "--port is no longer supported; use --server instead (for example, --server http://localhost:7437)"

	sessionServerInputID  = "you.flag.server"
	sessionJSONInputID    = "you.flag.json"
	sessionRemoteInputID  = "you.flag.remote"
	sessionVerboseInputID = "you.flag.verbose"
	sessionDebugInputID   = "you.flag.debug"

	sessionCreateDirInputID                   = "you.session.create.flag.dir"
	sessionCreateInitInputID                  = "you.session.create.flag.init-new-factory"
	sessionCreatePortInputID                  = "you.session.create.flag.port"
	sessionCreateTargetKindInputID            = "you.session.create.flag.target-kind"
	sessionCreateTargetNameInputID            = "you.session.create.flag.target-name"
	sessionCreateValidateInputID              = "you.session.create.flag.validate-only"
	sessionDeleteIDInputID                    = "you.session.delete.arg.0"
	sessionDeletePortInputID                  = "you.session.delete.flag.port"
	sessionListPortInputID                    = "you.session.list.flag.port"
	sessionListScopeInputID                   = "you.session.list.flag.scope"
	sessionListLiveOnlyInputID                = "you.session.list.flag.live-only"
	sessionListHistoryOnlyInputID             = "you.session.list.flag.history-only"
	sessionShowIDInputID                      = "you.session.show.arg.0"
	sessionShowPortInputID                    = "you.session.show.flag.port"
	sessionPauseIDInputID                     = "you.session.pause.arg.0"
	sessionPausePortInputID                   = "you.session.pause.flag.port"
	sessionResumeIDInputID                    = "you.session.resume.arg.0"
	sessionResumePortInputID                  = "you.session.resume.flag.port"
	sessionCancelIDInputID                    = "you.session.cancel.arg.0"
	sessionCancelPortInputID                  = "you.session.cancel.flag.port"
	sessionTerminateIDInputID                 = "you.session.terminate.arg.0"
	sessionTerminatePortInputID               = "you.session.terminate.flag.port"
	sessionResourceSetResourceIDInputID       = "you.session.resource.set.arg.0"
	sessionResourceSetCapacityInputID         = "you.session.resource.set.arg.1"
	sessionResourceSetSessionIDInputID        = "you.session.resource.set.arg.2"
	sessionResourceSetRequestIDInputID        = "you.session.resource.set.flag.request-id"
	sessionResourceSetExpectedRevisionInputID = "you.session.resource.set.flag.expected-revision"
	sessionResourceSetReasonInputID           = "you.session.resource.set.flag.reason"
)

// SessionResolvedServices are the explicit local and remote Factory Session
// CLI adapters and invocation-local collaborators consumed by stable-input
// transport adapters. A placement never falls back to the other adapter.
type SessionResolvedServices struct {
	LocalSessions  sessioncli.Service
	RemoteSessions sessioncli.Service
	PrepareList    func(context.Context, *sessioncli.ListConfig) error
	Diagnostics    func(*cobra.Command) io.Writer
}

// SessionResolvedServicesFromOps binds accepted remote and, when supplied,
// local session operations into registry services. Omitted local operations
// remain absent so local lifecycle placement fails explicitly instead of using
// remote behavior.
func SessionResolvedServicesFromOps(
	remoteOps sessioncli.Operations,
	prepareList func(context.Context, *sessioncli.ListConfig) error,
	diagnostics func(*cobra.Command) io.Writer,
	localOps ...sessioncli.Operations,
) SessionResolvedServices {
	remote := sessioncli.Bind(remoteOps)
	var local sessioncli.Service
	if len(localOps) > 0 {
		local = sessioncli.Bind(localOps[0])
	}
	return SessionResolvedServices{
		LocalSessions:  local,
		RemoteSessions: remote,
		PrepareList:    prepareList,
		Diagnostics:    diagnostics,
	}
}

func (services SessionResolvedServices) remote() sessioncli.Service {
	return services.RemoteSessions
}

func (services SessionResolvedServices) local() sessioncli.Service {
	return services.LocalSessions
}

func (services SessionResolvedServices) forPlacement(remote bool) sessioncli.Service {
	if remote {
		return services.remote()
	}
	return services.local()
}

// SessionResolvedHandler translates stable resolved inputs into the existing
// injected Factory Session operation configs.
type SessionResolvedHandler struct {
	services SessionResolvedServices
}

// SessionResolvedHandlers supplies typed handlers for every runnable Session
// command. Generic manifest projection maps them through stable handler IDs.
type SessionResolvedHandlers struct {
	Create      ResolvedRunE
	Delete      ResolvedRunE
	List        ResolvedRunE
	Show        ResolvedRunE
	Pause       ResolvedRunE
	Resume      ResolvedRunE
	Cancel      ResolvedRunE
	Terminate   ResolvedRunE
	ResourceSet ResolvedRunE
}

// BindSessionResolvedHandlers adapts the injected Factory Session operations
// into invocation-local stable-input handlers.
func BindSessionResolvedHandlers(services SessionResolvedServices) SessionResolvedHandlers {
	handler := &SessionResolvedHandler{services: services}
	return SessionResolvedHandlers{
		Create: handler.Create, Delete: handler.Delete, List: handler.List,
		Show:  handler.Show,
		Pause: handler.Pause, Resume: handler.Resume,
		Cancel: handler.Cancel, Terminate: handler.Terminate,
		ResourceSet: handler.ResourceSet,
	}
}

// NewSessionResolvedRegistry binds all session leaves by manifest handler ID.
func NewSessionResolvedRegistry(
	manifest climanifest.Manifest,
	services SessionResolvedServices,
) (*Registry, error) {
	handlers := BindSessionResolvedHandlers(services)
	bindings := map[string]ResolvedRunE{
		"you.session.create":       handlers.Create,
		"you.session.delete":       handlers.Delete,
		"you.session.list":         handlers.List,
		"you.session.show":         handlers.Show,
		"you.session.pause":        handlers.Pause,
		"you.session.resume":       handlers.Resume,
		"you.session.cancel":       handlers.Cancel,
		"you.session.terminate":    handlers.Terminate,
		"you.session.resource.set": handlers.ResourceSet,
	}
	registry := NewRegistry()
	for commandID, binding := range bindings {
		record, err := manifest.CommandByID(commandID)
		if err != nil {
			return nil, fmt.Errorf("build resolved session handler registry: %w", err)
		}
		if record.Handler == nil || record.Handler.ID == "" {
			return nil, fmt.Errorf("build resolved session handler registry: command %q has no handler ID", commandID)
		}
		if err := registry.RegisterResolved(record.Handler.ID, binding); err != nil {
			return nil, fmt.Errorf("build resolved session handler registry: %w", err)
		}
	}
	if err := registry.VerifySessionHandlerIDCoverage(manifest); err != nil {
		return nil, fmt.Errorf("build resolved session handler registry: %w", err)
	}
	return registry, nil
}

type sessionResolvedGlobals struct {
	server  string
	json    bool
	remote  bool
	verbose bool
	debug   bool
}

func readSessionResolvedGlobals(inputs resolvedinput.Inputs) (sessionResolvedGlobals, error) {
	server, err := inputs.String(sessionServerInputID)
	if err != nil {
		return sessionResolvedGlobals{}, err
	}
	jsonOutput, err := inputs.Bool(sessionJSONInputID)
	if err != nil {
		return sessionResolvedGlobals{}, err
	}
	remote, err := inputs.Bool(sessionRemoteInputID)
	if err != nil {
		// Older unit-level callers resolved session inputs without the root
		// placement flag. The authored command contract defaults it to local.
		if _, present := inputs.State(sessionRemoteInputID); present {
			return sessionResolvedGlobals{}, err
		}
		remote = false
	}
	verbose, err := inputs.Bool(sessionVerboseInputID)
	if err != nil {
		return sessionResolvedGlobals{}, err
	}
	debug, err := inputs.Bool(sessionDebugInputID)
	if err != nil {
		return sessionResolvedGlobals{}, err
	}
	return sessionResolvedGlobals{
		server: server, json: jsonOutput, remote: remote, verbose: verbose || debug, debug: debug,
	}, nil
}

func (h *SessionResolvedHandler) base(
	cmd *cobra.Command,
	inherited resolvedinput.Inputs,
) (sessionResolvedGlobals, io.Writer, error) {
	globals, err := readSessionResolvedGlobals(inherited)
	if err != nil {
		return sessionResolvedGlobals{}, nil, err
	}
	var diagnostics io.Writer
	if h != nil && h.services.Diagnostics != nil {
		diagnostics = h.services.Diagnostics(cmd)
	}
	return globals, diagnostics, nil
}

func optionalSessionID(inputs resolvedinput.Inputs, inputID string) (string, error) {
	if _, present := inputs.State(inputID); !present {
		return "", nil
	}
	return inputs.String(inputID)
}

func rejectDeprecatedSessionPort(inputs resolvedinput.Inputs, inputID string) error {
	state, present := inputs.State(inputID)
	if present && state.Changed {
		return clidiag.NewUsageError("you session", errors.New(deprecatedSessionPortFlagMessage))
	}
	return nil
}

func (h *SessionResolvedHandler) Create(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if h == nil || h.services.remote() == nil {
		return fmt.Errorf("session create service is required")
	}
	dir, err := inputs.String(sessionCreateDirInputID)
	if err != nil {
		return fmt.Errorf("resolve session create inputs: %w", err)
	}
	initNew, err := inputs.Bool(sessionCreateInitInputID)
	if err != nil {
		return fmt.Errorf("resolve session create inputs: %w", err)
	}
	port, err := inputs.Int(sessionCreatePortInputID)
	if err != nil {
		return fmt.Errorf("resolve session create inputs: %w", err)
	}
	targetKind, err := inputs.String(sessionCreateTargetKindInputID)
	if err != nil {
		return fmt.Errorf("resolve session create inputs: %w", err)
	}
	targetName, err := inputs.String(sessionCreateTargetNameInputID)
	if err != nil {
		return fmt.Errorf("resolve session create inputs: %w", err)
	}
	validateOnly, err := inputs.Bool(sessionCreateValidateInputID)
	if err != nil {
		return fmt.Errorf("resolve session create inputs: %w", err)
	}
	globals, diagnostics, err := h.base(cmd, inherited)
	if err != nil {
		return fmt.Errorf("resolve session create inputs: %w", err)
	}
	portState, _ := inputs.State(sessionCreatePortInputID)
	return h.services.remote().Create(sessioncli.CreateConfig{
		Server: globals.server, Port: port, PortExplicit: portState.Changed,
		Dir: dir, InitNewFactory: initNew, ValidateOnly: validateOnly,
		TargetKind: targetKind, TargetName: targetName, JSON: globals.json,
		Verbose: globals.verbose, Debug: globals.debug,
		Output: cmd.OutOrStdout(), Diagnostics: diagnostics,
	})
}

func (h *SessionResolvedHandler) Delete(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if h == nil || h.services.remote() == nil {
		return fmt.Errorf("session delete service is required")
	}
	if err := rejectDeprecatedSessionPort(inputs, sessionDeletePortInputID); err != nil {
		return err
	}
	sessionID, err := inputs.String(sessionDeleteIDInputID)
	if err != nil {
		return fmt.Errorf("resolve session delete inputs: %w", err)
	}
	globals, diagnostics, err := h.base(cmd, inherited)
	if err != nil {
		return fmt.Errorf("resolve session delete inputs: %w", err)
	}
	return h.services.remote().Delete(sessioncli.DeleteConfig{
		Server: globals.server, SessionID: sessionID, JSON: globals.json,
		Verbose: globals.verbose, Debug: globals.debug,
		Output: cmd.OutOrStdout(), Diagnostics: diagnostics,
	})
}

func (h *SessionResolvedHandler) List(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if h == nil || h.services.remote() == nil {
		return fmt.Errorf("session list service is required")
	}
	port, err := inputs.Int(sessionListPortInputID)
	if err != nil {
		return fmt.Errorf("resolve session list inputs: %w", err)
	}
	scope, err := inputs.String(sessionListScopeInputID)
	if err != nil {
		return fmt.Errorf("resolve session list inputs: %w", err)
	}
	liveOnly, err := inputs.Bool(sessionListLiveOnlyInputID)
	if err != nil {
		return fmt.Errorf("resolve session list inputs: %w", err)
	}
	historyOnly, err := inputs.Bool(sessionListHistoryOnlyInputID)
	if err != nil {
		return fmt.Errorf("resolve session list inputs: %w", err)
	}
	globals, diagnostics, err := h.base(cmd, inherited)
	if err != nil {
		return fmt.Errorf("resolve session list inputs: %w", err)
	}
	cfg := sessioncli.ListConfig{
		Context: cmd.Context(), Port: port, Scope: scope, LiveOnly: liveOnly, HistoryOnly: historyOnly, JSON: globals.json,
		Verbose: globals.verbose, Debug: globals.debug,
		Output: cmd.OutOrStdout(), Diagnostics: diagnostics,
	}
	if serverState, ok := inherited.State(sessionServerInputID); ok && serverState.Changed {
		cfg.Server = globals.server
	}
	if h.services.PrepareList != nil {
		if err := h.services.PrepareList(cmd.Context(), &cfg); err != nil {
			return err
		}
	}
	return h.services.remote().List(cfg)
}

func (h *SessionResolvedHandler) Show(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if h == nil || h.services.remote() == nil {
		return fmt.Errorf("session show service is required")
	}
	if err := rejectDeprecatedSessionPort(inputs, sessionShowPortInputID); err != nil {
		return err
	}
	sessionID, err := optionalSessionID(inputs, sessionShowIDInputID)
	if err != nil {
		return fmt.Errorf("resolve session show inputs: %w", err)
	}
	globals, diagnostics, err := h.base(cmd, inherited)
	if err != nil {
		return fmt.Errorf("resolve session show inputs: %w", err)
	}
	return h.services.remote().Show(sessioncli.ShowConfig{
		Context: cmd.Context(), Server: globals.server, SessionID: sessionID,
		JSON: globals.json, Verbose: globals.verbose, Debug: globals.debug,
		Output: cmd.OutOrStdout(), Diagnostics: diagnostics,
	})
}

func (h *SessionResolvedHandler) lifecycle(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
	inputID string,
	portInputID string,
	operation string,
) error {
	if err := rejectDeprecatedSessionPort(inputs, portInputID); err != nil {
		return err
	}
	sessionID, err := optionalSessionID(inputs, inputID)
	if err != nil {
		return fmt.Errorf("resolve session lifecycle inputs: %w", err)
	}
	globals, diagnostics, err := h.base(cmd, inherited)
	if err != nil {
		return fmt.Errorf("resolve session lifecycle inputs: %w", err)
	}
	remote := globals.remote
	if serverState, present := inherited.State(sessionServerInputID); present && serverState.Changed {
		remote = true
	}
	service := h.services.forPlacement(remote)
	if service == nil {
		return fmt.Errorf("session %s service is required for %s placement", operation, map[bool]string{true: "remote", false: "local"}[remote])
	}
	var control func(sessioncli.LifecycleControlConfig) error
	switch operation {
	case "pause":
		control = service.Pause
	case "resume":
		control = service.Resume
	case "cancel":
		control = service.Cancel
	case "terminate":
		control = service.Terminate
	default:
		return fmt.Errorf("unsupported session lifecycle operation %q", operation)
	}
	if control == nil {
		return fmt.Errorf("session %s service is required for %s placement", operation, map[bool]string{true: "remote", false: "local"}[remote])
	}
	return control(sessioncli.LifecycleControlConfig{
		Context: cmd.Context(), Server: globals.server, SessionID: sessionID,
		JSON: globals.json, Verbose: globals.verbose, Debug: globals.debug,
		Output: cmd.OutOrStdout(), Diagnostics: diagnostics,
	})
}

func (h *SessionResolvedHandler) Pause(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if h == nil {
		return fmt.Errorf("session pause handler is required")
	}
	return h.lifecycle(
		cmd, inputs, inherited,
		sessionPauseIDInputID, sessionPausePortInputID,
		"pause",
	)
}

func (h *SessionResolvedHandler) Resume(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if h == nil {
		return fmt.Errorf("session resume handler is required")
	}
	return h.lifecycle(
		cmd, inputs, inherited,
		sessionResumeIDInputID, sessionResumePortInputID,
		"resume",
	)
}

func (h *SessionResolvedHandler) Cancel(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if h == nil {
		return fmt.Errorf("session cancel handler is required")
	}
	return h.lifecycle(
		cmd, inputs, inherited,
		sessionCancelIDInputID, sessionCancelPortInputID,
		"cancel",
	)
}

func (h *SessionResolvedHandler) Terminate(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if h == nil {
		return fmt.Errorf("session terminate handler is required")
	}
	return h.lifecycle(
		cmd, inputs, inherited,
		sessionTerminateIDInputID, sessionTerminatePortInputID,
		"terminate",
	)
}

func (h *SessionResolvedHandler) ResourceSet(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) error {
	if h == nil || h.services.remote() == nil {
		return fmt.Errorf("session resource capacity service is required")
	}
	resourceID, err := inputs.String(sessionResourceSetResourceIDInputID)
	if err != nil {
		return fmt.Errorf("resolve session resource capacity inputs: %w", err)
	}
	capacity, err := inputs.Int(sessionResourceSetCapacityInputID)
	if err != nil {
		return fmt.Errorf("resolve session resource capacity inputs: %w", err)
	}
	sessionID, err := optionalSessionID(inputs, sessionResourceSetSessionIDInputID)
	if err != nil {
		return fmt.Errorf("resolve session resource capacity inputs: %w", err)
	}
	requestID, err := inputs.String(sessionResourceSetRequestIDInputID)
	if err != nil {
		return fmt.Errorf("resolve session resource capacity inputs: %w", err)
	}
	expectedRevision, err := inputs.Int(sessionResourceSetExpectedRevisionInputID)
	if err != nil {
		return fmt.Errorf("resolve session resource capacity inputs: %w", err)
	}
	reason, err := inputs.String(sessionResourceSetReasonInputID)
	if err != nil {
		return fmt.Errorf("resolve session resource capacity inputs: %w", err)
	}
	globals, diagnostics, err := h.base(cmd, inherited)
	if err != nil {
		return fmt.Errorf("resolve session resource capacity inputs: %w", err)
	}
	return h.services.remote().SetResourceCapacity(sessioncli.ResourceCapacityConfig{
		Context: cmd.Context(), Server: globals.server, SessionID: sessionID,
		ResourceID: resourceID, Capacity: capacity, ExpectedRevision: expectedRevision,
		RequestID: requestID, Reason: reason, JSON: globals.json,
		Verbose: globals.verbose, Debug: globals.debug,
		Output: cmd.OutOrStdout(), Diagnostics: diagnostics,
	})
}

// NewRepresentativeRegistry registers handwritten handlers for the representative
// root. Session commands are attached separately by manifest handler ID.
func NewRepresentativeRegistry(handlers RepresentativeHandlers) (*Registry, error) {
	if handlers.RootRunE == nil {
		return nil, fmt.Errorf("build representative handler registry: root handler is required")
	}

	registry := NewRegistry()
	if err := registry.Register("you", handlers.RootRunE); err != nil {
		return nil, fmt.Errorf("build representative handler registry: %w", err)
	}
	return registry, nil
}
