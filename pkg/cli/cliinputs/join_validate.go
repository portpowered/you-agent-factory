package cliinputs

import (
	"encoding/json"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/cli/commandidentity"
)

// CommandIdentityIndex maps Batch 01 command paths and idCandidates for join validation.
type CommandIdentityIndex struct {
	byPath        map[string]string
	byIDCandidate map[string]string
}

// NewCommandIdentityIndex builds a join index from Batch 01 command records.
func NewCommandIdentityIndex(commands []commandidentity.CommandRecord) CommandIdentityIndex {
	idx := CommandIdentityIndex{
		byPath:        make(map[string]string, len(commands)),
		byIDCandidate: make(map[string]string, len(commands)),
	}
	for _, record := range commands {
		idx.byPath[record.Path] = record.IDCandidate
		idx.byIDCandidate[record.IDCandidate] = record.Path
	}
	return idx
}

// LoadCommandIdentityIndexFromBaseline decodes a committed cli-commands.json payload.
func LoadCommandIdentityIndexFromBaseline(data []byte) (CommandIdentityIndex, error) {
	var inv commandidentity.Inventory
	if err := json.Unmarshal(data, &inv); err != nil {
		return CommandIdentityIndex{}, fmt.Errorf("decode command identity baseline: %w", err)
	}
	return NewCommandIdentityIndex(inv.Commands), nil
}

// ValidateCommandJoins ensures every inputs record joins to a known Batch 01 command
// identity and that argument/flag idCandidates do not collide within a command.
func ValidateCommandJoins(inv Inventory, index CommandIdentityIndex) error {
	if err := validateArgumentJoins(inv.Arguments, index); err != nil {
		return err
	}
	if err := validateFlagJoins(inv.Flags, index); err != nil {
		return err
	}
	if err := validateRelationshipJoins(inv.Relationships, index); err != nil {
		return err
	}
	return ensureUniquePerCommandIdentities(inv)
}

func validateArgumentJoins(arguments []ArgumentRecord, index CommandIdentityIndex) error {
	for _, record := range arguments {
		if err := validateCommandJoin(record.CommandPath, record.CommandIDCandidate, index); err != nil {
			return fmt.Errorf("argument %q: %w", record.IDCandidate, err)
		}
	}
	return nil
}

func validateFlagJoins(flags []FlagRecord, index CommandIdentityIndex) error {
	for _, record := range flags {
		if err := validateCommandJoin(record.CommandPath, record.CommandIDCandidate, index); err != nil {
			return fmt.Errorf("flag %q: %w", record.IDCandidate, err)
		}
	}
	return nil
}

func validateRelationshipJoins(relationships []RelationshipRecord, index CommandIdentityIndex) error {
	for _, record := range relationships {
		if err := validateCommandJoin(record.CommandPath, record.CommandIDCandidate, index); err != nil {
			return fmt.Errorf("relationship %q: %w", record.IDCandidate, err)
		}
	}
	return nil
}

func validateCommandJoin(path, idCandidate string, index CommandIdentityIndex) error {
	wantID, ok := index.byPath[path]
	if !ok {
		return fmt.Errorf("unknown command path %q", path)
	}
	if idCandidate != wantID {
		return fmt.Errorf(
			"commandIdCandidate %q does not match Batch 01 identity %q for path %q",
			idCandidate,
			wantID,
			path,
		)
	}
	if mappedPath, ok := index.byIDCandidate[idCandidate]; ok && mappedPath != path {
		return fmt.Errorf(
			"commandIdCandidate %q maps to path %q, not %q",
			idCandidate,
			mappedPath,
			path,
		)
	}
	return nil
}

func ensureUniquePerCommandIdentities(inv Inventory) error {
	argumentIDs := make(map[string]map[string]struct{})
	for _, record := range inv.Arguments {
		if err := trackPerCommandIdentity(argumentIDs, record.CommandPath, record.IDCandidate, "argument"); err != nil {
			return err
		}
	}

	flagIDs := make(map[string]map[string]struct{})
	for _, record := range inv.Flags {
		if err := trackPerCommandIdentity(flagIDs, record.CommandPath, record.IDCandidate, "flag"); err != nil {
			return err
		}
	}

	relationshipIDs := make(map[string]map[string]struct{})
	for _, record := range inv.Relationships {
		if err := trackPerCommandIdentity(relationshipIDs, record.CommandPath, record.IDCandidate, "relationship"); err != nil {
			return err
		}
	}
	return nil
}

func trackPerCommandIdentity(
	seen map[string]map[string]struct{},
	commandPath, idCandidate, recordKind string,
) error {
	perCommand := seen[commandPath]
	if perCommand == nil {
		perCommand = make(map[string]struct{})
		seen[commandPath] = perCommand
	}
	if _, exists := perCommand[idCandidate]; exists {
		return fmt.Errorf(
			"duplicate %s idCandidate %q within command %q",
			recordKind,
			idCandidate,
			commandPath,
		)
	}
	perCommand[idCandidate] = struct{}{}
	return nil
}
