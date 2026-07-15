package builtcliacceptance

import (
	"fmt"
	"net"
)

// ReserveLocalTCPPort binds 127.0.0.1:0 and returns the allocated port.
func ReserveLocalTCPPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address type %T", listener.Addr())
	}
	return addr.Port, nil
}
