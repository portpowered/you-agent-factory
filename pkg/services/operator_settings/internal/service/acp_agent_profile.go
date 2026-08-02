package service

import (
	"fmt"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func (s *Service) ResolveACPAgentProfile(
	request operatorsettings.ResolveACPAgentProfileRequest,
) (operatorsettings.ResolveACPAgentProfileResult, error) {
	if s == nil {
		return operatorsettings.ResolveACPAgentProfileResult{}, fmt.Errorf("operator settings service is required")
	}
	if request.AuthoredProfile == nil {
		return operatorsettings.ResolveACPAgentProfileResult{Profile: operatorsettings.BuiltInACPAgentProfile()}, nil
	}
	profile, err := operatorsettings.NormalizeACPAgentProfile(
		request.AuthoredProfile.DefaultFactoryReference,
		request.AuthoredProfile.Allowlist,
	)
	if err != nil {
		return operatorsettings.ResolveACPAgentProfileResult{}, err
	}
	return operatorsettings.ResolveACPAgentProfileResult{Profile: profile}, nil
}
