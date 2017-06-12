package main

import (
	"time"
	"errors"
)

var (
	WorkQueue   chan ConnInfo
	Workerstopchan = make(chan bool, 1)
	WorkerSlice = make([]Worker, 0)

	ReadQueue   chan ConnInfo
	Readerstopchan = make(chan bool, 1)
	ReaderSlice = make([]Worker, 0)
)


// This takes an incomming connection and queues it to be read
func QueueRead(Ci ConnInfo) {
	ReadQueue <- Ci
}


// This takes connections that have already been read and
// queues them to be processed
func QueueWork(Ci ConnInfo) {
	WorkQueue <- Ci
}


func StartWorkers(nworkers int, chanlen int) {
	// Init the channels
	WorkQueue = make(chan ConnInfo, chanlen)

	// Now, create all of our workers.
	for i := 0; i < nworkers; i++ {
		//fmt.Println("Starting worker", i+1)
		worker := NewWorker(WorkQueue, Workerstopchan)
		worker.Work()

		// Store this worker so we can stop it later
		WorkerSlice = append(WorkerSlice, worker)
	}
 }


func StartReaders(nreaders int, chanlen int) {
	// Init the channels
	ReadQueue = make(chan ConnInfo, chanlen)

	// Now, create all of our readers
	for i := 0; i < nreaders; i++ {

		reader := NewWorker(ReadQueue, Readerstopchan)
		reader.Read()

		// Store this worker so we can stop it later
		ReaderSlice = append(ReaderSlice, reader)
	}
}


func StopWorkers(nwriters int) error {

	for _, w := range WorkerSlice {
		w.Stop()
	}

	// As workers stop, they will tell us that.  We must wait for them
	// so that we can close the logging after the pending work is done
	for wstopped := 0; wstopped < nwriters; {
		select {
		case <-Workerstopchan:
			wstopped++
		case <-time.After(time.Second * 5):
			return errors.New("Timed out waiting for all workers to stop!")
		}
	}

	return nil
}


func StopReaders(nreaders int) error {

	for _, w := range ReaderSlice {
		w.Stop()
	}

	// As readers stop, they will tell us that.  We must wait for them
	// so that we can close the logging after the pending work is done
	for rstopped := 0; rstopped < nreaders; {
		select {
		case <-Readerstopchan:
			rstopped++
		case <-time.After(time.Second * 5):
			return errors.New("Timed out waiting for all readers to stop!")
		}
	}

	return nil
}
