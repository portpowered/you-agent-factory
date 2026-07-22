package globalconfiginventory

import "bytes"

// NormalizeFixtureBytes canonicalizes committed baseline fixture bytes so compares
// stay stable when Git checks out files with CRLF on Windows.
func NormalizeFixtureBytes(data []byte) []byte {
	normalized := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	return bytes.ReplaceAll(normalized, []byte("\r"), []byte("\n"))
}

// NormalizeSourceBytes canonicalizes production loader source bytes before hashing.
func NormalizeSourceBytes(data []byte) []byte {
	return NormalizeFixtureBytes(data)
}
