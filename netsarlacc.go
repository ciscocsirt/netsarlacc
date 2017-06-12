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
	"encoding/json"
	"runtime"
	// "runtime/pprof" // for profiling code
)

const (
	PROGNAME = "netsarlacc"
)

type ListenInfo struct {
	Host        string
	Port        string
	Proto       string
	App         string
	TLS         bool
	TLSCert     string
	TLSKey      string
	Socket      net.Listener
}

type ConnInfo struct {
	Host        string
	Port        string
	Proto       string
	App         string
	TLS         bool
	Conn        net.Conn
	Buffer      []byte
	BufferSize  int
	Time        time.Time
	Err         error
}

var (
	ListenList = []ListenInfo{
		ListenInfo{Host: "0.0.0.0", Port: "8000", Proto: "tcp", App: "http", TLS: false},
		ListenInfo{Host: "0.0.0.0", Port: "8443", Proto: "tcp", App: "http", TLS: true},
	}
        NWorkers           = flag.Int("workers", 4, "The number of workers to start")
	NReaders           = flag.Int("readers", 512, "The number of readers to start")
        SinkholeInstance   = flag.String("name", getInstanceName(), "The sinkhole instance name")
	Daemonize          = flag.Bool("daemonize", false, "Daemonize the sinkhole")
	DaemonEnvVar       = flag.String("daemon-env-var", "_NETSARLACC_DAEMON", "Environment variable to use for daemonization")
	LogClientErrors    = flag.Bool("log-client-errors", false, "Report client-based errors to syslog / stderr")
	LogBaseName        = flag.String("log-prefix", "sinkhole", "Log files will start with this name")
	LogChanLen         = flag.Int("log-buffer-len", 4096, "Maximum number of buffered log entries")
	ReadChanLen        = flag.Int("reader-queue-len", 32, "Maximum number of queued connections to read from")
	WorkChanLen        = flag.Int("worker-queue-len", 32, "Maximum number of queued read connections to work on")
	ClientReadTimeout  = flag.Int("client-read-timeout", 500, "Number of milliseconds before giving up trying to read client request")
	ClientWriteTimeout = flag.Int("client-write-timeout", 500, "Number of milliseconds before giving up trying to write client response")
	UseLocaltime       = flag.Bool("use-localtime", false, "Use the local time (and timezone) instead of UTC")
	Stopchan = make(chan os.Signal, 1)
	Daemonized = false
	PidFile *os.File
)

// Path variables
var (
	// Flags are pointers to a string
	FlpathWorkingDir = flag.String("working-dir", ".", "The base directory for searching relative paths")
	FlpathConfig     = flag.String("c", "", "Configuration file")
	FlpathTLSCert    = flag.String("tls-cert", "server.pem", "Path to the TLS certificate")
	FlpathTLSKey     = flag.String("tls-key", "server.key", "Path to the TLS certificate key")
	FlpathLogDir     = flag.String("log-dir", "/var/log", "Path to the directory to store logs")
	FlpathHTTPTemp   = flag.String("http-template", "template/HTTPResponse.tmpl", "Path to the HTTP response template")
	FlpathPIDFile    = flag.String("pid-file", "netsarlacc.pid", "Path to the daemonization pid file")
	// These get filled out by resolving paths from flags
	pathConfigFile = "" // Allowed to be blank
	pathWorkingDir string
	pathTLSCert    string
	pathTLSKey     string
	pathLogDir     string
	pathHTTPTemp   string
	pathPIDFile    string
)

// For profiling this var will need to be uncommented
// var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")


// These are the options you can specify in the JSON configuration file
// The options in the configuration file override options used on the
// command line
type Config struct {
	Daemonize          bool
	DaemonEnvVar       string
	UseLocaltime       bool
	LogClientErrors    bool
	ClientReadTimeout  int
	ClientWriteTimeout int
	Workers            int
	Readers            int
	LogBufferLen       int
	ReaderQueueLen     int
	WorkerQueueLen     int
	LogPrefix          string
	WorkingDirectory   string
	LogDirectory       string
	HTTPTemplate       string
	PIDFile            string
	TLSCert            string
	TLSKey             string
	ListenList         []ListenInfo
}


