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
	"regexp"
	"strings"
	"time"
)

var (
	HTTPtmpl *template.Template = nil // Allow caching of the HTTP template
)

// Compile the regular expressions once
var (
	// There are lots of HTTP methods but we really don't care which one is used
	req_re    = regexp.MustCompile(`^([A-Z]{3,10})\s(\S+)\s(HTTP\/1\.[01])$`)

	// We'll allow any header name as long as it starts with a letter and any non-emtpy value
	// RFC2616 section 4.2 is very specific about how to treat whitespace
	// https://www.w3.org/Protocols/rfc2616/rfc2616-sec4.html#sec4.2
	header_re = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9-]*):\s*([!-~\s]+?)\s*$`)
)


// NewWorker creates, and returns a new Worker object. Its only argument
// is a channel that the worker can add itself to whenever it is done its
// work
func NewWorker(workerQueue chan chan ConnInfo) Worker {
	//Creating the worker
	worker := Worker{
		WorkQueue:   make(chan ConnInfo),
		WorkerQueue: workerQueue,
		QuitChan:    make(chan bool)}

	return worker
}

// Allow a worker to remember its own details.
// This works just fine for both a normal worker and
// a ReadWorker
type Worker struct {
	WorkQueue   chan ConnInfo
	WorkerQueue chan chan ConnInfo
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
	SourceIP      string `json:"src_ip"`
	SourcePort    string `json:"src_port"`
	Sinkhole      string `json:"sinkhole_instance"`
	SinkholeAddr  string `json:"sinkhole_addr"`
	SinkholePort  string `json:"sinkhole_port"`
	SinkholeProto string `json:"sinkhole_proto"`
	SinkholeApp   string `json:"sinkhole_app"`
	SinkholeTLS   bool   `json:"sinkhole_tls"`
	EncodedConn
	ReqError      bool   `json:"request_error"`
	ErrorMsg      string `json:"request_error_message,omitempty"`
}

// This function "starts" the worker by starting a goroutine, that is
// an infinite "for-select" loop.
func (w *Worker) Work() {
	go func() {
		for {
			// Add ourselves into the worker queue.
			w.WorkerQueue <- w.WorkQueue
			select {
			case work := <-w.WorkQueue:
				// This is where we're going to store everything we log about this connection
				var req_log LoggedRequest

				// Fill out the basic info for a request based on
				// the data we have at this moment
				req_log.Timestamp = work.Time.Format("2006-01-02 15:04:05.000000 -0700 MST")
				req_log.Sinkhole = *SinkholeInstance
				req_log.SinkholeAddr = work.Host
				req_log.SinkholePort = work.Port
				req_log.SinkholeProto = work.Proto
				req_log.SinkholeApp = work.App
				req_log.SinkholeTLS = work.TLS

				var err error
				req_log.SourceIP, req_log.SourcePort, err = net.SplitHostPort(work.Conn.RemoteAddr().String())
				if err != nil {
					// Neither of these should ever happen
					// and if they do, they probably aren't a "client error"
					// so we'll log both
					AppLogger(errors.New(fmt.Sprintf("Error getting socket endpoint: %s", err.Error())))

					err = work.Conn.Close()
					if *LogClientErrors == true {
						AppLogger(err)
					}

					break
				}

				req_log.Header.BytesClient = fmt.Sprintf("%d", work.BufferSize)

				req_log.EncodedConn = EncodedConn{Encode: hex.EncodeToString(work.Buffer[:work.BufferSize])}

				if work.Err != nil {
					err = errors.New(fmt.Sprintf("Error reading on socket: %s", work.Err.Error()))
					if *LogClientErrors == true {
						AppLogger(err)
					}

					req_log.ReqError = true
					req_log.ErrorMsg = err.Error()

					jsonLog, err := ToJSON(req_log)
					if err != nil {
						AppLogger(err)

						err = work.Conn.Close()
						if *LogClientErrors == true {
							AppLogger(err)
						}
						break
					} else {
						queueLog(jsonLog)
					}

					err = work.Conn.Close()
					if *LogClientErrors == true {
						AppLogger(err)
					}
				} else {

					if work.App == "http" {
						err = parseConnHTTP(work.Buffer, work.BufferSize, &req_log)
					} else {
						err = errors.New("Only http can be parsed at this time.")
					}

					if err != nil {
						if *LogClientErrors == true {
							AppLogger(err)
						}

						jsonLog, err := ToJSON(req_log)
						if err != nil {
							AppLogger(err)

							err = work.Conn.Close()
							if *LogClientErrors == true {
								AppLogger(err)
							}
							break
						} else {
							queueLog(jsonLog)
						}

						err = work.Conn.Close()
						if *LogClientErrors == true {
							AppLogger(err)
						}
					} else {
						jsonLog, err := ToJSON(req_log)
						if err != nil {
							AppLogger(err)

							err = work.Conn.Close()
							if *LogClientErrors == true {
								AppLogger(err)
							}
							break
						} else {
							queueLog(jsonLog)
						}

						// Build the reponse using the template
						tmplBytes, err := fillTemplateHTTP(&req_log)
						if err != nil {
							AppLogger(errors.New(fmt.Sprintf("Unable to fill template: %s", err.Error())))

							err = work.Conn.Close()
							if *LogClientErrors == true {
								AppLogger(err)
							}
							break
						}

						// Mash the entire HTTP header and the template bytes into one write.
						// If this wasn't done in one write we'd need to turn off Nagle's algorithm
						// with "func (c *TCPConn) SetNoDelay(noDelay bool) error" so that the OS
						// doesn't do two seperate sends which is inefficient and may confuse
						// some clients that expect to get at least some of the body
						// at the same time they get the header
						_, err = work.Conn.Write(append([]byte(fmt.Sprintf("%s 200 OK\r\nServer: %s\r\nContent-Type: text/html;\r\nConnection: close\r\nContent-Length: %d\r\nCache-Control: no-cache, no-store, must-revalidate\r\nPragma: no-cache\r\nExpires: 0\r\n\r\n", req_log.Header.Version, PROGNAME, len(tmplBytes))), tmplBytes...))

						if err != nil {
							if *LogClientErrors == true {
								AppLogger(errors.New(fmt.Sprintf("Unable to write to socket: %s", err.Error())))
							}

							err = work.Conn.Close()
							if *LogClientErrors == true {
								AppLogger(err)
							}
							break
						}

						err = work.Conn.Close()
						if *LogClientErrors == true {
							AppLogger(err)
						}
					}
				}

			case <-w.QuitChan:
				// We have been asked to stop.
				Workerstopchan <- true
				// fmt.Fprintln(os.Stderr, "worker stopped")
				return
			}
		}
	}()
}


func (w *Worker) Read() {
	go func() {
		for {
			// Add ourselves into the reader queue.
			w.WorkerQueue <- w.WorkQueue
			select {
			case read := <-w.WorkQueue:

				// Make enough space to recieve client bytes
				read.Buffer = make([]byte, 4096)

				read.Conn.SetReadDeadline(time.Now().Add(time.Millisecond * time.Duration(*ClientReadTimeout)))
				var err error
				read.BufferSize, err = read.Conn.Read(read.Buffer)

				if err != nil {
					err = errors.New(fmt.Sprintf("Unable to read from socket: %s", err.Error()))
				}
				read.Err = err

				// Now that we've tried to read, queue the rest of the work
				QueueWork(read)

			case <-w.QuitChan:
				// We have been asked to stop.
				Readerstopchan <- true
				// fmt.Fprintln(os.Stderr, "worker stopped")
				return
			}
		}
	}()
}


// Stop tells the worker to stop listening for work requests.
// Note that the worker will only stop *after* it has finished its work
func (w *Worker) Stop() {
	w.QuitChan <- true
	// fmt.Fprintln(os.Stderr, "Worker got stop call")
}


func parseConnHTTP(buf []byte, bufSize int, req_log *LoggedRequest) error {

	if bufSize == 0 {
		req_log.ReqError = true
		req_log.ErrorMsg = "Got empty request"
		return errors.New(req_log.ErrorMsg)
	}

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
				req_log.ErrorMsg = "Request header failed regex validation"
				return errors.New(req_log.ErrorMsg)
			}
		} else {
			req_log.ReqError = true
			req_log.ErrorMsg = "First request line was truncated"
			return errors.New(req_log.ErrorMsg)
		}
	} else {
		req_log.ReqError = true
		req_log.ErrorMsg = fmt.Sprintf("Failed to read first line: %s", err.Error())
		return errors.New(req_log.ErrorMsg)
	}

	// Read any (optional) headers until first blank line indicating the end of the headers
	for {
		bufline, lineprefix, err := bufreader.ReadLine()
		if err != nil {
			req_log.ReqError = true
			req_log.ErrorMsg = fmt.Sprintf("Failed to read line: %s", err.Error())
			return errors.New(req_log.ErrorMsg)
		}
		if lineprefix == true {
			req_log.ReqError = true
			req_log.ErrorMsg = "Found truncated header"
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
				req_log.ErrorMsg = "Got duplicate header from client"
				return errors.New(req_log.ErrorMsg)
			}
			allHeaders[header_can] = matches[2]
		} else {
			req_log.ReqError = true
			req_log.ErrorMsg = "Header failed regex validation"
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


func fillTemplateHTTP(req_log *LoggedRequest) ([]byte, error) {

	// If we haven't cached the template yet, do so
	if HTTPtmpl == nil {
		templateData, err := ioutil.ReadFile(pathHTTPTemp)
		if err != nil {
			return nil, errors.New(fmt.Sprintf("Could not read template file: %s", err.Error()))
		}

		funcMap := template.FuncMap{
			"Date": func(s string) string {
				flds := strings.Fields(s)
				return fmt.Sprintf("%s", flds[0])

			},
			"Time": func(s string) string {
				flds := strings.Fields(s)
				return fmt.Sprintf("%s", flds[1])

			},
		}

		// This will cache the template into the HTTPtmpl var
		HTTPtmpl, err = template.New("HTTPresponse").Funcs(funcMap).Parse(string(templateData[:]))
		if err != nil {
			return nil, errors.New(fmt.Sprintf("Could not instantiate new template: %s", err.Error()))
		}
	}

	var tmplFilled bytes.Buffer
	err := HTTPtmpl.Execute(&tmplFilled, req_log)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Could not execute template fill: %s", err.Error()))
	}

	return tmplFilled.Bytes(), nil
}

// ToJSON converts a struct to a JSON string
func ToJSON(data interface{}) ([]byte, error) {
	jsonBytes, err := json.Marshal(data)

	if err != nil {
		return nil, errors.New(fmt.Sprintf("JSON conversion failed: %s", err.Error()))
	}

	return append(jsonBytes, "\n"...), nil
}
