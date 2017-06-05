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
)


func AppLogger(err error) {

	// Allow the passing of nil errors that we ignore
	if err == nil {
		return
	}

	// Send this error to stdout
	fmt.Fprintln(os.Stderr, err.Error())

	// Now send this error to syslog
        logwriter, e := syslog.New(syslog.LOG_CRIT, "netsarlacc")
        if e == nil {
                log.SetOutput(logwriter)
		log.Print(err)
        } else {
		fmt.Fprintln(os.Stderr, "Unable to send error to SYSLOG: " + e.Error())
	}
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
        	//get current datetime and creates filename based on current time
		newfilemutex.Lock()
        	now := time.Now()
        	filename := ("sinkhole-" + now.Format("2006-01-02-15-04-05") + ".log")

        	//create inital file
        	logFile, err = os.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0666)
        	if err != nil {
                	AppLogger(err)
			FatalAbort(false, -1)
       		}
		newfilemutex.Unlock()

                for {
			select {
			case <-ticker.C:
				newfilemutex.Lock()
				logFile.Close()
				//get current datetime and creates filename based on current time
				now := time.Now()
				filename := ("sinkhole-" + now.Format("2006-01-02-15-04-05") + ".log")
				logFile, err = os.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0666)
				if err != nil {
					AppLogger(err)
					FatalAbort(false, -1)
				}
				newfilemutex.Unlock()

			case <-LogRotstopchan:
				LogRotstopedchan <- true
				return
			}
		}
        }()

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

	// Close out the current log file
	err = logFile.Close()

	if err != nil {
		AppLogger(err)
		FatalAbort(false, -1)
	}

	// Notify the main routine that we've finished
	Logstopchan <- true
}
