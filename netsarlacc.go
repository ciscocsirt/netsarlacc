package main

import (
	"flag"
	"fmt"
	"log"
	"net"
)

//TODO:
// -- Ensure HTTP/S
// -- TCP server that accpets only the above protocol
// -- Test with real connections
//

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
		log.Fatal(err)
		Logger(err)
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
			log.Fatal(err)
			Logger(err)
		}

		go Collector(connection)
	}

}
