//go:build windows

package filesystem

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const secureTreeRemovalSupported = true

const (
	windowsMaxRemovalDepth     = 128
	windowsMaxRemovalEntries   = 100_000
	windowsMaxRemovalNameBytes = 16 * 1024 * 1024
	windowsMaxRemovalMetadata  = 64 * 1024 * 1024
	windowsDirectoryInfoHeader = 88
	windowsDirectoryInfoName   = 88
	windowsMaxParentDepth      = 128
	windowsMaxParentPathBytes  = 32 * 1024
	windowsMaxParentNameBytes  = 1024
)

type windowsDirectory struct {
	handle     windows.Handle
	identity   windowsIdentity
	filesystem string
	ancestors  []windows.Handle
}

type windowsEntry struct {
	name      string
	identity  windowsIdentity
	directory bool
	reparse   bool
}

// FileIdInfo exposes the complete NTFS file identity used by the removal
// boundary. The 128-bit ID is deliberately not reduced to the legacy 64-bit
// FileIndex pair returned by older information classes.
type windowsIdentity struct {
	volume uint64
	fileID [16]byte
}

type windowsRemovalBudget struct {
	entries       int
	nameBytes     int
	metadataBytes int
}

type windowsRemovalProgress struct {
	uncertain bool
}

type windowsFileIDInfo struct {
	volume uint64
	fileID [16]byte
}

func removeTreePlatform(
	ctx context.Context,
	parentName string,
	targetName string,
) (result RemoveTreeResult, err error) {
	result.State = RemoveTreeNotAttempted
	parent, err := openWindowsParent(parentName)
	if isWindowsNotFound(err) {
		if cancelErr := removalContextError(ctx); cancelErr != nil {
			return result, cancelErr
		}
		return RemoveTreeResult{State: RemoveTreeAbsent}, nil
	}
	if err != nil {
		return result, fmt.Errorf("open tree parent without reparse traversal: %w", err)
	}
	defer func() {
		if closeErr := parent.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close tree parent: %w", closeErr))
			result.State = RemoveTreeUnknown
		}
	}()

	budget := windowsRemovalBudget{}
	entries, err := readWindowsEntries(parent, &budget)
	if err != nil {
		return result, fmt.Errorf("read tree parent: %w", err)
	}
	target, present, err := findWindowsEntry(parent, entries, targetName)
	if err != nil {
		return result, fmt.Errorf("select tree target: %w", err)
	}
	if !present {
		if cancelErr := removalContextError(ctx); cancelErr != nil {
			return result, cancelErr
		}
		return RemoveTreeResult{State: RemoveTreeAbsent}, nil
	}
	if err := removalContextError(ctx); err != nil {
		return result, err
	}
	targetHandle, err := openWindowsEntry(parent, target)
	if err != nil {
		if isWindowsNotFound(err) {
			return result, fmt.Errorf("tree target disappeared during secure open: %w", err)
		}
		return result, fmt.Errorf("open tree target without following reparse points: %w", err)
	}
	targetClosed := false
	defer func() {
		if !targetClosed {
			if closeErr := targetHandle.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close tree target: %w", closeErr))
				result.State = RemoveTreeUnknown
			}
		}
	}()

	if !target.directory {
		return result, fmt.Errorf("tree target is not a directory")
	}
	progress, err := removeWindowsContents(ctx, targetHandle, 0, &budget)
	if err != nil {
		if progress.uncertain {
			result.State = RemoveTreeUnknown
		} else {
			result.State = RemoveTreeRemaining
		}
		return result, err
	}
	if err := removalContextError(ctx); err != nil {
		return RemoveTreeResult{State: RemoveTreeRemaining}, err
	}
	// A non-empty Windows directory cannot be detached up front. Keeping the
	// root named until its contents are gone also makes cancellation retryable
	// through the public (parent,target) operation. This disposition is the
	// final destructive commit point; no cancellation check occurs between it
	// and the required handle close.
	if err := markWindowsDeleted(ctx, targetHandle.handle); err != nil {
		return RemoveTreeResult{State: RemoveTreeRemaining}, fmt.Errorf("detach tree target by handle: %w", err)
	}
	if err := targetHandle.Close(); err != nil {
		targetClosed = true
		return RemoveTreeResult{State: RemoveTreeUnknown}, fmt.Errorf("close removed tree target handle: %w", err)
	}
	targetClosed = true
	result.State = RemoveTreeRemoved
	if err := removalContextError(ctx); err != nil {
		return result, err
	}
	return result, nil
}

