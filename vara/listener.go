package vara

import (
	"errors"
	"net"
)

// Implementation for the net.Listener interface.
// (Close method is implemented in connection.go.)

// Accept waits for and returns the next connection to the listener.
func (m *Modem) Accept() (net.Conn, error) {

	// Block until connected
	if <-m.connectChange != connected {
		m.dataConn = nil
		return nil, errors.New("connection failed")
	}

	// Hand the VARA data TCP port to the client code
	return &varaDataConn{*m.dataConn, *m}, nil
}

// Addr returns the listener's network address.
func (m *Modem) Addr() net.Addr {
	return Addr{m.myCall, m.scheme}
}

type Addr struct {
	string
	scheme string
}

func (a Addr) Network() string { return a.scheme }
func (a Addr) String() string  { return a.string }

func (m *Modem) Listen() (net.Listener, error) {
	if err := m.start(); err != nil {
		return nil, err
	}
	if err := m.writeCmd("LISTEN ON"); err != nil {
		return nil, err
	}
	return m, nil
}
