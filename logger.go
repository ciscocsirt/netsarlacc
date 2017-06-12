package main

import (
        "log"
        "log/syslog"
        "fmt"
        "os"
        "time"
	"sync"
	"errors"
	"path/filepath"
	"syscall"
)


var (
	Logchan chan []byte
	Logstopchan = make(chan bool, 1)
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


func getFileName(now time.Time) string {

	// This could be called with a timestamp not on a min % 10 == 0 boundary
	// so we need to subtract off some minutes.
	// The reason not to use Truncate() here is that truncate operates on absolute
	// time and in our given timezone there could be a shift by
	// 15 minutes which could interfere with the % 10 == 0 math.
	now = now.Add(-(time.Duration(now.Minute() % 10) * time.Minute))

	return filepath.Join(pathLogDir, fmt.Sprintf("%s-%s.log", *LogBaseName, now.Format("2006-01-02-15-04-MST")))
}


func queueLog(logbytes []byte) {
	Logchan <- logbytes
}


func writeLogger(logbuflen int) {
        //variables
        var logFile *os.File
	var err error

	newfilemutex := &sync.Mutex{}

	Logchan = make(chan []byte, logbuflen) // Allocate our main logging channel
	LogRotstopchan := make(chan bool, 1)
	LogRotstopedchan := make(chan bool, 1)

	// Make a ticker channel that wakes us up to check if we need to rotate the log
	ticker := time.NewTicker(time.Millisecond * 250) // four times a second should be often enough
        go func() {

		// This is the first time so no file is open yet so
		// we'll open a file and store it's name and then
		// after that we'll wake up several times a second to
		// see if we need to rotate the filename yet

		newfilemutex.Lock()
        	curfilename := getFileName(getTime())

        	// create inital file
        	logFile, err = os.OpenFile(curfilename, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
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

				// We're going to rotate every 10
				// minutes so let's check that we're
				// at (min % 10 == 0 and sec < 2) We do
				// sec < 2 to give us two seconds to
				// catch a new 10 minute transition in
				// the off case CPU is that badly
				// loaded
				now := getTime()
				if ((now.Minute() % 10 == 0) && (now.Second() < 2)) {
					newfilename := getFileName(now)

					// Now if the new file name doesn't match the current one
					// we'll need to rotate
					if newfilename != curfilename {
						curfilename = newfilename // We're going to rotate so store this name

						newfilemutex.Lock()

						// release the file lock
						err = syscall.Flock(int(logFile.Fd()), syscall.LOCK_UN)
						if err != nil {
							AppLogger(errors.New(fmt.Sprintf("Unable to release lock on log file: %s", err.Error())))
							FatalAbort(false, -1)
						}

						logFile.Close()

						logFile, err = os.OpenFile(curfilename, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
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
					}
				}
			case <-LogRotstopchan:
				LogRotstopedchan <- true
				return
			}
		}
        }()

	// This is the main loop that grabs logs and writes them to a file
        for l := range Logchan {
		newfilemutex.Lock()
		_, err := logFile.Write(l)
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


func StopLogger() error {

	// This will end the for ... range Logchan 
	close(Logchan)

	// Now wait to be told that everything stopped
	select {
	case <-Logstopchan:
		break
	case <-time.After(time.Second * 5):
		return errors.New("Timed out waiting for log flushing and closing!")
	}

	return nil
}
