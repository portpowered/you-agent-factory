package state

import (
	"fmt"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
)

// ResourceDef defines a resource type and its capacity.
// Resources are modeled as places with bounded tokens.
// A GPU with capacity 1 = a place "gpu:available" pre-loaded with 1 token.
type ResourceDef struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Capacity int    `json:"capacity"` // number of resource tokens to pre-load
}

// GenerateResourcePlaces produces a Place with ID '{resource_id}:available'
// and a slice of pre-loaded resource tokens for the given ResourceDef.
func GenerateResourcePlaces(def *ResourceDef) (*petri.Place, []*factorytoken.Token) {
	placeID := fmt.Sprintf("%s:%s", def.ID, interfaces.ResourceStateAvailable)

	place := &petri.Place{
		ID:     placeID,
		TypeID: def.ID,
		State:  interfaces.ResourceStateAvailable,
	}

	tokens := make([]*factorytoken.Token, 0, def.Capacity)
	for i := range def.Capacity {
		tokens = append(tokens, &factorytoken.Token{
			// Why?
			ID:      fmt.Sprintf("%s:resource:%d", def.ID, i),
			PlaceID: placeID,
			Color: factorytoken.Color{
				WorkID:     fmt.Sprintf("%s:%d", def.ID, i),
				WorkTypeID: def.ID,
				DataType:   factorytoken.DataTypeResource,
			},
			CreatedAt: time.Now(),
			EnteredAt: time.Now(),
			History:   factorytoken.History{},
		})
	}

	return place, tokens
}
