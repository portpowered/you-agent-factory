package subsystems_test

import (
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func firstInputToken(rawTokens []any) workers.Token {
	if len(rawTokens) == 0 {
		return workers.Token{}
	}
	tok, ok := rawTokens[0].(workers.Token)
	if !ok {
		return workers.Token{}
	}
	return tok
}
