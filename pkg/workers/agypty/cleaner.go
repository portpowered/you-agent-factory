package agypty

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unicode"
)

// CleanTerminal strips ANSI CSI/OSC sequences, control bytes, carriage-return
// repaint lines, and spinner/box-drawing chrome from raw PTY capture bytes.
// Story 17+ calls this before any public response emit; timeout partial output
// uses the same cleaner so public payloads never carry terminal noise.
func CleanTerminal(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}

	stripped := stripANSISequences(raw)
	stripped = stripControlBytes(stripped)
	normalized := strings.ReplaceAll(string(stripped), "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if idx := strings.LastIndex(line, "\r"); idx >= 0 {
			line = line[idx+1:]
		}
		line = strings.TrimRight(line, "\r")
		if isTerminalChromeLine(line) {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.TrimSpace(strings.Join(filtered, "\n"))
}

// ContainsTerminalEscapeOrControl reports whether text still carries bytes that
// must not appear in public timeout or final payloads.
func ContainsTerminalEscapeOrControl(text string) bool {
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\t', '\n', '\r':
			continue
		case 0x1b:
			return true
		default:
			if text[i] < 0x20 || text[i] == 0x7f {
				return true
			}
		}
	}
	return false
}

func stripControlBytes(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	out := make([]byte, 0, len(raw))
	for _, b := range raw {
		switch b {
		case '\t', '\n', '\r':
			out = append(out, b)
		case 0x1b:
			// stripANSISequences should have removed ESC; drop any remainder.
		default:
			if b >= 0x20 && b != 0x7f {
				out = append(out, b)
			}
		}
	}
	return out
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
			i = skipOSCSequence(raw, i)
			continue
		default:
			if raw[i+1] >= 0x40 && raw[i+1] <= 0x5f {
				i += 2
				continue
			}
		}
		i++
	}
	return out.Bytes()
}

func skipOSCSequence(raw []byte, start int) int {
	i := start + 2
	for i < len(raw) {
		if raw[i] == 0x07 {
			return i + 1
		}
		if raw[i] == 0x1b && i+1 < len(raw) && raw[i+1] == '\\' {
			return i + 2
		}
		i++
	}
	return len(raw)
}

func isCSITerminator(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func isTerminalChromeLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return true
	}
	for _, r := range trimmed {
		if unicode.IsSpace(r) {
			continue
		}
		if isSpinnerOrBoxDrawingRune(r) {
			continue
		}
		return false
	}
	return true
}

func isSpinnerOrBoxDrawingRune(r rune) bool {
	switch {
	case r >= 0x2500 && r <= 0x259f: // box drawing and block elements
		return true
	case r >= 0x2800 && r <= 0x28ff: // braille spinner frames
		return true
	case r == '◐' || r == '◓' || r == '◑' || r == '◒' || r == '◴' || r == '◷' || r == '◶' || r == '◵':
		return true
	default:
		return false
	}
}

// TerminalCleaningCase is one hermetic fixture for CleanTerminal.
type TerminalCleaningCase struct {
	Name  string `json:"name"`
	Raw   string `json:"raw_b64"`
	Want  string `json:"want"`
	Empty bool   `json:"empty,omitempty"`
}

type terminalCleaningCasesFile struct {
	Cases []TerminalCleaningCase `json:"cases"`
}

// TerminalCleaningCorpus is the cached terminal-cleaning regression fixture set.
type TerminalCleaningCorpus struct {
	allCases []TerminalCleaningCase
}

// Cases returns all fixtures in file order.
func (c TerminalCleaningCorpus) Cases() []TerminalCleaningCase {
	return append([]TerminalCleaningCase(nil), c.allCases...)
}

//go:embed testdata/terminal_cleaning_corpus.json
var terminalCleaningCasesJSON []byte

var (
	terminalCleaningCorpusOnce sync.Once
	terminalCleaningCorpus     TerminalCleaningCorpus
	terminalCleaningCorpusErr  error
)

// LoadTerminalCleaningCorpus returns the shared terminal-cleaning fixture corpus.
func LoadTerminalCleaningCorpus() (TerminalCleaningCorpus, error) {
	terminalCleaningCorpusOnce.Do(func() {
		terminalCleaningCorpus, terminalCleaningCorpusErr = loadTerminalCleaningCorpus()
	})
	return terminalCleaningCorpus, terminalCleaningCorpusErr
}

func loadTerminalCleaningCorpus() (TerminalCleaningCorpus, error) {
	var raw terminalCleaningCasesFile
	if err := json.Unmarshal(terminalCleaningCasesJSON, &raw); err != nil {
		return TerminalCleaningCorpus{}, fmt.Errorf("decode terminal cleaning cases: %w", err)
	}
	if len(raw.Cases) == 0 {
		return TerminalCleaningCorpus{}, fmt.Errorf("decode terminal cleaning cases: no cases")
	}

	for _, entry := range raw.Cases {
		if entry.Name == "" {
			return TerminalCleaningCorpus{}, fmt.Errorf("decode terminal cleaning cases: missing name")
		}
	}
	return TerminalCleaningCorpus{allCases: raw.Cases}, nil
}

// RawBytes decodes the fixture capture bytes.
func (c TerminalCleaningCase) RawBytes() ([]byte, error) {
	if c.Raw == "" {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(c.Raw)
	if err != nil {
		return nil, fmt.Errorf("decode raw_b64 for %q: %w", c.Name, err)
	}
	return raw, nil
}
