package climanifestcobra

import (
	"fmt"
	"sort"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	sessioncli "github.com/portpowered/infinite-you/pkg/transports/cli/session"
	"github.com/spf13/cobra"
)

// SessionFamilyComponents holds the generated canonical Factory Session tree.
type SessionFamilyComponents struct {
	Session    *cobra.Command
	Create     *cobra.Command
	List       *cobra.Command
	Show       *cobra.Command
	Delete     *cobra.Command
	Pause      *cobra.Command
	Resume     *cobra.Command
	Dispatches *cobra.Command
}

// SessionFamilyBindings supplies live local-flag storage for session handlers.
type SessionFamilyBindings struct {
	Create     *sessioncli.CreateConfig
	List       *sessioncli.ListConfig
	Delete     *sessioncli.DeleteConfig
	Dispatches *sessioncli.DispatchesConfig
	Pause      *sessioncli.LifecycleControlConfig
	Resume     *sessioncli.LifecycleControlConfig
	FlagUsages map[string]string
}

// NewSessionFamilyCommand builds the canonical session parent and all seven
// runnable leaves from generated metadata.
func NewSessionFamilyCommand(registry *commandregistry.Registry, bindings SessionFamilyBindings) (*cobra.Command, error) {
	components, err := NewSessionFamilyComponents(registry, bindings)
	if err != nil {
		return nil, err
	}
	attachSessionLeaves(components)
	return components.Session, nil
}

// NewSessionFamilyComponents builds detached commands from generated typed metadata.
func NewSessionFamilyComponents(registry *commandregistry.Registry, bindings SessionFamilyBindings) (SessionFamilyComponents, error) {
	manifest, err := generated.SessionFamilyManifest()
	if err != nil {
		return SessionFamilyComponents{}, fmt.Errorf("build session family command: %w", err)
	}
	return NewSessionFamilyComponentsFromManifest(manifest, registry, bindings)
}

// NewSessionFamilyComponentsFromManifest validates and builds detached session commands.
func NewSessionFamilyComponentsFromManifest(manifest climanifest.Manifest, registry *commandregistry.Registry, bindings SessionFamilyBindings) (SessionFamilyComponents, error) {
	if registry == nil {
		return SessionFamilyComponents{}, fmt.Errorf("build session family command: registry is required")
	}
	if err := validateSessionBindings(bindings); err != nil {
		return SessionFamilyComponents{}, err
	}
	if err := validateSessionManifest(manifest); err != nil {
		return SessionFamilyComponents{}, fmt.Errorf("build session family command: %w", err)
	}
	if err := registry.VerifySessionRunnableCoverage(manifest); err != nil {
		return SessionFamilyComponents{}, fmt.Errorf("build session family command: %w", err)
	}

	built := make(map[string]*cobra.Command, len(manifest.Commands))
	for _, commandID := range climanifestgen.SessionFamilyCommandIDs {
		record := manifest.Commands[commandID]
		cmd, err := buildSessionCommand(record, registry, bindings)
		if err != nil {
			return SessionFamilyComponents{}, fmt.Errorf("build session family command: %w", err)
		}
		built[commandID] = cmd
	}
	return sessionComponentsFromMap(built)
}

func attachSessionLeaves(components SessionFamilyComponents) {
	components.Session.AddCommand(
		components.List,
		components.Show,
		components.Dispatches,
		components.Pause,
		components.Resume,
		components.Create,
		components.Delete,
	)
}

func sessionComponentsFromMap(built map[string]*cobra.Command) (SessionFamilyComponents, error) {
	components := SessionFamilyComponents{
		Session: built["you.session"], Create: built["you.session.create"],
		List: built["you.session.list"], Show: built["you.session.show"],
		Delete: built["you.session.delete"], Pause: built["you.session.pause"],
		Resume: built["you.session.resume"], Dispatches: built["you.session.dispatches"],
	}
	if components.Session == nil || components.Create == nil || components.List == nil ||
		components.Show == nil || components.Delete == nil || components.Pause == nil ||
		components.Resume == nil || components.Dispatches == nil {
		return SessionFamilyComponents{}, fmt.Errorf("build session family command: incomplete command map")
	}
	return components, nil
}

