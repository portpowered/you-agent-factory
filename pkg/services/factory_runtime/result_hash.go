package factory

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

func hashLabel(path, text string) string {
	digest := sha256.Sum256([]byte(path + "\n" + text))
	return fmt.Sprintf("artifact-%s", hex.EncodeToString(digest[:8]))
}
