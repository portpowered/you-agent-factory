package service

import (
	"encoding/json"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

type captureOwnerIncarnation struct {
	Version           int                         `json:"version"`
	RuntimeInstanceID string                      `json:"runtimeInstanceId"`
	Process           platformprocess.Incarnation `json:"process"`
}

// Stamp once, at execution capture rather than inert graph construction. A
// failed OS query retains the opaque epoch: it never invents recovery authority.
func (writer *FileWriter) captureOwnerEpoch() string {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.ownerStamped {
		return writer.ownerEpoch
	}
	writer.ownerStamped = true
	if writer.ownerProbe == nil {
		return writer.ownerEpoch
	}
	identity, err := writer.ownerProbe.CurrentProcess()
	if err != nil || identity.Host == "" || identity.PID <= 0 || identity.Start == "" {
		return writer.ownerEpoch
	}
	encoded, err := json.Marshal(captureOwnerIncarnation{Version: 1, RuntimeInstanceID: writer.ownerEpoch, Process: identity})
	if err == nil {
		writer.ownerEpoch = string(encoded)
	}
	return writer.ownerEpoch
}
