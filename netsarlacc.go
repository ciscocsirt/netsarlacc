package main

import (
	"flag"
	"fmt"
	// "log"
	"net"
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
	CONN_HOST = "localhost"
	CONN_PORT = "3333"
	CONN_TYPE = "tcp"
)

func main() {
	var (
		NWorkers = flag.Int("n", 4, "The number of workers to start")
	)
	// Parse the command-line flags.
	flag.Parse()
	//starts the dispatcher
	StartDispatcher(*NWorkers)
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
