// Package factorytarget projects a resolved Chat Sessions Factory
// target-catalog result onto the merged ACP Factory target select-option
// contract, so ACP transports consume one deterministic mapping instead of
// building the select-option shape themselves.
package factorytarget

import (
	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/acp"
)

// FromCatalogResult maps a resolved Factory target catalog onto the
// domain-level ACP Factory target option, preserving choice order, choice
// names, and the current/default target exactly. It performs no I/O and
// invokes no downstream service.
func FromCatalogResult(result chatsessions.ResolveFactoryTargetCatalogResult) acp.FactoryTargetOption {
	choices := make([]acp.FactoryTargetChoice, 0, len(result.Choices))
	for _, choice := range result.Choices {
		choices = append(choices, acp.FactoryTargetChoice{
			Value: acpsdk.SessionConfigValueId(choice.Value),
			Name:  choice.Name,
		})
	}
	return acp.FactoryTargetOption{
		CurrentValue: acpsdk.SessionConfigValueId(result.CurrentTarget),
		Choices:      choices,
	}
}