func main() {

	// Setup the stop channel signal handler
	signal.Notify(Stopchan, os.Interrupt, syscall.SIGTERM)

        // Parse the command-line flags.
        flag.Parse()

	// Profiling can be started once the cmdline
	// flags are done parsing
	//if *cpuprofile != "" {
	//	f, err := os.Create(*cpuprofile)
	//	if err != nil {
	//		AppLogger(err)
	//	}
	//	pprof.StartCPUProfile(f)
	//	defer pprof.StopCPUProfile()
	//}

	// Fill out paths
	err := ResolvePaths()
	if err != nil {
		AppLogger(err)
		FatalAbort(false, -1)
	}

	// Load the configuration file (if it isn't blank)
	err = LoadConfig(pathConfigFile)
	if err != nil {
		AppLogger(err)
		FatalAbort(false, -1)
	}

	// Make sure the workers parameter isn't stupid
	if *NWorkers < 1 {
		AppLogger(errors.New("The number of workers must be at least 1"))
		FatalAbort(false, -1)
	}

	// Make sure the workers parameter isn't stupid
	if *NReaders < 1 {
		AppLogger(errors.New("The number of readers must be at least 1"))
		FatalAbort(false, -1)
	}

	// Make sure the client read timeout parameter isn't stupid
	if *ClientReadTimeout < 1 {
		AppLogger(errors.New("The client read timeout must be at least 1 millisecond"))
		FatalAbort(false, -1)
	}

	// Make sure the client write timeout parameter isn't stupid
	if *ClientWriteTimeout < 1 {
		AppLogger(errors.New("The client write timeout must be at least 1 millisecond"))
		FatalAbort(false, -1)
	}

	// Announce that we're starting
	AppLogger(errors.New(fmt.Sprintf("Starting sinkhole instance %s", *SinkholeInstance)))

	// This seems to be needed for GCCGO because for some
	// reason it uses 1 maximum thread at start (at least on my system)
	AppLogger(errors.New(fmt.Sprintf("Started with %d max threads", runtime.GOMAXPROCS(0))))
	runtime.GOMAXPROCS(runtime.NumCPU())
	AppLogger(errors.New(fmt.Sprintf("Now running with with %d max threads", runtime.GOMAXPROCS(0))))

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

        // Start the dispatcher that feeds workers
	// work as well as starting all the workers
	AppLogger(errors.New(fmt.Sprintf("Starting %d readers", *NReaders)))
        StartReaders(*NReaders, *ReadChanLen)
	AppLogger(errors.New(fmt.Sprintf("Starting %d workers", *NWorkers)))
	StartWorkers(*NWorkers, *WorkChanLen)

        // Start the log channel
        go writeLogger(*LogChanLen)

	// Iterate over the sockets we want to listen on
	stopacceptmutex := &sync.RWMutex{}
	stopaccept := false
	var Liwg sync.WaitGroup // Create the waitgroup
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
			// Make sure listener-specific cert, key are both set or unset
			if (((*Li).TLSCert == "") != ((*Li).TLSKey == "")) {
				AppLogger(errors.New("Can not specify one of {TLSCert, TLSKey} but not the other for listener!"))
				FatalAbort(false, -1)
			}

			var tlscer tls.Certificate
			// If listener-specific cert, keys aren't specified load the global ones
			if (*Li).TLSCert == "" {
				// Default key paths
				tlscer, err = tls.LoadX509KeyPair(pathTLSCert, pathTLSKey)
				if err != nil {
					AppLogger(errors.New(fmt.Sprintf("Unable to load global TLS cert / key: %s", err.Error())))
					FatalAbort(false, -1)
				}
			} else {
				// Listener specific key paths
				certfullpath, err := Fullpath((*Li).TLSCert)
				if err != nil {
					AppLogger(errors.New(fmt.Sprintf("Unable to load listener-specific TLS cert: %s", err.Error())))
					FatalAbort(false, -1)
				}

				keyfullpath, err := Fullpath((*Li).TLSKey)
				if err != nil {
					AppLogger(errors.New(fmt.Sprintf("Unable to load listener-specific TLS key: %s", err.Error())))
					FatalAbort(false, -1)
				}

				tlscer, err = tls.LoadX509KeyPair(certfullpath, keyfullpath)
				if err != nil {
					AppLogger(errors.New(fmt.Sprintf("Unable to load listener-specific TLS cert / key: %s", err.Error())))
					FatalAbort(false, -1)
				}
			}

			// Build the TLS listener configuration
			tlsconfig := &tls.Config{
				Certificates: []tls.Certificate{tlscer},
				MinVersion: 0, // TLS 1.0
				MaxVersion: 0, // TLS 1.2 or greater (latest version)
			}

			// Wrap listener with tls listener
			(*Li).Socket = tls.NewListener((*Li).Socket, tlsconfig)

			AppLogger(errors.New(fmt.Sprintf("Wrapped %s %s/%s with TLS", (*Li).Host, (*Li).Proto, (*Li).Port)))
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

				if stopaccept == true { // Check this flag
					stopacceptmutex.RLock() // lock and check the flag again
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
				}

				if err != nil {
					netErr, ok := err.(net.Error)
					// If this was a timeout just keep going
					if ((ok == true) && (netErr.Timeout() == true) && (netErr.Temporary() == true)) {
						continue;
					} else {
						AppLogger(errors.New(fmt.Sprintf("Error accepting: %s", err.Error())))
					}
				} else {
					// At this point we've accepted a new connection
					// so fill out the connection information struct
					// and hand it off into the worker pipeline
					// for appropriate processing
					Ci := *new(ConnInfo)
					Ci.Host = (*Li).Host
					Ci.Port = (*Li).Port
					Ci.Proto = (*Li).Proto
					Ci.App = (*Li).App
					Ci.TLS = (*Li).TLS
					Ci.Conn = connection
					Ci.Time = getTime()

					QueueRead(Ci)
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

		AppLogger(errors.New("Shutting down listening sockets"))
		for i, _ := range ListenList {
			Li := &(ListenList[i])

			// This will unblock any call to Accept() on this socket
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

	// The goal here is to try to stop the readers / workers
	// and close out the log file so that data isn't lost
	// and the log file is left consistent

	AppLogger(errors.New("Stopping readers"))
	err := StopReaders(*NReaders)
	AppLogger(err)

	AppLogger(errors.New("Stopping workers"))
	err = StopWorkers(*NWorkers)
	AppLogger(err)

	AppLogger(errors.New("Flushing logs and closing the log file"))
	err = StopLogger()
	AppLogger(err)

	if Daemonized == true {
		AppLogger(errors.New("Releasing lock on PID file"))
		err = syscall.Flock(int(PidFile.Fd()), syscall.LOCK_UN)
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


func Fullpath(filename string) (string, error) {

	// Bail out if the filename string is empty
	if len(filename) == 0 {
		return "", os.ErrInvalid
	}

	// If this is already an absolute path return it
	if filepath.IsAbs(filename) == true {
		return filename, nil
	}

	// Get an absolute path by joining with our working dir
	fullpath, err := filepath.Abs(filepath.Join(pathWorkingDir, filename))
	if err != nil {
		return "", err
	}

	// Optionally we could also call EvalSymlinks to get the real path
	// but I don't think that's needed here

	return fullpath, nil
}


func ResolvePaths() error {

	// Resolve the working dir first since it gets used by others
	var err error
	pathWorkingDir, err = filepath.Abs(*FlpathWorkingDir)
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to resolve working-dir path: %s", err.Error()))
	}

	pathTLSCert, err = Fullpath(*FlpathTLSCert)
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to resolve tls-cert path: %s", err.Error()))
	}

	pathTLSKey, err = Fullpath(*FlpathTLSKey)
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to resolve tls-key path: %s", err.Error()))
	}

	pathLogDir, err = Fullpath(*FlpathLogDir)
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to resolve log-dir path: %s", err.Error()))
	}

	pathHTTPTemp, err = Fullpath(*FlpathHTTPTemp)
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to resolve http-template path: %s", err.Error()))
	}

	pathPIDFile, err = Fullpath(*FlpathPIDFile)
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to resolve pid-file path: %s", err.Error()))
	}

	// The config file is allowed to be blank
	if *FlpathConfig != "" {
		pathConfigFile, err = Fullpath(*FlpathConfig)
		if err != nil {
			return errors.New(fmt.Sprintf("Unable to resolve config file path: %s", err.Error()))
		}
	} else {
		pathConfigFile = ""
	}

	return nil;
}


