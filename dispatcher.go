package main

var (
	WorkQueue   chan ConnInfo
	WorkerQueue chan chan ConnInfo
	WorkerSlice = make([]Worker, 0)

	ReadQueue   chan ConnInfo
	ReaderQueue chan chan ConnInfo
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


func StartWorkers(nworkers int) {
	// Init the channels
	WorkQueue = make(chan ConnInfo, *WorkChanLen)
	WorkerQueue = make(chan chan ConnInfo, nworkers)

	// Now, create all of our workers.
	for i := 0; i < nworkers; i++ {
		//fmt.Println("Starting worker", i+1)
		worker := NewWorker(WorkerQueue)
		worker.Work()

		// Store this worker so we can stop it later
		WorkerSlice = append(WorkerSlice, worker)
	}

	// This goroutine takes work from the main WorkQueue
	// and hands hands it to one of the worker's worqueues
	go func() {
		for {
			select {
			case work := <- WorkQueue:             // Get work off of the main workqueue
				select {
				case worker := <- WorkerQueue: // Get a worker's queue from the WorkeQueue
					worker <- work         // Add the work to the worker's queue
				}
			}
		}
	}()
 }


func StartReaders(nreaders int) {
	// Init the channels
	ReadQueue = make(chan ConnInfo, *ReadChanLen)
	ReaderQueue = make(chan chan ConnInfo, nreaders)

	// Now, create all of our readers
	for i := 0; i < nreaders; i++ {

		reader := NewWorker(ReaderQueue)
		reader.Read()

		// Store this worker so we can stop it later
		ReaderSlice = append(ReaderSlice, reader)
	}

	// This goroutine takes work from the main ReadQueue
	// and hands hands it to one of the reader's readqueues
	go func() {
		for {
			select {
			case read := <- ReadQueue:
				select {
				case reader := <- ReaderQueue:
					reader <- read
				}
			}
		}
	}()
}


func StopWorkers() {

	for _, w := range WorkerSlice {
		w.Stop()
	}
}


func StopReaders() {

	for _, w := range ReaderSlice {
		w.Stop()
	}
}
