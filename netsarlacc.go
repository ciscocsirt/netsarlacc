package main

import (
        "flag"
        "fmt"
        "os"
	"os/signal"
	"syscall"
	"time"
        "net"
	"path/filepath"
	"errors"
	"crypto/tls"
	"sync"
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

type ListenInfo struct {
	Host        string
	Port        string
	Proto       string
	App         string
	TLS         bool
	Socket      net.Listener
}

type ConnInfo struct {
	Host        string
	Port        string
	Proto       string
	App         string
	TLS         bool
	Conn        net.Conn
}

var (
	ListenList = []ListenInfo{
		ListenInfo{Host: "0.0.0.0", Port: "8000", Proto: "tcp", App: "http", TLS: false},
		ListenInfo{Host: "0.0.0.0", Port: "8443", Proto: "tcp", App: "http", TLS: true},
	}
        sinkHost, _      = os.Hostname()
        NWorkers         = flag.Int("n", 4, "The number of workers to start")
        SinkholeInstance = flag.String("i", "netsarlacc-" + sinkHost, "The sinkhole instance name")
	Daemonize        = flag.Bool("D", false, "Daemonize the sinkhole")
        Logchan = make(chan string, 1024)
	Stopchan = make(chan os.Signal, 1)
	Workerstopchan = make(chan bool, 1)
	Logstopchan = make(chan bool, 1)
	Daemonized = false
	PidFile *os.File
)

func main() {

	// Setup the stop channel signal handler
	signal.Notify(Stopchan, os.Interrupt, syscall.SIGTERM)

        // Parse the command-line flags.
        flag.Parse()

	// Check if we should daemonize
	if *Daemonize == true {
		pid, err := DaemonizeProc()

		if err != nil {
			AppLogger(errors.New(fmt.Sprintf("Daemonization failed: %s", err.Error())))
			FatalAbort(false, -1)
		}

		if pid != nil {
			// This means we started a proc and we're not the daemon
			os.Exit(0)
		}
	}

        //starts the dispatcher
        StartDispatcher(*NWorkers)
        //starts the log channel
        go writeLogger(Logchan)

	// Load the TLS cert and key
	tlscer, err := tls.LoadX509KeyPair("server.pem", "server.key")

	if err != nil {
		AppLogger(errors.New(fmt.Sprintf("Unable to load TLS cert / key: %s", err.Error())))
		FatalAbort(false, -1)
	}

	// Build the TLS listener configuration
	tlsconfig := &tls.Config{
		Certificates: []tls.Certificate{tlscer},
		MinVersion: 0, // TLS 1.0
		MaxVersion: 0, // TLS 1.2 or greater (latest version)
	}

	// Iterate over the sockets we want to listen on
	stopacceptmutex := &sync.RWMutex{}
	stopaccept := false
	var Liwg sync.WaitGroup
	for i, _ := range ListenList {
		// Get a pointer to the listen info
		Li := &(ListenList[i])

		// Get the TCPAddr
		listenAddrstring := fmt.Sprintf("%s:%s", (*Li).Host, (*Li).Port)

		listenAddr, err := net.ResolveTCPAddr((*Li).Proto, listenAddrstring)
		if err != nil {
			AppLogger(errors.New(fmt.Sprintf("Unable to resolve listening address for %s: %s", listenAddrstring, err.Error())))
			FatalAbort(false, -1)
		}

		//listen for incoming connections
		(*Li).Socket, err = net.ListenTCP((*Li).Proto, listenAddr)
		if err != nil {
			AppLogger(errors.New(fmt.Sprintf("Error listening: %s", err.Error())))
			FatalAbort(false, -1)
		}

		AppLogger(errors.New(fmt.Sprintf("Listening on %s %s/%s", (*Li).Host, (*Li).Proto, (*Li).Port)))


		if (*Li).TLS == true {
			// Wrap listener with tls listener
			(*Li).Socket = tls.NewListener((*Li).Socket, tlsconfig)
		}

		// Track the fact that we're about to start a goroutine
		Liwg.Add(1)
		go func() {
			// Now when this routine exits track it
			defer Liwg.Done()

			// Loop until we get the stop signal
			for {
				// Listen for any incoming connections or possibly time out
				connection, err := (*Li).Socket.Accept()

				stopacceptmutex.RLock()
				if stopaccept == true {
					stopacceptmutex.RUnlock()

					// If we got a valid connection close it
					if err == nil {
						err = connection.Close()

						if err != nil {
							AppLogger(errors.New(fmt.Sprintf("Error connection on shutdown: %s", err.Error())))
						}
					}

					return;
				}
				stopacceptmutex.RUnlock()

				if err != nil {
					netErr, ok := err.(net.Error)
					// If this was a timeout just keep going
					if ((ok == true) && (netErr.Timeout() == true) && (netErr.Temporary() == true)) {
						continue;
					} else {
						AppLogger(errors.New(fmt.Sprintf("Error accepting: %s", err.Error())))
					}
				} else {
					Ci := *new(ConnInfo)
					Ci.Host = (*Li).Host
					Ci.Port = (*Li).Port
					Ci.Proto = (*Li).Proto
					Ci.App = (*Li).App
					Ci.TLS = (*Li).TLS
					Ci.Conn = connection
					Collector(Ci)
				}
			}
		}()
	} // End ListenList range loop

	// Block on select until we're ready to stop
	select {
	case <-Stopchan:
		// We must be stopping
		AppLogger(errors.New("Got signal to stop..."))

		// We don't want to be calling accept on a closed socket
		stopacceptmutex.Lock()
		stopaccept = true
		stopacceptmutex.Unlock()

		// This will unblock any call to Accept() on this socket
		AppLogger(errors.New("Shutting down listening sockets"))
		for i, _ := range ListenList {
			Li := &(ListenList[i])

			err := (*Li).Socket.Close()

			if err != nil {
				AppLogger(errors.New(fmt.Sprintf("Unable to close socket for %s %s/%s: %s",
					(*Li).Host, (*Li).Proto, (*Li).Port, err.Error())))
				FatalAbort(false, -1)
			}
		}
	}

	// Don't continue until all of our listeners really exited
	Liwg.Wait()

	// Shut the rest of everything down
	AttemptShutdown()
}


func AttemptShutdown() {

	// The goal here is to try to stop the workers
	// and close out the log file so that data isn't lost
	// and the log file is left consistent

	AppLogger(errors.New("Stopping workers"))
	StopWorkers()

	// As workers stop, they will tell us that.  We must wait for them
	// so that we can close the logging after the pending work is done
	for wstopped := 0; wstopped < *NWorkers; {
		select {
		case <-Workerstopchan:
			wstopped++
		case <-time.After(time.Second * 5):
			AppLogger(errors.New("Timed out waiting for all workers to stop!"))
			wstopped = *NWorkers
		}
	}

	// Close the Logchan which will allow the remaining
	// logs to get written to the logfile before the logging
	// goroutine finally closes the file and tells us it finished
	AppLogger(errors.New("Flushing logs and closing the log file"))
	close(Logchan)

	select {
	case <-Logstopchan:
		break
	case <-time.After(time.Second * 5):
		AppLogger(errors.New("Timed out waiting for log flushing and closing!"))
		break
	}

	if Daemonized == true {
		AppLogger(errors.New("Releasing lock on PID file"))
		err := syscall.Flock(int(PidFile.Fd()), syscall.LOCK_UN)

		if err != nil {
			AppLogger(errors.New(fmt.Sprintf("Unable to release lock on pid file: %s", err.Error())))
			FatalAbort(false, -1)
		}

		AppLogger(errors.New("Closing out PID file"))
		err = PidFile.Close()

		if err != nil {
			AppLogger(errors.New(fmt.Sprintf("Unable to close pid file: %s", err.Error())))
			FatalAbort(false, -1)
		}
	}

	AppLogger(errors.New("Shutdown complete"))
}


func FatalAbort(cleanup bool, ecode int) {

	// If we're supposed to cleanup we'll attempt that first
	if cleanup == true {
		AttemptShutdown()
	}

	AppLogger(errors.New(fmt.Sprintf("Aborting with error code %d", ecode)))
	os.Exit(ecode)
}


func Fullpath(exe string) (string, error) {

	// Bail out if the exe string is empty
	if len(exe) == 0 {
		return "", os.ErrInvalid
	}

	// Get an absolute path
	fullpath, err := filepath.Abs(exe);

	if err != nil {
		return "", err
	}

	// Optionally we could also call EvalSymlinks to get the real path
	// but I don't think that's needed here

	return fullpath, nil
}


func DaemonizeProc() (*int, error) {
	pidlockchan := make(chan error, 1)
	pipeerrchan := make(chan error, 1)

	_, daemonVarExists := os.LookupEnv("_NETSARLACC_DAEMON")

	if daemonVarExists == true {
		// We're the started daemon
		Daemonized = true

		// Unset the env var
		err := os.Unsetenv("_NETSARLACC_DAEMON")

		if err != nil {
			return nil, err
		}

		// Break away from the parent
		pid, err := syscall.Setsid()

		if err != nil {
			return nil, err
		}

		// We may want to indicate that we need to setuid/gid here but
		// we'll have to figure out how to Setuid after the socket are bound

		// We may want to ensure we're at / by seting our cwd there too

		// Report the PID that we got
		AppLogger(errors.New(fmt.Sprintf("Daemon got a PID of %d", pid)))

		// Now open our PID file, get a lock, and write our PID to it
		PidFile, err = os.OpenFile("netsarlacc.pid", os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644)

		if err != nil {
			err = errors.New(fmt.Sprintf("Unable to open pid file: %s", err.Error()))
			return nil, err
		}

		// Now try to get an exclusive lock the file descriptor
		go func() {
			err := syscall.Flock(int(PidFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)

			if err != nil {
				err = errors.New(fmt.Sprintf("Unable to acquire lock on pid file: %s", err.Error()))
			}

			pidlockchan <- err
		}()

		var lockerr error
		select {
		case lockerr =  <- pidlockchan:
			if lockerr != nil {
				return nil, lockerr
			}
			break
		case <-time.After(time.Second * 5):
			err = errors.New("Timed out waiting for pid lock acquisition!")
			return nil, err
		}

		// Write our PID to the file
		_, err = PidFile.Write([]byte(fmt.Sprintf("%d\n", pid)))

		if err != nil {
			err = errors.New(fmt.Sprintf("Unable to write pid into pid file: %s", err.Error()))
			return nil, err
		}

		// Make sure the PID actually gets to the file because we aren't
		// going to close the file until the daemon is about to exit
		// so that we can keep holding onto our lock of the file
		err = PidFile.Sync()

		if err != nil {
			err = errors.New(fmt.Sprintf("Unable to sync written pid into pid file: %s", err.Error()))
			return nil, err
		}

		// Send a message over the pipe saying we made it.  NewFile fd numbers start from 0 not 1
		pipef := os.NewFile(3, "pipe")
		_, err = pipef.Write([]byte("0"))

		if err != nil {
			err = errors.New(fmt.Sprintf("Unable to write to daemon pipe: %s", err.Error()))
			return nil, err
		}

		err = pipef.Close()

		if err != nil {
			err = errors.New(fmt.Sprintf("Unable to close daemon side write pipe: %s", err.Error()))
			return nil, err
		}

		return nil, nil
	} else {
		// We need to start the daemon
		AppLogger(errors.New(fmt.Sprintf("Daemonizing...")))

		var err error
		// First we'll try to open and acquire a lock on the pid file
		// to ensure there isn't a daemon already running
		PidFile, err = os.OpenFile("netsarlacc.pid", os.O_RDONLY|os.O_CREATE, 0644)

		if err != nil {
			err = errors.New(fmt.Sprintf("Unable to open pid file: %s", err.Error()))
			return nil, err
		}

		// Now try to get an exclusive lock the file descriptor
		go func() {
			err := syscall.Flock(int(PidFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)

			if err != nil {
				err = errors.New(fmt.Sprintf("Unable to acquire lock on pid file: %s", err.Error()))
			}

			pidlockchan <- err
		}()

		var lockerr error
		select {
		case lockerr =  <- pidlockchan:
			if lockerr != nil {
				return nil, lockerr
			}
			break
		case <-time.After(time.Second * 5):
			err = errors.New("Timed out waiting for pid lock acquisition!")
			return nil, err
		}

		// The lock worked so let's release it and then close the file
		err = syscall.Flock(int(PidFile.Fd()), syscall.LOCK_UN)

		if err != nil {
			err = errors.New(fmt.Sprintf("Unable to release lock on pid file: %s", err.Error()))
			return nil, err
		}

		err = PidFile.Close()

		if err != nil {
			err = errors.New(fmt.Sprintf("Unable to close pid file: %s", err.Error()))
			return nil, err
		}

		// Get our exename and full path
		exe, err := Fullpath(os.Args[0])

		if err != nil {
			err = errors.New(fmt.Sprintf("Unable to get full path of exe: %s", err.Error()))
			return nil, err
		}

		var attrs syscall.ProcAttr
		// Eventually we may want to set the new proc's dir to /
		// but this requires that we have support for a logging directory
		// instead of just using the cwd for logs
		// attrs.Dir = "/"

		// Set the new process's stdin, stdout, and stderr to /dev/null
		f_devnull, err := os.Open("/dev/null")

		if err != nil {
			err = errors.New(fmt.Sprintf("Unable to open /dev/null: %s", err.Error()))
			return nil, err
		}

		// Get a pipe between us and the daemon to make sure it starts properly
		piper, pipew, err := os.Pipe()

		if err != nil {
			err = errors.New(fmt.Sprintf("Unable to create pipe: %s", err.Error()))
			return nil, err
		}

		//attrs.Files = []*os.File{f_devnull, f_devnull, f_devnull, pipew}
		attrs.Files = []uintptr{f_devnull.Fd(), f_devnull.Fd(), f_devnull.Fd(), pipew.Fd()}

		// Tell the next process it's the deamon
		os.Setenv("_NETSARLACC_DAEMON", "true")

		// Copy our environment to the proc attributes
		attrs.Env = os.Environ()

		// Try to start up the deamon process
		pid, _, err := syscall.StartProcess(exe, os.Args, &attrs)

		if err != nil {
			err = errors.New(fmt.Sprintf("Unable to start daemon process: %s", err.Error()))
			return nil, err
		}

		// Listen on the pipe for the daemon to tell us that it made it
		go func () {
			pipebuf := make([]byte, 4096)
			rlen, err := piper.Read(pipebuf)

			if err != nil {
				err = errors.New(fmt.Sprintf("Unable to read from daemon pipe: %s", err.Error()))
			} else {
				rstring := string(pipebuf[:rlen])

				if rstring != "0" {
					err = errors.New(fmt.Sprintf("Did not read a status of 0 from the daemon pipe"))
				}
			}

			pipeerrchan <- err
		}()

		var pipeerr error
		select {
		case pipeerr =  <- pipeerrchan:
			if pipeerr != nil {
				return nil, pipeerr
			}
			break
		case <-time.After(time.Second * 5):
			err = errors.New("Timed out waiting to read daemon pipe!")
			return nil, err
		}

		err = piper.Close()

		if err != nil {
			err = errors.New(fmt.Sprintf("Unable to close launch side read pipe: %s", err.Error()))
			return nil, err
		}

		err = pipew.Close()

		if err != nil {
			err = errors.New(fmt.Sprintf("Unable to close launch side write pipe: %s", err.Error()))
			return nil, err
		}

		fmt.Fprintln(os.Stderr, fmt.Sprintf("Daemon started as PID %d", pid))

		return &pid, nil
	}
}
