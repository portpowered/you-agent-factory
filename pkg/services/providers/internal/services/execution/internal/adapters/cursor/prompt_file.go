package cursor

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"unicode/utf16"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// CursorWindowsPromptArgumentLimit preserves the practical limit used for the
// Windows Cursor command shim. Prompt length is measured in UTF-16 code units.
const CursorWindowsPromptArgumentLimit = 7 * 1024

const (
	cursorPromptFilePattern = "cursor_prompt_*.md"
	windowsOperatingSystem  = "windows"
)

type promptFileOperationError struct {
	message string
	cause   error
}

func (e *promptFileOperationError) Error() string { return e.message }
func (e *promptFileOperationError) Unwrap() error { return e.cause }

func materializePrompt(
	ctx context.Context,
	request providers.ExecuteRequest,
	options CommandEffectOptions,
	args []string,
) ([]string, func(), error) {
	if len(args) == 0 {
		return args, nil, nil
	}
	prompt := args[len(args)-1]
	if utf16CodeUnits(prompt) <= CursorWindowsPromptArgumentLimit {
		return args, nil, nil
	}
	operatingSystem := strings.ToLower(strings.TrimSpace(options.OperatingSystem))
	if operatingSystem == "" {
		return nil, nil, errors.New("Cursor operating system is required to submit an oversized prompt.")
	}
	if operatingSystem != windowsOperatingSystem {
		return args, nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if options.TemporaryFiles == nil {
		return nil, nil, errors.New("Cursor temporary-file support is required to submit an oversized prompt.")
	}

	directory := options.TemporaryDir
	if directory == "" {
		directory = request.WorkingDirectory
	}
	if directory == "" {
		directory = "."
	}
	path, cleanup, err := writePromptFile(ctx, options.TemporaryFiles, directory, prompt)
	if err != nil {
		return nil, nil, err
	}
	materialized := append([]string(nil), args...)
	materialized[len(materialized)-1] = "@" + path
	return materialized, cleanup, nil
}

func writePromptFile(
	ctx context.Context,
	files platformfilesystem.TemporaryFileSystem,
	directory, prompt string,
) (string, func(), error) {
	file, createErr := files.CreateTemp(directory, cursorPromptFilePattern)
	if createErr != nil || file == nil {
		newPromptFileCleanup(files, file).cleanup()
		if createErr == nil {
			createErr = errors.New("temporary-file effect returned no file")
		}
		return "", nil, &promptFileOperationError{
			message: "Cursor could not create a temporary file for the oversized prompt.",
			cause:   createErr,
		}
	}
	cleanup := newPromptFileCleanup(files, file)
	if cleanup.path == "" {
		cleanup.cleanup()
		return "", nil, &promptFileOperationError{
			message: "Cursor could not create a temporary file for the oversized prompt.",
			cause:   errors.New("temporary-file effect returned an empty path"),
		}
	}
	written, writeErr := file.WriteString(prompt)
	if writeErr == nil && written != len(prompt) {
		writeErr = io.ErrShortWrite
	}
	if writeErr != nil {
		cleanup.cleanup()
		return "", nil, &promptFileOperationError{
			message: "Cursor could not write the oversized prompt to a temporary file.",
			cause:   writeErr,
		}
	}
	if closeErr := cleanup.close(); closeErr != nil {
		cleanup.remove()
		return "", nil, &promptFileOperationError{
			message: "Cursor could not close the temporary file for the oversized prompt.",
			cause:   closeErr,
		}
	}
	if err := ctx.Err(); err != nil {
		cleanup.remove()
		return "", nil, err
	}
	return cleanup.path, cleanup.remove, nil
}

type promptFileCleanup struct {
	files      platformfilesystem.TemporaryFileSystem
	file       platformfilesystem.TemporaryFile
	path       string
	closeOnce  sync.Once
	closeError error
	removeOnce sync.Once
}

func newPromptFileCleanup(
	files platformfilesystem.TemporaryFileSystem,
	file platformfilesystem.TemporaryFile,
) *promptFileCleanup {
	cleanup := &promptFileCleanup{files: files, file: file}
	if file != nil {
		cleanup.path = file.Name()
	}
	return cleanup
}

func (c *promptFileCleanup) close() error {
	c.closeOnce.Do(func() {
		if c.file != nil {
			c.closeError = c.file.Close()
		}
	})
	return c.closeError
}

func (c *promptFileCleanup) remove() {
	c.removeOnce.Do(func() {
		if c.path != "" {
			_ = c.files.Remove(c.path)
		}
	})
}

func (c *promptFileCleanup) cleanup() {
	_ = c.close()
	c.remove()
}

func utf16CodeUnits(value string) int {
	return len(utf16.Encode([]rune(value)))
}