func LoadConfig(filename string) error {

	// If the config file name is blank skip
	if filename == "" {
		return nil
	}

	conf := Config{} // Start with a black configurationt that will get filled out

	confFh, err := os.Open(filename)
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to open config file: %s", err.Error()))
	}

	jsonDecoder := json.NewDecoder(confFh)
	err = jsonDecoder.Decode(&conf) // This populate the config struct
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to parse config file: %s", err.Error()))
	}

	// Let the -D flag still work
	if conf.Daemonize == true {
		Daemonize        = &(conf.Daemonize)
	}

	// Let the --use-localtime flag still work
	if conf.UseLocaltime == true {
	        UseLocaltime     = &(conf.UseLocaltime)
	}

	// Allow not specifying workers in config not to
	// override default or cmdline param
	if conf.Workers > 0 {
		NWorkers         = &(conf.Workers)
	}

	// Allow not specifying readers in config not to
	// override default or cmdline param
	if conf.Readers > 0 {
		NReaders         = &(conf.Readers)
	}

	if conf.LogBufferLen >= 0 {
		LogChanLen         = &(conf.LogBufferLen)
	}

	if conf.ReaderQueueLen >= 0 {
		ReadChanLen         = &(conf.ReaderQueueLen)
	}

	if conf.WorkerQueueLen >= 0 {
		WorkChanLen         = &(conf.WorkerQueueLen)
	}

	// Allow client errors flag to work on cmdline if not in config
	if conf.LogClientErrors == true {
		LogClientErrors = &(conf.LogClientErrors)
	}

	// Allow not specifying the client read timeout
	// override default or cmdline param
	if conf.ClientReadTimeout > 0 {
		ClientReadTimeout = &(conf.ClientReadTimeout)
	}

	// Allow not specifying the client write timeout
	// override default or cmdline param
	if conf.ClientWriteTimeout > 0 {
		ClientWriteTimeout = &(conf.ClientWriteTimeout)
	}

	// Now copy any non-blank / non-nil values to our flag vars
	if conf.DaemonEnvVar != "" {
		DaemonEnvVar     = &(conf.DaemonEnvVar)
	}
	if conf.LogPrefix != "" {
		LogBaseName      = &(conf.LogPrefix)
	}
	if conf.WorkingDirectory != "" {
		FlpathWorkingDir = &(conf.WorkingDirectory)
	}
	if conf.LogDirectory != "" {
		FlpathLogDir     = &(conf.LogDirectory)
	}
	if conf.HTTPTemplate != "" {
		FlpathHTTPTemp   = &(conf.HTTPTemplate)
	}
	if conf.PIDFile != "" {
		FlpathPIDFile    = &(conf.PIDFile)
	}
	if conf.TLSCert != "" {
		FlpathTLSCert    = &(conf.TLSCert)
	}
	if conf.TLSKey != "" {
		FlpathTLSKey     = &(conf.TLSKey)
	}

	// If a listen list was specified, override built in list
	if len(conf.ListenList) > 0 {
		ListenList = conf.ListenList
	}

	// Now resolve the paths using the new parameters
	err = ResolvePaths()
	if err != nil {
		return errors.New(fmt.Sprintf("[config file] %s", err.Error()))
	}

	return nil
}

