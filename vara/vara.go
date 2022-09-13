package vara

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/imdario/mergo"
	"github.com/la5nta/wl2k-go/transport"
)

// ModemConfig defines configuration options for connecting with the VARA modem program.
type ModemConfig struct {
	// Host on the network which is hosting VARA; defaults to `localhost`
	Host string
	// CmdPort is the TCP port on which to reach VARA; defaults to 8300
	CmdPort int
	// DataPort is the TCP port on which to exchange over-the-air payloads with VARA;
	// defaults to 8301
	DataPort int
}

var defaultConfig = ModemConfig{
	Host:     "localhost",
	CmdPort:  8300,
	DataPort: 8301,
}

type Modem struct {
	scheme        string
	myCall        string
	config        ModemConfig
	cmdConn       *net.TCPConn
	dataConn      *net.TCPConn
	toCall        string
	busy          bool
	connectChange chan connectedState
	lastState     connectedState
	rig           transport.PTTController
}

type connectedState int

const (
	connected connectedState = iota
	disconnected
)

var bandwidths = []string{"500", "2300", "2750"}
var debug bool

func init() {
	debug = os.Getenv("VARA_DEBUG") != ""
}

func Bandwidths() []string {
	return bandwidths
}

// NewModem initializes configuration for a new VARA modem client stub.
func NewModem(scheme string, myCall string, config ModemConfig) (*Modem, error) {
	// Back-fill empty config values with defaults
	if err := mergo.Merge(&config, defaultConfig); err != nil {
		return nil, err
	}
	return &Modem{
		scheme:        scheme,
		myCall:        myCall,
		config:        config,
		busy:          false,
		connectChange: make(chan connectedState, 1),
		lastState:     disconnected,
	}, nil
}

// Start establishes TCP connections with the VARA modem program and initializes configuration. This
// must be called before sending commands to the modem.
func (m *Modem) start() error {
	var err error

	// Open the VARA command TCP port if it isn't
	if m.cmdConn == nil {
		m.cmdConn, err = m.connectTCP("command", m.config.CmdPort)
		if err != nil {
			return err
		}
	}

	// Start listening for incoming VARA commands
	go m.cmdListen()

	// Open the VARA data TCP port if it isn't
	if m.dataConn == nil {
		var err error
		if m.dataConn, err = m.connectTCP("data", m.config.DataPort); err != nil {
			return err
		}
	}

	// channel is not busy until Vara tells otherwise
	m.busy = false

	// Select public
	if err := m.writeCmd(fmt.Sprintf("PUBLIC ON")); err != nil {
		return err
	}

	// CWID enable
	if m.scheme == "varahf" {
		if err := m.writeCmd(fmt.Sprintf("CWID ON")); err != nil {
			return err
		}
	}

	// Set compression
	if err := m.writeCmd(fmt.Sprintf("COMPRESSION TEXT")); err != nil {
		return err
	}

	// Set MYCALL
	if err := m.writeCmd(fmt.Sprintf("MYCALL %s", m.myCall)); err != nil {
		return err
	}
	return nil
}

// Close closes the RF and then the TCP connections to the VARA modem. Blocks until finished.
func (m *Modem) Close() error {
	// Block until VARA modem acks disconnect
	if m.lastState == connected {
		// Send DISCONNECT command
		if m.cmdConn != nil {
			if err := m.writeCmd("DISCONNECT"); err != nil {
				return err
			}
		}

		select {
		case res := <-m.connectChange:
			if res != disconnected {
				log.Println("Disconnect failed, aborting!")
				if err := m.writeCmd("ABORT"); err != nil {
					return err
				}
			}
		case <-time.After(time.Second * 10):
			if err := m.writeCmd("ABORT"); err != nil {
				return err
			}
		}
	}

	// Make sure to stop TX (should have already happened, but this is a backup)
	m.sendPTT(false)

	// Clear up internal state
	m.handleDisconnect()
	return nil
}

