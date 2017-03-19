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
func NewWorker(workerQueue chan chan WorkRequest) Worker {
	//Creating the worker
	worker := Worker{
		Work:        make(chan WorkRequest),
		WorkerQueue: workerQueue,
		QuitChan:    make(chan bool)}

	return worker
}

type Worker struct {
	Work        chan WorkRequest
	WorkerQueue chan chan WorkRequest
	QuitChan    chan bool
}

type Header struct {
	BytesClient    string `json:"bytes_client"`
	Method         string `json:"http_method,omitempty"`
	Path           string `json:"url_path,omitempty"`
	Version        string `json:"http_version,omitempty"`
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
	ReqError bool   `json:"request_error"`
	ErrorMsg string `json:"request_error_message,omitempty"`
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

				// This is where we're going to store everything we log about this connection
				var req_log LoggedRequest

				//validConnLogging := LoggedRequest{Timestamp: time.Now().UTC().String(), Header: req_header, SourceIP: sourceIP, SourcePort: sourcePort, Sinkhole: SinkholeInstance, EncodedConn: raw}
				// return validConnLogging, nil
				req_log.Timestamp = time.Now().UTC().String()
				req_log.Sinkhole = *SinkholeInstance

				var err error
				req_log.SourceIP, req_log.SourcePort, err = net.SplitHostPort(work.Connection.RemoteAddr().String())
				if err != nil {
					fmt.Println("Error getting socket endpoint:", err.Error())
					AppLogger(err)
					work.Connection.Close()

					break
				}

				work.Connection.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
				bufSize, err := work.Connection.Read(buf)

				req_log.Header.BytesClient = strconv.Itoa(bufSize)

				req_log.EncodedConn = EncodedConn{Encode: hex.EncodeToString(buf[:bufSize])}

				if err != nil {
					fmt.Println("Error reading on socket:", err.Error())
					AppLogger(err)
					work.Connection.Close()

					req_log.ReqError = true
					req_log.ErrorMsg = err.Error()
					jsonLog, _ := ToJSON(req_log)
					ConnLogger(jsonLog)
				} else {
					err := parseConn(buf, bufSize, &req_log)
					if err != nil {
						fmt.Println(err)
						jsonLog, _ := ToJSON(req_log)
						ConnLogger(jsonLog)
						work.Connection.Close()
					} else {
						jsonLog, _ := ToJSON(req_log)
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
						err = tmpl.Execute(&test, req_log)
						work.Connection.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: text/html;\r\nContent-Length: " + strconv.Itoa(len(test.Bytes())) + "\r\n\r\n"))
						work.Connection.Write((test.Bytes()))
						work.Connection.Close()
					}
				}

			case <-w.QuitChan:
				// We have been asked to stop.
				fmt.Printf("worker stopped\n")
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

func parseConn(buf []byte, bufSize int, req_log *LoggedRequest) error {

	// There are lots of methods but we really don't care which one is used
	req_re := regexp.MustCompile(`^([A-Z]{3,10})\s(\S+)\s(HTTP\/1\.[01])$`)
	// We'll allow any header name as long as it starts with a letter and any non-emtpy value
	// RFC2616 section 4.2 is very specific about how to treat whitespace
	// https://www.w3.org/Protocols/rfc2616/rfc2616-sec4.html#sec4.2
	header_re := regexp.MustCompile(`^([A-Za-z][A-Za-z0-9-]*):\s*([!-~\s]+?)\s*$`)

	// This lets us use ReadLine() to get one line at a time
	bufreader := bufio.NewReader(bytes.NewReader(buf[:bufSize]))

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
				req_log.Header.Method = string(matches[1])
				req_log.Header.Path = string(matches[2])
				req_log.Header.Version = string(matches[3])
			} else {
				req_log.ReqError = true
				req_log.ErrorMsg = `Request header failed regex validation`
				return errors.New(req_log.ErrorMsg)
			}
		} else {
			req_log.ReqError = true
			req_log.ErrorMsg = `First request line was truncated`
			return errors.New(req_log.ErrorMsg)
		}
	} else {
		req_log.ReqError = true
		req_log.ErrorMsg = err.Error()
		return err
	}

	// Read any (optional) headers until first blank line indicating the end of the headers
	for {
		bufline, lineprefix, err := bufreader.ReadLine()
		if err != nil {
			req_log.ReqError = true
			req_log.ErrorMsg = err.Error()
			return err
		}
		if lineprefix == true {
			req_log.ReqError = true
			req_log.ErrorMsg = `Found truncated header`
			return errors.New(req_log.ErrorMsg)
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
				req_log.ReqError = true
				req_log.ErrorMsg = `Got duplicate header from client`
				return errors.New(req_log.ErrorMsg)
			}
			allHeaders[header_can] = matches[2]
		} else {
			req_log.ReqError = true
			req_log.ErrorMsg = `Header failed regex validation`
			return errors.New(req_log.ErrorMsg)
		}
	}

	// Now  set some of the headers we got into the header struct
	if val, ok := allHeaders["user-agent"]; ok {
		req_log.Header.User_Agent = val
	}
	if val, ok := allHeaders["referer"]; ok {
		req_log.Header.Referer = val
	}
	if val, ok := allHeaders["content-length"]; ok {
		req_log.Header.Content_Length = val
	}
	if val, ok := allHeaders["host"]; ok {
		req_log.Header.Host = val
	}

	req_log.ReqError = false
	return nil
}

// ToJSON converts a struct to a JSON string
func ToJSON(data interface{}) (string, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
