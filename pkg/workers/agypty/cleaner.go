package agypty

import (
	"bytes"
	"strings"
)

// CleanTerminal strips common ANSI CSI and OSC sequences plus carriage-return
// repaint lines from raw PTY capture bytes. Story 18 may extend the cleaning
// corpus; this function is the pure seam Story 17 calls before public emit.
func CleanTerminal(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}

	stripped := stripANSISequences(raw)
	normalized := strings.ReplaceAll(string(stripped), "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if idx := strings.LastIndex(line, "\r"); idx >= 0 {
			line = line[idx+1:]
		}
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.TrimSpace(strings.Join(filtered, "\n"))
}

func stripANSISequences(raw []byte) []byte {
	var out bytes.Buffer
	for i := 0; i < len(raw); {
		if raw[i] != 0x1b {
			out.WriteByte(raw[i])
			i++
			continue
		}
		if i+1 >= len(raw) {
			break
		}
		switch raw[i+1] {
		case '[':
			end := i + 2
			for end < len(raw) && !isCSITerminator(raw[end]) {
				end++
			}
			if end < len(raw) {
				i = end + 1
				continue
			}
		case ']':
			end := bytes.IndexByte(raw[i+2:], 0x07)
			if end >= 0 {
				i = i + 2 + end + 1
				continue
			}
		}
		i++
	}
	return out.Bytes()
}

func isCSITerminator(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}
