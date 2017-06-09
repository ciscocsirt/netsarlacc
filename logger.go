package main

import (
        "log"
        "log/syslog"
        "fmt"
        "os"
        "io"
        "time"
	"sync"
	"errors"
	"path/filepath"
	"syscall"
)


func AppLogger(err error) {

	// Allow the passing of nil errors that we ignore
	if err == nil {
		return
	}

	// Send this error to stdout
	fmt.Fprintln(os.Stderr, err.Error())

	// Now send this error to syslog
        logwriter, e := syslog.New(syslog.LOG_CRIT, PROGNAME)
        if e == nil {
                log.SetOutput(logwriter)
		log.Print(err)
        } else {
		fmt.Fprintln(os.Stderr, "Unable to send error to SYSLOG: " + e.Error())
	}
}


func getFileName() string {
	now := time.Now()
	return filepath.Join(pathLogDir, fmt.Sprintf("%s-%s.log", *LogBaseName, now.Format("2006-01-02-15-04-05")))
}


func writeLogger(Logchan chan string) {
        //variables
        var logFile *os.File
	var err error
	newfilemutex := &sync.Mutex{}
	LogRotstopchan := make(chan bool, 1)
	LogRotstopedchan := make(chan bool, 1)

        //ticker and file rotation goroutine
        ticker := time.NewTicker(time.Minute * 10)
        go func() {
		newfilemutex.Lock()
        	filename := getFileName()

        	// create inital file
        	logFile, err = os.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0644)
        	if err != nil {
                	AppLogger(err)
			FatalAbort(false, -1)
       		}

		// Get an OS-level advisory lock on this file to prevent
		// accidental stomping by a second instance of netsarlacc
		err := syscall.Flock(int(logFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err != nil {
			AppLogger(errors.New(fmt.Sprintf("Unable to acquire lock on log file: %s", err.Error())))
			FatalAbort(false, -1)
		}

		newfilemutex.Unlock()

                for {
			select {
			case <-ticker.C:
				newfilemutex.Lock()

				// release the file lock
				err = syscall.Flock(int(logFile.Fd()), syscall.LOCK_UN)
				if err != nil {
					AppLogger(errors.New(fmt.Sprintf("Unable to release lock on log file: %s", err.Error())))
					FatalAbort(false, -1)
				}

				logFile.Close()

				// Get the name of a new file
				filename := getFileName()

				logFile, err = os.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0644)
				if err != nil {
					AppLogger(err)
					FatalAbort(false, -1)
				}

				// Get a new file lock
				err := syscall.Flock(int(logFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
				if err != nil {
					AppLogger(errors.New(fmt.Sprintf("Unable to acquire lock on log file: %s", err.Error())))
					FatalAbort(false, -1)
				}

				newfilemutex.Unlock()

			case <-LogRotstopchan:
				LogRotstopedchan <- true
				return
			}
		}
        }()

	// This is the main loop that grabs logs and writes them to a file
        for l := range Logchan {
		newfilemutex.Lock()
		_, err := io.WriteString(logFile, l + "\n")
		if err != nil {
			AppLogger(err)
			FatalAbort(false, -1)
		}
		newfilemutex.Unlock()
	}

	// We need to stop the ticker and then abort the goroutine
	ticker.Stop()
	LogRotstopchan <- true

	select {
	case <-LogRotstopedchan:
		break
	case <-time.After(time.Second * 5):
		AppLogger(errors.New("Timed out waiting for log rotation goroutine to stop!"))
		break
	}

	// release the lock on the final log file
	err = syscall.Flock(int(logFile.Fd()), syscall.LOCK_UN)
	if err != nil {
		AppLogger(errors.New(fmt.Sprintf("Unable to release lock on log file: %s", err.Error())))
		FatalAbort(false, -1)
	}

	// Close out the current log file
	err = logFile.Close()

	if err != nil {
		AppLogger(err)
		FatalAbort(false, -1)
	}

	// Notify the main routine that we've finished
	Logstopchan <- true
}
