package vara

import (
	"errors"
	"fmt"
	"net"

	"github.com/la5nta/wl2k-go/transport"
)

// Implementations for various wl2k-go/transport interfaces.

func (m *Modem) DialURL(url *transport.URL) (net.Conn, error) {
	if url.Scheme != m.scheme {
		return nil, transport.ErrUnsupportedScheme
	}

	err := m.start()
	if err != nil {
		return nil, err
	}

	// Set bandwidth from the URL
	if err := m.setBandwidth(url); err != nil {
		return nil, err
	}

	if m.scheme == "varahf" {
		// VaraHF only - Winlink or P2P?
		p2p := url.Params.Get("p2p") == "true"
		if p2p {
			if err := m.writeCmd(fmt.Sprintf("P2P SESSION")); err != nil {
				return nil, err
			}
		} else {
			if err := m.writeCmd(fmt.Sprintf("WINLINK SESSION")); err != nil {
				return nil, err
			}
		}
	}

	// Start connecting
	m.toCall = url.Target
	if err := m.writeCmd(fmt.Sprintf("CONNECT %s %s", m.myCall, m.toCall)); err != nil {
		return nil, err
	}

	// Block until connected
	if <-m.connectChange != connected {
		m.dataConn = nil
		return nil, errors.New("connection failed")
	}

	// Hand the VARA data TCP port to the client code
	return &varaDataConn{*m.dataConn, *m}, nil
}

func (m *Modem) setBandwidth(url *transport.URL) error {
	bw := url.Params.Get("bw")
	if bw == "" {
		return nil
	}
	if !contains(bandwidths, bw) {
		return errors.New(fmt.Sprintf("bandwidth %s not supported", bw))
	}
	return m.writeCmd(fmt.Sprintf("BW%s", bw))
}

func contains(c []string, s string) bool {
	for _, e := range c {
		if e == s {
			return true
		}
	}
	return false
}

// Busy returns true if the channel is not clear.
func (m *Modem) Busy() bool {
	return m.busy
}

// SetPTT injects the PTTController (probably hooked to a transceiver) that should be controlled by
// the modem.
//
// If nil, the PTT request from the TNC is ignored. VOX may still work.
func (m *Modem) SetPTT(ptt transport.PTTController) {
	m.rig = ptt
}
