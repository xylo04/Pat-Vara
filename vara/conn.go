package vara

import (
	"net"
)

// Wrapper for the data port connection we hand to clients. Implements net.Conn.
type varaDataConn struct {
	// the underlying TCP conn we're wrapping (type embedding)
	net.TCPConn
	// the parent modem hosting this connection
	modem Modem
}

// Close closes the connection.
// Any blocked Read or Write operations will be unblocked and return errors.
//
// Implements net.Conn.Close.
func (v *varaDataConn) Close() error {
	// If client wants to close the data stream, close down RF and TCP as well
	return v.modem.Close()
}

// LocalAddr returns the local network address.
//
// Implements net.Conn.LocalAddr.
func (v *varaDataConn) LocalAddr() net.Addr {
	return Addr{v.modem.myCall, v.modem.scheme}
}

// RemoteAddr returns the remote network address.
//
// Implements net.Conn.RemoteAddr.
func (v *varaDataConn) RemoteAddr() net.Addr {
	return Addr{v.modem.toCall, v.modem.scheme}
}
