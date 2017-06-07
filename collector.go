package main

var WorkQueue = make(chan ConnInfo, 100)

// Reveive incoming work request (connections) and add them to the work queue
func Collector(Ci ConnInfo) {
	WorkQueue <- Ci
}
