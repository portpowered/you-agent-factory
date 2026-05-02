package functional_test

import "github.com/portpowered/infinite-you/pkg/interfaces"

type tokenIdentitySet struct {
	WorkIDs    []string
	WorkTypes  []string
	TokenNames []string
}

func deriveTokenIdentities(
	consumedTokens []interfaces.Token,
	outputMutations []interfaces.TokenMutationRecord,
) tokenIdentitySet {
	var identities tokenIdentitySet

	for _, token := range consumedTokens {
		addWorkTokenIdentity(&identities, token)
	}
	for _, mutation := range outputMutations {
		if mutation.Token == nil {
			continue
		}
		addWorkTokenIdentity(&identities, *mutation.Token)
	}
	return identities
}

func addWorkTokenIdentity(identities *tokenIdentitySet, token interfaces.Token) {
	if token.Color.DataType == interfaces.DataTypeResource {
		return
	}
	if token.Color.WorkID != "" {
		identities.WorkIDs = appendDistinct(identities.WorkIDs, token.Color.WorkID)
	}
	if token.Color.WorkTypeID != "" {
		identities.WorkTypes = appendDistinct(identities.WorkTypes, token.Color.WorkTypeID)
	}
	if token.Color.Name != "" {
		identities.TokenNames = appendDistinct(identities.TokenNames, token.Color.Name)
	}
}

func appendDistinct(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
