package ondemandtarget

import (
	"errors"
	"fmt"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// nextActivationID serializes only ID generation and collision inspection;
// runtime resolution and opening remain concurrent for distinct sessions.
func (s *Service) nextActivationID() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	wrapperID := strings.TrimSpace(s.generateID())
	if wrapperID == "" {
		return "", errors.New("on-demand Factory target activation: generated session identity was blank")
	}
	if _, exists := s.runtimes[wrapperID]; exists {
		return "", fmt.Errorf("on-demand Factory target activation: generated session identity %q collided with an existing activation", wrapperID)
	}
	if _, exists := s.controls[wrapperID]; exists {
		return "", fmt.Errorf("on-demand Factory target activation: generated session identity %q collided with an existing lifecycle control", wrapperID)
	}
	return wrapperID, nil
}

// publishActivation publishes a runtime under its preallocated identity. A
// collision that appeared while the runtime was opening fails without
// replacing or stranding the earlier activation.
func (s *Service) publishActivation(requestID, wrapperID string, active *activatedRuntime) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if wrapperID == "" {
		return errors.New("on-demand Factory target activation: generated session identity was blank")
	}
	if _, exists := s.runtimes[wrapperID]; exists {
		return fmt.Errorf("on-demand Factory target activation: generated session identity %q collided with an existing activation", wrapperID)
	}
	if _, exists := s.controls[wrapperID]; exists {
		return fmt.Errorf("on-demand Factory target activation: generated session identity %q collided with an existing lifecycle control", wrapperID)
	}
	s.runtimes[wrapperID] = active
	s.controls[wrapperID] = &activationControl{
		capturedTurnControls: make(map[capturedTurnControlKey]factoryruntime.TerminateResult),
	}
	if requestID != "" {
		s.startsByRequestID[requestID] = wrapperID
	}
	return nil
}
