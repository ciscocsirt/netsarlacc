package main

import (
	"net"
)

type WorkRequest struct {
	Connection net.Conn
}

var WorkQueue = make(chan WorkRequest, 100)

// Reveive incoming work request (connections) and add them to the work queue
func Collector(conn net.Conn) {
	// conn.SetDeadline(time.Now().Add(timeoutDuration))
	work := WorkRequest{Connection: conn}
	WorkQueue <- work
}
