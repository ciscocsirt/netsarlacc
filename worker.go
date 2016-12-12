package main

import (
	"fmt"
	// "net"
	// "bytes"
	"strings"
	//"encoding/gob"
	"bufio"
	"regexp"
	"time"
)

// NewWorker creates, and returns a new Worker object. Its only argument
// is a channel that the worker can add itself to whenever it is done its
// work.
func NewWorker(id int, workerQueue chan chan WorkRequest) Worker {
	//Creating the worker
	worker := Worker{
		ID:          id,
		Work:        make(chan WorkRequest),
		WorkerQueue: workerQueue,
		QuitChan:    make(chan bool)}

	return worker
}

type Worker struct {
	ID          int
	Work        chan WorkRequest
	WorkerQueue chan chan WorkRequest
	QuitChan    chan bool
}

// This function "starts" the worker by starting a goroutine, that is
// an infinite "for-select" loop.
func (w *Worker) Start() {
	go func() {
		for {
			// Add ourselves into the worker queue.
			w.WorkerQueue <- w.Work
			select {
			case work := <-w.Work:
				// Receive a work request.
				// Makes buffer no bigger than 2048 bytes
				buf := make([]byte, 4000)
				// Total request reads no more than 4kb set caps
				// Sets a read dead line. If it doesn't receive any information
				// Check to see if it'll accept trickled data
				// Whole transaction time no more than 500 mili
				//
				//work.Connection.SetReadBuffer()
				work.Connection.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
				//timer := time.NewTimer(time.Millisecond * 500)
				// Go utility for parsing headers
				// Buffer to stream reader then loop over each line
				//
				_, err := work.Connection.Read(buf)
				if err != nil {
					fmt.Println("Error reading:", err.Error())
					Logger(err)
					// Hex encode all error data
					work.Connection.Write([]byte("Error I/O timeout. \n"))
					work.Connection.Close()
				}
				//Print the buffer
				//n := bytes.IndexByte(buf, 45)
				s := string(buf[:])
				requestField := strings.Split(s, "\n")
				headRegex, _ := regexp.Compile("HTT(P|PS)\\/*.*")
				if headRegex.MatchString(requestField[0]) != true {
					work.Connection.Close()
				} else {
					// fmt.Printf("%s\n", buf)
					headerIndex := strings.LastIndex(s, "\r\n")
					headerFields := string(buf[:headerIndex-2])
					bodyField := string(buf[headerIndex:])
					scanner := bufio.NewScanner(strings.NewReader(headerFields))
					for scanner.Scan() {
						for scanner.Scan() {
							//TODO add comparison with header fields
							//scanner.Text()
						}
						if err := scanner.Err(); err != nil {
							fmt.Println("Error reading headers:", err)
						}
					}
				}
				// Send a response back to person contacting us.
				work.Connection.Write([]byte("HTML response would go here. \n"))
				work.Connection.Close()

			case <-w.QuitChan:
				// We have been asked to stop.
				fmt.Printf("worker%d stopping\n", w.ID)
				return
			}
		}
	}()
}

// Stop tells the worker to stop listening for work requests.
// Note that the worker will only stop *after* it has finished its work.
func (w *Worker) Stop() {
	go func() {
		w.QuitChan <- true
	}()
}
