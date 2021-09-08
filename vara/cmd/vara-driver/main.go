/**
 * Manual driver for VARA modem, mainly for testing the vara package.
 *
 * Program must be invoked with the -c flag to set myCall.
 * Setting VARA_DEBUG environment variable to anything will cause additional logging output.
 */

package main

import (
	"flag"
	"fmt"
	"net"
	"os"

	"github.com/benthor/gocli"
	"github.com/la5nta/wl2k-go/transport"
	"github.com/n8jja/Pat-Vara/vara"
)

var modem *vara.Modem
var conn net.Conn

func main() {
	var myCall = flag.String("c", "", "the callsign of my station")
	flag.Parse()
	if *myCall == "" {
		fmt.Printf("set mycall with -c")
		os.Exit(-1)
	}
	fmt.Printf("MyCall is %s\n", *myCall)

	start(myCall)

	cli := gocli.MkCLI("Type \"h\" for a list of options")
	_ = cli.AddOption("h", "prints this help message", cli.Help)
	_ = cli.AddOption("q", "", cli.Exit)
	_ = cli.AddOption("c", "connect to a remote station", connect)
	_ = cli.AddOption("d", "disconnect from the remote station", disconnect)
	cli.Loop("cmd? ")
}

func start(myCall *string) {
	fmt.Println("Initializing VARA modem...")
	var err error
	modem, err = vara.NewModem(*myCall, vara.ModemConfig{})
	if err != nil {
		fmt.Print(fmt.Errorf("%w", err))
		os.Exit(-1)
	}
	fmt.Println("Ready")
}

func connect(args []string) string {
	fmt.Println("connecting...")
	if len(args) < 1 {
		return "c ToCall [BW]"
	}
	toCall := args[0]
	params := map[string][]string{}
	if len(args) > 1 {
		params["bw"] = []string{args[1]}
	}
	url := &transport.URL{Scheme: "vara", Target: toCall, Params: params}
	var err error
	conn, err = modem.DialURL(url)
	if err != nil {
		return fmt.Sprintf("connect failed: %v", err)
	}
	return "connected"
}

func disconnect(args []string) string {
	if len(args) != 0 {
		return "disconnect requires zero arguments"
	}
	if conn == nil {
		return "not connected"
	}
	if err := conn.Close(); err != nil {
		return fmt.Sprintf("error disconnecting: %v", err)
	}
	return "disconnected"
}
