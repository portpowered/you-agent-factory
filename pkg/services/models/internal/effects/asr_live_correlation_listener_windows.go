//go:build windows

package effects

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	asrCorrelationAFInet                   = 2
	asrCorrelationAFInet6                  = 23
	asrCorrelationTCPTableOwnerPIDListener = 3
	asrCorrelationTCPRowOwnerPIDSize       = 24
	asrCorrelationTCP6RowOwnerPIDSize      = 56
)

var asrCorrelationGetExtendedTCPTable = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetExtendedTcpTable")

// WindowsASRLiveCorrelationListenerPIDLookup resolves the PID that owns the
// exact loopback listener using the Windows TCP owner table.
func WindowsASRLiveCorrelationListenerPIDLookup(
	ctx context.Context,
	host string,
	port int,
) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil || !validASRCorrelationPort(port) || port == 7437 ||
		(host != "127.0.0.1" && host != "::1") {
		return 0, ErrASRLiveCorrelationOwnership
	}
	addressFamily, rowSize := uint32(asrCorrelationAFInet), asrCorrelationTCPRowOwnerPIDSize
	if host == "::1" {
		addressFamily, rowSize = asrCorrelationAFInet6, asrCorrelationTCP6RowOwnerPIDSize
	}
	buffer, err := asrCorrelationTCPTable(addressFamily)
	if err != nil {
		return 0, ErrASRLiveCorrelationListenerLookup
	}
	if ctx.Err() != nil {
		return 0, ErrASRLiveCorrelationOwnership
	}
	return asrCorrelationListenerPIDFromTCPTable(buffer, host, port, rowSize)
}

func asrCorrelationTCPTable(addressFamily uint32) ([]byte, error) {
	var size uint32
	result, _, _ := asrCorrelationGetExtendedTCPTable.Call(
		0, uintptr(unsafe.Pointer(&size)), 0,
		uintptr(addressFamily), uintptr(asrCorrelationTCPTableOwnerPIDListener), 0,
	)
	if result != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) && result != 0 || size < 4 {
		return nil, errors.New("Windows TCP listener table unavailable")
	}
	buffer := make([]byte, size)
	result, _, _ = asrCorrelationGetExtendedTCPTable.Call(
		uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)), 0,
		uintptr(addressFamily), uintptr(asrCorrelationTCPTableOwnerPIDListener), 0,
	)
	if result != 0 || size > uint32(len(buffer)) || size < 4 {
		return nil, errors.New("Windows TCP listener table unavailable")
	}
	return buffer[:size], nil
}

func asrCorrelationListenerPIDFromTCPTable(
	buffer []byte,
	host string,
	port int,
	rowSize int,
) (int, error) {
	if len(buffer) < 4 || rowSize <= 0 {
		return 0, ErrASRLiveCorrelationOwnership
	}
	entryCount := binary.LittleEndian.Uint32(buffer[:4])
	if uint64(entryCount) > uint64((len(buffer)-4)/rowSize) {
		return 0, ErrASRLiveCorrelationOwnership
	}
	wantedAddress := net.ParseIP(host)
	if host == "127.0.0.1" {
		wantedAddress = wantedAddress.To4()
	} else {
		wantedAddress = wantedAddress.To16()
	}
	if wantedAddress == nil {
		return 0, ErrASRLiveCorrelationOwnership
	}
	ownerPID := 0
	for index := uint32(0); index < entryCount; index++ {
		row := 4 + int(index)*rowSize
		addressOffset, portOffset, pidOffset := row+4, row+8, row+20
		addressSize := net.IPv4len
		if host == "::1" {
			addressOffset, portOffset, pidOffset = row, row+20, row+52
			addressSize = net.IPv6len
		}
		localAddress := net.IP(buffer[addressOffset : addressOffset+addressSize])
		localPort := int(binary.BigEndian.Uint16(buffer[portOffset : portOffset+2]))
		if !localAddress.Equal(wantedAddress) || localPort != port {
			continue
		}
		processID := int(binary.LittleEndian.Uint32(buffer[pidOffset : pidOffset+4]))
		if processID <= 0 || ownerPID != 0 && ownerPID != processID {
			return 0, ErrASRLiveCorrelationOwnership
		}
		ownerPID = processID
	}
	if ownerPID == 0 {
		return 0, ErrASRLiveCorrelationListenerAbsent
	}
	return ownerPID, nil
}
