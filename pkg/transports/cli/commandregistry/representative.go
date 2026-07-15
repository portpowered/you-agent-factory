package commandregistry

import (
	"fmt"
	"sort"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
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
		return fmt.Errorf(
			"representative runnable command handlers missing for: %v",
			missing,
		)
	}
	return nil
}

// RunnableSessionCommandIDs returns contracted runnable session command IDs in
// stable sorted order and validates their stable handler identities.
func RunnableSessionCommandIDs(manifest climanifest.Manifest) ([]string, error) {
	ids := make([]string, 0, len(climanifestgen.SessionFamilyCommandIDs)-1)
	for _, commandID := range climanifestgen.SessionFamilyCommandIDs {
		record, err := manifest.CommandByID(commandID)
		if err != nil {
			return nil, err
		}
		if !record.Runnable {
			continue
		}
		if record.Handler == nil || record.Handler.ID != commandID+".handler" {
			return nil, fmt.Errorf("session runnable command %q has invalid handler id", commandID)
		}
		ids = append(ids, commandID)
	}
	sort.Strings(ids)
	return ids, nil
}

// VerifySessionRunnableCoverage rejects missing, extra, and cross-family
// registrations so each generated runnable leaf resolves exactly one handler.
func (r *Registry) VerifySessionRunnableCoverage(manifest climanifest.Manifest) error {
	runnableIDs, err := RunnableSessionCommandIDs(manifest)
	if err != nil {
		return err
	}
	want := make(map[string]bool, len(runnableIDs))
	for _, commandID := range runnableIDs {
		want[commandID] = true
	}
	var missing, extra []string
	for _, commandID := range runnableIDs {
		if _, lookupErr := r.Lookup(commandID); lookupErr != nil {
			missing = append(missing, commandID)
		}
	}
	if r != nil {
		for commandID := range r.handlers {
			if !want[commandID] {
				extra = append(extra, commandID)
			}
		}
	}
	sort.Strings(extra)
	if len(missing) > 0 || len(extra) > 0 {
		return fmt.Errorf("session runnable handler coverage mismatch: missing=%v extra=%v", missing, extra)
	}
	return nil
}
