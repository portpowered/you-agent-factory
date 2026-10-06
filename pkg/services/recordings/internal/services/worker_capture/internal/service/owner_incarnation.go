package service

import (
	"encoding/json"
	"errors"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

type captureOwnerIncarnation struct {
	Version           int                         `json:"version"`
	RuntimeInstanceID string                      `json:"runtimeInstanceId"`
	Process           platformprocess.Incarnation `json:"process"`
}

// ownerDeathWitness distinguishes affirmative local death from unknown
// ownership. It never treats an OS query failure as evidence of absence.
func (writer *FileWriter) ownerDeathWitness(epoch string) bool {
	probe, ok := writer.ownerProbe.(interface {
		LookupProcess(int) (platformprocess.Incarnation, error)
	})
	if !ok {
		return false
	}
	owner, valid := decodeCaptureOwner(epoch)
	if !valid {
		return false
	}
	local, err := writer.ownerProbe.CurrentProcess()
	if err != nil || local.Host == "" || local.Host != owner.Process.Host {
		return false
	}
	actual, err := probe.LookupProcess(owner.Process.PID)
	if errors.Is(err, platformprocess.ErrProcessGone) {
		return true
	}
	if err != nil || actual.Host != local.Host || actual.PID != owner.Process.PID || actual.Start == "" {
		return false
	}
	return actual.Start != owner.Process.Start
}

func decodeCaptureOwner(epoch string) (captureOwnerIncarnation, bool) {
	var owner captureOwnerIncarnation
	if json.Unmarshal([]byte(epoch), &owner) != nil || owner.Version != 1 || owner.RuntimeInstanceID == "" ||
		owner.Process.Host == "" || owner.Process.PID <= 0 || owner.Process.Start == "" {
		return captureOwnerIncarnation{}, false
	}
	// Stamps have one canonical encoding. Reject duplicate members, aliases,
	// unknown fields and alternate encodings rather than selecting an identity
	// from ambiguous recovered bytes.
	canonical, err := json.Marshal(owner)
	return owner, err == nil && string(canonical) == epoch
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