func buildSessionCommand(record climanifest.Command, registry *commandregistry.Registry, bindings SessionFamilyBindings) (*cobra.Command, error) {
	if err := climanifestgen.AssertSessionFamilyCommandID(record.ID); err != nil {
		return nil, err
	}
	cmd := &cobra.Command{
		Use: record.Usage.Line, Short: record.Documentation.Documentation.Title.CanonicalEnglish,
		Long:    record.Documentation.Documentation.Description.CanonicalEnglish,
		Example: record.Usage.Example, Aliases: append([]string(nil), record.Aliases...),
	}
	cmd.Hidden = record.Visibility == "hidden"
	if !record.Runnable {
		if record.ID != "you.session" {
			return nil, fmt.Errorf("non-runnable command %q must be the session parent", record.ID)
		}
		return cmd, nil
	}
	cmd.Args = positionalArgsFromManifest(record)
	if sessionUsesDeprecatedPort(record.ID) {
		cmd.PreRunE = rejectDeprecatedPortFlag
	}
	if err := registerSessionLocalFlags(cmd, record, bindings); err != nil {
		return nil, err
	}
	if err := applySessionRelationships(cmd, record); err != nil {
		return nil, err
	}
	if err := registry.AttachRunE(cmd, record.ID); err != nil {
		return nil, err
	}
	return cmd, nil
}

func validateSessionManifest(manifest climanifest.Manifest) error {
	if len(manifest.Commands) != len(climanifestgen.SessionFamilyCommandIDs) {
		return fmt.Errorf("manifest command count = %d, want %d session-family commands", len(manifest.Commands), len(climanifestgen.SessionFamilyCommandIDs))
	}
	for commandID := range manifest.Commands {
		if err := climanifestgen.AssertSessionFamilyCommandID(commandID); err != nil {
			return err
		}
	}
	for _, commandID := range climanifestgen.SessionFamilyCommandIDs {
		if _, ok := manifest.Commands[commandID]; !ok {
			return fmt.Errorf("manifest missing session-family command %q", commandID)
		}
	}
	return nil
}

func validateSessionBindings(bindings SessionFamilyBindings) error {
	required := []struct {
		name  string
		value any
	}{
		{"Create", bindings.Create}, {"List", bindings.List}, {"Delete", bindings.Delete},
		{"Dispatches", bindings.Dispatches}, {"Pause", bindings.Pause}, {"Resume", bindings.Resume},
	}
	for _, binding := range required {
		if binding.value == nil {
			return fmt.Errorf("build session family command: bindings.%s is required", binding.name)
		}
	}
	return nil
}

func sessionUsesDeprecatedPort(commandID string) bool {
	switch commandID {
	case "you.session.show", "you.session.pause", "you.session.resume", "you.session.dispatches":
		return true
	default:
		return false
	}
}

func registerSessionLocalFlags(cmd *cobra.Command, record climanifest.Command, bindings SessionFamilyBindings) error {
	var deprecatedPort int
	for _, flag := range sortedFlags(record.Flags) {
		if flag.Scope != "local" {
			continue
		}
		var target flagTarget
		var err error
		if flag.Long == "port" && sessionUsesDeprecatedPort(record.ID) {
			target = flagTarget{intValue: &deprecatedPort}
		} else {
			target, err = sessionLocalBindingTarget(record.ID, flag, bindings)
			if err != nil {
				return err
			}
		}
		if err := registerFlag(cmd.Flags(), flag, target, sessionFlagUsage(record.ID, flag.Long, bindings)); err != nil {
			return fmt.Errorf("register %s local flag %q: %w", record.ID, flag.Long, err)
		}
		if err := applyFlagContract(cmd.Flags().Lookup(flag.Long), flag); err != nil {
			return fmt.Errorf("apply %s local flag %q contract: %w", record.ID, flag.Long, err)
		}
		if flag.Required {
			_ = cmd.MarkFlagRequired(flag.Long)
		}
	}
	return nil
}

func sessionFlagUsage(commandID, longName string, bindings SessionFamilyBindings) string {
	if usage := bindings.FlagUsages[commandID+"."+longName]; usage != "" {
		return usage
	}
	return bindings.FlagUsages[longName]
}

