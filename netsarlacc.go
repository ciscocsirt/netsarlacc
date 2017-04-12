package main

import (
	"flag"
	"fmt"
	"os"
	// "log"
	"net"
	_ "net/http/pprof"
	// "reflect"
)

//TODO:
// -- Determine a payload struct for headers
// -- ALL CAPS letters for headers up to 20 bytes up to space
// -- URL encoding then space
// -- http 1.1 etc
// -- encode whole raw packet up to 4kb in one of the fileds in JSON whether it was valid or not
// -- Stress testing APACHE for golang
// -- Test cases
// -- Build Template for response
// -- Test speed and race conditions
// -- Test with real connections
// -- Format for writing to files

const (
	// Set these to flags after init testing
	CONN_HOST = "localhost"
	CONN_PORT = "8888"
	CONN_TYPE = "tcp"
)

var (
	sinkHost, _      = os.Hostname()
	NWorkers         = flag.Int("n", 100, "The number of workers to start")
	SinkholeInstance = flag.String("i", "netsarlacc-"+sinkHost, "The sinkhole instance name")
)

func main() {
	// Parse the command-line flags.
	flag.Parse()
	//create the Pool of work
	Pool := make(chan WorkRequest, *NWorkers)
	fmt.Println(Pool)
	//listen for incoming connections
	listen, err := net.Listen(CONN_TYPE, CONN_HOST+":"+CONN_PORT)
	if err != nil {
		fmt.Println("Error listening: ", err.Error())
		AppLogger(err)
	}

	//Close the listener when the app closes
	defer listen.Close()

	fmt.Println("Listening on "+CONN_TYPE, CONN_HOST+":"+CONN_PORT)
	//Loop will run forever or until the application closes
	for {
		//Listen for any incoming connections
		connection, err := listen.Accept()
		if err != nil {
			fmt.Println("Error accepting: ", err.Error())
			AppLogger(err)
		}
		go Collector(connection)
	}

}
