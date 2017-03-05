package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
	BytesClient    string `json:"bytes_client"`
	Method         string `json:"http_method"`
	Path           string `json:"url_path"`
	Version        string `json:"http_version"`
	User_Agent     string `json:"http_user_agent,omitempty"`
	Content_Length string `json:"http_content_length,omitempty"`
	Host           string `json:"dest_name,omitempty"`
	Referer        string `json:"http_referer,omitempty"`
}

type EncodedConn struct {
	Encode string `json:"raw_data"`
}

type LoggedRequest struct {
	Timestamp string `json:"timestamp"`
	Header
	SourceIP   string `json:"src_ip"`
	SourcePort string `json:"src_port"`
	Sinkhole   string `json:"sinkhole_instance"`
	EncodedConn
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
				buf := make([]byte, 4096)
				// Total request reads no more than 4kb set caps
				// Sets a read dead line. If it doesn't receive any information
				// Check to see if it'll accept trickled data
				// Whole transaction time no more than 500 mili
				//
				//work.Connection.SetReadBuffer()
				//60000
				work.Connection.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
				// If accpets trickled data, use timer below
				//timer := time.NewTimer(time.Millisecond * 500)
				sourceIP, sourcePort, _ := net.SplitHostPort(work.Connection.RemoteAddr().String())
				bufSize, err := work.Connection.Read(buf)
				rawData := EncodedConn{Encode: hex.EncodeToString(buf[:bufSize])}
				if err != nil {
					fmt.Println("Error reading:", err.Error())
					AppLogger(err)
					work.Connection.Close()
				} else {
					validConnLogging, err := parseConn(buf, bufSize, rawData, *SinkholeInstance, sourceIP, sourcePort)
					if err != nil {
						fmt.Println(err)
						jsonLog, _ := ToJSON(rawData)
						ConnLogger(jsonLog)
						work.Connection.Close()
					} else {
						jsonLog, _ := ToJSON(validConnLogging)
						ConnLogger(jsonLog)
						currentDir, err := os.Getwd()
						absPath, _ := filepath.Abs(currentDir + "/template/csirtResponse.tmpl")
						data, err := ioutil.ReadFile(absPath)
						if err != nil {
							fmt.Println("error is ", err)
						}
						funcMap := template.FuncMap{
							"Date": func(s string) string {
								tmp := strings.Fields(s)
								return fmt.Sprintf("%s", tmp[0])

							},
							"Time": func(s string) string {
								tmp := strings.Fields(s)
								return fmt.Sprintf("%s", tmp[1])

							},
						}
						var test bytes.Buffer
						tmpl, err := template.New("response").Funcs(funcMap).Parse(string(data[:]))
						if err != nil {
							fmt.Println("error is ", err)
						}
						err = tmpl.Execute(&test, validConnLogging)
						// server header, date header
						work.Connection.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: text/html;\r\nContent-Length: " + strconv.Itoa(len(test.Bytes())) + "\r\n\r\n"))
						work.Connection.Write((test.Bytes()))
						work.Connection.Close()
					}
				}

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

//If post and content lenght is greater than 0, remove non-ASCII and place into body field.
//
func parseConn(buf []byte, bufSize int, raw EncodedConn, SinkholeInstance, sourceIP, sourcePort string) (LoggedRequest, error) {

	// There are lots of methods but we really don't care which one is used
	req_re := regexp.MustCompile(`^([A-Z]{3,10})\s(\S+)\s(HTTP\/1\.[01])$`)
	// We'll allow any header name as long as it starts with a letter and any non-emtpy value
	// RFC2616 section 4.2 is very specific about how to treat whitespace
	// https://www.w3.org/Protocols/rfc2616/rfc2616-sec4.html#sec4.2
	header_re := regexp.MustCompile(`^([A-Za-z][A-Za-z0-9-]*):\s*([!-~\s]+?)\s*$`)

	// This lets us use ReadLine() to get one line at a time
	bufreader := bufio.NewReader(bytes.NewReader(buf[:bufSize]))

	// The struct describing the request
	var req_header Header

	// The map for storing all the headers
	allHeaders := make(map[string]string)

	// read first line of HTTP request
	bufline, lineprefix, err := bufreader.ReadLine()
	if err == nil {
		if lineprefix == false {
			// The first line came through intact
			// Apply validating regex
			matches := req_re.FindStringSubmatch(string(bufline))
			if matches != nil {
				req_header.Method = string(matches[1])
				req_header.Path = string(matches[2])
				req_header.Version = string(matches[3])
			} else {
				return LoggedRequest{EncodedConn: raw}, errors.New(`Request header failed regex validation`)
			}
		} else {
			return LoggedRequest{EncodedConn: raw}, errors.New(`First request line was truncated`)
		}
	} else {
		return LoggedRequest{EncodedConn: raw}, err
	}

	// Read any (optional) headers until first blank line indicating the end of the headers
	for {
		bufline, lineprefix, err := bufreader.ReadLine()
		if err != nil {
			return LoggedRequest{EncodedConn: raw}, err
		}
		if lineprefix == true {
			return LoggedRequest{EncodedConn: raw}, errors.New(`Found truncated header`)
		}
		bufstr := string(bufline)
		if bufstr == "" {
			// This is a blank line so it's last
			break
		}
		matches := header_re.FindStringSubmatch(bufstr)
		if matches != nil {
			// Canonical header name
			header_can := strings.ToLower(matches[1])
			if _, ok := allHeaders[header_can]; ok {
				return LoggedRequest{}, errors.New(`Got duplicate header from client`)
			}
			allHeaders[header_can] = matches[2]
		} else {
			return LoggedRequest{EncodedConn: raw}, errors.New(`Header failed regex validation`)
		}
	}

	// Set the non-optional parts of this header
	req_header.BytesClient = strconv.Itoa(bufSize)

	// Now  set some of the headers we got into the header struct
	if val, ok := allHeaders["user-agent"]; ok {
		req_header.User_Agent = val
	}
	if val, ok := allHeaders["referer"]; ok {
		req_header.Referer = val
	}
	if val, ok := allHeaders["content-length"]; ok {
		req_header.Content_Length = val
	}
	if val, ok := allHeaders["host"]; ok {
		req_header.Host = val
	}

	validConnLogging := LoggedRequest{Timestamp: time.Now().UTC().String(), Header: req_header, SourceIP: sourceIP, SourcePort: sourcePort, Sinkhole: SinkholeInstance, EncodedConn: raw}
	return validConnLogging, nil
}

// ToJSON converts a struct to a JSON string
func ToJSON(data interface{}) (string, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
