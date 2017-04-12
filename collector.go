package main

import (
	"net"
)

type WorkRequest struct {
	Connection net.Conn
}

var Pool chan WorkRequest

// Reveive incoming work request (connections) and add them to the work pool
func Collector(conn net.Conn) {
	work := WorkRequest{Connection: conn}
	// Pool <- work
	go Worker(work)
}
