package main

import (
        "flag"
        "fmt"
        "os"
	"os/signal"
	"syscall"
	"time"
        // "log"
        "net"
        // "reflect"
)

//TODO:
// -- Determine a payload struct for headers
// -- ALL CAPS letters for headers up to 20 bytes up to space
// -- URL encoding then space
// -- http 1.1 etc
// -- encode whole raw packet up to 4kb in one of the fileds in JSON whether it was valid or not
// -- Stress testing APACHE for golang
// -- Test cases
// -- Build Template for response
// -- Test speed and race conditions
// -- Test with real connections
// -- Format for writing to files

const (
        // Set these to flags
        CONN_HOST = "0.0.0.0"
        CONN_PORT = "8888"
        CONN_TYPE = "tcp"
)

var (
        sinkHost, _      = os.Hostname()
        NWorkers         = flag.Int("n", 4, "The number of workers to start")
        SinkholeInstance = flag.String("i", "netsarlacc-"+sinkHost, "The sinkhole instance name")
        Logchan = make(chan string, 1024)
	Stopchan = make(chan os.Signal, 1)
	Workerstopchan = make(chan bool, *NWorkers)
	Logstopchan = make(chan bool, 1)
)

func main() {

	// Setup the stop channel signal handler
	signal.Notify(Stopchan, os.Interrupt, syscall.SIGTERM)

        // Parse the command-line flags.
        flag.Parse()
        //starts the dispatcher
        StartDispatcher(*NWorkers)
        //starts the log channel
        go writeLogger(Logchan)

	// Get the TCPAddr
	listenAddr, err := net.ResolveTCPAddr(CONN_TYPE, CONN_HOST + ":" + CONN_PORT)
	if err != nil {
                fmt.Println("Unable to resolve listening port string: ", err.Error())
                AppLogger(err)
        }

        //listen for incoming connections
        listen, err := net.ListenTCP(CONN_TYPE, listenAddr)
        if err != nil {
                fmt.Println("Error listening: ", err.Error())
                AppLogger(err)
        }

	fmt.Println("Listening on "+CONN_TYPE, CONN_HOST+":"+CONN_PORT)
	// Loop until we get the stop signal
	running := true
	for (running == true) {
		// Set a listen timeout so we don't block indefinitely
		listen.SetDeadline(time.Now().Add(time.Second))

		// Listen for any incoming connections or possibly time out
		connection, err := listen.Accept()

		// Check to see if we're supposed to stop
		select {
		case <-Stopchan:
			running = false
			fmt.Fprintln(os.Stderr, "Got signal to stop...")
			// We'll let the connection we just accepted / timed out on get handled still
		default:
			// The Stopchan would have blocked because it's empty
		}

		if err != nil {

			netErr, ok := err.(net.Error)
			// If this was a timeout just keep going
			if ((ok == true) && (netErr.Timeout() == true) && (netErr.Temporary() == true)) {
				continue;
			} else {
				fmt.Println("Error accepting: ", err.Error())
				AppLogger(err)
			}
		} else {
			go Collector(connection)
		}
	} // End while running

	// We must be stopping, close the listening socket
	fmt.Fprintln(os.Stderr, "Shutting down listening socket")
	listen.Close()

	fmt.Fprintln(os.Stderr, "Stopping workers")
	StopWorkers()

	// As workers stop, they will tell us that.  We must wait for them
	// so that we can close the logging after the pending work is done
	for wstopped := 0; wstopped < *NWorkers; {
		select {
		case <-Workerstopchan:
			wstopped++
		case <-time.After(time.Second * 5):
			fmt.Fprintln(os.Stderr, "Timed out waiting for all workers to stop!")
			wstopped = *NWorkers // Maybe this should be a fatal error instead?
		}
	}

	// Close the Logchan which will allow the remaining
	// logs to get written to the logfile before the logging
	// goroutine finally closes the file and tells us it finished
	fmt.Fprintln(os.Stderr, "Flushing logs and closing the log file")
	close(Logchan)

	select {
	case <-Logstopchan:
		break
	case <-time.After(time.Second * 5):
		fmt.Fprintln(os.Stderr, "Timed out waiting for log flushing and closing!")
		break
	}

	fmt.Fprintln(os.Stderr, "Shutdown complete")
}
