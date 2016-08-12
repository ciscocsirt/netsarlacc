package main

import (
	"fmt"
	"net"
	"os"
)

const (
	CONN_HOST = "localhost"
	CONN_PORT = "3333"
	CONN_TYPE = "tcp"
)

func main() {
	//listen for incoming connections
	listen, err := net.Listen(CONN_TYPE, CONN_HOST+":"+CONN_PORT)
	if err != nil {
		fmt.Println("Error listening: ", err.Error())
		os.Exit(1)
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
			os.Exit(1)
		}
		//Handle connetions in a goroutine
		go handleServerConnection(connection)
	}

}

// Handles incoming requests.
func handleServerConnection(conn net.Conn) {
	// Make a buffer to hold incoming data.
	buf := make([]byte, 1024)
	// Read the incoming connection into the buffer.
	_, err := conn.Read(buf)
	if err != nil {
		fmt.Println("Error reading:", err.Error())
	}
	// Send a response back to person contacting us.
	conn.Write([]byte("Message received. \n"))
	// Close the connection when you're done with it.
	conn.Close()
}
