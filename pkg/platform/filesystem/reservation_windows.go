package filesystem

import (
	"errors"
	"io/fs"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// RemoveReserved excludes writers and pathname replacement while inspecting
// and deleting the original placeholder by handle, never by a second lookup.
func (Local) RemoveReserved(path string, original fs.FileInfo) (err error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(name, windows.DELETE|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
		return nil
	}
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(handle), path)
	defer func() { err = errors.Join(err, file.Close()) }()
	current, err := file.Stat()
	if err != nil {
		return err
	}
	if !current.Mode().IsRegular() || !os.SameFile(original, current) || current.Size() != 0 || !original.ModTime().Equal(current.ModTime()) {
		return nil
	}
	var remove byte = 1
	return windows.SetFileInformationByHandle(handle, windows.FileDispositionInfo, &remove, uint32(unsafe.Sizeof(remove)))
}
