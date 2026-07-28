package http

import (
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
)

// ProviderPrerequisiteResponse is the adapter-owned HTTP shape for one provider
// prerequisite.
type ProviderPrerequisiteResponse struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Description string `json:"description,omitempty"`
}

// ProviderDescriptorResponse is the adapter-owned HTTP shape for one catalog
// provider descriptor.
type ProviderDescriptorResponse struct {
	ID            string                         `json:"id"`
	Aliases       []string                       `json:"aliases,omitempty"`
	DisplayName   string                         `json:"displayName"`
	Availability  string                         `json:"availability"`
	Readiness     string                         `json:"readiness"`
	Prerequisites []ProviderPrerequisiteResponse `json:"prerequisites,omitempty"`
	Capabilities  []string                       `json:"capabilities,omitempty"`
}

// ListProvidersResponse is the adapter-owned HTTP success shape for catalog
// list.
type ListProvidersResponse struct {
	Providers []ProviderDescriptorResponse `json:"providers"`
}

// GetProviderResponse is the adapter-owned HTTP success shape for catalog get.
type GetProviderResponse struct {
	Provider ProviderDescriptorResponse `json:"provider"`
}

// GetProviderInput carries decoded HTTP inputs for one catalog get operation
// owned by this adapter.
type GetProviderInput struct {
	ProviderID string
}

// ListProvidersRequestFromHTTP maps one list-providers HTTP request into the
// accepted Providers root request vocabulary.
func ListProvidersRequestFromHTTP() providers.ListProvidersRequest {
	return providers.ListProvidersRequest{}
}

// GetProviderRequestFromHTTP maps one get-provider HTTP request into the
// accepted Providers root request vocabulary.
func GetProviderRequestFromHTTP(input GetProviderInput) (providers.GetProviderRequest, error) {
	request := providers.GetProviderRequest{ID: providers.ID(strings.TrimSpace(input.ProviderID))}
	if err := request.Validate(); err != nil {
		return providers.GetProviderRequest{}, err
	}
	return request, nil
}

// ListProvidersResponseToHTTP encodes one fake-root list result into the
// adapter-owned HTTP success response shape.
func ListProvidersResponseToHTTP(result providers.ListProvidersResult) ListProvidersResponse {
	providersOut := make([]ProviderDescriptorResponse, 0, len(result.Providers))
	for _, descriptor := range result.Providers {
		providersOut = append(providersOut, providerDescriptorToHTTP(descriptor))
	}
	return ListProvidersResponse{Providers: providersOut}
}

// GetProviderResponseToHTTP encodes one fake-root get result into the
// adapter-owned HTTP success response shape.
func GetProviderResponseToHTTP(result providers.GetProviderResult) GetProviderResponse {
	return GetProviderResponse{Provider: providerDescriptorToHTTP(result.Provider)}
}

func providerDescriptorToHTTP(descriptor providers.Descriptor) ProviderDescriptorResponse {
	response := ProviderDescriptorResponse{
		ID:           descriptor.ID.String(),
		DisplayName:  descriptor.DisplayName,
		Availability: string(descriptor.Availability),
		Readiness:    string(descriptor.Readiness),
	}
	if len(descriptor.Aliases) > 0 {
		response.Aliases = append([]string(nil), descriptor.Aliases...)
	}
	if len(descriptor.Prerequisites) > 0 {
		response.Prerequisites = make([]ProviderPrerequisiteResponse, 0, len(descriptor.Prerequisites))
		for _, prerequisite := range descriptor.Prerequisites {
			response.Prerequisites = append(response.Prerequisites, ProviderPrerequisiteResponse{
				Kind:        string(prerequisite.Kind),
				Name:        prerequisite.Name,
				Status:      string(prerequisite.Status),
				Description: prerequisite.Description,
			})
		}
	}
	if len(descriptor.Capabilities) > 0 {
		response.Capabilities = make([]string, 0, len(descriptor.Capabilities))
		for _, capability := range descriptor.Capabilities {
			response.Capabilities = append(response.Capabilities, string(capability))
		}
	}
	return response
}
