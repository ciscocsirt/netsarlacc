package main

import (
	"fmt"
	// "net"
	// "bytes"
	"bufio"
	"encoding/hex"
	"regexp"
	"strings"
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

type Header struct {
	Method         string `json:"http_method"`
	User_Agent     string `json:"user_agent"`
	Content_Length string `json:"bytes_client"`
	Host           string `json:"url"`
	Referer        string `json:"http_referer"`
}

type Body struct {
	Body string
}

type ValidRequest struct {
	Timestamp string `json:"timestamp"`
	Header
	// Source IP
	// Dest IP
	// dest_port
	// src_port
	Raw_Data Body `json:"raw_data"`
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
				bufSize, err := work.Connection.Read(buf)
				if err != nil {
					fmt.Println("Error reading:", err.Error())
					Logger(err)
					// Hex encode all error data
					work.Connection.Write([]byte("Error I/O timeout. \n"))
					work.Connection.Close()
				}
				s := string(buf[:])
				requestLines := strings.Split(s, "\n")
				headRegex, _ := regexp.Compile("HTT(P|PS)\\/*.*")
				if !headRegex.MatchString(requestLines[0]) {
					//  Remove below line after testing
					work.Connection.Write([]byte("Non http \n"))
					work.Connection.Close()
				} else {
					// fmt.Printf("%s\n", buf)
					headerIndex := strings.LastIndex(s, "\r\n") - 2
					headerFields := string(buf[:headerIndex])
					method := strings.Fields(requestLines[0])[0]
					bodyField := Body{Body: hex.EncodeToString(buf[headerIndex:bufSize])}
					fmt.Println(bodyField)
					validResponse := make(map[string]string)
					fullResponse := make(map[string]string)
					// Regex to compare headers
					fieldsRegex, _ := regexp.Compile("^(User-Agent:)|(Host)|(Referer:)|(Accept-Language:)|(Accept-Encoding:)|(Connection:)")
					scanner := bufio.NewScanner(strings.NewReader(headerFields))
					for scanner.Scan() {
						for scanner.Scan() {
							if fieldsRegex.MatchString(scanner.Text()) {
								value := strings.Split(scanner.Text(), ":")
								validResponse[value[0]] = strings.Join(value[1:len(value)], " ")
							}
							value := strings.Split(scanner.Text(), ":")
							fullResponse[value[0]] = strings.Join(value[1:len(value)], " ")

						}
						if len(validResponse) != 6 {
							work.Connection.Write([]byte("Not a valid header \n"))
							work.Connection.Close()
						} else {
							header := Header{Method: method, User_Agent: validResponse["User-Agent"], Content_Length: fullResponse["Content-Length"], Host: validResponse["Host"], Referer: validResponse["Referer"]}
							fmt.Println(header, "\n")
							fmt.Println(fullResponse)
						}
						if err := scanner.Err(); err != nil {
							fmt.Println("Error reading headers:", err)
						}
						fmt.Println("")
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