func sessionLocalBindingTarget(commandID string, flag climanifest.Flag, bindings SessionFamilyBindings) (flagTarget, error) {
	switch commandID {
	case "you.session.create":
		return sessionCreateFlagTarget(flag, bindings.Create)
	case "you.session.list":
		return sessionListFlagTarget(flag, bindings.List)
	case "you.session.delete":
		return sessionDeleteFlagTarget(flag, bindings.Delete)
	case "you.session.dispatches":
		return sessionDispatchesFlagTarget(flag, bindings.Dispatches)
	default:
		return flagTarget{}, fmt.Errorf("unsupported local flag %q on %q", flag.Long, commandID)
	}
}

func sessionCreateFlagTarget(flag climanifest.Flag, cfg *sessioncli.CreateConfig) (flagTarget, error) {
	switch flag.Long {
	case "dir":
		return flagTarget{stringValue: &cfg.Dir}, nil
	case "init-new-factory":
		return flagTarget{boolValue: &cfg.InitNewFactory}, nil
	case "validate-only":
		return flagTarget{boolValue: &cfg.ValidateOnly}, nil
	case "target-kind":
		return flagTarget{stringValue: &cfg.TargetKind}, nil
	case "target-name":
		return flagTarget{stringValue: &cfg.TargetName}, nil
	case "port":
		return flagTarget{intValue: &cfg.Port}, nil
	default:
		return flagTarget{}, fmt.Errorf("unsupported create local flag %q", flag.Long)
	}
}

func sessionListFlagTarget(flag climanifest.Flag, cfg *sessioncli.ListConfig) (flagTarget, error) {
	switch flag.Long {
	case "port":
		return flagTarget{intValue: &cfg.Port}, nil
	case "scope":
		return flagTarget{stringValue: &cfg.Scope}, nil
	default:
		return flagTarget{}, fmt.Errorf("unsupported list local flag %q", flag.Long)
	}
}

func sessionDeleteFlagTarget(flag climanifest.Flag, cfg *sessioncli.DeleteConfig) (flagTarget, error) {
	switch flag.Long {
	case "port":
		return flagTarget{intValue: &cfg.Port}, nil
	default:
		return flagTarget{}, fmt.Errorf("unsupported delete local flag %q", flag.Long)
	}
}

func sessionDispatchesFlagTarget(flag climanifest.Flag, cfg *sessioncli.DispatchesConfig) (flagTarget, error) {
	switch flag.Long {
	case "phase":
		return flagTarget{stringValue: &cfg.Phase}, nil
	case "status":
		return flagTarget{stringValue: &cfg.Status}, nil
	default:
		return flagTarget{}, fmt.Errorf("unsupported dispatches local flag %q", flag.Long)
	}
}

func applySessionRelationships(cmd *cobra.Command, record climanifest.Command) error {
	keys := make([]string, 0, len(record.Relationships))
	for key := range record.Relationships {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		relationship := record.Relationships[key]
		names, err := sessionRelationshipFlagNames(record, relationship)
		if err != nil {
			return err
		}
		switch relationship.Kind {
		case "mutually-exclusive":
			cmd.MarkFlagsMutuallyExclusive(names...)
		case "required-together":
			cmd.MarkFlagsRequiredTogether(names...)
		case "at-least-one":
			cmd.MarkFlagsOneRequired(names...)
		default:
			return fmt.Errorf("unsupported relationship %q kind %q", relationship.ID, relationship.Kind)
		}
	}
	return nil
}

func sessionRelationshipFlagNames(record climanifest.Command, relationship climanifest.Relationship) ([]string, error) {
	names := make([]string, 0, len(relationship.Participants))
	for _, participant := range relationship.Participants {
		if participant.Type != "flag" {
			return nil, fmt.Errorf("relationship %q participant %q is not a flag", relationship.ID, participant.ID)
		}
		found := ""
		for _, flag := range record.Flags {
			if flag.ID == participant.ID {
				found = flag.Long
				break
			}
		}
		if found == "" {
			return nil, fmt.Errorf("relationship %q missing participant %q", relationship.ID, participant.ID)
		}
		names = append(names, found)
	}
	return names, nil
}
