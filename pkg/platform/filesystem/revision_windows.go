package filesystem

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ReadRevision opens the selected file for reading and returns its identity,
// size, write time and filesystem change time. Change time is separate from the
// caller-restorable write time. No content or service policy is retained.
// Filesystems without change-time support return an empty revision, requiring
// the consumer to read the content instead.
func (reader RevisionReader) ReadRevision(path string) (revision string, err error) {
	file, err := reader.OpenFile(path)
	if err != nil {
		return "", err
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			revision, err = "", closeErr
		}
	}()
	handle := windows.Handle(file.Fd())
	var filesystemName [16]uint16
	if err := windows.GetVolumeInformationByHandle(handle, nil, 0, nil, nil, nil, &filesystemName[0], uint32(len(filesystemName))); err != nil {
		return "", nil
	}
	name := windows.UTF16ToString(filesystemName[:])
	if name != "NTFS" && name != "ReFS" {
		return "", nil
	}
	var identity windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &identity); err != nil {
		return "", nil
	}
	if identity.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return "", nil
	}
	// FILE_BASIC_INFO has four int64 times followed by attributes and padding.
	// The kernel requires an eight-byte-aligned FILE_BASIC_INFO buffer.
	var basic [5]uint64
	if err := windows.GetFileInformationByHandleEx(handle, windows.FileBasicInfo, (*byte)(unsafe.Pointer(&basic[0])), uint32(unsafe.Sizeof(basic))); err != nil {
		return "", nil
	}
	changeTime := basic[3]
	if changeTime == 0 {
		return "", nil
	}
	return fmt.Sprintf("%x:%x:%x:%x:%x:%x:%x:%x", identity.VolumeSerialNumber,
		identity.FileIndexHigh, identity.FileIndexLow, identity.FileSizeHigh, identity.FileSizeLow,
		identity.LastWriteTime.HighDateTime, identity.LastWriteTime.LowDateTime, changeTime), nil
}
