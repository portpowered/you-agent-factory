//go:build windows

package filesystem

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const secureTreeRemovalSupported = true

type windowsDirectory struct {
	handle windows.Handle
}

type windowsEntry struct {
	name      string
	identity  windowsIdentity
	directory bool
	reparse   bool
}

type windowsIdentity struct {
	volume uint32
	index  uint64
}

func removeTreePlatform(ctx context.Context, parentName, targetName string) (bool, error) {
	parent, err := openWindowsParent(parentName)
	if isWindowsNotFound(err) {
		if cancelErr := removalContextError(ctx); cancelErr != nil {
			return false, cancelErr
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open tree parent without reparse traversal: %w", err)
	}
	defer parent.Close()

	entries, err := readWindowsEntries(parent)
	if err != nil {
		return false, fmt.Errorf("read tree parent: %w", err)
	}
	target, targetPresent := findWindowsEntry(entries, targetName)
	if !targetPresent {
		if cancelErr := removalContextError(ctx); cancelErr != nil {
			return false, cancelErr
		}
		return false, nil
	}
	if err := removalContextError(ctx); err != nil {
		return false, err
	}
	targetHandle, err := openWindowsEntry(parent, target)
	if err != nil {
		if isWindowsNotFound(err) {
			if cancelErr := removalContextError(ctx); cancelErr != nil {
				return false, cancelErr
			}
			return false, fmt.Errorf("tree target disappeared during secure open: %w", err)
		}
		return false, fmt.Errorf("open tree target without following reparse points: %w", err)
	}
	defer targetHandle.Close()
	if !target.identity.equal(targetHandle.identity) {
		return false, fmt.Errorf("tree target changed during secure open")
	}
	if !targetHandle.directory {
		return false, fmt.Errorf("tree target is not a directory")
	}

	changed, err := removeWindowsContents(ctx, targetHandle)
	if err != nil {
		return changed, err
	}
	if err := removalContextError(ctx); err != nil {
		return changed, err
	}
	if err := markWindowsDeleted(ctx, targetHandle.handle); err != nil {
		return changed, fmt.Errorf("remove tree target by handle: %w", err)
	}
	if err := removalContextError(ctx); err != nil {
		return true, err
	}
	if err := targetHandle.Close(); err != nil {
		return true, fmt.Errorf("close removed tree target handle: %w", err)
	}
	return true, nil
}

type windowsHandle struct {
	handle    windows.Handle
	identity  windowsIdentity
	directory bool
	reparse   bool
}

func (handle *windowsHandle) Close() error {
	if handle == nil || handle.handle == 0 || handle.handle == windows.InvalidHandle {
		return nil
	}
	file := handle.handle
	handle.handle = 0
	return windows.CloseHandle(file)
}

func openWindowsParent(name string) (*windowsDirectory, error) {
	components, volume, err := windowsParentComponents(name)
	if err != nil {
		return nil, err
	}
	root, err := openWindowsNative(`\??\`+volume+`\`, windowsOpenDirectory, false)
	if err != nil {
		return nil, err
	}
	for _, component := range components {
		next, openErr := openWindowsNativeRelative(root.handle, component, windowsOpenDirectory, false)
		if openErr != nil {
			root.Close()
			return nil, openErr
		}
		root.Close()
		root = next
	}
	return root, nil
}

func windowsParentComponents(name string) ([]string, string, error) {
	if strings.TrimSpace(name) == "" || strings.ContainsRune(name, '\x00') {
		return nil, "", fmt.Errorf("invalid tree parent")
	}
	if strings.HasPrefix(name, `\\`) || strings.HasPrefix(name, `//`) ||
		strings.HasPrefix(name, `\\?\`) || strings.HasPrefix(name, `\\.\`) {
		return nil, "", fmt.Errorf("UNC and device tree parents are unsupported")
	}
	if !filepath.IsAbs(name) {
		return nil, "", fmt.Errorf("tree parent must be an absolute drive path")
	}
	absolute := name
	volume := filepath.VolumeName(absolute)
	if len(volume) != 2 || volume[1] != ':' || !filepath.IsAbs(absolute) {
		return nil, "", fmt.Errorf("tree parent must be an absolute drive path")
	}
	withoutVolume := strings.TrimPrefix(absolute, volume)
	rawComponents := strings.FieldsFunc(withoutVolume, func(r rune) bool { return r == '\\' || r == '/' })
	for _, component := range rawComponents {
		if component == "." || component == ".." || component == "" || strings.ContainsRune(component, ':') {
			return nil, "", fmt.Errorf("invalid tree parent component")
		}
	}
	return rawComponents, volume, nil
}

const (
	windowsOpenDirectory = iota
	windowsOpenEntry
)

func openWindowsNative(name string, kind int, reparsePoint bool) (*windowsDirectory, error) {
	file, err := openWindowsHandle(0, name, kind, reparsePoint)
	if err != nil {
		return nil, err
	}
	if !file.directory {
		file.Close()
		return nil, fmt.Errorf("tree parent component is not a directory")
	}
	return &windowsDirectory{handle: file.handle}, nil
}

func openWindowsNativeRelative(parent windows.Handle, name string, kind int, reparsePoint bool) (*windowsDirectory, error) {
	file, err := openWindowsHandle(parent, name, kind, reparsePoint)
	if err != nil {
		return nil, err
	}
	if !file.directory {
		file.Close()
		return nil, fmt.Errorf("tree parent component is not a directory")
	}
	return &windowsDirectory{handle: file.handle}, nil
}

func openWindowsEntry(parent *windowsDirectory, entry windowsEntry) (*windowsHandle, error) {
	handle, err := openWindowsHandle(
		parent.handle, entry.name, windowsOpenEntry, true,
	)
	if err != nil {
		return nil, err
	}
	if !entry.identity.equal(handle.identity) ||
		entry.directory != handle.directory || entry.reparse != handle.reparse {
		handle.Close()
		return nil, fmt.Errorf("tree entry changed during secure open")
	}
	return handle, nil
}

func openWindowsHandle(parent windows.Handle, name string, kind int, reparsePoint bool) (*windowsHandle, error) {
	// Do not share delete/rename. Once a canonical parent or selected object is
	// open, another same-privilege handle cannot move or replace it while this
	// operation is walking it. Deletion below is still issued against the open
	// handle, never against a name.
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	attributes := uint32(windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE)
	objectAttributes := &windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: parent,
		ObjectName:    objectName,
		Attributes:    attributes,
	}
	access := uint32(windows.FILE_GENERIC_READ)
	if kind == windowsOpenEntry {
		access |= windows.DELETE
	}
	options := uint32(windows.FILE_SYNCHRONOUS_IO_NONALERT | windows.FILE_OPEN_FOR_BACKUP_INTENT)
	if kind == windowsOpenDirectory {
		access |= windows.FILE_LIST_DIRECTORY
		options |= windows.FILE_DIRECTORY_FILE
	}
	if reparsePoint {
		options |= windows.FILE_OPEN_REPARSE_POINT
	}
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	var allocationSize int64
	if err := windows.NtCreateFile(
		&handle,
		access,
		objectAttributes,
		&status,
		&allocationSize,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		windows.FILE_OPEN,
		options,
		0,
		0,
	); err != nil {
		return nil, err
	}
	byHandle, err := windowsFileAttributes(handle)
	if err != nil {
		windows.CloseHandle(handle)
		return nil, err
	}
	reparse := byHandle.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
	directory := byHandle.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 && !reparse
	return &windowsHandle{
		handle: handle,
		identity: windowsIdentity{
			volume: byHandle.VolumeSerialNumber,
			index:  uint64(byHandle.FileIndexHigh)<<32 | uint64(byHandle.FileIndexLow),
		},
		directory: directory,
		reparse:   reparse,
	}, nil
}

func readWindowsEntries(directory *windowsDirectory) ([]windowsEntry, error) {
	buffer := make([]byte, 64*1024)
	result := make([]windowsEntry, 0)
	for {
		// GetFileInformationByHandleEx uses the same directory-information
		// record for the first and subsequent calls.  The RestartInfo class is
		// a separate information class, not a restart flag for this API.
		class := uint32(windows.FileIdBothDirectoryInfo)
		if err := windows.GetFileInformationByHandleEx(
			directory.handle, class, &buffer[0], uint32(len(buffer)),
		); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) || errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
				return result, nil
			}
			return nil, err
		}
		entries, err := parseWindowsDirectoryEntries(buffer)
		if err != nil {
			return nil, err
		}
		result = append(result, entries...)
	}
}

const (
	windowsDirectoryInfoHeaderSize = 104
	windowsDirectoryInfoNameOffset = 104
)

func parseWindowsDirectoryEntries(buffer []byte) ([]windowsEntry, error) {
	entries := make([]windowsEntry, 0)
	for offset := 0; offset < len(buffer); {
		if len(buffer)-offset < windowsDirectoryInfoHeaderSize {
			return nil, fmt.Errorf("short Windows directory information record")
		}
		nextOffset := binary.LittleEndian.Uint32(buffer[offset:])
		nameLength := binary.LittleEndian.Uint32(buffer[offset+60:])
		end := offset + windowsDirectoryInfoNameOffset + int(nameLength)
		if nameLength%2 != 0 || end > len(buffer) {
			return nil, fmt.Errorf("invalid Windows directory entry name length")
		}
		nameWords := make([]uint16, nameLength/2)
		for index := range nameWords {
			nameWords[index] = binary.LittleEndian.Uint16(
				buffer[offset+windowsDirectoryInfoNameOffset+index*2:],
			)
		}
		name := windows.UTF16ToString(nameWords)
		if name != "." && name != ".." {
			if err := validateTreeEntryName(name); err != nil {
				return nil, err
			}
			attributes := binary.LittleEndian.Uint32(buffer[offset+56:])
			entries = append(entries, windowsEntry{
				name: name,
				identity: windowsIdentity{
					index: binary.LittleEndian.Uint64(buffer[offset+96:]),
				},
				directory: attributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 &&
					attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT == 0,
				reparse: attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0,
			})
		}
		if nextOffset == 0 {
			break
		}
		if nextOffset < windowsDirectoryInfoHeaderSize || int(nextOffset) > len(buffer)-offset {
			return nil, fmt.Errorf("invalid Windows directory entry offset")
		}
		offset += int(nextOffset)
	}
	return entries, nil
}

func findWindowsEntry(entries []windowsEntry, name string) (windowsEntry, bool) {
	for _, entry := range entries {
		if strings.EqualFold(entry.name, name) {
			return entry, true
		}
	}
	return windowsEntry{}, false
}

func removeWindowsContents(ctx context.Context, directory *windowsHandle) (bool, error) {
	if err := removalContextError(ctx); err != nil {
		return false, err
	}
	entries, err := readWindowsEntries(&windowsDirectory{handle: directory.handle})
	if err != nil {
		return false, fmt.Errorf("read tree directory: %w", err)
	}
	changed := false
	for _, entry := range entries {
		if err := removalContextError(ctx); err != nil {
			return changed, err
		}
		child, err := openWindowsEntry(&windowsDirectory{handle: directory.handle}, entry)
		if err != nil {
			if isWindowsNotFound(err) {
				if cancelErr := removalContextError(ctx); cancelErr != nil {
					return changed, cancelErr
				}
				continue
			}
			return changed, fmt.Errorf("open tree entry without following reparse points: %w", err)
		}
		if child.directory {
			childChanged, childErr := removeWindowsContents(ctx, child)
			changed = changed || childChanged
			if childErr != nil {
				child.Close()
				return changed, childErr
			}
		}
		if err := removalContextError(ctx); err != nil {
			child.Close()
			return changed, err
		}
		if err := markWindowsDeleted(ctx, child.handle); err != nil {
			child.Close()
			return changed, fmt.Errorf("remove tree entry by handle: %w", err)
		}
		changed = true
		if err := removalContextError(ctx); err != nil {
			child.Close()
			return changed, err
		}
		if err := child.Close(); err != nil {
			return changed, err
		}
	}
	return changed, nil
}

func markWindowsDeleted(ctx context.Context, handle windows.Handle) error {
	if err := removalContextError(ctx); err != nil {
		return err
	}
	flags := uint32(
		windows.FILE_DISPOSITION_DELETE |
			windows.FILE_DISPOSITION_POSIX_SEMANTICS |
			windows.FILE_DISPOSITION_IGNORE_READONLY_ATTRIBUTE,
	)
	var status windows.IO_STATUS_BLOCK
	return windows.NtSetInformationFile(
		handle,
		&status,
		(*byte)(unsafe.Pointer(&flags)),
		uint32(unsafe.Sizeof(flags)),
		windows.FileDispositionInformationEx,
	)
}

func windowsFileAttributes(handle windows.Handle) (windows.ByHandleFileInformation, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return windows.ByHandleFileInformation{}, err
	}
	return info, nil
}

func (identity windowsIdentity) equal(other windowsIdentity) bool {
	if identity.index == 0 || other.index == 0 {
		return false
	}
	if identity.volume != 0 && other.volume != 0 && identity.volume != other.volume {
		return false
	}
	return identity.index == other.index
}

func isWindowsNotFound(err error) bool {
	return errors.Is(err, windows.ERROR_FILE_NOT_FOUND) ||
		errors.Is(err, windows.ERROR_PATH_NOT_FOUND) ||
		errors.Is(err, windows.ERROR_INVALID_NAME) ||
		errors.Is(err, windows.STATUS_OBJECT_NAME_NOT_FOUND) ||
		errors.Is(err, windows.STATUS_OBJECT_PATH_NOT_FOUND)
}

func (directory *windowsDirectory) Close() error {
	if directory == nil || directory.handle == 0 || directory.handle == windows.InvalidHandle {
		return nil
	}
	handle := directory.handle
	directory.handle = 0
	return windows.CloseHandle(handle)
}
