package main

import "fmt"

var (
	WorkerQueue chan chan WorkRequest
	WorkerSlice = make([]Worker, 0)
)


func StartDispatcher(nworkers int) {
	// First, initialize the channel we are going to put the workers' work channels into.
	WorkerQueue = make(chan chan WorkRequest, nworkers)

	// Now, create all of our workers.
	for i := 0; i < nworkers; i++ {
		fmt.Println("Starting worker", i+1)
		worker := NewWorker(WorkerQueue)
		worker.Start()

		// Store this worker so we can stop it later
		WorkerSlice = append(WorkerSlice, worker)
	}

	go func() {
		for {
			select {
			case work := <-WorkQueue:
				go func() {
					worker := <-WorkerQueue
					worker <- work
				}()
			}
		}
	}()
}


func StopWorkers() {

	for _, w := range WorkerSlice {
		w.Stop()
	}
}