func DaemonizeProc() (*int, error) {
	pidlockchan := make(chan error, 1)
	pipeerrchan := make(chan error, 1)

	// The better option here is to use os.LookupEnv
	// which returns both the variable value and whether or not
	// the var existed at all but that was added with Go 1.5
	// so for the sake of backwards compatability we'll go with
	// os.Getenv instead
	// _, daemonVarExists := os.LookupEnv(*DaemonEnvVar)
	daemonVarExists := false
	if os.Getenv(*DaemonEnvVar) != "" {
		daemonVarExists = true
	}

	if daemonVarExists == true {
		// We're the started daemon
		Daemonized = true

		// Unset the env var
		err := os.Unsetenv(*DaemonEnvVar)
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

		// Report the PID that we got
		AppLogger(errors.New(fmt.Sprintf("Daemon got a PID of %d", pid)))

		// Now open our PID file, get a lock, and write our PID to it
		PidFile, err = os.OpenFile(pathPIDFile, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644)
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
		PidFile, err = os.OpenFile(pathPIDFile, os.O_RDONLY|os.O_CREATE, 0644)
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

		// Start new process with cwd of /
		attrs.Dir = "/"

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

		attrs.Files = []uintptr{f_devnull.Fd(), f_devnull.Fd(), f_devnull.Fd(), pipew.Fd()}

		// Tell the next process it's the deamon
		os.Setenv(*DaemonEnvVar, "true")

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


func getTime() time.Time {
	now := time.Now()

	// Use the timezone offset to compute UTC time unless we're supposed to log in localtime
	if (*UseLocaltime) == false {
		now = now.UTC()
	}

	return now
}


func getInstanceName() (string) {

	sinkHost, err := os.Hostname()
	if err != nil {
		AppLogger(errors.New(fmt.Sprintf("Unable to get local hostname: %s", err.Error())))

		return PROGNAME
	} else {
		return fmt.Sprintf("%s-%s", PROGNAME, sinkHost)
	}
}
