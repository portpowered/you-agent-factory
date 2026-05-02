package state

import (
	"fmt"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
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
func GenerateResourcePlaces(def *ResourceDef) (*petri.Place, []*interfaces.Token) {
	placeID := fmt.Sprintf("%s:%s", def.ID, interfaces.ResourceStateAvailable)

	place := &petri.Place{
		ID:     placeID,
		TypeID: def.ID,
		State:  interfaces.ResourceStateAvailable,
	}

	tokens := make([]*interfaces.Token, 0, def.Capacity)
	for i := range def.Capacity {
		tokens = append(tokens, &interfaces.Token{
			// Why?
			ID:      fmt.Sprintf("%s:resource:%d", def.ID, i),
			PlaceID: placeID,
			Color: interfaces.TokenColor{
				WorkID:     fmt.Sprintf("%s:%d", def.ID, i),
				WorkTypeID: def.ID,
				DataType:   interfaces.DataTypeResource,
			},
			CreatedAt: time.Now(),
			EnteredAt: time.Now(),
			History:   interfaces.TokenHistory{},
		})
	}

	return place, tokens
}
