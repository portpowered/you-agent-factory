package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	scriptPollerSourceIDPrefix   = "script-poller"
	scriptPollerInstanceIDPrefix = "script-poller-instance"
)

func sourceIDForWorkstation(workstationName string) string {
	name := strings.TrimSpace(workstationName)
	if name == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", scriptPollerSourceIDPrefix, name)
}

func stableInstanceID(automationID, sourceID string) string {
	automationID = strings.TrimSpace(automationID)
	sourceID = strings.TrimSpace(sourceID)
	identity := fmt.Sprintf("%d:%s:%d:%s", len(automationID), automationID, len(sourceID), sourceID)
	sum := sha256.Sum256([]byte("automations-script-poller-instance:" + identity))
	return scriptPollerInstanceIDPrefix + ":" + hex.EncodeToString(sum[:16])
}

func supervisionFor(automationID, workstationName string) scriptPollerSupervision {
	automationID = strings.TrimSpace(automationID)
	if automationID == "" {
		return scriptPollerSupervision{}
	}
	sourceID := sourceIDForWorkstation(workstationName)
	return scriptPollerSupervision{
		automationID: automationID,
		sourceID:     sourceID,
		instanceID:   stableInstanceID(automationID, sourceID),
	}
}
