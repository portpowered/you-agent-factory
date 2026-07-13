package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(paths []string) error {
	if len(paths) == 0 {
		return errors.New("usage: markdown-linter <file-or-directory> [...]")
	}

	files, err := markdownFiles(paths)
	if err != nil {
		return err
	}

	var failures []string
	for _, path := range files {
		failures = append(failures, lintFile(path)...)
	}
	if len(failures) != 0 {
		return errors.New(strings.Join(failures, "\n"))
	}
	return nil
}

func markdownFiles(paths []string) ([]string, error) {
	var files []string
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("inspect %s: %w", path, err)
		}
		if !info.IsDir() {
			files = append(files, path)
			continue
		}
		err = filepath.WalkDir(path, func(candidate string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !entry.IsDir() && strings.EqualFold(filepath.Ext(candidate), ".md") {
				files = append(files, candidate)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk %s: %w", path, err)
		}
	}
	return files, nil
}

func lintFile(path string) []string {
	content, err := os.ReadFile(path)
	if err != nil {
		return []string{fmt.Sprintf("%s: read: %v", path, err)}
	}

	var failures []string
	if !utf8.Valid(content) {
		failures = append(failures, fmt.Sprintf("%s: content is not valid UTF-8", path))
	}
	if len(content) != 0 && content[len(content)-1] != '\n' {
		failures = append(failures, fmt.Sprintf("%s: missing final newline", path))
	}

	fence := ""
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		trimmed := strings.TrimLeft(line, " ")
		marker := fenceMarker(trimmed)
		if marker == "" {
			continue
		}
		if fence == "" {
			fence = marker
		} else if marker == fence {
			fence = ""
		}
	}
	if err := scanner.Err(); err != nil {
		failures = append(failures, fmt.Sprintf("%s: scan: %v", path, err))
	}
	if fence != "" {
		failures = append(failures, fmt.Sprintf("%s: unclosed %s code fence", path, fence))
	}
	return failures
}

func fenceMarker(line string) string {
	if strings.HasPrefix(line, "```") {
		return "```"
	}
	if strings.HasPrefix(line, "~~~") {
		return "~~~"
	}
	return ""
}
