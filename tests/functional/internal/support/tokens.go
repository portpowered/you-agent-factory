package support

import (
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func FirstInputToken(rawTokens any) workers.Token {
	switch tokens := rawTokens.(type) {
	case []any:
		if len(tokens) == 0 {
			return workers.Token{}
		}
		tok, ok := tokens[0].(workers.Token)
		if !ok {
			return workers.Token{}
		}
		return tok
	case []workers.Token:
		if len(tokens) == 0 {
			return workers.Token{}
		}
		return tokens[0]
	default:
		return workers.Token{}
	}
}