func openWindowsParent(name string) (*windowsDirectory, error) {
	components, volume, err := windowsParentComponents(name)
	if err != nil {
		return nil, err
	}
	current, err := openWindowsNative(`\??\`+volume+`\`, windowsOpenDirectory, false)
	if err != nil {
		return nil, err
	}
	for _, component := range components {
		next, openErr := openWindowsNativeRelative(current, component, windowsOpenDirectory, false)
		if openErr != nil {
			return nil, closeWindowsDirectoryOnError(current, openErr)
		}
		next.ancestors = append(next.ancestors, current.ancestors...)
		next.ancestors = append(next.ancestors, current.handle)
		current = next
	}
	return current, nil
}

func closeWindowsNativeHandleOnError(handle windows.Handle, err error) error {
	return errors.Join(err, windows.CloseHandle(handle))
}

func closeWindowsHandleOnError(handle *windowsHandle, err error) error {
	if handle == nil {
		return err
	}
	return errors.Join(err, handle.Close())
}

func closeWindowsDirectoryOnError(directory *windowsDirectory, err error) error {
	if directory == nil {
		return err
	}
	return errors.Join(err, directory.Close())
}

func windowsParentComponents(name string) ([]string, string, error) {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(name) != name || strings.ContainsRune(name, '\x00') {
		return nil, "", fmt.Errorf("invalid tree parent")
	}
	if len(name) > windowsMaxParentPathBytes {
		return nil, "", fmt.Errorf("tree parent exceeds bounded path length")
	}
	if strings.HasPrefix(name, `\\`) || strings.HasPrefix(name, `//`) ||
		strings.HasPrefix(name, `\\?\`) || strings.HasPrefix(name, `\\.\`) {
		return nil, "", fmt.Errorf("UNC and device tree parents are unsupported")
	}
	if !filepath.IsAbs(name) {
		return nil, "", fmt.Errorf("tree parent must be an absolute drive path")
	}
	volume := filepath.VolumeName(name)
	if len(volume) != 2 || volume[1] != ':' || !filepath.IsAbs(name) {
		return nil, "", fmt.Errorf("tree parent must be an absolute drive path")
	}
	withoutVolume := strings.TrimPrefix(name, volume)
	rawComponents := strings.FieldsFunc(withoutVolume, func(r rune) bool { return r == '\\' || r == '/' })
	if len(rawComponents) > windowsMaxParentDepth {
		return nil, "", fmt.Errorf("tree parent exceeds bounded depth")
	}
	for _, component := range rawComponents {
		if len(component) > windowsMaxParentNameBytes {
			return nil, "", fmt.Errorf("tree parent component exceeds bounded name length")
		}
		if component == "." || component == ".." || component == "" || strings.ContainsRune(component, ':') {
			return nil, "", fmt.Errorf("invalid tree parent component")
		}
	}
	absolute := filepath.Clean(name)
	if !filepath.IsAbs(absolute) {
		return nil, "", fmt.Errorf("tree parent must be an absolute drive path")
	}
	return rawComponents, volume, nil
}

const (
	windowsOpenDirectory = iota
	windowsOpenEntry
)

func openWindowsNative(name string, kind int, reparsePoint bool) (*windowsDirectory, error) {
	file, err := openWindowsHandle(0, name, kind, reparsePoint, false)
	if err != nil {
		return nil, err
	}
	if !file.directory {
		return nil, closeWindowsHandleOnError(file, fmt.Errorf("tree parent component is not a directory"))
	}
	filesystem, err := windowsFileSystemName(file.handle)
	if err != nil {
		return nil, closeWindowsHandleOnError(file, fmt.Errorf("identify tree parent filesystem: %w", err))
	}
	if !strings.EqualFold(filesystem, "NTFS") {
		return nil, closeWindowsHandleOnError(file, fmt.Errorf("tree removal requires NTFS, found %q", filesystem))
	}
	return &windowsDirectory{
		handle: file.handle, identity: file.identity, filesystem: filesystem,
	}, nil
}

func openWindowsNativeRelative(
	parent *windowsDirectory,
	name string,
	kind int,
	reparsePoint bool,
) (*windowsDirectory, error) {
	caseSensitive, err := windowsCaseSensitiveDirectory(parent.handle)
	if err != nil {
		return nil, fmt.Errorf("identify tree parent case semantics: %w", err)
	}
	entries, err := readWindowsEntries(parent, &windowsRemovalBudget{})
	if err != nil {
		return nil, fmt.Errorf("read tree parent component: %w", err)
	}
	entry, present, err := findWindowsEntryWithCase(entries, name, caseSensitive)
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, windows.ERROR_PATH_NOT_FOUND
	}
	if !entry.directory {
		return nil, fmt.Errorf("tree parent component is not a directory")
	}
	file, err := openWindowsHandle(parent.handle, entry.name, kind, reparsePoint, caseSensitive)
	if err != nil {
		return nil, err
	}
	if !file.directory {
		return nil, closeWindowsHandleOnError(file, fmt.Errorf("tree parent component is not a directory"))
	}
	if !entry.identity.equal(file.identity) || entry.reparse != file.reparse {
		return nil, closeWindowsHandleOnError(file, fmt.Errorf("tree parent component changed during secure open"))
	}
	filesystem, err := windowsFileSystemName(file.handle)
	if err != nil {
		return nil, closeWindowsHandleOnError(file, fmt.Errorf("identify tree parent filesystem: %w", err))
	}
	if !strings.EqualFold(filesystem, parent.filesystem) || file.identity.volume != parent.identity.volume {
		return nil, closeWindowsHandleOnError(file, fmt.Errorf("tree parent crossed an unsupported filesystem boundary"))
	}
	return &windowsDirectory{
		handle: file.handle, identity: file.identity, filesystem: filesystem,
	}, nil
}

func openWindowsEntry(parent *windowsDirectory, entry windowsEntry) (*windowsHandle, error) {
	caseSensitive, err := windowsCaseSensitiveDirectory(parent.handle)
	if err != nil {
		return nil, fmt.Errorf("identify tree directory case semantics: %w", err)
	}
	handle, err := openWindowsHandle(parent.handle, entry.name, windowsOpenEntry, true, caseSensitive)
	if err != nil {
		return nil, err
	}
	if !entry.identity.equal(handle.identity) ||
		entry.directory != handle.directory || entry.reparse != handle.reparse {
		return nil, closeWindowsHandleOnError(handle, fmt.Errorf("tree entry changed during secure open"))
	}
	return handle, nil
}

func openWindowsHandle(parent windows.Handle, name string, kind int, reparsePoint bool, caseSensitive bool) (*windowsHandle, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	attributes := uint32(windows.OBJ_DONT_REPARSE)
	if !caseSensitive {
		attributes |= windows.OBJ_CASE_INSENSITIVE
	}
	objectAttributes := &windows.OBJECT_ATTRIBUTES{
		Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})), RootDirectory: parent,
		ObjectName: objectName, Attributes: attributes,
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
		&handle, access, objectAttributes, &status, &allocationSize, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, windows.FILE_OPEN,
		options, 0, 0,
	); err != nil {
		return nil, err
	}
	byHandle, err := windowsFileAttributes(handle)
	if err != nil {
		return nil, closeWindowsNativeHandleOnError(handle, err)
	}
	identity, err := windowsFileIdentity(handle)
	if err != nil {
		return nil, closeWindowsNativeHandleOnError(handle, err)
	}
	reparse := byHandle.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
	directory := byHandle.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 && !reparse
	return &windowsHandle{handle: handle, identity: identity, directory: directory, reparse: reparse}, nil
}

func readWindowsEntries(directory *windowsDirectory, budget *windowsRemovalBudget) ([]windowsEntry, error) {
	buffer := make([]byte, 64*1024)
	result := make([]windowsEntry, 0)
	for {
		class := uint32(windows.FileIdExtdDirectoryInfo)
		if err := windows.GetFileInformationByHandleEx(
			directory.handle, class, &buffer[0], uint32(len(buffer)),
		); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) || errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
				sort.Slice(result, func(i, j int) bool {
					if result[i].name != result[j].name {
						return result[i].name < result[j].name
					}
					if result[i].identity.volume != result[j].identity.volume {
						return result[i].identity.volume < result[j].identity.volume
					}
					return bytes.Compare(result[i].identity.fileID[:], result[j].identity.fileID[:]) < 0
				})
				return result, nil
			}
			return nil, err
		}
		entries, err := parseWindowsDirectoryEntries(buffer, directory.identity.volume, budget)
		if err != nil {
			return nil, err
		}
		result = append(result, entries...)
	}
}

func parseWindowsDirectoryEntries(
	buffer []byte,
	volume uint64,
	budget *windowsRemovalBudget,
) ([]windowsEntry, error) {
	entries := make([]windowsEntry, 0)
	for offset := 0; offset < len(buffer); {
		if len(buffer)-offset < windowsDirectoryInfoHeader {
			return nil, fmt.Errorf("short Windows directory information record")
		}
		nextOffset := binary.LittleEndian.Uint32(buffer[offset:])
		nameLength := binary.LittleEndian.Uint32(buffer[offset+60:])
		end := offset + windowsDirectoryInfoName + int(nameLength)
		if nameLength%2 != 0 || end > len(buffer) {
			return nil, fmt.Errorf("invalid Windows directory entry name length")
		}
		nameWords := make([]uint16, nameLength/2)
		for index := range nameWords {
			nameWords[index] = binary.LittleEndian.Uint16(buffer[offset+windowsDirectoryInfoName+index*2:])
		}
		name := windows.UTF16ToString(nameWords)
		if name != "." && name != ".." {
			if err := validateTreeEntryName(name); err != nil {
				return nil, err
			}
			if err := budget.add(name); err != nil {
				return nil, err
			}
			attributes := binary.LittleEndian.Uint32(buffer[offset+56:])
			var fileID [16]byte
			copy(fileID[:], buffer[offset+72:offset+88])
			identity := windowsIdentity{volume: volume, fileID: fileID}
			if !identity.valid() {
				return nil, fmt.Errorf("Windows returned an incomplete directory entry identity")
			}
			entries = append(entries, windowsEntry{
				name:      name,
				identity:  identity,
				directory: attributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 && attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT == 0,
				reparse:   attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0,
			})
		}
		if nextOffset == 0 {
			break
		}
		if nextOffset < windowsDirectoryInfoHeader || int(nextOffset) > len(buffer)-offset {
			return nil, fmt.Errorf("invalid Windows directory entry offset")
		}
		offset += int(nextOffset)
	}
	return entries, nil
}

func (budget *windowsRemovalBudget) add(name string) error {
	if budget == nil {
		return fmt.Errorf("tree removal budget is missing")
	}
	budget.entries++
	budget.nameBytes += len(name)
	budget.metadataBytes += int(unsafe.Sizeof(windowsEntry{})) + len(name)
	if budget.entries > windowsMaxRemovalEntries {
		return fmt.Errorf("tree removal entry limit exceeded")
	}
	if budget.nameBytes > windowsMaxRemovalNameBytes {
		return fmt.Errorf("tree removal name-memory limit exceeded")
	}
	if budget.metadataBytes > windowsMaxRemovalMetadata {
		return fmt.Errorf("tree removal metadata-memory limit exceeded")
	}
	return nil
}

func findWindowsEntry(
	directory *windowsDirectory,
	entries []windowsEntry,
	name string,
) (windowsEntry, bool, error) {
	caseSensitive, err := windowsCaseSensitiveDirectory(directory.handle)
	if err != nil {
		return windowsEntry{}, false, err
	}
	return findWindowsEntryWithCase(entries, name, caseSensitive)
}

func findWindowsEntryWithCase(
	entries []windowsEntry,
	name string,
	caseSensitive bool,
) (windowsEntry, bool, error) {
	for _, entry := range entries {
		if entry.name == name {
			return entry, true, nil
		}
	}
	if caseSensitive {
		return windowsEntry{}, false, nil
	}
	var match windowsEntry
	matches := 0
	for _, entry := range entries {
		if strings.EqualFold(entry.name, name) {
			match = entry
			matches++
		}
	}
	if matches > 1 {
		return windowsEntry{}, false, fmt.Errorf("ambiguous case-insensitive tree target")
	}
	return match, matches == 1, nil
}

func removeWindowsContents(
	ctx context.Context,
	directory *windowsHandle,
	depth int,
	budget *windowsRemovalBudget,
) (windowsRemovalProgress, error) {
	progress := windowsRemovalProgress{}
	if err := removalContextError(ctx); err != nil {
		return progress, err
	}
	directoryView := &windowsDirectory{handle: directory.handle, identity: directory.identity}
	entries, err := readWindowsEntries(directoryView, budget)
	if err != nil {
		return progress, fmt.Errorf("read tree directory: %w", err)
	}
	if depth >= windowsMaxRemovalDepth && len(entries) > 0 {
		return progress, fmt.Errorf("tree removal depth limit exceeded")
	}
	for _, entry := range entries {
		if err := removalContextError(ctx); err != nil {
			return progress, err
		}
		child, err := openWindowsEntry(directoryView, entry)
		if err != nil {
			if isWindowsNotFound(err) {
				if cancelErr := removalContextError(ctx); cancelErr != nil {
					return progress, cancelErr
				}
				continue
			}
			return progress, fmt.Errorf("open tree entry without following reparse points: %w", err)
		}
		childProgress, childErr := removeWindowsChild(ctx, entry, child, depth, budget)
		progress.uncertain = progress.uncertain || childProgress.uncertain
		if childErr != nil {
			return progress, childErr
		}
	}
	return progress, nil
}

func removeWindowsChild(
	ctx context.Context,
	entry windowsEntry,
	child *windowsHandle,
	depth int,
	budget *windowsRemovalBudget,
) (windowsRemovalProgress, error) {
	progress := windowsRemovalProgress{}
	if child.directory {
		childProgress, err := removeWindowsContents(ctx, child, depth+1, budget)
		progress = childProgress
		if err != nil {
			if closeErr := child.Close(); closeErr != nil {
				progress.uncertain = true
				return progress, errors.Join(err, fmt.Errorf("close tree directory %q: %w", entry.name, closeErr))
			}
			return progress, fmt.Errorf("remove tree directory %q: %w", entry.name, err)
		}
	}
	if err := removalContextError(ctx); err != nil {
		if closeErr := child.Close(); closeErr != nil {
			progress.uncertain = true
			return progress, errors.Join(err, fmt.Errorf("close tree entry %q: %w", entry.name, closeErr))
		}
		return progress, err
	}
	if err := markWindowsDeleted(ctx, child.handle); err != nil {
		if closeErr := child.Close(); closeErr != nil {
			progress.uncertain = true
			return progress, errors.Join(err, fmt.Errorf("remove tree entry by handle: %w", err), fmt.Errorf("close tree entry %q: %w", entry.name, closeErr))
		}
		return progress, fmt.Errorf("remove tree entry by handle: %w", err)
	}
	if err := removalContextError(ctx); err != nil {
		if closeErr := child.Close(); closeErr != nil {
			progress.uncertain = true
			return progress, errors.Join(err, fmt.Errorf("close removed tree entry %q: %w", entry.name, closeErr))
		}
		return progress, err
	}
	if err := child.Close(); err != nil {
		progress.uncertain = true
		return progress, fmt.Errorf("close removed tree entry %q: %w", entry.name, err)
	}
	return progress, nil
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
		handle, &status, (*byte)(unsafe.Pointer(&flags)), uint32(unsafe.Sizeof(flags)), windows.FileDispositionInformationEx,
	)
}

func windowsFileAttributes(handle windows.Handle) (windows.ByHandleFileInformation, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return windows.ByHandleFileInformation{}, err
	}
	return info, nil
}

func windowsFileIdentity(handle windows.Handle) (windowsIdentity, error) {
	var info windowsFileIDInfo
	if err := windows.GetFileInformationByHandleEx(
		handle, windows.FileIdInfo, (*byte)(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)),
	); err != nil {
		return windowsIdentity{}, err
	}
	identity := windowsIdentity{volume: info.volume, fileID: info.fileID}
	if !identity.valid() {
		return windowsIdentity{}, fmt.Errorf("Windows returned an incomplete filesystem identity")
	}
	return identity, nil
}

func windowsFileSystemName(handle windows.Handle) (string, error) {
	var volumeName [256]uint16
	var filesystemName [256]uint16
	var serial, maxComponentLength, flags uint32
	if err := windows.GetVolumeInformationByHandle(
		handle, &volumeName[0], uint32(len(volumeName)), &serial, &maxComponentLength, &flags,
		&filesystemName[0], uint32(len(filesystemName)),
	); err != nil {
		return "", err
	}
	return windows.UTF16ToString(filesystemName[:]), nil
}

func windowsCaseSensitiveDirectory(handle windows.Handle) (bool, error) {
	var info uint32
	if err := windows.GetFileInformationByHandleEx(
		handle, windows.FileCaseSensitiveInfo, (*byte)(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)),
	); err != nil {
		return false, err
	}
	return info&windows.FILE_CS_FLAG_CASE_SENSITIVE_DIR != 0, nil
}

func (identity windowsIdentity) valid() bool {
	if identity.volume == 0 {
		return false
	}
	var zero [16]byte
	return identity.fileID != zero
}

func (identity windowsIdentity) equal(other windowsIdentity) bool {
	return identity.valid() && other.valid() && identity.volume == other.volume && identity.fileID == other.fileID
}

func isWindowsNotFound(err error) bool {
	return errors.Is(err, windows.ERROR_FILE_NOT_FOUND) ||
		errors.Is(err, windows.ERROR_PATH_NOT_FOUND) ||
		errors.Is(err, windows.ERROR_INVALID_NAME) ||
		errors.Is(err, windows.STATUS_OBJECT_NAME_NOT_FOUND) ||
		errors.Is(err, windows.STATUS_OBJECT_PATH_NOT_FOUND)
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

func (directory *windowsDirectory) Close() error {
	if directory == nil {
		return nil
	}
	var closeErr error
	if directory.handle != 0 && directory.handle != windows.InvalidHandle {
		handle := directory.handle
		directory.handle = 0
		closeErr = errors.Join(closeErr, windows.CloseHandle(handle))
	}
	for index := len(directory.ancestors) - 1; index >= 0; index-- {
		handle := directory.ancestors[index]
		if handle == 0 || handle == windows.InvalidHandle {
			continue
		}
		closeErr = errors.Join(closeErr, windows.CloseHandle(handle))
	}
	directory.ancestors = nil
	return closeErr
}
