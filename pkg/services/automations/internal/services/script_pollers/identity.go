package script_pollers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	// ScriptPollerSourceIDPrefix is the stable source-id namespace for one
	// script-poller workstation supervised through the Automations root.
	ScriptPollerSourceIDPrefix = "script-poller"

	scriptPollerInstanceIDPrefix = "script-poller-instance"
)

// SourceIDForWorkstation returns the Automations-owned source identity for one
// script-poller workstation.
func SourceIDForWorkstation(workstationName string) string {
	name := strings.TrimSpace(workstationName)
	if name == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", ScriptPollerSourceIDPrefix, name)
}

// StableInstanceID returns the opaque instance identity peers use with accepted
// Automations cursor contracts for one script-poller source.
func StableInstanceID(automationID, sourceID string) string {
	automationID = strings.TrimSpace(automationID)
	sourceID = strings.TrimSpace(sourceID)
	identity := fmt.Sprintf("%d:%s:%d:%s", len(automationID), automationID, len(sourceID), sourceID)
	sum := sha256.Sum256([]byte("automations-script-poller-instance:" + identity))
	return scriptPollerInstanceIDPrefix + ":" + hex.EncodeToString(sum[:16])
}

// SupervisionFor builds Automations-owned script-poller supervision facts for
// one workflow-bound workstation.
func SupervisionFor(automationID, workstationName string) ScriptPollerSupervision {
	automationID = strings.TrimSpace(automationID)
	sourceID := SourceIDForWorkstation(workstationName)
	return ScriptPollerSupervision{
		AutomationID: automationID,
		SourceID:     sourceID,
		InstanceID:   StableInstanceID(automationID, sourceID),
	}
}

// IsScriptPollerInstanceID reports whether instanceID belongs to the script-
// poller cursor namespace routed through the Automations root.
func IsScriptPollerInstanceID(instanceID string) bool {
	instanceID = strings.TrimSpace(instanceID)
	return strings.HasPrefix(instanceID, scriptPollerInstanceIDPrefix+":")
}
