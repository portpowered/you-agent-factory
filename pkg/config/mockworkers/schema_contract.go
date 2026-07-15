package mockworkers

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"unicode"
)

// SchemaRelativePath is the authored Draft-2020-12 mock-worker configuration schema.
const SchemaRelativePath = "contracts/config/mock-workers.schema.json"

// SchemaID is the stable $id for the mock-worker configuration schema.
const SchemaID = "https://schemas.portpowered.com/you/config/mock-workers.schema.json"

// ContractFormatVersion is the authored contract metadata format version.
const ContractFormatVersion = "1.0.0"

// DocumentationIDPrefix is the stable documentation item ID namespace for mock-worker fields.
const DocumentationIDPrefix = "config.mock-workers"

// DocumentationItemID maps an inventoried field ID to its stable documentation item ID.
func DocumentationItemID(inventoryID string) string {
	switch inventoryID {
	case "mockWorkers":
		return DocumentationIDPrefix
	case "unmatchedDispatchPolicy":
		return DocumentationIDPrefix + ".unmatched-dispatch-policy"
	}
	trimmed := strings.TrimPrefix(inventoryID, "mockWorkers[].")
	trimmed = strings.ReplaceAll(trimmed, "[]", "")
	return DocumentationIDPrefix + "." + camelPathToKebab(trimmed)
}

// DocumentationSourceHash returns a deterministic SHA-256 digest for contract metadata.
func DocumentationSourceHash(parts ...string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(parts, "\n"))))
}

func camelPathToKebab(path string) string {
	segments := strings.Split(path, ".")
	for i, segment := range segments {
		segments[i] = camelToKebab(segment)
	}
	return strings.Join(segments, ".")
}

func camelToKebab(value string) string {
	if value == "" {
		return value
	}
	var builder strings.Builder
	for i, r := range value {
		if unicode.IsUpper(r) {
			if i > 0 {
				builder.WriteByte('-')
			}
			builder.WriteRune(unicode.ToLower(r))
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}