func (m *Modem) connectTCP(name string, port int) (*net.TCPConn, error) {
	debugPrint(fmt.Sprintf("Connecting %s TCP port", name))
	cmdAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", m.config.Host, port))
	if err != nil {
		return nil, fmt.Errorf("couldn't resolve VARA %s address: %w", name, err)
	}
	conn, err := net.DialTCP("tcp", nil, cmdAddr)
	if err != nil {
		return nil, fmt.Errorf("couldn't connect to VARA %s port: %w", name, err)
	}
	return conn, nil
}

func disconnectTCP(name string, port *net.TCPConn) *net.TCPConn {
	if port == nil {
		return nil
	}
	_ = port.Close()
	debugPrint(fmt.Sprintf("disconnected %s TCP port", name))
	return nil
}

// wrapper around m.cmdConn.Write
func (m *Modem) writeCmd(cmd string) error {
	debugPrint(fmt.Sprintf("writing cmd: %v", cmd))
	_, err := m.cmdConn.Write([]byte(cmd + "\r"))
	return err
}

// goroutine listening for incoming commands
func (m *Modem) cmdListen() {
	var buf = make([]byte, 1<<16)
	for {
		if m.cmdConn == nil {
			// probably disconnected
			return
		}
		l, err := m.cmdConn.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				// VARA program killed?
				return
			}
			if errors.Is(err, net.ErrClosed) {
				// Connection closed
				return
			}
			debugPrint(fmt.Sprintf("cmdListen err: %v", err))
			continue
		}
		cmds := strings.Split(string(buf[:l]), "\r")
		for _, c := range cmds {
			if c == "" {
				continue
			}
			if !m.handleCmd(c) {
				return
			}
		}
	}
}

// handleCmd handles one command coming from the VARA modem. It returns true if listening should
// continue or false if listening should stop.
func (m *Modem) handleCmd(c string) bool {
	debugPrint(fmt.Sprintf("got cmd: %v", c))
	switch c {
	case "PTT ON":
		// VARA wants to start TX; send that to the PTTController
		m.sendPTT(true)
	case "PTT OFF":
		// VARA wants to stop TX; send that to the PTTController
		m.sendPTT(false)
	case "BUSY ON":
		m.busy = true
	case "BUSY OFF":
		m.busy = false
	case "OK":
		// nothing to do
	case "IAMALIVE":
		// nothing to do
	case "PENDING":
		// nothing to do
	case "DISCONNECTED":
		m.handleDisconnect()
		return false
	default:
		if strings.HasPrefix(c, "CONNECTED") {
			m.handleConnect()
			break
		}
		if strings.HasPrefix(c, "BUFFER") {
			// nothing to do
			break
		}
		if strings.HasPrefix(c, "REGISTERED") {
			parts := strings.Split(c, " ")
			if len(parts) > 1 {
				log.Printf("VARA full speed available, registered to %s", parts[1])
			}
			break
		}
		log.Printf("got a vara command I wasn't expecting: %v", c)
	}
	return true
}

func (m *Modem) sendPTT(on bool) {
	if m.rig != nil {
		_ = m.rig.SetPTT(on)
	}
}

func (m *Modem) handleConnect() {
	m.lastState = connected
	m.connectChange <- connected
}

func (m *Modem) handleDisconnect() {
	m.lastState = disconnected
	m.connectChange <- disconnected

	// Close data port TCP connection
	m.dataConn = disconnectTCP("data", m.dataConn)
	// Close command port TCP connection
	m.cmdConn = disconnectTCP("command", m.cmdConn)

	m.toCall = ""
	m.busy = false
}

func (m *Modem) Ping() bool {
	// TODO
	return true
}

func (m *Modem) Version() (string, error) {
	// TODO
	return "v1", nil
}

// If env var VARA_DEBUG exists, log more stuff
func debugPrint(msg string) {
	if debug {
		log.Printf("[VARA] %s", msg)
	}
}
